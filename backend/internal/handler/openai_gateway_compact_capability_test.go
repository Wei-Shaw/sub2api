package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpenAIRequestRequiresCompactCapability(t *testing.T) {
	tests := []struct {
		name          string
		legacyCompact bool
		nativeV2      bool
		platform      string
		want          bool
	}{
		{name: "native_v2_openai", nativeV2: true, platform: service.PlatformOpenAI, want: true},
		{name: "native_v2_deepseek", nativeV2: true, platform: service.PlatformDeepseek, want: false},
		{name: "native_v2_grok", nativeV2: true, platform: service.PlatformGrok, want: false},
		{name: "legacy_deepseek_keeps_existing_behavior", legacyCompact: true, platform: service.PlatformDeepseek, want: true},
		{name: "ordinary_request", platform: service.PlatformOpenAI, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, openAIRequestRequiresCompactCapability(tt.legacyCompact, tt.nativeV2, tt.platform))
		})
	}
}
