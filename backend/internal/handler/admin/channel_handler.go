package admin

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ChannelHandler handles admin channel management
type ChannelHandler struct {
	channelService *service.ChannelService
	billingService *service.BillingService
}

// NewChannelHandler creates a new admin channel handler
func NewChannelHandler(channelService *service.ChannelService, billingService *service.BillingService) *ChannelHandler {
	return &ChannelHandler{channelService: channelService, billingService: billingService}
}

// --- Request / Response types ---

type createChannelRequest struct {
	Name               string                       `json:"name" binding:"required,max=100"`
	Description        string                       `json:"description"`
	GroupIDs           []int64                      `json:"group_ids"`
	ModelPricing       []channelModelPricingRequest `json:"model_pricing"`
	ModelMapping       map[string]map[string]string `json:"model_mapping"`
	BillingModelSource string                       `json:"billing_model_source" binding:"omitempty,oneof=requested upstream channel_mapped"`
	RestrictModels     bool                         `json:"restrict_models"`
}

type updateChannelRequest struct {
	Name               string                        `json:"name" binding:"omitempty,max=100"`
	Description        *string                       `json:"description"`
	Status             string                        `json:"status" binding:"omitempty,oneof=active disabled"`
	GroupIDs           *[]int64                      `json:"group_ids"`
	ModelPricing       *[]channelModelPricingRequest `json:"model_pricing"`
	ModelMapping       map[string]map[string]string  `json:"model_mapping"`
	BillingModelSource string                        `json:"billing_model_source" binding:"omitempty,oneof=requested upstream channel_mapped"`
	RestrictModels     *bool                         `json:"restrict_models"`
}

type channelModelPricingRequest struct {
	Platform         string                   `json:"platform" binding:"omitempty,max=50"`
	Models           []string                 `json:"models" binding:"required,min=1,max=100"`
	BillingMode      string                   `json:"billing_mode" binding:"omitempty,oneof=token per_request image"`
	InputPrice       *float64                 `json:"input_price" binding:"omitempty,min=0"`
	OutputPrice      *float64                 `json:"output_price" binding:"omitempty,min=0"`
	CacheWritePrice  *float64                 `json:"cache_write_price" binding:"omitempty,min=0"`
	CacheReadPrice   *float64                 `json:"cache_read_price" binding:"omitempty,min=0"`
	ImageOutputPrice *float64                 `json:"image_output_price" binding:"omitempty,min=0"`
	PerRequestPrice  *float64                 `json:"per_request_price" binding:"omitempty,min=0"`
	Intervals        []pricingIntervalRequest `json:"intervals"`
}

type pricingIntervalRequest struct {
	MinTokens       int      `json:"min_tokens"`
	MaxTokens       *int     `json:"max_tokens"`
	TierLabel       string   `json:"tier_label"`
	InputPrice      *float64 `json:"input_price"`
	OutputPrice     *float64 `json:"output_price"`
	CacheWritePrice *float64 `json:"cache_write_price"`
	CacheReadPrice  *float64 `json:"cache_read_price"`
	PerRequestPrice *float64 `json:"per_request_price"`
	SortOrder       int      `json:"sort_order"`
}

type channelResponse struct {
	ID                 int64                         `json:"id"`
	Name               string                        `json:"name"`
	Description        string                        `json:"description"`
	Status             string                        `json:"status"`
	BillingModelSource string                        `json:"billing_model_source"`
	RestrictModels     bool                          `json:"restrict_models"`
	GroupIDs           []int64                       `json:"group_ids"`
	ModelPricing       []channelModelPricingResponse `json:"model_pricing"`
	ModelMapping       map[string]map[string]string  `json:"model_mapping"`
	CreatedAt          string                        `json:"created_at"`
	UpdatedAt          string                        `json:"updated_at"`
}

type channelModelPricingResponse struct {
	ID               int64                     `json:"id"`
	Platform         string                    `json:"platform"`
	Models           []string                  `json:"models"`
	BillingMode      string                    `json:"billing_mode"`
	InputPrice       *float64                  `json:"input_price"`
	OutputPrice      *float64                  `json:"output_price"`
	CacheWritePrice  *float64                  `json:"cache_write_price"`
	CacheReadPrice   *float64                  `json:"cache_read_price"`
	ImageOutputPrice *float64                  `json:"image_output_price"`
	PerRequestPrice  *float64                  `json:"per_request_price"`
	Intervals        []pricingIntervalResponse `json:"intervals"`
}

type pricingIntervalResponse struct {
	ID              int64    `json:"id"`
	MinTokens       int      `json:"min_tokens"`
	MaxTokens       *int     `json:"max_tokens"`
	TierLabel       string   `json:"tier_label,omitempty"`
	InputPrice      *float64 `json:"input_price"`
	OutputPrice     *float64 `json:"output_price"`
	CacheWritePrice *float64 `json:"cache_write_price"`
	CacheReadPrice  *float64 `json:"cache_read_price"`
	PerRequestPrice *float64 `json:"per_request_price"`
	SortOrder       int      `json:"sort_order"`
}

