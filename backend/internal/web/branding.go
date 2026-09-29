package web

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

const brandingAssetCacheControl = "public, max-age=31536000, immutable"

// BrandingAssetProvider supplies the current same-origin branding asset.
type BrandingAssetProvider interface {
	GetSiteBrandingAsset(ctx context.Context, requestedPath string) (content []byte, contentType string, ok bool)
}

// ServeBrandingAsset serves a content-addressed site logo. The restrictive CSP
// also prevents an uploaded SVG from executing when opened as a document.
func ServeBrandingAsset(provider BrandingAssetProvider) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Do not let a transient miss poison the CDN during an instance update.
		c.Header("Cache-Control", "no-store")
		if provider == nil {
			c.Status(http.StatusNotFound)
			return
		}

		content, contentType, ok := provider.GetSiteBrandingAsset(c.Request.Context(), c.Request.URL.Path)
		if !ok {
			c.Status(http.StatusNotFound)
			return
		}

		c.Header("Cache-Control", brandingAssetCacheControl)
		c.Header("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
		c.Header("X-Content-Type-Options", "nosniff")
		if c.Request.Method == http.MethodHead {
			c.Header("Content-Type", contentType)
			c.Header("Content-Length", strconv.Itoa(len(content)))
			c.Status(http.StatusOK)
			return
		}
		c.Data(http.StatusOK, contentType, content)
	}
}
