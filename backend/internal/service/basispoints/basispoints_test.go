package basispoints

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
)

func mustTestValue[T any](t *testing.T, value any) T {
	t.Helper()
	result, ok := value.(T)
	if !ok {
		t.Fatalf("expected %T, got %T", result, value)
	}
	return result
}

func mustPrepare(t *testing.T, source object, scope string, cache *ReplayCache) (object, *Bridge) {
	t.Helper()
	raw, _ := json.Marshal(source)
	body, bridge, err := Prepare(raw, scope, cache)
	if err != nil {
		t.Fatal(err)
	}
	var result object
	if err := decode(body, &result); err != nil {
		t.Fatal(err)
	}
	return result, bridge
}

type brokenStream struct{ err error }

func (s brokenStream) Read([]byte) (int, error) { return 0, s.err }
func (s brokenStream) Close() error             { return nil }

func TestNetworkReadFailureRetainsTransportError(t *testing.T) {
	_, bridge := mustPrepare(t, testSource(), "", nil)
	failure := errors.New("http2: client connection lost")
	body := bridge.Stream(brokenStream{err: failure})
	defer func() { _ = body.Close() }()
	output, err := io.ReadAll(body)
	if !errors.Is(err, failure) || len(output) != 0 {
		t.Fatalf("network failure must reach retry handling, not become a protocol event: %s, %v", output, err)
	}
}

func testSource() object {
	return object{"model": "gpt-5.6-sol", "input": "hello", "instructions": "Help with code", "reasoning": object{"effort": "max"}, "prompt_cache_key": "session-1"}
}

func TestPrepareWireShapeAndDeterminism(t *testing.T) {
	source := testSource()
	source["tools"] = []any{object{"type": "function", "name": "shell", "parameters": object{"type": "object"}}}
	source["include"] = []any{"reasoning.encrypted_content"}
	source["service_tier"] = "priority"
	first, bridge := mustPrepare(t, source, "account-1/key-1", nil)
	second, _ := mustPrepare(t, source, "account-1/key-1", nil)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("identical requests must serialize deterministically")
	}
	if first["model"] != source["model"] || first["reasoning_effort"] != "xhigh" || first["model_selection"] != "explicit" || first["store"] != false || first["stream"] != true {
		t.Fatalf("incorrect wire request: %+v", first)
	}
	if bridge.RequestedEffort != "max" || bridge.Effort != "xhigh" {
		t.Fatal("requested and effective effort must remain distinguishable")
	}
	management, ok := first["context_management"].([]any)
	if !ok || len(management) != 1 {
		t.Fatal("default context compaction contract missing")
	}
	entry, _ := management[0].(object)
	if entry["type"] != "compaction" || fmt.Sprint(entry["compact_threshold"]) != "920000" {
		t.Fatalf("unexpected default compaction threshold: %+v", entry)
	}
	for _, field := range []string{"tools", "reasoning", "include", "service_tier", "instructions"} {
		if _, exists := first[field]; exists {
			t.Fatalf("unsupported field leaked: %s", field)
		}
	}
	other, _ := mustPrepare(t, source, "account-2/key-1", nil)
	if first["prompt_cache_key"] == other["prompt_cache_key"] {
		t.Fatal("cache routing must be scoped by account and client")
	}
}

func TestEffortAndUnsupportedCapabilities(t *testing.T) {
	for requested, want := range map[string]string{"max": "xhigh", "ultra": "xhigh", "x-high": "xhigh", "high": "high", "none": "low", "minimal": "low", "": "medium"} {
		got, err := NormalizeEffort(requested)
		if err != nil || got != want {
			t.Fatalf("effort %q = %q, %v", requested, got, err)
		}
	}
	for _, patch := range []object{
		{"reasoning": object{"effort": "unknown"}},
		{"reasoning": object{"mode": "pro"}},
		{"tools": []any{object{"type": "unknown_hosted_tool"}}},
		{"tool_choice": "required"},
		{"previous_response_id": "resp_missing"},
		{"input": []any{object{"role": "user", "content": []any{object{"type": "input_image", "image_url": "data:image/png;base64,AAAA"}}}}},
		{"text": object{"format": object{"type": "json_schema"}}},
	} {
		source := testSource()
		for k, v := range patch {
			source[k] = v
		}
		raw, _ := json.Marshal(source)
		if _, _, err := Prepare(raw, "", nil); err == nil {
			t.Fatalf("unsupported capability silently accepted: %+v", patch)
		}
	}
}

