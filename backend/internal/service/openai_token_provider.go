package service

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"strings"
	"sync/atomic"
	"time"
)

const (
	openAITokenRefreshSkew    = 3 * time.Minute
	openAITokenCacheSkew      = 5 * time.Minute
	openAILockInitialWait     = 20 * time.Millisecond
	openAILockMaxWait         = 120 * time.Millisecond
	openAILockMaxAttempts     = 5
	openAILockJitterRatio     = 0.2
	openAILockWarnThresholdMs = 250
)

// OpenAITokenRuntimeMetrics 表示 OpenAI token 刷新与锁竞争保护指标快照。
type OpenAITokenRuntimeMetrics struct {
	RefreshRequests    int64
	RefreshSuccess     int64
	RefreshFailure     int64
	LockAcquireFailure int64
	LockContention     int64
	LockWaitSamples    int64
	LockWaitTotalMs    int64
	LockWaitHit        int64
	LockWaitMiss       int64
	LastObservedUnixMs int64
REDACTED

type openAITokenRuntimeMetricsStore struct {
	refreshRequests    atomic.Int64
	refreshSuccess     atomic.Int64
	refreshFailure     atomic.Int64
	lockAcquireFailure atomic.Int64
	lockContention     atomic.Int64
	lockWaitSamples    atomic.Int64
	lockWaitTotalMs    atomic.Int64
	lockWaitHit        atomic.Int64
	lockWaitMiss       atomic.Int64
	lastObservedUnixMs atomic.Int64
REDACTED

func (m *openAITokenRuntimeMetricsStore) snapshot() OpenAITokenRuntimeMetrics {
	if m == nil {
		return OpenAITokenRuntimeMetrics{REDACTED
REDACTED
	return OpenAITokenRuntimeMetrics{
		RefreshRequests:    m.refreshRequests.Load(),
		RefreshSuccess:     m.refreshSuccess.Load(),
		RefreshFailure:     m.refreshFailure.Load(),
		LockAcquireFailure: m.lockAcquireFailure.Load(),
		LockContention:     m.lockContention.Load(),
		LockWaitSamples:    m.lockWaitSamples.Load(),
		LockWaitTotalMs:    m.lockWaitTotalMs.Load(),
		LockWaitHit:        m.lockWaitHit.Load(),
		LockWaitMiss:       m.lockWaitMiss.Load(),
		LastObservedUnixMs: m.lastObservedUnixMs.Load(),
REDACTED
REDACTED

func (m *openAITokenRuntimeMetricsStore) touchNow() {
	if m == nil {
		return
REDACTED
	m.lastObservedUnixMs.Store(time.Now().UnixMilli())
REDACTED

// OpenAITokenCache Token 缓存接口（复用 GeminiTokenCache 接口定义）
type OpenAITokenCache = GeminiTokenCache

// OpenAITokenProvider 管理 OpenAI OAuth 账户的 access_token
type OpenAITokenProvider struct {
	accountRepo        AccountRepository
	tokenCache         OpenAITokenCache
	openAIOAuthService *OpenAIOAuthService
	metrics            *openAITokenRuntimeMetricsStore
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
		metrics:            &openAITokenRuntimeMetricsStore{REDACTED,
REDACTED
REDACTED

func (p *OpenAITokenProvider) SnapshotRuntimeMetrics() OpenAITokenRuntimeMetrics {
	if p == nil {
		return OpenAITokenRuntimeMetrics{REDACTED
REDACTED
	p.ensureMetrics()
	return p.metrics.snapshot()
REDACTED

func (p *OpenAITokenProvider) ensureMetrics() {
	if p != nil && p.metrics == nil {
		p.metrics = &openAITokenRuntimeMetricsStore{REDACTED
REDACTED
REDACTED

// GetAccessToken 获取有效的 access_token
func (p *OpenAITokenProvider) GetAccessToken(ctx context.Context, account *Account) (string, error) {
	p.ensureMetrics()
	if account == nil {
		return "", errors.New("account is nil")
REDACTED
	if (account.Platform != PlatformOpenAI && account.Platform != PlatformSora) || account.Type != AccountTypeOAuth {
		return "", errors.New("not an openai/sora oauth account")
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
		p.metrics.refreshRequests.Add(1)
		p.metrics.touchNow()
		locked, lockErr := p.tokenCache.AcquireRefreshLock(ctx, cacheKey, 30*time.Second)
		if lockErr == nil && locked {
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
				if account.Platform == PlatformSora {
					slog.Debug("openai_token_refresh_skipped_for_sora", "account_id", account.ID)
					// Sora 账号不走 OpenAI OAuth 刷新，交由 Sora 客户端的 ST/RT 恢复链路处理。
					refreshFailed = true
			REDACTED else if p.openAIOAuthService == nil {
					slog.Warn("openai_oauth_service_not_configured", "account_id", account.ID)
					p.metrics.refreshFailure.Add(1)
					refreshFailed = true // 无法刷新，标记失败
			REDACTED else {
					tokenInfo, err := p.openAIOAuthService.RefreshAccountToken(ctx, account)
					if err != nil {
						// 刷新失败时记录警告，但不立即返回错误，尝试使用现有 token
						slog.Warn("openai_token_refresh_failed", "account_id", account.ID, "error", err)
						p.metrics.refreshFailure.Add(1)
						refreshFailed = true // 刷新失败，标记以使用短 TTL
				REDACTED else {
						p.metrics.refreshSuccess.Add(1)
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
	REDACTED else if lockErr != nil {
			// Redis 错误导致无法获取锁，降级为无锁刷新（仅在 token 接近过期时）
			p.metrics.lockAcquireFailure.Add(1)
			p.metrics.touchNow()
			slog.Warn("openai_token_lock_failed_degraded_refresh", "account_id", account.ID, "error", lockErr)

			// 检查 ctx 是否已取消
			if ctx.Err() != nil {
				return "", ctx.Err()
		REDACTED

			// 从数据库获取最新账户信息
			if p.accountRepo != nil {
				fresh, err := p.accountRepo.GetByID(ctx, account.ID)
				if err == nil && fresh != nil {
					account = fresh
			REDACTED
		REDACTED
			expiresAt = account.GetCredentialAsTime("expires_at")

			// 仅在 expires_at 已过期/接近过期时才执行无锁刷新
			if expiresAt == nil || time.Until(*expiresAt) <= openAITokenRefreshSkew {
				if account.Platform == PlatformSora {
					slog.Debug("openai_token_refresh_skipped_for_sora_degraded", "account_id", account.ID)
					// Sora 账号不走 OpenAI OAuth 刷新，交由 Sora 客户端的 ST/RT 恢复链路处理。
					refreshFailed = true
			REDACTED else if p.openAIOAuthService == nil {
					slog.Warn("openai_oauth_service_not_configured", "account_id", account.ID)
					p.metrics.refreshFailure.Add(1)
					refreshFailed = true
			REDACTED else {
					tokenInfo, err := p.openAIOAuthService.RefreshAccountToken(ctx, account)
					if err != nil {
						slog.Warn("openai_token_refresh_failed_degraded", "account_id", account.ID, "error", err)
						p.metrics.refreshFailure.Add(1)
						refreshFailed = true
				REDACTED else {
						p.metrics.refreshSuccess.Add(1)
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
			// 锁被其他 worker 持有：使用短轮询+jitter，降低固定等待导致的尾延迟台阶。
			p.metrics.lockContention.Add(1)
			p.metrics.touchNow()
			token, waitErr := p.waitForTokenAfterLockRace(ctx, cacheKey)
			if waitErr != nil {
				return "", waitErr
		REDACTED
			if strings.TrimSpace(token) != "" {
				slog.Debug("openai_token_cache_hit_after_wait", "account_id", account.ID)
				return token, nil
		REDACTED
	REDACTED
REDACTED

	accessToken := account.GetCredential("access_token")
	if strings.TrimSpace(accessToken) == "" {
		return "", errors.New("access_token not found in credentials")
REDACTED

	// 3. 存入缓存（验证版本后再写入，避免异步刷新任务与请求线程的竞态条件）
	if p.tokenCache != nil {
		latestAccount, isStale := CheckTokenVersion(ctx, account, p.accountRepo)
		if isStale && latestAccount != nil {
			// 版本过时，使用 DB 中的最新 token
			slog.Debug("openai_token_version_stale_use_latest", "account_id", account.ID)
			accessToken = latestAccount.GetOpenAIAccessToken()
			if strings.TrimSpace(accessToken) == "" {
				return "", errors.New("access_token not found after version check")
		REDACTED
			// 不写入缓存，让下次请求重新处理
	REDACTED else {
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
REDACTED

	return accessToken, nil
REDACTED

func (p *OpenAITokenProvider) waitForTokenAfterLockRace(ctx context.Context, cacheKey string) (string, error) {
	wait := openAILockInitialWait
	totalWaitMs := int64(0)
	for i := 0; i < openAILockMaxAttempts; i++ {
		actualWait := jitterLockWait(wait)
		timer := time.NewTimer(actualWait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
			REDACTED
		REDACTED
			return "", ctx.Err()
		case <-timer.C:
	REDACTED

		waitMs := actualWait.Milliseconds()
		if waitMs < 0 {
			waitMs = 0
	REDACTED
		totalWaitMs += waitMs
		p.metrics.lockWaitSamples.Add(1)
		p.metrics.lockWaitTotalMs.Add(waitMs)
		p.metrics.touchNow()

		token, err := p.tokenCache.GetAccessToken(ctx, cacheKey)
		if err == nil && strings.TrimSpace(token) != "" {
			p.metrics.lockWaitHit.Add(1)
			if totalWaitMs >= openAILockWarnThresholdMs {
				slog.Warn("openai_token_lock_wait_high", "wait_ms", totalWaitMs, "attempts", i+1)
		REDACTED
			return token, nil
	REDACTED

		if wait < openAILockMaxWait {
			wait *= 2
			if wait > openAILockMaxWait {
				wait = openAILockMaxWait
		REDACTED
	REDACTED
REDACTED

	p.metrics.lockWaitMiss.Add(1)
	if totalWaitMs >= openAILockWarnThresholdMs {
		slog.Warn("openai_token_lock_wait_high", "wait_ms", totalWaitMs, "attempts", openAILockMaxAttempts)
REDACTED
	return "", nil
REDACTED

func jitterLockWait(base time.Duration) time.Duration {
	if base <= 0 {
		return 0
REDACTED
	minFactor := 1 - openAILockJitterRatio
	maxFactor := 1 + openAILockJitterRatio
	factor := minFactor + rand.Float64()*(maxFactor-minFactor)
	return time.Duration(float64(base) * factor)
REDACTED
