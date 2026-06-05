package main

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"time"
)

// extractedUsage holds token counts extracted from upstream responses.
type extractedUsage struct {
	inputTokens         int64
	outputTokens        int64
	cacheCreationTokens int64
	cacheReadTokens     int64
}

// usageExtractor defines how to pull usage data from upstream responses.
// Each upstream format (Claude SSE, Gemini SSE) has its own implementation.
type usageExtractor interface {
	// extractFromSSELine extracts usage from a single SSE data line.
	// Returns non-nil usage when the line contains usage info.
	extractFromSSELine(data []byte) *extractedUsage

	// extractFromBody extracts usage from a complete (non-streaming) response body.
	extractFromBody(body []byte) *extractedUsage

	// isContentEvent returns true if the SSE line carries content
	// (used for first-token-time tracking).
	isContentEvent(data []byte) bool
}

// sseTracker reads an SSE stream, forwarding raw bytes to a callback
// while accumulating usage data.
type sseTracker struct {
	extractor    usageExtractor
	accumulated  extractedUsage
	startTime    time.Time
	firstTokenAt *time.Time
	hasContent   bool
}

func newSSETracker(extractor usageExtractor, startTime time.Time) *sseTracker {
	return &sseTracker{
		extractor: extractor,
		startTime: startTime,
	}
}

// streamAndTrack reads an SSE body line-by-line. Each line (including
// newlines) is forwarded via the send callback. Usage events are parsed
// in parallel to accumulate token counts.
func (t *sseTracker) streamAndTrack(
	body io.Reader,
	send func(data []byte) error,
) error {
	scanner := bufio.NewScanner(body)
	// Allow large SSE lines (up to 1 MB).
	scanner.Buffer(make([]byte, 64*1024), 1<<20)

	for scanner.Scan() {
		line := scanner.Text()
		// Forward the raw SSE line (with trailing newline) to the caller.
		if err := send([]byte(line + "\n")); err != nil {
			return err
		}

		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "data:") {
			continue
		}
		jsonStr := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
		if jsonStr == "[DONE]" {
			continue
		}

		rawData := []byte(jsonStr)

		// Track first content token time.
		if !t.hasContent && t.extractor.isContentEvent(rawData) {
			t.hasContent = true
			now := time.Now()
			t.firstTokenAt = &now
		}

		// Accumulate usage.
		if u := t.extractor.extractFromSSELine(rawData); u != nil {
			t.mergeUsage(u)
		}
	}
	return scanner.Err()
}

func (t *sseTracker) mergeUsage(u *extractedUsage) {
	if u.inputTokens > t.accumulated.inputTokens {
		t.accumulated.inputTokens = u.inputTokens
	}
	if u.outputTokens > t.accumulated.outputTokens {
		t.accumulated.outputTokens = u.outputTokens
	}
	if u.cacheCreationTokens > t.accumulated.cacheCreationTokens {
		t.accumulated.cacheCreationTokens = u.cacheCreationTokens
	}
	if u.cacheReadTokens > t.accumulated.cacheReadTokens {
		t.accumulated.cacheReadTokens = u.cacheReadTokens
	}
}

func (t *sseTracker) usage() *extractedUsage {
	return &t.accumulated
}

func (t *sseTracker) firstTokenTime() *time.Time {
	return t.firstTokenAt
}

// --- Claude usage extractor ---

// claudeUsageExtractor extracts usage from Anthropic Messages API SSE events.
// Usage arrives in "message_delta" events with usage.output_tokens, and in
// "message_start" events with message.usage.input_tokens.
type claudeUsageExtractor struct{}

func (e *claudeUsageExtractor) extractFromSSELine(data []byte) *extractedUsage {
	var event map[string]any
	if err := json.Unmarshal(data, &event); err != nil {
		return nil
	}

	eventType, _ := event["type"].(string)
	u := &extractedUsage{}
	found := false

	switch eventType {
	case "message_start":
		found = e.extractMessageStartUsage(event, u)
	case "message_delta":
		found = e.extractMessageDeltaUsage(event, u)
	}

	if found {
		return u
	}
	return nil
}

func (e *claudeUsageExtractor) extractMessageStartUsage(event map[string]any, u *extractedUsage) bool {
	msg, ok := event["message"].(map[string]any)
	if !ok {
		return false
	}
	usage, ok := msg["usage"].(map[string]any)
	if !ok {
		return false
	}
	u.inputTokens = jsonInt64(usage, "input_tokens")
	u.cacheCreationTokens = jsonInt64(usage, "cache_creation_input_tokens")
	u.cacheReadTokens = jsonInt64(usage, "cache_read_input_tokens")
	return true
}

func (e *claudeUsageExtractor) extractMessageDeltaUsage(event map[string]any, u *extractedUsage) bool {
	usage, ok := event["usage"].(map[string]any)
	if !ok {
		return false
	}
	u.outputTokens = jsonInt64(usage, "output_tokens")
	return true
}

func (e *claudeUsageExtractor) extractFromBody(body []byte) *extractedUsage {
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil
	}
	usage, ok := resp["usage"].(map[string]any)
	if !ok {
		return nil
	}
	return &extractedUsage{
		inputTokens:         jsonInt64(usage, "input_tokens"),
		outputTokens:        jsonInt64(usage, "output_tokens"),
		cacheCreationTokens: jsonInt64(usage, "cache_creation_input_tokens"),
		cacheReadTokens:     jsonInt64(usage, "cache_read_input_tokens"),
	}
}

func (e *claudeUsageExtractor) isContentEvent(data []byte) bool {
	var event map[string]any
	if err := json.Unmarshal(data, &event); err != nil {
		return false
	}
	eventType, _ := event["type"].(string)
	return eventType == "content_block_delta"
}

// --- Gemini usage extractor ---

// geminiUsageExtractor extracts usage from Gemini SSE events.
// Usage arrives in the usageMetadata field of the response.
type geminiUsageExtractor struct{}

func (e *geminiUsageExtractor) extractFromSSELine(data []byte) *extractedUsage {
	var event map[string]any
	if err := json.Unmarshal(data, &event); err != nil {
		return nil
	}

	// Antigravity wraps the response in a "response" field.
	if resp, ok := event["response"].(map[string]any); ok {
		event = resp
	}

	usage, ok := event["usageMetadata"].(map[string]any)
	if !ok {
		return nil
	}
	return &extractedUsage{
		inputTokens:         jsonInt64(usage, "promptTokenCount"),
		outputTokens:        jsonInt64(usage, "candidatesTokenCount"),
		cacheCreationTokens: jsonInt64(usage, "cachedContentTokenCount"),
	}
}

func (e *geminiUsageExtractor) extractFromBody(body []byte) *extractedUsage {
	return e.extractFromSSELine(body)
}

func (e *geminiUsageExtractor) isContentEvent(data []byte) bool {
	var event map[string]any
	if err := json.Unmarshal(data, &event); err != nil {
		return false
	}
	if resp, ok := event["response"].(map[string]any); ok {
		event = resp
	}
	candidates, ok := event["candidates"].([]any)
	if !ok || len(candidates) == 0 {
		return false
	}
	candidate, ok := candidates[0].(map[string]any)
	if !ok {
		return false
	}
	content, ok := candidate["content"].(map[string]any)
	if !ok {
		return false
	}
	parts, ok := content["parts"].([]any)
	return ok && len(parts) > 0
}

// --- JSON helpers ---

// jsonInt64 extracts a numeric value as int64 from a map. Handles both
// float64 (standard JSON) and json.Number.
func jsonInt64(m map[string]any, key string) int64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	}
	return 0
}
