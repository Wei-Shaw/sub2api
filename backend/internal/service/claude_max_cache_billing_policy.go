package service

import (
	"encoding/json"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/tidwall/gjson"
)

type claudeMaxCacheBillingOutcome struct {
	Simulated bool
}

func applyClaudeMaxCacheBillingPolicyToUsage(usage *ClaudeUsage, parsed *ParsedRequest, group *Group, model string, accountID int64) claudeMaxCacheBillingOutcome {
	var out claudeMaxCacheBillingOutcome
	if usage == nil || !shouldApplyClaudeMaxBillingRulesForUsage(group, model, parsed) {
		return out
	}

	resolvedModel := strings.TrimSpace(model)
	if resolvedModel == "" && parsed != nil {
		resolvedModel = strings.TrimSpace(parsed.Model)
	}

	if hasCacheCreationTokens(*usage) {
		// Upstream already returned cache creation usage; keep original usage.
		return out
	}

	if !shouldSimulateClaudeMaxUsageForUsage(*usage, parsed) {
		return out
	}
	beforeInputTokens := usage.InputTokens
	out.Simulated = safelyProjectUsageToClaudeMax1H(usage, parsed)
	if out.Simulated {
		logger.LegacyPrintf("service.gateway", "simulate_claude_max_usage: model=%s account=%d input_tokens:%d->%d cache_creation_1h=%d",
			resolvedModel,
			accountID,
			beforeInputTokens,
			usage.InputTokens,
			usage.CacheCreation1hTokens,
		)
	}
	return out
}

func isClaudeFamilyModel(model string) bool {
	normalized := strings.ToLower(strings.TrimSpace(claude.NormalizeModelID(model)))
	if normalized == "" {
		return false
	}
	return strings.Contains(normalized, "claude-")
}

func shouldApplyClaudeMaxBillingRules(input *RecordUsageInput) bool {
	if input == nil || input.Result == nil || input.APIKey == nil || input.APIKey.Group == nil {
		return false
	}
	return shouldApplyClaudeMaxBillingRulesForUsage(input.APIKey.Group, input.Result.Model, input.ParsedRequest)
}

func shouldApplyClaudeMaxBillingRulesForUsage(group *Group, model string, parsed *ParsedRequest) bool {
	if group == nil {
		return false
	}
	if !group.SimulateClaudeMaxEnabled || group.Platform != PlatformAnthropic {
		return false
	}

	resolvedModel := model
	if resolvedModel == "" && parsed != nil {
		resolvedModel = parsed.Model
	}
	if !isClaudeFamilyModel(resolvedModel) {
		return false
	}
	return true
}

func hasCacheCreationTokens(usage ClaudeUsage) bool {
	return usage.CacheCreationInputTokens > 0 || usage.CacheCreation5mTokens > 0 || usage.CacheCreation1hTokens > 0
}

func shouldSimulateClaudeMaxUsage(input *RecordUsageInput) bool {
	if input == nil || input.Result == nil {
		return false
	}
	if !shouldApplyClaudeMaxBillingRules(input) {
		return false
	}
	return shouldSimulateClaudeMaxUsageForUsage(input.Result.Usage, input.ParsedRequest)
}

func shouldSimulateClaudeMaxUsageForUsage(usage ClaudeUsage, parsed *ParsedRequest) bool {
	if usage.InputTokens <= 0 {
		return false
	}
	if hasCacheCreationTokens(usage) {
		return false
	}
	if !hasClaudeCacheSignals(parsed) {
		return false
	}
	return true
}

func safelyProjectUsageToClaudeMax1H(usage *ClaudeUsage, parsed *ParsedRequest) (changed bool) {
	defer func() {
		if r := recover(); r != nil {
			logger.LegacyPrintf("service.gateway", "simulate_claude_max_usage skipped: panic=%v", r)
			changed = false
		}
	}()
	return projectUsageToClaudeMax1H(usage, parsed)
}

