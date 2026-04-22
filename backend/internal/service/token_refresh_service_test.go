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
	updateCalls            int
	fullUpdateCalls        int
	updateCredentialsCalls int
	setErrorCalls          int
	clearTempCalls         int
	setTempUnschedCalls    int
	lastAccount            *Account
	updateErr              error
}

func (r *tokenRefreshAccountRepo) Update(ctx context.Context, account *Account) error {
	r.updateCalls++
	r.fullUpdateCalls++
	r.lastAccount = account
	return r.updateErr
}

func (r *tokenRefreshAccountRepo) UpdateCredentials(ctx context.Context, id int64, credentials map[string]any) error {
	r.updateCalls++
	r.updateCredentialsCalls++
	if r.updateErr != nil {
		return r.updateErr
	}
	cloned := cloneCredentials(credentials)
	if r.accountsByID != nil {
		if acc, ok := r.accountsByID[id]; ok && acc != nil {
			acc.Credentials = cloned
			r.lastAccount = acc
			return nil
		}
	}
	r.lastAccount = &Account{ID: id, Credentials: cloned}
	return nil
}

func (r *tokenRefreshAccountRepo) SetError(ctx context.Context, id int64, errorMsg string) error {
	r.setErrorCalls++
	return nil
}

func (r *tokenRefreshAccountRepo) ClearTempUnschedulable(ctx context.Context, id int64) error {
	r.clearTempCalls++
	return nil
}

func (r *tokenRefreshAccountRepo) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	r.setTempUnschedCalls++
	return nil
}

type tokenCacheInvalidatorStub struct {
	calls int
	err   error
}

func (s *tokenCacheInvalidatorStub) InvalidateToken(ctx context.Context, account *Account) error {
	s.calls++
	return s.err
}

type tempUnschedCacheStub struct {
	deleteCalls int
}

func (s *tempUnschedCacheStub) SetTempUnsched(ctx context.Context, accountID int64, state *TempUnschedState) error {
	return nil
}

func (s *tempUnschedCacheStub) GetTempUnsched(ctx context.Context, accountID int64) (*TempUnschedState, error) {
	return nil, nil
}

func (s *tempUnschedCacheStub) DeleteTempUnsched(ctx context.Context, accountID int64) error {
	s.deleteCalls++
	return nil
}

type tokenRefresherStub struct {
	credentials map[string]any
	err         error
}

func (r *tokenRefresherStub) CanRefresh(account *Account) bool {
	return true
}

func (r *tokenRefresherStub) NeedsRefresh(account *Account, refreshWindowDuration time.Duration) bool {
	return true
}

func (r *tokenRefresherStub) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.credentials, nil
}

func (r *tokenRefresherStub) CacheKey(account *Account) string {
	return "test:stub:" + account.Platform
}

func TestTokenRefreshService_RefreshWithRetry_InvalidatesCache(t *testing.T) {
	repo := &tokenRefreshAccountRepo{}
	invalidator := &tokenCacheInvalidatorStub{}
	cfg := &config.Config{
		TokenRefresh: config.TokenRefreshConfig{
			MaxRetries:          1,
			RetryBackoffSeconds: 0,
		},
	}
	service := NewTokenRefreshService(repo, nil, nil, nil, nil, invalidator, nil, cfg, nil)
	account := &Account{
		ID:       5,
		Platform: PlatformGemini,
		Type:     AccountTypeOAuth,
	}
	refresher := &tokenRefresherStub{
		credentials: map[string]any{
			"access_token": "new-token",
		},
	}

	err := service.refreshWithRetry(context.Background(), account, refresher, refresher, time.Hour)
	require.NoError(t, err)
	require.Equal(t, 1, repo.updateCalls)
	require.Equal(t, 1, repo.updateCredentialsCalls)
	require.Equal(t, 0, repo.fullUpdateCalls)
	require.Equal(t, 1, invalidator.calls)
	require.Equal(t, "new-token", account.GetCredential("access_token"))
}

