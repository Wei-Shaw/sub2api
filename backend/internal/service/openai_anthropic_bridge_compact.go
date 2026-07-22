package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type anthropicBridgeCompactResult struct {
	Body    []byte
	Usage   OpenAIUsage
	Applied bool
}

type anthropicBridgeCompactResponse struct {
	Output []json.RawMessage `json:"output"`
	Usage  struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func anthropicBridgeActiveSuffixItemCount(req *apicompat.AnthropicRequest) int {
	if req == nil || len(req.Messages) == 0 {
		return 0
	}

	last := len(req.Messages) - 1
	if req.Messages[last].Role != "user" {
		return 0
	}
	start := last
	if anthropicMessageContainsToolResult(req.Messages[last]) && last > 0 && req.Messages[last-1].Role == "assistant" {
		start = last - 1
	}

	active := *req
	active.System = nil
	active.Messages = append([]apicompat.AnthropicMessage(nil), req.Messages[start:]...)
	converted, err := apicompat.AnthropicToResponses(&active)
	if err != nil {
		return 0
	}
	var items []json.RawMessage
	if err := json.Unmarshal(converted.Input, &items); err != nil {
		return 0
	}
	return len(items)
}

func anthropicMessageContainsToolResult(message apicompat.AnthropicMessage) bool {
	var blocks []apicompat.AnthropicContentBlock
	if err := json.Unmarshal(message.Content, &blocks); err != nil {
		return false
	}
	for _, block := range blocks {
		if block.Type == "tool_result" {
			return true
		}
	}
	return false
}

func (s *OpenAIGatewayService) maybeAutoCompactAnthropicBridge(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	requestedModel string,
	token string,
	promptCacheKey string,
	turnState string,
	activeSuffixItems int,
) anthropicBridgeCompactResult {
	result := anthropicBridgeCompactResult{Body: body}
	if s == nil || s.cfg == nil || s.httpUpstream == nil || !s.cfg.Gateway.AnthropicBridgeAutoCompactEnabled ||
		account == nil || account.Type != AccountTypeOAuth || account.Platform == PlatformGrok ||
		!account.AllowsOpenAICompact() || activeSuffixItems <= 0 || c == nil || c.Request == nil {
		return result
	}

	var requestFields map[string]json.RawMessage
	if err := json.Unmarshal(body, &requestFields); err != nil {
		return result
	}
	upstreamModel := jsonStringValue(requestFields["model"])
	if !isExplicitGPTAnthropicBridgeModel(requestedModel) || !shouldAutoInjectPromptCacheKeyForCompat(upstreamModel) {
		return result
	}
	inputRaw, ok := requestFields["input"]
	if !ok || len(inputRaw) < s.cfg.Gateway.AnthropicBridgeAutoCompactInputBytes {
		return result
	}

	var input []json.RawMessage
	if err := json.Unmarshal(inputRaw, &input); err != nil || len(input) <= activeSuffixItems {
		return result
	}
	prefix := input[:len(input)-activeSuffixItems]
	suffix := input[len(input)-activeSuffixItems:]
	prefixRaw, err := json.Marshal(prefix)
	if err != nil {
		return result
	}

	compactFields := make(map[string]json.RawMessage, 8)
	for _, field := range []string{
		"model",
		"input",
		"instructions",
		"tools",
		"parallel_tool_calls",
		"reasoning",
		"text",
		"previous_response_id",
	} {
		if value, exists := requestFields[field]; exists {
			compactFields[field] = value
		}
	}
	compactFields["input"] = prefixRaw
	compactModel := upstreamModel
	if mapped, matched := account.ResolveCompactMappedModel(upstreamModel); matched {
		compactModel = strings.TrimSpace(mapped)
	}
	if compactModel != "" {
		compactFields["model"], _ = json.Marshal(compactModel)
	}
	compactBody, err := json.Marshal(compactFields)
	if err != nil {
		return result
	}

	timeout := time.Duration(s.cfg.Gateway.AnthropicBridgeAutoCompactTimeoutSeconds) * time.Second
	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()
	compactCtx, cancel := context.WithTimeout(upstreamCtx, timeout)
	defer cancel()

	compactC := c.Copy()
	compactC.Request = c.Request.Clone(compactCtx)
	compactURL := *compactC.Request.URL
	compactURL.Path = "/v1/responses/compact"
	compactURL.RawPath = ""
	compactC.Request.URL = &compactURL
	setOpenAICompatMessagesBridgeContext(compactC, true)

	upstreamReq, err := s.buildUpstreamRequest(compactCtx, compactC, account, compactBody, token, false, promptCacheKey, true)
	if err != nil {
		s.logAnthropicBridgeCompactFallback(account, "build_request", 0, len(inputRaw), time.Duration(0), err)
		return result
	}
	ensureCodexIdentityHeaders(upstreamReq.Header)
	enforceCodexIdentityHeaders(upstreamReq.Header)
	if promptCacheKey != "" {
		apiKeyID := getAPIKeyIDFromContext(c)
		isolatedSessionID := generateSessionUUID(isolateOpenAISessionID(apiKeyID, promptCacheKey))
		upstreamReq.Header.Set("session_id", isolatedSessionID)
		if upstreamReq.Header.Get("conversation_id") != "" {
			upstreamReq.Header.Set("conversation_id", isolatedSessionID)
		}
		if strings.TrimSpace(c.GetHeader("conversation_id")) == "" {
			upstreamReq.Header.Del("conversation_id")
		}
	}
	if strings.TrimSpace(turnState) != "" {
		upstreamReq.Header.Set("x-codex-turn-state", strings.TrimSpace(turnState))
	}

	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	startedAt := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	latency := time.Since(startedAt)
	if err != nil {
		s.logAnthropicBridgeCompactFallback(account, "transport", 0, len(inputRaw), latency, err)
		return result
	}
	if resp == nil {
		s.logAnthropicBridgeCompactFallback(account, "empty_upstream_response", 0, len(inputRaw), latency, nil)
		return result
	}
	defer func() { _ = resp.Body.Close() }()

	responseBody, err := readUpstreamResponseBodyLimited(resp.Body, resolveUpstreamResponseReadLimit(s.cfg))
	if err != nil {
		s.logAnthropicBridgeCompactFallback(account, "read_response", resp.StatusCode, len(inputRaw), latency, err)
		return result
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		s.logAnthropicBridgeCompactFallback(account, "upstream_status", resp.StatusCode, len(inputRaw), latency, nil)
		return result
	}

	var compactResponse anthropicBridgeCompactResponse
	if err := json.Unmarshal(responseBody, &compactResponse); err != nil || len(compactResponse.Output) == 0 {
		if err == nil {
			err = fmt.Errorf("compact response output is empty")
		}
		s.logAnthropicBridgeCompactFallback(account, "invalid_response", resp.StatusCode, len(inputRaw), latency, err)
		return result
	}

	merged := make([]json.RawMessage, 0, len(compactResponse.Output)+len(suffix))
	merged = append(merged, compactResponse.Output...)
	merged = append(merged, suffix...)
	mergedRaw, err := json.Marshal(merged)
	if err != nil || len(mergedRaw) >= len(inputRaw) {
		if err == nil {
			err = fmt.Errorf("compact response did not reduce input size")
		}
		s.logAnthropicBridgeCompactFallback(account, "no_reduction", resp.StatusCode, len(inputRaw), latency, err)
		return result
	}

	requestFields["input"] = mergedRaw
	updatedBody, err := json.Marshal(requestFields)
	if err != nil {
		s.logAnthropicBridgeCompactFallback(account, "marshal_result", resp.StatusCode, len(inputRaw), latency, err)
		return result
	}

	logger.L().Info("openai messages: anthropic bridge history compacted",
		zap.Int64("account_id", account.ID),
		zap.Int("input_bytes_before", len(inputRaw)),
		zap.Int("input_bytes_after", len(mergedRaw)),
		zap.Int("history_items_before", len(prefix)),
		zap.Int("active_suffix_items", len(suffix)),
		zap.Int("compact_output_items", len(compactResponse.Output)),
		zap.Int("compact_input_tokens", compactResponse.Usage.InputTokens),
		zap.Int("compact_output_tokens", compactResponse.Usage.OutputTokens),
		zap.Int64("compact_latency_ms", latency.Milliseconds()),
	)
	result.Body = updatedBody
	result.Usage = OpenAIUsage{
		InputTokens:  compactResponse.Usage.InputTokens,
		OutputTokens: compactResponse.Usage.OutputTokens,
	}
	result.Applied = true
	return result
}

func isExplicitGPTAnthropicBridgeModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(lastOpenAIModelSegment(model)))
	if strings.HasPrefix(model, "claude-") {
		model = strings.TrimPrefix(model, "claude-")
	}
	return strings.HasPrefix(model, "gpt-5") || strings.HasPrefix(model, "gpt5")
}

func jsonStringValue(raw json.RawMessage) string {
	var value string
	_ = json.Unmarshal(raw, &value)
	return strings.TrimSpace(value)
}

func (s *OpenAIGatewayService) logAnthropicBridgeCompactFallback(
	account *Account,
	reason string,
	statusCode int,
	inputBytes int,
	latency time.Duration,
	err error,
) {
	fields := []zap.Field{
		zap.String("reason", reason),
		zap.Int("status_code", statusCode),
		zap.Int("input_bytes", inputBytes),
		zap.Int64("compact_latency_ms", latency.Milliseconds()),
	}
	if account != nil {
		fields = append(fields, zap.Int64("account_id", account.ID))
	}
	if err != nil {
		fields = append(fields, zap.String("error_type", fmt.Sprintf("%T", err)))
	}
	logger.L().Warn("openai messages: anthropic bridge compact failed open", fields...)
}
