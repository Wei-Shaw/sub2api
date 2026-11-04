//go:build unit

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAnthropicCacheCreationSnapshots(t *testing.T) {
	tests := []struct {
		name       string
		snapshots  []string
		wantTotals []int
		want5m     int
		want1h     int
		billed5m   int
		billed1h   int
	}{
		{
			name:       "aggregate 100 with 1h 200",
			snapshots:  []string{`{"cache_creation_input_tokens":100,"cache_creation":{"ephemeral_1h_input_tokens":200}}`, `{"cache_creation_input_tokens":100,"cache_creation":{"ephemeral_1h_input_tokens":200}}`},
			wantTotals: []int{100, 100}, want1h: 200, billed1h: 100,
		},
		{
			name:       "contradictory explicit aggregate",
			snapshots:  []string{`{"cache_creation_input_tokens":100,"cache_creation":{"ephemeral_5m_input_tokens":50,"ephemeral_1h_input_tokens":200}}`, `{"cache_creation":{"ephemeral_1h_input_tokens":300}}`},
			wantTotals: []int{100, 100}, want5m: 50, want1h: 300, billed5m: 14, billed1h: 86,
		},
		{
			name:       "partial TTL snapshots derive missing aggregate",
			snapshots:  []string{`{"cache_creation":{"ephemeral_5m_input_tokens":20}}`, `{"cache_creation":{"ephemeral_1h_input_tokens":30}}`, `{"cache_creation":{"ephemeral_5m_input_tokens":0}}`},
			wantTotals: []int{20, 50, 30}, want1h: 30, billed1h: 30,
		},
		{
			name:       "aggregate only delta replaces derived total",
			snapshots:  []string{`{"cache_creation":{"ephemeral_5m_input_tokens":20,"ephemeral_1h_input_tokens":30}}`, `{"cache_creation_input_tokens":40}`, `{"output_tokens":7}`, `{"cache_creation":{"ephemeral_1h_input_tokens":200}}`},
			wantTotals: []int{50, 40, 40, 40}, want5m: 20, want1h: 200, billed5m: 4, billed1h: 36,
		},
		{
			name:       "explicit zero survives TTL updates",
			snapshots:  []string{`{"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_1h_input_tokens":200}}`, `{"cache_creation":{"ephemeral_5m_input_tokens":50}}`},
			wantTotals: []int{0, 0}, want5m: 50, want1h: 200,
		},
		{
			name:       "explicit zero delta clears aggregate",
			snapshots:  []string{`{"cache_creation_input_tokens":100}`, `{"cache_creation_input_tokens":0}`, `{"cache_creation":{"ephemeral_1h_input_tokens":200}}`},
			wantTotals: []int{100, 0, 0}, want1h: 200,
		},
		{
			name:       "repeated cumulative snapshots do not double count",
			snapshots:  []string{`{"cache_creation":{"ephemeral_5m_input_tokens":20,"ephemeral_1h_input_tokens":30}}`, `{"cache_creation":{"ephemeral_5m_input_tokens":20,"ephemeral_1h_input_tokens":30}}`, `{"cache_creation":{"ephemeral_1h_input_tokens":30}}`},
			wantTotals: []int{50, 50, 50}, want5m: 20, want1h: 30, billed5m: 20, billed1h: 30,
		},
		{
			name:       "null aggregate derives merged partial TTL",
			snapshots:  []string{`{"cache_creation_input_tokens":null,"cache_creation":{"ephemeral_5m_input_tokens":20}}`, `{"cache_creation_input_tokens":null,"cache_creation":{"ephemeral_5m_input_tokens":null,"ephemeral_1h_input_tokens":30}}`, `{"cache_creation":{"ephemeral_5m_input_tokens":null,"ephemeral_1h_input_tokens":null}}`},
			wantTotals: []int{20, 50, 50}, want5m: 20, want1h: 30, billed5m: 20, billed1h: 30,
		},
		{
			name:       "null aggregate does not clear explicit aggregate",
			snapshots:  []string{`{"cache_creation_input_tokens":100,"cache_creation":{"ephemeral_1h_input_tokens":200}}`, `{"cache_creation_input_tokens":null,"cache_creation":{"ephemeral_1h_input_tokens":null}}`},
			wantTotals: []int{100, 100}, want1h: 200, billed1h: 100,
		},
		{
			name:       "null does not clear explicit zero",
			snapshots:  []string{`{"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_1h_input_tokens":200}}`, `{"cache_creation_input_tokens":null,"cache_creation":{"ephemeral_5m_input_tokens":20,"ephemeral_1h_input_tokens":null}}`},
			wantTotals: []int{0, 0}, want5m: 20, want1h: 200,
		},
		{
			name:       "zero to positive retains partial TTL history",
			snapshots:  []string{`{"cache_creation":{"ephemeral_5m_input_tokens":20}}`, `{"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_1h_input_tokens":30}}`, `{"cache_creation":{"ephemeral_1h_input_tokens":50}}`, `{"cache_creation_input_tokens":35}`},
			wantTotals: []int{20, 0, 0, 35}, want5m: 20, want1h: 50, billed5m: 10, billed1h: 25,
		},
		{
			name:       "all null is absent before partial TTL",
			snapshots:  []string{`{"cache_creation_input_tokens":null,"cache_creation":{"ephemeral_5m_input_tokens":null,"ephemeral_1h_input_tokens":null}}`, `{"cache_creation":{"ephemeral_5m_input_tokens":20}}`, `{"cache_creation":{"ephemeral_1h_input_tokens":30}}`},
			wantTotals: []int{0, 20, 50}, want5m: 20, want1h: 30, billed5m: 20, billed1h: 30,
		},
	}
	for _, test := range tests {
		for _, parser := range []string{"shared merge", "raw passthrough", "generic SSE"} {
			t.Run(test.name+"/"+parser, func(t *testing.T) {
				usage := &ClaudeUsage{}
				for index, snapshot := range test.snapshots {
					if parser == "shared merge" {
						var source apicompat.AnthropicUsage
						require.NoError(t, json.Unmarshal([]byte(snapshot), &source))
						mergeAnthropicUsage(usage, source)
					} else if parser == "generic SSE" {
						if index == 0 {
							(&GatewayService{}).parseSSEUsage(`{"type":"message_start","message":{"usage":`+snapshot+`}}`, usage)
						} else {
							(&GatewayService{}).parseSSEUsage(`{"type":"message_delta","usage":`+snapshot+`}`, usage)
						}
					} else if index == 0 {
						parseSSEUsagePassthrough(`{"type":"message_start","message":{"usage":`+snapshot+`}}`, usage)
					} else {
						parseSSEUsagePassthrough(`{"type":"message_delta","usage":`+snapshot+`}`, usage)
					}
					require.Equal(t, test.wantTotals[index], usage.CacheCreationInputTokens)
				}
				require.Equal(t, test.want5m, usage.CacheCreation5mTokens)
				require.Equal(t, test.want1h, usage.CacheCreation1hTokens)
				cache5m, cache1h := normalizeCacheCreationBreakdown(usage.billingUsageTokens())
				require.Equal(t, test.billed5m, cache5m)
				require.Equal(t, test.billed1h, cache1h)
				assertAnthropicCacheCreationRecordCosts(t, *usage, float64(test.billed5m)*0.002+float64(test.billed1h)*0.004)
			})
		}
	}
}

