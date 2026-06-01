package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	openAISilentRefusalMinRequestBodyBytes = 64 * 1024
	openAISilentRefusalErrorCode           = "openai_silent_refusal"
	openAISilentRefusalUpstreamMessage     = "OpenAI upstream returned an empty completion stream with finish_reason=stop and no usage"
	openAISilentRefusalClientMessage       = "Upstream returned an empty completion without usage; no fallback account was available"
)

type openAIChatSilentRefusalDetector struct {
	enabled         bool
	sawContent      bool
	sawToolCall     bool
	sawFunctionCall bool
	sawUsage        bool
	sawError        bool
	sawReasoning    bool
	sawFinish       bool
	finishReason    string
}

func newOpenAIChatSilentRefusalDetector(requestBodyLen int) *openAIChatSilentRefusalDetector {
	return &openAIChatSilentRefusalDetector{
		enabled: requestBodyLen >= openAISilentRefusalMinRequestBodyBytes,
	}
}

func (d *openAIChatSilentRefusalDetector) Enabled() bool {
	return d != nil && d.enabled
}

func (d *openAIChatSilentRefusalDetector) ObserveSSELine(line string) {
	if d == nil || !d.enabled {
		return
	}
	if eventType, ok := extractOpenAISSEEventLine(line); ok {
		d.observeEventType(eventType)
		return
	}
	if payload, ok := extractOpenAISSEDataLine(line); ok {
		d.ObservePayload([]byte(payload))
	}
}

func (d *openAIChatSilentRefusalDetector) ObservePayload(payload []byte) {
	if d == nil || !d.enabled {
		return
	}
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
		return
	}
	if !gjson.ValidBytes(payload) {
		return
	}

	eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
	d.observeEventType(eventType)

	if gjson.GetBytes(payload, "error").Exists() {
		d.sawError = true
	}
	if usage := gjson.GetBytes(payload, "usage"); usage.Exists() && usage.IsObject() {
		d.sawUsage = true
	}
	if usage := gjson.GetBytes(payload, "response.usage"); usage.Exists() && usage.IsObject() {
		d.sawUsage = true
	}

	d.observeChatChoicesPayload(payload)
	d.observeResponsesPayload(payload, eventType)
}

func (d *openAIChatSilentRefusalDetector) ObserveChatChunk(chunk apicompat.ChatCompletionsChunk) {
	if d == nil || !d.enabled {
		return
	}
	if chunk.Usage != nil {
		d.sawUsage = true
	}
	for _, choice := range chunk.Choices {
		if choice.FinishReason != nil {
			d.observeFinishReason(*choice.FinishReason)
		}
		delta := choice.Delta
		if delta.Content != nil && *delta.Content != "" {
			d.sawContent = true
		}
		if delta.ReasoningContent != nil {
			d.sawReasoning = true
		}
		if len(delta.ToolCalls) > 0 {
			d.sawToolCall = true
		}
	}
}

func (d *openAIChatSilentRefusalDetector) ShouldReleaseClientOutput() bool {
	if d == nil || !d.enabled {
		return true
	}
	if d.sawContent || d.sawToolCall || d.sawFunctionCall || d.sawUsage || d.sawError || d.sawReasoning {
		return true
	}
	return d.sawFinish && d.finishReason != "" && d.finishReason != "stop"
}

func (d *openAIChatSilentRefusalDetector) IsSilentRefusal() bool {
	if d == nil || !d.enabled {
		return false
	}
	return !d.sawContent &&
		!d.sawToolCall &&
		!d.sawFunctionCall &&
		!d.sawUsage &&
		!d.sawError &&
		!d.sawReasoning &&
		d.sawFinish &&
		d.finishReason == "stop"
}

func (d *openAIChatSilentRefusalDetector) observeEventType(eventType string) {
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return
	}
	if eventType == "error" || eventType == "response.failed" {
		d.sawError = true
	}
	if strings.Contains(eventType, "reasoning") || strings.Contains(eventType, "reasoning_summary") {
		d.sawReasoning = true
	}
}

func (d *openAIChatSilentRefusalDetector) observeFinishReason(reason string) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return
	}
	d.sawFinish = true
	d.finishReason = reason
}

