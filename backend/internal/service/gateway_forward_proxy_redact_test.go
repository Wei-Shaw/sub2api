//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayService_Forward_LogRedactsProxyPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logSink, restore := captureStructuredLog(t)
	defer restore()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	body := []byte(`{"model":"claude-3-5-sonnet-latest","messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
	require.NoError(t, err)

	upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-3-5-sonnet-latest","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":1,"output_tokens":1}}`)),
	}}
	svc := newForwardPartialUsageServiceForTest(upstream)
	account := newAnthropicOAuthAccountForPartialUsageTest()
	proxyID := int64(9)
	account.ProxyID = &proxyID
	account.Proxy = &Proxy{ID: proxyID, Protocol: "http", Host: "10.0.0.1", Port: 3128, Username: "proxyuser", Password: "s3cretpass"}

	_, err = svc.Forward(context.Background(), c, account, parsed)
	require.NoError(t, err)

	require.True(t, logSink.ContainsMessage("Proxy=http://proxyuser:xxxxx@10.0.0.1:3128"))
	logSink.mu.Lock()
	defer logSink.mu.Unlock()
	for _, ev := range logSink.events {
		require.NotContains(t, ev.Message, "s3cretpass")
	}
}