func TestTokenRefreshService_RefreshWithRetry_InvalidatorErrorIgnored(t *testing.T) {
	repo := &tokenRefreshAccountRepo{}
	invalidator := &tokenCacheInvalidatorStub{err: errors.New("invalidate failed")}
	cfg := &config.Config{
		TokenRefresh: config.TokenRefreshConfig{
			MaxRetries:          1,
			RetryBackoffSeconds: 0,
		},
	}
	service := NewTokenRefreshService(repo, nil, nil, nil, nil, invalidator, nil, cfg, nil)
	account := &Account{
		ID:       6,
		Platform: PlatformGemini,
		Type:     AccountTypeOAuth,
	}
	refresher := &tokenRefresherStub{
		credentials: map[string]any{
			"access_token": "token",
		},
	}

	err := service.refreshWithRetry(context.Background(), account, refresher, refresher, time.Hour)
	require.NoError(t, err)
	require.Equal(t, 1, repo.updateCalls)
	require.Equal(t, 1, invalidator.calls)
}

func TestTokenRefreshService_RefreshWithRetry_NilInvalidator(t *testing.T) {
	repo := &tokenRefreshAccountRepo{}
	cfg := &config.Config{
		TokenRefresh: config.TokenRefreshConfig{
			MaxRetries:          1,
			RetryBackoffSeconds: 0,
		},
	}
	service := NewTokenRefreshService(repo, nil, nil, nil, nil, nil, nil, cfg, nil)
	account := &Account{
		ID:       7,
		Platform: PlatformGemini,
		Type:     AccountTypeOAuth,
	}
	refresher := &tokenRefresherStub{
		credentials: map[string]any{
			"access_token": "token",
		},
	}

	err := service.refreshWithRetry(context.Background(), account, refresher, refresher, time.Hour)
	require.NoError(t, err)
	require.Equal(t, 1, repo.updateCalls)
}

// TestTokenRefreshService_RefreshWithRetry_Antigravity 测试 Antigravity 平台的缓存失效
func TestTokenRefreshService_RefreshWithRetry_Antigravity(t *testing.T) {
	repo := &tokenRefreshAccountRepo{}
	invalidator := &tokenCacheInvalidatorStub{}
	cfg := &config.Config{
		TokenRefresh: config.TokenRefreshConfig{
			MaxRetries:          1,
			RetryBackoffSeconds: 0,
		},
	}
	service := NewTokenRefreshService(repo, nil, nil, nil, nil, invalidator, nil, cfg, nil)
	account := &Account{
		ID:       8,
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
	}
	refresher := &tokenRefresherStub{
		credentials: map[string]any{
			"access_token": "ag-token",
		},
	}

	err := service.refreshWithRetry(context.Background(), account, refresher, refresher, time.Hour)
	require.NoError(t, err)
	require.Equal(t, 1, repo.updateCalls)
	require.Equal(t, 1, invalidator.calls) // Antigravity 也应触发缓存失效
}

// TestTokenRefreshService_RefreshWithRetry_NonOAuthAccount 测试非 OAuth 账号不触发缓存失效
func TestTokenRefreshService_RefreshWithRetry_NonOAuthAccount(t *testing.T) {
	repo := &tokenRefreshAccountRepo{}
	invalidator := &tokenCacheInvalidatorStub{}
	cfg := &config.Config{
		TokenRefresh: config.TokenRefreshConfig{
			MaxRetries:          1,
			RetryBackoffSeconds: 0,
		},
	}
	service := NewTokenRefreshService(repo, nil, nil, nil, nil, invalidator, nil, cfg, nil)
	account := &Account{
		ID:       9,
		Platform: PlatformGemini,
		Type:     AccountTypeAPIKey, // 非 OAuth
	}
	refresher := &tokenRefresherStub{
		credentials: map[string]any{
			"access_token": "token",
		},
	}

	err := service.refreshWithRetry(context.Background(), account, refresher, refresher, time.Hour)
	require.NoError(t, err)
	require.Equal(t, 1, repo.updateCalls)
	require.Equal(t, 0, invalidator.calls) // 非 OAuth 不触发缓存失效
}

