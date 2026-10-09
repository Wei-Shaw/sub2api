//go:build unit

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRelayPrivateAPIKeyWorksWithoutBalanceAndRejectsOtherUsers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ownerID := int64(7)
	for _, tc := range []struct {
		name     string
		userID   int64
		private  bool
		expected int
	}{
		{"owner zero balance", 7, true, 200}, {"other user", 8, true, 403}, {"public zero balance", 7, false, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			group := &service.Group{ID: 42, Status: service.StatusActive, Hydrated: true, SubscriptionType: service.SubscriptionTypeStandard}
			if tc.private {
				group.OwnerUserID = &ownerID
				group.IsExclusive = true
			}
			key := &service.APIKey{ID: 10, UserID: tc.userID, Key: "relay-test-key", Status: service.StatusActive, GroupID: &group.ID, Group: group,
				User: &service.User{ID: tc.userID, Status: service.StatusActive, Balance: 0, AllowedGroups: []int64{42}}}
			repo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) { copy := *key; return &copy, nil }}
			cfg := &config.Config{RunMode: config.RunModeStandard}
			svc := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)
			router := gin.New()
			router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(svc, nil, cfg)))
			router.GET("/t", func(c *gin.Context) { c.Status(http.StatusOK) })
			request := httptest.NewRequest(http.MethodGet, "/t", nil)
			request.Header.Set("Authorization", "Bearer relay-test-key")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, tc.expected, response.Code, response.Body.String())
		})
	}
}
