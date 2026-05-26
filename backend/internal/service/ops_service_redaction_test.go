package service

import (
	"encoding/json"
	"testing"
)

func TestIsSensitiveKey_TokenBudgetKeysNotRedacted(t *testing.T) {
	t.Parallel()

	for _, key := range []string{
		"max_tokens",
		"max_output_tokens",
		"max_input_tokens",
		"max_completion_tokens",
		"max_tokens_to_sample",
		"budget_tokens",
		"prompt_tokens",
		"completion_tokens",
		"input_tokens",
		"output_tokens",
		"total_tokens",
		"token_count",
	} {
		if isSensitiveKey(key) {
			t.Fatalf("expected key %q to NOT be treated as sensitive", key)
		}
	}

	for _, key := range []string{
		"authorization",
		"Authorization",
		"access_token",
		"refresh_token",
		"id_token",
		"session_token",
		"token",
		"client_secret",
		"private_key",
		"signature",
	} {
		if !isSensitiveKey(key) {
			t.Fatalf("expected key %q to be treated as sensitive", key)
		}
	}
}

func TestSanitizeAndTrimRequestBody_PreservesTokenBudgetFields(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"model":"claude-3","max_tokens":123,"thinking":{"type":"enabled","budget_tokens":456},"access_token":"abc","messages":[{"role":"user","content":"hi"}]}`)
	out, _, _ := sanitizeAndTrimRequestBody(raw, 10*1024)
	if out == "" {
		t.Fatalf("expected non-empty sanitized output")
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("unmarshal sanitized output: %v", err)
	}

	if got, ok := decoded["max_tokens"].(float64); !ok || got != 123 {
		t.Fatalf("expected max_tokens=123, got %#v", decoded["max_tokens"])
	}

	thinking, ok := decoded["thinking"].(map[string]any)
	if !ok || thinking == nil {
		t.Fatalf("expected thinking object to be preserved, got %#v", decoded["thinking"])
	}
	if got, ok := thinking["budget_tokens"].(float64); !ok || got != 456 {
		t.Fatalf("expected thinking.budget_tokens=456, got %#v", thinking["budget_tokens"])
	}

	if got := decoded["access_token"]; got != "[REDACTED]" {
		t.Fatalf("expected access_token to be redacted, got %#v", got)
	}
}

func TestShrinkToEssentials_IncludesThinking(t *testing.T) {
	t.Parallel()

	root := map[string]any{
		"model":      "claude-3",
		"max_tokens": 100,
		"thinking": map[string]any{
			"type":          "enabled",
			"budget_tokens": 200,
		},
		"messages": []any{
			map[string]any{"role": "user", "content": "first"},
			map[string]any{"role": "user", "content": "last"},
		},
	}

	out := shrinkToEssentials(root)
	if _, ok := out["thinking"]; !ok {
		t.Fatalf("expected thinking to be included in essentials: %#v", out)
	}
}
