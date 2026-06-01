package main

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

const (
	// maxSSELineSize is the maximum size of a single SSE line (1 MB).
	maxSSELineSize = 1 << 20

	// maxResponseBodySize limits non-streaming and error response reads.
	// Shared with sibling plugins via gatewayutil.MaxResponseBodySize.
	maxResponseBodySize = gatewayutil.MaxResponseBodySize
)

// streamResult holds accumulated usage data from a completed SSE stream
// or non-streaming response.
type streamResult struct {
	inputTokens       int64
	outputTokens      int64
	cacheReadTokens   int64
	imageOutputTokens int64
	firstTokenMs      int32
	clientDisconnect  bool
	sawTerminalEvent  bool // true when finishReason or [DONE] was seen
}

// sendResponseHeaders sends the initial GatewayResponseHeaders chunk
// with the upstream HTTP status code and response headers.
func sendResponseHeaders(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
) error {
	return gatewayutil.SendResponseHeaders(stream, resp, responseHeaderWhitelist)
}

// responseHeaderWhitelist lists the upstream headers forwarded to the
// host. Only safe, non-sensitive headers are included.
var responseHeaderWhitelist = []string{
	"x-request-id",
	"x-goog-request-id",
}

// proxySSEStream reads the upstream SSE stream line by line, forwarding
// each line as a GatewayResponseBody chunk. It extracts usage data from
// usageMetadata events for billing, and continues draining the upstream
// after client disconnect to capture final usage.
//
// Gemini SSE events carry usage in the usageMetadata field which
// accumulates over the stream. The last event has the final counts.
func proxySSEStream(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
	startTime time.Time,
	originalModel, mappedModel string,
) (*streamResult, error) {
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), maxSSELineSize)

	result := &streamResult{}
	firstTokenRecorded := false

	for scanner.Scan() {
		line := scanner.Text()

		// Extract data from SSE data lines for usage tracking.
		data, isData := extractSSEData(line)
		if isData {
			processGeminiSSEData(data, result, startTime, &firstTokenRecorded)
		}

		// Forward every line (including empty lines as SSE separators).
		chunk := line + "\n"
		if !result.clientDisconnect {
			if err := gatewayutil.SendBodyChunk(stream, []byte(chunk)); err != nil {
				result.clientDisconnect = true
				// Continue draining to capture usage from final event.
			}
		}
	}

	if err := scanner.Err(); err != nil {
		if result.sawTerminalEvent {
			// Usage already captured; scanner error after terminal is harmless.
			return result, nil
		}
		return result, err
	}
	return result, nil
}

// processGeminiSSEData extracts usage and content info from a single
// SSE data payload, updating the result accordingly.
func processGeminiSSEData(
	data string,
	result *streamResult,
	startTime time.Time,
	firstTokenRecorded *bool,
) {
	var event map[string]any
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return
	}

	// Code Assist wraps the response in a "response" field.
	if resp, ok := event["response"].(map[string]any); ok {
		event = resp
	}

	// Record first-token latency on the first content delta.
	if !*firstTokenRecorded && hasContentParts(event) {
		result.firstTokenMs = int32(time.Since(startTime).Milliseconds())
		*firstTokenRecorded = true
	}

	// Extract usage from usageMetadata.
	extractGeminiUsage(event, result)

	// Detect terminal event (finishReason present).
	if isGeminiTerminalEvent(event) {
		result.sawTerminalEvent = true
	}
}

// proxyNonStreamResponse reads the entire non-streaming response body,
// extracts usage, and sends it as a single body chunk.
func proxyNonStreamResponse(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
) (*streamResult, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
	if err != nil {
		return nil, err
	}

	result := &streamResult{}
	extractNonStreamUsage(body, result)

	return result, gatewayutil.SendBodyChunk(stream, body)
}

// --- SSE parsing helpers ---

// extractSSEData extracts the JSON data string from an SSE data line.
// Supports both "data: {...}" and "data:{...}" formats.
func extractSSEData(line string) (string, bool) {
	if !strings.HasPrefix(line, "data:") {
		return "", false
	}
	data := line[len("data:"):]
	data = strings.TrimLeft(data, " \t")
	if data == "" || data == "[DONE]" {
		return "", false
	}
	return data, true
}

// hasContentParts returns true if the Gemini event contains content
// parts (text or inlineData), used for first-token-time tracking.
func hasContentParts(event map[string]any) bool {
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

// isGeminiTerminalEvent returns true when the SSE event indicates the
// stream has completed (finishReason present in candidates).
func isGeminiTerminalEvent(event map[string]any) bool {
	candidates, ok := event["candidates"].([]any)
	if !ok || len(candidates) == 0 {
		return false
	}
	candidate, ok := candidates[0].(map[string]any)
	if !ok {
		return false
	}
	reason, ok := candidate["finishReason"].(string)
	return ok && reason != ""
}

// --- usage extraction ---

// extractGeminiUsage reads token counts from a Gemini usageMetadata
// field. Matches the core's extractGeminiUsage logic:
//   - promptTokenCount includes cachedContentTokenCount, so input_tokens
//     = promptTokenCount - cachedContentTokenCount
//   - outputTokens = candidatesTokenCount + thoughtsTokenCount
//   - imageOutputTokens from candidatesTokensDetails[modality=IMAGE]
func extractGeminiUsage(event map[string]any, result *streamResult) {
	usage, ok := event["usageMetadata"].(map[string]any)
	if !ok || len(usage) == 0 {
		return
	}

	prompt := jsonInt64(usage, "promptTokenCount")
	candidates := jsonInt64(usage, "candidatesTokenCount")
	cached := jsonInt64(usage, "cachedContentTokenCount")
	thoughts := jsonInt64(usage, "thoughtsTokenCount")

	// Gemini's promptTokenCount includes cachedContentTokenCount.
	// For billing parity with core, subtract cache from input.
	result.inputTokens = prompt - cached
	result.outputTokens = candidates + thoughts
	result.cacheReadTokens = cached

	// Extract image output tokens from candidatesTokensDetails.
	extractImageOutputTokens(usage, result)
}

// extractImageOutputTokens reads image tokens from the
// candidatesTokensDetails array when the modality is IMAGE.
func extractImageOutputTokens(usage map[string]any, result *streamResult) {
	details, ok := usage["candidatesTokensDetails"].([]any)
	if !ok {
		return
	}
	for _, d := range details {
		detail, ok := d.(map[string]any)
		if !ok {
			continue
		}
		modality, _ := detail["modality"].(string)
		if modality == "IMAGE" {
			result.imageOutputTokens = jsonInt64(detail, "tokenCount")
			return
		}
	}
}

// extractNonStreamUsage parses a full JSON response body and extracts
// usage fields. Handles the Gemini response format where usageMetadata
// is at the top level.
func extractNonStreamUsage(body []byte, result *streamResult) {
	var event map[string]any
	if err := json.Unmarshal(body, &event); err != nil {
		return
	}
	// Code Assist wraps the response in a "response" field.
	if resp, ok := event["response"].(map[string]any); ok {
		event = resp
	}
	extractGeminiUsage(event, result)
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
