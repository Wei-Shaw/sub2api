package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeOpenAIBuiltinTools(t *testing.T) {
	t.Parallel()

	webSearch := []map[string]any{{"type": "web_search"}}

	tests := []struct {
		name string
		raw  any
		want []map[string]any
	}{
		{
			name: "true enables web search",
			raw:  true,
			want: webSearch,
		},
		{
			name: "any slice ignores unsupported builtin tools",
			raw:  []any{"web_search", "code_interpreter"},
			want: webSearch,
		},
		{
			name: "string slice keeps only web search",
			raw:  []string{"web_search", "image_generation"},
			want: webSearch,
		},
		{
			name: "any slice accepts web search tool object",
			raw:  []any{map[string]any{"type": "web_search"}},
			want: webSearch,
		},
		{
			name: "map slice accepts web search tool object",
			raw:  []map[string]any{{"type": "web_search"}},
			want: webSearch,
		},
		{
			name: "map image generation true is ignored",
			raw:  map[string]any{"web_search": true, "image_generation": true},
			want: webSearch,
		},
		{
			name: "configured image generation requires enabled true",
			raw: map[string]any{"web_search": true, "image_generation": map[string]any{
				"model":         "gpt-image-2",
				"output_format": "png",
			}},
			want: webSearch,
		},
		{
			name: "enabled configured image generation keeps allowed fields",
			raw: map[string]any{"image_generation": map[string]any{
				"enabled":            true,
				"model":              " GPT-IMAGE-2 ",
				"size":               "1024x1024",
				"quality":            "low",
				"output_format":      "webp",
				"output_compression": 75,
				"input_fidelity":     "high",
				"ignored":            "drop-me",
			}},
			want: []map[string]any{{
				"type":               "image_generation",
				"model":              "gpt-image-2",
				"size":               "1024x1024",
				"quality":            "low",
				"output_format":      "webp",
				"output_compression": 75,
				"input_fidelity":     "high",
			}},
		},
		{
			name: "enabled image generation rejects non gpt image 2 model",
			raw: map[string]any{"web_search": true, "image_generation": map[string]any{
				"enabled": true,
				"model":   "gpt-image-1",
			}},
			want: webSearch,
		},
		{
			name: "false returns nil",
			raw:  false,
			want: nil,
		},
		{
			name: "map false returns nil",
			raw:  map[string]any{"web_search": false},
			want: nil,
		},
		{
			name: "duplicate requests collapse to one entry",
			raw:  []any{"web_search", "code_interpreter", "web_search"},
			want: webSearch,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, normalizeOpenAIBuiltinTools(tt.raw))
		})
	}
}
