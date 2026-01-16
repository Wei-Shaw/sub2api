package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
)

const (
	// CSPNonceKey is the context key for storing the CSP nonce
	CSPNonceKey = "csp_nonce"
	// NonceTemplate is the placeholder in CSP policy for nonce
	NonceTemplate = "__CSP_NONCE__"
)

// GenerateNonce generates a cryptographically secure random nonce
func GenerateNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
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

	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		if cfg.Enabled {
			// Generate nonce for this request
			nonce := GenerateNonce()
			c.Set(CSPNonceKey, nonce)

			// Replace nonce placeholder in policy
			finalPolicy := strings.ReplaceAll(policy, NonceTemplate, "'nonce-"+nonce+"'")
			c.Header("Content-Security-Policy", finalPolicy)
	REDACTED
		c.Next()
REDACTED
REDACTED
