package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
)

const (
	openAITokenRefreshSkew = 3 * time.Minute
	openAITokenCacheSkew   = 5 * time.Minute
	openAILockWaitTime     = 200 * time.Millisecond
)

// OpenAITokenCache Token 缓存接口（复用 GeminiTokenCache 接口定义）
type OpenAITokenCache = GeminiTokenCache

// OpenAITokenProvider 管理 OpenAI OAuth 账户的 access_token
type OpenAITokenProvider struct {
	accountRepo        AccountRepository
	tokenCache         OpenAITokenCache
	openAIOAuthService *OpenAIOAuthService
REDACTED

func NewOpenAITokenProvider(
	accountRepo AccountRepository,
	tokenCache OpenAITokenCache,
	openAIOAuthService *OpenAIOAuthService,
) *OpenAITokenProvider {
	return &OpenAITokenProvider{
		accountRepo:        accountRepo,
		tokenCache:         tokenCache,
		openAIOAuthService: openAIOAuthService,
REDACTED
REDACTED

// GetAccessToken 获取有效的 access_token
func (p *OpenAITokenProvider) GetAccessToken(ctx context.Context, account *Account) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
REDACTED
	if account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth {
		return "", errors.New("not an openai oauth account")
REDACTED

	cacheKey := OpenAITokenCacheKey(account)

	// 1. 先尝试缓存
	if p.tokenCache != nil {
		if token, err := p.tokenCache.GetAccessToken(ctx, cacheKey); err == nil && strings.TrimSpace(token) != "" {
			slog.Debug("openai_token_cache_hit", "account_id", account.ID)
			return token, nil
	REDACTED else if err != nil {
			slog.Warn("openai_token_cache_get_failed", "account_id", account.ID, "error", err)
	REDACTED
REDACTED

	slog.Debug("openai_token_cache_miss", "account_id", account.ID)

	// 2. 如果即将过期则刷新
	expiresAt := account.GetCredentialAsTime("expires_at")
	needsRefresh := expiresAt == nil || time.Until(*expiresAt) <= openAITokenRefreshSkew
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
			if expiresAt == nil || time.Until(*expiresAt) <= openAITokenRefreshSkew {
				if p.openAIOAuthService == nil {
					slog.Warn("openai_oauth_service_not_configured", "account_id", account.ID)
					refreshFailed = true // 无法刷新，标记失败
			REDACTED else {
					tokenInfo, err := p.openAIOAuthService.RefreshAccountToken(ctx, account)
					if err != nil {
						// 刷新失败时记录警告，但不立即返回错误，尝试使用现有 token
						slog.Warn("openai_token_refresh_failed", "account_id", account.ID, "error", err)
						refreshFailed = true // 刷新失败，标记以使用短 TTL
				REDACTED else {
						newCredentials := p.openAIOAuthService.BuildAccountCredentials(tokenInfo)
						for k, v := range account.Credentials {
							if _, exists := newCredentials[k]; !exists {
								newCredentials[k] = v
						REDACTED
					REDACTED
						account.Credentials = newCredentials
						if updateErr := p.accountRepo.Update(ctx, account); updateErr != nil {
							slog.Error("openai_token_provider_update_failed", "account_id", account.ID, "error", updateErr)
					REDACTED
						expiresAt = account.GetCredentialAsTime("expires_at")
				REDACTED
			REDACTED
		REDACTED
	REDACTED else {
			// 锁获取失败，等待 200ms 后重试读取缓存（改进：减少并发时的缓存未命中）
			time.Sleep(openAILockWaitTime)
			if token, err := p.tokenCache.GetAccessToken(ctx, cacheKey); err == nil && strings.TrimSpace(token) != "" {
				slog.Debug("openai_token_cache_hit_after_wait", "account_id", account.ID)
				return token, nil
		REDACTED
	REDACTED
REDACTED

	accessToken := account.GetOpenAIAccessToken()
	if strings.TrimSpace(accessToken) == "" {
		return "", errors.New("access_token not found in credentials")
REDACTED

	// 3. 存入缓存
	if p.tokenCache != nil {
		ttl := 30 * time.Minute
		if refreshFailed {
			// 刷新失败时使用短 TTL，避免失效 token 长时间缓存导致 401 抖动
			ttl = time.Minute
			slog.Debug("openai_token_cache_short_ttl", "account_id", account.ID, "reason", "refresh_failed")
	REDACTED else if expiresAt != nil {
			until := time.Until(*expiresAt)
			switch {
			case until > openAITokenCacheSkew:
				ttl = until - openAITokenCacheSkew
			case until > 0:
				ttl = until
			default:
				ttl = time.Minute
		REDACTED
	REDACTED
		if err := p.tokenCache.SetAccessToken(ctx, cacheKey, accessToken, ttl); err != nil {
			slog.Warn("openai_token_cache_set_failed", "account_id", account.ID, "error", err)
	REDACTED
REDACTED

	return accessToken, nil
REDACTED
