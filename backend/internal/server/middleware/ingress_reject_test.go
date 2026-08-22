package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type ingressRejectRecorderStub struct {
	mu       sync.Mutex
	calls    int
	clientIP string
}

func (r *ingressRejectRecorderStub) RecordIngressReject(_, _, _, clientIP string, _, _ int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.clientIP = clientIP
}

func TestNormalizeIngressRejectIP(t *testing.T) {
	require.Equal(t, "2001:db8:abcd:1234::", normalizeIngressRejectIP("2001:db8:abcd:1234:ffff::1"))
	require.Equal(t, "192.0.2.4", normalizeIngressRejectIP("::ffff:192.0.2.4"))
	require.Equal(t, "0.0.0.0", normalizeIngressRejectIP("not-an-ip"))
}

func TestLoggerRecordsIngressRejectOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &ingressRejectRecorderStub{}
	SetIngressRejectRecorder(recorder)
	t.Cleanup(func() { SetIngressRejectRecorder(nil) })
	router := gin.New()
	router.Use(Logger())
	router.GET("/v1/messages", func(c *gin.Context) {
		MarkIngressRejected(c, IngressRejectInvalidAPIKey)
		c.Status(http.StatusUnauthorized)
	})
	request := httptest.NewRequest(http.MethodGet, "/v1/messages", nil)
	request.RemoteAddr = "[2001:db8:abcd:1234:ffff::1]:1234"
	router.ServeHTTP(httptest.NewRecorder(), request)
	recorder.mu.Lock()
	require.Equal(t, 1, recorder.calls)
	require.Equal(t, "2001:db8:abcd:1234::", recorder.clientIP)
	recorder.mu.Unlock()
}
