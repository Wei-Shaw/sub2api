package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIStreamKeyReplayAllowed(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{"plain text input", `{"input":"hello"}`, true},
		{"self-contained text messages", `{"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}]}`, true},
		{"client function tools", `{"input":"hello","tools":[{"type":"function","name":"lookup"}]}`, true},
		{"prior response", `{"input":"hello","previous_response_id":"resp_1"}`, false},
		{"conversation state", `{"input":"hello","conversation":"thread_1"}`, false},
		{"background request", `{"input":"hello","background":true}`, false},
		{"encrypted history", `{"input":[{"type":"reasoning","encrypted_content":"fixture"}]}`, false},
		{"tool output history", `{"input":[{"type":"function_call_output","call_id":"call_1","output":"done"}]}`, false},
		{"server side tool", `{"input":"hello","tools":[{"type":"web_search"}]}`, false},
		{"prompt based request", `{"input":"hello","prompt":{"id":"prompt_1"}}`, false},
		{"invalid JSON", "{", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, OpenAIStreamKeyReplayAllowed(context.Background(), []byte(tt.body)))
		})
	}
}

func TestOpenAIStreamKeyReplayRejectsCanceledRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.False(t, OpenAIStreamKeyReplayAllowed(ctx, []byte(`{"input":"hello"}`)))
}
