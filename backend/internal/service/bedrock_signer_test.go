package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBedrockSignerFromAccount_DefaultRegion(t *testing.T) {
	account := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeBedrock,
		Credentials: map[string]any{
			"aws_access_key_id":     "test-akid",
			"aws_secret_access_key": "test-secret",
		},
	}

	signer, err := NewBedrockSignerFromAccount(account)
	require.NoError(t, err)
	require.NotNil(t, signer)
	assert.Equal(t, defaultBedrockRegion, signer.region)
}

func TestFilterBetaTokens(t *testing.T) {
	tokens := []string{"interleaved-thinking-2025-05-14", "tool-search-tool-2025-10-19"}
	filterSet := map[string]struct{}{
		"tool-search-tool-2025-10-19": {},
	}

	assert.Equal(t, []string{"interleaved-thinking-2025-05-14"}, filterBetaTokens(tokens, filterSet))
	assert.Equal(t, tokens, filterBetaTokens(tokens, nil))
	assert.Nil(t, filterBetaTokens(nil, filterSet))
}
