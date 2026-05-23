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

type rateLimitClearRepoStub struct {
	mockAccountRepoForGemini
	getByIDAccount            *Account
	getByIDErr                error
	getByIDCalls              int
	clearErrorCalls           int
	clearRateLimitCalls       int
	clearAntigravityCalls     int
	clearModelRateLimitCalls  int
	clearTempUnschedCalls     int
	clearErrorErr             error
	clearRateLimitErr         error
	clearAntigravityErr       error
	clearModelRateLimitErr    error
	clearTempUnschedulableErr error
REDACTED

func (r *rateLimitClearRepoStub) GetByID(ctx context.Context, id int64) (*Account, error) {
	r.getByIDCalls++
	if r.getByIDErr != nil {
		return nil, r.getByIDErr
REDACTED
	return r.getByIDAccount, nil
REDACTED

func (r *rateLimitClearRepoStub) ClearError(ctx context.Context, id int64) error {
	r.clearErrorCalls++
	return r.clearErrorErr
REDACTED

func (r *rateLimitClearRepoStub) ClearRateLimit(ctx context.Context, id int64) error {
	r.clearRateLimitCalls++
	return r.clearRateLimitErr
REDACTED

func (r *rateLimitClearRepoStub) ClearAntigravityQuotaScopes(ctx context.Context, id int64) error {
	r.clearAntigravityCalls++
	return r.clearAntigravityErr
REDACTED

func (r *rateLimitClearRepoStub) ClearModelRateLimits(ctx context.Context, id int64) error {
	r.clearModelRateLimitCalls++
	return r.clearModelRateLimitErr
REDACTED

func (r *rateLimitClearRepoStub) ClearTempUnschedulable(ctx context.Context, id int64) error {
	r.clearTempUnschedCalls++
	return r.clearTempUnschedulableErr
REDACTED

type tempUnschedCacheRecorder struct {
	deletedIDs []int64
	deleteErr  error
REDACTED

type recoverTokenInvalidatorStub struct {
	accounts []*Account
	err      error
REDACTED

func (c *tempUnschedCacheRecorder) SetTempUnsched(ctx context.Context, accountID int64, state *TempUnschedState) error {
	return nil
REDACTED

func (c *tempUnschedCacheRecorder) GetTempUnsched(ctx context.Context, accountID int64) (*TempUnschedState, error) {
	return nil, nil
REDACTED

func (c *tempUnschedCacheRecorder) DeleteTempUnsched(ctx context.Context, accountID int64) error {
	c.deletedIDs = append(c.deletedIDs, accountID)
	return c.deleteErr
REDACTED

func (s *recoverTokenInvalidatorStub) InvalidateToken(ctx context.Context, account *Account) error {
	s.accounts = append(s.accounts, account)
	return s.err
REDACTED

func TestRateLimitService_ClearRateLimit_AlsoClearsTempUnschedulable(t *testing.T) {
	repo := &rateLimitClearRepoStub{REDACTED
	cache := &tempUnschedCacheRecorder{REDACTED
	svc := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, cache)

	err := svc.ClearRateLimit(context.Background(), 42)
REDACTED

	require.Equal(t, 1, repo.clearRateLimitCalls)
	require.Equal(t, 1, repo.clearAntigravityCalls)
	require.Equal(t, 1, repo.clearModelRateLimitCalls)
	require.Equal(t, 1, repo.clearTempUnschedCalls)
	require.Equal(t, []int64{42REDACTED, cache.deletedIDs)
REDACTED

func TestRateLimitService_ClearRateLimit_ClearTempUnschedulableFailed(t *testing.T) {
	repo := &rateLimitClearRepoStub{
		clearTempUnschedulableErr: errors.New("clear temp unsched failed"),
REDACTED
	cache := &tempUnschedCacheRecorder{REDACTED
	svc := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, cache)

	err := svc.ClearRateLimit(context.Background(), 7)
REDACTED

	require.Equal(t, 1, repo.clearTempUnschedCalls)
	require.Empty(t, cache.deletedIDs)
REDACTED

func TestRateLimitService_ClearRateLimit_ClearRateLimitFailed(t *testing.T) {
	repo := &rateLimitClearRepoStub{
		clearRateLimitErr: errors.New("clear rate limit failed"),
REDACTED
	cache := &tempUnschedCacheRecorder{REDACTED
	svc := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, cache)

	err := svc.ClearRateLimit(context.Background(), 11)
REDACTED

	require.Equal(t, 1, repo.clearRateLimitCalls)
	require.Equal(t, 0, repo.clearAntigravityCalls)
	require.Equal(t, 0, repo.clearModelRateLimitCalls)
	require.Equal(t, 0, repo.clearTempUnschedCalls)
	require.Empty(t, cache.deletedIDs)
REDACTED

func TestRateLimitService_ClearRateLimit_ClearAntigravityFailed(t *testing.T) {
	repo := &rateLimitClearRepoStub{
		clearAntigravityErr: errors.New("clear antigravity failed"),
REDACTED
	cache := &tempUnschedCacheRecorder{REDACTED
	svc := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, cache)

	err := svc.ClearRateLimit(context.Background(), 12)
REDACTED

	require.Equal(t, 1, repo.clearRateLimitCalls)
	require.Equal(t, 1, repo.clearAntigravityCalls)
	require.Equal(t, 0, repo.clearModelRateLimitCalls)
	require.Equal(t, 0, repo.clearTempUnschedCalls)
	require.Empty(t, cache.deletedIDs)
REDACTED

func TestRateLimitService_ClearRateLimit_ClearModelRateLimitsFailed(t *testing.T) {
	repo := &rateLimitClearRepoStub{
		clearModelRateLimitErr: errors.New("clear model rate limits failed"),
REDACTED
	cache := &tempUnschedCacheRecorder{REDACTED
	svc := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, cache)

	err := svc.ClearRateLimit(context.Background(), 13)
REDACTED

	require.Equal(t, 1, repo.clearRateLimitCalls)
	require.Equal(t, 1, repo.clearAntigravityCalls)
	require.Equal(t, 1, repo.clearModelRateLimitCalls)
	require.Equal(t, 0, repo.clearTempUnschedCalls)
	require.Empty(t, cache.deletedIDs)
REDACTED

func TestRateLimitService_ClearRateLimit_CacheDeleteFailedShouldNotFail(t *testing.T) {
	repo := &rateLimitClearRepoStub{REDACTED
	cache := &tempUnschedCacheRecorder{
		deleteErr: errors.New("cache delete failed"),
REDACTED
	svc := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, cache)

	err := svc.ClearRateLimit(context.Background(), 14)
REDACTED

	require.Equal(t, 1, repo.clearRateLimitCalls)
	require.Equal(t, 1, repo.clearAntigravityCalls)
	require.Equal(t, 1, repo.clearModelRateLimitCalls)
	require.Equal(t, 1, repo.clearTempUnschedCalls)
	require.Equal(t, []int64{14REDACTED, cache.deletedIDs)
REDACTED

func TestRateLimitService_ClearRateLimit_WithoutTempUnschedCache(t *testing.T) {
	repo := &rateLimitClearRepoStub{REDACTED
	svc := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)

	err := svc.ClearRateLimit(context.Background(), 15)
REDACTED

	require.Equal(t, 1, repo.clearRateLimitCalls)
	require.Equal(t, 1, repo.clearAntigravityCalls)
	require.Equal(t, 1, repo.clearModelRateLimitCalls)
	require.Equal(t, 1, repo.clearTempUnschedCalls)
REDACTED

func TestRateLimitService_RecoverAccountAfterSuccessfulTest_ClearsErrorAndRateLimitRelatedState(t *testing.T) {
	now := time.Now()
	repo := &rateLimitClearRepoStub{
		getByIDAccount: &Account{
			ID:                     42,
			Status:                 StatusError,
			RateLimitedAt:          &now,
			TempUnschedulableUntil: &now,
			Extra: map[string]any{
				"model_rate_limits": map[string]any{
					"claude-sonnet-4-5": map[string]any{
						"rate_limit_reset_at": now.Format(time.RFC3339),
				REDACTED,
			REDACTED,
				"antigravity_quota_scopes": map[string]any{"gemini": trueREDACTED,
		REDACTED,
	REDACTED,
REDACTED
	cache := &tempUnschedCacheRecorder{REDACTED
	blocker := &runtimeBlockRecorder{REDACTED
	svc := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, cache)
	svc.SetAccountRuntimeBlocker(blocker)

	result, err := svc.RecoverAccountAfterSuccessfulTest(context.Background(), 42)
REDACTED
	require.NotNil(t, result)
	require.True(t, result.ClearedError)
	require.True(t, result.ClearedRateLimit)

	require.Equal(t, 1, repo.getByIDCalls)
	require.Equal(t, 1, repo.clearErrorCalls)
	require.Equal(t, 1, repo.clearRateLimitCalls)
	require.Equal(t, 1, repo.clearAntigravityCalls)
	require.Equal(t, 1, repo.clearModelRateLimitCalls)
	require.Equal(t, 1, repo.clearTempUnschedCalls)
	require.Equal(t, []int64{42REDACTED, cache.deletedIDs)
	require.Equal(t, []int64{42REDACTED, blocker.clearedIDs)
REDACTED

func TestRateLimitService_RecoverAccountAfterSuccessfulTest_NoRecoverableStateIsNoop(t *testing.T) {
	repo := &rateLimitClearRepoStub{
		getByIDAccount: &Account{
			ID:          7,
			Status:      StatusActive,
			Schedulable: true,
			Extra:       map[string]any{REDACTED,
	REDACTED,
REDACTED
	cache := &tempUnschedCacheRecorder{REDACTED
	svc := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, cache)

	result, err := svc.RecoverAccountAfterSuccessfulTest(context.Background(), 7)
REDACTED
	require.NotNil(t, result)
	require.False(t, result.ClearedError)
	require.False(t, result.ClearedRateLimit)

	require.Equal(t, 1, repo.getByIDCalls)
	require.Equal(t, 0, repo.clearErrorCalls)
	require.Equal(t, 0, repo.clearRateLimitCalls)
	require.Equal(t, 0, repo.clearAntigravityCalls)
	require.Equal(t, 0, repo.clearModelRateLimitCalls)
	require.Equal(t, 0, repo.clearTempUnschedCalls)
	require.Empty(t, cache.deletedIDs)
REDACTED

func TestRateLimitService_RecoverAccountAfterSuccessfulTest_ClearErrorFailed(t *testing.T) {
	repo := &rateLimitClearRepoStub{
		getByIDAccount: &Account{
			ID:     9,
			Status: StatusError,
	REDACTED,
		clearErrorErr: errors.New("clear error failed"),
REDACTED
	svc := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)

	result, err := svc.RecoverAccountAfterSuccessfulTest(context.Background(), 9)
REDACTED
	require.Nil(t, result)
	require.Equal(t, 1, repo.getByIDCalls)
	require.Equal(t, 1, repo.clearErrorCalls)
	require.Equal(t, 0, repo.clearRateLimitCalls)
REDACTED

func TestRateLimitService_RecoverAccountState_InvalidatesOAuthTokenOnErrorRecovery(t *testing.T) {
	repo := &rateLimitClearRepoStub{
		getByIDAccount: &Account{
			ID:     21,
			Type:   AccountTypeOAuth,
			Status: StatusError,
	REDACTED,
REDACTED
	invalidator := &recoverTokenInvalidatorStub{REDACTED
	svc := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)
	svc.SetTokenCacheInvalidator(invalidator)

	result, err := svc.RecoverAccountState(context.Background(), 21, AccountRecoveryOptions{
		InvalidateToken: true,
REDACTED)
REDACTED
	require.NotNil(t, result)
	require.True(t, result.ClearedError)
	require.False(t, result.ClearedRateLimit)
	require.Equal(t, 1, repo.clearErrorCalls)
	require.Len(t, invalidator.accounts, 1)
	require.Equal(t, int64(21), invalidator.accounts[0].ID)
REDACTED
