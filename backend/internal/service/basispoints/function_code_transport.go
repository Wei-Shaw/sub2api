package basispoints

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

const functionCodeTransportPrefix = "codex2api.function_code/"

// Only explicit string code parameters use the raw-code transport. Other
// function arguments keep their existing JSON envelope representation.
func supportsFunctionCodeTransport(name, kind string, parameters any) bool {
	if kind != "function" || name == "" || strings.ContainsAny(name, "/\\") || strings.IndexFunc(name, unicode.IsSpace) >= 0 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return false
	}
	schema, _ := parameters.(object)
	properties, _ := schema["properties"].(object)
	code, _ := properties["code"].(object)
	return schema["type"] == "object" && code["type"] == "string"
}

// Keep executable text in the native code string. extended_summary carries only
// the other JSON arguments; the server serializes the final client arguments.
// No source text is parsed, repaired, evaluated or treated as another tool call.
func (b *Bridge) functionCodeTransportEnvelope(arguments object) (object, bool, error) {
	summary, ok := arguments["summary"].(string)
	if !ok || !strings.HasPrefix(summary, functionCodeTransportPrefix) {
		return nil, false, nil
	}
	name := strings.TrimPrefix(summary, functionCodeTransportPrefix)
	info, allowed := b.tools[name]
	if !allowed || !supportsFunctionCodeTransport(name, info.Kind, info.Parameters) {
		return nil, true, fmt.Errorf("basispoints function code transport requires an exact catalog function with a string code parameter")
	}
	code, codeOK := arguments["code"].(string)
	metadata, metadataOK := arguments["extended_summary"].(string)
	if !codeOK || !metadataOK {
		return nil, true, fmt.Errorf("basispoints function code transport requires string code and JSON arguments in extended_summary")
	}
	if len(code) > maxEnvelopeBytes || len(metadata) > maxEnvelopeBytes-len(code) {
		return nil, true, fmt.Errorf("basispoints function code transport exceeds the size limit")
	}
	var args object
	if decode([]byte(metadata), &args) != nil || args == nil {
		return nil, true, fmt.Errorf("basispoints function code transport extended_summary must contain one JSON object")
	}
	if _, exists := args["code"]; exists {
		return nil, true, fmt.Errorf("basispoints function code transport must not duplicate code in extended_summary")
	}
	args["code"] = code
	encoded, err := json.Marshal(args)
	if err != nil || len(encoded) > maxEnvelopeBytes {
		return nil, true, fmt.Errorf("basispoints function code transport arguments exceed the size limit")
	}
	return object{"name": name, "arguments": args}, true, nil
}

func encodeFunctionCodeTransport(name string, args object) (object, error) {
	metadata := make(object, len(args))
	for key, value := range args {
		if key != "code" {
			metadata[key] = value
		}
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("basispoints history function metadata cannot be serialized")
	}
	return object{
		"summary": functionCodeTransportPrefix + name, "code": args["code"],
		"extended_summary": string(encoded), "destructive": false, "references": []any{},
	}, nil
}
