package service

// issue #7955 回归测试：上游只发心跳（`: ping` / `event: ping` /
// `data: {"type":"ping"}`）而无真实数据时，stream_data_interval_timeout
// 必须按时熔断，而不是被心跳行无限续命。

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAnthropicSSELineIsHeartbeat(t *testing.T) {
	cases := []struct {
		line      string
		heartbeat bool
	}{
		{"", true},
		{"\n", true},
		{": ping", true},
		{":ping", true},
		{": comments allowed", true},
		{"event: ping", true},
		{"event:ping", true},
		{"event: ping\n", true},
		{`data: {"type": "ping"}`, true},
		{`data: {"type":"ping"}`, true},
		{`data:`, true},
		{`data:   `, true},
		{"event: message_start", false},
		{"event: message_stop", false},
		{"event: content_block_delta", false},
		{`data: {"type":"message_start","message":{"usage":{"input_tokens":3}}}`, false},
		{`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"hi"}}`, false},
		{`data: {"type":"message_stop"}`, false},
		{"data: [DONE]", false},
		{"data: plain-text", false},
		{"garbage-line", false},
		{`  data: {"type":"ping"}  `, true},
		{`  event: ping  `, true},
	}
	for _, tc := range cases {
		if got := anthropicSSELineIsHeartbeat(tc.line); got != tc.heartbeat {
			t.Errorf("anthropicSSELineIsHeartbeat(%q) = %v, want %v", tc.line, got, tc.heartbeat)
		}
	}
}

func newHeartbeatHangTestService() *GatewayService {
	return &GatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				StreamDataIntervalTimeout: 1,
				StreamKeepaliveInterval:   0,
				MaxLineSize:               defaultMaxLineSize,
			},
		},
		rateLimitService: &RateLimitService{},
	}
}

// TestGatewayService_StreamingIntervalTimeoutFiresOnPingOnly 上游先给一个真实事件，
// 之后只发心跳且不断连：必须在 interval 附近熔断，而不是永久挂住。
func TestGatewayService_StreamingIntervalTimeoutFiresOnPingOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newHeartbeatHangTestService()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":3}}}\n\n"))
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				// 只发心跳、不断连：模拟 issue #7955 的上游静默。
				if _, err := pw.Write([]byte("event: ping\ndata: {\"type\": \"ping\"}\n\n")); err != nil {
					return
				}
			}
		}
	}()
	defer func() { _ = pr.Close() }()

	type result struct {
		res *streamingResult
		err error
	}
	ch := make(chan result, 1)
	go func() {
		res, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
		ch <- result{res, err}
	}()

	select {
	case r := <-ch:
		require.Error(t, r.err)
		require.Contains(t, r.err.Error(), "stream data interval timeout")
	case <-time.After(8 * time.Second):
		t.Fatal("handleStreamingResponse did not time out on ping-only upstream")
	}
}

// TestAnthropicPassthrough_IntervalTimeoutFiresOnPingOnly 同上，覆盖 APIKey 透传泵。
func TestAnthropicPassthrough_IntervalTimeoutFiresOnPingOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newHeartbeatHangTestService()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":3}}}\n\n"))
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if _, err := pw.Write([]byte(": ping\n\n")); err != nil {
					return
				}
			}
		}
	}()
	defer func() { _ = pr.Close() }()

	type result struct {
		res *streamingResult
		err error
	}
	ch := make(chan result, 1)
	go func() {
		res, err := svc.handleStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model")
		ch <- result{res, err}
	}()

	select {
	case r := <-ch:
		require.Error(t, r.err)
		require.Contains(t, r.err.Error(), "stream data interval timeout")
	case <-time.After(8 * time.Second):
		t.Fatal("passthrough did not time out on ping-only upstream")
	}
}

// TestAnthropicNativeLinePump_PingOnlyStillTimesOut 心跳持续流动时也必须超时。
// 旧语义下本测试恒失败（timer 被每一行重置）。
func TestAnthropicNativeLinePump_PingOnlyStillTimesOut(t *testing.T) {
	pr, pw := io.Pipe()
	scanner := bufio.NewScanner(pr)
	pump := newAnthropicNativeLinePump(scanner, 1*time.Second)
	defer pump.stop()
	defer func() { _ = pr.Close() }()

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		defer func() { _ = pw.Close() }()
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if _, err := pw.Write([]byte("event: ping\n")); err != nil {
					return
				}
			}
		}
	}()

	deadline := time.Now().Add(6 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("line pump did not time out while pings kept flowing")
		}
		_, err := pump.next()
		if err == nil {
			continue
		}
		if strings.Contains(err.Error(), "stream data interval timeout") {
			return
		}
		t.Fatalf("unexpected pump error: %v", err)
	}
}
