package main

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

// --- SSE stream processing for Forward ---

const (
	// maxSSELineSize is the maximum size of a single SSE line (1 MB).
	maxSSELineSize = 1 << 20
	// maxResponseBodySize is the limit for non-streaming response bodies.
	// Shared with sibling plugins via gatewayutil.MaxResponseBodySize.
	maxResponseBodySize = gatewayutil.MaxResponseBodySize
)

// streamResult holds the accumulated usage data from a completed SSE stream.
type streamResult struct {
	inputTokens         int64
	outputTokens        int64
	cacheCreationTokens int64
	cacheReadTokens     int64
	firstTokenMs        int32
	clientDisconnect    bool
	sawTerminalEvent    bool // true when message_stop or [DONE] was seen
}

// processSSEStream reads the upstream SSE response body line-by-line,
// extracts usage data from message_start/message_delta events, and
// streams each raw line back to the host as GatewayResponseBody chunks.
//
// When the gRPC client disconnects mid-stream, the plugin continues
// draining the upstream to capture final usage data (output_tokens in
// message_delta). This matches the core's "drain for usage" behaviour.
func processSSEStream(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	body io.Reader,
	startTime time.Time,
	originalModel, mappedModel string,
) (*streamResult, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), maxSSELineSize)

	result := &streamResult{}
	needModelReplace := originalModel != mappedModel
	firstTokenSent := false

	// SSE events can span multiple lines. We accumulate lines until
	// we hit an empty line (event boundary).
	var eventLines []string

	for scanner.Scan() {
		line := scanner.Text()

		// Empty line = end of SSE event.
		if line == "" {
			if len(eventLines) > 0 {
				processEventUsage(
					eventLines, result,
					needModelReplace, originalModel, mappedModel,
					startTime, &firstTokenSent,
				)
				if !result.clientDisconnect {
					if err := sendSSEEvent(stream, eventLines); err != nil {
						result.clientDisconnect = true
					}
				}
				eventLines = eventLines[:0]
			}
			// Send the empty line to preserve SSE framing.
			if !result.clientDisconnect {
				if err := gatewayutil.SendBodyChunk(stream, []byte("\n")); err != nil {
					result.clientDisconnect = true
				}
			}
			continue
		}

		eventLines = append(eventLines, line)
	}

	// Process any remaining lines (stream ended without trailing newline).
	if len(eventLines) > 0 {
		processEventUsage(
			eventLines, result,
			needModelReplace, originalModel, mappedModel,
			startTime, &firstTokenSent,
		)
		if !result.clientDisconnect {
			if err := sendSSEEvent(stream, eventLines); err != nil {
				result.clientDisconnect = true
			}
		}
	}

	if err := scanner.Err(); err != nil {
		if result.sawTerminalEvent {
			// Usage already captured; scanner error after terminal event is harmless.
			return result, nil
		}
		return result, err
	}
	return result, nil
}

// processEventUsage extracts usage data from an SSE event's lines and
// updates the result. It also performs model replacement on the lines
// in-place when needed. This is separated from sending so that usage
// extraction continues even after client disconnect.
func processEventUsage(
	lines []string,
	result *streamResult,
	needModelReplace bool,
	originalModel, mappedModel string,
	startTime time.Time,
	firstTokenSent *bool,
) {
	eventName, dataLine := parseSSEEventLines(lines)

	// Detect terminal events for stream completion tracking.
	if isTerminalEvent(eventName, dataLine) {
		result.sawTerminalEvent = true
	}

	// Extract usage from the parsed data.
	if dataLine == "" || dataLine == "[DONE]" {
		return
	}

	var event map[string]any
	if err := json.Unmarshal([]byte(dataLine), &event); err != nil {
		return
	}

	extractUsageFromEvent(event, result)

	// Record first token timing on content_block_delta.
	eventType, _ := event["type"].(string)
	if eventType == "content_block_delta" && !*firstTokenSent {
		*firstTokenSent = true
		result.firstTokenMs = int32(time.Since(startTime).Milliseconds())
	}

	// Replace model in response if mapping was applied.
	if needModelReplace {
		if replaced := replaceModelInEvent(event, mappedModel, originalModel); replaced {
			if newData, err := json.Marshal(event); err == nil {
				// Rebuild lines in-place with the new data.
				rebuilt := rebuildSSELines(eventName, string(newData))
				copy(lines, rebuilt)
				// Truncate if rebuilt is shorter (shouldn't happen, but be safe).
				for i := len(rebuilt); i < len(lines); i++ {
					lines[i] = ""
				}
			}
		}
	}
}

// parseSSEEventLines extracts the event name and first data line from
// a set of SSE lines.
func parseSSEEventLines(lines []string) (eventName, dataLine string) {
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if eventName == "" && strings.HasPrefix(trimmed, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
		}
		if dataLine == "" && strings.HasPrefix(trimmed, "data:") {
			dataLine = strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
		}
	}
	return eventName, dataLine
}

// isTerminalEvent returns true when the SSE event indicates the stream
// has completed (message_stop event or [DONE] sentinel). Matches the
// core's anthropicStreamEventIsTerminal logic.
func isTerminalEvent(eventName, dataLine string) bool {
	if strings.EqualFold(strings.TrimSpace(eventName), "message_stop") {
		return true
	}
	trimmed := strings.TrimSpace(dataLine)
	if trimmed == "[DONE]" {
		return true
	}
	// Check the JSON type field for message_stop.
	if trimmed != "" && trimmed[0] == '{' {
		var probe struct {
			Type string `json:"type"`
		}
		if json.Unmarshal([]byte(trimmed), &probe) == nil && probe.Type == "message_stop" {
			return true
		}
	}
	return false
}

