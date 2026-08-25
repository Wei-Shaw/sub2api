package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	kiropkg "github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
)

const kiroMaxWebSearchIterations = 5

var (
	errKiroWebSearchFallback = errors.New("kiro web search fallback")
	kiroWebSearchDescCache   sync.Map
)

type kiroWebSearchExecution struct {
	ResponseBody []byte
	Usage        ClaudeUsage
	RequestID    string
}

type kiroWebSearchHTTPError struct {
	Response *http.Response
}

type kiroStreamChunkCollector struct {
	chunks [][]byte
}

func (e *kiroWebSearchHTTPError) Error() string {
	if e == nil || e.Response == nil {
		return "kiro web search http error"
	}
	return fmt.Sprintf("kiro web search http error: %d", e.Response.StatusCode)
}

func (w *kiroStreamChunkCollector) Write(p []byte) (int, error) {
	if len(p) > 0 {
		w.chunks = append(w.chunks, append([]byte(nil), p...))
	}
	return len(p), nil
}

func bufferKiroAnthropicStream(ctx context.Context, body io.Reader, responseModel string, inputTokens int) ([][]byte, *kiropkg.StreamResult, error) {
	collector := &kiroStreamChunkCollector{}
	result, err := kiropkg.StreamEventStreamAsAnthropicWithContext(ctx, body, collector, responseModel, inputTokens, kiropkg.KiroRequestContext{})
	if err != nil {
		return nil, nil, err
	}
	return collector.chunks, result, nil
}

func writeSSEChunks(w io.Writer, chunks [][]byte) error {
	for _, chunk := range chunks {
		if len(chunk) == 0 {
			continue
		}
		if _, err := w.Write(chunk); err != nil {
			return err
		}
	}
	return nil
}

func writeAnthropicMessageStart(w io.Writer, msgID, model string, inputTokens int, cacheUsage *kiroCacheEmulationUsage) error {
	if strings.TrimSpace(msgID) == "" {
		msgID = "msg_" + kiropkg.GenerateToolUseID()
	}
	if strings.TrimSpace(model) == "" {
		model = "kiro"
	}
	usage := map[string]any{
		"input_tokens":  inputTokens,
		"output_tokens": 0,
	}
	if cacheUsage != nil {
		usage["input_tokens"] = cacheUsage.InputTokens
		usage["cache_creation_input_tokens"] = cacheUsage.CacheCreationInputTokens
		usage["cache_read_input_tokens"] = cacheUsage.CacheReadInputTokens
	}
	payload, err := json.Marshal(map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            msgID,
			"type":          "message",
			"role":          "assistant",
			"model":         model,
			"content":       []any{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         usage,
		},
	})
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, "event: message_start\ndata: "+string(payload)+"\n\n")
	return err
}

// sumKiroUsage 逐字段累加多轮 Kiro 上游响应的 usage（含 credits）。
func sumKiroUsage(total, add kiropkg.Usage) kiropkg.Usage {
	total.InputTokens += add.InputTokens
	total.OutputTokens += add.OutputTokens
	total.TotalTokens += add.TotalTokens
	total.CacheReadInputTokens += add.CacheReadInputTokens
	total.CacheCreationInputTokens += add.CacheCreationInputTokens
	total.CacheCreation5mInputTokens += add.CacheCreation5mInputTokens
	total.CacheCreation1hInputTokens += add.CacheCreation1hInputTokens
	total.KiroCredits += add.KiroCredits
	return total
}

// kiroWebSearchFinalUsageMap 生成与 kiro translator 输出同构的 usage 字段，
// 供 web search 合成 message_delta 使用（计费链路依赖其字段名）。
func kiroWebSearchFinalUsageMap(usage kiropkg.Usage) map[string]any {
	m := map[string]any{
		"input_tokens":                usage.InputTokens,
		"output_tokens":               usage.OutputTokens,
		"cache_read_input_tokens":     usage.CacheReadInputTokens,
		"cache_creation_input_tokens": usage.CacheCreationInputTokens,
	}
	if usage.KiroCredits > 0 {
		m["_sub2api_kiro_credits"] = usage.KiroCredits
	}
	if usage.CacheCreation5mInputTokens > 0 || usage.CacheCreation1hInputTokens > 0 {
		m["cache_creation"] = map[string]any{
			"ephemeral_5m_input_tokens": usage.CacheCreation5mInputTokens,
			"ephemeral_1h_input_tokens": usage.CacheCreation1hInputTokens,
		}
	}
	return m
}

