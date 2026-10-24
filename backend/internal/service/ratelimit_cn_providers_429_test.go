//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// 上游临时过载（overloaded_error / server is busy）与配额窗口无关：不得进
// opencode_go / Coding Plan 的快照选窗冷却（否则一次过载会被停调到数天后的
// 窗口重置点），必须返回 false 落默认 429 短冷却。
func TestApplyCNProviderReactive429_OverloadedFallsBackToDefault(t *testing.T) {
	t.Parallel()
	futureWeekly := time.Now().Add(3 * 24 * time.Hour).Format(time.RFC3339)
	bodies := []string{
		`{"type":"error","error":{"type":"overloaded_error","message":"server is busy, please retry later"}}`,
		`{"error":{"message":"Server is busy, please retry later"}}`,
	}
	accounts := []*Account{
		{
			Platform:    PlatformMiniMax,
			Credentials: map[string]any{"account_mode": AccountModeCoding},
			Extra:       map[string]any{"minimax_weekly_reset_at": futureWeekly},
		},
		{
			Platform: PlatformOpenCodeGo,
			Extra:    map[string]any{"opencode_go_weekly_reset_at": futureWeekly},
		},
	}
	for i, account := range accounts {
		for j, body := range bodies {
			repo := &rateLimitAccountRepoStub{}
			svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			handled := svc.applyCNProviderReactive429(context.Background(), account, http.Header{}, []byte(body))
			require.False(t, handled, "account#%d body#%d", i, j)
			require.Zero(t, repo.rateLimitedCalls, "account#%d body#%d", i, j)
			require.Zero(t, repo.tempCalls, "account#%d body#%d", i, j)
		}
	}
}

// 非 overloaded 语义的 429 仍按快照选窗冷却：修复不得把真配额限流放行。
func TestApplyCNProviderReactive429_Quota429StillCoolsToSnapshotReset(t *testing.T) {
	t.Parallel()
	resetAt := time.Now().Add(90 * time.Minute).Truncate(time.Second)
	repo := &rateLimitAccountRepoStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{
		Platform:    PlatformMiniMax,
		Credentials: map[string]any{"account_mode": AccountModeCoding},
		Extra:       map[string]any{"minimax_5h_reset_at": resetAt.Format(time.RFC3339)},
	}
	body := []byte(`{"error":{"type":"rate_limit_error","message":"usage limit reached"}}`)

	require.True(t, svc.applyCNProviderReactive429(context.Background(), account, http.Header{}, body))
	require.Equal(t, 1, repo.rateLimitedCalls)
	require.WithinDuration(t, resetAt, repo.lastRateLimitedAt, time.Minute)
}

// 过载识别签名收紧：确定性过载信号才命中，真配额限流/余额文案不得误判。
func TestCNProviderResponseIndicatesOverloaded(t *testing.T) {
	t.Parallel()
	require.True(t, cnProviderResponseIndicatesOverloaded(
		[]byte(`{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`)))
	require.True(t, cnProviderResponseIndicatesOverloaded(
		[]byte(`{"error":{"message":"server is busy, please retry later"}}`)))
	require.False(t, cnProviderResponseIndicatesOverloaded(
		[]byte(`{"error":{"type":"rate_limit_error","message":"usage limit reached"}}`)))
	require.False(t, cnProviderResponseIndicatesOverloaded(
		[]byte(`{"error":{"message":"insufficient balance"}}`)))
	require.False(t, cnProviderResponseIndicatesOverloaded(nil))
}
