package admin

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// kiroOAuthSvc is the narrow contract KiroOAuthHandler needs.
// *service.KiroOAuthService satisfies it; tests can pass a stub without
// constructing the full service.
type kiroOAuthSvc interface {
	ValidateSocialRefreshToken(ctx context.Context, refreshToken string, proxyID *int64) (*service.KiroTokenInfo, error)
	StartIdCLogin(ctx context.Context, startURL, region string, proxyID *int64) (*service.KiroIdCAuthURLResult, error)
	CompleteIdCLogin(ctx context.Context, sessionID, callbackURL string) (*service.KiroTokenInfo, error)
	StartBuilderIDLogin(ctx context.Context, region string, proxyID *int64) (*service.KiroBuilderIDLoginResult, error)
	PollBuilderIDLogin(ctx context.Context, sessionID string) (*service.KiroBuilderIDPollResult, error)
}

// KiroOAuthHandler exposes Kiro auth flows to admins.
// Phase 2: Social refresh-token paste. Phase 3 adds IdC + Builder ID.
type KiroOAuthHandler struct {
	svc kiroOAuthSvc
}

func NewKiroOAuthHandler(svc *service.KiroOAuthService) *KiroOAuthHandler {
	return &KiroOAuthHandler{svc: svc}
}

// KiroValidateSocialRefreshTokenRequest is the body of /admin/kiro/oauth/validate-social.
type KiroValidateSocialRefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
	ProxyID      *int64 `json:"proxy_id,omitempty"`
}

// ValidateSocialRefreshToken validates a pasted Kiro Social refresh token
// and returns the freshly-rotated tokens + best-effort email/userId for
// naming the new account.
//
// POST /api/v1/admin/kiro/oauth/validate-social
func (h *KiroOAuthHandler) ValidateSocialRefreshToken(c *gin.Context) {
	var req KiroValidateSocialRefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求无效: "+err.Error())
		return
	}

	tokenInfo, err := h.svc.ValidateSocialRefreshToken(c.Request.Context(), req.RefreshToken, req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, tokenInfo)
}

// KiroStartIdCLoginRequest is the body of /admin/kiro/oauth/idc/start.
type KiroStartIdCLoginRequest struct {
	StartURL string `json:"start_url" binding:"required"`
	Region   string `json:"region,omitempty"`
	ProxyID  *int64 `json:"proxy_id,omitempty"`
}

// StartIdCLogin starts a PKCE auth-code flow against the user-supplied
// AWS Identity Center startUrl.
//
// POST /api/v1/admin/kiro/oauth/idc/start
func (h *KiroOAuthHandler) StartIdCLogin(c *gin.Context) {
	var req KiroStartIdCLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求无效: "+err.Error())
		return
	}
	result, err := h.svc.StartIdCLogin(c.Request.Context(), req.StartURL, req.Region, req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// KiroCompleteIdCLoginRequest is the body of /admin/kiro/oauth/idc/complete.
type KiroCompleteIdCLoginRequest struct {
	SessionID   string `json:"session_id" binding:"required"`
	CallbackURL string `json:"callback_url" binding:"required"`
}

// CompleteIdCLogin finishes a PKCE auth-code flow. The admin pastes the
// full redirected URL (which includes ?code=...&state=...).
//
// POST /api/v1/admin/kiro/oauth/idc/complete
func (h *KiroOAuthHandler) CompleteIdCLogin(c *gin.Context) {
	var req KiroCompleteIdCLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求无效: "+err.Error())
		return
	}
	tokenInfo, err := h.svc.CompleteIdCLogin(c.Request.Context(), req.SessionID, req.CallbackURL)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, tokenInfo)
}

// KiroStartBuilderIDLoginRequest is the body of /admin/kiro/oauth/builderid/start.
type KiroStartBuilderIDLoginRequest struct {
	Region  string `json:"region,omitempty"`
	ProxyID *int64 `json:"proxy_id,omitempty"`
}

// StartBuilderIDLogin starts an AWS Builder ID device-code flow.
//
// POST /api/v1/admin/kiro/oauth/builderid/start
func (h *KiroOAuthHandler) StartBuilderIDLogin(c *gin.Context) {
	var req KiroStartBuilderIDLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Empty body is fine — region/proxy are optional. Reset req and continue.
		req = KiroStartBuilderIDLoginRequest{}
	}
	result, err := h.svc.StartBuilderIDLogin(c.Request.Context(), req.Region, req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// KiroPollBuilderIDLoginRequest is the body of /admin/kiro/oauth/builderid/poll.
type KiroPollBuilderIDLoginRequest struct {
	SessionID string `json:"session_id" binding:"required"`
}

// PollBuilderIDLogin polls a device-code session once and returns its
// current status. The UI loops on this endpoint with the interval the
// start response specified.
//
// POST /api/v1/admin/kiro/oauth/builderid/poll
func (h *KiroOAuthHandler) PollBuilderIDLogin(c *gin.Context) {
	var req KiroPollBuilderIDLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求无效: "+err.Error())
		return
	}
	result, err := h.svc.PollBuilderIDLogin(c.Request.Context(), req.SessionID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
