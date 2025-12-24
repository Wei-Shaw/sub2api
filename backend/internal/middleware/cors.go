package middleware

import (
	"strings"
	"sub2api/internal/config"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CORS 跨域中间件
func CORS(cfg *config.CORSConfig) gin.HandlerFunc {
	corsConfig := cors.Config{
		AllowMethods:  []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowHeaders:  []string{"Authorization", "Content-Type", "X-Requested-With", "X-API-Key", "X-Setup-Token"},
		ExposeHeaders: []string{"Content-Length"},
	}

	var allowedOrigins []string
	if cfg != nil {
		allowedOrigins = cfg.AllowedOrigins
	}

	if len(allowedOrigins) == 0 {
		// 未配置 allowlist 时允许全部来源，但不支持跨域携带 Cookie。
		corsConfig.AllowAllOrigins = true
	} else {
		// 配置 allowlist 时允许携带 Cookie，配合前端 withCredentials。
		corsConfig.AllowOrigins = allowedOrigins
		corsConfig.AllowCredentials = true
		for _, origin := range allowedOrigins {
			if strings.Contains(origin, "*") {
				corsConfig.AllowWildcard = true
				break
			}
		}
	}

	return cors.New(corsConfig)
}
