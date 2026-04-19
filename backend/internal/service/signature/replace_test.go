//go:build unit

package signature

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/tidwall/gjson"
)

func TestReplaceThinkingSignaturesInBody_CyclesPoolForMGreaterThanN(t *testing.T) {
	body := []byte(`{
		"model": "claude-opus-4-7",
		"messages": [
			{"role": "assistant", "content": [
				{"type": "thinking", "thinking": "a", "signature": "bad1"},
				{"type": "thinking", "thinking": "b", "signature": "bad2"},
				{"type": "thinking", "thinking": "c", "signature": "bad3"},
				{"type": "thinking", "thinking": "d", "signature": "bad4"},
				{"type": "thinking", "thinking": "e", "signature": "bad5"}
			]}
		]
	}`)
	pool := []string{"good0", "good1"}

	out, replaced := ReplaceThinkingSignaturesInBody(body, pool)
	if replaced != 5 {
		t.Fatalf("expected 5 replacements, got %d", replaced)
	}

	sigs := gjson.GetBytes(out, "messages.0.content.#.signature").Array()
	if len(sigs) != 5 {
		t.Fatalf("expected 5 signatures in result, got %d", len(sigs))
	}
	// Expected cycle: good0, good1, good0, good1, good0
	want := []string{"good0", "good1", "good0", "good1", "good0"}
	for i, s := range sigs {
		if s.String() != want[i] {
			t.Errorf("sig[%d]: got %q, want %q", i, s.String(), want[i])
		}
	}
}

func TestReplaceThinkingSignaturesInBody_EmptyPool(t *testing.T) {
	body := []byte(`{"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"x","signature":"bad"}]}]}`)
	out, n := ReplaceThinkingSignaturesInBody(body, nil)
	if n != 0 {
		t.Fatalf("expected 0 replacements on empty pool, got %d", n)
	}
	if string(out) != string(body) {
		t.Fatalf("empty pool must not modify body")
	}
}