func TestAnthropicCacheCreationMissingDetailForwardBillingBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name      string
		start     string
		delta     string
		aggregate int
		ttl5m     int
		ttl1h     int
		cacheCost float64
	}{
		{"5m start aggregate grows", `"cache_creation_input_tokens":3000,"cache_creation":{"ephemeral_5m_input_tokens":3000}`, `{"cache_creation_input_tokens":11000}`, 11000, 3000, 0, 22},
		{"1h start remainder defaults to 5m", `"cache_creation_input_tokens":3000,"cache_creation":{"ephemeral_1h_input_tokens":3000}`, `{"cache_creation_input_tokens":11000}`, 11000, 0, 3000, 28},
		{"mixed start remainder defaults to 5m", `"cache_creation_input_tokens":3000,"cache_creation":{"ephemeral_5m_input_tokens":1000,"ephemeral_1h_input_tokens":2000}`, `{"cache_creation_input_tokens":11000}`, 11000, 1000, 2000, 26},
		{"zero start aggregate grows", `"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":0}`, `{"cache_creation_input_tokens":11000}`, 11000, 0, 0, 22},
		{"unchanged aggregate", `"cache_creation_input_tokens":3000,"cache_creation":{"ephemeral_1h_input_tokens":3000}`, `{"cache_creation_input_tokens":3000}`, 3000, 0, 3000, 12},
		{"explicit zero suppresses retained detail", `"cache_creation_input_tokens":3000,"cache_creation":{"ephemeral_1h_input_tokens":3000}`, `{"cache_creation_input_tokens":0}`, 0, 0, 3000, 0},
		{"oversized detail ratio capped", `"cache_creation_input_tokens":100,"cache_creation":{"ephemeral_5m_input_tokens":50,"ephemeral_1h_input_tokens":200}`, `{"cache_creation_input_tokens":100}`, 100, 50, 200, 0.36},
	} {
		for _, route := range []string{"standard", "passthrough", "responses", "buffered responses", "chat", "buffered chat"} {
			for _, terminal := range []string{"stop", "eof"} {
				t.Run(test.name+"/"+route+"/"+terminal, func(t *testing.T) {
					payload := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_cache\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-4\",\"content\":[],\"usage\":{\"input_tokens\":10,\"output_tokens\":2," + test.start + "}}}\n\n"
					for range 2 {
						payload += "event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":" + test.delta + "}\n\n"
					}
					payload += "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"cache_creation_input_tokens\":null,\"cache_creation\":{\"ephemeral_5m_input_tokens\":null,\"ephemeral_1h_input_tokens\":null},\"output_tokens\":2}}\n\n"
					if terminal == "stop" {
						for range 2 {
							payload += "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
						}
					}
					recorder := httptest.NewRecorder()
					ctx, _ := gin.CreateTestContext(recorder)
					ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
					response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(payload))}
					service := &GatewayService{cfg: &config.Config{}, rateLimitService: &RateLimitService{}}
					var result *ForwardResult
					var err error
					switch route {
					case "standard", "passthrough":
						var streamed *streamingResult
						if route == "standard" {
							streamed, err = service.handleStreamingResponse(context.Background(), response, ctx, &Account{ID: 701}, time.Now(), "claude-sonnet-4", "claude-sonnet-4", false)
						} else {
							streamed, err = service.handleStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), response, ctx, &Account{ID: 701}, time.Now(), "claude-sonnet-4")
							require.Equal(t, payload, recorder.Body.String())
						}
						require.NotNil(t, streamed)
						result = &ForwardResult{RequestID: "cache_creation", Model: "claude-sonnet-4", Usage: *streamed.usage, Duration: time.Second}
					case "responses":
						result, err = service.handleResponsesStreamingResponse(response, ctx, "claude-sonnet-4", "claude-sonnet-4", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
					case "buffered responses":
						result, err = service.handleResponsesBufferedStreamingResponse(response, ctx, "claude-sonnet-4", "claude-sonnet-4", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
					case "chat":
						result, err = service.handleCCStreamingFromAnthropic(response, ctx, "claude-sonnet-4", "claude-sonnet-4", nil, time.Now())
					case "buffered chat":
						result, err = service.handleCCBufferedFromAnthropic(response, ctx, "claude-sonnet-4", "claude-sonnet-4", nil, time.Now())
					}
					if terminal == "eof" && (route == "standard" || route == "passthrough") {
						require.ErrorContains(t, err, "missing terminal event")
					} else {
						require.NoError(t, err)
					}
					require.NotNil(t, result)
					require.Equal(t, 10, result.Usage.InputTokens)
					require.Equal(t, 2, result.Usage.OutputTokens)
					require.Equal(t, test.aggregate, result.Usage.CacheCreationInputTokens)
					require.True(t, result.Usage.cacheCreationInputTokensPresent)
					require.Equal(t, test.ttl5m, result.Usage.CacheCreation5mTokens)
					require.Equal(t, test.ttl1h, result.Usage.CacheCreation1hTokens)
					if route != "standard" && route != "passthrough" {
						var usage gjson.Result
						if strings.HasPrefix(route, "buffered") {
							usage = gjson.Get(recorder.Body.String(), "usage")
						} else {
							count := 0
							for _, line := range strings.Split(recorder.Body.String(), "\n") {
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
						}
						require.True(t, usage.Exists())
						if strings.HasSuffix(route, "chat") {
							require.EqualValues(t, 10+test.aggregate, usage.Get("prompt_tokens").Int())
							require.EqualValues(t, test.aggregate, usage.Get("prompt_tokens_details.cache_creation_tokens").Int())
						} else {
							require.EqualValues(t, 10+test.aggregate, usage.Get("input_tokens").Int())
							require.EqualValues(t, test.aggregate, usage.Get("cache_creation_input_tokens").Int())
						}
					}
					assertAnthropicCacheCreationResultCosts(t, result, test.cacheCost)
				})
			}
		}
	}
}

