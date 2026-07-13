//go:build embed || unit

package web

import (
	"net/http"
	"strings"
)

const (
	// Vite emits content-hashed filenames under assets/, so long-lived immutable
	// caching is safe without relying on a reverse proxy.
	staticAssetsCacheControl = "public, max-age=31536000, immutable"
	// Fixed-name assets may be replaced by a release or data/public override.
	// Keep them cacheable while requiring revalidation before reuse.
	fixedAssetCacheControl = "public, max-age=0, must-revalidate"
)

// isLongCacheStaticPath reports whether a cleaned URL path (no leading slash)
// should receive long-lived Cache-Control headers. Aligned with deploy/Caddyfile.
func isLongCacheStaticPath(cleanPath string) bool {
	cleanPath = strings.TrimPrefix(cleanPath, "/")
	return strings.HasPrefix(cleanPath, "assets/")
}

// applyStaticAssetCacheHeaders sets immutable caching for hashed assets and
// revalidation caching for replaceable fixed-name assets. Other paths are untouched.
func applyStaticAssetCacheHeaders(header http.Header, cleanPath string) {
	if header == nil {
		return
	}
	cleanPath = strings.TrimPrefix(cleanPath, "/")
	switch {
	case isLongCacheStaticPath(cleanPath):
		header.Set("Cache-Control", staticAssetsCacheControl)
	case cleanPath == "logo.png" || cleanPath == "favicon.ico":
		header.Set("Cache-Control", fixedAssetCacheControl)
	}
}
