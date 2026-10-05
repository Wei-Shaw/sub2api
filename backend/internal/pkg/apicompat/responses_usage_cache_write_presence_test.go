package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResponsesUsageCacheWritePresence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		payload     string
		wantTokens  int
		wantPresent bool
	}{
		{
			name:        "missing",
			payload:     `{"input_tokens":100,"input_tokens_details":{"cached_tokens":64}}`,
			wantTokens:  0,
			wantPresent: false,
		},
		{
			name:        "nested explicit zero",
			payload:     `{"input_tokens":100,"input_tokens_details":{"cached_tokens":64,"cache_write_tokens":0}}`,
			wantTokens:  0,
			wantPresent: true,
		},
		{
			name:        "nested positive",
			payload:     `{"input_tokens":100,"input_tokens_details":{"cache_write_tokens":32}}`,
			wantTokens:  32,
			wantPresent: true,
		},
		{
			name:        "legacy explicit zero",
			payload:     `{"input_tokens":100,"cache_creation_input_tokens":0}`,
			wantTokens:  0,
			wantPresent: true,
		},
		{
			name:        "nested explicit null",
			payload:     `{"input_tokens":100,"input_tokens_details":{"cache_write_tokens":null}}`,
			wantTokens:  0,
			wantPresent: true,
		},
		{
			name:        "top-level explicit null",
			payload:     `{"input_tokens":100,"cache_write_tokens":null}`,
			wantTokens:  0,
			wantPresent: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var usage ResponsesUsage
			require.NoError(t, json.Unmarshal([]byte(tc.payload), &usage))
			require.Equal(t, tc.wantTokens, usage.CacheCreationInputTokens)
			require.Equal(t, tc.wantPresent, usage.CacheCreationInputTokensPresent)
		})
	}
}
