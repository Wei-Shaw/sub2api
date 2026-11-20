package middleware

import (
	"net/http"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"

	"github.com/gin-gonic/gin"
)

// RequestBodyLimit 使用 MaxBytesReader 限制请求体大小。
func RequestBodyLimit(maxBytes int64, budgets ...*pkghttputil.BodyMemoryBudget) gin.HandlerFunc {
	return func(c *gin.Context) {
		if len(budgets) > 0 {
			ctx, release := pkghttputil.WithRequestBodyMemoryBudget(c.Request.Context(), budgets[0])
			defer release()
			c.Request = c.Request.WithContext(ctx)
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}

// Apply these limits before allowlist and composite routing can preread the body.
func DeepseekToolsBodyLimit(textLimit, gatewayLimit int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodPost {
			return
		}
		var limit int64
		switch c.FullPath() {
		case "/v1/tools/search", "/v1/tools/fetch":
			limit = textLimit
		case "/v1/tools/tts":
			limit = 64 << 10
		case "/v1/tools/asr":
			limit = 8 << 20
		case "/v1/tools/ocr", "/v1/files":
			limit = 100 << 20
		default:
			return
		}
		if gatewayLimit > 0 {
			limit = min(limit, gatewayLimit)
		}
		if c.Request.ContentLength > limit {
			AbortWithRequestBodyReadFailure(c, "middleware.deepseek_tools", &http.MaxBytesError{Limit: limit})
			return
		}
		c.Request = c.Request.WithContext(pkghttputil.WithDecodedBodyLimit(c.Request.Context(), limit))
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
	}
}
