package claude

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEffortLevelsForModel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		model string
		want  []string
	}{
		{model: "claude-opus-4-6", want: []string{"low", "medium", "high", "max"}},
		{model: "anthropic/claude-sonnet-4-6", want: []string{"low", "medium", "high", "max"}},
		{model: "claude-opus-5", want: []string{"low", "medium", "high", "xhigh", "max"}},
		{model: "claude-opus-4-5-20251101", want: []string{"low", "medium", "high"}},
		{model: "claude-haiku-4-5-20251001", want: nil},
		{model: "gpt-5.6", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, EffortLevelsForModel(tt.model))
		})
	}
}
