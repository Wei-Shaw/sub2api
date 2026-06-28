// Package service ...
//
// oidc_provider_service.go 实现 sub2api 作为 OIDC Provider (OP) 的核心协议逻辑：
//
//   - HandleAuthorize 阶段的参数校验 (response_type / scope / redirect_uri / PKCE)
//   - 授权码签发 (opaque, 一次性, 默认 10 分钟)
//   - ExchangeCode: authorization_code 兑换 access/refresh/id token
//   - RefreshToken: refresh_token 轮转 + family reuse 检测
//   - BuildUserInfo: 依据 access token 存储的 scope 投影用户声明
//
// 本服务编排已有的 OidcSigningService / OidcClientService / OidcConsentService，
// 不直接处理 HTTP；redirect-vs-JSON 的错误分支由 handler 层依据 OidcError 决策。
//
// 设计决策 (与 design.md 对齐 + 本期受限于 schema 不变的务实取舍)：
//   - PKCE 仅支持 S256 (决策 3)；plain 一律拒绝。
//   - id_token 的 auth_time 取授权码的 created_at (≈ 用户完成 authorize 的时刻)；
//     acr 固定为 urn:sub2api:authn:basic。mfa-aware acr/amr 因授权码表未持久化
//     认证上下文字段而推迟 (避免本期再次 ent codegen + 改 migration)。
//   - 私有 scope (sub2api:apikey) 只在 UserInfo 暴露，绝不进 id_token (D8)。
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/oidcaccesstoken"
	"github.com/Wei-Shaw/sub2api/ent/oidcauthorizationcode"
	"github.com/Wei-Shaw/sub2api/ent/oidcrefreshtoken"

	"github.com/golang-jwt/jwt/v5"
)

// ─── 常量 ────────────────────────────────────────────────────────────────────

const (
	// OidcAcrBasic 基础认证 (用户名口令 / OAuth 登录) 的 acr 值。
	OidcAcrBasic = "urn:sub2api:authn:basic"
	// OidcAcrMfa 含二次验证的 acr 值 (预留)。
	OidcAcrMfa = "urn:sub2api:authn:mfa"

	// oidcTokenRandBytes opaque token (code/access/refresh) 随机字节数。
	oidcTokenRandBytes = 32

	// OidcScopeOpenID 等标准 scope 常量。
	OidcScopeOpenID        = "openid"
	OidcScopeProfile       = "profile"
	OidcScopeEmail         = "email"
	OidcScopeOfflineAccess = "offline_access"

	OidcScopeAPIKey = "sub2api:apikey"
)

// ─── OAuth2 错误 ─────────────────────────────────────────────────────────────

// OidcError 表示一个 OAuth2 / OIDC 协议错误。
//
// handler 层据此决定：
//   - authorize 阶段若 redirect_uri 与 response_type 合法 → 以 query 形式重定向回 RP；
//     否则直接渲染错误 (不可信 redirect 不能跳)。
//   - token / userinfo 阶段统一以 JSON (+ 适当 HTTP 状态码) 返回。
type OidcError struct {
	Code        string // OAuth2 error 代码，如 invalid_request
	Description string // 人类可读描述
	Status      int    // 建议 HTTP 状态码 (JSON 分支用)
}

func (e *OidcError) Error() string {
	if e == nil {
		return ""
	}
	if e.Description == "" {
		return e.Code
	}
	return e.Code + ": " + e.Description
}

func newOidcError(code, desc string, status int) *OidcError {
	return &OidcError{Code: code, Description: desc, Status: status}
}

// NewOidcError 供 handler 层构造协议错误 (如 unsupported_grant_type)。
func NewOidcError(code, desc string, status int) *OidcError {
	return newOidcError(code, desc, status)
}

// 常见错误构造器。
func errInvalidRequest(desc string) *OidcError { return newOidcError("invalid_request", desc, 400) }
func errInvalidClient(desc string) *OidcError  { return newOidcError("invalid_client", desc, 401) }
func errInvalidGrant(desc string) *OidcError   { return newOidcError("invalid_grant", desc, 400) }
func errInvalidScope(desc string) *OidcError   { return newOidcError("invalid_scope", desc, 400) }
func errServerError(desc string) *OidcError    { return newOidcError("server_error", desc, 500) }
func errUnsupportedResType(d string) *OidcError {
	return newOidcError("unsupported_response_type", d, 400)
}

// ─── 数据结构 ────────────────────────────────────────────────────────────────

// OidcAuthorizeRequest /oidc/authorize 的原始查询参数。
type OidcAuthorizeRequest struct {
	ClientID            string
	RedirectURI         string
	ResponseType        string
	Scope               string // 空格分隔
	State               string
	Nonce               string
	CodeChallenge       string
	CodeChallengeMethod string
	Prompt              string
}

// OidcValidatedAuthorize 通过校验后的 authorize 请求快照。
type OidcValidatedAuthorize struct {
	Client *OidcClientView
	Scopes []string
}

// OidcIssueCodeInput 签发授权码所需上下文。
type OidcIssueCodeInput struct {
	Client      *OidcClientView
	UserID      int64
	Scopes      []string
	RedirectURI string
	Nonce       string
	Challenge   string
	Method      string
}

// OidcExchangeCodeInput authorization_code 兑换入参 (client 已通过 Authenticate)。
type OidcExchangeCodeInput struct {
	Client       *OidcClientView
	Code         string
	RedirectURI  string
	CodeVerifier string
}

