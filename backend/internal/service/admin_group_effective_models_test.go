package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveGroupEffectiveModelsUsesAccountMappings(t *testing.T) {
	group := &Group{Platform: PlatformOpenAI}
	accounts := []Account{
		{
			Platform: PlatformOpenAI,
			Credentials: map[string]any{
				"model_mapping": map[string]any{
					"gpt-5.4": "gpt-5.4",
					"gpt-5.5": "gpt-5.5",
				},
			},
		},
		{
			Platform: PlatformAnthropic,
			Credentials: map[string]any{
				"model_mapping": map[string]any{"claude-sonnet-4-6": "claude-sonnet-4-6"},
			},
		},
	}

	models := resolveGroupEffectiveModels(group, accounts)

	require.Equal(t, []string{"gpt-5.4", "gpt-5.5"}, models)
}

func TestResolveGroupEffectiveModelsAppliesCustomGroupList(t *testing.T) {
	group := &Group{
		Platform: PlatformOpenAI,
		ModelAllowlist: GroupModelAllowlist{
			Enabled: true,
			Models:  []string{"gpt-5.4"},
		},
	}
	accounts := []Account{{
		Platform: PlatformOpenAI,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"gpt-5.4": "gpt-5.4",
				"gpt-5.5": "gpt-5.5",
			},
		},
	}}

	models := resolveGroupEffectiveModels(group, accounts)

	require.Equal(t, []string{"gpt-5.4"}, models)
}

func TestResolveGroupEffectiveModelsFallsBackToPlatformDefaults(t *testing.T) {
	accounts := []Account{{Platform: PlatformOpenAI, Credentials: map[string]any{}}}

	models := resolveGroupEffectiveModels(&Group{Platform: PlatformOpenAI}, accounts)

	require.Contains(t, models, "gpt-5.4")
}

func TestResolveGroupEffectiveModelsReturnsEmptyWithoutMatchingAccounts(t *testing.T) {
	accounts := []Account{{Platform: PlatformAnthropic, Credentials: map[string]any{}}}

	models := resolveGroupEffectiveModels(&Group{Platform: PlatformOpenAI}, accounts)

	require.Empty(t, models)
}
