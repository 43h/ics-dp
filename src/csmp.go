package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/ssh"
)

type VMItem struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

func flushVM(c *gin.Context) {
	deviceId := c.Param("id")

	if deviceId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 deviceId"})
		return
	}

	var config *CSMPDevice
	for i := range csmpDevices {
		if fmt.Sprintf("%v", csmpDevices[i].ID) == deviceId {
			config = &csmpDevices[i]
			break
		}
	}
	if config == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "设备配置不存在"})
		return
	}

	authMethods := []ssh.AuthMethod{ssh.Password(config.SSHPass)}
	sshConfig := &ssh.ClientConfig{
		User:            config.SSHUser,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
	}
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

	// 统一批量获取: id|domain|state|novaName
	remoteCmd := `
bash -c '
for d in $(virsh list --all --name | grep .); do
  id=$(virsh domid "$d" 2>/dev/null || echo -)
  state=$(virsh domstate "$d" 2>/dev/null | tr -d "\r")
  nova=$(virsh dumpxml "$d" 2>/dev/null | sed -n "s/.*<nova:name>\([^<]*\).*/\1/p" | head -n1)
  echo "$id|$state|$nova"
done
'`
	out, err := session.CombinedOutput(remoteCmd)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取 VM 列表失败"})
		return
	}

	var result []VMItem
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, "|") {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) < 3 {
			continue
		}
		idStr, state, name := parts[0], parts[1], parts[2]

		id := 0
		if idStr != "-" {
			// 忽略转换错误
			fmt.Sscanf(idStr, "%d", &id)
		} else {
			id = 0
		}

		result = append(result, VMItem{
			ID:     id,
			Name:   name,
			Status: state,
		})
	}

	go func() { //刷新后更新数据
		config.VM = result
		config.Count = len(result)
		config.TimeStamp = time.Now().Format("2006/1/2 15:04:05")
		saveDeviceInfos()
	}()
	c.JSON(http.StatusOK, result)
}