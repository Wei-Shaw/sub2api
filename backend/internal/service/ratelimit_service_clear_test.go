//go:build unit

package service

import (
	"context"
	"encoding/json"
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
	getByIDErrAfter           int
	getByIDCalls              int
	getByIDHook               func()
	clearErrorCalls           int
	clearRateLimitCalls       int
	clearAntigravityCalls     int
	clearModelRateLimitCalls  int
	clearModelRateLimitKeys   []string
	recoverRuntimeCalls       int
	clearTempUnschedCalls     int
	clearErrorErr             error
	clearRateLimitErr         error
	clearAntigravityErr       error
	clearModelRateLimitErr    error
	clearTempUnschedulableErr error
}

func (r *rateLimitClearRepoStub) GetByID(ctx context.Context, id int64) (*Account, error) {
	r.getByIDCalls++
	if r.getByIDHook != nil {
		r.getByIDHook()
	}
	if r.getByIDErr != nil && (r.getByIDErrAfter == 0 || r.getByIDCalls > r.getByIDErrAfter) {
		return nil, r.getByIDErr
	}
	return r.getByIDAccount, nil
}

func (r *rateLimitClearRepoStub) ClearError(ctx context.Context, id int64) error {
	r.clearErrorCalls++
	return r.clearErrorErr
}

func (r *rateLimitClearRepoStub) ClearRateLimit(ctx context.Context, id int64) error {
	r.clearRateLimitCalls++
	return r.clearRateLimitErr
}

func (r *rateLimitClearRepoStub) ClearAntigravityQuotaScopes(ctx context.Context, id int64) error {
	r.clearAntigravityCalls++
	return r.clearAntigravityErr
}

func (r *rateLimitClearRepoStub) ClearModelRateLimits(ctx context.Context, id int64) error {
	r.clearModelRateLimitCalls++
	return r.clearModelRateLimitErr
}

func (r *rateLimitClearRepoStub) ClearModelRateLimit(ctx context.Context, id int64, modelID string) error {
	r.clearModelRateLimitKeys = append(r.clearModelRateLimitKeys, modelID)
	return r.clearModelRateLimitErr
}

func (r *rateLimitClearRepoStub) ClearModelRateLimitIfMatch(_ context.Context, _ int64, modelID string, observed json.RawMessage) (bool, error) {
	if r.clearModelRateLimitErr != nil || r.getByIDAccount == nil {
		return false, r.clearModelRateLimitErr
	}
	limits, _ := r.getByIDAccount.Extra[modelRateLimitsKey].(map[string]any)
	current, ok := limits[modelID]
	if !ok {
		return false, nil
	}
	encoded, err := json.Marshal(current)
	if err != nil || string(encoded) != string(observed) {
		return false, err
	}
	delete(limits, modelID)
	r.clearModelRateLimitKeys = append(r.clearModelRateLimitKeys, modelID)
	r.getByIDAccount.UpdatedAt = r.getByIDAccount.UpdatedAt.Add(time.Nanosecond)
	return true, nil
}

func (r *rateLimitClearRepoStub) RecoverAccountRuntimeStateIfUnchanged(_ context.Context, _ int64, observedUpdatedAt time.Time) (bool, error) {
	if r.getByIDAccount == nil || !r.getByIDAccount.UpdatedAt.Equal(observedUpdatedAt) {
		return false, nil
	}
	r.recoverRuntimeCalls++
	r.getByIDAccount.Status = StatusActive
	r.getByIDAccount.Schedulable = true
	r.getByIDAccount.UpdatedAt = r.getByIDAccount.UpdatedAt.Add(time.Nanosecond)
	return true, nil
}

func (r *rateLimitClearRepoStub) ClearTempUnschedulable(ctx context.Context, id int64) error {
	r.clearTempUnschedCalls++
	return r.clearTempUnschedulableErr
}

type tempUnschedCacheRecorder struct {
	deletedIDs []int64
	deleteErr  error
}

type recoverTokenInvalidatorStub struct {
	accounts []*Account
	err      error
}

