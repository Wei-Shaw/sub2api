package service

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"
	"time"
)

const (
	antigravityTokenRefreshSkew = 3 * time.Minute
	antigravityTokenCacheSkew   = 5 * time.Minute
)

// AntigravityTokenCache Token 缓存接口（复用 GeminiTokenCache 接口定义）
type AntigravityTokenCache = GeminiTokenCache

// AntigravityTokenProvider 管理 Antigravity 账户的 access_token
type AntigravityTokenProvider struct {
	accountRepo            AccountRepository
	tokenCache             AntigravityTokenCache
	antigravityOAuthService *AntigravityOAuthService
REDACTED

func NewAntigravityTokenProvider(
	accountRepo AccountRepository,
	tokenCache AntigravityTokenCache,
	antigravityOAuthService *AntigravityOAuthService,
) *AntigravityTokenProvider {
	return &AntigravityTokenProvider{
		accountRepo:            accountRepo,
		tokenCache:             tokenCache,
		antigravityOAuthService: antigravityOAuthService,
REDACTED
REDACTED

// GetAccessToken 获取有效的 access_token
func (p *AntigravityTokenProvider) GetAccessToken(ctx context.Context, account *Account) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
REDACTED
	if account.Platform != PlatformAntigravity || account.Type != AccountTypeOAuth {
		return "", errors.New("not an antigravity oauth account")
REDACTED

	cacheKey := antigravityTokenCacheKey(account)

	// 1. 先尝试缓存
	if p.tokenCache != nil {
		if token, err := p.tokenCache.GetAccessToken(ctx, cacheKey); err == nil && strings.TrimSpace(token) != "" {
			return token, nil
	REDACTED
REDACTED

	// 2. 如果即将过期则刷新
	expiresAt := parseAntigravityExpiresAt(account)
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
			expiresAt = parseAntigravityExpiresAt(account)
			if expiresAt == nil || time.Until(*expiresAt) <= antigravityTokenRefreshSkew {
				if p.antigravityOAuthService == nil {
					return "", errors.New("antigravity oauth service not configured")
			REDACTED
				tokenInfo, err := p.antigravityOAuthService.RefreshAccountToken(ctx, account)
				if err != nil {
					return "", err
			REDACTED
				newCredentials := p.antigravityOAuthService.BuildAccountCredentials(tokenInfo)
				for k, v := range account.Credentials {
					if _, exists := newCredentials[k]; !exists {
						newCredentials[k] = v
				REDACTED
			REDACTED
				account.Credentials = newCredentials
				if updateErr := p.accountRepo.Update(ctx, account); updateErr != nil {
					log.Printf("[AntigravityTokenProvider] Failed to update account credentials: %v", updateErr)
			REDACTED
				expiresAt = parseAntigravityExpiresAt(account)
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

	return accessToken, nil
REDACTED

func antigravityTokenCacheKey(account *Account) string {
	projectID := strings.TrimSpace(account.GetCredential("project_id"))
	if projectID != "" {
		return "ag:" + projectID
REDACTED
	return "ag:account:" + strconv.FormatInt(account.ID, 10)
REDACTED

func parseAntigravityExpiresAt(account *Account) *time.Time {
	raw := strings.TrimSpace(account.GetCredential("expires_at"))
	if raw == "" {
		return nil
REDACTED
	if unixSec, err := strconv.ParseInt(raw, 10, 64); err == nil && unixSec > 0 {
		t := time.Unix(unixSec, 0)
		return &t
REDACTED
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return &t
REDACTED
	return nil
REDACTED
