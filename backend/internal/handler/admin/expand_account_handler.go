package admin

import (
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ExpandAccountHandler struct {
	service      service.ExpandAccountService
	adminService service.AdminService
}

func NewExpandAccountHandler(service service.ExpandAccountService, adminService service.AdminService) *ExpandAccountHandler {
	return &ExpandAccountHandler{service: service, adminService: adminService}
}

type createExpandAccountRequest struct {
	Email            string             `json:"email" binding:"required"`
	Platform         string             `json:"platform" binding:"required"`
	SubscriptionType string             `json:"subscription_type" binding:"required"`
	Country          string             `json:"country" binding:"required"`
	SessionKey       string             `json:"session_key" binding:"required"`
	ProxyInfo        *service.ProxyInfo `json:"proxy_info"`
	Used             *bool              `json:"used"`
}

type callbackExpandAccountProxyInfoRequest struct {
	Protocol string `json:"protocol" binding:"required"`
	Host     string `json:"host" binding:"required"`
	Port     int    `json:"port" binding:"required,gt=0"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type callbackExpandAccountRequest struct {
	Email            string                                 `json:"email" binding:"required"`
	Platform         string                                 `json:"platform" binding:"required"`
	SubscriptionType string                                 `json:"subscription_type" binding:"required"`
	Country          string                                 `json:"country" binding:"required"`
	SessionKey       string                                 `json:"session_key" binding:"required"`
	ProxyInfo        *callbackExpandAccountProxyInfoRequest `json:"proxy_info" binding:"required"`
}

type getExpandAccountRequest struct {
	Platform string `json:"platform" binding:"required"`
}

type getExpandAccountResponse struct {
	ID               int64                           `json:"id"`
	Email            string                          `json:"email"`
	Platform         string                          `json:"platform"`
	SubscriptionType string                          `json:"subscription_type"`
	Country          string                          `json:"country"`
	SessionKey       string                          `json:"session_key"`
	ProxyID          *int64                          `json:"proxy_id,omitempty"`
	ProxyInfo        *service.ProxyInfo              `json:"proxy_info,omitempty"`
	Proxy            *dto.AdminProxyWithAccountCount `json:"proxy,omitempty"`
	CreatedAt        string                          `json:"created_at"`
	UpdatedAt        string                          `json:"updated_at"`
}

type updateExpandAccountRequest struct {
	Email            string             `json:"email" binding:"required"`
	Platform         string             `json:"platform" binding:"required"`
	SubscriptionType string             `json:"subscription_type" binding:"required"`
	Country          string             `json:"country" binding:"required"`
	SessionKey       string             `json:"session_key" binding:"required"`
	ProxyInfo        *service.ProxyInfo `json:"proxy_info"`
	Used             *bool              `json:"used"`
}

func (h *ExpandAccountHandler) createFromRequest(c *gin.Context, req createExpandAccountRequest) {
	item, err := h.service.CreateExpandAccount(c.Request.Context(), &service.ExpandAccountCreateInput{
		Email:            strings.TrimSpace(req.Email),
		Platform:         strings.TrimSpace(req.Platform),
		SubscriptionType: strings.TrimSpace(req.SubscriptionType),
		Country:          strings.TrimSpace(req.Country),
		SessionKey:       strings.TrimSpace(req.SessionKey),
		ProxyInfo:        req.ProxyInfo,
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
	var req callbackExpandAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_REQUEST", "invalid request body").WithCause(err))
		return
	}
	_, err := h.service.CreateExpandAccount(c.Request.Context(), &service.ExpandAccountCreateInput{
		Email:            strings.TrimSpace(req.Email),
		Platform:         strings.TrimSpace(req.Platform),
		SubscriptionType: strings.TrimSpace(req.SubscriptionType),
		Country:          strings.TrimSpace(req.Country),
		SessionKey:       strings.TrimSpace(req.SessionKey),
		ProxyInfo: &service.ProxyInfo{
			Protocol: req.ProxyInfo.Protocol,
			Host:     req.ProxyInfo.Host,
			Port:     req.ProxyInfo.Port,
			Username: req.ProxyInfo.Username,
			Password: req.ProxyInfo.Password,
		},
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, nil)
}

func (h *ExpandAccountHandler) GetByPlatform(c *gin.Context) {
	var req getExpandAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_REQUEST", "invalid request body").WithCause(err))
		return
	}

	item, err := h.service.GetAndMarkExpandAccountByPlatform(c.Request.Context(), strings.TrimSpace(req.Platform))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	resp := &getExpandAccountResponse{
		ID:               item.ID,
		Email:            item.Email,
		Platform:         item.Platform,
		SubscriptionType: item.SubscriptionType,
		Country:          item.Country,
		SessionKey:       item.SessionKey,
		ProxyID:          item.ProxyID,
		ProxyInfo:        item.ProxyInfo,
		CreatedAt:        item.CreatedAt.Format(time.RFC3339),
		UpdatedAt:        item.UpdatedAt.Format(time.RFC3339),
	}

	if item.ProxyID != nil {
		proxy, err := h.adminService.GetProxyWithAccountCount(c.Request.Context(), *item.ProxyID)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		if proxy != nil {
			resp.Proxy = dto.ProxyWithAccountCountFromServiceAdmin(proxy)
		}
	}

	response.Success(c, resp)
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
		ProxyInfo:        req.ProxyInfo,
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
