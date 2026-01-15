package service

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

const (
	claudeTokenRefreshSkew = 3 * time.Minute
	claudeTokenCacheSkew   = 5 * time.Minute
	claudeLockWaitTime     = 200 * time.Millisecond
)

// ClaudeTokenCache Token 缓存接口（复用 GeminiTokenCache 接口定义）
type ClaudeTokenCache = GeminiTokenCache

// ClaudeTokenProvider 管理 Claude (Anthropic) OAuth 账户的 access_token
type ClaudeTokenProvider struct {
	accountRepo  AccountRepository
	tokenCache   ClaudeTokenCache
	oauthService *OAuthService
REDACTED

func NewClaudeTokenProvider(
	accountRepo AccountRepository,
	tokenCache ClaudeTokenCache,
	oauthService *OAuthService,
) *ClaudeTokenProvider {
	return &ClaudeTokenProvider{
		accountRepo:  accountRepo,
		tokenCache:   tokenCache,
		oauthService: oauthService,
REDACTED
REDACTED

// GetAccessToken 获取有效的 access_token
func (p *ClaudeTokenProvider) GetAccessToken(ctx context.Context, account *Account) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
REDACTED
	if account.Platform != PlatformAnthropic || account.Type != AccountTypeOAuth {
		return "", errors.New("not an anthropic oauth account")
REDACTED

	cacheKey := ClaudeTokenCacheKey(account)

	// 1. 先尝试缓存
	if p.tokenCache != nil {
		if token, err := p.tokenCache.GetAccessToken(ctx, cacheKey); err == nil && strings.TrimSpace(token) != "" {
			slog.Debug("claude_token_cache_hit", "account_id", account.ID)
			return token, nil
	REDACTED else if err != nil {
			slog.Warn("claude_token_cache_get_failed", "account_id", account.ID, "error", err)
	REDACTED
REDACTED

	slog.Debug("claude_token_cache_miss", "account_id", account.ID)

	// 2. 如果即将过期则刷新
	expiresAt := account.GetCredentialAsTime("expires_at")
	needsRefresh := expiresAt == nil || time.Until(*expiresAt) <= claudeTokenRefreshSkew
	refreshFailed := false
	if needsRefresh && p.tokenCache != nil {
		locked, err := p.tokenCache.AcquireRefreshLock(ctx, cacheKey, 30*time.Second)
		if err == nil && locked {
			defer func() { _ = p.tokenCache.ReleaseRefreshLock(ctx, cacheKey) REDACTED()

			// 拿到锁后再次检查缓存（另一个 worker 可能已刷新）
			if token, err := p.tokenCache.GetAccessToken(ctx, cacheKey); err == nil && strings.TrimSpace(token) != "" {
				return token, nil
		REDACTED

			// 从数据库获取最新账户信息
			fresh, err := p.accountRepo.GetByID(ctx, account.ID)
			if err == nil && fresh != nil {
				account = fresh
		REDACTED
			expiresAt = account.GetCredentialAsTime("expires_at")
			if expiresAt == nil || time.Until(*expiresAt) <= claudeTokenRefreshSkew {
				if p.oauthService == nil {
					slog.Warn("claude_oauth_service_not_configured", "account_id", account.ID)
					refreshFailed = true // 无法刷新，标记失败
			REDACTED else {
					tokenInfo, err := p.oauthService.RefreshAccountToken(ctx, account)
					if err != nil {
						// 刷新失败时记录警告，但不立即返回错误，尝试使用现有 token
						slog.Warn("claude_token_refresh_failed", "account_id", account.ID, "error", err)
						refreshFailed = true // 刷新失败，标记以使用短 TTL
				REDACTED else {
						// 构建新 credentials，保留原有字段
						newCredentials := make(map[string]any)
						for k, v := range account.Credentials {
							newCredentials[k] = v
					REDACTED
						newCredentials["access_token"] = tokenInfo.AccessToken
						newCredentials["token_type"] = tokenInfo.TokenType
						newCredentials["expires_in"] = strconv.FormatInt(tokenInfo.ExpiresIn, 10)
						newCredentials["expires_at"] = strconv.FormatInt(tokenInfo.ExpiresAt, 10)
						if tokenInfo.RefreshToken != "" {
							newCredentials["refresh_token"] = tokenInfo.RefreshToken
					REDACTED
						if tokenInfo.Scope != "" {
							newCredentials["scope"] = tokenInfo.Scope
					REDACTED
						account.Credentials = newCredentials
						if updateErr := p.accountRepo.Update(ctx, account); updateErr != nil {
							slog.Error("claude_token_provider_update_failed", "account_id", account.ID, "error", updateErr)
					REDACTED
						expiresAt = account.GetCredentialAsTime("expires_at")
				REDACTED
			REDACTED
		REDACTED
	REDACTED else {
			// 锁获取失败，等待 200ms 后重试读取缓存（改进：减少并发时的缓存未命中）
			time.Sleep(claudeLockWaitTime)
			if token, err := p.tokenCache.GetAccessToken(ctx, cacheKey); err == nil && strings.TrimSpace(token) != "" {
				slog.Debug("claude_token_cache_hit_after_wait", "account_id", account.ID)
				return token, nil
		REDACTED
	REDACTED
REDACTED

	accessToken := account.GetCredential("access_token")
	if strings.TrimSpace(accessToken) == "" {
		return "", errors.New("access_token not found in credentials")
REDACTED

	// 3. 存入缓存
	if p.tokenCache != nil {
		ttl := 30 * time.Minute
		if refreshFailed {
			// 刷新失败时使用短 TTL，避免失效 token 长时间缓存导致 401 抖动
			ttl = time.Minute
			slog.Debug("claude_token_cache_short_ttl", "account_id", account.ID, "reason", "refresh_failed")
	REDACTED else if expiresAt != nil {
			until := time.Until(*expiresAt)
			switch {
			case until > claudeTokenCacheSkew:
				ttl = until - claudeTokenCacheSkew
			case until > 0:
				ttl = until
			default:
				ttl = time.Minute
		REDACTED
	REDACTED
		if err := p.tokenCache.SetAccessToken(ctx, cacheKey, accessToken, ttl); err != nil {
			slog.Warn("claude_token_cache_set_failed", "account_id", account.ID, "error", err)
	REDACTED
REDACTED

	return accessToken, nil
REDACTED
