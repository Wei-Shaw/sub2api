package xai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultModelMappingExcludesCrossClientWildcards(t *testing.T) {
	original := RuntimeModelMappingOptions()
	t.Cleanup(func() { SetRuntimeModelMappingOptions(original) })
	SetRuntimeModelMappingOptions(ModelMappingOptions{})
	mapping := DefaultModelMapping()

	require.Equal(t, "grok-4.5", mapping["grok"])
	require.Equal(t, "grok-4.5", mapping["grok-latest"])
	require.Equal(t, "grok-build-0.1", mapping["grok-build"])
	require.Equal(t, DefaultTextModel, mapping["grok-build-latest"])
	require.Equal(t, DefaultImagineImageQualityModel, mapping["grok-imagine-edit"])
	require.Equal(t, DefaultImagineVideo15LegacyModel, mapping["grok-imagine-video-1.5"])
	require.Equal(t, DefaultImagineVideo15Model, mapping["grok-imagine-video-1.5-preview"])
	require.Equal(t, "grok-4.5", mapping["xai/grok"])

	// Cross-vendor wildcards must stay opt-in.
	_, hasGPT := mapping["gpt-*"]
	_, hasClaude := mapping["claude-*"]
	require.False(t, hasGPT)
	require.False(t, hasClaude)
}

func TestModelMappingWithOptionsCrossClient(t *testing.T) {
	t.Parallel()
	mapping := ModelMappingWithOptions(ModelMappingOptions{
		DefaultText:          "grok-4.3",
		EnableCrossClientMap: true,
	})
	require.Equal(t, "grok-4.3", mapping["grok"])
	require.Equal(t, "grok-4.3", mapping["gpt-*"])
	require.Equal(t, "grok-4.3", mapping["claude-*"])
	require.Equal(t, "grok-4.3", mapping["codex-*"])
}

func TestCanonicalImagineVideoModel(t *testing.T) {
	t.Parallel()
	require.Equal(t, DefaultImagineVideoModel, CanonicalImagineVideoModel("grok-imagine-video"))
	require.Equal(t, DefaultImagineVideo15Model, CanonicalImagineVideoModel("grok-imagine-video-1.5"))
	require.Equal(t, DefaultImagineVideo15Model, CanonicalImagineVideoModel("grok-imagine-video-1.5-preview"))
	require.Equal(t, DefaultImagineVideo15Model, CanonicalImagineVideoModel("xai/grok-video-1.5"))
	require.Equal(t, "grok-imagine-video-2", CanonicalImagineVideoModel("grok-imagine-video-2"))
}

func TestIsGrokModelID(t *testing.T) {
	t.Parallel()
	require.True(t, IsGrokModelID("grok-4.5"))
	require.True(t, IsGrokModelID("grok-4.6"))
	require.True(t, IsGrokModelID("x-ai/grok-4.3"))
	require.False(t, IsGrokModelID("gpt-5"))
	require.False(t, IsGrokModelID("claude-sonnet-4"))
}

func TestDefaultModelsIncludesGrok46(t *testing.T) {
	t.Parallel()
	ids := DefaultModelIDs()
	require.Contains(t, ids, "grok-4.6")
	require.Equal(t, "grok-4.6", ResolveGrokTextResponsesModelID("grok-4.6"))
	require.Equal(t, "grok-4.6", ResolveGrokTextResponsesModelID("grok-4.6-latest"))
}

func TestResolveGrokTextResponsesModelID(t *testing.T) {
	t.Parallel()
	require.Equal(t, "grok-4.5", ResolveGrokTextResponsesModelID(""))
	require.Equal(t, "grok-4.3", ResolveGrokTextResponsesModelID("grok", "grok-4.3"))
	require.Equal(t, "grok-4.20-multi-agent-0309", ResolveGrokTextResponsesModelID("grok-4.20-multi-agent"))
}
