//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestValidateImagePricingMatrix_AcceptsEmpty(t *testing.T) {
	require.NoError(t, validateImagePricingMatrix(nil))
	require.NoError(t, validateImagePricingMatrix(domain.ImagePricingMatrix{}))
}

func TestValidateImagePricingMatrix_AcceptsValidCells(t *testing.T) {
	matrix := domain.ImagePricingMatrix{
		ImagePricingTier1024x1024: {
			ImageQualityLow:    0,
			ImageQualityMedium: 0.053,
			ImageQualityHigh:   0.211,
		},
		ImagePricingTier3840x2160: {
			ImageQualityHigh: 0.401,
		},
	}
	require.NoError(t, validateImagePricingMatrix(matrix))
}

func TestValidateImagePricingMatrix_RejectsUnknownTier(t *testing.T) {
	matrix := domain.ImagePricingMatrix{
		"9999x9999": {ImageQualityHigh: 0.1},
	}
	err := validateImagePricingMatrix(matrix)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown tier")
}

func TestValidateImagePricingMatrix_RejectsUnknownQuality(t *testing.T) {
	matrix := domain.ImagePricingMatrix{
		ImagePricingTier1024x1024: {"ultra": 0.5},
	}
	err := validateImagePricingMatrix(matrix)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown quality")
}

func TestValidateImagePricingMatrix_RejectsNegativePrice(t *testing.T) {
	matrix := domain.ImagePricingMatrix{
		ImagePricingTier1024x1024: {ImageQualityHigh: -0.001},
	}
	err := validateImagePricingMatrix(matrix)
	require.Error(t, err)
	require.Contains(t, err.Error(), ">= 0")
}

func TestValidateImagePricingMatrix_RejectsExceedsUpperBound(t *testing.T) {
	matrix := domain.ImagePricingMatrix{
		ImagePricingTier1024x1024: {ImageQualityHigh: imagePricingMatrixCellMaxUSD + 0.01},
	}
	err := validateImagePricingMatrix(matrix)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds upper bound")
}

func TestNormalizeImagePricingMatrix_DropsEmptyRow(t *testing.T) {
	matrix := domain.ImagePricingMatrix{
		ImagePricingTier1024x1024: {ImageQualityHigh: 0.211},
		ImagePricingTier3840x2160: {}, // empty row 应被丢弃
	}
	out := normalizeImagePricingMatrix(matrix)
	require.Len(t, out, 1)
	_, ok := out[ImagePricingTier3840x2160]
	require.False(t, ok)
}

func TestNormalizeImagePricingMatrix_NilOrAllEmptyReturnsNil(t *testing.T) {
	require.Nil(t, normalizeImagePricingMatrix(nil))
	require.Nil(t, normalizeImagePricingMatrix(domain.ImagePricingMatrix{}))
	require.Nil(t, normalizeImagePricingMatrix(domain.ImagePricingMatrix{
		ImagePricingTier1024x1024: {},
	}))
}

func TestGroup_ImagePreferFalEnabled(t *testing.T) {
	t.Run("openai+true", func(t *testing.T) {
		g := &Group{Platform: PlatformOpenAI, ImagePreferFal: true}
		require.True(t, g.ImagePreferFalEnabled())
	})
	t.Run("openai+false", func(t *testing.T) {
		g := &Group{Platform: PlatformOpenAI, ImagePreferFal: false}
		require.False(t, g.ImagePreferFalEnabled())
	})
	t.Run("non-openai+true is ignored", func(t *testing.T) {
		g := &Group{Platform: PlatformAnthropic, ImagePreferFal: true}
		require.False(t, g.ImagePreferFalEnabled())
	})
	t.Run("nil group", func(t *testing.T) {
		var g *Group
		require.False(t, g.ImagePreferFalEnabled())
	})
}

func TestGroup_ImageDecodeSizeOnRspEnabled(t *testing.T) {
	t.Run("openai+true", func(t *testing.T) {
		g := &Group{Platform: PlatformOpenAI, ImageDecodeSizeOnRsp: true}
		require.True(t, g.ImageDecodeSizeOnRspEnabled())
	})
	t.Run("openai+false (default)", func(t *testing.T) {
		g := &Group{Platform: PlatformOpenAI}
		require.False(t, g.ImageDecodeSizeOnRspEnabled())
	})
	t.Run("non-openai+true is ignored", func(t *testing.T) {
		g := &Group{Platform: PlatformAnthropic, ImageDecodeSizeOnRsp: true}
		require.False(t, g.ImageDecodeSizeOnRspEnabled())
	})
	t.Run("nil group", func(t *testing.T) {
		var g *Group
		require.False(t, g.ImageDecodeSizeOnRspEnabled())
	})
	t.Run("coexists with ImagePreferFal independently", func(t *testing.T) {
		g := &Group{Platform: PlatformOpenAI, ImagePreferFal: true, ImageDecodeSizeOnRsp: true}
		require.True(t, g.ImagePreferFalEnabled())
		require.True(t, g.ImageDecodeSizeOnRspEnabled())
	})
}

func TestGroup_BuildImagePriceConfig_PassesThrough(t *testing.T) {
	matrix := domain.ImagePricingMatrix{
		ImagePricingTier1024x1024: {ImageQualityHigh: 0.211},
	}
	p1k := 0.01
	g := &Group{
		ImagePrice1K:       &p1k,
		ImagePricingMatrix: matrix,
	}
	cfg := g.BuildImagePriceConfig(1024, 1024, "high")
	require.NotNil(t, cfg)
	require.Equal(t, &p1k, cfg.Price1K)
	require.Equal(t, matrix, cfg.PricingMatrix)
	require.Equal(t, 1024, cfg.RawWidth)
	require.Equal(t, 1024, cfg.RawHeight)
	require.Equal(t, "high", cfg.Quality)
}

func TestGroup_BuildImagePriceConfig_NilGroup(t *testing.T) {
	var g *Group
	require.Nil(t, g.BuildImagePriceConfig(0, 0, ""))
}
