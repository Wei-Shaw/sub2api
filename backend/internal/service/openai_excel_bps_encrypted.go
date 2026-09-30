package service

import (
	"encoding/json"
	"strings"

	"github.com/tidwall/gjson"
)

// Only an explicit invalid-ciphertext rejection permits this recovery. Ordinary
// 400s, authentication, quota and transport errors must not replay a BPS request.
func isExcelBPSInvalidEncryptedContent(raw []byte) bool {
	if !gjson.ValidBytes(raw) {
		return false
	}
	if code := extractUpstreamErrorCode(raw); code != "" {
		return code == "invalid_encrypted_content"
	}
	// Accept the same diagnostic when a provider omits its error code. Do not classify
	// arbitrary mentions of encryption (or echoed request text) as this error.
	message := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(raw)))
	return strings.HasPrefix(message, "the encrypted content ") &&
		strings.Contains(message, "could not be verified") &&
		strings.Contains(message, "could not be decrypted or parsed")
}

// Prepare a single same-route retry from the already translated wire request.
// Drop only opaque reasoning, preserving messages, tools/results, attachments and
// routing metadata without decoding numbers or string contents. Compaction and
// encrypted message bodies may be the only copy of user context: never discard
// them to make a request pass.
func prepareExcelBPSInvalidEncryptedRetry(body, rejection []byte) ([]byte, bool) {
	if !isExcelBPSInvalidEncryptedContent(rejection) {
		return body, false
	}
	var request map[string]json.RawMessage
	if json.Unmarshal(body, &request) != nil {
		return body, false
	}
	var input []json.RawMessage
	if json.Unmarshal(request["input"], &input) != nil {
		return body, false
	}
	kept := make([]json.RawMessage, 0, len(input))
	removed := false
	hasHistory := false
	for _, item := range input {
		typeName := gjson.GetBytes(item, "type").String()
		encrypted := gjson.GetBytes(item, "encrypted_content")
		if typeName == "reasoning" && encrypted.Type == gjson.String && encrypted.String() != "" {
			removed = true
			continue
		}
		if encrypted.Exists() || len(gjson.GetBytes(item, "encrypted_function_args").Array()) > 0 {
			return body, false
		}
		for _, field := range []string{"content", "output"} {
			for _, part := range gjson.GetBytes(item, field).Array() {
				if part.Get("type").String() == "encrypted_content" || part.Get("encrypted_content").Exists() {
					return body, false
				}
			}
		}
		// Injected developer instructions and a compaction trigger alone are
		// not enough context to regenerate a user's task.
		role := gjson.GetBytes(item, "role").String()
		if role == "user" || role == "assistant" || typeName == "agent_message" ||
			typeName == "function_call" || typeName == "function_call_output" {
			hasHistory = true
		}
		kept = append(kept, item)
	}
	if !removed || !hasHistory {
		return body, false
	}
	var err error
	request["input"], err = json.Marshal(kept)
	if err != nil {
		return body, false
	}
	retry, err := json.Marshal(request)
	if err != nil {
		return body, false
	}
	return retry, true
}
