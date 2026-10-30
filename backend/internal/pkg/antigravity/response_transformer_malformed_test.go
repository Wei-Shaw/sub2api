package antigravity

import (
	"errors"
	"testing"
)

func TestMalformedFunctionCallTransform(t *testing.T) {
	for _, body := range []string{
		`{"candidates":[{"finishReason":"MALFORMED_FUNCTION_CALL","content":{"parts":[{"thoughtSignature":"test-signature"}]}}]}`,
		`{"response":{"candidates":[{"finishReason":"MALFORMED_FUNCTION_CALL"}]}}`,
	} {
		response, _, err := TransformGeminiToClaude([]byte(body), "test-model")
		if !errors.Is(err, ErrMalformedFunctionCall) || response != nil {
			t.Fatalf("expected malformed call error without successful response: %s %v", response, err)
		}
	}
}
