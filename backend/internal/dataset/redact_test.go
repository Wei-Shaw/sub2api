package dataset

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRedactSensitiveFields_RemovesAuthHeaders(t *testing.T) {
	entry := &DatasetEntry{
		Timestamp: time.Now(),
		Endpoint:  "chat.completions",
		Request: map[string]any{
			"headers": map[string]any{
				"authorization": "Bearer sk-12345",
				"x-api-key":     "secret-key",
				"content-type":  "application/json",
			},
		},
	}

	RedactSensitiveFields(entry)

	headers, ok := entry.Request["headers"].(map[string]any)
	require.True(t, ok)
	require.NotContains(t, headers, "authorization")
	require.NotContains(t, headers, "x-api-key")
	require.Contains(t, headers, "content-type", "non-sensitive headers should remain")
}

func TestRedactPII_MasksEmail(t *testing.T) {
	text := "Contact me at user@example.com or admin@test.org"
	redacted := redactPII(text)

	require.NotContains(t, redacted, "user@example.com")
	require.NotContains(t, redacted, "admin@test.org")
	require.Contains(t, redacted, "[EMAIL_REDACTED]")
}

func TestRedactPII_MasksPhone(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"dash format", "Call 123-456-7890"},
		{"dot format", "Phone: 123.456.7890"},
		{"space format", "Mobile 123 456 7890"},
		{"no separator", "Contact 1234567890"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redacted := redactPII(tt.input)
			require.Contains(t, redacted, "[PHONE_REDACTED]")
		})
	}
}

func TestRedactPII_MasksCreditCard(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"dash format", "Card: 1234-5678-9012-3456"},
		{"space format", "Payment 1234 5678 9012 3456"},
		{"no separator", "CC: 1234567890123456"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redacted := redactPII(tt.input)
			require.Contains(t, redacted, "[CARD_REDACTED]")
		})
	}
}

func TestRedactSensitiveFields_HandlesMessageContent(t *testing.T) {
	entry := &DatasetEntry{
		Timestamp: time.Now(),
		Endpoint:  "chat.completions",
		Request: map[string]any{
			"messages": []map[string]any{
				{
					"role":    "user",
					"content": "My email is test@example.com and phone is 123-456-7890",
				},
			},
		},
		Response: map[string]any{
			"message": map[string]any{
				"role":    "assistant",
				"content": "Sure, I'll send it to admin@company.com",
			},
		},
	}

	RedactSensitiveFields(entry)

	// Check request message redaction
	messages, ok := entry.Request["messages"].([]map[string]any)
	require.True(t, ok)
	reqContent, ok := messages[0]["content"].(string)
	require.True(t, ok)
	require.Contains(t, reqContent, "[EMAIL_REDACTED]")
	require.Contains(t, reqContent, "[PHONE_REDACTED]")

	// Check response message redaction
	respMsg, ok := entry.Response["message"].(map[string]any)
	require.True(t, ok)
	respContent, ok := respMsg["content"].(string)
	require.True(t, ok)
	require.Contains(t, respContent, "[EMAIL_REDACTED]")
}

func TestRedactSensitiveFields_GracefullyHandlesNilOrMissingFields(t *testing.T) {
	tests := []struct {
		name  string
		entry *DatasetEntry
	}{
		{"nil entry", nil},
		{"empty request", &DatasetEntry{Request: map[string]any{}}},
		{"no headers", &DatasetEntry{Request: map[string]any{"model": "gpt-4"}}},
		{"no response", &DatasetEntry{Request: map[string]any{}, Response: map[string]any{}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotPanics(t, func() {
				RedactSensitiveFields(tt.entry)
			})
		})
	}
}
