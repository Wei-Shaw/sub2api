package kiro

import (
	"encoding/json"
	"strings"
)

// StreamCallback is the interface the response transformer uses to emit
// to the inbound client. Implementations adapt the events to Anthropic
// SSE or OpenAI SSE (Phase 5) or to a buffered non-streaming response.
//
// All methods are called from a single goroutine; no concurrent access
// protection required.
type StreamCallback struct {
	OnText         func(text string, isThinking bool)
	OnToolUse      func(tu ToolUse)
	OnComplete     func(inputTokens, outputTokens int)
	OnError        func(err error)
	OnCredits      func(credits float64)
	OnContextUsage func(percentage float64)
}

// ProcessEvents walks decoded Kiro events and invokes the appropriate
// StreamCallback method for each. toolNameMap restores original tool
// names (set by the request transformer).
//
// Returns when the input event sequence ends. Token / credit totals are
// reported via OnComplete / OnCredits exactly once.
func ProcessEvents(events []Event, toolNameMap map[string]string, cb *StreamCallback) {
	if cb == nil {
		return
	}
	var inputTokens, outputTokens int
	var totalCredits float64
	var currentToolUse *toolUseState
	var lastAssistantContent string
	var lastReasoningContent string

	for _, ev := range events {
		inputTokens, outputTokens = updateTokensFromEvent(ev.Payload, inputTokens, outputTokens)
		dispatchEvent(ev, toolNameMap, cb,
			&currentToolUse,
			&lastAssistantContent,
			&lastReasoningContent,
			&totalCredits,
		)
	}

	// Flush any open tool use that didn't see an explicit stop event.
	if currentToolUse != nil {
		finishToolUse(currentToolUse, toolNameMap, cb)
	}
	if cb.OnCredits != nil && totalCredits > 0 {
		cb.OnCredits(totalCredits)
	}
	if cb.OnComplete != nil {
		cb.OnComplete(inputTokens, outputTokens)
	}
}

// ProcessEventsFromCallback exposes the per-event dispatch for streaming
// callers that decode the event stream incrementally (with
// DecodeEventStream). Returns a closure that should be invoked for every
// event, plus a finalizer to call after the stream ends so totals and
// flushes happen exactly once.
//
// Usage:
//
//	dispatch, finalize := ProcessEventsFromCallback(toolNameMap, cb)
//	_ = DecodeEventStream(body, dispatch)
//	finalize()
func ProcessEventsFromCallback(toolNameMap map[string]string, cb *StreamCallback) (func(Event), func()) {
	var inputTokens, outputTokens int
	var totalCredits float64
	var currentToolUse *toolUseState
	var lastAssistantContent string
	var lastReasoningContent string

	dispatch := func(ev Event) {
		if cb == nil {
			return
		}
		inputTokens, outputTokens = updateTokensFromEvent(ev.Payload, inputTokens, outputTokens)
		dispatchEvent(ev, toolNameMap, cb,
			&currentToolUse,
			&lastAssistantContent,
			&lastReasoningContent,
			&totalCredits,
		)
	}
	finalize := func() {
		if cb == nil {
			return
		}
		if currentToolUse != nil {
			finishToolUse(currentToolUse, toolNameMap, cb)
		}
		if cb.OnCredits != nil && totalCredits > 0 {
			cb.OnCredits(totalCredits)
		}
		if cb.OnComplete != nil {
			cb.OnComplete(inputTokens, outputTokens)
		}
	}
	return dispatch, finalize
}