// OidcRefreshInput refresh_token 兑换入参。
type OidcRefreshInput struct {
	Client       *OidcClientView
	RefreshToken string
	Scope        string // 可选；非空时必须为原 scope 子集 (降权)
}

// OidcTokenResponse /oidc/token 的成功响应体。
type OidcTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// ─── 服务体 ──────────────────────────────────────────────────────────────────

// OidcProviderService 编排 OIDC OP 核心协议。
type OidcProviderService struct {
	client      *ent.Client
	settingRepo SettingRepository
	signing     *OidcSigningService
	clients     *OidcClientService
	consents    *OidcConsentService

	now          func() time.Time
	randReadFunc func([]byte) (int, error)
}

// NewOidcProviderService 构造服务。所有依赖必须非 nil。
func NewOidcProviderService(
	client *ent.Client,
	settingRepo SettingRepository,
	signing *OidcSigningService,
	clients *OidcClientService,
	consents *OidcConsentService,
) *OidcProviderService {
	return &OidcProviderService{
		client:       client,
		settingRepo:  settingRepo,
		signing:      signing,
		clients:      clients,
		consents:     consents,
		now:          func() time.Time { return time.Now().UTC() },
		randReadFunc: rand.Read,
	}
}

// ─── 配置读取 ────────────────────────────────────────────────────────────────

// IsEnabled 读取 oidc_provider.enabled。读取失败或未设置时返回默认值 false。
func (s *OidcProviderService) IsEnabled(ctx context.Context) bool {
	v, err := s.settingRepo.GetValue(ctx, SettingKeyOidcProviderEnabled)
	if err != nil {
		return DefaultOidcProviderEnabled
	}
	return strings.EqualFold(strings.TrimSpace(v), "true")
}

// IssuerURL 读取并校验 issuer_url。空或非法返回错误。
func (s *OidcProviderService) IssuerURL(ctx context.Context) (string, error) {
	v, err := s.settingRepo.GetValue(ctx, SettingKeyOidcProviderIssuerURL)
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return "", fmt.Errorf("oidc provider: read issuer_url: %w", err)
	}
	v = strings.TrimSpace(v)
	if err := ValidateOidcIssuerURL(v); err != nil {
		return "", err
	}
	return v, nil
}

func (s *OidcProviderService) ttlSeconds(ctx context.Context, key string, def int) int {
	v, err := s.settingRepo.GetValue(ctx, key)
	if err != nil {
		return def
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

// Discovery 构造 OIDC Discovery 文档 (RFC 8414 / OIDC Discovery)。
func (s *OidcProviderService) Discovery(ctx context.Context) (map[string]any, error) {
	issuer, err := s.IssuerURL(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"issuer":                                issuer,
		"authorization_endpoint":                issuer + "/oidc/authorize",
		"token_endpoint":                        issuer + "/oidc/token",
		"userinfo_endpoint":                     issuer + "/oidc/userinfo",
		"jwks_uri":                              issuer + "/.well-known/jwks.json",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"scopes_supported":                      AllowedOidcProviderScopes,
		"token_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post"},
		"code_challenge_methods_supported":      []string{"S256"},
		"claims_supported": []string{
			"sub", "iss", "aud", "exp", "iat", "auth_time", "nonce", "acr",
			"name", "preferred_username", "email", "email_verified",
		},
	}, nil
}

// AuthenticateClient 供 token 端点鉴权 client (client_secret_basic / client_secret_post)。
// 失败统一归一为 invalid_client，不泄露具体原因。
func (s *OidcProviderService) AuthenticateClient(ctx context.Context, clientID, secret string) (*OidcClientView, *OidcError) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return nil, errInvalidClient("missing client credentials")
	}
	view, err := s.clients.Authenticate(ctx, clientID, secret)
	if err != nil {
		switch {
		case errors.Is(err, ErrOidcClientNotFound), errors.Is(err, ErrOidcClientWrongSecret):
			return nil, errInvalidClient("client authentication failed")
		case errors.Is(err, ErrOidcClientDisabled):
			return nil, errInvalidClient("client disabled")
		default:
			return nil, errServerError("client authentication error")
		}
	}
	return view, nil
}

// LookupClient 仅按 client_id 取 client 视图 (consent 页展示用，不校验 secret)。
func (s *OidcProviderService) LookupClient(ctx context.Context, clientID string) (*OidcClientView, error) {
	return s.clients.GetByClientID(ctx, clientID)
}

// JWKS 透传签名服务的公钥集投影 (供 /.well-known/jwks.json)。
func (s *OidcProviderService) JWKS() []map[string]any {
	return s.signing.JWKS()
}

// ─── Authorize 校验 ──────────────────────────────────────────────────────────

