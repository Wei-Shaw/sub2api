//go:build unit

package service

import (
	"context"
	"encoding/json"
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

func TestOpenAINativeAnthropicReasoningPricingUsesForwardedEffort(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"responses", "chat/completions"} {
		for _, stream := range []bool{false, true} {
			for _, tc := range []struct {
				effort     string
				wantEffort string
				wantCost   float64
			}{
				{"xhigh", "max", 3},
				{"high", "high", 2},
				{"", "", 1},
			} {
				t.Run(fmt.Sprintf("%s/stream=%t/effort=%s", endpoint, stream, tc.effort), func(t *testing.T) {
					request := map[string]any{"model": "k3", "stream": stream}
					if endpoint == "responses" {
						request["input"] = "hello"
						if tc.effort != "" {
							request["reasoning"] = map[string]string{"effort": tc.effort}
						}
					} else {
						request["messages"] = []map[string]string{{"role": "user", "content": "hello"}}
						if tc.effort != "" {
							request["reasoning_effort"] = tc.effort
						}
					}
					body, err := json.Marshal(request)
					require.NoError(t, err)
					resp := nativeAnthropicStreamResponse()
					responseBody, err := io.ReadAll(resp.Body)
					require.NoError(t, err)
					resp.Body = io.NopCloser(strings.NewReader(strings.ReplaceAll(string(responseBody), `"input_tokens":93`, `"input_tokens":1000000`)))
					upstream := &httpUpstreamRecorder{resp: resp}
					svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
					c := adaptiveProtocolTestContext("/v1/"+endpoint, body)
					account := nativeAnthropicTestAccount()
					account.Credentials["base_url"] = "http://anthropic.example"
					var result *OpenAIForwardResult
					if endpoint == "responses" {
						result, err = svc.Forward(context.Background(), c, account, body)
					} else {
						result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
					}
					require.NoError(t, err)
					require.Equal(t, "/v1/messages", upstream.lastReq.URL.Path)
					require.Equal(t, tc.wantEffort, gjson.GetBytes(upstream.lastBody, "output_config.effort").String())
					require.Equal(t, tc.wantEffort, optionalStringValue(result.ReasoningEffort))
					assertConvertedReasoningPrice(t, result, tc.wantCost)
				})
			}
		}
	}
}

func TestCNNativeAnthropicFinalReasoningCeiling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"responses", "chat/completions"} {
		for _, stream := range []bool{false, true} {
			for _, test := range []struct {
				name       string
				effort     string
				ceiling    string
				overLimit  string
				mappings   []ReasoningEffortMapping
				wantEffort string
				wantDenied bool
			}{
				{name: "conversion capped", effort: "xhigh", ceiling: "xhigh", wantEffort: "xhigh"},
				{name: "conversion denied", effort: "xhigh", ceiling: "xhigh", overLimit: "deny", wantDenied: true},
				{name: "high ceiling", effort: "high", ceiling: "high", wantEffort: "high"},
				{name: "max allowed", effort: "xhigh", ceiling: "max", wantEffort: "max"},
				{name: "no ceiling", effort: "xhigh", wantEffort: "max"},
				{name: "omitted effort", ceiling: "low"},
				{name: "one hop", effort: "low", ceiling: "max", mappings: []ReasoningEffortMapping{{From: "low", To: "high"}, {From: "high", To: "deny"}}, wantEffort: "high"},
			} {
				t.Run(fmt.Sprintf("%s/stream=%t/%s", endpoint, stream, test.name), func(t *testing.T) {
					request := map[string]any{"model": "k3", "stream": stream}
					if endpoint == "responses" {
						request["input"] = "hello"
						request["reasoning"] = map[string]string{"effort": test.effort}
					} else {
						request["messages"] = []map[string]string{{"role": "user", "content": "hello"}}
						request["reasoning_effort"] = test.effort
					}
					body, err := json.Marshal(request)
					require.NoError(t, err)
					body, _, err = ApplyReasoningEffortPolicy(body, test.ceiling, test.mappings, test.overLimit)
					require.NoError(t, err)
					ctx := WithOpenAIReasoningEffortPolicy(context.Background(), test.ceiling, []ReasoningEffortMapping{{From: "high", To: "deny"}, {From: "max", To: "low"}}, test.overLimit)
					for retry := 0; retry < 2; retry++ {
						upstream := &httpUpstreamRecorder{resp: nativeAnthropicStreamResponse()}
						svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
						recorder := httptest.NewRecorder()
						c, _ := gin.CreateTestContext(recorder)
						c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+endpoint, strings.NewReader(string(body))).WithContext(ctx)
						c.Request.Header.Set("Content-Type", "application/json")
						account := nativeAnthropicTestAccount()
						account.Platform = PlatformKimi
						var result *OpenAIForwardResult
						if endpoint == "responses" {
							result, err = svc.Forward(ctx, c, account, body)
						} else {
							result, err = svc.ForwardAsChatCompletions(ctx, c, account, body, "", "")
						}
						if test.wantDenied {
							require.True(t, IsReasoningEffortPolicyDenied(err), "error: %v", err)
							require.Nil(t, result)
							require.Empty(t, upstream.requests)
							require.Equal(t, http.StatusForbidden, c.Writer.Status())
							errorField := "error.type"
							if endpoint == "responses" {
								errorField = "error.code"
							}
							require.Equal(t, "permission_error", gjson.GetBytes(recorder.Body.Bytes(), errorField).String())
							continue
						}
						require.NoError(t, err)
						require.Equal(t, "/anthropic/v1/messages", upstream.lastReq.URL.Path)
						require.Equal(t, test.wantEffort, gjson.GetBytes(upstream.lastBody, "output_config.effort").String())
						require.Equal(t, test.wantEffort, optionalStringValue(result.ReasoningEffort))
					}
				})
			}
		}
	}
}

