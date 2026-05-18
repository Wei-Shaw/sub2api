//go:build embed

package web

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHTMLCache_perKey(t *testing.T) {
	c := NewHTMLCache()
	c.SetBaseHTML([]byte("<html></html>"))

	t.Run("get_missing_returns_nil", func(t *testing.T) {
		got := c.GetKey("path|zh")
		assert.Nil(t, got)
	})

	t.Run("set_then_get_returns_content", func(t *testing.T) {
		c.SetKey("/home|zh", []byte("HTML_zh"), []byte(`{"site_name":"S"}`))
		got := c.GetKey("/home|zh")
		assert.NotNil(t, got)
		assert.Equal(t, "HTML_zh", string(got.Content))
	})

	t.Run("invalidate_clears_all_entries", func(t *testing.T) {
		c.SetKey("/home|zh", []byte("a"), []byte("{}"))
		c.SetKey("/home|en", []byte("b"), []byte("{}"))
		c.Invalidate()
		assert.Nil(t, c.GetKey("/home|zh"))
		assert.Nil(t, c.GetKey("/home|en"))
	})

	t.Run("lru_evicts_oldest_when_full", func(t *testing.T) {
		small := NewHTMLCache()
		small.SetBaseHTML([]byte("<html></html>"))
		small.SetMaxEntries(2)
		small.SetKey("a", []byte("1"), []byte("{}"))
		small.SetKey("b", []byte("2"), []byte("{}"))
		small.SetKey("c", []byte("3"), []byte("{}"))
		assert.Nil(t, small.GetKey("a"), "a should be evicted")
		assert.NotNil(t, small.GetKey("b"))
		assert.NotNil(t, small.GetKey("c"))
	})
}
