package handler

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// billingFriendlyMessage returns a user-facing message for billing errors.
// Returns empty string if the error is not a billing insufficiency error
// (i.e. not something the user can fix by topping up or renewing).
func billingFriendlyMessage(err error) string {
	if errors.Is(err, service.ErrInsufficientBalance) {
		return "Your account balance is insufficient. Please top up your balance to continue using the service."
	}
	if errors.Is(err, service.ErrSubscriptionInvalid) {
		return "Your subscription is invalid or has expired. Please renew your subscription to continue using the service."
	}
	if errors.Is(err, service.ErrSubscriptionExpired) {
		return "Your subscription has expired. Please renew your subscription to continue using the service."
	}
	if errors.Is(err, service.ErrDailyLimitExceeded) {
		return "You have reached your daily usage limit. Please try again tomorrow or upgrade your plan."
	}
	if errors.Is(err, service.ErrWeeklyLimitExceeded) {
		return "You have reached your weekly usage limit. Please try again next week or upgrade your plan."
	}
	if errors.Is(err, service.ErrMonthlyLimitExceeded) {
		return "You have reached your monthly usage limit. Please renew or upgrade your plan to continue."
	}
	return ""
}

// respondBillingAsAssistantMessage writes a billing error as a valid assistant message
// so that OpenClaw (and similar clients) can render it in the chat UI instead of showing
// a generic "[assistant turn failed before producing content]" error.
//
// protocol: "anthropic" for /v1/messages, "openai" for /v1/chat/completions
// Returns true if the response was written (caller should return), false if not applicable.
func respondBillingAsAssistantMessage(c *gin.Context, err error, protocol string) bool {
	msg := billingFriendlyMessage(err)
	if msg == "" {
		return false
	}
	_, billingCode, _, _ := billingErrorDetails(err)
	metadata := billingResponseMetadata(billingCode)
	setBillingResponseHeaders(c, metadata)

	switch protocol {
	case "anthropic":
		respondAnthropicBillingMessage(c, msg, metadata)
	case "openai":
		respondOpenAIChatBillingMessage(c, msg, metadata)
	default:
		return false
	}
	return true
}

type billingMetadata struct {
	Code           string
	AutoSwitchable bool
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
	return gin.H{
		"billing_code":    metadata.Code,
		"auto_switchable": metadata.AutoSwitchable,
	}
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
