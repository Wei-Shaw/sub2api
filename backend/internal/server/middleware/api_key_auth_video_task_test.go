//go:build unit

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyAuthVideoTaskReadsAfterBillingExhausted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	routes := []string{}
	for _, prefix := range []string{"", "/v1"} {
		for _, route := range []string{"/videos/:request_id", "/videos/:request_id/content",
			"/videos/generations/:request_id", "/videos/generations/:request_id/content",
			"/videos/edits/:request_id", "/videos/edits/:request_id/content",
			"/videos/extensions/:request_id", "/videos/extensions/:request_id/content"} {
			routes = append(routes, prefix+route)
		}
	}
	for _, prefix := range []string{"/api/v3", "/v3", "/v1", ""} {
		routes = append(routes, prefix+"/contents/generations/tasks/:task_id")
	}
	for _, state := range []string{"quota status", "runtime quota", "expired status", "runtime expiry", "negative balance", "missing subscription"} {
		t.Run(state, func(t *testing.T) {
			user := &service.User{ID: 7, Role: service.RoleUser, Status: service.StatusActive, Balance: 10}
			group := &service.Group{ID: 42, Status: service.StatusActive, Hydrated: true}
			key := &service.APIKey{ID: 100, UserID: user.ID, Key: "video-task-read", Status: service.StatusActive,
				User: user, Group: group, GroupID: &group.ID}
			switch state {
			case "quota status":
				key.Status = service.StatusAPIKeyQuotaExhausted
			case "runtime quota":
				key.Quota, key.QuotaUsed = 1, 2
			case "expired status":
				key.Status = service.StatusAPIKeyExpired
			case "runtime expiry":
				past := time.Now().Add(-time.Hour)
				key.ExpiresAt = &past
			case "negative balance":
				user.Balance = -1
			case "missing subscription":
				group.SubscriptionType = service.SubscriptionTypeSubscription
			}
			cfg := &config.Config{RunMode: config.RunModeStandard}
			svc := service.NewAPIKeyService(&stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) {
				clone := *key
				return &clone, nil
			}}, nil, nil, nil, nil, nil, cfg)
			subs := service.NewSubscriptionService(nil, &stubUserSubscriptionRepo{
				getActive: func(context.Context, int64, int64) (*service.UserSubscription, error) {
					return nil, service.ErrSubscriptionNotFound
				}}, nil, nil, cfg)
			t.Cleanup(subs.Stop)
			router := gin.New()
			router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(svc, subs, cfg)))
			reached := 0
			ok := func(c *gin.Context) {
				authenticated, exists := GetAPIKeyFromContext(c)
				require.True(t, exists)
				require.Equal(t, key.ID, authenticated.ID)
				reached++
				c.Status(http.StatusOK)
			}
			for _, route := range routes {
				router.GET(route, ok)
			}
			router.POST("/v1/videos/generations", ok)
			router.POST("/api/v3/contents/generations/tasks", ok)
			router.DELETE("/api/v3/contents/generations/tasks/:task_id", ok)
			request := func(method, path string) int {
				w := httptest.NewRecorder()
				req := httptest.NewRequest(method, path, nil)
				req.Header.Set("x-api-key", key.Key)
				router.ServeHTTP(w, req)
				return w.Code
			}
			for _, route := range routes {
				path := strings.ReplaceAll(strings.ReplaceAll(route, ":request_id", "task-b"), ":task_id", "task-b")
				require.Equal(t, http.StatusOK, request(http.MethodGet, path), path)
			}
			require.Equal(t, len(routes), reached)
			for _, control := range []struct{ method, path string }{
				{http.MethodPost, "/v1/videos/generations"},
				{http.MethodPost, "/api/v3/contents/generations/tasks"},
				{http.MethodDelete, "/api/v3/contents/generations/tasks/task-b"},
			} {
				require.Contains(t, []int{http.StatusForbidden, http.StatusTooManyRequests}, request(control.method, control.path))
			}
			require.Equal(t, len(routes), reached, "new jobs and cancellation must still enforce billing")
		})
	}
}

func TestAPIKeyAuthVideoTaskReadsStillAuthenticate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, state := range []string{"disabled key", "unknown key", "disabled user", "disabled group", "IP restriction"} {
		t.Run(state, func(t *testing.T) {
			group := &service.Group{ID: 42, Status: service.StatusActive, Hydrated: true}
			key := &service.APIKey{ID: 100, Key: "video-read-auth", Status: service.StatusAPIKeyQuotaExhausted,
				User: &service.User{ID: 7, Status: service.StatusActive}, Group: group, GroupID: &group.ID}
			switch state {
			case "disabled key":
				key.Status = service.StatusDisabled
			case "disabled user":
				key.User.Status = service.StatusDisabled
			case "disabled group":
				group.Status = service.StatusDisabled
			case "IP restriction":
				key.IPWhitelist = []string{"1.2.3.4"}
			}
			cfg := &config.Config{RunMode: config.RunModeStandard}
			svc := service.NewAPIKeyService(&stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) {
				if state == "unknown key" {
					return nil, service.ErrAPIKeyNotFound
				}
				return key, nil
			}}, nil, nil, nil, nil, nil, cfg)
			router := gin.New()
			router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(svc, nil, cfg)))
			router.GET("/v1/videos/:request_id", func(*gin.Context) { t.Fatal("unauthorized lookup reached handler") })
			req := httptest.NewRequest(http.MethodGet, "/v1/videos/task-b", nil)
			req.RemoteAddr = "9.9.9.9:12345"
			req.Header.Set("x-api-key", key.Key)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, w.Code, w.Body.String())
		})
	}
}
