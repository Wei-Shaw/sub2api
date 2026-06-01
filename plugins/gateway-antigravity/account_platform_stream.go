package main

import (
	"bufio"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
	"io"
	"strings"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

// defaultUserAgentVersion is the Antigravity client version used in the
// User-Agent header. Must stay in sync with
// backend/internal/pkg/antigravity.DefaultUserAgentVersion in core.
//
// TODO(plugin-settings): once the plugin declares a SettingsSchema with
// "antigravity_user_agent_version" and the host seeds the admin-configured
// value, read it via PluginContext.Settings().Get() instead of hard-coding.
const defaultUserAgentVersion = "1.23.2"

// antigravityUserAgent is the full User-Agent string sent to Antigravity APIs.
var antigravityUserAgent = "antigravity/" + defaultUserAgentVersion + " windows/amd64"

// buildAntigravityTestPayload builds a minimal Gemini-format request for
// the v1internal streamGenerateContent endpoint. This is the native format
// for Antigravity's Code Assist API.
func buildAntigravityTestPayload(projectID, model string) map[string]any {
	payload := map[string]any{
		"model": model,
		"request": map[string]any{
			"contents": []map[string]any{
				{
					"role": "user",
					"parts": []map[string]any{
						{"text": "hi"},
					},
				},
			},
			"generationConfig": map[string]any{
				"maxOutputTokens": 16,
			},
		},
	}
	if projectID != "" {
		payload["project"] = projectID
	}
	return payload
}

// processAntigravityStream reads the SSE stream from the Antigravity
// v1internal API and forwards text events through the gRPC stream.
// Response format: Gemini SSE with candidates[].content.parts[].text.
func processAntigravityStream(
	stream grpc.ServerStreamingServer[pb.TestConnectionEvent],
	body io.Reader,
) error {
	return processSSEStream(stream, body, extractGeminiText)
}

// processClaudeStream reads the SSE stream from the Anthropic Messages
// API and forwards content events through the gRPC stream.
func processClaudeStream(
	stream grpc.ServerStreamingServer[pb.TestConnectionEvent],
	body io.Reader,
) error {
	return processSSEStream(stream, body, extractClaudeText)
}

// processGeminiStream reads the SSE stream from the Gemini generateContent
// API and forwards text events through the gRPC stream.
func processGeminiStream(
	stream grpc.ServerStreamingServer[pb.TestConnectionEvent],
	body io.Reader,
) error {
	return processSSEStream(stream, body, extractGeminiText)
}

// textExtractor extracts text from a parsed SSE JSON event.
// Returns (text, done) where done=true means the stream should end.
type textExtractor func(data map[string]any) (string, bool)

// processSSEStream is the generic SSE reader that dispatches to an
// extractor function for protocol-specific text extraction.
func processSSEStream(
	stream grpc.ServerStreamingServer[pb.TestConnectionEvent],
	body io.Reader,
	extract textExtractor,
) error {
	reader := bufio.NewReader(body)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				_ = stream.Send(&pb.TestConnectionEvent{
					Type: "test_end", Success: true,
				})
				return nil
			}
			return gatewayutil.SendErrorEnd(stream, "stream read error: "+err.Error())
		}

		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}

		jsonStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if jsonStr == "[DONE]" {
			_ = stream.Send(&pb.TestConnectionEvent{
				Type: "test_end", Success: true,
			})
			return nil
		}

		var data map[string]any
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
		}

		text, done := extract(data)
		if text != "" {
			_ = stream.Send(&pb.TestConnectionEvent{
				Type: "content", Text: text,
			})
		}
		if done {
			_ = stream.Send(&pb.TestConnectionEvent{
				Type: "test_end", Success: true,
			})
			return nil
		}
	}
}

// extractGeminiText extracts text from a Gemini SSE event.
// Path: candidates[0].content.parts[].text
func extractGeminiText(data map[string]any) (string, bool) {
	candidates, ok := data["candidates"].([]any)
	if !ok || len(candidates) == 0 {
		return "", false
	}
	candidate, ok := candidates[0].(map[string]any)
	if !ok {
		return "", false
	}

	// Check finishReason
	if reason, ok := candidate["finishReason"].(string); ok && reason != "" {
		text := extractPartsText(candidate)
		return text, true
	}

	return extractPartsText(candidate), false
}

// extractPartsText extracts text from candidate.content.parts[].text.
func extractPartsText(candidate map[string]any) string {
	content, ok := candidate["content"].(map[string]any)
	if !ok {
		return ""
	}
	parts, ok := content["parts"].([]any)
	if !ok {
		return ""
	}
	var texts []string
	for _, p := range parts {
		part, ok := p.(map[string]any)
		if !ok {
			continue
		}
		if t, ok := part["text"].(string); ok && t != "" {
			texts = append(texts, t)
		}
	}
	return strings.Join(texts, "")
}

// extractClaudeText extracts text from an Anthropic Messages SSE event.
// Handles content_block_delta (type=text_delta) and message_stop events.
func extractClaudeText(data map[string]any) (string, bool) {
	eventType, _ := data["type"].(string)

	switch eventType {
	case "content_block_delta":
		delta, ok := data["delta"].(map[string]any)
		if !ok {
			return "", false
		}
		if text, ok := delta["text"].(string); ok {
			return text, false
		}

	case "message_stop":
		return "", true

	case "error":
		return "", true
	}

	return "", false
}