func assertAnthropicCacheCreationRecordCosts(t *testing.T, usage ClaudeUsage, wantCacheCost float64) {
	t.Helper()
	usage.InputTokens = 10
	usage.OutputTokens = 2
	assertAnthropicCacheCreationResultCosts(t, &ForwardResult{RequestID: "cache_creation", Model: "claude-sonnet-4", Usage: usage, Duration: time.Second}, wantCacheCost)
}

func assertAnthropicCacheCreationResultCosts(t *testing.T, result *ForwardResult, wantCacheCost float64) {
	t.Helper()
	assertAnthropicCacheCreationOverrideCosts(t, result, wantCacheCost, &Account{ID: 701, Platform: PlatformAnthropic}, nil, false)
}

func assertAnthropicCacheCreationOverrideCosts(t *testing.T, result *ForwardResult, wantCacheCost float64, account *Account, settings *SettingService, wantOverridden bool) {
	t.Helper()
	for _, customStats := range []bool{false, true} {
		usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
		billingRepo := &openAIRecordUsageBillingRepoStub{}
		svc := newGatewayRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{})
		svc.settingService = settings
		svc.billingService = newTestBillingServiceWithPrices(map[string]*ModelPricing{
			"claude-sonnet-4": {
				InputPricePerToken: 0.001, OutputPricePerToken: 0.002,
				CacheCreationPricePerToken: 0.002, SupportsCacheBreakdown: true,
				CacheCreation5mPrice: 0.002, CacheCreation1hPrice: 0.004,
			},
		})
		channel := &Channel{ID: 1, Status: StatusActive}
		statsMultiplier := 1.0
		if customStats {
			statsMultiplier = 10
			channel.AccountStatsPricingRules = []AccountStatsPricingRule{{
				AccountIDs: []int64{701},
				Pricing: []ChannelModelPricing{{
					Platform: PlatformAnthropic, Models: []string{"claude-sonnet-4"}, BillingMode: BillingModeToken,
					InputPrice: float64Ptr(0.01), OutputPrice: float64Ptr(0.02),
					CacheWritePrice: float64Ptr(0.02), CacheWrite1hPrice: float64Ptr(0.04),
				}},
			}}
		}
		svc.channelService = newTestChannelServiceForStats(t, channel, 10, PlatformAnthropic)
		svc.resolver = NewModelPricingResolver(svc.channelService, svc.billingService)
		groupID := int64(10)
		require.NoError(t, svc.RecordUsage(context.Background(), &RecordUsageInput{
			Result: result,
			APIKey: &APIKey{ID: 501, GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformAnthropic, RateMultiplier: 0.5}},
			User:   &User{ID: 601}, Account: account,
		}))
		require.NotNil(t, usageRepo.lastLog)
		require.Equal(t, wantOverridden, usageRepo.lastLog.CacheTTLOverridden)
		require.Equal(t, result.Usage.CacheCreationInputTokens, usageRepo.lastLog.CacheCreationTokens)
		require.Equal(t, result.Usage.CacheCreation5mTokens, usageRepo.lastLog.CacheCreation5mTokens)
		require.Equal(t, result.Usage.CacheCreation1hTokens, usageRepo.lastLog.CacheCreation1hTokens)
		require.InDelta(t, wantCacheCost, usageRepo.lastLog.CacheCreationCost, 1e-12)
		wantTotalCost := float64(result.Usage.InputTokens)*0.001 + float64(result.Usage.OutputTokens)*0.002 + wantCacheCost
		require.Equal(t, result.Usage.InputTokens, usageRepo.lastLog.InputTokens)
		require.InDelta(t, wantTotalCost, usageRepo.lastLog.TotalCost, 1e-12)
		require.InDelta(t, wantTotalCost*0.5, usageRepo.lastLog.ActualCost, 1e-12)
		require.Equal(t, 1, billingRepo.calls)
		require.InDelta(t, wantTotalCost*0.5, billingRepo.lastCmd.BalanceCost, 1e-12)
		require.NotNil(t, usageRepo.lastLog.AccountStatsCost)
		require.InDelta(t, wantTotalCost*statsMultiplier, *usageRepo.lastLog.AccountStatsCost, 1e-12)
	}
}

