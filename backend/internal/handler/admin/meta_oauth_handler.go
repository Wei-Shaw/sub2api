package admin

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/meta"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type MetaOAuthHandler struct {
	metaOAuthService  *service.MetaOAuthService
	metaTokenProvider *service.MetaTokenProvider
	adminService      service.AdminService
}

func NewMetaOAuthHandler(metaOAuthService *service.MetaOAuthService, metaTokenProvider *service.MetaTokenProvider, adminService service.AdminService) *MetaOAuthHandler {
	return &MetaOAuthHandler{metaOAuthService: metaOAuthService, metaTokenProvider: metaTokenProvider, adminService: adminService}
}

type metaDeviceStartRequest struct {
	ProxyID *int64 `json:"proxy_id"`
}

type metaDeviceStartResponse struct {
	DeviceCodeID            string `json:"device_code_id"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	IntervalSeconds         int    `json:"interval"`
}

func (h *MetaOAuthHandler) StartDeviceFlow(c *gin.Context) {
	var req metaDeviceStartRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	start, err := h.metaOAuthService.StartDeviceFlow(c.Request.Context(), req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, metaDeviceStartResponse{
		DeviceCodeID:            start.DeviceCodeID,
		UserCode:                start.UserCode,
		VerificationURI:         start.VerificationURI,
		VerificationURIComplete: start.VerificationURIComplete,
		ExpiresIn:               start.ExpiresIn,
		IntervalSeconds:         int(start.Interval),
	})
}

type metaDevicePollRequest struct {
	DeviceCodeID string  `json:"device_code_id" binding:"required"`
	Name         string  `json:"name"`
	Concurrency  int     `json:"concurrency"`
	Priority     int     `json:"priority"`
	GroupIDs     []int64 `json:"group_ids"`
	ProxyID      *int64  `json:"proxy_id"`
}

type metaDevicePollResponse struct {
	State           string       `json:"state"`
	IntervalSeconds int          `json:"interval"`
	Message         string       `json:"message,omitempty"`
	Account         *dto.Account `json:"account,omitempty"`
}

func (h *MetaOAuthHandler) PollDeviceFlow(c *gin.Context) {
	var req metaDevicePollRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	poll, err := h.metaOAuthService.PollDeviceFlow(c.Request.Context(), req.DeviceCodeID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := metaDevicePollResponse{
		State:           string(poll.State),
		IntervalSeconds: int(poll.Interval / time.Second),
		Message:         poll.Message,
	}
	if poll.State != meta.PollAuthorized {
		response.Success(c, out)
		return
	}
	if !h.metaOAuthService.ClaimAccountCreation(req.DeviceCodeID) {
		response.Success(c, out)
		return
	}
	account, err := h.createDeviceFlowAccount(c, req, poll)
	if err != nil {
		h.metaOAuthService.ReleaseAccountCreation(req.DeviceCodeID)
		response.ErrorFrom(c, err)
		return
	}
	out.Account = dto.AccountFromService(account)
	response.Success(c, out)
}

func (h *MetaOAuthHandler) createDeviceFlowAccount(c *gin.Context, req metaDevicePollRequest, poll *service.MetaDevicePoll) (*service.Account, error) {
	if poll.MintedKey == nil {
		response.Error(c, http.StatusBadGateway, "Meta mint did not return an API key")
		return nil, errMetaMintMissing
	}
	name := strings.TrimSpace(req.Name)
	if name == "" && poll.MintedKey != nil {
		name = strings.TrimSpace(poll.MintedKey.UserEmail)
	}
	if name == "" && poll.MintedKey != nil {
		name = strings.TrimSpace(poll.MintedKey.UserFullName)
	}
	if name == "" {
		name = "Meta Muse Account"
	}
	credentials := h.metaOAuthService.BuildDeviceFlowCredentials(poll.MintedKey, poll.DCAToken)
	return h.adminService.CreateAccount(c.Request.Context(), &service.CreateAccountInput{
		Name:        name,
		Platform:    service.PlatformMeta,
		Type:        service.AccountTypeOAuth,
		Credentials: credentials,
		ProxyID:     req.ProxyID,
		Concurrency: req.Concurrency,
		Priority:    req.Priority,
		GroupIDs:    req.GroupIDs,
	})
}

func (h *MetaOAuthHandler) RefreshAccountKey(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	account, err := h.adminService.GetAccount(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if account == nil || !account.IsMeta() {
		response.Error(c, http.StatusBadRequest, "Account is not a Meta account")
		return
	}
	if account.GetMetaDCAToken() == "" {
		response.Error(c, http.StatusBadRequest, "Account has no DCA token to mint from")
		return
	}
	if h.metaTokenProvider == nil {
		response.Error(c, http.StatusServiceUnavailable, "Meta token provider is not available")
		return
	}
	if err := h.metaTokenProvider.InvalidateAPIKey(c.Request.Context(), account); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	apiKey, err := h.metaTokenProvider.GetAPIKey(c.Request.Context(), account)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"api_key_present": strings.TrimSpace(apiKey) != "",
		"email":           account.GetCredential("email"),
		"subs_tier_name":  account.GetCredential("subs_tier_name"),
		"is_subs_active":  account.GetCredential("is_subs_active"),
	})
}

var errMetaMintMissing = errors.New("meta: minted key is missing")
