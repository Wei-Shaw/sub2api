package service

import (
	"context"
	"time"

	"github.com/tidwall/gjson"
)

// ClaudeUsageIteration is one entry of Anthropic's usage.iterations[]: the separately billed passes of a request
// (advisor sub-inferences, compaction). The top-level usage fields exclude the advisor_message and compaction entries.
type ClaudeUsageIteration struct {
	Type                     string
	Model                    string
	InputTokens              int
	OutputTokens             int
	CacheCreationInputTokens int
	CacheReadInputTokens     int
}

type serverToolUsagePatch struct {
	webSearchRequests    int
	hasWebSearchRequests bool
	iterations           []ClaudeUsageIteration
	hasIterations        bool
}

func serverToolUsagePatchFromMap(usageObj map[string]any) *serverToolUsagePatch {
	p := &serverToolUsagePatch{}
	if stu, ok := usageObj["server_tool_use"].(map[string]any); ok {
		if v, ok := parseSSEUsageInt(stu["web_search_requests"]); ok {
			p.webSearchRequests, p.hasWebSearchRequests = v, true
		}
	}
	if arr, ok := usageObj["iterations"].([]any); ok {
		p.hasIterations = true
		for _, e := range arr {
			m, _ := e.(map[string]any)
			if m == nil {
				continue
			}
			it := ClaudeUsageIteration{}
			it.Type, _ = m["type"].(string)
			it.Model, _ = m["model"].(string)
			it.InputTokens, _ = parseSSEUsageInt(m["input_tokens"])
			it.OutputTokens, _ = parseSSEUsageInt(m["output_tokens"])
			it.CacheCreationInputTokens, _ = parseSSEUsageInt(m["cache_creation_input_tokens"])
			it.CacheReadInputTokens, _ = parseSSEUsageInt(m["cache_read_input_tokens"])
			p.iterations = append(p.iterations, it)
		}
	}
	if !p.hasWebSearchRequests && !p.hasIterations {
		return nil
	}
	return p
}

func applyServerToolUsagePatch(usage *ClaudeUsage, p *serverToolUsagePatch) {
	if usage == nil || p == nil {
		return
	}
	if p.hasWebSearchRequests {
		usage.WebSearchRequests = p.webSearchRequests
	}
	if p.hasIterations {
		usage.Iterations = p.iterations
	}
}

func applyServerToolUsageFromJSON(usage *ClaudeUsage, node gjson.Result) {
	if usage == nil || !node.Exists() {
		return
	}
	if v := node.Get("server_tool_use.web_search_requests"); v.Exists() {
		usage.WebSearchRequests = int(v.Int())
	}
	if arr := node.Get("iterations"); arr.IsArray() {
		usage.Iterations = usage.Iterations[:0]
		arr.ForEach(func(_, e gjson.Result) bool {
			usage.Iterations = append(usage.Iterations, ClaudeUsageIteration{
				Type:                     e.Get("type").String(),
				Model:                    e.Get("model").String(),
				InputTokens:              int(e.Get("input_tokens").Int()),
				OutputTokens:             int(e.Get("output_tokens").Int()),
				CacheCreationInputTokens: int(e.Get("cache_creation_input_tokens").Int()),
				CacheReadInputTokens:     int(e.Get("cache_read_input_tokens").Int()),
			})
			return true
		})
	}
}

// iterationSurcharge prices the provider-billed passes that the top-level usage excludes: advisor_message entries at the
// advisor model's rate and compaction entries at the executor's. Entries of type message are already in the top-level
// counts. An advisor model with no resolvable price is charged at the executor's price (a lower bound), never at zero.
func (s *GatewayService) iterationSurcharge(ctx context.Context, result *ForwardResult, apiKey *APIKey, billingModel string, multiplier float64, pricingAt time.Time) (total, actual float64) {
	for _, it := range result.Usage.Iterations {
		if it.Type != "advisor_message" && it.Type != "compaction" {
			continue
		}
		model := billingModel
		if it.Type == "advisor_message" && it.Model != "" && s.hasResolvableTokenPricing(ctx, it.Model, apiKey) {
			model = it.Model
		}
		sub := &ForwardResult{Model: model, ServiceTier: result.ServiceTier, Usage: ClaudeUsage{
			InputTokens: it.InputTokens, OutputTokens: it.OutputTokens,
			CacheCreationInputTokens: it.CacheCreationInputTokens, CacheReadInputTokens: it.CacheReadInputTokens,
		}}
		if c := s.calculateTokenCost(ctx, sub, apiKey, model, multiplier, pricingAt); c != nil {
			total += c.TotalCost
			actual += c.ActualCost
		}
	}
	return total, actual
}
