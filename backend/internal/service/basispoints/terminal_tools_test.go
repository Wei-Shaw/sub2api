package basispoints

import (
	"bufio"
	"bytes"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestToolsUseTerminalItemWhileTextStaysIncremental(t *testing.T) {
	cache := new(ReplayCache)
	source := testSource()
	source["tools"] = []any{object{"type": "function", "name": "shell"}}
	_, bridge := mustPrepare(t, source, "scope", cache)
	complete := nativeCall(object{"name": "shell", "arguments": object{"cmd": "pwd"}})
	partial := nativeCall(object{})
	partial["arguments"] = `{"summary":"partial","code":""}`
	upstream, writer := io.Pipe()
	body := bridge.Stream(upstream)
	defer func() { _ = body.Close() }()
	defer func() { _ = writer.Close() }()
	first := sse(object{"type": "response.output_text.delta", "delta": "Working."})
	go func() { _, _ = io.WriteString(writer, first) }()
	reader := bufio.NewReader(body)
	for i := 0; i < 3; i++ {
		if _, err := reader.ReadString('\n'); err != nil {
			t.Fatal(err)
		}
	}
	// No terminal event has been written yet, but the text already arrived.
	go func() {
		_, _ = io.WriteString(writer, sse(object{"type": "response.output_item.done", "item": partial}))
		_, _ = io.WriteString(writer, sse(object{"type": "response.completed", "response": object{"output": []any{complete}}}))
		_ = writer.Close()
	}()
	out, err := io.ReadAll(reader)
	if err != nil || bytes.Contains(out, []byte("response.failed")) || !bytes.Contains(out, []byte(`"name":"shell"`)) {
		t.Fatalf("terminal tool normalization failed: %s, %v", out, err)
	}
	if !reflect.DeepEqual(cache.get("scope", "call_native"), complete) {
		t.Fatal("partial item was retained instead of the authoritative original")
	}
}

func TestTerminalValidationDoesNotDispatchPartialOrMixedTools(t *testing.T) {
	source := testSource()
	source["tools"] = []any{object{"type": "function", "name": "shell"}}
	valid := nativeCall(object{"name": "shell", "arguments": object{}})
	invalid := nativeCall(object{"name": "other", "arguments": object{}})
	invalid["id"], invalid["call_id"] = "fc_other", "call_other"
	for _, terminal := range []object{
		{"type": "response.completed", "response": object{"output": []any{valid, invalid}}},
		{"type": "response.completed", "response": object{"output": []any{}}},
		{"type": "response.incomplete", "response": object{"output": []any{valid}}},
		{"type": "response.failed", "response": object{"output": []any{valid}, "error": object{"code": "upstream_failure"}}},
	} {
		_, bridge := mustPrepare(t, source, "", nil)
		wire := sse(object{"type": "response.output_item.done", "item": valid}) + sse(terminal)
		body := bridge.Stream(io.NopCloser(strings.NewReader(wire)))
		out, err := io.ReadAll(body)
		_ = body.Close()
		if err != nil || bytes.Contains(out, []byte("response.function_call_arguments")) || bytes.Contains(out, []byte("run_officejs")) {
			t.Fatalf("invalid response exposed a tool call: %s, %v", out, err)
		}
	}
}