func channelToResponse(ch *service.Channel) *channelResponse {
	if ch == nil {
		return nil
	}
	resp := &channelResponse{
		ID:             ch.ID,
		Name:           ch.Name,
		Description:    ch.Description,
		Status:         ch.Status,
		RestrictModels: ch.RestrictModels,
		GroupIDs:       ch.GroupIDs,
		ModelMapping:   ch.ModelMapping,
		CreatedAt:      ch.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:      ch.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
	resp.BillingModelSource = ch.BillingModelSource
	if resp.BillingModelSource == "" {
		resp.BillingModelSource = service.BillingModelSourceChannelMapped
	}
	if resp.GroupIDs == nil {
		resp.GroupIDs = []int64{}
	}
	if resp.ModelMapping == nil {
		resp.ModelMapping = map[string]map[string]string{}
	}

	resp.ModelPricing = make([]channelModelPricingResponse, 0, len(ch.ModelPricing))
	for _, p := range ch.ModelPricing {
		models := p.Models
		if models == nil {
			models = []string{}
		}
		billingMode := string(p.BillingMode)
		if billingMode == "" {
			billingMode = string(service.BillingModeToken)
		}
		platform := p.Platform
		if platform == "" {
			platform = service.PlatformAnthropic
		}
		intervals := make([]pricingIntervalResponse, 0, len(p.Intervals))
		for _, iv := range p.Intervals {
			intervals = append(intervals, pricingIntervalResponse{
				ID:              iv.ID,
				MinTokens:       iv.MinTokens,
				MaxTokens:       iv.MaxTokens,
				TierLabel:       iv.TierLabel,
				InputPrice:      iv.InputPrice,
				OutputPrice:     iv.OutputPrice,
				CacheWritePrice: iv.CacheWritePrice,
				CacheReadPrice:  iv.CacheReadPrice,
				PerRequestPrice: iv.PerRequestPrice,
				SortOrder:       iv.SortOrder,
			})
		}
		resp.ModelPricing = append(resp.ModelPricing, channelModelPricingResponse{
			ID:               p.ID,
			Platform:         platform,
			Models:           models,
			BillingMode:      billingMode,
			InputPrice:       p.InputPrice,
			OutputPrice:      p.OutputPrice,
			CacheWritePrice:  p.CacheWritePrice,
			CacheReadPrice:   p.CacheReadPrice,
			ImageOutputPrice: p.ImageOutputPrice,
			PerRequestPrice:  p.PerRequestPrice,
			Intervals:        intervals,
		})
	}
	return resp
}

func pricingRequestToService(reqs []channelModelPricingRequest) []service.ChannelModelPricing {
	result := make([]service.ChannelModelPricing, 0, len(reqs))
	for _, r := range reqs {
		billingMode := service.BillingMode(r.BillingMode)
		if billingMode == "" {
			billingMode = service.BillingModeToken
		}
		platform := r.Platform
		if platform == "" {
			platform = service.PlatformAnthropic
		}
		intervals := make([]service.PricingInterval, 0, len(r.Intervals))
		for _, iv := range r.Intervals {
			intervals = append(intervals, service.PricingInterval{
				MinTokens:       iv.MinTokens,
				MaxTokens:       iv.MaxTokens,
				TierLabel:       iv.TierLabel,
				InputPrice:      iv.InputPrice,
				OutputPrice:     iv.OutputPrice,
				CacheWritePrice: iv.CacheWritePrice,
				CacheReadPrice:  iv.CacheReadPrice,
				PerRequestPrice: iv.PerRequestPrice,
				SortOrder:       iv.SortOrder,
			})
		}
		result = append(result, service.ChannelModelPricing{
			Platform:         platform,
			Models:           r.Models,
			BillingMode:      billingMode,
			InputPrice:       r.InputPrice,
			OutputPrice:      r.OutputPrice,
			CacheWritePrice:  r.CacheWritePrice,
			CacheReadPrice:   r.CacheReadPrice,
			ImageOutputPrice: r.ImageOutputPrice,
			PerRequestPrice:  r.PerRequestPrice,
			Intervals:        intervals,
		})
	}
	return result
}

// validatePricingBillingMode 校验计费配置
func validatePricingBillingMode(pricing []service.ChannelModelPricing) error {
	for _, p := range pricing {
		// 按次/图片模式必须配置默认价格或区间
		if p.BillingMode == service.BillingModePerRequest || p.BillingMode == service.BillingModeImage {
			if p.PerRequestPrice == nil && len(p.Intervals) == 0 {
				return errors.New("per-request price or intervals required for per_request/image billing mode")
			}
		}
		// 校验价格不能为负
		if err := validatePriceNotNegative("input_price", p.InputPrice); err != nil {
			return err
		}
		if err := validatePriceNotNegative("output_price", p.OutputPrice); err != nil {
			return err
		}
		if err := validatePriceNotNegative("cache_write_price", p.CacheWritePrice); err != nil {
			return err
		}
		if err := validatePriceNotNegative("cache_read_price", p.CacheReadPrice); err != nil {
			return err
		}
		if err := validatePriceNotNegative("image_output_price", p.ImageOutputPrice); err != nil {
			return err
		}
		if err := validatePriceNotNegative("per_request_price", p.PerRequestPrice); err != nil {
			return err
		}
		// 校验 interval：至少有一个价格字段非空
		for _, iv := range p.Intervals {
			if iv.InputPrice == nil && iv.OutputPrice == nil &&
				iv.CacheWritePrice == nil && iv.CacheReadPrice == nil &&
				iv.PerRequestPrice == nil {
				return fmt.Errorf("interval [%d, %s] has no price fields set for model %v",
					iv.MinTokens, formatMaxTokens(iv.MaxTokens), p.Models)
			}
		}
	}
	return nil
}

func validatePriceNotNegative(field string, val *float64) error {
	if val != nil && *val < 0 {
		return fmt.Errorf("%s must be >= 0", field)
	}
	return nil
}

func formatMaxTokens(max *int) string {
	if max == nil {
		return "∞"
	}
	return fmt.Sprintf("%d", *max)
}

// --- Handlers ---

// List handles listing channels with pagination
// GET /api/v1/admin/channels
func (h *ChannelHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	status := c.Query("status")
	search := strings.TrimSpace(c.Query("search"))
	if len(search) > 100 {
		search = search[:100]
	}

	channels, pag, err := h.channelService.List(c.Request.Context(), pagination.PaginationParams{Page: page, PageSize: pageSize}, status, search)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]*channelResponse, 0, len(channels))
	for i := range channels {
		out = append(out, channelToResponse(&channels[i]))
	}
	response.Paginated(c, out, pag.Total, page, pageSize)
}

