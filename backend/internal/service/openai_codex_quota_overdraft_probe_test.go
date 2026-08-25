//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type codexOverdraftProbeRepoStub struct {
	AccountRepository
	account         *Account
	states          []*CodexQuotaOverdraftProbeState
	tempPauseCalls  int
	clearTempCalls  int
	clearLimitCalls int
}

type codexOverdraftRuntimeBlockerStub struct {
	clearCalls int
}

type codexOverdraftClaimConflictRepoStub struct {
	*codexOverdraftProbeRepoStub
}

func (r *codexOverdraftClaimConflictRepoStub) ClaimCodexQuotaOverdraftProbe(
	context.Context,
	int64,
	*CodexQuotaOverdraftProbeState,
) (bool, error) {
	return false, nil
}

func (b *codexOverdraftRuntimeBlockerStub) BlockAccountScheduling(*Account, time.Time, string) {}

func (b *codexOverdraftRuntimeBlockerStub) ClearAccountSchedulingBlock(int64) {
	b.clearCalls++
}

func (r *codexOverdraftProbeRepoStub) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}

func (r *codexOverdraftProbeRepoStub) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	if r.account.Extra == nil {
		r.account.Extra = make(map[string]any)
	}
	for key, value := range updates {
		r.account.Extra[key] = value
	}
	if state, ok := updates[CodexQuotaOverdraftProbeExtraKey].(*CodexQuotaOverdraftProbeState); ok {
		clone := *state
		r.states = append(r.states, &clone)
	}
	return nil
}

func (r *codexOverdraftProbeRepoStub) SetTempUnschedulable(_ context.Context, _ int64, until time.Time, reason string) error {
	r.tempPauseCalls++
	r.account.TempUnschedulableUntil = codexQuotaOverdraftTimePtr(until)
	r.account.TempUnschedulableReason = reason
	return nil
}

func (r *codexOverdraftProbeRepoStub) ClearTempUnschedulable(context.Context, int64) error {
	r.clearTempCalls++
	r.account.TempUnschedulableUntil = nil
	r.account.TempUnschedulableReason = ""
	return nil
}

func (r *codexOverdraftProbeRepoStub) ClearRateLimit(context.Context, int64) error {
	r.clearLimitCalls++
	r.account.RateLimitResetAt = nil
	return nil
}

func newCodexOverdraftProbeTestAccount(now time.Time) *Account {
	return &Account{
		ID:          77,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"codex_5h_used_percent": 100,
			"codex_5h_reset_at":     now.Add(5 * time.Hour).Format(time.RFC3339),
		},
	}
}

func TestCodexQuotaOverdraftProbeUsesOnePreferredModel(t *testing.T) {
	models := codexQuotaOverdraftProbeModels("gpt-5.4")
	require.Equal(t, []string{"gpt-5.4", "gpt-5.5", "gpt-5.4-mini"}, models)

	got := make([]string, 0, codexQuotaOverdraftProbeAttemptLimit)
	for attempt := 0; attempt < codexQuotaOverdraftProbeAttemptLimit; attempt++ {
		got = append(got, models[attempt%len(models)])
	}
	require.Equal(t, []string{"gpt-5.4"}, got)
}

func TestCodexQuotaOverdraftSignalsKeepFiveHourAndSevenDayCyclesSeparate(t *testing.T) {
	now := time.Date(2026, time.August, 13, 14, 0, 0, 0, time.UTC)
	fiveReset := now.Add(5 * time.Hour)
	sevenReset := now.Add(6 * 24 * time.Hour)
	account := newCodexOverdraftProbeTestAccount(now)

	five, exhausted := codexQuotaOverdraftSignalFromAccount(account, nil, now)
	require.True(t, exhausted)
	require.Equal(t, "5h", five.Window)
	require.Equal(t, "5h:"+formatCodexOverdraftUnix(fiveReset), five.CycleKey)
	require.WithinDuration(t, fiveReset, five.RecoverAt, time.Second)

	account.Extra["codex_7d_used_percent"] = 100
	account.Extra["codex_7d_reset_at"] = sevenReset.Format(time.RFC3339)
	multiple, exhausted := codexQuotaOverdraftSignalFromAccount(account, nil, now)
	require.True(t, exhausted)
	require.Equal(t, "multiple", multiple.Window)
	require.WithinDuration(t, sevenReset, multiple.RecoverAt, time.Second, "多个额度周期必须等待最晚恢复时间")
	require.Contains(t, multiple.CycleKey, "5h:")
	require.Contains(t, multiple.CycleKey, "|7d:")
}

