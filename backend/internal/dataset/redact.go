package dataset

import (
	"regexp"
)

var (
	// Regex patterns for PII detection
	emailPattern      = regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`)
	phonePattern      = regexp.MustCompile(`\b\d{3}[-.\s]?\d{3}[-.\s]?\d{4}\b`)
	creditCardPattern = regexp.MustCompile(`\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}\b`)

	// Sensitive header keys
	sensitiveHeaders = map[string]bool{
		"authorization": true,
		"x-api-key":     true,
		"cookie":        true,
		"api-key":       true,
		"auth-token":    true,
	}
)

// RedactSensitiveFields removes PII and credentials from a dataset entry.
// Designed to be safe: operates on copies and fails gracefully.
func RedactSensitiveFields(entry *DatasetEntry) {
	if entry == nil {
		return
	}

	// Redact request headers
	if headers, ok := entry.Request["headers"].(map[string]any); ok {
		for key := range headers {
			if sensitiveHeaders[key] {
				delete(headers, key)
			}
		}
	}

	// Redact response content
	if msg, ok := entry.Response["message"].(map[string]any); ok {
		if content, ok := msg["content"].(string); ok {
			msg["content"] = redactPII(content)
		}
	}

	// Redact request message content
	if messages, ok := entry.Request["messages"].([]map[string]any); ok {
		for i := range messages {
			if content, ok := messages[i]["content"].(string); ok {
				messages[i]["content"] = redactPII(content)
			}
		}
	}
}

// redactPII masks common PII patterns in text.
func redactPII(text string) string {
	text = emailPattern.ReplaceAllString(text, "[EMAIL_REDACTED]")
	text = phonePattern.ReplaceAllString(text, "[PHONE_REDACTED]")
	text = creditCardPattern.ReplaceAllString(text, "[CARD_REDACTED]")
	return text
}
