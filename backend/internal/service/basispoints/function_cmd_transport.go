package basispoints

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

const functionCmdTransportPrefix = "codex2api.function_cmd/"

// Only catalog exec_command functions with explicit string cmd use this transport. Other
// function arguments keep their existing JSON envelope representation.
func supportsFunctionCmdTransport(name, kind string, parameters any) bool {
	if name != "exec_command" && !strings.HasSuffix(name, ".exec_command") {
		return false
	}
	if supportsFunctionCodeTransport(name, kind, parameters) {
		return false
	}
	if kind != "function" || name == "" || strings.ContainsAny(name, "/\\") || strings.IndexFunc(name, unicode.IsSpace) >= 0 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return false
	}
	schema, _ := parameters.(object)
	properties, _ := schema["properties"].(object)
	code, _ := properties["cmd"].(object)
	return schema["type"] == "object" && code["type"] == "string"
}

// Keep executable text in the native code string. extended_summary carries only
// the other JSON arguments; the server serializes the final client arguments.
// No source text is parsed, repaired, evaluated or treated as another tool call.
func (b *Bridge) functionCmdTransportEnvelope(arguments object) (object, bool, error) {
	summary, ok := arguments["summary"].(string)
	if !ok || !strings.HasPrefix(summary, functionCmdTransportPrefix) {
		return nil, false, nil
	}
	name := strings.TrimPrefix(summary, functionCmdTransportPrefix)
	info, allowed := b.tools[name]
	if !allowed || !supportsFunctionCmdTransport(name, info.Kind, info.Parameters) {
		return nil, true, fmt.Errorf("basispoints function cmd transport requires an exact catalog function with a string cmd parameter")
	}
	code, codeOK := arguments["code"].(string)
	metadata, metadataOK := arguments["extended_summary"].(string)
	if !codeOK || !metadataOK {
		return nil, true, fmt.Errorf("basispoints function cmd transport requires string code and JSON arguments in extended_summary")
	}
	if len(code) > maxEnvelopeBytes || len(metadata) > maxEnvelopeBytes-len(code) {
		return nil, true, fmt.Errorf("basispoints function cmd transport exceeds the size limit")
	}
	var args object
	if decode([]byte(metadata), &args) != nil || args == nil {
		return nil, true, fmt.Errorf("basispoints function cmd transport extended_summary must contain one JSON object")
	}
	if duplicate, exists := args["cmd"]; exists {
		duplicateText, ok := duplicate.(string)
		if !ok || duplicateText != code {
			return nil, true, fmt.Errorf("basispoints function cmd transport has conflicting duplicate cmd in extended_summary")
		}
		// Some upstreams redundantly repeat the exact same cmd in both fields.
		// Dropping only an identical duplicate is safe; conflicting values remain rejected.
		delete(args, "cmd")
	}
	args["cmd"] = code
	encoded, err := json.Marshal(args)
	if err != nil || len(encoded) > maxEnvelopeBytes {
		return nil, true, fmt.Errorf("basispoints function cmd transport arguments exceed the size limit")
	}
	// The client is authoritative for the full tool schema. The relay only
	// validates transport invariants above; rejecting here turns harmless extra
	// or forward-compatible fields into a stream-level 502.
	return object{"name": name, "arguments": args}, true, nil
}

func encodeFunctionCmdTransport(name string, args object) (object, error) {
	metadata := make(object, len(args))
	for key, value := range args {
		if key != "cmd" {
			metadata[key] = value
		}
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("basispoints history function metadata cannot be serialized")
	}
	return object{
		"summary": functionCmdTransportPrefix + name, "code": args["cmd"],
		"extended_summary": string(encoded), "destructive": false, "references": []any{},
	}, nil
}