func nativeCall(envelope object) object {
	code, _ := json.Marshal(envelope)
	arguments, _ := json.Marshal(object{"code": string(code), "summary": "Call client tool", "extended_summary": "Preserve the original envelope", "destructive": false, "references": []any{"Tokyo weather"}})
	return object{"type": "function_call", "id": "fc_native", "call_id": "call_native", "name": "run_officejs", "arguments": string(arguments), "status": "completed"}
}

func sse(payload object) string {
	raw, _ := json.Marshal(payload)
	return "event: " + text(payload["type"]) + "\ndata: " + string(raw) + "\n\n"
}

func TestStreamingToolRoundTripPreservesNativeIdentity(t *testing.T) {
	for _, kind := range []string{"function", "custom"} {
		t.Run(kind, func(t *testing.T) {
			cache := new(ReplayCache)
			source := testSource()
			source["tools"] = []any{object{"type": "namespace", "name": "client", "tools": []any{object{"type": kind, "name": "run"}}}}
			_, bridge := mustPrepare(t, source, "account/key/session", cache)
			envelope := object{"name": "client.run", "arguments": object{"number": json.Number("9007199254740993")}}
			if kind == "custom" {
				delete(envelope, "arguments")
				envelope["input"] = "*** Begin Patch\n*** End Patch"
			}
			native := nativeCall(envelope)
			wire := sse(object{"type": "response.created", "response": object{"id": "resp_1", "output": []any{}}}) +
				sse(object{"type": "response.output_item.added", "output_index": 1, "item": native}) +
				sse(object{"type": "response.function_call_arguments.delta", "output_index": 1, "delta": "SECRET_NATIVE_ENVELOPE"}) +
				sse(object{"type": "response.output_item.done", "output_index": 1, "item": native}) +
				sse(object{"type": "response.completed", "response": object{"id": "resp_1", "output": []any{object{"type": "reasoning", "encrypted_content": "encrypted"}, native}}})
			body := bridge.Stream(io.NopCloser(strings.NewReader(wire)))
			defer func() { _ = body.Close() }()
			out, err := io.ReadAll(body)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(out, []byte("run_officejs")) || bytes.Contains(out, []byte("SECRET_NATIVE_ENVELOPE")) {
				t.Fatal("native tool transport leaked to the client")
			}
			var call object
			added := 0
			err = readEvents(bytes.NewReader(out), func(_ string, data []byte) error {
				var event object
				if err := decode(data, &event); err != nil {
					return err
				}
				if text(event["type"]) == "response.output_item.added" {
					added++
				}
				if text(event["type"]) == "response.output_item.done" {
					call = mustTestValue[object](t, event["item"])
				}
				return nil
			})
			if err != nil || added != 1 || call["namespace"] != "client" || call["name"] != "run" || call["call_id"] != "call_native" {
				t.Fatalf("invalid translated call: %+v, added=%d, err=%v", call, added, err)
			}
			if kind == "function" && !strings.Contains(text(call["arguments"]), "9007199254740993") {
				t.Fatal("large integer precision was lost")
			}
			outputType := "function_call_output"
			if kind == "custom" {
				outputType = "custom_tool_call_output"
			}
			source["input"] = []any{message("user", "hello"), call, object{"type": outputType, "call_id": "call_native", "output": "done"}, object{"type": "reasoning", "encrypted_content": "encrypted"}}
			next, _ := mustPrepare(t, source, "account/key/session", cache)
			items := mustTestValue[[]any](t, next["input"])
			var restored object
			for _, raw := range items {
				item := mustTestValue[object](t, raw)
				if isTool(item) {
					restored = item
				}
			}
			if !reflect.DeepEqual(native, restored) {
				t.Fatalf("native item not restored: got=%+v want=%+v", restored, native)
			}
			if cache.get("other-account/key/session", "call_native") != nil {
				t.Fatal("native identity crossed account boundaries")
			}
		})
	}
}

