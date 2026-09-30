package basispoints

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestTransportEnvelopeFormatting(t *testing.T) {
	const raw = `{"tool":"shell","args":{"cmd":"echo \"hello\"","n":9007199254740993}}`
	quoted, _ := json.Marshal(raw)
	var expected object
	if err := decode([]byte(raw), &expected); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]any{
		"plain": raw, "object": expected, "double encoded": string(quoted),
		"fenced":         "```json\n" + raw + "\n```",
		"labeled fence":  "Tool request:\n```json\n" + raw + "\n```",
		"labeled object": "Here is the tool request:\n" + raw,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := decodeTransportCode(value)
			if err != nil || !reflect.DeepEqual(got, expected) {
				t.Fatalf("decoded envelope changed its arguments: %+v, %v", got, err)
			}
		})
	}
}

func TestTransportEnvelopeRepairsOnlyIllegalEscapes(t *testing.T) {
	got, err := decodeTransportCode(`{"name":"shell","arguments":{"pattern":"\d+\s","path":"C:\Projects\file","line":"a\nb","literal":"\\n"}}`)
	if err != nil {
		t.Fatal(err)
	}
	args := mustTestValue[object](t, got["arguments"])
	// The original valid \f escape retains its JSON meaning; guessing paths would alter arguments.
	if args["pattern"] != `\d+\s` || args["path"] != "C:\\Projects\file" || args["line"] != "a\nb" || args["literal"] != `\n` {
		t.Fatalf("escape repair changed valid content: %#v", args)
	}
}

func TestTransportEnvelopeRejectsAmbiguousOrExecutableContent(t *testing.T) {
	for _, raw := range []any{
		nil, 42, "", `[]`, `null`, `{"name":"shell"} {"name":"other"}`,
		`const task = {"name":"shell","arguments":{}};`,
		`Excel.run(async () => { return {"name":"shell","arguments":{}}; });`,
		"```js\n{\"name\":\"shell\"}\n```",
		"```json\n{\"name\":\"shell\"}\n```\n{\"name\":\"other\"}",
		`{"name":"shell","arguments":`, strings.Repeat("x", maxEnvelopeBytes+1),
	} {
		if got, err := decodeTransportCode(raw); err == nil {
			t.Fatalf("invalid envelope accepted: %+v", got)
		}
	}
}

func TestFormattedEnvelopeAndOutputOnlyReplay(t *testing.T) {
	cache := new(ReplayCache)
	source := testSource()
	source["tools"] = []any{object{"type": "function", "name": "get_weather"}}
	_, bridge := mustPrepare(t, source, "account/key", cache)
	native := nativeCall(object{})
	args := object{"code": "```json\n{\"tool\":\"get_weather\",\"args\":{\"city\":\"Tokyo\"}}\n```", "summary": "Weather", "references": []any{"Tokyo"}}
	encoded, _ := json.Marshal(args)
	native["arguments"] = string(encoded)
	call, err := bridge.translateCall(native)
	if err != nil {
		t.Fatal(err)
	}
	source["input"] = []any{message("user", "weather"), object{"type": "function_call_output", "call_id": call["call_id"], "output": "18 C"}}
	replayed, _ := mustPrepare(t, source, "account/key", cache)
	items := mustTestValue[[]any](t, replayed["input"])
	if !reflect.DeepEqual(items[len(items)-2], native) {
		t.Fatal("formatting repair must not alter the original replay envelope")
	}
	output := mustTestValue[object](t, items[len(items)-1])
	if output["id"] != "fc_call_native" || output["call_id"] != call["call_id"] || output["output"] != "18 C" {
		t.Fatalf("incomplete tool output identity: %+v", output)
	}
}

func TestToolOutputIDsAreBoundedAndValidCallerIDsPreserved(t *testing.T) {
	for _, suppliedID := range []string{"", "caller_output_id", "ctco_client_result", "fc_valid_result", "fc_" + strings.Repeat("x", 61), "fc_" + strings.Repeat("x", 62)} {
		cache := new(ReplayCache)
		source := testSource()
		source["tools"] = []any{object{"type": "function", "name": "shell"}}
		_, bridge := mustPrepare(t, source, "account/key", cache)
		native := nativeCall(object{"name": "shell", "arguments": object{}})
		native["call_id"] = strings.Repeat("x", 100)
		call, err := bridge.translateCall(native)
		if err != nil {
			t.Fatal(err)
		}
		source["input"] = []any{message("user", "test"), call, object{"type": "function_call_output", "id": suppliedID, "call_id": call["call_id"], "output": "done"}}
		first, _ := mustPrepare(t, source, "account/key", cache)
		second, _ := mustPrepare(t, source, "account/key", cache)
		items := mustTestValue[[]any](t, first["input"])
		id := text(mustTestValue[object](t, items[len(items)-1])["id"])
		validCallerID := strings.HasPrefix(suppliedID, "fc_") && len(suppliedID) <= 64
		if !strings.HasPrefix(id, "fc_") || len(id) > 64 || (validCallerID && id != suppliedID) || !reflect.DeepEqual(first, second) {
			t.Fatal("tool output ID must be valid, bounded, stable and preserve valid supplied IDs")
		}
	}
}

