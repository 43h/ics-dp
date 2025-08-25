package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
)

// SSH会话结构
type SSHSession struct {
	Client        *ssh.Client
	Session       *ssh.Session
	StdinPipe     io.WriteCloser
	StdoutPipe    io.Reader
	StderrPipe    io.Reader
	Config        *CSMPDevice
	CreatedAt     time.Time
	LastUsed      time.Time
	isActive      bool
	WebSocketConn *websocket.Conn
}

// WebShell连接请求
type WebShellConnectRequest struct {
	DeviceID   int    `json:"device_id"`
	DeviceName string `json:"device_name"`
	Host       string `json:"host"`
	User       string `json:"user"`
	Pass       string `json:"pass"`
	Port       string `json:"port"`
}

// WebShell命令请求
type WebShellCommandRequest struct {
	SessionID string `json:"session_id"`
	Command   string `json:"command"`
}

var sshSessions = make(map[string]*SSHSession)
var sshSessionsMutex sync.RWMutex

// WebSocket升级器
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有源，生产环境中应该限制
	},
}

// WebShell WebSocket处理
func handleWebShellWebSocket(c *gin.Context) {
	deviceIdStr := c.Query("device_id")
	if deviceIdStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少设备ID"})
		return
	}

	// 获取设备配置
	var config *CSMPDevice
	for _, cfg := range csmpDevices {
		if fmt.Sprintf("%d", cfg.ID) == deviceIdStr {
			config = &cfg
			break
		}
	}

	if config == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "设备配置不存在"})
		return
	}

	// 升级到WebSocket连接
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket升级失败: %v", err)
		return
	}
	defer conn.Close()

	// 建立SSH连接
	sshSession, err := createSSHSession(config, conn)
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("SSH连接失败: %v", err)))
		return
	}
	defer closeSSHSession(sshSession)

	// 生成会话ID并保存
	sessionID := fmt.Sprintf("ws_%s_%d", deviceIdStr, time.Now().Unix())
	sshSessionsMutex.Lock()
	sshSessions[sessionID] = sshSession
	sshSessionsMutex.Unlock()

	// 发送连接成功消息
	conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("WebShell连接成功 - %s\r\n", config.SSHHost)))

	// 启动数据转发
	go handleSSHOutput(sshSession, conn)
	handleWebSocketInput(sshSession, conn)

	// 清理会话
	sshSessionsMutex.Lock()
	delete(sshSessions, sessionID)
	sshSessionsMutex.Unlock()
}

// 创建SSH会话
func createSSHSession(config *CSMPDevice, wsConn *websocket.Conn) (*SSHSession, error) {
	if config.SSHPort == "" {
		config.SSHPort = "22"
	}

	// SSH认证方法
	authMethods := []ssh.AuthMethod{
		ssh.Password(config.SSHPass),
		ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
			answers := make([]string, len(questions))
			for i := range answers {
				answers[i] = config.SSHPass
			}
			return answers, nil
		}),
	}

	// SSH客户端配置
	sshConfig := &ssh.ClientConfig{
		User:            config.SSHUser,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
	}

	// 连接SSH服务器
	address := fmt.Sprintf("%s:%s", config.SSHHost, config.SSHPort)
	client, err := ssh.Dial("tcp", address, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("SSH连接失败: %v", err)
	}

	// 创建SSH会话
	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("创建SSH会话失败: %v", err)
	}

	// 设置终端模式（减少控制序列和回显问题）
	modes := ssh.TerminalModes{
		ssh.ECHO:          0, // 禁用回显，避免双重显示
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
		ssh.ICANON:        1,
		ssh.ISIG:          1,
		ssh.ICRNL:         1,
		ssh.OPOST:         1,
		ssh.VERASE:        127, // 设置删除键为DEL (0x7F)
		ssh.VKILL:         21,  // Ctrl+U
		ssh.VEOF:          4,   // Ctrl+D
		ssh.VINTR:         3,   // Ctrl+C
		ssh.VQUIT:         28,  // Ctrl+\
		ssh.VSTART:        17,  // Ctrl+Q
		ssh.VSTOP:         19,  // Ctrl+S
	}

	// 请求PTY（伪终端）- 使用简单的终端类型减少控制序列
	if err := session.RequestPty("dumb", 80, 24, modes); err != nil {
		session.Close()
		client.Close()
		return nil, fmt.Errorf("请求伪终端失败: %v", err)
	}

	// 获取输入输出管道
	stdinPipe, err := session.StdinPipe()
	if err != nil {
		session.Close()
		client.Close()
		return nil, fmt.Errorf("获取输入管道失败: %v", err)
	}

	stdoutPipe, err := session.StdoutPipe()
	if err != nil {
		stdinPipe.Close()
		session.Close()
		client.Close()
		return nil, fmt.Errorf("获取输出管道失败: %v", err)
	}

	stderrPipe, err := session.StderrPipe()
	if err != nil {
		stdinPipe.Close()
		session.Close()
		client.Close()
		return nil, fmt.Errorf("获取错误输出管道失败: %v", err)
	}

	// 设置环境变量来减少控制序列
	session.Setenv("TERM", "dumb")              // 使用简单终端类型
	session.Setenv("PS1", "$ ")                 // 简单的提示符
	session.Setenv("COLORTERM", "")             // 禁用颜色
	session.Setenv("HISTCONTROL", "ignoredups") // 历史命令去重
	session.Setenv("PAGER", "cat")              // 禁用分页器
	session.Setenv("EDITOR", "nano")            // 设置简单编辑器

	// 启动shell
	if err := session.Shell(); err != nil {
		stdinPipe.Close()
		session.Close()
		client.Close()
		return nil, fmt.Errorf("启动shell失败: %v", err)
	}

	return &SSHSession{
		Client:        client,
		Session:       session,
		StdinPipe:     stdinPipe,
		StdoutPipe:    stdoutPipe,
		StderrPipe:    stderrPipe,
		Config:        config,
		CreatedAt:     time.Now(),
		LastUsed:      time.Now(),
		isActive:      true,
		WebSocketConn: wsConn,
	}, nil
}

