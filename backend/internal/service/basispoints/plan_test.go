package basispoints

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func nativePlanTestSchema() object {
	return object{
		"type": "object", "required": []any{"plan"}, "additionalProperties": false,
		"properties": object{
			"explanation": object{"type": "string"},
			"plan": object{"type": "array", "minItems": json.Number("1"), "items": object{
				"type": "object", "required": []any{"step", "status"}, "additionalProperties": false,
				"properties": object{
					"step":   object{"type": "string", "minLength": json.Number("1")},
					"status": object{"type": "string", "enum": []any{"pending", "in_progress", "completed"}},
				},
			}},
		},
	}
}

func nativePlanTestBridge(namespace string) *Bridge {
	key := "update_plan"
	if namespace != "" {
		key = namespace + "." + key
	}
	return &Bridge{tools: map[string]tool{key: {Name: "update_plan", Namespace: namespace, Kind: "function", Parameters: nativePlanTestSchema()}}, replay: new(ReplayCache), scope: "plan-account/key"}
}

func nativePlanTestItem(arguments any) object {
	return object{"type": "function_call", "name": "update_plan", "id": "fc_plan_native", "call_id": "call_plan_native", "arguments": arguments, "summary": "Native summary retained", "references": []any{"native reference"}, "status": "completed"}
}

func TestNativePlanAdaptsOnlyDeclaredFunctionAndPreservesOriginal(t *testing.T) {
	for _, namespace := range []string{"", "functions"} {
		for _, nativeName := range []string{"update_plan", "functions.update_plan"} {
			for _, encodeArguments := range []bool{false, true} {
				bridge := nativePlanTestBridge(namespace)
				arguments := object{"summary": "Follow the user's plan", "plan": []any{
					object{"id": "step1", "description": "Inspect files", "status": "done", "result": "Existing result preserved upstream"},
					object{"id": "step2", "title": "Run tests", "status": "active"},
					object{"id": "step3", "step": "Report findings", "status": "not_started"},
				}}
				var input any = arguments
				if encodeArguments {
					raw, _ := json.Marshal(arguments)
					input = string(raw)
				}
				native := nativePlanTestItem(input)
				native["name"] = nativeName
				translated, err := bridge.translateNativePlan(native)
				if err != nil {
					t.Fatal(err)
				}
				if translated["name"] != "update_plan" || translated["call_id"] != native["call_id"] || translated["id"] != native["id"] || translated["type"] != "function_call" {
					t.Fatal("native plan lost its client or transport identity")
				}
				if namespace != "" && translated["namespace"] != namespace {
					t.Fatal("client plan namespace was not retained")
				}
				var got object
				if err := decode([]byte(text(translated["arguments"])), &got); err != nil {
					t.Fatal(err)
				}
				want := object{"explanation": "Follow the user's plan", "plan": []any{
					object{"step": "Inspect files", "status": "completed"},
					object{"step": "Run tests", "status": "in_progress"},
					object{"step": "Report findings", "status": "pending"},
				}}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("unexpected normalized plan: %#v", got)
				}
				if !reflect.DeepEqual(bridge.replay.get(bridge.scope, "call_plan_native"), native) {
					t.Fatal("complete native plan was not cached intact")
				}
				output := object{"type": "function_call_output", "call_id": "call_plan_native", "output": "client explicitly rejected the plan"}
				history, err := bridge.translateHistory([]any{translated, output})
				if err != nil || !reflect.DeepEqual(history[0], native) || mustTestValue[object](t, history[1])["output"] != output["output"] {
					t.Fatalf("replay must retain the native item and the client's real result: %v", err)
				}
			}
		}
	}
}

func TestNativePlanRejectsUnavailableOrAmbiguousCatalog(t *testing.T) {
	for _, change := range []func(*Bridge){
		func(b *Bridge) { b.tools = nil },
		func(b *Bridge) { b.tools["functions.update_plan"] = b.tools["update_plan"] },
		func(b *Bridge) { info := b.tools["update_plan"]; info.Kind = "custom"; b.tools["update_plan"] = info },
		func(b *Bridge) { info := b.tools["update_plan"]; info.Parameters = nil; b.tools["update_plan"] = info },
		func(b *Bridge) {
			info := b.tools["update_plan"]
			info.Parameters = object{"type": "object"}
			b.tools["update_plan"] = info
		},
		func(b *Bridge) { b.tools["other.update_plan"] = b.tools["update_plan"]; delete(b.tools, "update_plan") },
	} {
		bridge := nativePlanTestBridge("")
		change(bridge)
		if _, err := bridge.translateNativePlan(nativePlanTestItem(object{"plan": []any{object{"step": "Inspect", "status": "pending"}}})); err == nil {
			t.Fatal("native plan accepted without one compatible declared client function")
		}
	}
}