// ValidateAuthorize 校验 authorize 请求 (不涉及用户登录态)。
//
// 返回 (validated, redirectable, err)：
//   - redirectable=true 表示 redirect_uri 与 response_type 都已确认合法，handler 可以
//     把错误以 query 形式回跳给 RP；false 表示连 redirect_uri 都不可信，必须直接渲染错误。
func (s *OidcProviderService) ValidateAuthorize(ctx context.Context, req OidcAuthorizeRequest) (*OidcValidatedAuthorize, bool, *OidcError) {
	clientID := strings.TrimSpace(req.ClientID)
	if clientID == "" {
		return nil, false, errInvalidRequest("missing client_id")
	}
	client, err := s.clients.GetByClientID(ctx, clientID)
	if err != nil {
		if errors.Is(err, ErrOidcClientNotFound) {
			return nil, false, errInvalidClient("unknown client")
		}
		return nil, false, errServerError("client lookup failed")
	}
	if !client.Enabled {
		return nil, false, errInvalidClient("client disabled")
	}

	// redirect_uri 必须严格匹配已注册集合
	redirectURI := strings.TrimSpace(req.RedirectURI)
	if redirectURI == "" || !containsString(client.RedirectURIs, redirectURI) {
		return nil, false, errInvalidRequest("redirect_uri mismatch")
	}
	// 此后 redirect_uri 已可信 → 错误可回跳

	if strings.TrimSpace(req.ResponseType) != "code" {
		return &OidcValidatedAuthorize{Client: client}, true, errUnsupportedResType("only response_type=code is supported")
	}

	scopes := splitScopes(req.Scope)
	if len(scopes) == 0 || !containsString(scopes, OidcScopeOpenID) {
		return &OidcValidatedAuthorize{Client: client}, true, errInvalidScope("scope must include openid")
	}
	// 必须是 client.allowed_scopes 子集
	for _, sc := range scopes {
		if !containsString(client.AllowedScopes, sc) {
			return &OidcValidatedAuthorize{Client: client}, true, errInvalidScope("scope " + sc + " not allowed for client")
		}
	}

	// PKCE：S256 强制
	if strings.TrimSpace(req.CodeChallenge) == "" {
		return &OidcValidatedAuthorize{Client: client}, true, errInvalidRequest("code_challenge required (PKCE)")
	}
	method := strings.TrimSpace(req.CodeChallengeMethod)
	if method == "" {
		method = "S256"
	}
	if method != "S256" {
		return &OidcValidatedAuthorize{Client: client}, true, errInvalidRequest("only code_challenge_method=S256 is supported")
	}

	return &OidcValidatedAuthorize{Client: client, Scopes: scopes}, true, nil
}

// ConsentNeeded 判断是否需要展示同意页。
//
//   - client.ConsentRequired=false → 永不展示 (false)
//   - 否则：已存在的 consent 覆盖本次请求 scope → 不需要 (并刷新 last_used)；否则需要。
func (s *OidcProviderService) ConsentNeeded(ctx context.Context, client *OidcClientView, userID int64, scopes []string) (bool, error) {
	if client == nil {
		return false, fmt.Errorf("oidc provider: nil client")
	}
	if !client.ConsentRequired {
		return false, nil
	}
	granted, found, err := s.consents.LoadGrantedScopes(ctx, userID, client.ClientID)
	if err != nil {
		return false, err
	}
	if found && s.consents.IsCovered(granted, scopes) {
		_ = s.consents.TouchLastUsed(ctx, userID, client.ClientID)
		return false, nil
	}
	return true, nil
}

// RecordConsent 用户在同意页点击 Allow 后调用：并集记忆已授权 scope。
func (s *OidcProviderService) RecordConsent(ctx context.Context, userID int64, clientID string, scopes []string) error {
	return s.consents.Grant(ctx, userID, clientID, scopes)
}

// ─── 同意会话令牌 (stateless) ────────────────────────────────────────────────
//
// 由于 schema 不含专门的 consent-session 表，本期以一个由 OidcSigningService 签名的
// 短期 JWT 承载 authorize 请求快照，作为 /oidc/consent 页面的 ?consent= 参数。
// 该 token 绑定 user_id，consent POST 时重新解析 SSO 并比对 user_id (防 CSRF)。

const (
	// oidcConsentTokenPurpose 同意令牌 purpose 声明，防止与 id_token 混用。
	oidcConsentTokenPurpose = "oidc_consent"
	// oidcConsentTokenTTLSeconds 同意令牌有效期 (10 分钟)。
	oidcConsentTokenTTLSeconds = 600
)

// OidcConsentTokenInput 签发同意令牌所需的 authorize 请求快照。
type OidcConsentTokenInput struct {
	UserID      int64
	ClientID    string
	Scopes      []string
	RedirectURI string
	State       string
	Nonce       string
	Challenge   string
	Method      string
}

// OidcConsentTokenClaims 解码后的同意令牌内容。
type OidcConsentTokenClaims struct {
	UserID      int64
	ClientID    string
	Scopes      []string
	RedirectURI string
	State       string
	Nonce       string
	Challenge   string
	Method      string
}

// IssueConsentToken 签发承载 authorize 快照的短期同意令牌。
func (s *OidcProviderService) IssueConsentToken(ctx context.Context, in OidcConsentTokenInput) (string, error) {
	issuer, err := s.IssuerURL(ctx)
	if err != nil {
		return "", err
	}
	now := s.now()
	claims := jwt.MapClaims{
		"iss":     issuer,
		"purpose": oidcConsentTokenPurpose,
		"sub":     strconv.FormatInt(in.UserID, 10),
		"cid":     in.ClientID,
		"scp":     strings.Join(normalizeStringSlice(in.Scopes), " "),
		"ruri":    in.RedirectURI,
		"state":   in.State,
		"nonce":   in.Nonce,
		"cc":      in.Challenge,
		"ccm":     in.Method,
		"iat":     now.Unix(),
		"exp":     now.Add(oidcConsentTokenTTLSeconds * time.Second).Unix(),
	}
	return s.signing.SignIDToken(claims)
}

