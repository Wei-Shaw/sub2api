package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestMinimaxMiMoProviderDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		platform      string
		mode          string
		chatBase      string
		anthropicBase string
	}{
		{"minimax payg", PlatformMinimax, AccountModePayG, DefaultMinimaxBaseURL, DefaultMinimaxAnthropicBaseURL},
		{"minimax token plan", PlatformMinimax, AccountModeCoding, DefaultMinimaxBaseURL, DefaultMinimaxAnthropicBaseURL},
		{"mimo payg", PlatformMiMo, AccountModePayG, DefaultMiMoPayGBaseURL, DefaultMiMoPayGAnthropicBaseURL},
		{"mimo token plan", PlatformMiMo, AccountModeCoding, DefaultMiMoCodingBaseURL, DefaultMiMoCodingAnthropicBaseURL},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{Platform: tt.platform, Type: AccountTypeAPIKey, Credentials: map[string]any{
				"account_mode": tt.mode,
				"api_protocol": APIProtocolAdaptive,
			}}
			require.True(t, account.IsCNProvider())
			require.True(t, account.SupportsNativeResponses())
			require.Equal(t, tt.chatBase, account.GetCNProtocolBaseURL(APIProtocolChatCompletions))
			require.Equal(t, tt.anthropicBase, account.GetCNProtocolBaseURL(APIProtocolAnthropic))
			require.Equal(t, tt.chatBase, account.GetCNProtocolBaseURL(APIProtocolResponses))
		})
	}
}

func TestMiMoAdaptiveRegionalBaseURLsArePreserved(t *testing.T) {
	t.Parallel()

	account := &Account{Platform: PlatformMiMo, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"account_mode": AccountModeCoding,
		"api_protocol": APIProtocolAdaptive,
		"api_base_urls": map[string]any{
			APIProtocolChatCompletions: "https://token-plan-sgp.xiaomimimo.com/v1",
			APIProtocolAnthropic:       "https://token-plan-sgp.xiaomimimo.com/anthropic",
			APIProtocolResponses:       "https://token-plan-sgp.xiaomimimo.com/v1",
		},
	}}

	require.Equal(t, "https://token-plan-sgp.xiaomimimo.com/v1", account.GetCNProtocolBaseURL(APIProtocolChatCompletions))
	require.Equal(t, "https://token-plan-sgp.xiaomimimo.com/anthropic", account.GetCNProtocolBaseURL(APIProtocolAnthropic))
	require.Equal(t, "https://token-plan-sgp.xiaomimimo.com/v1", account.GetCNProtocolBaseURL(APIProtocolResponses))
}

func TestMinimaxMiMoAnthropicModelSyncPreservesOrigin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		platform string
		baseURL  string
		want     string
	}{
		{"mimo sgp", PlatformMiMo, "https://token-plan-sgp.xiaomimimo.com/anthropic", "https://token-plan-sgp.xiaomimimo.com/v1"},
		{"mimo custom relay", PlatformMiMo, "https://relay.example.com/vendor/mimo/anthropic", "https://relay.example.com/vendor/mimo/v1"},
		{"minimax custom relay", PlatformMinimax, "https://relay.example.com/vendor/minimax/anthropic", "https://relay.example.com/vendor/minimax/v1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{Platform: tt.platform, Type: AccountTypeAPIKey, Credentials: map[string]any{
				"api_protocol": APIProtocolAnthropic,
				"account_mode": AccountModeCoding,
				"base_url":     tt.baseURL,
			}}
			require.Equal(t, tt.want, account.GetOpenAIFormatBaseURL())
		})
	}
}

func TestMinimaxMiMoProtocolsAndAnthropicAuth(t *testing.T) {
	t.Parallel()

	for _, platform := range []string{PlatformMinimax, PlatformMiMo} {
		account := &Account{Platform: platform, Type: AccountTypeAPIKey, Credentials: map[string]any{
			"api_protocol": APIProtocolResponses,
		}}
		require.Equal(t, APIProtocolResponses, account.GetAPIProtocol())
		require.Equal(t, AnthropicAPIKeyAuthSchemeAuthorizationBearer, account.GetAnthropicAPIKeyAuthScheme())

		account.Extra = map[string]any{anthropicAPIKeyAuthSchemeExtraKey: AnthropicAPIKeyAuthSchemeXAPIKey}
		require.Equal(t, AnthropicAPIKeyAuthSchemeXAPIKey, account.GetAnthropicAPIKeyAuthScheme())
	}
}

func TestMinimaxMiMoSchedulerAndModelFallbacks(t *testing.T) {
	t.Parallel()

	require.Equal(t, PlatformMinimax, NormalizeOpenAICompatiblePlatform(PlatformMinimax))
	require.Equal(t, PlatformMiMo, NormalizeOpenAICompatiblePlatform(PlatformMiMo))
	require.Contains(t, DefaultCNProviderModelIDs(PlatformMinimax), "MiniMax-M3")
	require.Equal(t, []string{"mimo-v2.5", "mimo-v2.5-pro"}, DefaultCNProviderModelIDs(PlatformMiMo))
	require.Equal(t, "MiniMax-M3", minimaxMiMoTestModel(&Account{Platform: PlatformMinimax}, ""))
	require.Equal(t, "mimo-v2.5", minimaxMiMoTestModel(&Account{Platform: PlatformMiMo}, ""))
	require.Empty(t, minimaxMiMoTestModel(&Account{Platform: PlatformKimi}, ""))
}

func TestMiMoFallbackPricing(t *testing.T) {
	t.Parallel()

	svc := NewBillingService(&config.Config{}, nil)
	mimo, err := svc.GetModelPricing("mimo-v2.5")
	require.NoError(t, err)
	require.InDelta(t, 0.14e-6, mimo.InputPricePerToken, 1e-15)
	require.InDelta(t, 0.28e-6, mimo.OutputPricePerToken, 1e-15)
	require.InDelta(t, 0.0028e-6, mimo.CacheReadPricePerToken, 1e-15)

	mimoPro, err := svc.GetModelPricing("mimo-v2.5-pro")
	require.NoError(t, err)
	require.InDelta(t, 0.435e-6, mimoPro.InputPricePerToken, 1e-15)
	require.InDelta(t, 0.87e-6, mimoPro.OutputPricePerToken, 1e-15)
	require.InDelta(t, 0.0036e-6, mimoPro.CacheReadPricePerToken, 1e-15)
}
