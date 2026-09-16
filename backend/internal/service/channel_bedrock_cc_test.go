package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannel_IsBedrockCCCompatEnabled_Enabled(t *testing.T) {
	c := &Channel{
		FeaturesConfig: map[string]any{
			featureKeyBedrockCCCompat: true,
		},
	}
	require.True(t, c.IsBedrockCCCompatEnabled("bedrock"))
}

func TestChannel_IsBedrockCCCompatEnabled_AppliesToAllPlatforms(t *testing.T) {
	c := &Channel{
		FeaturesConfig: map[string]any{
			featureKeyBedrockCCCompat: true,
		},
	}
	require.True(t, c.IsBedrockCCCompatEnabled("anthropic"))
	require.True(t, c.IsBedrockCCCompatEnabled("openai"))
	require.True(t, c.IsBedrockCCCompatEnabled(""))
}

func TestChannel_IsBedrockCCCompatEnabled_Disabled(t *testing.T) {
	c := &Channel{
		FeaturesConfig: map[string]any{
			featureKeyBedrockCCCompat: false,
		},
	}
	require.False(t, c.IsBedrockCCCompatEnabled("bedrock"))
}

func TestChannel_IsBedrockCCCompatEnabled_NilFeaturesConfig(t *testing.T) {
	c := &Channel{FeaturesConfig: nil}
	require.False(t, c.IsBedrockCCCompatEnabled("bedrock"))
}

func TestChannel_IsBedrockCCCompatEnabled_NilChannel(t *testing.T) {
	var c *Channel
	require.False(t, c.IsBedrockCCCompatEnabled("bedrock"))
}

func TestChannel_IsBedrockCCCompatEnabled_WrongType(t *testing.T) {
	c := &Channel{
		FeaturesConfig: map[string]any{
			featureKeyBedrockCCCompat: "yes",
		},
	}
	require.False(t, c.IsBedrockCCCompatEnabled("bedrock"))
}

func TestChannel_IsBedrockCCCompatEnabled_OldMapFormat(t *testing.T) {
	c := &Channel{
		FeaturesConfig: map[string]any{
			featureKeyBedrockCCCompat: map[string]any{"bedrock": true},
		},
	}
	require.False(t, c.IsBedrockCCCompatEnabled("bedrock"))
}

func TestChannel_IsBedrockCCCompatEnabled_MissingKey(t *testing.T) {
	c := &Channel{
		FeaturesConfig: map[string]any{
			"other_feature": true,
		},
	}
	require.False(t, c.IsBedrockCCCompatEnabled("bedrock"))
}