func TestHandleNonStreamingResponse_CacheCreationRecordUsageBoundary(t *testing.T) {
	assertAnthropicCacheCreationForwardBoundary(t, false)
}

func TestAntigravityForwardUpstream_CacheCreationRecordUsageBoundary(t *testing.T) {
	assertAnthropicCacheCreationForwardBoundary(t, true)
}

func assertAnthropicCacheCreationForwardBoundary(t *testing.T, antigravity bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name      string
		cache     string
		aggregate int
		present   bool
		cache5m   int
		cache1h   int
		cacheCost float64
	}{
		{"explicit aggregate zero", `"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_5m_input_tokens":50,"ephemeral_1h_input_tokens":200}`, 0, true, 50, 200, 0},
		{"missing aggregate", `"cache_creation":{"ephemeral_5m_input_tokens":50,"ephemeral_1h_input_tokens":200}`, 250, false, 50, 200, 0.9},
		{"null aggregate", `"cache_creation_input_tokens":null,"cache_creation":{"ephemeral_5m_input_tokens":50,"ephemeral_1h_input_tokens":200}`, 250, false, 50, 200, 0.9},
		{"positive aggregate caps TTL", `"cache_creation_input_tokens":100,"cache_creation":{"ephemeral_5m_input_tokens":50,"ephemeral_1h_input_tokens":200}`, 100, true, 50, 200, 0.36},
		{"explicit zero TTL falls back to aggregate", `"cache_creation_input_tokens":200,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":0}`, 200, true, 0, 0, 0.4},
		{"missing TTL falls back to aggregate", `"cache_creation_input_tokens":200`, 200, true, 0, 0, 0.4},
		{"null TTL falls back to aggregate", `"cache_creation_input_tokens":200,"cache_creation":{"ephemeral_5m_input_tokens":null,"ephemeral_1h_input_tokens":null}`, 200, true, 0, 0, 0.4},
		{"explicit zero TTL without aggregate", `"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":0}`, 0, false, 0, 0, 0},
	} {
		for _, stream := range []bool{false, true} {
			if !antigravity && stream {
				continue
			}
			mode := "nonstream"
			if stream {
				mode = "stream"
			}
			t.Run(test.name+"/"+mode, func(t *testing.T) {
				usageJSON := `{"input_tokens":10,"output_tokens":2,"cache_read_input_tokens":0,` + test.cache + `}`
				body := `{"id":"msg_cache","type":"message","model":"claude-sonnet-4","usage":` + usageJSON + `}`
				contentType := "application/json"
				if stream {
					contentType = "text/event-stream"
					body = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":" + usageJSON + "}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
				}
				recorder := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(recorder)
				ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(body))}
				var result *ForwardResult
				if antigravity {
					service := &AntigravityGatewayService{httpUpstream: &httpUpstreamStub{resp: response}, settingService: &SettingService{}}
					requestBody := `{"model":"claude-sonnet-4","messages":[],"stream":` + strconv.FormatBool(stream) + `}`
					var err error
					result, err = service.ForwardUpstream(context.Background(), ctx, &Account{ID: 701, Credentials: map[string]any{"base_url": "https://upstream.example", "api_key": "test-key"}}, []byte(requestBody))
					require.NoError(t, err)
					require.Equal(t, stream, result.Stream)
				} else {
					service := &GatewayService{cfg: &config.Config{}, rateLimitService: &RateLimitService{}}
					usage, err := service.handleNonStreamingResponse(context.Background(), response, ctx, &Account{ID: 701}, "claude-sonnet-4", "claude-sonnet-4")
					require.NoError(t, err)
					result = &ForwardResult{Model: "claude-sonnet-4", Usage: *usage}
				}
				require.Equal(t, http.StatusOK, recorder.Code)
				require.Equal(t, body, recorder.Body.String())
				wantAggregate := test.aggregate
				if antigravity && !test.present {
					wantAggregate = 0
				}
				require.Equal(t, wantAggregate, result.Usage.CacheCreationInputTokens)
				require.Equal(t, test.present, result.Usage.cacheCreationInputTokensPresent)
				require.Equal(t, test.cache5m, result.Usage.CacheCreation5mTokens)
				require.Equal(t, test.cache1h, result.Usage.CacheCreation1hTokens)
				assertAnthropicCacheCreationResultCosts(t, result, test.cacheCost)
			})
		}
	}
}