func dispatchEvent(
	ev Event,
	toolNameMap map[string]string,
	cb *StreamCallback,
	currentToolUse **toolUseState,
	lastAssistantContent, lastReasoningContent *string,
	totalCredits *float64,
) {
	switch ev.Type {
	case "assistantResponseEvent":
		if content, ok := ev.Payload["content"].(string); ok && content != "" {
			delta := normalizeChunk(content, lastAssistantContent)
			if delta != "" && cb.OnText != nil {
				cb.OnText(delta, false)
			}
		}
	case "reasoningContentEvent":
		if text, ok := ev.Payload["text"].(string); ok && text != "" {
			delta := normalizeChunk(text, lastReasoningContent)
			if delta != "" && cb.OnText != nil {
				cb.OnText(delta, true)
			}
		}
	case "toolUseEvent":
		*currentToolUse = handleToolUseEvent(ev.Payload, *currentToolUse, toolNameMap, cb)
	case "meteringEvent":
		if usage, ok := ev.Payload["usage"].(float64); ok {
			*totalCredits += usage
		}
	case "contextUsageEvent":
		if pct, ok := ev.Payload["contextUsagePercentage"].(float64); ok {
			if cb.OnContextUsage != nil {
				cb.OnContextUsage(pct)
			}
		}
	}
}

// normalizeChunk handles upstream's habit of re-sending growing prefixes
// of the assistant message. Returns just the new delta since the last
// chunk; updates *previous in place.
func normalizeChunk(chunk string, previous *string) string {
	if chunk == "" {
		return ""
	}
	prev := *previous
	if prev == "" {
		*previous = chunk
		return chunk
	}
	if chunk == prev {
		return ""
	}
	if strings.HasPrefix(chunk, prev) {
		delta := chunk[len(prev):]
		*previous = chunk
		return delta
	}
	if strings.HasPrefix(prev, chunk) {
		return ""
	}
	// Overlap heuristic: shrink to the longest suffix of prev that
	// matches a prefix of chunk.
	maxOverlap := 0
	maxLen := len(prev)
	if len(chunk) < maxLen {
		maxLen = len(chunk)
	}
	for i := maxLen; i > 0; i-- {
		if strings.HasSuffix(prev, chunk[:i]) {
			maxOverlap = i
			break
		}
	}
	*previous = chunk
	if maxOverlap > 0 {
		return chunk[maxOverlap:]
	}
	return chunk
}

type toolUseState struct {
	ToolUseID   string
	Name        string
	InputBuffer strings.Builder
}

func handleToolUseEvent(event map[string]any, current *toolUseState, toolNameMap map[string]string, cb *StreamCallback) *toolUseState {
	toolUseID, _ := event["toolUseId"].(string)
	name, _ := event["name"].(string)
	isStop, _ := event["stop"].(bool)

	if toolUseID != "" && name != "" {
		if current == nil {
			current = &toolUseState{ToolUseID: toolUseID, Name: name}
		} else if current.ToolUseID != toolUseID {
			finishToolUse(current, toolNameMap, cb)
			current = &toolUseState{ToolUseID: toolUseID, Name: name}
		}
	}

	if current != nil {
		switch v := event["input"].(type) {
		case string:
			current.InputBuffer.WriteString(v)
		case map[string]any:
			data, _ := json.Marshal(v)
			current.InputBuffer.Reset()
			current.InputBuffer.Write(data)
		}
	}

	if isStop && current != nil {
		finishToolUse(current, toolNameMap, cb)
		return nil
	}
	return current
}

func finishToolUse(state *toolUseState, toolNameMap map[string]string, cb *StreamCallback) {
	if cb.OnToolUse == nil {
		return
	}
	var input map[string]any
	if state.InputBuffer.Len() > 0 {
		_ = json.Unmarshal([]byte(state.InputBuffer.String()), &input)
	}
	if input == nil {
		input = map[string]any{}
	}
	// Restore original tool name if it was sanitized on the way out.
	name := state.Name
	if original, ok := toolNameMap[name]; ok {
		name = original
	}
	cb.OnToolUse(ToolUse{
		ToolUseID: state.ToolUseID,
		Name:      name,
		Input:     input,
	})
}

// updateTokensFromEvent walks the event payload looking for a usage map
// and returns the most-current input/output token counts. Handles both
// camelCase and snake_case shapes plus the split uncached / cacheRead /
// cacheWrite variant Kiro sometimes returns.
func updateTokensFromEvent(event map[string]any, currentInput, currentOutput int) (int, int) {
	return updateTokensInternal(event, currentInput, currentOutput)
}

