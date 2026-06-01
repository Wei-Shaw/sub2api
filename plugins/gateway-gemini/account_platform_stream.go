package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
	"io"
	"strings"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

// --- test payload ---

// createGeminiTestPayload creates a minimal test payload for the Gemini API.
func createGeminiTestPayload(model string) []byte {
	if isImageGenerationModel(model) {
		return buildImagePayload()
	}
	return buildTextPayload()
}

func buildTextPayload() []byte {
	payload := map[string]any{
		"contents": []map[string]any{
			{
				"role": "user",
				"parts": []map[string]any{
					{"text": defaultTextTestPrompt},
				},
			},
		},
		"systemInstruction": map[string]any{
			"parts": []map[string]any{
				{"text": "You are a helpful AI assistant."},
			},
		},
	}
	b, _ := json.Marshal(payload)
	return b
}

func buildImagePayload() []byte {
	payload := map[string]any{
		"contents": []map[string]any{
			{
				"role": "user",
				"parts": []map[string]any{
					{"text": defaultImageTestPrompt},
				},
			},
		},
		"generationConfig": map[string]any{
			"responseModalities": []string{"TEXT", "IMAGE"},
			"imageConfig": map[string]any{
				"aspectRatio": "1:1",
			},
		},
	}
	b, _ := json.Marshal(payload)
	return b
}

// isImageGenerationModel checks if the model ID refers to an image generation model.
func isImageGenerationModel(model string) bool {
	m := strings.ToLower(strings.TrimPrefix(model, "models/"))
	imageModels := []string{
		"gemini-3.1-flash-image",
		"gemini-3.1-flash-image-preview",
		"gemini-3-pro-image",
		"gemini-3-pro-image-preview",
		"gemini-2.5-flash-image",
		"gemini-2.5-flash-image-preview",
	}
	for _, prefix := range imageModels {
		if m == prefix || strings.HasPrefix(m, prefix+"-") {
			return true
		}
	}
	return false
}

// --- SSE stream processing ---

// processGeminiStream reads the SSE stream from the Gemini API and
// forwards events through the gRPC stream.
func processGeminiStream(
	stream grpc.ServerStreamingServer[pb.TestConnectionEvent],
	body io.Reader,
) error {
	reader := bufio.NewReader(body)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return sendSuccessEnd(stream)
			}
			return gatewayutil.SendErrorEnd(stream, "stream read error: "+err.Error())
		}

		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}

		jsonStr := strings.TrimPrefix(line, "data: ")
		if jsonStr == "[DONE]" {
			return sendSuccessEnd(stream)
		}

		var data map[string]any
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
		}

		if handleGeminiSSEEvent(stream, data) {
			return nil
		}
	}
}

// handleGeminiSSEEvent processes a single parsed SSE event. Returns true
// if the stream should be terminated.
func handleGeminiSSEEvent(
	stream grpc.ServerStreamingServer[pb.TestConnectionEvent],
	data map[string]any,
) bool {
	// Gemini CLI wraps the response in a "response" field.
	if resp, ok := data["response"].(map[string]any); ok && resp != nil {
		data = resp
	}

	// Extract candidates.
	candidates, ok := data["candidates"].([]any)
	if ok && len(candidates) > 0 {
		return handleCandidate(stream, candidates[0])
	}

	// Handle error responses.
	if errData, ok := data["error"].(map[string]any); ok {
		msg := "unknown error"
		if m, ok := errData["message"].(string); ok && m != "" {
			msg = m
		}
		_ = gatewayutil.SendErrorEnd(stream, msg)
		return true
	}

	return false
}

// handleCandidate extracts text/image from a single candidate and sends
// events. Returns true if the stream should be terminated.
func handleCandidate(
	stream grpc.ServerStreamingServer[pb.TestConnectionEvent],
	rawCandidate any,
) bool {
	candidate, ok := rawCandidate.(map[string]any)
	if !ok {
		return false
	}

	// Extract content parts.
	if content, ok := candidate["content"].(map[string]any); ok {
		if parts, ok := content["parts"].([]any); ok {
			sendCandidateParts(stream, parts)
		}
	}

	// Check for completion.
	if reason, ok := candidate["finishReason"].(string); ok && reason != "" {
		_ = sendSuccessEnd(stream)
		return true
	}
	return false
}

// sendCandidateParts iterates parts and sends text/image events.
func sendCandidateParts(
	stream grpc.ServerStreamingServer[pb.TestConnectionEvent],
	parts []any,
) {
	for _, part := range parts {
		partMap, ok := part.(map[string]any)
		if !ok {
			continue
		}
		if text, ok := partMap["text"].(string); ok && text != "" {
			_ = stream.Send(&pb.TestConnectionEvent{Type: "content", Text: text})
		}
		if inlineData, ok := partMap["inlineData"].(map[string]any); ok {
			mime, _ := inlineData["mimeType"].(string)
			b64, _ := inlineData["data"].(string)
			if strings.HasPrefix(strings.ToLower(mime), "image/") && b64 != "" {
				imageURL := fmt.Sprintf("data:%s;base64,%s", mime, b64)
				_ = stream.Send(&pb.TestConnectionEvent{
					Type: "content",
					Text: imageURL,
				})
			}
		}
	}
}

// --- error helpers ---

func sendSuccessEnd(stream grpc.ServerStreamingServer[pb.TestConnectionEvent]) error {
	_ = stream.Send(&pb.TestConnectionEvent{Type: "test_end", Success: true})
	return nil
}