func TestMalformedAndUndeclaredCallsFailWithoutDispatch(t *testing.T) {
	source := testSource()
	source["tools"] = []any{object{"type": "function", "name": "shell"}}
	for _, native := range []object{
		nativeCall(object{"name": "delete_workbook", "arguments": object{}}),
		nativeCall(object{"name": "shell", "tool": "other", "args": object{}}),
		nativeCall(object{"name": "shell", "arguments": object{}, "args": object{}}),
		{"type": "function_call", "name": "run_officejs", "arguments": `{"code":"Excel.run(...)"}`},
		// A direct native call to a tool outside the client's catalog stays rejected;
		// direct calls to declared tools are recovered separately in direct_call_test.go.
		{"type": "function_call", "name": "delete_workbook", "arguments": `{}`, "call_id": "call_other"},
	} {
		_, bridge := mustPrepare(t, source, "", nil)
		body := bridge.Stream(io.NopCloser(strings.NewReader(sse(object{"type": "response.completed", "response": object{"output": []any{native}}}))))
		out, err := io.ReadAll(body)
		_ = body.Close()
		if err != nil || !bytes.Contains(out, []byte("response.failed")) || bytes.Contains(out, []byte("response.output_item.added")) {
			t.Fatalf("unsafe tool was not rejected: %s; %v", out, err)
		}
	}
}

func TestToolAliasAndObjectArgumentsPreserveCompleteReplay(t *testing.T) {
	for _, outerObject := range []bool{false, true} {
		t.Run(fmt.Sprint(outerObject), func(t *testing.T) {
			cache := new(ReplayCache)
			source := testSource()
			source["tools"] = []any{object{"type": "function", "name": "get_weather"}}
			_, bridge := mustPrepare(t, source, "account/key", cache)
			native := nativeCall(object{"tool": "get_weather", "args": object{"city": "Tokyo", "note": "a \"quote\" and \\ slash"}})
			if outerObject {
				var arguments object
				if err := decode([]byte(text(native["arguments"])), &arguments); err != nil {
					t.Fatal(err)
				}
				native["arguments"] = arguments
			}
			call, err := bridge.translateCall(native)
			if err != nil {
				t.Fatal(err)
			}
			var args object
			if err := decode([]byte(text(call["arguments"])), &args); err != nil || call["name"] != "get_weather" || args["city"] != "Tokyo" || args["note"] != "a \"quote\" and \\ slash" {
				t.Fatalf("nested JSON was not decoded correctly: %+v, %v", call, err)
			}
			output := object{"type": "function_call_output", "call_id": call["call_id"], "output": "18 C"}
			source["input"] = []any{message("user", "weather"), call, output}
			next, _ := mustPrepare(t, source, "account/key", cache)
			items := mustTestValue[[]any](t, next["input"])
			output["id"] = "fc_" + text(call["call_id"])
			if !reflect.DeepEqual(items[len(items)-2], native) || !reflect.DeepEqual(items[len(items)-1], output) {
				t.Fatal("replay lost the original item ID, arguments, references or tool result")
			}
		})
	}
}

func TestMissingOriginalToolItemCannotBeFabricatedFromOutputOnly(t *testing.T) {
	source := testSource()
	source["tools"] = []any{object{"type": "function", "name": "get_weather"}}
	source["input"] = []any{message("user", "weather"), object{"type": "function_call_output", "call_id": "call_missing", "output": "18 C"}}
	raw, _ := json.Marshal(source)
	if _, _, err := Prepare(raw, "account/key", new(ReplayCache)); err == nil || !strings.Contains(err.Error(), "original tool item is unavailable") || !strings.Contains(err.Error(), "path=input[1]") || !strings.Contains(err.Error(), "matching complete tool call") || strings.Contains(err.Error(), "call_missing") {
		t.Fatalf("missing native identity must produce an actionable error: %v", err)
	}
}

