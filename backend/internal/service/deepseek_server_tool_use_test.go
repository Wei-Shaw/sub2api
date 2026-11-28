//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Exercise the usage parser and the billing path for each wire protocol.
func TestDeepseekConversationServerToolsBillTokensOnly(t *testing.T) {
	const model = "deepseek-flash"
	const tools = `"server_tool_use":{"tool_search":2,"tool_fetch":3,"tool_image_search":4,"web_search_requests":5,"web_fetch_requests":6}`
	const usage = `{"input_tokens":100,"output_tokens":50,"output_tokens_details":{"safety_tokens":10},` + tools + `}`
	const ccUsage = `{"prompt_tokens":100,"completion_tokens":50,"completion_tokens_details":{"safety_tokens":10},` + tools + `}`
	group := &Group{ID: 100, Platform: PlatformDeepseek, SearchPricePer1k: testPtrFloat64(1000),
		ModelPricing: []ChannelModelPricing{{
			Models: []string{model}, BillingMode: BillingModeToken,
			InputPrice: testPtrFloat64(0.001), OutputPrice: testPtrFloat64(0.002),
		}},
	}
	key := &APIKey{Group: group, GroupID: &group.ID}
	billing := newTestBillingServiceForResolver()
	resolver := NewModelPricingResolver(nil, billing)
	gateway := &GatewayService{billingService: billing, resolver: resolver}
	openAI := &OpenAIGatewayService{billingService: billing, resolver: resolver}

	for _, name := range []string{"messages", "messages/stream", "responses", "responses/stream", "chat", "chat/stream"} {
		t.Run(name, func(t *testing.T) {
			var cost *CostBreakdown
			switch name {
			case "messages", "messages/stream":
				var parsed ClaudeUsage
				if name == "messages" {
					parsed = *parseClaudeUsageFromResponseBody([]byte(`{"usage":` + usage + `}`))
				} else {
					parseSSEUsagePassthrough(`{"type":"message_start","message":{"usage":{"input_tokens":100}}}`, &parsed)
					parseSSEUsagePassthrough(`{"type":"message_delta","usage":`+usage+`}`, &parsed)
				}
				result := &ForwardResult{Model: model, Usage: parsed}
				cost = gateway.calculateRecordUsageCost(t.Context(), result, key, model, 1, 1, time.Time{})
			default:
				var parsed OpenAIUsage
				switch name {
				case "responses", "chat":
					raw := usage
					if name == "chat" {
						raw = ccUsage
					}
					var ok bool
					parsed, ok = extractOpenAIUsageFromJSONBytes([]byte(`{"usage":` + raw + `}`))
					require.True(t, ok)
				case "responses/stream":
					openAI.parseSSEUsage(`{"type":"response.completed","response":{"usage":`+usage+`}}`, &parsed)
				case "chat/stream":
					chunk := extractCCStreamUsage(`{"choices":[],"usage":` + ccUsage + `}`)
					require.NotNil(t, chunk)
					parsed = *chunk
				}
				result := &OpenAIForwardResult{Model: model, Usage: parsed}
				var err error
				cost, err = openAI.calculateOpenAIRecordUsageCost(t.Context(), result, key, []string{model}, 1, 1, 1, 1,
					parsed.billingUsageTokens(), "", nil, false, time.Time{})
				require.NoError(t, err)
			}
			require.InDelta(t, 0.10, cost.InputCost, 1e-12)
			require.InDelta(t, 0.10, cost.OutputCost, 1e-12)
			require.InDelta(t, 0.20, cost.TotalCost, 1e-12)
			require.InDelta(t, 0.20, cost.ActualCost, 1e-12)
		})
	}
}
