package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ExpandAccountHandler struct {
	service      service.ExpandAccountService
	adminService service.AdminService
	cfg          *config.Config
}

func NewExpandAccountHandler(service service.ExpandAccountService, adminService service.AdminService, cfg *config.Config) *ExpandAccountHandler {
	return &ExpandAccountHandler{service: service, adminService: adminService, cfg: cfg}
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

type reportExpandAccountRequest struct {
	ID          int64  `json:"id" binding:"required"`
	Email       string `json:"email" binding:"required"`
	LoginStatus *int64 `json:"login_status" binding:"required"`
	DeviceID    string `json:"device_id"`
	APIKey      string `json:"api_key"`
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
	AccountID        *int64                          `json:"account_id,omitempty"`
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
	filters := service.ExpandAccountListFilters{
		Search:      strings.TrimSpace(c.Query("search")),
		Used:        strings.TrimSpace(c.Query("used")),
		AccountType: strings.TrimSpace(c.Query("account_type")),
	}
	if raw := strings.TrimSpace(c.Query("login_status")); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
			filters.LoginStatus = &v
		}
	}
	items, total, err := h.service.ListExpandAccounts(c.Request.Context(), page, pageSize, filters)
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
		AccountID:        item.AccountID,
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

func (h *ExpandAccountHandler) Report(c *gin.Context) {
	var req reportExpandAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Warn("expand-accounts report: invalid request body", "client_ip", c.ClientIP(), "err", err)
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_REQUEST", "invalid request body").WithCause(err))
		return
	}

	loginStatusValue := int64(-1)
	if req.LoginStatus != nil {
		loginStatusValue = *req.LoginStatus
	}
	slog.Info("expand-accounts report: entry",
		"client_ip", c.ClientIP(),
		"id", req.ID,
		"email", req.Email,
		"login_status", loginStatusValue,
		"device_id", req.DeviceID,
		"api_key_present", strings.TrimSpace(req.APIKey) != "",
	)

	if req.LoginStatus == nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_LOGIN_STATUS", "login_status is required"))
		return
	}
	loginStatus := *req.LoginStatus
	if loginStatus != 1 && loginStatus != 2 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_LOGIN_STATUS", "login_status must be 1 or 2"))
		return
	}

	deviceID := strings.TrimSpace(req.DeviceID)
	apiKey := strings.TrimSpace(req.APIKey)
	if loginStatus == 1 {
		if deviceID == "" {
			response.ErrorFrom(c, infraerrors.BadRequest("DEVICE_ID_REQUIRED", "device_id is required when login_status is 1"))
			return
		}
		if apiKey == "" {
			response.ErrorFrom(c, infraerrors.BadRequest("API_KEY_REQUIRED", "api_key is required when login_status is 1"))
			return
		}
	}

	item, err := h.service.ReportExpandAccountLogin(c.Request.Context(), &service.ExpandAccountReportInput{
		ID:          req.ID,
		Email:       strings.TrimSpace(req.Email),
		LoginStatus: loginStatus,
		DeviceID:    deviceID,
		APIKey:      apiKey,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	if loginStatus == 1 && item != nil && strings.EqualFold(strings.TrimSpace(item.Platform), "anthropic") {
		h.submitOpenAPIChannel(c.Request.Context(), item.Email, apiKey)
	}

	response.Success(c, nil)
}

// submitOpenAPIChannel 向外部渠道保存接口提交一条新渠道记录。
// 失败仅记录日志，不影响主流程返回，避免外部接口故障阻塞上号回调。
func (h *ExpandAccountHandler) submitOpenAPIChannel(ctx context.Context, email, apiKey string) {
	if h.cfg == nil {
		slog.Warn("expand-accounts report: openapi_channel skipped, cfg is nil")
		return
	}
	url := strings.TrimSpace(h.cfg.OpenAPIChannel.URL)
	token := strings.TrimSpace(h.cfg.OpenAPIChannel.Token)
	if url == "" || token == "" {
		slog.Warn("expand-accounts report: openapi_channel skipped, url/token not configured")
		return
	}

	payload := map[string]any{
		"mode": "single",
		"channel": map[string]any{
			"type":     14,
			"key":      apiKey,
			"name":     email,
			"base_url": "http://claude.opsapi.cn",
			"models":   "claude-opus-4-7,claude-sonnet-4-6,claude-opus-4-6,claude-haiku-4-5-20251001,claude-opus-4-8",
			"group":    "claude专属",
			"tag":      "opsapi",
			"remark":   "",
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		slog.Error("expand-accounts report: marshal openapi_channel payload failed", "err", err)
		return
	}

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		slog.Error("expand-accounts report: build openapi_channel request failed", "err", err)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-OpenAPI-Token", token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		slog.Error("expand-accounts report: call openapi_channel failed", "url", url, "err", err)
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slog.Error("expand-accounts report: openapi_channel returned non-2xx",
			"url", url,
			"status", resp.StatusCode,
			"body", string(respBody),
		)
		return
	}
	slog.Info("expand-accounts report: openapi_channel submitted",
		"email", email,
		"status", resp.StatusCode,
	)
}
