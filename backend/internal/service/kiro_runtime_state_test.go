//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/kirocooldown"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type stubKiroCooldownStore struct {
	reserveWait  time.Duration
	reserveErr   error
	successErr   error
	mark429TTL   time.Duration
	mark429Err   error
	suspendedTTL time.Duration
	suspendedErr error
	state        *kirocooldown.State
	stateErr     error
	clearCalled  bool
	clearKeys    []string
	clearResult  bool
	clearErr     error
}

type recordingKiroTempUnschedRepo struct {
	mockAccountRepoForGemini
	called          bool
	id              int64
	until           time.Time
	reason          string
	rateCalled      bool
	rateID          int64
	rateLimitReset  time.Time
	rateLimitedCall int
}

func (r *recordingKiroTempUnschedRepo) SetTempUnschedulable(_ context.Context, id int64, until time.Time, reason string) error {
	r.called = true
	r.id = id
	r.until = until
	r.reason = reason
	return nil
}

func (r *recordingKiroTempUnschedRepo) SetRateLimited(_ context.Context, id int64, resetAt time.Time) error {
	r.rateCalled = true
	r.rateID = id
	r.rateLimitReset = resetAt
	r.rateLimitedCall++
	return nil
}

type recordingKiroErrorRepo struct {
	recordingKiroTempUnschedRepo
	setErrorCalls int
	errorID       int64
	errorMsg      string
}

func (r *recordingKiroErrorRepo) SetError(_ context.Context, id int64, errorMsg string) error {
	r.setErrorCalls++
	r.errorID = id
	r.errorMsg = errorMsg
	return nil
}

func (s *stubKiroCooldownStore) ReserveRequest(context.Context, string) (time.Duration, error) {
	return s.reserveWait, s.reserveErr
}

func (s *stubKiroCooldownStore) MarkSuccess(context.Context, string) error {
	return s.successErr
}

func (s *stubKiroCooldownStore) Mark429(context.Context, string) (time.Duration, error) {
	return s.mark429TTL, s.mark429Err
}

func (s *stubKiroCooldownStore) MarkSuspended(context.Context, string) (time.Duration, error) {
	return s.suspendedTTL, s.suspendedErr
}

func (s *stubKiroCooldownStore) GetState(context.Context, string) (*kirocooldown.State, error) {
	if s.clearCalled && s.clearResult {
		return nil, nil
	}
	return s.state, s.stateErr
}

func (s *stubKiroCooldownStore) ClearEarliestTransientCooldown(_ context.Context, tokenKeys []string) (bool, error) {
	s.clearCalled = true
	s.clearKeys = append([]string(nil), tokenKeys...)
	return s.clearResult, s.clearErr
}

func TestCalculateKiro429Cooldown(t *testing.T) {
	require.Equal(t, time.Minute, kirocooldown.Calculate429Cooldown(0))
	require.Equal(t, 2*time.Minute, kirocooldown.Calculate429Cooldown(1))
	require.Equal(t, 4*time.Minute, kirocooldown.Calculate429Cooldown(2))
	require.Equal(t, 5*time.Minute, kirocooldown.Calculate429Cooldown(3))
	require.Equal(t, 5*time.Minute, kirocooldown.Calculate429Cooldown(10))
}

func TestGatewayServiceCheckAndWaitKiroCooldownReturnsNilWithoutWait(t *testing.T) {
	svc := &GatewayService{
		kiroCooldownStore: &stubKiroCooldownStore{},
	}

	require.NoError(t, svc.checkAndWaitKiroCooldown(context.Background(), "token1"))
}

func TestGatewayServiceCheckAndWaitKiroCooldownPropagatesReserveError(t *testing.T) {
	expected := errors.New("redis unavailable")
	svc := &GatewayService{
		kiroCooldownStore: &stubKiroCooldownStore{reserveErr: expected},
	}

	err := svc.checkAndWaitKiroCooldown(context.Background(), "token1")
	require.ErrorIs(t, err, expected)
}