func TestCodexQuotaOverdraftSingleProbePassesOnSuccess(t *testing.T) {
	now := time.Date(2026, time.August, 13, 14, 0, 0, 0, time.UTC)
	account := newCodexOverdraftProbeTestAccount(now)
	repo := &codexOverdraftProbeRepoStub{account: account}
	coordinator := &CodexQuotaOverdraftCoordinator{accountRepo: repo, now: func() time.Time { return now }}
	models := make([]string, 0, 1)
	coordinator.probeAttemptForTest = func(_ context.Context, _ *Account, model string) codexQuotaOverdraftProbeResult {
		models = append(models, model)
		return codexQuotaOverdraftProbeResult{Status: "available", ReasonCode: "model_response_ok", StatusCode: http.StatusOK, Model: model}
	}
	signal, exhausted := codexQuotaOverdraftSignalFromAccount(account, nil, now)
	require.True(t, exhausted)
	state := newCodexOverdraftPendingState(signal, now)

	coordinator.runProbePlan(account.ID, signal, "gpt-5.4", state)

	require.Equal(t, codexQuotaOverdraftProbePassed, state.Status)
	require.Equal(t, 1, state.Attempts)
	require.Equal(t, []string{"gpt-5.4"}, models)
	require.NotNil(t, state.FiveHourStartedAt)
	require.Zero(t, repo.tempPauseCalls)
}

func TestCodexQuotaOverdraftSingleProbeQuotaFailurePauses(t *testing.T) {
	now := time.Date(2026, time.August, 13, 14, 0, 0, 0, time.UTC)
	account := newCodexOverdraftProbeTestAccount(now)
	repo := &codexOverdraftProbeRepoStub{account: account}
	coordinator := &CodexQuotaOverdraftCoordinator{accountRepo: repo, now: func() time.Time { return now }}
	models := make([]string, 0, codexQuotaOverdraftProbeAttemptLimit)
	coordinator.probeAttemptForTest = func(_ context.Context, _ *Account, model string) codexQuotaOverdraftProbeResult {
		models = append(models, model)
		return codexQuotaOverdraftProbeResult{Status: "retry", ReasonCode: "quota_limited", StatusCode: http.StatusTooManyRequests, Model: model}
	}
	signal, _ := codexQuotaOverdraftSignalFromAccount(account, nil, now)
	state := newCodexOverdraftPendingState(signal, now)

	coordinator.runProbePlan(account.ID, signal, "gpt-5.4", state)

	require.Equal(t, codexQuotaOverdraftProbeFailed, state.Status)
	require.Equal(t, codexQuotaOverdraftProbeAttemptLimit, state.Attempts)
	require.Len(t, models, codexQuotaOverdraftProbeAttemptLimit)
	require.Equal(t, 1, repo.tempPauseCalls)
	require.True(t, codexQuotaOverdraftPauseReason(account.TempUnschedulableReason))
	require.False(t, codexQuotaOverdraftSchedulingAllowed(account, now), "同周期确认失败后不得继续绕过限额")
}

func TestCodexQuotaOverdraftBusinessSuccessAtExhaustionPassesWithoutProbe(t *testing.T) {
	now := time.Date(2026, time.August, 13, 14, 0, 0, 0, time.UTC)
	account := newCodexOverdraftProbeTestAccount(now)
	repo := &codexOverdraftProbeRepoStub{account: account}
	coordinator := &CodexQuotaOverdraftCoordinator{accountRepo: repo, now: func() time.Time { return now }}

	coordinator.observeBusinessSuccess(account, "gpt-5.4")

	state, ok := codexQuotaOverdraftStateFromAccount(account)
	require.True(t, ok)
	require.Equal(t, codexQuotaOverdraftProbePassed, state.Status)
	require.Equal(t, "business_request_ok", state.ReasonCode)
	require.Equal(t, "gpt-5.4", state.Model)
	require.Equal(t, 1, state.Attempts)
	require.Equal(t, 1, state.Limit)
	require.NotNil(t, state.FiveHourStartedAt)
}

