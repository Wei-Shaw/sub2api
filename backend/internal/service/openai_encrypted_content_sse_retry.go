package service

import (
	"github.com/tidwall/gjson"
	"strings"
)

// This signal is internal to an uncommitted, eligible HTTP streaming attempt.
// The caller owns the shared HTTP/SSE retry budget and request sanitization.
type openAIEncryptedContentRecoverySignal struct {
	payload   []byte
	eventType string
}

func (*openAIEncryptedContentRecoverySignal) Error() string {
	return "upstream rejected encrypted reasoning before output"
}

type openAIEncryptedContentRecoveryContextKey struct{}

func isOpenAIInvalidEncryptedContentError(code, message string) bool {
	return code == "invalid_encrypted_content" || (code == "thinking_signature_invalid" &&
		strings.Contains(message, "The encrypted content") && strings.Contains(message, "could not be verified") &&
		strings.Contains(message, "could not be decrypted or parsed"))
}

func isOpenAIInvalidEncryptedContentSSE(payload []byte, eventType string) bool {
	if !gjson.ValidBytes(payload) {
		return false
	}
	path := ""
	switch eventType {
	case "response.failed":
		path = "response.error."
	case "error":
		if gjson.GetBytes(payload, "error").IsObject() {
			path = "error."
		}
	default:
		return false
	}
	return isOpenAIInvalidEncryptedContentError(gjson.GetBytes(payload, path+"code").String(), gjson.GetBytes(payload, path+"message").String())
}

func openAIRequestHasEncryptedReasoning(body []byte) bool {
	input := gjson.GetBytes(body, "input")
	has := func(item gjson.Result) bool {
		switch strings.TrimSpace(item.Get("type").String()) {
		case "reasoning", "compaction", "compaction_summary":
			return item.Get("encrypted_content").Exists()
		default:
			return false
		}
	}
	if input.IsObject() {
		return has(input)
	}
	if input.IsArray() {
		for _, item := range input.Array() {
			if has(item) {
				return true
			}
		}
	}
	return false
}
