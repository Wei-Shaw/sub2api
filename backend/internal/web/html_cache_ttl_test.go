//go:build embed

package web

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHTMLCache_ExpiresAsMissedInvalidationFallback(t *testing.T) {
	cache := newHTMLCache(10 * time.Millisecond)
	cache.SetBaseHTML([]byte("<html></html>"))
	cache.Set([]byte("rendered"), []byte(`{"backend_mode_enabled":true}`))

	require.NotNil(t, cache.Get())
	require.Eventually(t, func() bool {
		return cache.Get() == nil
	}, time.Second, 5*time.Millisecond)
}