func TestCodexQuotaOverdraftBusinessSuccessCannotReplaceFailedCycle(t *testing.T) {
	now := time.Date(2026, time.August, 13, 14, 0, 0, 0, time.UTC)
	account := newCodexOverdraftProbeTestAccount(now)
	signal, _ := codexQuotaOverdraftSignalFromAccount(account, nil, now)
	failed := newCodexOverdraftPendingState(signal, now)
	failed.Status = codexQuotaOverdraftProbeFailed
	failed.ReasonCode = "business_quota_limited"
	account.Extra[CodexQuotaOverdraftProbeExtraKey] = failed
	repo := &codexOverdraftProbeRepoStub{account: account}
	coordinator := &CodexQuotaOverdraftCoordinator{accountRepo: repo, now: func() time.Time { return now }}

	coordinator.observeBusinessSuccess(account, "gpt-5.4")

	state, ok := codexQuotaOverdraftStateFromAccount(account)
	require.True(t, ok)
	require.Equal(t, codexQuotaOverdraftProbeFailed, state.Status)
	require.Empty(t, repo.states)
}

func TestCodexQuotaOverdraftInjectedBusinessQuota429FailsImmediately(t *testing.T) {
	now := time.Date(2026, time.August, 13, 14, 0, 0, 0, time.UTC)
	account := newCodexOverdraftProbeTestAccount(now)
	repo := &codexOverdraftProbeRepoStub{account: account}
	coordinator := &CodexQuotaOverdraftCoordinator{
		accountRepo:  repo,
		httpUpstream: &queuedHTTPUpstream{},
		cfg: &config.Config{Gateway: config.GatewayConfig{
			CodexQuotaOverdraftEnabled: true,
		}},
		now: func() time.Time { return now },
	}
	ctx := WithCodexQuotaOverdraftScheduling(context.Background())
	markCodexQuotaOverdraftInjected(ctx, account.ID)

	handled := coordinator.HandleQuota429(
		ctx,
		account,
		nil,
		[]byte(`{"error":{"type":"usage_limit_reached"}}`),
		"gpt-5.4",
	)

	require.True(t, handled)
	state, ok := codexQuotaOverdraftStateFromAccount(account)
	require.True(t, ok)
	require.Equal(t, codexQuotaOverdraftProbeFailed, state.Status)
	require.Equal(t, "business_quota_limited", state.ReasonCode)
	require.Equal(t, 1, repo.tempPauseCalls)
}

func TestCodexQuotaOverdraftTransient429DoesNotEnterQuotaCooldown(t *testing.T) {
	now := time.Date(2026, time.August, 13, 14, 0, 0, 0, time.UTC)
	account := newCodexOverdraftProbeTestAccount(now)
	repo := &codexOverdraftProbeRepoStub{account: account}
	coordinator := &CodexQuotaOverdraftCoordinator{
		accountRepo:  repo,
		httpUpstream: &queuedHTTPUpstream{},
		cfg: &config.Config{Gateway: config.GatewayConfig{
			CodexQuotaOverdraftEnabled: true,
		}},
		now: func() time.Time { return now },
	}
	ctx := WithCodexQuotaOverdraftScheduling(context.Background())
	markCodexQuotaOverdraftInjected(ctx, account.ID)
	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "100")
	headers.Set("x-codex-primary-reset-after-seconds", "3600")
	headers.Set("x-codex-primary-window-minutes", "300")

	handled := coordinator.HandleQuota429(
		ctx,
		account,
		headers,
		[]byte(`{"error":{"type":"rate_limit_exceeded","message":"too many requests"}}`),
		"gpt-5.4",
	)

	require.False(t, handled)
	require.Zero(t, repo.tempPauseCalls)
	_, hasState := codexQuotaOverdraftStateFromAccount(account)
	require.False(t, hasState)
}

