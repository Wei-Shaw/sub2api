package service

import (
	"context"
	"errors"
	"strings"
	"time"
)

const grokTokenRefreshSkew = time.Hour

type GrokTokenRefresher struct {
	grokOAuthService *GrokOAuthService
REDACTED

func NewGrokTokenRefresher(grokOAuthService *GrokOAuthService) *GrokTokenRefresher {
	return &GrokTokenRefresher{grokOAuthService: grokOAuthServiceREDACTED
REDACTED

func (r *GrokTokenRefresher) CacheKey(account *Account) string {
	return GrokTokenCacheKey(account)
REDACTED

func (r *GrokTokenRefresher) CanRefresh(account *Account) bool {
	return account != nil && account.Platform == PlatformGrok && account.Type == AccountTypeOAuth
REDACTED

func (r *GrokTokenRefresher) NeedsRefresh(account *Account, refreshWindow time.Duration) bool {
	if account == nil || strings.TrimSpace(account.GetGrokRefreshToken()) == "" {
		return false
REDACTED
	expiresAt := account.GetCredentialAsTime("expires_at")
	if expiresAt == nil {
		return true
REDACTED
	if refreshWindow < grokTokenRefreshSkew {
		refreshWindow = grokTokenRefreshSkew
REDACTED
	return time.Until(*expiresAt) < refreshWindow
REDACTED

func (r *GrokTokenRefresher) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	if r == nil || r.grokOAuthService == nil {
		return nil, errors.New("grok oauth service is not configured")
REDACTED
	tokenInfo, err := r.grokOAuthService.RefreshAccountToken(ctx, account)
	if err != nil {
		return nil, err
REDACTED
	newCredentials := r.grokOAuthService.BuildAccountCredentials(tokenInfo)
	newCredentials = MergeCredentials(account.Credentials, newCredentials)
	if baseURL := strings.TrimSpace(account.GetCredential("base_url")); baseURL != "" {
		newCredentials["base_url"] = baseURL
REDACTED
	return newCredentials, nil
REDACTED
