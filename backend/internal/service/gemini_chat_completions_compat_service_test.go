package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGeminiResponseToChatCompletionsPreservesInlineData(t *testing.T) {
	tests := []struct {
		name  string
		parts []any
		want  string
	}{
		{
			name: "image only",
			parts: []any{
				map[string]any{"inlineData": map[string]any{"mimeType": "image/png", "data": "aW1hZ2U="}},
			},
			want: "![image](data:image/png;base64,aW1hZ2U=)",
		},
		{
			name: "text and image",
			parts: []any{
				map[string]any{"text": "rendered image:\n"},
				map[string]any{"inlineData": map[string]any{"mimeType": "image/webp", "data": "d2VicA=="}},
			},
			want: "rendered image:\n![image](data:image/webp;base64,d2VicA==)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			geminiResp := map[string]any{
				"candidates": []any{map[string]any{
					"content":      map[string]any{"parts": tt.parts},
					"finishReason": "STOP",
				}},
			}
			rawData, err := json.Marshal(geminiResp)
			require.NoError(t, err)

			got, _, err := geminiResponseToChatCompletions(geminiResp, "gemini-test", rawData, nil)
			require.NoError(t, err)
			require.Len(t, got.Choices, 1)

			var content string
			require.NoError(t, json.Unmarshal(got.Choices[0].Message.Content, &content))
			require.Equal(t, tt.want, content)
			require.Equal(t, "stop", got.Choices[0].FinishReason)
		})
	}
}

func TestGeminiResponseToChatCompletionsOmitsInvalidInlineData(t *testing.T) {
	tests := []struct {
		name       string
		inlineData map[string]any
	}{
		{
			name:       "unsupported MIME type",
			inlineData: map[string]any{"mimeType": "image/svg+xml", "data": "PHN2Zz48L3N2Zz4="},
		},
		{
			name:       "malformed MIME type",
			inlineData: map[string]any{"mimeType": "image/png; charset=utf-8", "data": "aW1hZ2U="},
		},
		{
			name:       "malformed base64",
			inlineData: map[string]any{"mimeType": "image/png", "data": "not-valid-base64!!!"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			geminiResp := map[string]any{
				"candidates": []any{map[string]any{
					"content":      map[string]any{"parts": []any{map[string]any{"text": "before"}, map[string]any{"inlineData": tt.inlineData}, map[string]any{"text": "after"}}},
					"finishReason": "STOP",
				}},
			}
			rawData, err := json.Marshal(geminiResp)
			require.NoError(t, err)

			got, _, err := geminiResponseToChatCompletions(geminiResp, "gemini-test", rawData, nil)
			require.NoError(t, err)

			var content string
			require.NoError(t, json.Unmarshal(got.Choices[0].Message.Content, &content))
			require.Equal(t, "beforeafter", content)
		})
	}
}

func TestConvertGeminiToClaudeMessageOmitsInlineDataForAnthropicMessages(t *testing.T) {
	geminiResp := map[string]any{
		"candidates": []any{map[string]any{
			"content": map[string]any{"parts": []any{
				map[string]any{"text": "before"},
				map[string]any{"inlineData": map[string]any{"mimeType": "image/png", "data": "aW1hZ2U="}},
				map[string]any{"functionCall": map[string]any{"name": "get_weather", "args": map[string]any{"city": "Paris"}}},
				map[string]any{"text": "after"},
			}},
			"finishReason": "STOP",
		}},
	}
	rawData, err := json.Marshal(geminiResp)
	require.NoError(t, err)

	withInlineData, _ := convertGeminiToClaudeMessage(geminiResp, "gemini-test", rawData, true)
	contentWithInlineData, ok := withInlineData["content"].([]any)
	require.True(t, ok)
	require.Len(t, contentWithInlineData, 4)
	require.Equal(t, map[string]any{"type": "text", "text": "before"}, contentWithInlineData[0])
	require.Equal(t, map[string]any{"type": "text", "text": "![image](data:image/png;base64,aW1hZ2U=)"}, contentWithInlineData[1])
	toolUse, ok := contentWithInlineData[2].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "tool_use", toolUse["type"])
	require.Equal(t, "get_weather", toolUse["name"])
	require.Equal(t, map[string]any{"type": "text", "text": "after"}, contentWithInlineData[3])

	withoutInlineData, _ := convertGeminiToClaudeMessage(geminiResp, "gemini-test", rawData, false)
	contentWithoutInlineData, ok := withoutInlineData["content"].([]any)
	require.True(t, ok)
	require.Len(t, contentWithoutInlineData, 3)
	require.Equal(t, map[string]any{"type": "text", "text": "before"}, contentWithoutInlineData[0])
	toolUseWithoutInlineData, ok := contentWithoutInlineData[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "tool_use", toolUseWithoutInlineData["type"])
	require.Equal(t, "get_weather", toolUseWithoutInlineData["name"])
	require.Equal(t, map[string]any{"type": "text", "text": "after"}, contentWithoutInlineData[2])
}

func TestGeminiResponseToChatCompletionsRetainsTextAndToolBehavior(t *testing.T) {
	geminiResp := map[string]any{
		"candidates": []any{map[string]any{
			"content": map[string]any{"parts": []any{
				map[string]any{"text": "checking"},
				map[string]any{"functionCall": map[string]any{
					"name": "get_weather",
					"args": map[string]any{"city": "Paris"},
				}},
			}},
			"finishReason": "STOP",
		}},
	}
	rawData, err := json.Marshal(geminiResp)
	require.NoError(t, err)

	got, _, err := geminiResponseToChatCompletions(geminiResp, "gemini-test", rawData, nil)
	require.NoError(t, err)
	require.Len(t, got.Choices, 1)

	choice := got.Choices[0]
	var content string
	require.NoError(t, json.Unmarshal(choice.Message.Content, &content))
	require.Equal(t, "checking", content)
	require.Equal(t, "tool_calls", choice.FinishReason)
	require.Len(t, choice.Message.ToolCalls, 1)
	require.Equal(t, "get_weather", choice.Message.ToolCalls[0].Function.Name)
	require.JSONEq(t, `{"city":"Paris"}`, choice.Message.ToolCalls[0].Function.Arguments)
}