func TestCodexQuotaOverdraftBusinessFailureReloadsAfterClaimConflict(t *testing.T) {
	now := time.Date(2026, time.August, 13, 14, 0, 0, 0, time.UTC)
	latest := newCodexOverdraftProbeTestAccount(now)
	signal, _ := codexQuotaOverdraftSignalFromAccount(latest, nil, now)
	passed := newCodexOverdraftPendingState(signal, now)
	passed.Status = codexQuotaOverdraftProbePassed
	latest.Extra[CodexQuotaOverdraftProbeExtraKey] = passed
	baseRepo := &codexOverdraftProbeRepoStub{account: latest}
	repo := &codexOverdraftClaimConflictRepoStub{codexOverdraftProbeRepoStub: baseRepo}
	coordinator := &CodexQuotaOverdraftCoordinator{accountRepo: repo, now: func() time.Time { return now }}
	stale := newCodexOverdraftProbeTestAccount(now)
	delete(stale.Extra, CodexQuotaOverdraftProbeExtraKey)

	handled := coordinator.finishBusinessQuotaFailure(stale, signal, "gpt-5.4")

	require.True(t, handled)
	state, ok := codexQuotaOverdraftStateFromAccount(latest)
	require.True(t, ok)
	require.Equal(t, codexQuotaOverdraftProbeFailed, state.Status)
	require.Equal(t, 1, baseRepo.tempPauseCalls)
}

func TestCodexQuotaOverdraftFailedPauseIsIdempotentForPersistedCycle(t *testing.T) {
	now := time.Date(2026, time.August, 13, 14, 0, 0, 0, time.UTC)
	account := newCodexOverdraftProbeTestAccount(now)
	recoverAt := now.Add(5 * time.Hour)
	state := &CodexQuotaOverdraftProbeState{
		Status:      codexQuotaOverdraftProbeFailed,
		QuotaWindow: "5h",
		CycleKey:    "5h:" + formatCodexOverdraftUnix(recoverAt),
		Attempts:    codexQuotaOverdraftProbeAttemptLimit,
		Limit:       codexQuotaOverdraftProbeAttemptLimit,
		StartedAt:   now,
		RecoverAt:   codexQuotaOverdraftTimePtr(recoverAt),
	}
	account.Extra[CodexQuotaOverdraftProbeExtraKey] = state
	account.TempUnschedulableUntil = codexQuotaOverdraftTimePtr(recoverAt)
	account.TempUnschedulableReason = BuildTempUnschedReasonPayload(codexQuotaOverdraftPauseSource, "quota exhausted")
	repo := &codexOverdraftProbeRepoStub{account: account}
	coordinator := &CodexQuotaOverdraftCoordinator{accountRepo: repo, now: func() time.Time { return now }}

	require.True(t, coordinator.ensureFailedPause(account, state))
	require.Zero(t, repo.tempPauseCalls)
	require.Empty(t, repo.states)
}

func TestCodexQuotaOverdraftProbeInconclusiveNeverPauses(t *testing.T) {
	now := time.Date(2026, time.August, 13, 14, 0, 0, 0, time.UTC)
	for _, result := range []codexQuotaOverdraftProbeResult{
		{Status: "inconclusive", ReasonCode: "request_timeout", StatusCode: http.StatusGatewayTimeout, Model: "gpt-5.4"},
		{Status: "inconclusive", ReasonCode: "upstream_unavailable", StatusCode: http.StatusServiceUnavailable, Model: "gpt-5.4"},
	} {
		t.Run(result.ReasonCode, func(t *testing.T) {
			account := newCodexOverdraftProbeTestAccount(now)
			repo := &codexOverdraftProbeRepoStub{account: account}
			coordinator := &CodexQuotaOverdraftCoordinator{accountRepo: repo, now: func() time.Time { return now }}
			coordinator.probeAttemptForTest = func(context.Context, *Account, string) codexQuotaOverdraftProbeResult { return result }
			signal, _ := codexQuotaOverdraftSignalFromAccount(account, nil, now)
			state := newCodexOverdraftPendingState(signal, now)

			coordinator.runProbePlan(account.ID, signal, "gpt-5.4", state)

			require.Equal(t, codexQuotaOverdraftProbeInconclusive, state.Status)
			require.Equal(t, 1, state.Attempts)
			require.Nil(t, state.RetryAt)
			require.Zero(t, state.RetryCount)
			require.Zero(t, repo.tempPauseCalls)
		})
	}
}

