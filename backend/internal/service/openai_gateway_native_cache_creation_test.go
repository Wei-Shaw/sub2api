//go:build unit

package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNativeAnthropicCacheCreationRouteBillingBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name      string
		snapshots []string
		aggregate int
		ttl5m     int
		ttl1h     int
		billed5m  int
		billed1h  int
		uncached  int
	}{
		{"legacy aggregate only", []string{`{"cache_creation_input_tokens":50}`}, 50, 0, 0, 50, 0, 10},
		{"missing cache creation usage", []string{`{}`}, 0, 0, 0, 0, 0, 10},
		{"missing aggregate", []string{`{"cache_creation":{"ephemeral_5m_input_tokens":20,"ephemeral_1h_input_tokens":30}}`}, 50, 20, 30, 20, 30, 10},
		{"null aggregate", []string{`{"cache_creation_input_tokens":null,"cache_creation":{"ephemeral_5m_input_tokens":20,"ephemeral_1h_input_tokens":30}}`}, 50, 20, 30, 20, 30, 10},
		{"explicit zero", []string{`{"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_5m_input_tokens":20,"ephemeral_1h_input_tokens":30}}`}, 0, 20, 30, 0, 0, 10},
		{"details below aggregate", []string{`{"cache_creation_input_tokens":100,"cache_creation":{"ephemeral_5m_input_tokens":20,"ephemeral_1h_input_tokens":30}}`}, 100, 20, 30, 20, 30, 10},
		{"contradictory aggregate", []string{`{"cache_creation_input_tokens":100,"cache_creation":{"ephemeral_5m_input_tokens":50,"ephemeral_1h_input_tokens":200}}`}, 100, 50, 200, 20, 80, 10},
		{"negative TTL", []string{`{"cache_creation":{"ephemeral_5m_input_tokens":-20,"ephemeral_1h_input_tokens":30}}`}, 30, 0, 30, 0, 30, 10},
		{"partial TTL clear", []string{`{"cache_creation":{"ephemeral_5m_input_tokens":20}}`, `{"cache_creation":{"ephemeral_1h_input_tokens":30}}`, `{"cache_creation":{"ephemeral_5m_input_tokens":0}}`}, 30, 0, 30, 0, 30, 10},
		{"explicit zero delta", []string{`{"cache_creation_input_tokens":100,"cache_creation":{"ephemeral_5m_input_tokens":20,"ephemeral_1h_input_tokens":30}}`, `{"cache_creation_input_tokens":0}`, `{"cache_creation":{"ephemeral_1h_input_tokens":40}}`}, 0, 20, 40, 0, 0, 10},
		{"zero to positive", []string{`{"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_5m_input_tokens":20,"ephemeral_1h_input_tokens":30}}`, `{"cache_creation_input_tokens":40}`}, 40, 20, 30, 16, 24, 10},
		{"null preserves aggregate", []string{`{"cache_creation_input_tokens":100,"cache_creation":{"ephemeral_5m_input_tokens":20,"ephemeral_1h_input_tokens":30}}`, `{"cache_creation_input_tokens":null,"cache_creation":{"ephemeral_1h_input_tokens":40}}`}, 100, 20, 40, 20, 40, 10},
		{"normalized prompt total", []string{`{"prompt_tokens":100,"cached_tokens":7,"cache_creation":{"ephemeral_5m_input_tokens":20,"ephemeral_1h_input_tokens":30}}`}, 50, 20, 30, 20, 30, 43},
		{"normalized prompt explicit zero", []string{`{"prompt_tokens":100,"cached_tokens":7,"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_5m_input_tokens":20,"ephemeral_1h_input_tokens":30}}`}, 0, 20, 30, 0, 0, 93},
	} {
		uncachedInput := test.uncached
		for _, route := range []string{"messages", "chat", "responses"} {
			for _, stream := range []bool{false, true} {
				for _, terminal := range []string{"stop", "eof"} {
					if route == "messages" && !stream && (terminal == "eof" || len(test.snapshots) != 1) {
						continue
					}
					t.Run(fmt.Sprintf("%s/%s/stream=%t/%s", test.name, route, stream, terminal), func(t *testing.T) {
						startUsage := `{"input_tokens":10,"output_tokens":2,"cache_read_input_tokens":7}`
						if test.snapshots[0] != `{}` {
							startUsage = strings.TrimSuffix(startUsage, "}") + "," + strings.TrimPrefix(test.snapshots[0], "{")
						}
						message := `{"id":"msg_cache","type":"message","role":"assistant","model":"k3","content":[],"usage":` + startUsage + `}`
						payload := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":" + message + "}\n\n"
						for _, snapshot := range test.snapshots {
							for range 2 {
								payload += "event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":" + snapshot + "}\n\n"
							}
						}
						payload += "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\n"
						if terminal == "stop" {
							for range 2 {
								payload += "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
							}
						}
						contentType := "text/event-stream"
						if route == "messages" && !stream {
							payload, contentType = message, "application/json"
						}
						upstream := &httpUpstreamRecorder{resp: &http.Response{
							StatusCode: http.StatusOK,
							Header:     http.Header{"Content-Type": []string{contentType}, "X-Request-Id": []string{"native-cache"}},
							Body:       io.NopCloser(strings.NewReader(payload)),
						}}
						svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
						body := []byte(fmt.Sprintf(`{"model":"k3","max_tokens":32,"messages":[{"role":"user","content":"hi"}],"stream":%t}`, stream))
						path := "/v1/messages"
						if route == "responses" {
							path = "/v1/responses"
							body = []byte(fmt.Sprintf(`{"model":"k3","input":"hi","stream":%t}`, stream))
						} else if route == "chat" {
							path = "/v1/chat/completions"
						}
						recorder := httptest.NewRecorder()
						ctx, _ := gin.CreateTestContext(recorder)
						ctx.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(body)))
						account := nativeAnthropicTestAccount()
						var result *OpenAIForwardResult
						var err error
						switch route {
						case "messages":
							result, err = svc.ForwardAsAnthropic(context.Background(), ctx, account, body, "", "")
						case "chat":
							result, err = svc.ForwardAsChatCompletions(context.Background(), ctx, account, body, "", "")
						case "responses":
							result, err = svc.Forward(context.Background(), ctx, account, body)
						}
						if route == "messages" && stream && terminal == "eof" {
							require.ErrorContains(t, err, "missing terminal event")
						} else {
							require.NoError(t, err)
						}
						require.NotNil(t, result)
						require.Len(t, upstream.requests, 1)
						require.True(t, strings.HasSuffix(upstream.lastReq.URL.Path, "/v1/messages"))
						require.Equal(t, stream || route != "messages", gjson.GetBytes(upstream.lastBody, "stream").Bool())
						require.Equal(t, "/v1/messages", result.UpstreamEndpoint)
						require.Equal(t, stream, result.Stream)
						require.Equal(t, uncachedInput+7+test.aggregate, result.Usage.InputTokens)
						require.Equal(t, 2, result.Usage.OutputTokens)
						require.Equal(t, 7, result.Usage.CacheReadInputTokens)
						require.Equal(t, test.aggregate, result.Usage.CacheCreationInputTokens)
						require.Equal(t, test.ttl5m, result.Usage.CacheCreation5mTokens)
						require.Equal(t, test.ttl1h, result.Usage.CacheCreation1hTokens)
						require.Equal(t, http.StatusOK, recorder.Code)
						if route == "messages" {
							require.Equal(t, payload, recorder.Body.String())
						} else {
							assertNativeCacheCompatibleUsage(t, recorder.Body.String(), route, stream, uncachedInput, test.aggregate)
						}
						cacheCost := float64(test.billed5m)*0.002 + float64(test.billed1h)*0.004
						assertNativeCacheRecordUsage(t, result, account, uncachedInput, cacheCost)
					})
				}
			}
		}
	}
}

