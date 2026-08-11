package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractOpenAIReasoningEffortFromModelSuffixPreservesModelID(t *testing.T) {
	tests := []struct {
		name  string
		body  []byte
		model string
		want  string
	}{
		{
			name:  "Chat Completions real codex max model",
			body:  []byte(`{"model":"gpt-5.1-codex-max","messages":[]}`),
			model: "gpt-5.1-codex-max",
			want:  "xhigh",
		},
		{
			name:  "Responses real gemini high model",
			body:  []byte(`{"model":"gemini-3.6-flash-high","input":"hello"}`),
			model: "gemini-3.6-flash-high",
			want:  "high",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := append([]byte(nil), tt.body...)

			got := extractOpenAIReasoningEffortFromBody(tt.body, tt.model)

			require.NotNil(t, got)
			require.Equal(t, tt.want, *got)
			require.Equal(t, before, tt.body, "effort derivation must not rewrite the request model")
		})
	}
}

func TestExtractOpenAIReasoningEffortExplicitValueTakesPrecedenceOverModelSuffix(t *testing.T) {
	tests := []struct {
		name string
		body []byte
		want string
	}{
		{
			name: "Chat Completions flat effort",
			body: []byte(`{"model":"gemini-3.6-flash-high","reasoning_effort":"low","messages":[]}`),
			want: "low",
		},
		{
			name: "Responses nested effort",
			body: []byte(`{"model":"gpt-5.1-codex-max","reasoning":{"effort":"medium"},"input":"hello"}`),
			want: "medium",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := append([]byte(nil), tt.body...)

			got := extractOpenAIReasoningEffortFromBody(tt.body, "gpt-5.1-codex-max")

			require.NotNil(t, got)
			require.Equal(t, tt.want, *got)
			require.Equal(t, before, tt.body, "explicit effort extraction must not rewrite the request model")
		})
	}
}
