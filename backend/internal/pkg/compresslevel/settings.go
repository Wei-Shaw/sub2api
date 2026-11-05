// Package compresslevel defines the runtime compression-level settings shared
// by upstream request compressors and downstream response compressors.
package compresslevel

import (
	"context"
	"fmt"
)

const (
	MinBrotliLevel  = 0
	MaxBrotliLevel  = 11
	MinZstdLevel    = 1
	MaxZstdLevel    = 19
	MinGzipLevel    = 0
	MaxGzipLevel    = 9
	MinDeflateLevel = 0
	MaxDeflateLevel = 9

	DefaultBrotliLevel  = 5
	DefaultZstdLevel    = 3
	DefaultGzipLevel    = 6
	DefaultDeflateLevel = 6
)

// Settings is a per-request snapshot. Numeric levels follow the conventional
// ranges of each algorithm; the zstd encoder maps its 1-19 setting to the
// nearest predefined klauspost/compress encoder mode.
type Settings struct {
	Brotli  int `json:"brotli"`
	Zstd    int `json:"zstd"`
	Gzip    int `json:"gzip"`
	Deflate int `json:"deflate"`
}

// Default returns the settings used before an administrator overrides them.
func Default() Settings {
	return Settings{
		Brotli:  DefaultBrotliLevel,
		Zstd:    DefaultZstdLevel,
		Gzip:    DefaultGzipLevel,
		Deflate: DefaultDeflateLevel,
	}
}

// Validate rejects values unsupported by the configured encoders.
func (s Settings) Validate() error {
	if s.Brotli < MinBrotliLevel || s.Brotli > MaxBrotliLevel {
		return fmt.Errorf("brotli compression level must be between %d and %d", MinBrotliLevel, MaxBrotliLevel)
	}
	if s.Zstd < MinZstdLevel || s.Zstd > MaxZstdLevel {
		return fmt.Errorf("zstd compression level must be between %d and %d", MinZstdLevel, MaxZstdLevel)
	}
	if s.Gzip < MinGzipLevel || s.Gzip > MaxGzipLevel {
		return fmt.Errorf("gzip compression level must be between %d and %d", MinGzipLevel, MaxGzipLevel)
	}
	if s.Deflate < MinDeflateLevel || s.Deflate > MaxDeflateLevel {
		return fmt.Errorf("deflate compression level must be between %d and %d", MinDeflateLevel, MaxDeflateLevel)
	}
	return nil
}

// Provider resolves a consistent settings snapshot for one request.
type Provider interface {
	GetTrafficCompressionLevels(ctx context.Context) Settings
}

// ProviderFunc adapts a function to Provider.
type ProviderFunc func(ctx context.Context) Settings

// GetTrafficCompressionLevels calls f(ctx).
func (f ProviderFunc) GetTrafficCompressionLevels(ctx context.Context) Settings {
	return f(ctx)
}

// Resolve returns defaults when no provider is configured.
func Resolve(ctx context.Context, provider Provider) Settings {
	if provider == nil {
		return Default()
	}
	return provider.GetTrafficCompressionLevels(ctx)
}
