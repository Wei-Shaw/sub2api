package service

import (
	"context"
	"encoding/json"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/sjson"
)

type claudeMaxResponseRewriteContext struct {
	Parsed *ParsedRequest
	Group  *Group
}

type claudeMaxResponseRewriteContextKeyType struct{}

var claudeMaxResponseRewriteContextKey = claudeMaxResponseRewriteContextKeyType{}

func withClaudeMaxResponseRewriteContext(ctx context.Context, c *gin.Context, parsed *ParsedRequest) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	value := claudeMaxResponseRewriteContext{
		Parsed: parsed,
		Group:  claudeMaxGroupFromGinContext(c),
	}
	return context.WithValue(ctx, claudeMaxResponseRewriteContextKey, value)
}

func claudeMaxResponseRewriteContextFromContext(ctx context.Context) claudeMaxResponseRewriteContext {
	if ctx == nil {
		return claudeMaxResponseRewriteContext{}
	}
	value, _ := ctx.Value(claudeMaxResponseRewriteContextKey).(claudeMaxResponseRewriteContext)
	return value
}

func claudeMaxGroupFromGinContext(c *gin.Context) *Group {
	if c == nil {
		return nil
	}
	raw, exists := c.Get("api_key")
	if !exists {
		return nil
	}
	apiKey, ok := raw.(*APIKey)
	if !ok || apiKey == nil {
		return nil
	}
	return apiKey.Group
}

func parsedRequestFromGinContext(c *gin.Context) *ParsedRequest {
	if c == nil {
		return nil
	}
	raw, exists := c.Get("parsed_request")
	if !exists {
		return nil
	}
	parsed, _ := raw.(*ParsedRequest)
	return parsed
}

func applyClaudeMaxSimulationToUsage(ctx context.Context, usage *ClaudeUsage, model string, accountID int64) claudeMaxCacheBillingOutcome {
	var out claudeMaxCacheBillingOutcome
	if usage == nil {
		return out
	}
	rewriteCtx := claudeMaxResponseRewriteContextFromContext(ctx)
	return applyClaudeMaxCacheBillingPolicyToUsage(usage, rewriteCtx.Parsed, rewriteCtx.Group, model, accountID)
}

func applyClaudeMaxSimulationToUsageJSONMap(ctx context.Context, usageObj map[string]any, model string, accountID int64) claudeMaxCacheBillingOutcome {
	var out claudeMaxCacheBillingOutcome
	if usageObj == nil {
		return out
	}
	usage := claudeUsageFromJSONMap(usageObj)
	out = applyClaudeMaxSimulationToUsage(ctx, &usage, model, accountID)
	if out.Simulated {
		rewriteClaudeUsageJSONMap(usageObj, usage)
	}
	return out
}

func rewriteClaudeUsageJSONBytes(body []byte, usage ClaudeUsage) []byte {
	updated := body
	var err error

	updated, err = sjson.SetBytes(updated, "usage.input_tokens", usage.InputTokens)
	if err != nil {
		return body
	}
	updated, err = sjson.SetBytes(updated, "usage.cache_creation_input_tokens", usage.CacheCreationInputTokens)
	if err != nil {
		return body
	}
	updated, err = sjson.SetBytes(updated, "usage.cache_creation.ephemeral_5m_input_tokens", usage.CacheCreation5mTokens)
	if err != nil {
		return body
	}
	updated, err = sjson.SetBytes(updated, "usage.cache_creation.ephemeral_1h_input_tokens", usage.CacheCreation1hTokens)
	if err != nil {
		return body
	}
	return updated
}

func claudeUsageFromJSONMap(usageObj map[string]any) ClaudeUsage {
	var usage ClaudeUsage
	if usageObj == nil {
		return usage
	}

	usage.InputTokens = usageIntFromAny(usageObj["input_tokens"])
	usage.OutputTokens = usageIntFromAny(usageObj["output_tokens"])
	usage.CacheCreationInputTokens = usageIntFromAny(usageObj["cache_creation_input_tokens"])
	usage.CacheReadInputTokens = usageIntFromAny(usageObj["cache_read_input_tokens"])

	if ccObj, ok := usageObj["cache_creation"].(map[string]any); ok {
		usage.CacheCreation5mTokens = usageIntFromAny(ccObj["ephemeral_5m_input_tokens"])
		usage.CacheCreation1hTokens = usageIntFromAny(ccObj["ephemeral_1h_input_tokens"])
	}
	return usage
}

func rewriteClaudeUsageJSONMap(usageObj map[string]any, usage ClaudeUsage) {
	if usageObj == nil {
		return
	}
	usageObj["input_tokens"] = usage.InputTokens
	usageObj["cache_creation_input_tokens"] = usage.CacheCreationInputTokens

	ccObj, _ := usageObj["cache_creation"].(map[string]any)
	if ccObj == nil {
		ccObj = make(map[string]any, 2)
		usageObj["cache_creation"] = ccObj
	}
	ccObj["ephemeral_5m_input_tokens"] = usage.CacheCreation5mTokens
	ccObj["ephemeral_1h_input_tokens"] = usage.CacheCreation1hTokens
}

func usageIntFromAny(v any) int {
	switch value := v.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		if n, err := value.Int64(); err == nil {
			return int(n)
		}
	}
	return 0
}

// setupClaudeMaxStreamingHook 为 Antigravity 流式路径设置 SSE usage 改写 hook。
func setupClaudeMaxStreamingHook(c *gin.Context, processor *antigravity.StreamingProcessor, originalModel string, accountID int64) {
	group := claudeMaxGroupFromGinContext(c)
	parsed := parsedRequestFromGinContext(c)
	if !shouldApplyClaudeMaxBillingRulesForUsage(group, originalModel, parsed) {
		return
	}
	processor.SetUsageMapHook(func(usageMap map[string]any) {
		svcUsage := claudeUsageFromJSONMap(usageMap)
		outcome := applyClaudeMaxCacheBillingPolicyToUsage(&svcUsage, parsed, group, originalModel, accountID)
		if outcome.Simulated {
			rewriteClaudeUsageJSONMap(usageMap, svcUsage)
		}
	})
}

// applyClaudeMaxNonStreamingRewrite 为 Antigravity 非流式路径改写响应体中的 usage。
func applyClaudeMaxNonStreamingRewrite(c *gin.Context, claudeResp []byte, agUsage *antigravity.ClaudeUsage, originalModel string, accountID int64) []byte {
	group := claudeMaxGroupFromGinContext(c)
	parsed := parsedRequestFromGinContext(c)
	if !shouldApplyClaudeMaxBillingRulesForUsage(group, originalModel, parsed) {
		return claudeResp
	}
	svcUsage := &ClaudeUsage{
		InputTokens:              agUsage.InputTokens,
		OutputTokens:             agUsage.OutputTokens,
		CacheCreationInputTokens: agUsage.CacheCreationInputTokens,
		CacheReadInputTokens:     agUsage.CacheReadInputTokens,
	}
	outcome := applyClaudeMaxCacheBillingPolicyToUsage(svcUsage, parsed, group, originalModel, accountID)
	if outcome.Simulated {
		return rewriteClaudeUsageJSONBytes(claudeResp, *svcUsage)
	}
	return claudeResp
}
