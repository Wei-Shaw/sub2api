// Package service ...
//
// oidc_provider_service.go 实现 sub2api 作为 OIDC Provider 的核心业务逻辑。
// 提供 OIDC 协议的核心功能：授权、令牌交换、令牌刷新、用户信息等。
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/oidc_access_token"
	"github.com/Wei-Shaw/sub2api/ent/oidc_authorization_code"
	"github.com/Wei-Shaw/sub2api/ent/oidc_client"
	"github.com/Wei-Shaw/sub2api/ent/oidc_refresh_token"
	"github.com/Wei-Shaw/sub2api/ent/oidc_signature_key"
	"github.com/Wei-Shaw/sub2api/ent/oidc_consent"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

var (
	ErrOIDCClientNotFound      = infraerrors.Unauthorized("OIDC_CLIENT_NOT_FOUND", "OIDC client not found")
	ErrOIDCClientDisabled      = infraerrors.Unauthorized("OIDC_CLIENT_DISABLED", "OIDC client is disabled")
	ErrOIDCInvalidRedirectURI  = infraerrors.Unauthorized("OIDC_INVALID_REDIRECT_URI", "Invalid redirect URI")
	ErrOIDCInvalidScope        = infraerrors.Unauthorized("OIDC_INVALID_SCOPE", "Invalid scope")
	ErrOIDCMissingPKCE         = infraerrors.Unauthorized("OIDC_MISSING_PKCE", "PKCE required but not provided")
	ErrOIDCCodeNotFound        = infraerrors.Unauthorized("OIDC_CODE_NOT_FOUND", "Authorization code not found or expired")
	ErrOIDCTokenNotFound       = infraerrors.Unauthorized("OIDC_TOKEN_NOT_FOUND", "Access token not found or expired")
	ErrOIDCRefreshTokenRevoked = infraerrors.Unauthorized("OIDC_REFRESH_TOKEN_REVOKED", "Refresh token has been revoked")
)

// OIDCProviderService 处理 OIDC 协议的核心业务逻辑
type OIDCProviderService struct {
	entClient           *dbent.Client
	cfg                 *config.Config
	ssoSessionService   *SSOSessionService
	consentService      *OidcConsentService
	signingKeyService   *OIDCSigningKeyService
}

// NewOIDCProviderService 创建 OIDC Provider Service 实例
func NewOIDCProviderService(
	entClient *dbent.Client,
	cfg *config.Config,
	ssoSessionService *SSOSessionService,
	consentService *OidcConsentService,
	signingKeyService *OIDCSigningKeyService,
) *OIDCProviderService {
	return &OIDCProviderService{
		entClient:         entClient,
		cfg:               cfg,
		ssoSessionService: ssoSessionService,
		consentService:    consentService,
		signingKeyService: signingKeyService,
	}
}

// AuthorizeParams 授权请求参数
type AuthorizeParams struct {
	ClientID     string
	RedirectURI  string
	ResponseType string
	Scope        string
	State        string
	CodeChallenge string
	CodeChallengeMethod string
	Prompt       string
}

// AuthorizeOutcome 授权结果
type AuthorizeOutcome struct {
	RequiresConsent bool
	RedirectURI     string
	Code            string
	State           string
}

