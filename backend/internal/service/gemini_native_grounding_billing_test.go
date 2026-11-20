//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestGeminiNativeGroundingBilling(t *testing.T) {
	const model = "gemini-3-flash-preview"
	const tokens = 0.01068850
	const groupRate = 2.20
	const pricePerThousand = 14.0
	const queryCost = 3 * pricePerThousand / 1000

	body := []byte(`{"contents":[{"role":"user","parts":[{"text":"hello"}]}],"tools":[{"googleSearch":{}}]}`)
	usage := `"usageMetadata":{"promptTokenCount":617,"candidatesTokenCount":3460}`
	grounded := `{"candidates":[{"content":{"parts":[{"text":"answer"}]},"finishReason":"STOP","groundingMetadata":{"webSearchQueries":["alpha","beta","alpha"]}},{"groundingMetadata":{"webSearchQueries":["  beta  ","","gamma"]}}],` + usage + `}`
	functionOnly := `{"candidates":[{"content":{"parts":[{"functionCall":{"name":"local_tool","args":{}}}]},"finishReason":"STOP"}],` + usage + `}`
	first := `{"candidates":[{"content":{"parts":[{"text":"answer"}]},"groundingMetadata":{"webSearchQueries":["alpha","beta"]}}]}`
	last := `{"candidates":[{"finishReason":"STOP","groundingMetadata":{"webSearchQueries":["  beta  ","","gamma"]}}],` + usage + `}`

	tests := []struct {
		name           string
		model          string
		response       string
		earlyFrame     string
		stream         bool
		bufferUpstream bool
		price          *float64
		wantSearch     float64
	}{
		{name: "json", model: model, response: grounded, price: testPtrFloat64(pricePerThousand), wantSearch: queryCost},
		{name: "sse", model: model, response: last, earlyFrame: first, stream: true, price: testPtrFloat64(pricePerThousand), wantSearch: queryCost},
		{name: "buffered_sse", model: model, response: last, earlyFrame: first, bufferUpstream: true, price: testPtrFloat64(pricePerThousand), wantSearch: queryCost},
		{name: "unset_price", model: model, response: grounded},
		{name: "zero_price", model: model, response: grounded, price: testPtrFloat64(0)},
		{name: "client_tool", model: model, response: functionOnly, price: testPtrFloat64(pricePerThousand)},
		{name: "older_model", model: "gemini-2.5-flash", response: grounded, price: testPtrFloat64(pricePerThousand)},
		{name: "older_response_model", model: model, response: `{"modelVersion":"gemini-2.5-flash",` + grounded[1:], price: testPtrFloat64(pricePerThousand)},
		{name: "alias_with_response_model", model: "gateway-flash", response: `{"modelVersion":"gemini-3-flash-preview",` + grounded[1:], price: testPtrFloat64(pricePerThousand), wantSearch: queryCost},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			account := geminiSignalTestAccount()
			action := "generateContent"
			if tc.stream {
				action = "streamGenerateContent"
			}

			response, contentType := tc.response, "application/json"
			if tc.stream || tc.bufferUpstream {
				early, terminal := tc.earlyFrame, tc.response
				if tc.bufferUpstream {
					account.Type = AccountTypeOAuth
					account.Credentials["project_id"] = "test-project"
					account.Credentials["access_token"] = "test-token"
					early = `{"response":` + early + `}`
					terminal = `{"response":` + terminal + `}`
				}
				contentType = "text/event-stream"
				response = "data: " + early + "\n\ndata: " + terminal + "\n\n"
			}
			upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{contentType}},
				Body:       io.NopCloser(strings.NewReader(response)),
			}}}
			service := &GeminiMessagesCompatService{
				httpUpstream: upstream, cfg: &config.Config{}, tokenProvider: &GeminiTokenProvider{},
			}

			groupID := int64(77)
			apiKey := &APIKey{ID: 10, GroupID: &groupID, Group: &Group{
				ID: groupID, Platform: PlatformGemini, Status: StatusActive, Hydrated: true,
				RateMultiplier: groupRate, SearchPricePer1k: tc.price,
				ModelPricing: []ChannelModelPricing{{
					Models: []string{tc.model}, BillingMode: BillingModeToken,
					InputPrice: testPtrFloat64(0.50 / 1_000_000), OutputPrice: testPtrFloat64(3.00 / 1_000_000),
				}},
			}}
			c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1beta/models/"+tc.model+":"+action, body)
			c.Set("api_key", apiKey)
			result, err := service.ForwardNative(context.Background(), c, account, tc.model, action, tc.stream, body)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Len(t, upstream.requestBodies, 1)
			require.Equal(t, 617, result.Usage.InputTokens)
			require.Equal(t, 3460, result.Usage.OutputTokens)

			logs := &openAIRecordUsageLogRepoStub{inserted: true}
			users := &openAIRecordUsageUserRepoStub{}
			gateway := newGatewayRecordUsageServiceForTest(logs, users, &openAIRecordUsageSubRepoStub{})
			gateway.resolver = NewModelPricingResolver(nil, gateway.billingService)
			require.NoError(t, gateway.RecordUsage(context.Background(), &RecordUsageInput{
				Result: result, APIKey: apiKey, User: &User{ID: 20}, Account: account,
			}))
			require.NotNil(t, logs.lastLog)
			require.InDelta(t, tokens+tc.wantSearch, logs.lastLog.TotalCost, 1e-12)
			require.InDelta(t, (tokens+tc.wantSearch)*groupRate, logs.lastLog.ActualCost, 1e-12)
			require.InDelta(t, logs.lastLog.ActualCost, users.lastAmount, 1e-12)
		})
	}
}