func TestCodexQuotaOverdraftModelNotFoundIsInconclusiveWithoutRetry(t *testing.T) {
	now := time.Date(2026, time.August, 13, 14, 0, 0, 0, time.UTC)
	account := newCodexOverdraftProbeTestAccount(now)
	repo := &codexOverdraftProbeRepoStub{account: account}
	coordinator := &CodexQuotaOverdraftCoordinator{accountRepo: repo, now: func() time.Time { return now }}
	models := make([]string, 0, codexQuotaOverdraftProbeAttemptLimit)
	coordinator.probeAttemptForTest = func(_ context.Context, _ *Account, model string) codexQuotaOverdraftProbeResult {
		models = append(models, model)
		return codexQuotaOverdraftProbeResult{Status: "retry", ReasonCode: "model_not_found", StatusCode: http.StatusNotFound, Model: model}
	}
	signal, _ := codexQuotaOverdraftSignalFromAccount(account, nil, now)
	state := newCodexOverdraftPendingState(signal, now)

	coordinator.runProbePlan(account.ID, signal, "gpt-5.4", state)

	require.Equal(t, codexQuotaOverdraftProbeInconclusive, state.Status)
	require.Equal(t, codexQuotaOverdraftProbeAttemptLimit, state.Attempts)
	require.Zero(t, state.RetryCount)
	require.Nil(t, state.RetryAt)
	require.Equal(t, []string{"gpt-5.4"}, models)
	require.Zero(t, repo.tempPauseCalls)
}

func TestCodexQuotaOverdraftNewWindowPreservesExistingWindowBaseline(t *testing.T) {
	now := time.Date(2026, time.August, 13, 14, 0, 0, 0, time.UTC)
	fiveStarted := now.Add(-time.Hour)
	fiveRecover := now.Add(4 * time.Hour)
	sevenRecover := now.Add(6 * 24 * time.Hour)
	current := &CodexQuotaOverdraftProbeState{
		Status:            codexQuotaOverdraftProbePassed,
		QuotaWindow:       "5h",
		CycleKey:          "5h:" + formatCodexOverdraftUnix(fiveRecover),
		FiveHourRecoverAt: codexQuotaOverdraftTimePtr(fiveRecover),
		FiveHourStartedAt: codexQuotaOverdraftTimePtr(fiveStarted),
	}
	signal := codexQuotaOverdraftSignal{
		Window:            "multiple",
		CycleKey:          "5h:" + formatCodexOverdraftUnix(fiveRecover) + "|7d:" + formatCodexOverdraftUnix(sevenRecover),
		RecoverAt:         sevenRecover,
		FiveHourRecoverAt: codexQuotaOverdraftTimePtr(fiveRecover),
		SevenDayRecoverAt: codexQuotaOverdraftTimePtr(sevenRecover),
	}
	target := newCodexOverdraftPendingState(signal, now)
	carryCodexQuotaOverdraftWindowStarts(target, current, signal, now)
	startCodexQuotaOverdraftWindows(target, signal, now)

	require.NotNil(t, target.FiveHourStartedAt)
	require.True(t, target.FiveHourStartedAt.Equal(fiveStarted), "已有 5h 透支统计起点不能因 7d 后续耗尽而重置")
	require.NotNil(t, target.SevenDayStartedAt)
	require.True(t, target.SevenDayStartedAt.Equal(now))
}

func TestCodexQuotaOverdraftFailedMultipleCycleStillCoversRemainingWindow(t *testing.T) {
	now := time.Date(2026, time.August, 13, 14, 0, 0, 0, time.UTC)
	fiveRecover := now.Add(5 * time.Hour)
	sevenRecover := now.Add(6 * 24 * time.Hour)
	state := &CodexQuotaOverdraftProbeState{
		Status:            codexQuotaOverdraftProbeFailed,
		QuotaWindow:       "multiple",
		CycleKey:          "5h:" + formatCodexOverdraftUnix(fiveRecover) + "|7d:" + formatCodexOverdraftUnix(sevenRecover),
		FiveHourRecoverAt: codexQuotaOverdraftTimePtr(fiveRecover),
		SevenDayRecoverAt: codexQuotaOverdraftTimePtr(sevenRecover),
	}
	remaining := codexQuotaOverdraftSignal{
		Window:            "7d",
		CycleKey:          "7d:" + formatCodexOverdraftUnix(sevenRecover),
		RecoverAt:         sevenRecover,
		SevenDayRecoverAt: codexQuotaOverdraftTimePtr(sevenRecover),
	}

	require.True(t, codexQuotaOverdraftStateCoversSignal(state, remaining))
}