func TestRateLimitService_RecoverErrorTargetClearsGatewayModelTransientState(t *testing.T) {
	limit := map[string]any{"rate_limit_reset_at": time.Now().Add(10 * time.Minute).Format(time.RFC3339)}
	repo := &rateLimitClearRepoStub{getByIDAccount: &Account{
		ID: 42, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
		Extra: map[string]any{modelRateLimitsKey: map[string]any{
			"gpt-5.6-sol": limit,
		}},
	}}
	state := newOpenAIAccountModelTransientState(1)
	now := time.Now()
	var initialClaim openAIAccountModelTransientDecision
	for i, requestID := range []string{"initial-1", "initial-2", "initial-3"} {
		initialClaim = state.recordFailure(42, "gpt-5.6-sol", now.Add(time.Duration(i)*time.Millisecond), requestID)
	}
	state.finishCircuitPersistence(42, "gpt-5.6-sol", initialClaim.PersistenceGeneration, true)
	gateway := &OpenAIGatewayService{openaiModelTransient: state}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	svc.SetAccountRuntimeBlocker(gateway)
	targets, err := svc.ErrorRecoveryTargets(context.Background(), 42, "")
	require.NoError(t, err)
	require.Len(t, targets, 1)

	result, err := svc.RecoverErrorTargetAfterSuccessfulTest(context.Background(), 42, targets[0])

	require.NoError(t, err)
	require.True(t, result.ClearedModelRateLimit)
	require.Zero(t, state.size())
	var rearmedClaim openAIAccountModelTransientDecision
	for i, requestID := range []string{"rearmed-1", "rearmed-2", "rearmed-3"} {
		rearmedClaim = state.recordFailure(42, "gpt-5.6-sol", now.Add(time.Duration(i+3)*time.Millisecond), requestID)
	}
	require.True(t, rearmedClaim.OpenCircuit)
	require.NotEqual(t, initialClaim.PersistenceGeneration, rearmedClaim.PersistenceGeneration)
}

func TestRateLimitService_RecoverErrorTargetPreservesFailureAfterProbeSnapshot(t *testing.T) {
	limit := map[string]any{"rate_limit_reset_at": time.Now().Add(10 * time.Minute).Format(time.RFC3339)}
	account := &Account{
		ID: 43, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
		Extra: map[string]any{modelRateLimitsKey: map[string]any{"gpt-5.6-sol": limit}},
	}
	repo := &rateLimitClearRepoStub{getByIDAccount: account}
	state := newOpenAIAccountModelTransientState(1)
	now := time.Now()
	var claim openAIAccountModelTransientDecision
	for i, requestID := range []string{"initial-1", "initial-2", "initial-3"} {
		claim = state.recordFailure(account.ID, "gpt-5.6-sol", now.Add(time.Duration(i)*time.Millisecond), requestID)
	}
	state.finishCircuitPersistence(account.ID, "gpt-5.6-sol", claim.PersistenceGeneration, true)
	gateway := &OpenAIGatewayService{openaiModelTransient: state}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	svc.SetAccountRuntimeBlocker(gateway)
	targets, err := svc.ErrorRecoveryTargets(context.Background(), account.ID, "")
	require.NoError(t, err)
	require.Len(t, targets, 1)

	state.recordFailure(account.ID, "gpt-5.6-sol", now.Add(4*time.Millisecond), "after-probe-snapshot")
	result, err := svc.RecoverErrorTargetAfterSuccessfulTest(context.Background(), account.ID, targets[0])

	require.NoError(t, err)
	require.False(t, result.ClearedModelRateLimit)
	require.Empty(t, repo.clearModelRateLimitKeys)
	require.Equal(t, 1, state.size())
}

