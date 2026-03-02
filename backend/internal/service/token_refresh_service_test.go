//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type tokenRefreshAccountRepo struct {
	mockAccountRepoForGemini
	updateCalls    int
	setErrorCalls  int
	clearTempCalls int
	lastAccount    *Account
	updateErr      error
REDACTED

func (r *tokenRefreshAccountRepo) Update(ctx context.Context, account *Account) error {
	r.updateCalls++
	r.lastAccount = account
	return r.updateErr
REDACTED

func (r *tokenRefreshAccountRepo) SetError(ctx context.Context, id int64, errorMsg string) error {
	r.setErrorCalls++
	return nil
REDACTED

func (r *tokenRefreshAccountRepo) ClearTempUnschedulable(ctx context.Context, id int64) error {
	r.clearTempCalls++
	return nil
REDACTED

type tokenCacheInvalidatorStub struct {
	calls int
	err   error
REDACTED

func (s *tokenCacheInvalidatorStub) InvalidateToken(ctx context.Context, account *Account) error {
	s.calls++
	return s.err
REDACTED

type tempUnschedCacheStub struct {
	deleteCalls int
REDACTED

func (s *tempUnschedCacheStub) SetTempUnsched(ctx context.Context, accountID int64, state *TempUnschedState) error {
	return nil
REDACTED

func (s *tempUnschedCacheStub) GetTempUnsched(ctx context.Context, accountID int64) (*TempUnschedState, error) {
	return nil, nil
REDACTED

func (s *tempUnschedCacheStub) DeleteTempUnsched(ctx context.Context, accountID int64) error {
	s.deleteCalls++
	return nil
REDACTED

type tokenRefresherStub struct {
	credentials map[string]any
	err         error
REDACTED

func (r *tokenRefresherStub) CanRefresh(account *Account) bool {
	return true
REDACTED

func (r *tokenRefresherStub) NeedsRefresh(account *Account, refreshWindowDuration time.Duration) bool {
	return true
REDACTED

func (r *tokenRefresherStub) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	if r.err != nil {
		return nil, r.err
REDACTED
	return r.credentials, nil
REDACTED

func TestTokenRefreshService_RefreshWithRetry_InvalidatesCache(t *testing.T) {
	repo := &tokenRefreshAccountRepo{REDACTED
	invalidator := &tokenCacheInvalidatorStub{REDACTED
	cfg := &config.Config{
		TokenRefresh: config.TokenRefreshConfig{
			MaxRetries:          1,
			RetryBackoffSeconds: 0,
	REDACTED,
REDACTED
	service := NewTokenRefreshService(repo, nil, nil, nil, nil, invalidator, nil, cfg, nil)
	account := &Account{
		ID:       5,
REDACTED
		Type:     AccountTypeOAuth,
REDACTED
	refresher := &tokenRefresherStub{
		credentials: map[string]any{
			"access_token": "new-token",
	REDACTED,
REDACTED

	err := service.refreshWithRetry(context.Background(), account, refresher)
REDACTED
	require.Equal(t, 1, repo.updateCalls)
	require.Equal(t, 1, invalidator.calls)
	require.Equal(t, "new-token", account.GetCredential("access_token"))
REDACTED

func TestTokenRefreshService_RefreshWithRetry_InvalidatorErrorIgnored(t *testing.T) {
	repo := &tokenRefreshAccountRepo{REDACTED
	invalidator := &tokenCacheInvalidatorStub{err: errors.New("invalidate failed")REDACTED
	cfg := &config.Config{
		TokenRefresh: config.TokenRefreshConfig{
			MaxRetries:          1,
			RetryBackoffSeconds: 0,
	REDACTED,
REDACTED
	service := NewTokenRefreshService(repo, nil, nil, nil, nil, invalidator, nil, cfg, nil)
	account := &Account{
		ID:       6,
REDACTED
		Type:     AccountTypeOAuth,
REDACTED
	refresher := &tokenRefresherStub{
		credentials: map[string]any{
			"access_token": "token",
	REDACTED,
REDACTED

	err := service.refreshWithRetry(context.Background(), account, refresher)
REDACTED
	require.Equal(t, 1, repo.updateCalls)
	require.Equal(t, 1, invalidator.calls)
REDACTED

func TestTokenRefreshService_RefreshWithRetry_NilInvalidator(t *testing.T) {
	repo := &tokenRefreshAccountRepo{REDACTED
	cfg := &config.Config{
		TokenRefresh: config.TokenRefreshConfig{
			MaxRetries:          1,
			RetryBackoffSeconds: 0,
	REDACTED,
REDACTED
	service := NewTokenRefreshService(repo, nil, nil, nil, nil, nil, nil, cfg, nil)
	account := &Account{
		ID:       7,
REDACTED
		Type:     AccountTypeOAuth,
REDACTED
	refresher := &tokenRefresherStub{
		credentials: map[string]any{
			"access_token": "token",
	REDACTED,
REDACTED

	err := service.refreshWithRetry(context.Background(), account, refresher)
REDACTED
	require.Equal(t, 1, repo.updateCalls)
REDACTED

// TestTokenRefreshService_RefreshWithRetry_Antigravity 测试 Antigravity 平台的缓存失效
func TestTokenRefreshService_RefreshWithRetry_Antigravity(t *testing.T) {
	repo := &tokenRefreshAccountRepo{REDACTED
	invalidator := &tokenCacheInvalidatorStub{REDACTED
	cfg := &config.Config{
		TokenRefresh: config.TokenRefreshConfig{
			MaxRetries:          1,
			RetryBackoffSeconds: 0,
	REDACTED,
REDACTED
	service := NewTokenRefreshService(repo, nil, nil, nil, nil, invalidator, nil, cfg, nil)
	account := &Account{
		ID:       8,
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
REDACTED
	refresher := &tokenRefresherStub{
		credentials: map[string]any{
			"access_token": "ag-token",
	REDACTED,
REDACTED

	err := service.refreshWithRetry(context.Background(), account, refresher)
REDACTED
	require.Equal(t, 1, repo.updateCalls)
	require.Equal(t, 1, invalidator.calls) // Antigravity 也应触发缓存失效
REDACTED

// TestTokenRefreshService_RefreshWithRetry_NonOAuthAccount 测试非 OAuth 账号不触发缓存失效
func TestTokenRefreshService_RefreshWithRetry_NonOAuthAccount(t *testing.T) {
	repo := &tokenRefreshAccountRepo{REDACTED
	invalidator := &tokenCacheInvalidatorStub{REDACTED
	cfg := &config.Config{
		TokenRefresh: config.TokenRefreshConfig{
			MaxRetries:          1,
			RetryBackoffSeconds: 0,
	REDACTED,
REDACTED
	service := NewTokenRefreshService(repo, nil, nil, nil, nil, invalidator, nil, cfg, nil)
	account := &Account{
		ID:       9,
REDACTED
		Type:     AccountTypeAPIKey, // 非 OAuth
REDACTED
	refresher := &tokenRefresherStub{
		credentials: map[string]any{
			"access_token": "token",
	REDACTED,
REDACTED

	err := service.refreshWithRetry(context.Background(), account, refresher)
REDACTED
	require.Equal(t, 1, repo.updateCalls)
	require.Equal(t, 0, invalidator.calls) // 非 OAuth 不触发缓存失效
REDACTED

// TestTokenRefreshService_RefreshWithRetry_OtherPlatformOAuth 测试所有 OAuth 平台都触发缓存失效
func TestTokenRefreshService_RefreshWithRetry_OtherPlatformOAuth(t *testing.T) {
	repo := &tokenRefreshAccountRepo{REDACTED
	invalidator := &tokenCacheInvalidatorStub{REDACTED
	cfg := &config.Config{
		TokenRefresh: config.TokenRefreshConfig{
			MaxRetries:          1,
			RetryBackoffSeconds: 0,
	REDACTED,
REDACTED
	service := NewTokenRefreshService(repo, nil, nil, nil, nil, invalidator, nil, cfg, nil)
	account := &Account{
		ID:       10,
		Platform: PlatformOpenAI, // OpenAI OAuth 账户
		Type:     AccountTypeOAuth,
REDACTED
	refresher := &tokenRefresherStub{
		credentials: map[string]any{
			"access_token": "token",
	REDACTED,
REDACTED

	err := service.refreshWithRetry(context.Background(), account, refresher)
REDACTED
	require.Equal(t, 1, repo.updateCalls)
	require.Equal(t, 1, invalidator.calls) // 所有 OAuth 账户刷新后触发缓存失效
REDACTED

// TestTokenRefreshService_RefreshWithRetry_UpdateFailed 测试更新失败的情况
func TestTokenRefreshService_RefreshWithRetry_UpdateFailed(t *testing.T) {
	repo := &tokenRefreshAccountRepo{updateErr: errors.New("update failed")REDACTED
	invalidator := &tokenCacheInvalidatorStub{REDACTED
	cfg := &config.Config{
		TokenRefresh: config.TokenRefreshConfig{
			MaxRetries:          1,
			RetryBackoffSeconds: 0,
	REDACTED,
REDACTED
	service := NewTokenRefreshService(repo, nil, nil, nil, nil, invalidator, nil, cfg, nil)
	account := &Account{
		ID:       11,
REDACTED
		Type:     AccountTypeOAuth,
REDACTED
	refresher := &tokenRefresherStub{
		credentials: map[string]any{
			"access_token": "token",
	REDACTED,
REDACTED

	err := service.refreshWithRetry(context.Background(), account, refresher)
REDACTED
	require.Contains(t, err.Error(), "failed to save credentials")
	require.Equal(t, 1, repo.updateCalls)
	require.Equal(t, 0, invalidator.calls) // 更新失败时不应触发缓存失效
REDACTED

// TestTokenRefreshService_RefreshWithRetry_RefreshFailed 测试可重试错误耗尽不标记 error
func TestTokenRefreshService_RefreshWithRetry_RefreshFailed(t *testing.T) {
	repo := &tokenRefreshAccountRepo{REDACTED
	invalidator := &tokenCacheInvalidatorStub{REDACTED
	cfg := &config.Config{
		TokenRefresh: config.TokenRefreshConfig{
			MaxRetries:          2,
			RetryBackoffSeconds: 0,
	REDACTED,
REDACTED
	service := NewTokenRefreshService(repo, nil, nil, nil, nil, invalidator, nil, cfg, nil)
	account := &Account{
		ID:       12,
REDACTED
		Type:     AccountTypeOAuth,
REDACTED
	refresher := &tokenRefresherStub{
		err: errors.New("refresh failed"),
REDACTED

	err := service.refreshWithRetry(context.Background(), account, refresher)
REDACTED
	require.Equal(t, 0, repo.updateCalls)   // 刷新失败不应更新
	require.Equal(t, 0, invalidator.calls)  // 刷新失败不应触发缓存失效
	require.Equal(t, 0, repo.setErrorCalls) // 可重试错误耗尽不标记 error，下个周期继续重试
REDACTED

// TestTokenRefreshService_RefreshWithRetry_AntigravityRefreshFailed 测试 Antigravity 刷新失败不设置错误状态
func TestTokenRefreshService_RefreshWithRetry_AntigravityRefreshFailed(t *testing.T) {
	repo := &tokenRefreshAccountRepo{REDACTED
	invalidator := &tokenCacheInvalidatorStub{REDACTED
	cfg := &config.Config{
		TokenRefresh: config.TokenRefreshConfig{
			MaxRetries:          1,
			RetryBackoffSeconds: 0,
	REDACTED,
REDACTED
	service := NewTokenRefreshService(repo, nil, nil, nil, nil, invalidator, nil, cfg, nil)
	account := &Account{
		ID:       13,
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
REDACTED
	refresher := &tokenRefresherStub{
		err: errors.New("network error"), // 可重试错误
REDACTED

	err := service.refreshWithRetry(context.Background(), account, refresher)
REDACTED
	require.Equal(t, 0, repo.updateCalls)
	require.Equal(t, 0, invalidator.calls)
	require.Equal(t, 0, repo.setErrorCalls) // Antigravity 可重试错误不设置错误状态
REDACTED

// TestTokenRefreshService_RefreshWithRetry_AntigravityNonRetryableError 测试 Antigravity 不可重试错误
func TestTokenRefreshService_RefreshWithRetry_AntigravityNonRetryableError(t *testing.T) {
	repo := &tokenRefreshAccountRepo{REDACTED
	invalidator := &tokenCacheInvalidatorStub{REDACTED
	cfg := &config.Config{
		TokenRefresh: config.TokenRefreshConfig{
			MaxRetries:          3,
			RetryBackoffSeconds: 0,
	REDACTED,
REDACTED
	service := NewTokenRefreshService(repo, nil, nil, nil, nil, invalidator, nil, cfg, nil)
	account := &Account{
		ID:       14,
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
REDACTED
	refresher := &tokenRefresherStub{
		err: errors.New("invalid_grant: token revoked"), // 不可重试错误
REDACTED

	err := service.refreshWithRetry(context.Background(), account, refresher)
REDACTED
	require.Equal(t, 0, repo.updateCalls)
	require.Equal(t, 0, invalidator.calls)
	require.Equal(t, 1, repo.setErrorCalls) // 不可重试错误应设置错误状态
REDACTED

// TestTokenRefreshService_RefreshWithRetry_ClearsTempUnschedulable 测试刷新成功后清除临时不可调度（DB + Redis）
func TestTokenRefreshService_RefreshWithRetry_ClearsTempUnschedulable(t *testing.T) {
	repo := &tokenRefreshAccountRepo{REDACTED
	invalidator := &tokenCacheInvalidatorStub{REDACTED
	tempCache := &tempUnschedCacheStub{REDACTED
	cfg := &config.Config{
		TokenRefresh: config.TokenRefreshConfig{
			MaxRetries:          1,
			RetryBackoffSeconds: 0,
	REDACTED,
REDACTED
	service := NewTokenRefreshService(repo, nil, nil, nil, nil, invalidator, nil, cfg, tempCache)
	until := time.Now().Add(10 * time.Minute)
	account := &Account{
		ID:                     15,
		Platform:               PlatformGemini,
		Type:                   AccountTypeOAuth,
		TempUnschedulableUntil: &until,
REDACTED
	refresher := &tokenRefresherStub{
		credentials: map[string]any{
			"access_token": "new-token",
	REDACTED,
REDACTED

	err := service.refreshWithRetry(context.Background(), account, refresher)
REDACTED
	require.Equal(t, 1, repo.updateCalls)
	require.Equal(t, 1, repo.clearTempCalls)  // DB 清除
	require.Equal(t, 1, tempCache.deleteCalls) // Redis 缓存也应清除
REDACTED

// TestTokenRefreshService_RefreshWithRetry_NonRetryableErrorAllPlatforms 测试所有平台不可重试错误都 SetError
func TestTokenRefreshService_RefreshWithRetry_NonRetryableErrorAllPlatforms(t *testing.T) {
	tests := []struct {
		name     string
		platform string
REDACTED{
		{name: "gemini", platform: PlatformGeminiREDACTED,
		{name: "anthropic", platform: PlatformAnthropicREDACTED,
		{name: "openai", platform: PlatformOpenAIREDACTED,
		{name: "antigravity", platform: PlatformAntigravityREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &tokenRefreshAccountRepo{REDACTED
			invalidator := &tokenCacheInvalidatorStub{REDACTED
			cfg := &config.Config{
				TokenRefresh: config.TokenRefreshConfig{
					MaxRetries:          3,
					RetryBackoffSeconds: 0,
			REDACTED,
		REDACTED
			service := NewTokenRefreshService(repo, nil, nil, nil, nil, invalidator, nil, cfg, nil)
			account := &Account{
				ID:       16,
				Platform: tt.platform,
				Type:     AccountTypeOAuth,
		REDACTED
			refresher := &tokenRefresherStub{
				err: errors.New("invalid_grant: token revoked"),
		REDACTED

			err := service.refreshWithRetry(context.Background(), account, refresher)
		REDACTED
			require.Equal(t, 1, repo.setErrorCalls) // 所有平台不可重试错误都应 SetError
	REDACTED)
REDACTED
REDACTED

// TestIsNonRetryableRefreshError 测试不可重试错误判断
func TestIsNonRetryableRefreshError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
REDACTED{
		{name: "nil_error", err: nil, expected: falseREDACTED,
		{name: "network_error", err: errors.New("network timeout"), expected: falseREDACTED,
		{name: "invalid_grant", err: errors.New("invalid_grant"), expected: trueREDACTED,
		{name: "invalid_client", err: errors.New("invalid_client"), expected: trueREDACTED,
		{name: "unauthorized_client", err: errors.New("unauthorized_client"), expected: trueREDACTED,
		{name: "access_denied", err: errors.New("access_denied"), expected: trueREDACTED,
		{name: "invalid_grant_with_desc", err: errors.New("Error: invalid_grant - token revoked"), expected: trueREDACTED,
		{name: "case_insensitive", err: errors.New("INVALID_GRANT"), expected: trueREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNonRetryableRefreshError(tt.err)
			require.Equal(t, tt.expected, result)
	REDACTED)
REDACTED
REDACTED
