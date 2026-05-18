//go:build embed

package seo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegistry_loadsEmbeddedJSON(t *testing.T) {
	reg, err := NewRegistry()
	require.NoError(t, err)
	require.NotNil(t, reg)

	assert.Equal(t, "Sub2API", reg.Site.Name)
	assert.Equal(t, "zh", reg.Site.DefaultLang)
	assert.Contains(t, reg.Site.SupportedLangs, "zh")
	assert.Contains(t, reg.Site.SupportedLangs, "en")

	home, ok := reg.Routes["/home"]
	require.True(t, ok, "/home route must exist")
	assert.True(t, home.Indexable)
	assert.Contains(t, home.Langs["zh"].Title, "Sub2API")
}

func TestRegistry_Resolve_static(t *testing.T) {
	reg, err := NewRegistry()
	require.NoError(t, err)

	t.Run("home_zh_returns_zh_payload", func(t *testing.T) {
		rec, lp := reg.Resolve("/home", "zh", nil)
		assert.True(t, rec.Indexable)
		assert.Contains(t, lp.Title, "Sub2API")
		assert.Contains(t, lp.Description, "Claude")
	})

	t.Run("home_en_returns_en_payload", func(t *testing.T) {
		_, lp := reg.Resolve("/home", "en", nil)
		assert.Contains(t, lp.Title, "Sub2API")
		assert.Contains(t, lp.Description, "unified access")
	})

	t.Run("unknown_lang_falls_back_to_default", func(t *testing.T) {
		_, lp := reg.Resolve("/home", "ja", nil)
		assert.NotEmpty(t, lp.Title, "must fall back to defaultLang(zh)")
	})

	t.Run("unknown_route_returns_default_noindex", func(t *testing.T) {
		rec, lp := reg.Resolve("/no-such-route", "zh", nil)
		assert.False(t, rec.Indexable)
		assert.NotEmpty(t, lp.Title, "default record must have a title")
	})

	t.Run("login_is_not_indexable", func(t *testing.T) {
		rec, _ := reg.Resolve("/login", "zh", nil)
		assert.False(t, rec.Indexable)
	})
}

func TestRegistry_Resolve_legalDoc(t *testing.T) {
	reg, err := NewRegistry()
	require.NoError(t, err)

	settingsJSON := []byte(`{
		"login_agreement_documents": [
			{"id": "terms-of-service", "title": "服务条款", "content_md": "## 第一章..."},
			{"id": "privacy-policy", "title": "隐私政策", "content_md": "..."}
		]
	}`)

	t.Run("matched_legal_doc_uses_doc_title", func(t *testing.T) {
		_, lp := reg.Resolve("/legal/terms-of-service", "zh", settingsJSON)
		assert.Contains(t, lp.Title, "服务条款")
		assert.Contains(t, lp.Title, "Sub2API")
	})

	t.Run("missing_legal_doc_returns_default_noindex", func(t *testing.T) {
		rec, _ := reg.Resolve("/legal/no-such-doc", "zh", settingsJSON)
		assert.False(t, rec.Indexable)
	})

	t.Run("malformed_settings_returns_default", func(t *testing.T) {
		rec, _ := reg.Resolve("/legal/terms", "zh", []byte("not json"))
		assert.False(t, rec.Indexable)
	})
}
