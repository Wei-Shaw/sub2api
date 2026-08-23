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

func TestVideoAccountSupportsRequestUsesExactAtlasMappingForSeedanceTextToVideo(t *testing.T) {
	service := &GatewayService{}
	const model = "bytedance/seedance-2.5/text-to-video"
	account := &Account{
		Platform: PlatformAtlasCloud,
		Extra:    map[string]any{"video_models_enabled": true},
		Credentials: map[string]any{
			"model_mapping": map[string]any{model: model},
		},
	}

	mappingSupported, whitelistSupported := videoMappingSupportsRequest(account, model)
	require.True(t, mappingSupported)
	require.False(t, whitelistSupported)
	require.True(t, service.videoAccountSupportsRequest(context.Background(), account, model, ""))
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

func TestPrepareVideoRequestPayloadAdaptsAtlasCloudSeedance25ImageToVideo(t *testing.T) {
	original := map[string]any{
		"image_url":         "https://example.com/first.png",
		"aspect_ratio":      " auto ",
		"end_image_url":     "https://example.com/last.png",
		"watermark":         true,
		"output_format":     "webm",
		"return_last_frame": true,
	}
	prepared := prepareVideoRequestPayload(
		&Account{Platform: PlatformAtlasCloud},
		" bytedance/seedance-2.5/image-to-video ",
		original,
	)

	require.Equal(t, "https://example.com/first.png", prepared["image"])
	require.Equal(t, "adaptive", prepared["ratio"])
	require.Equal(t, "https://example.com/last.png", prepared["last_image"])
	require.Equal(t, false, prepared["watermark"])
	require.Equal(t, "mp4", prepared["output_format"])
	require.Equal(t, false, prepared["return_last_frame"])
	require.NotContains(t, prepared, "image_url")
	require.NotContains(t, prepared, "aspect_ratio")
	require.NotContains(t, prepared, "end_image_url")

	// The persisted client request remains in the universal API shape.
	require.Equal(t, "https://example.com/first.png", original["image_url"])
	require.Equal(t, " auto ", original["aspect_ratio"])
	require.Equal(t, "https://example.com/last.png", original["end_image_url"])
	require.Equal(t, true, original["watermark"])
}

func TestPrepareVideoRequestPayloadPreservesExplicitRatioForAtlasCloudSeedance25(t *testing.T) {
	prepared := prepareVideoRequestPayload(
		&Account{Platform: PlatformAtlasCloud},
		"bytedance/seedance-2.5/image-to-video",
		map[string]any{"aspect_ratio": "9:16"},
	)

	require.Equal(t, "9:16", prepared["ratio"])
	require.NotContains(t, prepared, "aspect_ratio")
}

func TestPrepareVideoRequestPayloadAdaptsAtlasCloudSeedance25ReferenceToVideo(t *testing.T) {
	original := map[string]any{
		"image_urls":               []any{"https://example.com/image.png"},
		"audio_urls":               []any{"https://example.com/audio.mp3"},
		"video_urls":               []any{"https://example.com/video.mp4"},
		"omni_reference_task_type": "manual",
	}
	prepared := prepareVideoRequestPayload(
		&Account{Platform: PlatformAtlasCloud},
		"bytedance/seedance-2.5/reference-to-video",
		original,
	)

	require.Equal(t, original["image_urls"], prepared["reference_images"])
	require.Equal(t, original["audio_urls"], prepared["reference_audios"])
	require.Equal(t, original["video_urls"], prepared["reference_videos"])
	require.Equal(t, "auto", prepared["omni_reference_task_type"])
	require.NotContains(t, prepared, "image_urls")
	require.NotContains(t, prepared, "audio_urls")
	require.NotContains(t, prepared, "video_urls")

	// Keep the persisted client payload in the universal API shape.
	require.Contains(t, original, "image_urls")
	require.Contains(t, original, "audio_urls")
	require.Contains(t, original, "video_urls")
	require.Equal(t, "manual", original["omni_reference_task_type"])
}

func TestPrepareVideoRequestPayloadPreservesNonAutoAtlasCloudParams(t *testing.T) {
	tests := []struct {
		input any
		want  any
	}{
		{input: 12, want: 12},
		{input: "12", want: 12},
		{input: " 12.5 ", want: 12.5},
		{input: "not-a-number", want: "not-a-number"},
		{input: nil, want: nil},
	}
	for _, test := range tests {
		original := map[string]any{
			"duration":       test.input,
			"resolution":     "720p",
			"aspect_ratio":   "auto",
			"generate_audio": true,
		}
		prepared := prepareVideoRequestPayload(
			&Account{Platform: PlatformAtlasCloud},
			"mapped-model",
			original,
		)

		require.Equal(t, test.want, prepared["duration"])
		require.Equal(t, test.input, original["duration"])
		require.Equal(t, "720p", prepared["resolution"])
		require.Equal(t, "auto", prepared["aspect_ratio"])
		require.Equal(t, true, prepared["generate_audio"])
	}
}
