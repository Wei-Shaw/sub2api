// oidc_provider_handler.go 实现 sub2api 作为 OIDC Provider (OP) 的对外 HTTP 端点：
//
//	GET  /.well-known/openid-configuration   Discovery 文档
//	GET  /.well-known/jwks.json              公钥集 (JWKS)
//	GET  /oidc/authorize                     授权端点 (浏览器跳转)
//	POST /oidc/token                         令牌端点
//	GET  /oidc/userinfo                      用户信息端点
//
// 错误响应遵循 design.md D10：
//   - authorize: 能信任 redirect_uri 时 302 回跳 error+state；否则 400 JSON
//   - token / userinfo: 4xx JSON {error, error_description}
//
// 当 oidc_provider.enabled=false 时所有端点一律 404 (design.md D11)。
package handler

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// OidcProviderHandler 编排 OIDC OP 对外端点。
type OidcProviderHandler struct {
	provider *service.OidcProviderService
	sso      *service.SsoSessionService
	apiKeys  *service.APIKeyService
}

// NewOidcProviderHandler 构造 handler。
func NewOidcProviderHandler(
	provider *service.OidcProviderService,
	sso *service.SsoSessionService,
	apiKeys *service.APIKeyService,
) *OidcProviderHandler {
	return &OidcProviderHandler{provider: provider, sso: sso, apiKeys: apiKeys}
}

// ─── 内部：错误输出 ──────────────────────────────────────────────────────────

// writeOAuthError 以 OIDC 规范的 JSON 形式输出错误 (token / userinfo / 不可回跳的 authorize)。
func writeOAuthError(c *gin.Context, e *service.OidcError) {
	status := e.Status
	if status == 0 {
		status = http.StatusBadRequest
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.JSON(status, gin.H{
		"error":             e.Code,
		"error_description": e.Description,
	})
}

// redirectAuthorizeError 把 authorize 阶段错误以 query 形式回跳给 RP。
func redirectAuthorizeError(c *gin.Context, redirectURI, state string, e *service.OidcError) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		writeOAuthError(c, e)
		return
	}
	q := u.Query()
	q.Set("error", e.Code)
	if e.Description != "" {
		q.Set("error_description", e.Description)
	}
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	c.Redirect(http.StatusFound, u.String())
}

// ─── Discovery ───────────────────────────────────────────────────────────────

// Discovery GET /.well-known/openid-configuration
func (h *OidcProviderHandler) Discovery(c *gin.Context) {
	ctx := c.Request.Context()
	if !h.provider.IsEnabled(ctx) {
		c.Status(http.StatusNotFound)
		return
	}
	doc, err := h.provider.Discovery(ctx)
	if err != nil {
		writeOAuthError(c, service.NewOidcError("server_error", "discovery unavailable", http.StatusInternalServerError))
		return
	}
	c.JSON(http.StatusOK, doc)
}

// JWKS GET /.well-known/jwks.json
func (h *OidcProviderHandler) JWKS(c *gin.Context) {
	ctx := c.Request.Context()
	if !h.provider.IsEnabled(ctx) {
		c.Status(http.StatusNotFound)
		return
	}
	c.JSON(http.StatusOK, gin.H{"keys": h.provider.JWKS()})
}

// ─── Authorize ───────────────────────────────────────────────────────────────

// Authorize GET /oidc/authorize
func (h *OidcProviderHandler) Authorize(c *gin.Context) {
	ctx := c.Request.Context()
	if !h.provider.IsEnabled(ctx) {
		c.Status(http.StatusNotFound)
		return
	}

	req := service.OidcAuthorizeRequest{
		ClientID:            c.Query("client_id"),
		RedirectURI:         c.Query("redirect_uri"),
		ResponseType:        c.Query("response_type"),
		Scope:               c.Query("scope"),
		State:               c.Query("state"),
		Nonce:               c.Query("nonce"),
		CodeChallenge:       c.Query("code_challenge"),
		CodeChallengeMethod: c.Query("code_challenge_method"),
		Prompt:              c.Query("prompt"),
	}

	validated, redirectable, oerr := h.provider.ValidateAuthorize(ctx, req)
	if oerr != nil {
		if redirectable {
			redirectAuthorizeError(c, req.RedirectURI, req.State, oerr)
			return
		}
		writeOAuthError(c, oerr)
		return
	}

	// 解析登录态 (SSO cookie)。prompt=login 时强制重新登录。
	forceLogin := strings.Contains(req.Prompt, "login")
	sess, serr := h.sso.Resolve(ctx, c.Request)
	if serr != nil || sess == nil || forceLogin {
		h.redirectToLogin(c)
		return
	}
	// 异步刷新 last_seen_at (限流)。
	h.sso.TouchLastSeen(sess.SessionID)

	userID := sess.UserID

	// 是否需要展示同意页。
	needed, err := h.provider.ConsentNeeded(ctx, validated.Client, userID, validated.Scopes)
	if err != nil {
		redirectAuthorizeError(c, req.RedirectURI, req.State,
			service.NewOidcError("server_error", "consent check failed", http.StatusInternalServerError))
		return
	}
	if needed {
		token, terr := h.provider.IssueConsentToken(ctx, service.OidcConsentTokenInput{
			UserID:      userID,
			ClientID:    validated.Client.ClientID,
			Scopes:      validated.Scopes,
			RedirectURI: req.RedirectURI,
			State:       req.State,
			Nonce:       req.Nonce,
			Challenge:   req.CodeChallenge,
			Method:      req.CodeChallengeMethod,
		})
		if terr != nil {
			redirectAuthorizeError(c, req.RedirectURI, req.State,
				service.NewOidcError("server_error", "consent token issue failed", http.StatusInternalServerError))
			return
		}
		c.Redirect(http.StatusFound, "/oauth/consent?consent="+url.QueryEscape(token))
		return
	}

	// 无需 consent → 直接发码回跳。
	h.issueCodeAndRedirect(c, oidcCodeContext{
		client:      validated.Client,
		userID:      userID,
		scopes:      validated.Scopes,
		redirectURI: req.RedirectURI,
		state:       req.State,
		nonce:       req.Nonce,
		challenge:   req.CodeChallenge,
		method:      req.CodeChallengeMethod,
	})
}