// TokenResponse 令牌响应
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// HandleAuthorize 处理授权请求
func (s *OIDCProviderService) HandleAuthorize(r *http.Request, w http.ResponseWriter, params AuthorizeParams) (*AuthorizeOutcome, error) {
	ctx := r.Context()
	// 验证客户端
	client, err := s.validateClient(ctx, params.ClientID)
	if err != nil {
		return nil, err
	}

	// 验证重定向 URI
	if err := s.validateRedirectURI(client, params.RedirectURI); err != nil {
		return nil, err
	}

	// 验证响应类型
	if err := s.validateResponseType(params.ResponseType); err != nil {
		return nil, err
	}

	// 验证 Scope
	requestedScopes, err := s.validateScope(params.Scope, client.AllowedScopes)
	if err != nil {
		return nil, err
	}

	// 验证 PKCE
	if err := s.validatePKCE(params.CodeChallenge, params.CodeChallengeMethod); err != nil {
		return nil, err
	}

	// 检查用户是否已登录
	userID, err := s.ssoSessionService.Validate(ctx, r, w)
	if err != nil {
		// 用户未登录，需要重定向到登录页面
		return nil, infraerrors.Unauthorized("OIDC_USER_NOT_LOGGED_IN", "User not logged in")
	}

	// 检查是否需要重新认证
	if params.Prompt == "login" {
		// 强制重新登录
		return nil, infraerrors.Unauthorized("OIDC_PROMPT_LOGIN", "Prompt=login requires re-authentication")
	}

	// 检查是否已有足够的授权
	grantedScopes, found, err := s.consentService.LoadGrantedScopes(ctx, userID, params.ClientID)
	if err != nil {
		return nil, fmt.Errorf("failed to load granted scopes: %w", err)
	}

	// 检查是否需要同意
	requiresConsent := !found || !s.isScopeCovered(grantedScopes, requestedScopes)

	if requiresConsent {
		// 需要用户同意
		return &AuthorizeOutcome{
			RequiresConsent: true,
			RedirectURI:     params.RedirectURI,
			State:           params.State,
		}, nil
	}

	// 直接授权，生成授权码
	code, err := s.generateAuthorizationCode(ctx, userID, params)
	if err != nil {
		return nil, fmt.Errorf("failed to generate authorization code: %w", err)
	}

	return &AuthorizeOutcome{
		RequiresConsent: false,
		RedirectURI:     params.RedirectURI,
		Code:            code,
		State:           params.State,
	}, nil
}

// ExchangeCode 交换授权码
func (s *OIDCProviderService) ExchangeCode(ctx context.Context, code, redirectURI, codeVerifier string) (*TokenResponse, error) {
	// 查询授权码记录
	authCode, err := s.entClient.OidcAuthorizationCode.Query().
		Where(oidc_authorization_code.CodeEQ(code)).
		First(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, ErrOIDCCodeNotFound
		}
		return nil, fmt.Errorf("failed to query authorization code: %w", err)
	}

	// 检查授权码是否已使用或过期
	if authCode.Used || authCode.ExpiresAt.Before(time.Now()) {
		return nil, ErrOIDCCodeNotFound
	}

	// 验证重定向 URI 匹配
	if authCode.RedirectURI != redirectURI {
		return nil, ErrOIDCInvalidRedirectURI
	}

	// 验证 PKCE code_verifier
	if authCode.CodeChallenge != "" {
		if err := s.verifyPKCE(authCode.CodeChallenge, authCode.CodeChallengeMethod, codeVerifier); err != nil {
			return nil, err
		}
	}

	// 标记授权码为已使用
	_, err = s.entClient.OidcAuthorizationCode.UpdateOne(authCode).
		SetUsed(true).
		SetUsedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to mark authorization code as used: %w", err)
	}

	// 生成访问令牌
	accessToken, err := s.generateAccessToken(ctx, authCode.UserID, authCode.ClientID, authCode.Scope)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	// 生成刷新令牌
	refreshToken, err := s.generateRefreshToken(ctx, authCode.UserID, authCode.ClientID, authCode.Scope)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	// 生成 ID Token
	idToken, err := s.generateIDToken(ctx, authCode.UserID, authCode.ClientID, authCode.Scope)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ID token: %w", err)
	}

	return &TokenResponse{
		AccessToken:  accessToken,
		TokenType:    "Bearer",
		ExpiresIn:    3600, // 1小时
		RefreshToken: refreshToken,
		IDToken:      idToken,
		Scope:        authCode.Scope,
	}, nil
}