// DecodeConsentToken 校验并解码同意令牌。签名/purpose/过期 任一不符 → 返回 *OidcError。
func (s *OidcProviderService) DecodeConsentToken(tokenStr string) (*OidcConsentTokenClaims, *OidcError) {
	tokenStr = strings.TrimSpace(tokenStr)
	if tokenStr == "" {
		return nil, errInvalidRequest("missing consent token")
	}
	parsed, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		kid, _ := t.Header["kid"].(string)
		pub := s.signing.VerificationKey(kid)
		if pub == nil {
			return nil, fmt.Errorf("unknown kid")
		}
		return pub, nil
	}, jwt.WithValidMethods([]string{"RS256"}))
	if err != nil || !parsed.Valid {
		return nil, errInvalidRequest("invalid or expired consent token")
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errInvalidRequest("invalid consent token claims")
	}
	if p, _ := claims["purpose"].(string); p != oidcConsentTokenPurpose {
		return nil, errInvalidRequest("consent token purpose mismatch")
	}
	subStr, _ := claims["sub"].(string)
	uid, perr := strconv.ParseInt(subStr, 10, 64)
	if perr != nil {
		return nil, errInvalidRequest("invalid consent token subject")
	}
	str := func(k string) string {
		v, _ := claims[k].(string)
		return v
	}
	return &OidcConsentTokenClaims{
		UserID:      uid,
		ClientID:    str("cid"),
		Scopes:      splitScopes(str("scp")),
		RedirectURI: str("ruri"),
		State:       str("state"),
		Nonce:       str("nonce"),
		Challenge:   str("cc"),
		Method:      str("ccm"),
	}, nil
}

// ─── 授权码签发 ──────────────────────────────────────────────────────────────

// IssueCode 生成并持久化一次性授权码，返回 code 明文。
func (s *OidcProviderService) IssueCode(ctx context.Context, in OidcIssueCodeInput) (string, error) {
	if in.Client == nil {
		return "", fmt.Errorf("oidc provider: nil client")
	}
	code, err := s.randToken()
	if err != nil {
		return "", err
	}
	method := in.Method
	if method == "" {
		method = "S256"
	}
	ttl := s.ttlSeconds(ctx, SettingKeyOidcProviderCodeTTLSeconds, DefaultOidcProviderCodeTTLSeconds)
	expiresAt := s.now().Add(time.Duration(ttl) * time.Second)

	if _, err := s.client.OidcAuthorizationCode.Create().
		SetCode(code).
		SetClientID(in.Client.ClientID).
		SetUserID(in.UserID).
		SetRedirectURI(in.RedirectURI).
		SetScopes(normalizeStringSlice(in.Scopes)).
		SetCodeChallenge(in.Challenge).
		SetCodeChallengeMethod(method).
		SetNonce(in.Nonce).
		SetExpiresAt(expiresAt).
		Save(ctx); err != nil {
		return "", fmt.Errorf("oidc provider: persist code: %w", err)
	}
	return code, nil
}

// ─── ExchangeCode ────────────────────────────────────────────────────────────

// ExchangeCode 兑换授权码。失败返回 *OidcError。
func (s *OidcProviderService) ExchangeCode(ctx context.Context, in OidcExchangeCodeInput) (*OidcTokenResponse, *OidcError) {
	if in.Client == nil {
		return nil, errInvalidClient("missing client")
	}
	codeVal := strings.TrimSpace(in.Code)
	if codeVal == "" {
		return nil, errInvalidGrant("missing code")
	}
	row, err := s.client.OidcAuthorizationCode.Query().
		Where(oidcauthorizationcode.CodeEQ(codeVal)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errInvalidGrant("unknown code")
		}
		return nil, errServerError("code lookup failed")
	}

	// 绑定的 client 必须一致
	if row.ClientID != in.Client.ClientID {
		return nil, errInvalidGrant("code was not issued to this client")
	}

	// 复用检测：已消费的 code 被再次提交 → 视为泄露，吊销该 code 的派生 token family
	if row.ConsumedAt != nil {
		s.revokeTokensByUserClient(ctx, row.UserID, row.ClientID)
		return nil, errInvalidGrant("code already used")
	}
	if !row.ExpiresAt.IsZero() && s.now().After(row.ExpiresAt) {
		return nil, errInvalidGrant("code expired")
	}
	if strings.TrimSpace(in.RedirectURI) != row.RedirectURI {
		return nil, errInvalidGrant("redirect_uri mismatch")
	}
	// PKCE 校验
	if perr := verifyPKCES256(in.CodeVerifier, row.CodeChallenge); perr != nil {
		return nil, perr
	}

	// 原子消费：仅当 consumed_at 仍为 nil 时置位
	consumedAt := s.now()
	n, err := s.client.OidcAuthorizationCode.Update().
		Where(
			oidcauthorizationcode.IDEQ(row.ID),
			oidcauthorizationcode.ConsumedAtIsNil(),
		).
		SetConsumedAt(consumedAt).
		Save(ctx)
	if err != nil {
		return nil, errServerError("consume code failed")
	}
	if n == 0 {
		// 竞态：被并发请求抢先消费
		s.revokeTokensByUserClient(ctx, row.UserID, row.ClientID)
		return nil, errInvalidGrant("code already used")
	}

	scopes := row.Scopes
	familyID := ""
	var refreshPlain string
	if containsString(scopes, OidcScopeOfflineAccess) {
		familyID, refreshPlain, err = s.issueRefreshToken(ctx, in.Client.ClientID, row.UserID, scopes, "")
		if err != nil {
			return nil, errServerError("issue refresh token failed")
		}
	}

	accessPlain, err := s.issueAccessToken(ctx, in.Client.ClientID, row.UserID, scopes, familyID)
	if err != nil {
		return nil, errServerError("issue access token failed")
	}

	resp := &OidcTokenResponse{
		AccessToken:  accessPlain,
		TokenType:    "Bearer",
		ExpiresIn:    s.ttlSeconds(ctx, SettingKeyOidcProviderAccessTokenTTLSeconds, DefaultOidcProviderAccessTokenTTLSeconds),
		RefreshToken: refreshPlain,
		Scope:        strings.Join(scopes, " "),
	}

	if containsString(scopes, OidcScopeOpenID) {
		idToken, ierr := s.signIDToken(ctx, in.Client.ClientID, row.UserID, scopes, row.Nonce, row.CreatedAt)
		if ierr != nil {
			return nil, errServerError("sign id token failed")
		}
		resp.IDToken = idToken
	}
	return resp, nil
}