// GetByID handles getting a channel by ID
// GET /api/v1/admin/channels/:id
func (h *ChannelHandler) GetByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid channel ID")
		return
	}

	channel, err := h.channelService.GetByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, channelToResponse(channel))
}

// Create handles creating a new channel
// POST /api/v1/admin/channels
func (h *ChannelHandler) Create(c *gin.Context) {
	var req createChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	pricing := pricingRequestToService(req.ModelPricing)
	if err := validatePricingBillingMode(pricing); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	channel, err := h.channelService.Create(c.Request.Context(), &service.CreateChannelInput{
		Name:               req.Name,
		Description:        req.Description,
		GroupIDs:           req.GroupIDs,
		ModelPricing:       pricing,
		ModelMapping:       req.ModelMapping,
		BillingModelSource: req.BillingModelSource,
		RestrictModels:     req.RestrictModels,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, channelToResponse(channel))
}

// Update handles updating a channel
// PUT /api/v1/admin/channels/:id
func (h *ChannelHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid channel ID")
		return
	}

	var req updateChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	input := &service.UpdateChannelInput{
		Name:               req.Name,
		Description:        req.Description,
		Status:             req.Status,
		GroupIDs:           req.GroupIDs,
		ModelMapping:       req.ModelMapping,
		BillingModelSource: req.BillingModelSource,
		RestrictModels:     req.RestrictModels,
	}
	if req.ModelPricing != nil {
		pricing := pricingRequestToService(*req.ModelPricing)
		if err := validatePricingBillingMode(pricing); err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		input.ModelPricing = &pricing
	}

	channel, err := h.channelService.Update(c.Request.Context(), id, input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, channelToResponse(channel))
}

// Delete handles deleting a channel
// DELETE /api/v1/admin/channels/:id
func (h *ChannelHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid channel ID")
		return
	}

	if err := h.channelService.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"message": "Channel deleted successfully"})
}

// GetModelDefaultPricing 获取模型的默认定价（用于前端自动填充）
// GET /api/v1/admin/channels/model-pricing?model=claude-sonnet-4
func (h *ChannelHandler) GetModelDefaultPricing(c *gin.Context) {
	model := strings.TrimSpace(c.Query("model"))
	if model == "" {
		response.BadRequest(c, "model parameter is required")
		return
	}

	pricing, err := h.billingService.GetModelPricing(model)
	if err != nil {
		// 模型不在定价列表中
		response.Success(c, gin.H{"found": false})
		return
	}

	response.Success(c, gin.H{
		"found":              true,
		"input_price":        pricing.InputPricePerToken,
		"output_price":       pricing.OutputPricePerToken,
		"cache_write_price":  pricing.CacheCreationPricePerToken,
		"cache_read_price":   pricing.CacheReadPricePerToken,
		"image_output_price": pricing.ImageOutputPricePerToken,
	})
}