func TestCodexQuotaOverdraftRecoveryClearsWindowState(t *testing.T) {
	now := time.Date(2026, time.August, 13, 14, 0, 0, 0, time.UTC)
	account := newCodexOverdraftProbeTestAccount(now)
	account.Extra["codex_5h_used_percent"] = 0
	started := now.Add(-time.Hour)
	recoverAt := now.Add(4 * time.Hour)
	state := &CodexQuotaOverdraftProbeState{
		Status:             codexQuotaOverdraftProbePassed,
		QuotaWindow:        "5h",
		CycleKey:           "5h:" + formatCodexOverdraftUnix(recoverAt),
		RecoverAt:          codexQuotaOverdraftTimePtr(recoverAt),
		FiveHourRecoverAt:  codexQuotaOverdraftTimePtr(recoverAt),
		OverdraftStartedAt: codexQuotaOverdraftTimePtr(started),
		FiveHourStartedAt:  codexQuotaOverdraftTimePtr(started),
	}
	account.Extra[CodexQuotaOverdraftProbeExtraKey] = state
	repo := &codexOverdraftProbeRepoStub{account: account}
	coordinator := &CodexQuotaOverdraftCoordinator{accountRepo: repo, now: func() time.Time { return now }}

	coordinator.observeAccount(account, "gpt-5.4")

	recovered, ok := codexQuotaOverdraftStateFromAccount(account)
	require.True(t, ok)
	require.Equal(t, codexQuotaOverdraftProbeRecovered, recovered.Status)
	require.Equal(t, "quota_recovered", recovered.ReasonCode)
	require.Nil(t, recovered.OverdraftStartedAt)
	require.Nil(t, recovered.FiveHourStartedAt)
	require.Nil(t, recovered.FiveHourRecoverAt)
	require.Zero(t, repo.tempPauseCalls)
}

func TestCodexQuotaOverdraftAvailableStateClearsMismatchedStaleRateLimit(t *testing.T) {
	now := time.Date(2026, time.August, 14, 10, 0, 0, 0, time.UTC)
	for _, status := range []string{codexQuotaOverdraftProbePassed, codexQuotaOverdraftProbeRecovered} {
		t.Run(status, func(t *testing.T) {
			account := newCodexOverdraftProbeTestAccount(now)
			account.RateLimitedAt = codexQuotaOverdraftTimePtr(now.Add(-time.Minute))
			account.RateLimitResetAt = codexQuotaOverdraftTimePtr(now.Add(2 * time.Hour))
			repo := &codexOverdraftProbeRepoStub{account: account}
			blocker := &codexOverdraftRuntimeBlockerStub{}
			coordinator := &CodexQuotaOverdraftCoordinator{accountRepo: repo, runtimeBlocker: blocker}
			signal, _ := codexQuotaOverdraftSignalFromAccount(account, nil, now)
			state := &CodexQuotaOverdraftProbeState{
				Status:            status,
				CycleKey:          signal.CycleKey,
				QuotaWindow:       signal.Window,
				ObservedRateLimit: codexQuotaOverdraftTimePtr(now.Add(time.Hour)),
			}
			account.Extra[CodexQuotaOverdraftProbeExtraKey] = state

			coordinator.clearQuotaPause(account.ID, state)

			require.Equal(t, 1, repo.clearLimitCalls)
			require.Nil(t, account.RateLimitResetAt)
			require.Equal(t, 1, blocker.clearCalls)
		})
	}
}

func TestCodexQuotaOverdraftFailedStateDoesNotClearRateLimit(t *testing.T) {
	now := time.Date(2026, time.August, 14, 10, 0, 0, 0, time.UTC)
	account := newCodexOverdraftProbeTestAccount(now)
	account.RateLimitResetAt = codexQuotaOverdraftTimePtr(now.Add(2 * time.Hour))
	repo := &codexOverdraftProbeRepoStub{account: account}
	blocker := &codexOverdraftRuntimeBlockerStub{}
	coordinator := &CodexQuotaOverdraftCoordinator{accountRepo: repo, runtimeBlocker: blocker}
	state := &CodexQuotaOverdraftProbeState{
		Status:            codexQuotaOverdraftProbeFailed,
		ObservedRateLimit: codexQuotaOverdraftTimePtr(now.Add(time.Hour)),
	}

	coordinator.clearQuotaPause(account.ID, state)

	require.Zero(t, repo.clearLimitCalls)
	require.NotNil(t, account.RateLimitResetAt)
	require.Zero(t, blocker.clearCalls)
}

