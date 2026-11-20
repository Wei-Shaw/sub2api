package service

import (
	"strings"

	"github.com/tidwall/gjson"
)

// ccStreamUpstreamError preserves an error carried inside HTTP 200 SSE.
// Comments, role-only chunks and DONE are transport metadata, not model output.
type ccStreamUpstreamError struct {
	Payload []byte
	Message string
}

func (e *ccStreamUpstreamError) Error() string { return e.Message }

func parseCCStreamUpstreamError(payload string) *ccStreamUpstreamError {
	if !gjson.Valid(payload) {
		return nil
	}
	root := gjson.Parse(payload)
	typ := root.Get("type").String()
	err := root.Get("error")
	if (!err.Exists() || err.Type == gjson.Null) && typ != "response.failed" && typ != "error" {
		return nil
	}
	message := extractOpenAISSEErrorMessage([]byte(payload))
	if strings.TrimSpace(message) == "" {
		message = "Upstream returned an error inside HTTP 200"
	}
	return &ccStreamUpstreamError{Payload: []byte(payload), Message: message}
}

func ccPayloadHasModelOutput(payload string) bool {
	for _, choice := range gjson.Get(payload, "choices").Array() {
		delta := choice.Get("delta")
		for _, field := range []string{"content", "reasoning_content", "reasoning", "refusal"} {
			if delta.Get(field).String() != "" {
				return true
			}
		}
		if len(delta.Get("tool_calls").Array()) > 0 || delta.Get("function_call").Exists() {
			return true
		}
	}
	return false
}
