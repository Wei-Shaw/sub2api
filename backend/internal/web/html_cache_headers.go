//go:build embed || unit

package web

import "net/http"

// applyHTMLNoCacheHeaders marks settings-injected HTML as uncacheable at every
// layer: browser memory/disk cache, back/forward cache (browsers skip bfcache
// for no-store responses) and CDN edge caches. Settings are injected into the
// document, so a stale copy would paint outdated branding until the next
// revalidation.
func applyHTMLNoCacheHeaders(header http.Header) {
	header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	header.Set("Pragma", "no-cache")
	header.Set("Expires", "0")
	header.Set("CDN-Cache-Control", "no-store")
	header.Set("Surrogate-Control", "no-store")
}
