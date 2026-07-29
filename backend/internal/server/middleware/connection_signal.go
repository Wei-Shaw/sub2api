package middleware

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ConnectionSignalMiddleware emits connection-risk signals after API key auth
// and enforces Phase B soft-throttle caps when present.
// Must be placed immediately after apiKeyAuth on every gateway surface.
// Fail-open on Redis errors: never aborts the request for emit failures;
// throttle only rejects when the mark is readable and the cap is exceeded.
func ConnectionSignalMiddleware(emitter *service.ConnectionSignalEmitter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if emitter == nil {
			c.Next()
			return
		}
		apiKey, ok := GetAPIKeyFromContext(c)
		if !ok || apiKey == nil {
			c.Next()
			return
		}
		userID := int64(0)
		if apiKey.User != nil {
			userID = apiKey.User.ID
		} else if sub, ok := GetAuthSubjectFromContext(c); ok {
			userID = sub.UserID
		}
		if userID <= 0 || apiKey.ID <= 0 {
			c.Next()
			return
		}

		// Phase B soft throttle (absolute per-key RPM) — fail-open on errors.
		if blocked, msg := emitter.CheckThrottle(c.Request.Context(), apiKey.ID); blocked {
			AbortWithError(c, 429, "CONNECTION_RISK_THROTTLED", msg)
			return
		}

		readOnly := isConnectionRiskReadOnlyPath(c.Request.URL.Path)
		if !emitter.ShouldIncludePath(c.Request.Context(), readOnly) {
			c.Next()
			return
		}

		rawIP := SecurityClientIP(c)
		ua := c.Request.UserAgent()
		emitter.EmitWithPrefix(c.Request.Context(), userID, apiKey.ID, rawIP, ua, apiKey.Key)
		c.Next()
	}
}

func isConnectionRiskReadOnlyPath(path string) bool {
	p := strings.ToLower(strings.TrimSpace(path))
	switch {
	case strings.HasSuffix(p, "/models"):
		return true
	case strings.Contains(p, "/usage"):
		return true
	case strings.Contains(p, "/billing"):
		return true
	case strings.Contains(p, "/sub2api/"):
		return true
	default:
		return false
	}
}
