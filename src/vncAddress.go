package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/ssh"
)

func getVNCAddress(c *gin.Context) {
	// 获取 deviceId 路径参数
	deviceId := c.Param("id")
	// 获取 itemName 查询参数
	itemName := c.Query("itemName")
	if deviceId == "" || itemName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 deviceId 或 itemName"})
		return
	}

	// 查找 config
	var config *CSMPDevice
	for _, cfg := range csmpDevices {
		if fmt.Sprintf("%v", cfg.ID) == deviceId {
			config = &cfg
			break
		}
	}
	if config == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "设备配置不存在"})
		return
	}

	// 准备多种认证方法
	authMethods := []ssh.AuthMethod{
		ssh.Password(config.SSHPass),
	}
	// SSH连接配置
	sshConfig := &ssh.ClientConfig{
		User:            config.SSHUser,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
	}

	// 连接 SSH
	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%s", config.SSHHost, config.SSHPort), sshConfig)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "SSH connection failed"})
		return
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "SSH session failed"})
		return
	}
	defer session.Close()

	cmd := fmt.Sprintf("virsh list --all --name | grep . | while read vm; do virsh dumpxml \"$vm\" | grep -q \"<nova:name>%s</nova:name>\" && echo \"$vm\"; done", itemName)
	output, err := session.CombinedOutput(cmd)
	if err != nil && len(output) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get VM name failed"})
		return
	}

	session2, err := client.NewSession()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "SSH session failed"})
		return
	}
	defer session2.Close()
	cmd = fmt.Sprintf("virsh vncdisplay %s", strings.TrimSpace(string(output)))
	output, err = session2.CombinedOutput(cmd)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get VNC display failed"})
		return
	}

	vncAddress := config.SSHHost + strings.TrimSpace(string(output))
	c.JSON(http.StatusOK, gin.H{
		"address": vncAddress,
		"pass":    config.VNCPass,
	})
}