// ─── RefreshToken ────────────────────────────────────────────────────────────

// RefreshToken 轮转 refresh token。失败返回 *OidcError。
func (s *OidcProviderService) RefreshToken(ctx context.Context, in OidcRefreshInput) (*OidcTokenResponse, *OidcError) {
	if in.Client == nil {
		return nil, errInvalidClient("missing client")
	}
	tokenVal := strings.TrimSpace(in.RefreshToken)
	if tokenVal == "" {
		return nil, errInvalidGrant("missing refresh_token")
	}
	row, err := s.client.OidcRefreshToken.Query().
		Where(oidcrefreshtoken.TokenEQ(tokenVal)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errInvalidGrant("unknown refresh_token")
		}
		return nil, errServerError("refresh lookup failed")
	}
	if row.ClientID != in.Client.ClientID {
		return nil, errInvalidGrant("refresh_token was not issued to this client")
	}

	// 复用检测：已吊销的 refresh 被再次使用 → 吊销整个 family
	if row.RevokedAt != nil {
		_ = s.RevokeFamily(ctx, row.FamilyID)
		fmt.Printf("[oidc] oidc.refresh_token.reuse_detected family=%s client=%s user=%d\n", row.FamilyID, row.ClientID, row.UserID)
		return nil, errInvalidGrant("refresh_token reuse detected")
	}
	if !row.ExpiresAt.IsZero() && s.now().After(row.ExpiresAt) {
		return nil, errInvalidGrant("refresh_token expired")
	}

	// scope 处理：默认沿用；若请求降权必须是原 scope 子集
	newScopes := row.Scopes
	if reqScope := strings.TrimSpace(in.Scope); reqScope != "" {
		requested := splitScopes(reqScope)
		if !s.consents.IsCovered(row.Scopes, requested) {
			return nil, errInvalidScope("requested scope exceeds original grant")
		}
		newScopes = requested
	}

	// 轮转：吊销旧 token + 在同 family 内插入新 refresh
	parentHash := sha256Hex(tokenVal)
	if _, err := s.client.OidcRefreshToken.UpdateOneID(row.ID).
		SetRevokedAt(s.now()).
		Save(ctx); err != nil {
		return nil, errServerError("revoke old refresh failed")
	}
	_, newRefresh, err := s.issueRefreshToken(ctx, in.Client.ClientID, row.UserID, newScopes, parentHash, row.FamilyID)
	if err != nil {
		return nil, errServerError("rotate refresh failed")
	}
	accessPlain, err := s.issueAccessToken(ctx, in.Client.ClientID, row.UserID, newScopes, row.FamilyID)
	if err != nil {
		return nil, errServerError("issue access token failed")
	}

	resp := &OidcTokenResponse{
		AccessToken:  accessPlain,
		TokenType:    "Bearer",
		ExpiresIn:    s.ttlSeconds(ctx, SettingKeyOidcProviderAccessTokenTTLSeconds, DefaultOidcProviderAccessTokenTTLSeconds),
		RefreshToken: newRefresh,
		Scope:        strings.Join(newScopes, " "),
	}
	if containsString(newScopes, OidcScopeOpenID) {
		idToken, ierr := s.signIDToken(ctx, in.Client.ClientID, row.UserID, newScopes, "", s.now())
		if ierr != nil {
			return nil, errServerError("sign id token failed")
		}
		resp.IDToken = idToken
	}
	return resp, nil
}

// RevokeFamily 吊销同一 family 的所有 refresh token 及关联 access token。
func (s *OidcProviderService) RevokeFamily(ctx context.Context, familyID string) error {
	familyID = strings.TrimSpace(familyID)
	if familyID == "" {
		return nil
	}
	now := s.now()
	if _, err := s.client.OidcRefreshToken.Update().
		Where(
			oidcrefreshtoken.FamilyIDEQ(familyID),
			oidcrefreshtoken.RevokedAtIsNil(),
		).
		SetRevokedAt(now).
		Save(ctx); err != nil {
		return fmt.Errorf("oidc provider: revoke refresh family: %w", err)
	}
	if _, err := s.client.OidcAccessToken.Update().
		Where(
			oidcaccesstoken.RefreshFamilyIDEQ(familyID),
			oidcaccesstoken.RevokedAtIsNil(),
		).
		SetRevokedAt(now).
		Save(ctx); err != nil {
		return fmt.Errorf("oidc provider: revoke access family: %w", err)
	}
	return nil
}

// ─── UserInfo ────────────────────────────────────────────────────────────────

// OidcResolvedAccessToken 是 ResolveAccessToken 的解析结果，供资源端点
// (如 /oidc/resource/api-keys) 在拿到不透明 access token 后做用户/scope 鉴权。
type OidcResolvedAccessToken struct {
	UserID   int64
	ClientID string
	Scopes   []string
}

// HasScope 检测 token 是否携带指定 scope。
func (r *OidcResolvedAccessToken) HasScope(scope string) bool {
	if r == nil {
		return false
	}
	return containsString(r.Scopes, scope)
}

