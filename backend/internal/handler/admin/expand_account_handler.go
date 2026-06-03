package admin

import (
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ExpandAccountHandler struct {
	service service.ExpandAccountService
}

func NewExpandAccountHandler(service service.ExpandAccountService) *ExpandAccountHandler {
	return &ExpandAccountHandler{service: service}
}

type createExpandAccountRequest struct {
	Email            string `json:"email" binding:"required"`
	Platform         string `json:"platform" binding:"required"`
	SubscriptionType string `json:"subscription_type" binding:"required"`
	Country          string `json:"country" binding:"required"`
	SessionKey       string `json:"session_key" binding:"required"`
	Used             *bool  `json:"used"`
}

type updateExpandAccountRequest struct {
	Email            string `json:"email" binding:"required"`
	Platform         string `json:"platform" binding:"required"`
	SubscriptionType string `json:"subscription_type" binding:"required"`
	Country          string `json:"country" binding:"required"`
	SessionKey       string `json:"session_key" binding:"required"`
	Used             *bool  `json:"used"`
}

func (h *ExpandAccountHandler) createFromRequest(c *gin.Context, req createExpandAccountRequest) {
	item, err := h.service.CreateExpandAccount(c.Request.Context(), &service.ExpandAccountCreateInput{
		Email:            strings.TrimSpace(req.Email),
		Platform:         strings.TrimSpace(req.Platform),
		SubscriptionType: strings.TrimSpace(req.SubscriptionType),
		Country:          strings.TrimSpace(req.Country),
		SessionKey:       strings.TrimSpace(req.SessionKey),
		Used:             req.Used,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *ExpandAccountHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.service.ListExpandAccounts(c.Request.Context(), page, pageSize, service.ExpandAccountListFilters{
		Search: strings.TrimSpace(c.Query("search")),
		Used:   strings.TrimSpace(c.Query("used")),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, page, pageSize)
}

func (h *ExpandAccountHandler) GetByID(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	item, err := h.service.GetExpandAccount(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *ExpandAccountHandler) Create(c *gin.Context) {
	var req createExpandAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_REQUEST", "invalid request body").WithCause(err))
		return
	}
	h.createFromRequest(c, req)
}

func (h *ExpandAccountHandler) Callback(c *gin.Context) {
	var req createExpandAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_REQUEST", "invalid request body").WithCause(err))
		return
	}
	h.createFromRequest(c, req)
}

func (h *ExpandAccountHandler) Update(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	var req updateExpandAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_REQUEST", "invalid request body").WithCause(err))
		return
	}

	item, err := h.service.UpdateExpandAccount(c.Request.Context(), id, &service.ExpandAccountUpdateInput{
		Email:            strings.TrimSpace(req.Email),
		Platform:         strings.TrimSpace(req.Platform),
		SubscriptionType: strings.TrimSpace(req.SubscriptionType),
		Country:          strings.TrimSpace(req.Country),
		SessionKey:       strings.TrimSpace(req.SessionKey),
		Used:             req.Used,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *ExpandAccountHandler) Delete(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteExpandAccount(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "Expand account deleted successfully"})
}

func (h *ExpandAccountHandler) MarkUsed(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	item, err := h.service.MarkExpandAccountUsed(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}
