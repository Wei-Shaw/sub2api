package admin

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *AccountHandler) GetUpstreamSiteLogo(c *gin.Context) {
	if h.upstreamBillingProbe == nil {
		response.ErrorFrom(c, service.ErrUpstreamBillingProbeUnavailable)
		return
	}
	key := c.Param("key")
	logo, err := h.upstreamBillingProbe.GetUpstreamSiteLogo(c.Request.Context(), key)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	etag := `"` + key + `"`
	c.Header("Cache-Control", "private, max-age=31536000, immutable")
	c.Header("ETag", etag)
	c.Header("Vary", "If-None-Match")
	if ifNoneMatchMatched(c.GetHeader("If-None-Match"), etag) {
		c.Status(http.StatusNotModified)
		return
	}
	c.Data(http.StatusOK, logo.ContentType, logo.Data)
}
