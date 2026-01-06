package middleware

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
)

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
			c.Header("Content-Security-Policy", policy)
	REDACTED
		c.Next()
REDACTED
REDACTED
