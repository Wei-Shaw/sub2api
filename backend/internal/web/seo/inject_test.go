//go:build embed

package seo

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHeadInjector_Render_basic(t *testing.T) {
	reg, err := NewRegistry()
	require.NoError(t, err)

	baseHTML := []byte(`<html><head><title>Sub2API - AI API Gateway</title></head><body></body></html>`)
	inj := NewHeadInjector(reg)

	out := inj.Render(baseHTML, "/home", "zh", nil, "https://example.com")
	s := string(out)

	assert.Contains(t, s, "<title>Sub2API")
	assert.Contains(t, s, `<meta name="description"`)
	assert.Contains(t, s, `<meta name="robots" content="index,follow"`)
	assert.Contains(t, s, `<link rel="canonical" href="https://example.com/home"`)
	assert.Contains(t, s, `<meta property="og:title"`)
	assert.Contains(t, s, `<meta property="og:locale" content="zh_CN"`)
	assert.Contains(t, s, `<meta name="twitter:card"`)
	assert.Contains(t, s, `<link rel="alternate" hreflang="zh-CN"`)
	assert.Contains(t, s, `<link rel="alternate" hreflang="en"`)
	assert.Contains(t, s, `application/ld+json`)
}

func TestHeadInjector_Render_noindex(t *testing.T) {
	reg, err := NewRegistry()
	require.NoError(t, err)

	baseHTML := []byte(`<html><head><title>X</title></head><body></body></html>`)
	out := NewHeadInjector(reg).Render(baseHTML, "/login", "zh", nil, "https://example.com")
	s := string(out)

	assert.Contains(t, s, `<meta name="robots" content="noindex,follow"`)
	assert.NotContains(t, s, `hreflang`)
}

func TestHeadInjector_Render_escapesXSS(t *testing.T) {
	reg, err := NewRegistry()
	require.NoError(t, err)

	rec := reg.Routes["/home"]
	rec.Langs["zh"] = LangPayload{
		Title:       `<script>alert(1)</script>`,
		Description: `"><img src=x onerror=alert(2)>`,
	}
	reg.Routes["/home"] = rec

	baseHTML := []byte(`<html><head><title>X</title></head><body></body></html>`)
	out := NewHeadInjector(reg).Render(baseHTML, "/home", "zh", nil, "https://example.com")
	s := string(out)

	assert.NotContains(t, s, "<script>alert(1)</script>")
	assert.Contains(t, s, "&lt;script&gt;alert(1)&lt;/script&gt;")
	assert.NotContains(t, s, `"><img`)
}

func TestHeadInjector_Render_unknownRouteIsNoindex(t *testing.T) {
	reg, err := NewRegistry()
	require.NoError(t, err)

	baseHTML := []byte(`<html><head><title>X</title></head><body></body></html>`)
	out := NewHeadInjector(reg).Render(baseHTML, "/no-such-thing", "zh", nil, "https://example.com")
	s := string(out)

	assert.Contains(t, s, `<meta name="robots" content="noindex,follow"`)
}

func TestHeadInjector_Render_preservesExistingHeadContent(t *testing.T) {
	reg, err := NewRegistry()
	require.NoError(t, err)

	baseHTML := []byte(`<html><head><link rel="icon" href="/logo.png"/><title>X</title></head><body></body></html>`)
	out := NewHeadInjector(reg).Render(baseHTML, "/home", "zh", nil, "https://example.com")
	s := string(out)

	assert.Contains(t, s, `<link rel="icon" href="/logo.png"/>`)
	assert.True(t, strings.Contains(s, "<title>Sub2API"))
}
