package antigravity

import (
	"encoding/json"
	"testing"
)

// Images returned by a tool (tool_result content array) must reach the model: parseToolResultContent only keeps
// text, so they are forwarded as inlineData parts after the function response.
func TestBuildParts_ToolResultImageIsForwarded(t *testing.T) {
	content := json.RawMessage(`[{"type":"tool_result","tool_use_id":"call_1","content":[` +
		`{"type":"text","text":"shot"},` +
		`{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAAA"}}]}]`)
	parts, _, err := buildParts(content, map[string]string{"call_1": "screenshot"}, true)
	if err != nil {
		t.Fatal(err)
	}
	var fr, img int
	for i, p := range parts {
		if p.FunctionResponse != nil {
			fr++
			if img > 0 {
				t.Fatalf("function response at %d must come before the image parts", i)
			}
		}
		if p.InlineData != nil {
			img++
			if p.InlineData.MimeType != "image/png" || p.InlineData.Data != "AAAA" {
				t.Fatalf("unexpected inline data: %+v", p.InlineData)
			}
		}
	}
	if fr != 1 || img != 1 {
		t.Fatalf("want 1 function response and 1 image part, got %d and %d (%+v)", fr, img, parts)
	}
}