func TestGatewayServiceCheckAndWaitKiroCooldownRequiresStore(t *testing.T) {
	svc := &GatewayService{}
	err := svc.checkAndWaitKiroCooldown(context.Background(), "token1")
	require.ErrorIs(t, err, errKiroCooldownStoreUnavailable)
}

func TestGatewayServiceCheckAndWaitKiroCooldownWaitsAndHonorsContext(t *testing.T) {
	svc := &GatewayService{
		kiroCooldownStore: &stubKiroCooldownStore{reserveWait: 200 * time.Millisecond},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := svc.checkAndWaitKiroCooldown(ctx, "token1")
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestAsKiroCooldownFailoverError(t *testing.T) {
	err := kirocooldown.NewError(32500*time.Millisecond, kirocooldown.CooldownReason429)

	var cooldownErr *kirocooldown.Error
	require.ErrorAs(t, err, &cooldownErr)

	failoverErr := asKiroCooldownFailoverError(err)
	require.NotNil(t, failoverErr)
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	require.Equal(t, "kiro token is in cooldown for 33s (reason: rate_limit_exceeded)", string(failoverErr.ResponseBody))
	require.False(t, failoverErr.RetryableOnSameAccount)
}

func TestAsKiroCooldownFailoverErrorIgnoresNonCooldownErrors(t *testing.T) {
	require.Nil(t, asKiroCooldownFailoverError(errors.New("redis unavailable")))
}

func TestGatewayServiceTryRecoverKiroCooldownPoolClearsOnlyTransientCooldown(t *testing.T) {
	store := &stubKiroCooldownStore{
		state: &kirocooldown.State{
			Active:        true,
			Reason:        kirocooldown.CooldownReason429,
			CooldownUntil: time.Now().Add(time.Minute),
			Remaining:     time.Minute,
		},
		clearResult: true,
	}
	svc := &GatewayService{kiroCooldownStore: store}
	accounts := []Account{
		{
			ID:          42,
			Platform:    PlatformKiro,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: true,
		},
	}

	recovered := svc.tryRecoverKiroCooldownPool(context.Background(), accounts, "", nil, false)
	require.True(t, recovered)
	require.True(t, store.clearCalled)
	require.Len(t, store.clearKeys, 1)
	require.Equal(t, buildKiroAccountKey(&accounts[0]), store.clearKeys[0])
}

func TestGatewayServiceTryRecoverKiroCooldownPoolSkipsSuspended(t *testing.T) {
	store := &stubKiroCooldownStore{
		state: &kirocooldown.State{
			Active:        true,
			Reason:        kirocooldown.CooldownReasonSuspended,
			CooldownUntil: time.Now().Add(time.Hour),
			Remaining:     time.Hour,
		},
		clearResult: true,
	}
	svc := &GatewayService{kiroCooldownStore: store}
	accounts := []Account{
		{
			ID:          42,
			Platform:    PlatformKiro,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: true,
		},
	}

	recovered := svc.tryRecoverKiroCooldownPool(context.Background(), accounts, "", nil, false)
	require.False(t, recovered)
	require.False(t, store.clearCalled)
}

func TestSelectAccountWithLoadAwarenessRecoversKiroCooldownPool(t *testing.T) {
	cfg := testConfig()
	cfg.Gateway.Scheduling.LoadBatchEnabled = true

	account := Account{
		ID:          42,
		Platform:    PlatformKiro,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
	}
	store := &stubKiroCooldownStore{
		state: &kirocooldown.State{
			Active:        true,
			Reason:        kirocooldown.CooldownReason429,
			CooldownUntil: time.Now().Add(time.Minute),
			Remaining:     time.Minute,
		},
		clearResult: true,
	}
	svc := &GatewayService{
		accountRepo:         &mockAccountRepoForGemini{accounts: []Account{account}},
		concurrencyService:  NewConcurrencyService(&mockConcurrencyCache{}),
		cfg:                 cfg,
		kiroCooldownStore:   store,
		tlsFPProfileService: &TLSFingerprintProfileService{},
	}
	ctx := context.WithValue(context.Background(), ctxkey.ForcePlatform, PlatformKiro)

	result, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "", "", nil, "", 0)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, account.ID, result.Account.ID)
	require.True(t, store.clearCalled)
	require.Equal(t, []string{buildKiroAccountKey(&account)}, store.clearKeys)
}

