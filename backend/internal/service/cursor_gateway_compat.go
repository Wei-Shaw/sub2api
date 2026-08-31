package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
	"github.com/gin-gonic/gin"
)

// ForwardAsAnthropic serves Anthropic /v1/messages clients through Cursor.
// output_config.effort and thinking.type are mapped onto Cursor Run slugs.
func (s *CursorGatewayService) ForwardAsAnthropic(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
) (*ForwardResult, error) {
	startTime := time.Now()

	var req apicompat.AnthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeAnthropicError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return nil, fmt.Errorf("cursor anthropic: parse: %w", err)
	}
	if strings.TrimSpace(req.Model) == "" {
		writeAnthropicError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return nil, fmt.Errorf("cursor anthropic: model is required")
	}

	ccReq, err := apicompat.AnthropicToChatCompletionsRequest(&req)
	if err != nil {
		writeAnthropicError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, fmt.Errorf("cursor anthropic: convert: %w", err)
	}

	messages := cursorMessagesFromChat(ccReq.Messages)
	mappedModel := account.GetMappedModel(req.Model)
	resp, _, warnings, err := s.startCursorChat(ctx, c, account, messages, mappedModel, cursorRunOptsFromAnthropic(&req))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if req.Stream {
		return s.streamCursorAsAnthropic(c, resp.Body, req.Model, warnings, startTime)
	}
	return s.nonStreamCursorAsAnthropic(c, resp.Body, req.Model, warnings, startTime)
}

// ForwardAsResponses serves OpenAI /v1/responses clients through Cursor.
// reasoning.effort is mapped onto Cursor Run slugs.
func (s *CursorGatewayService) ForwardAsResponses(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
) (*ForwardResult, error) {
	startTime := time.Now()

	var req apicompat.ResponsesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeResponsesError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return nil, fmt.Errorf("cursor responses: parse: %w", err)
	}
	if strings.TrimSpace(req.Model) == "" {
		writeResponsesError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return nil, fmt.Errorf("cursor responses: model is required")
	}

	ccReq, err := apicompat.ResponsesToChatCompletionsRequest(&req)
	if err != nil {
		writeResponsesError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, fmt.Errorf("cursor responses: convert: %w", err)
	}

	messages := cursorMessagesFromChat(ccReq.Messages)
	mappedModel := account.GetMappedModel(req.Model)
	resp, _, warnings, err := s.startCursorChat(ctx, c, account, messages, mappedModel, cursorRunOptsFromResponses(&req))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if req.Stream {
		return s.streamCursorAsResponses(c, resp.Body, req.Model, warnings, startTime)
	}
	return s.nonStreamCursorAsResponses(c, resp.Body, req.Model, warnings, startTime)
}

func cursorMessagesFromChat(messages []apicompat.ChatMessage) []cursor.ChatMessage {
	out := make([]cursor.ChatMessage, 0, len(messages))
	for _, message := range messages {
		out = append(out, cursor.ChatMessage{
			Role:    message.Role,
			Content: chatRawContentText(message.Content),
		})
	}
	return out
}

func chatRawContentText(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}
	if raw[0] == '"' {
		var text string
		if json.Unmarshal(raw, &text) == nil {
			return text
		}
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		var b strings.Builder
		for _, part := range parts {
			if part.Type == "" || part.Type == "text" {
				b.WriteString(part.Text)
			}
		}
		return b.String()
	}
	return string(raw)
}

func collectCursorAssistant(body io.Reader) (text, thinking, connectErr string, usage cursor.TokenUsage) {
	var textBuf, thinkingBuf strings.Builder
	usage, connectErr = iterCursorAssistant(body, func(kind, payload string) error {
		switch kind {
		case "text":
			textBuf.WriteString(payload)
		case "thinking":
			thinkingBuf.WriteString(payload)
		}
		return nil
	})
	return textBuf.String(), thinkingBuf.String(), connectErr, usage
}

func iterCursorAssistant(body io.Reader, emit func(kind, text string) error) (usage cursor.TokenUsage, connectErr string) {
	return cursor.ConsumeAssistantStream(body, func(ev cursor.StreamEvent) error {
		switch ev.Type {
		case "text", "thinking":
			if err := emit(ev.Type, ev.Text); err != nil {
				return err
			}
		}
		return nil
	})
}

func cursorChatCompletion(id, model, text, thinking string, usage cursor.TokenUsage) *apicompat.ChatCompletionsResponse {
	content, _ := json.Marshal(text)
	return &apicompat.ChatCompletionsResponse{
		ID:      id,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []apicompat.ChatChoice{{
			Index: 0,
			Message: apicompat.ChatMessage{
				Role:             "assistant",
				Content:          content,
				ReasoningContent: thinking,
			},
			FinishReason: "stop",
		}},
		Usage: chatUsageFromCursor(usage),
	}
}

func cursorUsageChunk(id, model string, usage cursor.TokenUsage) *apicompat.ChatCompletionsChunk {
	chatUsage := chatUsageFromCursor(usage)
	if chatUsage == nil {
		return nil
	}
	return &apicompat.ChatCompletionsChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []apicompat.ChatChunkChoice{},
		Usage:   chatUsage,
	}
}

func cursorTextChunk(id, model, text, thinking string) *apicompat.ChatCompletionsChunk {
	chunk := &apicompat.ChatCompletionsChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []apicompat.ChatChunkChoice{{Index: 0}},
	}
	if thinking != "" {
		chunk.Choices[0].Delta.ReasoningContent = &thinking
	}
	if text != "" {
		chunk.Choices[0].Delta.Content = &text
	}
	return chunk
}

