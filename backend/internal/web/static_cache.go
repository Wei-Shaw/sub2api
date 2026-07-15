//go:build embed || unit

package web

import (
	"net/http"
	"path"
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

// isFingerprintedEmbeddedAssetPath reports whether a cleaned URL path refers to
// a Vite asset whose filename contains the default eight-character build hash.
func isFingerprintedEmbeddedAssetPath(cleanPath string) bool {
	cleanPath = strings.TrimPrefix(cleanPath, "/")
	if !strings.HasPrefix(cleanPath, "assets/") {
		return false
	}

	filename := path.Base(cleanPath)
	extension := path.Ext(filename)
	stem := strings.TrimSuffix(filename, extension)
	const fingerprintLength = 8
	delimiterIndex := len(stem) - fingerprintLength - 1
	if extension == "" || delimiterIndex < 1 || stem[delimiterIndex] != '-' {
		return false
	}

	// Vite hashes use URL-safe characters and are stable for immutable caching.
	fingerprint := stem[delimiterIndex+1:]
	for _, char := range fingerprint {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}

// applyStaticAssetCacheHeaders sets immutable caching for hashed assets and
// revalidation caching for replaceable fixed-name assets. Other paths are untouched.
func applyStaticAssetCacheHeaders(header http.Header, cleanPath string) {
	if header == nil {
		return
	}
	cleanPath = strings.TrimPrefix(cleanPath, "/")
	switch {
	case isFingerprintedEmbeddedAssetPath(cleanPath):
		header.Set("Cache-Control", staticAssetsCacheControl)
	case cleanPath == "logo.png" || cleanPath == "favicon.ico":
		header.Set("Cache-Control", fixedAssetCacheControl)
	}
}
