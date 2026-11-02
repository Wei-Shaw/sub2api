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
)

func TestApplyCacheTTLOverrideAuthoritativeTotal(t *testing.T) {
	for _, test := range []struct {
		name    string
		usage   ClaudeUsage
		total   int
		changed bool
	}{
		{"partial details", ClaudeUsage{CacheCreationInputTokens: 11000, CacheCreation1hTokens: 3000, cacheCreationInputTokensPresent: true}, 11000, true},
		{"oversized details", ClaudeUsage{CacheCreationInputTokens: 100, CacheCreation5mTokens: 50, CacheCreation1hTokens: 200, cacheCreationInputTokensPresent: true}, 100, true},
		{"explicit zero", ClaudeUsage{CacheCreation5mTokens: 50, CacheCreation1hTokens: 200, cacheCreationInputTokensPresent: true}, 0, true},
		{"synthetic positive aggregate", ClaudeUsage{CacheCreationInputTokens: 11000, CacheCreation1hTokens: 3000}, 11000, true},
		{"synthetic detail only", ClaudeUsage{CacheCreation5mTokens: 50, CacheCreation1hTokens: 200}, 250, true},
		{"synthetic negative detail", ClaudeUsage{CacheCreation5mTokens: -50, CacheCreation1hTokens: 200}, 200, true},
		{"aggregate only", ClaudeUsage{CacheCreationInputTokens: 11000}, 11000, true},
		{"empty", ClaudeUsage{}, 0, false},
	} {
		for _, target := range []string{"5m", "1h"} {
			t.Run(test.name+"/"+target, func(t *testing.T) {
				usage := test.usage
				require.Equal(t, test.changed, applyCacheTTLOverride(&usage, target))
				want5m, want1h := 0, test.total
				if target == "5m" {
					want5m, want1h = test.total, 0
				}
				require.Equal(t, want5m, usage.CacheCreation5mTokens)
				require.Equal(t, want1h, usage.CacheCreation1hTokens)
				require.Equal(t, test.usage.CacheCreationInputTokens, usage.CacheCreationInputTokens)
				require.Equal(t, test.usage.cacheCreationInputTokensPresent, usage.cacheCreationInputTokensPresent)
				snapshot := usage
				require.False(t, applyCacheTTLOverride(&usage, target))
				require.Equal(t, snapshot, usage)
			})
		}
	}
}

type cacheTTLOverrideBoundaryCase struct {
	name      string
	snapshots []string
	aggregate int
	present   bool
	ttl5m     int
	ttl1h     int
	rawCost   float64
}

func cacheTTLOverrideBoundaryCases() []cacheTTLOverrideBoundaryCase {
	return []cacheTTLOverrideBoundaryCase{
		{"growing aggregate", []string{`{"cache_creation_input_tokens":3000,"cache_creation":{"ephemeral_1h_input_tokens":3000}}`, `{"cache_creation_input_tokens":11000}`}, 11000, true, 0, 3000, 28},
		{"explicit zero delta", []string{`{"cache_creation_input_tokens":3000,"cache_creation":{"ephemeral_1h_input_tokens":3000}}`, `{"cache_creation_input_tokens":0}`}, 0, true, 0, 3000, 0},
		{"oversized details", []string{`{"cache_creation_input_tokens":100,"cache_creation":{"ephemeral_5m_input_tokens":50,"ephemeral_1h_input_tokens":200}}`}, 100, true, 50, 200, 0.36},
		{"explicit zero snapshot", []string{`{"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_5m_input_tokens":50,"ephemeral_1h_input_tokens":200}}`}, 0, true, 50, 200, 0},
		{"detail only", []string{`{"cache_creation":{"ephemeral_5m_input_tokens":50,"ephemeral_1h_input_tokens":200}}`}, 250, false, 50, 200, 0.9},
		{"null aggregate", []string{`{"cache_creation_input_tokens":null,"cache_creation":{"ephemeral_5m_input_tokens":50,"ephemeral_1h_input_tokens":200}}`}, 250, false, 50, 200, 0.9},
		{"aggregate only", []string{`{"cache_creation_input_tokens":11000}`}, 11000, true, 0, 0, 22},
	}
}