// RefreshToken 刷新访问令牌
func (s *OIDCProviderService) RefreshToken(ctx context.Context, refreshToken string, scope string) (*TokenResponse, error) {
	// 查询刷新令牌记录
	token, err := s.entClient.OidcRefreshToken.Query().
		Where(oidc_refresh_token.TokenEQ(refreshToken)).
		First(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, ErrOIDCTokenNotFound
		}
		return nil, fmt.Errorf("failed to query refresh token: %w", err)
	}

	// 检查令牌是否已撤销或过期
	if token.Revoked || token.ExpiresAt.Before(time.Now()) {
		return nil, ErrOIDCRefreshTokenRevoked
	}

	// 检测令牌重用 - 如果已使用过，撤销整个令牌家族
	if token.UsedAt != nil {
		// 撤销整个令牌家族
		if err := s.RevokeFamily(ctx, token.FamilyID); err != nil {
			logger.Error(ctx, "Failed to revoke token family", "family_id", token.FamilyID, "error", err)
		}
		return nil, ErrOIDCRefreshTokenRevoked
	}

	// 标记当前刷新令牌为已使用
	_, err = s.entClient.OidcRefreshToken.UpdateOne(token).
		SetUsedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to mark refresh token as used: %w", err)
	}

	// 验证请求的 scope 是否在原始授权范围内
	if scope != "" {
		originalScopes := strings.Split(token.Scope, " ")
		requestedScopes := strings.Split(scope, " ")
		
		if !s.isScopeCovered(originalScopes, requestedScopes) {
			return nil, ErrOIDCInvalidScope
		}
	} else {
		scope = token.Scope
	}

	// 生成新的访问令牌
	newAccessToken, err := s.generateAccessToken(ctx, token.UserID, token.ClientID, scope)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	// 生成新的刷新令牌（使用相同的家族ID）
	newRefreshToken, err := s.generateRefreshTokenWithFamily(ctx, token.UserID, token.ClientID, scope, token.FamilyID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	// 生成新的 ID Token
	idToken, err := s.generateIDToken(ctx, token.UserID, token.ClientID, scope)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ID token: %w", err)
	}

	return &TokenResponse{
		AccessToken:  newAccessToken,
		TokenType:    "Bearer",
		ExpiresIn:    3600, // 1小时
		RefreshToken: newRefreshToken,
		IDToken:      idToken,
		Scope:        scope,
	}, nil
}

// RevokeFamily 撤销令牌家族
func (s *OIDCProviderService) RevokeFamily(ctx context.Context, familyID string) error {
	// 查询指定家族的所有令牌
	tokens, err := s.entClient.OidcRefreshToken.Query().
		Where(oidc_refresh_token.FamilyIDEQ(familyID)).
		All(ctx)
	if err != nil {
		return fmt.Errorf("failed to query tokens for family: %w", err)
	}

	// 标记所有令牌为已撤销
	for _, token := range tokens {
		_, err := s.entClient.OidcRefreshToken.UpdateOne(token).
			SetRevoked(true).
			SetRevokedAt(time.Now().UTC()).
			Save(ctx)
		if err != nil {
			logger.Error(ctx, "Failed to revoke token", "token_id", token.ID, "error", err)
			// 继续处理其他令牌
		}
	}

	// 记录安全日志
	logger.Info(ctx, "Revoked token family", "family_id", familyID, "token_count", len(tokens))

	return nil
}

// BuildUserInfo 构建用户信息
func (s *OIDCProviderService) BuildUserInfo(ctx context.Context, accessToken string) (map[string]interface{}, error) {
	// 验证访问令牌
	token, err := s.entClient.OidcAccessToken.Query().
		Where(oidc_access_token.TokenEQ(accessToken)).
		First(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, ErrOIDCTokenNotFound
		}
		return nil, fmt.Errorf("failed to query access token: %w", err)
	}

	// 检查令牌是否已过期
	if token.ExpiresAt.Before(time.Now()) {
		return nil, ErrOIDCTokenNotFound
	}

	// 查询用户信息
	user, err := s.entClient.User.Query().
		Where(user.IDEQ(token.UserID)).
		First(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query user: %w", err)
	}

	// 根据 scope 构建 claims
	userInfo := make(map[string]interface{})
	scopes := strings.Split(token.Scope, " ")

	// 始终包含 sub (subject) 声明
	userInfo["sub"] = fmt.Sprintf("%d", user.ID)

	for _, scope := range scopes {
		switch scope {
		case "profile":
			userInfo["name"] = user.Name
			userInfo["given_name"] = user.Name
			userInfo["locale"] = "zh-CN" // TODO: 从用户配置获取
			userInfo["updated_at"] = user.UpdatedAt.Unix()
		case "email":
			userInfo["email"] = user.Email
			userInfo["email_verified"] = user.EmailVerified
		case "phone":
			// TODO: 添加电话号码信息
			userInfo["phone_number"] = ""
			userInfo["phone_number_verified"] = false
		case "address":
			// TODO: 添加地址信息
			userInfo["address"] = map[string]interface{}{
				"formatted": "",
			}
		}
	}

	return userInfo, nil
}