func TestClassifyKiroHTTPErrorMonthlyRequestCount(t *testing.T) {
	tests := []string{
		`{"message":"You have reached the limit.","reason":"MONTHLY_REQUEST_COUNT"}`,
		`{"error":{"reason":"MONTHLY_REQUEST_COUNT"}}`,
		`API returned 402: {"message":"You have reached the limit.","reason":"MONTHLY_REQUEST_COUNT"}`,
	}

	for _, body := range tests {
		classification := classifyKiroHTTPError(http.StatusPaymentRequired, body)
		require.Equal(t, kiroErrorMonthlyRequest, classification.Category)
	}
}

func TestClassifyKiroHTTPErrorPlain402IsTransient(t *testing.T) {
	classification := classifyKiroHTTPError(http.StatusPaymentRequired, `{"message":"payment required"}`)
	require.Equal(t, kiroErrorUpstreamTransient, classification.Category)
}

func TestExecuteKiroUpstreamCooldownReturnsFailoverError(t *testing.T) {
	svc := &GatewayService{
		kiroCooldownStore: &stubKiroCooldownStore{
			reserveErr: kirocooldown.NewError(32500*time.Millisecond, kirocooldown.CooldownReason429),
		},
	}

	_, _, err := svc.executeKiroUpstream(context.Background(), &Account{ID: 42}, []byte(`{}`), "claude-sonnet-4-6", "claude-sonnet-4-6", "token", nil)
	require.Error(t, err)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	require.Equal(t, "kiro token is in cooldown for 33s (reason: rate_limit_exceeded)", string(failoverErr.ResponseBody))
	require.False(t, failoverErr.RetryableOnSameAccount)
}

func TestExecuteKiroUpstreamInvalidModelDoesNotRefreshProfileArnOrRetry(t *testing.T) {
	account := &Account{
		ID:          42,
		Platform:    PlatformKiro,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"profile_arn": "arn:aws:codewhisperer:us-east-1:123456789012:profile/STALE",
		},
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	upstream := &queuedHTTPUpstream{
		responses: []*http.Response{
			newJSONResponse(http.StatusBadRequest, `{"message":"Invalid model ID. Please select a different model to continue.","reason":"INVALID_MODEL_ID"}`),
		},
	}
	svc := &GatewayService{
		accountRepo:         repo,
		httpUpstream:        upstream,
		kiroCooldownStore:   &stubKiroCooldownStore{},
		tlsFPProfileService: &TLSFingerprintProfileService{},
	}

	payload, err := createTestPayload("claude-opus-4-6")
	require.NoError(t, err)
	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	resp, _, err := svc.executeKiroUpstream(context.Background(), account, payloadBytes, "claude-opus-4-6", "claude-opus-4-6", "test-token", nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.Len(t, upstream.requests, 1)

	firstBody, readErr := io.ReadAll(upstream.requests[0].Body)
	require.NoError(t, readErr)
	require.Contains(t, string(firstBody), `"profileArn":"arn:aws:codewhisperer:us-east-1:123456789012:profile/STALE"`)
	require.Equal(t, "arn:aws:codewhisperer:us-east-1:123456789012:profile/STALE", account.GetCredential("profile_arn"))
}

func TestHandleKiroHTTPErrorOAuthInvalidModelRateLimitsAndFailovers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("Anthropic-Beta", "context-1m-2025-08-07")

	account := &Account{
		ID:       42,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Name:     "kiro-oauth",
	}
	repo := &recordingKiroTempUnschedRepo{}
	svc := &GatewayService{accountRepo: repo}
	requestBody := []byte(`{"model":"claude-opus-4-7","tools":[{"name":"search"}],"thinking":{"type":"adaptive"}}`)
	resp := newJSONResponse(http.StatusBadRequest, `{"error":{"message":"Invalid model. Please select a different model to continue.","type":"upstream_error"}}`)
	resp.Header.Set("x-request-id", "req-invalid-model")

	err := svc.handleKiroHTTPError(context.Background(), resp, c, account, "claude-opus-4.6", requestBody)
	require.Error(t, err)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadRequest, failoverErr.StatusCode)
	require.Contains(t, string(failoverErr.ResponseBody), "Invalid model")
	require.False(t, failoverErr.RetryableOnSameAccount)

	require.False(t, repo.called)
	require.True(t, repo.rateCalled)
	require.Equal(t, account.ID, repo.rateID)
	require.WithinDuration(t, time.Now().Add(kiroInvalidModelTempUnschedDuration), repo.rateLimitReset, 5*time.Second)

	rawEvents, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := rawEvents.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, events, 1)
	require.Equal(t, PlatformKiro, events[0].Platform)
	require.Equal(t, account.ID, events[0].AccountID)
	require.Equal(t, account.Name, events[0].AccountName)
	require.Equal(t, http.StatusBadRequest, events[0].UpstreamStatusCode)
	require.Equal(t, "req-invalid-model", events[0].UpstreamRequestID)
	require.Equal(t, "failover", events[0].Kind)
	require.Equal(t, "claude-opus-4-7", events[0].RequestedModel)
	require.Equal(t, "claude-opus-4.6", events[0].MappedModel)
	require.Equal(t, "claude-opus-4.6", events[0].KiroModelID)
	require.True(t, events[0].HasTools)
	require.True(t, events[0].HasAdaptiveThinking)
	require.True(t, events[0].HasContext1MBeta)
}