func TestNativePlanRejectsMalformedArgumentsWithoutDroppingSteps(t *testing.T) {
	for _, arguments := range []any{
		nil, `{"plan":`, `{"plan":[]} {}`, object{}, object{"plan": nil}, object{"plan": "invalid"},
		object{"plan": []any{}},
		object{"plan": []any{"invalid"}},
		object{"plan": []any{object{"step": "Inspect", "status": "invented"}}},
		object{"plan": []any{object{"description": "Inspect", "step": "Different", "status": "pending"}}},
		object{"plan": []any{object{"step": 7, "description": "Inspect", "status": "pending"}}},
		object{"plan": []any{object{"step": " ", "status": "pending"}}},
		object{"plan": []any{object{"step": "Inspect", "status": 1}}},
		object{"plan": []any{object{"step": "Inspect", "status": "pending"}, object{"status": "pending"}}},
		object{"plan": []any{object{"step": "Inspect", "status": "pending"}}, "summary": "One", "explanation": "Two"},
		object{"plan": []any{object{"step": "Inspect", "status": "pending"}}, "explanation": nil},
	} {
		bridge := nativePlanTestBridge("")
		if _, err := bridge.translateNativePlan(nativePlanTestItem(arguments)); err == nil {
			t.Fatal("native plan accepted malformed or ambiguous arguments")
		}
		if bridge.replay.get(bridge.scope, "call_plan_native") != nil {
			t.Fatal("a rejected plan must not be remembered as a completed client call")
		}
	}
}

func TestNativePlanRespectsRequiredAndUnsupportedSchemaConstraints(t *testing.T) {
	schemaAt := func(s object, path ...string) object {
		for _, key := range path {
			s = mustTestValue[object](t, s[key])
		}
		return s
	}
	for _, change := range []func(object){
		func(s object) { s["required"] = []any{"plan", "approval"} },
		func(s object) { s["required"] = []any{"plan", "explanation"} },
		func(s object) { s["allOf"] = []any{object{}} },
		func(s object) { s["$ref"] = "#/$defs/plan" },
		func(s object) { s["additionalProperties"] = object{"type": "string"} },
		func(s object) {
			schemaAt(s, "properties", "plan")["minItems"] = json.Number("2")
		},
		func(s object) {
			schemaAt(s, "properties", "plan")["maxItems"] = json.Number("0")
		},
		func(s object) {
			schemaAt(s, "properties", "plan")["minItems"] = json.Number("1.5")
		},
		func(s object) {
			schemaAt(s, "properties", "plan", "items")["required"] = []any{"step", "status", "owner"}
		},
		func(s object) {
			schemaAt(s, "properties", "plan", "items")["properties"] = "invalid schema"
		},
		func(s object) {
			schemaAt(s, "properties", "plan", "items", "properties", "status")["enum"] = []any{"completed"}
		},
		func(s object) {
			schemaAt(s, "properties", "plan", "items", "properties", "step")["pattern"] = "^authorized$"
		},
		func(s object) {
			schemaAt(s, "properties", "plan", "items", "properties", "step")["maxLength"] = json.Number("2")
		},
	} {
		bridge := nativePlanTestBridge("")
		change(bridge.tools["update_plan"].Parameters)
		if _, err := bridge.translateNativePlan(nativePlanTestItem(object{"plan": []any{object{"step": "Inspect", "status": "pending"}}})); err == nil {
			t.Fatal("native plan accepted a contract it cannot satisfy or validate")
		}
	}
}

func TestNativePlanRejectsUnrelatedNativeCallsAndMissingIdentity(t *testing.T) {
	for _, change := range []func(object){
		func(n object) { n["name"] = "read_ranges" },
		func(n object) { n["name"] = "other.update_plan" },
		func(n object) { n["type"] = "custom_tool_call" },
		func(n object) { delete(n, "call_id") },
		func(n object) { n["call_id"] = " invalid " },
	} {
		bridge := nativePlanTestBridge("")
		native := nativePlanTestItem(object{"plan": []any{object{"step": "private-plan-text", "status": "pending"}}})
		change(native)
		_, err := bridge.translateNativePlan(native)
		if err == nil || strings.Contains(err.Error(), "private-plan-text") {
			t.Fatal("unrelated native tools and missing identities must be rejected without leaking arguments")
		}
	}
}
