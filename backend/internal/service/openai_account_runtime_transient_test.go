package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

type transientCooldownAccountRepo struct {
	AccountRepository
}

func (transientCooldownAccountRepo) SetOverloaded(context.Context, int64, time.Time) error {
	return nil
}

type transientCircuitRateLimitCall struct {
	accountID int64
	model     string
	resetAt   time.Time
}

type transientCircuitAccountRepo struct {
	AccountRepository
	calls []transientCircuitRateLimitCall
}

func (r *transientCircuitAccountRepo) SetOverloaded(context.Context, int64, time.Time) error {
	return nil
}

func (r *transientCircuitAccountRepo) SetModelRateLimit(_ context.Context, accountID int64, model string, resetAt time.Time, _ ...string) error {
	r.calls = append(r.calls, transientCircuitRateLimitCall{accountID: accountID, model: model, resetAt: resetAt})
	return nil
}

func TestHandleOpenAITransientError_ThreeDistinctRequestsPersistModelCircuit(t *testing.T) {
	repo := &transientCircuitAccountRepo{}
	svc := &OpenAIGatewayService{rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil)}
	account := &Account{ID: 5110, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	startedAt := time.Now()

	for _, requestID := range []string{"request-1", "request-2", "request-3"} {
		ctx := context.WithValue(context.Background(), ctxkey.RequestID, requestID)
		svc.handleOpenAIAccountUpstreamError(ctx, account, http.StatusGatewayTimeout, http.Header{}, []byte(`{"error":{"message":"gateway timeout"}}`), "gpt-5.6-sol")
	}

	require.Len(t, repo.calls, 1)
	require.Equal(t, account.ID, repo.calls[0].accountID)
	require.Equal(t, "gpt-5.6-sol", repo.calls[0].model)
	require.WithinDuration(t, startedAt.Add(10*time.Minute), repo.calls[0].resetAt, 2*time.Second)
}

func TestHandleOpenAITransientError_RetriesWithinOneRequestCountOnce(t *testing.T) {
	repo := &transientCircuitAccountRepo{}
	svc := &OpenAIGatewayService{rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil)}
	account := &Account{ID: 5111, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "same-request")

	for range 3 {
		svc.handleOpenAIAccountUpstreamError(ctx, account, http.StatusGatewayTimeout, http.Header{}, []byte(`{"error":{"message":"gateway timeout"}}`), "gpt-5.6-sol")
	}

	require.Empty(t, repo.calls)
	require.False(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "gpt-5.6-sol"))
}

func TestHandleOpenAITransientError_MissingRequestIDDoesNotAdvanceDurableCircuit(t *testing.T) {
	repo := &transientCircuitAccountRepo{}
	svc := &OpenAIGatewayService{rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil)}
	account := &Account{ID: 5112, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	for range 2 {
		svc.handleOpenAIAccountUpstreamError(context.Background(), account, http.StatusGatewayTimeout, http.Header{}, []byte(`{"error":{"message":"gateway timeout"}}`), "gpt-5.6-sol")
	}
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "first-proven-request")
	svc.handleOpenAIAccountUpstreamError(ctx, account, http.StatusGatewayTimeout, http.Header{}, []byte(`{"error":{"message":"gateway timeout"}}`), "gpt-5.6-sol")

	require.Empty(t, repo.calls)
}

func TestHandleOpenAITransientError_OpenCircuitPersistsOnce(t *testing.T) {
	repo := &transientCircuitAccountRepo{}
	svc := &OpenAIGatewayService{rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil)}
	account := &Account{ID: 5113, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	for _, requestID := range []string{"request-1", "request-2", "request-3", "request-4", "request-5"} {
		ctx := context.WithValue(context.Background(), ctxkey.RequestID, requestID)
		svc.handleOpenAIAccountUpstreamError(ctx, account, http.StatusGatewayTimeout, http.Header{}, []byte(`{"error":{"message":"gateway timeout"}}`), "gpt-5.6-sol")
	}

	require.Len(t, repo.calls, 1)
}

