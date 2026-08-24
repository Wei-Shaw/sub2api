package service

// 国产供应商 Anthropic 协议转换路径的上游读间隔超时回归测试（B3）：
// 上游挂住 SSE（不发数据也不断连）时，CC×anthropic / Responses×anthropic
// 的读循环必须按 gateway.stream_data_interval_timeout 结束，而不是永久阻塞。

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
)

func newNativeAnthropicHangTestService(intervalSec int) *OpenAIGatewayService {
	return &OpenAIGatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				StreamDataIntervalTimeout: intervalSec,
				MaxLineSize:               defaultMaxLineSize,
			},
		},
	}
}

func newHangingUpstreamResponse() (*http.Response, *io.PipeReader, *io.PipeWriter) {
	pr, pw := io.Pipe()
	return &http.Response{StatusCode: http.StatusOK, Body: pr, Header: http.Header{}}, pr, pw
}

// miniAnthropicSSEStream 是一段最小可转换的 Anthropic 事件流。
func miniAnthropicSSEStream() string {
	return strings.Join([]string{
		"event: message_start",
		`data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"glm-4.7","usage":{"input_tokens":10,"output_tokens":1}}}`,
		"",
		"event: content_block_start",
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		"",
		"event: content_block_delta",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
		"",
		"event: content_block_stop",
		`data: {"type":"content_block_stop","index":0}`,
		"",
		"event: message_delta",
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`,
		"",
		"event: message_stop",
		`data: {"type":"message_stop"}`,
		"",
		"",
	}, "\n")
}

func TestAnthropicNativeLinePump_TimesOutWithoutData(t *testing.T) {
	pr, _ := io.Pipe()
	scanner := bufio.NewScanner(pr)
	defer func() { _ = pr.Close() }()

	pump := newAnthropicNativeLinePump(scanner, 50*time.Millisecond)
	defer pump.stop()

	start := time.Now()
	_, err := pump.next()
	if err == nil || !strings.Contains(err.Error(), "stream data interval timeout") {
		t.Fatalf("expected interval timeout, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("timeout not respected: %v", elapsed)
	}
}

func TestAnthropicNativeLinePump_DataResetsTimer(t *testing.T) {
	pr, pw := io.Pipe()
	scanner := bufio.NewScanner(pr)
	pump := newAnthropicNativeLinePump(scanner, 1*time.Second)
	defer pump.stop()

	go func() {
		_, _ = pw.Write([]byte("event: ping\n"))
		// 保持流打开且不再发数据：第二次 next 必须超时。
		time.Sleep(3 * time.Second)
		_ = pw.Close()
	}()
	defer func() { _ = pr.Close() }()

	line, err := pump.next()
	if err != nil || line != "event: ping" {
		t.Fatalf("expected first line, got %q err=%v", line, err)
	}

	start := time.Now()
	_, err = pump.next()
	if err == nil || !strings.Contains(err.Error(), "stream data interval timeout") {
		t.Fatalf("expected interval timeout after data stops, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("timeout not respected: %v", elapsed)
	}
}

func TestCCStreamingFromNativeAnthropic_HangTimesOut(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newNativeAnthropicHangTestService(1)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	resp, pr, pw := newHangingUpstreamResponse()
	start := time.Now()
	res, err := svc.handleCCStreamingFromNativeAnthropic(resp, c, "glm-4.7", "glm-4.7", "glm-4.7", nil, start, true)
	_ = pw.Close()
	_ = pr.Close()

	if err == nil || !strings.Contains(err.Error(), "stream data interval timeout") {
		t.Fatalf("expected stream timeout error, got %v", err)
	}
	if res == nil {
		t.Fatalf("expected result carrying accumulated usage")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("handler did not respect interval bound: %v", elapsed)
	}
}

func TestCCBufferedFromNativeAnthropic_HangTimesOut(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newNativeAnthropicHangTestService(1)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	resp, pr, pw := newHangingUpstreamResponse()
	start := time.Now()
	_, err := svc.handleCCBufferedFromNativeAnthropic(resp, c, "glm-4.7", "glm-4.7", "glm-4.7", nil, start)
	_ = pw.Close()
	_ = pr.Close()

	if err == nil || !strings.Contains(err.Error(), "stream data interval timeout") {
		t.Fatalf("expected stream timeout error, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("handler did not respect interval bound: %v", elapsed)
	}
	if !strings.Contains(rec.Body.String(), "Upstream stream data interval timeout") {
		t.Fatalf("expected 502 error body, got %q", rec.Body.String())
	}
}

func TestResponsesStreamingFromNativeAnthropic_HangTimesOut(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newNativeAnthropicHangTestService(1)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	resp, pr, pw := newHangingUpstreamResponse()
	start := time.Now()
	res, err := svc.handleResponsesStreamingFromNativeAnthropic(resp, c, "glm-4.7", "glm-4.7", "glm-4.7", nil, start, apicompat.ResponsesClientToolMapping{})
	_ = pw.Close()
	_ = pr.Close()

	if err == nil || !strings.Contains(err.Error(), "stream data interval timeout") {
		t.Fatalf("expected stream timeout error, got %v", err)
	}
	if res == nil {
		t.Fatalf("expected result carrying accumulated usage")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("handler did not respect interval bound: %v", elapsed)
	}
}

func TestCCStreamingFromNativeAnthropic_HappyPathStillConverts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newNativeAnthropicHangTestService(5)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	resp, pr, pw := newHangingUpstreamResponse()
	go func() {
		_, _ = pw.Write([]byte(miniAnthropicSSEStream()))
		_ = pw.Close()
	}()
	defer func() { _ = pr.Close() }()

	res, err := svc.handleCCStreamingFromNativeAnthropic(resp, c, "glm-4.7", "glm-4.7", "glm-4.7", nil, time.Now(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatalf("expected result")
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Hello") {
		t.Fatalf("expected converted text chunk, got %q", body)
	}
	if !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("expected [DONE] terminator, got %q", body)
	}
}

func TestCCBufferedFromNativeAnthropic_HappyPathStillConverts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newNativeAnthropicHangTestService(5)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	resp, pr, pw := newHangingUpstreamResponse()
	go func() {
		_, _ = pw.Write([]byte(miniAnthropicSSEStream()))
		_ = pw.Close()
	}()
	defer func() { _ = pr.Close() }()

	res, err := svc.handleCCBufferedFromNativeAnthropic(resp, c, "glm-4.7", "glm-4.7", "glm-4.7", nil, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatalf("expected result")
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Hello") {
		t.Fatalf("expected converted text in buffered response, got %q", body)
	}
	if res.Usage.InputTokens != 10 || res.Usage.OutputTokens != 5 {
		t.Fatalf("expected usage 10/5, got %+v", res.Usage)
	}
}