// oidcCodeContext 汇集发码所需上下文 (authorize / consent POST 共用)。
type oidcCodeContext struct {
	client      *service.OidcClientView
	userID      int64
	scopes      []string
	redirectURI string
	state       string
	nonce       string
	challenge   string
	method      string
}

// issueCodeAndRedirect 发授权码并 302 回跳 redirect_uri?code=&state=。
func (h *OidcProviderHandler) issueCodeAndRedirect(c *gin.Context, in oidcCodeContext) {
	ctx := c.Request.Context()
	code, err := h.provider.IssueCode(ctx, service.OidcIssueCodeInput{
		Client:      in.client,
		UserID:      in.userID,
		Scopes:      in.scopes,
		RedirectURI: in.redirectURI,
		Nonce:       in.nonce,
		Challenge:   in.challenge,
		Method:      in.method,
	})
	if err != nil {
		redirectAuthorizeError(c, in.redirectURI, in.state,
			service.NewOidcError("server_error", "code issue failed", http.StatusInternalServerError))
		return
	}
	u, perr := url.Parse(in.redirectURI)
	if perr != nil {
		writeOAuthError(c, service.NewOidcError("server_error", "invalid redirect_uri", http.StatusInternalServerError))
		return
	}
	q := u.Query()
	q.Set("code", code)
	if in.state != "" {
		q.Set("state", in.state)
	}
	u.RawQuery = q.Encode()
	c.Redirect(http.StatusFound, u.String())
}

// redirectToLogin 把未登录用户跳转到前端登录页，登录后回跳本次 authorize 请求。
func (h *OidcProviderHandler) redirectToLogin(c *gin.Context) {
	next := c.Request.URL.RequestURI()
	c.Redirect(http.StatusFound, "/login?redirect="+url.QueryEscape(next))
}

// ─── Token ───────────────────────────────────────────────────────────────────

