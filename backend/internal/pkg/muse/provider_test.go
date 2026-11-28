package muse

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

func TestCapabilitiesRejectUnqualifiedSemantics(t *testing.T) {
	base := Capabilities{Models: []string{"muse/assistant"}}
	for _, raw := range []string{
		`{"model":"muse/assistant","input":"hello","stream":true}`,
		`{"model":"muse/assistant","input":"hello","instructions":"system"}`,
		`{"model":"muse/assistant","input":"hello","max_output_tokens":1}`,
		`{"model":"muse/assistant","input":"hello","previous_response_id":"resp_x"}`,
		`{"model":"muse/assistant","input":[{"type":"function_call_output","call_id":"x","output":"result"}]}`,
		`{"model":"muse/assistant","input":[{"role":"user","content":[{"type":"input_image","image_url":"https://example.invalid/image"}]}]}`,
	} {
		var req apicompat.ResponsesRequest
		if err := json.Unmarshal([]byte(raw), &req); err != nil {
			t.Fatal(err)
		}
		if base.Validate(&req) == nil {
			t.Fatalf("silently accepted unsupported contract: %s", raw)
		}
	}
	var req apicompat.ResponsesRequest
	_ = json.Unmarshal([]byte(`{"model":"muse/assistant","input":"hello"}`), &req)
	if err := base.Validate(&req); err != nil {
		t.Fatal(err)
	}
}
