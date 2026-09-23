//go:build unit

package service

// 国产供应商功能修复回归测试：
//  1. CN 分组不适用 /v1/messages 调度级模型映射（openai 的 gpt-5.x 默认值发给
//     CN 上游必错）；
//  2. 计费候选链对 CN 账号过滤 claude-* 兜底候选（防按 Claude 原价误计 CN 流量）；
//  3. 空候选按 ErrModelPricingUnavailable 处理（零成本落账而非丢弃 usage 记录）；
//  4. Responses×anthropic 流式转换器客户端断开后继续排水、usage 汇总完整。

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResolveMessagesDispatchModel_CNProvidersNoDispatchMapping(t *testing.T) {
	for _, platform := range []string{PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax, PlatformOpenCodeGo} {
		g := &Group{Platform: platform}
		require.Empty(t, g.ResolveMessagesDispatchModel("claude-sonnet-4-5"),
			"CN 分组(%s)不得返回调度级映射模型（openai 默认值会发给 CN 上游）", platform)
		require.Empty(t, g.ResolveMessagesDispatchModel("claude-opus-4-1"), platform)
	}
	// 非回归：openai 分组保持原有默认映射行为。
	openaiGroup := &Group{Platform: PlatformOpenAI}
	require.NotEmpty(t, openaiGroup.ResolveMessagesDispatchModel("claude-sonnet-4-5"),
		"openai 分组的调度默认映射不应受 CN 修复影响")
}

func TestFilterCNProviderBillingModelCandidates(t *testing.T) {
	svc := &OpenAIGatewayService{} // resolver 为 nil → 无显式分组/渠道定价
	apiKey := &APIKey{Group: &Group{ID: 1, Platform: PlatformKimi}}

	cnAccount := &Account{ID: 1, Platform: PlatformKimi}
	filtered := svc.filterCNProviderBillingModelCandidates(context.Background(), cnAccount, apiKey,
		[]string{"kimi-k2-0905-preview", "claude-sonnet-4-5", "moonshot-v1-8k"})
	require.Equal(t, []string{"kimi-k2-0905-preview", "moonshot-v1-8k"}, filtered,
		"无显式定价时 claude-* 候选必须被过滤")

	allClaude := svc.filterCNProviderBillingModelCandidates(context.Background(), cnAccount, apiKey,
		[]string{"claude-sonnet-4-5", "claude-sonnet-4-5"})
	require.Empty(t, allClaude, "全 claude 候选应被清空（上层走零成本+告警落账）")

	// 非 CN 账号完全不受影响。
	openaiAccount := &Account{ID: 2, Platform: PlatformOpenAI}
	passthrough := svc.filterCNProviderBillingModelCandidates(context.Background(), openaiAccount, apiKey,
		[]string{"claude-sonnet-4-5", "gpt-5.4"})
	require.Equal(t, []string{"claude-sonnet-4-5", "gpt-5.4"}, passthrough)

	require.Nil(t, svc.filterCNProviderBillingModelCandidates(context.Background(), nil, apiKey, nil))

	openCodeAccount := &Account{ID: 3, Platform: PlatformOpenCodeGo}
	openCodeFiltered := svc.filterCNProviderBillingModelCandidates(context.Background(), openCodeAccount, apiKey,
		[]string{"claude-sonnet-4-5", "muse-spark-1.3-contributor-free"})
	require.Equal(t, []string{"muse-spark-1.3-contributor-free"}, openCodeFiltered,
		"OpenCode 无显式定价时不得按 Claude 原价计费 claude-*")
}

func TestCalculateOpenAIRecordUsageCost_EmptyCandidatesIsPricingUnavailable(t *testing.T) {
	svc := &OpenAIGatewayService{}
	apiKey := &APIKey{Group: &Group{ID: 1, Platform: PlatformKimi}}

	_, err := svc.calculateOpenAIRecordUsageCost(
		context.Background(), nil, apiKey, nil,
		1.0, 1.0, 1.0, 1.0, UsageTokens{InputTokens: 100}, "", nil, time.Time{},
	)
	require.Error(t, err)
	require.True(t, isUsagePricingUnavailableError(err),
		"空候选必须按无价可循处理（上层零成本落账），而不是丢弃整条 usage 记录: %v", err)
}

