//go:build embed || unit

package web

import (
	"net/http"
	"strings"
)

// staticAssetsCacheControl matches deploy/Caddyfile for hashed frontend assets.
// Vite emits content-hashed filenames under assets/, so long-lived immutable
// caching is safe without relying on a reverse proxy.
const staticAssetsCacheControl = "public, max-age=31536000, immutable"

// isLongCacheStaticPath reports whether a cleaned URL path (no leading slash)
// should receive long-lived Cache-Control headers. Aligned with deploy/Caddyfile.
func isLongCacheStaticPath(cleanPath string) bool {
	cleanPath = strings.TrimPrefix(cleanPath, "/")
	return strings.HasPrefix(cleanPath, "assets/") ||
		cleanPath == "logo.png" ||
		cleanPath == "favicon.ico"
}

// applyStaticAssetCacheHeaders sets Cache-Control for long-cacheable static paths.
// index.html / SPA routes must keep no-cache and are not handled here.
func applyStaticAssetCacheHeaders(header http.Header, cleanPath string) {
	if header == nil || !isLongCacheStaticPath(cleanPath) {
		return
	}
	header.Set("Cache-Control", staticAssetsCacheControl)
}
