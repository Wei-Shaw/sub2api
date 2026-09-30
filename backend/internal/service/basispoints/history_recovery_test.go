package basispoints

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestHistoryRecoveryPreservesCompleteCallsWithoutCurrentCatalog(t *testing.T) {
	for _, kind := range []string{"function_call", "custom_tool_call"} {
		t.Run(kind, func(t *testing.T) {
			call := object{"type": kind, "id": "ctc_original", "call_id": "call_old", "namespace": "old.tools", "name": "execute"}
			if kind == "function_call" {
				call["arguments"] = `{"large":9007199254740993,"path":"C:\\work\\file","nested":{"active":true}}`
			} else {
				call["input"] = "*** Begin Patch\n*** Add File: exact.txt\n+unchanged\n*** End Patch"
			}
			source := testSource()
			source["input"] = []any{message("user", "continue"), call, object{"type": strings.Replace(kind, "_call", "_call_output", 1), "call_id": "call_old", "output": "recorded result"}}
			cache := new(ReplayCache)
			body, _ := mustPrepare(t, source, "new-account|key", cache)
			items := mustTestValue[[]any](t, body["input"])
			native := mustTestValue[object](t, items[len(items)-2])
			if native["name"] != "run_officejs" || native["call_id"] != "call_old" || !strings.HasPrefix(text(native["id"]), "fc_") {
				t.Fatalf("incomplete native call: %+v", native)
			}
			var outer object
			if err := decode([]byte(text(native["arguments"])), &outer); err != nil {
				t.Fatal(err)
			}
			for _, field := range []string{"summary", "extended_summary", "references", "destructive"} {
				if _, exists := outer[field]; !exists {
					t.Fatalf("missing native argument %s", field)
				}
			}
			var envelope object
			if err := decode([]byte(text(outer["code"])), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope["name"] != "old.tools.execute" {
				t.Fatalf("namespace lost: %+v", envelope)
			}
			if kind == "custom_tool_call" {
				if envelope["input"] != call["input"] {
					t.Fatal("custom input changed")
				}
			} else {
				var want object
				if err := decode([]byte(text(call["arguments"])), &want); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(envelope["arguments"], want) {
					t.Fatalf("function argument semantics changed: %+v", envelope)
				}
			}
			retry, _ := mustPrepare(t, source, "new-account|key", cache)
			if !reflect.DeepEqual(body, retry) {
				t.Fatal("history reconstruction must be deterministic across retries")
			}
		})
	}
}

func TestCodexToolOutputIDRoundTrip(t *testing.T) {
	for _, kind := range []string{"function", "custom"} {
		t.Run(kind, func(t *testing.T) {
			cache := new(ReplayCache)
			source := testSource()
			source["tools"] = []any{object{"type": kind, "name": "execute"}}
			_, bridge := mustPrepare(t, source, "scope", cache)
			envelope := object{"name": "execute", "arguments": object{}}
			if kind == "custom" {
				envelope = object{"name": "execute", "input": "exact tool input"}
			}
			call, err := bridge.translateCall(nativeCall(envelope))
			if err != nil {
				t.Fatal(err)
			}
			output := object{
				"type": kind + "_tool_call_output", "id": "ctco_client_result",
				"call_id": call["call_id"], "output": "recorded tool result",
			}
			if kind == "function" {
				output["type"] = "function_call_output"
				output["id"] = "fco_client_result"
			}
			source["input"] = []any{message("user", "continue"), call, output}
			var firstID string
			for _, replay := range []*ReplayCache{cache, new(ReplayCache)} {
				body, _ := mustPrepare(t, source, "scope", replay)
				items := mustTestValue[[]any](t, body["input"])
				result := mustTestValue[object](t, items[len(items)-1])
				id := text(result["id"])
				if !strings.HasPrefix(id, "fc_") || len(id) > 64 || id == mustTestValue[object](t, items[len(items)-2])["id"] {
					t.Fatalf("invalid or colliding BPS output ID: %q", id)
				}
				if result["type"] != "function_call_output" || result["call_id"] != call["call_id"] || result["output"] != output["output"] {
					t.Fatal("tool result type, correlation or content changed")
				}
				if firstID != "" && id != firstID {
					t.Fatal("output ID changed after replay cache loss")
				}
				firstID = id
			}
		})
	}
}

func TestHistoryRecoveryRejectsIncompleteCalls(t *testing.T) {
	for _, patch := range []object{
		{"call_id": ""}, {"name": ""}, {"namespace": 42}, {"arguments": nil},
		{"arguments": `null`}, {"arguments": `[]`}, {"arguments": `{} {}`}, {"arguments": `{"partial":`},
		{"type": "custom_tool_call", "input": object{"not": "text"}},
	} {
		call := object{"type": "function_call", "call_id": "call_history", "name": "shell", "arguments": `{}`}
		for key, value := range patch {
			call[key] = value
		}
		if _, err := (&Bridge{}).rebuildNativeHistoryCall(call); err == nil {
			t.Fatalf("invalid call was reconstructed: %+v", patch)
		}
	}
}

func TestHostedDeclarationsFilteredAndUnknownTypesRejected(t *testing.T) {
	for _, kind := range []string{"web_search", "web_search_preview", "web_search_preview_2025_03_11", "web_search_2025_08_26", "tool_search", "image_generation", "file_search", "code_interpreter", "computer", "computer_use_preview", "mcp"} {
		source := testSource()
		source["tools"] = []any{object{"type": kind}, object{"type": "function", "name": "shell"}}
		body, bridge := mustPrepare(t, source, "scope", new(ReplayCache))
		if len(bridge.tools) != 1 || bridge.tools["shell"].Name != "shell" || body["tools"] != nil {
			t.Fatalf("hosted %s affected available client tool or leaked upstream", kind)
		}
	}
	source := testSource()
	source["tools"] = []any{object{"type": "not_a_known_tool"}}
	raw, _ := json.Marshal(source)
	if _, _, err := Prepare(raw, "scope", nil); err == nil {
		t.Fatal("unknown tool types must not be silently ignored")
	}
}

func TestNestedNamespaceAndConflictingCatalogDefinitions(t *testing.T) {
	source := testSource()
	decl := object{"type": "function", "name": "run", "parameters": object{"type": "object"}}
	source["tools"] = []any{object{"type": "namespace", "name": "outer", "tools": []any{object{"type": "namespace", "name": "inner", "tools": []any{decl, decl}}}}}
	_, bridge := mustPrepare(t, source, "scope", nil)
	if len(bridge.tools) != 1 || bridge.tools["outer.inner.run"].Namespace != "outer.inner" {
		t.Fatalf("nested namespace lost: %+v", bridge.tools)
	}
	call, err := bridge.translateCall(nativeCall(object{"name": "outer.inner.run", "arguments": object{}}))
	if err != nil || call["namespace"] != "outer.inner" || call["name"] != "run" {
		t.Fatalf("nested tool failed round trip: %+v, %v", call, err)
	}
	source["input"] = []any{message("user", "continue"), object{"type": "additional_tools", "tools": []any{object{"type": "namespace", "name": "outer.inner", "tools": []any{object{"type": "function", "name": "run", "parameters": object{"type": "array"}}}}}}}
	raw, _ := json.Marshal(source)
	if _, _, err := Prepare(raw, "scope", nil); err == nil || !strings.Contains(err.Error(), "conflicting duplicate") {
		t.Fatalf("conflicting same-name tool must fail: %v", err)
	}
}

func TestToolOutputHTTPSImagesRemainIntact(t *testing.T) {
	source := testSource()
	image := object{"type": "input_image", "image_url": "https://example.com/screenshot.png?signature=unchanged%2F"}
	source["input"] = []any{message("user", "inspect"), object{"type": "function_call", "name": "view_image", "call_id": "call_image", "arguments": `{}`}, object{"type": "function_call_output", "call_id": "call_image", "output": []any{image}}}
	body, _ := mustPrepare(t, source, "scope", nil)
	items := mustTestValue[[]any](t, body["input"])
	content := mustTestValue[[]any](t, mustTestValue[object](t, items[len(items)-1])["content"])
	if !reflect.DeepEqual(content[len(content)-1], image) {
		t.Fatal("HTTPS tool image changed")
	}
}
