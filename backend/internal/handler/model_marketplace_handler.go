package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type ModelMarketplaceHandler struct {
	service *service.ModelMarketplaceService
}

func NewModelMarketplaceHandler(service *service.ModelMarketplaceService) *ModelMarketplaceHandler {
	return &ModelMarketplaceHandler{service: service}
}

// ListPricing returns the user-facing model pricing marketplace.
// GET /api/v1/models/pricing
func (h *ModelMarketplaceHandler) ListPricing(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, "model marketplace service unavailable")
		return
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	page, pageSize := response.ParsePagination(c)
	groupID, err := service.ParseModelMarketplaceGroupID(c.Query("group_id"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	result, err := h.service.ListPricing(c.Request.Context(), subject.UserID, service.ModelMarketplaceListRequest{
		Query:       c.Query("q"),
		Provider:    c.Query("provider"),
		Mode:        c.Query("mode"),
		BillingMode: c.Query("billing_mode"),
		Endpoint:    c.Query("endpoint"),
		GroupID:     groupID,
		ServiceTier: c.Query("service_tier"),
		Unit:        c.Query("unit"),
		Page:        page,
		PageSize:    pageSize,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
