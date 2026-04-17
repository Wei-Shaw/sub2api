package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannel_IsWebSearchEmulationEnabled_Enabled(t *testing.T) {
	c := &Channel{
		FeaturesConfig: map[string]any{
			featureKeyWebSearchEmulation: map[string]any{"anthropic": true},
		},
	}
	require.True(t, c.IsWebSearchEmulationEnabled("anthropic"))
}

func TestChannel_IsWebSearchEmulationEnabled_DifferentPlatform(t *testing.T) {
	c := &Channel{
		FeaturesConfig: map[string]any{
			featureKeyWebSearchEmulation: map[string]any{"anthropic": true},
		},
	}
	require.False(t, c.IsWebSearchEmulationEnabled("openai"))
}

func TestChannel_IsWebSearchEmulationEnabled_Disabled(t *testing.T) {
	c := &Channel{
		FeaturesConfig: map[string]any{
			featureKeyWebSearchEmulation: map[string]any{"anthropic": false},
		},
	}
	require.False(t, c.IsWebSearchEmulationEnabled("anthropic"))
}

func TestChannel_IsWebSearchEmulationEnabled_NilFeaturesConfig(t *testing.T) {
	c := &Channel{FeaturesConfig: nil}
	require.False(t, c.IsWebSearchEmulationEnabled("anthropic"))
}

func TestChannel_IsWebSearchEmulationEnabled_NilChannel(t *testing.T) {
	var c *Channel
	require.False(t, c.IsWebSearchEmulationEnabled("anthropic"))
}

func TestChannel_IsWebSearchEmulationEnabled_WrongStructure(t *testing.T) {
	c := &Channel{
		FeaturesConfig: map[string]any{
			featureKeyWebSearchEmulation: true, // not a map
		},
	}
	require.False(t, c.IsWebSearchEmulationEnabled("anthropic"))
}

func TestChannel_IsWebSearchEmulationEnabled_PlatformValueNotBool(t *testing.T) {
	c := &Channel{
		FeaturesConfig: map[string]any{
			featureKeyWebSearchEmulation: map[string]any{"anthropic": "yes"},
		},
	}
	require.False(t, c.IsWebSearchEmulationEnabled("anthropic"))
}