func TestOpenAIMessagesGLMReasoningPricingUsesForwardedEffort(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		for _, tc := range []struct {
			effort     string
			maxPolicy  string
			wantEffort string
			wantCost   float64
		}{
			{"medium", "", "high", 2},
			{"xhigh", "", "max", 3},
			{"xhigh", "medium", "medium", 1.5},
			{"low", "medium", "medium", 1.5},
		} {
			t.Run(fmt.Sprintf("stream=%t/effort=%s/policy=%s", stream, tc.effort, tc.maxPolicy), func(t *testing.T) {
				for _, overLimit := range []string{"", ReasoningEffortOverLimitDowngrade, ReasoningEffortOverLimitDeny} {
					t.Run("overLimit="+overLimit, func(t *testing.T) {
						body := []byte(fmt.Sprintf(`{"model":"glm-5.2","stream":%t,"max_tokens":32,"output_config":{"effort":%q},"messages":[{"role":"user","content":"hello"}]}`, stream, tc.effort))
						responseBody := `{"id":"chatcmpl_glm","object":"chat.completion","model":"glm-5.2","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1000000,"completion_tokens":2,"total_tokens":1000002}}`
						contentType := "application/json"
						if stream {
							contentType = "text/event-stream"
							responseBody = "data: " + `{"id":"chatcmpl_glm","object":"chat.completion.chunk","model":"glm-5.2","choices":[{"index":0,"delta":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1000000,"completion_tokens":2,"total_tokens":1000002}}` + "\n\ndata: [DONE]\n\n"
						}
						upstream := &httpUpstreamRecorder{resp: &http.Response{
							StatusCode: http.StatusOK,
							Header:     http.Header{"Content-Type": []string{contentType}},
							Body:       io.NopCloser(strings.NewReader(responseBody)),
						}}
						svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
						ctx := context.Background()
						if tc.maxPolicy != "" {
							ctx = WithOpenAIReasoningEffortPolicyForMessages(ctx, body, tc.maxPolicy, nil, overLimit)
						}
						requestContext := adaptiveProtocolTestContext("/v1/messages", body)
						account := forceChatMessagesFallbackAccount()
						require.Equal(t, PlatformOpenAI, account.Platform)
						result, err := svc.ForwardAsAnthropic(ctx, requestContext, account, body, "", "")
						if tc.maxPolicy != "" && overLimit == ReasoningEffortOverLimitDeny {
							require.True(t, IsReasoningEffortPolicyDenied(err), "expected policy denial, got %v", err)
							require.Equal(t, http.StatusForbidden, requestContext.Writer.Status())
							require.Empty(t, upstream.requests)
							require.Nil(t, result)
							return
						}
						require.NoError(t, err)
						require.Equal(t, http.StatusOK, requestContext.Writer.Status())
						require.Len(t, upstream.requests, 1)
						require.Equal(t, "/v1/chat/completions", upstream.lastReq.URL.Path)
						require.Equal(t, tc.wantEffort, gjson.GetBytes(upstream.lastBody, "reasoning_effort").String())
						require.Equal(t, tc.wantEffort, optionalStringValue(result.ReasoningEffort))
						assertConvertedReasoningPrice(t, result, tc.wantCost)
					})
				}
			})
		}
	}
}

func assertConvertedReasoningPrice(t *testing.T, result *OpenAIForwardResult, wantCost float64) {
	t.Helper()
	require.Equal(t, 1_000_000, result.Usage.InputTokens)
	billing := NewBillingService(rawChatCompletionsTestConfig(), nil)
	group := &Group{ID: 1, Platform: PlatformOpenAI, ModelPricing: []ChannelModelPricing{{
		Models: []string{result.BillingModel}, BillingMode: BillingModeToken,
		InputPrice: testPtrFloat64(1e-6), OutputPrice: testPtrFloat64(0),
		ReasoningEffortMultipliers: map[string]float64{"medium": 1.5, "high": 2, "xhigh": 2, "max": 3},
	}}}
	cost, err := billing.CalculateTokenCostForRequest(TokenCostRequest{
		Ctx: context.Background(), Model: result.BillingModel, Group: group,
		Tokens:          UsageTokens{InputTokens: result.Usage.InputTokens, OutputTokens: result.Usage.OutputTokens},
		ReasoningEffort: optionalStringValue(result.ReasoningEffort), RateMultiplier: 1,
		Resolver: NewModelPricingResolver(nil, billing),
	})
	require.NoError(t, err)
	require.InDelta(t, wantCost, cost.TotalCost, 1e-12)
	require.InDelta(t, wantCost, cost.ActualCost, 1e-12)
}
