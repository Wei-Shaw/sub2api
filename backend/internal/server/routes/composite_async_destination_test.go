package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCompositeExplicitDestinationRejectsAsyncVideoWithoutOwnerGrant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, target := range []string{"group", "account", "legacy"} {
		for _, path := range []string{"/v1/videos", "/v1/videos/generations", "/videos/edits", "/videos/extensions"} {
			t.Run(target+path, func(t *testing.T) {
				id := int64(20)
				route := service.CompositeModelRoute{
					ID: 1, GroupID: 1, PublicModel: "video-alias", MatchType: service.CompositeRouteMatchExact,
					TargetPlatform: service.PlatformGrok, UpstreamModel: "grok-imagine-video", Endpoint: service.CompositeRouteEndpointAny,
					Enabled: true, Priority: 100,
				}
				switch target {
				case "group":
					route.TargetGroupID = &id
				case "account":
					route.TargetAccountID = &id
				}
				resolver := service.NewCompositeRouteResolver(compositeRouteRepoStub{routes: []service.CompositeModelRoute{route}})
				router := gin.New()
				router.Use(gin.HandlerFunc(servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
					groupID := int64(1)
					c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
						GroupID: &groupID, Group: &service.Group{ID: groupID, Platform: service.PlatformComposite},
					})
					c.Next()
				})))
				router.Use(compositeTargetPlatformMiddleware(resolver))
				forwarded := false
				router.POST(path, func(c *gin.Context) {
					forwarded = true
					c.Status(http.StatusNoContent)
				})
				req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"video-alias","prompt":"test"}`))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				if target == "legacy" {
					require.Equal(t, http.StatusNoContent, rec.Code)
					require.True(t, forwarded)
				} else {
					require.Equal(t, http.StatusBadRequest, rec.Code)
					require.Contains(t, rec.Body.String(), "composite_async_destination_unsupported")
					require.False(t, forwarded)
				}
			})
		}
	}
}