// ResolveAccessToken 校验不透明 access token 并返回其绑定的 (user_id, client_id, scopes)。
//
// 失败语义与 BuildUserInfo 完全一致：未知 / 已吊销 / 过期 → invalid_token (401)，
// 其他内部错 → server_error (500)。资源端点应据此设置 WWW-Authenticate 响应头。
func (s *OidcProviderService) ResolveAccessToken(ctx context.Context, accessTokenValue string) (*OidcResolvedAccessToken, *OidcError) {
	tokenVal := strings.TrimSpace(accessTokenValue)
	if tokenVal == "" {
		return nil, newOidcError("invalid_token", "missing access token", 401)
	}
	row, err := s.client.OidcAccessToken.Query().
		Where(oidcaccesstoken.TokenEQ(tokenVal)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, newOidcError("invalid_token", "unknown access token", 401)
		}
		return nil, errServerError("access token lookup failed")
	}
	if row.RevokedAt != nil {
		return nil, newOidcError("invalid_token", "access token revoked", 401)
	}
	if !row.ExpiresAt.IsZero() && s.now().After(row.ExpiresAt) {
		return nil, newOidcError("invalid_token", "access token expired", 401)
	}
	scopes := append([]string(nil), row.Scopes...)
	return &OidcResolvedAccessToken{
		UserID:   row.UserID,
		ClientID: row.ClientID,
		Scopes:   scopes,
	}, nil
}

// BuildUserInfo 依据 access token 存储的 scope 投影用户声明。
//
// access token 未知/已吊销/过期 → 返回 *OidcError (handler → 401 + WWW-Authenticate)。
func (s *OidcProviderService) BuildUserInfo(ctx context.Context, accessTokenValue string) (map[string]any, *OidcError) {
	resolved, oerr := s.ResolveAccessToken(ctx, accessTokenValue)
	if oerr != nil {
		return nil, oerr
	}

	user, err := s.client.User.Get(ctx, resolved.UserID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, newOidcError("invalid_token", "user not found", 401)
		}
		return nil, errServerError("user lookup failed")
	}

	claims := map[string]any{
		"sub": strconv.FormatInt(resolved.UserID, 10),
	}
	scopes := resolved.Scopes
	if containsString(scopes, OidcScopeProfile) {
		claims["name"] = user.Username
		claims["preferred_username"] = user.Username
	}
	if containsString(scopes, OidcScopeEmail) {
		claims["email"] = user.Email
		claims["email_verified"] = true
	}

	if containsString(scopes, OidcScopeAPIKey) {
		// 获取用户的 API Key 列表（包含明文 key 值，供 RP 直接调用 sub2api 网关）
		apiKeys, err := user.QueryAPIKeys().All(ctx)
		if err != nil {
			return nil, errServerError("apikey list failed")
		}

		apiKeyInfos := make([]map[string]any, len(apiKeys))
		for i, apiKey := range apiKeys {
			apiKeyInfos[i] = map[string]any{
				"id":           apiKey.ID,
				"key":          apiKey.Key,
				"name":         apiKey.Name,
				"status":       apiKey.Status,
				"created_at":   apiKey.CreatedAt,
				"last_used_at": apiKey.LastUsedAt,
				"expires_at":   apiKey.ExpiresAt,
			}
		}
		claims["sub2api_apikeys"] = apiKeyInfos
		claims["sub2api_apikey_count"] = len(apiKeys)
	}
	return claims, nil
}

// ─── 内部：token 生成与签名 ──────────────────────────────────────────────────

func (s *OidcProviderService) issueAccessToken(ctx context.Context, clientID string, userID int64, scopes []string, familyID string) (string, error) {
	token, err := s.randToken()
	if err != nil {
		return "", err
	}
	ttl := s.ttlSeconds(ctx, SettingKeyOidcProviderAccessTokenTTLSeconds, DefaultOidcProviderAccessTokenTTLSeconds)
	if _, err := s.client.OidcAccessToken.Create().
		SetToken(token).
		SetClientID(clientID).
		SetUserID(userID).
		SetScopes(normalizeStringSlice(scopes)).
		SetRefreshFamilyID(familyID).
		SetExpiresAt(s.now().Add(time.Duration(ttl) * time.Second)).
		Save(ctx); err != nil {
		return "", err
	}
	return token, nil
}

// issueRefreshToken 创建 refresh token；familyID 为空时新建 family (复用 token 值作为 family id 种子)。
// parentHash 可空 (首次签发)；extraFamily 可选传入既有 family id (轮转时沿用)。
func (s *OidcProviderService) issueRefreshToken(ctx context.Context, clientID string, userID int64, scopes []string, parentHash string, extraFamily ...string) (string, string, error) {
	token, err := s.randToken()
	if err != nil {
		return "", "", err
	}
	familyID := ""
	if len(extraFamily) > 0 && strings.TrimSpace(extraFamily[0]) != "" {
		familyID = extraFamily[0]
	} else {
		familyID = sha256Hex(token)[:32]
	}
	ttl := s.ttlSeconds(ctx, SettingKeyOidcProviderRefreshTokenTTLSeconds, DefaultOidcProviderRefreshTokenTTLSeconds)
	if _, err := s.client.OidcRefreshToken.Create().
		SetToken(token).
		SetFamilyID(familyID).
		SetClientID(clientID).
		SetUserID(userID).
		SetScopes(normalizeStringSlice(scopes)).
		SetExpiresAt(s.now().Add(time.Duration(ttl) * time.Second)).
		SetParentTokenHash(parentHash).
		Save(ctx); err != nil {
		return "", "", err
	}
	return familyID, token, nil
}