func TestClassifyCodexQuotaOverdraftProbeResponses(t *testing.T) {
	status, reason := classifyCodexQuotaOverdraftProbe(http.StatusOK, nil, []byte(`data: {"type":"response.completed"}`))
	require.Equal(t, "available", status)
	require.Equal(t, "model_response_ok", reason)

	status, reason = classifyCodexQuotaOverdraftProbe(http.StatusTooManyRequests, nil, []byte(`{"error":{"type":"usage_limit_reached"}}`))
	require.Equal(t, "retry", status)
	require.Equal(t, "quota_limited", reason)

	status, reason = classifyCodexQuotaOverdraftProbe(http.StatusTooManyRequests, nil, []byte(`{"error":{"code":"weekly_limit_reached"}}`))
	require.Equal(t, "retry", status)
	require.Equal(t, "quota_limited", reason)

	status, reason = classifyCodexQuotaOverdraftProbe(http.StatusTooManyRequests, nil, []byte(`{"rate_limit":{"allowed":false,"limit_reached":true}}`))
	require.Equal(t, "retry", status)
	require.Equal(t, "quota_limited", reason)

	status, reason = classifyCodexQuotaOverdraftProbe(http.StatusTooManyRequests, nil, []byte(`{"error":{"type":"rate_limit_exceeded","message":"too many requests"}}`))
	require.Equal(t, "inconclusive", status)
	require.Equal(t, "transient_failure", reason)

	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "100")
	headers.Set("x-codex-primary-reset-after-seconds", "3600")
	headers.Set("x-codex-primary-window-minutes", "300")
	status, reason = classifyCodexQuotaOverdraftProbe(http.StatusTooManyRequests, headers, []byte(`{"error":{"type":"rate_limit_exceeded","message":"too many requests"}}`))
	require.Equal(t, "inconclusive", status)
	require.Equal(t, "transient_failure", reason)

	status, reason = classifyCodexQuotaOverdraftProbe(http.StatusServiceUnavailable, nil, nil)
	require.Equal(t, "inconclusive", status)
	require.Equal(t, "upstream_unavailable", reason)

	status, reason = classifyCodexQuotaOverdraftProbe(http.StatusUnauthorized, nil, nil)
	require.Equal(t, "authentication_failed", status)
	require.Equal(t, "authentication_failed", reason)

	status, reason = classifyCodexQuotaOverdraftProbe(http.StatusNotFound, nil, []byte(`{"detail":"model not found"}`))
	require.Equal(t, "retry", status)
	require.Equal(t, "model_not_found", reason)

	status, reason = classifyCodexQuotaOverdraftProbe(http.StatusOK, nil, []byte(`data: {"type":"response.failed"}\ndata: {"type":"response.completed"}`))
	require.Equal(t, "retry", status)
	require.Equal(t, "invalid_response", reason)
}

func TestCodexQuotaOverdraftProbePayloadAcceptsInjection(t *testing.T) {
	payload := map[string]any{
		"model": "gpt-5.4",
		"input": []map[string]any{{
			"type":    "message",
			"role":    "user",
			"content": []map[string]string{{"type": "input_text", "text": "hi"}},
		}},
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	updated, changed, err := injectCodexQuotaOverdraft(raw)
	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, string(updated), `"custom_tool_call"`)
}

func newCodexOverdraftPendingState(signal codexQuotaOverdraftSignal, now time.Time) *CodexQuotaOverdraftProbeState {
	return &CodexQuotaOverdraftProbeState{
		Status:             codexQuotaOverdraftProbePending,
		QuotaWindow:        signal.Window,
		CycleKey:           signal.CycleKey,
		Limit:              codexQuotaOverdraftProbeAttemptLimit,
		StartedAt:          now,
		RecoverAt:          codexQuotaOverdraftTimePtr(signal.RecoverAt),
		FiveHourRecoverAt:  cloneTimePtr(signal.FiveHourRecoverAt),
		SevenDayRecoverAt:  cloneTimePtr(signal.SevenDayRecoverAt),
		OverdraftStartedAt: nil,
	}
}

func formatCodexOverdraftUnix(value time.Time) string {
	return strconv.FormatInt(value.Unix(), 10)
}
