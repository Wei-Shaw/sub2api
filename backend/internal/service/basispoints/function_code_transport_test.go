package basispoints

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"
)

func functionCodeTestTool(name string) object {
	return object{"type": "function", "name": name, "parameters": object{
		"type": "object", "required": []any{"code", "description"},
		"properties": object{
			"code": object{"type": "string"}, "description": object{"type": "string"},
			"justification": object{"type": "string"}, "sandbox_permissions": object{"type": "string"},
			"timeoutMs": object{"type": "number"},
		},
	}}
}

func functionCodeTestNative(t *testing.T, name string, code any, metadata any) object {
	t.Helper()
	outer, err := json.Marshal(object{
		"summary": functionCodeTransportPrefix + name, "code": code,
		"extended_summary": metadata, "destructive": false, "references": []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return object{"type": "function_call", "id": "fc_code", "call_id": "call_code", "name": "run_officejs", "arguments": string(outer), "status": "completed"}
}

func functionCodeTestArguments(t *testing.T, call object) object {
	t.Helper()
	var args object
	if err := decode([]byte(text(call["arguments"])), &args); err != nil {
		t.Fatal(err)
	}
	return args
}

func TestFunctionCodeTransportPreservesObservedQuotePatterns(t *testing.T) {
	// Synthetic variants of the reported PowerShell quote failure. No captured
	// business data or executable test commands are copied into this fixture.
	for _, code := range []string{
		"await tools.exec({command:'Write-Output \"First suite passed\"',cwd:'C:/fixture'});",
		"console.log(await tools.exec({command:'Write-Output \"Second suite passed\"'}));",
	} {
		source := testSource()
		source["tools"] = []any{functionCodeTestTool("run_code")}
		_, bridge := mustPrepare(t, source, "scope", nil)
		broken := "{\"name\":\"run_code\",\"arguments\":{\"code\":\"" + code + "\",\"description\":\"Run checks\"}}"
		if _, err := decodeTransportEnvelope(broken); err == nil || !strings.Contains(err.Error(), "missing_separator") {
			t.Fatal("the unescaped nested JSON regression must still be rejected")
		}
		native := functionCodeTestNative(t, "run_code", code, "{\"description\":\"Run checks\"}")
		call, err := bridge.translateCall(native)
		if err != nil {
			t.Fatal(err)
		}
		args := functionCodeTestArguments(t, call)
		if args["code"] != code || args["description"] != "Run checks" || call["name"] != "run_code" {
			t.Fatal("raw code or additional function arguments changed")
		}
	}
}

func TestFunctionCodeTransportPreservesPayloadAndOptionalArguments(t *testing.T) {
	for _, code := range []string{
		"", "  \tconst value = \"中文\";\r\nconsole.log(value);\n",
		"C:\\fixture\\path\\file; literal \\n \\t \\r \\d+ \\u1234",
		"\x00\b\f", "{\"name\":\"other_tool\",\"arguments\":{}}",
		strings.Repeat("\tWrite-Output \"quotes \\\"inside\\\" 中文\"\r\n", 300),
	} {
		source := testSource()
		source["tools"] = []any{functionCodeTestTool("run_code")}
		_, bridge := mustPrepare(t, source, "scope", nil)
		metadata := object{
			"description": "Inspect \"quoted\" output", "justification": "Requested check",
			"sandbox_permissions": "use_default", "timeoutMs": json.Number("9007199254740993"),
		}
		encoded, err := json.Marshal(metadata)
		if err != nil {
			t.Fatal(err)
		}
		native := functionCodeTestNative(t, "run_code", code, string(encoded))
		before, _ := json.Marshal(native)
		call, err := bridge.translateCall(native)
		if err != nil {
			t.Fatal(err)
		}
		got := functionCodeTestArguments(t, call)
		metadata["code"] = code
		if !reflect.DeepEqual(got, metadata) {
			t.Fatal("code, permission metadata or numeric precision changed")
		}
		after, _ := json.Marshal(native)
		if !bytes.Equal(before, after) {
			t.Fatal("native arguments were mutated")
		}
		if encrypted, ok := call["encrypted_function_args"].([]string); !ok || len(encrypted) != 0 {
			t.Fatal("plaintext arguments must retain explicit empty encryption metadata")
		}
	}
}

func TestFunctionCodeTransportRejectsInvalidMetadataAndBounds(t *testing.T) {
	source := testSource()
	source["tools"] = []any{functionCodeTestTool("run_code")}
	_, bridge := mustPrepare(t, source, "scope", nil)
	const private = "private-code-never-in-errors"
	for _, metadata := range []any{
		nil, 42, object{}, "", "null", "[]", "{} {}", "{", "plain prose",
		"{\"description\":\"" + private + "\" trailing}",
		"{\"code\":null}", "{\"code\":\"duplicate\"}", strings.Repeat(" ", maxEnvelopeBytes),
	} {
		_, err := bridge.translateCall(functionCodeTestNative(t, "run_code", private, metadata))
		if err == nil || strings.Contains(err.Error(), private) {
			t.Fatal("invalid metadata must fail without leaking payload text")
		}
	}
	for _, code := range []any{nil, 7, object{}, []any{}, strings.Repeat("x", maxEnvelopeBytes), strings.Repeat("\"", maxEnvelopeBytes/2)} {
		if _, err := bridge.translateCall(functionCodeTestNative(t, "run_code", code, "{}")); err == nil {
			t.Fatal("invalid or oversized raw/serialized arguments were accepted")
		}
	}
	empty, _ := json.Marshal(object{"code": "", "description": "Run checks"})
	exact := strings.Repeat("x", maxEnvelopeBytes-len(empty))
	for _, tc := range []struct {
		code string
		ok   bool
	}{{exact, true}, {exact + "x", false}} {
		call, err := bridge.translateCall(functionCodeTestNative(t, "run_code", tc.code, "{\"description\":\"Run checks\"}"))
		if (err == nil) != tc.ok {
			t.Fatal("serialized argument byte limit was not enforced exactly")
		}
		if tc.ok && len(text(call["arguments"])) != maxEnvelopeBytes {
			t.Fatal("expected the exact serialized byte boundary")
		}
	}
}

func TestFunctionCodeTransportRequiresExplicitCatalogContract(t *testing.T) {
	source := testSource()
	source["tools"] = []any{
		functionCodeTestTool("run_code"), object{"type": "custom", "name": "custom_code"},
		object{"type": "function", "name": "plain"},
		object{"type": "function", "name": "numeric", "parameters": object{"type": "object", "properties": object{"code": object{"type": "number"}}}},
	}
	_, bridge := mustPrepare(t, source, "scope", nil)
	for _, name := range []string{"", "missing", "custom_code", "plain", "numeric", "run_code ", " run_code", "functions.run_code", "run_code\n", "run_code/extra"} {
		if _, err := bridge.translateCall(functionCodeTestNative(t, name, "private payload", "{}")); err == nil {
			t.Fatal("undeclared, wrong-kind or approximate catalog name was accepted")
		}
	}
	for _, summary := range []any{nil, 1, "Run code", "codex2api.function_code", " codex2api.function_code/run_code", "Codex2api.function_code/run_code"} {
		if envelope, marked, err := bridge.functionCodeTransportEnvelope(object{"summary": summary, "code": "source", "extended_summary": "{}"}); envelope != nil || marked || err != nil {
			t.Fatal("an approximate marker activated raw code transport")
		}
	}
	for _, patch := range []object{{"call_id": ""}, {"name": "not_run_officejs"}} {
		native := functionCodeTestNative(t, "run_code", "source", "{}")
		for key, value := range patch {
			native[key] = value
		}
		if _, err := bridge.translateCall(native); err == nil {
			t.Fatal("native call identity must still be validated")
		}
	}
}

func TestFunctionCodeCatalogAndHistoryUseRawTransport(t *testing.T) {
	source := testSource()
	source["tools"] = []any{object{"type": "namespace", "name": "client", "tools": []any{functionCodeTestTool("run_code")}}}
	body, bridge := mustPrepare(t, source, "scope", new(ReplayCache))
	items := mustTestValue[[]any](t, body["input"])
	protocolMessage := mustTestValue[object](t, items[1])
	protocol := text(mustTestValue[object](t, mustTestValue[[]any](t, protocolMessage["content"])[0])["text"])
	for _, want := range []string{functionCodeTransportPrefix + "client.run_code", "FUNCTION_CODE", "all other supplied arguments", "only fields declared", "Do not include code"} {
		if !strings.Contains(protocol, want) {
			t.Fatalf("missing transport contract: %s", want)
		}
	}
	const code = "Write-Output \"history survives\"\r\nC:\\fixture\\file"
	native := functionCodeTestNative(t, "client.run_code", code, "{\"description\":\"Run checks\",\"timeoutMs\":123}")
	call, err := bridge.translateCall(native)
	if err != nil {
		t.Fatal(err)
	}
	if call["namespace"] != "client" || call["name"] != "run_code" {
		t.Fatal("namespaced identity changed")
	}
	source["input"] = []any{message("user", "continue"), call, object{"type": "function_call_output", "call_id": "call_code", "output": "recorded result"}}
	for i, cache := range []*ReplayCache{bridge.replay, new(ReplayCache)} {
		next, replayBridge := mustPrepare(t, source, "scope", cache)
		input := mustTestValue[[]any](t, next["input"])
		restored := mustTestValue[object](t, input[len(input)-2])
		if i == 0 && !reflect.DeepEqual(native, restored) {
			t.Fatal("cached native tool item must replay exactly")
		}
		outer := functionCodeTestArguments(t, restored)
		if outer["summary"] != functionCodeTransportPrefix+"client.run_code" || outer["code"] != code {
			t.Fatal("cache-miss history reintroduced nested code JSON")
		}
		roundTrip, err := replayBridge.translateCall(restored)
		if err != nil || !reflect.DeepEqual(functionCodeTestArguments(t, call), functionCodeTestArguments(t, roundTrip)) {
			t.Fatal("history changed function arguments")
		}
	}
}

func TestFunctionCodeTransportStreamingIsAtomic(t *testing.T) {
	for _, metadata := range []string{"{\"description\":\"Run checks\"}", "{invalid"} {
		source := testSource()
		source["tools"] = []any{functionCodeTestTool("run_code")}
		_, bridge := mustPrepare(t, source, "scope", nil)
		const code = "Write-Output \"exact code\"\r\nC:\\fixture\\file"
		native := functionCodeTestNative(t, "run_code", code, metadata)
		wire := sse(object{"type": "response.output_item.added", "output_index": 0, "item": native}) +
			sse(object{"type": "response.function_call_arguments.delta", "output_index": 0, "delta": "UNVALIDATED_NATIVE_CODE"}) +
			sse(object{"type": "response.output_item.done", "output_index": 0, "item": native}) +
			sse(object{"type": "response.completed", "response": object{"id": "resp_code", "output": []any{native}}})
		stream := bridge.Stream(io.NopCloser(strings.NewReader(wire)))
		output, err := io.ReadAll(stream)
		_ = stream.Close()
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(output, []byte("UNVALIDATED_NATIVE_CODE")) || bytes.Contains(output, []byte(functionCodeTransportPrefix)) {
			t.Fatal("raw native transport leaked before conversion")
		}
		calls, completed, failed := 0, 0, 0
		err = readEvents(bytes.NewReader(output), func(_ string, data []byte) error {
			var event object
			if err := decode(data, &event); err != nil {
				return err
			}
			switch event["type"] {
			case "response.output_item.done":
				calls++
				if functionCodeTestArguments(t, mustTestValue[object](t, event["item"]))["code"] != code {
					t.Fatal("streaming changed code content")
				}
			case "response.completed":
				completed++
			case "response.failed":
				failed++
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if metadata == "{invalid" {
			if calls != 0 || completed != 0 || failed != 1 {
				t.Fatal("invalid metadata must fail without emitting a client tool call")
			}
		} else if calls != 1 || completed != 1 || failed != 0 {
			t.Fatal("valid raw code must produce exactly one complete client call")
		}
	}
}