func (s *OidcProviderService) signIDToken(ctx context.Context, clientID string, userID int64, scopes []string, nonce string, authTime time.Time) (string, error) {
	issuer, err := s.IssuerURL(ctx)
	if err != nil {
		return "", err
	}
	now := s.now()
	ttl := s.ttlSeconds(ctx, SettingKeyOidcProviderIDTokenTTLSeconds, DefaultOidcProviderIDTokenTTLSeconds)
	claims := jwt.MapClaims{
		"iss": issuer,
		"sub": strconv.FormatInt(userID, 10),
		"aud": clientID,
		"exp": now.Add(time.Duration(ttl) * time.Second).Unix(),
		"iat": now.Unix(),
		"acr": OidcAcrBasic,
	}
	if !authTime.IsZero() {
		claims["auth_time"] = authTime.Unix()
	}
	if strings.TrimSpace(nonce) != "" {
		claims["nonce"] = nonce
	}
	// 标准身份声明 (私有 scope 绝不进 id_token)
	if containsString(scopes, OidcScopeProfile) || containsString(scopes, OidcScopeEmail) {
		if user, err := s.client.User.Get(ctx, userID); err == nil {
			if containsString(scopes, OidcScopeProfile) {
				claims["name"] = user.Username
				claims["preferred_username"] = user.Username
			}
			if containsString(scopes, OidcScopeEmail) {
				claims["email"] = user.Email
				claims["email_verified"] = true
			}
		}
	}
	return s.signing.SignIDToken(claims)
}

// revokeTokensByUserClient 在 code 复用检测时尽力吊销该 (user, client) 的活跃 token。
func (s *OidcProviderService) revokeTokensByUserClient(ctx context.Context, userID int64, clientID string) {
	now := s.now()
	_, _ = s.client.OidcRefreshToken.Update().
		Where(
			oidcrefreshtoken.UserIDEQ(userID),
			oidcrefreshtoken.ClientIDEQ(clientID),
			oidcrefreshtoken.RevokedAtIsNil(),
		).
		SetRevokedAt(now).
		Save(ctx)
	_, _ = s.client.OidcAccessToken.Update().
		Where(
			oidcaccesstoken.UserIDEQ(userID),
			oidcaccesstoken.ClientIDEQ(clientID),
			oidcaccesstoken.RevokedAtIsNil(),
		).
		SetRevokedAt(now).
		Save(ctx)
}

