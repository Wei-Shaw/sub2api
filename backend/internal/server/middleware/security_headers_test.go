package middleware

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestGenerateNonce(t *testing.T) {
	t.Run("generates_valid_base64_string", func(t *testing.T) {
		nonce, err := GenerateNonce()
		require.NoError(t, err)

		// Should be valid base64
		decoded, err := base64.StdEncoding.DecodeString(nonce)
		require.NoError(t, err)

		// Should decode to 16 bytes
		assert.Len(t, decoded, 16)
	})

	t.Run("generates_unique_nonces", func(t *testing.T) {
		nonces := make(map[string]bool)
		for i := 0; i < 100; i++ {
			nonce, err := GenerateNonce()
			require.NoError(t, err)
			assert.False(t, nonces[nonce], "nonce should be unique")
			nonces[nonce] = true
		}
	})

	t.Run("nonce_has_expected_length", func(t *testing.T) {
		nonce, err := GenerateNonce()
		require.NoError(t, err)
		// 16 bytes -> 24 chars in base64 (with padding)
		assert.Len(t, nonce, 24)
	})
}

func TestGetNonceFromContext(t *testing.T) {
	t.Run("returns_nonce_when_present", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		expectedNonce := "test-nonce-123"
		c.Set(CSPNonceKey, expectedNonce)

		nonce := GetNonceFromContext(c)
		assert.Equal(t, expectedNonce, nonce)
	})

	t.Run("returns_empty_string_when_not_present", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		nonce := GetNonceFromContext(c)
		assert.Empty(t, nonce)
	})

	t.Run("returns_empty_for_wrong_type", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		// Set a non-string value
		c.Set(CSPNonceKey, 12345)

		// Should return empty string for wrong type (safe type assertion)
		nonce := GetNonceFromContext(c)
		assert.Empty(t, nonce)
	})
}

