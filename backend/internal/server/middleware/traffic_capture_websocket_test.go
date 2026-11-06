package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/gin-gonic/gin"
)

func TestTrafficHijackImmediatelyDrainsCallbacks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(TrafficCapture(), ResponseCompress())
	results := make(chan *traffic.Usage, 2)
	release := make(chan struct{})
	engine.GET("/", func(c *gin.Context) {
		traffic.AfterFinalize(c.Request.Context(), func(u *traffic.Usage) { results <- u })
		conn, rw, err := c.Writer.Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")
		_ = rw.Flush()
		// Usage emitted by a WS turn must not wait for session shutdown.
		traffic.AfterFinalize(c.Request.Context(), func(u *traffic.Usage) { results <- u })
		<-release
	})
	server := httptest.NewServer(engine)
	defer server.Close()
	defer close(release)
	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatal(resp.StatusCode)
	}
	for range 2 {
		select {
		case usage := <-results:
			if usage != nil {
				t.Fatal("hijacked body must be unknown")
			}
		case <-time.After(time.Second):
			t.Fatal("usage callback waited for WebSocket session shutdown")
		}
	}
}

func TestTrafficSwitchingProtocolsWithoutHijackIsExcluded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(TrafficCapture(), ResponseCompress())
	called := false
	engine.GET("/", func(c *gin.Context) {
		traffic.AfterFinalize(c.Request.Context(), func(u *traffic.Usage) {
			called = true
			if u != nil {
				t.Fatal("101 was measured as HTTP payload")
			}
		})
		c.Status(http.StatusSwitchingProtocols)
	})
	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if !called {
		t.Fatal("callback missing")
	}
}
