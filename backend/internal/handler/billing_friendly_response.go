package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/googleapi"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	billingProtocolAnthropic       = "anthropic"
	billingProtocolAnthropicError  = "anthropic-error"
	billingProtocolGemini          = "gemini"
	billingProtocolGeminiError     = "gemini-error"
	billingProtocolOpenAIChat      = "openai"
	billingProtocolOpenAIImages    = "openai-images"
	billingProtocolOpenAIResponses = "openai-responses"
)

// billingFriendlyMessage returns a debuggable user-facing assistant message for
// billing/rate-limit errors. The text intentionally includes the stable billing
// code because OpenClaw users often only have the chat bubble/screenshot when
// reporting failures.
func billingFriendlyMessage(err error) string {
	_, code, detail, retryAfter := billingErrorDetails(err)
	summary := billingFriendlySummary(err, code)
	action := billingFriendlyAction(code, retryAfter)
	detail = strings.TrimSpace(detail)
	if detail == "" {
		detail = summary
	}

	return fmt.Sprintf(
		"V-Claw could not send this request to the model because sub2api blocked it before provider execution.\n\nReason: %s\nCode: %s\nDetails: %s\nAction: %s",
		summary,
		code,
		detail,
		action,
	)
}

func billingFriendlySummary(err error, code string) string {
	switch {
	case errors.Is(err, service.ErrInsufficientBalance):
		return "Account balance is insufficient."
	case errors.Is(err, service.ErrSubscriptionInvalid):
		return "Subscription is invalid or expired."
	case errors.Is(err, service.ErrSubscriptionExpired):
		return "Subscription has expired."
	case errors.Is(err, service.ErrDailyLimitExceeded):
		return "Daily subscription usage limit has been reached."
	case errors.Is(err, service.ErrWeeklyLimitExceeded):
		return "Weekly subscription usage limit has been reached."
	case errors.Is(err, service.ErrMonthlyLimitExceeded):
		return "Monthly subscription usage limit has been reached."
	case errors.Is(err, service.ErrAPIKeyRateLimit5hExceeded):
		return "Current API key has reached its 5-hour usage limit."
	case errors.Is(err, service.ErrAPIKeyRateLimit1dExceeded):
		return "Current API key has reached its daily usage limit."
	case errors.Is(err, service.ErrAPIKeyRateLimit7dExceeded):
		return "Current API key has reached its 7-day usage limit."
	case errors.Is(err, service.ErrGroupRPMExceeded):
		return "Group requests-per-minute limit has been reached."
	case errors.Is(err, service.ErrUserRPMExceeded):
		return "User requests-per-minute limit has been reached."
	case errors.Is(err, service.ErrBillingServiceUnavailable):
		return "Billing service is temporarily unavailable."
	}
	if code != "" {
		return "Billing or rate-limit policy blocked the request."
	}
	return "Request was blocked before provider execution."
}

func billingFriendlyAction(code string, retryAfter int) string {
	switch code {
	case "subscription_limit_exceeded":
		return "Switch to an available balance/API-key entitlement if one exists, or renew/upgrade the subscription."
	case "insufficient_balance":
		return "Top up the account balance or assign a valid balance package/group, then retry."
	case "rate_limit_exceeded":
		if retryAfter > 0 {
			return fmt.Sprintf("Retry after at least %d seconds, or use another available API key/group.", retryAfter)
		}
		return "Retry after the rate-limit window resets, or use another available API key/group."
	case "billing_service_error":
		return "Retry later. If this continues, contact support with this code and timestamp."
	default:
		return "Contact support with this code and timestamp."
	}
}

