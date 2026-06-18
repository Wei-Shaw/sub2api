package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/deepseek"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// forwardAsDeepSeekChatCompletions handles DeepSeek cookie-based accounts.
func (s *OpenAIGatewayService) forwardAsDeepSeekChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()

	originalModel := gjson.GetBytes(body, "model").String()
	if originalModel == "" {
		writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return nil, fmt.Errorf("missing model in request")
	}
	clientStream := gjson.GetBytes(body, "stream").Bool()

	token, _ := account.Credentials["token"].(string)
	cookie, _ := account.Credentials["cookie"].(string)
	if token == "" {
		writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", "DeepSeek token is required")
		return nil, fmt.Errorf("missing DeepSeek token in credentials")
	}

	dsClient := deepseek.NewClient(nil)

	sessionID, err := dsClient.CreateSession(ctx, token, cookie)
	if err != nil {
		logger.L().Error("deepseek: create session failed", zap.Int64("account_id", account.ID), zap.Error(err))
		writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", "Failed to create DeepSeek session")
		return nil, fmt.Errorf("deepseek create session: %w", err)
	}

	defer func() {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = dsClient.DeleteSession(bgCtx, token, cookie, sessionID)
		}()
	}()

	challenge, err := dsClient.CreatePowChallenge(ctx, token, cookie)
	if err != nil {
		logger.L().Error("deepseek: pow challenge failed", zap.Int64("account_id", account.ID), zap.Error(err))
		writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", "Failed to get DeepSeek PoW challenge")
		return nil, fmt.Errorf("deepseek pow challenge: %w", err)
	}

	powResp, err := deepseek.SolveAndBuildHeader(ctx, challenge)
	if err != nil {
		logger.L().Error("deepseek: pow solve failed", zap.Int64("account_id", account.ID), zap.Error(err))
		writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", "Failed to solve DeepSeek PoW")
		return nil, fmt.Errorf("deepseek pow solve: %w", err)
	}

	completionPayload := buildDeepSeekPayload(body, sessionID)

	resp, err := dsClient.CallCompletion(ctx, token, cookie, powResp, completionPayload)
	if err != nil {
		logger.L().Error("deepseek: completion failed", zap.Int64("account_id", account.ID), zap.Error(err))
		writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", "DeepSeek completion request failed")
		return nil, fmt.Errorf("deepseek completion: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		logger.L().Error("deepseek: completion error",
			zap.Int64("account_id", account.ID),
			zap.Int("status", resp.StatusCode),
			zap.String("body", string(errBody)),
		)
		writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", "DeepSeek completion error")
		return nil, fmt.Errorf("deepseek completion: status=%d body=%s", resp.StatusCode, string(errBody))
	}

	completionID := "chatcmpl-" + uuid.New().String()[:8]

	if clientStream {
		return s.handleDeepSeekStreamResponse(c, resp, completionID, originalModel, startTime)
	}

	return s.handleDeepSeekNonStreamResponse(c, resp, completionID, originalModel, startTime)
}

func (s *OpenAIGatewayService) handleDeepSeekStreamResponse(
	c *gin.Context,
	resp *http.Response,
	completionID, model string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	defer func() { _ = resp.Body.Close() }()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	converter := deepseek.NewStreamConverter(resp.Body, model, completionID)
	defer converter.Close()

	flusher, canFlush := c.Writer.(http.Flusher)

	for {
		event, done, err := converter.NextEvent()
		if err != nil {
			logger.L().Error("deepseek: stream read error", zap.Error(err))
			break
		}

		if _, writeErr := c.Writer.WriteString(event); writeErr != nil {
			break
		}
		if canFlush {
			flusher.Flush()
		}

		if done {
			break
		}
	}

	return &OpenAIForwardResult{
		RequestID: completionID,
		Stream:    true,
		Duration:  time.Since(startTime),
	}, nil
}