// writeKiroWebSearchTerminalEvents 在 web search 多轮流末尾补发 message_delta
// (携带全轮累计 usage 与最终 stop_reason) 与 message_stop。
func writeKiroWebSearchTerminalEvents(w io.Writer, usage kiropkg.Usage, stopReason string) error {
	if strings.TrimSpace(stopReason) == "" {
		stopReason = "end_turn"
	}
	payload, err := json.Marshal(map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": kiroWebSearchFinalUsageMap(usage),
	})
	if err != nil {
		return err
	}
	if _, err := io.WriteString(w, "event: message_delta\ndata: "+string(payload)+"\n\n"); err != nil {
		return err
	}
	_, err = io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	return err
}

func (s *GatewayService) streamKiroWebSearchAsAnthropic(
	ctx context.Context, account *Account, anthropicBody []byte, mappedModel, requestModel, token string, inputTokens int, headers http.Header, w io.Writer, plan *kiroCacheEmulationPlan,
) error {
	query := kiropkg.ExtractSearchQuery(anthropicBody)
	if strings.TrimSpace(query) == "" {
		return errKiroWebSearchFallback
	}

	currentBody, err := kiropkg.ReplaceWebSearchToolDescription(anthropicBody)
	if err != nil {
		currentBody = anthropicBody
	}
	currentToolUseID := "srvtoolu_" + kiropkg.GenerateToolUseID()
	nextContentBlockIndex := 0
	var totalUsage kiropkg.Usage
	finalStopReason := "end_turn"

	if err := writeAnthropicMessageStart(w, "", requestModel, inputTokens, plan.result()); err != nil {
		return err
	}

	for iteration := 0; iteration < kiroMaxWebSearchIterations; iteration++ {
		s.prefetchKiroWebSearchDescription(ctx, account, token)

		results, nextToken, mcpErr := s.callKiroWebSearchMCP(ctx, account, token, query)
		if strings.TrimSpace(nextToken) != "" {
			token = nextToken
		}
		if mcpErr != nil {
			results = nil
		}

		if err := writeSSEChunks(w, kiropkg.GenerateSearchIndicatorEvents(query, currentToolUseID, results, nextContentBlockIndex)); err != nil {
			return err
		}
		nextContentBlockIndex += 2

		currentBody, err = kiropkg.InjectToolResultsClaude(currentBody, currentToolUseID, query, results)
		if err != nil {
			return errKiroWebSearchFallback
		}

		resp, _, err := s.executeKiroUpstream(ctx, account, currentBody, mappedModel, requestModel, token, headers)
		if err != nil {
			return err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return &kiroWebSearchHTTPError{Response: resp}
		}

		chunks, streamResult, streamErr := func() ([][]byte, *kiropkg.StreamResult, error) {
			defer func() { _ = resp.Body.Close() }()
			return bufferKiroAnthropicStream(ctx, resp.Body, requestModel, inputTokens)
		}()
		if streamErr != nil {
			return streamErr
		}
		// 多轮 web search 每轮都真实消耗上游：累加每轮 usage 与 credits，
		// 避免最终只按最后一轮计费。
		if streamResult != nil {
			totalUsage = sumKiroUsage(totalUsage, streamResult.Usage)
			if strings.TrimSpace(streamResult.StopReason) != "" {
				finalStopReason = streamResult.StopReason
			}
		}

		analysis := kiropkg.AnalyzeBufferedStream(chunks)
		if analysis.HasWebSearchToolUse && strings.TrimSpace(analysis.WebSearchQuery) != "" && iteration+1 < kiroMaxWebSearchIterations {
			filtered := kiropkg.FilterChunksForClient(chunks, analysis.WebSearchToolUseIndex, nextContentBlockIndex)
			if err := writeSSEChunks(w, filtered); err != nil {
				return err
			}
			if maxIndex := kiropkg.MaxContentBlockIndex(filtered); maxIndex >= nextContentBlockIndex {
				nextContentBlockIndex = maxIndex + 1
			}
			query = analysis.WebSearchQuery
			if strings.TrimSpace(analysis.WebSearchToolUseID) == "" {
				currentToolUseID = "srvtoolu_" + kiropkg.GenerateToolUseID()
			} else {
				currentToolUseID = analysis.WebSearchToolUseID
			}
			continue
		}

		for _, chunk := range chunks {
			adjusted, shouldForward := kiropkg.AdjustSSEChunk(chunk, nextContentBlockIndex)
			if !shouldForward {
				continue
			}
			if _, err := w.Write(adjusted); err != nil {
				return err
			}
		}

		// 上游流的 message_start/message_delta/message_stop 已被 AdjustSSEChunk
		// 抑制（message_start 已在函数开头合成发送）；此处补发携带全轮累计
		// usage 的终止事件，保证客户端拿到完整 Anthropic 流、通用流处理器
		// 看到终止事件，且多轮用量不漏计费。
		if err := writeKiroWebSearchTerminalEvents(w, totalUsage, finalStopReason); err != nil {
			return err
		}
		// 整个多轮流完整写出后才提交缓存前缀；中途失败不污染下一次估算。
		plan.commit()
		return nil
	}

	return fmt.Errorf("kiro web search exceeded max iterations")
}