func assertNativeCacheCompatibleUsage(t *testing.T, body, route string, stream bool, uncachedInput, aggregate int) {
	t.Helper()
	var usage gjson.Result
	if !stream {
		usage = gjson.Get(body, "usage")
	} else {
		count := 0
		for _, line := range strings.Split(body, "\n") {
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if route == "responses" && gjson.Get(data, "type").String() == "response.completed" {
				usage = gjson.Get(data, "response.usage")
				count++
			} else if route == "chat" && gjson.Get(data, "usage").Exists() {
				usage = gjson.Get(data, "usage")
				count++
			}
		}
		require.Equal(t, 1, count)
		if route == "chat" {
			require.Equal(t, 1, strings.Count(body, "data: [DONE]"))
		}
	}
	require.True(t, usage.Exists())
	if route == "responses" {
		require.EqualValues(t, uncachedInput+7+aggregate, usage.Get("input_tokens").Int())
		require.EqualValues(t, 2, usage.Get("output_tokens").Int())
		require.EqualValues(t, 7, usage.Get("input_tokens_details.cached_tokens").Int())
		require.EqualValues(t, aggregate, usage.Get("cache_creation_input_tokens").Int())
	} else {
		require.EqualValues(t, uncachedInput+7+aggregate, usage.Get("prompt_tokens").Int())
		require.EqualValues(t, 2, usage.Get("completion_tokens").Int())
		require.EqualValues(t, 7, usage.Get("prompt_tokens_details.cached_tokens").Int())
		require.EqualValues(t, aggregate, usage.Get("prompt_tokens_details.cache_creation_tokens").Int())
	}
	require.EqualValues(t, uncachedInput+9+aggregate, usage.Get("total_tokens").Int())
}

