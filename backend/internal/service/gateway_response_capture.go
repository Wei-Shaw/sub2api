package service

import (
	"bytes"

	"github.com/gin-gonic/gin"
)

type cappedSink struct {
	buf       bytes.Buffer
	limit     int
	total     int64
	truncated bool
}

func newCappedSink(limit int) *cappedSink {
	if limit <= 0 {
		limit = defaultGenerationResponseMaxBytes
	}
	return &cappedSink{limit: limit}
}

func (s *cappedSink) Write(p []byte) (int, error) {
	if s == nil {
		return len(p), nil
	}
	s.total += int64(len(p))
	remaining := s.limit - s.buf.Len()
	if remaining <= 0 {
		s.truncated = s.truncated || len(p) > 0
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = s.buf.Write(p[:remaining])
		s.truncated = true
		return len(p), nil
	}
	_, _ = s.buf.Write(p)
	return len(p), nil
}

func (s *cappedSink) Bytes() []byte {
	if s == nil {
		return nil
	}
	return append([]byte(nil), s.buf.Bytes()...)
}

func (s *cappedSink) Truncated() bool {
	return s != nil && s.truncated
}

func (s *cappedSink) Total() int64 {
	if s == nil {
		return 0
	}
	return s.total
}

// capturingResponseWriter 是 M1 采集口的「旁路写出器」：包在 gin 的 ResponseWriter 外面，
// 每写一笔给客户端的字节，就顺手抄一份进 cappedSink，但绝不改变发给客户端的内容。
//
// 透明性（判断基准0 最高红线）：Write/WriteString 先调用底层 writer，再 sink.Write 已写出的字节，
// 并原样返回底层的 (n, err)。客户端永远收到与「采集关闭时」完全一致的字节。
//
// 缓冲独立：cappedSink 自持缓冲并 copy 输入，即便底层把池化缓冲（如 SSE scanner buffer）传进来也安全；
// 本类型绝不引用任何池化 SSE scanner 缓冲（避免归还后 use-after-return 损坏 / 跨请求泄露）。
//
// 它只重写 Write/WriteString 两个写出方法；Status/Size/Written/Flush/Hijack/Pusher 等全部由内嵌的
// gin.ResponseWriter 接口自动提升到底层（含 http.Flusher，流式 flush 不受影响）。
type capturingResponseWriter struct {
	gin.ResponseWriter
	sink *cappedSink
}

func (w *capturingResponseWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if n > 0 {
		_, _ = w.sink.Write(p[:n])
	}
	return n, err
}

func (w *capturingResponseWriter) WriteString(s string) (int, error) {
	n, err := w.ResponseWriter.WriteString(s)
	if n > 0 {
		_, _ = w.sink.Write([]byte(s[:n]))
	}
	return n, err
}

// beginResponseCapture 在采集开关打开时，把 c.Writer 临时换成 capturingResponseWriter，
// 返回限容 sink 与一个还原 c.Writer 的函数（调用方应 defer 它）。
// 采集关闭 / c 为 nil 时不分配、不包装，返回 (nil, 空函数) —— 热路径零开销（判断基准5）。
func (s *GatewayService) beginResponseCapture(c *gin.Context) (*cappedSink, func()) {
	if c == nil || !s.contentCaptureEnabled() {
		return nil, func() {}
	}
	sink := newCappedSink(s.responseCaptureMaxBytes())
	orig := c.Writer
	c.Writer = &capturingResponseWriter{ResponseWriter: orig, sink: sink}
	return sink, func() { c.Writer = orig }
}

// responseCaptureMaxBytes 返回 response 抽样上限（字节），默认 defaultGenerationResponseMaxBytes(64KiB)。
func (s *GatewayService) responseCaptureMaxBytes() int {
	if s != nil && s.cfg != nil && s.cfg.Gateway.ContentCapture.ResponseMaxBytes > 0 {
		return s.cfg.Gateway.ContentCapture.ResponseMaxBytes
	}
	return defaultGenerationResponseMaxBytes
}

// fillResponseSample 把限容 sink 的抽样结果回填进 ForwardResult 的三个 response 字段。
// r 或 sink 任一为 nil 即 no-op（采集关闭时 sink 为 nil → 三字段保持零值）。
func fillResponseSample(r *ForwardResult, sink *cappedSink) {
	if r == nil || sink == nil {
		return
	}
	r.ResponseSample = sink.Bytes()
	r.ResponseTruncated = sink.Truncated()
	r.ResponseBytes = int(sink.Total())
}