func TestHandleOpenAITransientError_RetriesAfterCircuitThresholdCountOnce(t *testing.T) {
	repo := &transientCircuitAccountRepo{}
	svc := &OpenAIGatewayService{rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil)}
	account := &Account{ID: 5114, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	for _, requestID := range []string{"request-1", "request-2", "request-3", "request-4", "request-4"} {
		ctx := context.WithValue(context.Background(), ctxkey.RequestID, requestID)
		svc.handleOpenAIAccountUpstreamError(ctx, account, http.StatusGatewayTimeout, http.Header{}, []byte(`{"error":{"message":"gateway timeout"}}`), "gpt-5.6-sol")
	}

	state := svc.getOpenAIAccountModelTransientState()
	key, ok := openAIAccountModelTransientKey(account.ID, "gpt-5.6-sol")
	require.True(t, ok)
	state.mu.Lock()
	entry := state.entries[key]
	state.mu.Unlock()
	require.Equal(t, 4, entry.failureStreak)
}

func TestOpenAIModelTransientState_StalePersistenceCompletionDoesNotMutateNewClaim(t *testing.T) {
	state := newOpenAIAccountModelTransientState(1)
	now := time.Now()
	var oldClaim openAIAccountModelTransientDecision
	for i, requestID := range []string{"old-1", "old-2", "old-3"} {
		oldClaim = state.recordFailure(5115, "gpt-5.6-sol", now.Add(time.Duration(i)*time.Millisecond), requestID)
	}
	state.recordSuccess(5115, "gpt-5.6-sol")

	var newClaim openAIAccountModelTransientDecision
	for i, requestID := range []string{"new-1", "new-2", "new-3"} {
		newClaim = state.recordFailure(5115, "gpt-5.6-sol", now.Add(time.Duration(i+3)*time.Millisecond), requestID)
	}
	require.NotEqual(t, oldClaim.PersistenceGeneration, newClaim.PersistenceGeneration)

	state.finishCircuitPersistence(5115, "gpt-5.6-sol", oldClaim.PersistenceGeneration, true)
	key, ok := openAIAccountModelTransientKey(5115, "gpt-5.6-sol")
	require.True(t, ok)
	state.mu.Lock()
	entry := state.entries[key]
	state.mu.Unlock()
	require.True(t, entry.persisting)
	require.False(t, entry.persisted)
	require.Equal(t, newClaim.PersistenceGeneration, entry.persistenceGeneration)
}

func TestOpenAIModelTransientState_RecoveryClearDoesNotBlockOtherAccount(t *testing.T) {
	state := newOpenAIAccountModelTransientState(2)
	now := time.Now()
	state.recordFailure(5201, "gpt-5.6-sol", now, "request-a")
	observed := state.mutationGeneration(5201, "gpt-5.6-sol")
	clearStarted := make(chan struct{})
	releaseClear := make(chan struct{})
	clearDone := make(chan struct{})
	go func() {
		defer close(clearDone)
		_, _ = state.clearCircuitIfGeneration(5201, "gpt-5.6-sol", observed, func() (bool, error) {
			close(clearStarted)
			<-releaseClear
			return true, nil
		})
	}()
	<-clearStarted

	otherDone := make(chan struct{})
	go func() {
		state.recordFailure(5202, "gpt-5.6-sol", now, "request-b")
		close(otherDone)
	}()
	otherCompleted := false
	select {
	case <-otherDone:
		otherCompleted = true
	case <-time.After(200 * time.Millisecond):
	}
	close(releaseClear)
	<-clearDone

	require.True(t, otherCompleted)
	require.Equal(t, 1, state.size())
}

func TestHandleOpenAITransientError_BlocksOnlyRequestedModel(t *testing.T) {
	svc := &OpenAIGatewayService{}
	svc.rateLimitService = NewRateLimitService(transientCooldownAccountRepo{}, nil, &config.Config{}, nil, nil)
	account := &Account{
		ID:       5105,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
	}

	firstShouldDisable := svc.handleOpenAIAccountUpstreamError(context.Background(), account, http.StatusBadGateway, http.Header{}, []byte(`{"error":{"message":"Upstream request failed","type":"upstream_error"}}`), "gpt-5.5")
	secondShouldDisable := svc.handleOpenAIAccountUpstreamError(context.Background(), account, http.StatusBadGateway, http.Header{}, []byte(`{"error":{"message":"Upstream request failed","type":"upstream_error"}}`), "gpt-5.5")

	require.False(t, firstShouldDisable)
	require.False(t, secondShouldDisable)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.True(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "gpt-5.5"))
	require.False(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "gpt-5.6-terra"))
}

