package handler

// oidc_provider_consent_handler.go 实现 OIDC 同意页的后端 API：
//
//	GET  /oidc/consent   解码同意令牌 + 校验登录态，返回 client / scope 展示数据
//	POST /oidc/consent   用户 allow/deny 决策；allow 时记忆同意并发码，返回 {redirect}
//
// 同意页本身是前端 SPA 路由 /oauth/consent，通过 ?consent=<token> 携带由
// OidcProviderService 签发的短期签名令牌 (无状态 consent session)。
//
// POST 返回 JSON {redirect}，由前端执行 window.location 跳转，避免 XHR 上的
// 跨域 302 问题。

import (
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// oidcConsentScopeView 单个 scope 的展示信息 (前端据此渲染人话描述/红字警示)。
type oidcConsentScopeView struct {
	Scope     string `json:"scope"`
	Sensitive bool   `json:"sensitive"`
}

// ConsentGet GET /oidc/consent?consent=<token>
//
// 返回当前同意请求的展示信息。要求 SSO 登录态与令牌 sub 一致 (防 CSRF/会话错配)。
func (h *OidcProviderHandler) ConsentGet(c *gin.Context) {
	ctx := c.Request.Context()
	if !h.provider.IsEnabled(ctx) {
		c.Status(http.StatusNotFound)
		return
	}

	claims, oerr := h.provider.DecodeConsentToken(c.Query("consent"))
	if oerr != nil {
		writeOAuthError(c, oerr)
		return
	}

	sess, serr := h.sso.Resolve(ctx, c.Request)
	if serr != nil || sess == nil || sess.UserID != claims.UserID {
		writeOAuthError(c, service.NewOidcError("login_required",
			"login required", http.StatusUnauthorized))
		return
	}

	clientName := claims.ClientID
	if view, err := h.provider.LookupClient(ctx, claims.ClientID); err == nil && view != nil {
		clientName = view.ClientName
	}

	scopes := make([]oidcConsentScopeView, 0, len(claims.Scopes))
	for _, sc := range claims.Scopes {
		scopes = append(scopes, oidcConsentScopeView{
			Scope:     sc,
			Sensitive: isSensitiveScope(sc),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"client_id":   claims.ClientID,
		"client_name": clientName,
		"scopes":      scopes,
	})
}

// consentDecisionRequest POST /oidc/consent 请求体。
type consentDecisionRequest struct {
	Consent string `json:"consent" form:"consent"`
	Action  string `json:"action" form:"action"` // "allow" | "deny"
}

// ConsentPost POST /oidc/consent
//
// allow → 记忆同意 + 发码 → 返回 {redirect: redirect_uri?code=&state=}
// deny  → 返回 {redirect: redirect_uri?error=access_denied&state=}
func (h *OidcProviderHandler) ConsentPost(c *gin.Context) {
	ctx := c.Request.Context()
	if !h.provider.IsEnabled(ctx) {
		c.Status(http.StatusNotFound)
		return
	}

	var req consentDecisionRequest
	if err := c.ShouldBind(&req); err != nil {
		writeOAuthError(c, service.NewOidcError("invalid_request", "malformed request", http.StatusBadRequest))
		return
	}

	claims, oerr := h.provider.DecodeConsentToken(req.Consent)
	if oerr != nil {
		writeOAuthError(c, oerr)
		return
	}

	sess, serr := h.sso.Resolve(ctx, c.Request)
	if serr != nil || sess == nil || sess.UserID != claims.UserID {
		writeOAuthError(c, service.NewOidcError("login_required",
			"login required", http.StatusUnauthorized))
		return
	}

	// 拒绝授权 → 回跳 access_denied。
	if req.Action != "allow" {
		c.JSON(http.StatusOK, gin.H{
			"redirect": buildErrorRedirect(claims.RedirectURI, claims.State, "access_denied", "user denied consent"),
		})
		return
	}

	// 记忆同意 (并集语义)。
	if err := h.provider.RecordConsent(ctx, claims.UserID, claims.ClientID, claims.Scopes); err != nil {
		writeOAuthError(c, service.NewOidcError("server_error", "record consent failed", http.StatusInternalServerError))
		return
	}

	client, err := h.provider.LookupClient(ctx, claims.ClientID)
	if err != nil || client == nil {
		writeOAuthError(c, service.NewOidcError("server_error", "client lookup failed", http.StatusInternalServerError))
		return
	}

	code, cerr := h.provider.IssueCode(ctx, service.OidcIssueCodeInput{
		Client:      client,
		UserID:      claims.UserID,
		Scopes:      claims.Scopes,
		RedirectURI: claims.RedirectURI,
		Nonce:       claims.Nonce,
		Challenge:   claims.Challenge,
		Method:      claims.Method,
	})
	if cerr != nil {
		writeOAuthError(c, service.NewOidcError("server_error", "code issue failed", http.StatusInternalServerError))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"redirect": buildSuccessRedirect(claims.RedirectURI, claims.State, code),
	})
}

// isSensitiveScope 标记需要红字警示的私有 scope (design.md D8)。
func isSensitiveScope(scope string) bool {
	return scope == service.OidcScopeBalance || scope == service.OidcScopeAPIKey
}

// buildSuccessRedirect 拼 redirect_uri?code=&state=。
func buildSuccessRedirect(redirectURI, state, code string) string {
	u, err := url.Parse(redirectURI)
	if err != nil {
		return redirectURI
	}
	q := u.Query()
	q.Set("code", code)
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// buildErrorRedirect 拼 redirect_uri?error=&error_description=&state=。
func buildErrorRedirect(redirectURI, state, code, desc string) string {
	u, err := url.Parse(redirectURI)
	if err != nil {
		return redirectURI
	}
	q := u.Query()
	q.Set("error", code)
	if desc != "" {
		q.Set("error_description", desc)
	}
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	return u.String()
}