func TestResponsesStreamingFromNativeAnthropic_ClientDisconnectDrainsUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newNativeAnthropicHangTestService(5)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	// failAfter=0：首次写出即失败，模拟客户端断开（复用测试包既有 failingGinWriter）。
	failWriter := &failingGinWriter{ResponseWriter: c.Writer, failAfter: 0}
	c.Writer = failWriter

	resp, pr, pw := newHangingUpstreamResponse()
	go func() {
		// 首事件触发客户端写失败后，末尾 message_delta 才携带最终 output_tokens：
		// 断开即弃会把整段生成记成 1 token。
		_, _ = pw.Write([]byte(miniAnthropicSSEStream()))
		_ = pw.Close()
	}()
	defer func() { _ = pr.Close() }()

	res, err := svc.handleResponsesStreamingFromNativeAnthropic(
		resp, c, "glm-4.7", "glm-4.7", "glm-4.7", nil, time.Now(), apicompat.ResponsesClientToolMapping{})

	require.NoError(t, err, "断开排水至上游自然结束应返回 nil error（usage 走成功路径落账）")
	require.NotNil(t, res)
	require.True(t, res.ClientDisconnect)
	require.Equal(t, 10, res.Usage.InputTokens, "input_tokens 应来自 message_start")
	require.Equal(t, 5, res.Usage.OutputTokens,
		"output_tokens 必须来自排水读到的末尾 message_delta（断开即弃时会是 1）")
}

func TestResponsesStreamingFromNativeAnthropic_NormalizesTerminalUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name              string
		startUsage        string
		deltaUsage        string
		repeatDelta       bool
		wantInput         int
		wantOutput        int
		wantCached        int
		wantCacheCreation int
	}{
		{name: "full cache", startUsage: `"input_tokens":1200,"prompt_tokens":1200`, deltaUsage: `"input_tokens":0,"output_tokens":30,"prompt_tokens":1200,"cache_read_input_tokens":1200`, wantOutput: 30, wantCached: 1200},
		{name: "partial cache", startUsage: `"input_tokens":1200,"prompt_tokens":1200`, deltaUsage: `"input_tokens":0,"output_tokens":30,"prompt_tokens":1200,"cache_read_input_tokens":800`, wantInput: 400, wantOutput: 30, wantCached: 800},
		{name: "cache creation", startUsage: `"input_tokens":1200,"prompt_tokens":1200`, deltaUsage: `"input_tokens":0,"output_tokens":30,"prompt_tokens":1200,"cache_creation_input_tokens":800`, wantInput: 400, wantOutput: 30, wantCacheCreation: 800},
		{name: "repeated cumulative cache bucket", startUsage: `"input_tokens":1200,"prompt_tokens":1200`, deltaUsage: `"input_tokens":0,"output_tokens":30,"prompt_tokens":1200,"cache_read_input_tokens":800`, repeatDelta: true, wantInput: 400, wantOutput: 30, wantCached: 800},
	}

	for _, tt := range tests {
		for _, terminal := range []string{"message_stop", "eof"} {
			t.Run(tt.name+"/"+terminal, func(t *testing.T) {
				lines := []string{
					`event: message_start`,
					`data: {"type":"message_start","message":{"id":"msg_usage","type":"message","role":"assistant","content":[],"model":"k3","stop_reason":"","usage":{` + tt.startUsage + `}}}`,
					``,
					`event: message_delta`,
					`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{` + tt.deltaUsage + `}}`,
					``,
				}
				if tt.repeatDelta {
					lines = append(lines, `event: message_delta`, `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{`+tt.deltaUsage+`}}`, ``)
				}
				if terminal == "message_stop" {
					lines = append(lines, `event: message_stop`, `data: {"type":"message_stop"}`, ``)
				}

				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(strings.Join(lines, "\n")))}

				result, err := (&OpenAIGatewayService{}).handleResponsesStreamingFromNativeAnthropic(
					resp, c, "k3", "k3", "k3", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
				require.NoError(t, err)
				require.Equal(t, tt.wantInput+tt.wantCached+tt.wantCacheCreation, result.Usage.InputTokens)
				require.Equal(t, tt.wantOutput, result.Usage.OutputTokens)
				require.Equal(t, tt.wantCached, result.Usage.CacheReadInputTokens)
				require.Equal(t, tt.wantCacheCreation, result.Usage.CacheCreationInputTokens)

				var completed apicompat.ResponsesStreamEvent
				for _, line := range strings.Split(rec.Body.String(), "\n") {
					if !strings.HasPrefix(line, "data: ") {
						continue
					}
					var event apicompat.ResponsesStreamEvent
					require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event))
					if event.Type == "response.completed" {
						completed = event
					}
				}
				require.NotNil(t, completed.Response)
				require.NotNil(t, completed.Response.Usage)
				require.Equal(t, tt.wantInput+tt.wantCached+tt.wantCacheCreation, completed.Response.Usage.InputTokens)
				require.Equal(t, tt.wantOutput, completed.Response.Usage.OutputTokens)
				require.Equal(t, completed.Response.Usage.InputTokens+tt.wantOutput, completed.Response.Usage.TotalTokens)
				require.Equal(t, tt.wantCacheCreation, completed.Response.Usage.CacheCreationInputTokens)
				if tt.wantCached == 0 {
					require.Nil(t, completed.Response.Usage.InputTokensDetails)
				} else {
					require.Equal(t, tt.wantCached, completed.Response.Usage.InputTokensDetails.CachedTokens)
				}
			})
		}
	}
}

