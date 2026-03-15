package service

import (
	"context"
	"time"
)

type GeminiTokenRefresher struct {
	geminiOAuthService *GeminiOAuthService
REDACTED

func NewGeminiTokenRefresher(geminiOAuthService *GeminiOAuthService) *GeminiTokenRefresher {
	return &GeminiTokenRefresher{geminiOAuthService: geminiOAuthServiceREDACTED
REDACTED

// CacheKey 返回用于分布式锁的缓存键
func (r *GeminiTokenRefresher) CacheKey(account *Account) string {
	return GeminiTokenCacheKey(account)
REDACTED

func (r *GeminiTokenRefresher) CanRefresh(account *Account) bool {
	return account.Platform == PlatformGemini && account.Type == AccountTypeOAuth
REDACTED

func (r *GeminiTokenRefresher) NeedsRefresh(account *Account, refreshWindow time.Duration) bool {
	if !r.CanRefresh(account) {
		return false
REDACTED
	expiresAt := account.GetCredentialAsTime("expires_at")
	if expiresAt == nil {
		return false
REDACTED
	return time.Until(*expiresAt) < refreshWindow
REDACTED

func (r *GeminiTokenRefresher) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	tokenInfo, err := r.geminiOAuthService.RefreshAccountToken(ctx, account)
	if err != nil {
		return nil, err
REDACTED

	newCredentials := r.geminiOAuthService.BuildAccountCredentials(tokenInfo)
	newCredentials = MergeCredentials(account.Credentials, newCredentials)

	return newCredentials, nil
REDACTED
