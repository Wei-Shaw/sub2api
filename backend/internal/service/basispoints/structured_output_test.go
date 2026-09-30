package basispoints

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func testStructuredFormat() object {
	return object{"type": "json_schema", "name": "answer", "strict": true, "schema": object{
		"type": "object", "properties": object{"answer": object{"type": "integer", "enum": []any{json.Number("9007199254740993")}}},
		"required": []any{"answer"}, "additionalProperties": false,
	}}
}

func testStructuredSource(format object) object {
	source := testSource()
	source["text"] = object{"format": format}
	return source
}

func structuredMessage(value string) object {
	return object{"type": "message", "id": "msg_structured", "role": "assistant", "status": "completed",
		"content": []any{object{"type": "output_text", "text": value, "annotations": []any{}}}}
}

func structuredTerminal(kind string, output ...any) string {
	return sse(object{"type": kind, "response": object{"id": "resp_structured", "status": strings.TrimPrefix(kind, "response."), "output": output}})
}

func structuredEvents(t *testing.T, bridge *Bridge, wire string) []object {
	t.Helper()
	stream := bridge.Stream(io.NopCloser(strings.NewReader(wire)))
	defer func() { _ = stream.Close() }()
	var events []object
	err := readEvents(stream, func(_ string, data []byte) error {
		var event object
		if err := decode(data, &event); err != nil {
			return err
		}
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, event := range events {
		if event["sequence_number"] != json.Number(strconv.Itoa(i)) {
			t.Fatalf("non-contiguous event sequence: %v", events)
		}
	}
	return events
}

func TestStructuredOutputPreservesModelAndSchema(t *testing.T) {
	for _, model := range []string{"gpt-6-luna", "gpt-6-sol", "gpt-6-astra"} {
		t.Run(model, func(t *testing.T) {
			source := testStructuredSource(testStructuredFormat())
			source["model"] = model
			before, err := json.Marshal(source)
			if err != nil {
				t.Fatal(err)
			}
			body, bridge := mustPrepare(t, source, "account/key/thread", nil)
			after, err := json.Marshal(source)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || body["model"] != model || bridge.structured == nil {
				t.Fatal("structured preparation changed input or model")
			}
			for _, key := range []string{"text", "response_format"} {
				if _, present := body[key]; present {
					t.Fatalf("BPS rejects native field %s", key)
				}
			}
			wire, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(wire), "9007199254740993") || !strings.Contains(string(wire), "exactly one JSON value") {
				t.Fatal("schema constraints or JSON instructions were lost")
			}
		})
	}
}

func TestStructuredOutputRejectsInvalidFormats(t *testing.T) {
	cases := []any{
		"invalid", object{"format": "invalid"}, object{"format": object{}},
		object{"format": object{"type": "grammar"}},
		object{"format": object{"type": "json_object", "schema": object{}}},
		object{"format": object{"type": "json_schema", "name": "bad name", "schema": object{}}},
		object{"format": object{"type": "json_schema", "name": "answer", "schema": false}},
		object{"format": object{"type": "json_schema", "name": "answer", "schema": object{"type": "invalid"}}},
		object{"format": object{"type": "json_schema", "name": "answer", "schema": object{}, "strict": "true"}},
		object{"format": object{"type": "json_schema", "name": "answer", "schema": object{}, "description": 42}},
		object{"format": object{"type": "json_schema", "name": "answer", "schema": object{"description": strings.Repeat("x", 1<<20)}}},
	}
	for _, raw := range cases {
		if _, err := prepareStructuredOutput(raw); err == nil {
			t.Fatalf("invalid format accepted: %T", raw)
		}
	}
	for _, raw := range []any{nil, object{}, object{"format": nil}, object{"format": object{"type": "text"}}} {
		if format, err := prepareStructuredOutput(raw); err != nil || format != nil {
			t.Fatalf("plain text behavior changed: %v", err)
		}
	}
}

func TestStructuredSchemaReferencesStayLocal(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, "{}")
	}))
	defer server.Close()
	for _, ref := range []string{server.URL + "/schema", "file:///etc/passwd"} {
		for _, keyword := range []string{"$ref", "$schema"} {
			format := testStructuredFormat()
			format["schema"] = object{keyword: ref}
			if _, err := prepareStructuredOutput(object{"format": format}); err == nil {
				t.Fatalf("external %s was accepted", keyword)
			}
		}
	}
	if calls.Load() != 0 {
		t.Fatal("schema compilation made a network request")
	}
	format := testStructuredFormat()
	format["schema"] = object{"$defs": object{"answer": object{"type": "string", "pattern": "^ok$"}}, "$ref": "#/$defs/answer"}
	_, bridge := mustPrepare(t, testStructuredSource(format), "scope", nil)
	for answer, valid := range map[string]bool{"\"ok\"": true, "\"bad\"": false} {
		err := bridge.structured.validate(object{"output": []any{structuredMessage(answer)}})
		if (err == nil) != valid {
			t.Fatalf("local reference validation = %v for %s", err, answer)
		}
	}
}

