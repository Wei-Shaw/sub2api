package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// AvailableChannelHandler 处理「可用渠道」聚合视图的管理员接口。
//
// 该视图以只读方式聚合渠道基础信息、关联分组与推导出的支持模型列表（无通配符）。
type AvailableChannelHandler struct {
	channelService *service.ChannelService
}

// NewAvailableChannelHandler 创建 AvailableChannelHandler 实例。
func NewAvailableChannelHandler(channelService *service.ChannelService) *AvailableChannelHandler {
	return &AvailableChannelHandler{channelService: channelService}
}

// availableGroupResponse 响应中的分组概要。
type availableGroupResponse struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
}

// supportedModelResponse 响应中的支持模型条目。
type supportedModelResponse struct {
	Name     string                       `json:"name"`
	Platform string                       `json:"platform"`
	Pricing  *channelModelPricingResponse `json:"pricing"`
}

// availableChannelResponse 管理员视图完整字段集。
type availableChannelResponse struct {
	ID                 int64                    `json:"id"`
	Name               string                   `json:"name"`
	Description        string                   `json:"description"`
	Status             string                   `json:"status"`
	BillingModelSource string                   `json:"billing_model_source"`
	RestrictModels     bool                     `json:"restrict_models"`
	Groups             []availableGroupResponse `json:"groups"`
	SupportedModels    []supportedModelResponse `json:"supported_models"`
}

// availableChannelToAdminResponse 将 service 层的 AvailableChannel 转为管理员 DTO。
// 同 package 内复用；也用于构造测试 fixture。
func availableChannelToAdminResponse(ch service.AvailableChannel) availableChannelResponse {
	groups := make([]availableGroupResponse, 0, len(ch.Groups))
	for _, g := range ch.Groups {
		groups = append(groups, availableGroupResponse{ID: g.ID, Name: g.Name, Platform: g.Platform})
	}
	models := make([]supportedModelResponse, 0, len(ch.SupportedModels))
	for i := range ch.SupportedModels {
		m := ch.SupportedModels[i]
		var pricing *channelModelPricingResponse
		if m.Pricing != nil {
			p := pricingToResponse(m.Pricing)
			pricing = &p
		}
		models = append(models, supportedModelResponse{
			Name:     m.Name,
			Platform: m.Platform,
			Pricing:  pricing,
		})
	}
	return availableChannelResponse{
		ID:                 ch.ID,
		Name:               ch.Name,
		Description:        ch.Description,
		Status:             ch.Status,
		BillingModelSource: ch.BillingModelSource,
		RestrictModels:     ch.RestrictModels,
		Groups:             groups,
		SupportedModels:    models,
	}
}

// List 列出所有可用渠道（管理员视图）。
// GET /api/v1/admin/channels/available
func (h *AvailableChannelHandler) List(c *gin.Context) {
	channels, err := h.channelService.ListAvailable(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]availableChannelResponse, 0, len(channels))
	for _, ch := range channels {
		out = append(out, availableChannelToAdminResponse(ch))
	}
	response.Success(c, gin.H{"items": out})
}