func TestHandle403_CNProviderHTMLBodySkipsAccountPenalty(t *testing.T) {
	for _, platform := range []string{PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax} {
		repo := &rateLimitAccountRepoStub{}
		service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
		account := &Account{ID: 401, Platform: platform, Type: AccountTypeAPIKey}

		shouldDisable := service.HandleUpstreamError(
			context.Background(),
			account,
			http.StatusForbidden,
			http.Header{},
			[]byte("<html><body>Access denied by CDN</body></html>"),
		)

		require.False(t, shouldDisable, "%s: HTML 403（CDN/代理拦截页）不得作为账号失效证据", platform)
		require.Equal(t, 0, repo.setErrorCalls, "%s: 不得永久禁用账号", platform)
		require.Equal(t, 0, repo.tempCalls, "%s: 不得临时停调账号", platform)
	}
}

func TestHandle403_CNProviderStructured403TempUnschedulableFirstHit(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	counter := &openAI403CounterCacheStub{counts: []int64{1}}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	service.SetOpenAI403CounterCache(counter)
	account := &Account{ID: 402, Platform: PlatformKimi, Type: AccountTypeAPIKey}

	shouldDisable := service.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusForbidden,
		http.Header{},
		[]byte(`{"error":{"message":"forbidden"}}`),
	)

	require.True(t, shouldDisable)
	require.Equal(t, 0, repo.setErrorCalls, "首次结构化 403 应临时停调而非永久禁用")
	require.Equal(t, 1, repo.tempCalls)
	require.Contains(t, repo.lastTempReason, "(1/3)")
}

func TestIsCNProviderConcurrencyLimit403_ExactClassification(t *testing.T) {
	kimi := &Account{Platform: PlatformKimi}

	require.True(t, isCNProviderConcurrencyLimit403(kimi, kimiConcurrentRequestLimitMessage))
	require.True(t, isCNProviderConcurrencyLimit403(kimi, "  "+kimiConcurrentRequestLimitMessage+"\n"))

	for name, tc := range map[string]struct {
		account *Account
		message string
	}{
		"permission denied":              {kimi, "You do not have permission to access this resource."},
		"generic concurrency wording":    {kimi, "concurrent request limit reached"},
		"near match missing punctuation": {kimi, "You've reached your concurrent request limit. Please wait for your ongoing requests to finish and try again"},
		"other CN provider":              {&Account{Platform: PlatformZhipu}, kimiConcurrentRequestLimitMessage},
		"non CN provider":                {&Account{Platform: PlatformOpenAI}, kimiConcurrentRequestLimitMessage},
		"nil account":                    {nil, kimiConcurrentRequestLimitMessage},
	} {
		t.Run(name, func(t *testing.T) {
			require.False(t, isCNProviderConcurrencyLimit403(tc.account, tc.message))
		})
	}
}