func TestSecurityHeaders(t *testing.T) {
	t.Run("sets_basic_security_headers", func(t *testing.T) {
		cfg := config.CSPConfig{Enabled: false}
		middleware := SecurityHeaders(cfg, nil)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

		middleware(c)

		assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
		assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
		assert.Equal(t, "strict-origin-when-cross-origin", w.Header().Get("Referrer-Policy"))
	})

	t.Run("csp_disabled_no_csp_header", func(t *testing.T) {
		cfg := config.CSPConfig{Enabled: false}
		middleware := SecurityHeaders(cfg, nil)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

		middleware(c)

		assert.Empty(t, w.Header().Get("Content-Security-Policy"))
	})

	t.Run("csp_enabled_sets_csp_header", func(t *testing.T) {
		cfg := config.CSPConfig{
			Enabled: true,
			Policy:  "default-src 'self'",
		}
		middleware := SecurityHeaders(cfg, nil)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

		middleware(c)

		csp := w.Header().Get("Content-Security-Policy")
		assert.NotEmpty(t, csp)
		// Policy is auto-enhanced with nonce and Cloudflare Insights domain
		assert.Contains(t, csp, "default-src 'self'")
		assert.Contains(t, csp, "'nonce-")
		assert.Contains(t, csp, CloudflareInsightsDomain)
		assert.Equal(t, 1, countDirectiveValue(csp, "worker-src", TencentCaptchaWorkerSource))
	})

	t.Run("old_custom_policy_dynamically_allows_same_origin_frames", func(t *testing.T) {
		cfg := config.CSPConfig{
			Enabled: true,
			Policy:  "default-src 'self'; frame-src https://checkout.example.com",
		}
		middleware := SecurityHeaders(cfg, nil)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/admin/plugins", nil)

		middleware(c)

		csp := w.Header().Get("Content-Security-Policy")
		assert.Equal(t, 1, countDirectiveValue(csp, "frame-src", "'self'"))
		assert.Equal(t, 1, countDirectiveValue(csp, "frame-src", "https://checkout.example.com"))
		assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
	})

	t.Run("api_route_skips_csp_nonce_generation", func(t *testing.T) {
		cfg := config.CSPConfig{
			Enabled: true,
			Policy:  "default-src 'self'; script-src 'self' __CSP_NONCE__",
		}
		middleware := SecurityHeaders(cfg, nil)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

		middleware(c)

		assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
		assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
		assert.Equal(t, "strict-origin-when-cross-origin", w.Header().Get("Referrer-Policy"))
		assert.Empty(t, w.Header().Get("Content-Security-Policy"))
		assert.Empty(t, GetNonceFromContext(c))
	})

	t.Run("csp_enabled_with_nonce_placeholder", func(t *testing.T) {
		cfg := config.CSPConfig{
			Enabled: true,
			Policy:  "script-src 'self' __CSP_NONCE__",
		}
		middleware := SecurityHeaders(cfg, nil)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

		middleware(c)

		csp := w.Header().Get("Content-Security-Policy")
		assert.NotEmpty(t, csp)
		assert.NotContains(t, csp, "__CSP_NONCE__", "placeholder should be replaced")
		assert.Contains(t, csp, "'nonce-", "should contain nonce directive")

		// Verify nonce is stored in context
		nonce := GetNonceFromContext(c)
		assert.NotEmpty(t, nonce)
		assert.Contains(t, csp, "'nonce-"+nonce+"'")
	})

	t.Run("uses_default_policy_when_empty", func(t *testing.T) {
		cfg := config.CSPConfig{
			Enabled: true,
			Policy:  "",
		}
		middleware := SecurityHeaders(cfg, nil)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

		middleware(c)

		csp := w.Header().Get("Content-Security-Policy")
		assert.NotEmpty(t, csp)
		// Default policy should contain these elements
		assert.Contains(t, csp, "default-src 'self'")
		assert.Contains(t, csp, TencentCaptchaDomain)
	})

	t.Run("uses_default_policy_when_whitespace_only", func(t *testing.T) {
		cfg := config.CSPConfig{
			Enabled: true,
			Policy:  "   \t\n  ",
		}
		middleware := SecurityHeaders(cfg, nil)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

		middleware(c)

		csp := w.Header().Get("Content-Security-Policy")
		assert.NotEmpty(t, csp)
		assert.Contains(t, csp, "default-src 'self'")
	})

	t.Run("multiple_nonce_placeholders_replaced", func(t *testing.T) {
		cfg := config.CSPConfig{
			Enabled: true,
			Policy:  "script-src __CSP_NONCE__; style-src __CSP_NONCE__",
		}
		middleware := SecurityHeaders(cfg, nil)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

		middleware(c)

		csp := w.Header().Get("Content-Security-Policy")
		nonce := GetNonceFromContext(c)

		// Count occurrences of the nonce
		count := strings.Count(csp, "'nonce-"+nonce+"'")
		assert.Equal(t, 2, count, "both placeholders should be replaced with same nonce")
	})

	t.Run("calls_next_handler", func(t *testing.T) {
		cfg := config.CSPConfig{Enabled: true, Policy: "default-src 'self'"}
		middleware := SecurityHeaders(cfg, nil)

		nextCalled := false
		router := gin.New()
		router.Use(middleware)
		router.GET("/test", func(c *gin.Context) {
			nextCalled = true
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		router.ServeHTTP(w, req)

		assert.True(t, nextCalled, "next handler should be called")
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("nonce_unique_per_request", func(t *testing.T) {
		cfg := config.CSPConfig{
			Enabled: true,
			Policy:  "script-src __CSP_NONCE__",
		}
		middleware := SecurityHeaders(cfg, nil)

		nonces := make(map[string]bool)
		for i := 0; i < 10; i++ {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

			middleware(c)

			nonce := GetNonceFromContext(c)
			assert.False(t, nonces[nonce], "nonce should be unique per request")
			nonces[nonce] = true
		}
	})
}

func TestCSPNonceKey(t *testing.T) {
	t.Run("constant_value", func(t *testing.T) {
		assert.Equal(t, "csp_nonce", CSPNonceKey)
	})
}

func TestNonceTemplate(t *testing.T) {
	t.Run("constant_value", func(t *testing.T) {
		assert.Equal(t, "__CSP_NONCE__", NonceTemplate)
	})
}

func countDirectiveValue(policy, directive, value string) int {
	for _, rawDirective := range strings.Split(policy, ";") {
		fields := strings.Fields(strings.TrimSpace(rawDirective))
		if len(fields) == 0 || fields[0] != directive {
			continue
		}
		count := 0
		for _, field := range fields[1:] {
			if field == value {
				count++
			}
		}
		return count
	}
	return 0
}

func TestAddToDirective(t *testing.T) {
	t.Run("adds_to_existing_directive", func(t *testing.T) {
		policy := "script-src 'self'; style-src 'self'"
		result := addToDirective(policy, "script-src", "https://example.com")

		assert.Contains(t, result, "script-src 'self' https://example.com")
	})

	t.Run("creates_directive_if_not_exists", func(t *testing.T) {
		policy := "default-src 'self'"
		result := addToDirective(policy, "script-src", "https://example.com")

		assert.Contains(t, result, "script-src")
		assert.Contains(t, result, "https://example.com")
	})

	t.Run("handles_directive_at_end_without_semicolon", func(t *testing.T) {
		policy := "default-src 'self'; script-src 'self'"
		result := addToDirective(policy, "script-src", "https://example.com")

		assert.Contains(t, result, "https://example.com")
	})

	t.Run("handles_empty_policy", func(t *testing.T) {
		policy := ""
		result := addToDirective(policy, "script-src", "https://example.com")

		assert.Contains(t, result, "script-src")
		assert.Contains(t, result, "https://example.com")
	})

	t.Run("does_not_match_a_directive_name_suffix", func(t *testing.T) {
		policy := "default-src 'none'; child-frame-src https://legacy.example.com"
		result := addToDirective(policy, "frame-src", "'self'")

		assert.Equal(t, 1, countDirectiveValue(result, "frame-src", "'self'"))
		assert.Equal(t, 0, countDirectiveValue(result, "child-frame-src", "'self'"))
		assert.Equal(t, 1, countDirectiveValue(result, "child-frame-src", "https://legacy.example.com"))
	})
}

// Benchmark tests
func BenchmarkGenerateNonce(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = GenerateNonce()
	}
}

func BenchmarkSecurityHeadersMiddleware(b *testing.B) {
	cfg := config.CSPConfig{
		Enabled: true,
		Policy:  "script-src 'self' __CSP_NONCE__",
	}
	middleware := SecurityHeaders(cfg, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		middleware(c)
	}
}