// ExchangeCode 交换授权码
func (s *OIDCProviderService) ExchangeCode(ctx context.Context, code, redirectURI, codeVerifier string) (*TokenResponse, error) {
	// TODO: 实现授权码验证、PKCE验证、令牌生成等
	return nil, fmt.Errorf("not implemented")
}

// RefreshToken 刷新访问令牌
func (s *OIDCProviderService) RefreshToken(ctx context.Context, refreshToken string, scope string) (*TokenResponse, error) {
	// TODO: 实现刷新令牌验证、重用检测、新令牌生成等
	return nil, fmt.Errorf("not implemented")
}

// RevokeFamily 撤销令牌家族
func (s *OIDCProviderService) RevokeFamily(ctx context.Context, familyID string) error {
	// TODO: 实现令牌家族撤销
	return fmt.Errorf("not implemented")
}

// BuildUserInfo 构建用户信息
func (s *OIDCProviderService) BuildUserInfo(ctx context.Context, accessToken string) (map[string]interface{}, error) {
	// TODO: 实现用户信息构建
	return nil, fmt.Errorf("not implemented")
}

// ==================== 辅助方法 ====================

// validateClient 验证客户端
func (s *OIDCProviderService) validateClient(ctx context.Context, clientID string) (*dbent.OidcClient, error) {
	client, err := s.entClient.OidcClient.Query().
		Where(oidc_client.ClientIDEQ(clientID)).
		First(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, ErrOIDCClientNotFound
		}
		return nil, fmt.Errorf("failed to query client: %w", err)
	}

	if !client.Enabled {
		return nil, ErrOIDCClientDisabled
	}

	return client, nil
}

// validateRedirectURI 验证重定向 URI
func (s *OIDCProviderService) validateRedirectURI(client *dbent.OidcClient, redirectURI string) error {
	// 检查重定向 URI 是否在客户端允许的列表中
	allowedURIs := client.RedirectUris
	for _, allowedURI := range allowedURIs {
		if allowedURI == redirectURI {
			return nil
		}
	}

	return ErrOIDCInvalidRedirectURI
}

// validateResponseType 验证响应类型
func (s *OIDCProviderService) validateResponseType(responseType string) error {
	// 目前只支持 code 响应类型
	if responseType != "code" {
		return infraerrors.Unauthorized("OIDC_UNSUPPORTED_RESPONSE_TYPE", "Only 'code' response type is supported")
	}
	return nil
}

// validateScope 验证 Scope
func (s *OIDCProviderService) validateScope(requestedScope string, allowedScopes []string) ([]string, error) {
	if requestedScope == "" {
		return nil, nil
	}

	requestedScopes := strings.Split(requestedScope, " ")
	
	// 检查每个请求的 scope 是否在允许的范围内
	for _, scope := range requestedScopes {
		if !s.containsScope(allowedScopes, scope) {
			return nil, ErrOIDCInvalidScope
		}
	}

	return requestedScopes, nil
}

// validatePKCE 验证 PKCE
func (s *OIDCProviderService) validatePKCE(codeChallenge, codeChallengeMethod string) error {
	if codeChallenge == "" {
		return ErrOIDCMissingPKCE
	}

	// 目前只支持 S256 方法
	if codeChallengeMethod != "S256" && codeChallengeMethod != "" {
		return infraerrors.Unauthorized("OIDC_UNSUPPORTED_PKCE_METHOD", "Only 'S256' PKCE method is supported")
	}

	return nil
}