// 处理SSH输出并转发到WebSocket
func handleSSHOutput(sshSession *SSHSession, wsConn *websocket.Conn) {
	// 处理stdout
	go func() {
		buffer := make([]byte, 1024)
		for sshSession.isActive {
			n, err := sshSession.StdoutPipe.Read(buffer)
			if err != nil {
				if err != io.EOF {
					log.Printf("读取SSH stdout失败: %v", err)
				}
				break
			}
			if n > 0 {
				// 添加小的延迟避免消息过于频繁
				time.Sleep(10 * time.Millisecond)
				if err := wsConn.WriteMessage(websocket.TextMessage, buffer[:n]); err != nil {
					log.Printf("发送WebSocket消息失败: %v", err)
					break
				}
			}
		}
	}()

	// 处理stderr
	go func() {
		buffer := make([]byte, 1024)
		for sshSession.isActive {
			n, err := sshSession.StderrPipe.Read(buffer)
			if err != nil {
				if err != io.EOF {
					log.Printf("读取SSH stderr失败: %v", err)
				}
				break
			}
			if n > 0 {
				// 添加小的延迟避免消息过于频繁
				time.Sleep(10 * time.Millisecond)
				if err := wsConn.WriteMessage(websocket.TextMessage, buffer[:n]); err != nil {
					log.Printf("发送WebSocket错误消息失败: %v", err)
					break
				}
			}
		}
	}()
}

// 处理WebSocket输入并转发到SSH
func handleWebSocketInput(sshSession *SSHSession, wsConn *websocket.Conn) {
	for sshSession.isActive {
		_, message, err := wsConn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket读取错误: %v", err)
			}
			break
		}

		sshSession.LastUsed = time.Now()

		// 将WebSocket消息转发到SSH输入
		if _, err := sshSession.StdinPipe.Write(message); err != nil {
			log.Printf("写入SSH stdin失败: %v", err)
			break
		}
	}
}

// 关闭SSH会话
func closeSSHSession(sshSession *SSHSession) {
	if sshSession == nil {
		return
	}

	sshSession.isActive = false

	if sshSession.StdinPipe != nil {
		sshSession.StdinPipe.Close()
	}
	if sshSession.Session != nil {
		sshSession.Session.Close()
	}
	if sshSession.Client != nil {
		sshSession.Client.Close()
	}
}

func executeCommand(c *gin.Context) {
	var req ExecuteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var config *CSMPDevice
	for _, cfg := range csmpDevices {
		if cfg.ID == req.ConfigID {
			config = &cfg
			break
		}
	}

	if config == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "配置未找到"})
		return
	}

	// SSH连接并执行命令
	result, err := executeSSHCommand(config, req.Command)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "命令执行失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"item_id": req.ItemID,
		"command": req.Command,
		"result":  result,
		"status":  "success",
	})
}

func executeSSHCommand(config *CSMPDevice, command string) (string, error) {
	// 验证SSH配置
	if config.SSHHost == "" {
		return "", fmt.Errorf("SSH主机地址不能为空")
	}
	if config.SSHUser == "" {
		return "", fmt.Errorf("SSH用户名不能为空")
	}
	if config.SSHPass == "" {
		return "", fmt.Errorf("SSH密码不能为空")
	}
	if config.SSHPort == "" {
		config.SSHPort = "22" // 默认端口
	}

	// 准备多种认证方法
	authMethods := []ssh.AuthMethod{
		ssh.Password(config.SSHPass),
	}

	// 如果可能，添加键盘交互认证
	authMethods = append(authMethods, ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
		answers := make([]string, len(questions))
		for i := range answers {
			answers[i] = config.SSHPass
		}
		return answers, nil
	}))

	// SSH连接配置
	sshConfig := &ssh.ClientConfig{
		User:            config.SSHUser,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
	}

	// 构建连接地址
	address := config.SSHHost + ":" + config.SSHPort

	// 连接SSH
	conn, err := ssh.Dial("tcp", address, sshConfig)
	if err != nil {
		return "", fmt.Errorf("SSH连接失败 (%s): %v", address, err)
	}
	defer conn.Close()

	// 创建会话
	session, err := conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("创建SSH会话失败: %v", err)
	}
	defer session.Close()

	// 设置会话模式
	modes := ssh.TerminalModes{
		ssh.ECHO:          0,     // disable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}

	// 请求伪终端（对某些命令可能需要）
	if err := session.RequestPty("xterm", 80, 40, modes); err != nil {
		// 如果无法获取PTY，继续执行（某些命令不需要PTY）
	}

	// 执行命令
	output, err := session.Output(command)
	if err != nil {
		// 如果命令执行失败，尝试获取错误输出
		if exitError, ok := err.(*ssh.ExitError); ok {
			return string(output), fmt.Errorf("命令执行失败 (退出码: %d): %s", exitError.ExitStatus(), string(output))
		}
		return string(output), fmt.Errorf("命令执行失败: %v", err)
	}

	return string(output), nil
}