// Token POST /oidc/token
func (h *OidcProviderHandler) Token(c *gin.Context) {
	ctx := c.Request.Context()
	if !h.provider.IsEnabled(ctx) {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")

	// client 鉴权：client_secret_basic 优先，回退 client_secret_post。
	clientID, secret, ok := c.Request.BasicAuth()
	if !ok {
		clientID = c.PostForm("client_id")
		secret = c.PostForm("client_secret")
	}
	client, oerr := h.provider.AuthenticateClient(ctx, clientID, secret)
	if oerr != nil {
		if oerr.Code == "invalid_client" {
			c.Header("WWW-Authenticate", `Basic realm="oidc"`)
		}
		writeOAuthError(c, oerr)
		return
	}

	switch c.PostForm("grant_type") {
	case "authorization_code":
		resp, terr := h.provider.ExchangeCode(ctx, service.OidcExchangeCodeInput{
			Client:       client,
			Code:         c.PostForm("code"),
			RedirectURI:  c.PostForm("redirect_uri"),
			CodeVerifier: c.PostForm("code_verifier"),
		})
		if terr != nil {
			writeOAuthError(c, terr)
			return
		}
		c.JSON(http.StatusOK, resp)
	case "refresh_token":
		resp, terr := h.provider.RefreshToken(ctx, service.OidcRefreshInput{
			Client:       client,
			RefreshToken: c.PostForm("refresh_token"),
			Scope:        c.PostForm("scope"),
		})
		if terr != nil {
			writeOAuthError(c, terr)
			return
		}
		c.JSON(http.StatusOK, resp)
	default:
		writeOAuthError(c, service.NewOidcError("unsupported_grant_type",
			"only authorization_code and refresh_token are supported", http.StatusBadRequest))
	}
}

// ─── UserInfo ────────────────────────────────────────────────────────────────

// UserInfo GET /oidc/userinfo
func (h *OidcProviderHandler) UserInfo(c *gin.Context) {
	ctx := c.Request.Context()
	if !h.provider.IsEnabled(ctx) {
		c.Status(http.StatusNotFound)
		return
	}

	token := bearerToken(c.GetHeader("Authorization"))
	if token == "" {
		token = c.PostForm("access_token")
	}
	if token == "" {
		c.Header("WWW-Authenticate", `Bearer realm="oidc"`)
		writeOAuthError(c, service.NewOidcError("invalid_token", "missing access token", http.StatusUnauthorized))
		return
	}

	claims, oerr := h.provider.BuildUserInfo(ctx, token)
	if oerr != nil {
		if oerr.Status == http.StatusUnauthorized {
			c.Header("WWW-Authenticate", `Bearer realm="oidc", error="`+oerr.Code+`", error_description="`+oerr.Description+`"`)
		}
		writeOAuthError(c, oerr)
		return
	}
	c.JSON(http.StatusOK, claims)
}

// bearerToken 从 Authorization 头提取 Bearer token；非 Bearer 返回空串。
func bearerToken(authHeader string) string {
	authHeader = strings.TrimSpace(authHeader)
	const prefix = "Bearer "
	if len(authHeader) > len(prefix) && strings.EqualFold(authHeader[:len(prefix)], prefix) {
		return strings.TrimSpace(authHeader[len(prefix):])
	}
	return ""
}

// ─── Resource: API Keys ──────────────────────────────────────────────────────

// resolveOidcBearer 校验 Authorization Bearer token，要求 access token 携带指定 scope。
//
// 失败时已写好响应（含 WWW-Authenticate 头），调用方直接 return 即可。
func (h *OidcProviderHandler) resolveOidcBearer(c *gin.Context, requiredScope string) (*service.OidcResolvedAccessToken, bool) {
	ctx := c.Request.Context()
	if !h.provider.IsEnabled(ctx) {
		c.Status(http.StatusNotFound)
		return nil, false
	}

	token := bearerToken(c.GetHeader("Authorization"))
	if token == "" {
		c.Header("WWW-Authenticate", `Bearer realm="oidc"`)
		writeOAuthError(c, service.NewOidcError("invalid_token", "missing access token", http.StatusUnauthorized))
		return nil, false
	}

	resolved, oerr := h.provider.ResolveAccessToken(ctx, token)
	if oerr != nil {
		if oerr.Status == http.StatusUnauthorized {
			c.Header("WWW-Authenticate", `Bearer realm="oidc", error="`+oerr.Code+`", error_description="`+oerr.Description+`"`)
		}
		writeOAuthError(c, oerr)
		return nil, false
	}

	if requiredScope != "" && !resolved.HasScope(requiredScope) {
		c.Header("WWW-Authenticate", `Bearer realm="oidc", error="insufficient_scope", scope="`+requiredScope+`"`)
		writeOAuthError(c, service.NewOidcError("insufficient_scope", "scope "+requiredScope+" is required", http.StatusForbidden))
		return nil, false
	}
	return resolved, true
}

// ListAPIKeys GET /oidc/resource/api-keys
//
// OAuth2 受保护资源端点：以 OIDC access_token 鉴权，返回当前 access_token 绑定用户的
// API Key 列表（含 key 明文）。
//
// 鉴权要求：
//   - 提供 Authorization: Bearer <opaque-access-token>
//   - access_token 必须携带 scope `sub2api:apikey`
//
// 查询参数（与 /api/v1/user/keys 对齐）：
//   - page (default=1) / page_size (default=20, 最大见 ParsePagination)
//   - sort_by / sort_order (默认 created_at desc)
//   - search / status / group_id
//
// 响应体形如 response.Paginated：{data: [APIKey...], total, page, page_size}。
func (h *OidcProviderHandler) ListAPIKeys(c *gin.Context) {
	resolved, ok := h.resolveOidcBearer(c, service.OidcScopeAPIKey)
	if !ok {
		return
	}

	if h.apiKeys == nil {
		writeOAuthError(c, service.NewOidcError("server_error", "api key service not available", http.StatusInternalServerError))
		return
	}

	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "created_at"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}

	var filters service.APIKeyListFilters
	if search := strings.TrimSpace(c.Query("search")); search != "" {
		if len(search) > 100 {
			search = search[:100]
		}
		filters.Search = search
	}
	filters.Status = c.Query("status")
	if groupIDStr := c.Query("group_id"); groupIDStr != "" {
		if gid, err := strconv.ParseInt(groupIDStr, 10, 64); err == nil {
			filters.GroupID = &gid
		}
	}

	keys, result, err := h.apiKeys.List(c.Request.Context(), resolved.UserID, params, filters)
	if err != nil {
		writeOAuthError(c, service.NewOidcError("server_error", "list api keys failed", http.StatusInternalServerError))
		return
	}

	out := make([]dto.APIKey, 0, len(keys))
	for i := range keys {
		out = append(out, *dto.APIKeyFromService(&keys[i]))
	}
	response.Paginated(c, out, result.Total, page, pageSize)
}
