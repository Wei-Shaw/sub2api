package service

import (
	"bytes"
	"context"
	"fmt"
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

func relayTierResponseBody(observed string) string {
	tier := ""
	if observed != "" {
		tier = fmt.Sprintf(`,"service_tier":%q`, observed)
	}
	return `{"id":"resp_tier","object":"response","model":"gpt-5.5","status":"completed","output":[],"usage":{"input_tokens":100,"output_tokens":10}` + tier + `}`
}

func relayTierLifecycleSSE(observed string) string {
	body := relayTierResponseBody(observed)
	var wire strings.Builder
	for _, event := range []string{"response.created", "response.in_progress"} {
		fmt.Fprintf(&wire, "data: {\"type\":%q,\"response\":%s}\n\n", event, body)
	}
	_, _ = wire.WriteString("data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n")
	fmt.Fprintf(&wire, "data: {\"type\":\"response.completed\",\"response\":%s}\n\ndata: [DONE]\n\n", body)
	return wire.String()
}

func relayTierSSEData(t *testing.T, wire string) [][]byte {
	t.Helper()
	var payloads [][]byte
	for _, line := range strings.Split(wire, "\n") {
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			continue
		}
		require.True(t, gjson.Valid(data), data)
		payloads = append(payloads, []byte(data))
	}
	return payloads
}

func TestOpenAIServiceTierResponseBoundariesKeepRawBilling(t *testing.T) {
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
		for _, requested := range []string{"priority", "fast", ""} {
			for _, observed := range []string{"default", ""} {
				for _, passthrough := range []bool{false, true} {
					for _, boundary := range []string{"json", "sse_to_json", "sse"} {
						t.Run(fmt.Sprintf("%s/%s/%s/passthrough=%t/%s", accountType, requested, observed, passthrough, boundary), func(t *testing.T) {
							body, contentType := relayTierResponseBody(observed), "application/json"
							if boundary != "json" {
								body, contentType = relayTierLifecycleSSE(observed), "text/event-stream"
							}
							resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body))}
							var account *Account
							var result *OpenAIForwardResult
							var rec *httptest.ResponseRecorder
							if passthrough && boundary != "sse" {
								// OAuth passthrough forces stream=true on the wire; exercise
								// its JSON/SSE-to-JSON response boundaries directly as well.
								account = &Account{Platform: PlatformOpenAI, Type: accountType}
								rec = httptest.NewRecorder()
								c, _ := gin.CreateTestContext(rec)
								c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
								var tier *string
								if requested != "" {
									value := normalizedOpenAIServiceTierValue(requested)
									tier = &value
								}
								setOpenAIResponseServiceTier(c, tier)
								svc := &OpenAIGatewayService{cfg: &config.Config{}}
								partial, err := svc.handleNonStreamingResponsePassthrough(context.Background(), resp, c, account, "gpt-5.5", "gpt-5.5")
								require.NoError(t, err)
								result = &OpenAIForwardResult{Usage: *partial.usage, ServiceTier: tier, UpstreamResponseServiceTier: observedUpstreamResponseServiceTier(c)}
							} else {
								account, result, rec = forwardServiceTierRelayFixture(t, accountType, passthrough, boundary == "sse", requested, resp)
							}
							require.Equal(t, observed, result.UpstreamResponseServiceTier)
							require.Equal(t, 100, result.Usage.InputTokens)
							require.Equal(t, 10, result.Usage.OutputTokens)
							want := observed
							if accountType == AccountTypeOAuth && requested != "" {
								want = "priority"
								resolution := ApplyOpenAIServiceTierBillingResolution(account, result)
								require.Equal(t, "priority", resolution.Billing)
								require.Equal(t, observed, result.UpstreamResponseServiceTier)
								// Apply the resolved tier to a known 2x model/card. GPT-5.5
								// itself has a model-specific 2.5x multiplier; do not change it.
								billing := NewBillingService(&config.Config{}, &PricingService{pricingData: map[string]*LiteLLMModelPricing{
									"gpt-5.4": {InputCostPerToken: 1e-6, OutputCostPerToken: 2e-6},
								}})
								tokens := UsageTokens{InputTokens: result.Usage.InputTokens, OutputTokens: result.Usage.OutputTokens}
								base, err := billing.CalculateCost("gpt-5.4", tokens, 1)
								require.NoError(t, err)
								cost, err := billing.CalculateCostWithServiceTier("gpt-5.4", tokens, 1, resolution.Billing)
								require.NoError(t, err)
								require.InDelta(t, base.TotalCost*2, cost.TotalCost, 1e-12)
							}
							if boundary == "sse" {
								seen := 0
								for _, payload := range relayTierSSEData(t, rec.Body.String()) {
									if !gjson.GetBytes(payload, "response").IsObject() {
										continue
									}
									require.Equal(t, want, gjson.GetBytes(payload, "response.service_tier").String())
									seen++
								}
								require.Equal(t, 3, seen, "created, in_progress and completed all use the client-facing tier")
							} else {
								require.Equal(t, want, gjson.GetBytes(rec.Body.Bytes(), "service_tier").String())
							}
						})
					}
				}
			}
		}
	}
}

func TestOpenAIServiceTierChatEveryChunk(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, requested := range []string{"priority", "fast", ""} {
			for _, observed := range []string{"default", ""} {
				t.Run(fmt.Sprintf("stream=%t/%s/%s", stream, requested, observed), func(t *testing.T) {
					body := []byte(fmt.Sprintf(`{"model":"gpt-5.5","stream":%t,"messages":[{"role":"user","content":"hi"}]}`, stream))
					if requested != "" {
						body = append(body[:len(body)-1], []byte(fmt.Sprintf(`,"service_tier":%q}`, requested))...)
					}
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
					upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(relayTierLifecycleSSE(observed)))}}
					svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream, cache: &stubGatewayCache{}}
					account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1, Credentials: map[string]any{"access_token": "fixture-token", "chatgpt_account_id": "fixture-account"}}
					result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
					require.NoError(t, err)
					require.Equal(t, observed, result.UpstreamResponseServiceTier)
					want := observed
					if requested != "" {
						want = "priority"
					}
					if !stream {
						require.Equal(t, want, gjson.GetBytes(rec.Body.Bytes(), "service_tier").String())
						return
					}
					chunks := relayTierSSEData(t, rec.Body.String())
					require.GreaterOrEqual(t, len(chunks), 4, "role, text, finish and usage chunks")
					for _, chunk := range chunks {
						require.Equal(t, want, gjson.GetBytes(chunk, "service_tier").String(), string(chunk))
					}
				})
			}
		}
	}
}
