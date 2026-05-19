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
}

// KiroOAuthHandler exposes Kiro auth flows to admins.
// Phase 2: Social refresh-token paste only. Phase 3 adds IdC + Builder ID.
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
