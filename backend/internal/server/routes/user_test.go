package routes

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRegisterUserRoutes_SubscriptionProgressRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	v1 := router.Group("/api/v1")
	RegisterUserRoutes(v1, &handler.Handlers{
		User:             &handler.UserHandler{},
		APIKey:           &handler.APIKeyHandler{},
		Usage:            &handler.UsageHandler{},
		Redeem:           &handler.RedeemHandler{},
		Subscription:     &handler.SubscriptionHandler{},
		Announcement:     &handler.AnnouncementHandler{},
		ChannelMonitor:   &handler.ChannelMonitorUserHandler{},
		Totp:             &handler.TotpHandler{},
		AvailableChannel: &handler.AvailableChannelHandler{},
	}, middleware.JWTAuthMiddleware(func(c *gin.Context) {
		c.Next()
	}), nil)

	paths := make(map[string]struct{})
	for _, route := range router.Routes() {
		if route.Method == "GET" {
			paths[route.Path] = struct{}{}
		}
	}

	require.Contains(t, paths, "/api/v1/subscriptions/progress")
	require.Contains(t, paths, "/api/v1/subscriptions/:id/progress")
}