func (s *GatewayService) executeKiroWebSearch(ctx context.Context, account *Account, group *Group, anthropicBody []byte, mappedModel, requestModel, token string, headers http.Header) (*kiroWebSearchExecution, error) {
	query := kiropkg.ExtractSearchQuery(anthropicBody)
	if strings.TrimSpace(query) == "" {
		return nil, errKiroWebSearchFallback
	}

	currentBody, err := kiropkg.ReplaceWebSearchToolDescription(anthropicBody)
	if err != nil {
		currentBody = anthropicBody
	}

	inputTokens := estimateKiroInputTokens(ctx, anthropicBody)
	currentToolUseID := "srvtoolu_" + kiropkg.GenerateToolUseID()
	searches := make([]kiropkg.SearchIndicator, 0, 2)
	requestID := ""
	var cacheUsage *kiroCacheEmulationUsage
	cacheUsageResolved := false
	var totalUsage kiropkg.Usage

	for iteration := 0; iteration < kiroMaxWebSearchIterations; iteration++ {
		s.prefetchKiroWebSearchDescription(ctx, account, token)

		results, nextToken, mcpErr := s.callKiroWebSearchMCP(ctx, account, token, query)
		if strings.TrimSpace(nextToken) != "" {
			token = nextToken
		}
		if mcpErr != nil {
			results = nil
		}
		searches = append(searches, kiropkg.SearchIndicator{
			ToolUseID: currentToolUseID,
			Query:     query,
			Results:   results,
		})

		currentBody, err = kiropkg.InjectToolResultsClaude(currentBody, currentToolUseID, query, results)
		if err != nil {
			return nil, errKiroWebSearchFallback
		}

		resp, _, err := s.executeKiroUpstream(ctx, account, currentBody, mappedModel, requestModel, token, headers)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, &kiroWebSearchHTTPError{Response: resp}
		}

		parseResult, parseErr := func() (*kiropkg.ParseResult, error) {
			defer func() { _ = resp.Body.Close() }()
			if !cacheUsageResolved {
				cachePlan := s.prepareKiroCacheEmulationUsage(ctx, account, group, anthropicBody, mappedModel, inputTokens)
				cacheUsage = cachePlan.result()
				cacheUsageResolved = true
				parse, err := kiropkg.ParseNonStreamingEventStreamWithContext(resp.Body, requestModel, kiropkg.KiroRequestContext{
					CacheEmulationUsage:  cacheUsage.toKiroUsage(),
					EstimatedInputTokens: inputTokens,
				})
				// 首轮解析成功后才提交缓存前缀；解析失败不污染下一次估算。
				if err == nil {
					cachePlan.commit()
				}
				return parse, err
			}
			return kiropkg.ParseNonStreamingEventStreamWithContext(resp.Body, requestModel, kiropkg.KiroRequestContext{
				CacheEmulationUsage:  cacheUsage.toKiroUsage(),
				EstimatedInputTokens: inputTokens,
			})
		}()
		if parseErr != nil {
			return nil, parseErr
		}
		// 多轮 web search 每轮都真实消耗上游：累加每轮 usage 与 credits，避免少计费。
		totalUsage = sumKiroUsage(totalUsage, parseResult.Usage)
		if requestID == "" {
			requestID = buildKiroRequestID(resp)
		}

		nextToolUseID, nextQuery, hasNext := kiropkg.ExtractWebSearchToolUseFromResponse(parseResult.ResponseBody)
		if !hasNext || strings.TrimSpace(nextQuery) == "" || iteration+1 >= kiroMaxWebSearchIterations {
			finalBody, injectErr := kiropkg.InjectSearchIndicatorsInResponse(parseResult.ResponseBody, searches)
			if injectErr == nil {
				parseResult.ResponseBody = finalBody
			}
			return &kiroWebSearchExecution{
				ResponseBody: parseResult.ResponseBody,
				Usage:        kiroUsageToClaude(totalUsage, inputTokens),
				RequestID:    requestID,
			}, nil
		}

		query = nextQuery
		if strings.TrimSpace(nextToolUseID) == "" {
			nextToolUseID = "srvtoolu_" + kiropkg.GenerateToolUseID()
		}
		currentToolUseID = nextToolUseID
	}

	return nil, fmt.Errorf("kiro web search exceeded max iterations")
}

