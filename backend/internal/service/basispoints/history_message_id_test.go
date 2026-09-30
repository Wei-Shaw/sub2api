package basispoints

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestInlineHistoryOmitsCompatibilityMessageID(t *testing.T) {
	const legacyID = "item_0123456789abcdef01234567"
	for _, typed := range []bool{false, true} {
		for _, attributed := range []bool{false, true} {
			source := testSource()
			item := object{"role": "assistant", "id": legacyID, "status": "completed", "phase": "commentary", "content": []any{object{"type": "output_text", "text": "Keep the literal " + legacyID + " unchanged.", "annotations": []any{}}}}
			if typed {
				item["type"] = "message"
			}
			if attributed {
				item["author"] = "/root/worker"
			}
			source["input"] = []any{message("user", "Before."), item, message("user", "After.")}
			before, _ := json.Marshal(source)
			prepared, _ := mustPrepare(t, source, "compatibility-id", nil)
			items := mustTestValue[[]any](t, prepared["input"])
			got := mustTestValue[object](t, items[len(items)-2])
			if _, exists := got["id"]; exists {
				t.Errorf("typed=%t attributed=%t: compatibility-only message ID reached BPS", typed, attributed)
			}
			if got["role"] != item["role"] || got["status"] != item["status"] || got["phase"] != item["phase"] {
				t.Fatal("message semantics changed")
			}
			content := mustTestValue[[]any](t, got["content"])
			if !reflect.DeepEqual(content[len(content)-1], mustTestValue[[]any](t, item["content"])[0]) {
				t.Fatal("message text or annotations changed")
			}
			if attributed && !strings.Contains(text(mustTestValue[object](t, content[0])["text"]), "/root/worker") {
				t.Fatal("attribution lost")
			}
			after, _ := json.Marshal(source)
			if string(before) != string(after) {
				t.Fatal("client source was mutated")
			}
			source["input"] = []any{got}
			second, _ := mustPrepare(t, source, "compatibility-id", nil)
			replay := mustTestValue[[]any](t, second["input"])
			if !reflect.DeepEqual(replay[len(replay)-1], got) {
				t.Fatal("normalization is not idempotent")
			}
		}
	}
}

func TestInlineHistoryRetainsOtherMessageIDs(t *testing.T) {
	for _, id := range []any{"msg_0123456789abcdef01234567", "item_other", "item_0123456789abcdef0123456z", "item_0123456789abcdef0123456789", "fc_old", nil, json.Number("42")} {
		source := testSource()
		source["input"] = []any{object{"type": "message", "role": "assistant", "id": id, "content": "Keep content."}}
		prepared, _ := mustPrepare(t, source, "other-id", nil)
		items := mustTestValue[[]any](t, prepared["input"])
		if !reflect.DeepEqual(mustTestValue[object](t, items[len(items)-1])["id"], id) {
			t.Fatalf("unrecognized/native ID %v was silently changed", id)
		}
	}
}

func TestCompatibilityMessageIDDoesNotAuthorizeItemReferences(t *testing.T) {
	source := testSource()
	source["input"] = []any{object{"type": "message", "role": "assistant", "id": "item_0123456789abcdef01234567", "content": "History"}, object{"type": "item_reference", "id": "item_0123456789abcdef01234567"}}
	raw, _ := json.Marshal(source)
	if _, _, err := Prepare(raw, "refs", nil); err == nil || !strings.Contains(err.Error(), "item_reference") {
		t.Fatal("item references must still be rejected")
	}
}

func TestCompatibilityMessageIDLeavesNonMessageIdentityUntouched(t *testing.T) {
	for _, kind := range []string{"function_call", "custom_tool_call", "function_call_output", "reasoning"} {
		item := object{"type": kind, "id": "item_0123456789abcdef01234567", "call_id": "item_0123456789abcdef01234567", "arguments": "opaque"}
		got, err := normalizeHistoryMessage(item, 0)
		if err != nil || !reflect.DeepEqual(got, item) {
			t.Fatalf("non-message %s was changed", kind)
		}
	}
}
