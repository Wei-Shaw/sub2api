package middleware

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
	"net/http"
)

// GlobalIPBlacklist rejects every request matching an administrator-configured IP or CIDR.
func GlobalIPBlacklist(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if cfg == nil {
			c.Next()
			return
		}
		rules := cfg.GlobalIPBlacklistRules()
		if rules == nil || rules.PatternCount == 0 {
			c.Next()
			return
		}
		allowed, _ := ip.CheckIPRestrictionWithCompiledRules(SecurityClientIP(c), nil, rules)
		if allowed {
			c.Next()
			return
		}
		response.Error(c, http.StatusForbidden, "Your IP address is blocked")
		c.Abort()
	}
}