func (s *OidcProviderService) randToken() (string, error) {
	buf := make([]byte, oidcTokenRandBytes)
	if _, err := s.randReadFunc(buf); err != nil {
		return "", fmt.Errorf("oidc provider: rand token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// ─── 工具函数 ────────────────────────────────────────────────────────────────

// verifyPKCES256 校验 base64url(sha256(verifier)) == challenge。
func verifyPKCES256(verifier, challenge string) *OidcError {
	v := strings.TrimSpace(verifier)
	if v == "" {
		return errInvalidGrant("missing code_verifier")
	}
	sum := sha256.Sum256([]byte(v))
	computed := base64.RawURLEncoding.EncodeToString(sum[:])
	if computed != strings.TrimSpace(challenge) {
		return errInvalidGrant("PKCE verification failed")
	}
	return nil
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// splitScopes 把空格分隔的 scope 串切成去重切片。
func splitScopes(raw string) []string {
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(fields))
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if _, ok := seen[f]; ok {
			continue
		}
		seen[f] = struct{}{}
		out = append(out, f)
	}
	return out
}

func containsString(haystack []string, needle string) bool {
	return slices.Contains(haystack, needle)
}

// formatDecimal 把余额类 float 格式化为定点字符串 (避免科学计数法)。
func formatDecimal(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// ─── Admin 设置读写 (task 8.1/8.2) ───────────────────────────────────────────

// OidcProviderSettingsView 是 admin GET 设置返回的快照 (已套用默认值)。
type OidcProviderSettingsView struct {
	Enabled                bool   `json:"enabled"`
	IssuerURL              string `json:"issuer_url"`
	AccessTokenTTLSeconds  int    `json:"access_token_ttl_seconds"`
	IDTokenTTLSeconds      int    `json:"id_token_ttl_seconds"`
	RefreshTokenTTLSeconds int    `json:"refresh_token_ttl_seconds"`
	CodeTTLSeconds         int    `json:"code_ttl_seconds"`
	SSOCookieMaxAgeSeconds int    `json:"sso_cookie_max_age_seconds"`
	SSOCookieDomain        string `json:"sso_cookie_domain"`
}

// OidcProviderSettingsInput 是 admin PUT 设置入参；nil 指针字段表示不修改。
type OidcProviderSettingsInput struct {
	Enabled                *bool   `json:"enabled"`
	IssuerURL              *string `json:"issuer_url"`
	AccessTokenTTLSeconds  *int    `json:"access_token_ttl_seconds"`
	IDTokenTTLSeconds      *int    `json:"id_token_ttl_seconds"`
	RefreshTokenTTLSeconds *int    `json:"refresh_token_ttl_seconds"`
	CodeTTLSeconds         *int    `json:"code_ttl_seconds"`
	SSOCookieMaxAgeSeconds *int    `json:"sso_cookie_max_age_seconds"`
	SSOCookieDomain        *string `json:"sso_cookie_domain"`
}

// GetProviderSettings 读取全部 8 个 oidc_provider.* 设置 (缺省套用默认值)。
func (s *OidcProviderService) GetProviderSettings(ctx context.Context) (*OidcProviderSettingsView, error) {
	issuer, err := s.settingRepo.GetValue(ctx, SettingKeyOidcProviderIssuerURL)
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return nil, fmt.Errorf("oidc provider: read issuer_url: %w", err)
	}
	domain, err := s.settingRepo.GetValue(ctx, SettingKeyOidcProviderSSOCookieDomain)
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return nil, fmt.Errorf("oidc provider: read sso_cookie_domain: %w", err)
	}
	return &OidcProviderSettingsView{
		Enabled:                s.IsEnabled(ctx),
		IssuerURL:              strings.TrimSpace(issuer),
		AccessTokenTTLSeconds:  s.ttlSeconds(ctx, SettingKeyOidcProviderAccessTokenTTLSeconds, DefaultOidcProviderAccessTokenTTLSeconds),
		IDTokenTTLSeconds:      s.ttlSeconds(ctx, SettingKeyOidcProviderIDTokenTTLSeconds, DefaultOidcProviderIDTokenTTLSeconds),
		RefreshTokenTTLSeconds: s.ttlSeconds(ctx, SettingKeyOidcProviderRefreshTokenTTLSeconds, DefaultOidcProviderRefreshTokenTTLSeconds),
		CodeTTLSeconds:         s.ttlSeconds(ctx, SettingKeyOidcProviderCodeTTLSeconds, DefaultOidcProviderCodeTTLSeconds),
		SSOCookieMaxAgeSeconds: s.ttlSeconds(ctx, SettingKeyOidcProviderSSOCookieMaxAgeSeconds, DefaultOidcProviderSSOCookieMaxAgeSeconds),
		SSOCookieDomain:        strings.TrimSpace(domain),
	}, nil
}

// UpdateProviderSettings 局部更新设置。
//
// 校验规则：
//   - 若本次把 enabled 置为 true，则 issuer_url (本次入参或既有值) 必须通过
//     ValidateOidcIssuerURL，否则返回 ErrOidcProviderIssuerURLEmpty 等哨兵错误。
//   - issuer_url 非空时一律按格式严格校验 (允许显式清空为 "" 仅当 enabled=false)。
//   - 各 TTL 必须为正整数。
func (s *OidcProviderService) UpdateProviderSettings(ctx context.Context, in OidcProviderSettingsInput) error {
	updates := make(map[string]string)

	// 解析最终的 enabled / issuer 以做交叉校验。
	finalEnabled := s.IsEnabled(ctx)
	if in.Enabled != nil {
		finalEnabled = *in.Enabled
	}
	finalIssuer := ""
	if cur, err := s.settingRepo.GetValue(ctx, SettingKeyOidcProviderIssuerURL); err == nil {
		finalIssuer = strings.TrimSpace(cur)
	}
	if in.IssuerURL != nil {
		finalIssuer = strings.TrimSpace(*in.IssuerURL)
	}

	if in.IssuerURL != nil {
		// 非空时严格校验；允许清空 (但若同时 enabled=true 下面会拦截)。
		if finalIssuer != "" {
			if err := ValidateOidcIssuerURL(finalIssuer); err != nil {
				return err
			}
		}
		updates[SettingKeyOidcProviderIssuerURL] = finalIssuer
	}

	if finalEnabled {
		if err := ValidateOidcIssuerURL(finalIssuer); err != nil {
			return err
		}
	}

	if in.Enabled != nil {
		updates[SettingKeyOidcProviderEnabled] = strconv.FormatBool(*in.Enabled)
	}
	if err := putPositiveTTL(updates, SettingKeyOidcProviderAccessTokenTTLSeconds, in.AccessTokenTTLSeconds); err != nil {
		return err
	}
	if err := putPositiveTTL(updates, SettingKeyOidcProviderIDTokenTTLSeconds, in.IDTokenTTLSeconds); err != nil {
		return err
	}
	if err := putPositiveTTL(updates, SettingKeyOidcProviderRefreshTokenTTLSeconds, in.RefreshTokenTTLSeconds); err != nil {
		return err
	}
	if err := putPositiveTTL(updates, SettingKeyOidcProviderCodeTTLSeconds, in.CodeTTLSeconds); err != nil {
		return err
	}
	if err := putPositiveTTL(updates, SettingKeyOidcProviderSSOCookieMaxAgeSeconds, in.SSOCookieMaxAgeSeconds); err != nil {
		return err
	}
	if in.SSOCookieDomain != nil {
		updates[SettingKeyOidcProviderSSOCookieDomain] = strings.TrimSpace(*in.SSOCookieDomain)
	}

	if len(updates) == 0 {
		return nil
	}

	// 启用 OIDC Provider 时确保至少存在一把可用签名密钥。
	if finalEnabled && s.signing != nil {
		if s.signing.ActiveKid() == "" {
			if err := s.signing.EnsureActiveKey(ctx); err != nil {
				return fmt.Errorf("oidc provider: ensure signing key: %w", err)
			}
		}
	}

	if err := s.settingRepo.SetMultiple(ctx, updates); err != nil {
		return fmt.Errorf("oidc provider: persist settings: %w", err)
	}
	return nil
}

// putPositiveTTL 校验 TTL 为正整数后写入 updates；nil 表示不修改。
func putPositiveTTL(updates map[string]string, key string, v *int) error {
	if v == nil {
		return nil
	}
	if *v <= 0 {
		return fmt.Errorf("%w: %s", ErrOidcProviderInvalidTTL, key)
	}
	updates[key] = strconv.Itoa(*v)
	return nil
}
