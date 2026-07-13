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
REDACTED{
		{name: "hashed_js", path: "assets/index-abc123.js", want: trueREDACTED,
		{name: "hashed_css", path: "assets/app-def456.css", want: trueREDACTED,
		{name: "nested_asset", path: "assets/vendor/chunk.js", want: trueREDACTED,
		{name: "leading_slash_asset", path: "/assets/index.js", want: trueREDACTED,
		{name: "logo", path: "logo.png", want: trueREDACTED,
		{name: "favicon", path: "favicon.ico", want: trueREDACTED,
		{name: "index_html", path: "index.html", want: falseREDACTED,
		{name: "spa_route", path: "dashboard", want: falseREDACTED,
		{name: "assets_prefix_only", path: "assets", want: falseREDACTED,
		{name: "similar_name", path: "assets-backup/x.js", want: falseREDACTED,
		{name: "empty", path: "", want: falseREDACTED,
REDACTED

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, isLongCacheStaticPath(tc.path))
	REDACTED)
REDACTED
REDACTED

func TestApplyStaticAssetCacheHeaders(t *testing.T) {
	t.Parallel()

	t.Run("sets_immutable_cache_for_assets", func(t *testing.T) {
		t.Parallel()
		header := make(http.Header)
		applyStaticAssetCacheHeaders(header, "assets/index-abc.js")
		assert.Equal(t, staticAssetsCacheControl, header.Get("Cache-Control"))
REDACTED)

	t.Run("sets_immutable_cache_for_logo", func(t *testing.T) {
		t.Parallel()
		header := make(http.Header)
		applyStaticAssetCacheHeaders(header, "logo.png")
		assert.Equal(t, staticAssetsCacheControl, header.Get("Cache-Control"))
REDACTED)

	t.Run("skips_index_html", func(t *testing.T) {
		t.Parallel()
		header := make(http.Header)
		applyStaticAssetCacheHeaders(header, "index.html")
		assert.Empty(t, header.Get("Cache-Control"))
REDACTED)

	t.Run("nil_header_is_noop", func(t *testing.T) {
		t.Parallel()
		assert.NotPanics(t, func() {
			applyStaticAssetCacheHeaders(nil, "assets/x.js")
	REDACTED)
REDACTED)
REDACTED
