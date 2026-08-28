package middleware

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// IPBanGuard rejects LLM gateway requests from globally banned client IPs
// before API-key authentication and account scheduling are reached.
func IPBanGuard(next APIKeyAuthMiddleware, ipBanService *service.IPBanService, cfg *config.Config) APIKeyAuthMiddleware {
	return APIKeyAuthMiddleware(func(c *gin.Context) {
		clientIP := ip.GetSecurityClientIP(c, cfg.TrustForwardedIPForAPIKeyACL())
		banned, err := ipBanService.IsBanned(c.Request.Context(), clientIP)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{
				"type":    "server_error",
				"code":    "IP_BAN_CHECK_UNAVAILABLE",
				"message": "IP access policy is temporarily unavailable",
			}})
			return
		}
		if banned {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": gin.H{
				"type":    "access_denied",
				"code":    "IP_BANNED",
				"message": "Access denied",
			}})
			return
		}
		next(c)
	})
}
