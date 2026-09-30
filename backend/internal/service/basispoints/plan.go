package basispoints

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"
)

// translateNativePlan adapts only a declared client plan tool. It never executes
// a plan, changes its result, or treats a same-named arbitrary tool as compatible.
func (b *Bridge) translateNativePlan(native object) (object, error) {
	if text(native["type"]) != "function_call" || (text(native["name"]) != "update_plan" && text(native["name"]) != "functions.update_plan") {
		return nil, fmt.Errorf("basispoints native plan adapter requires an update_plan function call")
	}
	var selected tool
	matches := 0
	for _, key := range []string{"update_plan", "functions.update_plan"} {
		if candidate, ok := b.tools[key]; ok {
			selected = candidate
			matches++
		}
	}
	if matches != 1 || selected.Kind != "function" {
		return nil, fmt.Errorf("basispoints native update_plan requires one unambiguous client function declaration")
	}
	properties, _ := selected.Parameters["properties"].(object)
	if text(selected.Parameters["type"]) != "object" || properties["plan"] == nil {
		return nil, fmt.Errorf("basispoints native update_plan requires an explicit client plan argument schema")
	}
	arguments, ok := native["arguments"].(object)
	if !ok {
		if err := decode([]byte(text(native["arguments"])), &arguments); err != nil {
			return nil, fmt.Errorf("basispoints native update_plan arguments must be one JSON object")
		}
	}
	steps, ok := arguments["plan"].([]any)
	if !ok {
		return nil, fmt.Errorf("basispoints native update_plan requires a plan array")
	}
	plan := make([]any, 0, len(steps))
	for _, value := range steps {
		step, ok := value.(object)
		if !ok {
			return nil, fmt.Errorf("basispoints native update_plan entries must be objects")
		}
		description, present, err := planTextAlias(step, "step", "description", "title")
		if err != nil || !present || strings.TrimSpace(description) == "" {
			return nil, fmt.Errorf("basispoints native update_plan has an ambiguous or missing step description")
		}
		status := normalizeNativePlanStatus(text(step["status"]))
		if status == "" {
			return nil, fmt.Errorf("basispoints native update_plan has an unsupported step status")
		}
		plan = append(plan, object{"step": description, "status": status})
	}
	translated := object{"plan": plan}
	explanation, present, err := planTextAlias(arguments, "explanation", "summary")
	if err != nil {
		return nil, fmt.Errorf("basispoints native update_plan has an ambiguous explanation")
	}
	if present {
		translated["explanation"] = explanation
	}
	if !planSchemaAccepts(translated, selected.Parameters, 0) {
		return nil, fmt.Errorf("basispoints native update_plan does not satisfy the declared client argument schema")
	}
	callID := text(native["call_id"])
	if callID == "" || strings.TrimSpace(callID) != callID {
		return nil, fmt.Errorf("basispoints native update_plan is missing a valid call_id")
	}
	itemID := text(native["id"])
	if itemID == "" {
		itemID = "fc_" + fingerprint(callID)
	}
	encoded, _ := json.Marshal(translated)
	result := object{"type": "function_call", "id": itemID, "call_id": callID, "name": selected.Name, "arguments": string(encoded), "status": "completed"}
	if selected.Namespace != "" {
		result["namespace"] = selected.Namespace
	}
	b.rememberReplay(callID, native, result)
	return result, nil
}

func planTextAlias(value object, keys ...string) (string, bool, error) {
	var result string
	present := false
	for _, key := range keys {
		if raw, exists := value[key]; exists {
			candidate, ok := raw.(string)
			if !ok || (present && result != candidate) {
				return "", false, fmt.Errorf("invalid or conflicting plan text fields")
			}
			result, present = candidate, true
		}
	}
	return result, present, nil
}

func normalizeNativePlanStatus(status string) string {
	switch strings.NewReplacer("-", "_", " ", "_").Replace(strings.ToLower(strings.TrimSpace(status))) {
	case "pending", "not_started", "todo", "planned", "queued", "blocked":
		return "pending"
	case "in_progress", "active", "started", "doing", "current":
		return "in_progress"
	case "completed", "complete", "done", "finished":
		return "completed"
	default:
		return ""
	}
}

// This is deliberately a small, fail-closed validator for the standard Codex
// plan schema, not a general JSON Schema engine. Unknown assertions are rejected
// rather than silently treating their contracts as satisfied.
func planSchemaAccepts(value any, schema object, depth int) bool {
	if schema == nil || depth > 8 {
		return false
	}
	for key := range schema {
		switch key {
		case "type", "properties", "required", "additionalProperties", "items", "enum", "const", "minItems", "maxItems", "minLength", "maxLength":
		case "description", "title", "default", "examples", "$comment":
		default:
			return false
		}
	}
	if raw, exists := schema["properties"]; exists {
		if properties, ok := raw.(object); !ok || properties == nil {
			return false
		}
	}
	if kind, exists := schema["type"]; exists && !planSchemaTypeMatches(value, kind) {
		return false
	}
	if constant, exists := schema["const"]; exists && !reflect.DeepEqual(value, constant) {
		return false
	}
	if choices, exists := schema["enum"]; exists {
		values, ok := choices.([]any)
		if !ok || len(values) == 0 {
			return false
		}
		matched := false
		for _, choice := range values {
			matched = matched || reflect.DeepEqual(value, choice)
		}
		if !matched {
			return false
		}
	}
	switch typed := value.(type) {
	case object:
		properties, _ := schema["properties"].(object)
		if required, exists := schema["required"]; exists {
			names, ok := required.([]any)
			if !ok {
				return false
			}
			for _, name := range names {
				key, ok := name.(string)
				if _, exists := typed[key]; !ok || !exists {
					return false
				}
			}
		}
		additional, exists := schema["additionalProperties"]
		if exists {
			if _, ok := additional.(bool); !ok {
				return false
			}
		}
		for key, nested := range typed {
			if raw, exists := properties[key]; exists {
				nestedSchema, ok := raw.(object)
				if !ok || !planSchemaAccepts(nested, nestedSchema, depth+1) {
					return false
				}
			} else if additional == false {
				return false
			}
		}
	case []any:
		if !planSchemaLength(len(typed), schema, "minItems", "maxItems") {
			return false
		}
		if raw, exists := schema["items"]; exists {
			nested, ok := raw.(object)
			if !ok {
				return false
			}
			for _, item := range typed {
				if !planSchemaAccepts(item, nested, depth+1) {
					return false
				}
			}
		}
	case string:
		if !planSchemaLength(utf8.RuneCountInString(typed), schema, "minLength", "maxLength") {
			return false
		}
	}
	return true
}

func planSchemaTypeMatches(value, kind any) bool {
	if alternatives, ok := kind.([]any); ok {
		for _, candidate := range alternatives {
			if planSchemaTypeMatches(value, candidate) {
				return true
			}
		}
		return false
	}
	switch kind {
	case "object":
		_, ok := value.(object)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "null":
		return value == nil
	default:
		return false
	}
}

func planSchemaLength(length int, schema object, minimum, maximum string) bool {
	for _, key := range []string{minimum, maximum} {
		if value, exists := schema[key]; exists {
			var limit int64
			switch number := value.(type) {
			case json.Number:
				parsed, err := number.Int64()
				if err != nil {
					return false
				}
				limit = parsed
			case int:
				limit = int64(number)
			default:
				return false
			}
			if limit < 0 || (key == minimum && int64(length) < limit) || (key == maximum && int64(length) > limit) {
				return false
			}
		}
	}
	return true
}