func TestAnthropicCacheCreationMarshaledSSEBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name          string
		aggregate     int
		present       bool
		wantAggregate int
		wantCacheCost float64
	}{
		{"missing", 0, false, 50, 0.16},
		{"explicit zero", 0, true, 0, 0},
		{"synthetic nonzero", 40, false, 40, 0.128},
	} {
		for _, terminal := range []string{"message_stop", "eof"} {
			t.Run(test.name+"/"+terminal, func(t *testing.T) {
				cache5m, cache1h := 20, 30
				events := []apicompat.AnthropicStreamEvent{
					{Type: "message_start", Message: &apicompat.AnthropicResponse{ID: "msg_cache", Type: "message", Role: "assistant", Model: "claude-sonnet-4", Usage: apicompat.AnthropicUsage{
						InputTokens: 10, CacheCreationInputTokens: test.aggregate, CacheCreationInputTokensPresent: test.present,
						CacheCreation: &apicompat.AnthropicCacheCreation{Ephemeral5mInputTokens: &cache5m},
					}}},
					{Type: "message_delta", Usage: &apicompat.AnthropicUsage{OutputTokens: 2, CacheCreation: &apicompat.AnthropicCacheCreation{Ephemeral1hInputTokens: &cache1h}}},
					{Type: "message_delta", Delta: &apicompat.AnthropicDelta{StopReason: "end_turn"}, Usage: &apicompat.AnthropicUsage{OutputTokens: 2}},
				}
				if terminal == "message_stop" {
					events = append(events, apicompat.AnthropicStreamEvent{Type: "message_stop"})
				}
				var stream strings.Builder
				for _, event := range events {
					encoded, err := apicompat.ResponsesAnthropicEventToSSE(event)
					require.NoError(t, err)
					stream.WriteString(encoded)
				}
				recorder := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(recorder)
				response := &http.Response{Body: io.NopCloser(strings.NewReader(stream.String()))}
				result, err := (&GatewayService{}).handleResponsesStreamingResponse(response, ctx, "claude-sonnet-4", "claude-sonnet-4", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
				require.NoError(t, err)
				require.Equal(t, test.wantAggregate, result.Usage.CacheCreationInputTokens)
				var completed apicompat.ResponsesStreamEvent
				for _, line := range strings.Split(recorder.Body.String(), "\n") {
					if strings.HasPrefix(line, "data: ") {
						var event apicompat.ResponsesStreamEvent
						require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event))
						if event.Type == "response.completed" {
							completed = event
						}
					}
				}
				require.NotNil(t, completed.Response)
				require.NotNil(t, completed.Response.Usage)
				require.Equal(t, 10+test.wantAggregate, completed.Response.Usage.InputTokens)
				require.Equal(t, test.wantAggregate, completed.Response.Usage.CacheCreationInputTokens)
				assertAnthropicCacheCreationRecordCosts(t, result.Usage, test.wantCacheCost)
			})
		}
	}
}

