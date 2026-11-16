package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFastManifestAndRuntimeUseAccountCapabilities(t *testing.T) {
	for _, model := range []string{"gpt-6-astra", "gpt-6-luna", "gpt-5.5"} {
		for _, testCase := range []struct {
			name    string
			baseURL string
			tiers   json.RawMessage
			want    bool
		}{
			{name: "missing custom", baseURL: "https://relay.example/v1"},
			{name: "empty custom", baseURL: "https://relay.example/v1", tiers: json.RawMessage(`[]`)},
			{name: "null custom", baseURL: "https://relay.example/v1", tiers: json.RawMessage(`null`)},
			{name: "declared custom", baseURL: "https://relay.example/v1", tiers: json.RawMessage(`[{"id":"priority"}]`), want: true},
			{name: "empty official", baseURL: "https://api.openai.com/v1", tiers: json.RawMessage(`[]`)},
			{name: "missing official", baseURL: "https://api.openai.com/v1", want: model == "gpt-6-astra"},
		} {
			t.Run(model+"/"+testCase.name, func(t *testing.T) {
				account := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
					Credentials: map[string]any{"base_url": testCase.baseURL, "model_mapping": map[string]any{model: model}}}
				if testCase.tiers != nil {
					account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
						model: {CodexToolCapabilities: map[string]json.RawMessage{"service_tiers": testCase.tiers}},
					}})
				}
				body, err := buildCodexModelsManifestForAccounts(PlatformOpenAI, []string{model}, []Account{account}, nil, nil, true)
				require.NoError(t, err)
				models := decodeCodexManifestModels(t, body)
				require.Len(t, models, 1)
				advertised := false
				tiers, ok := models[0]["service_tiers"].([]any)
				require.True(t, ok)
				for _, tier := range tiers {
					declaration, ok := tier.(map[string]any)
					require.True(t, ok)
					advertised = advertised || declaration["id"] == OpenAIFastTierPriority
				}
				require.Equal(t, testCase.want, advertised, "manifest must respect the target declaration")
				require.Equal(t, advertised, accountSupportsOpenAIServiceTier(&account, model, OpenAIFastTierPriority))
				if !advertised {
					require.Empty(t, models[0]["default_service_tier"])
				}
			})
		}
	}
}

func TestFastManifestPreservesUpstreamSpeedTierDeclaration(t *testing.T) {
	account := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{
			"base_url":      "https://chatgpt.com/backend-api/codex",
			"model_mapping": map[string]any{"gpt-6-astra": "gpt-6-astra"},
		}}
	account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
		"gpt-6-astra": {CodexToolCapabilities: map[string]json.RawMessage{
			"service_tiers":          json.RawMessage("[{\"id\":\"priority\",\"name\":\"Fast\",\"description\":\"2x speed\"}]"),
			"additional_speed_tiers": json.RawMessage("[\"fast\"]"),
		}},
	}})

	body, err := buildCodexModelsManifestForAccounts(PlatformOpenAI, []string{"gpt-6-astra"}, []Account{account}, nil, nil, true)
	require.NoError(t, err)
	model := decodeCodexManifestModels(t, body)[0]
	require.Equal(t, []any{
		map[string]any{"id": "priority", "name": "Fast", "description": "2x speed"},
	}, model["service_tiers"])
	require.Equal(t, []any{"fast"}, model["additional_speed_tiers"])
}

func TestFastManifestKeepsConflictingAccountCapabilitiesDisabled(t *testing.T) {
	accounts := []Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://relay.example/v1"}},
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://relay.example/v1"}},
	}
	accounts[0].SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
		"gpt-6-astra": {CodexToolCapabilities: map[string]json.RawMessage{"service_tiers": json.RawMessage(`[{"id":"priority"}]`)}},
	}})
	body, err := buildCodexModelsManifestForAccounts(PlatformOpenAI, []string{"gpt-6-astra"}, accounts, nil, nil, true)
	require.NoError(t, err)
	models := decodeCodexManifestModels(t, body)
	require.Empty(t, models[0]["service_tiers"])
}
