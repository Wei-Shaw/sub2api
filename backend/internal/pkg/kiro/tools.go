package kiro

import "strings"

// MaxToolDescLen is the upper bound the Kiro upstream enforces on a
// tool's description field. Longer descriptions are truncated.
const MaxToolDescLen = 10237

// MaxToolNameLen is the upper bound on a sanitized tool name.
const MaxToolNameLen = 64

// SanitizeToolName converts a tool name into the pure-camelCase form
// Kiro requires. Separators "_" and "-" become camelCase boundaries
// (e.g. "list_files" → "listFiles", "get-weather" → "getWeather"). Empty
// input falls back to "tool".
//
// Ported verbatim from Kiro-Go/proxy/translator.go:sanitizeToolName so
// the resulting names match exactly across implementations.
func SanitizeToolName(name string) string {
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == '-'
	})
	if len(parts) == 0 {
		return "tool"
	}
	var b strings.Builder
	for i, part := range parts {
		if part == "" {
			continue
		}
		if i == 0 {
			b.WriteString(strings.ToLower(part[:1]) + part[1:])
		} else {
			b.WriteString(strings.ToUpper(part[:1]) + part[1:])
		}
	}
	result := b.String()
	if result == "" {
		return "tool"
	}
	return result
}

// ShortenToolName clips a tool name to MaxToolNameLen. MCP tools
// (mcp__server__tool) are shortened by dropping the server namespace
// segment first before falling back to a hard truncation.
func ShortenToolName(name string) string {
	if len(name) <= MaxToolNameLen {
		return name
	}
	if strings.HasPrefix(name, "mcp__") {
		lastIdx := strings.LastIndex(name, "__")
		if lastIdx > 5 {
			shortened := "mcp__" + name[lastIdx+2:]
			if len(shortened) <= MaxToolNameLen {
				return shortened
			}
		}
	}
	return name[:MaxToolNameLen]
}

// EnsureObjectSchema guarantees the JSON-Schema-like value has
// `"type": "object"` at the top level. Used for tool input schemas
// since Kiro requires the wrapping object type.
//
// It also recursively scrubs invalid `"required": null` fields and
// empty `"required": []` arrays, which Kiro rejects.
func EnsureObjectSchema(schema any) any {
	m, ok := schema.(map[string]any)
	if !ok {
		return map[string]any{"type": "object"}
	}
	cleanSchema(m)
	if _, hasType := m["type"]; !hasType {
		m["type"] = "object"
	}
	return m
}

// cleanSchema recursively normalises a JSON-Schema-like map.
func cleanSchema(m map[string]any) {
	// "required" must be a non-empty string array, or absent.
	if req, exists := m["required"]; exists {
		switch v := req.(type) {
		case nil:
			delete(m, "required")
		case []any:
			if len(v) == 0 {
				delete(m, "required")
			}
		}
	}
	if props, ok := m["properties"].(map[string]any); ok {
		for _, v := range props {
			if sub, ok := v.(map[string]any); ok {
				cleanSchema(sub)
			}
		}
	}
	if items, ok := m["items"].(map[string]any); ok {
		cleanSchema(items)
	}
	if sub, ok := m["additionalProperties"].(map[string]any); ok {
		cleanSchema(sub)
	}
	for _, key := range []string{"allOf", "oneOf", "anyOf"} {
		if arr, ok := m[key].([]any); ok {
			for _, item := range arr {
				if sub, ok := item.(map[string]any); ok {
					cleanSchema(sub)
				}
			}
		}
	}
}

// TruncateToolDescription clips a tool description to MaxToolDescLen.
// Returns the original string when already short enough.
func TruncateToolDescription(desc string) string {
	if len(desc) <= MaxToolDescLen {
		return desc
	}
	return desc[:MaxToolDescLen] + "..."
}
