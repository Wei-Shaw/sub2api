package basispoints

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestContentErrorsIdentifyPathAndKnownType(t *testing.T) {
	for _, field := range []string{"content", "function_call_output", "custom_tool_call_output"} {
		for _, kind := range []string{"input_file", "input_audio", "image_url", "image", "encrypted_content"} {
			t.Run(field+"/"+kind, func(t *testing.T) {
				source := testSource()
				parts := []any{object{"type": "input_text", "text": "private-text"}, object{"type": kind, "data": "private-payload", "url": "https://private-url.example/image"}}
				input := []any{message("user", "inspect")}
				path := "input[1].content[1]"
				if field != "content" {
					call := object{"type": "function_call", "call_id": "call_diagnostic", "name": "inspect", "arguments": `{}`}
					if field == "custom_tool_call_output" {
						call = object{"type": "custom_tool_call", "call_id": "call_diagnostic", "name": "inspect", "input": "diagnostic"}
					}
					input = append(input, call, object{"type": field, "call_id": "call_diagnostic", "output": parts})
					path = "input[2].output[1]"
				} else {
					input = append(input, object{"role": "user", "content": parts})
				}
				source["input"] = input
				raw, err := json.Marshal(source)
				if err != nil {
					t.Fatal(err)
				}
				_, _, err = Prepare(raw, "scope", nil)
				if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "type="+kind) || strings.Contains(err.Error(), "private-") {
					t.Fatalf("content error lacks safe structural diagnostics: %v", err)
				}
			})
		}
	}
}

func TestContentDiagnosticsDoNotEchoUntrustedTypeValues(t *testing.T) {
	for _, tc := range []struct {
		part any
		kind string
	}{
		{nil, "non_object"},
		{"private-payload", "non_object"},
		{object{}, "missing"},
		{object{"type": "private-type"}, "unknown"},
		{object{"type": "input_file\nprivate-log-injection"}, "unknown"},
		{object{"type": object{"private-key": "private-value"}}, "non_string"},
	} {
		source := testSource()
		source["input"] = []any{object{"role": "user", "content": []any{tc.part}}}
		raw, err := json.Marshal(source)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = Prepare(raw, "scope", nil)
		if err == nil || !strings.Contains(err.Error(), "input[0].content[0]") || !strings.Contains(err.Error(), "type="+tc.kind) || strings.Contains(err.Error(), "private-") {
			t.Fatalf("missing path or untrusted data echoed: %v", err)
		}
	}
}

func TestImageValidationErrorIncludesPathWithoutURL(t *testing.T) {
	source := testSource()
	source["input"] = []any{message("user", "diagnostic"), object{"role": "user", "content": []any{object{"type": "input_image", "image_url": "http://private-url.example/image?private-token"}}}}
	raw, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = Prepare(raw, "scope", nil)
	if err == nil || !strings.Contains(err.Error(), "path=input[1].content[0]") || !strings.Contains(err.Error(), "HTTPS") || strings.Contains(err.Error(), "private-") {
		t.Fatalf("image error lacks a safe path: %v", err)
	}
}
