package basispoints

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// describeCatalog presents tool contracts as documentation, not native tool definitions.
func describeCatalog(catalog []any) string {
	var lines []string
	for _, raw := range catalog {
		// collectTools constructs every catalog entry as an object.
		entry, _ := raw.(object)
		line := "Client tool " + quoted(entry["name"]) + " (" + text(entry["type"]) + ")."
		if description := text(entry["description"]); description != "" {
			line += " " + description
		}
		if text(entry["type"]) == "custom" {
			line += " Set run_officejs summary to " + quoted("codex2api.custom/"+text(entry["name"])) + " and pass its exact raw text directly in code."
			if format := entry["format"]; format != nil {
				line += " Input format: " + quoted(format) + "."
			}
		} else {
			if supportsFunctionCodeTransport(text(entry["name"]), text(entry["type"]), entry["parameters"]) {
				line += " Use FUNCTION_CODE transport: set run_officejs summary to " + quoted(functionCodeTransportPrefix+text(entry["name"])) + ". Put the exact code argument directly in native code. Put all other supplied arguments in one JSON object in extended_summary, using only fields declared in the contract; use {} when there are none. Do not include code in that object."
			} else if supportsFunctionCmdTransport(text(entry["name"]), text(entry["type"]), entry["parameters"]) {
				line += " Use FUNCTION_CMD transport: set run_officejs summary to " + quoted(functionCmdTransportPrefix+text(entry["name"])) + ". Put the exact cmd argument directly in native code. Put all other supplied arguments in one JSON object in extended_summary, using only fields declared in the contract; use {} when there are none. Do not include cmd in that object."
			} else {
				line += " Pass a JSON object in the envelope's arguments field."
			}
			line += " Argument contract: " + describeSchema(entry["parameters"], 0)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n\n")
}

func quoted(value any) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

// Preserve constraints that are not expanded into prose, including schema references.
func describeSchema(value any, depth int) string {
	schema, ok := value.(object)
	if !ok || depth >= 8 {
		if value == nil {
			return "Use the arguments described by the tool."
		}
		return quoted(value)
	}
	var parts []string
	if kind := schema["type"]; kind != nil {
		parts = append(parts, "Value type: "+quoted(kind)+".")
	}
	if description := text(schema["description"]); description != "" {
		parts = append(parts, description)
	}
	required := make(map[string]bool)
	if names, ok := schema["required"].([]any); ok {
		for _, name := range names {
			required[text(name)] = true
		}
	}
	if properties, ok := schema["properties"].(object); ok {
		names := make([]string, 0, len(properties))
		for name := range properties {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			presence := "optional"
			if required[name] {
				presence = "required"
			}
			parts = append(parts, fmt.Sprintf("Field %s (%s): %s", quoted(name), presence, describeSchema(properties[name], depth+1)))
		}
	}
	if items := schema["items"]; items != nil {
		parts = append(parts, "Each array item: "+describeSchema(items, depth+1))
	}
	constraints := make(object)
	for key, value := range schema {
		switch key {
		case "type", "description", "properties", "items":
		default:
			constraints[key] = value
		}
	}
	if len(constraints) > 0 {
		parts = append(parts, "Additional constraints: "+quoted(constraints)+".")
	}
	if len(parts) == 0 {
		return "Any JSON value."
	}
	return strings.Join(parts, " ")
}
