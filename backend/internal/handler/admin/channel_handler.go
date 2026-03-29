package admin

import (
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
}

// NewChannelHandler creates a new admin channel handler
func NewChannelHandler(channelService *service.ChannelService) *ChannelHandler {
	return &ChannelHandler{channelService: channelService}
}

// --- Request / Response types ---

type createChannelRequest struct {
	Name         string                       `json:"name" binding:"required,max=100"`
	Description  string                       `json:"description"`
	GroupIDs     []int64                      `json:"group_ids"`
	ModelPricing []channelModelPricingRequest `json:"model_pricing"`
}

type updateChannelRequest struct {
	Name         string                        `json:"name" binding:"omitempty,max=100"`
	Description  *string                       `json:"description"`
	Status       string                        `json:"status" binding:"omitempty,oneof=active disabled"`
	GroupIDs     *[]int64                      `json:"group_ids"`
	ModelPricing *[]channelModelPricingRequest `json:"model_pricing"`
}

type channelModelPricingRequest struct {
	Models           []string                 `json:"models" binding:"required,min=1,max=100"`
	BillingMode      string                   `json:"billing_mode" binding:"omitempty,oneof=token per_request image"`
	InputPrice       *float64                 `json:"input_price" binding:"omitempty,min=0"`
	OutputPrice      *float64                 `json:"output_price" binding:"omitempty,min=0"`
	CacheWritePrice  *float64                 `json:"cache_write_price" binding:"omitempty,min=0"`
	CacheReadPrice   *float64                 `json:"cache_read_price" binding:"omitempty,min=0"`
	ImageOutputPrice *float64                 `json:"image_output_price" binding:"omitempty,min=0"`
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
	ID           int64                         `json:"id"`
	Name         string                        `json:"name"`
	Description  string                        `json:"description"`
	Status       string                        `json:"status"`
	GroupIDs     []int64                       `json:"group_ids"`
	ModelPricing []channelModelPricingResponse `json:"model_pricing"`
	CreatedAt    string                        `json:"created_at"`
	UpdatedAt    string                        `json:"updated_at"`
}

type channelModelPricingResponse struct {
	ID               int64                     `json:"id"`
	Models           []string                  `json:"models"`
	BillingMode      string                    `json:"billing_mode"`
	InputPrice       *float64                  `json:"input_price"`
	OutputPrice      *float64                  `json:"output_price"`
	CacheWritePrice  *float64                  `json:"cache_write_price"`
	CacheReadPrice   *float64                  `json:"cache_read_price"`
	ImageOutputPrice *float64                  `json:"image_output_price"`
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
		ID:          ch.ID,
		Name:        ch.Name,
		Description: ch.Description,
		Status:      ch.Status,
		GroupIDs:    ch.GroupIDs,
		CreatedAt:   ch.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   ch.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if resp.GroupIDs == nil {
		resp.GroupIDs = []int64{}
	}

	resp.ModelPricing = make([]channelModelPricingResponse, 0, len(ch.ModelPricing))
	for _, p := range ch.ModelPricing {
		models := p.Models
		if models == nil {
			models = []string{}
		}
		billingMode := string(p.BillingMode)
		if billingMode == "" {
			billingMode = "token"
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
			Models:           models,
			BillingMode:      billingMode,
			InputPrice:       p.InputPrice,
			OutputPrice:      p.OutputPrice,
			CacheWritePrice:  p.CacheWritePrice,
			CacheReadPrice:   p.CacheReadPrice,
			ImageOutputPrice: p.ImageOutputPrice,
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
			Models:           r.Models,
			BillingMode:      billingMode,
			InputPrice:       r.InputPrice,
			OutputPrice:      r.OutputPrice,
			CacheWritePrice:  r.CacheWritePrice,
			CacheReadPrice:   r.CacheReadPrice,
			ImageOutputPrice: r.ImageOutputPrice,
			Intervals:        intervals,
		})
	}
	return result
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

	channel, err := h.channelService.Create(c.Request.Context(), &service.CreateChannelInput{
		Name:         req.Name,
		Description:  req.Description,
		GroupIDs:     req.GroupIDs,
		ModelPricing: pricingRequestToService(req.ModelPricing),
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
		Name:        req.Name,
		Description: req.Description,
		Status:      req.Status,
		GroupIDs:    req.GroupIDs,
	}
	if req.ModelPricing != nil {
		pricing := pricingRequestToService(*req.ModelPricing)
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
