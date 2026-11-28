package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayFailoverExhausted_RequestTooLarge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, simple := range []bool{false, true} {
		name := "with_body"
		if simple {
			name = "without_body"
		}
		for _, streamStarted := range []bool{false, true} {
			mode := "json"
			if streamStarted {
				mode = "sse"
			}
			t.Run(name+"/"+mode, func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				if streamStarted {
					c.Header("Content-Type", "text/event-stream")
					_, err := c.Writer.Write([]byte("event: ping\ndata: {}\n\n"))
					require.NoError(t, err)
					c.Writer.Flush()
				}

				h := &GatewayHandler{}
				if simple {
					h.handleFailoverExhaustedSimple(c, http.StatusRequestEntityTooLarge, streamStarted)
				} else {
					h.handleFailoverExhausted(c, &service.UpstreamFailoverError{
						StatusCode:   http.StatusRequestEntityTooLarge,
						ResponseBody: []byte(`{"error":{"type":"request_too_large","message":"Request exceeds the maximum allowed size of 32 MB"},"request_id":"private-upstream-id"}`),
					}, service.PlatformAnthropic, streamStarted)
				}

				const wantBody = `{"type":"error","error":{"type":"request_too_large","message":"Request exceeds the upstream size limit"}}`
				if streamStarted {
					require.Equal(t, http.StatusOK, rec.Code)
					errorEvent := strings.TrimPrefix(rec.Body.String(), "event: ping\ndata: {}\n\ndata: ")
					require.JSONEq(t, wantBody, strings.TrimSpace(errorEvent))
					streamErr, ok := service.GetOpsStreamError(c)
					require.True(t, ok)
					require.Equal(t, http.StatusRequestEntityTooLarge, streamErr.IntendedStatus)
					require.Equal(t, "request_too_large", streamErr.ErrType)
				} else {
					require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
					require.JSONEq(t, wantBody, rec.Body.String())
				}
				require.Equal(t, http.StatusRequestEntityTooLarge, c.GetInt(service.OpsUpstreamStatusCodeKey))
			})
		}
	}
}
