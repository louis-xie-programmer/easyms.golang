package config

import (
	"easyms/internal/shared/discovery"
	"easyms/internal/shared/entities"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ConfigHandler 配置管理处理器
type ConfigHandler struct {
	discovery  *discovery.Discovery
	serverName string
	env        string
	configMgr  *ConfigurationManager
}

// NewConfigHandler 创建配置管理处理器
func NewConfigHandler(d *discovery.Discovery, appConfigProvider AppConfigProvider, serverName, env string) *ConfigHandler {
	return &ConfigHandler{
		discovery:  d,
		serverName: serverName,
		env:        env,
		configMgr:  NewConfigurationManager(appConfigProvider),
	}
}

// SaveVersion 保存当前配置为新版本
// POST /config/version
func (h *ConfigHandler) SaveVersion(c *gin.Context) {
	var req struct {
		Description string `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取当前服务是否为网关
	isGateway := h.serverName == "gateway"

	versionID, err := h.configMgr.SaveConfigVersion(h.discovery, h.serverName, h.env, req.Description, isGateway)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"version_id": versionID,
		"message":    "Configuration version saved successfully",
	})
}

// ListVersions 获取配置版本列表
// GET /config/versions
func (h *ConfigHandler) ListVersions(c *gin.Context) {
	// 获取当前服务是否为网关
	isGateway := h.serverName == "gateway"

	versions, err := h.configMgr.GetConfigVersions(h.discovery, h.serverName, h.env, isGateway)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 按时间倒序排列
	for i, j := 0, len(versions)-1; i < j; i, j = i+1, j-1 {
		versions[i], versions[j] = versions[j], versions[i]
	}

	c.JSON(http.StatusOK, versions)
}

// GetVersion 获取指定版本的配置详情
// GET /config/version/:versionID
func (h *ConfigHandler) GetVersion(c *gin.Context) {
	versionID := c.Param("versionID")

	version, err := h.configMgr.GetConfigVersion(h.discovery, h.serverName, h.env, versionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, version)
}

// RollbackToVersion 回滚到指定版本
// POST /config/rollback/:versionID
func (h *ConfigHandler) RollbackToVersion(c *gin.Context) {
	versionID := c.Param("versionID")

	err := h.configMgr.RollbackToVersion(h.discovery, h.serverName, h.env, versionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("Successfully rolled back to version %s", versionID),
	})
}

// GetCurrentConfig 获取当前配置
// GET /config/current
func (h *ConfigHandler) GetCurrentConfig(c *gin.Context) {
	currentConfig := GetAppConfig()
	if currentConfig == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Current config not found"})
		return
	}

	// 创建配置副本以避免并发问题
	configCopy := *currentConfig

	c.JSON(http.StatusOK, configCopy)
}

// UpdateConfig 更新当前配置
// PUT /config/current
func (h *ConfigHandler) UpdateConfig(c *gin.Context) {
	var newConfig entities.AppConfig

	if err := c.ShouldBindJSON(&newConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 更新配置
	globalAppConfig = &newConfig

	c.JSON(http.StatusOK, gin.H{"message": "Configuration updated successfully"})
}

// RegisterConfigRoutes 注册配置管理路由
func (h *ConfigHandler) RegisterConfigRoutes(router *gin.Engine) {
	configGroup := router.Group("/config")
	{
		configGroup.POST("/version", h.SaveVersion)
		configGroup.GET("/versions", h.ListVersions)
		configGroup.GET("/version/:versionID", h.GetVersion)
		configGroup.POST("/rollback/:versionID", h.RollbackToVersion)
		configGroup.GET("/current", h.GetCurrentConfig)
		configGroup.PUT("/current", h.UpdateConfig)
	}
}
