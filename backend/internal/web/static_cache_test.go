//go:build unit

package web

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsFingerprintedEmbeddedAssetPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		path string
		want bool
	}{
		{name: "fingerprinted_js", path: "assets/index-AbCd1234.js", want: true},
		{name: "fingerprinted_css", path: "assets/app-a1B2c3D4.css", want: true},
		{name: "fingerprinted_url_safe_hash", path: "assets/app-aB1-2_Cd.css", want: true},
		{name: "nested_fingerprinted_asset", path: "assets/vendor/chunk-AbCd1234.js", want: true},
		{name: "leading_slash_fingerprinted_asset", path: "/assets/index-AbCd1234.js", want: true},
		{name: "unhashed_asset", path: "assets/index.js", want: false},
		{name: "short_suffix", path: "assets/index-abc123.js", want: false},
		{name: "logo", path: "logo.png", want: false},
		{name: "favicon", path: "favicon.ico", want: false},
		{name: "fingerprint_outside_assets", path: "downloads/index-AbCd1234.js", want: false},
		{name: "index_html", path: "index.html", want: false},
		{name: "spa_route", path: "dashboard", want: false},
		{name: "assets_prefix_only", path: "assets", want: false},
		{name: "similar_name", path: "assets-backup/x.js", want: false},
		{name: "empty", path: "", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, isFingerprintedEmbeddedAssetPath(tc.path))
		})
	}
}

func TestApplyStaticAssetCacheHeaders(t *testing.T) {
	t.Parallel()

	t.Run("sets_immutable_cache_for_fingerprinted_asset", func(t *testing.T) {
		t.Parallel()
		header := make(http.Header)
		applyStaticAssetCacheHeaders(header, "assets/index-AbCd1234.js")
		assert.Equal(t, staticAssetsCacheControl, header.Get("Cache-Control"))
	})

	for _, path := range []string{"assets/index.js", "logo.png", "favicon.ico", "index.html"} {
		path := path
		t.Run("skips_"+path, func(t *testing.T) {
			t.Parallel()
			header := make(http.Header)
			applyStaticAssetCacheHeaders(header, path)
			assert.Empty(t, header.Get("Cache-Control"))
		})
	}

	t.Run("nil_header_is_noop", func(t *testing.T) {
		t.Parallel()
		assert.NotPanics(t, func() {
			applyStaticAssetCacheHeaders(nil, "assets/index-AbCd1234.js")
		})
	})
}
