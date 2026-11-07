package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
)

type messagesReasoningReplayContextKey struct{}

const messagesReasoningReplayRetryAllowedKey = "messages_reasoning_replay_retry_allowed"

var errMessagesReasoningReplay = errors.New("messages encrypted reasoning replay rejected before client output")

func messagesReasoningReplayRetried(ctx context.Context) bool {
	tried, _ := ctx.Value(messagesReasoningReplayContextKey{}).(bool)
	return tried
}

func markMessagesReasoningReplayRetried(ctx context.Context) context.Context {
	return context.WithValue(ctx, messagesReasoningReplayContextKey{}, true)
}

// Only explicit codes at the documented HTTP statuses qualify. In particular,
// a generic gateway 502 or message merely mentioning ciphertext is not enough.
// SSE callers use 502 as the local semantic error status, before any output.
func isMessagesReasoningReplayError(status int, body []byte) bool {
	if status != http.StatusBadRequest && status != http.StatusBadGateway {
		return false
	}
	// Some compatible gateways nest a JSON error document in error.message.
	// The shared extractor still requires an explicit code, not a text match.
	if extractUpstreamErrorCode(body) == "invalid_encrypted_content" {
		return true
	}
	for _, path := range []string{"error", "response.error", ""} {
		errObj := gjson.ParseBytes(body)
		if path != "" {
			errObj = errObj.Get(path)
		}
		code := errObj.Get("code").String()
		if code == "invalid_encrypted_content" {
			return true
		}
		if code == "thinking_signature_invalid" {
			msg := errObj.Get("message").String()
			return strings.Contains(msg, "The encrypted content") &&
				strings.Contains(msg, "could not be verified") &&
				strings.Contains(msg, "could not be decrypted or parsed")
		}
	}
	return false
}

func messagesRequestHasEncryptedReasoning(body []byte) bool {
	input := gjson.GetBytes(body, "input")
	if input.IsObject() {
		return input.Get("type").String() == "reasoning" && strings.TrimSpace(input.Get("encrypted_content").String()) != ""
	}
	for _, item := range input.Array() {
		if item.Get("type").String() == "reasoning" && strings.TrimSpace(item.Get("encrypted_content").String()) != "" {
			return true
		}
	}
	return false
}

// dropClearlyForeignAnthropicThinking removes only recognizably OpenAI thinking
// blocks from native Anthropic history. Never invent a native signature or touch
// arbitrary opaque/native signatures (including Gemini's existing safeguards).
// Raw messages/blocks preserve unknown fields and tool inputs without float loss.
func dropClearlyForeignAnthropicThinking(body []byte) ([]byte, bool) {
	var root map[string]json.RawMessage
	if json.Unmarshal(body, &root) != nil {
		return body, false
	}
	var messages []map[string]json.RawMessage
	if json.Unmarshal(root["messages"], &messages) != nil {
		return body, false
	}
	changed := false
	emptyAfterRemoval := make(map[int]bool)
	for index, msg := range messages {
		if string(msg["role"]) != `"assistant"` {
			continue
		}
		var blocks []json.RawMessage
		if json.Unmarshal(msg["content"], &blocks) != nil {
			continue
		}
		kept := make([]json.RawMessage, 0, len(blocks))
		removed := false
		for _, block := range blocks {
			if gjson.GetBytes(block, "type").String() == "thinking" && strings.HasPrefix(strings.TrimSpace(gjson.GetBytes(block, "signature").String()), "gAAAA") {
				removed = true
				continue
			}
			kept = append(kept, block)
		}
		if removed {
			// An empty assistant turn is invalid. Remove the entire turn below
			// rather than fabricating text or a native signed thinking block.
			msg["content"], _ = json.Marshal(kept)
			emptyAfterRemoval[index] = len(kept) == 0
			changed = true
		}
	}
	if !changed {
		return body, false
	}
	keptMessages := make([]map[string]json.RawMessage, 0, len(messages))
	for index, msg := range messages {
		if emptyAfterRemoval[index] {
			continue
		}
		keptMessages = append(keptMessages, msg)
	}
	root["messages"], _ = json.Marshal(keptMessages)
	out, err := json.Marshal(root)
	if err != nil {
		return body, false
	}
	return out, true
}