func projectUsageToClaudeMax1H(usage *ClaudeUsage, parsed *ParsedRequest) bool {
	if usage == nil {
		return false
	}
	totalWindowTokens := usage.InputTokens + usage.CacheCreation5mTokens + usage.CacheCreation1hTokens
	if totalWindowTokens <= 1 {
		return false
	}

	simulatedInputTokens := computeClaudeMaxProjectedInputTokens(totalWindowTokens, parsed)
	if simulatedInputTokens <= 0 {
		simulatedInputTokens = 1
	}
	if simulatedInputTokens >= totalWindowTokens {
		simulatedInputTokens = totalWindowTokens - 1
	}

	cacheCreation1hTokens := totalWindowTokens - simulatedInputTokens
	if usage.InputTokens == simulatedInputTokens &&
		usage.CacheCreation5mTokens == 0 &&
		usage.CacheCreation1hTokens == cacheCreation1hTokens &&
		usage.CacheCreationInputTokens == cacheCreation1hTokens {
		return false
	}

	usage.InputTokens = simulatedInputTokens
	usage.CacheCreation5mTokens = 0
	usage.CacheCreation1hTokens = cacheCreation1hTokens
	usage.CacheCreationInputTokens = cacheCreation1hTokens
	return true
}

type claudeCacheProjection struct {
	HasBreakpoint        bool
	BreakpointCount      int
	TotalEstimatedTokens int
	TailEstimatedTokens  int
}

func computeClaudeMaxProjectedInputTokens(totalWindowTokens int, parsed *ParsedRequest) int {
	if totalWindowTokens <= 1 {
		return totalWindowTokens
	}

	projection := analyzeClaudeCacheProjection(parsed)
	if !projection.HasBreakpoint || projection.TotalEstimatedTokens <= 0 || projection.TailEstimatedTokens <= 0 {
		return totalWindowTokens
	}

	totalEstimate := int64(projection.TotalEstimatedTokens)
	tailEstimate := int64(projection.TailEstimatedTokens)
	if tailEstimate > totalEstimate {
		tailEstimate = totalEstimate
	}

	scaled := (int64(totalWindowTokens)*tailEstimate + totalEstimate/2) / totalEstimate
	if scaled <= 0 {
		scaled = 1
	}
	if scaled >= int64(totalWindowTokens) {
		scaled = int64(totalWindowTokens - 1)
	}
	return int(scaled)
}

func hasClaudeCacheSignals(parsed *ParsedRequest) bool {
	if parsed == nil {
		return false
	}
	if hasTopLevelEphemeralCacheControl(parsed) {
		return true
	}
	return countExplicitCacheBreakpoints(parsed) > 0
}

func hasTopLevelEphemeralCacheControl(parsed *ParsedRequest) bool {
	if parsed == nil || len(parsed.Body) == 0 {
		return false
	}
	cacheType := strings.TrimSpace(gjson.GetBytes(parsed.Body, "cache_control.type").String())
	return strings.EqualFold(cacheType, "ephemeral")
}

func analyzeClaudeCacheProjection(parsed *ParsedRequest) claudeCacheProjection {
	var projection claudeCacheProjection
	if parsed == nil {
		return projection
	}

	total := 0
	lastBreakpointAt := -1

	switch system := parsed.System.(type) {
	case string:
		total += claudeMaxMessageOverheadTokens + estimateClaudeTextTokens(system)
	case []any:
		for _, raw := range system {
			block, ok := raw.(map[string]any)
			if !ok {
				total += claudeMaxUnknownContentTokens
				continue
			}
			total += estimateClaudeBlockTokens(block)
			if hasEphemeralCacheControl(block) {
				lastBreakpointAt = total
				projection.BreakpointCount++
				projection.HasBreakpoint = true
			}
		}
	}

	for _, rawMsg := range parsed.Messages {
		total += claudeMaxMessageOverheadTokens
		msg, ok := rawMsg.(map[string]any)
		if !ok {
			total += claudeMaxUnknownContentTokens
			continue
		}
		content, exists := msg["content"]
		if !exists {
			continue
		}
		msgTokens, msgLastBreak, msgBreakCount := estimateClaudeContentTokens(content)
		total += msgTokens
		if msgBreakCount > 0 {
			lastBreakpointAt = total - msgTokens + msgLastBreak
			projection.BreakpointCount += msgBreakCount
			projection.HasBreakpoint = true
		}
	}

	if total <= 0 {
		total = 1
	}
	projection.TotalEstimatedTokens = total

	if projection.HasBreakpoint && lastBreakpointAt >= 0 {
		tail := total - lastBreakpointAt
		if tail <= 0 {
			tail = 1
		}
		projection.TailEstimatedTokens = tail
		return projection
	}

	if hasTopLevelEphemeralCacheControl(parsed) {
		tail := estimateLastUserMessageTokens(parsed)
		if tail <= 0 {
			tail = 1
		}
		projection.HasBreakpoint = true
		projection.BreakpointCount = 1
		projection.TailEstimatedTokens = tail
	}
	return projection
}

