package service

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/tidwall/gjson"
)

func TestToolChangeReferencesFollowExistingRewrite(t *testing.T) {
	body := []byte(`{"tools":[{"name":"read"},{"name":"bash"},{"name":"codemode"},{"name":"subagent_supervisor"},{"name":"cron_silent"},{"name":"__pi_deferred_placeholder__","defer_loading":true},{"name":"web_search","type":"web_search_20250305"}],"tool_choice":{"type":"tool","name":"subagent_supervisor"},"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"call-1","name":"subagent_supervisor","input":{"text":"subagent_supervisor"}}]},{"role":"system","content":[{"type":"text","text":"Keep subagent_supervisor in this literal text."},{"type":"tool_removal","tool":{"type":"tool_reference","name":"subagent_supervisor"}},{"type":"tool_removal","tool":{"type":"tool_reference","name":"cron_silent"}},{"type":"tool_addition","tool":{"type":"tool_definition","definition":{"name":"codemode","description":"codemode subagent_supervisor","input_schema":{"type":"object","properties":{"subagent_supervisor":{"type":"string"}}}}}},{"type":"tool_addition","tool":{"type":"tool_reference","name":"read"}},{"type":"tool_addition","tool":{"type":"tool_definition","definition":{"name":"late_lookup","input_schema":{"type":"object"}}}},{"type":"tool_addition","tool":{"type":"tool_definition","definition":{"name":"web_search","type":"web_search_20250305"}}},{"type":"tool_removal","tool":{"type":"mcp_tool_reference","server_name":"subagent_supervisor","name":"read"}}]},{"role":"user","content":[{"type":"tool_reference","tool_name":"read"},{"type":"tool_result","tool_use_id":"call-1","content":[{"type":"tool_reference","tool_name":"codemode"},{"type":"text","text":"subagent_supervisor"}]}]}]}`)
	original := append([]byte(nil), body...)
	rw := buildToolNameRewriteFromBody(body)
	if rw == nil {
		t.Fatal("fixture must exercise the production dynamic-name rewrite")
	}
	out := applyToolNameRewriteToBody(body, rw)
	for path, real := range map[string]string{
		"tool_choice.name":                          "subagent_supervisor",
		"messages.0.content.0.name":                 "subagent_supervisor",
		"messages.1.content.1.tool.name":            "subagent_supervisor",
		"messages.1.content.2.tool.name":            "cron_silent",
		"messages.1.content.3.tool.definition.name": "codemode",
		"messages.1.content.4.tool.name":            "read",
		"messages.2.content.0.tool_name":            "read",
		"messages.2.content.1.content.0.tool_name":  "codemode",
	} {
		if got := gjson.GetBytes(out, path).String(); got != rw.Forward[real] {
			t.Errorf("%s = %q, want the same alias as its declaration: %q", path, got, rw.Forward[real])
		}
	}
	for path, want := range map[string]string{
		"messages.0.content.0.id":                          "call-1",
		"messages.0.content.0.input.text":                  "subagent_supervisor",
		"messages.1.content.0.text":                        "Keep subagent_supervisor in this literal text.",
		"messages.1.content.3.tool.definition.description": "codemode subagent_supervisor",
		"messages.1.content.5.tool.definition.name":        "late_lookup",
		"messages.1.content.6.tool.definition.name":        "web_search",
		"messages.1.content.7.tool.server_name":            "subagent_supervisor",
		"messages.1.content.7.tool.name":                   "read",
		"messages.2.content.1.tool_use_id":                 "call-1",
		"messages.2.content.1.content.1.text":              "subagent_supervisor",
	} {
		if got := gjson.GetBytes(out, path).String(); got != want {
			t.Errorf("unrelated field %s changed: got %q, want %q", path, got, want)
		}
	}
	if !gjson.GetBytes(out, "messages.1.content.3.tool.definition.input_schema.properties.subagent_supervisor").Exists() {
		t.Error("tool schema property names must remain unchanged")
	}
	if string(body) != string(original) {
		t.Error("request transformation mutated the caller's original bytes")
	}
	if got := gjson.GetBytes(out, "messages.1.content.#").Int(); got != 8 {
		t.Fatalf("tool changes must all be retained, got %d blocks", got)
	}
	response := []byte(`data: {"type":"content_block_start","content_block":{"type":"tool_use","name":"` + rw.Forward["subagent_supervisor"] + `","id":"reply-1","input":{}}}`)
	if got := gjson.Get(string(restoreToolNamesInBytes(response, rw))[6:], "content_block.name").String(); got != "subagent_supervisor" {
		t.Errorf("streamed tool name did not round-trip: %q", got)
	}
}

func TestToolChangeReferencesStaticAndIdentityMappings(t *testing.T) {
	for _, name := range []string{"sessions_list", "ordinary_lookup"} {
		t.Run(name, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{
				"tools": []any{map[string]any{"name": name}},
				"messages": []any{map[string]any{"role": "system", "content": []any{
					map[string]any{"type": "tool_addition", "tool": map[string]any{"type": "tool_reference", "name": name}},
					map[string]any{"type": "tool_removal", "tool": map[string]any{"type": "tool_reference", "name": name}},
				}}},
			})
			if err != nil {
				t.Fatal(err)
			}
			rw := buildToolNameRewriteFromBody(body)
			out := applyToolNameRewriteToBody(body, rw)
			want := gjson.GetBytes(out, "tools.0.name").String()
			for _, path := range []string{"messages.0.content.0.tool.name", "messages.0.content.1.tool.name"} {
				if got := gjson.GetBytes(out, path).String(); got != want {
					t.Errorf("%s = %q, want %q", path, got, want)
				}
			}
		})
	}
}

func TestInlineToolsDoNotReseedExistingCachePrefix(t *testing.T) {
	first := []byte(`{"tools":[{"name":"read"},{"name":"bash"},{"name":"codemode"},{"name":"subagent_supervisor"},{"name":"cron_silent"},{"name":"__pi_deferred_placeholder__"}],"messages":[{"role":"user","content":"synthetic"}]}`)
	second := []byte(`{"tools":[{"name":"read"},{"name":"bash"},{"name":"codemode"},{"name":"subagent_supervisor"},{"name":"cron_silent"},{"name":"__pi_deferred_placeholder__"}],"messages":[{"role":"user","content":"synthetic"},{"role":"system","content":[{"type":"tool_addition","tool":{"type":"tool_definition","definition":{"name":"late_lookup","input_schema":{"type":"object"}}}}]}]}`)
	a, b := buildToolNameRewriteFromBody(first), buildToolNameRewriteFromBody(second)
	if a == nil || b == nil || !reflect.DeepEqual(a.Forward, b.Forward) {
		t.Fatal("later inline tools changed the aliases of the cached initial declarations")
	}
	firstOut, secondOut := applyToolNameRewriteToBody(first, a), applyToolNameRewriteToBody(second, b)
	if gjson.GetBytes(firstOut, "tools").Raw != gjson.GetBytes(secondOut, "tools").Raw {
		t.Error("tool additions changed the request-level tool prefix")
	}
}
