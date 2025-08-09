package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// 配置结构
type Device struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	LoginURL string `json:"login_url"`
	Username string `json:"username"`
	Password string `json:"password"`
	SSHHost  string `json:"ssh_host"`
	SSHPort  string `json:"ssh_port"`
	SSHUser  string `json:"ssh_user"`
	SSHPass  string `json:"ssh_pass"`
}

// 列表项结构
type VmItem struct {
	Name          string `json:"name"`
	Status        string `json:"status"`
	CanExecute    bool   `json:"can_execute"`
	ComponentType string `json:"component_type"`
	IPAddress     string `json:"ip_address"`
}

// 执行请求结构
type ExecuteRequest struct {
	ConfigID int    `json:"config_id"`
	ItemID   string `json:"item_id"`
	Command  string `json:"command"`
}

// 全局变量
var devices []Device
var configFile = "devices.json"

// 加载配置文件
func loadDeviceInfos() error {
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		// 如果文件不存在，创建默认配置
		devices = []Device{}
		return saveDeviceInfos()
	}

	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &devices)
}

// 保存配置到文件
func saveDeviceInfos() error {
	data, err := json.MarshalIndent(devices, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(configFile, data, 0644)
}

// 获取下一个可用的ID
func getNextConfigID() int {
	maxID := 0
	for _, config := range devices {
		if config.ID > maxID {
			maxID = config.ID
		}
	}
	return maxID + 1
}

func main() {
	// 加载配置文件
	if err := loadDeviceInfos(); err != nil {
		fmt.Printf("加载配置文件失败: %v\n", err)
		// 继续运行，使用空配置
		devices = []Device{}
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
	r.StaticFile("/api/defaults.json", "./static/noVNC-1.6.0/defaults.json")
	r.StaticFile("/api/mandatory.json", "./static/noVNC-1.6.0/mandatory.json")
	r.StaticFile("/api/package.json", "./static/noVNC-1.6.0/package.json")
	r.Static("/api/app", "./static/noVNC-1.6.0/app")
	r.Static("/api/core", "./static/noVNC-1.6.0/core")
	r.Static("/api/vendor", "./static/noVNC-1.6.0/vendor")

	r.LoadHTMLGlob("html/*")

	// 主页
	r.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", nil)
	})

	// API 路由
	api := r.Group("/api")
	{
		// 配置管理相关路由
		api.GET("/devices", getDevices)
		api.POST("/devices", createDevice)
		api.PUT("/devices/:id", updateDevice)
		api.DELETE("/devices/:id", deleteConfig)

		// csmp实例信息路由
		api.GET("/csmp/:id", handleCsmp)

		api.GET("/webshell", func(c *gin.Context) {
			c.HTML(http.StatusOK, "webshell.html", nil)
		})

		// WebShell WebSocket API
		api.GET("/webshell/ws", handleWebShellWebSocket)

		// vnc地址
		api.GET("/vnc/:id", getVNCAddress)
		api.GET("/vnc", func(c *gin.Context) {
			c.HTML(http.StatusOK, "vnc.html", nil)
		})
		api.GET("/vnc/ws", handleVNCWebSocket)
	}

	fmt.Println("服务器运行在 http://localhost:8080")
	_ = r.Run(":8080")
}