func TestNestedTransportRetainsExactReplayAndArguments(t *testing.T) {
	for _, kind := range []string{"function", "custom"} {
		for depth := 0; depth <= 3; depth++ {
			cache := new(ReplayCache)
			source := testSource()
			source["tools"] = []any{object{"type": kind, "name": "shell"}}
			_, bridge := mustPrepare(t, source, "account/key", cache)
			envelope := object{"tool": "shell", "args": object{"number": json.Number("9007199254740993")}}
			if kind == "custom" {
				envelope["args"] = "exact raw input\nwith \\ and quotes \""
			}
			for i := 0; i < depth; i++ {
				raw, _ := json.Marshal(envelope)
				args, _ := json.Marshal(object{"code": string(raw)})
				envelope = object{"name": "functions.run_officejs", "arguments": string(args)}
			}
			native := nativeCall(envelope)
			call, err := bridge.translateCall(native)
			if depth > 2 {
				if err == nil {
					t.Fatal("excessive nested transport accepted")
				}
				continue
			}
			if err != nil || call["name"] != "shell" {
				t.Fatalf("kind=%s depth=%d: %v", kind, depth, err)
			}
			if kind == "function" && !strings.Contains(text(call["arguments"]), "9007199254740993") {
				t.Fatal("nested arguments lost numeric precision")
			}
			if kind == "custom" && call["input"] != "exact raw input\nwith \\ and quotes \"" {
				t.Fatal("custom input was modified")
			}
			if !reflect.DeepEqual(cache.get("account/key", "call_native"), native) {
				t.Fatal("nested wrapper must remain intact for upstream replay")
			}
		}
	}
}

func TestTransportShapeDoesNotDiscloseCode(t *testing.T) {
	const secret = "private-secret-never-in-errors"
	for _, value := range []any{nil, 42, "", "Excel.run(" + secret + ")", `{"name":"` + secret, "[\"" + secret + "\"]"} {
		_, err := decodeTransportCode(value)
		if err == nil || strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "format=") {
			t.Fatalf("missing or unsafe structural diagnostic: %v", err)
		}
	}
}

func TestNestedAndCustomAliasesRejectAmbiguity(t *testing.T) {
	source := testSource()
	source["tools"] = []any{object{"type": "custom", "name": "patch"}}
	_, bridge := mustPrepare(t, source, "", nil)
	for _, envelope := range []object{
		{"name": "run_officejs", "tool": "patch", "arguments": object{}},
		{"name": "run_officejs", "arguments": object{}, "args": object{}},
		{"name": "patch", "input": "one", "args": "two"},
		{"name": "patch", "args": object{"text": "do not stringify"}},
		{"name": "patch", "arguments": object{}, "input": "one"},
	} {
		if _, err := bridge.translateCall(nativeCall(envelope)); err == nil {
			t.Fatal("ambiguous or non-string custom input accepted")
		}
	}
}

func TestRecoverTransportEnvelopeFromWrapper(t *testing.T) {
	catalog := map[string]tool{"functions.exec": {Name: "exec", Kind: "function"}}
	for _, raw := range []string{
		`functions.exec({"name":"functions.exec","arguments":{"cmd":["pwd"]}})`,
		`The model returned: {"name":"functions.exec","arguments":{"cmd":["pwd"]}}`,
	} {
		got, ok := recoverTransportEnvelope(raw, catalog)
		if !ok || got["name"] != "functions.exec" {
			t.Fatalf("wrapper was not recovered: %q -> %#v", raw, got)
		}
	}
	if _, ok := recoverTransportEnvelope(`prefix {"name":"functions.exec"} and {"name":"functions.exec"}`, catalog); ok {
		t.Fatal("ambiguous multiple envelopes must remain rejected")
	}
}
