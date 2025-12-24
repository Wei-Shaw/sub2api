package middleware

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// enforceCookieOrigin 对基于 Cookie 的状态变更请求执行来源校验，降低 CSRF 风险。
// requireOrigin 为 false 时，允许缺失 Origin/Referer 的请求通过。
func enforceCookieOrigin(c *gin.Context, allowedOrigins []string, requireOrigin bool) bool {
	if !isStateChangingMethod(c.Request.Method) {
		return true
	}

	// 优先使用 Origin，其次回退到 Referer 的 scheme+host。
	origin := requestOrigin(c)
	if origin == "" {
		return !requireOrigin
	}

	// 未配置 allowlist 时仅允许同源请求。
	if len(allowedOrigins) == 0 {
		return normalizeOrigin(origin) == normalizeOrigin(requestBaseOrigin(c))
	}

	for _, allowed := range allowedOrigins {
		if matchAllowedOrigin(origin, allowed) {
			return true
		}
	}
	return false
}

// isStateChangingMethod 标记可能修改服务端状态的 HTTP 方法。
func isStateChangingMethod(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	default:
		return true
	}
}

// requestOrigin 获取请求来源（Origin / Referer），用于来源校验。
func requestOrigin(c *gin.Context) string {
	origin := strings.TrimSpace(c.GetHeader("Origin"))
	if origin != "" {
		return origin
	}
	referer := strings.TrimSpace(c.GetHeader("Referer"))
	if referer == "" {
		return ""
	}
	parsed, err := url.Parse(referer)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

// requestBaseOrigin 组合当前请求的 scheme + host，用于同源比对。
func requestBaseOrigin(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if forwarded := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")); forwarded != "" {
		if parts := strings.Split(forwarded, ","); len(parts) > 0 {
			if candidate := strings.TrimSpace(parts[0]); candidate != "" {
				scheme = candidate
			}
		}
	}
	return scheme + "://" + c.Request.Host
}

// matchAllowedOrigin 根据 allowlist 规则匹配来源，支持 *.example.com 形式。
func matchAllowedOrigin(origin, allowed string) bool {
	allowed = strings.TrimSpace(allowed)
	if allowed == "" {
		return false
	}
	if allowed == "*" {
		return true
	}
	origin = normalizeOrigin(origin)
	allowed = normalizeOrigin(allowed)

	if !strings.Contains(allowed, "*") {
		return origin == allowed
	}

	allowedURL, err := url.Parse(allowed)
	if err != nil {
		return false
	}
	originURL, err := url.Parse(origin)
	if err != nil {
		return false
	}

	if allowedURL.Scheme != "" && allowedURL.Scheme != originURL.Scheme {
		return false
	}

	allowedHost := allowedURL.Host
	if allowedHost == "" {
		allowedHost = allowedURL.Path
	}
	if strings.HasPrefix(allowedHost, "*.") {
		return strings.HasSuffix(originURL.Host, allowedHost[1:])
	}
	return false
}

// normalizeOrigin 统一处理末尾 /，避免字符串比较误差。
func normalizeOrigin(origin string) string {
	return strings.TrimSuffix(origin, "/")
}