func cacheTTLOverrideBoundaryAccount(t *testing.T, mode string) (*Account, *SettingService, string) {
	t.Helper()
	resetGatewayForwardingSettingsCacheForTest(t)
	account := &Account{ID: 701, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	var settings *SettingService
	target := ""
	if strings.Contains(mode, "global") {
		settings = NewSettingService(&gatewayTTLSettingRepo{data: map[string]string{
			SettingKeyEnableAnthropicCacheTTL1hInjection: "true",
		}}, &config.Config{})
		target = "5m"
	}
	if strings.HasPrefix(mode, "account") {
		target = strings.Split(mode, "/")[1]
		account.Extra = map[string]any{"cache_ttl_override_enabled": true, "cache_ttl_override_target": target}
	}
	return account, settings, target
}

func cacheTTLOverrideBoundaryResponse(test cacheTTLOverrideBoundaryCase, stream bool, cacheRead int) string {
	base := `"input_tokens":10,"output_tokens":2,"cache_read_input_tokens":` + strconv.Itoa(cacheRead)
	if !stream {
		cache := `"cache_creation":{"ephemeral_5m_input_tokens":` + strconv.Itoa(test.ttl5m) + `,"ephemeral_1h_input_tokens":` + strconv.Itoa(test.ttl1h) + `}`
		if test.present {
			cache += `,"cache_creation_input_tokens":` + strconv.Itoa(test.aggregate)
		}
		return `{"id":"msg_cache","type":"message","role":"assistant","model":"k3","content":[],"usage":{` + base + `,` + cache + `}}`
	}
	payload := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_cache\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"k3\",\"content\":[],\"usage\":{" + base + "," + strings.TrimPrefix(test.snapshots[0], "{") + "}}\n\n"
	for _, snapshot := range test.snapshots {
		for range 2 {
			payload += "event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":" + snapshot + "}\n\n"
		}
	}
	payload += "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	return payload
}

func TestCacheTTLOverrideForwardBillingBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range cacheTTLOverrideBoundaryCases() {
		for _, route := range []string{"nonstream", "standard", "passthrough", "responses", "buffered responses", "chat", "buffered chat"} {
			for _, mode := range []string{"raw", "account/5m", "account/1h", "global", "account/1h/global"} {
				t.Run(test.name+"/"+route+"/"+mode, func(t *testing.T) {
					account, settings, target := cacheTTLOverrideBoundaryAccount(t, mode)
					payload := cacheTTLOverrideBoundaryResponse(test, route != "nonstream", 0)
					contentType := "text/event-stream"
					if route == "nonstream" {
						contentType = "application/json"
					}
					recorder := httptest.NewRecorder()
					ctx, _ := gin.CreateTestContext(recorder)
					ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
					response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(payload))}
					service := &GatewayService{cfg: &config.Config{}, rateLimitService: &RateLimitService{}, settingService: settings}
					var result *ForwardResult
					var err error
					switch route {
					case "nonstream":
						var usage *ClaudeUsage
						usage, err = service.handleNonStreamingResponse(context.Background(), response, ctx, account, "k3", "k3")
						require.NotNil(t, usage)
						result = &ForwardResult{Usage: *usage}
					case "standard", "passthrough":
						var streamed *streamingResult
						if route == "standard" {
							streamed, err = service.handleStreamingResponse(context.Background(), response, ctx, account, time.Now(), "k3", "k3", false)
						} else {
							streamed, err = service.handleStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), response, ctx, account, time.Now(), "k3")
						}
						require.NotNil(t, streamed)
						result = &ForwardResult{Usage: *streamed.usage}
					case "responses":
						result, err = service.handleResponsesStreamingResponse(response, ctx, "k3", "k3", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
					case "buffered responses":
						result, err = service.handleResponsesBufferedStreamingResponse(response, ctx, "k3", "k3", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
					case "chat":
						result, err = service.handleCCStreamingFromAnthropic(response, ctx, "k3", "k3", nil, time.Now())
					case "buffered chat":
						result, err = service.handleCCBufferedFromAnthropic(response, ctx, "k3", "k3", nil, time.Now())
					}
					require.NoError(t, err)
					require.NotNil(t, result)
					require.Equal(t, http.StatusOK, recorder.Code)
					require.Equal(t, test.aggregate, result.Usage.CacheCreationInputTokens)
					require.Equal(t, test.present, result.Usage.cacheCreationInputTokensPresent)
					if target == "" || route == "passthrough" {
						if route == "nonstream" || route == "standard" || route == "passthrough" {
							require.Equal(t, payload, recorder.Body.String())
						}
					}
					if target == "" || (route != "standard" && route != "nonstream") {
						require.Equal(t, test.ttl5m, result.Usage.CacheCreation5mTokens)
						require.Equal(t, test.ttl1h, result.Usage.CacheCreation1hTokens)
					}
					want5m, want1h, wantCost := test.ttl5m, test.ttl1h, test.rawCost
					if target == "5m" {
						want5m, want1h, wantCost = test.aggregate, 0, float64(test.aggregate)*0.002
					} else if target == "1h" {
						want5m, want1h, wantCost = 0, test.aggregate, float64(test.aggregate)*0.004
					}
					if target != "" && (route == "nonstream" || (route == "standard" && test.aggregate > 0)) {
						require.Equal(t, want5m, result.Usage.CacheCreation5mTokens)
						require.Equal(t, want1h, result.Usage.CacheCreation1hTokens)
					}
					result.Model = "claude-sonnet-4"
					result.UpstreamModel = "claude-sonnet-4"
					assertAnthropicCacheCreationOverrideCosts(t, result, wantCost, account, settings, target != "" && test.aggregate > 0)
					require.Equal(t, test.aggregate, result.Usage.CacheCreationInputTokens)
					require.Equal(t, want5m, result.Usage.CacheCreation5mTokens)
					require.Equal(t, want1h, result.Usage.CacheCreation1hTokens)
				})
			}
		}
	}
}

