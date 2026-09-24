package dataset

import (
	"time"

	"github.com/tidwall/gjson"
)

// CaptureFromOpenAIRequest attempts to capture dataset entry from OpenAI API request/response.
// Designed for fail-open: returns silently on any error to avoid impacting the hot path.
func CaptureFromOpenAIRequest(
	requestBody []byte,
	responseBody []byte,
	statusCode int,
) {
	if CollectorSingleton == nil || statusCode < 200 || statusCode >= 300 {
		return
	}

	// Extract request fields
	reqModel := gjson.GetBytes(requestBody, "model").String()
	if reqModel == "" {
		return
	}

	messages := gjson.GetBytes(requestBody, "messages").Array()
	if len(messages) == 0 {
		return
	}

	// Extract response fields (non-streaming only for now)
	respModel := gjson.GetBytes(responseBody, "model").String()
	choices := gjson.GetBytes(responseBody, "choices").Array()
	if len(choices) == 0 {
		return
	}

	// Build dataset entry
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

	// Parse request messages
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
	entry.Request["messages"] = reqMessages

	// Parse response choice
	if len(choices) > 0 {
		choice := choices[0]
		assistantMsg := choice.Get("message")
		entry.Response["message"] = map[string]any{
			"role":    assistantMsg.Get("role").String(),
			"content": assistantMsg.Get("content").String(),
		}

		finishReason := choice.Get("finish_reason").String()
		if finishReason != "" {
			entry.Response["finish_reason"] = finishReason
		}
	}

	// Capture usage if present
	usage := gjson.GetBytes(responseBody, "usage")
	if usage.Exists() {
		entry.Response["usage"] = map[string]any{
			"prompt_tokens":     usage.Get("prompt_tokens").Int(),
			"completion_tokens": usage.Get("completion_tokens").Int(),
			"total_tokens":      usage.Get("total_tokens").Int(),
		}
	}

	// Submit to bounded collector (fail-open)
	CollectorSingleton.Collect(&entry)
}