func TestHandleOpenAITransientError_TransientStatusesUseModelScope(t *testing.T) {
	for _, statusCode := range []int{http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout, 520, 521, 522, 523, 524} {
		t.Run(http.StatusText(statusCode), func(t *testing.T) {
			svc := &OpenAIGatewayService{}
			svc.rateLimitService = NewRateLimitService(transientCooldownAccountRepo{}, nil, &config.Config{}, nil, nil)
			account := &Account{
				ID:       int64(5100 + statusCode),
				Platform: PlatformOpenAI,
				Type:     AccountTypeAPIKey,
			}

			firstShouldDisable := svc.handleOpenAIAccountUpstreamError(context.Background(), account, statusCode, http.Header{}, []byte(`{"error":{"message":"temporary upstream failure"}}`), "gpt-5.5")
			secondShouldDisable := svc.handleOpenAIAccountUpstreamError(context.Background(), account, statusCode, http.Header{}, []byte(`{"error":{"message":"temporary upstream failure"}}`), "gpt-5.5")

			require.False(t, firstShouldDisable)
			require.False(t, secondShouldDisable)
			require.False(t, svc.isOpenAIAccountRuntimeBlocked(account), "status %d must not block the whole account", statusCode)
			require.True(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "gpt-5.5"), "status %d should block the failing model", statusCode)
		})
	}
}

func TestHandleOpenAITransientError_529RemainsOverloadOnly(t *testing.T) {
	require.False(t, shouldCooldownOpenAITransientUpstreamError(529, []byte(`{"error":{"message":"overloaded"}}`)))
}

func TestHandleOpenAITransientError_Transient400DoesNotPersistDurableCircuit(t *testing.T) {
	repo := &transientCircuitAccountRepo{}
	svc := &OpenAIGatewayService{rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil)}
	account := &Account{ID: 5116, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	for _, requestID := range []string{"request-1", "request-2", "request-3"} {
		ctx := context.WithValue(context.Background(), ctxkey.RequestID, requestID)
		svc.handleOpenAIAccountUpstreamError(ctx, account, http.StatusBadRequest, http.Header{}, []byte(`{"error":{"message":"An error occurred while processing your request"}}`), "gpt-5.6-sol")
	}

	require.Empty(t, repo.calls)
	require.True(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "gpt-5.6-sol"))
}

func TestHandleOpenAITransientError_CanonicalModelIsNotMappedTwice(t *testing.T) {
	svc := &OpenAIGatewayService{}
	svc.rateLimitService = NewRateLimitService(transientCooldownAccountRepo{}, nil, &config.Config{}, nil, nil)
	account := &Account{
		ID:       5107,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"public-alias": "upstream-a",
				"upstream-a":   "upstream-b",
			},
		},
	}
	canonicalModel := account.GetMappedModel("public-alias")
	require.Equal(t, "upstream-a", canonicalModel)

	for range 2 {
		svc.handleOpenAIAccountUpstreamError(context.Background(), account, http.StatusBadGateway, http.Header{}, []byte(`{"error":{"message":"temporary upstream failure"}}`), canonicalModel)
	}

	require.True(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "public-alias"))
	svc.ReportOpenAIAccountScheduleResult(account.ID, canonicalModel, true, nil)
	require.False(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "public-alias"))
}

func TestHandleOpenAITransientError_DoesNotBlockParameter400(t *testing.T) {
	svc := &OpenAIGatewayService{}
	svc.rateLimitService = NewRateLimitService(transientCooldownAccountRepo{}, nil, &config.Config{}, nil, nil)
	account := &Account{
		ID:       5103,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
	}

	shouldDisable := svc.handleOpenAIAccountUpstreamError(context.Background(), account, http.StatusBadRequest, http.Header{}, []byte(`{"error":{"message":"Invalid type for input[0].arguments"}}`), "gpt-5.5")

	require.False(t, shouldDisable)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.False(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "gpt-5.5"))
}

func TestHandleOpenAITransientError_HardDisableStillBlocksWholeAccount(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 5106, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	svc.BlockAccountScheduling(account, time.Now().Add(time.Minute), "upstream_disable")

	require.True(t, svc.isOpenAIAccountRequestRuntimeBlocked(account, "gpt-5.5"))
	require.True(t, svc.isOpenAIAccountRequestRuntimeBlocked(account, "gpt-5.6-sol"))
}
