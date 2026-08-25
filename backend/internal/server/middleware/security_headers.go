package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
)

const (
	// CSPNonceKey is the context key for storing the CSP nonce
	CSPNonceKey = "csp_nonce"
	// NonceTemplate is the placeholder in CSP policy for nonce
	NonceTemplate = "__CSP_NONCE__"
	// CloudflareInsightsDomain is the domain for Cloudflare Web Analytics
	CloudflareInsightsDomain = "https://static.cloudflareinsights.com"
	// TencentCaptchaDomain is the Tencent Captcha 2.0 Web SDK domain (Chinese mainland site).
	TencentCaptchaDomain = "https://turing.captcha.qcloud.com"
	// TencentCaptchaStaticDomain is the Tencent Captcha static asset domain.
	TencentCaptchaStaticDomain = "https://*.captcha.gtimg.com"
	// TencentCaptchaCDNDomain 是天御国内站的核心 JS CDN 主机：
	// 入口脚本 TJCaptcha.js 会再从这里加载 /1/tgJCap.*.js，缺失时会被 script-src 拦截。
	TencentCaptchaCDNDomain = "https://turing.captcha.gtimg.com"
	// TencentCaptchaGlobalDomain 是天御国际站的 Web SDK 与验证弹窗 iframe 主机。
	TencentCaptchaGlobalDomain = "https://ca.turing.captcha.qcloud.com"
	// TencentCaptchaGlobalCDNDomain 是天御国际站的核心 JS CDN 主机。
	TencentCaptchaGlobalCDNDomain = "https://global.turing.captcha.gtimg.com"
	// TencentCaptchaPrehandleDomain 是天御 SDK 动态预处理脚本与预处理接口主机。
	TencentCaptchaPrehandleDomain = "https://www.tycaptcha.com"
	// TencentCaptchaJQueryDomain 是国内站入口脚本动态加载的 jQuery CDN 主机。
	TencentCaptchaJQueryDomain = "https://cloudcache.tencentcs.com"
	// TencentCaptchaRceDomain 是国际站风控校验接口主机。
	TencentCaptchaRceDomain = "https://rce.tencentrio.com"
	// TencentCaptchaWorkerSource 是天御国际站创建验证码 Web Worker 时使用的来源。
	TencentCaptchaWorkerSource = "blob:"
	// StripeDomain is the domain for Stripe.js SDK
	StripeDomain = "https://*.stripe.com"
	// AirwallexStaticDomain 是 Airwallex 生产环境 SDK 脚本域名。
	AirwallexStaticDomain = "https://static.airwallex.com"
	// AirwallexCheckoutDomain 是 Airwallex 生产环境收银台元素和 iframe 域名。
	AirwallexCheckoutDomain = "https://checkout.airwallex.com"
	// AirwallexDemoStaticDomain 是 Airwallex 沙箱环境 SDK 脚本域名。
	AirwallexDemoStaticDomain = "https://static-demo.airwallex.com"
	// AirwallexDemoCheckoutDomain 是 Airwallex 沙箱环境收银台元素和 iframe 域名。
	AirwallexDemoCheckoutDomain = "https://checkout-demo.airwallex.com"
)

var requiredCSPDirectiveValues = []struct {
	directive string
	value     string
}{
	// 插件配置 UI 使用同源 iframe；目标响应仍必须显式放开 X-Frame-Options，
	// 因此这里只允许 'self' 不会使其他默认 DENY 的管理/API 页面可被嵌入。
	{"frame-src", "'self'"},
	{"script-src", CloudflareInsightsDomain},
	{"script-src", TencentCaptchaDomain},
	{"frame-src", TencentCaptchaDomain},
	{"style-src", TencentCaptchaStaticDomain},
	{"script-src", TencentCaptchaCDNDomain},
	{"script-src", TencentCaptchaGlobalDomain},
	{"script-src", TencentCaptchaGlobalCDNDomain},
	{"script-src", TencentCaptchaPrehandleDomain},
	{"script-src", TencentCaptchaJQueryDomain},
	{"connect-src", TencentCaptchaDomain},
	{"connect-src", TencentCaptchaPrehandleDomain},
	{"connect-src", TencentCaptchaRceDomain},
	{"frame-src", TencentCaptchaGlobalDomain},
	{"frame-src", TencentCaptchaPrehandleDomain},
	{"worker-src", TencentCaptchaWorkerSource},
	{"script-src", StripeDomain},
	{"frame-src", StripeDomain},
	{"script-src", AirwallexStaticDomain},
	{"script-src", AirwallexCheckoutDomain},
	{"style-src", AirwallexStaticDomain},
	{"style-src", AirwallexCheckoutDomain},
	{"frame-src", AirwallexCheckoutDomain},
	{"script-src", AirwallexDemoStaticDomain},
	{"script-src", AirwallexDemoCheckoutDomain},
	{"style-src", AirwallexDemoStaticDomain},
	{"style-src", AirwallexDemoCheckoutDomain},
	{"frame-src", AirwallexDemoCheckoutDomain},
}