// respondBillingAsAssistantMessage writes a billing error as a valid assistant
// response so OpenClaw and similar clients render actionable content instead of
// collapsing the turn into a generic "[assistant turn failed before producing content]".
//
// protocol values:
//   - "anthropic" for /v1/messages
//   - "anthropic-error" for non-generation Anthropic endpoints such as /v1/messages/count_tokens
//   - "gemini" for /v1beta/models/*:generateContent and *:streamGenerateContent
//   - "gemini-error" for non-generation Gemini model actions such as countTokens
//   - "openai" for /v1/chat/completions
//   - "openai-images" for /v1/images/generations and /v1/images/edits
//   - "openai-responses" for /v1/responses
//
// stream must match the inbound request stream flag; returning non-stream JSON to
// a streaming client can be parsed as an empty/failed assistant turn.
// Returns true if the response was written.
func respondBillingAsAssistantMessage(c *gin.Context, err error, protocol string, stream bool) bool {
	msg := billingFriendlyMessage(err)
	_, billingCode, _, retryAfter := billingErrorDetails(err)
	metadata := billingResponseMetadata(billingCode)
	metadata.RetryAfter = retryAfter
	setBillingResponseHeaders(c, metadata)
	if retryAfter > 0 {
		c.Header("Retry-After", strconv.Itoa(retryAfter))
	}

	switch protocol {
	case billingProtocolAnthropic:
		if stream {
			respondAnthropicBillingStream(c, msg, metadata)
			return true
		}
		respondAnthropicBillingMessage(c, msg, metadata)
	case billingProtocolAnthropicError:
		respondAnthropicBillingError(c, msg, metadata, err)
	case billingProtocolGemini:
		if stream {
			respondGeminiBillingStream(c, msg, metadata)
			return true
		}
		respondGeminiBillingMessage(c, msg, metadata)
	case billingProtocolGeminiError:
		respondGeminiBillingError(c, msg, metadata, err)
	case billingProtocolOpenAIChat:
		if stream {
			respondOpenAIChatBillingStream(c, msg, metadata)
			return true
		}
		respondOpenAIChatBillingMessage(c, msg, metadata)
	case billingProtocolOpenAIImages:
		if stream {
			respondOpenAIImagesBillingStream(c, msg, metadata)
			return true
		}
		respondOpenAIImagesBillingError(c, msg, metadata, err)
	case billingProtocolOpenAIResponses:
		if stream {
			respondOpenAIResponsesBillingStream(c, msg, metadata)
			return true
		}
		respondOpenAIResponsesBillingMessage(c, msg, metadata)
	default:
		return false
	}
	return true
}

type billingMetadata struct {
	Code           string
	AutoSwitchable bool
	RetryAfter     int
}

func billingResponseMetadata(code string) billingMetadata {
	return billingMetadata{
		Code:           code,
		AutoSwitchable: code == "subscription_limit_exceeded" || code == "api_key_quota_exhausted",
	}
}

func setBillingResponseHeaders(c *gin.Context, metadata billingMetadata) {
	if c == nil {
		return
	}
	if metadata.Code != "" {
		c.Header("X-Sub2API-Billing-Code", metadata.Code)
	}
	if metadata.AutoSwitchable {
		c.Header("X-Sub2API-Auto-Switchable", "true")
		return
	}
	c.Header("X-Sub2API-Auto-Switchable", "false")
}

func billingResponseMetadataBody(metadata billingMetadata) gin.H {
	body := gin.H{
		"billing_code":    metadata.Code,
		"auto_switchable": metadata.AutoSwitchable,
		"source":          "sub2api_billing",
	}
	if metadata.RetryAfter > 0 {
		body["retry_after_seconds"] = metadata.RetryAfter
	}
	return body
}

// respondAnthropicBillingMessage writes a valid Anthropic Messages API response
// with the billing message as assistant content.
func respondAnthropicBillingMessage(c *gin.Context, message string, metadata billingMetadata) {
	c.JSON(http.StatusOK, gin.H{
		"id":   fmt.Sprintf("msg_billing_%d", time.Now().UnixNano()),
		"type": "message",
		"role": "assistant",
		"content": []gin.H{
			{
				"type": "text",
				"text": message,
			},
		},
		"metadata":      billingResponseMetadataBody(metadata),
		"model":         "system",
		"stop_reason":   "end_turn",
		"stop_sequence": nil,
		"usage": gin.H{
			"input_tokens":  0,
			"output_tokens": 0,
		},
	})
}

func respondAnthropicBillingError(c *gin.Context, message string, metadata billingMetadata, err error) {
	status, code, _, _ := billingErrorDetails(err)
	c.JSON(status, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    code,
			"message": message,
		},
		"metadata": billingResponseMetadataBody(metadata),
	})
}

func respondGeminiBillingMessage(c *gin.Context, message string, metadata billingMetadata) {
	c.JSON(http.StatusOK, geminiBillingGenerateContentPayload(message, metadata))
}

func respondGeminiBillingError(c *gin.Context, message string, metadata billingMetadata, err error) {
	status, _, _, _ := billingErrorDetails(err)
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    status,
			"message": message,
			"status":  googleapi.HTTPStatusToGoogleStatus(status),
		},
		"metadata": billingResponseMetadataBody(metadata),
	})
}

func respondOpenAIImagesBillingError(c *gin.Context, message string, metadata billingMetadata, err error) {
	status, code, _, _ := billingErrorDetails(err)
	c.JSON(status, gin.H{
		"error": gin.H{
			"type":    code,
			"code":    code,
			"message": message,
		},
		"metadata": billingResponseMetadataBody(metadata),
	})
}