// isScopeCovered 检查已授权 scope 是否覆盖请求的 scope
func (s *OIDCProviderService) isScopeCovered(grantedScopes, requestedScopes []string) bool {
	if len(requestedScopes) == 0 {
		return true
	}

	if len(grantedScopes) == 0 {
		return false
	}

	// 检查每个请求的 scope 是否在已授权的 scope 中
	for _, requested := range requestedScopes {
		if !s.containsScope(grantedScopes, requested) {
			return false
		}
	}

	return true
}

// containsScope 检查 scope 是否在列表中
func (s *OIDCProviderService) containsScope(scopes []string, target string) bool {
	for _, scope := range scopes {
		if scope == target {
			return true
		}
	}
	return false
}

// generateAuthorizationCode 生成授权码
func (s *OIDCProviderService) generateAuthorizationCode(ctx context.Context, userID int64, params AuthorizeParams) (string, error) {
	// 生成随机授权码
	codeBytes := make([]byte, 32)
	if _, err := rand.Read(codeBytes); err != nil {
		return "", fmt.Errorf("failed to generate authorization code: %w", err)
	}
	code := base64.RawURLEncoding.EncodeToString(codeBytes)

	// 保存授权码到数据库
	_, err := s.entClient.OidcAuthorizationCode.Create().
		SetCode(code).
		SetUserID(userID).
		SetClientID(params.ClientID).
		SetRedirectURI(params.RedirectURI).
		SetScope(params.Scope).
		SetCodeChallenge(params.CodeChallenge).
		SetCodeChallengeMethod(params.CodeChallengeMethod).
		SetExpiresAt(time.Now().Add(10 * time.Minute)). // 10分钟有效期
		SetUsed(false).
		SetCreatedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to save authorization code: %w", err)
	}

	return code, nil
}

// verifyPKCE 验证 PKCE code_verifier
func (s *OIDCProviderService) verifyPKCE(codeChallenge, codeChallengeMethod, codeVerifier string) error {
	if codeChallengeMethod == "S256" {
		// 计算 code_verifier 的 SHA256 哈希
		hash := sha256.Sum256([]byte(codeVerifier))
		expectedChallenge := base64.RawURLEncoding.EncodeToString(hash[:])
		
		if expectedChallenge != codeChallenge {
			return infraerrors.Unauthorized("OIDC_INVALID_PKCE", "Invalid PKCE code verifier")
		}
	} else {
		// plain 方法直接比较
		if codeVerifier != codeChallenge {
			return infraerrors.Unauthorized("OIDC_INVALID_PKCE", "Invalid PKCE code verifier")
		}
	}
	
	return nil
}

// generateAccessToken 生成访问令牌
func (s *OIDCProviderService) generateAccessToken(ctx context.Context, userID int64, clientID, scope string) (string, error) {
	// 生成随机访问令牌
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("failed to generate access token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)

	// 保存访问令牌到数据库
	_, err := s.entClient.OidcAccessToken.Create().
		SetToken(token).
		SetUserID(userID).
		SetClientID(clientID).
		SetScope(scope).
		SetExpiresAt(time.Now().Add(time.Hour)). // 1小时
		SetCreatedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to save access token: %w", err)
	}

	return token, nil
}

// generateRefreshToken 生成刷新令牌
func (s *OIDCProviderService) generateRefreshToken(ctx context.Context, userID int64, clientID, scope string) (string, error) {
	// 生成家族ID
	familyBytes := make([]byte, 16)
	if _, err := rand.Read(familyBytes); err != nil {
		return "", fmt.Errorf("failed to generate family ID: %w", err)
	}
	familyID := base64.RawURLEncoding.EncodeToString(familyBytes)

	return s.generateRefreshTokenWithFamily(ctx, userID, clientID, scope, familyID)
}

