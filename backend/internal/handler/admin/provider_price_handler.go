package admin

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ProviderPriceHandler struct {
	settingService *service.SettingService
}

func NewProviderPriceAdminHandler(settingService *service.SettingService) *ProviderPriceHandler {
	return &ProviderPriceHandler{settingService: settingService}
}

type updateProviderPriceOverridesRequest struct {
	Items []service.ProviderPriceOverride `json:"items"`
}

func hasProviderPriceValue(item service.ProviderPriceOverride) bool {
	for _, value := range []*float64{
		item.InputPrice,
		item.OutputPrice,
		item.CacheWritePrice,
		item.CacheReadPrice,
		item.ImageOutputPrice,
		item.CacheInputPrice,
		item.CacheCreatePrice,
		item.CacheCreatePrice1h,
	} {
		if value != nil && *value > 0 {
			return true
		}
	}
	return false
}

func (h *ProviderPriceHandler) List(c *gin.Context) {
	items, err := h.settingService.GetProviderPriceOverrides(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if items == nil {
		items = []service.ProviderPriceOverride{}
	}
	response.Success(c, gin.H{"items": items})
}

func (h *ProviderPriceHandler) Update(c *gin.Context) {
	var req updateProviderPriceOverridesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	for _, item := range req.Items {
		if strings.TrimSpace(item.GroupName) == "" {
			response.BadRequest(c, "Group name is required")
			return
		}
		if strings.TrimSpace(item.ModelName) == "" {
			response.BadRequest(c, "Model name is required")
			return
		}
		if !hasProviderPriceValue(item) {
			response.BadRequest(c, "At least one price field must be greater than 0")
			return
		}
	}
	items, err := h.settingService.UpdateProviderPriceOverrides(c.Request.Context(), req.Items)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if items == nil {
		items = []service.ProviderPriceOverride{}
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"items": items,
		},
	})
}
