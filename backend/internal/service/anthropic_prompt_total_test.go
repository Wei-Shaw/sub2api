//go:build unit

package service

import (
	"context"
	"encoding/json"
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
	"github.com/tidwall/gjson"
)

func TestAnthropicPromptTotalSnapshots(t *testing.T) {
	for _, test := range []struct {
		name   string
		start  string
		deltas []string
		inputs []int
	}{
		{"prompt total", `"prompt_tokens":100`, []string{`{"cache_creation":{"ephemeral_1h_input_tokens":30}}`, `{"cache_creation":{"ephemeral_1h_input_tokens":30}}`, `{"cache_creation":{"ephemeral_5m_input_tokens":0}}`, `{"cache_read_input_tokens":10}`, `{"prompt_tokens":120}`, `{"output_tokens":2}`}, []int{50, 50, 70, 60, 80, 80}},
		{"independent input", `"input_tokens":80`, []string{`{"cache_creation":{"ephemeral_1h_input_tokens":30}}`, `{"cache_creation":{"ephemeral_1h_input_tokens":30}}`}, []int{80, 80}},
		{"miss overrides total", `"prompt_tokens":100`, []string{`{"prompt_tokens":100,"prompt_cache_miss_tokens":40}`, `{"cache_creation":{"ephemeral_1h_input_tokens":30}}`}, []int{40, 40}},
		{"late read aliases", `"prompt_tokens":100`, []string{`{"cached_tokens":10}`, `{"cached_tokens":15}`, `{"prompt_tokens_details":{"cached_tokens":20}}`, `{"prompt_cache_hit_tokens":25}`, `{"cache_creation":{"ephemeral_1h_input_tokens":30}}`}, []int{70, 65, 60, 55, 25}},
		{"aggregate clears", `"prompt_tokens":100`, []string{`{"cache_creation_input_tokens":50}`, `{"cache_creation_input_tokens":0}`, `{"cache_creation":{"ephemeral_1h_input_tokens":30}}`}, []int{50, 100, 100}},
		{"cache exceeds prompt", `"prompt_tokens":100`, []string{`{"cache_creation":{"ephemeral_1h_input_tokens":100}}`, `{"cache_creation":{"ephemeral_1h_input_tokens":30}}`}, []int{0, 50}},
	} {
		for _, parser := range []string{"typed", "raw", "generic"} {
			t.Run(test.name+"/"+parser, func(t *testing.T) {
				usage := &ClaudeUsage{}
				merge := func(snapshot string, start bool) {
					if parser == "typed" {
						var source apicompat.AnthropicUsage
						require.NoError(t, json.Unmarshal([]byte(snapshot), &source))
						mergeAnthropicUsage(usage, source)
						return
					}
					data := `{"type":"message_delta","usage":` + snapshot + `}`
					if start {
						data = `{"type":"message_start","message":{"usage":` + snapshot + `}}`
					}
					if parser == "raw" {
						parseSSEUsagePassthrough(data, usage)
					} else {
						(&GatewayService{}).parseSSEUsage(data, usage)
					}
				}
				merge(`{`+test.start+`,"cache_creation":{"ephemeral_5m_input_tokens":20}}`, true)
				require.Equal(t, 80, usage.InputTokens)
				for index, delta := range test.deltas {
					merge(delta, false)
					require.Equal(t, test.inputs[index], usage.InputTokens, "delta %d", index)
				}
			})
		}
	}
}

func TestAnthropicPromptTotalCacheReadPresence(t *testing.T) {
	for _, test := range []struct {
		name  string
		delta string
		read  int
	}{
		{"canonical zero", `{"cache_read_input_tokens":0}`, 0},
		{"cached zero", `{"cached_tokens":0}`, 0},
		{"details zero", `{"prompt_tokens_details":{"cached_tokens":0}}`, 0},
		{"hit zero", `{"prompt_cache_hit_tokens":0}`, 0},
		{"missing", `{"output_tokens":2}`, 20},
		{"null", `{"cache_read_input_tokens":null,"cached_tokens":null,"prompt_tokens_details":{"cached_tokens":null},"prompt_cache_hit_tokens":null}`, 20},
		{"canonical precedence", `{"cache_read_input_tokens":12,"cached_tokens":13,"prompt_tokens_details":{"cached_tokens":14},"prompt_cache_hit_tokens":15}`, 12},
		{"zero falls back", `{"cache_read_input_tokens":0,"cached_tokens":13,"prompt_tokens_details":{"cached_tokens":14},"prompt_cache_hit_tokens":15}`, 13},
		{"alias zero falls back", `{"cached_tokens":0,"prompt_tokens_details":{"cached_tokens":14},"prompt_cache_hit_tokens":15}`, 14},
		{"details zero falls back", `{"prompt_tokens_details":{"cached_tokens":0},"prompt_cache_hit_tokens":15}`, 15},
	} {
		for _, parser := range []string{"typed", "raw", "generic"} {
			t.Run(test.name+"/"+parser, func(t *testing.T) {
				usage := &ClaudeUsage{}
				merge := func(snapshot string, start bool) {
					if parser == "typed" {
						var source apicompat.AnthropicUsage
						require.NoError(t, json.Unmarshal([]byte(snapshot), &source))
						mergeAnthropicUsage(usage, source)
						return
					}
					data := `{"type":"message_delta","usage":` + snapshot + `}`
					if start {
						data = `{"type":"message_start","message":{"usage":` + snapshot + `}}`
					}
					if parser == "raw" {
						parseSSEUsagePassthrough(data, usage)
					} else {
						(&GatewayService{}).parseSSEUsage(data, usage)
					}
				}
				merge(`{"prompt_tokens":100,"cache_read_input_tokens":20,"cache_creation_input_tokens":10}`, true)
				require.Equal(t, 70, usage.InputTokens)
				merge(test.delta, false)
				require.Equal(t, test.read, usage.CacheReadInputTokens)
				require.Equal(t, 90-test.read, usage.InputTokens)
				merge(`{"output_tokens":3}`, false)
				require.Equal(t, test.read, usage.CacheReadInputTokens)
				require.Equal(t, 90-test.read, usage.InputTokens)
			})
		}
	}
}

