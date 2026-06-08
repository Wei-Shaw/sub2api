package xai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseModelList_NormalizesOpenAIStyleResponse(t *testing.T) {
	models, err := ParseModelList([]byte(`{
		"object": "list",
		"data": [
			{"id": "grok-4.3-fast", "object": "model", "created": 1735689600, "owned_by": "xai", "display_name": "Grok 4.3 Fast"},
			{"id": "  ", "object": "model"},
			{"id": "grok-4.3-fast", "object": "model"},
			{"id": "grok-code-fast-1", "name": "Grok Code Fast 1"}
		]
	}`))

	require.NoError(t, err)
	require.Equal(t, []Model{
		{
			ID:          "grok-4.3-fast",
			Object:      "model",
			Created:     1735689600,
			OwnedBy:     "xai",
			Type:        "model",
			DisplayName: "Grok 4.3 Fast",
			Name:        "Grok 4.3 Fast",
		},
		{
			ID:          "grok-code-fast-1",
			Object:      "model",
			OwnedBy:     "xai",
			Type:        "model",
			DisplayName: "Grok Code Fast 1",
			Name:        "Grok Code Fast 1",
		},
	}, models)
}

func TestModelsFromIDsUsesDefaultsAndCustomFallbacks(t *testing.T) {
	models := ModelsFromIDs([]string{"custom-grok", "grok-4.3-fast", "custom-grok", ""})

	require.Len(t, models, 2)
	require.Equal(t, "custom-grok", models[0].ID)
	require.Equal(t, "custom-grok", models[0].DisplayName)
	require.Equal(t, "grok-4.3-fast", models[1].ID)
	require.Equal(t, "Grok 4.3 Fast", models[1].DisplayName)
}

func TestDefaultModelsIncludeXAIMediaModels(t *testing.T) {
	require.Subset(t, DefaultModelIDs(), []string{
		"grok-imagine-image",
		"grok-imagine-image-quality",
		"grok-imagine-video",
		"grok-imagine-video-1.5-preview",
	})
}
