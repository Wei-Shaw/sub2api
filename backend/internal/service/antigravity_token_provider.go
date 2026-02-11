package service

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	antigravityTokenRefreshSkew = 3 * time.Minute
	antigravityTokenCacheSkew   = 5 * time.Minute
	antigravityBackfillCooldown = 5 * time.Minute
)

// AntigravityTokenCache Token 缓存接口（复用 GeminiTokenCache 接口定义）
type AntigravityTokenCache = GeminiTokenCache

// AntigravityTokenProvider 管理 Antigravity 账户的 access_token
type AntigravityTokenProvider struct {
	accountRepo             AccountRepository
	tokenCache              AntigravityTokenCache
	antigravityOAuthService *AntigravityOAuthService
	backfillCooldown        sync.Map // key: int64 (account.ID) → value: time.Time
REDACTED

func NewAntigravityTokenProvider(
	accountRepo AccountRepository,
	tokenCache AntigravityTokenCache,
	antigravityOAuthService *AntigravityOAuthService,
) *AntigravityTokenProvider {
	return &AntigravityTokenProvider{
		accountRepo:             accountRepo,
		tokenCache:              tokenCache,
		antigravityOAuthService: antigravityOAuthService,
REDACTED
REDACTED

// GetAccessToken 获取有效的 access_token
func (p *AntigravityTokenProvider) GetAccessToken(ctx context.Context, account *Account) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
REDACTED
	if account.Platform != PlatformAntigravity {
		return "", errors.New("not an antigravity account")
REDACTED
	// upstream 类型：直接从 credentials 读取 api_key，不走 OAuth 刷新流程
	if account.Type == AccountTypeUpstream {
		apiKey := account.GetCredential("api_key")
		if apiKey == "" {
			return "", errors.New("upstream account missing api_key in credentials")
	REDACTED
		return apiKey, nil
REDACTED
	if account.Type != AccountTypeOAuth {
		return "", errors.New("not an antigravity oauth account")
REDACTED

	cacheKey := AntigravityTokenCacheKey(account)

	// 1. 先尝试缓存
	if p.tokenCache != nil {
		if token, err := p.tokenCache.GetAccessToken(ctx, cacheKey); err == nil && strings.TrimSpace(token) != "" {
			return token, nil
	REDACTED
REDACTED

	// 2. 如果即将过期则刷新
	expiresAt := account.GetCredentialAsTime("expires_at")
	needsRefresh := expiresAt == nil || time.Until(*expiresAt) <= antigravityTokenRefreshSkew
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
			if expiresAt == nil || time.Until(*expiresAt) <= antigravityTokenRefreshSkew {
				if p.antigravityOAuthService == nil {
					return "", errors.New("antigravity oauth service not configured")
			REDACTED
				tokenInfo, err := p.antigravityOAuthService.RefreshAccountToken(ctx, account)
				if err != nil {
					return "", err
			REDACTED
				p.mergeCredentials(account, tokenInfo)
				if updateErr := p.accountRepo.Update(ctx, account); updateErr != nil {
					log.Printf("[AntigravityTokenProvider] Failed to update account credentials: %v", updateErr)
			REDACTED
				expiresAt = account.GetCredentialAsTime("expires_at")
		REDACTED
	REDACTED
REDACTED

	accessToken := account.GetCredential("access_token")
	if strings.TrimSpace(accessToken) == "" {
		return "", errors.New("access_token not found in credentials")
REDACTED

	// 如果账号还没有 project_id，尝试在线补齐，避免请求 daily/sandbox 时出现
	// "Invalid project resource name projects/"。
	// 仅调用 loadProjectIDWithRetry，不刷新 OAuth token；带冷却机制防止频繁重试。
	if strings.TrimSpace(account.GetCredential("project_id")) == "" && p.antigravityOAuthService != nil {
		if p.shouldAttemptBackfill(account.ID) {
			p.markBackfillAttempted(account.ID)
			if projectID, err := p.antigravityOAuthService.FillProjectID(ctx, account, accessToken); err == nil && projectID != "" {
				account.Credentials["project_id"] = projectID
				if updateErr := p.accountRepo.Update(ctx, account); updateErr != nil {
					log.Printf("[AntigravityTokenProvider] project_id 补齐持久化失败: %v", updateErr)
			REDACTED
		REDACTED
	REDACTED
REDACTED

	// 3. 存入缓存（验证版本后再写入，避免异步刷新任务与请求线程的竞态条件）
	if p.tokenCache != nil {
		latestAccount, isStale := CheckTokenVersion(ctx, account, p.accountRepo)
		if isStale && latestAccount != nil {
			// 版本过时，使用 DB 中的最新 token
			slog.Debug("antigravity_token_version_stale_use_latest", "account_id", account.ID)
			accessToken = latestAccount.GetCredential("access_token")
			if strings.TrimSpace(accessToken) == "" {
				return "", errors.New("access_token not found after version check")
		REDACTED
			// 不写入缓存，让下次请求重新处理
	REDACTED else {
			ttl := 30 * time.Minute
			if expiresAt != nil {
				until := time.Until(*expiresAt)
				switch {
				case until > antigravityTokenCacheSkew:
					ttl = until - antigravityTokenCacheSkew
				case until > 0:
					ttl = until
				default:
					ttl = time.Minute
			REDACTED
		REDACTED
			_ = p.tokenCache.SetAccessToken(ctx, cacheKey, accessToken, ttl)
	REDACTED
REDACTED

	return accessToken, nil
REDACTED

// mergeCredentials 将 tokenInfo 构建的凭证合并到 account 中，保留原有未覆盖的字段
func (p *AntigravityTokenProvider) mergeCredentials(account *Account, tokenInfo *AntigravityTokenInfo) {
	newCredentials := p.antigravityOAuthService.BuildAccountCredentials(tokenInfo)
	for k, v := range account.Credentials {
		if _, exists := newCredentials[k]; !exists {
			newCredentials[k] = v
	REDACTED
REDACTED
	account.Credentials = newCredentials
REDACTED

// shouldAttemptBackfill 检查是否应该尝试补齐 project_id（冷却期内不重复尝试）
func (p *AntigravityTokenProvider) shouldAttemptBackfill(accountID int64) bool {
	if v, ok := p.backfillCooldown.Load(accountID); ok {
		if lastAttempt, ok := v.(time.Time); ok {
			return time.Since(lastAttempt) > antigravityBackfillCooldown
	REDACTED
REDACTED
	return true
REDACTED

func (p *AntigravityTokenProvider) markBackfillAttempted(accountID int64) {
	p.backfillCooldown.Store(accountID, time.Now())
REDACTED

func AntigravityTokenCacheKey(account *Account) string {
	projectID := strings.TrimSpace(account.GetCredential("project_id"))
	if projectID != "" {
		return "ag:" + projectID
REDACTED
	return "ag:account:" + strconv.FormatInt(account.ID, 10)
REDACTED
