package main

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

// --- SSE stream processing for Forward ---

const (
	// maxSSELineSize is the maximum size of a single SSE line (1 MB).
	maxSSELineSize = 1 << 20
	// maxResponseBodySize is the limit for non-streaming response bodies.
	maxResponseBodySize = 10 << 20 // 10 MB
)

// streamResult holds the accumulated usage data from a completed SSE stream.
type streamResult struct {
	inputTokens         int64
	outputTokens        int64
	cacheCreationTokens int64
	cacheReadTokens     int64
	firstTokenMs        int32
	clientDisconnect    bool
}

// processSSEStream reads the upstream SSE response body line-by-line,
// extracts usage data from message_start/message_delta events, and
// streams each raw line back to the host as GatewayResponseBody chunks.
//
// The host is responsible for writing the SSE data to the downstream
// HTTP client; the plugin just relays bytes and extracts usage.
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
				if err := processAndSendEvent(
					stream, eventLines, result,
					needModelReplace, originalModel, mappedModel,
					startTime, &firstTokenSent,
				); err != nil {
					return result, err
				}
				eventLines = eventLines[:0]
			}
			// Send the empty line to preserve SSE framing.
			if err := sendBodyChunk(stream, []byte("\n")); err != nil {
				result.clientDisconnect = true
				return result, nil
			}
			continue
		}

		eventLines = append(eventLines, line)
	}

	// Process any remaining lines (stream ended without trailing newline).
	if len(eventLines) > 0 {
		if err := processAndSendEvent(
			stream, eventLines, result,
			needModelReplace, originalModel, mappedModel,
			startTime, &firstTokenSent,
		); err != nil {
			return result, err
		}
	}

	if err := scanner.Err(); err != nil {
		return result, err
	}
	return result, nil
}

// processAndSendEvent handles a single SSE event (one or more lines).
// It extracts usage, optionally replaces the model name, and sends the
// lines downstream.
func processAndSendEvent(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	lines []string,
	result *streamResult,
	needModelReplace bool,
	originalModel, mappedModel string,
	startTime time.Time,
	firstTokenSent *bool,
) error {
	var eventName, dataLine string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
		}
		if dataLine == "" && strings.HasPrefix(trimmed, "data:") {
			dataLine = strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
		}
	}

	// Extract usage from the parsed data.
	if dataLine != "" && dataLine != "[DONE]" {
		var event map[string]any
		if err := json.Unmarshal([]byte(dataLine), &event); err == nil {
			extractUsageFromEvent(event, result)

			// Record first token timing on content_block_delta.
			eventType, _ := event["type"].(string)
			if eventType == "content_block_delta" && !*firstTokenSent {
				*firstTokenSent = true
				ms := int32(time.Since(startTime).Milliseconds())
				result.firstTokenMs = ms
			}

			// Replace model in response if mapping was applied.
			if needModelReplace {
				if replaced := replaceModelInEvent(event, mappedModel, originalModel); replaced {
					newData, err := json.Marshal(event)
					if err == nil {
						dataLine = string(newData)
						// Rebuild lines with new data.
						lines = rebuildSSELines(eventName, dataLine)
					}
				}
			}
		}
	}

	// Send all lines as a single body chunk, preserving SSE framing.
	var buf strings.Builder
	for _, line := range lines {
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	return sendBodyChunk(stream, []byte(buf.String()))
}

// extractUsageFromEvent reads usage fields from message_start and
// message_delta events.
func extractUsageFromEvent(event map[string]any, result *streamResult) {
	eventType, _ := event["type"].(string)

	switch eventType {
	case "message_start":
		msg, _ := event["message"].(map[string]any)
		usage, _ := msg["usage"].(map[string]any)
		if len(usage) == 0 {
			return
		}
		if v, ok := usageInt(usage, "input_tokens"); ok {
			result.inputTokens = v
		}
		if v, ok := usageInt(usage, "cache_creation_input_tokens"); ok {
			result.cacheCreationTokens = v
		}
		if v, ok := usageInt(usage, "cache_read_input_tokens"); ok {
			result.cacheReadTokens = v
		}

	case "message_delta":
		usage, _ := event["usage"].(map[string]any)
		if len(usage) == 0 {
			return
		}
		if v, ok := usageInt(usage, "output_tokens"); ok && v > 0 {
			result.outputTokens = v
		}
		if v, ok := usageInt(usage, "input_tokens"); ok && v > 0 {
			result.inputTokens = v
		}
		if v, ok := usageInt(usage, "cache_creation_input_tokens"); ok && v > 0 {
			result.cacheCreationTokens = v
		}
		if v, ok := usageInt(usage, "cache_read_input_tokens"); ok && v > 0 {
			result.cacheReadTokens = v
		}
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

	// Extract usage from the response body.
	var resp struct {
		Usage struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &resp); err == nil {
		result.inputTokens = resp.Usage.InputTokens
		result.outputTokens = resp.Usage.OutputTokens
		result.cacheCreationTokens = resp.Usage.CacheCreationInputTokens
		result.cacheReadTokens = resp.Usage.CacheReadInputTokens
	}

	// Replace model in response body if needed.
	if originalModel != mappedModel {
		data = replaceModelInResponseJSON(data, mappedModel, originalModel)
	}

	return result, sendBodyChunk(stream, data)
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

// --- chunk sending helpers ---

// sendBodyChunk sends a GatewayResponseBody chunk to the gRPC stream.
func sendBodyChunk(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	data []byte,
) error {
	return stream.Send(&pb.GatewayForwardChunk{
		Chunk: &pb.GatewayForwardChunk_Body{
			Body: &pb.GatewayResponseBody{Data: data},
		},
	})
}
