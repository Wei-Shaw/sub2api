//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// 智谱实测 1305：HTTP 529 与 429 都出现过（同一错误体），语义是
// 「该模型当前访问量过大，请您稍后再试」，与账号额度无关。
const zhipuModelOverloadedBody = `{"error":{"code":"1305","message":"[1305][该模型当前访问量过大，请您稍后再试][20261123201526f68ca5b7053543dc]","type":"overloaded_error"},"request_id":"20261123201526f68ca5b7053543dc","type":"error"}`

type modelOverloadAccountRepoStub struct {
	rateLimitAccountRepoStub
	overloadCalls     int
	lastOverloadUntil time.Time
	modelLimitCalls   int
	lastModelScope    string
	lastModelResetAt  time.Time
	lastModelReason   string
	modelLimitErr     error
}

func (r *modelOverloadAccountRepoStub) SetOverloaded(_ context.Context, _ int64, until time.Time) error {
	r.overloadCalls++
	r.lastOverloadUntil = until
	return nil
}

func (r *modelOverloadAccountRepoStub) SetModelRateLimit(_ context.Context, _ int64, scope string, resetAt time.Time, reason ...string) error {
	r.modelLimitCalls++
	r.lastModelScope = scope
	r.lastModelResetAt = resetAt
	if len(reason) > 0 {
		r.lastModelReason = reason[0]
	}
	return r.modelLimitErr
}

func newModelOverloadService(repo AccountRepository) *RateLimitService {
	return NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
}

func TestCNProviderResponseIndicatesModelOverload(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"zhipu 1305 with overloaded_error type", zhipuModelOverloadedBody, true},
		{"zhipu 1305 without type", `{"error":{"code":"1305","message":"该模型当前访问量过大，请您稍后再试"}}`, true},
		{"overloaded_error type only", `{"error":{"type":"overloaded_error","message":"overloaded"}}`, true},
		{"wording only", `{"error":{"message":"该模型当前访问量过大"}}`, true},
		{"frequency 1302", zhipuFrequencyLimitedBody, false},
		{"quota exhaustion", `{"error":{"message":"本月额度已用尽，请升级套餐"}}`, false},
		{"insufficient balance", `{"error":{"message":"余额不足，请充值"}}`, false},
		{"empty body", ``, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, cnProviderResponseIndicatesModelOverload([]byte(tc.body)))
		})
	}
}

// 529 + 1305：只写模型级短冷却，不写账号级 overload_until。
func TestHandleUpstreamError_Zhipu529ModelOverloadUsesModelCooldown(t *testing.T) {
	repo := &modelOverloadAccountRepoStub{}
	svc := newModelOverloadService(repo)
	account := newZhipuCodingPlanAccount(1, 15, 3*time.Hour)

	disabled := svc.HandleUpstreamError(
		context.Background(), account, 529, http.Header{}, []byte(zhipuModelOverloadedBody), "glm-5.4")

	require.False(t, disabled)
	require.Zero(t, repo.overloadCalls, "zhipu 1305 must not park the whole account")
	require.Zero(t, repo.rateLimitedCalls)
	require.Zero(t, repo.tempCalls)
	require.Equal(t, 1, repo.modelLimitCalls)
	require.Equal(t, "glm-5.4", repo.lastModelScope)
	require.Equal(t, zhipuModelOverloadReason, repo.lastModelReason)
	require.WithinDuration(t, time.Now().Add(zhipuModelOverloadCooldown), repo.lastModelResetAt, 5*time.Second,
		"model overload must cool down minutes, not to the 5h/weekly window reset")
}

// 429 + 1305：同样只写模型级短冷却，绝不落到按快照选窗的窗口冷却。
func TestHandleUpstreamError_Zhipu429ModelOverloadUsesModelCooldown(t *testing.T) {
	repo := &modelOverloadAccountRepoStub{}
	svc := newModelOverloadService(repo)
	account := newZhipuCodingPlanAccount(1, 15, 2*time.Hour)

	disabled := svc.HandleUpstreamError(
		context.Background(), account, http.StatusTooManyRequests, http.Header{}, []byte(zhipuModelOverloadedBody), "glm-5.4")

	require.False(t, disabled)
	require.Zero(t, repo.rateLimitedCalls, "1305 must never cool down to the quota snapshot reset")
	require.Zero(t, repo.tempCalls)
	require.Equal(t, 1, repo.modelLimitCalls)
	require.Equal(t, "glm-5.4", repo.lastModelScope)
	require.WithinDuration(t, time.Now().Add(zhipuModelOverloadCooldown), repo.lastModelResetAt, 5*time.Second)
}

// 模型名缺失时无法落模型级冷却：fail-open（不写任何账号级冷却），
// 不能因为拿不到模型名就退回按窗口冷却。
func TestHandleUpstreamError_ZhipuModelOverloadWithoutModelSkipsWindowCooldown(t *testing.T) {
	repo := &modelOverloadAccountRepoStub{}
	svc := newModelOverloadService(repo)
	account := newZhipuCodingPlanAccount(1, 15, 2*time.Hour)

	disabled := svc.HandleUpstreamError(
		context.Background(), account, http.StatusTooManyRequests, http.Header{}, []byte(zhipuModelOverloadedBody))

	require.False(t, disabled)
	require.Zero(t, repo.modelLimitCalls)
	require.Zero(t, repo.rateLimitedCalls)
	require.Zero(t, repo.tempCalls)
	require.Zero(t, repo.overloadCalls)
}

// 非 1305 的 529 仍走原有账号级过载冷却，避免本次改动放宽真实过载。
func TestHandleUpstreamError_Zhipu529WithoutModelOverloadKeepsAccountCooldown(t *testing.T) {
	repo := &modelOverloadAccountRepoStub{}
	svc := newModelOverloadService(repo)
	account := newZhipuCodingPlanAccount(1, 15, 2*time.Hour)

	disabled := svc.HandleUpstreamError(
		context.Background(), account, 529, http.Header{}, []byte(`{"error":{"message":"upstream is overloaded"}}`), "glm-5.3")

	require.False(t, disabled)
	require.Zero(t, repo.modelLimitCalls)
	require.Equal(t, 1, repo.overloadCalls)
	require.WithinDuration(t, time.Now().Add(10*time.Minute), repo.lastOverloadUntil, 10*time.Second)
}

// 模型级冷却写库失败时 fail-open：不升级成账号级冷却。
func TestHandleUpstreamError_ZhipuModelOverloadWriteFailureFailsOpen(t *testing.T) {
	repo := &modelOverloadAccountRepoStub{modelLimitErr: context.DeadlineExceeded}
	svc := newModelOverloadService(repo)
	account := newZhipuCodingPlanAccount(1, 15, 2*time.Hour)

	disabled := svc.HandleUpstreamError(
		context.Background(), account, http.StatusTooManyRequests, http.Header{}, []byte(zhipuModelOverloadedBody), "glm-5.4")

	require.False(t, disabled)
	require.Equal(t, 1, repo.modelLimitCalls)
	require.Zero(t, repo.rateLimitedCalls)
	require.Zero(t, repo.overloadCalls)
}
