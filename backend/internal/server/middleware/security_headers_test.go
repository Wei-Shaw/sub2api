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
REDACTED

func TestGenerateNonce(t *testing.T) {
	t.Run("generates_valid_base64_string", func(t *testing.T) {
		nonce, err := GenerateNonce()
	REDACTED

		// Should be valid base64
		decoded, err := base64.StdEncoding.DecodeString(nonce)
	REDACTED

		// Should decode to 16 bytes
		assert.Len(t, decoded, 16)
REDACTED)

	t.Run("generates_unique_nonces", func(t *testing.T) {
		nonces := make(map[string]bool)
		for i := 0; i < 100; i++ {
			nonce, err := GenerateNonce()
		REDACTED
			assert.False(t, nonces[nonce], "nonce should be unique")
			nonces[nonce] = true
	REDACTED
REDACTED)

	t.Run("nonce_has_expected_length", func(t *testing.T) {
		nonce, err := GenerateNonce()
	REDACTED
		// 16 bytes -> 24 chars in base64 (with padding)
		assert.Len(t, nonce, 24)
REDACTED)
REDACTED

func TestGetNonceFromContext(t *testing.T) {
	t.Run("returns_nonce_when_present", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		expectedNonce := "test-nonce-123"
		c.Set(CSPNonceKey, expectedNonce)

		nonce := GetNonceFromContext(c)
		assert.Equal(t, expectedNonce, nonce)
REDACTED)

	t.Run("returns_empty_string_when_not_present", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		nonce := GetNonceFromContext(c)
		assert.Empty(t, nonce)
REDACTED)

	t.Run("returns_empty_for_wrong_type", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		// Set a non-string value
		c.Set(CSPNonceKey, 12345)

		// Should return empty string for wrong type (safe type assertion)
		nonce := GetNonceFromContext(c)
		assert.Empty(t, nonce)
REDACTED)
REDACTED

func TestSecurityHeaders(t *testing.T) {
	t.Run("sets_basic_security_headers", func(t *testing.T) {
		cfg := config.CSPConfig{Enabled: falseREDACTED
		middleware := SecurityHeaders(cfg, nil)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

		middleware(c)

		assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
		assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
		assert.Equal(t, "strict-origin-when-cross-origin", w.Header().Get("Referrer-Policy"))
REDACTED)

	t.Run("csp_disabled_no_csp_header", func(t *testing.T) {
		cfg := config.CSPConfig{Enabled: falseREDACTED
		middleware := SecurityHeaders(cfg, nil)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

		middleware(c)

		assert.Empty(t, w.Header().Get("Content-Security-Policy"))
REDACTED)

	t.Run("csp_enabled_sets_csp_header", func(t *testing.T) {
		cfg := config.CSPConfig{
			Enabled: true,
			Policy:  "default-src 'self'",
	REDACTED
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
REDACTED)

	t.Run("api_route_skips_csp_nonce_generation", func(t *testing.T) {
		cfg := config.CSPConfig{
			Enabled: true,
			Policy:  "default-src 'self'; script-src 'self' __CSP_NONCE__",
	REDACTED
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
REDACTED)

	t.Run("csp_enabled_with_nonce_placeholder", func(t *testing.T) {
		cfg := config.CSPConfig{
			Enabled: true,
			Policy:  "script-src 'self' __CSP_NONCE__",
	REDACTED
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
REDACTED)

	t.Run("uses_default_policy_when_empty", func(t *testing.T) {
		cfg := config.CSPConfig{
			Enabled: true,
			Policy:  "",
	REDACTED
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
REDACTED)

	t.Run("uses_default_policy_when_whitespace_only", func(t *testing.T) {
		cfg := config.CSPConfig{
			Enabled: true,
			Policy:  "   \t\n  ",
	REDACTED
		middleware := SecurityHeaders(cfg, nil)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

		middleware(c)

		csp := w.Header().Get("Content-Security-Policy")
		assert.NotEmpty(t, csp)
		assert.Contains(t, csp, "default-src 'self'")
REDACTED)

	t.Run("multiple_nonce_placeholders_replaced", func(t *testing.T) {
		cfg := config.CSPConfig{
			Enabled: true,
			Policy:  "script-src __CSP_NONCE__; style-src __CSP_NONCE__",
	REDACTED
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
REDACTED)

	t.Run("calls_next_handler", func(t *testing.T) {
		cfg := config.CSPConfig{Enabled: true, Policy: "default-src 'self'"REDACTED
		middleware := SecurityHeaders(cfg, nil)

		nextCalled := false
		router := gin.New()
		router.Use(middleware)
		router.GET("/test", func(c *gin.Context) {
			nextCalled = true
			c.Status(http.StatusOK)
	REDACTED)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		router.ServeHTTP(w, req)

		assert.True(t, nextCalled, "next handler should be called")
		assert.Equal(t, http.StatusOK, w.Code)
REDACTED)

	t.Run("nonce_unique_per_request", func(t *testing.T) {
		cfg := config.CSPConfig{
			Enabled: true,
			Policy:  "script-src __CSP_NONCE__",
	REDACTED
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
	REDACTED
REDACTED)
REDACTED

func TestCSPNonceKey(t *testing.T) {
	t.Run("constant_value", func(t *testing.T) {
		assert.Equal(t, "csp_nonce", CSPNonceKey)
REDACTED)
REDACTED

func TestNonceTemplate(t *testing.T) {
	t.Run("constant_value", func(t *testing.T) {
		assert.Equal(t, "__CSP_NONCE__", NonceTemplate)
REDACTED)
REDACTED

func TestEnhanceCSPPolicy(t *testing.T) {
	t.Run("adds_nonce_placeholder_if_missing", func(t *testing.T) {
		policy := "default-src 'self'; script-src 'self'"
		enhanced := enhanceCSPPolicy(policy)

		assert.Contains(t, enhanced, NonceTemplate)
		assert.Contains(t, enhanced, CloudflareInsightsDomain)
REDACTED)

	t.Run("does_not_duplicate_nonce_placeholder", func(t *testing.T) {
		policy := "default-src 'self'; script-src 'self' __CSP_NONCE__"
		enhanced := enhanceCSPPolicy(policy)

		// Should not duplicate
		count := strings.Count(enhanced, NonceTemplate)
		assert.Equal(t, 1, count)
REDACTED)

	t.Run("does_not_duplicate_cloudflare_domain", func(t *testing.T) {
		policy := "default-src 'self'; script-src 'self' https://static.cloudflareinsights.com"
		enhanced := enhanceCSPPolicy(policy)

		count := strings.Count(enhanced, CloudflareInsightsDomain)
		assert.Equal(t, 1, count)
REDACTED)

	t.Run("adds_tencent_captcha_domain_for_web_sdk", func(t *testing.T) {
		policy := "default-src 'self'; script-src 'self' __CSP_NONCE__"
		enhanced := enhanceCSPPolicy(policy)

		assert.Equal(t, 1, countDirectiveValue(enhanced, "script-src", TencentCaptchaDomain))
		assert.Equal(t, 1, countDirectiveValue(enhanced, "frame-src", TencentCaptchaDomain))
		assert.Equal(t, 1, countDirectiveValue(enhanced, "style-src", TencentCaptchaStaticDomain))
		assert.Contains(t, config.DefaultCSPPolicy, "style-src 'self' 'unsafe-inline' https://*.captcha.gtimg.com")

		// 入口脚本会再从 CDN 拉核心 JS，国际站还会换用 ca./global. 两个主机；
		// 缺任意一个都会让天御 SDK 触发 script-src 拦截。
		assert.Equal(t, 1, countDirectiveValue(enhanced, "script-src", TencentCaptchaCDNDomain))
		assert.Equal(t, 1, countDirectiveValue(enhanced, "script-src", TencentCaptchaGlobalDomain))
		assert.Equal(t, 1, countDirectiveValue(enhanced, "script-src", TencentCaptchaGlobalCDNDomain))
		assert.Equal(t, 1, countDirectiveValue(enhanced, "script-src", TencentCaptchaPrehandleDomain))
		assert.Equal(t, 1, countDirectiveValue(enhanced, "connect-src", TencentCaptchaDomain))
		assert.Equal(t, 1, countDirectiveValue(enhanced, "connect-src", TencentCaptchaPrehandleDomain))
		assert.Equal(t, 1, countDirectiveValue(enhanced, "frame-src", TencentCaptchaGlobalDomain))
		assert.Equal(t, 1, countDirectiveValue(enhanced, "frame-src", TencentCaptchaPrehandleDomain))
		assert.Equal(t, 1, countDirectiveValue(enhanced, "worker-src", TencentCaptchaWorkerSource))
REDACTED)

	t.Run("does_not_duplicate_tencent_captcha_worker_source", func(t *testing.T) {
		policy := "default-src 'self'; worker-src 'self' blob:; script-src 'self' __CSP_NONCE__"
		enhanced := enhanceCSPPolicy(policy)

		assert.Equal(t, 1, countDirectiveValue(enhanced, "worker-src", TencentCaptchaWorkerSource))
REDACTED)

	t.Run("default_policy_already_carries_tencent_captcha_domains", func(t *testing.T) {
		// 默认策略与中间件强制注入表必须同形，否则 config.example.yaml 会误导自建用户
		for _, required := range requiredCSPDirectiveValues {
			assert.Equal(t, 1, countDirectiveValue(config.DefaultCSPPolicy, required.directive, required.value),
				"DefaultCSPPolicy 缺少 %s %s", required.directive, required.value)
	REDACTED
REDACTED)

	t.Run("handles_policy_without_script_src", func(t *testing.T) {
		policy := "default-src 'self'"
		enhanced := enhanceCSPPolicy(policy)

		assert.Contains(t, enhanced, "script-src")
		assert.Contains(t, enhanced, NonceTemplate)
		assert.Contains(t, enhanced, CloudflareInsightsDomain)
REDACTED)

	t.Run("preserves_existing_nonce", func(t *testing.T) {
		policy := "script-src 'self' 'nonce-existing'"
		enhanced := enhanceCSPPolicy(policy)

		// Should not add placeholder if nonce already exists
		assert.NotContains(t, enhanced, NonceTemplate)
		assert.Contains(t, enhanced, "'nonce-existing'")
REDACTED)

	t.Run("adds_airwallex_domains_for_payment_sdk", func(t *testing.T) {
		policy := "default-src 'self'; script-src 'self' __CSP_NONCE__; style-src 'self'; frame-src 'self'"
		enhanced := enhanceCSPPolicy(policy)

		assert.Contains(t, enhanced, "script-src 'self' __CSP_NONCE__")
		assert.Contains(t, enhanced, AirwallexStaticDomain)
		assert.Contains(t, enhanced, AirwallexCheckoutDomain)
		assert.Contains(t, enhanced, AirwallexDemoStaticDomain)
		assert.Contains(t, enhanced, AirwallexDemoCheckoutDomain)
		assert.Contains(t, enhanced, "style-src 'self'")
		assert.Contains(t, enhanced, "frame-src 'self'")
REDACTED)

	t.Run("does_not_duplicate_airwallex_domains", func(t *testing.T) {
		policy := "default-src 'self'; script-src 'self' https://static.airwallex.com https://static-demo.airwallex.com; frame-src https://checkout.airwallex.com https://checkout-demo.airwallex.com"
		enhanced := enhanceCSPPolicy(policy)

		assert.Equal(t, 1, countDirectiveValue(enhanced, "script-src", AirwallexStaticDomain))
		assert.Equal(t, 1, countDirectiveValue(enhanced, "script-src", AirwallexCheckoutDomain))
		assert.Equal(t, 1, countDirectiveValue(enhanced, "style-src", AirwallexStaticDomain))
		assert.Equal(t, 1, countDirectiveValue(enhanced, "style-src", AirwallexCheckoutDomain))
		assert.Equal(t, 1, countDirectiveValue(enhanced, "frame-src", AirwallexCheckoutDomain))
		assert.Equal(t, 1, countDirectiveValue(enhanced, "script-src", AirwallexDemoStaticDomain))
		assert.Equal(t, 1, countDirectiveValue(enhanced, "script-src", AirwallexDemoCheckoutDomain))
		assert.Equal(t, 1, countDirectiveValue(enhanced, "style-src", AirwallexDemoStaticDomain))
		assert.Equal(t, 1, countDirectiveValue(enhanced, "style-src", AirwallexDemoCheckoutDomain))
		assert.Equal(t, 1, countDirectiveValue(enhanced, "frame-src", AirwallexDemoCheckoutDomain))
REDACTED)
REDACTED

func countDirectiveValue(policy, directive, value string) int {
	for _, rawDirective := range strings.Split(policy, ";") {
		fields := strings.Fields(strings.TrimSpace(rawDirective))
		if len(fields) == 0 || fields[0] != directive {
			continue
	REDACTED
		count := 0
		for _, field := range fields[1:] {
			if field == value {
				count++
		REDACTED
	REDACTED
		return count
REDACTED
	return 0
REDACTED

func TestAddToDirective(t *testing.T) {
	t.Run("adds_to_existing_directive", func(t *testing.T) {
		policy := "script-src 'self'; style-src 'self'"
		result := addToDirective(policy, "script-src", "https://example.com")

		assert.Contains(t, result, "script-src 'self' https://example.com")
REDACTED)

	t.Run("creates_directive_if_not_exists", func(t *testing.T) {
		policy := "default-src 'self'"
		result := addToDirective(policy, "script-src", "https://example.com")

		assert.Contains(t, result, "script-src")
		assert.Contains(t, result, "https://example.com")
REDACTED)

	t.Run("handles_directive_at_end_without_semicolon", func(t *testing.T) {
		policy := "default-src 'self'; script-src 'self'"
		result := addToDirective(policy, "script-src", "https://example.com")

		assert.Contains(t, result, "https://example.com")
REDACTED)

	t.Run("handles_empty_policy", func(t *testing.T) {
		policy := ""
		result := addToDirective(policy, "script-src", "https://example.com")

		assert.Contains(t, result, "script-src")
		assert.Contains(t, result, "https://example.com")
REDACTED)
REDACTED

// Benchmark tests
func BenchmarkGenerateNonce(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = GenerateNonce()
REDACTED
REDACTED

func BenchmarkSecurityHeadersMiddleware(b *testing.B) {
	cfg := config.CSPConfig{
		Enabled: true,
		Policy:  "script-src 'self' __CSP_NONCE__",
REDACTED
	middleware := SecurityHeaders(cfg, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		middleware(c)
REDACTED
REDACTED