func TestRawAnthropicCacheCreationExplicitZero(t *testing.T) {
	for _, prompt := range []string{"", `,"prompt_tokens":500`} {
		usage := parseClaudeUsageFromResponseBody([]byte(`{"usage":{"input_tokens":10,"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_1h_input_tokens":200}` + prompt + `}}`))
		require.Zero(t, usage.CacheCreationInputTokens)
		require.Equal(t, 200, usage.CacheCreation1hTokens)
		assertAnthropicCacheCreationRecordCosts(t, *usage, 0)
		if prompt != "" {
			require.Equal(t, 500, usage.InputTokens)
		}
	}
	usage := &ClaudeUsage{CacheCreationInputTokens: 100}
	(&AntigravityGatewayService{}).extractSSEUsage(`data: {"type":"message_delta","usage":{"cache_creation_input_tokens":0}}`, usage)
	require.Zero(t, usage.CacheCreationInputTokens)
}

func TestAnthropicCacheCreationNullBodyParity(t *testing.T) {
	for _, test := range []struct {
		name     string
		payload  string
		want     int
		present  bool
		wantCost float64
	}{
		{"missing aggregate", `{"cache_creation":{"ephemeral_1h_input_tokens":200}}`, 200, false, 0.8},
		{"null aggregate", `{"cache_creation_input_tokens":null,"cache_creation":{"ephemeral_1h_input_tokens":200}}`, 200, false, 0.8},
		{"zero aggregate", `{"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_1h_input_tokens":200}}`, 0, true, 0},
		{"null TTL with positive aggregate", `{"cache_creation_input_tokens":100,"cache_creation":{"ephemeral_5m_input_tokens":null,"ephemeral_1h_input_tokens":null}}`, 100, true, 0.2},
		{"all null", `{"cache_creation_input_tokens":null,"cache_creation":{"ephemeral_5m_input_tokens":null,"ephemeral_1h_input_tokens":null}}`, 0, false, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			var source apicompat.AnthropicUsage
			require.NoError(t, json.Unmarshal([]byte(test.payload), &source))
			typed := &ClaudeUsage{}
			mergeAnthropicUsage(typed, source)
			raw := parseClaudeUsageFromResponseBody([]byte(`{"usage":` + test.payload + `}`))
			generic := &ClaudeUsage{}
			(&GatewayService{}).parseSSEUsage(`{"type":"message_start","message":{"usage":`+test.payload+`}}`, generic)
			require.Equal(t, typed, raw)
			require.Equal(t, typed, generic)
			require.Equal(t, test.want, raw.CacheCreationInputTokens)
			require.Equal(t, test.present, raw.cacheCreationInputTokensPresent)
			assertAnthropicCacheCreationRecordCosts(t, *raw, test.wantCost)
		})
	}
}

