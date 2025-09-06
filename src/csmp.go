package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/ssh"
)

type VMIP struct {
	IP  string `json:"ip"`
	MAC string `json:"mac"`
}
type VMItem struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	CreateTime string `json:"create_time"`
	IP         []VMIP `json:"ips"`
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

	if config.SSHHost == "" || config.SSHPort == "" || config.SSHUser == "" || config.SSHPass == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "SSH参数缺失"})
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
	var remoteCmd string
	if config.DevType == "CSMP" {
		remoteCmd = `
bash -c '
for d in $(virsh list --all --name | grep .); do
  id=$(virsh domid "$d" 2>/dev/null || echo -)
  state=$(virsh domstate "$d" 2>/dev/null | tr -d "\r")
  nova=$(virsh dumpxml "$d" 2>/dev/null | sed -n "s/.*<nova:name>\([^<]*\).*/\1/p" | head -n1)
  ctime=$(virsh dumpxml "$d" 2>/dev/null | sed -n "s/.*<nova:creationTime>\([^<]*\).*/\1/p" | head -n1)
  macs=$(virsh dumpxml "$d" 2>/dev/null | sed -n "s/.*mac address=\(.*\)[/].*/\1/p" | tr "\n" "," | sed "s/,$//")
  echo "$id|$state|$nova|$ctime|$macs"
done
'`
	} else if config.DevType == "XC" {
		remoteCmd = `
bash -c '
for d in $(virsh list --all --name | grep .); do
  id=$(virsh domid "$d" 2>/dev/null || echo -)
  state=$(virsh domstate "$d" 2>/dev/null | tr -d "\r")
  nova=$(virsh dumpxml "$d" 2>/dev/null | sed -n "s/.*<name>\([^<]*\).*/\1/p" | head -n1)
  macs=$(virsh dumpxml "$d" 2>/dev/null | sed -n "s/.*mac address=\(.*\)[/].*/\1/p" | tr "\n" "," | sed "s/,$//")
  echo "$id|$state|$nova|-|$macs"
done
'`
	}

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
		parts := strings.SplitN(line, "|", 5)
		if len(parts) < 5 {
			continue
		}

		idStr, state, name, createTime, macs := parts[0], parts[1], parts[2], parts[3], parts[4]

		id := 0
		if idStr != "-" {
			// 忽略转换错误
			fmt.Sscanf(idStr, "%d", &id)
		} else {
			id = 0
		}

		if state == "运行中" {
			state = "running"
		}

		var ipList []VMIP
		macArr := strings.Split(macs, ",")
		for _, mac := range macArr {
			mac = strings.TrimSpace(mac)
			if mac == "" {
				continue
			}
			mac = strings.ReplaceAll(mac, "'", "")
			mac = strings.ReplaceAll(mac, "\"", "")
			mac = strings.ToLower(mac)
			ipList = append(ipList, VMIP{
				MAC: mac,
				IP:  "",
			})
		}

		result = append(result, VMItem{
			ID:         id,
			Name:       name,
			Status:     state,
			CreateTime: createTime,
			IP:         ipList,
		})
	}

	session2, err := client.NewSession()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "SSH session failed"})
		return
	}
	defer session2.Close()
	arpCmd := `bash -c '
arp -an | awk "{print \$2 \"|\" \$4}"
'`
	out, err = session2.CombinedOutput(arpCmd)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取 VM 列表失败"})
		return
	}

	var ipAll []VMIP
	lines = strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, "|") {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		if len(parts) < 2 {
			continue
		}
		ip := parts[0]
		mac := parts[1]
		if mac == "<incomplete>" {
			continue
		}
		ip = strings.ReplaceAll(ip, "(", "")
		ip = strings.ReplaceAll(ip, ")", "")
		mac = strings.ToLower(mac)
		ipAll = append(ipAll, VMIP{
			MAC: mac,
			IP:  ip,
		})
	}

	for i := range result {
		for j := range result[i].IP {
			mac := result[i].IP[j].MAC
			for _, ipItem := range ipAll {
				if ipItem.MAC == mac {
					result[i].IP[j].IP = ipItem.IP
					break
				}
			}
		}
	}

	go func() { //刷新后更新数据
		config.VM = result
		config.Count = len(result)
		config.TimeStamp = time.Now().Format("2006/1/2 15:04:05")
		saveDeviceInfos()
	}()
	c.JSON(http.StatusOK, result)
}