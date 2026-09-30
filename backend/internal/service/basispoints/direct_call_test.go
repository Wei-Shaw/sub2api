package basispoints

import (
	"reflect"
	"strings"
	"testing"
)

// The model sometimes calls a declared client tool by its own name instead of
// wrapping it in run_officejs. The bridge recovers this into the client tool call
// rather than failing the whole response, and never executes anything.
func TestDirectFunctionCallRecoveredIntoClientTool(t *testing.T) {
	cache := new(ReplayCache)
	source := testSource()
	source["tools"] = []any{object{"type": "function", "name": "shell", "parameters": object{"type": "object"}}}
	_, bridge := mustPrepare(t, source, "scope", cache)
	for _, name := range []string{"shell", "functions.shell"} {
		native := object{"type": "function_call", "id": "fc_" + name, "call_id": "call_" + name, "name": name, "arguments": `{"command":["ls","-la"]}`}
		call, err := bridge.translateCall(native)
		if err != nil {
			t.Fatalf("%s: direct call not recovered: %v", name, err)
		}
		if call["type"] != "function_call" || call["name"] != "shell" || call["call_id"] != "call_"+name {
			t.Fatalf("%s: wrong client tool item: %+v", name, call)
		}
		var args object
		if decode([]byte(text(call["arguments"])), &args) != nil {
			t.Fatalf("%s: arguments not valid JSON object: %v", name, call["arguments"])
		}
		if cmd, _ := args["command"].([]any); len(cmd) != 2 {
			t.Fatalf("%s: arguments lost: %+v", name, args)
		}
	}
}

// A direct custom tool call is relayed as custom_tool_call with its exact input.
func TestDirectCustomCallRecoveredIntoClientTool(t *testing.T) {
	cache := new(ReplayCache)
	source := testSource()
	source["tools"] = []any{object{"type": "namespace", "name": "functions", "tools": []any{object{"type": "custom", "name": "apply_patch"}}}}
	_, bridge := mustPrepare(t, source, "scope", cache)
	patch := "*** Begin Patch\n*** Add File: a.txt\n+hi\n*** End Patch"
	native := object{"type": "custom_tool_call", "id": "ctc_a", "call_id": "call_a", "name": "functions.apply_patch", "input": patch}
	call, err := bridge.translateCall(native)
	if err != nil {
		t.Fatalf("direct custom call not recovered: %v", err)
	}
	if call["type"] != "custom_tool_call" || call["name"] != "apply_patch" || call["namespace"] != "functions" || call["input"] != patch {
		t.Fatalf("wrong custom tool item: %+v", call)
	}
}

// A direct call still refuses tools outside the catalog and genuine native tools.
func TestDirectCallRejectsUnknownAndMismatchedTools(t *testing.T) {
	source := testSource()
	source["tools"] = []any{object{"type": "function", "name": "shell"}, object{"type": "custom", "name": "patch"}}
	_, bridge := mustPrepare(t, source, "scope", nil)
	cases := []object{
		{"type": "function_call", "id": "1", "call_id": "c1", "name": "read_ranges", "arguments": "{}"},
		{"type": "function_call", "id": "2", "call_id": "c2", "name": "search_workbook", "arguments": "{}"},
		{"type": "custom_tool_call", "id": "3", "call_id": "c3", "name": "shell", "input": "ls"},  // function tool arriving as custom
		{"type": "function_call", "id": "4", "call_id": "c4", "name": "patch", "arguments": "{}"}, // custom tool arriving as function
	}
	for i, native := range cases {
		if _, err := bridge.translateCall(native); err == nil {
			t.Fatalf("case %d must be rejected: %+v", i, native)
		}
	}
}

// A direct call cached for replay must reappear as a BPS-known run_officejs wrapper
// on the next turn, and the recorded tool output must round-trip under that identity.
func TestDirectCallReplaysAsTransportWrapper(t *testing.T) {
	cache := new(ReplayCache)
	source := testSource()
	source["tools"] = []any{object{"type": "function", "name": "shell", "parameters": object{"type": "object"}}}
	_, bridge := mustPrepare(t, source, "account/key", cache)
	native := object{"type": "function_call", "id": "fc_d", "call_id": "call_d", "name": "shell", "arguments": `{"command":["pwd"]}`}
	call, err := bridge.translateCall(native)
	if err != nil {
		t.Fatal(err)
	}
	source["input"] = []any{message("user", "run"), call, object{"type": "function_call_output", "call_id": call["call_id"], "output": "/root"}}
	replayed, _ := mustPrepare(t, source, "account/key", cache)
	items := mustTestValue[[]any](t, replayed["input"])
	wrapper, _ := items[len(items)-2].(object)
	if text(wrapper["type"]) != "function_call" || text(wrapper["name"]) != "run_officejs" {
		t.Fatalf("direct call did not replay as a run_officejs transport item: %+v", wrapper)
	}
	if !strings.Contains(text(wrapper["arguments"]), `\"command\"`) {
		t.Fatalf("replayed transport lost the client arguments: %+v", wrapper["arguments"])
	}
	output, _ := items[len(items)-1].(object)
	if output["call_id"] != call["call_id"] || output["output"] != "/root" {
		t.Fatalf("tool output identity broken: %+v", output)
	}
	// Determinism: an identical follow-up request must serialize identically.
	again, _ := mustPrepare(t, source, "account/key", cache)
	if !reflect.DeepEqual(replayed, again) {
		t.Fatal("direct-call replay is not deterministic")
	}
}