func TestAntigravityCacheCreationUsage(t *testing.T) {
	for _, test := range []struct {
		name      string
		payload   string
		aggregate int
		present   bool
		wantCost  float64
	}{
		{"explicit zero with TTL", `{"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_5m_input_tokens":50,"ephemeral_1h_input_tokens":200}}`, 0, true, 0},
		{"missing aggregate keeps TTL fallback", `{"cache_creation":{"ephemeral_5m_input_tokens":50,"ephemeral_1h_input_tokens":200}}`, 0, false, 0.9},
		{"null aggregate keeps TTL fallback", `{"cache_creation_input_tokens":null,"cache_creation":{"ephemeral_5m_input_tokens":50,"ephemeral_1h_input_tokens":200}}`, 0, false, 0.9},
		{"positive aggregate", `{"cache_creation_input_tokens":100,"cache_creation":{"ephemeral_1h_input_tokens":200}}`, 100, true, 0.4},
		{"all null", `{"cache_creation_input_tokens":null,"cache_creation":{"ephemeral_5m_input_tokens":null,"ephemeral_1h_input_tokens":null}}`, 0, false, 0},
	} {
		for _, parser := range []string{"body", "stream"} {
			t.Run(test.name+"/"+parser, func(t *testing.T) {
				service := &AntigravityGatewayService{}
				usage := &ClaudeUsage{}
				if parser == "body" {
					usage = service.extractClaudeUsage([]byte(`{"usage":` + test.payload + `}`))
				} else {
					service.extractSSEUsage(`data: {"type":"message_start","message":{"usage":`+test.payload+`}}`, usage)
				}
				require.Equal(t, test.aggregate, usage.CacheCreationInputTokens)
				require.Equal(t, test.present, usage.cacheCreationInputTokensPresent)
				require.Equal(t, int(gjson.Get(test.payload, "cache_creation.ephemeral_5m_input_tokens").Int()), usage.CacheCreation5mTokens)
				require.Equal(t, int(gjson.Get(test.payload, "cache_creation.ephemeral_1h_input_tokens").Int()), usage.CacheCreation1hTokens)
				assertAnthropicCacheCreationRecordCosts(t, *usage, test.wantCost)
			})
		}
	}
}

func TestAntigravityCacheCreationLateDeltas(t *testing.T) {
	for _, test := range []struct {
		name      string
		aggregate string
		wantTotal int
		present   bool
		wantCost  float64
	}{
		{"explicit zero survives", `"cache_creation_input_tokens":0,`, 0, true, 0},
		{"missing aggregate keeps TTL fallback", "", 0, false, 1.3},
		{"null aggregate keeps TTL fallback", `"cache_creation_input_tokens":null,`, 0, false, 1.3},
		{"positive aggregate survives", `"cache_creation_input_tokens":100,`, 100, true, 0.372},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &AntigravityGatewayService{}
			usage := &ClaudeUsage{}
			service.extractSSEUsage(`data: {"type":"message_start","message":{"usage":{`+test.aggregate+`"cache_creation":{"ephemeral_1h_input_tokens":200}}}}`, usage)
			for _, delta := range []string{
				`{"cache_creation":{"ephemeral_5m_input_tokens":50}}`,
				`{"cache_creation_input_tokens":null,"cache_creation":{"ephemeral_5m_input_tokens":null,"ephemeral_1h_input_tokens":300}}`,
				`{"cache_creation_input_tokens":null,"cache_creation":{"ephemeral_5m_input_tokens":null,"ephemeral_1h_input_tokens":null}}`,
			} {
				service.extractSSEUsage(`data: {"type":"message_delta","usage":`+delta+`}`, usage)
				require.Equal(t, test.wantTotal, usage.CacheCreationInputTokens)
				require.Equal(t, test.present, usage.cacheCreationInputTokensPresent)
			}
			require.Equal(t, 50, usage.CacheCreation5mTokens)
			require.Equal(t, 300, usage.CacheCreation1hTokens)
			assertAnthropicCacheCreationRecordCosts(t, *usage, test.wantCost)
		})
	}

	t.Run("explicit zero delta clears aggregate with retained TTL", func(t *testing.T) {
		service := &AntigravityGatewayService{}
		usage := &ClaudeUsage{}
		service.extractSSEUsage(`data: {"type":"message_start","message":{"usage":{"cache_creation_input_tokens":100,"cache_creation":{"ephemeral_1h_input_tokens":200}}}}`, usage)
		service.extractSSEUsage(`data: {"type":"message_delta","usage":{"cache_creation_input_tokens":0}}`, usage)
		service.extractSSEUsage(`data: {"type":"message_delta","usage":{"cache_creation_input_tokens":null,"cache_creation":{"ephemeral_5m_input_tokens":50}}}`, usage)
		require.Zero(t, usage.CacheCreationInputTokens)
		require.True(t, usage.cacheCreationInputTokensPresent)
		require.Equal(t, 50, usage.CacheCreation5mTokens)
		require.Equal(t, 200, usage.CacheCreation1hTokens)
		assertAnthropicCacheCreationRecordCosts(t, *usage, 0)
	})
}