func TestRateLimitService_ErrorRecoveryTargetPreservesFailureDuringAccountRead(t *testing.T) {
	limit := map[string]any{"rate_limit_reset_at": time.Now().Add(10 * time.Minute).Format(time.RFC3339)}
	account := &Account{
		ID: 44, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
		Extra: map[string]any{modelRateLimitsKey: map[string]any{"gpt-5.6-sol": limit}},
	}
	state := newOpenAIAccountModelTransientState(1)
	now := time.Now()
	var claim openAIAccountModelTransientDecision
	for i, requestID := range []string{"initial-1", "initial-2", "initial-3"} {
		claim = state.recordFailure(account.ID, "gpt-5.6-sol", now.Add(time.Duration(i)*time.Millisecond), requestID)
	}
	state.finishCircuitPersistence(account.ID, "gpt-5.6-sol", claim.PersistenceGeneration, true)
	repo := &rateLimitClearRepoStub{getByIDAccount: account}
	repo.getByIDHook = func() {
		state.recordFailure(account.ID, "gpt-5.6-sol", now.Add(4*time.Millisecond), "during-account-read")
		repo.getByIDHook = nil
	}
	gateway := &OpenAIGatewayService{openaiModelTransient: state}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	svc.SetAccountRuntimeBlocker(gateway)

	targets, err := svc.ErrorRecoveryTargets(context.Background(), account.ID, "")
	require.NoError(t, err)
	require.Len(t, targets, 1)
	result, err := svc.RecoverErrorTargetAfterSuccessfulTest(context.Background(), account.ID, targets[0])

	require.NoError(t, err)
	require.False(t, result.ClearedModelRateLimit)
	require.Empty(t, repo.clearModelRateLimitKeys)
	require.Equal(t, 1, state.size())
}

func TestRateLimitService_ErrorRecoveryTargetMatchesCanonicalModelCase(t *testing.T) {
	limit := map[string]any{"rate_limit_reset_at": time.Now().Add(10 * time.Minute).Format(time.RFC3339)}
	account := &Account{
		ID: 45, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
		Extra: map[string]any{modelRateLimitsKey: map[string]any{"GPT-5.6-SOL": limit}},
	}
	repo := &rateLimitClearRepoStub{getByIDAccount: account}
	state := newOpenAIAccountModelTransientState(1)
	now := time.Now()
	var claim openAIAccountModelTransientDecision
	for i, requestID := range []string{"initial-1", "initial-2", "initial-3"} {
		claim = state.recordFailure(account.ID, "GPT-5.6-SOL", now.Add(time.Duration(i)*time.Millisecond), requestID)
	}
	state.finishCircuitPersistence(account.ID, "GPT-5.6-SOL", claim.PersistenceGeneration, true)
	gateway := &OpenAIGatewayService{openaiModelTransient: state}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	svc.SetAccountRuntimeBlocker(gateway)

	targets, err := svc.ErrorRecoveryTargets(context.Background(), account.ID, "")
	require.NoError(t, err)
	require.Len(t, targets, 1)
	result, err := svc.RecoverErrorTargetAfterSuccessfulTest(context.Background(), account.ID, targets[0])

	require.NoError(t, err)
	require.True(t, result.ClearedModelRateLimit)
	require.Equal(t, []string{"GPT-5.6-SOL"}, repo.clearModelRateLimitKeys)
	require.Zero(t, state.size())
}

func (c *tempUnschedCacheRecorder) SetTempUnsched(ctx context.Context, accountID int64, state *TempUnschedState) error {
	return nil
}

func (c *tempUnschedCacheRecorder) GetTempUnsched(ctx context.Context, accountID int64) (*TempUnschedState, error) {
	return nil, nil
}

func (c *tempUnschedCacheRecorder) DeleteTempUnsched(ctx context.Context, accountID int64) error {
	c.deletedIDs = append(c.deletedIDs, accountID)
	return c.deleteErr
}

func (s *recoverTokenInvalidatorStub) InvalidateToken(ctx context.Context, account *Account) error {
	s.accounts = append(s.accounts, account)
	return s.err
}

func TestRateLimitService_ClearRateLimit_AlsoClearsTempUnschedulable(t *testing.T) {
	repo := &rateLimitClearRepoStub{}
	cache := &tempUnschedCacheRecorder{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, cache)

	err := svc.ClearRateLimit(context.Background(), 42)
	require.NoError(t, err)

	require.Equal(t, 1, repo.clearRateLimitCalls)
	require.Equal(t, 1, repo.clearAntigravityCalls)
	require.Equal(t, 1, repo.clearModelRateLimitCalls)
	require.Equal(t, 1, repo.clearTempUnschedCalls)
	require.Equal(t, []int64{42}, cache.deletedIDs)
}