func TestStructuredOutputUsesValidatedTerminalText(t *testing.T) {
	format := testStructuredFormat()
	_, bridge := mustPrepare(t, testStructuredSource(format), "scope", nil)
	value := "{\"answer\":9007199254740993}"
	wire := sse(object{"type": "response.created", "response": object{"id": "resp_structured", "output": []any{structuredMessage("UNVALIDATED")}}}) +
		sse(object{"type": "response.output_item.added", "output_index": 0, "item": structuredMessage("UNVALIDATED")}) +
		sse(object{"type": "response.output_text.delta", "delta": "UNVALIDATED"}) +
		sse(object{"type": "response.content_part.done", "part": object{"type": "output_text", "text": "UNVALIDATED"}}) +
		structuredTerminal("response.completed", structuredMessage(value))
	events := structuredEvents(t, bridge, wire)
	var kinds []string
	for _, event := range events {
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "UNVALIDATED") {
			t.Fatal("unvalidated upstream text leaked")
		}
		kind := text(event["type"])
		kinds = append(kinds, kind)
		if kind == "response.output_text.delta" && event["delta"] != value {
			t.Fatal("delta must come from the validated terminal answer")
		}
	}
	want := []string{"response.created", "response.output_item.added", "response.content_part.added", "response.output_text.delta", "response.output_text.done", "response.content_part.done", "response.output_item.done", "response.completed"}
	if !reflect.DeepEqual(want, kinds) {
		t.Fatalf("message event sequence = %v", kinds)
	}
	response := mustTestValue[object](t, events[len(events)-1]["response"])
	config := mustTestValue[object](t, response["text"])
	if !reflect.DeepEqual(config["format"], format) {
		t.Fatal("requested format was not retained in the response")
	}
}

func TestStructuredOutputInvalidAnswersNeverComplete(t *testing.T) {
	for _, answer := range []string{"NOT_JSON", "{}", "{\"answer\":9007199254740992}", "{\"answer\":9007199254740993,\"extra\":true}", "{\"answer\":\"wrong\"}", "{\"answer\":9007199254740993} null", ""} {
		_, bridge := mustPrepare(t, testStructuredSource(testStructuredFormat()), "scope", nil)
		events := structuredEvents(t, bridge, sse(object{"type": "response.output_text.delta", "delta": "UNVALIDATED"})+structuredTerminal("response.completed", structuredMessage(answer)))
		if len(events) != 1 || events[0]["type"] != "response.failed" {
			t.Fatalf("invalid answer emitted content or completed: %v", events)
		}
		response := mustTestValue[object](t, events[0]["response"])
		if len(mustTestValue[[]any](t, response["output"])) != 0 {
			t.Fatal("invalid answer was included in the error")
		}
	}
}

