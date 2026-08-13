package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeResponsesFunctionToolChoice(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		want        string
		wantChanged bool
	}{
		{
			name:        "chat function choice",
			input:       `{"type":"function","function":{"name":"get_weather"}}`,
			want:        `{"type":"function","name":"get_weather"}`,
			wantChanged: true,
		},
		{
			name:        "top-level name wins and extensions survive",
			input:       `{"type":"function","name":"get_weather","function":{"name":"legacy_name"},"provider_extension":{"enabled":true}}`,
			want:        `{"type":"function","name":"get_weather","provider_extension":{"enabled":true}}`,
			wantChanged: true,
		},
		{
			name:  "responses function choice",
			input: ` {"type":"function","name":"get_weather"} `,
			want:  ` {"type":"function","name":"get_weather"} `,
		},
		{
			name:  "string choice",
			input: `"required"`,
			want:  `"required"`,
		},
		{
			name:  "non-function object choice",
			input: `{"type":"web_search_preview"}`,
			want:  `{"type":"web_search_preview"}`,
		},
		{
			name:  "missing legacy name",
			input: `{"type":"function","function":{}}`,
			want:  `{"type":"function","function":{}}`,
		},
		{
			name:  "malformed JSON",
			input: `{"type":"function"`,
			want:  `{"type":"function"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := json.RawMessage(tt.input)
			got, changed := NormalizeResponsesFunctionToolChoice(input)
			require.Equal(t, tt.wantChanged, changed)
			if tt.wantChanged {
				require.JSONEq(t, tt.want, string(got))
				return
			}
			require.Equal(t, tt.want, string(got), "unchanged choices must retain their original bytes")
		})
	}
}
