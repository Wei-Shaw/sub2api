package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	antigravityWebSearchBrokerTool       = "__sub2api_google_search"
	antigravityAgentMaxResponseBytes     = 16 << 20
	antigravityAgentMaxHistoryBytes      = 64 << 20
	antigravityAgentDefaultMaxToolRounds = 8
)

// Raw JSON objects retain unknown protocol fields and opaque signatures. Do not
// round-trip internal history through the shared Claude request converter.
type agentJSONObject = map[string]json.RawMessage

func agentDecodeObject(raw []byte) (agentJSONObject, error) {
	var obj agentJSONObject
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, errors.New("expected JSON object")
	}
	return obj, nil
}

func agentJSON(v any) json.RawMessage {
	// All callers pass JSON-compatible protocol values, never arbitrary objects.
	b, err := json.Marshal(v)
	if err != nil {
		panic("invalid internal agent JSON value")
	}
	return b
}

func agentString(raw json.RawMessage) string {
	var v string
	_ = json.Unmarshal(raw, &v)
	return v
}

func (s *AntigravityGatewayService) agentSearchSetting(ctx context.Context, key, env string) string {
	v := strings.TrimSpace(os.Getenv(env))
	if s != nil && s.settingService != nil && s.settingService.settingRepo != nil {
		if configured, err := s.settingService.settingRepo.GetValue(ctx, key); err == nil && strings.TrimSpace(configured) != "" {
			v = strings.TrimSpace(configured)
		}
	}
	return v
}

func (s *AntigravityGatewayService) antigravityAgentWebSearchEnabled(ctx context.Context) bool {
	v := s.agentSearchSetting(ctx, "antigravity_agent_web_search_enabled", "ANTIGRAVITY_AGENT_WEB_SEARCH_ENABLED")
	if v == "" {
		return true
	}
	enabled, err := strconv.ParseBool(v)
	return err == nil && enabled
}

func (s *AntigravityGatewayService) antigravityAgentToolRoundLimit(ctx context.Context) int {
	v := s.agentSearchSetting(ctx, "antigravity_agent_max_tool_rounds", "ANTIGRAVITY_AGENT_MAX_TOOL_ROUNDS")
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return antigravityAgentDefaultMaxToolRounds
	}
	return min(n, 32)
}

// prepareAgentSearch is called only for Gemini Responses, after mapping the
// model and before the first upstream request. The enable switch is snapshotted
// here, so disabling it cannot leave an unhandled internal function upstream.
func prepareAgentSearch(body []byte, enabled bool) ([]byte, bool, error) {
	envelope, err := agentDecodeObject(body)
	if err != nil {
		return nil, false, err
	}
	request, err := agentDecodeObject(envelope["request"])
	if err != nil {
		return nil, false, err
	}
	var tools []agentJSONObject
	if err := json.Unmarshal(request["tools"], &tools); err != nil {
		return body, false, nil
	}
	hasSearch, hasFunctions, reserved := false, false, false
	for _, tool := range tools {
		hasSearch = hasSearch || tool["googleSearch"] != nil
		var declarations []agentJSONObject
		if err := json.Unmarshal(tool["functionDeclarations"], &declarations); err == nil {
			hasFunctions = hasFunctions || len(declarations) > 0
			for _, declaration := range declarations {
				if enabled && agentString(declaration["name"]) == antigravityWebSearchBrokerTool {
					reserved = true
				}
			}
		}
	}
	if !hasSearch || !hasFunctions {
		return body, hasSearch && enabled, nil
	}
	if reserved {
		return nil, false, errors.New("reserved Google Search function name")
	}
	var lowered []agentJSONObject
	for _, tool := range tools {
		delete(tool, "googleSearch")
		if len(tool) > 0 {
			lowered = append(lowered, tool)
		}
	}
	if enabled {
		lowered = append(lowered, agentJSONObject{"functionDeclarations": agentJSON([]any{map[string]any{
			"name":        antigravityWebSearchBrokerTool,
			"description": "Search Google for current information when needed. Returns a grounded summary and sources. This tool is executed by the server.",
			"parameters":  map[string]any{"type": "OBJECT", "properties": map[string]any{"query": map[string]any{"type": "STRING"}}, "required": []string{"query"}},
		}})})
	}
	request["tools"] = agentJSON(lowered)
	config, _ := agentDecodeObject(request["toolConfig"])
	if config != nil {
		delete(config, "includeServerSideToolInvocations")
		request["toolConfig"] = agentJSON(config)
	}
	envelope["request"] = agentJSON(request)
	return agentJSON(envelope), enabled, nil
}

