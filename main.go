package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
)

type Config struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	LoginURL string `json:"login_url"`
	DataURL  string `json:"data_url"`
	Username string `json:"username"`
	Password string `json:"password"`
	SSHHost  string `json:"ssh_host"`
	SSHUser  string `json:"ssh_user"`
	SSHPass  string `json:"ssh_pass"`
	SSHPort  string `json:"ssh_port"`
}

type ListItem struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	URL           string `json:"url"`
	Status        string `json:"status"`
	CanExecute    bool   `json:"can_execute"`
	ComponentType string `json:"component_type"`
	IPAddress     string `json:"ip_address"`
}

type ExecuteRequest struct {
	ConfigID int    `json:"config_id"`
	ItemID   string `json:"item_id"`
	Command  string `json:"command"`
}

// SSH会话结构
type SSHSession struct {
	Client        *ssh.Client
	Session       *ssh.Session
	StdinPipe     io.WriteCloser
	StdoutPipe    io.Reader
	StderrPipe    io.Reader
	Config        *Config
	CreatedAt     time.Time
	LastUsed      time.Time
	mutex         sync.Mutex
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

var configs []Config
var sessions = make(map[string]*http.Client)
var sshSessions = make(map[string]*SSHSession)
var sshSessionsMutex sync.RWMutex
var configFile = "configs.json"

// WebSocket升级器
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有源，生产环境中应该限制
	},
}

// 加载配置文件
func loadConfigs() error {
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		// 如果文件不存在，创建默认配置
		configs = []Config{}
		return saveConfigs()
	}

	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &configs)
}

// 保存配置到文件
func saveConfigs() error {
	data, err := json.MarshalIndent(configs, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(configFile, data, 0644)
}

// 获取下一个可用的ID
func getNextConfigID() int {
	maxID := 0
	for _, config := range configs {
		if config.ID > maxID {
			maxID = config.ID
		}
	}
	return maxID + 1
}

func main() {
	// 加载配置文件
	if err := loadConfigs(); err != nil {
		fmt.Printf("加载配置文件失败: %v\n", err)
		// 继续运行，使用空配置
		configs = []Config{}
	}

	r := gin.Default()

	// 配置CORS
	config := cors.DefaultConfig()
	config.AllowAllOrigins = true
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	config.AllowHeaders = []string{"Origin", "Content-Type", "Accept", "Authorization"}
	r.Use(cors.New(config))

	// 静态文件服务
	r.Static("/static", "./static")
	r.LoadHTMLGlob("templates/*")

	// 主页
	r.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", nil)
	})

	// WebShell页面
	r.GET("/webshell", func(c *gin.Context) {
		c.HTML(http.StatusOK, "webshell.html", nil)
	})

	// API 路由
	api := r.Group("/api")
	{
		api.GET("/configs", getConfigs)
		api.POST("/configs", createConfig)
		api.PUT("/configs/:id", updateConfig)
		api.DELETE("/configs/:id", deleteConfig)
		api.POST("/login", login)
		api.GET("/scrape/:id", scrapeData)
		api.POST("/execute", executeCommand)
		api.POST("/test-ssh", testSSHConnection)

		// WebShell WebSocket API
		api.GET("/webshell/ws", handleWebShellWebSocket)
	}

	fmt.Println("服务器运行在 http://localhost:8080")
	r.Run(":8080")
}

func getConfigs(c *gin.Context) {
	c.JSON(http.StatusOK, configs)
}

func createConfig(c *gin.Context) {
	var config Config
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	config.ID = getNextConfigID()
	configs = append(configs, config)

	// 保存到文件
	if err := saveConfigs(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存配置失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, config)
}

func updateConfig(c *gin.Context) {
	id := c.Param("id")
	var updatedConfig Config
	if err := c.ShouldBindJSON(&updatedConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	for i, config := range configs {
		if fmt.Sprintf("%d", config.ID) == id {
			updatedConfig.ID = config.ID
			configs[i] = updatedConfig

			// 保存到文件
			if err := saveConfigs(); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "保存配置失败: " + err.Error()})
				return
			}

			c.JSON(http.StatusOK, updatedConfig)
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "配置未找到"})
}

