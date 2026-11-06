package apicompat

import (
	"encoding/json"
	"testing"
)

// A replayed thinking block becomes a reasoning input item, and the Responses API rejects such an item
// when "summary" is absent (400 "`input[n]` missing required field `summary`").
func TestAnthropicAssistantToResponses_ReasoningItemHasSummary(t *testing.T) {
	raw := json.RawMessage(`[{"type":"thinking","thinking":"","signature":"opaque-sig"},{"type":"text","text":"hi"}]`)
	items, err := anthropicAssistantToResponses(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 || items[0].Type != "reasoning" {
		t.Fatalf("expected a leading reasoning item, got %+v", items)
	}
	out, err := json.Marshal(items[0])
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if string(m["summary"]) != "[]" {
		t.Fatalf("reasoning item must carry summary:[] , got %s", out)
	}
	if string(m["encrypted_content"]) != `"opaque-sig"` {
		t.Fatalf("encrypted_content lost: %s", out)
	}
}
