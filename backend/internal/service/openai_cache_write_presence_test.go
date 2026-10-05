package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAICacheCreationTokensFromUsageWithPresence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		usage       string
		wantTokens  int
		wantPresent bool
	}{
		{
			name:        "missing",
			usage:       `{"input_tokens":100,"input_tokens_details":{"cached_tokens":64}}`,
			wantTokens:  0,
			wantPresent: false,
		},
		{
			name:        "nested explicit zero",
			usage:       `{"input_tokens":100,"input_tokens_details":{"cached_tokens":64,"cache_write_tokens":0}}`,
			wantTokens:  0,
			wantPresent: true,
		},
		{
			name:        "nested positive",
			usage:       `{"input_tokens":100,"input_tokens_details":{"cache_write_tokens":32}}`,
			wantTokens:  32,
			wantPresent: true,
		},
		{
			name:        "legacy explicit zero",
			usage:       `{"input_tokens":100,"cache_creation_input_tokens":0}`,
			wantTokens:  0,
			wantPresent: true,
		},
		{
			name:        "top-level zero does not hide later positive alias",
			usage:       `{"input_tokens":100,"cache_write_tokens":0,"cache_creation_input_tokens":19}`,
			wantTokens:  19,
			wantPresent: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, present := openAICacheCreationTokensFromUsageWithPresence(gjson.Parse(tc.usage))
			require.Equal(t, tc.wantTokens, got)
			require.Equal(t, tc.wantPresent, present)
		})
	}
}

func TestOpenAIUsageFromGJSON_PreservesCacheWritePresence(t *testing.T) {
	t.Parallel()

	missing, ok := openAIUsageFromGJSON(gjson.Parse(`{"input_tokens":100,"input_tokens_details":{"cached_tokens":64}}`))
	require.True(t, ok)
	require.False(t, missing.CacheCreationInputTokensPresent)

	explicitZero, ok := openAIUsageFromGJSON(gjson.Parse(`{"input_tokens":100,"input_tokens_details":{"cached_tokens":64,"cache_write_tokens":0}}`))
	require.True(t, ok)
	require.True(t, explicitZero.CacheCreationInputTokensPresent)
	require.Zero(t, explicitZero.CacheCreationInputTokens)
}
