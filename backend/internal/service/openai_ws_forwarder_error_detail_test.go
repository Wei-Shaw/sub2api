package service

import (
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func TestEnsureOpenAIWSErrorEventClientDetail(t *testing.T) {
	t.Run("injects retryable detail into empty error object", func(t *testing.T) {
		message := []byte(`{"type":"error","error":{}}`)
		updated, changed := ensureOpenAIWSErrorEventClientDetail(message)
		if !changed {
			t.Fatalf("expected empty error event to be rewritten")
		}
		if got := gjson.GetBytes(updated, "error.code").String(); got != openAICapacityShedRetryableClientCode {
			t.Fatalf("error.code = %q, want %q", got, openAICapacityShedRetryableClientCode)
		}
		if got := gjson.GetBytes(updated, "error.message").String(); !strings.Contains(got, "You can retry your request") {
			t.Fatalf("error.message = %q, want retry guidance", got)
		}
		if got := gjson.GetBytes(updated, "type").String(); got != "error" {
			t.Fatalf("event type mutated to %q", got)
		}
	})

	t.Run("injects when error object is missing entirely", func(t *testing.T) {
		message := []byte(`{"type":"error"}`)
		updated, changed := ensureOpenAIWSErrorEventClientDetail(message)
		if !changed {
			t.Fatalf("expected error event without error object to be rewritten")
		}
		if got := gjson.GetBytes(updated, "error.message").String(); got == "" {
			t.Fatalf("expected injected error.message")
		}
	})

	t.Run("keeps upstream detail verbatim", func(t *testing.T) {
		message := []byte(`{"type":"error","error":{"code":"rate_limit_exceeded","message":"slow down"}}`)
		updated, changed := ensureOpenAIWSErrorEventClientDetail(message)
		if changed {
			t.Fatalf("expected populated error event to pass through unchanged")
		}
		if string(updated) != string(message) {
			t.Fatalf("payload mutated: %s", updated)
		}
	})

	t.Run("keeps events with only a type populated", func(t *testing.T) {
		message := []byte(`{"type":"error","error":{"type":"server_error"}}`)
		if _, changed := ensureOpenAIWSErrorEventClientDetail(message); changed {
			t.Fatalf("expected error event with populated type to pass through unchanged")
		}
	})

	t.Run("leaves invalid json untouched", func(t *testing.T) {
		message := []byte(`not-json`)
		updated, changed := ensureOpenAIWSErrorEventClientDetail(message)
		if changed || string(updated) != "not-json" {
			t.Fatalf("expected invalid payload to pass through unchanged")
		}
	})
}
