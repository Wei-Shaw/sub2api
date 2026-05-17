//go:build unit

package service

import "testing"

import "github.com/stretchr/testify/require"

func TestNormalizeGeminiImageSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty", raw: "", want: ""},
		{name: "512 literal", raw: "512", want: "1K"},
		{name: "1k literal", raw: "1k", want: "1K"},
		{name: "2k literal", raw: "2K", want: "2K"},
		{name: "4k literal", raw: "4k", want: "4K"},
		{name: "21:9 1k resolution", raw: "1584x672", want: "1K"},
		{name: "21:9 2k resolution", raw: "3168x1344", want: "2K"},
		{name: "21:9 4k resolution", raw: "6336x2688", want: "4K"},
		{name: "1:8 1k resolution", raw: "384x3072", want: "1K"},
		{name: "4:1 1k resolution", raw: "2048x512", want: "1K"},
		{name: "unicode separator", raw: "1792×2400", want: "2K"},
		{name: "invalid", raw: "panorama", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, normalizeGeminiImageSize(tt.raw))
		})
	}
}

func TestNormalizeGeminiImageSizeOrDefault(t *testing.T) {
	t.Parallel()

	require.Equal(t, "1K", normalizeGeminiImageSizeOrDefault(""))
	require.Equal(t, "1K", normalizeGeminiImageSizeOrDefault("invalid"))
	require.Equal(t, "1K", normalizeGeminiImageSizeOrDefault("512"))
	require.Equal(t, "2K", normalizeGeminiImageSizeOrDefault("3168x1344"))
}

func TestGeminiImageSizeExtraction_UsesNormalizedBillingTier(t *testing.T) {
	t.Parallel()

	antigravitySvc := &AntigravityGatewayService{}
	compatSvc := &GeminiMessagesCompatService{}

	require.Equal(t, "1K", antigravitySvc.extractImageSize([]byte(`{"generationConfig":{"imageConfig":{"imageSize":"512"}}}`)))
	require.Equal(t, "1K", compatSvc.extractImageSize([]byte(`{"generationConfig":{"imageConfig":{"imageSize":"1584x672"}}}`)))
	require.Equal(t, "2K", antigravitySvc.extractImageSize([]byte(`{"generationConfig":{"imageConfig":{"imageSize":"3168x1344"}}}`)))
	require.Equal(t, "4K", compatSvc.extractImageSize([]byte(`{"generationConfig":{"imageConfig":{"imageSize":"6336x2688"}}}`)))
	require.Equal(t, "1K", antigravitySvc.extractImageSize([]byte(`{"generationConfig":{"imageConfig":{"aspectRatio":"21:9"}}}`)))
	require.Equal(t, "1K", compatSvc.extractImageSize([]byte(`{"contents":[]}`)))
}
