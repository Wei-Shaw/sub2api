package middleware

import (
	"io"
	"net/http"
	"sync/atomic"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// byteCountingResponseWriter 统计写往客户端的线上字节（SSE 帧开销计入）。
type byteCountingResponseWriter struct {
	gin.ResponseWriter
	n int64
}

func (w *byteCountingResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.n += int64(n)
	return n, err
}

func (w *byteCountingResponseWriter) WriteString(s string) (int, error) {
	n, err := w.ResponseWriter.WriteString(s)
	w.n += int64(n)
	return n, err
}

// byteCountingRequestBody 统计 chunked 上传的请求体字节（有 Content-Length 时不包）。
type byteCountingRequestBody struct {
	io.ReadCloser
	n *int64
}

func (c *byteCountingRequestBody) Read(p []byte) (int, error) {
	n, err := c.ReadCloser.Read(p)
	if n > 0 {
		atomic.AddInt64(c.n, int64(n))
	}
	return n, err
}

// ByteCounter 线上字节账的网关入口：入口挂累计器，结束时写回客户端两路字节，
// 供 usage 落库（GatewayWireBytesFromContext）。usage_logs 只记成功请求，
// 失败/中止请求的字节在这里落一行 warn 日志，可按 request 关联对账。
func ByteCounter() gin.HandlerFunc {
	return func(c *gin.Context) {
		var in int64
		if c.Request != nil && c.Request.Body != nil {
			if c.Request.ContentLength >= 0 {
				in = c.Request.ContentLength
			} else {
				c.Request.Body = &byteCountingRequestBody{ReadCloser: c.Request.Body, n: &in}
			}
			c.Request = c.Request.WithContext(service.WithGatewayWireBytes(c.Request.Context()))
		}
		w := &byteCountingResponseWriter{ResponseWriter: c.Writer}
		c.Writer = w
		c.Next()
		if c.Request == nil {
			return
		}
		service.SetClientWireBytes(c.Request.Context(), in, w.n)
		if status := c.Writer.Status(); status >= http.StatusBadRequest {
			inBytes, outBytes, upstreamBytes := service.GatewayWireBytesFromContext(c.Request.Context())
			logger.L().Warn("gateway.wire_bytes_failed_request",
				zap.Int("status", status),
				zap.String("path", c.Request.URL.Path),
				zap.Int64("in_bytes", inBytes),
				zap.Int64("out_bytes", outBytes),
				zap.Int64("upstream_out_bytes", upstreamBytes),
			)
		}
	}
}
