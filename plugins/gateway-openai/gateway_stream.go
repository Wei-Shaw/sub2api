package main

import (
	"bufio"
	"bytes"
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
	sawTerminalEvent  bool // true when response.completed / [DONE] was seen
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
	"openai-processing-ms",
	"x-ratelimit-limit-requests",
	"x-ratelimit-remaining-requests",
	"x-ratelimit-limit-tokens",
	"x-ratelimit-remaining-tokens",
	"x-ratelimit-reset-requests",
	"x-ratelimit-reset-tokens",
}

// proxySSEStream reads the upstream SSE stream line by line, forwarding
// each line as a GatewayResponseBody chunk. It extracts usage data from
// terminal events for billing, and continues draining the upstream after
// client disconnect to capture final usage.
//
// Supports both OpenAI Responses API (response.completed) and Chat
// Completions API (choices[].delta) SSE formats.
func proxySSEStream(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
	startTime time.Time,
	originalModel, mappedModel string,
) (*streamResult, error) {
	scanner := bufio.NewScanner(resp.Body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxSSELineSize)

	result := &streamResult{}
	needModelReplace := originalModel != mappedModel
	firstTokenRecorded := false

	for scanner.Scan() {
		line := scanner.Text()

		// Extract data from SSE data lines for usage tracking.
		data, isData := extractSSEData(line)
		if isData {
			// Record first-token latency on the first content delta.
			if !firstTokenRecorded && isContentDelta(data) {
				result.firstTokenMs = int32(time.Since(startTime).Milliseconds())
				firstTokenRecorded = true
			}

			// Extract usage from terminal events.
			if u := extractUsageFromSSEData(data); u != nil {
				applyUsageToResult(u, result)
				result.sawTerminalEvent = true
			}

			// Replace model in response if mapping was applied.
			if needModelReplace {
				if replaced := replaceModelInSSEData(data, mappedModel, originalModel); replaced != "" {
					line = "data: " + replaced
				}
			}
		}

		// Forward every line (including empty lines as SSE separators).
		chunk := line + "\n"
		if !result.clientDisconnect {
			if err := gatewayutil.SendBodyChunk(stream, []byte(chunk)); err != nil {
				result.clientDisconnect = true
				// Continue draining to capture usage from terminal event.
			}
		}
	}

	if err := scanner.Err(); err != nil {
		if result.sawTerminalEvent {
			return result, nil
		}
		return result, err
	}
	return result, nil
}

// proxyNonStreamResponse reads the entire non-streaming response body,
// extracts usage, optionally replaces the model, and sends it as a
// single body chunk.
//
// Some OpenAI-compatible upstreams (including cascaded sub2api instances)
// may return SSE even when stream=false was requested. When the response
// Content-Type is text/event-stream, or when JSON parsing yields no usage
// and the body looks like SSE (contains "data:" or "event:"), the function
// falls back to extracting usage from SSE data lines.
func proxyNonStreamResponse(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
	originalModel, mappedModel string,
) (*streamResult, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
	if err != nil {
		return nil, err
	}

	result := &streamResult{}

	if isEventStreamContentType(resp.Header) {
		// Content-Type explicitly indicates SSE.
		extractUsageFromSSEBody(body, result)
	} else {
		extractNonStreamUsage(body, result)
		// Fallback: if JSON parsing yielded no usage and body looks like
		// SSE, try SSE extraction. This handles upstreams that omit the
		// Content-Type header while still sending SSE data.
		if result.inputTokens == 0 && result.outputTokens == 0 && bodyLooksLikeSSE(body) {
			extractUsageFromSSEBody(body, result)
		}
	}

	if originalModel != mappedModel {
		body = replaceModelInResponseJSON(body, mappedModel, originalModel)
	}

	return result, gatewayutil.SendBodyChunk(stream, body)
}

// isEventStreamContentType returns true if the response Content-Type
// indicates a text/event-stream (SSE) response.
func isEventStreamContentType(header http.Header) bool {
	ct := strings.ToLower(header.Get("Content-Type"))
	return strings.Contains(ct, "text/event-stream")
}

// bodyLooksLikeSSE returns true if the body contains SSE markers,
// suggesting the upstream returned SSE despite not being requested.
func bodyLooksLikeSSE(body []byte) bool {
	return bytes.Contains(body, []byte("data:")) || bytes.Contains(body, []byte("event:"))
}

// extractUsageFromSSEBody scans a buffered SSE body (line by line) and
// extracts usage from any terminal events found. This is used when a
// non-streaming request unexpectedly receives an SSE response.
func extractUsageFromSSEBody(body []byte, result *streamResult) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), maxSSELineSize)
	for scanner.Scan() {
		data, isData := extractSSEData(scanner.Text())
		if !isData {
			continue
		}
		if u := extractUsageFromSSEData(data); u != nil {
			applyUsageToResult(u, result)
		}
	}
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

