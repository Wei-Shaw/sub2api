package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

// openaiUsage mirrors the token usage fields from OpenAI API responses.
// Kept minimal — only the fields needed for billing.
type openaiUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	ImageOutputTokens        int `json:"image_output_tokens,omitempty"`
}

// sendResponseHeaders sends the initial GatewayResponseHeaders chunk
// with the upstream HTTP status code and response headers.
func sendResponseHeaders(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
) error {
	headers := make(map[string]string)
	for _, key := range responseHeaderWhitelist {
		if v := resp.Header.Get(key); v != "" {
			headers[key] = v
		}
	}
	// Always propagate Content-Type for downstream rendering
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		headers["Content-Type"] = ct
	}

	return stream.Send(&pb.GatewayForwardChunk{
		Chunk: &pb.GatewayForwardChunk_Headers{
			Headers: &pb.GatewayResponseHeaders{
				StatusCode: int32(resp.StatusCode),
				Headers:    headers,
			},
		},
	})
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
// each line as a GatewayResponseBody chunk. It also extracts usage data
// from the terminal event for billing.
//
// Returns the extracted usage, first-token latency (ms), and any error.
func proxySSEStream(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
	startTime time.Time,
) (*openaiUsage, int32, error) {
	scanner := bufio.NewScanner(resp.Body)
	// Allow up to 1 MB per SSE line (image responses can be large)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1<<20)

	var usage *openaiUsage
	var firstTokenMs int32
	firstTokenRecorded := false

	for scanner.Scan() {
		line := scanner.Text()

		// Forward every line (including empty lines as SSE separators)
		chunk := line + "\n"
		if err := sendBodyChunk(stream, []byte(chunk)); err != nil {
			return usage, firstTokenMs, fmt.Errorf("send body: %w", err)
		}

		// Extract data from SSE data lines
		data, isData := extractSSEData(line)
		if !isData {
			continue
		}

		// Record first-token latency on the first content delta
		if !firstTokenRecorded && isContentDelta(data) {
			firstTokenMs = int32(time.Since(startTime).Milliseconds())
			firstTokenRecorded = true
		}

		// Extract usage from the terminal event
		if u := extractUsageFromSSEData(data); u != nil {
			usage = u
		}
	}

	if err := scanner.Err(); err != nil {
		return usage, firstTokenMs, fmt.Errorf("scan stream: %w", err)
	}

	return usage, firstTokenMs, nil
}

// proxyFullResponse reads the entire non-streaming response body,
// forwards it as a single body chunk, and extracts usage.
func proxyFullResponse(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
) (*openaiUsage, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if err := sendBodyChunk(stream, body); err != nil {
		return nil, fmt.Errorf("send body: %w", err)
	}

	usage := extractUsageFromJSON(body)
	return usage, nil
}

// proxyErrorResponse reads an error response body and forwards it.
// Usage is not expected in error responses.
func proxyErrorResponse(
	stream grpc.ServerStreamingServer[pb.GatewayForwardChunk],
	resp *http.Response,
) (*openaiUsage, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
	if err != nil {
		return nil, fmt.Errorf("read error body: %w", err)
	}

	if err := sendBodyChunk(stream, body); err != nil {
		return nil, fmt.Errorf("send error body: %w", err)
	}

	return nil, nil
}

// sendBodyChunk sends a single GatewayResponseBody chunk to the stream.
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
func isContentDelta(data string) bool {
	// Fast check: look for output_text.delta without full JSON parse
	return strings.Contains(data, `"response.output_text.delta"`) ||
		strings.Contains(data, `"type":"response.output_text.delta"`)
}

// extractUsageFromSSEData extracts usage from a terminal SSE event
// (response.completed or response.done). Returns nil if the event
// does not contain usage data.
func extractUsageFromSSEData(data string) *openaiUsage {
	// Only parse terminal events that contain usage
	if !strings.Contains(data, `"usage"`) {
		return nil
	}
	// Must be a terminal event type
	if !strings.Contains(data, `"response.completed"`) &&
		!strings.Contains(data, `"response.done"`) {
		return nil
	}

	return extractUsageFromTerminalEvent([]byte(data))
}

// extractUsageFromJSON extracts usage from a full JSON response body.
func extractUsageFromJSON(body []byte) *openaiUsage {
	var envelope struct {
		Usage *openaiUsage `json:"usage"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil
	}
	return envelope.Usage
}

// extractUsageFromTerminalEvent parses usage from a terminal SSE event.
// The event structure is: {"type":"response.completed","response":{...,"usage":{...}}}
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

	// Fallback: try top-level usage (some upstream variants)
	return extractUsageFromJSON(data)
}
