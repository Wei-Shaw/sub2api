//go:build unit

package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnthropicUsageCacheReadPresenceRoundtrip(t *testing.T) {
	for _, test := range []struct {
		name    string
		payload string
		present bool
		value   int
	}{
		{"missing", `{}`, false, 0},
		{"null", `{"cache_read_input_tokens":null,"cached_tokens":null,"prompt_tokens_details":{"cached_tokens":null}}`, false, 0},
		{"zero", `{"cache_read_input_tokens":0,"cached_tokens":0,"prompt_tokens_details":{"cached_tokens":0}}`, true, 0},
		{"positive", `{"cache_read_input_tokens":20,"cached_tokens":20,"prompt_tokens_details":{"cached_tokens":20}}`, true, 20},
	} {
		t.Run(test.name, func(t *testing.T) {
			var usage AnthropicUsage
			require.NoError(t, json.Unmarshal([]byte(test.payload), &usage))
			for range 2 {
				require.Equal(t, test.present, usage.CacheReadInputTokensPresent)
				require.Equal(t, test.present, usage.CachedTokensPresent)
				require.Equal(t, test.value, usage.CacheReadInputTokens)
				require.Equal(t, test.value, usage.CachedTokens)
				if usage.PromptTokensDetails != nil {
					require.Equal(t, test.present, usage.PromptTokensDetails.CachedTokensPresent)
					require.Equal(t, test.value, usage.PromptTokensDetails.CachedTokens)
				}
				encoded, err := json.Marshal(usage)
				require.NoError(t, err)
				require.NoError(t, json.Unmarshal(encoded, &usage))
			}
		})
	}
}
