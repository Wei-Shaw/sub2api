package basispoints

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestRawCustomTransportPreservesExactCode(t *testing.T) {
	for _, name := range []string{"apply_patch", "functions.apply_patch", "plugin_namespace.custom-tool"} {
		for _, input := range []string{
			"", "  exact input  \n", "*** Begin Patch\r\n+\tprint(\"a \\\"nested\\\" quote\")\r\n*** End Patch\r\n",
			`{"name":"another_tool","arguments":{"cmd":"do not decode this as a call"}}`,
			`const task = {"name":"shell"};`,
			"```json\n{\"tool\":\"different\"}\n```",
			`C:\Projects\path\d+\s and literal \n`,
		} {
			envelope, matched, err := customTransportEnvelope(object{"summary": customTransportPrefix + name, "code": input})
			if err != nil || !matched || !reflect.DeepEqual(envelope, object{"name": name, "input": input}) {
				t.Fatalf("raw custom input was interpreted or modified: matched=%t error=%v", matched, err)
			}
		}
	}
}

func TestRawCustomTransportRequiresExactMarker(t *testing.T) {
	for _, summary := range []any{
		nil, 1, object{"marker": customTransportPrefix + "apply_patch"},
		"Apply a patch", "codex2api.custom", "codex2api.custom:apply_patch",
		"Codex2api.custom/apply_patch", " codex2api.custom/apply_patch",
		"Use codex2api.custom/apply_patch", "\ncodex2api.custom/apply_patch",
	} {
		if envelope, matched, err := customTransportEnvelope(object{"summary": summary, "code": "raw input"}); matched || err != nil || envelope != nil {
			t.Fatal("ordinary or approximate summary must not activate raw transport")
		}
	}
}

func TestRawCustomTransportRejectsMalformedExplicitMarkers(t *testing.T) {
	for _, name := range []string{
		"", " apply_patch", "apply_patch ", "apply patch", "apply_patch\n", "apply_patch\r\nextra",
		"apply_patch\t", "apply_patch\x00", "namespace/apply_patch", `namespace\apply_patch`,
		"apply_patch\u2003suffix",
	} {
		envelope, matched, err := customTransportEnvelope(object{"summary": customTransportPrefix + name, "code": "private-code-never-in-errors"})
		if !matched || err == nil || envelope != nil || strings.Contains(err.Error(), "private-code-never-in-errors") {
			t.Fatal("malformed explicit marker must fail closed without returning payload text")
		}
	}
}

func TestRawCustomTransportValidatesCodeTypeAndSize(t *testing.T) {
	for _, code := range []any{nil, 1, object{"input": "private"}, []any{"private"}, strings.Repeat("x", maxEnvelopeBytes+1)} {
		if envelope, matched, err := customTransportEnvelope(object{"summary": customTransportPrefix + "apply_patch", "code": code}); !matched || err == nil || envelope != nil {
			t.Fatal("raw transport must accept only bounded string input")
		}
	}
	input := strings.Repeat("x", maxEnvelopeBytes)
	if envelope, matched, err := customTransportEnvelope(object{"summary": customTransportPrefix + "apply_patch", "code": input}); err != nil || !matched || envelope["input"] != input {
		t.Fatal("the exact byte limit must remain valid")
	}
}

func TestRawCustomTransportLargePatchNeedsOnlyOuterJSON(t *testing.T) {
	var patch strings.Builder
	_, _ = patch.WriteString("*** Begin Patch\r\n*** Update File: src/example.ts\r\n@@\r\n")
	for i := 0; i < 125; i++ {
		fmt.Fprintf(&patch, "+\tconst item%d = \"中文 \\\"nested\\\" C:\\\\Projects\\\\file\"; // literal \\n and \\d+\r\n", i)
	}
	_, _ = patch.WriteString("*** End Patch\r\n")
	input := patch.String()
	if len(input) < 9000 || len(input) > 13000 {
		t.Fatalf("expected a roughly 10 KB patch, got %d bytes", len(input))
	}
	outer := object{"summary": customTransportPrefix + "functions.apply_patch", "code": input, "destructive": false, "references": []any{}}
	encoded, err := json.Marshal(outer)
	if err != nil {
		t.Fatal(err)
	}
	var wireArguments object
	if err := decode(encoded, &wireArguments); err != nil {
		t.Fatal(err)
	}
	envelope, matched, err := customTransportEnvelope(wireArguments)
	if err != nil || !matched || envelope["name"] != "functions.apply_patch" || envelope["input"] != input {
		t.Fatal("single JSON layer did not preserve the complete patch")
	}
	if !reflect.DeepEqual(wireArguments, outer) {
		t.Fatal("raw transport parsing must not mutate the native arguments")
	}
}
