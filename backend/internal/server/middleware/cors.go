package middleware

import (
	"github.com/gin-gonic/gin"
)

// CORS 跨域中间件
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 设置允许跨域的响应头
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-API-Key")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")

		// 处理预检请求
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
	REDACTED

		c.Next()
REDACTED
REDACTED
