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
)

func TestGatewayCCBufferedUpstreamReadErrorDoesNotPublishPartialSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const partial = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_partial\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"claude-sonnet-4-6\",\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n"
	for _, tt := range []struct {
		name string
		body string
	}{
		{"before any event", ""},
		{"after message start", partial},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"X-Request-Id": []string{"upstream-cc-read"}}, Body: io.NopCloser(&errAfterReader{data: strings.NewReader(tt.body), err: errors.New("stream error: stream ID 7; INTERNAL_ERROR")})}
			result, err := (&GatewayService{}).handleCCBufferedFromAnthropic(resp, c, "gpt-5", "claude-sonnet-4-6", nil, time.Now())
			require.Nil(t, result)
			var failover *UpstreamFailoverError
			require.ErrorAs(t, err, &failover)
			require.True(t, failover.ShouldRetryNextAccount())
			require.Equal(t, "upstream-cc-read", failover.ResponseHeaders.Get("x-request-id"))
			require.NotContains(t, string(failover.ResponseBody), "stream ID 7")
			require.False(t, c.Writer.Written())
			require.Empty(t, rec.Body.String())
		})
	}
}

func TestGatewayCCBufferedCanceledRequestDoesNotFailOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(&errAfterReader{data: strings.NewReader(""), err: io.ErrUnexpectedEOF})}
	_, err := (&GatewayService{}).handleCCBufferedFromAnthropic(resp, c, "gpt-5", "claude-sonnet-4-6", nil, time.Now())
	require.Error(t, err)
	var failover *UpstreamFailoverError
	require.NotErrorAs(t, err, &failover)
}
