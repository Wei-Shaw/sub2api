package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func codexReasoningEfforts(t *testing.T, model map[string]any) []string {
	t.Helper()
	raw, ok := model["supported_reasoning_levels"].([]any)
	require.True(t, ok, "supported_reasoning_levels missing: %#v", model)
	efforts := make([]string, 0, len(raw))
	for _, level := range raw {
		efforts = append(efforts, level.(map[string]any)["effort"].(string))
	}
	return efforts
}

func officialGPT61SolEfforts(t *testing.T) ([]string, string) {
	t.Helper()
	descriptor := newConfiguredCodexModelDescriptor("gpt-6.1-sol")
	efforts := make([]string, 0, len(descriptor.SupportedReasoningLevels))
	for _, level := range descriptor.SupportedReasoningLevels {
		efforts = append(efforts, level.Effort)
	}
	require.NotNil(t, descriptor.DefaultReasoningLevel)
	require.NotContains(t, efforts, "none")
	require.Contains(t, efforts, "low")
	return efforts, *descriptor.DefaultReasoningLevel
}

func TestNewConfiguredCodexAliasDescriptorInheritsTargetReasoning(t *testing.T) {
	solEfforts, solDefault := officialGPT61SolEfforts(t)
	tests := []struct {
		name          string
		slug          string
		target        string
		wantEfforts   []string
		wantDefault   string
		wantSlugTitle bool
	}{
		{name: "direct gpt-6.1-sol", slug: "gpt-6.1-sol", target: "", wantEfforts: solEfforts, wantDefault: solDefault},
		{name: "spelling variant", slug: "GPT-6.1-Sol", target: "", wantEfforts: solEfforts, wantDefault: solDefault},
		{name: "effort suffix spelling", slug: "gpt-6.1-sol-high", target: "", wantEfforts: solEfforts, wantDefault: solDefault},
		{name: "mapped alias to gpt-6.1-sol", slug: "team-sol", target: "gpt-6.1-sol", wantEfforts: solEfforts, wantDefault: solDefault, wantSlugTitle: true},
		{name: "unknown model stays conservative", slug: "my-custom-model", target: "", wantEfforts: []string{"none"}, wantDefault: "none"},
		{name: "alias to unknown model stays conservative", slug: "alias-x", target: "vendor-unknown", wantEfforts: []string{"none"}, wantDefault: "none", wantSlugTitle: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			descriptor := newConfiguredCodexAliasDescriptor(tt.slug, tt.target)
			encoded, err := json.Marshal(descriptor)
			require.NoError(t, err)
			var model map[string]any
			require.NoError(t, json.Unmarshal(encoded, &model))
			require.Equal(t, tt.slug, model["slug"])
			require.Equal(t, tt.wantEfforts, codexReasoningEfforts(t, model))
			require.Equal(t, tt.wantDefault, model["default_reasoning_level"])
			if tt.wantSlugTitle {
				require.Equal(t, tt.slug, model["display_name"])
			}
		})
	}
}

func TestMergeConfiguredCodexModelsManifestUsesMappedTargetReasoning(t *testing.T) {
	solEfforts, solDefault := officialGPT61SolEfforts(t)
	body, changed, err := mergeConfiguredCodexModelsManifest(
		[]byte(`{"models":[]}`),
		[]string{"team-sol", "plain-custom"},
		nil,
		false,
		map[string]string{"team-sol": "gpt-6.1-sol"},
	)
	require.NoError(t, err)
	require.True(t, changed)
	models := decodeCodexManifestModels(t, body)
	require.Len(t, models, 2)
	require.Equal(t, "team-sol", models[0]["slug"])
	require.Equal(t, solEfforts, codexReasoningEfforts(t, models[0]))
	require.Equal(t, solDefault, models[0]["default_reasoning_level"])
	require.Equal(t, []string{"none"}, codexReasoningEfforts(t, models[1]))
}

func TestCompleteAPIKeyCodexModelsManifestMetadataUsesMappedTargetReasoning(t *testing.T) {
	solEfforts, solDefault := officialGPT61SolEfforts(t)
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url":      "https://provider.example/v1",
			"model_mapping": map[string]any{"team-sol": "gpt-6.1-sol"},
		},
	}
	body, err := completeAPIKeyCodexModelsManifestMetadata([]byte(`{"models":[
		{"slug":"team-sol"},
		{"slug":"gpt-6.1-sol"},
		{"slug":"plain-custom"}
	]}`), true, account)
	require.NoError(t, err)
	models := decodeCodexManifestModels(t, body)
	require.Len(t, models, 3)
	for _, model := range models[:2] {
		require.Equal(t, solEfforts, codexReasoningEfforts(t, model), model["slug"])
		require.Equal(t, solDefault, model["default_reasoning_level"], model["slug"])
	}
	require.Equal(t, "team-sol", models[0]["slug"])
	require.Equal(t, []string{"none"}, codexReasoningEfforts(t, models[2]))
}