func TestAnthropicPromptTotalCacheReadZeroForwardBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, field := range []string{"cache_read_input_tokens", "cached_tokens"} {
		for _, route := range []string{"typed", "raw", "generic"} {
			for _, terminal := range []string{"stop", "eof"} {
				t.Run(field+"/"+route+"/"+terminal, func(t *testing.T) {
					payload := "event: message_start\ndata: " + `{"type":"message_start","message":{"id":"msg_zero","type":"message","role":"assistant","model":"claude-sonnet-4","content":[],"usage":{"input_tokens":100,"prompt_tokens":100,"cache_read_input_tokens":20,"cache_creation_input_tokens":10}}}` + "\n\n"
					payload += "event: message_delta\ndata: " + `{"type":"message_delta","usage":{"` + field + `":0}}` + "\n\n"
					payload += "event: message_delta\ndata: " + `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}` + "\n\n"
					if terminal == "stop" {
						payload += "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
					}
					response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(payload))}
					recorder := httptest.NewRecorder()
					ctx, _ := gin.CreateTestContext(recorder)
					ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
					service := &GatewayService{cfg: &config.Config{}, rateLimitService: &RateLimitService{}}
					var usage *ClaudeUsage
					var err error
					if route == "typed" {
						var result *ForwardResult
						result, err = service.handleResponsesStreamingResponse(response, ctx, "claude-sonnet-4", "claude-sonnet-4", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
						require.NotNil(t, result)
						usage = &result.Usage
					} else {
						var result *streamingResult
						if route == "raw" {
							result, err = service.handleStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), response, ctx, &Account{ID: 701}, time.Now(), "claude-sonnet-4")
						} else {
							result, err = service.handleStreamingResponse(context.Background(), response, ctx, &Account{ID: 701}, time.Now(), "claude-sonnet-4", "claude-sonnet-4", false)
						}
						require.NotNil(t, result)
						usage = result.usage
					}
					if terminal == "eof" && route != "typed" {
						require.ErrorContains(t, err, "missing terminal event")
					} else {
						require.NoError(t, err)
					}
					require.NotNil(t, usage)
					require.Equal(t, 90, usage.InputTokens)
					require.Zero(t, usage.CacheReadInputTokens)
					require.Equal(t, 10, usage.CacheCreationInputTokens)
					require.Equal(t, 2, usage.OutputTokens)
				})
			}
		}
	}
}

func TestAnthropicCacheReadZeroPreservesIndependentInput(t *testing.T) {
	for _, field := range []string{"cache_read_input_tokens", "cached_tokens"} {
		for _, parser := range []string{"typed", "generic", "raw"} {
			t.Run(field+"/"+parser, func(t *testing.T) {
				usage := &ClaudeUsage{}
				for index, snapshot := range []string{
					`{"input_tokens":80,"cache_read_input_tokens":20,"cache_creation_input_tokens":10}`,
					`{"` + field + `":0}`,
				} {
					if parser == "typed" {
						var source apicompat.AnthropicUsage
						require.NoError(t, json.Unmarshal([]byte(snapshot), &source))
						mergeAnthropicUsage(usage, source)
					} else if parser == "raw" {
						if index == 0 {
							parseSSEUsagePassthrough(`{"type":"message_start","message":{"usage":`+snapshot+`}}`, usage)
						} else {
							parseSSEUsagePassthrough(`{"type":"message_delta","usage":`+snapshot+`}`, usage)
						}
					} else if index == 0 {
						(&GatewayService{}).parseSSEUsage(`{"type":"message_start","message":{"usage":`+snapshot+`}}`, usage)
					} else {
						(&GatewayService{}).parseSSEUsage(`{"type":"message_delta","usage":`+snapshot+`}`, usage)
					}
					require.Equal(t, 80, usage.InputTokens)
				}
				require.Zero(t, usage.CacheReadInputTokens)
				require.Equal(t, 10, usage.CacheCreationInputTokens)
			})
		}
	}
}

func TestAnthropicPromptTotalForwardBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, source := range []string{"prompt total", "independent input"} {
		for _, route := range []string{"messages", "passthrough", "native messages", "responses", "buffered responses", "chat", "buffered chat", "native responses", "buffered native responses", "native chat", "buffered native chat"} {
			for _, terminal := range []string{"stop", "eof"} {
				t.Run(source+"/"+route+"/"+terminal, func(t *testing.T) {
					inputField, wantInput := `"input_tokens":80`, 80
					if source == "prompt total" {
						inputField, wantInput = `"input_tokens":100,"prompt_tokens":100`, 50
					}
					payload := "event: message_start\ndata: " + `{"type":"message_start","message":{"id":"msg_total","type":"message","role":"assistant","model":"claude-sonnet-4","content":[],"usage":{` + inputField + `,"cache_creation":{"ephemeral_5m_input_tokens":20}}}}` + "\n\n"
					for range 2 {
						payload += "event: message_delta\ndata: " + `{"type":"message_delta","usage":{"cache_creation":{"ephemeral_1h_input_tokens":30}}}` + "\n\n"
					}
					payload += "event: message_delta\ndata: " + `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}` + "\n\n"
					if terminal == "stop" {
						payload += "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
					}
					response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(payload))}
					recorder := httptest.NewRecorder()
					ctx, _ := gin.CreateTestContext(recorder)
					ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
					service := &GatewayService{cfg: &config.Config{}, rateLimitService: &RateLimitService{}}
					native := &OpenAIGatewayService{cfg: &config.Config{}}
					var result *ForwardResult
					var nativeResult *OpenAIForwardResult
					var err error
					switch route {
					case "messages", "passthrough":
						var streamed *streamingResult
						if route == "messages" {
							streamed, err = service.handleStreamingResponse(context.Background(), response, ctx, &Account{ID: 701}, time.Now(), "claude-sonnet-4", "claude-sonnet-4", false)
						} else {
							streamed, err = service.handleStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), response, ctx, &Account{ID: 701}, time.Now(), "claude-sonnet-4")
						}
						require.NotNil(t, streamed)
						result = &ForwardResult{Usage: *streamed.usage}
					case "native messages":
						nativeResult, err = native.handleNativeAnthropicStreamingResponse(context.Background(), response, ctx, &Account{ID: 701}, "claude-sonnet-4", "claude-sonnet-4", "claude-sonnet-4", nil, time.Now())
					case "responses":
						result, err = service.handleResponsesStreamingResponse(response, ctx, "claude-sonnet-4", "claude-sonnet-4", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
					case "buffered responses":
						result, err = service.handleResponsesBufferedStreamingResponse(response, ctx, "claude-sonnet-4", "claude-sonnet-4", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
					case "chat":
						result, err = service.handleCCStreamingFromAnthropic(response, ctx, "claude-sonnet-4", "claude-sonnet-4", nil, time.Now())
					case "buffered chat":
						result, err = service.handleCCBufferedFromAnthropic(response, ctx, "claude-sonnet-4", "claude-sonnet-4", nil, time.Now())
					case "native responses":
						nativeResult, err = native.handleResponsesStreamingFromNativeAnthropic(response, ctx, "claude-sonnet-4", "claude-sonnet-4", "claude-sonnet-4", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
					case "buffered native responses":
						nativeResult, err = native.handleResponsesBufferedFromNativeAnthropic(response, ctx, "claude-sonnet-4", "claude-sonnet-4", "claude-sonnet-4", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
					case "native chat":
						nativeResult, err = native.handleCCStreamingFromNativeAnthropic(response, ctx, "claude-sonnet-4", "claude-sonnet-4", "claude-sonnet-4", nil, time.Now())
					case "buffered native chat":
						nativeResult, err = native.handleCCBufferedFromNativeAnthropic(response, ctx, "claude-sonnet-4", "claude-sonnet-4", "claude-sonnet-4", nil, time.Now())
					}
					if terminal == "eof" && (route == "messages" || route == "passthrough" || route == "native messages") {
						require.ErrorContains(t, err, "missing terminal event")
					} else {
						require.NoError(t, err)
					}
					if nativeResult != nil {
						require.Equal(t, wantInput+50, nativeResult.Usage.InputTokens)
						billingResult := *nativeResult
						billingResult.Model, billingResult.BillingModel, billingResult.UpstreamModel = "k3", "kimi-k3", "k3"
						assertNativeCacheRecordUsage(t, &billingResult, nativeAnthropicTestAccount(), wantInput, 0.16)
						result = &ForwardResult{Usage: ClaudeUsage{InputTokens: nativeResult.Usage.InputTokens - nativeResult.Usage.CacheCreationInputTokens, OutputTokens: nativeResult.Usage.OutputTokens, CacheCreationInputTokens: nativeResult.Usage.CacheCreationInputTokens, CacheCreation5mTokens: nativeResult.Usage.CacheCreation5mTokens, CacheCreation1hTokens: nativeResult.Usage.CacheCreation1hTokens}}
					}
					require.NotNil(t, result)
					require.Equal(t, wantInput, result.Usage.InputTokens)
					require.Equal(t, 2, result.Usage.OutputTokens)
					require.Equal(t, 50, result.Usage.CacheCreationInputTokens)
					require.Equal(t, 20, result.Usage.CacheCreation5mTokens)
					require.Equal(t, 30, result.Usage.CacheCreation1hTokens)
					result.Model = "claude-sonnet-4"
					assertAnthropicCacheCreationResultCosts(t, result, 0.16)
					if strings.Contains(route, "messages") || route == "passthrough" {
						return
					}
					var clientUsage gjson.Result
					if strings.HasPrefix(route, "buffered") {
						clientUsage = gjson.GetBytes(recorder.Body.Bytes(), "usage")
					} else {
						count := 0
						for _, line := range strings.Split(recorder.Body.String(), "\n") {
							if !strings.HasPrefix(line, "data: ") {
								continue
							}
							data := strings.TrimPrefix(line, "data: ")
							if gjson.Get(data, "type").String() == "response.completed" {
								clientUsage = gjson.Get(data, "response.usage")
								count++
							} else if gjson.Get(data, "usage").Exists() {
								clientUsage = gjson.Get(data, "usage")
								count++
							}
						}
						require.Equal(t, 1, count)
					}
					require.True(t, clientUsage.Exists())
					inputPath, creationPath := "input_tokens", "cache_creation_input_tokens"
					if strings.HasSuffix(route, "chat") {
						inputPath, creationPath = "prompt_tokens", "prompt_tokens_details.cache_creation_tokens"
					}
					require.EqualValues(t, wantInput+50, clientUsage.Get(inputPath).Int())
					require.EqualValues(t, 50, clientUsage.Get(creationPath).Int())
				})
			}
		}
	}
}

