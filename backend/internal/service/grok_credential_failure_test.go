//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type grokCredentialPersistingRepo struct {
	*tokenRefreshAccountRepo
REDACTED

func (r *grokCredentialPersistingRepo) SetError(ctx context.Context, id int64, message string) error {
	if err := ctx.Err(); err != nil {
		return err
REDACTED
	if err := r.tokenRefreshAccountRepo.SetError(ctx, id, message); err != nil {
		return err
REDACTED
	if account := r.accountsByID[id]; account != nil {
		account.Status = StatusError
		account.Schedulable = false
		account.ErrorMessage = message
REDACTED
	return nil
REDACTED

type grokCredentialProxyRepoStub struct {
	ProxyRepository
	proxy *Proxy
	err   error
REDACTED

func (r *grokCredentialProxyRepoStub) GetByID(context.Context, int64) (*Proxy, error) {
	return r.proxy, r.err
REDACTED

type grokCredentialBlockingRepo struct {
	*tokenRefreshAccountRepo
	setErrorStarted chan struct{REDACTED
	setTempStarted  chan struct{REDACTED
	onceError       sync.Once
	onceTemp        sync.Once
REDACTED

type grokCredentialCommitThenCancelRepo struct {
	*tokenRefreshAccountRepo
	returnErr error
REDACTED

type grokCredentialUncommittedDeadlineRepo struct {
	*tokenRefreshAccountRepo
REDACTED

func (r *grokCredentialUncommittedDeadlineRepo) SetGrokCredentialErrorIfMatch(
	context.Context,
	int64,
	GrokCredentialMutationSnapshot,
	string,
) (bool, error) {
	return false, context.DeadlineExceeded
REDACTED

func (r *grokCredentialUncommittedDeadlineRepo) SetGrokCredentialTempUnschedulableIfMatch(
	context.Context,
	int64,
	GrokCredentialMutationSnapshot,
	time.Time,
	string,
) (bool, error) {
	return false, context.DeadlineExceeded
REDACTED

func (r *grokCredentialCommitThenCancelRepo) SetGrokCredentialErrorIfMatch(
	ctx context.Context,
	id int64,
	_ GrokCredentialMutationSnapshot,
	reason string,
) (bool, error) {
	account := r.accountsByID[id]
	account.Status = StatusError
	account.Schedulable = false
	account.ErrorMessage = reason
	if r.returnErr != nil {
		return false, r.returnErr
REDACTED
	<-ctx.Done()
	return false, ctx.Err()
REDACTED

func (r *grokCredentialCommitThenCancelRepo) SetGrokCredentialTempUnschedulableIfMatch(
	ctx context.Context,
	id int64,
	_ GrokCredentialMutationSnapshot,
	until time.Time,
	reason string,
) (bool, error) {
	account := r.accountsByID[id]
	account.TempUnschedulableUntil = &until
	account.TempUnschedulableReason = reason
	if r.returnErr != nil {
		return false, r.returnErr
REDACTED
	<-ctx.Done()
	return false, ctx.Err()
REDACTED

func (r *grokCredentialBlockingRepo) SetError(ctx context.Context, _ int64, _ string) error {
	r.onceError.Do(func() { close(r.setErrorStarted) REDACTED)
	<-ctx.Done()
	return ctx.Err()
REDACTED

func (r *grokCredentialBlockingRepo) SetTempUnschedulable(ctx context.Context, _ int64, _ time.Time, _ string) error {
	r.onceTemp.Do(func() { close(r.setTempStarted) REDACTED)
	<-ctx.Done()
	return ctx.Err()
REDACTED

func (r *grokCredentialBlockingRepo) SetGrokCredentialErrorIfMatch(
	ctx context.Context,
	_ int64,
	_ GrokCredentialMutationSnapshot,
	_ string,
) (bool, error) {
	r.onceError.Do(func() { close(r.setErrorStarted) REDACTED)
	<-ctx.Done()
	return false, ctx.Err()
REDACTED

func (r *grokCredentialBlockingRepo) SetGrokCredentialTempUnschedulableIfMatch(
	ctx context.Context,
	_ int64,
	_ GrokCredentialMutationSnapshot,
	_ time.Time,
	_ string,
) (bool, error) {
	r.onceTemp.Do(func() { close(r.setTempStarted) REDACTED)
	<-ctx.Done()
	return false, ctx.Err()
REDACTED

type grokCredentialBlockingCache struct {
	GrokTokenCache
	deleteStarted chan struct{REDACTED
	releaseDelete chan struct{REDACTED
	once          sync.Once
	mu            sync.Mutex
	deleted       bool
REDACTED

type grokCredentialSequencedRepo struct {
	*tokenRefreshAccountRepo
	mu      sync.Mutex
	latest  *Account
	getCall int
REDACTED

type grokCredentialRereadFailureRepo struct {
	*tokenRefreshAccountRepo
	account *Account
	err     error
REDACTED

type grokCredentialCountingRefresher struct {
	refreshCalls int
REDACTED

func (r *grokCredentialCountingRefresher) CacheKey(account *Account) string {
	return GrokTokenCacheKey(account)
REDACTED

func (r *grokCredentialCountingRefresher) CanRefresh(*Account) bool { return true REDACTED

func (r *grokCredentialCountingRefresher) NeedsRefresh(*Account, time.Duration) bool { return true REDACTED

func (r *grokCredentialCountingRefresher) Refresh(context.Context, *Account) (map[string]any, error) {
	r.refreshCalls++
	return map[string]any{"access_token": "must-not-be-used"REDACTED, nil
REDACTED

func (r *grokCredentialRereadFailureRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, r.err
REDACTED

func (r *grokCredentialSequencedRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.getCall++
	if r.getCall > 1 && r.latest != nil {
		return r.latest, nil
REDACTED
	return r.tokenRefreshAccountRepo.GetByID(ctx, id)
REDACTED

func (c *grokCredentialBlockingCache) DeleteAccessToken(ctx context.Context, _ string) error {
	c.once.Do(func() { close(c.deleteStarted) REDACTED)
	select {
	case <-c.releaseDelete:
		c.mu.Lock()
		c.deleted = true
		c.mu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
REDACTED
REDACTED

func (c *grokCredentialBlockingCache) wasDeleted() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.deleted
REDACTED

func TestUpstreamFailoverErrorNextAccountActionPreservesLegacyRetry(t *testing.T) {
	t.Parallel()

	require.True(t, (&UpstreamFailoverError{REDACTED).ShouldRetryNextAccount())
	require.True(t, (&UpstreamFailoverError{NextAccountAction: NextAccountRetryREDACTED).ShouldRetryNextAccount())
	require.False(t, (&UpstreamFailoverError{NextAccountAction: NextAccountStopREDACTED).ShouldRetryNextAccount())
REDACTED

func TestGetRequestCredentialMapsPermanentGrokOAuthFailureAndRedactsSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := expiredGrokOAuthAccountForCredentialTest(701)
	repo := &tokenRefreshAccountRepo{REDACTED
	repo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
	cache := &grokTokenCacheForProviderTest{lockResult: trueREDACTED
	provider := NewGrokTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{
		err: infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_TOKEN_REFRESH_FAILED", "invalid_grant access_token=leaked-access refresh_token=leaked-refresh"),
REDACTED)
	svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: providerREDACTED
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	token, kind, err := svc.getRequestCredential(context.Background(), c, account)
REDACTED
	require.Empty(t, token)
	require.Empty(t, kind)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, GatewayFailureStageAccountAuth, failoverErr.Stage)
	require.Equal(t, GatewayFailureScopeAccount, failoverErr.Scope)
	require.Equal(t, GrokCredentialReasonRevoked, failoverErr.Reason)
	require.True(t, failoverErr.ShouldRetryNextAccount())
	require.Equal(t, 0, failoverErr.StatusCode)
	require.Equal(t, http.StatusServiceUnavailable, failoverErr.ClientStatusCode)
	require.NotContains(t, err.Error(), "leaked-access")
	require.NotContains(t, err.Error(), "leaked-refresh")

	require.Equal(t, 1, repo.setErrorCalls)
	require.Zero(t, repo.setTempUnschedCalls)
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.Equal(t, []string{GrokTokenCacheKey(account)REDACTED, cache.deletedKeys)
	require.NotContains(t, repo.lastErrorMessage, "leaked-access")
	require.NotContains(t, repo.lastErrorMessage, "leaked-refresh")

	rawEvents, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := rawEvents.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, events, 1)
	require.Equal(t, string(GatewayFailureStageAccountAuth), events[0].Stage)
	require.Equal(t, string(GatewayFailureScopeAccount), events[0].Scope)
	require.Equal(t, string(GrokCredentialReasonRevoked), events[0].Reason)
	require.Zero(t, events[0].UpstreamStatusCode)
	require.NotContains(t, events[0].Message, "leaked-access")
	require.NotContains(t, events[0].Message, "leaked-refresh")
REDACTED

func TestGetRequestCredentialPermanentMappingsPersistAndInvalidate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		prepare     func(*Account)
		refreshErr  error
		wantReason  GatewayFailureReason
		cachedToken string
REDACTED{
		{
			name: "missing refresh credential",
			prepare: func(account *Account) {
				delete(account.Credentials, "refresh_token")
		REDACTED,
			wantReason: GrokCredentialReasonMissing,
	REDACTED,
		{
			name: "missing access credential",
			prepare: func(account *Account) {
				delete(account.Credentials, "access_token")
				account.Credentials["expires_at"] = time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
		REDACTED,
			wantReason:  GrokCredentialReasonMissing,
			cachedToken: "stale-cached-access",
	REDACTED,
		{
			name:       "explicit entitlement action required",
			prepare:    func(*Account) {REDACTED,
			refreshErr: infraerrors.New(http.StatusForbidden, "GROK_OAUTH_ENTITLEMENT_DENIED", "access_denied"),
			wantReason: GrokCredentialReasonEntitlement,
	REDACTED,
REDACTED

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := expiredGrokOAuthAccountForCredentialTest(int64(720 + index))
			account.Status = StatusActive
			account.Schedulable = true
			tt.prepare(account)
			baseRepo := &tokenRefreshAccountRepo{REDACTED
			baseRepo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
			repo := &grokCredentialPersistingRepo{tokenRefreshAccountRepo: baseRepoREDACTED
			cache := &grokTokenCacheForProviderTest{lockResult: true, token: tt.cachedTokenREDACTED
			provider := NewGrokTokenProvider(repo, cache)
			provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{err: tt.refreshErrREDACTED)
			svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: providerREDACTED
			c, _ := gin.CreateTestContext(httptest.NewRecorder())

			_, _, err := svc.getRequestCredential(context.Background(), c, account)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, tt.wantReason, failoverErr.Reason)
			require.Equal(t, GatewayFailureScopeAccount, failoverErr.Scope)
			require.Equal(t, 1, baseRepo.setErrorCalls)
			require.Equal(t, StatusError, account.Status)
			require.False(t, account.Schedulable)
			require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
			require.Equal(t, []string{GrokTokenCacheKey(account)REDACTED, cache.deletedKeys)
	REDACTED)
REDACTED
REDACTED

func TestGetRequestCredentialMissingAccessNeverRefreshesAndPermanentlyFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name      string
		expiresAt *time.Time
REDACTED{
		{name: "expiry missing"REDACTED,
		{name: "expired", expiresAt: func() *time.Time { value := time.Now().Add(-time.Minute); return &value REDACTED()REDACTED,
		{name: "near expiry", expiresAt: func() *time.Time { value := time.Now().Add(30 * time.Minute); return &value REDACTED()REDACTED,
REDACTED

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := expiredGrokOAuthAccountForCredentialTest(int64(760 + index))
			account.Schedulable = true
			delete(account.Credentials, "access_token")
			if tt.expiresAt == nil {
				delete(account.Credentials, "expires_at")
		REDACTED else {
				account.Credentials["expires_at"] = tt.expiresAt.UTC().Format(time.RFC3339)
		REDACTED
			baseRepo := &tokenRefreshAccountRepo{REDACTED
			baseRepo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
			repo := &grokCredentialPersistingRepo{tokenRefreshAccountRepo: baseRepoREDACTED
			cache := &grokTokenCacheForProviderTest{lockResult: true, token: "stale-cache-must-not-win"REDACTED
			refresher := &grokCredentialCountingRefresher{REDACTED
			provider := NewGrokTokenProvider(repo, cache)
			provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), refresher)
			svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: providerREDACTED
			c, _ := gin.CreateTestContext(httptest.NewRecorder())

			token, kind, err := svc.getRequestCredential(context.Background(), c, account)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Empty(t, token)
			require.Empty(t, kind)
			require.Equal(t, GrokCredentialReasonMissing, failoverErr.Reason)
			require.Equal(t, NextAccountRetry, failoverErr.NextAccountAction)
			require.Zero(t, refresher.refreshCalls, "structurally missing access credentials must not reach the token endpoint")
			require.Equal(t, 1, baseRepo.setErrorCalls)
			require.Equal(t, StatusError, account.Status)
			require.False(t, account.Schedulable)
			require.Equal(t, []string{GrokTokenCacheKey(account)REDACTED, cache.deletedKeys)
			require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
	REDACTED)
REDACTED
REDACTED

func TestGetRequestCredentialWarmCachedAccessWithMissingRefreshPermanentlyFailsOver(t *testing.T) {
	account := expiredGrokOAuthAccountForCredentialTest(764)
	account.Credentials["access_token"] = "valid-access"
	account.Credentials["expires_at"] = time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	delete(account.Credentials, "refresh_token")
	baseRepo := &tokenRefreshAccountRepo{REDACTED
	baseRepo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
	repo := &grokCredentialPersistingRepo{tokenRefreshAccountRepo: baseRepoREDACTED
	cache := &grokTokenCacheForProviderTest{lockResult: true, token: "valid-access"REDACTED
	refresher := &grokCredentialCountingRefresher{REDACTED
	provider := NewGrokTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), refresher)
	svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: providerREDACTED
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	_, _, err := svc.getRequestCredential(context.Background(), c, account)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, GrokCredentialReasonMissing, failoverErr.Reason)
	require.Equal(t, NextAccountRetry, failoverErr.NextAccountAction)
	require.Zero(t, refresher.refreshCalls)
	require.Equal(t, 1, baseRepo.setErrorCalls)
	require.Equal(t, []string{GrokTokenCacheKey(account)REDACTED, cache.deletedKeys)
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED

func TestGetRequestCredentialMapsTransientAndProviderFailuresSeparately(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("account transient temporarily unschedules", func(t *testing.T) {
		account := expiredGrokOAuthAccountForCredentialTest(702)
		repo := &tokenRefreshAccountRepo{REDACTED
		repo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
		cache := &grokTokenCacheForProviderTest{lockResult: trueREDACTED
		provider := NewGrokTokenProvider(repo, cache)
		provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{err: errors.New("temporary refresh transport failure")REDACTED)
		svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: providerREDACTED
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		_, _, err := svc.getRequestCredential(context.Background(), c, account)
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, err, &failoverErr)
		require.Equal(t, GatewayFailureScopeAccount, failoverErr.Scope)
		require.Equal(t, GrokCredentialReasonRefreshTransient, failoverErr.Reason)
		require.True(t, failoverErr.ShouldRetryNextAccount())
		require.Zero(t, repo.setErrorCalls)
		require.Equal(t, 1, repo.setTempUnschedCalls)
		require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED)

	t.Run("shared provider configuration stops without mutation", func(t *testing.T) {
		account := expiredGrokOAuthAccountForCredentialTest(703)
		repo := &tokenRefreshAccountRepo{REDACTED
		repo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
		provider := NewGrokTokenProvider(repo, nil)
		svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: providerREDACTED
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		_, _, err := svc.getRequestCredential(context.Background(), c, account)
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, err, &failoverErr)
		require.Equal(t, GatewayFailureScopeProvider, failoverErr.Scope)
		require.Equal(t, NextAccountStop, failoverErr.NextAccountAction)
		require.Zero(t, repo.setErrorCalls)
		require.Zero(t, repo.setTempUnschedCalls)
		require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED)

	t.Run("account reread failures preserve shared versus missing-row scope", func(t *testing.T) {
		for _, tt := range []struct {
			name    string
			account *Account
			err     error
	REDACTED{
			{name: "repository error", err: errors.New("database temporarily unavailable")REDACTED,
			{name: "missing row"REDACTED,
	REDACTED {
			t.Run(tt.name, func(t *testing.T) {
				account := expiredGrokOAuthAccountForCredentialTest(712)
				baseRepo := &tokenRefreshAccountRepo{REDACTED
				baseRepo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
				repo := &grokCredentialRereadFailureRepo{tokenRefreshAccountRepo: baseRepo, account: tt.account, err: tt.errREDACTED
				cache := &grokTokenCacheForProviderTest{lockResult: trueREDACTED
				provider := NewGrokTokenProvider(repo, cache)
				provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), NewGrokTokenRefresher(nil))
				svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: providerREDACTED
				c, _ := gin.CreateTestContext(httptest.NewRecorder())

				_, _, err := svc.getRequestCredential(context.Background(), c, account)
				var failoverErr *UpstreamFailoverError
				require.ErrorAs(t, err, &failoverErr)
				if tt.err != nil {
					require.Equal(t, GatewayFailureScopeProvider, failoverErr.Scope)
					require.Equal(t, GrokCredentialReasonProviderDown, failoverErr.Reason)
					require.Equal(t, NextAccountStop, failoverErr.NextAccountAction)
			REDACTED else {
					require.Equal(t, GatewayFailureScopeAccount, failoverErr.Scope)
					require.Equal(t, GrokCredentialReasonAccountChanged, failoverErr.Reason)
					require.Equal(t, NextAccountRetry, failoverErr.NextAccountAction)
			REDACTED
				require.Zero(t, baseRepo.setErrorCalls)
				require.Zero(t, baseRepo.setTempUnschedCalls)
				require.Empty(t, cache.deletedKeys)
				require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
		REDACTED)
	REDACTED
REDACTED)

	t.Run("fresh account eligibility changes retry without mutating stale state", func(t *testing.T) {
		for _, tt := range []struct {
			name   string
			mutate func(*Account)
	REDACTED{
			{
				name: "account disabled",
				mutate: func(account *Account) {
					account.Status = StatusDisabled
			REDACTED,
		REDACTED,
			{
				name: "account converted",
				mutate: func(account *Account) {
					account.Type = AccountTypeUpstream
			REDACTED,
		REDACTED,
			{
				name: "account manually unschedulable",
				mutate: func(account *Account) {
					account.Schedulable = false
			REDACTED,
		REDACTED,
			{
				name: "account temporarily unschedulable",
				mutate: func(account *Account) {
					until := time.Now().Add(time.Minute)
					account.TempUnschedulableUntil = &until
			REDACTED,
		REDACTED,
	REDACTED {
			t.Run(tt.name, func(t *testing.T) {
				staleAccount := expiredGrokOAuthAccountForCredentialTest(713)
				freshAccount := *staleAccount
				freshAccount.Credentials = shallowCopyMap(staleAccount.Credentials)
				tt.mutate(&freshAccount)
				repo := &tokenRefreshAccountRepo{REDACTED
				repo.accountsByID = map[int64]*Account{staleAccount.ID: &freshAccountREDACTED
				cache := &grokTokenCacheForProviderTest{lockResult: trueREDACTED
				provider := NewGrokTokenProvider(repo, cache)
				provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), NewGrokTokenRefresher(nil))
				svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: providerREDACTED
				c, _ := gin.CreateTestContext(httptest.NewRecorder())

				_, _, err := svc.getRequestCredential(context.Background(), c, staleAccount)
				var failoverErr *UpstreamFailoverError
				require.ErrorAs(t, err, &failoverErr)
				require.Equal(t, GatewayFailureScopeAccount, failoverErr.Scope)
				require.Equal(t, GrokCredentialReasonAccountChanged, failoverErr.Reason)
				require.Equal(t, NextAccountRetry, failoverErr.NextAccountAction)
				require.Zero(t, repo.setErrorCalls)
				require.Zero(t, repo.setTempUnschedCalls)
				require.Empty(t, cache.deletedKeys)
				require.False(t, svc.isOpenAIAccountRuntimeBlocked(staleAccount))
		REDACTED)
	REDACTED
REDACTED)

	t.Run("fresh missing refresh credential permanently blocks the account", func(t *testing.T) {
		staleAccount := expiredGrokOAuthAccountForCredentialTest(714)
		staleAccount.Schedulable = true
		freshAccount := *staleAccount
		freshAccount.Credentials = shallowCopyMap(staleAccount.Credentials)
		delete(freshAccount.Credentials, "refresh_token")
		baseRepo := &tokenRefreshAccountRepo{REDACTED
		baseRepo.accountsByID = map[int64]*Account{staleAccount.ID: &freshAccountREDACTED
		repo := &grokCredentialPersistingRepo{tokenRefreshAccountRepo: baseRepoREDACTED
		cache := &grokTokenCacheForProviderTest{lockResult: trueREDACTED
		provider := NewGrokTokenProvider(repo, cache)
		provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), NewGrokTokenRefresher(nil))
		svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: providerREDACTED
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		_, _, err := svc.getRequestCredential(context.Background(), c, staleAccount)
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, err, &failoverErr)
		require.Equal(t, GatewayFailureScopeAccount, failoverErr.Scope)
		require.Equal(t, GrokCredentialReasonMissing, failoverErr.Reason)
		require.Equal(t, NextAccountRetry, failoverErr.NextAccountAction)
		require.Equal(t, 1, baseRepo.setErrorCalls)
		require.Zero(t, baseRepo.setTempUnschedCalls)
		require.Equal(t, StatusError, freshAccount.Status)
		require.False(t, freshAccount.Schedulable)
		require.Equal(t, []string{GrokTokenCacheKey(staleAccount)REDACTED, cache.deletedKeys)
		require.True(t, svc.isOpenAIAccountRuntimeBlocked(staleAccount))
REDACTED)

	t.Run("refresh added after locked structural failure wins conditional mutation", func(t *testing.T) {
		staleAccount := expiredGrokOAuthAccountForCredentialTest(717)
		freshAccount := *staleAccount
		freshAccount.Credentials = shallowCopyMap(staleAccount.Credentials)
		delete(freshAccount.Credentials, "refresh_token")
		repo := &tokenRefreshAccountRepo{REDACTED
		repo.accountsByID = map[int64]*Account{staleAccount.ID: &freshAccountREDACTED
		repo.beforeConditionalState = func() {
			repaired := freshAccount
			repaired.Credentials = shallowCopyMap(freshAccount.Credentials)
			repaired.Credentials["refresh_token"] = "repaired-refresh-token"
			repaired.Credentials["expires_at"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
			repo.accountsByID[staleAccount.ID] = &repaired
	REDACTED
		cache := &grokTokenCacheForProviderTest{lockResult: trueREDACTED
		provider := NewGrokTokenProvider(repo, cache)
		provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), NewGrokTokenRefresher(nil))
		svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: providerREDACTED
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		token, kind, err := svc.getRequestCredential(context.Background(), c, staleAccount)

	REDACTED
		require.Equal(t, "expired-access-token", token)
		require.Equal(t, "oauth", kind)
		require.Zero(t, repo.setErrorCalls)
		require.Zero(t, repo.setTempUnschedCalls)
		require.Empty(t, cache.deletedKeys)
		require.False(t, svc.isOpenAIAccountRuntimeBlocked(staleAccount))
REDACTED)

	t.Run("expiry-only repair wins full credential fingerprint CAS", func(t *testing.T) {
		account := expiredGrokOAuthAccountForCredentialTest(718)
		repo := &tokenRefreshAccountRepo{REDACTED
		repo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
		repo.beforeConditionalState = func() {
			repaired := *account
			repaired.Credentials = shallowCopyMap(account.Credentials)
			repaired.Credentials["expires_at"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
			repo.accountsByID[account.ID] = &repaired
	REDACTED
		cache := &grokTokenCacheForProviderTest{lockResult: trueREDACTED
		provider := NewGrokTokenProvider(repo, cache)
		provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{
			err: infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_TOKEN_REFRESH_FAILED", "invalid_grant"),
	REDACTED)
		svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: providerREDACTED
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		token, kind, err := svc.getRequestCredential(context.Background(), c, account)

	REDACTED
		require.Equal(t, "expired-access-token", token)
		require.Equal(t, "oauth", kind)
		require.Zero(t, repo.setErrorCalls)
		require.Empty(t, cache.deletedKeys)
		require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED)

	t.Run("generic token endpoint 403 stops as shared provider failure", func(t *testing.T) {
		account := expiredGrokOAuthAccountForCredentialTest(708)
		repo := &tokenRefreshAccountRepo{REDACTED
		repo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
		cache := &grokTokenCacheForProviderTest{lockResult: trueREDACTED
		provider := NewGrokTokenProvider(repo, cache)
		provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{
			err: infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_TOKEN_REFRESH_FAILED", "token refresh failed: status 403, body: forbidden"),
	REDACTED)
		svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: providerREDACTED
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		_, _, err := svc.getRequestCredential(context.Background(), c, account)
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, err, &failoverErr)
		require.Equal(t, GatewayFailureScopeProvider, failoverErr.Scope)
		require.Equal(t, GrokCredentialReasonProviderDown, failoverErr.Reason)
		require.Equal(t, NextAccountStop, failoverErr.NextAccountAction)
		require.Zero(t, repo.setErrorCalls)
		require.Zero(t, repo.setTempUnschedCalls)
		require.Empty(t, cache.deletedKeys)
		require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED)

	t.Run("account proxy generic 403 remains bounded account transient", func(t *testing.T) {
		account := expiredGrokOAuthAccountForCredentialTest(711)
		proxyID := int64(43)
		account.ProxyID = &proxyID
		account.Proxy = &Proxy{REDACTED
		repo := &tokenRefreshAccountRepo{REDACTED
		repo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
		cache := &grokTokenCacheForProviderTest{lockResult: trueREDACTED
		provider := NewGrokTokenProvider(repo, cache)
		provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{
			err: infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_TOKEN_REFRESH_FAILED", "token refresh failed: status 403, body: forbidden"),
	REDACTED)
		svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: providerREDACTED
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		_, _, err := svc.getRequestCredential(context.Background(), c, account)
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, err, &failoverErr)
		require.Equal(t, GatewayFailureScopeAccount, failoverErr.Scope)
		require.Equal(t, GrokCredentialReasonRefreshTransient, failoverErr.Reason)
		require.Equal(t, NextAccountRetry, failoverErr.NextAccountAction)
		require.Zero(t, repo.setErrorCalls)
		require.Equal(t, 1, repo.setTempUnschedCalls)
		require.Empty(t, cache.deletedKeys)
		require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED)

	t.Run("proxy repository read failure stops without account mutation", func(t *testing.T) {
		account := expiredGrokOAuthAccountForCredentialTest(709)
		proxyID := int64(41)
		account.ProxyID = &proxyID
		account.Proxy = &Proxy{REDACTED
		repo := &tokenRefreshAccountRepo{REDACTED
		repo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
		cache := &grokTokenCacheForProviderTest{lockResult: trueREDACTED
		oauthSvc := NewGrokOAuthService(&grokCredentialProxyRepoStub{err: errors.New("database temporarily unavailable")REDACTED, &grokOAuthClientStub{REDACTED)
		defer oauthSvc.Stop()
		provider := NewGrokTokenProvider(repo, cache)
		provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), NewGrokTokenRefresher(oauthSvc))
		svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: providerREDACTED
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		_, _, err := svc.getRequestCredential(context.Background(), c, account)
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, err, &failoverErr)
		require.Equal(t, GatewayFailureScopeProvider, failoverErr.Scope)
		require.Equal(t, GrokCredentialReasonProviderDown, failoverErr.Reason)
		require.Equal(t, NextAccountStop, failoverErr.NextAccountAction)
		require.Zero(t, repo.setErrorCalls)
		require.Zero(t, repo.setTempUnschedCalls)
		require.Empty(t, cache.deletedKeys)
		require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED)

	t.Run("structurally missing configured proxy permanently blocks only that account", func(t *testing.T) {
		account := expiredGrokOAuthAccountForCredentialTest(710)
		account.Status = StatusActive
		account.Schedulable = true
		proxyID := int64(42)
		account.ProxyID = &proxyID
		baseRepo := &tokenRefreshAccountRepo{REDACTED
		baseRepo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
		repo := &grokCredentialPersistingRepo{tokenRefreshAccountRepo: baseRepoREDACTED
		cache := &grokTokenCacheForProviderTest{lockResult: trueREDACTED
		oauthSvc := NewGrokOAuthService(&grokCredentialProxyRepoStub{err: ErrProxyNotFoundREDACTED, &grokOAuthClientStub{REDACTED)
		defer oauthSvc.Stop()
		provider := NewGrokTokenProvider(repo, cache)
		provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), NewGrokTokenRefresher(oauthSvc))
		svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: providerREDACTED
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		_, _, err := svc.getRequestCredential(context.Background(), c, account)
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, err, &failoverErr)
		require.Equal(t, GatewayFailureScopeAccount, failoverErr.Scope)
		require.Equal(t, GrokCredentialReasonProxyInvalid, failoverErr.Reason)
		require.Equal(t, NextAccountRetry, failoverErr.NextAccountAction)
		require.Equal(t, 1, baseRepo.setErrorCalls)
		require.Equal(t, StatusError, account.Status)
		require.False(t, account.Schedulable)
		require.Equal(t, []string{GrokTokenCacheKey(account)REDACTED, cache.deletedKeys)
		require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED)
REDACTED

func TestGetRequestCredentialRuntimeBlockWinsBeforeWarmTokenCache(t *testing.T) {
	account := expiredGrokOAuthAccountForCredentialTest(716)
	account.Credentials["access_token"] = "valid-access"
	account.Credentials["expires_at"] = time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	repo := &tokenRefreshAccountRepo{REDACTED
	repo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
	cache := &grokTokenCacheForProviderTest{token: "valid-access"REDACTED
	provider := NewGrokTokenProvider(repo, cache)
	svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: providerREDACTED
	svc.BlockAccountScheduling(account, time.Now().Add(time.Minute), "independent")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	_, _, err := svc.getRequestCredential(context.Background(), c, account)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, GrokCredentialReasonAccountChanged, failoverErr.Reason)
	require.Equal(t, NextAccountRetry, failoverErr.NextAccountAction)
	require.Zero(t, cache.getCalls)
	require.Zero(t, repo.setErrorCalls)
	require.Zero(t, repo.setTempUnschedCalls)
REDACTED

func TestGetRequestCredentialWarmCachedAccessWithMissingConfiguredProxyPermanentlyFailsOver(t *testing.T) {
	account := expiredGrokOAuthAccountForCredentialTest(715)
	account.Credentials["access_token"] = "valid-access"
	account.Credentials["expires_at"] = time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	proxyID := int64(44)
	account.ProxyID = &proxyID
	account.Proxy = nil
	baseRepo := &tokenRefreshAccountRepo{REDACTED
	baseRepo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
	repo := &grokCredentialPersistingRepo{tokenRefreshAccountRepo: baseRepoREDACTED
	cache := &grokTokenCacheForProviderTest{lockResult: true, token: "valid-access"REDACTED
	refresher := &grokCredentialCountingRefresher{REDACTED
	provider := NewGrokTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), refresher)
	svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: providerREDACTED
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	_, _, err := svc.getRequestCredential(context.Background(), c, account)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, GrokCredentialReasonProxyInvalid, failoverErr.Reason)
	require.Equal(t, NextAccountRetry, failoverErr.NextAccountAction)
	require.Zero(t, refresher.refreshCalls)
	require.Equal(t, 1, baseRepo.setErrorCalls)
	require.Equal(t, []string{GrokTokenCacheKey(account)REDACTED, cache.deletedKeys)
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED

func TestGetRequestCredentialCancellationAndBudgetDoNotMutateAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := expiredGrokOAuthAccountForCredentialTest(704)
	repo := &tokenRefreshAccountRepo{REDACTED
	repo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
	provider := NewGrokTokenProvider(repo, nil)
	svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: providerREDACTED

	t.Run("parent cancellation is returned directly", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		_, _, err := svc.getRequestCredential(ctx, c, account)
		require.ErrorIs(t, err, context.Canceled)
		var failoverErr *UpstreamFailoverError
		require.False(t, errors.As(err, &failoverErr))
REDACTED)

	t.Run("request credential budget stops safely", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set(grokCredentialFailoverDeadlineKey, time.Now().Add(-time.Second))

		_, _, err := svc.getRequestCredential(context.Background(), c, account)
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, err, &failoverErr)
		require.Equal(t, GatewayFailureScopeRequest, failoverErr.Scope)
		require.Equal(t, GrokCredentialReasonFailoverTimeout, failoverErr.Reason)
		require.False(t, failoverErr.ShouldRetryNextAccount())
REDACTED)

	require.Zero(t, repo.setErrorCalls)
	require.Zero(t, repo.setTempUnschedCalls)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED

func TestGetRequestCredentialStateMutationFailureStopsAndKeepsRuntimeBlock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name            string
		refreshErr      error
		configure       func(*tokenRefreshAccountRepo, *grokTokenCacheForProviderTest)
		wantSetError    int
		wantSetTemp     int
		wantCacheDelete int
REDACTED{
		{
			name:       "permanent state persistence",
			refreshErr: infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_TOKEN_REFRESH_FAILED", "invalid_grant"),
			configure: func(repo *tokenRefreshAccountRepo, _ *grokTokenCacheForProviderTest) {
				repo.setErrorErr = errors.New("database write failed")
		REDACTED,
			wantSetError: 1,
	REDACTED,
		{
			name:       "transient state persistence",
			refreshErr: errors.New("temporary refresh transport failure"),
			configure: func(repo *tokenRefreshAccountRepo, _ *grokTokenCacheForProviderTest) {
				repo.setTempUnschedErr = errors.New("database write failed")
		REDACTED,
			wantSetTemp: 1,
	REDACTED,
		{
			name:       "permanent token cache invalidation",
			refreshErr: infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_TOKEN_REFRESH_FAILED", "invalid_grant"),
			configure: func(_ *tokenRefreshAccountRepo, cache *grokTokenCacheForProviderTest) {
				cache.deleteErr = errors.New("cache delete failed")
		REDACTED,
			wantSetError:    1,
			wantCacheDelete: 1,
	REDACTED,
REDACTED

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := expiredGrokOAuthAccountForCredentialTest(int64(740 + index))
			repo := &tokenRefreshAccountRepo{REDACTED
			repo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
			cache := &grokTokenCacheForProviderTest{lockResult: trueREDACTED
			tt.configure(repo, cache)
			provider := NewGrokTokenProvider(repo, cache)
			provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{err: tt.refreshErrREDACTED)
			svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: providerREDACTED
			c, _ := gin.CreateTestContext(httptest.NewRecorder())

			_, _, err := svc.getRequestCredential(context.Background(), c, account)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, GatewayFailureScopeProvider, failoverErr.Scope)
			require.Equal(t, GrokCredentialReasonStateUpdate, failoverErr.Reason)
			require.Equal(t, NextAccountStop, failoverErr.NextAccountAction)
			require.Equal(t, tt.wantSetError, repo.setErrorCalls)
			require.Equal(t, tt.wantSetTemp, repo.setTempUnschedCalls)
			require.Len(t, cache.deletedKeys, tt.wantCacheDelete)
			require.True(t, svc.isOpenAIAccountRuntimeBlocked(account), "failed mutation must retain the immediate local block")
	REDACTED)
REDACTED
REDACTED

func TestGrokCredentialMutationBoundariesHonorParentCancellation(t *testing.T) {
	t.Run("blocked SetError cancellation prevents cache and runtime mutation", func(t *testing.T) {
		account := expiredGrokOAuthAccountForCredentialTest(730)
		baseRepo := &tokenRefreshAccountRepo{REDACTED
		baseRepo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
		repo := &grokCredentialBlockingRepo{
			tokenRefreshAccountRepo: baseRepo,
			setErrorStarted:         make(chan struct{REDACTED),
			setTempStarted:          make(chan struct{REDACTED),
	REDACTED
		cache := &grokTokenCacheForProviderTest{REDACTED
		svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: NewGrokTokenProvider(repo, cache)REDACTED
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() {
			_, err := svc.applyGrokCredentialAccountFailure(ctx, account, grokCredentialFailureClass{
				reason: GrokCredentialReasonRevoked, permanent: true,
		REDACTED)
			result <- err
	REDACTED()
		<-repo.setErrorStarted
		require.True(t, svc.isOpenAIAccountRuntimeBlocked(account), "runtime block must precede persistent SetError")
		cancel()

		require.ErrorIs(t, <-result, context.Canceled)
		require.Zero(t, baseRepo.setErrorCalls)
		require.Empty(t, cache.deletedKeys)
		require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED)

	t.Run("blocked temporary unschedule cancellation prevents runtime mutation", func(t *testing.T) {
		account := expiredGrokOAuthAccountForCredentialTest(731)
		baseRepo := &tokenRefreshAccountRepo{REDACTED
		baseRepo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
		repo := &grokCredentialBlockingRepo{
			tokenRefreshAccountRepo: baseRepo,
			setErrorStarted:         make(chan struct{REDACTED),
			setTempStarted:          make(chan struct{REDACTED),
	REDACTED
		svc := &OpenAIGatewayService{accountRepo: repoREDACTED
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() {
			_, err := svc.applyGrokCredentialAccountFailure(ctx, account, grokCredentialFailureClass{
				reason: GrokCredentialReasonRefreshTransient, transient: true,
		REDACTED)
			result <- err
	REDACTED()
		<-repo.setTempStarted
		require.True(t, svc.isOpenAIAccountRuntimeBlocked(account), "runtime block must precede temporary unscheduling")
		cancel()

		require.ErrorIs(t, <-result, context.Canceled)
		require.Zero(t, baseRepo.setTempUnschedCalls)
		require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED)

	t.Run("post-commit cancellation finishes cache cleanup and retains quarantine", func(t *testing.T) {
		account := expiredGrokOAuthAccountForCredentialTest(732)
		repo := &tokenRefreshAccountRepo{REDACTED
		repo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
		cache := &grokCredentialBlockingCache{deleteStarted: make(chan struct{REDACTED), releaseDelete: make(chan struct{REDACTED)REDACTED
		svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: NewGrokTokenProvider(repo, cache)REDACTED
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() {
			_, err := svc.applyGrokCredentialAccountFailure(ctx, account, grokCredentialFailureClass{
				reason: GrokCredentialReasonRevoked, permanent: true,
		REDACTED)
			result <- err
	REDACTED()
		<-cache.deleteStarted
		require.True(t, svc.isOpenAIAccountRuntimeBlocked(account), "runtime block must precede cache invalidation")
		cancel()
		close(cache.releaseDelete)

		require.ErrorIs(t, <-result, context.Canceled)
		require.Equal(t, 1, repo.setErrorCalls)
		require.True(t, cache.wasDeleted())
		require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED)
REDACTED

func TestGrokCredentialMutationLockWaitHonorsCredentialBudget(t *testing.T) {
	account := expiredGrokOAuthAccountForCredentialTest(735)
	repo := &tokenRefreshAccountRepo{REDACTED
	repo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
	svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: NewGrokTokenProvider(repo, &grokTokenCacheForProviderTest{REDACTED)REDACTED
	mutationLock := svc.grokCredentialMutationLock(account.ID)
	require.NoError(t, mutationLock.Lock(context.Background()))
	defer mutationLock.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	startedAt := time.Now()
	token, err := svc.applyGrokCredentialAccountFailure(ctx, account, grokCredentialFailureClass{
		reason: GrokCredentialReasonRevoked, permanent: true,
REDACTED)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Empty(t, token)
	require.Less(t, time.Since(startedAt), 500*time.Millisecond)
	require.Zero(t, repo.setErrorCalls)
	require.Zero(t, repo.setTempUnschedCalls)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED

func TestGetRequestCredentialBudgetBoundsBlockedConditionalMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := expiredGrokOAuthAccountForCredentialTest(736)
	baseRepo := &tokenRefreshAccountRepo{REDACTED
	baseRepo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
	repo := &grokCredentialBlockingRepo{
		tokenRefreshAccountRepo: baseRepo,
		setErrorStarted:         make(chan struct{REDACTED),
		setTempStarted:          make(chan struct{REDACTED),
REDACTED
	cache := &grokTokenCacheForProviderTest{lockResult: trueREDACTED
	provider := NewGrokTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{
		err: infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_TOKEN_REFRESH_FAILED", "invalid_grant"),
REDACTED)
	svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: providerREDACTED
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(grokCredentialFailoverDeadlineKey, time.Now().Add(40*time.Millisecond))

	startedAt := time.Now()
	token, kind, err := svc.getRequestCredential(context.Background(), c, account)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, GatewayFailureScopeRequest, failoverErr.Scope)
	require.Equal(t, GrokCredentialReasonFailoverTimeout, failoverErr.Reason)
	require.Equal(t, NextAccountStop, failoverErr.NextAccountAction)
	require.Empty(t, token)
	require.Empty(t, kind)
	require.Less(t, time.Since(startedAt), 500*time.Millisecond)
	require.Zero(t, baseRepo.setErrorCalls)
	require.Zero(t, baseRepo.setTempUnschedCalls)
	require.Empty(t, cache.deletedKeys)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED

func TestGetRequestCredentialLockHeldTimeoutDoesNotQuarantineAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		buildRepo  func(*Account) AccountRepository
		wantScope  GatewayFailureScope
		wantReason GatewayFailureReason
		wantAction NextAccountAction
REDACTED{
		{
			name: "authoritative row unchanged",
			buildRepo: func(account *Account) AccountRepository {
				repo := &tokenRefreshAccountRepo{REDACTED
				repo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
				return repo
		REDACTED,
			wantScope:  GatewayFailureScopeAccount,
			wantReason: GrokCredentialReasonAccountChanged,
			wantAction: NextAccountRetry,
	REDACTED,
		{
			name: "selected account was deleted",
			buildRepo: func(account *Account) AccountRepository {
				base := &tokenRefreshAccountRepo{REDACTED
				base.accountsByID = map[int64]*Account{account.ID: accountREDACTED
				return &grokCredentialRereadFailureRepo{tokenRefreshAccountRepo: baseREDACTED
		REDACTED,
			wantScope:  GatewayFailureScopeAccount,
			wantReason: GrokCredentialReasonAccountChanged,
			wantAction: NextAccountRetry,
	REDACTED,
		{
			name: "shared account store unavailable",
			buildRepo: func(account *Account) AccountRepository {
				base := &tokenRefreshAccountRepo{REDACTED
				base.accountsByID = map[int64]*Account{account.ID: accountREDACTED
				return &grokCredentialRereadFailureRepo{tokenRefreshAccountRepo: base, err: errors.New("database unavailable")REDACTED
		REDACTED,
			wantScope:  GatewayFailureScopeProvider,
			wantReason: GrokCredentialReasonProviderDown,
			wantAction: NextAccountStop,
	REDACTED,
REDACTED

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := expiredGrokOAuthAccountForCredentialTest(int64(7400 + index))
			repo := tt.buildRepo(account)
			cache := &grokTokenCacheForProviderTest{lockResult: falseREDACTED
			provider := NewGrokTokenProvider(repo, cache)
			provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{REDACTED)
			svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: providerREDACTED
			c, _ := gin.CreateTestContext(httptest.NewRecorder())

			startedAt := time.Now()
			_, _, err := svc.getRequestCredential(context.Background(), c, account)

			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, tt.wantScope, failoverErr.Scope)
			require.Equal(t, tt.wantReason, failoverErr.Reason)
			require.Equal(t, tt.wantAction, failoverErr.NextAccountAction)
			require.Less(t, time.Since(startedAt), 3*time.Second)
			switch countingRepo := repo.(type) {
			case *tokenRefreshAccountRepo:
				require.Zero(t, countingRepo.setErrorCalls)
				require.Zero(t, countingRepo.setTempUnschedCalls)
			case *grokCredentialRereadFailureRepo:
				require.Zero(t, countingRepo.tokenRefreshAccountRepo.setErrorCalls)
				require.Zero(t, countingRepo.tokenRefreshAccountRepo.setTempUnschedCalls)
		REDACTED
			require.Empty(t, cache.deletedKeys)
			require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	REDACTED)
REDACTED
REDACTED

func TestGrokCredentialMutationCancellationAmbiguityConfirmsDurableCommit(t *testing.T) {
	tests := []struct {
		name      string
		class     grokCredentialFailureClass
		committed func(*Account) bool
REDACTED{
		{
			name:  "permanent quarantine",
			class: grokCredentialFailureClass{reason: GrokCredentialReasonRevoked, permanent: trueREDACTED,
			committed: func(account *Account) bool {
				return account.Status == StatusError && !account.Schedulable && account.ErrorMessage == string(GrokCredentialReasonRevoked)
		REDACTED,
	REDACTED,
		{
			name:  "temporary quarantine",
			class: grokCredentialFailureClass{reason: GrokCredentialReasonRefreshTransient, transient: trueREDACTED,
			committed: func(account *Account) bool {
				return account.TempUnschedulableUntil != nil && account.TempUnschedulableReason == string(GrokCredentialReasonRefreshTransient)
		REDACTED,
	REDACTED,
REDACTED

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := expiredGrokOAuthAccountForCredentialTest(int64(737 + index))
			baseRepo := &tokenRefreshAccountRepo{REDACTED
			baseRepo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
			repo := &grokCredentialCommitThenCancelRepo{tokenRefreshAccountRepo: baseRepoREDACTED
			cache := &grokTokenCacheForProviderTest{REDACTED
			svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: NewGrokTokenProvider(repo, cache)REDACTED
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()

			token, err := svc.applyGrokCredentialAccountFailure(ctx, account, tt.class)

			require.ErrorIs(t, err, context.DeadlineExceeded)
			require.Empty(t, token)
			require.True(t, tt.committed(account), "the detached confirmation must recognize the durable mutation")
			require.True(t, svc.isOpenAIAccountRuntimeBlocked(account), "a confirmed durable quarantine must retain its runtime block")
			if tt.class.permanent {
				require.Equal(t, []string{GrokTokenCacheKey(account)REDACTED, cache.deletedKeys)
		REDACTED else {
				require.Empty(t, cache.deletedKeys)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestGrokCredentialInnerStateDeadlineAmbiguityConfirmsDurableCommit(t *testing.T) {
	tests := []struct {
		name  string
		class grokCredentialFailureClass
REDACTED{
		{name: "permanent quarantine", class: grokCredentialFailureClass{reason: GrokCredentialReasonRevoked, permanent: trueREDACTEDREDACTED,
		{name: "temporary quarantine", class: grokCredentialFailureClass{reason: GrokCredentialReasonRefreshTransient, transient: trueREDACTEDREDACTED,
REDACTED

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := expiredGrokOAuthAccountForCredentialTest(int64(7500 + index))
			baseRepo := &tokenRefreshAccountRepo{REDACTED
			baseRepo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
			repo := &grokCredentialCommitThenCancelRepo{
				tokenRefreshAccountRepo: baseRepo,
				returnErr:               context.DeadlineExceeded,
		REDACTED
			cache := &grokTokenCacheForProviderTest{REDACTED
			svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: NewGrokTokenProvider(repo, cache)REDACTED

			token, err := svc.applyGrokCredentialAccountFailure(context.Background(), account, tt.class)

			require.NoError(t, err, "the detached readback must resolve the inner timeout's commit ambiguity")
			require.Empty(t, token)
			require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
			if tt.class.permanent {
				require.Equal(t, StatusError, account.Status)
				require.False(t, account.Schedulable)
				require.Equal(t, []string{GrokTokenCacheKey(account)REDACTED, cache.deletedKeys)
		REDACTED else {
				require.NotNil(t, account.TempUnschedulableUntil)
				require.Equal(t, string(GrokCredentialReasonRefreshTransient), account.TempUnschedulableReason)
				require.Empty(t, cache.deletedKeys)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestGrokCredentialUnconfirmedInnerStateDeadlineStopsAndRetainsSafetyBlock(t *testing.T) {
	tests := []struct {
		name  string
		class grokCredentialFailureClass
REDACTED{
		{name: "permanent quarantine", class: grokCredentialFailureClass{reason: GrokCredentialReasonRevoked, permanent: trueREDACTEDREDACTED,
		{name: "temporary quarantine", class: grokCredentialFailureClass{reason: GrokCredentialReasonRefreshTransient, transient: trueREDACTEDREDACTED,
REDACTED

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := expiredGrokOAuthAccountForCredentialTest(int64(7600 + index))
			baseRepo := &tokenRefreshAccountRepo{REDACTED
			baseRepo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
			repo := &grokCredentialUncommittedDeadlineRepo{tokenRefreshAccountRepo: baseRepoREDACTED
			cache := &grokTokenCacheForProviderTest{REDACTED
			svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: NewGrokTokenProvider(repo, cache)REDACTED

			token, err := svc.applyGrokCredentialAccountFailure(context.Background(), account, tt.class)

			require.ErrorIs(t, err, errGrokCredentialStateUpdateFailed)
			require.Empty(t, token)
			require.True(t, svc.isOpenAIAccountRuntimeBlocked(account), "an unknown commit outcome must retain the local safety block")
			require.Equal(t, StatusActive, account.Status)
			require.True(t, account.Schedulable)
			require.Nil(t, account.TempUnschedulableUntil)
			require.Empty(t, cache.deletedKeys)
	REDACTED)
REDACTED
REDACTED

func TestGrokCredentialRuntimeRollbackOwnership(t *testing.T) {
	account := expiredGrokOAuthAccountForCredentialTest(734)
	t.Run("later extending block survives", func(t *testing.T) {
		svc := &OpenAIGatewayService{REDACTED
		until := time.Now().Add(time.Minute)
		rollbackFirst := svc.blockGrokCredentialRuntime(account, until, "first")

		secondInstalled := make(chan struct{REDACTED)
		go func() {
			svc.BlockAccountScheduling(account, until.Add(time.Minute), "independent")
			close(secondInstalled)
	REDACTED()
		<-secondInstalled
		rollbackFirst()

		require.True(t, svc.isOpenAIAccountRuntimeBlocked(account),
			"rollback owned by the first invocation must not remove a later extending block")
REDACTED)

	t.Run("independent shorter block steals rollback ownership", func(t *testing.T) {
		svc := &OpenAIGatewayService{REDACTED
		until := time.Now().Add(2 * time.Minute)
		rollbackFirst := svc.blockGrokCredentialRuntime(account, until, "first")
		svc.BlockAccountScheduling(account, until.Add(-time.Minute), "shorter-no-op")

		rollbackFirst()

		require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED)

	t.Run("serialized tentative rollbacks leave no block", func(t *testing.T) {
		svc := &OpenAIGatewayService{REDACTED
		for i := 0; i < 2; i++ {
			mu := svc.grokCredentialMutationLock(account.ID)
			require.NoError(t, mu.Lock(context.Background()))
			rollback := svc.blockGrokCredentialRuntime(account, time.Now().Add(time.Minute), "tentative")
			rollback()
			mu.Unlock()
	REDACTED
		require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED)
REDACTED

func TestGetRequestCredentialAPIKeyBypassesOAuthFailureMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{
		ID:       705,
		Platform: PlatformGrok,
		Type:     AccountTypeAPIKey,
REDACTED
			"api_key":  "third-party-key",
			"base_url": "https://grok.example.test/v1",
	REDACTED,
REDACTED
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	svc := &OpenAIGatewayService{REDACTED

	token, kind, err := svc.getRequestCredential(context.Background(), c, account)
REDACTED
	require.Equal(t, "third-party-key", token)
	require.Equal(t, "apikey", kind)
	_, hasEvents := c.Get(OpsUpstreamErrorsKey)
	require.False(t, hasEvents)
REDACTED

func TestPermanentCredentialFailureDoesNotDisableConcurrentlyRefreshedAccount(t *testing.T) {
	account := expiredGrokOAuthAccountForCredentialTest(707)
	latest := *account
	latest.Credentials = shallowCopyMap(account.Credentials)
	latest.Credentials["access_token"] = "fresh-access-token"
	latest.Credentials["refresh_token"] = "rotated-refresh-token"
	latest.Credentials["expires_at"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	latest.Credentials["_token_version"] = time.Now().UnixMilli()
	repo := &tokenRefreshAccountRepo{REDACTED
	repo.accountsByID = map[int64]*Account{account.ID: &latestREDACTED
	cache := &grokTokenCacheForProviderTest{REDACTED
	svc := &OpenAIGatewayService{
		accountRepo:       repo,
		grokTokenProvider: NewGrokTokenProvider(repo, cache),
REDACTED

	_, mutationErr := svc.applyGrokCredentialAccountFailure(context.Background(), account, grokCredentialFailureClass{
		scope:     GatewayFailureScopeAccount,
		reason:    GrokCredentialReasonRevoked,
		action:    NextAccountRetry,
		permanent: true,
REDACTED)

	require.NoError(t, mutationErr)
	require.Zero(t, repo.setErrorCalls)
	require.Empty(t, cache.deletedKeys)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED

func TestCredentialFailureConditionalMutationLosesToConcurrentRefresh(t *testing.T) {
	for index, tt := range []struct {
		name       string
		refreshErr error
REDACTED{
		{name: "permanent", refreshErr: infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_TOKEN_REFRESH_FAILED", "invalid_grant")REDACTED,
		{name: "transient", refreshErr: errors.New("temporary refresh transport failure")REDACTED,
REDACTED {
		t.Run(tt.name, func(t *testing.T) {
			account := expiredGrokOAuthAccountForCredentialTest(int64(770 + index))
			repo := &tokenRefreshAccountRepo{REDACTED
			repo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
			repo.beforeConditionalState = func() {
				fresh := *account
				fresh.Credentials = shallowCopyMap(account.Credentials)
				fresh.Credentials["access_token"] = "refresh-won-token"
				fresh.Credentials["refresh_token"] = "refresh-won-refresh"
				fresh.Credentials["expires_at"] = time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
				fresh.Credentials["_token_version"] = time.Now().UnixMilli()
				repo.accountsByID[account.ID] = &fresh
		REDACTED
			cache := &grokTokenCacheForProviderTest{lockResult: trueREDACTED
			provider := NewGrokTokenProvider(repo, cache)
			provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{err: tt.refreshErrREDACTED)
			svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: providerREDACTED
			c, _ := gin.CreateTestContext(httptest.NewRecorder())

			token, kind, err := svc.getRequestCredential(context.Background(), c, account)

		REDACTED
			require.Equal(t, "refresh-won-token", token)
			require.Equal(t, "oauth", kind)
			require.Zero(t, repo.setErrorCalls)
			require.Zero(t, repo.setTempUnschedCalls)
			require.Empty(t, cache.deletedKeys)
			require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	REDACTED)
REDACTED
REDACTED

func TestCredentialFailureConditionalMutationLosesToConcurrentProxyRepair(t *testing.T) {
	for index, tt := range []struct {
		name       string
		refreshErr error
REDACTED{
		{name: "permanent", refreshErr: infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_TOKEN_REFRESH_FAILED", "invalid_grant")REDACTED,
		{name: "transient", refreshErr: errors.New("temporary refresh transport failure")REDACTED,
REDACTED {
		t.Run(tt.name, func(t *testing.T) {
			account := expiredGrokOAuthAccountForCredentialTest(int64(780 + index))
			oldProxyID := int64(10)
			account.ProxyID = &oldProxyID
			account.Proxy = &Proxy{REDACTED
			repo := &tokenRefreshAccountRepo{REDACTED
			repo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
			repo.beforeConditionalState = func() {
				fresh := *account
				fresh.Credentials = shallowCopyMap(account.Credentials)
				repairedProxyID := int64(11)
				fresh.ProxyID = &repairedProxyID
				repo.accountsByID[account.ID] = &fresh
		REDACTED
			cache := &grokTokenCacheForProviderTest{lockResult: trueREDACTED
			provider := NewGrokTokenProvider(repo, cache)
			provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{err: tt.refreshErrREDACTED)
			svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: providerREDACTED
			c, _ := gin.CreateTestContext(httptest.NewRecorder())

			_, _, err := svc.getRequestCredential(context.Background(), c, account)

			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, GrokCredentialReasonAccountChanged, failoverErr.Reason)
			require.Equal(t, NextAccountRetry, failoverErr.NextAccountAction)
			require.Zero(t, repo.setErrorCalls)
			require.Zero(t, repo.setTempUnschedCalls)
			require.Empty(t, cache.deletedKeys)
			require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	REDACTED)
REDACTED
REDACTED

func TestCredentialFailureConditionalMutationLosesToSameIDProxyRestoration(t *testing.T) {
	account := expiredGrokOAuthAccountForCredentialTest(790)
	proxyID := int64(10)
	account.ProxyID = &proxyID
	account.Proxy = nil
	repo := &tokenRefreshAccountRepo{REDACTED
	repo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
	repo.beforeConditionalState = func() {
		account.Proxy = &Proxy{ID: proxyIDREDACTED
REDACTED
	cache := &grokTokenCacheForProviderTest{lockResult: trueREDACTED
	provider := NewGrokTokenProvider(repo, cache)
	svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: providerREDACTED
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	_, _, err := svc.getRequestCredential(context.Background(), c, account)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, GrokCredentialReasonAccountChanged, failoverErr.Reason)
	require.Equal(t, NextAccountRetry, failoverErr.NextAccountAction)
	require.Zero(t, repo.setErrorCalls)
	require.Zero(t, repo.setTempUnschedCalls)
	require.Empty(t, cache.deletedKeys)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED

func TestCredentialFailureConditionalMutationLosesToConcurrentUnschedulableState(t *testing.T) {
	future := time.Now().Add(time.Hour)
	states := []struct {
		name   string
		mutate func(*Account)
REDACTED{
		{name: "admin schedulable false", mutate: func(account *Account) { account.Schedulable = false REDACTEDREDACTED,
		{name: "temporary cooldown", mutate: func(account *Account) { account.TempUnschedulableUntil = &future REDACTEDREDACTED,
		{name: "rate limit cooldown", mutate: func(account *Account) { account.RateLimitResetAt = &future REDACTEDREDACTED,
		{name: "overload cooldown", mutate: func(account *Account) { account.OverloadUntil = &future REDACTEDREDACTED,
REDACTED
	classes := []struct {
		name  string
		class grokCredentialFailureClass
REDACTED{
		{name: "permanent", class: grokCredentialFailureClass{reason: GrokCredentialReasonRevoked, permanent: trueREDACTEDREDACTED,
		{name: "transient", class: grokCredentialFailureClass{reason: GrokCredentialReasonRefreshTransient, transient: trueREDACTEDREDACTED,
REDACTED

	for classIndex, classCase := range classes {
		for stateIndex, stateCase := range states {
			t.Run(classCase.name+"/"+stateCase.name, func(t *testing.T) {
				account := expiredGrokOAuthAccountForCredentialTest(int64(791 + classIndex*10 + stateIndex))
				repo := &tokenRefreshAccountRepo{REDACTED
				repo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
				repo.beforeConditionalState = func() { stateCase.mutate(account) REDACTED
				svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: NewGrokTokenProvider(repo, &grokTokenCacheForProviderTest{REDACTED)REDACTED

				token, err := svc.applyGrokCredentialAccountFailure(context.Background(), account, classCase.class)

				require.ErrorIs(t, err, errOAuthRefreshAccountStateChanged)
				require.Empty(t, token)
				require.Zero(t, repo.setErrorCalls)
				require.Zero(t, repo.setTempUnschedCalls)
				require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
		REDACTED)
	REDACTED
REDACTED
REDACTED

func TestCredentialFailureCASMissDoesNotRecoverIneligibleLatestCredential(t *testing.T) {
	future := time.Now().Add(time.Hour)
	states := []struct {
		name               string
		mutate             func(*OpenAIGatewayService, *Account)
		wantRuntimeBlocked bool
REDACTED{
		{name: "disabled", mutate: func(_ *OpenAIGatewayService, account *Account) { account.Status = StatusDisabled REDACTEDREDACTED,
		{name: "not schedulable", mutate: func(_ *OpenAIGatewayService, account *Account) { account.Schedulable = false REDACTEDREDACTED,
		{name: "temporarily unschedulable", mutate: func(_ *OpenAIGatewayService, account *Account) { account.TempUnschedulableUntil = &future REDACTEDREDACTED,
		{name: "rate limited", mutate: func(_ *OpenAIGatewayService, account *Account) { account.RateLimitResetAt = &future REDACTEDREDACTED,
		{name: "overloaded", mutate: func(_ *OpenAIGatewayService, account *Account) { account.OverloadUntil = &future REDACTEDREDACTED,
		{
			name: "independently runtime blocked",
			mutate: func(svc *OpenAIGatewayService, account *Account) {
				svc.BlockAccountScheduling(account, time.Now().Add(24*time.Hour), "independent")
		REDACTED,
			wantRuntimeBlocked: true,
	REDACTED,
REDACTED
	classes := []struct {
		name  string
		class grokCredentialFailureClass
REDACTED{
		{name: "permanent", class: grokCredentialFailureClass{reason: GrokCredentialReasonRevoked, permanent: trueREDACTEDREDACTED,
		{name: "transient", class: grokCredentialFailureClass{reason: GrokCredentialReasonRefreshTransient, transient: trueREDACTEDREDACTED,
REDACTED

	for classIndex, classCase := range classes {
		for stateIndex, stateCase := range states {
			t.Run(classCase.name+"/"+stateCase.name, func(t *testing.T) {
				account := expiredGrokOAuthAccountForCredentialTest(int64(800 + classIndex*20 + stateIndex))
				repo := &tokenRefreshAccountRepo{REDACTED
				repo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
				svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: NewGrokTokenProvider(repo, &grokTokenCacheForProviderTest{REDACTED)REDACTED
				repo.beforeConditionalState = func() {
					latest := *account
					latest.Credentials = shallowCopyMap(account.Credentials)
					latest.Credentials["access_token"] = "fresh-but-ineligible-token"
					latest.Credentials["refresh_token"] = "fresh-but-ineligible-refresh"
					latest.Credentials["expires_at"] = time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
					latest.Credentials["_token_version"] = time.Now().UnixMilli()
					stateCase.mutate(svc, &latest)
					repo.accountsByID[account.ID] = &latest
			REDACTED

				token, err := svc.applyGrokCredentialAccountFailure(context.Background(), account, classCase.class)

				require.ErrorIs(t, err, errOAuthRefreshAccountStateChanged)
				require.Empty(t, token)
				require.Zero(t, repo.setErrorCalls)
				require.Zero(t, repo.setTempUnschedCalls)
				require.Equal(t, stateCase.wantRuntimeBlocked, svc.isOpenAIAccountRuntimeBlocked(account))
		REDACTED)
	REDACTED
REDACTED
REDACTED

func TestGetRequestCredentialSharedCredentialPersistenceFailureStopsWithoutAccountMutation(t *testing.T) {
	account := expiredGrokOAuthAccountForCredentialTest(782)
	repo := &tokenRefreshAccountRepo{updateErr: errors.New("database unavailable")REDACTED
	repo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
	cache := &grokTokenCacheForProviderTest{lockResult: trueREDACTED
	provider := NewGrokTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{credentials: map[string]any{
		"access_token":  "new-access-token",
		"refresh_token": "new-refresh-token",
		"expires_at":    time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
REDACTEDREDACTED)
	svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: providerREDACTED
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	_, _, err := svc.getRequestCredential(context.Background(), c, account)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, GatewayFailureScopeProvider, failoverErr.Scope)
	require.Equal(t, GrokCredentialReasonProviderDown, failoverErr.Reason)
	require.Equal(t, NextAccountStop, failoverErr.NextAccountAction)
	require.Equal(t, 1, repo.updateCredentialsCalls)
	require.Zero(t, repo.setErrorCalls)
	require.Zero(t, repo.setTempUnschedCalls)
	require.Empty(t, cache.deletedKeys)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED

func TestGetRequestCredentialRecoversConcurrentRefreshWithoutFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := expiredGrokOAuthAccountForCredentialTest(733)
	latest := *account
	latest.Credentials = shallowCopyMap(account.Credentials)
	latest.Credentials["access_token"] = "fresh-concurrent-access"
	latest.Credentials["refresh_token"] = "fresh-concurrent-refresh"
	latest.Credentials["expires_at"] = time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	latest.Credentials["_token_version"] = time.Now().UnixMilli()
	baseRepo := &tokenRefreshAccountRepo{REDACTED
	baseRepo.accountsByID = map[int64]*Account{account.ID: accountREDACTED
	repo := &grokCredentialSequencedRepo{tokenRefreshAccountRepo: baseRepo, latest: &latestREDACTED
	cache := &grokTokenCacheForProviderTest{lockResult: trueREDACTED
	provider := NewGrokTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{
		err: infraerrors.New(http.StatusForbidden, "GROK_OAUTH_ENTITLEMENT_DENIED", "access_denied"),
REDACTED)
	svc := &OpenAIGatewayService{accountRepo: repo, grokTokenProvider: providerREDACTED
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	token, kind, err := svc.getRequestCredential(context.Background(), c, account)

REDACTED
	require.Equal(t, "fresh-concurrent-access", token)
	require.Equal(t, "oauth", kind)
	require.Zero(t, baseRepo.setErrorCalls)
	require.Zero(t, baseRepo.setTempUnschedCalls)
	require.Empty(t, cache.deletedKeys)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	_, hasEvents := c.Get(OpsUpstreamErrorsKey)
	require.False(t, hasEvents)
REDACTED

func expiredGrokOAuthAccountForCredentialTest(id int64) *Account {
REDACTED
		ID:          id,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
REDACTED
			"access_token":  "expired-access-token",
			"refresh_token": "refresh-token",
			"expires_at":    time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
			"base_url":      xai.DefaultCLIBaseURL,
	REDACTED,
REDACTED
REDACTED
