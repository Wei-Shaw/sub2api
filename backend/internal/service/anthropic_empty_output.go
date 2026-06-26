package service

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
)

const emptyAnthropicCompletionMessage = "upstream returned empty Anthropic completion in HTTP 200"

func appendRawSSEPair(dst *strings.Builder, eventLine, dataLine string) {
	if dst == nil {
		return
	}
	dst.WriteString(eventLine)
	dst.WriteByte('\n')
	dst.WriteString(dataLine)
	dst.WriteByte('\n')
	dst.WriteByte('\n')
}

func logEmptyAnthropicHTTP200Response(scope string, c *gin.Context, resp *http.Response, originalModel, mappedModel, reason string, upstreamBody string) {
	if strings.TrimSpace(upstreamBody) == "" {
		upstreamBody = "(empty)"
	}
	requestID := ""
	statusCode := 0
	contentType := ""
	upstreamRequestID := ""
	if resp != nil {
		statusCode = resp.StatusCode
		contentType = resp.Header.Get("Content-Type")
		upstreamRequestID = resp.Header.Get("x-request-id")
		requestID = upstreamRequestID
	}
	method := ""
	path := ""
	if c != nil && c.Request != nil {
		method = c.Request.Method
		path = c.Request.URL.Path
		if v, ok := c.Get("request_id"); ok && v != nil {
			switch typed := v.(type) {
			case string:
				requestID = strings.TrimSpace(typed)
			case fmt.Stringer:
				requestID = strings.TrimSpace(typed.String())
			}
		}
	}
	slog.Warn("anthropic.http_200_empty_response_body",
		"scope", scope,
		"reason", reason,
		"request_id", requestID,
		"upstream_request_id", upstreamRequestID,
		"status_code", statusCode,
		"content_type", contentType,
		"method", method,
		"path", path,
		"model", originalModel,
		"upstream_model", mappedModel,
		"upstream_response_body", upstreamBody,
	)
}

func anthropicResponseHasOutput(resp *apicompat.AnthropicResponse) bool {
	if resp == nil {
		return false
	}
	for _, block := range resp.Content {
		if anthropicContentBlockHasOutput(block) {
			return true
		}
	}
	return false
}

func anthropicStreamEventHasOutput(event *apicompat.AnthropicStreamEvent) bool {
	if event == nil {
		return false
	}
	if event.Message != nil && anthropicResponseHasOutput(event.Message) {
		return true
	}
	if event.ContentBlock != nil && anthropicContentBlockHasOutput(*event.ContentBlock) {
		return true
	}
	if event.Delta != nil {
		switch event.Delta.Type {
		case "text_delta":
			return strings.TrimSpace(event.Delta.Text) != ""
		case "thinking_delta":
			return strings.TrimSpace(event.Delta.Thinking) != ""
		case "input_json_delta":
			return rawJSONHasObjectContent(json.RawMessage(event.Delta.PartialJSON))
		}
	}
	return false
}

func anthropicStreamEventHasVisibleCompletionOutput(event *apicompat.AnthropicStreamEvent) bool {
	if event == nil {
		return false
	}
	if event.ContentBlock != nil {
		switch event.ContentBlock.Type {
		case "text":
			return strings.TrimSpace(event.ContentBlock.Text) != ""
		case "tool_use", "server_tool_use":
			return strings.TrimSpace(event.ContentBlock.Name) != "" || strings.TrimSpace(event.ContentBlock.ID) != "" || rawJSONHasObjectContent(event.ContentBlock.Input)
		}
	}
	if event.Delta != nil {
		switch event.Delta.Type {
		case "text_delta":
			return strings.TrimSpace(event.Delta.Text) != ""
		case "input_json_delta":
			return rawJSONHasObjectContent(json.RawMessage(event.Delta.PartialJSON))
		}
	}
	return false
}

func anthropicContentBlockHasOutput(block apicompat.AnthropicContentBlock) bool {
	switch block.Type {
	case "text":
		return strings.TrimSpace(block.Text) != ""
	case "thinking":
		return strings.TrimSpace(block.Thinking) != ""
	case "tool_use", "server_tool_use":
		return strings.TrimSpace(block.Name) != "" || strings.TrimSpace(block.ID) != "" || rawJSONHasObjectContent(block.Input)
	default:
		return strings.TrimSpace(block.Text) != "" || strings.TrimSpace(block.Thinking) != "" || rawJSONHasObjectContent(block.Input)
	}
}