func TestAnthropicPromptTotalBufferedMessages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, source := range []string{"prompt total", "independent input"} {
		for _, route := range []string{"messages", "passthrough", "native messages"} {
			t.Run(source+"/"+route, func(t *testing.T) {
				inputField, wantInput := `"input_tokens":80`, 80
				if source == "prompt total" {
					inputField, wantInput = `"input_tokens":100,"prompt_tokens":100`, 50
				}
				body := `{"id":"msg_total","type":"message","model":"claude-sonnet-4","role":"assistant","content":[],"usage":{` + inputField + `,"output_tokens":2,"cache_creation":{"ephemeral_5m_input_tokens":20,"ephemeral_1h_input_tokens":30}}}`
				response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
				recorder := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(recorder)
				ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				service := &GatewayService{cfg: &config.Config{}, rateLimitService: &RateLimitService{}}
				account := &Account{ID: 701}
				var usage *ClaudeUsage
				var err error
				switch route {
				case "messages":
					usage, err = service.handleNonStreamingResponse(context.Background(), response, ctx, account, "claude-sonnet-4", "claude-sonnet-4")
				case "passthrough":
					usage, err = service.handleNonStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), response, ctx, account)
				case "native messages":
					var result *OpenAIForwardResult
					result, err = (&OpenAIGatewayService{cfg: &config.Config{}}).handleNativeAnthropicBufferedResponse(context.Background(), response, ctx, account, "claude-sonnet-4", "claude-sonnet-4", "claude-sonnet-4", nil, time.Now())
					require.NoError(t, err)
					require.Equal(t, wantInput+50, result.Usage.InputTokens)
					result.Model, result.BillingModel, result.UpstreamModel = "k3", "kimi-k3", "k3"
					assertNativeCacheRecordUsage(t, result, nativeAnthropicTestAccount(), wantInput, 0.16)
					usage = parseClaudeUsageFromResponseBody([]byte(body))
				}
				require.NoError(t, err)
				require.Equal(t, wantInput, usage.InputTokens)
				assertAnthropicCacheCreationResultCosts(t, &ForwardResult{Model: "claude-sonnet-4", Usage: *usage}, 0.16)
				require.JSONEq(t, body, recorder.Body.String())
			})
		}
	}
}
