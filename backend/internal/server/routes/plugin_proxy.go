package routes

import (
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pluginrelay"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

const (
	defaultPluginServiceURL      = "http://plugin-server:8091"
	defaultPluginServicePort     = "8091"
	localhostPluginServiceHost   = "127.0.0.1"
	localhostPluginServiceScheme = "http"
	dockerHostPluginServiceHost  = "host.docker.internal"
)

var pluginServiceDialTimeout = 200 * time.Millisecond
var localhostPluginServiceReachableFunc = localhostPluginServiceReachable
var dockerHostPluginServiceReachableFunc = dockerHostPluginServiceReachable

// RegisterPluginProxyRoutes mounts the plugin-service reverse proxy on the main site.
func RegisterPluginProxyRoutes(r *gin.Engine, upstreamBaseURL string, jwtAuth middleware.JWTAuthMiddleware) {
	if r == nil {
		return
	}

	proxy, err := newPluginReverseProxy(resolvePluginServiceBaseURL(upstreamBaseURL))
	if err != nil {
		return
	}

	handler := gin.WrapH(proxy)
	r.Any("/plugins/*path", func(c *gin.Context) {
		if requiresPluginAuthentication(c.Request) {
			if !authenticatePluginProxyRequest(c, jwtAuth) {
				return
			}
			attachPluginPrincipalHeaders(c.Request, c)
		}
		handler(c)
	})
}

func resolvePluginServiceBaseURL(explicit string) string {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSpace(os.Getenv("PLUGIN_SERVICE_BASE_URL")); trimmed != "" {
		return trimmed
	}
	port := strings.TrimSpace(os.Getenv("PLUGIN_SERVER_PORT"))
	if port == "" {
		port = defaultPluginServicePort
	}
	if localhostPluginServiceReachableFunc(port) {
		return localhostPluginServiceScheme + "://" + localhostPluginServiceHost + ":" + port
	}
	if dockerHostPluginServiceReachableFunc(port) {
		return localhostPluginServiceScheme + "://" + dockerHostPluginServiceHost + ":" + port
	}
	if strings.TrimSpace(os.Getenv("PLUGIN_SERVER_PORT")) != "" {
		return "http://plugin-server:" + port
	}
	return defaultPluginServiceURL
}

func newPluginReverseProxy(baseURL string) (*httputil.ReverseProxy, error) {
	target, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		pluginrelay.StripProxyID(req.Header)
		target = refreshPluginProxyTarget(target)
		originalHost := forwardedHost(req)
		originalProto := forwardedProto(req)
		clientIP := realIP(req.RemoteAddr)
		originalDirector(req)
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host
		req.Header.Set("X-Forwarded-Host", originalHost)
		req.Header.Set("X-Forwarded-Proto", originalProto)
		req.Header.Set("X-Real-IP", clientIP)
		appendForwardedFor(req, clientIP)
	}
	return proxy, nil
}

func refreshPluginProxyTarget(target *url.URL) *url.URL {
	if target == nil || target.Hostname() != "plugin-server" {
		return target
	}
	refreshed, err := url.Parse(resolvePluginServiceBaseURL(""))
	if err != nil || refreshed.Host == "" {
		return target
	}
	return refreshed
}

func forwardedHost(req *http.Request) string {
	if req == nil {
		return ""
	}
	if host := strings.TrimSpace(req.Header.Get("X-Forwarded-Host")); host != "" {
		return host
	}
	return req.Host
}

func forwardedProto(req *http.Request) string {
	if req == nil {
		return "http"
	}
	if proto := strings.TrimSpace(req.Header.Get("X-Forwarded-Proto")); proto != "" {
		return proto
	}
	if req.TLS != nil {
		return "https"
	}
	return "http"
}

func realIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		return strings.TrimSpace(remoteAddr)
	}
	return host
}

func appendForwardedFor(req *http.Request, clientIP string) {
	if req == nil {
		return
	}
	if clientIP == "" {
		return
	}
	if prior := strings.TrimSpace(req.Header.Get("X-Forwarded-For")); prior != "" {
		req.Header.Set("X-Forwarded-For", prior+", "+clientIP)
		return
	}
	req.Header.Set("X-Forwarded-For", clientIP)
}

func authenticatePluginProxyRequest(c *gin.Context, jwtAuth middleware.JWTAuthMiddleware) bool {
	if c == nil || jwtAuth == nil {
		return true
	}
	if strings.TrimSpace(c.GetHeader("Authorization")) == "" {
		if token := firstNonEmptyPluginToken(c.Request); token != "" {
			c.Request.Header.Set("Authorization", "Bearer "+token)
		}
	}
	jwtAuth(c)
	return !c.IsAborted()
}

func firstNonEmptyPluginToken(req *http.Request) string {
	if req == nil {
		return ""
	}
	for _, key := range []string{"token", "session"} {
		if value := strings.TrimSpace(req.URL.Query().Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func attachPluginPrincipalHeaders(req *http.Request, c *gin.Context) {
	if req == nil || c == nil {
		return
	}
	if subject, ok := middleware.GetAuthSubjectFromContext(c); ok && subject.UserID > 0 {
		req.Header.Set("X-Sub2api-User-Id", strconv.FormatInt(subject.UserID, 10))
	}
	if role, ok := middleware.GetUserRoleFromContext(c); ok && strings.TrimSpace(role) != "" {
		req.Header.Set("X-Sub2api-User-Role", strings.TrimSpace(role))
	}
}

func requiresPluginAuthentication(req *http.Request) bool {
	if req == nil || req.URL == nil {
		return false
	}
	path := strings.TrimSpace(req.URL.Path)
	if path == "" {
		return false
	}
	return strings.Contains(path, "/api/")
}

func localhostPluginServiceReachable(port string) bool {
	address := net.JoinHostPort(localhostPluginServiceHost, strings.TrimSpace(port))
	conn, err := net.DialTimeout("tcp", address, pluginServiceDialTimeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func dockerHostPluginServiceReachable(port string) bool {
	address := net.JoinHostPort(dockerHostPluginServiceHost, strings.TrimSpace(port))
	conn, err := net.DialTimeout("tcp", address, pluginServiceDialTimeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