func geminiBillingGenerateContentPayload(message string, metadata billingMetadata) gin.H {
	return gin.H{
		"candidates": []gin.H{
			{
				"index": 0,
				"content": gin.H{
					"role": "model",
					"parts": []gin.H{
						{"text": message},
					},
				},
				"finishReason": "STOP",
			},
		},
		"metadata": billingResponseMetadataBody(metadata),
		"usageMetadata": gin.H{
			"promptTokenCount":     0,
			"candidatesTokenCount": 0,
			"totalTokenCount":      0,
		},
	}
}

func openAIWSBillingErrorPayload(err error) []byte {
	msg := billingFriendlyMessage(err)
	_, code, _, retryAfter := billingErrorDetails(err)
	metadata := billingResponseMetadata(code)
	metadata.RetryAfter = retryAfter
	payload, marshalErr := json.Marshal(gin.H{
		"event_id": "evt_sub2api_billing_blocked",
		"type":     "error",
		"error": gin.H{
			"type":    code,
			"code":    code,
			"message": msg,
		},
		"metadata": billingResponseMetadataBody(metadata),
	})
	if marshalErr != nil {
		return []byte(`{"event_id":"evt_sub2api_billing_blocked","type":"error","error":{"type":"billing_error","code":"billing_error","message":"sub2api billing blocked this request"}}`)
	}
	return payload
}

