//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

type grokTokenCacheForProviderTest struct {
	token        string
	setKey       string
	setToken     string
	setTTL       time.Duration
	lockResult   bool
	releaseCalls int
	deletedKeys  []string
	deleteErr    error
	getCalls     int
	mu           sync.Mutex
REDACTED

type grokCredentialRaceRepo struct {
	*tokenRefreshAccountRepo
	mu sync.RWMutex
REDACTED

func (r *grokCredentialRaceRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tokenRefreshAccountRepo.GetByID(ctx, id)
REDACTED

func (r *grokCredentialRaceRepo) setAccount(account *Account) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.accountsByID[account.ID] = account
REDACTED

func (c *grokTokenCacheForProviderTest) GetAccessToken(context.Context, string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.getCalls++
	if c.token == "" {
		return "", errors.New("not cached")
REDACTED
	return c.token, nil
REDACTED

func (c *grokTokenCacheForProviderTest) SetAccessToken(_ context.Context, key string, token string, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.setKey = key
	c.setToken = token
	c.setTTL = ttl
	return nil
REDACTED

func (c *grokTokenCacheForProviderTest) DeleteAccessToken(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deletedKeys = append(c.deletedKeys, key)
	return c.deleteErr
REDACTED

func (c *grokTokenCacheForProviderTest) AcquireRefreshLock(context.Context, string, time.Duration) (bool, error) {
	return c.lockResult, nil
REDACTED

func (c *grokTokenCacheForProviderTest) ReleaseRefreshLock(context.Context, string) error {
	c.releaseCalls++
	return nil
REDACTED

func TestGrokTokenProviderRefreshesExpiredTokenOnRequestPath(t *testing.T) {
	t.Setenv(xai.EnvBaseURL, xai.DefaultCLIBaseURL)

	expiredAt := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	account := &Account{
		ID:          54,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
REDACTED
			"access_token":  "expired-access-token",
			"refresh_token": "refresh-token",
			"expires_at":    expiredAt,
			"base_url":      xai.DefaultCLIBaseURL,
			"client_id":     "client-id",
	REDACTED,
REDACTED
	repo := &tokenRefreshAccountRepo{REDACTED
	repo.accountsByID = map[int64]*Account{54: accountREDACTED
	cache := &grokTokenCacheForProviderTest{lockResult: trueREDACTED
	oauthSvc := NewGrokOAuthService(nil, &grokOAuthClientStub{
		refreshResponse: &xai.TokenResponse{
			AccessToken: "new-access-token",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
	REDACTED,
REDACTED)
	defer oauthSvc.Stop()

	provider := NewGrokTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), NewGrokTokenRefresher(oauthSvc))

	token, err := provider.GetAccessToken(context.Background(), account)
REDACTED
	require.Equal(t, "new-access-token", token)
	require.Equal(t, 1, repo.updateCredentialsCalls)
	require.Equal(t, "new-access-token", repo.accountsByID[54].GetGrokAccessToken())
	require.Equal(t, "refresh-token", repo.accountsByID[54].GetGrokRefreshToken())
	require.Equal(t, xai.DefaultCLIBaseURL, repo.accountsByID[54].GetGrokBaseURL())
	require.Equal(t, "grok:account:54", cache.setKey)
	require.Equal(t, "new-access-token", cache.setToken)
	require.Greater(t, cache.setTTL, time.Duration(0))
	require.Equal(t, 1, cache.releaseCalls)
REDACTED

func TestGrokTokenProviderRefreshFailureUnschedulesWithRedactedReason(t *testing.T) {
	expiredAt := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	account := &Account{
		ID:          55,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
REDACTED
			"access_token":  "expired-access-token",
			"refresh_token": "refresh-token",
			"expires_at":    expiredAt,
			"base_url":      xai.DefaultCLIBaseURL,
	REDACTED,
REDACTED
	repo := &tokenRefreshAccountRepo{REDACTED
	repo.accountsByID = map[int64]*Account{55: accountREDACTED
	cache := &grokTokenCacheForProviderTest{lockResult: trueREDACTED
	provider := NewGrokTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{
		err: errors.New("temporary refresh failure access_token=leaked-access refresh_token=leaked-refresh"),
REDACTED)

	token, err := provider.GetAccessToken(context.Background(), account)
REDACTED
	require.Empty(t, token)
	require.Equal(t, 0, repo.setTempUnschedCalls)
	require.Equal(t, 0, repo.setErrorCalls)
REDACTED

func TestGrokTokenProviderLockHeldWaitsForRefreshedCacheAndNeverUsesExpiredToken(t *testing.T) {
	account := expiredGrokOAuthAccountForCredentialTest(56)
	baseRepo := &tokenRefreshAccountRepo{REDACTED
	baseRepo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
	repo := &grokCredentialRaceRepo{tokenRefreshAccountRepo: baseRepoREDACTED
	cache := &grokTokenCacheForProviderTest{lockResult: false, token: "expired-access-token"REDACTED
	provider := NewGrokTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{REDACTED)

	go func() {
		time.Sleep(40 * time.Millisecond)
		refreshed := *account
		refreshed.Credentials = shallowCopyMap(account.Credentials)
		refreshed.Credentials["access_token"] = "refreshed-after-lock"
		refreshed.Credentials["expires_at"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
		refreshed.Credentials["_token_version"] = time.Now().UnixMilli()
		repo.setAccount(&refreshed)
		cache.mu.Lock()
		cache.token = "refreshed-after-lock"
		cache.mu.Unlock()
REDACTED()

	startedAt := time.Now()
	token, err := provider.GetAccessToken(context.Background(), account)
REDACTED
	require.Equal(t, "refreshed-after-lock", token)
	require.NotEqual(t, "expired-access-token", token)
	require.GreaterOrEqual(t, time.Since(startedAt), 25*time.Millisecond,
		"expired account metadata must prevent returning the old cached token")
REDACTED

func TestGrokTokenProviderLockHeldTimeoutDoesNotReturnExpiredToken(t *testing.T) {
	account := expiredGrokOAuthAccountForCredentialTest(57)
	repo := &tokenRefreshAccountRepo{REDACTED
	repo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
	cache := &grokTokenCacheForProviderTest{lockResult: falseREDACTED
	provider := NewGrokTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{REDACTED)
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	token, err := provider.GetAccessToken(ctx, account)
REDACTED
	require.Empty(t, token)
REDACTED

func TestGrokTokenProviderLockHeldRejectsChangedTokenWithoutExpiry(t *testing.T) {
	account := expiredGrokOAuthAccountForCredentialTest(58)
	baseRepo := &tokenRefreshAccountRepo{REDACTED
	baseRepo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
	repo := &grokCredentialRaceRepo{tokenRefreshAccountRepo: baseRepoREDACTED
	cache := &grokTokenCacheForProviderTest{lockResult: false, token: "expired-access-token"REDACTED
	provider := NewGrokTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{REDACTED)

	go func() {
		time.Sleep(30 * time.Millisecond)
		refreshed := *account
		refreshed.Credentials = shallowCopyMap(account.Credentials)
		refreshed.Credentials["access_token"] = "changed-without-expiry"
		delete(refreshed.Credentials, "expires_at")
		refreshed.Credentials["_token_version"] = time.Now().UnixMilli()
		repo.setAccount(&refreshed)
		cache.mu.Lock()
		cache.token = "changed-without-expiry"
		cache.mu.Unlock()
REDACTED()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	token, err := provider.GetAccessToken(ctx, account)

REDACTED
	require.Empty(t, token, "an unbounded credential must not win the lock-held race")
REDACTED

func TestGrokTokenProviderLockHeldUsesVersionedDBTokenAndRepairsStaleCache(t *testing.T) {
	account := expiredGrokOAuthAccountForCredentialTest(60)
	baseRepo := &tokenRefreshAccountRepo{REDACTED
	baseRepo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
	repo := &grokCredentialRaceRepo{tokenRefreshAccountRepo: baseRepoREDACTED
	cache := &grokTokenCacheForProviderTest{lockResult: false, token: "expired-access-token"REDACTED
	provider := NewGrokTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{REDACTED)

	go func() {
		time.Sleep(30 * time.Millisecond)
		refreshed := *account
		refreshed.Credentials = shallowCopyMap(account.Credentials)
		refreshed.Credentials["access_token"] = "db-authoritative-token"
		refreshed.Credentials["expires_at"] = time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
		refreshed.Credentials["_token_version"] = time.Now().UnixMilli()
		repo.setAccount(&refreshed)
REDACTED()

	token, err := provider.GetAccessToken(context.Background(), account)

REDACTED
	require.Equal(t, "db-authoritative-token", token)
	require.Equal(t, "db-authoritative-token", cache.setToken)
	require.Greater(t, cache.setTTL, time.Duration(0))
REDACTED

func TestGrokTokenProviderRejectsStaleDBTokenWithoutExpiry(t *testing.T) {
	expiresAt := time.Now().Add(2 * grokTokenRefreshSkew).UTC().Format(time.RFC3339)
	account := &Account{
		ID:          59,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
REDACTED
			"access_token":  "old-access-token",
			"refresh_token": "refresh-token",
			"expires_at":    expiresAt,
	REDACTED,
REDACTED
	latest := *account
	latest.Credentials = shallowCopyMap(account.Credentials)
	latest.Credentials["access_token"] = "new-access-token-without-expiry"
	latest.Credentials["_token_version"] = time.Now().UnixMilli()
	delete(latest.Credentials, "expires_at")
	repo := &tokenRefreshAccountRepo{REDACTED
	repo.accountsByID = map[int64]*Account{account.ID: &latestREDACTED
	cache := &grokTokenCacheForProviderTest{REDACTED
	provider := NewGrokTokenProvider(repo, cache)

	token, err := provider.GetAccessToken(context.Background(), account)

	require.ErrorIs(t, err, errGrokOAuthAccessTokenExpired)
	require.Empty(t, token)
REDACTED

// TestGrokTokenProviderManualTestBypassesSchedulingGate reproduces #4598:
// admins must be able to run "test connection" against accounts that the
// scheduler currently excludes (manual switch off, rate limited, overloaded,
// temporarily cooled down). The production request path keeps rejecting them.
func TestGrokTokenProviderManualTestBypassesSchedulingGate(t *testing.T) {
	future := time.Now().Add(time.Hour)
	tests := []struct {
		name   string
		mutate func(*Account)
REDACTED{
		{name: "not schedulable", mutate: func(account *Account) { account.Schedulable = false REDACTEDREDACTED,
		{name: "temporarily unschedulable", mutate: func(account *Account) { account.TempUnschedulableUntil = &future REDACTEDREDACTED,
		{name: "rate limited", mutate: func(account *Account) { account.RateLimitResetAt = &future REDACTEDREDACTED,
		{name: "overloaded", mutate: func(account *Account) { account.OverloadUntil = &future REDACTEDREDACTED,
		{name: "disabled by error", mutate: func(account *Account) { account.Status = StatusError REDACTEDREDACTED,
REDACTED

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := expiredGrokOAuthAccountForCredentialTest(int64(120 + index))
			account.Credentials["access_token"] = "still-valid-token"
			account.Credentials["expires_at"] = time.Now().Add(2 * grokTokenRefreshSkew).UTC().Format(time.RFC3339)
			tt.mutate(account)
			provider := NewGrokTokenProvider(&tokenRefreshAccountRepo{REDACTED, &grokTokenCacheForProviderTest{REDACTED)

			// Production request path keeps excluding this account.
			_, requestErr := provider.GetAccessToken(context.Background(), account)
			require.ErrorIs(t, requestErr, errOAuthRefreshAccountStateChanged)

			// Manual test path returns the valid credential for probing.
			token, err := provider.GetAccessTokenForManualTest(context.Background(), account)
		REDACTED
			require.Equal(t, "still-valid-token", token)
	REDACTED)
REDACTED
REDACTED

func TestGrokTokenProviderManualTestRefreshesExpiredTokenWhileUnschedulable(t *testing.T) {
	t.Setenv(xai.EnvBaseURL, xai.DefaultCLIBaseURL)

	expiredAt := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	account := &Account{
		ID:          130,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: false,
REDACTED
			"access_token":  "expired-access-token",
			"refresh_token": "refresh-token",
			"expires_at":    expiredAt,
			"base_url":      xai.DefaultCLIBaseURL,
			"client_id":     "client-id",
	REDACTED,
REDACTED
	repo := &tokenRefreshAccountRepo{REDACTED
	repo.accountsByID = map[int64]*Account{130: accountREDACTED
	cache := &grokTokenCacheForProviderTest{lockResult: trueREDACTED
	oauthSvc := NewGrokOAuthService(nil, &grokOAuthClientStub{
		refreshResponse: &xai.TokenResponse{
			AccessToken: "manual-test-refreshed-token",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
	REDACTED,
REDACTED)
	defer oauthSvc.Stop()

	provider := NewGrokTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), NewGrokTokenRefresher(oauthSvc))

	token, err := provider.GetAccessTokenForManualTest(context.Background(), account)
REDACTED
	require.Equal(t, "manual-test-refreshed-token", token)
	require.Equal(t, 1, repo.updateCredentialsCalls)
REDACTED

func TestGrokTokenProviderManualTestFallsBackToValidTokenOnRefreshFailure(t *testing.T) {
	account := &Account{
		ID:          131,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: false,
REDACTED
			"access_token":  "near-expiry-token",
			"refresh_token": "refresh-token",
			// Inside the refresh window but not expired yet.
			"expires_at": time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339),
	REDACTED,
REDACTED
	repo := &tokenRefreshAccountRepo{REDACTED
	repo.accountsByID = map[int64]*Account{131: accountREDACTED
	cache := &grokTokenCacheForProviderTest{lockResult: trueREDACTED
	provider := NewGrokTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{
		err: errors.New("upstream refresh unavailable"),
REDACTED)

	token, err := provider.GetAccessTokenForManualTest(context.Background(), account)
REDACTED
	require.Equal(t, "near-expiry-token", token)
REDACTED

func TestGrokTokenProviderManualTestReportsRefreshFailureWhenTokenExpired(t *testing.T) {
	account := expiredGrokOAuthAccountForCredentialTest(132)
	account.Schedulable = false
	repo := &tokenRefreshAccountRepo{REDACTED
	repo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
	cache := &grokTokenCacheForProviderTest{lockResult: trueREDACTED
	provider := NewGrokTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{
		err: errors.New("invalid_client: client credentials rejected"),
REDACTED)

	token, err := provider.GetAccessTokenForManualTest(context.Background(), account)
REDACTED
	require.Empty(t, token)
	require.Contains(t, err.Error(), "invalid_client")
REDACTED

func TestGrokTokenProviderManualTestLockHeldWithExpiredTokenReturnsSpecificError(t *testing.T) {
	account := expiredGrokOAuthAccountForCredentialTest(133)
	repo := &tokenRefreshAccountRepo{REDACTED
	repo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
	cache := &grokTokenCacheForProviderTest{lockResult: falseREDACTED
	provider := NewGrokTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{REDACTED)

	token, err := provider.GetAccessTokenForManualTest(context.Background(), account)
REDACTED
	require.Empty(t, token)
	require.Contains(t, err.Error(), "refresh is already in progress")
REDACTED

func TestGrokTokenProviderManualTestRequiresRefreshToken(t *testing.T) {
	account := expiredGrokOAuthAccountForCredentialTest(134)
	delete(account.Credentials, "refresh_token")
	provider := NewGrokTokenProvider(&tokenRefreshAccountRepo{REDACTED, &grokTokenCacheForProviderTest{REDACTED)

	token, err := provider.GetAccessTokenForManualTest(context.Background(), account)
	require.ErrorIs(t, err, errGrokOAuthRefreshTokenMissing)
	require.Empty(t, token)
REDACTED

func TestGrokTokenProviderRejectsIneligibleSelectedAccountBeforeWarmCache(t *testing.T) {
	future := time.Now().Add(time.Hour)
	tests := []struct {
		name   string
		mutate func(*Account)
REDACTED{
		{name: "disabled", mutate: func(account *Account) { account.Status = StatusDisabled REDACTEDREDACTED,
		{name: "not schedulable", mutate: func(account *Account) { account.Schedulable = false REDACTEDREDACTED,
		{name: "temporarily unschedulable", mutate: func(account *Account) { account.TempUnschedulableUntil = &future REDACTEDREDACTED,
		{name: "rate limited", mutate: func(account *Account) { account.RateLimitResetAt = &future REDACTEDREDACTED,
		{name: "overloaded", mutate: func(account *Account) { account.OverloadUntil = &future REDACTEDREDACTED,
REDACTED

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := expiredGrokOAuthAccountForCredentialTest(int64(90 + index))
			account.Credentials["access_token"] = "warm-cache-token"
			account.Credentials["expires_at"] = time.Now().Add(2 * grokTokenRefreshSkew).UTC().Format(time.RFC3339)
			tt.mutate(account)
			cache := &grokTokenCacheForProviderTest{token: "warm-cache-token"REDACTED
			provider := NewGrokTokenProvider(&tokenRefreshAccountRepo{REDACTED, cache)

			token, err := provider.GetAccessToken(context.Background(), account)

			require.ErrorIs(t, err, errOAuthRefreshAccountStateChanged)
			require.Empty(t, token)
			require.Zero(t, cache.getCalls, "an ineligible selected account must be rejected before cache lookup")
	REDACTED)
REDACTED
REDACTED
