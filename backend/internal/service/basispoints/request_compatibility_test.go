package basispoints

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestHostedToolOmissionIsExplicitAndDeterministic(t *testing.T) {
	source := testSource()
	source["tools"] = []any{object{"type": "function", "name": "get_weather", "parameters": object{"type": "object"}}, object{"type": "web_search"}, object{"type": "image_generation"}}
	body, bridge := mustPrepare(t, source, "scope", nil)
	if len(bridge.Warnings) != 1 || !strings.Contains(bridge.Warnings[0], "image_generation, web_search") {
		t.Fatalf("missing capability warning: %v", bridge.Warnings)
	}
	encoded, _ := json.Marshal(body)
	if !strings.Contains(string(encoded), "Do not claim to have used them") {
		t.Fatal("model must know omitted hosted capabilities are unavailable")
	}
	if _, present := body["tools"]; present {
		t.Fatal("native schema must not be forwarded")
	}
	source["tool_choice"] = "none"
	_, bridge = mustPrepare(t, source, "scope", nil)
	if len(bridge.tools) != 0 || len(bridge.Warnings) != 0 {
		t.Fatal("none must skip the entire tool catalog")
	}
}

func TestTurnWithoutUserRemainsStableAcrossToolResults(t *testing.T) {
	source := testSource()
	source["input"] = []any{message("developer", "synthetic task")}
	first, _ := mustPrepare(t, source, "scope", nil)
	source["input"] = append(mustTestValue[[]any](t, source["input"]), object{"type": "function_call", "call_id": "call_old", "name": "get_weather", "arguments": `{"city":"Tokyo"}`}, object{"type": "function_call_output", "call_id": "call_old", "output": "18 C"})
	next, _ := mustPrepare(t, source, "scope", nil)
	a, b := mustTestValue[object](t, first["metadata"]), mustTestValue[object](t, next["metadata"])
	if !reflect.DeepEqual(a["turn_id"], b["turn_id"]) || b["agent_iteration"] != "2" {
		t.Fatalf("unstable continuation metadata: %v / %v", a, b)
	}
}
