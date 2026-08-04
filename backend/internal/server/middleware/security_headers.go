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
	// TencentCaptchaDomain is the Tencent Captcha 2.0 Web SDK domain.
	TencentCaptchaDomain = "https://turing.captcha.qcloud.com"
	// TencentCaptchaStaticDomain is the Tencent Captcha static asset domain.
	TencentCaptchaStaticDomain = "https://*.captcha.gtimg.com"
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
REDACTED{
	{"script-src", CloudflareInsightsDomainREDACTED,
	{"script-src", TencentCaptchaDomainREDACTED,
	{"frame-src", TencentCaptchaDomainREDACTED,
	{"style-src", TencentCaptchaStaticDomainREDACTED,
	{"script-src", StripeDomainREDACTED,
	{"frame-src", StripeDomainREDACTED,
	{"script-src", AirwallexStaticDomainREDACTED,
	{"script-src", AirwallexCheckoutDomainREDACTED,
	{"style-src", AirwallexStaticDomainREDACTED,
	{"style-src", AirwallexCheckoutDomainREDACTED,
	{"frame-src", AirwallexCheckoutDomainREDACTED,
	{"script-src", AirwallexDemoStaticDomainREDACTED,
	{"script-src", AirwallexDemoCheckoutDomainREDACTED,
	{"style-src", AirwallexDemoStaticDomainREDACTED,
	{"style-src", AirwallexDemoCheckoutDomainREDACTED,
	{"frame-src", AirwallexDemoCheckoutDomainREDACTED,
REDACTED

// GenerateNonce generates a cryptographically secure random nonce.
// 返回 error 以确保调用方在 crypto/rand 失败时能正确降级。
func GenerateNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate CSP nonce: %w", err)
REDACTED
	return base64.StdEncoding.EncodeToString(b), nil
REDACTED

// GetNonceFromContext retrieves the CSP nonce from gin context
func GetNonceFromContext(c *gin.Context) string {
	if nonce, exists := c.Get(CSPNonceKey); exists {
		if s, ok := nonce.(string); ok {
			return s
	REDACTED
REDACTED
	return ""
REDACTED

// SecurityHeaders sets baseline security headers for all responses.
// getFrameSrcOrigins is an optional function that returns extra origins to inject into frame-src;
// pass nil to disable dynamic frame-src injection.
func SecurityHeaders(cfg config.CSPConfig, getFrameSrcOrigins func() []string) gin.HandlerFunc {
	policy := strings.TrimSpace(cfg.Policy)
	if policy == "" {
		policy = config.DefaultCSPPolicy
REDACTED

	// Enhance policy with required directives (nonce placeholder and Cloudflare Insights)
	policy = enhanceCSPPolicy(policy)

	return func(c *gin.Context) {
		finalPolicy := policy
		if getFrameSrcOrigins != nil {
			for _, origin := range getFrameSrcOrigins() {
				if origin != "" {
					finalPolicy = addToDirective(finalPolicy, "frame-src", origin)
			REDACTED
		REDACTED
	REDACTED

		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		if isAPIRoutePath(c) {
			c.Next()
			return
	REDACTED

		if cfg.Enabled {
			// Generate nonce for this request
			nonce, err := GenerateNonce()
			if err != nil {
				// crypto/rand 失败时降级为无 nonce 的 CSP 策略
				log.Printf("[SecurityHeaders] %v — 降级为无 nonce 的 CSP", err)
				c.Header("Content-Security-Policy", strings.ReplaceAll(finalPolicy, NonceTemplate, "'unsafe-inline'"))
		REDACTED else {
				c.Set(CSPNonceKey, nonce)
				c.Header("Content-Security-Policy", strings.ReplaceAll(finalPolicy, NonceTemplate, "'nonce-"+nonce+"'"))
		REDACTED
	REDACTED
		c.Next()
REDACTED
REDACTED

func isAPIRoutePath(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
REDACTED
	path := c.Request.URL.Path
	return strings.HasPrefix(path, "/v1/") ||
		strings.HasPrefix(path, "/v1beta/") ||
		strings.HasPrefix(path, "/antigravity/") ||
		strings.HasPrefix(path, "/responses") ||
		strings.HasPrefix(path, "/images")
REDACTED

// enhanceCSPPolicy 确保 CSP 策略包含 nonce 支持和运行时组件必需域名。
// 这样旧配置文件没有及时补域名时，验证码和支付组件仍能正常加载。
func enhanceCSPPolicy(policy string) string {
	// Add nonce placeholder to script-src if not present
	if !strings.Contains(policy, NonceTemplate) && !strings.Contains(policy, "'nonce-") {
		policy = addToDirective(policy, "script-src", NonceTemplate)
REDACTED

	for _, required := range requiredCSPDirectiveValues {
		if !directiveHasValue(policy, required.directive, required.value) {
			policy = addToDirective(policy, required.directive, required.value)
	REDACTED
REDACTED

	return policy
REDACTED

func directiveHasValue(policy, directive, value string) bool {
	for _, rawDirective := range strings.Split(policy, ";") {
		fields := strings.Fields(strings.TrimSpace(rawDirective))
		if len(fields) == 0 || fields[0] != directive {
			continue
	REDACTED
		for _, field := range fields[1:] {
			if field == value {
				return true
		REDACTED
	REDACTED
		return false
REDACTED
	return false
REDACTED

// addToDirective adds a value to a specific CSP directive.
// If the directive doesn't exist, it will be added after default-src.
func addToDirective(policy, directive, value string) string {
	// Find the directive in the policy
	directivePrefix := directive + " "
	idx := strings.Index(policy, directivePrefix)

	if idx == -1 {
		// Directive not found, add it after default-src or at the beginning
		defaultSrcIdx := strings.Index(policy, "default-src ")
		if defaultSrcIdx != -1 {
			// Find the end of default-src directive (next semicolon)
			endIdx := strings.Index(policy[defaultSrcIdx:], ";")
			if endIdx != -1 {
				insertPos := defaultSrcIdx + endIdx + 1
				// Insert new directive after default-src
				return policy[:insertPos] + " " + directive + " 'self' " + value + ";" + policy[insertPos:]
		REDACTED
	REDACTED
		// Fallback: prepend the directive
		return directive + " 'self' " + value + "; " + policy
REDACTED

	// Find the end of this directive (next semicolon or end of string)
	endIdx := strings.Index(policy[idx:], ";")

	if endIdx == -1 {
		// No semicolon found, directive goes to end of string
		return policy + " " + value
REDACTED

	// Insert value before the semicolon
	insertPos := idx + endIdx
	return policy[:insertPos] + " " + value + policy[insertPos:]
REDACTED