// respondOpenAIChatBillingMessage writes a valid OpenAI Chat Completions API response
// with the billing message as assistant content.
func respondOpenAIChatBillingMessage(c *gin.Context, message string, metadata billingMetadata) {
	c.JSON(http.StatusOK, gin.H{
		"id":      fmt.Sprintf("chatcmpl-billing-%d", time.Now().UnixNano()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   "system",
		"choices": []gin.H{
			{
				"index": 0,
				"message": gin.H{
					"role":    "assistant",
					"content": message,
				},
				"finish_reason": "stop",
			},
		},
		"metadata": billingResponseMetadataBody(metadata),
		"usage": gin.H{
			"prompt_tokens":     0,
			"completion_tokens": 0,
			"total_tokens":      0,
		},
	})
}

func respondOpenAIResponsesBillingMessage(c *gin.Context, message string, metadata billingMetadata) {
	responseID := fmt.Sprintf("resp_billing_%d", time.Now().UnixNano())
	messageID := fmt.Sprintf("msg_billing_%d", time.Now().UnixNano())
	c.JSON(http.StatusOK, gin.H{
		"id":         responseID,
		"object":     "response",
		"created_at": time.Now().Unix(),
		"model":      "system",
		"status":     "completed",
		"output": []gin.H{
			{
				"id":     messageID,
				"type":   "message",
				"role":   "assistant",
				"status": "completed",
				"content": []gin.H{
					{
						"type": "output_text",
						"text": message,
					},
				},
			},
		},
		"metadata": billingResponseMetadataBody(metadata),
		"usage": gin.H{
			"input_tokens":  0,
			"output_tokens": 0,
			"total_tokens":  0,
		},
	})
}

func respondAnthropicBillingStream(c *gin.Context, message string, metadata billingMetadata) {
	prepareBillingEventStream(c)
	msgID := fmt.Sprintf("msg_billing_%d", time.Now().UnixNano())
	writeSSEEvent(c, "message_start", gin.H{
		"type": "message_start",
		"message": gin.H{
			"id":            msgID,
			"type":          "message",
			"role":          "assistant",
			"model":         "system",
			"content":       []gin.H{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"metadata":      billingResponseMetadataBody(metadata),
			"usage": gin.H{
				"input_tokens":  0,
				"output_tokens": 0,
			},
		},
	})
	writeSSEEvent(c, "content_block_start", gin.H{
		"type":          "content_block_start",
		"index":         0,
		"content_block": gin.H{"type": "text", "text": ""},
	})
	writeSSEEvent(c, "content_block_delta", gin.H{
		"type":  "content_block_delta",
		"index": 0,
		"delta": gin.H{"type": "text_delta", "text": message},
	})
	writeSSEEvent(c, "content_block_stop", gin.H{"type": "content_block_stop", "index": 0})
	writeSSEEvent(c, "message_delta", gin.H{
		"type":  "message_delta",
		"delta": gin.H{"stop_reason": "end_turn", "stop_sequence": nil},
		"usage": gin.H{"output_tokens": 0},
	})
	writeSSEEvent(c, "message_stop", gin.H{"type": "message_stop"})
}

func respondOpenAIChatBillingStream(c *gin.Context, message string, metadata billingMetadata) {
	prepareBillingEventStream(c)
	id := fmt.Sprintf("chatcmpl-billing-%d", time.Now().UnixNano())
	created := time.Now().Unix()
	writeSSEData(c, gin.H{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   "system",
		"choices": []gin.H{
			{
				"index": 0,
				"delta": gin.H{
					"role":    "assistant",
					"content": message,
				},
				"finish_reason": nil,
			},
		},
		"metadata": billingResponseMetadataBody(metadata),
	})
	writeSSEData(c, gin.H{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   "system",
		"choices": []gin.H{
			{
				"index":         0,
				"delta":         gin.H{},
				"finish_reason": "stop",
			},
		},
		"metadata": billingResponseMetadataBody(metadata),
	})
	_, _ = c.Writer.WriteString("data: [DONE]\n\n")
	c.Writer.Flush()
}

func respondOpenAIResponsesBillingStream(c *gin.Context, message string, metadata billingMetadata) {
	prepareBillingEventStream(c)
	responseID := fmt.Sprintf("resp_billing_%d", time.Now().UnixNano())
	messageID := fmt.Sprintf("msg_billing_%d", time.Now().UnixNano())
	created := time.Now().Unix()
	metadataBody := billingResponseMetadataBody(metadata)
	responseBase := gin.H{
		"id":         responseID,
		"object":     "response",
		"created_at": created,
		"model":      "system",
		"metadata":   metadataBody,
	}
	writeSSEEvent(c, "response.created", gin.H{
		"type": "response.created",
		"response": mergeGinH(responseBase, gin.H{
			"status": "in_progress",
			"output": []gin.H{},
		}),
	})
	writeSSEEvent(c, "response.output_item.added", gin.H{
		"type":         "response.output_item.added",
		"output_index": 0,
		"item": gin.H{
			"id":      messageID,
			"type":    "message",
			"role":    "assistant",
			"status":  "in_progress",
			"content": []gin.H{},
		},
	})
	writeSSEEvent(c, "response.output_text.delta", gin.H{
		"type":          "response.output_text.delta",
		"item_id":       messageID,
		"output_index":  0,
		"content_index": 0,
		"delta":         message,
	})
	writeSSEEvent(c, "response.output_text.done", gin.H{
		"type":          "response.output_text.done",
		"item_id":       messageID,
		"output_index":  0,
		"content_index": 0,
		"text":          message,
	})
	messageItem := gin.H{
		"id":     messageID,
		"type":   "message",
		"role":   "assistant",
		"status": "completed",
		"content": []gin.H{
			{
				"type": "output_text",
				"text": message,
			},
		},
	}
	writeSSEEvent(c, "response.output_item.done", gin.H{
		"type":         "response.output_item.done",
		"output_index": 0,
		"item":         messageItem,
	})
	writeSSEEvent(c, "response.completed", gin.H{
		"type": "response.completed",
		"response": mergeGinH(responseBase, gin.H{
			"status": "completed",
			"output": []gin.H{messageItem},
			"usage": gin.H{
				"input_tokens":  0,
				"output_tokens": 0,
				"total_tokens":  0,
			},
		}),
	})
}

func respondGeminiBillingStream(c *gin.Context, message string, metadata billingMetadata) {
	prepareBillingEventStream(c)
	writeSSEData(c, geminiBillingGenerateContentPayload(message, metadata))
}

func respondOpenAIImagesBillingStream(c *gin.Context, message string, metadata billingMetadata) {
	prepareBillingEventStream(c)
	writeSSEEvent(c, "error", gin.H{
		"type": "error",
		"error": gin.H{
			"type":    metadata.Code,
			"code":    metadata.Code,
			"message": message,
		},
		"metadata": billingResponseMetadataBody(metadata),
	})
}

func prepareBillingEventStream(c *gin.Context) {
	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
}

func writeSSEEvent(c *gin.Context, event string, payload any) {
	if event != "" {
		_, _ = c.Writer.WriteString("event: " + event + "\n")
	}
	writeSSEData(c, payload)
}

func writeSSEData(c *gin.Context, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte(`{"type":"error","message":"failed to serialize billing response"}`)
	}
	_, _ = c.Writer.WriteString("data: " + string(data) + "\n\n")
	c.Writer.Flush()
}

func mergeGinH(base gin.H, extra gin.H) gin.H {
	out := make(gin.H, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}
