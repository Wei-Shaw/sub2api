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

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGatewayAnthropicCompatReasoningPricingUsesForwardedEffort(t *testing.T) {
	gin.SetMode(gin.TestMode)
	endpoints := []struct {
		name string
		path string
		call func(*GatewayService, context.Context, *gin.Context, *Account, []byte) (*ForwardResult, error)
	}{
		{"chat_completions", "/v1/chat/completions", func(s *GatewayService, ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
			return s.ForwardAsChatCompletions(ctx, c, account, body, nil)
		}},
		{"responses", "/v1/responses", func(s *GatewayService, ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
			return s.ForwardAsResponses(ctx, c, account, body, nil)
		}},
	}
	for _, endpoint := range endpoints {
		for _, stream := range []bool{false, true} {
			for _, tc := range []struct {
				name       string
				model      string
				effort     string
				thinking   string
				wantEffort string
				multiplier float64
			}{
				{"xhigh_converts_to_max", "claude-fable-5-1", "xhigh", "", "max", 3},
				{"native_max", "claude-fable-5-1", "max", "", "max", 3},
				{"missing_effort", "kimi-k3", "", "", "", 1},
				{"disabled_thinking", "kimi-k3", "", "disabled", "", 1},
				{"unforwarded_thinking", "kimi-k3", "", "enabled", "", 1},
			} {
				mode := "buffered"
				if stream {
					mode = "streaming"
				}
				t.Run(endpoint.name+"/"+mode+"/"+tc.name, func(t *testing.T) {
					request := map[string]any{"model": tc.model, "stream": stream}
					if endpoint.name == "chat_completions" {
						request["messages"] = []map[string]string{{"role": "user", "content": "hello"}}
						if tc.effort != "" {
							request["reasoning_effort"] = tc.effort
						}
					} else {
						request["input"] = "hello"
						if tc.effort != "" {
							request["reasoning"] = map[string]string{"effort": tc.effort}
						}
					}
					if tc.thinking != "" {
						request["thinking"] = map[string]string{"type": tc.thinking}
					}
					body, err := json.Marshal(request)
					require.NoError(t, err)
					upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
						Body:       io.NopCloser(strings.NewReader(namespaceToolAnthropicStream())),
					}}}
					svc := &GatewayService{
						cfg: &config.Config{}, httpUpstream: upstream,
						tlsFPProfileService: &TLSFingerprintProfileService{},
					}
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, endpoint.path, strings.NewReader(string(body)))
					account := &Account{
						ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey,
						Credentials: map[string]any{"api_key": "test-key"},
					}
					result, err := endpoint.call(svc, context.Background(), c, account, body)
					require.NoError(t, err)
					require.Len(t, upstream.requestBodies, 1)
					require.Equal(t, tc.wantEffort, gjson.GetBytes(upstream.requestBodies[0], "output_config.effort").String())
					require.Equal(t, tc.wantEffort, optionalStringValue(result.ReasoningEffort))
					if tc.wantEffort == "" {
						require.Nil(t, result.ReasoningEffort)
						require.False(t, OpenAIBodyHasThinkingEnabled(upstream.requestBodies[0]))
					}

					billing := NewBillingService(&config.Config{}, nil)
					group := &Group{ID: 1, Platform: PlatformAnthropic, ModelPricing: []ChannelModelPricing{{
						Models: []string{tc.model}, BillingMode: BillingModeToken,
						InputPrice: testPtrFloat64(1e-6), OutputPrice: testPtrFloat64(2e-6),
						ReasoningEffortMultipliers: map[string]float64{"xhigh": 2, "max": 3, "high": 1.5},
					}}}
					cost, err := billing.CalculateTokenCostForRequest(TokenCostRequest{
						Ctx: context.Background(), Model: result.Model, Group: group,
						Tokens:          UsageTokens{InputTokens: result.Usage.InputTokens, OutputTokens: result.Usage.OutputTokens},
						ReasoningEffort: optionalStringValue(result.ReasoningEffort), RateMultiplier: 1,
						Resolver: NewModelPricingResolver(nil, billing),
					})
					require.NoError(t, err)
					require.InDelta(t, 20e-6*tc.multiplier, cost.TotalCost, 1e-12)
				})
			}
		}
	}
}
