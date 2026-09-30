package basispoints

import (
	"encoding/json"
	"reflect"
	"testing"
)

func historyCollisionEnvelope(t *testing.T, native object) object {
	t.Helper()
	var outer object
	if err := decode([]byte(text(native["arguments"])), &outer); err != nil {
		t.Fatal(err)
	}
	var envelope object
	if err := decode([]byte(text(outer["code"])), &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope
}

func TestCompleteHistoryCallIDsCannotSubstituteAnotherConversationsArguments(t *testing.T) {
	cache := new(ReplayCache)
	for _, path := range []string{"conversation-A", "conversation-B", "conversation-A"} {
		source := testSource()
		arguments, _ := json.Marshal(object{"path": path, "exact": json.Number("9007199254740993")})
		source["input"] = []any{
			message("user", path),
			object{"type": "function_call", "call_id": "call_1", "name": "read_file", "arguments": string(arguments)},
			object{"type": "function_call_output", "call_id": "call_1", "output": path + " result"},
		}
		body, _ := mustPrepare(t, source, "same-account|same-key", cache)
		items := mustTestValue[[]any](t, body["input"])
		envelope := historyCollisionEnvelope(t, mustTestValue[object](t, items[len(items)-2]))
		args := mustTestValue[object](t, envelope["arguments"])
		if args["path"] != path || args["exact"] != json.Number("9007199254740993") {
			t.Fatalf("reused historical call ID substituted another request's arguments: %+v", args)
		}
		if mustTestValue[object](t, items[len(items)-1])["output"] != path+" result" {
			t.Fatal("collision recovery altered the client's recorded result")
		}
	}
}

func TestReplayMatchesSemanticClientCallAndPreservesExactNativeItem(t *testing.T) {
	cache := new(ReplayCache)
	source := testSource()
	source["tools"] = []any{object{"type": "namespace", "name": "files", "tools": []any{object{"type": "function", "name": "read"}}}}
	_, bridge := mustPrepare(t, source, "scope", cache)
	native := nativeCall(object{"name": "files.read", "arguments": object{"n": json.Number("9007199254740993"), "nested": object{"a": json.Number("1"), "b": json.Number("2")}}})
	native["provider_extension"] = object{"must": "survive"}
	call, err := bridge.translateCall(native)
	if err != nil {
		t.Fatal(err)
	}
	call["id"] = "different-client-wire-id"
	call["status"] = "in_progress"
	call["arguments"] = ` { "nested": { "b": 2, "a": 1 }, "n": 9007199254740993 } `
	if got := cache.getForCall("scope", text(call["call_id"]), call); !reflect.DeepEqual(got, native) {
		t.Fatal("equivalent JSON arguments failed to retain the exact original native item")
	}
	if got := cache.getForCall("other-key-scope", text(call["call_id"]), call); got != nil {
		t.Fatal("call signature bypassed cache owner isolation")
	}
	for _, change := range []object{
		{"name": "write"}, {"namespace": "other"}, {"call_id": "other-id"},
		{"arguments": `{"n":9007199254740992,"nested":{"a":1,"b":2}}`},
		{"arguments": `null`}, {"arguments": `{} {}`},
		{"type": "custom_tool_call", "input": "raw text"},
	} {
		changed := make(object, len(call))
		for k, v := range call {
			changed[k] = v
		}
		for k, v := range change {
			changed[k] = v
		}
		if cache.getForCall("scope", text(call["call_id"]), changed) != nil {
			t.Fatalf("different or incomplete client call matched native history: %+v", change)
		}
	}
}

func TestCustomHistorySignaturePreservesExactRawInput(t *testing.T) {
	first := object{"type": "custom_tool_call", "call_id": "call_1", "name": "patch", "namespace": "files", "input": "line one\r\n\tline two \\n"}
	changed := object{"type": "custom_tool_call", "call_id": "call_1", "name": "patch", "namespace": "files", "input": "line one\n\tline two \\n"}
	cache := new(ReplayCache)
	native, err := (&Bridge{}).rebuildNativeHistoryCall(first)
	if err != nil {
		t.Fatal(err)
	}
	cache.put("scope", "call_1", native, first)
	if cache.getForCall("scope", "call_1", first) == nil || cache.getForCall("scope", "call_1", changed) != nil {
		t.Fatal("custom history signature did not distinguish exact raw input")
	}
}

func TestUnsignedReplayEntryCannotOverrideCompleteClientHistory(t *testing.T) {
	cache := new(ReplayCache)
	old := nativeCall(object{"name": "old_tool", "arguments": object{"path": "old"}})
	cache.put("scope", "call_1", old)
	source := testSource()
	source["tool_choice"] = "none"
	source["input"] = []any{
		message("user", "synthetic"),
		object{"type": "function_call", "call_id": "call_1", "name": "current_tool", "arguments": `{"path":"current"}`},
		object{"type": "function_call_output", "call_id": "call_1", "output": "current result"},
	}
	body, _ := mustPrepare(t, source, "scope", cache)
	items := mustTestValue[[]any](t, body["input"])
	envelope := historyCollisionEnvelope(t, mustTestValue[object](t, items[len(items)-2]))
	if envelope["name"] != "current_tool" || mustTestValue[object](t, envelope["arguments"])["path"] != "current" {
		t.Fatal("entry without a client signature overrode complete current history")
	}
	// Output-only replay still requires a cached original; it cannot infer an
	// unseen call's arguments. Matching is enforced when the complete call exists.
	source["input"] = []any{object{"type": "function_call_output", "call_id": "call_1", "output": "current result"}}
	replayed, _ := mustPrepare(t, source, "scope", cache)
	items = mustTestValue[[]any](t, replayed["input"])
	if envelope := historyCollisionEnvelope(t, mustTestValue[object](t, items[len(items)-2])); envelope["name"] != "current_tool" {
		t.Fatal("collision-safe storage broke output-only replay")
	}
}