// TestTokenRefreshService_RefreshWithRetry_OtherPlatformOAuth 测试所有 OAuth 平台都触发缓存失效
func TestTokenRefreshService_RefreshWithRetry_OtherPlatformOAuth(t *testing.T) {
	repo := &tokenRefreshAccountRepo{}
	invalidator := &tokenCacheInvalidatorStub{}
	cfg := &config.Config{
		TokenRefresh: config.TokenRefreshConfig{
			MaxRetries:          1,
			RetryBackoffSeconds: 0,
		},
	}
	service := NewTokenRefreshService(repo, nil, nil, nil, nil, invalidator, nil, cfg, nil)
	account := &Account{
		ID:       10,
		Platform: PlatformOpenAI, // OpenAI OAuth 账户
		Type:     AccountTypeOAuth,
	}
	refresher := &tokenRefresherStub{
		credentials: map[string]any{
			"access_token": "token",
		},
	}

	err := service.refreshWithRetry(context.Background(), account, refresher, refresher, time.Hour)
	require.NoError(t, err)
	require.Equal(t, 1, repo.updateCalls)
	require.Equal(t, 1, repo.updateCredentialsCalls)
	require.Equal(t, 1, invalidator.calls) // 所有 OAuth 账户刷新后触发缓存失效
}

func TestTokenRefreshService_RefreshWithRetry_UsesCredentialsUpdater(t *testing.T) {
	repo := &tokenRefreshAccountRepo{}
	cfg := &config.Config{
		TokenRefresh: config.TokenRefreshConfig{
			MaxRetries:          1,
			RetryBackoffSeconds: 0,
		},
	}
	service := NewTokenRefreshService(repo, nil, nil, nil, nil, nil, nil, cfg, nil)
	resetAt := time.Now().Add(30 * time.Minute)
	account := &Account{
		ID:               17,
		Platform:         PlatformOpenAI,
		Type:             AccountTypeOAuth,
		RateLimitResetAt: &resetAt,
		Credentials: map[string]any{
			"access_token": "old-token",
		},
	}
	refresher := &tokenRefresherStub{
		credentials: map[string]any{
			"access_token": "new-token",
		},
	}

	err := service.refreshWithRetry(context.Background(), account, refresher, refresher, time.Hour)
	require.NoError(t, err)
	require.Equal(t, 1, repo.updateCredentialsCalls)
	require.Equal(t, 0, repo.fullUpdateCalls)
	require.NotNil(t, account.RateLimitResetAt)
	require.WithinDuration(t, resetAt, *account.RateLimitResetAt, time.Second)
}

// TestTokenRefreshService_RefreshWithRetry_UpdateFailed 测试更新失败的情况
func TestTokenRefreshService_RefreshWithRetry_UpdateFailed(t *testing.T) {
	repo := &tokenRefreshAccountRepo{updateErr: errors.New("update failed")}
	invalidator := &tokenCacheInvalidatorStub{}
	cfg := &config.Config{
		TokenRefresh: config.TokenRefreshConfig{
			MaxRetries:          1,
			RetryBackoffSeconds: 0,
		},
	}
	service := NewTokenRefreshService(repo, nil, nil, nil, nil, invalidator, nil, cfg, nil)
	account := &Account{
		ID:       11,
		Platform: PlatformGemini,
		Type:     AccountTypeOAuth,
	}
	refresher := &tokenRefresherStub{
		credentials: map[string]any{
			"access_token": "token",
		},
	}

	err := service.refreshWithRetry(context.Background(), account, refresher, refresher, time.Hour)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to save credentials")
	require.Equal(t, 1, repo.updateCalls)
	require.Equal(t, 0, invalidator.calls) // 更新失败时不应触发缓存失效
}

// TestTokenRefreshService_RefreshWithRetry_RefreshFailed 测试可重试错误耗尽不标记 error
func TestTokenRefreshService_RefreshWithRetry_RefreshFailed(t *testing.T) {
	repo := &tokenRefreshAccountRepo{}
	invalidator := &tokenCacheInvalidatorStub{}
	cfg := &config.Config{
		TokenRefresh: config.TokenRefreshConfig{
			MaxRetries:          2,
			RetryBackoffSeconds: 0,
		},
	}
	service := NewTokenRefreshService(repo, nil, nil, nil, nil, invalidator, nil, cfg, nil)
	account := &Account{
		ID:       12,
		Platform: PlatformGemini,
		Type:     AccountTypeOAuth,
	}
	refresher := &tokenRefresherStub{
		err: errors.New("refresh failed"),
	}

	err := service.refreshWithRetry(context.Background(), account, refresher, refresher, time.Hour)
	require.Error(t, err)
	require.Equal(t, 0, repo.updateCalls)   // 刷新失败不应更新
	require.Equal(t, 0, invalidator.calls)  // 刷新失败不应触发缓存失效
	require.Equal(t, 0, repo.setErrorCalls) // 可重试错误耗尽不标记 error，下个周期继续重试
}

