package service

import (
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Strict Responses clients (Rust serde: grok-build, Codex) declare
// Response.created_at as required — a response object without it fails the
// whole turn with `missing field 'created_at'` (issues #5601, #8118).
// Official upstreams always send it, but vendor gateways in front of Grok
// subscriptions and gateway-synthesized terminal frames may not, so every
// Response object leaving the gateway is stamped when the field is absent.

// responsesCreatedAtStamper keeps one created_at per Responses stream: the
// upstream's own value once seen, otherwise a single timestamp taken on first
// use, so response.created and the terminal event always agree.
type responsesCreatedAtStamper struct {
	createdAt int64
}

func (s *responsesCreatedAtStamper) learn(value gjson.Result) {
	if s.createdAt == 0 && value.Type == gjson.Number && value.Int() > 0 {
		s.createdAt = value.Int()
	}
}

// Value returns the stream's created_at, fixing it to now on first use.
func (s *responsesCreatedAtStamper) Value() int64 {
	if s.createdAt <= 0 {
		s.createdAt = time.Now().Unix()
	}
	return s.createdAt
}

// Observe records the upstream created_at without modifying the payload.
func (s *responsesCreatedAtStamper) Observe(payload []byte, eventType string) {
	if !strings.HasPrefix(strings.TrimSpace(eventType), "response.") {
		return
	}
	s.learn(gjson.GetBytes(payload, "response.created_at"))
}

// Stamp adds response.created_at to a Responses lifecycle event whose
// response object lacks it (absent or null). Any other value is left as-is.
func (s *responsesCreatedAtStamper) Stamp(payload []byte, eventType string) ([]byte, bool) {
	if !strings.HasPrefix(strings.TrimSpace(eventType), "response.") {
		return payload, false
	}
	response := gjson.GetBytes(payload, "response")
	if !response.IsObject() {
		return payload, false
	}
	createdAt := response.Get("created_at")
	if createdAt.Exists() && createdAt.Type != gjson.Null {
		s.learn(createdAt)
		return payload, false
	}
	stamped, err := sjson.SetBytes(payload, "response.created_at", s.Value())
	if err != nil {
		return payload, false
	}
	return stamped, true
}

// ensureOpenAIResponseObjectCreatedAt stamps created_at onto a non-streaming
// Response object (`"object":"response"`) that lacks it.
func ensureOpenAIResponseObjectCreatedAt(body []byte) []byte {
	if gjson.GetBytes(body, "object").String() != "response" {
		return body
	}
	createdAt := gjson.GetBytes(body, "created_at")
	if createdAt.Exists() && createdAt.Type != gjson.Null {
		return body
	}
	stamped, err := sjson.SetBytes(body, "created_at", time.Now().Unix())
	if err != nil {
		return body
	}
	return stamped
}

// openAIResponsesFailedSequenceNumber reuses the sequence slot of the bare
// error event a synthesized response.failed replaces (that error frame is
// suppressed), keeping the client-visible sequence monotonic; 0 when unknown.
func openAIResponsesFailedSequenceNumber(source []byte) int64 {
	if seq := gjson.GetBytes(source, "sequence_number"); seq.Type == gjson.Number && seq.Int() >= 0 {
		return seq.Int()
	}
	return 0
}
