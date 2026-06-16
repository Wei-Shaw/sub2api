package middleware

import (
	"bytes"

	"github.com/gin-gonic/gin"
)

const archiveCaptureContextKey = "archive_capture_writer"

// archiveCaptureWriter 包裹 gin.ResponseWriter，在转发响应给客户端的同时
// 把响应字节 tee 进一个有上限的 buffer。
//
// 关键：通过接口嵌入 gin.ResponseWriter，Flush/Size/Status/Hijack/CloseNotify
// 等方法自动透传——流式 SSE 的 w.(http.Flusher) 断言与 c.Writer.Size()
// 的 failover 判断都不受影响。
type archiveCaptureWriter struct {
	gin.ResponseWriter
	limit     int
	buf       bytes.Buffer
	truncated bool
}

func (w *archiveCaptureWriter) capture(p []byte) {
	if w.limit <= 0 {
		return
	}
	if w.buf.Len() >= w.limit {
		w.truncated = true
		return
	}
	remaining := w.limit - w.buf.Len()
	if len(p) > remaining {
		_, _ = w.buf.Write(p[:remaining])
		w.truncated = true
	} else {
		_, _ = w.buf.Write(p)
	}
}

func (w *archiveCaptureWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	if n > 0 {
		w.capture(b[:n])
	}
	return n, err
}

func (w *archiveCaptureWriter) WriteString(s string) (int, error) {
	n, err := w.ResponseWriter.WriteString(s)
	if n > 0 {
		w.capture([]byte(s[:n]))
	}
	return n, err
}

// ArchiveCaptureMiddleware 在网关路由上捕获完整响应体（含流式）。
// 仅在归档启用时挂载；maxBytes 为单请求捕获上限，超出截断。
func ArchiveCaptureMiddleware(maxBytes int) gin.HandlerFunc {
	if maxBytes <= 0 {
		maxBytes = 16 << 20
	}
	return func(c *gin.Context) {
		original := c.Writer
		w := &archiveCaptureWriter{ResponseWriter: original, limit: maxBytes}
		c.Writer = w
		c.Set(archiveCaptureContextKey, w)
		defer func() {
			// 还原原始 writer，避免外层中间件观察到包裹器。
			if c.Writer == w {
				c.Writer = original
			}
		}()
		c.Next()
	}
}

// GetArchivedResponse 返回中间件捕获到的响应体及是否被截断。
// 返回的字节切片底层引用 buffer，调用方应在使用时自行拷贝（如 string(...)）。
func GetArchivedResponse(c *gin.Context) (body []byte, truncated bool, ok bool) {
	v, exists := c.Get(archiveCaptureContextKey)
	if !exists {
		return nil, false, false
	}
	w, good := v.(*archiveCaptureWriter)
	if !good || w == nil {
		return nil, false, false
	}
	return w.buf.Bytes(), w.truncated, true
}