// TestTokenRefreshService_RefreshWithRetry_AntigravityRefreshFailed 测试 Antigravity 刷新失败不设置错误状态
func TestTokenRefreshService_RefreshWithRetry_AntigravityRefreshFailed(t *testing.T) {
	repo := &tokenRefreshAccountRepo{}
	invalidator := &tokenCacheInvalidatorStub{}
	cfg := &config.Config{
		TokenRefresh: config.TokenRefreshConfig{
			MaxRetries:          1,
			RetryBackoffSeconds: 0,
		},
	}
	service := NewTokenRefreshService(repo, nil, nil, nil, nil, invalidator, nil, cfg, nil)
	account := &Account{
		ID:       13,
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
	}
	refresher := &tokenRefresherStub{
		err: errors.New("network error"), // 可重试错误
	}

	err := service.refreshWithRetry(context.Background(), account, refresher, refresher, time.Hour)
	require.Error(t, err)
	require.Equal(t, 0, repo.updateCalls)
	require.Equal(t, 0, invalidator.calls)
	require.Equal(t, 0, repo.setErrorCalls) // Antigravity 可重试错误不设置错误状态
}

// TestTokenRefreshService_RefreshWithRetry_AntigravityNonRetryableError 测试 Antigravity 不可重试错误
func TestTokenRefreshService_RefreshWithRetry_AntigravityNonRetryableError(t *testing.T) {
	repo := &tokenRefreshAccountRepo{}
	invalidator := &tokenCacheInvalidatorStub{}
	cfg := &config.Config{
		TokenRefresh: config.TokenRefreshConfig{
			MaxRetries:          3,
			RetryBackoffSeconds: 0,
		},
	}
	service := NewTokenRefreshService(repo, nil, nil, nil, nil, invalidator, nil, cfg, nil)
	account := &Account{
		ID:       14,
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
	}
	refresher := &tokenRefresherStub{
		err: errors.New("invalid_grant: token revoked"), // 不可重试错误
	}

	err := service.refreshWithRetry(context.Background(), account, refresher, refresher, time.Hour)
	require.Error(t, err)
	require.Equal(t, 0, repo.updateCalls)
	require.Equal(t, 0, invalidator.calls)
	require.Equal(t, 1, repo.setErrorCalls) // 不可重试错误应设置错误状态
}

// TestTokenRefreshService_RefreshWithRetry_ClearsTempUnschedulable 测试刷新成功后清除临时不可调度（DB + Redis）
func TestTokenRefreshService_RefreshWithRetry_ClearsTempUnschedulable(t *testing.T) {
	repo := &tokenRefreshAccountRepo{}
	invalidator := &tokenCacheInvalidatorStub{}
	tempCache := &tempUnschedCacheStub{}
	cfg := &config.Config{
		TokenRefresh: config.TokenRefreshConfig{
			MaxRetries:          1,
			RetryBackoffSeconds: 0,
		},
	}
	service := NewTokenRefreshService(repo, nil, nil, nil, nil, invalidator, nil, cfg, tempCache)
	until := time.Now().Add(10 * time.Minute)
	account := &Account{
		ID:                     15,
		Platform:               PlatformGemini,
		Type:                   AccountTypeOAuth,
		TempUnschedulableUntil: &until,
	}
	refresher := &tokenRefresherStub{
		credentials: map[string]any{
			"access_token": "new-token",
		},
	}

	err := service.refreshWithRetry(context.Background(), account, refresher, refresher, time.Hour)
	require.NoError(t, err)
	require.Equal(t, 1, repo.updateCalls)
	require.Equal(t, 1, repo.clearTempCalls)   // DB 清除
	require.Equal(t, 1, tempCache.deleteCalls) // Redis 缓存也应清除
}