func countExplicitCacheBreakpoints(parsed *ParsedRequest) int {
	if parsed == nil {
		return 0
	}
	total := 0
	if system, ok := parsed.System.([]any); ok {
		for _, raw := range system {
			if block, ok := raw.(map[string]any); ok && hasEphemeralCacheControl(block) {
				total++
			}
		}
	}
	for _, rawMsg := range parsed.Messages {
		msg, ok := rawMsg.(map[string]any)
		if !ok {
			continue
		}
		content, ok := msg["content"].([]any)
		if !ok {
			continue
		}
		for _, raw := range content {
			if block, ok := raw.(map[string]any); ok && hasEphemeralCacheControl(block) {
				total++
			}
		}
	}
	return total
}

func hasEphemeralCacheControl(block map[string]any) bool {
	if block == nil {
		return false
	}
	raw, ok := block["cache_control"]
	if !ok || raw == nil {
		return false
	}
	switch cc := raw.(type) {
	case map[string]any:
		cacheType, _ := cc["type"].(string)
		return strings.EqualFold(strings.TrimSpace(cacheType), "ephemeral")
	case map[string]string:
		return strings.EqualFold(strings.TrimSpace(cc["type"]), "ephemeral")
	default:
		return false
	}
}

func estimateClaudeContentTokens(content any) (tokens int, lastBreakAt int, breakpointCount int) {
	switch value := content.(type) {
	case string:
		return estimateClaudeTextTokens(value), -1, 0
	case []any:
		total := 0
		lastBreak := -1
		breaks := 0
		for _, raw := range value {
			block, ok := raw.(map[string]any)
			if !ok {
				total += claudeMaxUnknownContentTokens
				continue
			}
			total += estimateClaudeBlockTokens(block)
			if hasEphemeralCacheControl(block) {
				lastBreak = total
				breaks++
			}
		}
		return total, lastBreak, breaks
	default:
		return estimateStructuredTokens(value), -1, 0
	}
}

func estimateClaudeBlockTokens(block map[string]any) int {
	if block == nil {
		return claudeMaxUnknownContentTokens
	}
	tokens := claudeMaxBlockOverheadTokens
	blockType, _ := block["type"].(string)
	switch blockType {
	case "text":
		if text, ok := block["text"].(string); ok {
			tokens += estimateClaudeTextTokens(text)
		}
	case "tool_result":
		if content, ok := block["content"]; ok {
			nested, _, _ := estimateClaudeContentTokens(content)
			tokens += nested
		}
	case "tool_use":
		if name, ok := block["name"].(string); ok {
			tokens += estimateClaudeTextTokens(name)
		}
		if input, ok := block["input"]; ok {
			tokens += estimateStructuredTokens(input)
		}
	default:
		if text, ok := block["text"].(string); ok {
			tokens += estimateClaudeTextTokens(text)
		} else if content, ok := block["content"]; ok {
			nested, _, _ := estimateClaudeContentTokens(content)
			tokens += nested
		}
	}
	if tokens <= claudeMaxBlockOverheadTokens {
		tokens += claudeMaxUnknownContentTokens
	}
	return tokens
}

func estimateLastUserMessageTokens(parsed *ParsedRequest) int {
	if parsed == nil || len(parsed.Messages) == 0 {
		return 0
	}
	for i := len(parsed.Messages) - 1; i >= 0; i-- {
		msg, ok := parsed.Messages[i].(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		if !strings.EqualFold(strings.TrimSpace(role), "user") {
			continue
		}
		tokens, _, _ := estimateClaudeContentTokens(msg["content"])
		return claudeMaxMessageOverheadTokens + tokens
	}
	return 0
}

func estimateStructuredTokens(v any) int {
	if v == nil {
		return 0
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return claudeMaxUnknownContentTokens
	}
	return estimateClaudeTextTokens(string(raw))
}

func estimateClaudeTextTokens(text string) int {
	if tokens, ok := estimateTokensByThirdPartyTokenizer(text); ok {
		return tokens
	}
	return estimateClaudeTextTokensHeuristic(text)
}

func estimateClaudeTextTokensHeuristic(text string) int {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if normalized == "" {
		return 0
	}
	asciiChars := 0
	nonASCIIChars := 0
	for _, r := range normalized {
		if r <= 127 {
			asciiChars++
		} else {
			nonASCIIChars++
		}
	}
	tokens := nonASCIIChars
	if asciiChars > 0 {
		tokens += (asciiChars + 3) / 4
	}
	if words := len(strings.Fields(normalized)); words > tokens {
		tokens = words
	}
	if tokens <= 0 {
		return 1
	}
	return tokens
}
