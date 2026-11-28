package openai_compat

import "testing"

func TestShouldRejectGPT56ResponsesExplicitCache(t *testing.T) {
	tests := []struct {
		name  string
		model string
		body  []byte
		want  bool
	}{
		{
			name:  "rejects sol",
			model: "gpt-5.6-sol",
			body:  []byte(`{"model":"gpt-5.6-sol","input":"hello","explicit_cache":{"ttl":"30m"}}`),
			want:  true,
		},
		{
			name:  "rejects provider prefixed terra",
			model: "openai/gpt-5.6-terra",
			body:  []byte(`{"model":"openai/gpt-5.6-terra","input":"hello","explicit_cache":true}`),
			want:  true,
		},
		{
			name:  "rejects case insensitive dated luna",
			model: "GPT-5.6-LUNA-2026-07-09",
			body:  []byte(`{"model":"GPT-5.6-LUNA-2026-07-09","input":"hello","explicit_cache":null}`),
			want:  true,
		},
		{
			name:  "rejects alias",
			model: "gpt-5.6",
			body:  []byte(`{"model":"gpt-5.6","input":"hello","explicit_cache":{}}`),
			want:  true,
		},
		{
			name:  "does not reject older model",
			model: "gpt-5.5",
			body:  []byte(`{"model":"gpt-5.5","input":"hello","explicit_cache":{}}`),
			want:  false,
		},
		{
			name:  "does not reject missing explicit cache",
			model: "gpt-5.6-sol",
			body:  []byte(`{"model":"gpt-5.6-sol","input":"hello"}`),
			want:  false,
		},
		{
			name:  "does not reject nested explicit cache",
			model: "gpt-5.6-sol",
			body:  []byte(`{"model":"gpt-5.6-sol","input":[{"role":"user","content":[{"type":"input_text","text":"hello","explicit_cache":true}]}]}`),
			want:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ShouldRejectGPT56ResponsesExplicitCache(tc.model, tc.body)
			if got != tc.want {
				t.Fatalf("ShouldRejectGPT56ResponsesExplicitCache(%q, body) = %v, want %v", tc.model, got, tc.want)
			}
		})
	}
}