// generateRefreshTokenWithFamily 生成指定家族的刷新令牌
func (s *OIDCProviderService) generateRefreshTokenWithFamily(ctx context.Context, userID int64, clientID, scope, familyID string) (string, error) {
	// 生成随机刷新令牌
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("failed to generate refresh token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)

	// 保存刷新令牌到数据库
	_, err := s.entClient.OidcRefreshToken.Create().
		SetToken(token).
		SetUserID(userID).
		SetClientID(clientID).
		SetScope(scope).
		SetFamilyID(familyID).
		SetExpiresAt(time.Now().Add(30 * 24 * time.Hour)). // 30天
		SetRevoked(false).
		SetCreatedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to save refresh token: %w", err)
	}

	return token, nil
}

// generateIDToken 生成 ID Token
func (s *OIDCProviderService) generateIDToken(ctx context.Context, userID int64, clientID, scope string) (string, error) {
	// 查询用户信息
	user, err := s.entClient.User.Query().
		Where(user.IDEQ(userID)).
		First(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to query user: %w", err)
	}

	// 构建 ID Token claims
	params := SignIDTokenParams{
		Issuer:    s.cfg.OIDC.Issuer,
		Subject:   fmt.Sprintf("%d", userID),
		Audience:  clientID,
		ExpiresAt: time.Now().Add(time.Hour),
		IssuedAt:  time.Now(),
		Claims: map[string]interface{}{
			"email": user.Email,
			"name":  user.Name,
		},
	}

	// 根据 scope 添加额外的 claims
	scopes := strings.Split(scope, " ")
	for _, scope := range scopes {
		switch scope {
		case "profile":
			// 添加个人信息
			params.Claims["given_name"] = user.Name
			params.Claims["locale"] = "zh-CN" // TODO: 从用户配置获取
		case "email":
			params.Claims["email_verified"] = user.EmailVerified
		}
	}

	return s.signingKeyService.SignIDToken(ctx, params)
}

// RevokeToken 撤销访问令牌
func (s *OIDCProviderService) RevokeToken(ctx context.Context, token string) error {
	// 尝试撤销访问令牌
	accessToken, err := s.entClient.OidcAccessToken.Query().
		Where(oidc_access_token.TokenEQ(token)).
		First(ctx)
	if err == nil {
		// 找到访问令牌，标记为已撤销
		_, err = s.entClient.OidcAccessToken.UpdateOne(accessToken).
			SetRevoked(true).
			SetRevokedAt(time.Now().UTC()).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("failed to revoke access token: %w", err)
		}
		return nil
	}

	// 尝试撤销刷新令牌
	refreshToken, err := s.entClient.OidcRefreshToken.Query().
		Where(oidc_refresh_token.TokenEQ(token)).
		First(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return ErrOIDCTokenNotFound
		}
		return fmt.Errorf("failed to query token: %w", err)
	}

	// 标记刷新令牌为已撤销
	_, err = s.entClient.OidcRefreshToken.UpdateOne(refreshToken).
		SetRevoked(true).
		SetRevokedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to revoke refresh token: %w", err)
	}

	return nil
}

// ValidateAccessToken 验证访问令牌
func (s *OIDCProviderService) ValidateAccessToken(ctx context.Context, accessToken string) (*dbent.OidcAccessToken, error) {
	// 查询访问令牌
	token, err := s.entClient.OidcAccessToken.Query().
		Where(oidc_access_token.TokenEQ(accessToken)).
		First(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, ErrOIDCTokenNotFound
		}
		return nil, fmt.Errorf("failed to query access token: %w", err)
	}

	// 检查令牌是否已过期或已撤销
	if token.ExpiresAt.Before(time.Now()) {
		return nil, ErrOIDCTokenNotFound
	}

	if token.Revoked {
		return nil, ErrOIDCTokenNotFound
	}

	return token, nil
}

// GetClientInfo 获取客户端信息
func (s *OIDCProviderService) GetClientInfo(ctx context.Context, clientID string) (*dbent.OidcClient, error) {
	client, err := s.entClient.OidcClient.Query().
		Where(oidc_client.ClientIDEQ(clientID)).
		First(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, ErrOIDCClientNotFound
		}
		return nil, fmt.Errorf("failed to query client: %w", err)
	}

	if !client.Enabled {
		return nil, ErrOIDCClientDisabled
	}

	return client, nil
}