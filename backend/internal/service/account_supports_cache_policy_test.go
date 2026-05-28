package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccount_SupportsCachePolicy_NilSafe(t *testing.T) {
	var a *Account
	require.False(t, a.SupportsCachePolicy(), "nil receiver must not panic and must return false")
}

func TestAccount_SupportsCachePolicy_AnthropicTypes(t *testing.T) {
	supported := []string{
		AccountTypeOAuth,
		AccountTypeSetupToken,
		AccountTypeAPIKey,
		AccountTypeBedrock,
		AccountTypeServiceAccount,
		AccountTypeVertex,
	}
	for _, typ := range supported {
		t.Run(typ, func(t *testing.T) {
			a := &Account{Platform: PlatformAnthropic, Type: typ}
			require.True(t, a.SupportsCachePolicy(), "Anthropic %s should support cache policy", typ)
		})
	}
}

func TestAccount_SupportsCachePolicy_NonAnthropicPlatforms(t *testing.T) {
	cases := []struct {
		platform string
		typ      string
	}{
		{PlatformOpenAI, AccountTypeOAuth},
		{PlatformOpenAI, AccountTypeAPIKey},
		{PlatformGemini, AccountTypeOAuth},
		{PlatformGemini, AccountTypeAPIKey},
		{PlatformKiro, AccountTypeOAuth},
		{PlatformAntigravity, AccountTypeAPIKey},
	}
	for _, tc := range cases {
		t.Run(tc.platform+"/"+tc.typ, func(t *testing.T) {
			a := &Account{Platform: tc.platform, Type: tc.typ}
			require.False(t, a.SupportsCachePolicy(),
				"%s/%s should not surface cache policy controls", tc.platform, tc.typ)
		})
	}
}

func TestAccount_SupportsCachePolicy_UnknownAnthropicTypeRejected(t *testing.T) {
	a := &Account{Platform: PlatformAnthropic, Type: "definitely_not_a_real_type"}
	require.False(t, a.SupportsCachePolicy(),
		"unrecognized Anthropic account type must fail closed to avoid accidentally exposing cache controls")
}

func TestAccount_SupportsCachePolicy_AlignsWithExistingBedrockVertexHelpers(t *testing.T) {
	bedrock := &Account{Platform: PlatformAnthropic, Type: AccountTypeBedrock}
	require.True(t, bedrock.IsBedrock())
	require.True(t, bedrock.SupportsCachePolicy())

	vertex := &Account{Platform: PlatformAnthropic, Type: AccountTypeServiceAccount}
	require.True(t, vertex.IsVertex())
	require.True(t, vertex.SupportsCachePolicy())

	vertexLegacy := &Account{Platform: PlatformAnthropic, Type: AccountTypeVertex}
	require.True(t, vertexLegacy.IsVertex())
	require.True(t, vertexLegacy.SupportsCachePolicy())
}

func TestAccount_SupportsCachePolicy_IsSupersetOfAnthropicOAuthSetupToken(t *testing.T) {
	for _, typ := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		a := &Account{Platform: PlatformAnthropic, Type: typ}
		require.True(t, a.IsAnthropicOAuthOrSetupToken())
		require.True(t, a.SupportsCachePolicy(),
			"SupportsCachePolicy must remain a superset of IsAnthropicOAuthOrSetupToken so existing behavior never regresses")
	}
}
