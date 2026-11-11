//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestIsSingleAccountRetry_ProbeEvaluatedLazilyOnce(t *testing.T) {
	calls := 0
	ctx := WithSingleAccountRetryProbe(context.Background(), func(context.Context) bool {
		calls++
		return true
	})
	require.Zero(t, calls, "挂载时不应判定")

	require.True(t, isSingleAccountRetry(ctx))
	require.True(t, isSingleAccountRetry(ctx))
	require.Equal(t, 1, calls, "同一请求内只判定一次")
}

func TestIsSingleAccountRetry_ProbeFalse(t *testing.T) {
	ctx := WithSingleAccountRetryProbe(context.Background(), func(context.Context) bool { return false })
	require.False(t, isSingleAccountRetry(ctx))
}

func TestIsSingleAccountRetry_ExplicitFlagOverridesProbe(t *testing.T) {
	calls := 0
	probe := func(context.Context) bool {
		calls++
		return false
	}

	ctx := WithSingleAccountRetryProbe(context.Background(), probe)
	ctx = WithSingleAccountRetry(ctx, true, false)
	require.True(t, isSingleAccountRetry(ctx))

	legacy := context.WithValue(WithSingleAccountRetryProbe(context.Background(), probe), ctxkey.SingleAccountRetry, true)
	require.True(t, isSingleAccountRetry(legacy))

	explicitFalse := WithSingleAccountRetry(WithSingleAccountRetryProbe(context.Background(), func(context.Context) bool {
		calls++
		return true
	}), false, false)
	require.False(t, isSingleAccountRetry(explicitFalse))

	require.Zero(t, calls, "显式标记存在时不应触发判定")
}

func TestIsSingleAccountRetry_ProbeIgnoresCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(WithSingleAccountRetryProbe(context.Background(), func(probeCtx context.Context) bool {
		return probeCtx.Err() == nil
	}))
	cancel()
	require.True(t, isSingleAccountRetry(ctx))
}

func TestIsSingleAccountRetry_ProbeSurvivesMetadataUpdates(t *testing.T) {
	ctx := WithSingleAccountRetryProbe(context.Background(), func(context.Context) bool { return true })
	ctx = WithAccountSwitchCount(ctx, 1, false)
	ctx = WithThinkingEnabled(ctx, true, false)
	require.True(t, isSingleAccountRetry(ctx))
}

// groupRecordingAccountRepo 记录单平台分组查询的分组与平台。
type groupRecordingAccountRepo struct {
	*mockAccountRepoForPlatform
	groupIDs  []int64
	platforms []string
}

func (r *groupRecordingAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error) {
	r.groupIDs = append(r.groupIDs, groupID)
	r.platforms = append(r.platforms, platform)
	return r.mockAccountRepoForPlatform.ListSchedulableByGroupIDAndPlatform(ctx, groupID, platform)
}

func newSingleAntigravityProbeTestService(accounts ...Account) (*GatewayService, *groupRecordingAccountRepo) {
	repo := &groupRecordingAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{accounts: accounts}}
	return &GatewayService{accountRepo: repo}, repo
}

func probeTestAccount(id int64, platform string) Account {
	return Account{ID: id, Platform: platform, Status: StatusActive, Schedulable: true}
}

func TestWithSingleAntigravityAccountGroupProbe_QueriesOnlyWhenRead(t *testing.T) {
	svc, repo := newSingleAntigravityProbeTestService(probeTestAccount(1, PlatformAntigravity))
	groupID := int64(42)

	ctx := svc.WithSingleAntigravityAccountGroupProbe(context.Background(), &groupID)
	require.Empty(t, repo.groupIDs, "挂载时不应查询账号")

	require.True(t, isSingleAccountRetry(ctx))
	require.True(t, isSingleAccountRetry(ctx))
	require.Equal(t, []int64{42}, repo.groupIDs)
	require.Equal(t, []string{PlatformAntigravity}, repo.platforms)
}

func TestWithSingleAntigravityAccountGroupProbe_UsesGroupAtMountTime(t *testing.T) {
	svc, repo := newSingleAntigravityProbeTestService(probeTestAccount(1, PlatformAntigravity))
	groupID := int64(42)

	ctx := svc.WithSingleAntigravityAccountGroupProbe(context.Background(), &groupID)
	groupID = 99

	require.True(t, isSingleAccountRetry(ctx))
	require.Equal(t, []int64{42}, repo.groupIDs)
}

func TestWithSingleAntigravityAccountGroupProbe_CountsOnlyAntigravityAccounts(t *testing.T) {
	groupID := int64(42)
	tests := []struct {
		name     string
		accounts []Account
		want     bool
	}{
		{name: "no antigravity accounts", accounts: []Account{probeTestAccount(1, PlatformGemini)}, want: false},
		{name: "single antigravity account", accounts: []Account{probeTestAccount(1, PlatformAntigravity), probeTestAccount(2, PlatformGemini)}, want: true},
		{name: "two antigravity accounts", accounts: []Account{probeTestAccount(1, PlatformAntigravity), probeTestAccount(2, PlatformAntigravity)}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := newSingleAntigravityProbeTestService(tt.accounts...)
			ctx := svc.WithSingleAntigravityAccountGroupProbe(context.Background(), &groupID)
			require.Equal(t, tt.want, isSingleAccountRetry(ctx))
		})
	}
}

func runRateLimitedPreCheckWithProbe(t *testing.T, single bool) (*antigravityRetryLoopResult, *recordingOKUpstream, error) {
	t.Helper()
	upstream := &recordingOKUpstream{}
	account := &Account{
		ID:          30,
		Type:        AccountTypeOAuth,
		Platform:    PlatformAntigravity,
		Schedulable: true,
		Status:      StatusActive,
		Concurrency: 1,
		Extra: map[string]any{
			modelRateLimitsKey: map[string]any{
				"claude-sonnet-4-5": map[string]any{
					"rate_limit_reset_at": time.Now().Add(30 * time.Second).Format(time.RFC3339),
				},
			},
		},
	}
	ctx := WithSingleAccountRetryProbe(context.Background(), func(context.Context) bool { return single })
	svc := &AntigravityGatewayService{}
	result, err := svc.antigravityRetryLoop(antigravityRetryLoopParams{
		ctx:            ctx,
		prefix:         "[test]",
		account:        account,
		accessToken:    "token",
		action:         "generateContent",
		body:           []byte(`{"input":"test"}`),
		httpUpstream:   upstream,
		requestedModel: "claude-sonnet-4-5",
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	})
	return result, upstream, err
}

func TestAntigravityRetryLoop_PreCheck_ProbeSingleAccount_SkipsRateLimit(t *testing.T) {
	result, upstream, err := runRateLimitedPreCheckWithProbe(t, true)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.resp)
	require.Equal(t, http.StatusOK, result.resp.StatusCode)
	require.Equal(t, 1, upstream.calls)
}

func TestAntigravityRetryLoop_PreCheck_ProbeMultiAccount_SwitchesOnRateLimit(t *testing.T) {
	result, upstream, err := runRateLimitedPreCheckWithProbe(t, false)

	require.Nil(t, result)
	var switchErr *AntigravityAccountSwitchError
	require.ErrorAs(t, err, &switchErr)
	require.Equal(t, "claude-sonnet-4-5", switchErr.RateLimitedModel)
	require.Zero(t, upstream.calls)
}
