package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

var handlerStructuredLogCaptureMu sync.Mutex

type handlerInMemoryLogSink struct {
	mu     sync.Mutex
	events []*logger.LogEvent
}

func (s *handlerInMemoryLogSink) WriteLogEvent(event *logger.LogEvent) {
	if event == nil {
		return
	}
	cloned := *event
	if event.Fields != nil {
		cloned.Fields = make(map[string]any, len(event.Fields))
		for k, v := range event.Fields {
			cloned.Fields[k] = v
		}
	}
	s.mu.Lock()
	s.events = append(s.events, &cloned)
	s.mu.Unlock()
}

func (s *handlerInMemoryLogSink) ContainsMessageAtLevel(substr, level string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	wantLevel := strings.ToLower(strings.TrimSpace(level))
	for _, ev := range s.events {
		if ev == nil {
			continue
		}
		if strings.Contains(ev.Message, substr) && strings.ToLower(strings.TrimSpace(ev.Level)) == wantLevel {
			return true
		}
	}
	return false
}

func (s *handlerInMemoryLogSink) ContainsFieldValue(field, substr string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ev := range s.events {
		if ev == nil || ev.Fields == nil {
			continue
		}
		if v, ok := ev.Fields[field]; ok && strings.Contains(fmt.Sprint(v), substr) {
			return true
		}
	}
	return false
}

func captureHandlerStructuredLog(t *testing.T) (*handlerInMemoryLogSink, func()) {
	t.Helper()
	handlerStructuredLogCaptureMu.Lock()

	err := logger.Init(logger.InitOptions{
		Level:       "debug",
		Format:      "json",
		ServiceName: "sub2api",
		Environment: "test",
		Output: logger.OutputOptions{
			ToStdout: true,
			ToFile:   false,
		},
		Sampling: logger.SamplingOptions{Enabled: false},
	})
	require.NoError(t, err)

	sink := &handlerInMemoryLogSink{}
	logger.SetSink(sink)
	return sink, func() {
		logger.SetSink(nil)
		handlerStructuredLogCaptureMu.Unlock()
	}
}

func TestIsOpenAIRemoteCompactPath(t *testing.T) {
	require.False(t, isOpenAIRemoteCompactPath(nil))

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", nil)
	require.True(t, isOpenAIRemoteCompactPath(c))

	c.Request = httptest.NewRequest(http.MethodPost, "/responses/compact/", nil)
	require.True(t, isOpenAIRemoteCompactPath(c))

	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	require.False(t, isOpenAIRemoteCompactPath(c))
}

