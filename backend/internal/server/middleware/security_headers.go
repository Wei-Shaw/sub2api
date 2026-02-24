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
)

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
func SecurityHeaders(cfg config.CSPConfig) gin.HandlerFunc {
	policy := strings.TrimSpace(cfg.Policy)
	if policy == "" {
		policy = config.DefaultCSPPolicy
REDACTED

	// Enhance policy with required directives (nonce placeholder and Cloudflare Insights)
	policy = enhanceCSPPolicy(policy)

	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		if cfg.Enabled {
			// Generate nonce for this request
			nonce, err := GenerateNonce()
			if err != nil {
				// crypto/rand 失败时降级为无 nonce 的 CSP 策略
				log.Printf("[SecurityHeaders] %v — 降级为无 nonce 的 CSP", err)
				finalPolicy := strings.ReplaceAll(policy, NonceTemplate, "'unsafe-inline'")
				c.Header("Content-Security-Policy", finalPolicy)
		REDACTED else {
				c.Set(CSPNonceKey, nonce)
				finalPolicy := strings.ReplaceAll(policy, NonceTemplate, "'nonce-"+nonce+"'")
				c.Header("Content-Security-Policy", finalPolicy)
		REDACTED
	REDACTED
		c.Next()
REDACTED
REDACTED

// enhanceCSPPolicy ensures the CSP policy includes nonce support and Cloudflare Insights domain.
// This allows the application to work correctly even if the config file has an older CSP policy.
func enhanceCSPPolicy(policy string) string {
	// Add nonce placeholder to script-src if not present
	if !strings.Contains(policy, NonceTemplate) && !strings.Contains(policy, "'nonce-") {
		policy = addToDirective(policy, "script-src", NonceTemplate)
REDACTED

	// Add Cloudflare Insights domain to script-src if not present
	if !strings.Contains(policy, CloudflareInsightsDomain) {
		policy = addToDirective(policy, "script-src", CloudflareInsightsDomain)
REDACTED

	return policy
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
