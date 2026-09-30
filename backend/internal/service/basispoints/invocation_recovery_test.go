package basispoints

import (
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestSingleCatalogInvocationPreservesArgumentsAndReplay(t *testing.T) {
	const argsJSON = `{"command":"echo \"hello\"","number":9007199254740993,"nested":{"enabled":true}}`
	var want object
	if err := decode([]byte(argsJSON), &want); err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{"", "await ", "return ", "return await "} {
		t.Run(strings.TrimSpace(prefix), func(t *testing.T) {
			source := testSource()
			source["tools"] = []any{object{"type": "namespace", "name": "functions", "tools": []any{object{"type": "function", "name": "shell"}}}}
			cache := new(ReplayCache)
			_, bridge := mustPrepare(t, source, "invocation-scope", cache)
			native := nativeCall(object{})
			native["arguments"] = object{"code": prefix + "functions.shell(" + argsJSON + " ) ; ", "summary": "Run a diagnostic"}
			call, err := bridge.translateCall(native)
			if err != nil {
				t.Fatal(err)
			}
			var got object
			if err := decode([]byte(text(call["arguments"])), &got); err != nil {
				t.Fatal(err)
			}
			if call["name"] != "shell" || call["namespace"] != "functions" || !reflect.DeepEqual(got, want) {
				t.Fatalf("invocation changed its identity or arguments: %#v", call)
			}
			if !reflect.DeepEqual(cache.get(bridge.scope, text(call["call_id"])), native) {
				t.Fatal("the original native transport must remain available for replay")
			}
			source["input"] = []any{message("user", "diagnostic"), call, object{"type": "function_call_output", "call_id": call["call_id"], "output": "done"}}
			replayed, _ := mustPrepare(t, source, bridge.scope, cache)
			items := mustTestValue[[]any](t, replayed["input"])
			if !reflect.DeepEqual(items[len(items)-2], native) {
				t.Fatal("replay changed the original native invocation")
			}
		})
	}
}

func TestSingleCatalogInvocationPreservesLiteralInput(t *testing.T) {
	input := "text(\"quoted\");\n literal \\n and 中文"
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	catalog := map[string]tool{"exec": {Name: "exec", Kind: "custom"}}
	envelope, ok := recoverTransportEnvelope("await functions.exec("+string(encoded)+");", catalog)
	if !ok || envelope["name"] != "exec" || envelope["input"] != input {
		t.Fatalf("custom literal input was not preserved: %#v", envelope)
	}
	catalog["shell"] = tool{Name: "shell", Kind: "function"}
	envelope, ok = recoverTransportEnvelope(`functions.shell({"name":"exec","arguments":{"value":1}})`, catalog)
	if !ok || envelope["name"] != "shell" {
		t.Fatal("an argument named name must not select another catalog tool")
	}
	args := mustTestValue[object](t, envelope["arguments"])
	if args["name"] != "exec" || args["arguments"] == nil {
		t.Fatal("envelope-like function arguments must remain intact")
	}
}

func TestCustomInvocationRoundTripUsesExactLiteral(t *testing.T) {
	source := testSource()
	source["tools"] = []any{object{"type": "namespace", "name": "functions", "tools": []any{object{"type": "custom", "name": "exec"}}}}
	input := "text(\"diagnostic\");\n"
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	cache := new(ReplayCache)
	_, bridge := mustPrepare(t, source, "scope", cache)
	native := nativeCall(object{})
	native["arguments"] = object{"code": "functions.exec(" + string(encoded) + ");", "summary": "Run diagnostic"}
	call, err := bridge.translateCall(native)
	if err != nil {
		t.Fatal(err)
	}
	if call["type"] != "custom_tool_call" || call["namespace"] != "functions" || call["name"] != "exec" || call["input"] != input {
		t.Fatalf("custom invocation changed its identity or input: %#v", call)
	}
	source["input"] = []any{message("user", "diagnostic"), call, object{"type": "custom_tool_call_output", "call_id": call["call_id"], "output": "done"}}
	replayed, _ := mustPrepare(t, source, bridge.scope, cache)
	items := mustTestValue[[]any](t, replayed["input"])
	if !reflect.DeepEqual(items[len(items)-2], native) || mustTestValue[object](t, items[len(items)-1])["output"] != "done" {
		t.Fatal("custom invocation or result changed during replay")
	}
}

func TestInvocationRecoveryRejectsExecutableOrAmbiguousWrappers(t *testing.T) {
	catalog := map[string]tool{"functions.shell": {Name: "shell", Kind: "function"}, "functions.exec": {Name: "exec", Kind: "custom"}}
	for _, raw := range []string{
		`functions.shell({"name":"functions.shell","arguments":{}}); functions.shell({})`,
		`functions.shell({"name":"functions.shell","arguments":{}}`,
		`functions.shell({"name":"functions.shell","arguments":{}}, {})`,
		`const call = {"name":"functions.shell","arguments":{}};`,
		`[ {"name":"functions.shell","arguments":{}} ]`,
		`"{\"name\":\"functions.shell\",\"arguments\":{}}" trailing`,
		`Excel.run(() => ({"name":"functions.shell","arguments":{}}))`,
		`unknown({"name":"functions.shell","arguments":{}})`,
		`functions.shell({command: "pwd"})`,
		`functions.shell({"command": other()})`,
		`functions.shell("not an object")`,
		`functions.exec({"not":"raw input"})`,
		`functions.exec("exact"); other()`,
		`prefix {"name":"functions.shell","arguments":{}} trailing`,
	} {
		if got, ok := recoverTransportEnvelope(raw, catalog); ok {
			t.Errorf("unsafe wrapper accepted: %q -> %#v", raw, got)
		}
	}
}

func TestRejectedInvocationDoesNotEmitPartialClientTool(t *testing.T) {
	source := testSource()
	source["tools"] = []any{object{"type": "function", "name": "shell"}}
	_, bridge := mustPrepare(t, source, "scope", nil)
	native := nativeCall(object{})
	native["arguments"] = object{"code": `shell({"name":"shell","arguments":{}}); shell({"command":"private-command"})`}
	wire := sse(object{"type": "response.output_item.done", "item": native}) + sse(object{"type": "response.completed", "response": object{"output": []any{native}}})
	body := bridge.Stream(io.NopCloser(strings.NewReader(wire)))
	defer func() { _ = body.Close() }()
	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "response.failed") || strings.Contains(string(got), "response.function_call_arguments") || strings.Contains(string(got), "private-command") {
		t.Fatal("invalid wrapper must fail without exposing or dispatching a partial tool call")
	}
}
