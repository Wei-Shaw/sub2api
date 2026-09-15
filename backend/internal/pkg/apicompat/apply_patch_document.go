package apicompat

import (
	"encoding/json"
	"strings"
)

const applyPatchToolName = "apply_patch"

const applyPatchToolInputSchema = `{"type":"object","properties":{"input":{"type":"string","description":"The entire apply_patch document. First line must be exactly *** Begin Patch. Last line must be exactly *** End Patch. Do not add extra asterisks to those markers."}},"required":["input"]}`

const applyPatchToolDescription = "Use the apply_patch tool to edit files. Pass the full patch text in input. The first line must be exactly *** Begin Patch. The last line must be exactly *** End Patch."

var customToolInputKeys = []string{"input", "patch", "command", "content"}

func looksLikeApplyPatch(raw string) bool {
	return strings.Contains(raw, "*** Begin Patch") || strings.Contains(raw, "*** End Patch")
}

func stripXMLInvoke(raw string) string {
	trimmed := strings.TrimSpace(raw)
	rest, ok := strings.CutPrefix(trimmed, "<invoke")
	if !ok {
		return raw
	}
	gt := strings.IndexByte(rest, '>')
	if gt < 0 {
		return raw
	}
	inner := rest[gt+1:]
	if end := strings.LastIndex(inner, "</invoke>"); end >= 0 {
		return strings.TrimSpace(inner[:end])
	}
	return strings.TrimSpace(inner)
}

// normalizeApplyPatchDocument rewrites Grok-style patch envelopes so Codex can
// parse them. Codex requires the first line to equal "*** Begin Patch" and the
// last non-empty line to equal "*** End Patch".
func normalizeApplyPatchDocument(raw string) string {
	stripped := stripXMLInvoke(raw)
	var b strings.Builder
	sawEnd := false
	for _, line := range strings.Split(stripped, "\n") {
		trimmed := strings.TrimSpace(line)
		if sawEnd {
			continue
		}
		if trimmed == "*** End of File ***" {
			continue
		}
		if strings.HasPrefix(trimmed, "*** Begin Patch") {
			_, _ = b.WriteString("*** Begin Patch\n")
			continue
		}
		if strings.HasPrefix(trimmed, "*** End Patch") {
			_, _ = b.WriteString("*** End Patch\n")
			sawEnd = true
			continue
		}
		_, _ = b.WriteString(line)
		_ = b.WriteByte('\n')
	}
	out := b.String()
	if !strings.Contains(out, "*** End Patch") {
		return strings.TrimSuffix(out, "\n")
	}
	return out
}

func maybeNormalizeApplyPatchDocument(raw string) string {
	stripped := stripXMLInvoke(raw)
	if !looksLikeApplyPatch(stripped) {
		return raw
	}
	return normalizeApplyPatchDocument(stripped)
}

func unwrapCustomToolInput(arguments string) string {
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" {
		return ""
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
		return trimmed
	}
	for _, key := range customToolInputKeys {
		raw, ok := obj[key]
		if !ok {
			continue
		}
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			continue
		}
		if key == "input" || looksLikeApplyPatch(s) {
			return s
		}
	}
	if len(obj) == 0 {
		return ""
	}
	return trimmed
}

func applyPatchFunctionParameters() json.RawMessage {
	return json.RawMessage(applyPatchToolInputSchema)
}

func customToolFunctionParameters(name string) json.RawMessage {
	if name == applyPatchToolName {
		return applyPatchFunctionParameters()
	}
	return json.RawMessage(customToolInputSchema)
}

func applyPatchCallInput(item map[string]any) string {
	if text := strings.TrimSpace(stringValue(item["input"])); text != "" {
		return text
	}
	operation, ok := item["operation"].(map[string]any)
	if !ok {
		return ""
	}
	return applyPatchOperationDocument(operation)
}

func applyPatchOperationDocument(operation map[string]any) string {
	typ := strings.TrimSpace(stringValue(operation["type"]))
	path := stringValue(operation["path"])
	var b strings.Builder
	_, _ = b.WriteString("*** Begin Patch\n")
	switch typ {
	case "add_file", "create_file":
		_, _ = b.WriteString("*** Add File: ")
		_, _ = b.WriteString(path)
		_ = b.WriteByte('\n')
		contents := stringValue(operation["contents"])
		if contents == "" {
			contents = stringValue(operation["diff"])
		}
		for _, line := range strings.Split(contents, "\n") {
			if line == "" {
				continue
			}
			if !strings.HasPrefix(line, "+") {
				_ = b.WriteByte('+')
			}
			_, _ = b.WriteString(line)
			_ = b.WriteByte('\n')
		}
	case "delete_file":
		_, _ = b.WriteString("*** Delete File: ")
		_, _ = b.WriteString(path)
		_ = b.WriteByte('\n')
	default:
		_, _ = b.WriteString("*** Update File: ")
		_, _ = b.WriteString(path)
		_ = b.WriteByte('\n')
		diff := stringValue(operation["diff"])
		if diff != "" {
			_, _ = b.WriteString(diff)
			if !strings.HasSuffix(diff, "\n") {
				_ = b.WriteByte('\n')
			}
		}
	}
	_, _ = b.WriteString("*** End Patch\n")
	return b.String()
}

func lowerApplyPatchTool(tool map[string]any) map[string]any {
	copy := copyClientTool(tool)
	copy["type"] = "function"
	copy["name"] = applyPatchToolName
	copy["parameters"] = applyPatchFunctionParameters()
	if strings.TrimSpace(stringValue(copy["description"])) == "" {
		copy["description"] = applyPatchToolDescription
	}
	delete(copy, "format")
	return copy
}
