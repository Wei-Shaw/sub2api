package middleware

import (
	"errors"
	"net/http"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/gin-gonic/gin"
)

// RejectRequestBodyReadError handles early ingress termination before the route
// handler runs. Cancelled clients have no response body and an uncommitted
// response is marked 499, matching the gateway handler's cancellation path.
func RejectRequestBodyReadError(c *gin.Context, err error) bool {
	if err != nil && c != nil && c.Request != nil && c.Request.Context().Err() != nil {
		if !c.Writer.Written() {
			c.Status(499)
		}
		c.Abort()
		return true
	}
	return RejectRequestBodyMemoryOverload(c, err)
}

// RejectRequestBodyMemoryOverload keeps temporary capacity errors distinct
// from invalid JSON and the per-request 413 size limit.
func RejectRequestBodyMemoryOverload(c *gin.Context, err error) bool {
	if !errors.Is(err, pkghttputil.ErrRequestBodyMemoryBudget) {
		return false
	}
	c.Header("Retry-After", "1")
	groupModelAllowlistErrorWriter(c)(c, http.StatusServiceUnavailable, "Request body buffering capacity is exhausted; retry later")
	c.Abort()
	return true
}
