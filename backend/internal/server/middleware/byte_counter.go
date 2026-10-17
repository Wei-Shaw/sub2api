package middleware

import (
	"context"
	"io"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// byteCountingResponseWriter 统计写往客户端的线上字节（SSE 帧开销计入），
// 发生即累加进 ctx 累计器——usage 落库发生在 handler 内部，读的是当时值。
type byteCountingResponseWriter struct {
	gin.ResponseWriter
	ctx context.Context
}

func (w *byteCountingResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	service.AddClientOutBytes(w.ctx, int64(n))
	return n, err
}

func (w *byteCountingResponseWriter) WriteString(s string) (int, error) {
	n, err := w.ResponseWriter.WriteString(s)
	service.AddClientOutBytes(w.ctx, int64(n))
	return n, err
}

// byteCountingRequestBody 统计 chunked 上传的请求体字节（有 Content-Length 时不包）。
type byteCountingRequestBody struct {
	io.ReadCloser
	ctx context.Context
}

func (c *byteCountingRequestBody) Read(p []byte) (int, error) {
	n, err := c.ReadCloser.Read(p)
	if n > 0 {
		service.AddClientInBytes(c.ctx, int64(n))
	}
	return n, err
}

// ByteCounter 线上字节账的网关入口：入口挂累计器，字节收发即累加，
// 供 usage 落库（GatewayWireBytesFromContext）读取当时已发生的字节数。
// usage_logs 只记成功请求，失败/中止请求的字节在这里落一行 warn 日志，
// 可按 request 关联对账。
func ByteCounter() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request != nil && c.Request.Body != nil {
			ctx := service.WithGatewayWireBytes(c.Request.Context())
			c.Request = c.Request.WithContext(ctx)
			if c.Request.ContentLength >= 0 {
				service.AddClientInBytes(ctx, c.Request.ContentLength)
			} else {
				c.Request.Body = &byteCountingRequestBody{ReadCloser: c.Request.Body, ctx: ctx}
			}
		}
		if c.Request != nil {
			c.Writer = &byteCountingResponseWriter{ResponseWriter: c.Writer, ctx: c.Request.Context()}
		}
		c.Next()
		if c.Request == nil {
			return
		}
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
