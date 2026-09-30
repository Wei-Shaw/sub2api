package basispoints

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestTransportJSONFailureDiagnosticsAreStructural(t *testing.T) {
	const private = "private-code-must-not-appear"
	for _, tc := range []struct {
		name, raw, kind string
	}{
		{"newline", "{\"input\":\"" + private + "\nnext\"}", "raw_control"},
		{"tab", "{\"input\":\"" + private + "\tnext\"}", "raw_control"},
		{"return", "{\"input\":\"" + private + "\rnext\"}", "raw_control"},
		{"nul", "{\"input\":\"" + private + "\x00next\"}", "raw_control"},
		{"escape", `{"input":"` + private + `\q"}`, "invalid_escape"},
		{"unicode escape", `{"input":"` + private + `\uZZZZ"}`, "invalid_escape"},
		{"trailing object", `{"input":"` + private + `"} {}`, "trailing_data"},
		{"trailing text", `{"input":"` + private + `"} private`, "trailing_data"},
		{"comma", `{"input":"` + private + `" "name":"patch"}`, "missing_separator"},
		{"colon", `{"input" "` + private + `"}`, "missing_separator"},
		{"array separator", `["` + private + `" "other"]`, "missing_separator"},
		{"truncated value", `{"input":"` + private + `"`, "unexpected_eof"},
		{"truncated string", `{"input":"` + private, "unexpected_eof"},
		{"unquoted key", `{private-code-must-not-appear:1}`, "unexpected_token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shape := transportShape(tc.raw)
			if !strings.Contains(shape, "json_failure="+tc.kind) || !strings.Contains(shape, "json_offset=") || strings.Contains(shape, private) {
				t.Fatalf("unexpected or unsafe structural diagnostic: %s", shape)
			}
			var value any
			err := json.Unmarshal([]byte(tc.raw), &value)
			syntax, ok := err.(*json.SyntaxError)
			if !ok || !strings.Contains(shape, fmt.Sprintf("json_offset=%d;", syntax.Offset)) {
				t.Fatalf("diagnostic must retain the original byte offset: %s", shape)
			}
		})
	}
}

func TestTransportJSONRepairsRawStringControlsWithoutChangingTheirValues(t *testing.T) {
	for _, control := range []string{"\n", "\r", "\t", "\r\n"} {
		t.Run(fmt.Sprintf("%q", control), func(t *testing.T) {
			raw := "{\n\"name\":\"patch\",\"input\":\"before" + control + `after \"quoted\" \\n \\t \\r \\path 中文"}`
			got, err := decodeTransportCode(raw)
			if err != nil {
				t.Fatal(err)
			}
			want := "before" + control + `after "quoted" \n \t \r \path 中文`
			if got["input"] != want {
				t.Fatal("repair changed the payload's control characters, quotes or literal backslashes")
			}
		})
	}
}

func TestTransportJSONControlRepairRejectsAmbiguousOrIncompleteInput(t *testing.T) {
	for _, raw := range []string{
		"{\"name\":\"patch\",\"input\":\"raw\nline\"} {}",
		"{\"name\":\"patch\",\"input\":\"raw\nline\"} trailing",
		"{\"name\":\"patch\",\"input\":\"raw\nline\"",
		"{\"name\":\"patch\",\"input\":\"raw\nline",
		"{\"name\":\"patch\",\"input\":\"raw\n\"unescaped quote\"\"}",
		"{\"name\":\"patch\" \"input\":\"raw\nline\"}",
		"{\"name\":\"patch\",\"input\":\"raw\x00line\"}",
		"{\"name\":\"patch\",\"input\":\"raw\bline\"}",
		"{\"name\":\"patch\",\"input\":\"raw\fline\"}",
	} {
		if _, err := decodeTransportCode(raw); err == nil {
			t.Fatal("control repair must not guess quotes, separators, truncation or additional values")
		}
	}
}

// Model the observed formatting error without corrupting literal \\n strings:
// each real JSON control escape becomes the same raw character inside its value.
func rawControlsInTestJSON(raw string) string {
	var output strings.Builder
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\\' && i+1 < len(raw) {
			i++
			switch raw[i] {
			case 'n':
				_ = output.WriteByte('\n')
			case 'r':
				_ = output.WriteByte('\r')
			case 't':
				_ = output.WriteByte('\t')
			default:
				_ = output.WriteByte('\\')
				_ = output.WriteByte(raw[i])
			}
		} else {
			_ = output.WriteByte(raw[i])
		}
	}
	return output.String()
}

func TestLargeMultilineCustomPatchTransportRoundTrip(t *testing.T) {
	var patch strings.Builder
	_, _ = patch.WriteString("*** Begin Patch\r\n*** Update File: src/example.ts\r\n@@\r\n")
	for i := 0; i < 110; i++ {
		fmt.Fprintf(&patch, "+\tconst line%d = \"中文 \\\"quoted\\\" C:\\\\Projects\\\\data\"; // regex \\d+ and literal \\n\r\n", i)
	}
	_, _ = patch.WriteString("*** End Patch\r\n")
	input := patch.String()
	if len(input) < 9000 || len(input) > 13000 {
		t.Fatalf("expected a representative roughly 10 KB patch, got %d bytes", len(input))
	}
	innerJSON, err := json.Marshal(object{"name": "patch", "input": input})
	if err != nil {
		t.Fatal(err)
	}
	for _, rawControls := range []bool{false, true} {
		t.Run(fmt.Sprintf("raw_controls=%t", rawControls), func(t *testing.T) {
			code := string(innerJSON)
			if rawControls {
				code = rawControlsInTestJSON(code)
			}
			outerJSON, err := json.Marshal(object{"code": code, "summary": "Apply multiline patch", "references": []any{}})
			if err != nil {
				t.Fatal(err)
			}
			native := object{"type": "function_call", "name": "run_officejs", "id": "fc_large_patch", "call_id": "call_large_patch", "arguments": string(outerJSON), "status": "completed"}
			cache := new(ReplayCache)
			source := testSource()
			source["tools"] = []any{object{"type": "custom", "name": "patch"}}
			_, bridge := mustPrepare(t, source, "patch-account/key", cache)
			call, err := bridge.translateCall(native)
			if err != nil {
				t.Fatal(err)
			}
			if call["input"] != input || call["type"] != "custom_tool_call" {
				t.Fatal("large patch transport failed to preserve the exact custom-tool input")
			}
			if !reflect.DeepEqual(cache.get("patch-account/key", "call_large_patch"), native) {
				t.Fatal("repair must preserve the original native item for exact history replay")
			}
			source["input"] = []any{message("user", "Apply the patch"), call, object{"type": "custom_tool_call_output", "call_id": "call_large_patch", "output": "patch accepted"}}
			replayed, _ := mustPrepare(t, source, "patch-account/key", cache)
			items := mustTestValue[[]any](t, replayed["input"])
			if !reflect.DeepEqual(items[len(items)-2], native) || mustTestValue[object](t, items[len(items)-1])["output"] != "patch accepted" {
				t.Fatal("tool-result replay did not preserve the original transport and client result")
			}
		})
	}
}
