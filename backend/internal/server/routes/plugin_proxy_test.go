package routes

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pluginrelay"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type closeNotifyRecorder struct {
	*httptest.ResponseRecorder
}

func (r *closeNotifyRecorder) CloseNotify() <-chan bool {
	ch := make(chan bool, 1)
	return ch
}

func TestPluginProxyRoutesForwardRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)

	type upstreamResult struct {
		method         string
		path           string
		query          string
		forwardedHost  string
		forwardedProto string
		realIP         string
		auth           string
		userID         string
		userRole       string
		cookie         string
		body           string
	}

	results := make(chan upstreamResult, 3)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		results <- upstreamResult{
			method:         r.Method,
			path:           r.URL.Path,
			query:          r.URL.RawQuery,
			forwardedHost:  r.Header.Get("X-Forwarded-Host"),
			forwardedProto: r.Header.Get("X-Forwarded-Proto"),
			realIP:         r.Header.Get("X-Real-IP"),
			auth:           r.Header.Get("Authorization"),
			userID:         r.Header.Get("X-Sub2api-User-Id"),
			userRole:       r.Header.Get("X-Sub2api-User-Role"),
			cookie:         r.Header.Get("Cookie"),
			body:           string(payload),
		}
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'self' https://main.example.com")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("proxied"))
	}))
	defer upstream.Close()

	router := gin.New()
	RegisterPluginProxyRoutes(router, upstream.URL, nil)

	tests := []struct {
		name      string
		method    string
		target    string
		body      string
		wantPath  string
		wantQuery string
	}{
		{
			name:      "plugin page route",
			method:    http.MethodGet,
			target:    "/plugins/image-generation",
			wantPath:  "/plugins/image-generation",
			wantQuery: "",
		},
		{
			name:      "plugin scoped api route",
			method:    http.MethodPost,
			target:    "/plugins/image-generation/api/generate?mode=fast",
			body:      `{"prompt":"hello"}`,
			wantPath:  "/plugins/image-generation/api/generate",
			wantQuery: "mode=fast",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.target, strings.NewReader(tc.body))
			req.Host = "main.example.com"
			req.RemoteAddr = "203.0.113.10:4567"
			req.Header.Set("Authorization", "Bearer test-token")
			req.Header.Set("Cookie", "session=abc")
			req.Header.Set("X-Forwarded-Proto", "https")
			w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}

			router.ServeHTTP(w, req)

			require.Equal(t, http.StatusCreated, w.Code)
			require.Equal(t, "proxied", w.Body.String())
			require.Equal(t, "frame-ancestors 'self' https://main.example.com", w.Header().Get("Content-Security-Policy"))

			got := <-results
			require.Equal(t, tc.method, got.method)
			require.Equal(t, tc.wantPath, got.path)
			require.Equal(t, tc.wantQuery, got.query)
			require.Equal(t, "main.example.com", got.forwardedHost)
			require.Equal(t, "https", got.forwardedProto)
			require.Equal(t, "203.0.113.10", got.realIP)
			require.Equal(t, "Bearer test-token", got.auth)
			require.Equal(t, "session=abc", got.cookie)
			require.Equal(t, tc.body, got.body)
		})
	}
}

func TestResolvePluginServiceBaseURL(t *testing.T) {
	originalTimeout := pluginServiceDialTimeout
	pluginServiceDialTimeout = 10 * time.Millisecond
	originalReachable := localhostPluginServiceReachableFunc
	localhostPluginServiceReachableFunc = func(string) bool { return false }
	t.Cleanup(func() {
		pluginServiceDialTimeout = originalTimeout
		localhostPluginServiceReachableFunc = originalReachable
	})

	t.Setenv("PLUGIN_SERVICE_BASE_URL", "")
	t.Setenv("PLUGIN_SERVER_PORT", "")
	require.Equal(t, "http://plugin-server:8091", resolvePluginServiceBaseURL(""))

	t.Setenv("PLUGIN_SERVER_PORT", "18091")
	require.Equal(t, "http://plugin-server:18091", resolvePluginServiceBaseURL(""))

	t.Setenv("PLUGIN_SERVICE_BASE_URL", "http://custom-plugin:19091")
	require.Equal(t, "http://custom-plugin:19091", resolvePluginServiceBaseURL(""))

	require.Equal(t, "http://explicit-plugin:28091", resolvePluginServiceBaseURL("http://explicit-plugin:28091"))
}

func TestResolvePluginServiceBaseURLPrefersReachableLocalhost(t *testing.T) {
	originalReachable := localhostPluginServiceReachableFunc
	localhostPluginServiceReachableFunc = func(string) bool { return true }
	t.Cleanup(func() {
		localhostPluginServiceReachableFunc = originalReachable
	})

	port := strconv.Itoa(19091)
	t.Setenv("PLUGIN_SERVICE_BASE_URL", "")
	t.Setenv("PLUGIN_SERVER_PORT", port)

	require.Equal(t, "http://127.0.0.1:"+port, resolvePluginServiceBaseURL(""))
}

