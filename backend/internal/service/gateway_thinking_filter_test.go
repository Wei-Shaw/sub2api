//go:build unit

package service

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestFilterInvalidSignatureThinkingBlocks_PreservesValidSignature(t *testing.T) {
	body := []byte(`{"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"reasoned step","signature":"AbCdEf+/=ValidBase64Sig=="}]}]}`)
	out := FilterInvalidSignatureThinkingBlocks(body)
	got := gjson.GetBytes(out, "messages.0.content.0.type").String()
	if got != "thinking" {
		t.Fatalf("valid thinking block removed: %s", out)
	}
	sig := gjson.GetBytes(out, "messages.0.content.0.signature").String()
	if sig != "AbCdEf+/=ValidBase64Sig==" {
		t.Fatalf("signature mutated: %s", sig)
	}
}

func TestFilterInvalidSignatureThinkingBlocks_RemovesMissingSignature(t *testing.T) {
	body := []byte(`{"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"reasoned"},{"type":"text","text":"hello"}]}]}`)
	out := FilterInvalidSignatureThinkingBlocks(body)
	if count := gjson.GetBytes(out, "messages.0.content.#").Int(); count != 1 {
		t.Fatalf("expected 1 content block after strip, got %d: %s", count, out)
	}
	if got := gjson.GetBytes(out, "messages.0.content.0.type").String(); got != "text" {
		t.Fatalf("expected text to remain, got: %s (body=%s)", got, out)
	}
}

func TestFilterInvalidSignatureThinkingBlocks_RemovesEmptySignature(t *testing.T) {
	body := []byte(`{"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"reasoned","signature":""}]}]}`)
	out := FilterInvalidSignatureThinkingBlocks(body)
	if count := gjson.GetBytes(out, "messages.0.content.#").Int(); count != 0 {
		t.Fatalf("empty-signature thinking block not removed: %s", out)
	}
}

func TestFilterInvalidSignatureThinkingBlocks_AlwaysRemovesRedactedThinking(t *testing.T) {
	body := []byte(`{"messages":[{"role":"assistant","content":[{"type":"redacted_thinking","data":"some opaque bytes"}]}]}`)
	out := FilterInvalidSignatureThinkingBlocks(body)
	if count := gjson.GetBytes(out, "messages.0.content.#").Int(); count != 0 {
		t.Fatalf("redacted_thinking not removed: %s", out)
	}
}

func TestFilterInvalidSignatureThinkingBlocks_LeavesNonThinkingUntouched(t *testing.T) {
	in := `{"model":"claude-opus","messages":[{"role":"user","content":"hi"},{"role":"assistant","content":[{"type":"text","text":"hello"}]}]}`
	body := []byte(in)
	out := FilterInvalidSignatureThinkingBlocks(body)
	if string(out) != in {
		t.Fatalf("non-thinking body mutated:\nin:  %s\nout: %s", in, out)
	}
}

func TestFilterInvalidSignatureThinkingBlocks_MalformedSignatureRemoved(t *testing.T) {
	body := []byte(`{"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"reasoned","signature":"!!!not-base64!!!"}]}]}`)
	out := FilterInvalidSignatureThinkingBlocks(body)
	if count := gjson.GetBytes(out, "messages.0.content.#").Int(); count != 0 {
		t.Fatalf("malformed-signature thinking block not removed: %s", out)
	}
}

func TestFilterInvalidSignatureThinkingBlocks_UserMessagesUntouched(t *testing.T) {
	// Only assistant-role messages should have thinking blocks stripped.
	in := `{"messages":[{"role":"user","content":[{"type":"thinking","thinking":"sketchy","signature":""}]}]}`
	body := []byte(in)
	out := FilterInvalidSignatureThinkingBlocks(body)
	if string(out) != in {
		t.Fatalf("user-role content mutated: %s", out)
	}
}

func TestFilterInvalidSignatureThinkingBlocks_EmptyAssistantMessageDropped(t *testing.T) {
	// Assistant message whose only block had a bad signature → whole message removed.
	body := []byte(`{"messages":[{"role":"user","content":"hi"},{"role":"assistant","content":[{"type":"thinking","thinking":"x"}]}]}`)
	out := FilterInvalidSignatureThinkingBlocks(body)
	if count := gjson.GetBytes(out, "messages.#").Int(); count != 1 {
		t.Fatalf("expected 1 message after dropping empty assistant, got %d: %s", count, out)
	}
	if role := gjson.GetBytes(out, "messages.0.role").String(); role != "user" {
		t.Fatalf("remaining message role should be user, got %s: %s", role, out)
	}
}

func TestFilterInvalidSignatureThinkingBlocks_FastPathNoThinking(t *testing.T) {
	// Body with no thinking markers at all should be returned byte-for-byte.
	in := `{"messages":[{"role":"user","content":"hello"}],"model":"x"}`
	body := []byte(in)
	out := FilterInvalidSignatureThinkingBlocks(body)
	if &body[0] != &out[0] {
		t.Fatalf("fast-path should return the same slice header")
	}
}

func TestFilterInvalidSignatureThinkingBlocks_RemovesEmptyThinkingContentWithValidSignature(t *testing.T) {
	// Valid-looking signature but empty thinking content should be removed.
	// Upstream rejects: "each thinking block must contain thinking"
	body := []byte(`{"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"","signature":"AbCdEfGhIjKlMnOpQrStUvWx"},{"type":"text","text":"answer"}]}]}`)
	out := FilterInvalidSignatureThinkingBlocks(body)
	if count := gjson.GetBytes(out, "messages.0.content.#").Int(); count != 1 {
		t.Fatalf("expected 1 content block after strip, got %d: %s", count, out)
	}
	if got := gjson.GetBytes(out, "messages.0.content.0.type").String(); got != "text" {
		t.Fatalf("expected text to remain, got: %s (body=%s)", got, out)
	}
	if txt := gjson.GetBytes(out, "messages.0.content.0.text").String(); txt != "answer" {
		t.Fatalf("text content mismatch: %s", txt)
	}
}
