//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGeminiNativeReadFailoverBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	readErr := errors.New("read tcp 10.0.0.1:1234->52.1.2.3:443: stream ID 33; INTERNAL_ERROR")
	for _, tc := range []struct {
		name     string
		stream   bool
		prefix   string
		cancel   bool
		failover bool
	}{
		{name: "buffered before output", failover: true},
		{name: "buffered partial body", prefix: `{"candidates":[{"content":{"parts":[{"text":"partial"}]}}]}`, failover: true},
		{name: "stream before output", stream: true, failover: true},
		{name: "stream after semantic output", stream: true, prefix: "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"hello\"}]}}]}\n\n"},
		{name: "stream after committed keepalive", stream: true, prefix: ": ping\n\n"},
		{name: "buffered canceled", cancel: true},
		{name: "stream canceled", stream: true, cancel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			ctx := context.Background()
			if tc.cancel {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini:streamGenerateContent", nil).WithContext(ctx)
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"X-Request-Id": []string{"gemini-read-boundary"}},
				Body:       io.NopCloser(&errAfterReader{data: strings.NewReader(tc.prefix), err: readErr}),
			}
			svc := &GeminiMessagesCompatService{}
			var err error
			if tc.stream {
				_, err = svc.handleNativeStreamingResponse(c, resp, time.Now(), false, &Account{ID: 1}, "gemini-read-boundary")
			} else {
				_, err = svc.handleNativeNonStreamingResponse(c, resp, false, &Account{ID: 1}, "gemini-read-boundary")
			}
			var failover *UpstreamFailoverError
			if tc.failover {
				require.ErrorAs(t, err, &failover)
				require.Equal(t, http.StatusBadGateway, failover.StatusCode)
				require.True(t, failover.RetryableOnSameAccount)
				require.Equal(t, "gemini-read-boundary", failover.ResponseHeaders.Get("x-request-id"))
				require.Equal(t, "UNAVAILABLE", gjson.GetBytes(failover.ResponseBody, "error.status").String())
				require.NotContains(t, string(failover.ResponseBody), "stream ID 33")
				require.Empty(t, rec.Body.String())
				require.False(t, c.Writer.Written())
			} else {
				require.Error(t, err)
				require.NotErrorAs(t, err, &failover)
			}
		})
	}
}