func TestHandleKiroHTTPErrorAPIKeyInvalidModelDoesNotFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	account := &Account{
		ID:       43,
		Platform: PlatformKiro,
		Type:     AccountTypeAPIKey,
	}
	repo := &recordingKiroTempUnschedRepo{}
	svc := &GatewayService{accountRepo: repo}
	resp := newJSONResponse(http.StatusBadRequest, `{"message":"Invalid model. Please select a different model to continue."}`)

	err := svc.handleKiroHTTPError(context.Background(), resp, c, account, "claude-opus-4.6", []byte(`{"model":"claude-opus-4-7"}`))
	require.Error(t, err)

	var failoverErr *UpstreamFailoverError
	require.NotErrorAs(t, err, &failoverErr)
	require.False(t, repo.called)
	require.False(t, repo.rateCalled)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestNextKiroMonthlyResetUTC(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			name: "middle of month",
			now:  time.Date(2026, time.April, 27, 10, 30, 45, 123, time.FixedZone("CST", 8*3600)),
			want: time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "december rolls year",
			now:  time.Date(2026, time.December, 31, 23, 59, 59, 0, time.UTC),
			want: time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, nextKiroMonthlyResetUTC(tt.now))
		})
	}
}

func TestExecuteKiroUpstreamMonthlyRequestCountRateLimitsUntilNextMonthAndFailovers(t *testing.T) {
	account := &Account{
		ID:          42,
		Platform:    PlatformKiro,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
	}
	repo := &recordingKiroTempUnschedRepo{}
	upstream := &queuedHTTPUpstream{
		responses: []*http.Response{
			newJSONResponse(http.StatusPaymentRequired, `{"message":"You have reached the limit.","reason":"MONTHLY_REQUEST_COUNT"}`),
		},
	}
	svc := &GatewayService{
		accountRepo:         repo,
		httpUpstream:        upstream,
		kiroCooldownStore:   &stubKiroCooldownStore{},
		tlsFPProfileService: &TLSFingerprintProfileService{},
	}

	payload, err := createTestPayload("claude-sonnet-4-6")
	require.NoError(t, err)
	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	_, _, err = svc.executeKiroUpstream(context.Background(), account, payloadBytes, "claude-sonnet-4-6", "claude-sonnet-4-6", "test-token", nil)
	require.Error(t, err)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusPaymentRequired, failoverErr.StatusCode)
	require.Contains(t, string(failoverErr.ResponseBody), "MONTHLY_REQUEST_COUNT")
	require.False(t, repo.called)
	require.True(t, repo.rateCalled)
	require.Equal(t, account.ID, repo.rateID)
	require.Equal(t, nextKiroMonthlyResetUTC(time.Now()), repo.rateLimitReset)
}