// GenerateNonce generates a cryptographically secure random nonce.
// 返回 error 以确保调用方在 crypto/rand 失败时能正确降级。
func GenerateNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate CSP nonce: %w", err)
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// GetNonceFromContext retrieves the CSP nonce from gin context
func GetNonceFromContext(c *gin.Context) string {
	if nonce, exists := c.Get(CSPNonceKey); exists {
		if s, ok := nonce.(string); ok {
			return s
		}
	}
	return ""
}

// SecurityHeaders sets baseline security headers for all responses.
// getFrameSrcOrigins is an optional function that returns extra origins to inject into frame-src;
// pass nil to disable dynamic frame-src injection.
func SecurityHeaders(cfg config.CSPConfig, getFrameSrcOrigins func() []string) gin.HandlerFunc {
	policy := strings.TrimSpace(cfg.Policy)
	if policy == "" {
		policy = config.DefaultCSPPolicy
	}

	// Enhance policy with required directives (nonce placeholder and Cloudflare Insights)
	policy = enhanceCSPPolicy(policy)

	return func(c *gin.Context) {
		finalPolicy := policy
		if getFrameSrcOrigins != nil {
			for _, origin := range getFrameSrcOrigins() {
				if origin != "" {
					finalPolicy = addToDirective(finalPolicy, "frame-src", origin)
				}
			}
		}

		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		if isAPIRoutePath(c) {
			c.Next()
			return
		}

		if cfg.Enabled {
			// Generate nonce for this request
			nonce, err := GenerateNonce()
			if err != nil {
				// crypto/rand 失败时降级为无 nonce 的 CSP 策略
				log.Printf("[SecurityHeaders] %v — 降级为无 nonce 的 CSP", err)
				c.Header("Content-Security-Policy", strings.ReplaceAll(finalPolicy, NonceTemplate, "'unsafe-inline'"))
			} else {
				c.Set(CSPNonceKey, nonce)
				c.Header("Content-Security-Policy", strings.ReplaceAll(finalPolicy, NonceTemplate, "'nonce-"+nonce+"'"))
			}
		}
		c.Next()
	}
}

func isAPIRoutePath(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}
	path := c.Request.URL.Path
	return strings.HasPrefix(path, "/v1/") ||
		strings.HasPrefix(path, "/v1beta/") ||
		strings.HasPrefix(path, "/antigravity/") ||
		strings.HasPrefix(path, "/responses") ||
		strings.HasPrefix(path, "/images")
}

// enhanceCSPPolicy 确保 CSP 策略包含 nonce 支持和运行时组件必需域名。
// 这样旧配置文件没有及时补域名时，验证码和支付组件仍能正常加载。
func enhanceCSPPolicy(policy string) string {
	// Add nonce placeholder to script-src if not present
	if !strings.Contains(policy, NonceTemplate) && !strings.Contains(policy, "'nonce-") {
		policy = addToDirective(policy, "script-src", NonceTemplate)
	}

	for _, required := range requiredCSPDirectiveValues {
		if !directiveHasValue(policy, required.directive, required.value) {
			policy = addToDirective(policy, required.directive, required.value)
		}
	}

	return policy
}

func directiveHasValue(policy, directive, value string) bool {
	for _, rawDirective := range strings.Split(policy, ";") {
		fields := strings.Fields(strings.TrimSpace(rawDirective))
		if len(fields) == 0 || fields[0] != directive {
			continue
		}
		for _, field := range fields[1:] {
			if field == value {
				return true
			}
		}
		return false
	}
	return false
}

// addToDirective adds a value to a specific CSP directive.
// If the directive doesn't exist, it will be added after default-src.
func addToDirective(policy, directive, value string) string {
	if end, ok := cspDirectiveEnd(policy, directive); ok {
		return policy[:end] + " " + value + policy[end:]
	}
	trimmed := strings.TrimSpace(policy)
	if trimmed == "" {
		return newCSPDirective(directive, value)
	}
	if !strings.HasSuffix(trimmed, ";") {
		trimmed += ";"
	}
	return trimmed + " " + newCSPDirective(directive, value)
}

func cspDirectiveEnd(policy, directive string) (int, bool) {
	start := 0
	for start <= len(policy) {
		end := len(policy)
		if relativeEnd := strings.IndexByte(policy[start:], ';'); relativeEnd >= 0 {
			end = start + relativeEnd
		}
		fields := strings.Fields(policy[start:end])
		if len(fields) > 0 && fields[0] == directive {
			return end, true
		}
		if end == len(policy) {
			break
		}
		start = end + 1
	}
	return 0, false
}

func newCSPDirective(directive, value string) string {
	if value == "'self'" {
		return directive + " 'self';"
	}
	return directive + " 'self' " + value + ";"
}
