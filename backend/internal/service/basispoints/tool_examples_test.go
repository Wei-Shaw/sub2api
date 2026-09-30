package basispoints

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func exampleFunction(name, field string) object {
	return object{"type": "function", "name": name, "parameters": object{
		"type": "object", "required": []any{field}, "additionalProperties": false,
		"properties": object{field: object{"type": "string"}},
	}}
}

func TestToolExamplesUseCallableCatalog(t *testing.T) {
	cases := []struct {
		name       string
		tools      []any
		additional []any
		disabled   bool
		count      int
	}{
		{name: "custom patch", tools: []any{object{"type": "custom", "name": "apply_patch"}}, count: 1},
		{name: "function patch", tools: []any{exampleFunction("apply_patch", "patch")}, count: 1},
		{name: "raw code", tools: []any{exampleFunction("exec", "code")}, count: 1},
		{name: "raw cmd", tools: []any{exampleFunction("exec_command", "cmd")}, count: 1},
		{name: "namespace", tools: []any{object{"type": "namespace", "name": "plugin", "tools": []any{exampleFunction("apply_patch", "patch"), object{"type": "custom", "name": "exec"}}}}, count: 2},
		{name: "different schema", tools: []any{exampleFunction("exec_command", "request")}},
		{name: "constrained custom", tools: []any{object{"type": "custom", "name": "exec", "format": object{"type": "grammar", "syntax": "regex", "definition": "ONLY_THIS"}}}},
		{name: "unrelated", tools: []any{exampleFunction("echo", "text")}},
		{name: "disabled", tools: []any{exampleFunction("exec_command", "cmd")}, disabled: true},
		{name: "additional", additional: []any{exampleFunction("apply_patch", "patch")}, count: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := testSource()
			source["tools"] = tc.tools
			if tc.disabled {
				source["tool_choice"] = "none"
			}
			if tc.additional != nil {
				source["input"] = []any{object{"type": "additional_tools", "tools": tc.additional}, message("user", "Continue.")}
			}
			prepared, bridge := mustPrepare(t, source, "examples", nil)
			var protocol strings.Builder
			for _, raw := range mustTestValue[[]any](t, prepared["input"]) {
				item := mustTestValue[object](t, raw)
				if item["role"] != "developer" {
					continue
				}
				for _, part := range mustTestValue[[]any](t, item["content"]) {
					_, _ = protocol.WriteString(text(mustTestValue[object](t, part)["text"]))
				}
			}
			if strings.Contains(protocol.String(), "For example, custom functions.exec") {
				t.Error("unconditional tool identity example remains")
			}
			pieces := strings.Split(protocol.String(), "\nExample native run_officejs arguments: ")
			if len(pieces)-1 != tc.count {
				t.Fatalf("got %d examples, want %d", len(pieces)-1, tc.count)
			}
			for i, piece := range pieces[1:] {
				var outer object
				decoder := json.NewDecoder(strings.NewReader(piece))
				decoder.UseNumber()
				if err := decoder.Decode(&outer); err != nil {
					t.Fatal(err)
				}
				encoded, err := json.Marshal(outer)
				if err != nil {
					t.Fatal(err)
				}
				call, err := bridge.translateCall(object{"type": "function_call", "name": "run_officejs", "call_id": fmt.Sprintf("example-%d", i), "arguments": string(encoded)})
				if err != nil {
					t.Fatalf("example rejected by production parser: %v", err)
				}
				name := text(call["name"])
				if ns := text(call["namespace"]); ns != "" {
					name = ns + "." + name
				}
				info, ok := bridge.tools[name]
				if !ok {
					t.Fatalf("example references absent tool %s", name)
				}
				if info.Kind == "function" {
					var args object
					if err := decode([]byte(text(call["arguments"])), &args); err != nil {
						t.Fatal(err)
					}
					if info.Schema == nil || info.Schema.Validate(args) != nil {
						t.Fatal("example violates declared schema")
					}
				} else if text(outer["summary"]) != customTransportPrefix+name || call["input"] != outer["code"] {
					t.Fatal("custom example changed raw input or transport")
				}
			}
			again, _ := mustPrepare(t, source, "examples", nil)
			firstJSON, _ := json.Marshal(prepared)
			secondJSON, _ := json.Marshal(again)
			if string(firstJSON) != string(secondJSON) {
				t.Fatal("examples are not deterministic")
			}
		})
	}
}
