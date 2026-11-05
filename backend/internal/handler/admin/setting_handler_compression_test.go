package admin

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUpdateSettingsCompressionLevels(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeyCompressionBrotliLevel:  "5",
		service.SettingKeyCompressionZstdLevel:    "3",
		service.SettingKeyCompressionGzipLevel:    "6",
		service.SettingKeyCompressionDeflateLevel: "6",
	})

	rec := doUpdateSettings(t, h, map[string]any{
		"compression_brotli_level":  8,
		"compression_zstd_level":    10,
		"compression_gzip_level":    4,
		"compression_deflate_level": 2,
	}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "8", repo.values[service.SettingKeyCompressionBrotliLevel])
	require.Equal(t, "10", repo.values[service.SettingKeyCompressionZstdLevel])
	require.Equal(t, "4", repo.values[service.SettingKeyCompressionGzipLevel])
	require.Equal(t, "2", repo.values[service.SettingKeyCompressionDeflateLevel])
	require.Contains(t, rec.Body.String(), `"compression_brotli_level":8`)

	rec = doUpdateSettings(t, h, map[string]any{"site_name": "compression-preserve"}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "8", repo.values[service.SettingKeyCompressionBrotliLevel])
	require.Equal(t, "10", repo.values[service.SettingKeyCompressionZstdLevel])
	require.Equal(t, "4", repo.values[service.SettingKeyCompressionGzipLevel])
	require.Equal(t, "2", repo.values[service.SettingKeyCompressionDeflateLevel])
}

func TestUpdateSettingsRejectsInvalidCompressionLevel(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeyCompressionBrotliLevel:  "5",
		service.SettingKeyCompressionZstdLevel:    "3",
		service.SettingKeyCompressionGzipLevel:    "6",
		service.SettingKeyCompressionDeflateLevel: "6",
	})
	rec := doUpdateSettings(t, h, map[string]any{"compression_zstd_level": 0}, nil)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "INVALID_COMPRESSION_LEVEL")
	require.Equal(t, "3", repo.values[service.SettingKeyCompressionZstdLevel])
}
