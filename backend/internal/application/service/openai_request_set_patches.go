package service

import (
	"bytes"
	"sort"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const openAIRequestPatchBatchMinBytes = 64 << 10

type openAIRequestPatchedField struct {
	name        string
	original    gjson.Result
	replacement string
}

// Apply scalar/object sets to a small object containing only the affected
// roots, then splice those roots into the original request in one allocation.
// sjson still owns nested-path semantics and patch order. Deletions, duplicate
// roots, special paths and large modified subtrees use the existing slow path.
// As with sjson, callers supply a well-formed JSON request.
func applyOpenAIRequestSetPatches(body []byte, patches []openAIRequestPatch) ([]byte, bool, error) {
	if len(body) < openAIRequestPatchBatchMinBytes || len(patches) < 2 {
		return nil, false, nil
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) < 2 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
		return nil, false, nil
	}
	closing := len(bytes.TrimRight(body, " \t\r\n")) - 1
	fields := make([]openAIRequestPatchedField, 0, len(patches))
	byName := make(map[string]int, len(patches))
	for _, patch := range patches {
		if patch.delete || !isOpenAIRequestLiteralPatchPath(patch.path) {
			return nil, false, nil
		}
		name, _, _ := strings.Cut(patch.path, ".")
		if _, exists := byName[name]; !exists {
			byName[name] = len(fields)
			fields = append(fields, openAIRequestPatchedField{name: name})
		}
	}
	usable, hasMembers := true, false
	parseRawJSONView(body).ForEach(func(key, value gjson.Result) bool {
		hasMembers = true
		if i, exists := byName[key.Str]; exists {
			if fields[i].original.Exists() || len(value.Raw) > openAIRequestPatchBatchMinBytes ||
				value.Index <= 0 || value.Index+len(value.Raw) > closing {
				usable = false
				return false
			}
			fields[i].original = value
		}
		return true
	})
	if !usable {
		return nil, false, nil
	}

	shadow := []byte{'{'}
	for _, field := range fields {
		if !field.original.Exists() {
			continue
		}
		if len(shadow) > 1 {
			shadow = append(shadow, ',')
		}
		shadow = append(shadow, '"')
		shadow = append(shadow, field.name...)
		shadow = append(shadow, '"', ':')
		shadow = append(shadow, field.original.Raw...)
	}
	shadow = append(shadow, '}')
	for _, patch := range patches {
		var err error
		shadow, err = sjson.SetBytes(shadow, patch.path, patch.value)
		if err != nil {
			return nil, true, err
		}
	}

	var additions []byte
	edits := fields[:0]
	outputLen := len(body)
	for _, field := range fields {
		field.replacement = gjson.GetBytes(shadow, field.name).Raw
		if field.original.Exists() {
			if field.original.Raw != field.replacement {
				outputLen += len(field.replacement) - len(field.original.Raw)
				edits = append(edits, field)
			}
			continue
		}
		if hasMembers {
			additions = append(additions, ',')
		}
		hasMembers = true
		additions = append(additions, '"')
		additions = append(additions, field.name...)
		additions = append(additions, '"', ':')
		additions = append(additions, field.replacement...)
	}
	if len(edits) == 0 && len(additions) == 0 {
		return body, true, nil
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].original.Index < edits[j].original.Index })
	out := make([]byte, 0, outputLen+len(additions))
	pos := 0
	for _, edit := range edits {
		out = append(out, body[pos:edit.original.Index]...)
		out = append(out, edit.replacement...)
		pos = edit.original.Index + len(edit.original.Raw)
	}
	out = append(out, body[pos:closing]...)
	out = append(out, additions...)
	out = append(out, body[closing:]...)
	return out, true, nil
}

func isOpenAIRequestLiteralPatchPath(path string) bool {
	for _, part := range strings.Split(path, ".") {
		if part == "" {
			return false
		}
		for i := range len(part) {
			c := part[i]
			if c != '_' && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (i == 0 || c < '0' || c > '9') {
				return false
			}
		}
	}
	return true
}
