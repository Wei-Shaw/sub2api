package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeWebSearchEmulationDimensions_MissingDimensionsInherited(t *testing.T) {
	existing := map[string]any{
		featureKeyWebSearchEmulation: map[string]any{"openai": true, "command_code": true},
	}
	incoming := map[string]any{
		featureKeyWebSearchEmulation: map[string]any{"anthropic": false},
	}
	merged := mergeWebSearchEmulationDimensions(existing, incoming)

	dims, ok := merged[featureKeyWebSearchEmulation].(map[string]any)
	require.True(t, ok)
	require.Equal(t, false, dims["anthropic"])
	require.Equal(t, true, dims["openai"])
	require.Equal(t, true, dims["command_code"])
}

func TestMergeWebSearchEmulationDimensions_RequestValueWins(t *testing.T) {
	existing := map[string]any{
		featureKeyWebSearchEmulation: map[string]any{"openai": true},
	}
	incoming := map[string]any{
		featureKeyWebSearchEmulation: map[string]any{"openai": false},
	}
	merged := mergeWebSearchEmulationDimensions(existing, incoming)

	dims, ok := merged[featureKeyWebSearchEmulation].(map[string]any)
	require.True(t, ok)
	require.Equal(t, false, dims["openai"])
}

func TestMergeWebSearchEmulationDimensions_KeyAbsentInheritsWholeKey(t *testing.T) {
	existing := map[string]any{
		featureKeyWebSearchEmulation: map[string]any{"openai": true},
		"bedrock_cc_compat":          map[string]any{"anthropic": true},
	}
	incoming := map[string]any{"other": 2}
	merged := mergeWebSearchEmulationDimensions(existing, incoming)

	dims, ok := merged[featureKeyWebSearchEmulation].(map[string]any)
	require.True(t, ok, "clients that do not send the key must not erase it")
	require.Equal(t, true, dims["openai"])
	require.Equal(t, 2, merged["other"])

	// The inherited value must be a copy: mutating the result must not modify
	// the previous configuration.
	dims["openai"] = false
	existingDims, ok := existing[featureKeyWebSearchEmulation].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, existingDims["openai"])
}

func TestMergeWebSearchEmulationDimensions_KeyAbsentWithoutExistingIsNoop(t *testing.T) {
	merged := mergeWebSearchEmulationDimensions(map[string]any{}, map[string]any{"other": 3})
	_, ok := merged[featureKeyWebSearchEmulation]
	require.False(t, ok)
	require.Equal(t, 3, merged["other"])
}

func TestMergeWebSearchEmulationDimensions_EmptyObjectMeansClear(t *testing.T) {
	existing := map[string]any{
		featureKeyWebSearchEmulation: map[string]any{"openai": true},
	}
	merged := mergeWebSearchEmulationDimensions(existing, map[string]any{
		featureKeyWebSearchEmulation: map[string]any{},
	})
	dims, ok := merged[featureKeyWebSearchEmulation].(map[string]any)
	require.True(t, ok)
	require.Empty(t, dims, "an explicit empty object must clear the value")
}

func TestMergeWebSearchEmulationDimensions_NilIncoming(t *testing.T) {
	require.Nil(t, mergeWebSearchEmulationDimensions(map[string]any{featureKeyWebSearchEmulation: map[string]any{"openai": true}}, nil))
}

// Wiring-level: the merge must be applied by applyUpdateInput.
func TestChannelService_ApplyUpdateInputMergesWebSearchEmulationDimensions(t *testing.T) {
	svc := &ChannelService{}
	channel := &Channel{
		FeaturesConfig: map[string]any{
			featureKeyWebSearchEmulation: map[string]any{"anthropic": false, "openai": true, "command_code": true},
		},
	}

	require.NoError(t, svc.applyUpdateInput(context.Background(), channel, &UpdateChannelInput{
		FeaturesConfig: map[string]any{
			featureKeyWebSearchEmulation: map[string]any{"anthropic": false},
		},
	}))

	dims, ok := channel.FeaturesConfig[featureKeyWebSearchEmulation].(map[string]any)
	require.True(t, ok)
	require.Equal(t, false, dims["anthropic"])
	require.Equal(t, true, dims["openai"])
	require.Equal(t, true, dims["command_code"])
}

func TestChannelService_ApplyUpdateInputInheritsWebSearchEmulationWhenKeyAbsent(t *testing.T) {
	svc := &ChannelService{}
	channel := &Channel{
		FeaturesConfig: map[string]any{
			featureKeyWebSearchEmulation: map[string]any{"openai": true, "command_code": true},
			"bedrock_cc_compat":          map[string]any{"anthropic": true},
		},
	}

	require.NoError(t, svc.applyUpdateInput(context.Background(), channel, &UpdateChannelInput{
		FeaturesConfig: map[string]any{
			"bedrock_cc_compat": map[string]any{"anthropic": false},
		},
	}))

	dims, ok := channel.FeaturesConfig[featureKeyWebSearchEmulation].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, dims["openai"])
	require.Equal(t, true, dims["command_code"])

	// Other keys keep the historical whole-block replacement semantics.
	bedrock, ok := channel.FeaturesConfig["bedrock_cc_compat"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, false, bedrock["anthropic"])
}
