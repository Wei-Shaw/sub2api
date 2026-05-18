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
