package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func init() {
	// cors_test 与 security_headers_test 在同一个包，但 init 是幂等的
	gin.SetMode(gin.TestMode)
REDACTED

// --- Task 8.2: 验证 CORS 条件化头部 ---

func TestCORS_DisallowedOrigin_NoAllowHeaders(t *testing.T) {
	cfg := config.CORSConfig{
		AllowedOrigins:   []string{"https://allowed.example.com"REDACTED,
		AllowCredentials: false,
REDACTED
	middleware := CORS(cfg)

	tests := []struct {
		name   string
		method string
		origin string
REDACTED{
		{
			name:   "preflight_disallowed_origin",
			method: http.MethodOptions,
			origin: "https://evil.example.com",
	REDACTED,
		{
			name:   "get_disallowed_origin",
			method: http.MethodGet,
			origin: "https://evil.example.com",
	REDACTED,
		{
			name:   "post_disallowed_origin",
			method: http.MethodPost,
			origin: "https://attacker.example.com",
	REDACTED,
		{
			name:   "preflight_no_origin",
			method: http.MethodOptions,
			origin: "",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(tt.method, "/", nil)
			if tt.origin != "" {
				c.Request.Header.Set("Origin", tt.origin)
		REDACTED

			middleware(c)

			// 不应设置 Allow-Headers、Allow-Methods 和 Max-Age
			assert.Empty(t, w.Header().Get("Access-Control-Allow-Headers"),
				"不允许的 origin 不应收到 Allow-Headers")
			assert.Empty(t, w.Header().Get("Access-Control-Allow-Methods"),
				"不允许的 origin 不应收到 Allow-Methods")
			assert.Empty(t, w.Header().Get("Access-Control-Max-Age"),
				"不允许的 origin 不应收到 Max-Age")
			assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"),
				"不允许的 origin 不应收到 Allow-Origin")
	REDACTED)
REDACTED
REDACTED

func TestCORS_AllowedOrigin_HasAllowHeaders(t *testing.T) {
	cfg := config.CORSConfig{
		AllowedOrigins:   []string{"https://allowed.example.com"REDACTED,
		AllowCredentials: false,
REDACTED
	middleware := CORS(cfg)

	tests := []struct {
		name   string
		method string
REDACTED{
		{name: "preflight_OPTIONS", method: http.MethodOptionsREDACTED,
		{name: "normal_GET", method: http.MethodGetREDACTED,
		{name: "normal_POST", method: http.MethodPostREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(tt.method, "/", nil)
			c.Request.Header.Set("Origin", "https://allowed.example.com")

			middleware(c)

			// 应设置 Allow-Headers、Allow-Methods 和 Max-Age
			assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Headers"),
				"允许的 origin 应收到 Allow-Headers")
			assert.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "X-Admin-UI-Request")
			assert.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "X-User-UI-Request")
			assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Methods"),
				"允许的 origin 应收到 Allow-Methods")
			assert.Contains(t, w.Header().Get("Access-Control-Expose-Headers"), "Server-Timing")
			assert.Equal(t, "86400", w.Header().Get("Access-Control-Max-Age"),
				"允许的 origin 应收到 Max-Age=86400")
			assert.Equal(t, "https://allowed.example.com", w.Header().Get("Access-Control-Allow-Origin"),
				"允许的 origin 应收到 Allow-Origin")
	REDACTED)
REDACTED
REDACTED

func TestCORS_PreflightDisallowedOrigin_ReturnsForbidden(t *testing.T) {
	cfg := config.CORSConfig{
		AllowedOrigins:   []string{"https://allowed.example.com"REDACTED,
		AllowCredentials: false,
REDACTED
	middleware := CORS(cfg)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodOptions, "/", nil)
	c.Request.Header.Set("Origin", "https://evil.example.com")

	middleware(c)

	assert.Equal(t, http.StatusForbidden, w.Code,
		"不允许的 origin 的 preflight 请求应返回 403")
REDACTED

func TestCORS_PreflightAllowedOrigin_ReturnsNoContent(t *testing.T) {
	cfg := config.CORSConfig{
		AllowedOrigins:   []string{"https://allowed.example.com"REDACTED,
		AllowCredentials: false,
REDACTED
	middleware := CORS(cfg)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodOptions, "/", nil)
	c.Request.Header.Set("Origin", "https://allowed.example.com")

	middleware(c)

	assert.Equal(t, http.StatusNoContent, w.Code,
		"允许的 origin 的 preflight 请求应返回 204")
REDACTED

func TestCORS_WildcardOrigin_AllowsAny(t *testing.T) {
	cfg := config.CORSConfig{
		AllowedOrigins:   []string{"*"REDACTED,
		AllowCredentials: false,
REDACTED
	middleware := CORS(cfg)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("Origin", "https://any-origin.example.com")

	middleware(c)

	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"),
		"通配符配置应返回 *")
	assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Headers"),
		"通配符 origin 应设置 Allow-Headers")
	assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Methods"),
		"通配符 origin 应设置 Allow-Methods")
REDACTED

func TestCORS_AllowCredentials_SetCorrectly(t *testing.T) {
	cfg := config.CORSConfig{
		AllowedOrigins:   []string{"https://allowed.example.com"REDACTED,
		AllowCredentials: true,
REDACTED
	middleware := CORS(cfg)

	t.Run("allowed_origin_gets_credentials", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c.Request.Header.Set("Origin", "https://allowed.example.com")

		middleware(c)

		assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"),
			"允许的 origin 且开启 credentials 应设置 Allow-Credentials")
REDACTED)

	t.Run("disallowed_origin_no_credentials", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c.Request.Header.Set("Origin", "https://evil.example.com")

		middleware(c)

		assert.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"),
			"不允许的 origin 不应收到 Allow-Credentials")
REDACTED)
REDACTED

func TestCORS_WildcardWithCredentials_DisablesCredentials(t *testing.T) {
	cfg := config.CORSConfig{
		AllowedOrigins:   []string{"*"REDACTED,
		AllowCredentials: true,
REDACTED
	middleware := CORS(cfg)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("Origin", "https://any.example.com")

	middleware(c)

	// 通配符 + credentials 不兼容，credentials 应被禁用
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"),
		"通配符 origin 应禁用 Allow-Credentials")
REDACTED

func TestCORS_MultipleAllowedOrigins(t *testing.T) {
	cfg := config.CORSConfig{
		AllowedOrigins: []string{
			"https://app1.example.com",
			"https://app2.example.com",
	REDACTED,
		AllowCredentials: false,
REDACTED
	middleware := CORS(cfg)

	t.Run("first_origin_allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c.Request.Header.Set("Origin", "https://app1.example.com")

		middleware(c)

		assert.Equal(t, "https://app1.example.com", w.Header().Get("Access-Control-Allow-Origin"))
		assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Headers"))
REDACTED)

	t.Run("second_origin_allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c.Request.Header.Set("Origin", "https://app2.example.com")

		middleware(c)

		assert.Equal(t, "https://app2.example.com", w.Header().Get("Access-Control-Allow-Origin"))
		assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Headers"))
REDACTED)

	t.Run("unlisted_origin_rejected", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c.Request.Header.Set("Origin", "https://app3.example.com")

		middleware(c)

		assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
		assert.Empty(t, w.Header().Get("Access-Control-Allow-Headers"))
REDACTED)
REDACTED

func TestCORS_VaryHeader_SetForSpecificOrigin(t *testing.T) {
	cfg := config.CORSConfig{
		AllowedOrigins:   []string{"https://allowed.example.com"REDACTED,
		AllowCredentials: false,
REDACTED
	middleware := CORS(cfg)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("Origin", "https://allowed.example.com")

	middleware(c)

	assert.Contains(t, w.Header().Values("Vary"), "Origin",
		"非通配符允许的 origin 应设置 Vary: Origin")
REDACTED

func TestNormalizeOrigins(t *testing.T) {
	tests := []struct {
		name   string
		input  []string
		expect []string
REDACTED{
		{name: "nil_input", input: nil, expect: nilREDACTED,
		{name: "empty_input", input: []string{REDACTED, expect: nilREDACTED,
		{name: "trims_whitespace", input: []string{" https://a.com ", "  https://b.com"REDACTED, expect: []string{"https://a.com", "https://b.com"REDACTEDREDACTED,
		{name: "removes_empty_strings", input: []string{"", "  ", "https://a.com"REDACTED, expect: []string{"https://a.com"REDACTEDREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeOrigins(tt.input)
			assert.Equal(t, tt.expect, result)
	REDACTED)
REDACTED
REDACTED