func TestNativeCacheTTLOverridePreservesRawBillingBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range cacheTTLOverrideBoundaryCases() {
		for _, route := range []string{"messages", "chat", "responses"} {
			for _, stream := range []bool{false, true} {
				for _, mode := range []string{"raw", "account/5m", "account/1h", "global", "account/1h/global"} {
					t.Run(test.name+"/"+route+"/stream="+strconv.FormatBool(stream)+"/"+mode, func(t *testing.T) {
						overrideAccount, settings, _ := cacheTTLOverrideBoundaryAccount(t, mode)
						account := nativeAnthropicTestAccount()
						account.Extra = overrideAccount.Extra
						payload := cacheTTLOverrideBoundaryResponse(test, stream || route != "messages", 7)
						contentType := "text/event-stream"
						if route == "messages" && !stream {
							contentType = "application/json"
						}
						upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(payload))}}
						service := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream, settingService: settings}
						body := []byte(`{"model":"k3","max_tokens":32,"messages":[{"role":"user","content":"hi"}],"stream":` + strconv.FormatBool(stream) + `}`)
						path := "/v1/messages"
						if route == "chat" {
							path = "/v1/chat/completions"
						} else if route == "responses" {
							path = "/v1/responses"
							body = []byte(`{"model":"k3","input":"hi","stream":` + strconv.FormatBool(stream) + `}`)
						}
						recorder := httptest.NewRecorder()
						ctx, _ := gin.CreateTestContext(recorder)
						ctx.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(body)))
						var result *OpenAIForwardResult
						var err error
						switch route {
						case "messages":
							result, err = service.ForwardAsAnthropic(context.Background(), ctx, account, body, "", "")
						case "chat":
							result, err = service.ForwardAsChatCompletions(context.Background(), ctx, account, body, "", "")
						case "responses":
							result, err = service.Forward(context.Background(), ctx, account, body)
						}
						require.NoError(t, err)
						require.NotNil(t, result)
						require.Equal(t, http.StatusOK, recorder.Code)
						require.Equal(t, 17+test.aggregate, result.Usage.InputTokens)
						require.Equal(t, test.aggregate, result.Usage.CacheCreationInputTokens)
						require.Equal(t, test.present, result.Usage.cacheCreationInputTokensPresent)
						require.Equal(t, test.ttl5m, result.Usage.CacheCreation5mTokens)
						require.Equal(t, test.ttl1h, result.Usage.CacheCreation1hTokens)
						if route == "messages" {
							require.Equal(t, payload, recorder.Body.String())
						} else {
							assertNativeCacheCompatibleUsage(t, recorder.Body.String(), route, stream, 10, test.aggregate)
						}
						assertNativeCacheRecordUsageWithSettings(t, result, account, 10, test.rawCost, settings)
					})
				}
			}
		}
	}
}

func TestRewriteCacheCreationJSONAuthoritativeTotal(t *testing.T) {
	for _, test := range cacheTTLOverrideBoundaryCases() {
		for _, target := range []string{"5m", "1h"} {
			t.Run(test.name+"/"+target, func(t *testing.T) {
				var response struct {
					Usage map[string]any `json:"usage"`
				}
				require.NoError(t, json.Unmarshal([]byte(cacheTTLOverrideBoundaryResponse(test, false, 0)), &response))
				aggregate, present := response.Usage["cache_creation_input_tokens"]
				require.True(t, rewriteCacheCreationJSON(response.Usage, target))
				want5m, want1h := 0, test.aggregate
				if target == "5m" {
					want5m, want1h = test.aggregate, 0
				}
				details := response.Usage["cache_creation"].(map[string]any)
				require.EqualValues(t, want5m, details["ephemeral_5m_input_tokens"])
				require.EqualValues(t, want1h, details["ephemeral_1h_input_tokens"])
				require.False(t, rewriteCacheCreationJSON(response.Usage, target))
				afterAggregate, afterPresent := response.Usage["cache_creation_input_tokens"]
				require.Equal(t, aggregate, afterAggregate)
				require.Equal(t, present, afterPresent)
			})
		}
	}
}
