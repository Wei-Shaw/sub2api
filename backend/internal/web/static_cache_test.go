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
REDACTED{
		{name: "fingerprinted_js", path: "assets/index-AbCd1234.js", want: trueREDACTED,
		{name: "fingerprinted_css", path: "assets/app-a1B2c3D4.css", want: trueREDACTED,
		{name: "fingerprinted_url_safe_hash", path: "assets/app-aB1-2_Cd.css", want: trueREDACTED,
		{name: "nested_fingerprinted_asset", path: "assets/vendor/chunk-AbCd1234.js", want: trueREDACTED,
		{name: "leading_slash_fingerprinted_asset", path: "/assets/index-AbCd1234.js", want: trueREDACTED,
		{name: "unhashed_asset", path: "assets/index.js", want: falseREDACTED,
		{name: "short_suffix", path: "assets/index-abc123.js", want: falseREDACTED,
		{name: "logo", path: "logo.png", want: falseREDACTED,
		{name: "favicon", path: "favicon.ico", want: falseREDACTED,
		{name: "fingerprint_outside_assets", path: "downloads/index-AbCd1234.js", want: falseREDACTED,
		{name: "index_html", path: "index.html", want: falseREDACTED,
		{name: "spa_route", path: "dashboard", want: falseREDACTED,
		{name: "assets_prefix_only", path: "assets", want: falseREDACTED,
		{name: "similar_name", path: "assets-backup/x.js", want: falseREDACTED,
		{name: "empty", path: "", want: falseREDACTED,
REDACTED

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, isFingerprintedEmbeddedAssetPath(tc.path))
	REDACTED)
REDACTED
REDACTED

func TestApplyStaticAssetCacheHeaders(t *testing.T) {
	t.Parallel()

	t.Run("sets_immutable_cache_for_fingerprinted_asset", func(t *testing.T) {
		t.Parallel()
		header := make(http.Header)
		applyStaticAssetCacheHeaders(header, "assets/index-AbCd1234.js")
		assert.Equal(t, staticAssetsCacheControl, header.Get("Cache-Control"))
REDACTED)

	for _, path := range []string{"assets/index.js", "logo.png", "favicon.ico", "index.html"REDACTED {
		path := path
		t.Run("skips_"+path, func(t *testing.T) {
			t.Parallel()
			header := make(http.Header)
			applyStaticAssetCacheHeaders(header, path)
			assert.Empty(t, header.Get("Cache-Control"))
	REDACTED)
REDACTED

	t.Run("nil_header_is_noop", func(t *testing.T) {
		t.Parallel()
		assert.NotPanics(t, func() {
			applyStaticAssetCacheHeaders(nil, "assets/index-AbCd1234.js")
	REDACTED)
REDACTED)
REDACTED