// TestTokenRefreshService_RefreshWithRetry_NonRetryableErrorAllPlatforms 测试所有平台不可重试错误都 SetError
func TestTokenRefreshService_RefreshWithRetry_NonRetryableErrorAllPlatforms(t *testing.T) {
	tests := []struct {
		name     string
		platform string
	}{
		{name: "gemini", platform: PlatformGemini},
		{name: "anthropic", platform: PlatformAnthropic},
		{name: "openai", platform: PlatformOpenAI},
		{name: "antigravity", platform: PlatformAntigravity},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &tokenRefreshAccountRepo{}
			invalidator := &tokenCacheInvalidatorStub{}
			cfg := &config.Config{
				TokenRefresh: config.TokenRefreshConfig{
					MaxRetries:          3,
					RetryBackoffSeconds: 0,
				},
			}
			service := NewTokenRefreshService(repo, nil, nil, nil, nil, invalidator, nil, cfg, nil)
			account := &Account{
				ID:       16,
				Platform: tt.platform,
				Type:     AccountTypeOAuth,
			}
			refresher := &tokenRefresherStub{
				err: errors.New("invalid_grant: token revoked"),
			}

			err := service.refreshWithRetry(context.Background(), account, refresher, refresher, time.Hour)
			require.Error(t, err)
			require.Equal(t, 1, repo.setErrorCalls) // 所有平台不可重试错误都应 SetError
		})
	}
}

func TestTokenRefreshService_RefreshWithRetry_NoRefreshTokenDoesNotTempUnschedule(t *testing.T) {
	repo := &tokenRefreshAccountRepo{}
	cfg := &config.Config{
		TokenRefresh: config.TokenRefreshConfig{
			MaxRetries:          2,
			RetryBackoffSeconds: 0,
		},
	}
	service := NewTokenRefreshService(repo, nil, nil, nil, nil, nil, nil, cfg, nil)
	account := &Account{
		ID:       18,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
	}
	refresher := &tokenRefresherStub{
		err: errors.New("no refresh token available"),
	}

	err := service.refreshWithRetry(context.Background(), account, refresher, refresher, time.Hour)
	require.Error(t, err)
	require.Equal(t, 0, repo.updateCalls)
	require.Equal(t, 0, repo.setTempUnschedCalls, "missing refresh token should not mark the account temp unschedulable")
	require.Equal(t, 1, repo.setErrorCalls, "missing refresh token should be treated as a non-retryable credential state")
}

// TestIsNonRetryableRefreshError 测试不可重试错误判断
func TestIsNonRetryableRefreshError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{name: "nil_error", err: nil, expected: false},
		{name: "network_error", err: errors.New("network timeout"), expected: false},
		{name: "invalid_grant", err: errors.New("invalid_grant"), expected: true},
		{name: "invalid_client", err: errors.New("invalid_client"), expected: true},
		{name: "unauthorized_client", err: errors.New("unauthorized_client"), expected: true},
		{name: "access_denied", err: errors.New("access_denied"), expected: true},
		{name: "no_refresh_token", err: errors.New("no refresh token available"), expected: true},
		{name: "invalid_grant_with_desc", err: errors.New("Error: invalid_grant - token revoked"), expected: true},
		{name: "case_insensitive", err: errors.New("INVALID_GRANT"), expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNonRetryableRefreshError(tt.err)
			require.Equal(t, tt.expected, result)
		})
	}
}

// ========== Path A (refreshAPI) 测试用例 ==========

// mockTokenCacheForRefreshAPI 用于 Path A 测试的 GeminiTokenCache mock
type mockTokenCacheForRefreshAPI struct {
	lockResult   bool
	lockErr      error
	releaseCalls int
}

func (m *mockTokenCacheForRefreshAPI) GetAccessToken(_ context.Context, _ string) (string, error) {
	return "", errors.New("not cached")
}

func (m *mockTokenCacheForRefreshAPI) SetAccessToken(_ context.Context, _ string, _ string, _ time.Duration) error {
	return nil
}

func (m *mockTokenCacheForRefreshAPI) DeleteAccessToken(_ context.Context, _ string) error {
	return nil
}

func (m *mockTokenCacheForRefreshAPI) AcquireRefreshLock(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return m.lockResult, m.lockErr
}

