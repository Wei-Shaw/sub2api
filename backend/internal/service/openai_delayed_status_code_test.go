//go:build unit

package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newDelayedStatusCodeTestContext(t *testing.T, group *Group) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	if group != nil {
		c.Set("api_key", &APIKey{Group: group})
	}
	return c, rec
}

func TestOpenAIDelayedStatusCodeEnabledDefaultsFalse(t *testing.T) {
	c, _ := newDelayedStatusCodeTestContext(t, nil)
	require.False(t, openAIDelayedStatusCodeEnabled(c), "无 apiKey 时必须默认关闭")
}

func TestOpenAIDelayedStatusCodeEnabledFalseWhenSwitchOff(t *testing.T) {
	c, _ := newDelayedStatusCodeTestContext(t, &Group{DelayedStatusCode: false})
	require.False(t, openAIDelayedStatusCodeEnabled(c))
}

func TestOpenAIDelayedStatusCodeEnabledTrueWhenSwitchOn(t *testing.T) {
	c, _ := newDelayedStatusCodeTestContext(t, &Group{DelayedStatusCode: true})
	require.True(t, openAIDelayedStatusCodeEnabled(c))
}

func TestOpenAIDelayedStatusCodeEnabledNilContext(t *testing.T) {
	require.False(t, openAIDelayedStatusCodeEnabled(nil))
}

func TestOpenAIDelayedStatusCodeEnabledNilGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set("api_key", &APIKey{Group: nil})
	require.False(t, openAIDelayedStatusCodeEnabled(c))
}

// keepaliveTestBody 先发 preamble，静默超过一个心跳周期制造心跳触发窗口，再发终止事件。
func keepaliveTestBody(t *testing.T, terminal string) io.ReadCloser {
	t.Helper()
	pr, pw := io.Pipe()
	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_ka\"}}\n\n"))
		time.Sleep(1500 * time.Millisecond)
		_, _ = pw.Write([]byte(terminal))
	}()
	return pr
}

const keepaliveTestErrorEvent = "data: {\"type\":\"error\",\"error\":{\"type\":\"service_unavailable_error\",\"code\":\"server_is_overloaded\",\"message\":\"overloaded\"}}\n\n"

func newKeepaliveTestService() *OpenAIGatewayService {
	return &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		MaxLineSize:             defaultMaxLineSize,
		StreamKeepaliveInterval: 1,
	}}}
}

// 开关打开时，首个语义输出前的心跳必须被抑制：期间不得有任何字节写给客户端，
// 否则 200 落地、后续 failover 失效。
func TestOpenAINativeStreamKeepaliveSuppressedWhenDelayedStatusCode(t *testing.T) {
	svc := newKeepaliveTestService()
	c, rec := newDelayedStatusCodeTestContext(t, &Group{DelayedStatusCode: true})

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       keepaliveTestBody(t, keepaliveTestErrorEvent),
	}
	_, err := svc.handleStreamingResponse(
		c.Request.Context(), resp, c,
		&Account{ID: 1, Platform: PlatformOpenAI}, time.Now(), "model", "model",
	)

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Empty(t, rec.Body.String(), "开关打开时心跳必须被抑制")
}

// 开关关闭时行为与现状一致：心跳照常发出。
func TestOpenAINativeStreamKeepaliveSentWhenSwitchOff(t *testing.T) {
	svc := newKeepaliveTestService()
	c, rec := newDelayedStatusCodeTestContext(t, &Group{DelayedStatusCode: false})

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       keepaliveTestBody(t, keepaliveTestErrorEvent),
	}
	_, _ = svc.handleStreamingResponse(
		c.Request.Context(), resp, c,
		&Account{ID: 1, Platform: PlatformOpenAI}, time.Now(), "model", "model",
	)

	require.Contains(t, rec.Body.String(), ":", "开关关闭时心跳应照常发出")
}
