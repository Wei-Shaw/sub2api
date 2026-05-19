package service

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestStripRedactedThinkingBlocks_RemovesRedactedThinking(t *testing.T) {
	body := []byte(`{"messages":[{"role":"assistant","content":[{"type":"redacted_thinking","data":"encrypted_data_here"},{"type":"text","text":"hello"}]}]}`)
	out := StripRedactedThinkingBlocks(body)

	count := gjson.GetBytes(out, "messages.0.content.#").Int()
	if count != 1 {
		t.Fatalf("expected 1 content block after strip, got %d: %s", count, out)
	}
	blockType := gjson.GetBytes(out, "messages.0.content.0.type").String()
	if blockType != "text" {
		t.Fatalf("expected remaining block to be text, got %s", blockType)
	}
}

func TestStripRedactedThinkingBlocks_PreservesNonRedacted(t *testing.T) {
	body := []byte(`{"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"reasoning","signature":"AbCdEf=="},{"type":"text","text":"hello"}]}]}`)
	out := StripRedactedThinkingBlocks(body)

	// Should be unchanged
	count := gjson.GetBytes(out, "messages.0.content.#").Int()
	if count != 2 {
		t.Fatalf("expected 2 content blocks (unchanged), got %d: %s", count, out)
	}
}

func TestStripRedactedThinkingBlocks_UserMessagesUntouched(t *testing.T) {
	// Only assistant messages should be affected
	in := `{"messages":[{"role":"user","content":[{"type":"redacted_thinking","data":"should_stay"}]}]}`
	body := []byte(in)
	out := StripRedactedThinkingBlocks(body)

	if string(out) != in {
		t.Fatalf("user-role content mutated:\nin:  %s\nout: %s", in, out)
	}
}

func TestStripRedactedThinkingBlocks_EmptyContentGetsPlaceholder(t *testing.T) {
	// Assistant message with only redacted_thinking → gets placeholder
	body := []byte(`{"messages":[{"role":"assistant","content":[{"type":"redacted_thinking","data":"encrypted"}]}]}`)
	out := StripRedactedThinkingBlocks(body)

	count := gjson.GetBytes(out, "messages.0.content.#").Int()
	if count != 1 {
		t.Fatalf("expected 1 placeholder block, got %d: %s", count, out)
	}
	text := gjson.GetBytes(out, "messages.0.content.0.text").String()
	if text == "" {
		t.Fatalf("expected non-empty placeholder text, got empty")
	}
}

func TestStripRedactedThinkingBlocks_FastPathNoRedacted(t *testing.T) {
	// Body with no redacted_thinking markers should be returned byte-for-byte
	in := `{"messages":[{"role":"user","content":"hello"}],"model":"x"}`
	body := []byte(in)
	out := StripRedactedThinkingBlocks(body)

	if &body[0] != &out[0] {
		t.Fatalf("fast-path should return the same slice header")
	}
}

func TestStripRedactedThinkingBlocks_MultipleMessages(t *testing.T) {
	body := []byte(`{"messages":[
		{"role":"user","content":"hi"},
		{"role":"assistant","content":[{"type":"redacted_thinking","data":"enc1"},{"type":"text","text":"response1"}]},
		{"role":"user","content":"follow up"},
		{"role":"assistant","content":[{"type":"redacted_thinking","data":"enc2"},{"type":"redacted_thinking","data":"enc3"},{"type":"text","text":"response2"}]}
	]}`)
	out := StripRedactedThinkingBlocks(body)

	// First assistant message: 1 block (text only)
	count1 := gjson.GetBytes(out, "messages.1.content.#").Int()
	if count1 != 1 {
		t.Fatalf("expected 1 block in first assistant msg, got %d", count1)
	}

	// Second assistant message: 1 block (text only, both redacted_thinking removed)
	count2 := gjson.GetBytes(out, "messages.3.content.#").Int()
	if count2 != 1 {
		t.Fatalf("expected 1 block in second assistant msg, got %d", count2)
	}
}
