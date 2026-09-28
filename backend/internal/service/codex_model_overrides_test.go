package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func modelOverrideFields(t *testing.T, body string) map[string]json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(body), &fields))
	return fields
}

func TestCodexModelOverridesPreserveNestedFieldsAndMembership(t *testing.T) {
	body := []byte(`{"future_envelope":true,"models":[{"slug":"MiniMax-M3","supports_parallel_tool_calls":true,"model_messages":{"instructions_template":"original","permissions":{"allow":true}},"future_field":"retained"}]}`)
	group := &Group{CodexModelsManifestConfig: GroupCodexModelsManifestConfig{ModelOverrides: map[string]map[string]json.RawMessage{
		"MiniMax-M3":  modelOverrideFields(t, `{"supports_parallel_tool_calls":false,"priority":0,"experimental_supported_tools":[],"model_messages":{"permissions":null},"context_window":1000000,"max_context_window":1000000}`),
		"unavailable": modelOverrideFields(t, `{"display_name":"must not appear"}`),
	}}}
	require.NoError(t, ValidateCodexModelOverrides(group.CodexModelsManifestConfig.ModelOverrides))
	result, err := ApplyGroupCodexModelOverrides(body, group)
	require.NoError(t, err)
	require.JSONEq(t, `{"future_envelope":true,"models":[{"slug":"MiniMax-M3","supports_parallel_tool_calls":false,"priority":0,"experimental_supported_tools":[],"model_messages":{"instructions_template":"original","permissions":null},"future_field":"retained","context_window":1000000,"max_context_window":1000000}]}`, string(result))
	original, err := ApplyGroupCodexModelOverrides(body, &Group{})
	require.NoError(t, err)
	require.Equal(t, body, original)
}

func TestCodexModelOverrideValidation(t *testing.T) {
	for _, body := range []string{
		`{"slug":"other"}`, `{"id":"other"}`, `{"context_window":"big"}`, `{"context_window":0}`,
		`{"context_window":1000,"max_context_window":900}`, `{"input_modalities":null}`,
		`{"model_messages":{"instructions_template":null}}`, `{"supports_search_tool":null}`,
		`{"default_reasoning_level":"high","supported_reasoning_levels":[{"effort":"low"}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			require.Error(t, ValidateCodexModelOverrides(map[string]map[string]json.RawMessage{"test": modelOverrideFields(t, body)}))
		})
	}
	require.NoError(t, ValidateCodexModelOverrides(map[string]map[string]json.RawMessage{"test": modelOverrideFields(t, `{"context_window":1000000,"default_reasoning_level":null,"future_extension":{"enabled":false}}`)}))
	base := modelOverrideFields(t, `{"slug":"test","context_window":100,"max_context_window":200}`)
	require.Error(t, ValidateEffectiveCodexModelOverride("test", base, modelOverrideFields(t, `{"context_window":300}`)))
}

type modelOverrideRepoStub struct{ AccountRepository }

func TestCodexModelOverridesETagAndPlatformNormalization(t *testing.T) {
	fields := map[string]map[string]json.RawMessage{"test": modelOverrideFields(t, `{"description":"custom"}`)}
	group := &Group{Platform: PlatformOpenAI, CodexModelsManifestConfig: GroupCodexModelsManifestConfig{Enabled: true, ModelOverrides: fields}}
	s := &OpenAIGatewayService{accountRepo: &modelOverrideRepoStub{}}
	manifest := &OpenAIModelsResponse{Body: []byte(`{"models":[{"slug":"test"}]}`), ETag: `"old"`}
	require.NoError(t, s.MergeGroupConfiguredCodexModels(context.Background(), group, manifest, `"old"`))
	require.False(t, manifest.NotModified)
	require.NotEqual(t, `"old"`, manifest.ETag)
	copy := *manifest
	require.NoError(t, s.MergeGroupConfiguredCodexModels(context.Background(), group, &copy, manifest.ETag))
	require.True(t, copy.NotModified)
	require.Empty(t, copy.Body)
	for _, platform := range []string{PlatformMiniMax, PlatformComposite} {
		cfg := normalizeCodexModelsManifestConfig(platform, group.CodexModelsManifestConfig)
		require.False(t, cfg.Enabled)
		require.Equal(t, fields, cfg.ModelOverrides)
	}
}

func TestModelConfigRegistrySearchAndPartialMetadata(t *testing.T) {
	reasoning := true
	s := &AccountTestService{modelMetadataRegistryAt: time.Now(), modelMetadataRegistry: map[string]modelsDevProvider{
		"minimax": {Models: map[string]modelsDevModel{"MiniMax-M3": {Name: "MiniMax M3", Reasoning: &reasoning, Limit: modelsDevLimit{Context: 1000000}, Modalities: modelsDevModalities{Input: []string{"text", "image"}}}}},
	}}
	results, err := s.SearchModelConfigs(context.Background(), "minimax-m3")
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "minimax", results[0].Provider)
	require.JSONEq(t, `1000000`, string(results[0].Fields["context_window"]))
	require.NotContains(t, results[0].Fields, "default_reasoning_level", "reasoning support must not fabricate adjustable levels")
	require.NotContains(t, results[0].Fields, "slug")
	require.NoError(t, ValidateCodexModelOverrides(map[string]map[string]json.RawMessage{"MiniMax-M3": results[0].Fields}))
}
