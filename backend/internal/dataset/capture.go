package dataset

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// MaxCaptureBytes bounds per-request dataset capture memory.
const MaxCaptureBytes = 10 * 1024 * 1024

// CaptureFromOpenAIRequest attempts to capture a completed non-streaming OpenAI
// Chat Completions request/response pair. The capture path is deliberately
// fail-open: malformed, failed, or oversized payloads are ignored.
func CaptureFromOpenAIRequest(
	requestBody []byte,
	responseBody []byte,
	statusCode int,
) {
	if CollectorSingleton == nil || statusCode < 200 || statusCode >= 300 {
		return
	}
	if len(requestBody) == 0 || len(requestBody) > MaxCaptureBytes || len(responseBody) == 0 || len(responseBody) > MaxCaptureBytes {
		return
	}
	if !gjson.ValidBytes(requestBody) || !gjson.ValidBytes(responseBody) {
		return
	}

	reqModel := gjson.GetBytes(requestBody, "model").String()
	if reqModel == "" {
		return
	}

	messages := gjson.GetBytes(requestBody, "messages").Array()
	if len(messages) == 0 {
		return
	}

	respModel := gjson.GetBytes(responseBody, "model").String()
	choices := gjson.GetBytes(responseBody, "choices").Array()
	if len(choices) == 0 {
		return
	}

	entry := DatasetEntry{
		Timestamp: time.Now().UTC(),
		Endpoint:  "chat.completions",
		Request: map[string]any{
			"model": reqModel,
		},
		Response: map[string]any{
			"model": respModel,
		},
	}

	reqMessages := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		role := msg.Get("role").String()
		content := msg.Get("content").String()
		if role != "" {
			reqMessages = append(reqMessages, map[string]any{
				"role":    role,
				"content": content,
			})
		}
	}
	if len(reqMessages) == 0 {
		return
	}
	entry.Request["messages"] = reqMessages

	choice := choices[0]
	assistantMsg := choice.Get("message")
	entry.Response["message"] = map[string]any{
		"role":    assistantMsg.Get("role").String(),
		"content": assistantMsg.Get("content").String(),
	}
	if finishReason := choice.Get("finish_reason"); finishReason.Exists() && finishReason.Type != gjson.Null {
		entry.Response["finish_reason"] = finishReason.String()
	}

	if usage := gjson.GetBytes(responseBody, "usage"); usage.Exists() && usage.IsObject() {
		entry.Response["usage"] = map[string]any{
			"prompt_tokens":     usage.Get("prompt_tokens").Int(),
			"completion_tokens": usage.Get("completion_tokens").Int(),
			"total_tokens":      usage.Get("total_tokens").Int(),
		}
	}

	RedactSensitiveFields(&entry)
	CollectorSingleton.Collect(&entry)
}

// CaptureFromOpenAIStream reassembles a completed Chat Completions SSE stream
// and captures it using the same schema as a non-streaming response. A stream
// is accepted only when a [DONE] frame is observed; partial/cancelled streams
// are intentionally discarded.
func CaptureFromOpenAIStream(requestBody, streamBody []byte, statusCode int) {
	if CollectorSingleton == nil || statusCode < 200 || statusCode >= 300 || len(streamBody) == 0 || len(streamBody) > MaxCaptureBytes {
		return
	}

	responseBody, ok := reassembleOpenAIChatStream(streamBody)
	if !ok {
		return
	}
	CaptureFromOpenAIRequest(requestBody, responseBody, statusCode)
}

func reassembleOpenAIChatStream(streamBody []byte) ([]byte, bool) {
	scanner := bufio.NewScanner(bytes.NewReader(streamBody))
	scanner.Buffer(make([]byte, 64*1024), MaxCaptureBytes)

	var dataLines []string
	var response struct {
		ID      string `json:"id,omitempty"`
		Object  string `json:"object,omitempty"`
		Created int64  `json:"created,omitempty"`
		Model   string `json:"model,omitempty"`
		Choices []struct {
			Index   int `json:"index"`
			Message struct {
				Role    string `json:"role,omitempty"`
				Content string `json:"content,omitempty"`
			} `json:"message"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
		Usage json.RawMessage `json:"usage,omitempty"`
	}
	var content strings.Builder
	var role string
	var finishReason *string
	seenData := false
	done := false

	flushFrame := func() bool {
		if len(dataLines) == 0 {
			return true
		}
		payload := strings.TrimSpace(strings.Join(dataLines, "\n"))
		dataLines = dataLines[:0]
		if payload == "[DONE]" {
			done = true
			return true
		}
		if payload == "" || !gjson.Valid(payload) {
			return false
		}
		seenData = true
		if value := gjson.Get(payload, "id"); value.Exists() && response.ID == "" {
			response.ID = value.String()
		}
		if value := gjson.Get(payload, "object"); value.Exists() && response.Object == "" {
			response.Object = value.String()
		}
		if value := gjson.Get(payload, "created"); value.Exists() && response.Created == 0 {
			response.Created = value.Int()
		}
		if value := gjson.Get(payload, "model"); value.Exists() && response.Model == "" {
			response.Model = value.String()
		}
		if value := gjson.Get(payload, "usage"); value.Exists() && value.IsObject() {
			response.Usage = json.RawMessage(value.Raw)
		}
		choice := gjson.Get(payload, "choices.0")
		if choice.Exists() {
			if value := choice.Get("delta.role"); value.Exists() && value.String() != "" {
				role = value.String()
			}
			if value := choice.Get("delta.content"); value.Exists() && value.Type == gjson.String {
				if _, err := content.WriteString(value.String()); err != nil {
					return false
				}
			}
			if value := choice.Get("finish_reason"); value.Exists() && value.Type != gjson.Null {
				reason := value.String()
				finishReason = &reason
			}
		}
		return true
	}

	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if !flushFrame() {
				return nil, false
			}
			continue
		}
		if strings.HasPrefix(line, ":") || strings.HasPrefix(line, "event:") {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data:")
			dataLines = append(dataLines, strings.TrimPrefix(data, " "))
		}
	}
	if scanner.Err() != nil || !flushFrame() || !done || !seenData {
		return nil, false
	}
	if role == "" {
		role = "assistant"
	}
	if response.Object == "" {
		response.Object = "chat.completion"
	}
	response.Choices = []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role,omitempty"`
			Content string `json:"content,omitempty"`
		} `json:"message"`
		FinishReason *string `json:"finish_reason"`
	}{{Index: 0, FinishReason: finishReason}}
	response.Choices[0].Message.Role = role
	response.Choices[0].Message.Content = content.String()

	body, err := json.Marshal(response)
	return body, err == nil && len(body) <= MaxCaptureBytes
}
