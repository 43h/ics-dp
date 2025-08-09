package main

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// 配置管理相关函数
func getDevices(c *gin.Context) {
	c.JSON(http.StatusOK, devices)
}

func createDevice(c *gin.Context) {
	var config Device
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	config.ID = getNextConfigID()
	devices = append(devices, config)

	// 保存到文件
	if err := saveDeviceInfos(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存配置失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, config)
}

func updateDevice(c *gin.Context) {
	id := c.Param("id")
	var updatedConfig Device
	if err := c.ShouldBindJSON(&updatedConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	for i, config := range devices {
		if fmt.Sprintf("%d", config.ID) == id {
			updatedConfig.ID = config.ID
			devices[i] = updatedConfig

			// 保存到文件
			if err := saveDeviceInfos(); err != nil {
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
	for i, config := range devices {
		if fmt.Sprintf("%d", config.ID) == id {
			devices = append(devices[:i], devices[i+1:]...)

			// 保存到文件
			if err := saveDeviceInfos(); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "保存配置失败: " + err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"message": "配置已删除"})
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "配置未找到"})
}