// UpdateTokensFromEvent is the exported form used by the service-layer
// gateway to tap into running token totals without re-implementing the
// extraction logic.
func UpdateTokensFromEvent(event map[string]any, currentInput, currentOutput int) (int, int) {
	return updateTokensInternal(event, currentInput, currentOutput)
}

func updateTokensInternal(event map[string]any, currentInput, currentOutput int) (int, int) {
	candidates := []map[string]any{event}
	collectUsageMaps(event, &candidates)

	inputTokens := currentInput
	outputTokens := currentOutput

	for _, usage := range candidates {
		if usage == nil {
			continue
		}
		if v, ok := readTokenNumber(usage,
			"outputTokens", "completionTokens", "totalOutputTokens",
			"output_tokens", "completion_tokens", "total_output_tokens",
		); ok {
			outputTokens = v
		}
		if v, ok := readTokenNumber(usage,
			"inputTokens", "promptTokens", "totalInputTokens",
			"input_tokens", "prompt_tokens", "total_input_tokens",
		); ok {
			inputTokens = v
			continue
		}
		uncached, _ := readTokenNumber(usage, "uncachedInputTokens", "uncached_input_tokens")
		cacheRead, _ := readTokenNumber(usage, "cacheReadInputTokens", "cache_read_input_tokens")
		cacheWrite, _ := readTokenNumber(usage,
			"cacheWriteInputTokens", "cache_write_input_tokens",
			"cacheCreationInputTokens", "cache_creation_input_tokens")
		if uncached+cacheRead+cacheWrite > 0 {
			inputTokens = uncached + cacheRead + cacheWrite
			continue
		}
		total, ok := readTokenNumber(usage, "totalTokens", "total_tokens")
		if ok && total > 0 {
			candidateOutput := outputTokens
			if v, vok := readTokenNumber(usage,
				"outputTokens", "completionTokens", "totalOutputTokens",
				"output_tokens", "completion_tokens", "total_output_tokens",
			); vok {
				candidateOutput = v
			}
			if total-candidateOutput > 0 {
				inputTokens = total - candidateOutput
			}
		}
	}
	return inputTokens, outputTokens
}

// collectUsageMaps walks the event tree and gathers every "usage" /
// "tokenUsage" / "token_usage" sub-map into out. Kiro nests usage data
// at varying depths, so a generic walk is the simplest correct option.
func collectUsageMaps(v any, out *[]map[string]any) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			lk := strings.ToLower(k)
			if lk == "usage" || lk == "tokenusage" || lk == "token_usage" {
				if m, ok := child.(map[string]any); ok {
					*out = append(*out, m)
				}
			}
			collectUsageMaps(child, out)
		}
	case []any:
		for _, child := range t {
			collectUsageMaps(child, out)
		}
	}
}

func readTokenNumber(m map[string]any, keys ...string) (int, bool) {
	for _, k := range keys {
		v, ok := m[k]
		if !ok {
			continue
		}
		switch n := v.(type) {
		case float64:
			return int(n), true
		case int:
			return n, true
		case int64:
			return int(n), true
		case json.Number:
			if parsed, err := n.Int64(); err == nil {
				return int(parsed), true
			}
		case string:
			var parsed int
			if _, err := fmtSscanInt(n, &parsed); err == nil {
				return parsed, true
			}
		}
	}
	return 0, false
}

// fmtSscanInt is fmt.Sscanf("%d") in disguise — kept tiny to avoid the
// extra import for one line.
func fmtSscanInt(s string, out *int) (int, error) {
	var n int
	consumed := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			if i == 0 {
				return 0, errInvalidInt
			}
			break
		}
		n = n*10 + int(c-'0')
		consumed++
	}
	if consumed == 0 {
		return 0, errInvalidInt
	}
	*out = n
	return consumed, nil
}

type kiroErr string

func (e kiroErr) Error() string { return string(e) }

const errInvalidInt = kiroErr("not an integer")