func deleteConfig(c *gin.Context) {
	id := c.Param("id")
	for i, config := range configs {
		if fmt.Sprintf("%d", config.ID) == id {
			configs = append(configs[:i], configs[i+1:]...)

			// 保存到文件
			if err := saveConfigs(); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "保存配置失败: " + err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"message": "配置已删除"})
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "配置未找到"})
}

func login(c *gin.Context) {
	var loginData struct {
		ConfigID int `json:"config_id"`
	}

	if err := c.ShouldBindJSON(&loginData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var config *Config
	for _, cfg := range configs {
		if cfg.ID == loginData.ConfigID {
			config = &cfg
			break
		}
	}

	if config == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "配置未找到"})
		return
	}

	// 创建HTTP客户端并保持会话，跳过TLS证书验证
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			DisableCompression: true, // 禁用压缩以更好处理chunked编码
		},
	}

	// 清理和验证登录URL
	loginURL := strings.TrimSpace(config.LoginURL)
	if loginURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "登录URL不能为空"})
		return
	}

	// 验证URL格式
	parsedURL, err := url.Parse(loginURL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "登录URL格式无效: " + err.Error()})
		return
	}

	// 确保URL有协议
	if parsedURL.Scheme == "" {
		loginURL = "http://" + loginURL
		parsedURL, err = url.Parse(loginURL)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无法修复URL格式: " + err.Error()})
			return
		}
	}

	// 模拟登录过程
	data := url.Values{}
	data.Set("username", config.Username)
	data.Set("password", config.Password)

	resp, err := client.PostForm(loginURL, data)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "登录失败: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	// 保存会话
	sessionKey := fmt.Sprintf("config_%d", config.ID)
	sessions[sessionKey] = client

	c.JSON(http.StatusOK, gin.H{"message": "登录成功", "session_key": sessionKey})
}