func TestReplaceThinkingSignaturesInBody_NoThinkingBlocks(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"hi"}]}`)
	out, n := ReplaceThinkingSignaturesInBody(body, []string{"g"})
	if n != 0 {
		t.Fatalf("expected 0 replacements when no thinking blocks, got %d", n)
	}
	if string(out) != string(body) {
		t.Fatalf("body must be unchanged")
	}
}

func TestReplaceThinkingSignaturesInBody_SkipsEmptySignatureAndOtherTypes(t *testing.T) {
	body := []byte(`{
		"messages": [
			{"role":"assistant","content":[
				{"type":"text","text":"hello"},
				{"type":"thinking","thinking":"keep-empty-sig","signature":""},
				{"type":"redacted_thinking","data":"xyz"},
				{"type":"thinking","thinking":"replace-me","signature":"bad"}
			]}
		]
	}`)
	pool := []string{"good"}
	out, n := ReplaceThinkingSignaturesInBody(body, pool)
	if n != 1 {
		t.Fatalf("expected 1 replacement, got %d", n)
	}
	// Empty signature stays empty (not replaced)
	if gjson.GetBytes(out, "messages.0.content.1.signature").String() != "" {
		t.Errorf("empty signature must be preserved unchanged")
	}
	// Real bad signature gets replaced
	if gjson.GetBytes(out, "messages.0.content.3.signature").String() != "good" {
		t.Errorf("thinking signature should be replaced with pool entry")
	}
}

func TestReplaceThinkingSignaturesInClaudeRequest_Cycles(t *testing.T) {
	req := &antigravity.ClaudeRequest{
		Messages: []antigravity.ClaudeMessage{
			{
				Role: "assistant",
				Content: json.RawMessage(`[
					{"type":"thinking","thinking":"a","signature":"bad1"},
					{"type":"thinking","thinking":"b","signature":"bad2"},
					{"type":"thinking","thinking":"c","signature":"bad3"}
				]`),
			},
		},
	}
	pool := []string{"G0", "G1"}
	replaced, err := ReplaceThinkingSignaturesInClaudeRequest(req, pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if replaced != 3 {
		t.Fatalf("expected 3 replacements, got %d", replaced)
	}
	sigs := gjson.GetBytes(req.Messages[0].Content, "#.signature").Array()
	want := []string{"G0", "G1", "G0"}
	for i, s := range sigs {
		if s.String() != want[i] {
			t.Errorf("sig[%d]: got %q, want %q", i, s.String(), want[i])
		}
	}
}

func TestReplaceThinkingSignaturesInClaudeRequest_StringContentUntouched(t *testing.T) {
	req := &antigravity.ClaudeRequest{
		Messages: []antigravity.ClaudeMessage{
			{Role: "user", Content: json.RawMessage(`"plain text"`)},
		},
	}
	replaced, err := ReplaceThinkingSignaturesInClaudeRequest(req, []string{"x"})
	if err != nil || replaced != 0 {
		t.Fatalf("expected 0 replacements for string content, got replaced=%d err=%v", replaced, err)
	}
}

func TestReplaceThinkingSignaturesInClaudeRequest_NilPoolNoop(t *testing.T) {
	orig := json.RawMessage(`[{"type":"thinking","thinking":"a","signature":"bad"}]`)
	req := &antigravity.ClaudeRequest{
		Messages: []antigravity.ClaudeMessage{{Role: "assistant", Content: orig}},
	}
	replaced, err := ReplaceThinkingSignaturesInClaudeRequest(req, nil)
	if err != nil || replaced != 0 {
		t.Fatalf("expected 0 replacements for nil pool, got replaced=%d err=%v", replaced, err)
	}
	if string(req.Messages[0].Content) != string(orig) {
		t.Errorf("content must be unchanged for nil pool")
	}
}

func TestReplaceThinkingSignaturesInBody_MultipleAssistantMessages(t *testing.T) {
	body := []byte(`{
		"messages": [
			{"role":"user","content":"hi"},
			{"role":"assistant","content":[
				{"type":"thinking","thinking":"a","signature":"bad1"}
			]},
			{"role":"user","content":"follow up"},
			{"role":"assistant","content":[
				{"type":"text","text":"ok"},
				{"type":"thinking","thinking":"b","signature":"bad2"},
				{"type":"thinking","thinking":"c","signature":"bad3"}
			]}
		]
	}`)
	pool := []string{"G0", "G1"}
	out, replaced := ReplaceThinkingSignaturesInBody(body, pool)
	if replaced != 3 {
		t.Fatalf("expected 3 replacements across messages, got %d", replaced)
	}
	// msg[1].content[0] → G0, msg[3].content[1] → G1, msg[3].content[2] → G0
	if got := gjson.GetBytes(out, "messages.1.content.0.signature").String(); got != "G0" {
		t.Errorf("msg1 sig: got %q, want G0", got)
	}
	if got := gjson.GetBytes(out, "messages.3.content.1.signature").String(); got != "G1" {
		t.Errorf("msg3 sig[1]: got %q, want G1", got)
	}
	if got := gjson.GetBytes(out, "messages.3.content.2.signature").String(); got != "G0" {
		t.Errorf("msg3 sig[2]: got %q, want G0 (cycle)", got)
	}
}

func TestReplaceThinkingSignaturesInClaudeRequest_MalformedContentSkipped(t *testing.T) {
	req := &antigravity.ClaudeRequest{
		Messages: []antigravity.ClaudeMessage{
			{Role: "assistant", Content: json.RawMessage(`[{"type":"thinking","thinking":"ok","signature":"bad"}]`)},
			{Role: "assistant", Content: json.RawMessage(`[{"type":`)}, // malformed
			{Role: "assistant", Content: json.RawMessage(`[{"type":"thinking","thinking":"y","signature":"bad2"}]`)},
		},
	}
	replaced, err := ReplaceThinkingSignaturesInClaudeRequest(req, []string{"G0", "G1"})
	if err != nil {
		t.Fatalf("malformed content should be skipped, not error: %v", err)
	}
	if replaced != 2 {
		t.Fatalf("expected 2 replacements (skip malformed), got %d", replaced)
	}
}