func TestStructuredJSONModeAndRefusals(t *testing.T) {
	for answer, valid := range map[string]bool{"{\"ok\":true}": true, "[1,2]": true, "null": true, "{\"ok\":true} {}": false, "not JSON": false} {
		_, bridge := mustPrepare(t, testStructuredSource(object{"type": "json_object"}), "scope", nil)
		events := structuredEvents(t, bridge, structuredTerminal("response.completed", structuredMessage(answer)))
		if (events[len(events)-1]["type"] == "response.completed") != valid {
			t.Fatalf("JSON validation mismatch for %s", answer)
		}
	}
	_, bridge := mustPrepare(t, testStructuredSource(testStructuredFormat()), "scope", nil)
	refusal := structuredMessage("")
	refusal["content"] = []any{object{"type": "refusal", "refusal": "Cannot help with that request."}}
	events := structuredEvents(t, bridge, structuredTerminal("response.completed", refusal))
	found := false
	for _, event := range events {
		if event["type"] == "response.refusal.delta" {
			found = true
		}
		if event["type"] == "response.output_text.delta" {
			t.Fatal("refusal was replaced with JSON text")
		}
	}
	if !found || events[len(events)-1]["type"] != "response.completed" {
		t.Fatal("refusal was not preserved")
	}
}

func TestStructuredOutputPreservesToolTurnsAndUpstreamFailures(t *testing.T) {
	source := testStructuredSource(testStructuredFormat())
	source["tools"] = []any{object{"type": "function", "name": "get_weather", "parameters": object{"type": "object"}}}
	_, bridge := mustPrepare(t, source, "scope", new(ReplayCache))
	native := nativeCall(object{"name": "get_weather", "arguments": object{}})
	events := structuredEvents(t, bridge, structuredTerminal("response.completed", native))
	if events[len(events)-1]["type"] != "response.completed" {
		t.Fatal("a tool continuation was incorrectly required to contain final JSON")
	}
	response := mustTestValue[object](t, events[len(events)-1]["response"])
	output := mustTestValue[[]any](t, response["output"])
	call := mustTestValue[object](t, output[0])
	if call["name"] != "get_weather" || call["call_id"] != "call_native" {
		t.Fatal("tool identity was not preserved")
	}
	for _, kind := range []string{"response.failed", "response.incomplete"} {
		_, bridge := mustPrepare(t, source, "scope", nil)
		events := structuredEvents(t, bridge, sse(object{"type": "response.output_text.delta", "delta": "UNVALIDATED"})+structuredTerminal(kind, structuredMessage("UNVALIDATED")))
		if len(events) != 1 || events[0]["type"] != kind {
			t.Fatal("upstream terminal status was changed")
		}
		response := mustTestValue[object](t, events[0]["response"])
		if len(mustTestValue[[]any](t, response["output"])) != 0 {
			t.Fatal("incomplete structured text leaked")
		}
	}
}

func TestStructuredOutputRequiresAuthoritativeCompletion(t *testing.T) {
	_, bridge := mustPrepare(t, testStructuredSource(testStructuredFormat()), "scope", nil)
	events := structuredEvents(t, bridge, sse(object{"type": "response.completed"}))
	if len(events) != 1 || events[0]["type"] != "response.failed" {
		t.Fatal("missing terminal response was accepted")
	}
	stream := bridge.Stream(io.NopCloser(strings.NewReader(sse(object{"type": "response.output_text.delta", "delta": "UNVALIDATED"}))))
	defer func() { _ = stream.Close() }()
	body, err := io.ReadAll(stream)
	if !errors.Is(err, io.ErrUnexpectedEOF) || len(body) != 0 {
		t.Fatalf("interrupted structured output leaked: %s, %v", body, err)
	}
}

func TestStructuredOutputChecksNestedSchemaConstraints(t *testing.T) {
	format := testStructuredFormat()
	format["schema"] = object{
		"type": "object", "properties": object{"values": object{"type": "array", "minItems": 1, "maxItems": 2,
			"items": object{"anyOf": []any{object{"type": "integer", "minimum": 1, "maximum": 5}, object{"type": "null"}}}}},
		"required": []any{"values"}, "additionalProperties": false,
	}
	_, bridge := mustPrepare(t, testStructuredSource(format), "scope", nil)
	for answer, valid := range map[string]bool{
		"{\"values\":[1,null]}": true, "{\"values\":[]}": false, "{\"values\":[9]}": false,
		"{\"values\":[\"1\"]}": false, "{\"values\":[1,2,3]}": false,
	} {
		if err := bridge.structured.validate(object{"output": []any{structuredMessage(answer)}}); (err == nil) != valid {
			t.Fatalf("nested schema validation mismatch: %s: %v", answer, err)
		}
	}
}
