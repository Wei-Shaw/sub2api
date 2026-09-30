package basispoints

import (
	"encoding/json"
	"strings"
	"testing"
)

func duplicateExecTool(description string) object {
	return object{
		"type": "custom", "name": "exec", "description": description,
		"format": object{"type": "grammar", "syntax": "lark", "definition": "start: /[\\s\\S]+/"},
	}
}

func execToolNamespace(declaration object) object {
	return object{"type": "namespace", "name": "functions", "tools": []any{declaration}}
}

func TestDuplicateToolDescriptionsKeepCurrentContract(t *testing.T) {
	for _, cached := range []bool{false, true} {
		name := "explicit catalog"
		if cached {
			name = "inherited catalog"
		}
		t.Run(name, func(t *testing.T) {
			cache := new(CatalogCache)
			source := testSource()
			source["tools"] = []any{execToolNamespace(duplicateExecTool("Current execution instructions"))}
			if cached {
				prepareCatalogTest(t, cache, source, "account/key/thread")
				delete(source, "tools")
			}
			historical := duplicateExecTool("Older client execution instructions")
			historical["defer_loading"] = true
			source["input"] = []any{
				object{"type": "additional_tools", "tools": []any{execToolNamespace(historical)}},
				message("user", "Continue"),
			}
			raw, _ := json.Marshal(source)
			before := string(raw)
			wire, bridge, err := PrepareWithCatalog(raw, "account/key/thread", nil, cache)
			if err != nil {
				t.Fatal(err)
			}
			if string(raw) != before || len(bridge.tools) != 1 {
				t.Fatal("duplicate handling changed the request or retained duplicate tools")
			}
			if got := bridge.tools["functions.exec"]; got.Kind != "custom" || text(got.Catalog["description"]) != "Current execution instructions" {
				t.Fatal("historical annotations replaced the current tool contract")
			}
			var prepared object
			if err := json.Unmarshal(wire, &prepared); err != nil {
				t.Fatal(err)
			}
			input := mustTestValue[[]any](t, prepared["input"])
			protocol := text(mustTestValue[object](t, mustTestValue[[]any](t, mustTestValue[object](t, input[1])["content"])[0])["text"])
			if strings.Count(protocol, `Client tool "functions.exec"`) != 1 || strings.Contains(protocol, "Older client execution instructions") {
				t.Fatal("upstream catalog did not retain one current declaration")
			}
			source["input"] = "Continue"
			delete(source, "tools")
			if got := prepareCatalogTest(t, cache, source, "account/key/thread").tools["functions.exec"]; text(got.Catalog["description"]) != "Current execution instructions" {
				t.Fatal("compatible duplicate corrupted the cached catalog")
			}
		})
	}
}

func TestDuplicateFunctionToolsNormalizeSchemaAliases(t *testing.T) {
	schema := object{"type": "object", "properties": object{"cmd": object{"type": "string"}}, "required": []any{"cmd"}, "additionalProperties": false}
	for _, alias := range []string{"parameters", "inputSchema", "input_schema"} {
		t.Run(alias, func(t *testing.T) {
			source := testSource()
			source["tools"] = []any{
				object{"type": "function", "name": "shell", "description": "Current description", "parameters": schema, "strict": true},
				object{"type": "function", "name": "shell", "description": "Previous description", alias: schema, "strict": true},
			}
			_, bridge := mustPrepare(t, source, "scope", nil)
			if len(bridge.tools) != 1 || bridge.tools["shell"].Schema == nil {
				t.Fatal("function schema or deduplication was lost")
			}
			if err := bridge.translateResponse(object{"output": []any{nativeCall(object{"name": "shell", "arguments": object{"cmd": 3}})}}); err == nil {
				t.Fatal("compatible duplicates bypassed argument validation")
			}
		})
	}
}

func TestDuplicateToolContractConflictsStillFailAtomically(t *testing.T) {
	for _, field := range []string{"type", "format", "strict", "parameters", "encrypted"} {
		t.Run(field, func(t *testing.T) {
			cache := new(CatalogCache)
			source := testSource()
			source["tools"] = []any{execToolNamespace(duplicateExecTool("Current"))}
			prepareCatalogTest(t, cache, source, "scope")
			before, _ := cache.snapshot("scope")
			conflicting := duplicateExecTool("Historical")
			switch field {
			case "type":
				conflicting[field] = "function"
			case "format":
				conflicting[field] = object{"type": "text"}
			case "parameters":
				conflicting[field] = object{"type": "object"}
			default:
				conflicting[field] = true
			}
			delete(source, "tools")
			source["input"] = []any{object{"type": "additional_tools", "tools": []any{execToolNamespace(conflicting)}}, message("user", "Continue")}
			raw, _ := json.Marshal(source)
			_, _, err := PrepareWithCatalog(raw, "scope", nil, cache)
			if err == nil || !strings.Contains(err.Error(), "conflicting duplicate") {
				t.Fatalf("accepted a changed %s contract: %v", field, err)
			}
			after, _ := cache.snapshot("scope")
			if string(after) != string(before) {
				t.Fatal("rejected conflict mutated the catalog")
			}
		})
	}
}