func TestRateLimitService_ClearRateLimit_ClearTempUnschedulableFailed(t *testing.T) {
	repo := &rateLimitClearRepoStub{
		clearTempUnschedulableErr: errors.New("clear temp unsched failed"),
	}
	cache := &tempUnschedCacheRecorder{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, cache)

	err := svc.ClearRateLimit(context.Background(), 7)
	require.Error(t, err)

	require.Equal(t, 1, repo.clearTempUnschedCalls)
	require.Empty(t, cache.deletedIDs)
}

func TestRateLimitService_ClearRateLimit_ClearRateLimitFailed(t *testing.T) {
	repo := &rateLimitClearRepoStub{
		clearRateLimitErr: errors.New("clear rate limit failed"),
	}
	cache := &tempUnschedCacheRecorder{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, cache)

	err := svc.ClearRateLimit(context.Background(), 11)
	require.Error(t, err)

	require.Equal(t, 1, repo.clearRateLimitCalls)
	require.Equal(t, 0, repo.clearAntigravityCalls)
	require.Equal(t, 0, repo.clearModelRateLimitCalls)
	require.Equal(t, 0, repo.clearTempUnschedCalls)
	require.Empty(t, cache.deletedIDs)
}

func TestRateLimitService_ClearRateLimit_ClearAntigravityFailed(t *testing.T) {
	repo := &rateLimitClearRepoStub{
		clearAntigravityErr: errors.New("clear antigravity failed"),
	}
	cache := &tempUnschedCacheRecorder{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, cache)

	err := svc.ClearRateLimit(context.Background(), 12)
	require.Error(t, err)

	require.Equal(t, 1, repo.clearRateLimitCalls)
	require.Equal(t, 1, repo.clearAntigravityCalls)
	require.Equal(t, 0, repo.clearModelRateLimitCalls)
	require.Equal(t, 0, repo.clearTempUnschedCalls)
	require.Empty(t, cache.deletedIDs)
}

func TestRateLimitService_ClearRateLimit_ClearModelRateLimitsFailed(t *testing.T) {
	repo := &rateLimitClearRepoStub{
		clearModelRateLimitErr: errors.New("clear model rate limits failed"),
	}
	cache := &tempUnschedCacheRecorder{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, cache)

	err := svc.ClearRateLimit(context.Background(), 13)
	require.Error(t, err)

	require.Equal(t, 1, repo.clearRateLimitCalls)
	require.Equal(t, 1, repo.clearAntigravityCalls)
	require.Equal(t, 1, repo.clearModelRateLimitCalls)
	require.Equal(t, 0, repo.clearTempUnschedCalls)
	require.Empty(t, cache.deletedIDs)
}

func TestRateLimitService_ClearRateLimit_CacheDeleteFailedShouldNotFail(t *testing.T) {
	repo := &rateLimitClearRepoStub{}
	cache := &tempUnschedCacheRecorder{
		deleteErr: errors.New("cache delete failed"),
	}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, cache)

	err := svc.ClearRateLimit(context.Background(), 14)
	require.NoError(t, err)

	require.Equal(t, 1, repo.clearRateLimitCalls)
	require.Equal(t, 1, repo.clearAntigravityCalls)
	require.Equal(t, 1, repo.clearModelRateLimitCalls)
	require.Equal(t, 1, repo.clearTempUnschedCalls)
	require.Equal(t, []int64{14}, cache.deletedIDs)
}

func TestRateLimitService_ClearRateLimit_WithoutTempUnschedCache(t *testing.T) {
	repo := &rateLimitClearRepoStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)

	err := svc.ClearRateLimit(context.Background(), 15)
	require.NoError(t, err)

	require.Equal(t, 1, repo.clearRateLimitCalls)
	require.Equal(t, 1, repo.clearAntigravityCalls)
	require.Equal(t, 1, repo.clearModelRateLimitCalls)
	require.Equal(t, 1, repo.clearTempUnschedCalls)
}