func responsesResponseHasOutput(resp *apicompat.ResponsesResponse) bool {
	if resp == nil {
		return false
	}
	for _, item := range resp.Output {
		if responsesOutputHasOutput(item) {
			return true
		}
	}
	return false
}

func ensureResponsesResponseHasOutput(resp *apicompat.ResponsesResponse, acc *apicompat.BufferedResponseAccumulator) error {
	if acc != nil && acc.HasContent() {
		return nil
	}
	if responsesResponseHasOutput(resp) {
		return nil
	}
	return newUpstreamStreamEndedFailoverError(emptyAnthropicCompletionMessage)
}

func responsesStreamEventHasOutput(event *apicompat.ResponsesStreamEvent) bool {
	if event == nil {
		return false
	}
	if responsesResponseHasOutput(event.Response) {
		return true
	}
	if event.Item != nil && responsesOutputHasOutput(*event.Item) {
		return true
	}

	switch strings.TrimSpace(event.Type) {
	case "response.output_text.delta":
		return strings.TrimSpace(event.Delta) != ""
	case "response.function_call_arguments.delta":
		return strings.TrimSpace(event.Delta) != "" || strings.TrimSpace(event.Arguments) != ""
	case "response.reasoning_summary_text.delta":
		return strings.TrimSpace(event.Delta) != "" || strings.TrimSpace(event.Text) != ""
	}
	return false
}

func responsesOutputHasOutput(item apicompat.ResponsesOutput) bool {
	switch strings.TrimSpace(item.Type) {
	case "message":
		for _, part := range item.Content {
			if responsesContentPartHasOutput(part) {
				return true
			}
		}
	case "function_call":
		return strings.TrimSpace(item.CallID) != "" ||
			strings.TrimSpace(item.Name) != "" ||
			rawJSONHasObjectContent(json.RawMessage(item.Arguments))
	case "reasoning":
		if strings.TrimSpace(item.EncryptedContent) != "" {
			return true
		}
		for _, summary := range item.Summary {
			if strings.TrimSpace(summary.Text) != "" {
				return true
			}
		}
	case "web_search_call":
		if item.Action != nil {
			return strings.TrimSpace(item.Action.Query) != ""
		}
	}
	return false
}

func responsesContentPartHasOutput(part apicompat.ResponsesContentPart) bool {
	switch strings.TrimSpace(part.Type) {
	case "output_text", "text":
		return strings.TrimSpace(part.Text) != ""
	default:
		return strings.TrimSpace(part.Text) != ""
	}
}

func chatChunkHasOutput(chunk apicompat.ChatCompletionsChunk) bool {
	for _, choice := range chunk.Choices {
		delta := choice.Delta
		if delta.Content != nil && strings.TrimSpace(*delta.Content) != "" {
			return true
		}
		if delta.ReasoningContent != nil && strings.TrimSpace(*delta.ReasoningContent) != "" {
			return true
		}
		if len(delta.ToolCalls) > 0 {
			return true
		}
	}
	return false
}

func chatResponseHasOutput(resp *apicompat.ChatCompletionsResponse) bool {
	if resp == nil {
		return false
	}
	for _, choice := range resp.Choices {
		if chatMessageHasOutput(choice.Message) {
			return true
		}
	}
	return false
}

func chatMessageHasOutput(message apicompat.ChatMessage) bool {
	if rawJSONHasTextContent(message.Content) {
		return true
	}
	if strings.TrimSpace(message.ReasoningContent) != "" {
		return true
	}
	if len(message.ToolCalls) > 0 || message.FunctionCall != nil {
		return true
	}
	return false
}

func rawJSONHasTextContent(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" || trimmed == `""` || trimmed == "[]" || trimmed == "{}" {
		return false
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text) != ""
	}
	var parts []apicompat.ChatContentPart
	if err := json.Unmarshal(raw, &parts); err == nil {
		for _, part := range parts {
			if strings.TrimSpace(part.Text) != "" {
				return true
			}
		}
		return false
	}
	return rawJSONHasObjectContent(raw)
}

func rawJSONHasObjectContent(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null" && trimmed != "{}" && trimmed != "[]"
}