// sendSSEEvent sends a set of SSE lines as a single body chunk,
// preserving SSE framing.
func sendSSEEvent(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	lines []string,
) error {
	var buf strings.Builder
	for _, line := range lines {
		if line == "" {
			continue
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	return gatewayutil.SendBodyChunk(stream, []byte(buf.String()))
}

// extractUsageFromEvent reads usage fields from message_start and
// message_delta events. Handles both flat usage fields and the nested
// cache_creation object (ephemeral_5m/1h), as well as the cached_tokens
// fallback for cache_read_input_tokens.
func extractUsageFromEvent(event map[string]any, result *streamResult) {
	eventType, _ := event["type"].(string)

	switch eventType {
	case "message_start":
		msg, _ := event["message"].(map[string]any)
		usage, _ := msg["usage"].(map[string]any)
		if len(usage) == 0 {
			return
		}
		extractUsageFields(usage, result, false)

	case "message_delta":
		usage, _ := event["usage"].(map[string]any)
		if len(usage) == 0 {
			return
		}
		extractUsageFields(usage, result, true)
	}
}

// extractUsageFields reads token counts from a usage map. When
// onlyPositive is true (message_delta), only non-zero values overwrite
// the result -- matching the core's "only update if v > 0" semantics.
func extractUsageFields(usage map[string]any, result *streamResult, onlyPositive bool) {
	setField := func(dest *int64, key string) {
		v, ok := usageInt(usage, key)
		if !ok {
			return
		}
		if onlyPositive && v <= 0 {
			return
		}
		*dest = v
	}

	setField(&result.inputTokens, "input_tokens")
	setField(&result.outputTokens, "output_tokens")
	setField(&result.cacheCreationTokens, "cache_creation_input_tokens")
	setField(&result.cacheReadTokens, "cache_read_input_tokens")

	// Granular cache_creation: sum ephemeral_5m + ephemeral_1h when the
	// flat cache_creation_input_tokens is absent or zero.
	applyCacheCreationGranular(usage, result, onlyPositive)

	// Fallback: cached_tokens -> cache_read_input_tokens (some upstream
	// variants report this field instead of cache_read_input_tokens).
	if result.cacheReadTokens == 0 {
		if v, ok := usageInt(usage, "cached_tokens"); ok && v > 0 {
			result.cacheReadTokens = v
		}
	}
}

// applyCacheCreationGranular sums the nested cache_creation.ephemeral_5m
// and cache_creation.ephemeral_1h fields when the flat
// cache_creation_input_tokens is zero. This matches the core's behaviour
// of computing CacheCreationInputTokens = cc5m + cc1h as a fallback.
func applyCacheCreationGranular(usage map[string]any, result *streamResult, onlyPositive bool) {
	ccObj, _ := usage["cache_creation"].(map[string]any)
	if len(ccObj) == 0 {
		return
	}
	cc5m, _ := usageInt(ccObj, "ephemeral_5m_input_tokens")
	cc1h, _ := usageInt(ccObj, "ephemeral_1h_input_tokens")
	total := cc5m + cc1h
	if total <= 0 {
		return
	}
	if result.cacheCreationTokens == 0 || (!onlyPositive) {
		result.cacheCreationTokens = total
	}
}

// usageInt extracts an integer from a JSON usage field.
func usageInt(m map[string]any, key string) (int64, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return i, true
		}
	}
	return 0, false
}

// replaceModelInEvent replaces the model field in a message_start event's
// message object. Returns true if a replacement was made.
func replaceModelInEvent(event map[string]any, from, to string) bool {
	msg, ok := event["message"].(map[string]any)
	if !ok {
		return false
	}
	model, ok := msg["model"].(string)
	if !ok || model != from {
		return false
	}
	msg["model"] = to
	return true
}

// rebuildSSELines constructs SSE lines from an event name and data.
func rebuildSSELines(eventName, dataLine string) []string {
	var lines []string
	if eventName != "" {
		lines = append(lines, "event: "+eventName)
	}
	lines = append(lines, "data: "+dataLine)
	return lines
}

// --- non-streaming response processing ---

// processNonStreamResponse reads a non-streaming response body,
// extracts usage, optionally replaces the model, and sends it as a
// single body chunk.
func processNonStreamResponse(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	body io.Reader,
	originalModel, mappedModel string,
) (*streamResult, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxResponseBodySize))
	if err != nil {
		return nil, err
	}

	result := &streamResult{}
	extractNonStreamUsage(data, result)

	// Replace model in response body if needed.
	if originalModel != mappedModel {
		data = replaceModelInResponseJSON(data, mappedModel, originalModel)
	}

	return result, gatewayutil.SendBodyChunk(stream, data)
}

// extractNonStreamUsage parses the full JSON response body and extracts
// usage fields, including granular cache_creation and cached_tokens
// fallback. Mirrors the core's parseClaudeUsageFromResponseBody.
func extractNonStreamUsage(body []byte, result *streamResult) {
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return
	}
	usage, _ := parsed["usage"].(map[string]any)
	if len(usage) == 0 {
		return
	}
	// Reuse the same extraction logic as streaming events.
	extractUsageFields(usage, result, false)
}

// replaceModelInResponseJSON replaces the top-level "model" field in a JSON
// response body.
func replaceModelInResponseJSON(body []byte, from, to string) []byte {
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(body, &parsed); err != nil {
		return body
	}
	var model string
	if err := json.Unmarshal(parsed["model"], &model); err != nil || model != from {
		return body
	}
	modelBytes, err := json.Marshal(to)
	if err != nil {
		return body
	}
	parsed["model"] = modelBytes
	result, err := json.Marshal(parsed)
	if err != nil {
		return body
	}
	return result
}