func TestExecuteKiroUpstreamPlain402FailoversWithoutTempUnschedule(t *testing.T) {
	account := &Account{
		ID:          42,
		Platform:    PlatformKiro,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
	}
	repo := &recordingKiroTempUnschedRepo{}
	upstream := &queuedHTTPUpstream{
		responses: []*http.Response{
			newJSONResponse(http.StatusPaymentRequired, `{"message":"payment required"}`),
		},
	}
	svc := &GatewayService{
		accountRepo:         repo,
		httpUpstream:        upstream,
		kiroCooldownStore:   &stubKiroCooldownStore{},
		tlsFPProfileService: &TLSFingerprintProfileService{},
	}

	payload, err := createTestPayload("claude-sonnet-4-6")
	require.NoError(t, err)
	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	_, _, err = svc.executeKiroUpstream(context.Background(), account, payloadBytes, "claude-sonnet-4-6", "claude-sonnet-4-6", "test-token", nil)
	require.Error(t, err)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusPaymentRequired, failoverErr.StatusCode)
	require.False(t, repo.called)
	require.False(t, repo.rateCalled)
}

func TestExecuteKiroUpstreamInvalidGrantForceRefreshSetsErrorWithoutTempUnschedule(t *testing.T) {
	account := &Account{
		ID:          42,
		Platform:    PlatformKiro,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"refresh_token": "old-refresh",
		},
	}
	repo := &recordingKiroErrorRepo{
		recordingKiroTempUnschedRepo: recordingKiroTempUnschedRepo{
			mockAccountRepoForGemini: mockAccountRepoForGemini{
				accountsByID: map[int64]*Account{account.ID: account},
			},
		},
	}
	upstream := &queuedHTTPUpstream{
		responses: []*http.Response{
			newJSONResponse(http.StatusUnauthorized, `{"message":"token expired"}`),
		},
	}
	provider := NewKiroTokenProvider(repo, nil, nil)
	provider.kiroOAuthService = &stubKiroAccountTokenRefresher{err: errors.New("invalid_grant: token revoked")}
	svc := &GatewayService{
		accountRepo:         repo,
		httpUpstream:        upstream,
		kiroCooldownStore:   &stubKiroCooldownStore{},
		tlsFPProfileService: &TLSFingerprintProfileService{},
		kiroTokenProvider:   provider,
	}

	payload, err := createTestPayload("claude-sonnet-4-6")
	require.NoError(t, err)
	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	resp, _, err := svc.executeKiroUpstream(context.Background(), account, payloadBytes, "claude-sonnet-4-6", "claude-sonnet-4-6", "stale-token", nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	require.Equal(t, 1, repo.setErrorCalls)
	require.Equal(t, account.ID, repo.errorID)
	require.Contains(t, repo.errorMsg, "invalid_grant")
	require.False(t, repo.called, "non-retryable refresh errors should not mark temporary unschedulable")
}

func TestGatewayServiceIsAccountSchedulableForSelectionSkipsActiveKiroCooldown(t *testing.T) {
	now := time.Now().Add(2 * time.Minute)
	svc := &GatewayService{
		kiroCooldownStore: &stubKiroCooldownStore{
			state: &kirocooldown.State{
				Active:        true,
				Reason:        kirocooldown.CooldownReason429,
				CooldownUntil: now,
				Remaining:     2 * time.Minute,
			},
		},
	}

	account := &Account{
		ID:          42,
		Platform:    PlatformKiro,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
	}
	require.False(t, svc.isAccountSchedulableForSelection(account))
}