func scrapeData(c *gin.Context) {
	configIDStr := c.Param("id")
	sessionKey := fmt.Sprintf("config_%s", configIDStr)

	client, exists := sessions[sessionKey]
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}

	var config *Config
	for _, cfg := range configs {
		if fmt.Sprintf("%d", cfg.ID) == configIDStr {
			config = &cfg
			break
		}
	}

	if config == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "配置未找到"})
		return
	}

	// 清理和验证数据URL
	dataURL := strings.TrimSpace(config.DataURL)
	if dataURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "数据URL不能为空"})
		return
	}

	// 验证URL格式
	parsedDataURL, err := url.Parse(dataURL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "数据URL格式无效: " + err.Error()})
		return
	}

	// 确保URL有协议
	if parsedDataURL.Scheme == "" {
		dataURL = "http://" + dataURL
		parsedDataURL, err = url.Parse(dataURL)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无法修复数据URL格式: " + err.Error()})
			return
		}
	}

	// 抓取数据页面
	fmt.Printf("=== 数据请求调试 ===\n")
	fmt.Printf("请求URL: %s\n", dataURL)
	fmt.Printf("会话存在: %v\n", client != nil)

	// 创建请求
	req, err := http.NewRequest("GET", dataURL, nil)
	if err != nil {
		fmt.Printf("创建请求失败: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建请求失败: " + err.Error()})
		return
	}

	// 设置请求头以处理chunked编码
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json, text/html, */*")
	req.Header.Set("Accept-Encoding", "identity") // 禁用压缩编码
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("请求失败: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "抓取失败: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	// 读取响应体 - 支持chunked编码
	var bodyBytes []byte
	if resp.Header.Get("Transfer-Encoding") == "chunked" {
		fmt.Printf("检测到chunked编码，使用流式读取\n")
		// 对于chunked编码，使用io.ReadAll会自动处理
		bodyBytes, err = io.ReadAll(resp.Body)
		if err != nil {
			fmt.Printf("读取chunked响应体失败: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取chunked响应失败: " + err.Error()})
			return
		}
	} else {
		fmt.Printf("使用标准方式读取响应体\n")
		bodyBytes, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			fmt.Printf("读取响应体失败: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取响应失败: " + err.Error()})
			return
		}
	}

	// 详细的调试输出
	fmt.Printf("=== 数据响应调试 ===\n")
	fmt.Printf("URL: %s\n", dataURL)
	fmt.Printf("响应状态: %s (%d)\n", resp.Status, resp.StatusCode)
	fmt.Printf("响应头Content-Type: %s\n", resp.Header.Get("Content-Type"))
	fmt.Printf("响应头Content-Length: %s\n", resp.Header.Get("Content-Length"))
	fmt.Printf("响应头Transfer-Encoding: %s\n", resp.Header.Get("Transfer-Encoding"))
	fmt.Printf("响应头Connection: %s\n", resp.Header.Get("Connection"))
	fmt.Printf("响应头Set-Cookie: %s\n", resp.Header.Get("Set-Cookie"))

	// 检查是否为chunked编码
	if resp.Header.Get("Transfer-Encoding") == "chunked" {
		fmt.Printf("✓ 检测到chunked传输编码\n")
	} else if resp.Header.Get("Content-Length") != "" {
		fmt.Printf("✓ 使用Content-Length: %s\n", resp.Header.Get("Content-Length"))
	} else {
		fmt.Printf("⚠ 未检测到明确的传输编码方式\n")
	}

	// 打印所有响应头
	fmt.Printf("所有响应头:\n")
	for key, values := range resp.Header {
		for _, value := range values {
			fmt.Printf("  %s: %s\n", key, value)
		}
	}

	fmt.Printf("响应体总长度: %d 字节\n", len(bodyBytes))

	// 分段显示响应体
	if len(bodyBytes) > 2000 {
		fmt.Printf("响应体前1000字符:\n%s\n", string(bodyBytes[:1000]))
		fmt.Printf("响应体后1000字符:\n%s\n", string(bodyBytes[len(bodyBytes)-1000:]))
	} else {
		fmt.Printf("完整响应体:\n%s\n", string(bodyBytes))
	}
	fmt.Printf("===================\n")

	var items []ListItem

	// 检查是否是JSON响应
	contentType := resp.Header.Get("Content-Type")
	fmt.Printf("检查Content-Type: %s\n", contentType)

	if strings.Contains(contentType, "application/json") || strings.Contains(string(bodyBytes), `"data"`) {
		fmt.Printf("识别为JSON响应，开始解析...\n")
		// 解析JSON响应
		var jsonResp map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &jsonResp); err != nil {
			fmt.Printf("JSON解析失败: %v\n", err)
			fmt.Printf("尝试HTML解析...\n")
			// 如果JSON解析失败，尝试HTML解析
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(bodyBytes)))
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "解析失败: " + err.Error()})
				return
			}
			items = parseHTMLTable(doc)
		} else {
			fmt.Printf("JSON解析成功，数据结构:\n")
			// 打印JSON结构概要
			if data, ok := jsonResp["data"]; ok {
				fmt.Printf("找到data字段，类型: %T\n", data)
				if dataMap, ok := data.(map[string]interface{}); ok {
					for key, value := range dataMap {
						fmt.Printf("  data.%s: %T\n", key, value)
						if key == "list" {
							if list, ok := value.([]interface{}); ok {
								fmt.Printf("    list长度: %d\n", len(list))
								if len(list) > 0 {
									fmt.Printf("    第一个元素类型: %T\n", list[0])
								}
							}
						}
					}
				}
			}
			items = parseJSONData(jsonResp)
		}
	} else {
		fmt.Printf("识别为HTML响应，开始解析...\n")
		// 解析HTML
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(bodyBytes)))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "解析失败: " + err.Error()})
			return
		}
		items = parseHTMLTable(doc)
	}

	fmt.Printf("解析完成，共获得 %d 个项目\n", len(items))

	c.JSON(http.StatusOK, items)
}

// 解析JSON数据
func parseJSONData(jsonResp map[string]interface{}) []ListItem {
	var items []ListItem

	// 检查是否有data.list结构
	if data, ok := jsonResp["data"].(map[string]interface{}); ok {
		if list, ok := data["list"].([]interface{}); ok {
			fmt.Printf("找到JSON数据列表，共 %d 个项目\n", len(list))

			for i, item := range list {
				if itemMap, ok := item.(map[string]interface{}); ok {
					// 提取组件信息
					name := ""
					if n, exists := itemMap["name"]; exists {
						name = fmt.Sprintf("%v", n)
					}

					componentType := "未知组件"
					if safeKit, exists := itemMap["safe_kit"].(map[string]interface{}); exists {
						if imageName, exists := safeKit["name"]; exists {
							componentType = fmt.Sprintf("%v", imageName)
						}
					}

					// 提取IP地址
					ipAddress := "未知IP"
					if address, exists := itemMap["address"].(map[string]interface{}); exists {
						if private, exists := address["private"].([]interface{}); exists && len(private) > 0 {
							// 优先使用IPv4地址
							for _, ip := range private {
								ipStr := fmt.Sprintf("%v", ip)
								if !strings.Contains(ipStr, ":") { // 简单判断IPv4
									ipAddress = ipStr
									break
								}
							}
							// 如果没有IPv4，使用第一个IP
							if ipAddress == "未知IP" && len(private) > 0 {
								ipAddress = fmt.Sprintf("%v", private[0])
							}
						}
					}

					// 提取状态
					status := "未知"
					if runStatus, exists := itemMap["run_status"].(map[string]interface{}); exists {
						if text, exists := runStatus["text"]; exists {
							status = fmt.Sprintf("%v", text)
						}
					}

					listItem := ListItem{
						ID:            fmt.Sprintf("component_%d", i+1),
						Title:         name,
						Description:   fmt.Sprintf("类型: %s, IP: %s", componentType, ipAddress),
						URL:           "",
						Status:        mapStatus(status),
						CanExecute:    true,
						ComponentType: componentType,
						IPAddress:     ipAddress,
					}
					items = append(items, listItem)
				}
			}
		}
	}

	return items
}

// 解析HTML表格数据
func parseHTMLTable(doc *goquery.Document) []ListItem {
	var items []ListItem

	// 抓取组件表格数据
	doc.Find("table tbody tr").Each(func(i int, s *goquery.Selection) {
		// 获取表格的各个列
		tds := s.Find("td")
		if tds.Length() >= 8 { // 确保有足够的列
			componentName := strings.TrimSpace(tds.Eq(0).Text())
			componentType := strings.TrimSpace(tds.Eq(1).Text())
			ipAddress := strings.TrimSpace(tds.Eq(6).Text())
			status := strings.TrimSpace(tds.Eq(7).Text())

			// 清理IP地址，移除端口和额外信息
			ipCleaned := cleanIPAddress(ipAddress)

			// 只有组件名称不为空时才添加
			if componentName != "" {
				item := ListItem{
					ID:            fmt.Sprintf("component_%d", i+1),
					Title:         componentName,
					Description:   fmt.Sprintf("类型: %s, IP: %s", componentType, ipCleaned),
					URL:           "",
					Status:        mapStatus(status),
					CanExecute:    true,
					ComponentType: componentType,
					IPAddress:     ipCleaned,
				}
				items = append(items, item)
			}
		}
	})

	// 如果表格结构不匹配，尝试其他选择器
	if len(items) == 0 {
		// 尝试寻找包含组件名称的元素
		doc.Find("tr").Each(func(i int, s *goquery.Selection) {
			// 跳过表头
			if i == 0 {
				return
			}

			cells := s.Find("td")
			if cells.Length() >= 3 {
				name := strings.TrimSpace(cells.Eq(0).Text())
				ip := strings.TrimSpace(cells.Eq(1).Text())
				status := ""

				if cells.Length() >= 8 {
					ip = strings.TrimSpace(cells.Eq(6).Text())
				}
				if cells.Length() >= 8 {
					status = strings.TrimSpace(cells.Eq(7).Text())
				}

				if name != "" {
					item := ListItem{
						ID:            fmt.Sprintf("item_%d", i),
						Title:         name,
						Description:   fmt.Sprintf("IP: %s", cleanIPAddress(ip)),
						URL:           "",
						Status:        mapStatus(status),
						CanExecute:    true,
						ComponentType: "未知组件",
						IPAddress:     cleanIPAddress(ip),
					}
					items = append(items, item)
				}
			}
		})
	}

	// 如果还是没有找到项目，返回示例数据
	if len(items) == 0 {
		items = []ListItem{
			{
				ID:            "sample_1",
				Title:         "wuzx-vquota-test",
				Description:   "类型: 鉴权组件, IP: 10.1.71.19",
				URL:           "",
				Status:        "错误",
				CanExecute:    true,
				ComponentType: "鉴权组件",
				IPAddress:     "10.1.71.19",
			},
			{
				ID:            "sample_2",
				Title:         "vquota-qax-716",
				Description:   "类型: 鉴权组件, IP: 10.1.71.18",
				URL:           "",
				Status:        "正常",
				CanExecute:    true,
				ComponentType: "鉴权组件",
				IPAddress:     "10.1.71.18",
			},
			{
				ID:            "sample_3",
				Title:         "vwaf-qax-716",
				Description:   "类型: Web应用防护系统, IP: 10.1.71.17",
				URL:           "",
				Status:        "正常",
				CanExecute:    true,
				ComponentType: "Web应用防护系统",
				IPAddress:     "10.1.71.17",
			},
		}
	}

	return items
}

func testSSHConnection(c *gin.Context) {
	var testConfig Config
	if err := c.ShouldBindJSON(&testConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 执行简单的SSH连接测试
	result, err := executeSSHCommand(&testConfig, "echo 'SSH连接测试成功'")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "SSH连接测试成功",
		"result":  result,
	})
}

func executeCommand(c *gin.Context) {
	var req ExecuteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var config *Config
	for _, cfg := range configs {
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

func executeSSHCommand(config *Config, command string) (string, error) {
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

// cleanIPAddress 清理IP地址，移除端口和额外信息
func cleanIPAddress(ipStr string) string {
	if ipStr == "" {
		return ""
	}

	// 移除多余的空格和换行
	ipStr = strings.TrimSpace(ipStr)

	// 处理多个IP的情况，用分号分隔
	ips := strings.Split(ipStr, ";")
	var cleanedIPs []string

	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}

		// 移除括号内的内容 (如端口信息)
		if idx := strings.Index(ip, "("); idx != -1 {
			ip = ip[:idx]
		}

		// 移除端口号
		if idx := strings.LastIndex(ip, ":"); idx != -1 {
			// 检查是否是IPv6地址
			if strings.Count(ip, ":") <= 1 {
				ip = ip[:idx]
			}
		}

		ip = strings.TrimSpace(ip)
		if ip != "" {
			cleanedIPs = append(cleanedIPs, ip)
		}
	}

	return strings.Join(cleanedIPs, "; ")
}

// mapStatus 将页面状态映射为标准状态
func mapStatus(status string) string {
	status = strings.TrimSpace(status)
	switch status {
	case "运行中":
		return "正常"
	case "已停止":
		return "错误"
	case "服务网络检测中":
		return "警告"
	default:
		if strings.Contains(status, "运行") {
			return "正常"
		} else if strings.Contains(status, "停止") || strings.Contains(status, "失败") {
			return "错误"
		} else {
			return "警告"
		}
	}
}

// WebShell WebSocket处理
func handleWebShellWebSocket(c *gin.Context) {
	deviceIdStr := c.Query("device_id")
	if deviceIdStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少设备ID"})
		return
	}

	// 获取设备配置
	var config *Config
	for _, cfg := range configs {
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
func createSSHSession(config *Config, wsConn *websocket.Conn) (*SSHSession, error) {
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
