package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// COSImageHandler 提供全局图片转存（腾讯云 COS / S3 兼容）配置的管理接口。
type COSImageHandler struct {
	cosService *service.COSImageTransferService
}

func NewCOSImageHandler(cosService *service.COSImageTransferService) *COSImageHandler {
	return &COSImageHandler{cosService: cosService}
}

// GetConfig 返回当前 COS 转存配置（SecretKey 不回显明文）。
func (h *COSImageHandler) GetConfig(c *gin.Context) {
	cfg, err := h.cosService.GetConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	// 不回显密钥明文，仅标识是否已设置。
	masked := *cfg
	hasSecret := masked.SecretAccessKey != ""
	masked.SecretAccessKey = ""
	response.Success(c, gin.H{
		"config":                masked,
		"secret_access_key_set": hasSecret,
	})
}

// UpdateConfig 更新 COS 转存配置。SecretKey 留空表示保持原值。
func (h *COSImageHandler) UpdateConfig(c *gin.Context) {
	var req service.COSImageConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	cfg, err := h.cosService.UpdateConfig(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	masked := *cfg
	masked.SecretAccessKey = ""
	response.Success(c, masked)
}