func TestRateLimitService_RecoverAccountAfterSuccessfulTest_ClearsErrorAndRateLimitRelatedState(t *testing.T) {
	now := time.Now()
	resetAt := now.Add(time.Hour)
	repo := &rateLimitClearRepoStub{
		getByIDAccount: &Account{
			ID:                     42,
			Status:                 StatusError,
			RateLimitedAt:          &now,
			TempUnschedulableUntil: &now,
			Extra: map[string]any{
				"model_rate_limits": map[string]any{
					"gpt-5.6-sol": map[string]any{
						"rate_limit_reset_at": resetAt.Format(time.RFC3339),
					},
					"gpt-5.6-luna": map[string]any{
						"rate_limit_reset_at": resetAt.Format(time.RFC3339),
					},
					"AICredits": map[string]any{
						"rate_limit_reset_at": resetAt.Format(time.RFC3339),
					},
				},
				"antigravity_quota_scopes": map[string]any{"gemini": true},
			},
		},
	}
	cache := &tempUnschedCacheRecorder{}
	blocker := &runtimeBlockRecorder{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, cache)
	svc.SetAccountRuntimeBlocker(blocker)

	result, err := svc.RecoverAccountAfterSuccessfulTest(context.Background(), 42, "gpt-5.6-sol")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.ClearedError)
	require.True(t, result.ClearedRateLimit)
	require.True(t, result.ClearedModelRateLimit)

	require.Equal(t, 1, repo.getByIDCalls)
	require.Equal(t, 1, repo.clearErrorCalls)
	require.Equal(t, 1, repo.clearRateLimitCalls)
	require.Equal(t, 1, repo.clearAntigravityCalls)
	require.Equal(t, 0, repo.clearModelRateLimitCalls)
	require.Equal(t, []string{"gpt-5.6-sol"}, repo.clearModelRateLimitKeys)
	require.Equal(t, 1, repo.clearTempUnschedCalls)
	require.Equal(t, []int64{42}, cache.deletedIDs)
	require.Equal(t, []int64{42}, blocker.clearedIDs)
}

func TestRateLimitService_RecoverAccountAfterSuccessfulTest_NoRecoverableStateIsNoop(t *testing.T) {
	repo := &rateLimitClearRepoStub{
		getByIDAccount: &Account{
			ID:          7,
			Status:      StatusActive,
			Schedulable: true,
			Extra:       map[string]any{},
		},
	}
	cache := &tempUnschedCacheRecorder{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, cache)

	result, err := svc.RecoverAccountAfterSuccessfulTest(context.Background(), 7, "gpt-5.6-sol")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.ClearedError)
	require.False(t, result.ClearedRateLimit)
	require.False(t, result.ClearedModelRateLimit)

	require.Equal(t, 1, repo.getByIDCalls)
	require.Equal(t, 0, repo.clearErrorCalls)
	require.Equal(t, 0, repo.clearRateLimitCalls)
	require.Equal(t, 0, repo.clearAntigravityCalls)
	require.Equal(t, 0, repo.clearModelRateLimitCalls)
	require.Empty(t, repo.clearModelRateLimitKeys)
	require.Equal(t, 0, repo.clearTempUnschedCalls)
	require.Empty(t, cache.deletedIDs)
}

func TestRateLimitService_RecoverAccountAfterSuccessfulTest_ClearErrorFailed(t *testing.T) {
	repo := &rateLimitClearRepoStub{
		getByIDAccount: &Account{
			ID:     9,
			Status: StatusError,
		},
		clearErrorErr: errors.New("clear error failed"),
	}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)

	result, err := svc.RecoverAccountAfterSuccessfulTest(context.Background(), 9, "gpt-5.6-sol")
	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, 1, repo.getByIDCalls)
	require.Equal(t, 1, repo.clearErrorCalls)
	require.Equal(t, 0, repo.clearRateLimitCalls)
}

func TestRateLimitService_RecoverAccountState_InvalidatesOAuthTokenOnErrorRecovery(t *testing.T) {
	repo := &rateLimitClearRepoStub{
		getByIDAccount: &Account{
			ID:     21,
			Type:   AccountTypeOAuth,
			Status: StatusError,
		},
	}
	invalidator := &recoverTokenInvalidatorStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	svc.SetTokenCacheInvalidator(invalidator)

	result, err := svc.RecoverAccountState(context.Background(), 21, AccountRecoveryOptions{
		InvalidateToken: true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.ClearedError)
	require.False(t, result.ClearedRateLimit)
	require.Equal(t, 1, repo.clearErrorCalls)
	require.Len(t, invalidator.accounts, 1)
	require.Equal(t, int64(21), invalidator.accounts[0].ID)
}