// isContentDelta returns true if the SSE data represents a content delta
// event (first text output from the model).
//
// Covers both OpenAI Responses API format (response.output_text.delta)
// and Chat Completions format (choices[].delta.content).
func isContentDelta(data string) bool {
	// Responses API format
	if strings.Contains(data, `"response.output_text.delta"`) ||
		strings.Contains(data, `"type":"response.output_text.delta"`) {
		return true
	}
	// Chat Completions format: {"choices":[{"delta":{"content":"..."}}]}
	if strings.Contains(data, `"delta"`) && strings.Contains(data, `"content"`) {
		return true
	}
	return false
}

// --- usage extraction ---

// openaiUsage mirrors the token usage fields from OpenAI API responses.
type openaiUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	ImageOutputTokens        int `json:"image_output_tokens,omitempty"`
	// Chat Completions format fields
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	// Chat Completions cache detail
	PromptTokensDetails *promptTokensDetails `json:"prompt_tokens_details,omitempty"`
}

// promptTokensDetails holds the nested cache detail from Chat Completions.
type promptTokensDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
}

// applyUsageToResult writes the parsed openaiUsage into the streamResult.
func applyUsageToResult(u *openaiUsage, result *streamResult) {
	// Responses API fields take priority.
	if u.InputTokens > 0 {
		result.inputTokens = int64(u.InputTokens)
	}
	if u.OutputTokens > 0 {
		result.outputTokens = int64(u.OutputTokens)
	}
	if u.CacheReadInputTokens > 0 {
		result.cacheReadTokens = int64(u.CacheReadInputTokens)
	}
	if u.ImageOutputTokens > 0 {
		result.imageOutputTokens = int64(u.ImageOutputTokens)
	}
	// Fallback: Chat Completions format fields.
	if result.inputTokens == 0 && u.PromptTokens > 0 {
		result.inputTokens = int64(u.PromptTokens)
	}
	if result.outputTokens == 0 && u.CompletionTokens > 0 {
		result.outputTokens = int64(u.CompletionTokens)
	}
	if result.cacheReadTokens == 0 && u.PromptTokensDetails != nil && u.PromptTokensDetails.CachedTokens > 0 {
		result.cacheReadTokens = int64(u.PromptTokensDetails.CachedTokens)
	}
}

// extractUsageFromSSEData extracts usage from a terminal SSE event.
// Supports:
//   - Responses API: response.completed / response.done events
//   - Chat Completions: final chunk with usage field
//
// Returns nil if the event does not contain usage data.
func extractUsageFromSSEData(data string) *openaiUsage {
	if !strings.Contains(data, `"usage"`) {
		return nil
	}
	// Responses API terminal events.
	if strings.Contains(data, `"response.completed"`) ||
		strings.Contains(data, `"response.done"`) {
		return extractUsageFromTerminalEvent([]byte(data))
	}
	// Chat Completions: usage appears in a chunk with no content delta,
	// typically the last chunk before [DONE].
	return extractUsageFromJSON([]byte(data))
}

// extractUsageFromJSON extracts usage from a flat JSON object with a
// top-level "usage" field (Chat Completions format).
func extractUsageFromJSON(body []byte) *openaiUsage {
	var envelope struct {
		Usage *openaiUsage `json:"usage"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil
	}
	return envelope.Usage
}

// extractUsageFromTerminalEvent parses usage from a Responses API
// terminal SSE event. Structure:
// {"type":"response.completed","response":{...,"usage":{...}}}
func extractUsageFromTerminalEvent(data []byte) *openaiUsage {
	var event struct {
		Response struct {
			Usage *openaiUsage `json:"usage"`
		} `json:"response"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return nil
	}
	if event.Response.Usage != nil {
		return event.Response.Usage
	}
	// Fallback: try top-level usage (some upstream variants).
	return extractUsageFromJSON(data)
}

// extractNonStreamUsage parses a full JSON response body and extracts
// usage fields. Handles both Responses API and Chat Completions formats.
func extractNonStreamUsage(body []byte, result *streamResult) {
	// Try Responses API format first (nested response.usage).
	var responsesEnvelope struct {
		Usage *openaiUsage `json:"usage"`
	}
	if err := json.Unmarshal(body, &responsesEnvelope); err != nil {
		return
	}
	if responsesEnvelope.Usage != nil {
		applyUsageToResult(responsesEnvelope.Usage, result)
	}
}

// --- model replacement helpers ---

// replaceModelInSSEData replaces the model field in an SSE data JSON
// payload. Returns the modified JSON string, or "" if no replacement.
func replaceModelInSSEData(data, from, to string) string {
	if !strings.Contains(data, from) {
		return ""
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(data), &parsed); err != nil {
		return ""
	}
	replaced := false
	// Responses API: response.model
	if resp, ok := parsed["response"].(map[string]any); ok {
		if model, ok := resp["model"].(string); ok && model == from {
			resp["model"] = to
			replaced = true
		}
	}
	// Chat Completions: top-level model
	if model, ok := parsed["model"].(string); ok && model == from {
		parsed["model"] = to
		replaced = true
	}
	if !replaced {
		return ""
	}
	out, err := json.Marshal(parsed)
	if err != nil {
		return ""
	}
	return string(out)
}

// replaceModelInResponseJSON replaces the top-level "model" field in a
// non-streaming JSON response body.
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