func (d *openAIChatSilentRefusalDetector) observeChatChoicesPayload(payload []byte) {
	choices := gjson.GetBytes(payload, "choices")
	if !choices.Exists() || !choices.IsArray() {
		return
	}
	for _, choice := range choices.Array() {
		if finish := choice.Get("finish_reason"); finish.Exists() {
			d.observeFinishReason(finish.String())
		}
		delta := choice.Get("delta")
		if !delta.Exists() {
			continue
		}
		if content := delta.Get("content"); content.Exists() && content.String() != "" {
			d.sawContent = true
		}
		if delta.Get("tool_calls").Exists() {
			d.sawToolCall = true
		}
		if delta.Get("function_call").Exists() {
			d.sawFunctionCall = true
		}
		if delta.Get("reasoning").Exists() ||
			delta.Get("reasoning_content").Exists() ||
			delta.Get("reasoning_summary").Exists() {
			d.sawReasoning = true
		}
	}
}

func (d *openAIChatSilentRefusalDetector) observeResponsesPayload(payload []byte, eventType string) {
	switch eventType {
	case "response.output_text.delta":
		if gjson.GetBytes(payload, "delta").String() != "" {
			d.sawContent = true
		}
	case "response.output_item.added":
		switch strings.TrimSpace(gjson.GetBytes(payload, "item.type").String()) {
		case "function_call":
			d.sawToolCall = true
		case "reasoning":
			d.sawReasoning = true
		}
	case "response.function_call_arguments.delta":
		d.sawToolCall = true
	case "response.reasoning_summary_text.delta", "response.reasoning_summary_text.done":
		d.sawReasoning = true
	case "response.completed", "response.done":
		d.observeFinishReason("stop")
	case "response.incomplete":
		d.observeFinishReason("length")
	case "response.failed":
		d.sawError = true
	}

	if output := gjson.GetBytes(payload, "response.output"); output.Exists() && output.IsArray() {
		for _, item := range output.Array() {
			switch strings.TrimSpace(item.Get("type").String()) {
			case "function_call":
				d.sawToolCall = true
			case "reasoning":
				d.sawReasoning = true
			case "message":
				d.observeResponseMessageItem(item)
			}
		}
	}
}

func (d *openAIChatSilentRefusalDetector) observeResponseMessageItem(item gjson.Result) {
	content := item.Get("content")
	if !content.Exists() || !content.IsArray() {
		return
	}
	for _, part := range content.Array() {
		if part.Get("text").String() != "" {
			d.sawContent = true
			return
		}
	}
}

func newOpenAISilentRefusalFailoverError(c *gin.Context, account *Account, upstreamRequestID string) *UpstreamFailoverError {
	accountID := int64(0)
	accountName := ""
	platform := PlatformOpenAI
	if account != nil {
		accountID = account.ID
		accountName = account.Name
		platform = account.Platform
	}

	setOpsUpstreamError(c, http.StatusBadGateway, openAISilentRefusalUpstreamMessage, "")
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           platform,
		AccountID:          accountID,
		AccountName:        accountName,
		UpstreamStatusCode: http.StatusBadGateway,
		UpstreamRequestID:  upstreamRequestID,
		Kind:               "failover",
		Message:            openAISilentRefusalUpstreamMessage,
	})

	headers := http.Header{}
	if strings.TrimSpace(upstreamRequestID) != "" {
		headers.Set("x-request-id", strings.TrimSpace(upstreamRequestID))
	}
	return &UpstreamFailoverError{
		StatusCode:      http.StatusBadGateway,
		ResponseBody:    openAISilentRefusalErrorBody(),
		ResponseHeaders: headers,
	}
}

func openAISilentRefusalErrorBody() []byte {
	body, err := json.Marshal(map[string]any{
		"error": map[string]any{
			"type":    "upstream_error",
			"code":    openAISilentRefusalErrorCode,
			"message": openAISilentRefusalUpstreamMessage,
		},
	})
	if err != nil {
		return []byte(`{"error":{"type":"upstream_error","code":"openai_silent_refusal","message":"OpenAI upstream returned an empty completion stream with finish_reason=stop and no usage"}}`)
	}
	return body
}

// IsOpenAISilentRefusalErrorBody reports whether a failover body was produced
// by the OpenAI silent-refusal detector.
func IsOpenAISilentRefusalErrorBody(body []byte) bool {
	return strings.TrimSpace(gjson.GetBytes(body, "error.code").String()) == openAISilentRefusalErrorCode
}

// OpenAISilentRefusalClientMessage returns the exhausted-failover client message
// for OpenAI silent refusals.
func OpenAISilentRefusalClientMessage() string {
	return openAISilentRefusalClientMessage
}
