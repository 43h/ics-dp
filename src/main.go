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
type Config struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	LoginURL string `json:"login_url"`
	Username string `json:"username"`
	Password string `json:"password"`
	SSHHost  string `json:"ssh_host"`
	SSHUser  string `json:"ssh_user"`
	SSHPass  string `json:"ssh_pass"`
	SSHPort  string `json:"ssh_port"`
}

// 列表项结构
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

// 执行请求结构
type ExecuteRequest struct {
	ConfigID int    `json:"config_id"`
	ItemID   string `json:"item_id"`
	Command  string `json:"command"`
}

// 全局变量
var configs []Config
var configFile = "configs.json"

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
		// 配置管理相关路由
		api.GET("/configs", getConfigs)
		api.POST("/configs", createConfig)
		api.PUT("/configs/:id", updateConfig)
		api.DELETE("/configs/:id", deleteConfig)

		// 数据抓取相关路由
		api.GET("/csmp/:id", csmp)

		// WebShell WebSocket API
		api.GET("/webshell/ws", handleWebShellWebSocket)
	}

	fmt.Println("服务器运行在 http://localhost:8080")
	_ = r.Run(":8080")
}

// 配置管理相关函数
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