func assertNativeCacheRecordUsage(t *testing.T, result *OpenAIForwardResult, account *Account, uncachedInput int, cacheCost float64) {
	t.Helper()
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	billingRepo := &openAIRecordUsageBillingRepoStub{}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	svc.billingService = newTestBillingServiceWithPrices(map[string]*ModelPricing{
		"kimi-k3": {
			InputPricePerToken: 0.001, OutputPricePerToken: 0.002, CacheReadPricePerToken: 0.0001,
			CacheCreationPricePerToken: 0.002, SupportsCacheBreakdown: true,
			CacheCreation5mPrice: 0.002, CacheCreation1hPrice: 0.004,
		},
	})
	channel := &Channel{ID: 1, Status: StatusActive, AccountStatsPricingRules: []AccountStatsPricingRule{{
		AccountIDs: []int64{account.ID},
		Pricing: []ChannelModelPricing{{
			Platform: account.Platform, Models: []string{"k3"}, BillingMode: BillingModeToken,
			InputPrice: float64Ptr(0.01), OutputPrice: float64Ptr(0.02), CacheReadPrice: float64Ptr(0.001),
			CacheWritePrice: float64Ptr(0.02), CacheWrite1hPrice: float64Ptr(0.04),
		}},
	}}}
	svc.channelService = newTestChannelServiceForStats(t, channel, 10, account.Platform)
	svc.resolver = NewModelPricingResolver(svc.channelService, svc.billingService)
	groupID := int64(10)
	require.NoError(t, svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: result, Account: account, User: &User{ID: 601},
		APIKey: &APIKey{ID: 501, GroupID: &groupID, Group: &Group{ID: groupID, Platform: account.Platform, RateMultiplier: 0.5}},
	}))
	require.Equal(t, 1, usageRepo.calls)
	require.Equal(t, 1, billingRepo.calls)
	require.Equal(t, uncachedInput, usageRepo.lastLog.InputTokens)
	require.Equal(t, result.Usage.CacheCreationInputTokens, usageRepo.lastLog.CacheCreationTokens)
	require.Equal(t, result.Usage.CacheCreation5mTokens, usageRepo.lastLog.CacheCreation5mTokens)
	require.Equal(t, result.Usage.CacheCreation1hTokens, usageRepo.lastLog.CacheCreation1hTokens)
	require.InDelta(t, cacheCost, usageRepo.lastLog.CacheCreationCost, 1e-12)
	totalCost := float64(uncachedInput)*0.001 + 0.0047 + cacheCost
	require.InDelta(t, totalCost, usageRepo.lastLog.TotalCost, 1e-12)
	require.InDelta(t, totalCost*0.5, usageRepo.lastLog.ActualCost, 1e-12)
	require.InDelta(t, totalCost*0.5, billingRepo.lastCmd.BalanceCost, 1e-12)
	require.NotNil(t, usageRepo.lastLog.AccountStatsCost)
	require.InDelta(t, totalCost*10, *usageRepo.lastLog.AccountStatsCost, 1e-12)
}