func (s *CursorGatewayService) nonStreamCursorAsAnthropic(
	c *gin.Context,
	body io.Reader,
	model string,
	warnings []map[string]string,
	startTime time.Time,
) (*ForwardResult, error) {
	text, thinking, connectErr, usage := collectCursorAssistant(body)
	if connectErr != "" && text == "" {
		status, errType, message := classifyCursorConnectError(connectErr)
		writeAnthropicError(c, status, errType, message)
		return nil, fmt.Errorf("cursor anthropic: %s", message)
	}
	ccResp := cursorChatCompletion("chatcmpl-cursor-"+time.Now().Format("20060102150405"), model, text, thinking, usage)
	c.JSON(http.StatusOK, apicompat.ChatCompletionsResponseToAnthropic(ccResp, model))
	return cursorForwardResult(model, false, startTime, nil, usage), nil
}

func (s *CursorGatewayService) streamCursorAsAnthropic(
	c *gin.Context,
	body io.Reader,
	model string,
	warnings []map[string]string,
	startTime time.Time,
) (*ForwardResult, error) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Writer.Flush()

	state := apicompat.NewChatCompletionsToAnthropicStreamState(model)
	completionID := "chatcmpl-cursor-" + time.Now().Format("20060102150405")
	var firstTokenMs *int
	var totalText strings.Builder

	writeEvents := func(events []apicompat.AnthropicStreamEvent) {
		for _, event := range events {
			sse, err := apicompat.ResponsesAnthropicEventToSSE(event)
			if err != nil {
				continue
			}
			fmt.Fprint(c.Writer, sse)
		}
		c.Writer.Flush()
	}

	usage, connectErr := iterCursorAssistant(body, func(kind, payload string) error {
		if firstTokenMs == nil {
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
		}
		text, thinking := "", ""
		if kind == "thinking" {
			thinking = payload
		} else {
			text = payload
			totalText.WriteString(payload)
		}
		writeEvents(apicompat.ChatCompletionsChunkToAnthropicEvents(cursorTextChunk(completionID, model, text, thinking), state))
		return nil
	})
	if connectErr != "" && totalText.Len() == 0 {
		_, errType, message := classifyCursorConnectError(connectErr)
		fmt.Fprint(c.Writer, buildAnthropicStreamErrorSSE(errType, message))
		c.Writer.Flush()
		return cursorForwardResult(model, true, startTime, firstTokenMs, usage), nil
	}
	if chunk := cursorUsageChunk(completionID, model, usage); chunk != nil {
		writeEvents(apicompat.ChatCompletionsChunkToAnthropicEvents(chunk, state))
	}
	writeEvents(apicompat.FinalizeChatCompletionsAnthropicStream(state))
	return cursorForwardResult(model, true, startTime, firstTokenMs, usage), nil
}

func (s *CursorGatewayService) nonStreamCursorAsResponses(
	c *gin.Context,
	body io.Reader,
	model string,
	warnings []map[string]string,
	startTime time.Time,
) (*ForwardResult, error) {
	text, thinking, connectErr, usage := collectCursorAssistant(body)
	if connectErr != "" && text == "" {
		status, errType, message := classifyCursorConnectError(connectErr)
		writeResponsesError(c, status, errType, message)
		return nil, fmt.Errorf("cursor responses: %s", message)
	}
	ccResp := cursorChatCompletion("chatcmpl-cursor-"+time.Now().Format("20060102150405"), model, text, thinking, usage)
	c.JSON(http.StatusOK, apicompat.ChatCompletionsResponseToResponses(ccResp, model, nil, nil, false, nil))
	return cursorForwardResult(model, false, startTime, nil, usage), nil
}

func (s *CursorGatewayService) streamCursorAsResponses(
	c *gin.Context,
	body io.Reader,
	model string,
	warnings []map[string]string,
	startTime time.Time,
) (*ForwardResult, error) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Writer.Flush()

	state := apicompat.NewChatCompletionsToResponsesStreamState(model)
	completionID := "chatcmpl-cursor-" + time.Now().Format("20060102150405")
	var firstTokenMs *int
	var totalText strings.Builder

	writeEvents := func(events []apicompat.ResponsesStreamEvent) {
		for _, event := range events {
			sse, err := apicompat.ResponsesEventToSSE(event)
			if err != nil {
				continue
			}
			fmt.Fprint(c.Writer, sse)
		}
		c.Writer.Flush()
	}

	usage, connectErr := iterCursorAssistant(body, func(kind, payload string) error {
		if firstTokenMs == nil {
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
		}
		text, thinking := "", ""
		if kind == "thinking" {
			thinking = payload
		} else {
			text = payload
			totalText.WriteString(payload)
		}
		writeEvents(apicompat.ChatCompletionsChunkToResponsesEvents(cursorTextChunk(completionID, model, text, thinking), state))
		return nil
	})
	if connectErr != "" && totalText.Len() == 0 {
		_, errType, message := classifyCursorConnectError(connectErr)
		payload, _ := json.Marshal(map[string]string{"code": errType, "message": message})
		fmt.Fprintf(c.Writer, "event: error\ndata: %s\n\n", payload)
		c.Writer.Flush()
		return cursorForwardResult(model, true, startTime, firstTokenMs, usage), nil
	}
	if chunk := cursorUsageChunk(completionID, model, usage); chunk != nil {
		writeEvents(apicompat.ChatCompletionsChunkToResponsesEvents(chunk, state))
	}
	writeEvents(apicompat.FinalizeChatCompletionsResponsesStream(state))
	return cursorForwardResult(model, true, startTime, firstTokenMs, usage), nil
}
