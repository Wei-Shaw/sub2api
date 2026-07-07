package middleware

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
)

const (
	// usageMetadataHeader carries caller-supplied request metadata as a JSON object,
	// e.g. {"source":"agent","uid":"u_123","feature":"digest"}. It is recorded on
	// usage_logs.metadata for per-business / per-user / per-feature usage attribution.
	usageMetadataHeader = "X-Usage-Metadata"

	// usageMetadataMaxBytes bounds the raw header size to keep the jsonb column small
	// and avoid unbounded write amplification. Oversized headers are ignored.
	usageMetadataMaxBytes = 2048

	// usageMetadataMaxKeys bounds the number of top-level keys per request.
	usageMetadataMaxKeys = 16
)

// UsageMetadata parses the X-Usage-Metadata header (a JSON object) and stores it in
// the request context for the usage-recording path.
//
// The header is best-effort attribution data: any problem (missing, oversized,
// malformed JSON, non-object, too many keys) is silently ignored so a bad
// metadata header can never fail the underlying LLM request.
func UsageMetadata() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request == nil {
			c.Next()
			return
		}
		raw := strings.TrimSpace(c.GetHeader(usageMetadataHeader))
		if raw == "" || len(raw) > usageMetadataMaxBytes {
			c.Next()
			return
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			c.Next()
			return
		}
		if len(parsed) == 0 || len(parsed) > usageMetadataMaxKeys {
			c.Next()
			return
		}
		ctx := context.WithValue(c.Request.Context(), ctxkey.UsageMetadata, parsed)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
