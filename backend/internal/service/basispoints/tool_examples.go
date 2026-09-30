package basispoints

import (
	"encoding/json"
	"sort"
	"strings"
)

// Build examples from this request's accepted catalog, not fixed tool identities.
// Inspired by cpa-plugin-oai-basispoints 280e28b; use this bridge's own transports.
func (b *Bridge) toolExamples() string {
	names := make([]string, 0, len(b.tools))
	for name := range b.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	var examples strings.Builder
	count := 0
	for _, name := range names {
		if count == 4 {
			break
		}
		info := b.tools[name]
		var field, payload string
		switch info.Name {
		case "apply_patch":
			field, payload = "patch", "*** Begin Patch\n*** Add File: example.txt\n+Example text.\n*** End Patch"
		case "exec":
			field, payload = "code", "text(\"Example text.\");"
		case "exec_command":
			field, payload = "cmd", "printf '%s\\n' \"Example text.\""
		default:
			continue
		}
		outer := object{"summary": "Call client tool " + name, "extended_summary": "Relay one declared client tool", "destructive": false, "references": []any{}}
		if info.Kind == "custom" {
			// We do not implement arbitrary client grammars. Avoid teaching a payload
			// when its declared grammar has not been checked by the bridge.
			if format := info.Catalog["format"]; format != nil {
				spec, ok := format.(object)
				if !ok || text(spec["type"]) != "text" {
					continue
				}
			}
			outer["summary"], outer["code"] = customTransportPrefix+name, payload
		} else {
			args := object{field: payload}
			if info.Schema == nil || info.Schema.Validate(args) != nil {
				continue
			}
			var err error
			switch {
			case supportsFunctionCodeTransport(name, info.Kind, info.Parameters) && field == "code":
				outer, err = encodeFunctionCodeTransport(name, args)
			case supportsFunctionCmdTransport(name, info.Kind, info.Parameters) && field == "cmd":
				outer, err = encodeFunctionCmdTransport(name, args)
			default:
				var encoded []byte
				encoded, err = json.Marshal(object{"name": name, "arguments": args})
				outer["code"] = string(encoded)
			}
			if err != nil {
				continue
			}
		}
		encoded, err := json.Marshal(outer)
		if err != nil {
			continue
		}
		_, _ = examples.WriteString("\nExample native run_officejs arguments: ")
		_, _ = examples.Write(encoded)
		count++
	}
	return examples.String()
}
