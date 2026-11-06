package compresslevel

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultSettingsAreValid(t *testing.T) {
	defaults := Default()
	require.NoError(t, defaults.Validate())
	require.Equal(t, Settings{Brotli: 5, Zstd: 3, Gzip: 6, Deflate: 6}, defaults)
}

func TestSettingsValidateBoundaries(t *testing.T) {
	valid := []Settings{
		{Brotli: 0, Zstd: 1, Gzip: 0, Deflate: 0},
		{Brotli: 11, Zstd: 19, Gzip: 9, Deflate: 9},
	}
	for _, settings := range valid {
		require.NoError(t, settings.Validate())
	}

	invalid := []Settings{
		{Brotli: -1, Zstd: 3, Gzip: 6, Deflate: 6},
		{Brotli: 12, Zstd: 3, Gzip: 6, Deflate: 6},
		{Brotli: 5, Zstd: 0, Gzip: 6, Deflate: 6},
		{Brotli: 5, Zstd: 20, Gzip: 6, Deflate: 6},
		{Brotli: 5, Zstd: 3, Gzip: -1, Deflate: 6},
		{Brotli: 5, Zstd: 3, Gzip: 10, Deflate: 6},
		{Brotli: 5, Zstd: 3, Gzip: 6, Deflate: -1},
		{Brotli: 5, Zstd: 3, Gzip: 6, Deflate: 10},
	}
	for _, settings := range invalid {
		require.Error(t, settings.Validate())
	}
}

func TestResolveUsesProviderOrDefaults(t *testing.T) {
	require.Equal(t, Default(), Resolve(context.Background(), nil))
	want := Settings{Brotli: 8, Zstd: 9, Gzip: 2, Deflate: 4}
	provider := ProviderFunc(func(context.Context) Settings { return want })
	require.Equal(t, want, Resolve(context.Background(), provider))
}