func TestHandle403_OtherCNProviderWithKimiConcurrencyMessageUsesNormalPolicy(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	counter := &openAI403CounterCacheStub{counts: []int64{openAI403DisableThreshold}}
	blocker := &runtimeBlockRecorder{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	service.SetOpenAI403CounterCache(counter)
	service.SetAccountRuntimeBlocker(blocker)
	account := &Account{ID: 405, Platform: PlatformZhipu, Type: AccountTypeAPIKey}

	shouldDisable := service.HandleUpstreamError(
		context.Background(), account, http.StatusForbidden, http.Header{},
		[]byte(`{"error":{"message":"You've reached your concurrent request limit. Please wait for your ongoing requests to finish and try again."}}`),
	)

	require.True(t, shouldDisable)
	require.Equal(t, 1, repo.setErrorCalls, "non-Kimi CN provider must retain the normal permanent-error policy")
	require.Equal(t, 0, repo.tempCalls)
	require.Empty(t, counter.counts, "normal CN 403 policy must consume the counter result")
	require.Equal(t, []string{"auth_error"}, blocker.reasons, "the Kimi-specific runtime block must not apply")
}

func TestHandle403_CNProviderConcurrencyLimitAlwaysUsesTemporaryCooldown(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	counter := &openAI403CounterCacheStub{counts: []int64{openAI403DisableThreshold}}
	blocker := &runtimeBlockRecorder{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	service.SetOpenAI403CounterCache(counter)
	service.SetAccountRuntimeBlocker(blocker)
	account := &Account{ID: 403, Platform: PlatformKimi, Type: AccountTypeAPIKey}

	shouldDisable := service.HandleUpstreamError(
		context.Background(), account, http.StatusForbidden, http.Header{},
		[]byte(`{"error":{"message":"You've reached your concurrent request limit. Please wait for your ongoing requests to finish and try again."}}`),
	)

	require.True(t, shouldDisable, "the request must still fail over to another account")
	require.Equal(t, 0, repo.setErrorCalls)
	require.Equal(t, 1, repo.tempCalls)
	require.Contains(t, repo.lastTempReason, cnConcurrencyLimitReasonPrefix)
	require.Equal(t, []int64{openAI403DisableThreshold}, counter.counts, "transient concurrency 403 must bypass the permanent-error counter")
	require.Len(t, blocker.accounts, 1)
	require.Equal(t, cnConcurrencyLimitReasonPrefix, blocker.reasons[0])
	require.True(t, blocker.until[0].After(time.Now()))
}

func TestHandle403_KimiConcurrencyLimitRepositoryFailureKeepsRuntimeBlock(t *testing.T) {
	repo := &rateLimitAccountRepoStub{tempErr: errors.New("repository unavailable")}
	counter := &openAI403CounterCacheStub{counts: []int64{openAI403DisableThreshold}}
	blocker := &runtimeBlockRecorder{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	service.SetOpenAI403CounterCache(counter)
	service.SetAccountRuntimeBlocker(blocker)
	account := &Account{ID: 406, Platform: PlatformKimi, Type: AccountTypeAPIKey}

	shouldDisable := service.HandleUpstreamError(
		context.Background(), account, http.StatusForbidden, http.Header{},
		[]byte(`{"error":{"message":"You've reached your concurrent request limit. Please wait for your ongoing requests to finish and try again."}}`),
	)

	require.True(t, shouldDisable, "the current request must fail over even when persistence fails")
	require.Equal(t, 1, repo.tempCalls, "the temporary cooldown should still be persisted when possible")
	require.Equal(t, 0, repo.setErrorCalls, "persistence failure must not fall back to permanent account error")
	require.Equal(t, []int64{openAI403DisableThreshold}, counter.counts, "persistence failure must not enter the permanent-error counter path")
	require.Len(t, blocker.accounts, 1, "the in-memory runtime block must survive repository failure")
	require.Same(t, account, blocker.accounts[0])
	require.Equal(t, cnConcurrencyLimitReasonPrefix, blocker.reasons[0])
	require.True(t, blocker.until[0].After(time.Now()))
}

func TestHandle403_CNProviderNearMatchRetainsNormalPermanentErrorPolicy(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	counter := &openAI403CounterCacheStub{counts: []int64{openAI403DisableThreshold}}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	service.SetOpenAI403CounterCache(counter)
	account := &Account{ID: 404, Platform: PlatformKimi, Type: AccountTypeAPIKey}

	shouldDisable := service.HandleUpstreamError(
		context.Background(), account, http.StatusForbidden, http.Header{},
		[]byte(`{"error":{"message":"You've reached your concurrent request limit. Please contact support."}}`),
	)

	require.True(t, shouldDisable)
	require.Equal(t, 1, repo.setErrorCalls, "non-exact 403 must retain existing permission/auth protection")
	require.Equal(t, 0, repo.tempCalls)
}
