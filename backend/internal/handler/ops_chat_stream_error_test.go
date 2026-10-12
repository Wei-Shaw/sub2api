package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpsChatStreamErrorEnvelope(t *testing.T) {
	for _, tc := range []struct {
		name, frame string
		failure     bool
	}{
		{"nested error", "data: {\"error\":{\"message\":\"busy\",\"code\":\"service_unavailable\"}}\n\n", true},
		{"typed nested error CRLF", "data: {\"error\":{\"type\":\"upstream_error\",\"message\":\"busy\"}}\r\n\r\n", true},
		{"normal text", "data: {\"choices\":[{\"delta\":{\"content\":\"error\"}}]}\n\n", false},
		{"null error", "data: {\"error\":null,\"choices\":[]}\n\n", false},
		{"empty error", "data: {\"error\":{}}\n\n", false},
		{"nested user data", "data: {\"choices\":[{\"delta\":{\"error\":{\"message\":\"text\"}}}]}\n\n", false},
		{"non-error event", "data: {\"type\":\"response.completed\",\"error\":null}\n\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for split := 0; split <= len(tc.frame); split++ {
				state := &opsCaptureWriterState{limit: opsCaptureWriterLimit}
				state.captureResponseChunk([]byte(tc.frame[:split]), http.StatusOK)
				state.captureResponseChunk([]byte(tc.frame[split:]), http.StatusOK)
				state.finalizeResponseCapture()
				parsed, ok := parseOpsSSEFailure(state.buf.Bytes())
				require.Equal(t, tc.failure, ok, "split %d", split)
				if tc.failure {
					require.True(t, parsed.StreamFailure)
					require.Equal(t, "busy", parsed.Message)
				} else {
					require.Empty(t, state.buf.Bytes(), "ordinary response must not be persisted")
				}
			}
		})
	}
}

func TestOpsChatStreamFailureIsNotRecovered(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 2)
	gin.SetMode(gin.TestMode)
	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(OpsErrorLoggerMiddleware(ops))
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		setOpsRequestContext(c, "gpt-5.6-sol", true)
		service.SetOpsUpstreamError(c, http.StatusBadGateway, "busy", "")
		c.Status(http.StatusOK)
		_, _ = c.Writer.WriteString("data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n")
		_, _ = c.Writer.WriteString("data: {\"error\":{\"type\":\"upstream_error\",\"message\":\"busy\"}}\n\n")
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	require.Equal(t, http.StatusOK, rec.Code, "HTTP headers were already committed")
	require.Equal(t, int64(1), OpsErrorLogQueueLength())
	job := <-opsErrorLogQueue
	require.Equal(t, http.StatusBadGateway, job.entry.StatusCode)
	require.Equal(t, "busy", job.entry.ErrorMessage)
	require.NotContains(t, job.entry.ErrorMessage, "Recovered")
}