type antigravityAgentTurn struct {
	content    agentJSONObject
	grounding  json.RawMessage
	responseID string
	usage      antigravity.GeminiUsageMetadata
	raw        []byte
}

func configureAgentToolChoice(body, original []byte) ([]byte, bool, error) {
	client, err := agentDecodeObject(original)
	if err != nil {
		return nil, false, err
	}
	var originalTools []agentJSONObject
	if value := client["tools"]; value != nil {
		if err := json.Unmarshal(value, &originalTools); err != nil {
			return nil, false, err
		}
	}
	for _, tool := range originalTools {
		kind := agentString(tool["type"])
		if kind != "web_search" && kind != "web_search_preview" && kind != "web_search_preview_2025_03_11" {
			continue
		}
		// Do not silently bypass client search restrictions that Google's
		// native tool cannot represent through this bridge.
		if raw, present := tool["external_web_access"]; present {
			var externalWebAccess *bool
			if err := json.Unmarshal(raw, &externalWebAccess); err != nil || externalWebAccess == nil {
				return nil, false, errors.New("external_web_access must be a boolean")
			}
			if !*externalWebAccess {
				return nil, false, errors.New("unsupported Google Search restrictions")
			}
		}
		if tool["filters"] != nil || tool["user_location"] != nil {
			return nil, false, errors.New("unsupported Google Search restrictions")
		}
	}
	choice := agentString(client["tool_choice"])
	var named agentJSONObject
	if len(client["tool_choice"]) > 0 && choice == "" && string(client["tool_choice"]) != "null" {
		named, err = agentDecodeObject(client["tool_choice"])
		if err != nil {
			return nil, false, err
		}
		choice = agentString(named["type"])
	}
	if choice == "" || choice == "auto" {
		return body, true, nil
	}
	envelope, err := agentDecodeObject(body)
	if err != nil {
		return nil, false, err
	}
	request, err := agentDecodeObject(envelope["request"])
	if err != nil {
		return nil, false, err
	}
	var tools []agentJSONObject
	if err := json.Unmarshal(request["tools"], &tools); err != nil {
		return nil, false, err
	}
	var cleaned []agentJSONObject
	broker := false
	for _, tool := range tools {
		var declarations []agentJSONObject
		_ = json.Unmarshal(tool["functionDeclarations"], &declarations)
		var kept []agentJSONObject
		for _, declaration := range declarations {
			internal := agentString(declaration["name"]) == antigravityWebSearchBrokerTool
			broker = broker || internal
			if choice != "none" || !internal {
				kept = append(kept, declaration)
			}
		}
		if choice == "none" {
			delete(tool, "googleSearch")
			delete(tool, "functionDeclarations")
			if len(kept) > 0 {
				tool["functionDeclarations"] = agentJSON(kept)
			}
		}
		if len(tool) > 0 {
			cleaned = append(cleaned, tool)
		}
	}
	var mode, name string
	switch choice {
	case "none":
		mode = "NONE"
	case "required":
		mode = "ANY"
	case "function":
		mode, name = "ANY", agentString(named["name"])
	case "web_search", "web_search_preview":
		mode, name = "ANY", antigravityWebSearchBrokerTool
	default:
		return nil, false, errors.New("unsupported search tool choice")
	}
	// Native search-only requests do not accept a function calling config.
	if !broker && choice != "none" {
		return body, true, nil
	}
	fc := map[string]any{"mode": mode}
	if name != "" {
		fc["allowedFunctionNames"] = []string{name}
	}
	request["tools"] = agentJSON(cleaned)
	request["toolConfig"] = agentJSON(map[string]any{"functionCallingConfig": fc})
	envelope["request"] = agentJSON(request)
	return agentJSON(envelope), choice != "none", nil
}

type antigravityAgentSearchRecord struct {
	callID string
	query  string
	turn   antigravityAgentTurn
}

type antigravityAgentSearchResult struct {
	usage    antigravity.GeminiUsageMetadata
	searches []antigravityAgentSearchRecord
}

