package claude

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRejectsTemperature(t *testing.T) {
	t.Parallel()

	tests := []struct {
		model string
		want  bool
	}{
		{model: "claude-opus-4-8", want: true},
		{model: "claude-opus-4-8-20260529", want: true},
		{model: "claude-opus-4-7", want: true},
		{model: "anthropic/claude-opus-4-8", want: true},
		{model: "us.anthropic.claude-opus-4-8-v1", want: true},
		{model: "claude-opus-5-5", want: true},
		{model: "anthropic/claude-opus-5.5", want: true},
		{model: "claude-fable-5-2", want: true},
		{model: "claude-mythos-5-2", want: true},
		{model: "claude-opus-4-6", want: false},
		{model: "claude-opus-4-5-20251101", want: false},
		{model: "claude-sonnet-4-6", want: false},
		{model: "claude-sonnet-5-5", want: false},
		{model: "claude-haiku-5-5", want: false},
		{model: "claude-haiku-4-5-20251001", want: false},
		{model: "gpt-5.6", want: false},
		{model: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, RejectsTemperature(tt.model))
		})
	}
}
