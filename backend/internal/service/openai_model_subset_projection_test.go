//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildOpenAICanonicalModelCatalog_UsesFiniteSources(t *testing.T) {
	t.Parallel()

	accounts := []Account{
		{
			Credentials: map[string]any{
				"model_mapping": map[string]any{
					"gpt-5.4-Sys":              "gpt-5.4",
					"gpt-*":                    "gpt-5.4",
					"gpt-5.3-codex-spark-high": "gpt-5.3-codex-spark",
				},
			},
			Extra: map[string]any{
				"openai_capability_explicit_models": []string{"gpt-5.6", "gpt-5.4-mini-medium"},
			},
			Groups: []*Group{
				{
					DefaultMappedModel:    "gpt-5.2-high",
					AllowMessagesDispatch: true,
					MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{
						OpusMappedModel: "gpt-5.4-high",
						ExactModelMappings: map[string]string{
							"claude-sonnet-4-5-20250929": "gpt-5.1-codex-max-high",
						},
					},
				},
			},
		},
	}

	catalog := BuildOpenAICanonicalModelCatalog(accounts, nil, []string{"gpt-5.4-nano-Sys"})

	require.ElementsMatch(t, []string{
		"gpt-5.1-codex-max",
		"gpt-5.2",
		"gpt-5.3-codex",
		"gpt-5.4",
		"gpt-5.4-mini",
		"gpt-5.4-nano",
		"gpt-5.6",
	}, catalog)
	require.NotContains(t, catalog, "gpt-*")
	require.NotContains(t, catalog, "gpt-unknown-live")
}

func TestProjectionModelReachability_UnknownModelNeedsExplicitCapability(t *testing.T) {
	t.Parallel()

	account := Account{Credentials: map[string]any{}, Extra: map[string]any{}}
	require.False(t, accountSupportsProjectionModel(account, "gpt-5.6"))
}

func TestProjectionModelReachability_MissingMappingRejectsUnknownModel(t *testing.T) {
	t.Parallel()

	account := Account{Credentials: map[string]any{}, Extra: map[string]any{}}
	require.False(t, accountSupportsProjectionModel(account, "gpt-5.unknown"))
}

func TestProjectionModelReachability_EmptyMappingRejectsUnknownModel(t *testing.T) {
	t.Parallel()

	account := Account{
		Credentials: map[string]any{"model_mapping": map[string]any{}},
		Extra:       map[string]any{},
	}
	require.False(t, accountSupportsProjectionModel(account, "gpt-5.unknown"))
}

func TestProjectionModelReachability_UnknownModelRejectsNoMappingAndWildcardByDefault(t *testing.T) {
	t.Parallel()

	account := Account{
		Credentials: map[string]any{"model_mapping": map[string]any{"gpt-*": "gpt-5.4"}},
		Extra:       map[string]any{},
	}
	require.False(t, accountSupportsProjectionModel(account, "gpt-5.unknown"))
}

func TestProjectionModelReachability_ExplicitCapabilityOverridesConservativeDefault(t *testing.T) {
	t.Parallel()

	account := Account{Extra: map[string]any{"openai_capability_explicit_models": []string{"gpt-5.6"}}}
	require.True(t, accountSupportsProjectionModel(account, "gpt-5.6"))
}

func TestNormalizeOpenAIProjectionModelKey_ReusesCompatNormalization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "sys suffix", input: "gpt-5.4-Sys", want: "gpt-5.4"},
		{name: "compat reasoning and sys", input: " gpt-5.4-xhigh-Sys ", want: "gpt-5.4"},
		{name: "codex alias and sys", input: "gpt-5.3-codex-spark-high-Sys", want: "gpt-5.3-codex"},
		{name: "unknown model preserved", input: "gpt-5.6-Sys", want: "gpt-5.6"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, NormalizeOpenAIProjectionModelKey(tt.input))
		})
	}
}
