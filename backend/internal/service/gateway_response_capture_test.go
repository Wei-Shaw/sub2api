package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
)

func captureEnabledService(maxBytes int) *GatewayService {
	return &GatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		ContentCapture: config.ContentCaptureConfig{Enabled: true, ResponseMaxBytes: maxBytes},
	}}}
}

// 最小可跑流式的网关服务（与 unit-tagged 的 newMinimalGatewayService 同构，但不带构建标签，
// 使本透明性测试在默认 go test 下即可运行）。
func newStreamCapableService() *GatewayService {
	return &GatewayService{
		cfg: &config.Config{Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			MaxLineSize:               defaultMaxLineSize,
		}},
		rateLimitService: &RateLimitService{},
	}
}

// 包装器透明性：写客户端的字节与「未包装时」逐字节一致，sink 抄到同一份副本（判断基准0）。
func TestCapturingResponseWriter_Transparent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	sink := newCappedSink(1 << 20)
	w := &capturingResponseWriter{ResponseWriter: c.Writer, sink: sink}

	payload := "data: {\"hello\":\"世界\"}\n\n"
	if _, err := io.WriteString(w, payload); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	tail := []byte("tail-bytes-尾巴")
	if _, err := w.Write(tail); err != nil {
		t.Fatalf("Write: %v", err)
	}

	want := payload + string(tail)
	if got := rec.Body.String(); got != want {
		t.Fatalf("client bytes changed by tee: got %q want %q", got, want)
	}
	if got := string(sink.Bytes()); got != want {
		t.Fatalf("sink mismatch: got %q want %q", got, want)
	}
	if sink.Truncated() {
		t.Errorf("should not be truncated under cap")
	}
	if sink.Total() != int64(len(want)) {
		t.Errorf("Total=%d want %d", sink.Total(), len(want))
	}
}

// 限容：sink 抓满 cap 即停，但客户端必须收到完整字节（cap 只约束副本，不约束转发）。
func TestCapturingResponseWriter_CapsSinkNotClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	const cap = 8
	sink := newCappedSink(cap)
	w := &capturingResponseWriter{ResponseWriter: c.Writer, sink: sink}

	full := strings.Repeat("A", 20)
	if _, err := io.WriteString(w, full); err != nil {
		t.Fatalf("WriteString: %v", err)
	}

	if got := rec.Body.String(); got != full {
		t.Fatalf("client must receive ALL bytes, got %d want %d", len(got), len(full))
	}
	if len(sink.Bytes()) != cap {
		t.Errorf("sink len=%d want cap=%d", len(sink.Bytes()), cap)
	}
	if !sink.Truncated() {
		t.Errorf("sink should be truncated")
	}
	if sink.Total() != int64(len(full)) {
		t.Errorf("Total=%d want %d", sink.Total(), len(full))
	}
}

// beginResponseCapture 开启时经 c.Data 写出 → 客户端字节不变，sink 抓到同一份。
func TestBeginResponseCapture_Enabled_CData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := captureEnabledService(1 << 20)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	sink, restore := svc.beginResponseCapture(c)
	if sink == nil {
		t.Fatal("enabled capture must return a sink")
	}
	body := []byte(`{"id":"msg_1","content":"hi 世界"}`)
	c.Data(http.StatusOK, "application/json", body)
	restore()

	if !bytes.Equal(rec.Body.Bytes(), body) {
		t.Fatalf("client bytes changed: got %q want %q", rec.Body.Bytes(), body)
	}
	if !bytes.Equal(sink.Bytes(), body) {
		t.Fatalf("sink mismatch: got %q want %q", sink.Bytes(), body)
	}
}

// beginResponseCapture 关闭时不分配/不包装 c.Writer（热路径零开销，判断基准5）。
func TestBeginResponseCapture_Disabled_NoOp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &GatewayService{cfg: &config.Config{}} // ContentCapture.Enabled = false
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	orig := c.Writer

	sink, restore := svc.beginResponseCapture(c)
	if sink != nil {
		t.Fatal("disabled capture must return nil sink")
	}
	if c.Writer != orig {
		t.Fatal("disabled capture must not replace c.Writer")
	}
	restore() // 必须是安全 no-op

	c.Data(http.StatusOK, "application/json", []byte("x"))
	if rec.Body.String() != "x" {
		t.Fatalf("unexpected client body: %q", rec.Body.String())
	}
}

// 真实流式写出路径透明性：同一份 SSE 跑两遍 handleStreamingResponse——
// 一遍裸 writer、一遍包装 writer——断言客户端字节逐字节相等，且包装那遍 sink 被填充。
func TestHandleStreamingResponse_TeeTransparent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newStreamCapableService()

	sse := "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello \\\"world\\\"\\n你好\"}}\n\n" +
		"data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":5}}}\n\n" +
		"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":3}}\n\n" +
		"data: [DONE]\n\n"

	run := func(wrap bool) ([]byte, *cappedSink) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

		var sink *cappedSink
		if wrap {
			sink = newCappedSink(1 << 20)
			c.Writer = &capturingResponseWriter{ResponseWriter: c.Writer, sink: sink}
		}

		pr, pw := io.Pipe()
		resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}
		go func() {
			defer func() { _ = pw.Close() }()
			_, _ = io.WriteString(pw, sse)
		}()

		_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
		_ = pr.Close()
		if err != nil {
			t.Fatalf("handleStreamingResponse (wrap=%v): %v", wrap, err)
		}
		return rec.Body.Bytes(), sink
	}

	plain, _ := run(false)
	wrapped, sink := run(true)

	if !bytes.Equal(plain, wrapped) {
		t.Fatalf("client bytes differ with capture on:\n off=%q\n on =%q", plain, wrapped)
	}
	if sink == nil || len(sink.Bytes()) == 0 {
		t.Fatal("sink not filled on wrapped run")
	}
	if !bytes.Equal(sink.Bytes(), wrapped) {
		t.Fatalf("sink should equal client bytes:\n sink=%q\n cli =%q", sink.Bytes(), wrapped)
	}
	if !bytes.Contains(wrapped, []byte("content_block_delta")) {
		t.Errorf("expected forwarded content in client output, got: %q", wrapped)
	}
}