func TestAnthropicCacheOnlyBufferedSSEUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name          string
		start         string
		delta         string
		wantAggregate int
	}{
		{"explicit zero transition", `{"cache_creation_input_tokens":100,"cache_creation":{"ephemeral_1h_input_tokens":200}}`, `{"cache_creation_input_tokens":0}`, 0},
		{"derived zero transition", `{"cache_creation":{"ephemeral_5m_input_tokens":20}}`, `{"cache_creation":{"ephemeral_5m_input_tokens":0}}`, 0},
		{"merged partial TTL", `{"cache_creation":{"ephemeral_5m_input_tokens":20}}`, `{"cache_creation_input_tokens":null,"cache_creation":{"ephemeral_5m_input_tokens":null,"ephemeral_1h_input_tokens":30}}`, 50},
	} {
		for _, adapter := range []string{"responses", "chat", "native responses", "native chat"} {
			t.Run(test.name+"/"+adapter, func(t *testing.T) {
				stream := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_cache\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-4\",\"content\":[],\"usage\":" + test.start + "}}\n\n" +
					"event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":" + test.delta + "}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
				recorder := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(recorder)
				response := &http.Response{Body: io.NopCloser(strings.NewReader(stream))}
				var aggregate int
				var err error
				switch adapter {
				case "responses":
					var result *ForwardResult
					result, err = (&GatewayService{}).handleResponsesBufferedStreamingResponse(response, ctx, "claude-sonnet-4", "claude-sonnet-4", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
					require.NoError(t, err)
					aggregate = result.Usage.CacheCreationInputTokens
				case "chat":
					var result *ForwardResult
					result, err = (&GatewayService{}).handleCCBufferedFromAnthropic(response, ctx, "claude-sonnet-4", "claude-sonnet-4", nil, time.Now())
					require.NoError(t, err)
					aggregate = result.Usage.CacheCreationInputTokens
				case "native responses":
					var result *OpenAIForwardResult
					result, err = (&OpenAIGatewayService{}).handleResponsesBufferedFromNativeAnthropic(response, ctx, "claude-sonnet-4", "claude-sonnet-4", "claude-sonnet-4", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
					require.NoError(t, err)
					aggregate = result.Usage.CacheCreationInputTokens
				case "native chat":
					var result *OpenAIForwardResult
					result, err = (&OpenAIGatewayService{}).handleCCBufferedFromNativeAnthropic(response, ctx, "claude-sonnet-4", "claude-sonnet-4", "claude-sonnet-4", nil, time.Now())
					require.NoError(t, err)
					aggregate = result.Usage.CacheCreationInputTokens
				}
				require.Equal(t, test.wantAggregate, aggregate)
				require.Equal(t, http.StatusOK, recorder.Code)
				require.True(t, json.Valid(recorder.Body.Bytes()))
				inputPath := "usage.input_tokens"
				cachePath := "usage.cache_creation_input_tokens"
				if strings.HasSuffix(adapter, "chat") {
					inputPath = "usage.prompt_tokens"
					cachePath = "usage.prompt_tokens_details.cache_creation_tokens"
				}
				require.True(t, gjson.GetBytes(recorder.Body.Bytes(), inputPath).Exists())
				require.Equal(t, int64(test.wantAggregate), gjson.GetBytes(recorder.Body.Bytes(), inputPath).Int())
				require.Equal(t, int64(test.wantAggregate), gjson.GetBytes(recorder.Body.Bytes(), cachePath).Int())
			})
		}
	}
}