func (m *mockTokenCacheForRefreshAPI) ReleaseRefreshLock(_ context.Context, _ string) error {
	m.releaseCalls++
	return nil
}

// buildPathAService 构建注入了 refreshAPI 的 service（Path A 测试辅助）
func buildPathAService(repo *tokenRefreshAccountRepo, cache GeminiTokenCache, invalidator TokenCacheInvalidator) (*TokenRefreshService, *tokenRefresherStub) {
	cfg := &config.Config{
		TokenRefresh: config.TokenRefreshConfig{
			MaxRetries:          1,
			RetryBackoffSeconds: 0,
		},
	}
	service := NewTokenRefreshService(repo, nil, nil, nil, nil, invalidator, nil, cfg, nil)
	refreshAPI := NewOAuthRefreshAPI(repo, cache)
	service.SetRefreshAPI(refreshAPI)

	refresher := &tokenRefresherStub{
		credentials: map[string]any{
			"access_token": "refreshed-token",
		},
	}
	return service, refresher
}

// TestPathA_Success 统一 API 路径正常成功：刷新 + DB 更新 + postRefreshActions
func TestPathA_Success(t *testing.T) {
	account := &Account{
		ID:       100,
		Platform: PlatformGemini,
		Type:     AccountTypeOAuth,
	}
	repo := &tokenRefreshAccountRepo{}
	repo.accountsByID = map[int64]*Account{account.ID: account}
	invalidator := &tokenCacheInvalidatorStub{}
	cache := &mockTokenCacheForRefreshAPI{lockResult: true}

	service, refresher := buildPathAService(repo, cache, invalidator)

	err := service.refreshWithRetry(context.Background(), account, refresher, refresher, time.Hour)
	require.NoError(t, err)
	require.Equal(t, 1, repo.updateCalls)   // DB 更新被调用
	require.Equal(t, 1, invalidator.calls)  // 缓存失效被调用
	require.Equal(t, 1, cache.releaseCalls) // 锁被释放
}

// TestPathA_LockHeld 锁被其他 worker 持有 → 返回 errRefreshSkipped
func TestPathA_LockHeld(t *testing.T) {
	account := &Account{
		ID:       101,
		Platform: PlatformGemini,
		Type:     AccountTypeOAuth,
	}
	repo := &tokenRefreshAccountRepo{}
	invalidator := &tokenCacheInvalidatorStub{}
	cache := &mockTokenCacheForRefreshAPI{lockResult: false} // 锁获取失败（被占）

	service, refresher := buildPathAService(repo, cache, invalidator)

	err := service.refreshWithRetry(context.Background(), account, refresher, refresher, time.Hour)
	require.ErrorIs(t, err, errRefreshSkipped)
	require.Equal(t, 0, repo.updateCalls)  // 不应更新 DB
	require.Equal(t, 0, invalidator.calls) // 不应触发缓存失效
}

// TestPathA_AlreadyRefreshed 二次检查发现已被其他路径刷新 → 返回 errRefreshSkipped
func TestPathA_AlreadyRefreshed(t *testing.T) {
	// NeedsRefresh 返回 false → RefreshIfNeeded 返回 {Refreshed: false}
	account := &Account{
		ID:       102,
		Platform: PlatformGemini,
		Type:     AccountTypeOAuth,
	}
	repo := &tokenRefreshAccountRepo{}
	repo.accountsByID = map[int64]*Account{account.ID: account}
	invalidator := &tokenCacheInvalidatorStub{}
	cache := &mockTokenCacheForRefreshAPI{lockResult: true}

	service, _ := buildPathAService(repo, cache, invalidator)

	// 使用一个 NeedsRefresh 返回 false 的 stub
	noRefreshNeeded := &tokenRefresherStub{
		credentials: map[string]any{"access_token": "token"},
	}
	// 覆盖 NeedsRefresh 行为 — 我们需要一个新的 stub 类型
	alwaysFreshStub := &alwaysFreshRefresherStub{}

	err := service.refreshWithRetry(context.Background(), account, noRefreshNeeded, alwaysFreshStub, time.Hour)
	require.ErrorIs(t, err, errRefreshSkipped)
	require.Equal(t, 0, repo.updateCalls)
	require.Equal(t, 0, invalidator.calls)
}

