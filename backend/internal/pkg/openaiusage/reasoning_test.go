package openaiusage

import "testing"

func TestOutputTokensWithMissingReasoning(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                         string
		input, output, total, reason int64
		want                         int64
	}{
		{name: "additive xAI usage", input: 10, output: 20, total: 35, reason: 5, want: 25},
		{name: "bounded by unexplained total", input: 10, output: 20, total: 33, reason: 5, want: 23},
		{name: "output already includes reasoning", input: 10, output: 25, total: 35, reason: 5, want: 25},
		{name: "absent total represented as zero", input: 10, output: 20, total: 0, reason: 5, want: 20},
		{name: "inconsistent total", input: 10, output: 20, total: 25, reason: 5, want: 20},
		{name: "negative input", input: -1, output: 20, total: 24, reason: 5, want: 20},
		{name: "negative output", input: 10, output: -1, total: 14, reason: 5, want: -1},
		{name: "negative total", input: 10, output: 20, total: -1, reason: 5, want: 20},
		{name: "negative reasoning", input: 10, output: 20, total: 35, reason: -5, want: 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := OutputTokensWithMissingReasoning(tt.input, tt.output, tt.total, tt.reason); got != tt.want {
				t.Fatalf("got %d, want %d", got, tt.want)
			}
		})
	}
}