// readAntigravityAgentTurn bounds the complete response, not just individual
// SSE lines. Cancellation closes the body to unblock a pending network read.
func readAntigravityAgentTurn(ctx context.Context, resp *http.Response, timeout time.Duration) (antigravityAgentTurn, error) {
	var out antigravityAgentTurn
	if resp == nil || resp.Body == nil {
		return out, errors.New("empty agent response")
	}
	defer func() { _ = resp.Body.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = resp.Body.Close() })
	defer stop()
	var timedOut atomic.Bool
	var timer *time.Timer
	if timeout > 0 {
		timer = time.AfterFunc(timeout, func() { timedOut.Store(true); _ = resp.Body.Close() })
		defer timer.Stop()
	}
	var raw bytes.Buffer
	limited := &io.LimitedReader{R: resp.Body, N: antigravityAgentMaxResponseBytes + 1}
	scanner := bufio.NewScanner(io.TeeReader(limited, &raw))
	scanner.Buffer(make([]byte, 64<<10), antigravityAgentMaxResponseBytes)
	var data []string
	var parts []agentJSONObject
	valid, finished := false, false
	consume := func() error {
		if len(data) == 0 {
			return nil
		}
		payload := strings.Join(data, "\n")
		data = nil
		if payload == "[DONE]" {
			return nil
		}
		envelope, err := agentDecodeObject([]byte(payload))
		if err != nil {
			return errors.New("malformed agent SSE JSON")
		}
		response, err := agentDecodeObject(envelope["response"])
		if err != nil {
			return errors.New("missing agent response envelope")
		}
		valid = true
		if usage := response["usageMetadata"]; usage != nil {
			if err := json.Unmarshal(usage, &out.usage); err != nil {
				return err
			}
		}
		if id := agentString(response["responseId"]); id != "" {
			out.responseID = id
		}
		var candidates []agentJSONObject
		if value := response["candidates"]; value != nil {
			if err := json.Unmarshal(value, &candidates); err != nil {
				return err
			}
		}
		if len(candidates) > 1 {
			return errors.New("multiple agent candidates are unsupported")
		}
		if len(candidates) == 0 {
			return nil
		}
		candidate := candidates[0]
		finished = finished || agentString(candidate["finishReason"]) != ""
		if value := candidate["content"]; value != nil {
			content, err := agentDecodeObject(value)
			if err != nil {
				return err
			}
			var chunk []agentJSONObject
			if err := json.Unmarshal(content["parts"], &chunk); err != nil {
				return err
			}
			for _, part := range chunk {
				if fc := part["functionCall"]; fc != nil {
					call, err := agentDecodeObject(fc)
					if err != nil {
						return err
					}
					if agentString(call["id"]) == "" {
						call["id"] = agentJSON("call_" + uuid.NewString())
						part["functionCall"] = agentJSON(call)
					}
				}
				// Preserve signature-only chunks as explicit empty text parts.
				if part["thoughtSignature"] != nil {
					hasData := false
					for _, field := range []string{"text", "inlineData", "fileData", "functionCall", "functionResponse", "executableCode", "codeExecutionResult"} {
						hasData = hasData || part[field] != nil
					}
					if !hasData {
						part["text"] = agentJSON("")
					}
				}
				parts = append(parts, part)
			}
			out.content = content
		}
		if value := candidate["groundingMetadata"]; value != nil {
			out.grounding = append(json.RawMessage(nil), value...)
		}
		return nil
	}
	for scanner.Scan() {
		if timer != nil {
			timer.Reset(timeout)
		}
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if err := consume(); err != nil {
				return out, err
			}
		} else if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	if timedOut.Load() {
		return out, errors.New("stream data interval timeout")
	}
	if limited.N <= 0 {
		return out, errors.New("agent response exceeds byte limit")
	}
	if err := scanner.Err(); err != nil {
		return out, err
	}
	if err := consume(); err != nil {
		return out, err
	}
	if !valid || !finished {
		return out, errors.New("incomplete agent response")
	}
	if out.content == nil {
		out.content = agentJSONObject{"role": agentJSON("model")}
	}
	out.content["parts"] = agentJSON(parts)
	out.raw = raw.Bytes()
	return out, nil
}

func agentFunctionCalls(content agentJSONObject) ([]agentJSONObject, error) {
	var parts []agentJSONObject
	if err := json.Unmarshal(content["parts"], &parts); err != nil {
		return nil, err
	}
	var calls []agentJSONObject
	seen := map[string]bool{}
	for _, part := range parts {
		if part["functionCall"] == nil {
			continue
		}
		call, err := agentDecodeObject(part["functionCall"])
		if err != nil {
			return nil, err
		}
		id := agentString(call["id"])
		if id == "" || seen[id] {
			return nil, errors.New("missing or duplicate agent call ID")
		}
		seen[id] = true
		calls = append(calls, call)
	}
	return calls, nil
}