// alwaysFreshRefresherStub 二次检查时认为不需要刷新（模拟已被其他路径刷新）
type alwaysFreshRefresherStub struct{}

func (r *alwaysFreshRefresherStub) CanRefresh(_ *Account) bool                    { return true }
func (r *alwaysFreshRefresherStub) NeedsRefresh(_ *Account, _ time.Duration) bool { return false }
func (r *alwaysFreshRefresherStub) Refresh(_ context.Context, _ *Account) (map[string]any, error) {
	return nil, errors.New("should not be called")
}
func (r *alwaysFreshRefresherStub) CacheKey(account *Account) string {
	return "test:fresh:" + account.Platform
}

// TestPathA_NonRetryableError 统一 API 路径返回不可重试错误 → SetError
func TestPathA_NonRetryableError(t *testing.T) {
	account := &Account{
		ID:       103,
		Platform: PlatformGemini,
		Type:     AccountTypeOAuth,
	}
	repo := &tokenRefreshAccountRepo{}
	repo.accountsByID = map[int64]*Account{account.ID: account}
	invalidator := &tokenCacheInvalidatorStub{}
	cache := &mockTokenCacheForRefreshAPI{lockResult: true}

	service, _ := buildPathAService(repo, cache, invalidator)

	refresher := &tokenRefresherStub{
		err: errors.New("invalid_grant: token revoked"),
	}

	err := service.refreshWithRetry(context.Background(), account, refresher, refresher, time.Hour)
	require.Error(t, err)
	require.Equal(t, 1, repo.setErrorCalls) // 应标记 error 状态
	require.Equal(t, 0, repo.updateCalls)   // 不应更新 credentials
	require.Equal(t, 0, invalidator.calls)  // 不应触发缓存失效
}

// TestPathA_RetryableErrorExhausted 统一 API 路径可重试错误耗尽 → 不标记 error
func TestPathA_RetryableErrorExhausted(t *testing.T) {
	account := &Account{
		ID:       104,
		Platform: PlatformGemini,
		Type:     AccountTypeOAuth,
	}
	repo := &tokenRefreshAccountRepo{}
	repo.accountsByID = map[int64]*Account{account.ID: account}
	invalidator := &tokenCacheInvalidatorStub{}
	cache := &mockTokenCacheForRefreshAPI{lockResult: true}

	cfg := &config.Config{
		TokenRefresh: config.TokenRefreshConfig{
			MaxRetries:          2,
			RetryBackoffSeconds: 0,
		},
	}
	service := NewTokenRefreshService(repo, nil, nil, nil, nil, invalidator, nil, cfg, nil)
	refreshAPI := NewOAuthRefreshAPI(repo, cache)
	service.SetRefreshAPI(refreshAPI)

	refresher := &tokenRefresherStub{
		err: errors.New("network timeout"),
	}

	err := service.refreshWithRetry(context.Background(), account, refresher, refresher, time.Hour)
	require.Error(t, err)
	require.Equal(t, 0, repo.setErrorCalls) // 可重试错误不标记 error
	require.Equal(t, 0, repo.updateCalls)   // 刷新失败不应更新
	require.Equal(t, 0, invalidator.calls)  // 不应触发缓存失效
}

// TestPathA_DBUpdateFailed 统一 API 路径 DB 更新失败 → 返回 error，不执行 postRefreshActions
func TestPathA_DBUpdateFailed(t *testing.T) {
	account := &Account{
		ID:       105,
		Platform: PlatformGemini,
		Type:     AccountTypeOAuth,
	}
	repo := &tokenRefreshAccountRepo{updateErr: errors.New("db connection lost")}
	repo.accountsByID = map[int64]*Account{account.ID: account}
	invalidator := &tokenCacheInvalidatorStub{}
	cache := &mockTokenCacheForRefreshAPI{lockResult: true}

	service, refresher := buildPathAService(repo, cache, invalidator)

	err := service.refreshWithRetry(context.Background(), account, refresher, refresher, time.Hour)
	require.Error(t, err)
	require.Contains(t, err.Error(), "DB update failed")
	require.Equal(t, 1, repo.updateCalls)  // DB 更新被尝试
	require.Equal(t, 0, invalidator.calls) // DB 失败时不应触发缓存失效
}
