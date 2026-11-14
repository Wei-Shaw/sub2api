//go:build unit

package web

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplyHTMLNoCacheHeaders(t *testing.T) {
	header := http.Header{}

	applyHTMLNoCacheHeaders(header)

	assert.Equal(t, "no-cache, no-store, must-revalidate", header.Get("Cache-Control"))
	assert.Equal(t, "no-cache", header.Get("Pragma"))
	assert.Equal(t, "0", header.Get("Expires"))
	assert.Equal(t, "no-store", header.Get("CDN-Cache-Control"))
	assert.Equal(t, "no-store", header.Get("Surrogate-Control"))
}

func TestApplyHTMLNoCacheHeadersReplacesExistingValues(t *testing.T) {
	header := http.Header{
		"Cache-Control":     {"public, max-age=3600"},
		"Pragma":            {"cache"},
		"Expires":           {"Thu, 01 Jan 2099 00:00:00 GMT"},
		"CDN-Cache-Control": {"max-age=3600"},
		"Surrogate-Control": {"max-age=3600"},
	}

	applyHTMLNoCacheHeaders(header)

	assert.Equal(t, "no-cache, no-store, must-revalidate", header.Get("Cache-Control"))
	assert.Equal(t, "no-cache", header.Get("Pragma"))
	assert.Equal(t, "0", header.Get("Expires"))
	assert.Equal(t, "no-store", header.Get("CDN-Cache-Control"))
	assert.Equal(t, "no-store", header.Get("Surrogate-Control"))
}
