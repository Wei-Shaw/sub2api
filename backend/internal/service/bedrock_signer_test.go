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
REDACTED
			"aws_access_key_id":     "test-akid",
			"aws_secret_access_key": "test-secret",
	REDACTED,
REDACTED

	signer, err := NewBedrockSignerFromAccount(account)
REDACTED
	require.NotNil(t, signer)
	assert.Equal(t, defaultBedrockRegion, signer.region)
REDACTED

func TestFilterBetaTokens(t *testing.T) {
	tokens := []string{"REDACTED", "tool-search-tool-2025-10-19"REDACTED
	filterSet := map[string]struct{REDACTED{
		"tool-search-tool-2025-10-19": {REDACTED,
REDACTED

	assert.Equal(t, []string{"REDACTED"REDACTED, filterBetaTokens(tokens, filterSet))
	assert.Equal(t, tokens, filterBetaTokens(tokens, nil))
	assert.Nil(t, filterBetaTokens(nil, filterSet))
REDACTED
