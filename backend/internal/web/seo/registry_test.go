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
