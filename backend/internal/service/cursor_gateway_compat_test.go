package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCursorRunOptsFromAnthropicMapsEffortAndThinking(t *testing.T) {
	enabled := true
	opts := cursorRunOptsFromAnthropic(&apicompat.AnthropicRequest{
		OutputConfig: &apicompat.AnthropicOutputConfig{Effort: "max"},
		Thinking:     &apicompat.AnthropicThinking{Type: "enabled"},
	})
	require.Equal(t, "max", opts.Effort)
	require.NotNil(t, opts.Thinking)
	require.Equal(t, enabled, *opts.Thinking)

	upstream, warnings := resolveCursorChatModel("claude-opus-5", opts)
	require.Equal(t, "claude-opus-5-thinking-max", upstream)
	require.Equal(t, "model_variant", warnings[0]["code"])
}

func TestCursorRunOptsFromAnthropicDoesNotForceMedium(t *testing.T) {
	opts := cursorRunOptsFromAnthropic(&apicompat.AnthropicRequest{})
	require.Empty(t, opts.Effort)
	require.Nil(t, opts.Thinking)

	upstream, _ := resolveCursorChatModel("claude-opus-5", opts)
	require.Equal(t, "claude-opus-5-medium", upstream)
}

func TestCursorRunOptsFromResponsesMapsEffort(t *testing.T) {
	opts := cursorRunOptsFromResponses(&apicompat.ResponsesRequest{
		Reasoning: &apicompat.ResponsesReasoning{Effort: "high"},
	})
	require.Equal(t, "high", opts.Effort)

	upstream, _ := resolveCursorChatModel("grok-4.6", opts)
	require.Equal(t, "cursor-grok-4.6-high", upstream)
}

func TestCursorMessagesFromChatFlattensTextParts(t *testing.T) {
	parts, err := json.Marshal([]map[string]string{
		{"type": "text", "text": "hello "},
		{"type": "text", "text": "world"},
	})
	require.NoError(t, err)

	got := cursorMessagesFromChat([]apicompat.ChatMessage{
		{Role: "user", Content: parts},
		{Role: "assistant", Content: json.RawMessage(`"prior"`)},
	})
	require.Equal(t, "hello world", got[0].Content)
	require.Equal(t, "prior", got[1].Content)
}

func TestClaudeUsageFromCursorMapsCacheAndReasoning(t *testing.T) {
	got := claudeUsageFromCursor(cursor.TokenUsage{
		InputTokens:      10,
		OutputTokens:     4,
		CacheReadTokens:  6,
		CacheWriteTokens: 2,
		ReasoningTokens:  3,
	})
	require.Equal(t, 10, got.InputTokens)
	require.Equal(t, 4, got.OutputTokens)
	require.Equal(t, 6, got.CacheReadInputTokens)
	require.Equal(t, 2, got.CacheCreationInputTokens)

	got = claudeUsageFromCursor(cursor.TokenUsage{ReasoningTokens: 8})
	require.Equal(t, 8, got.OutputTokens)
}

func TestChatUsageFromCursorUsesGrossPromptTokens(t *testing.T) {
	got := chatUsageFromCursor(cursor.TokenUsage{
		InputTokens:      10,
		OutputTokens:     4,
		CacheReadTokens:  6,
		CacheWriteTokens: 2,
		ReasoningTokens:  3,
	})
	require.NotNil(t, got)
	require.Equal(t, 18, got.PromptTokens)
	require.Equal(t, 4, got.CompletionTokens)
	require.Equal(t, 22, got.TotalTokens)
	require.Equal(t, 6, got.PromptTokensDetails.CachedTokens)
	require.Equal(t, 2, got.PromptTokensDetails.CacheWriteTokens)
	require.Equal(t, 3, got.CompletionTokensDetails.ReasoningTokens)
	require.Nil(t, chatUsageFromCursor(cursor.TokenUsage{}))
}

func TestCursorNonStreamResponseRecordsTurnEndedUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	frames := encodeCursorAssistantFrames(t, "Hi", 12, 7)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	result, err := NewCursorGatewayService(nil, nil).nonStreamResponse(c, bytes.NewReader(frames), "grok-4.6", nil, time.Now())
	require.NoError(t, err)
	require.Equal(t, 12, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage *apicompat.ChatUsage `json:"usage"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &parsed))
	require.Equal(t, "Hi", parsed.Choices[0].Message.Content)
	require.Equal(t, 12, parsed.Usage.PromptTokens)
	require.Equal(t, 7, parsed.Usage.CompletionTokens)
}

func TestCursorNonStreamResponseIncludesReasoningContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	frames := encodeCursorThinkingAndTextFrames(t, "plan first", "Hi", 3, 2)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	_, err := NewCursorGatewayService(nil, nil).nonStreamResponse(c, bytes.NewReader(frames), "claude-opus-5", nil, time.Now())
	require.NoError(t, err)

	var parsed struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &parsed))
	require.Equal(t, "Hi", parsed.Choices[0].Message.Content)
	require.Equal(t, "plan first", parsed.Choices[0].Message.ReasoningContent)
}

func TestCursorStreamResponseForwardsThinking(t *testing.T) {
	gin.SetMode(gin.TestMode)
	frames := encodeCursorThinkingAndTextFrames(t, "plan first", "Hi", 3, 2)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	_, err := NewCursorGatewayService(nil, nil).streamResponse(c, bytes.NewReader(frames), "claude-opus-5", nil, time.Now(), false)
	require.NoError(t, err)

	body := rec.Body.String()
	require.Contains(t, body, `"reasoning_content":"plan first"`)
	require.Contains(t, body, `"content":"Hi"`)
}

func encodeCursorAssistantFrames(t *testing.T, text string, inputTokens, outputTokens int) []byte {
	t.Helper()
	return encodeCursorThinkingAndTextFrames(t, "", text, inputTokens, outputTokens)
}

func encodeCursorThinkingAndTextFrames(t *testing.T, thinking, text string, inputTokens, outputTokens int) []byte {
	t.Helper()
	var out []byte
	if thinking != "" {
		var delta cursor.ProtobufWriter
		delta.String(1, thinking)
		var update cursor.ProtobufWriter
		update.Bytes(4, delta.Result())
		var server cursor.ProtobufWriter
		server.Bytes(1, update.Result())
		frame, err := cursor.EncodeFrame(server.Result(), false)
		require.NoError(t, err)
		out = append(out, frame...)
	}
	if text != "" {
		var delta cursor.ProtobufWriter
		delta.String(1, text)
		var textUpdate cursor.ProtobufWriter
		textUpdate.Bytes(1, delta.Result())
		var textServer cursor.ProtobufWriter
		textServer.Bytes(1, textUpdate.Result())
		textFrame, err := cursor.EncodeFrame(textServer.Result(), false)
		require.NoError(t, err)
		out = append(out, textFrame...)
	}

	var ended cursor.ProtobufWriter
	ended.Varint(1, inputTokens)
	ended.Varint(2, outputTokens)
	var endUpdate cursor.ProtobufWriter
	endUpdate.Bytes(14, ended.Result())
	var endServer cursor.ProtobufWriter
	endServer.Bytes(1, endUpdate.Result())
	endFrame, err := cursor.EncodeFrame(endServer.Result(), false)
	require.NoError(t, err)
	return append(out, endFrame...)
}
