package middleware

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const maxDeveloperKeyAuthorizationHeaderBytes = 256

func NewDeveloperKeyAuthMiddleware(keyService *service.DeveloperKeyService) DeveloperKeyAuthMiddleware {
	return DeveloperKeyAuthMiddleware(func(c *gin.Context) {
		header := strings.TrimSpace(c.GetHeader("Authorization"))
		if header == "" || len(header) > maxDeveloperKeyAuthorizationHeaderBytes {
			AbortWithError(c, http.StatusUnauthorized, "DEVELOPER_KEY_REQUIRED", "Developer key is required in the Authorization header")
			return
		}
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
			AbortWithError(c, http.StatusUnauthorized, "INVALID_DEVELOPER_KEY", "Invalid developer key")
			return
		}
		key, err := keyService.Authenticate(c.Request.Context(), strings.TrimSpace(parts[1]))
		if err != nil {
			AbortWithError(c, http.StatusUnauthorized, "INVALID_DEVELOPER_KEY", "Invalid developer key")
			return
		}
		c.Set(string(ContextKeyDeveloperKey), key)
		c.Set(string(ContextKeyUser), AuthSubject{UserID: key.UserID})
		c.Next()
	})
}

func GetDeveloperKeyFromContext(c *gin.Context) (*service.DeveloperKey, bool) {
	value, ok := c.Get(string(ContextKeyDeveloperKey))
	if !ok {
		return nil, false
	}
	key, ok := value.(*service.DeveloperKey)
	return key, ok && key != nil
}