func (s *GatewayService) prefetchKiroWebSearchDescription(ctx context.Context, account *Account, token string) {
	endpoint := kiropkg.BuildMcpEndpoint(kiroAPIRegion(account))
	if cached, ok := kiroWebSearchDescCache.Load(endpoint); ok {
		if desc, ok := cached.(string); ok && strings.TrimSpace(desc) != "" {
			kiropkg.SetCachedWebSearchDescription(desc)
		}
		return
	}

	reqBody, _ := json.Marshal(kiropkg.MCPRequest{
		ID:      "tools_list",
		JSONRPC: "2.0",
		Method:  "tools/list",
	})
	resp, _, err := s.doKiroMCPJSONRequest(ctx, account, endpoint, reqBody, token)
	if err != nil || resp == nil {
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	var result kiropkg.MCPResponse
	if err := json.Unmarshal(body, &result); err != nil || result.Result == nil {
		return
	}
	for _, tool := range result.Result.Tools {
		if strings.EqualFold(tool.Name, "web_search") && strings.TrimSpace(tool.Description) != "" {
			kiroWebSearchDescCache.Store(endpoint, tool.Description)
			kiropkg.SetCachedWebSearchDescription(tool.Description)
			return
		}
	}
}

func (s *GatewayService) callKiroWebSearchMCP(ctx context.Context, account *Account, token, query string) (*kiropkg.WebSearchResults, string, error) {
	reqBody, err := json.Marshal(buildKiroWebSearchMCPRequest(query))
	if err != nil {
		return nil, token, err
	}

	endpoint := kiropkg.BuildMcpEndpoint(kiroAPIRegion(account))
	resp, nextToken, err := s.doKiroMCPJSONRequest(ctx, account, endpoint, reqBody, token)
	if err != nil {
		return nil, nextToken, err
	}
	if resp == nil {
		return nil, nextToken, fmt.Errorf("kiro web search returned nil response")
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nextToken, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nextToken, fmt.Errorf("kiro mcp status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed kiropkg.MCPResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, nextToken, err
	}
	if parsed.Error != nil {
		msg := "unknown error"
		if parsed.Error.Message != nil && strings.TrimSpace(*parsed.Error.Message) != "" {
			msg = strings.TrimSpace(*parsed.Error.Message)
		}
		code := 0
		if parsed.Error.Code != nil {
			code = *parsed.Error.Code
		}
		return nil, nextToken, fmt.Errorf("kiro mcp error %d: %s", code, msg)
	}

	return kiropkg.ParseSearchResults(&parsed), nextToken, nil
}

func buildKiroWebSearchMCPRequest(query string) kiropkg.MCPRequest {
	return kiropkg.MCPRequest{
		ID:      fmt.Sprintf("web_search_%s", kiropkg.GenerateToolUseID()),
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params: map[string]any{
			"name": "web_search",
			"arguments": map[string]any{
				"query": query,
				"_meta": map[string]any{
					"_isValid":        true,
					"_activePath":     []string{"query"},
					"_completedPaths": [][]string{{"query"}},
				},
			},
		},
	}
}

func (s *GatewayService) doKiroMCPJSONRequest(ctx context.Context, account *Account, endpoint string, payload []byte, token string) (*http.Response, string, error) {
	currentToken := token
	accountKey := buildKiroAccountKey(account)
	proxyURL := kiroProxyURL(account)
	tlsProfile := s.tlsFPProfileService.ResolveTLSProfile(account)

	for attempt := 0; attempt < 3; attempt++ {
		if err := s.checkKiroCooldown(ctx, accountKey); err != nil {
			if failoverErr := asKiroCooldownFailoverError(err); failoverErr != nil {
				return nil, currentToken, failoverErr
			}
			return nil, currentToken, err
		}

		req, err := newKiroJSONRequest(ctx, endpoint, payload, currentToken, accountKey, buildKiroMachineID(account), "", account)
		if err != nil {
			return nil, currentToken, err
		}

		resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, tlsProfile)
		if err != nil {
			return nil, currentToken, err
		}

		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			respBody, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr != nil {
				return nil, currentToken, readErr
			}
			if resp.StatusCode == http.StatusForbidden && isKiroSuspendedBody(respBody) {
				if _, err := s.markKiroSuspended(ctx, accountKey); err != nil {
					return nil, currentToken, err
				}
				resp.Body = io.NopCloser(strings.NewReader(string(respBody)))
				return resp, currentToken, nil
			}
			if resp.StatusCode == http.StatusForbidden && !isKiroTokenErrorBody(respBody) {
				resp.Body = io.NopCloser(strings.NewReader(string(respBody)))
				return resp, currentToken, nil
			}
			if s.kiroTokenProvider == nil {
				resp.Body = io.NopCloser(strings.NewReader(string(respBody)))
				return resp, currentToken, nil
			}
			refreshedToken, refreshErr := s.kiroTokenProvider.ForceRefreshAccessToken(ctx, account)
			if refreshErr != nil {
				resp.Body = io.NopCloser(strings.NewReader(string(respBody)))
				return resp, currentToken, nil
			}
			currentToken = refreshedToken
			accountKey = buildKiroAccountKey(account)
			if sleepErr := sleepKiroRetry(ctx, attempt); sleepErr != nil {
				return nil, currentToken, sleepErr
			}
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			if _, err := s.markKiro429(ctx, account.ID, accountKey); err != nil {
				_ = resp.Body.Close()
				return nil, currentToken, err
			}
		}
		if resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode >= 500 {
			if attempt < 2 {
				_ = resp.Body.Close()
				if sleepErr := sleepKiroRetry(ctx, attempt); sleepErr != nil {
					return nil, currentToken, sleepErr
				}
				continue
			}
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if err := s.markKiroSuccess(ctx, account.ID, accountKey); err != nil {
				_ = resp.Body.Close()
				return nil, currentToken, err
			}
		}

		return resp, currentToken, nil
	}

	return nil, currentToken, fmt.Errorf("kiro mcp request retries exhausted")
}
