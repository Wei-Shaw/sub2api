package middleware

import (
	"context"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func CustomDomainGuard(customDomainService *service.CustomDomainService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if customDomainService == nil {
			c.Next()
			return
		}

		domain, matched, err := customDomainService.ResolveRequestHost(c.Request.Context(), c.Request.Host)
		if err != nil {
			abortWithApplicationError(c, err)
			return
		}
		if !matched || domain == nil {
			c.Next()
			return
		}

		authValue, exists := c.Get(string(ContextKeyUser))
		if !exists {
			AbortWithError(c, http.StatusUnauthorized, "API_KEY_REQUIRED", "API key is required")
			return
		}
		subject, ok := authValue.(AuthSubject)
		if !ok {
			AbortWithError(c, http.StatusUnauthorized, "INVALID_API_KEY", "Invalid API key")
			return
		}
		if !domain.CanUse(subject.UserID) {
			abortWithApplicationError(c, service.ErrCustomDomainForbidden)
			return
		}

		ctx := context.WithValue(c.Request.Context(), ctxkey.CustomDomainID, domain.ID)
		ctx = context.WithValue(ctx, ctxkey.CustomDomain, domain.Domain)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func abortWithApplicationError(c *gin.Context, err error) {
	response.ErrorFrom(c, err)
	c.Abort()
}
