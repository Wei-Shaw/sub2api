package service

import "github.com/tidwall/gjson"

// HasCompactionTriggerInInput detects the Codex remote compact v2 body signal:
// an input item with type "compaction_trigger". When the client sends this
// inside a normal POST /v1/responses (instead of POST /v1/responses/compact),
// the request must still be treated as a compact request — otherwise the
// upstream path, model mapping, and body normalization are all wrong, causing
// Codex to receive a non-compact response and fail with:
//
//	"remote compaction v2 expected exactly one compaction output item, got 0"
//
// The gateway handler promotes such requests by rewriting the URL path to the
// compact form before stream parsing, compact body normalization, and
// compact-capable account scheduling, so both inbound forms share one code path.
func HasCompactionTriggerInInput(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return false
	}
	found := false
	input.ForEach(func(_, item gjson.Result) bool {
		if item.Get("type").String() == "compaction_trigger" {
			found = true
			return false
		}
		return true
	})
	return found
}