func TestGeminiResponseToChatCompletionsOmitsThoughtParts(t *testing.T) {
	rawData := []byte(`{"candidates":[{"content":{"role":"model","parts":[` +
		`{"text":"**Solving the puzzle**\n\nThe ball costs 0.05.\n\n\n","thought":true},` +
		`{"text":"0.05","thoughtSignature":"sig_1"}]},"finishReason":"STOP","index":0}],` +
		`"usageMetadata":{"promptTokenCount":36,"candidatesTokenCount":4,"thoughtsTokenCount":217,"totalTokenCount":257}}`)
	var geminiResp map[string]any
	require.NoError(t, json.Unmarshal(rawData, &geminiResp))

	got, usage, err := geminiResponseToChatCompletions(geminiResp, "gemini-test", rawData, nil)
	require.NoError(t, err)
	require.Len(t, got.Choices, 1)

	var content string
	require.NoError(t, json.Unmarshal(got.Choices[0].Message.Content, &content))
	require.Equal(t, "0.05", content)
	require.Empty(t, got.Choices[0].Message.ReasoningContent)
	require.Equal(t, "stop", got.Choices[0].FinishReason)
	require.Equal(t, 221, usage.OutputTokens)
}

func TestGeminiResponseToChatCompletionsThoughtOnlyResponse(t *testing.T) {
	rawData := []byte(`{"candidates":[{"content":{"role":"model","parts":[` +
		`{"text":"Still reasoning about","thought":true}]},"finishReason":"MAX_TOKENS","index":0}],` +
		`"usageMetadata":{"promptTokenCount":36,"candidatesTokenCount":0,"thoughtsTokenCount":64,"totalTokenCount":100}}`)
	var geminiResp map[string]any
	require.NoError(t, json.Unmarshal(rawData, &geminiResp))

	got, usage, err := geminiResponseToChatCompletions(geminiResp, "gemini-test", rawData, nil)
	require.NoError(t, err)
	require.Len(t, got.Choices, 1)
	require.Empty(t, got.Choices[0].Message.Content)
	require.Empty(t, got.Choices[0].Message.ReasoningContent)
	require.Equal(t, "length", got.Choices[0].FinishReason)
	require.Equal(t, 64, usage.OutputTokens)
}

func TestConvertGeminiToClaudeMessageOmitsThoughtParts(t *testing.T) {
	geminiResp := map[string]any{
		"candidates": []any{map[string]any{
			"content": map[string]any{"parts": []any{
				map[string]any{"text": "planning the call", "thought": true},
				map[string]any{"text": "checking", "thought": false},
				map[string]any{"functionCall": map[string]any{"name": "get_weather", "args": map[string]any{"city": "Paris"}}, "thoughtSignature": "sig_1"},
			}},
			"finishReason": "STOP",
		}},
	}
	rawData, err := json.Marshal(geminiResp)
	require.NoError(t, err)

	got, _ := convertGeminiToClaudeMessage(geminiResp, "gemini-test", rawData, false)
	content, ok := got["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 2)
	require.Equal(t, map[string]any{"type": "text", "text": "checking"}, content[0])
	toolUse, ok := content[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "tool_use", toolUse["type"])
	require.Equal(t, "get_weather", toolUse["name"])
	require.Equal(t, "tool_use", got["stop_reason"])
}

// TestGeminiChatCompletionsStreamingSameNameToolCalls is the chat-completions
// counterpart of TestGeminiMessagesHandleStreamingResponse_SameNameToolCalls:
// every same-name parallel call needs its own tool_calls index and id.
func TestGeminiChatCompletionsStreamingSameNameToolCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range geminiSameNameToolCallStreams {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			svc := &GeminiMessagesCompatService{}
			_, err := svc.handleChatCompletionsStreamingResponseFromGemini(c, geminiSSEResponse(tt.payloads...), time.Now(), "gemini-test", false, false)
			require.NoError(t, err)

			type toolCall struct{ id, args string }
			calls := map[int]*toolCall{}
			for _, line := range strings.Split(rec.Body.String(), "\n") {
				data, ok := strings.CutPrefix(line, "data: ")
				if !ok || data == "[DONE]" {
					continue
				}
				var chunk apicompat.ChatCompletionsChunk
				require.NoError(t, json.Unmarshal([]byte(data), &chunk))
				for _, choice := range chunk.Choices {
					for _, tc := range choice.Delta.ToolCalls {
						require.NotNil(t, tc.Index)
						call := calls[*tc.Index]
						if call == nil {
							call = &toolCall{}
							calls[*tc.Index] = call
						}
						if tc.ID != "" {
							require.Truef(t, call.id == "" || call.id == tc.ID, "tool call %d changed id from %q to %q", *tc.Index, call.id, tc.ID)
							call.id = tc.ID
						}
						call.args += tc.Function.Arguments
					}
				}
			}

			require.Len(t, calls, len(tt.wantArgs))
			ids := map[string]bool{}
			for i, want := range tt.wantArgs {
				call := calls[i]
				require.NotNilf(t, call, "missing tool call index %d", i)
				require.JSONEq(t, want, call.args, "tool call %d", i)
				require.NotEmpty(t, call.id)
				require.Falsef(t, ids[call.id], "tool call id %q reused", call.id)
				ids[call.id] = true
			}
		})
	}
}
