package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
	"github.com/gin-gonic/gin"
)

func runServerTimingRequest(
	t *testing.T,
	enabled bool,
	path string,
	marker string,
	role string,
	handler gin.HandlerFunc,
) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(ServerTiming(enabled))
	engine.Any("/*path", func(c *gin.Context) {
		if role != "" {
			c.Set(string(ContextKeyUserRole), role)
		}
		handler(c)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	if marker != "" {
		request.Header.Set(servertiming.AdminUIHeader, marker)
	}
	engine.ServeHTTP(recorder, request)
	return recorder
}

func TestServerTimingScopesAndRoleGate(t *testing.T) {
	tests := []struct {
		name       string
		enabled    bool
		path       string
		marker     string
		role       string
		wantHeader bool
	}{
		{name: "disabled", enabled: false, path: "/api/v1/admin/users", role: "admin"},
		{name: "admin API path", enabled: true, path: "/api/v1/admin/users", role: "admin", wantHeader: true},
		{name: "shared API marked by admin UI", enabled: true, path: "/api/v1/groups/available", marker: "1", role: "admin", wantHeader: true},
		{name: "non admin role", enabled: true, path: "/api/v1/groups/available", marker: "1", role: "user"},
		{name: "unauthenticated public request", enabled: true, path: "/api/v1/settings/public", marker: "1"},
		{name: "unmarked shared API", enabled: true, path: "/api/v1/groups/available", role: "admin"},
		{name: "invalid marker", enabled: true, path: "/api/v1/groups/available", marker: "true", role: "admin"},
		{name: "admin prefix boundary", enabled: true, path: "/api/v1/administrator", role: "admin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := runServerTimingRequest(t, tt.enabled, tt.path, tt.marker, tt.role, func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"ok": true})
			})
			header := recorder.Header().Get(servertiming.HeaderName)
			if tt.wantHeader && header == "" {
				t.Fatalf("%s header missing", servertiming.HeaderName)
			}
			if !tt.wantHeader && header != "" {
				t.Fatalf("unexpected %s header: %q", servertiming.HeaderName, header)
			}
			if header != "" && (!strings.Contains(header, "total;dur=") || !strings.Contains(header, `cache;desc="bypass"`)) {
				t.Fatalf("incomplete timing header: %q", header)
			}
		})
	}
}

func TestServerTimingCollectorIsRequestScoped(t *testing.T) {
	active := false
	recorder := runServerTimingRequest(t, true, "/api/v1/keys", "1", "admin", func(c *gin.Context) {
		active = servertiming.Active(c.Request.Context())
		c.Status(http.StatusNoContent)
	})
	if !active {
		t.Fatal("collector was not attached to marked request context")
	}
	if recorder.Header().Get(servertiming.HeaderName) == "" {
		t.Fatal("timing header missing from status-only response")
	}
}

func TestServerTimingFinalizesBeforeEarlyCommit(t *testing.T) {
	recorder := runServerTimingRequest(t, true, "/api/v1/admin/stream", "", "admin", func(c *gin.Context) {
		c.Status(http.StatusAccepted)
		c.Writer.WriteHeaderNow()
	})
	if got := recorder.Header().Get(servertiming.HeaderName); got == "" {
		t.Fatal("timing header was not written before response commit")
	}
}

func TestServerTimingFinalizesOnFlush(t *testing.T) {
	recorder := runServerTimingRequest(t, true, "/api/v1/admin/export", "", "admin", func(c *gin.Context) {
		c.Writer.Flush()
	})
	if got := recorder.Header().Get(servertiming.HeaderName); got == "" {
		t.Fatal("timing header was not written before stream flush")
	}
}

func TestServerTimingStatusResponses(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{name: "not modified", status: http.StatusNotModified},
		{name: "internal error", status: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := runServerTimingRequest(t, true, "/api/v1/admin/test", "", "admin", func(c *gin.Context) {
				c.Status(tt.status)
			})
			if recorder.Code != tt.status {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.status)
			}
			if got := recorder.Header().Get(servertiming.HeaderName); got == "" {
				t.Fatalf("timing header missing from status %d response", tt.status)
			}
		})
	}
}

func TestServerTimingResponseWriterUnwraps(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	baseWriter := c.Writer
	writer := &serverTimingResponseWriter{ResponseWriter: baseWriter}
	if got := writer.Unwrap(); got != baseWriter {
		t.Fatalf("Unwrap() = %T, want original Gin writer", got)
	}
}

func TestServerTimingCacheOutcome(t *testing.T) {
	tests := []struct {
		name       string
		headerName string
		value      string
		want       string
	}{
		{name: "snapshot hit", headerName: snapshotCacheHeader, value: "hit", want: "hit"},
		{name: "usage miss", headerName: usageCacheHeader, value: "MISS", want: "miss"},
		{name: "invalid", headerName: snapshotCacheHeader, value: "stale", want: "bypass"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := runServerTimingRequest(t, true, "/api/v1/admin/dashboard", "", "admin", func(c *gin.Context) {
				c.Header(tt.headerName, tt.value)
				c.JSON(http.StatusOK, gin.H{"ok": true})
			})
			want := `cache;desc="` + tt.want + `"`
			if got := recorder.Header().Get(servertiming.HeaderName); !strings.Contains(got, want) {
				t.Fatalf("timing header %q does not contain %q", got, want)
			}
		})
	}
}

func TestServerTimingResponseHeaderForWebSocket(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/ws/qps", nil)
	collector := servertiming.New(time.Now())
	c.Request = c.Request.WithContext(servertiming.WithCollector(c.Request.Context(), collector))
	c.Set(string(ContextKeyUserRole), "admin")

	header := ServerTimingResponseHeader(c)
	if header.Get(servertiming.HeaderName) == "" {
		t.Fatal("WebSocket response header missing timing value")
	}

	c.Set(string(ContextKeyUserRole), "user")
	if got := ServerTimingResponseHeader(c); got != nil {
		t.Fatalf("non-admin WebSocket received timing header: %#v", got)
	}
}
