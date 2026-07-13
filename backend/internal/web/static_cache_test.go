//go:build unit

package web

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsLongCacheStaticPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		path string
		want bool
	}{
		{name: "hashed_js", path: "assets/index-abc123.js", want: true},
		{name: "hashed_css", path: "assets/app-def456.css", want: true},
		{name: "nested_asset", path: "assets/vendor/chunk.js", want: true},
		{name: "leading_slash_asset", path: "/assets/index.js", want: true},
		{name: "logo", path: "logo.png", want: true},
		{name: "favicon", path: "favicon.ico", want: true},
		{name: "index_html", path: "index.html", want: false},
		{name: "spa_route", path: "dashboard", want: false},
		{name: "assets_prefix_only", path: "assets", want: false},
		{name: "similar_name", path: "assets-backup/x.js", want: false},
		{name: "empty", path: "", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, isLongCacheStaticPath(tc.path))
		})
	}
}

func TestApplyStaticAssetCacheHeaders(t *testing.T) {
	t.Parallel()

	t.Run("sets_immutable_cache_for_assets", func(t *testing.T) {
		t.Parallel()
		header := make(http.Header)
		applyStaticAssetCacheHeaders(header, "assets/index-abc.js")
		assert.Equal(t, staticAssetsCacheControl, header.Get("Cache-Control"))
	})

	t.Run("sets_immutable_cache_for_logo", func(t *testing.T) {
		t.Parallel()
		header := make(http.Header)
		applyStaticAssetCacheHeaders(header, "logo.png")
		assert.Equal(t, staticAssetsCacheControl, header.Get("Cache-Control"))
	})

	t.Run("skips_index_html", func(t *testing.T) {
		t.Parallel()
		header := make(http.Header)
		applyStaticAssetCacheHeaders(header, "index.html")
		assert.Empty(t, header.Get("Cache-Control"))
	})

	t.Run("nil_header_is_noop", func(t *testing.T) {
		t.Parallel()
		assert.NotPanics(t, func() {
			applyStaticAssetCacheHeaders(nil, "assets/x.js")
		})
	})
}