func TestLogOpenAIRemoteCompactOutcome_Succeeded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logSink, restore := captureHandlerStructuredLog(t)
	defer restore()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", nil)
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.125.0")
	c.Request.Header.Set("originator", "codex_cli_rs")
	c.Request.Header.Set("CF-Ray", "9fb7c368a8031fc4-HKG")
	c.Set(openAICompactRequestBodyBytesKey, 1234)
	c.Set(openAICompactRequestContentLengthKey, int64(1234))
	c.Set(openAICompactReadBodyDurationMsKey, int64(7))
	c.Set(openAICompactRequestModelKey, "gpt-5.5")
	c.Set(openAICompactUpstreamModelKey, "gpt-5.4")
	c.Set(openAICompactUpstreamURLHostKey, "chatgpt.com")
	c.Set(openAICompactUpstreamURLPathKey, "/backend-api/codex/responses/compact")
	c.Set(openAICompactUpstreamStatusCodeKey, http.StatusOK)
	c.Set(openAICompactUpstreamErrorClassKey, "none")
	c.Set(openAICompactUpstreamHeaderDurationMsKey, int64(55))
	c.Set(openAICompactUpstreamTotalDurationMsKey, int64(88))
	c.Set(opsModelKey, "gpt-5.3-codex")
	c.Set(opsAccountIDKey, int64(123))
	c.Header("x-request-id", "rid-compact-ok")
	c.Status(http.StatusOK)

	h := &OpenAIGatewayHandler{}
	h.logOpenAIRemoteCompactOutcome(c, time.Now().Add(-8*time.Millisecond))

	require.True(t, logSink.ContainsMessageAtLevel("codex.remote_compact.succeeded", "info"))
	require.True(t, logSink.ContainsFieldValue("compact_outcome", "succeeded"))
	require.True(t, logSink.ContainsFieldValue("status_code", "200"))
	require.True(t, logSink.ContainsFieldValue("path", "/v1/responses/compact"))
	require.True(t, logSink.ContainsFieldValue("request_id", "rid-compact-ok"))
	require.True(t, logSink.ContainsFieldValue("cf_ray", "9fb7c368a8031fc4-HKG"))
	require.True(t, logSink.ContainsFieldValue("cf_colo", "HKG"))
	require.True(t, logSink.ContainsFieldValue("originator", "codex_cli_rs"))
	require.True(t, logSink.ContainsFieldValue("request_body_bytes", "1234"))
	require.True(t, logSink.ContainsFieldValue("request_content_length", "1234"))
	require.True(t, logSink.ContainsFieldValue("request_body_read_ms", "7"))
	require.True(t, logSink.ContainsFieldValue("request_model", "gpt-5.5"))
	require.True(t, logSink.ContainsFieldValue("upstream_model", "gpt-5.4"))
	require.True(t, logSink.ContainsFieldValue("upstream_url_host", "chatgpt.com"))
	require.True(t, logSink.ContainsFieldValue("upstream_url_path", "/backend-api/codex/responses/compact"))
	require.True(t, logSink.ContainsFieldValue("upstream_status_code", "200"))
	require.True(t, logSink.ContainsFieldValue("upstream_error_class", "none"))
	require.True(t, logSink.ContainsFieldValue("upstream_header_ms", "55"))
	require.True(t, logSink.ContainsFieldValue("upstream_total_ms", "88"))
	require.True(t, logSink.ContainsFieldValue("codex_official_client_match", "true"))
	require.True(t, logSink.ContainsFieldValue("codex_cli_client_match", "true"))
	require.True(t, logSink.ContainsFieldValue("ops_request_model", "gpt-5.3-codex"))
	require.True(t, logSink.ContainsFieldValue("account_id", "123"))
	require.True(t, logSink.ContainsFieldValue("upstream_request_id", "rid-compact-ok"))
}

func TestLogOpenAIRemoteCompactOutcome_Failed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logSink, restore := captureHandlerStructuredLog(t)
	defer restore()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/responses/compact", nil)
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.125.0")
	c.Status(http.StatusBadGateway)

	h := &OpenAIGatewayHandler{}
	h.logOpenAIRemoteCompactOutcome(c, time.Now())

	require.True(t, logSink.ContainsMessageAtLevel("codex.remote_compact.failed", "warn"))
	require.True(t, logSink.ContainsFieldValue("compact_outcome", "failed"))
	require.True(t, logSink.ContainsFieldValue("status_code", "502"))
	require.True(t, logSink.ContainsFieldValue("path", "/responses/compact"))
}

func TestLogOpenAIRemoteCompactOutcome_NonCompactSkips(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logSink, restore := captureHandlerStructuredLog(t)
	defer restore()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Status(http.StatusOK)

	h := &OpenAIGatewayHandler{}
	h.logOpenAIRemoteCompactOutcome(c, time.Now())

	require.False(t, logSink.ContainsMessageAtLevel("codex.remote_compact.succeeded", "info"))
	require.False(t, logSink.ContainsMessageAtLevel("codex.remote_compact.failed", "warn"))
}

func TestOpenAIResponses_CompactUnauthorizedLogsFailed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logSink, restore := captureHandlerStructuredLog(t)
	defer restore()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", strings.NewReader(`{"model":"gpt-5.3-codex"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.125.0")

	h := &OpenAIGatewayHandler{}
	h.Responses(c)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.True(t, logSink.ContainsMessageAtLevel("codex.remote_compact.failed", "warn"))
	require.True(t, logSink.ContainsFieldValue("status_code", "401"))
	require.True(t, logSink.ContainsFieldValue("path", "/v1/responses/compact"))
}
