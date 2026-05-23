// Package handler exposes the channel-management plugin HTTP endpoints.
// Handlers translate Gin requests into service calls and back into the
// project standard JSON response envelope.
//
// Wire-shape DTOs and DTO<->service struct converters live in the sibling
// internal/dto package (T17 split). This file only owns:
//   - protocol adaptation (param parsing, status code mapping)
//   - the ModelPricingLookup port the host can implement to wire its
//     billing service into the model-default-pricing endpoint
package handler

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/dto"
	infraerrors "github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/errors"
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/pagination"
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/response"
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/service"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// ModelPricingLookup is the minimal interface the handler needs from the
// core BillingService for the "default pricing" lookup endpoint.
//
// The plugin can be wired with nil here; in that case the endpoint reports
// found=false. Once the core exposes a default-pricing query through the
// SDK, the host can implement this interface and inject it.
type ModelPricingLookup interface {
	GetModelPricing(model string) (*ModelDefaultPricing, error)
}

// ModelDefaultPricing is the per-token default pricing returned by the
// core billing service. Mirrors the subset the GetModelDefaultPricing
// endpoint reads.
//
// Fields use decimal.Decimal to preserve precision across the host->plugin
// boundary (CLAUDE.md: "金额计算必须用 shopspring/decimal"). Conversion to
// float64 happens only at the JSON-response edge, where the frontend
// contract (channels.ts ModelDefaultPricing) expects numbers.
type ModelDefaultPricing struct {
	InputPricePerToken         decimal.Decimal
	OutputPricePerToken        decimal.Decimal
	CacheCreationPricePerToken decimal.Decimal
	CacheReadPricePerToken     decimal.Decimal
	ImageOutputPricePerToken   decimal.Decimal
}

// ChannelHandler exposes the admin channel CRUD + pricing endpoints.
type ChannelHandler struct {
	channelService *service.ChannelService
	pricingLookup  ModelPricingLookup
}

// NewChannelHandler builds a handler. pricingLookup may be nil when the
// plugin host has not provided a default-pricing source.
func NewChannelHandler(channelService *service.ChannelService, pricingLookup ModelPricingLookup) *ChannelHandler {
	return &ChannelHandler{channelService: channelService, pricingLookup: pricingLookup}
}

// List handles GET /admin/channels.
func (h *ChannelHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	status := c.Query("status")
	search := strings.TrimSpace(c.Query("search"))
	if len(search) > 100 {
		search = search[:100]
	}

	channels, pag, err := h.channelService.List(c.Request.Context(), pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "created_at"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}, status, search)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]*dto.ChannelResponse, 0, len(channels))
	for i := range channels {
		out = append(out, dto.ChannelToResponse(&channels[i]))
	}
	response.Paginated(c, out, pag.Total, page, pageSize)
}

// GetByID handles GET /admin/channels/:id.
func (h *ChannelHandler) GetByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_CHANNEL_ID", "Invalid channel ID"))
		return
	}

	channel, err := h.channelService.GetByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.ChannelToResponse(channel))
}

// Create handles POST /admin/channels.
func (h *ChannelHandler) Create(c *gin.Context) {
	var req dto.CreateChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}

	channel, err := h.channelService.Create(c.Request.Context(), dto.CreateChannelInputFromRequest(&req))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.ChannelToResponse(channel))
}

// Update handles PUT /admin/channels/:id.
func (h *ChannelHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_CHANNEL_ID", "Invalid channel ID"))
		return
	}

	var req dto.UpdateChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}

	channel, err := h.channelService.Update(c.Request.Context(), id, dto.UpdateChannelInputFromRequest(&req))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.ChannelToResponse(channel))
}

// Delete handles DELETE /admin/channels/:id.
func (h *ChannelHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_CHANNEL_ID", "Invalid channel ID"))
		return
	}

	if err := h.channelService.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "Channel deleted successfully"})
}

// GetModelDefaultPricing handles GET /admin/channels/model-pricing?model=...
// Returns the default pricing for a model so the frontend can autofill the
// channel-pricing form. Reports found=false when no pricingLookup is wired
// or the model has no default.
func (h *ChannelHandler) GetModelDefaultPricing(c *gin.Context) {
	model := strings.TrimSpace(c.Query("model"))
	if model == "" {
		response.ErrorFrom(c, infraerrors.BadRequest("MISSING_PARAMETER", "model parameter is required").
			WithMetadata(map[string]string{"param": "model"}))
		return
	}

	if h.pricingLookup == nil {
		response.Success(c, gin.H{"found": false})
		return
	}

	pricing, err := h.pricingLookup.GetModelPricing(model)
	if err != nil {
		response.Success(c, gin.H{"found": false})
		return
	}

	response.Success(c, gin.H{
		"found":              true,
		"input_price":        pricing.InputPricePerToken.InexactFloat64(),
		"output_price":       pricing.OutputPricePerToken.InexactFloat64(),
		"cache_write_price":  pricing.CacheCreationPricePerToken.InexactFloat64(),
		"cache_read_price":   pricing.CacheReadPricePerToken.InexactFloat64(),
		"image_output_price": pricing.ImageOutputPricePerToken.InexactFloat64(),
	})
}