func TestPluginProxyRoutesReResolvesLocalPluginServiceWhenDefaultHostFails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalLocalReachable := localhostPluginServiceReachableFunc
	originalDockerHostReachable := dockerHostPluginServiceReachableFunc
	localhostPluginServiceReachableFunc = func(string) bool { return false }
	dockerHostPluginServiceReachableFunc = func(string) bool { return false }
	t.Cleanup(func() {
		localhostPluginServiceReachableFunc = originalLocalReachable
		dockerHostPluginServiceReachableFunc = originalDockerHostReachable
	})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("proxied"))
	}))
	defer upstream.Close()

	port := strings.TrimPrefix(upstream.URL, "http://127.0.0.1:")
	if strings.Contains(port, ":") {
		t.Fatalf("test server URL %q did not use IPv4 localhost", upstream.URL)
	}

	router := gin.New()
	t.Setenv("PLUGIN_SERVICE_BASE_URL", "")
	t.Setenv("PLUGIN_SERVER_PORT", port)
	RegisterPluginProxyRoutes(router, "", nil)

	localhostPluginServiceReachableFunc = func(string) bool { return true }

	req := httptest.NewRequest(http.MethodGet, "/plugins/image-generation", nil)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "proxied", w.Body.String())
}

func TestResolvePluginServiceBaseURLPrefersReachableDockerHost(t *testing.T) {
	originalLocalReachable := localhostPluginServiceReachableFunc
	originalDockerHostReachable := dockerHostPluginServiceReachableFunc
	localhostPluginServiceReachableFunc = func(string) bool { return false }
	dockerHostPluginServiceReachableFunc = func(string) bool { return true }
	t.Cleanup(func() {
		localhostPluginServiceReachableFunc = originalLocalReachable
		dockerHostPluginServiceReachableFunc = originalDockerHostReachable
	})

	port := strconv.Itoa(19091)
	t.Setenv("PLUGIN_SERVICE_BASE_URL", "")
	t.Setenv("PLUGIN_SERVER_PORT", port)

	require.Equal(t, "http://host.docker.internal:"+port, resolvePluginServiceBaseURL(""))
}

func TestPluginProxyRoutesAuthenticateEmbeddedTokenBeforeForwarding(t *testing.T) {
	gin.SetMode(gin.TestMode)

	results := make(chan map[string]string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		results <- map[string]string{
			"auth":     r.Header.Get("Authorization"),
			"user_id":  r.Header.Get("X-Sub2api-User-Id"),
			"userRole": r.Header.Get("X-Sub2api-User-Role"),
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	router := gin.New()
	RegisterPluginProxyRoutes(router, upstream.URL, func(c *gin.Context) {
		require.Equal(t, "Bearer embedded-token", c.GetHeader("Authorization"))
		c.Set("user", middleware.AuthSubject{UserID: 42})
		c.Set("user_role", "admin")
		c.Next()
	})

	req := httptest.NewRequest(http.MethodGet, "/plugins/image-generation/api/me?token=embedded-token", nil)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	got := <-results
	require.Equal(t, "Bearer embedded-token", got["auth"])
	require.Equal(t, "42", got["user_id"])
	require.Equal(t, "admin", got["userRole"])
}

func TestPluginProxyRoutesSkipAuthenticationForStaticAssets(t *testing.T) {
	gin.SetMode(gin.TestMode)

	results := make(chan map[string]string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		results <- map[string]string{
			"path":      r.URL.Path,
			"auth":      r.Header.Get("Authorization"),
			"user_id":   r.Header.Get("X-Sub2api-User-Id"),
			"user_role": r.Header.Get("X-Sub2api-User-Role"),
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("asset"))
	}))
	defer upstream.Close()

	router := gin.New()
	authCalls := 0
	RegisterPluginProxyRoutes(router, upstream.URL, func(c *gin.Context) {
		authCalls++
		c.AbortWithStatus(http.StatusUnauthorized)
	})

	req := httptest.NewRequest(http.MethodGet, "/plugins/image-generation/assets/app.js?v=123", nil)
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "asset", w.Body.String())
	require.Equal(t, 0, authCalls)
	got := <-results
	require.Equal(t, "/plugins/image-generation/assets/app.js", got["path"])
	require.Empty(t, got["auth"])
	require.Empty(t, got["user_id"])
	require.Empty(t, got["user_role"])
}

func TestPluginProxyRoutesStripReservedProxyHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)

	forwardedProxyID := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwardedProxyID <- r.Header.Get(pluginrelay.ProxyIDHeader)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	router := gin.New()
	RegisterPluginProxyRoutes(router, upstream.URL, nil)
	req := httptest.NewRequest(http.MethodPost, "/plugins/ai-relay/agnes/1", nil)
	req.Header.Set(pluginrelay.ProxyIDHeader, "999")
	w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.Empty(t, <-forwardedProxyID)
}
