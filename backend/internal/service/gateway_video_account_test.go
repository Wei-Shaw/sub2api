//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVideoAccountSupportsRequestRejectsAccountWhenVideoDisabled(t *testing.T) {
	service := &GatewayService{}
	for _, platform := range []string{PlatformFal, PlatformAtlasCloud, PlatformApiz} {
		t.Run(platform, func(t *testing.T) {
			account := &Account{
				Platform: platform,
				Extra:    map[string]any{},
				Credentials: map[string]any{
					"model_mapping": map[string]any{
						"bytedance/seedance-2.5/text-to-video": "upstream-model",
					},
				},
			}

			require.False(t, service.videoAccountSupportsRequest(
				context.Background(),
				account,
				"bytedance/seedance-2.5/text-to-video",
				"",
			))
		})
	}
}

func TestVideoAccountSupportsRequestUsesMappingKeysAndPlatformModelWhitelist(t *testing.T) {
	service := &GatewayService{}
	const (
		publicModel = "bytedance/seedance-2.5/reference-to-video"
		atlasModel  = "bytedance/seedance-2.5/image-to-video"
		apizModel   = "clawsea/seedance2.0"
	)

	tests := []struct {
		name      string
		platform  string
		requested string
		want      bool
	}{
		{name: "atlascloud public mapping key", platform: PlatformAtlasCloud, requested: publicModel, want: true},
		{name: "atlascloud platform whitelist", platform: PlatformAtlasCloud, requested: atlasModel, want: true},
		{name: "apiz public mapping key", platform: PlatformApiz, requested: publicModel, want: true},
		{name: "apiz platform whitelist", platform: PlatformApiz, requested: apizModel, want: true},
		{name: "fal public mapping key", platform: PlatformFal, requested: publicModel, want: true},
		{name: "fal endpoint whitelist value", platform: PlatformFal, requested: atlasModel, want: true},
		{name: "atlascloud unknown model", platform: PlatformAtlasCloud, requested: "vendor/unknown/video", want: false},
		{name: "apiz unknown model", platform: PlatformApiz, requested: "vendor/unknown/video", want: false},
		{name: "fal unknown model", platform: PlatformFal, requested: "vendor/unknown/video", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			account := &Account{
				Platform: test.platform,
				Extra:    map[string]any{"video_models_enabled": true},
				Credentials: map[string]any{
					"model_mapping": map[string]any{publicModel: atlasModel},
				},
			}

			require.Equal(t, test.want, service.videoAccountSupportsRequest(
				context.Background(), account, test.requested, "",
			))
		})
	}
}

func TestVideoAccountSupportsRequestUsesAtlasWhitelistWhenMappingDoesNotMatch(t *testing.T) {
	service := &GatewayService{}
	account := &Account{
		Platform: PlatformAtlasCloud,
		Extra:    map[string]any{"video_models_enabled": true},
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"bytedance/seedance-2.5/reference-to-video": "atlas-reference-model",
			},
		},
	}

	mappingSupported, whitelistSupported := videoMappingSupportsRequest(
		account, "bytedance/seedance-2.5/image-to-video",
	)
	require.False(t, mappingSupported)
	require.True(t, whitelistSupported)
	require.True(t, service.videoAccountSupportsRequest(
		context.Background(), account, "bytedance/seedance-2.5/image-to-video", "",
	))
}

func TestVideoAccountSupportsRequestRejectsUnknownModelWithoutMappingOrWhitelist(t *testing.T) {
	service := &GatewayService{}
	for _, platform := range []string{PlatformAtlasCloud, PlatformApiz} {
		t.Run(platform, func(t *testing.T) {
			account := &Account{
				Platform: platform,
				Extra:    map[string]any{"video_models_enabled": true},
			}
			require.False(t, service.videoAccountSupportsRequest(
				context.Background(), account, "vendor/unknown/video", "",
			))
		})
	}
}

func TestVideoAccountSupportsRequestDoesNotTreatAtlasMappingValueAsWhitelist(t *testing.T) {
	service := &GatewayService{}
	account := &Account{
		Platform: PlatformAtlasCloud,
		Extra:    map[string]any{"video_models_enabled": true},
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"public/video-model": "vendor/internal/video-model",
			},
		},
	}
	require.False(t, service.videoAccountSupportsRequest(
		context.Background(), account, "vendor/internal/video-model", "",
	))
}

func TestPrepareVideoRequestPayloadAddsAtlasCloudModel(t *testing.T) {
	original := map[string]any{"prompt": "test", "duration": " auto "}
	prepared := prepareVideoRequestPayload(
		&Account{Platform: PlatformAtlasCloud},
		"bytedance/seedance-2.5/image-to-video",
		original,
	)

	require.Equal(t, "bytedance/seedance-2.5/image-to-video", prepared["model"])
	require.Equal(t, -1, prepared["duration"])
	require.Equal(t, " auto ", original["duration"])
	require.NotContains(t, original, "model")
}

func TestPrepareVideoRequestPayloadOverridesAtlasCloudModel(t *testing.T) {
	original := map[string]any{"model": "explicit-model"}
	prepared := prepareVideoRequestPayload(
		&Account{Platform: PlatformAtlasCloud},
		"mapped-model",
		original,
	)

	require.Equal(t, "mapped-model", prepared["model"])
	require.Equal(t, "explicit-model", original["model"])
}

func TestPrepareVideoRequestPayloadPreservesNonAutoAtlasCloudParams(t *testing.T) {
	for _, duration := range []any{12, "12", nil} {
		original := map[string]any{
			"duration":       duration,
			"resolution":     "720p",
			"aspect_ratio":   "auto",
			"generate_audio": true,
		}
		prepared := prepareVideoRequestPayload(
			&Account{Platform: PlatformAtlasCloud},
			"mapped-model",
			original,
		)

		require.Equal(t, duration, prepared["duration"])
		require.Equal(t, "720p", prepared["resolution"])
		require.Equal(t, "auto", prepared["aspect_ratio"])
		require.Equal(t, true, prepared["generate_audio"])
	}
}