func TestToolLoopKeepsTurnIdentityUntilNextUserMessage(t *testing.T) {
	cache := new(ReplayCache)
	source := testSource()
	delete(source, "prompt_cache_key")
	source["tools"] = []any{object{"type": "function", "name": "update_plan"}}
	history := []any{message("user", "complete the task")}
	source["input"] = history
	first, bridge := mustPrepare(t, source, "account/key", cache)
	initial := mustTestValue[object](t, first["metadata"])
	if initial["agent_iteration"] != "1" {
		t.Fatal("first iteration must be one")
	}
	for i := 1; i <= 2; i++ {
		native := nativeCall(object{"tool": "update_plan", "args": object{"step": fmt.Sprint(i)}})
		native["id"], native["call_id"] = fmt.Sprint("fc_", i), fmt.Sprint("call_", i)
		call, err := bridge.translateCall(native)
		if err != nil {
			t.Fatal(err)
		}
		history = append(history, call, object{"type": "function_call_output", "call_id": call["call_id"], "output": "completed"})
		source["input"] = history
		next, nextBridge := mustPrepare(t, source, "account/key", cache)
		bridge = nextBridge
		metadata := mustTestValue[object](t, next["metadata"])
		if metadata["turn_id"] != initial["turn_id"] || metadata["task_id"] != initial["task_id"] || metadata["agent_iteration"] != fmt.Sprint(i+1) {
			t.Fatalf("tool loop changed identity or lost its iteration: %+v", metadata)
		}
		retry, _ := mustPrepare(t, source, "account/key", cache)
		if !reflect.DeepEqual(metadata, retry["metadata"]) {
			t.Fatal("a network retry must not start a new iteration")
		}
	}
	source["input"] = append(history, message("user", "next task"))
	next, _ := mustPrepare(t, source, "account/key", cache)
	metadata := mustTestValue[object](t, next["metadata"])
	if metadata["turn_id"] == initial["turn_id"] || metadata["task_id"] != initial["task_id"] || metadata["agent_iteration"] != "1" {
		t.Fatalf("a new user turn must reset only the turn and iteration: %+v", metadata)
	}
}

func TestIncrementalTextCancellationAndTerminalEOF(t *testing.T) {
	_, bridge := mustPrepare(t, testSource(), "", nil)
	upstream, producer := io.Pipe()
	body := bridge.Stream(upstream)
	writeDone := make(chan error, 1)
	go func() {
		_, err := io.WriteString(producer, sse(object{"type": "response.output_text.delta", "delta": "first"}))
		writeDone <- err
	}()
	reader := bufio.NewReader(body)
	line, err := reader.ReadString('\n')
	if err != nil || !strings.Contains(line, "response.output_text.delta") {
		t.Fatalf("text did not stream before completion: %s, %v", line, err)
	}
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}
	_ = body.Close()
	if _, err := producer.Write([]byte("next")); err == nil {
		t.Fatal("downstream close must interrupt upstream")
	}
	_ = producer.Close()
	upstream2, producer2 := io.Pipe()
	body2 := bridge.Stream(upstream2)
	go func() {
		_, _ = io.WriteString(producer2, sse(object{"type": "response.completed", "response": object{"output": []any{}}}))
	}()
	if _, err := io.ReadAll(body2); err != nil {
		t.Fatal(err)
	}
	_ = body2.Close()
	_ = producer2.Close()
}

func TestLiteCatalogAndCompactionStayOrdered(t *testing.T) {
	source := testSource()
	source["input"] = []any{object{"type": "additional_tools", "tools": []any{object{"type": "function", "name": "shell"}}}, object{"type": "compaction_trigger"}, message("user", "hello")}
	out, bridge := mustPrepare(t, source, "", nil)
	items := mustTestValue[[]any](t, out["input"])
	if text(mustTestValue[object](t, items[len(items)-1])["type"]) != "compaction_trigger" || len(bridge.tools) != 1 {
		t.Fatal("Lite tool catalog or terminal compaction was lost")
	}
}

func TestReplayCacheEvictionIsBounded(t *testing.T) {
	cache := new(ReplayCache)
	for i := 0; i < 1025; i++ {
		cache.put("scope", fmt.Sprint(i), object{"id": fmt.Sprint(i)})
	}
	if len(cache.entries) != 1024 || cache.get("scope", "0") != nil || cache.get("scope", "1024") == nil {
		t.Fatal("replay cache did not evict the oldest entry")
	}
}