func buildAgentSearchOnlyBody(body []byte, query string) ([]byte, error) {
	envelope, err := agentDecodeObject(body)
	if err != nil {
		return nil, err
	}
	request, err := agentDecodeObject(envelope["request"])
	if err != nil {
		return nil, err
	}
	request["contents"] = agentJSON([]any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": query}}}})
	request["systemInstruction"] = agentJSON(map[string]any{"role": "user", "parts": []any{map[string]any{"text": "Search Google for the requested information. Return a concise factual summary grounded in sources."}}})
	request["tools"] = agentJSON([]any{map[string]any{"googleSearch": map[string]any{}}})
	delete(request, "toolConfig")
	// A client's output schema governs the final answer, not the search summary.
	if generation, err := agentDecodeObject(request["generationConfig"]); err == nil {
		delete(generation, "responseSchema")
		delete(generation, "responseJsonSchema")
		delete(generation, "responseMimeType")
		request["generationConfig"] = agentJSON(generation)
	}
	envelope["requestId"] = agentJSON("agent-search-" + uuid.NewString())
	envelope["request"] = agentJSON(request)
	return agentJSON(envelope), nil
}

func appendAgentToolRound(body []byte, model agentJSONObject, results map[string]json.RawMessage) ([]byte, error) {
	envelope, err := agentDecodeObject(body)
	if err != nil {
		return nil, err
	}
	request, err := agentDecodeObject(envelope["request"])
	if err != nil {
		return nil, err
	}
	var contents []json.RawMessage
	if err := json.Unmarshal(request["contents"], &contents); err != nil {
		return nil, err
	}
	calls, err := agentFunctionCalls(model)
	if err != nil {
		return nil, err
	}
	var parts []any
	for _, call := range calls {
		id, name := agentString(call["id"]), agentString(call["name"])
		result, ok := results[id]
		if !ok {
			if name == antigravityWebSearchBrokerTool {
				return nil, errors.New("unpaired search result")
			}
			result = agentJSON(map[string]any{"ok": false, "deferred": true, "message": "Client tool not executed; decide again after reading the search results."})
		}
		parts = append(parts, map[string]any{"functionResponse": map[string]any{"id": id, "name": name, "response": result}})
	}
	contents = append(contents, agentJSON(model), agentJSON(map[string]any{"role": "user", "parts": parts}))
	request["contents"] = agentJSON(contents)
	// A forced/required tool choice is satisfied by the internal search. Do
	// not force it again on every continuation and manufacture an endless loop.
	request["toolConfig"] = agentJSON(map[string]any{"functionCallingConfig": map[string]any{"mode": "AUTO"}})
	envelope["request"] = agentJSON(request)
	envelope["requestId"] = agentJSON("agent-continue-" + uuid.NewString())
	out := agentJSON(envelope)
	if len(out) > antigravityAgentMaxHistoryBytes {
		return nil, errors.New("agent history exceeds byte limit")
	}
	return out, nil
}

func addAgentUsage(dst *antigravity.GeminiUsageMetadata, src antigravity.GeminiUsageMetadata) {
	dst.PromptTokenCount += src.PromptTokenCount
	dst.CandidatesTokenCount += src.CandidatesTokenCount
	dst.CachedContentTokenCount += src.CachedContentTokenCount
	dst.ThoughtsTokenCount += src.ThoughtsTokenCount
	dst.TotalTokenCount += src.TotalTokenCount
}

func agentToolError(code string) json.RawMessage {
	return agentJSON(map[string]any{"ok": false, "error": map[string]any{"code": code, "message": "Search could not be completed; revise the query or answer without claiming verified search results."}})
}

func (s *AntigravityGatewayService) runAntigravityWebSearchAgentLoop(ctx context.Context, c *gin.Context, account *Account, call *antigravityCompatUpstreamCall, first *http.Response) (*http.Response, error) {
	body, resp := call.geminiBody, first
	result := &antigravityAgentSearchResult{}
	limit := s.antigravityAgentToolRoundLimit(ctx)
	lastQuery, repeated := "", 0
	searchAttempts := 0
	for round := 0; ; round++ {
		if resp.StatusCode >= 400 {
			return resp, nil
		}
		turn, err := readAntigravityAgentTurn(ctx, resp, s.antigravityCompatStreamTimeout())
		if err != nil {
			return nil, err
		}
		addAgentUsage(&result.usage, turn.usage)
		calls, err := agentFunctionCalls(turn.content)
		if err != nil {
			return nil, err
		}
		var searches []agentJSONObject
		for _, fc := range calls {
			if agentString(fc["name"]) == antigravityWebSearchBrokerTool {
				searches = append(searches, fc)
			}
		}
		if len(searches) == 0 {
			if turn.grounding != nil {
				var grounding antigravity.GeminiGroundingMetadata
				if json.Unmarshal(turn.grounding, &grounding) == nil && len(grounding.WebSearchQueries) > 0 {
					result.searches = append(result.searches, antigravityAgentSearchRecord{query: strings.Join(grounding.WebSearchQueries, "; "), turn: turn})
				}
			}
			call.agentResult = result
			resp.Body = io.NopCloser(bytes.NewReader(turn.raw))
			return resp, nil
		}
		if round >= limit {
			return nil, errors.New("google search tool round limit reached")
		}
		if len(searches) > 32 {
			return nil, errors.New("too many Google Search calls in one round")
		}
		results := make(map[string]json.RawMessage, len(searches))
		for _, fc := range searches {
			id := agentString(fc["id"])
			if searchAttempts >= 32 {
				results[id] = agentToolError("search_call_limit")
				continue
			}
			args, _ := agentDecodeObject(fc["args"])
			query := strings.TrimSpace(agentString(args["query"]))
			if query == "" || len(query) > 8192 {
				results[id] = agentToolError("invalid_query")
				continue
			}
			if query == lastQuery {
				repeated++
			} else {
				lastQuery, repeated = query, 1
			}
			if repeated >= 3 {
				results[id] = agentToolError("repeated_query")
				continue
			}
			searchBody, err := buildAgentSearchOnlyBody(body, query)
			if err != nil {
				return nil, err
			}
			searchAttempts++
			searchResp, err := s.doAntigravityAgentRequest(ctx, c, account, call, searchBody)
			if err != nil {
				return nil, err
			}
			if searchResp.StatusCode >= 400 {
				if searchResp.StatusCode != http.StatusBadRequest {
					return searchResp, nil
				}
				_ = searchResp.Body.Close()
				results[id] = agentToolError("upstream_rejected_search")
				continue
			}
			searchTurn, err := readAntigravityAgentTurn(ctx, searchResp, s.antigravityCompatStreamTimeout())
			if err != nil {
				return nil, err
			}
			addAgentUsage(&result.usage, searchTurn.usage)
			searchTurn.raw = nil
			var grounding antigravity.GeminiGroundingMetadata
			if json.Unmarshal(searchTurn.grounding, &grounding) != nil || (len(grounding.WebSearchQueries) == 0 && len(grounding.GroundingChunks) == 0) {
				results[id] = agentToolError("no_search_grounding")
				continue
			}
			// Keep the real search candidate and opaque signatures with its own
			// result. Grounding offsets are never moved onto unrelated final text.
			results[id] = agentJSON(map[string]any{"ok": true, "query": query, "call_id": id, "response_id": searchTurn.responseID, "content": searchTurn.content, "groundingMetadata": searchTurn.grounding})
			result.searches = append(result.searches, antigravityAgentSearchRecord{callID: id, query: query, turn: searchTurn})
		}
		body, err = appendAgentToolRound(body, turn.content, results)
		if err != nil {
			return nil, err
		}
		resp, err = s.doAntigravityAgentRequest(ctx, c, account, call, body)
		if err != nil {
			return nil, err
		}
	}
}

func (s *AntigravityGatewayService) doAntigravityAgentRequest(ctx context.Context, c *gin.Context, account *Account, call *antigravityCompatUpstreamCall, body []byte) (*http.Response, error) {
	result, err := s.antigravityRetryLoop(antigravityRetryLoopParams{
		ctx: ctx, prefix: call.prefix, account: account, proxyURL: call.proxyURL, accessToken: call.accessToken,
		action: "streamGenerateContent", body: body, c: c, httpUpstream: s.httpUpstream,
		settingService: s.settingService, accountRepo: s.accountRepo, handleError: s.handleUpstreamError,
		requestedModel: call.request.originalModel,
	})
	if err != nil {
		return nil, fmt.Errorf("agent upstream request: %w", err)
	}
	return result.resp, nil
}
