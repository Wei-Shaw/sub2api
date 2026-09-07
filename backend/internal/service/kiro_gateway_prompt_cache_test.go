//go:build unit

package service

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/stretchr/testify/require"
)

// kiroPromptCacheTestRequestBody 构造一个带 system cache_control 断点、长度
// 足以越过最小可缓存 token 门槛（1024）的请求体——kiroTestRequestBody 那个
// 极短的固定 body 不带任何 cache_control，测不出这里要验证的行为。
func kiroPromptCacheTestRequestBody(t *testing.T, userText string) string {
	t.Helper()
	longSystem := strings.Repeat("You are a helpful coding assistant with deep knowledge of Go, Rust, Python, and TypeScript. ", 80)
	body := map[string]any{
		"model": "claude-sonnet-4-5-20250929",
		"system": []map[string]any{
			{
				"type":          "text",
				"text":          longSystem,
				"cache_control": map[string]any{"type": "ephemeral"},
			},
		},
		"max_tokens": 100,
		"messages": []map[string]any{
			{"role": "user", "content": userText},
		},
		"stream": true,
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	return string(raw)
}

// TestKiroForwardUpstreamPromptCacheSimulationAcrossTwoRequests 是本地模拟
// prompt cache 的端到端回归：真实调查证实 Kiro 的 meteringEvent 在实践中从未
// 给出非零缓存值（本次改动的动机），这里构造一个"上游全程不返回任何
// meteringEvent"的假上游（比全零 meteringEvent 更贴近真实观测），验证同一
// 账号连续两次请求：第一次只应该有 cache_creation（该账号第一次见到这个
// 前缀），第二次重复相同前缀应该命中 cache_read——且两次都不影响
// TestKiroForwardUpstreamSuccessStreamingWithRealCacheReadTokens 锁定的
// "真实 meteringEvent 优先"行为（该测试独立构造了自己的 service 实例）。
func TestKiroForwardUpstreamPromptCacheSimulationAcrossTwoRequests(t *testing.T) {
	frames := kiroTestConcatFrames(
		kiroTestEventFrame("assistantResponseEvent", `{"content":"Hello"}`),
		kiroTestEventFrame("metadataEvent", `{"stopReason":"end_turn"}`),
	)

	srv, calls := kiroTestFakeUpstream(t, func(int) (int, []byte) {
		return 200, frames
	})

	svc := &KiroGatewayService{promptCache: kiro.NewPromptCacheTracker()}
	svc.callEndpointOverride = kiroTestOverrideCallingServer(srv)

	account := kiroTestOAuthAccount(1)

	body1 := kiroPromptCacheTestRequestBody(t, "question one")
	_, c1 := kiroTestContext()
	result1, err := svc.ForwardUpstream(context.Background(), c1, account, []byte(body1))
	require.NoError(t, err)
	require.NotNil(t, result1)
	require.Positive(t, result1.Usage.CacheCreationInputTokens, "first request for this account should create cache, not read it")
	require.Zero(t, result1.Usage.CacheReadInputTokens)

	body2 := kiroPromptCacheTestRequestBody(t, "question one")
	_, c2 := kiroTestContext()
	result2, err := svc.ForwardUpstream(context.Background(), c2, account, []byte(body2))
	require.NoError(t, err)
	require.NotNil(t, result2)
	require.Positive(t, result2.Usage.CacheReadInputTokens, "repeating the same prefix for the same account should hit the simulated cache")
	require.Zero(t, result2.Usage.CacheCreationInputTokens)

	require.EqualValues(t, 2, atomic.LoadInt32(calls))
}

// TestKiroForwardUpstreamPromptCacheDoesNotLeakAcrossAccounts 覆盖账号隔离：
// 账号 2 不应该因为账号 1 发过同样的前缀就白得一次缓存命中——真实 Kiro 的
// prompt cache（哪怕它存在）也是按账号物理隔离的，见 kiro2cc-proxy 文档 §3
// 的"跨账号缓存彻底作废"说明；本地模拟必须复刻同样的隔离边界。
func TestKiroForwardUpstreamPromptCacheDoesNotLeakAcrossAccounts(t *testing.T) {
	frames := kiroTestConcatFrames(
		kiroTestEventFrame("assistantResponseEvent", `{"content":"Hello"}`),
		kiroTestEventFrame("metadataEvent", `{"stopReason":"end_turn"}`),
	)

	srv, _ := kiroTestFakeUpstream(t, func(int) (int, []byte) {
		return 200, frames
	})

	svc := &KiroGatewayService{promptCache: kiro.NewPromptCacheTracker()}
	svc.callEndpointOverride = kiroTestOverrideCallingServer(srv)

	body := kiroPromptCacheTestRequestBody(t, "question one")

	_, c1 := kiroTestContext()
	_, err := svc.ForwardUpstream(context.Background(), c1, kiroTestOAuthAccount(1), []byte(body))
	require.NoError(t, err)

	_, c2 := kiroTestContext()
	result2, err := svc.ForwardUpstream(context.Background(), c2, kiroTestOAuthAccount(2), []byte(body))
	require.NoError(t, err)
	require.Zero(t, result2.Usage.CacheReadInputTokens, "account 2 must not benefit from account 1's cache entries")
}

// TestKiroForwardUpstreamNilPromptCacheTrackerIsNoop 覆盖零值安全：本文件
// 其余全部既有测试都用 &KiroGatewayService{...} 结构体字面量构造（不走
// NewKiroGatewayService），promptCache 字段是 nil——这些测试在本次改动前
// 就存在且必须继续通过，不能因为忘了初始化这个新字段就 panic 或产生模拟值。
func TestKiroForwardUpstreamNilPromptCacheTrackerIsNoop(t *testing.T) {
	frames := kiroTestConcatFrames(
		kiroTestEventFrame("assistantResponseEvent", `{"content":"Hello"}`),
		kiroTestEventFrame("metadataEvent", `{"stopReason":"end_turn"}`),
	)
	srv, _ := kiroTestFakeUpstream(t, func(int) (int, []byte) {
		return 200, frames
	})

	svc := &KiroGatewayService{} // promptCache 保持零值 nil
	svc.callEndpointOverride = kiroTestOverrideCallingServer(srv)

	body := kiroPromptCacheTestRequestBody(t, "question one")
	_, c := kiroTestContext()

	require.NotPanics(t, func() {
		result, err := svc.ForwardUpstream(context.Background(), c, kiroTestOAuthAccount(1), []byte(body))
		require.NoError(t, err)
		require.Zero(t, result.Usage.CacheCreationInputTokens)
		require.Zero(t, result.Usage.CacheReadInputTokens)
	})
}