// handleDeepSeekNonStreamResponse reads DeepSeek SSE stream and accumulates content.
func (s *OpenAIGatewayService) handleDeepSeekNonStreamResponse(
	c *gin.Context,
	resp *http.Response,
	completionID, model string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	defer func() { _ = resp.Body.Close() }()

	var fullContent string
	var fullThinking string

	// Read entire body first — resp.Body may already be partially consumed
	bodyBytes, err := io.ReadAll(resp.Body)
	logger.L().Info("deepseek: non-stream body read", zap.Int("bytes", len(bodyBytes)), zap.Error(err))

	// Parse each line of the SSE stream
	lines := strings.Split(string(bodyBytes), "\n")
	for _, lineStr := range lines {
		lineStr = strings.TrimSpace(lineStr)

		// Skip empty lines and SSE event name lines
		if lineStr == "" || strings.HasPrefix(lineStr, "event:") {
			continue
		}

		// Handle [DONE]
		if lineStr == "data: [DONE]" || lineStr == "[DONE]" {
			break
		}

		// Strip "data: " prefix
		data := lineStr
		if strings.HasPrefix(data, "data: ") {
			data = data[6:]
		}

		// Parse JSON event
		var event map[string]any
		if jsonErr := json.Unmarshal([]byte(data), &event); jsonErr != nil {
			continue
		}

		// Extract content from DeepSeek events
		path, _ := event["p"].(string)
		op, _ := event["o"].(string)
		value := event["v"]

		// New format: {"v":"text"} — content token without p/o fields
		if path == "" && op == "" {
			if text, ok := value.(string); ok && text != "" {
				fullContent += text
			}
			continue
		}

		// Old format: fragment content
		if strings.Contains(path, "fragments") && strings.HasSuffix(path, "/content") && op == "REPLACE" {
			if text, ok := value.(string); ok && text != "" {
				fullContent += text
			}
			continue
		}

		// Old format: fragment append
		if path == "response/fragments" && op == "APPEND" {
			if fragments, ok := value.([]any); ok {
				for _, f := range fragments {
					if frag, ok := f.(map[string]any); ok {
						fragType, _ := frag["type"].(string)
						content, _ := frag["content"].(string)
						if content == "" {
							continue
						}
						if fragType == "THINKING" {
							fullThinking += content
						} else {
							fullContent += content
						}
					}
				}
			}
			continue
		}

		// BATCH and status events don't contribute content, just skip
	}

	// Build OpenAI Chat Completion response
	response := map[string]any{
		"id":      completionID,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": fullContent,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     0,
			"completion_tokens": 0,
			"total_tokens":      0,
		},
	}

	if fullThinking != "" {
		if choices, ok := response["choices"].([]map[string]any); ok && len(choices) > 0 {
			choices[0]["message"].(map[string]any)["reasoning_content"] = fullThinking
		}
	}

	c.JSON(http.StatusOK, response)

	return &OpenAIForwardResult{
		RequestID: completionID,
		Stream:    false,
		Duration:  time.Since(startTime),
	}, nil
}

// buildDeepSeekPayload converts an OpenAI Chat Completions request body to DeepSeek format.
// The DeepSeek web API uses a single "prompt" field. We extract system messages as prefix
// and the last user message as the main prompt.
func buildDeepSeekPayload(body []byte, sessionID string) map[string]any {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return map[string]any{
			"chat_session_id": sessionID,
			"model":           "deepseek-chat",
			"prompt":          "",
			"stream":          true,
			"ref_file_ids":    []any{},
		}
	}

	messages := payload["messages"]

	model, _ := payload["model"].(string)
	if model == "" {
		model = "deepseek-chat"
	}
	if model != "deepseek-chat" && model != "deepseek-coder" && !strings.HasPrefix(model, "deepseek") {
		model = "deepseek-chat"
	}

	stream := true
	if s, ok := payload["stream"].(bool); ok {
		stream = s
	}

	// Extract system message prefix and last user message
	var systemPrefix string
	var lastUserMsg string
	if msgArr, ok := messages.([]any); ok {
		for _, msg := range msgArr {
			msgMap, ok := msg.(map[string]any)
			if !ok {
				continue
			}
			role, _ := msgMap["role"].(string)
			content, _ := msgMap["content"].(string)
			if content == "" {
				continue
			}
			if role == "system" {
				systemPrefix = content
			} else if role == "user" {
				lastUserMsg = content
			}
		}
	}

	prompt := lastUserMsg
	if systemPrefix != "" && lastUserMsg != "" {
		prompt = systemPrefix + "\n\n" + lastUserMsg
	}

	dsPayload := map[string]any{
		"chat_session_id": sessionID,
		"prompt":          prompt,
		"model":           model,
		"stream":          stream,
		"ref_file_ids":    []any{},
		"search_enabled":  true,
	}

	if temp, ok := payload["temperature"]; ok {
		dsPayload["temperature"] = temp
	}
	if topP, ok := payload["top_p"]; ok {
		dsPayload["top_p"] = topP
	}
	if maxTokens, ok := payload["max_tokens"]; ok {
		dsPayload["max_tokens"] = maxTokens
	}
	if maxTokens, ok := payload["max_completion_tokens"]; ok {
		if _, exists := dsPayload["max_tokens"]; !exists {
			dsPayload["max_tokens"] = maxTokens
		}
	}

	return dsPayload
}
