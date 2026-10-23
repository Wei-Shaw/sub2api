//go:build unit

package handler

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGeminiV1BetaModels_ImageGenerationGateAppliesToBothNativeActions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, action := range []string{"generateContent", "streamGenerateContent"} {
		t.Run(action, func(t *testing.T) {
			ep := geminiImageGateEndpoints("gemini-3.1-flash")[3]
			ep.path = "/v1beta/models/gemini-3.1-flash:" + action
			ep.params = gin.Params{{Key: "modelAction", Value: "/gemini-3.1-flash:" + action}}
			ep.body = `{"contents":[{"role":"user","parts":[{"text":"draw"}]}],"generationConfig":{"responseModalities":["TEXT","IMAGE"]}}`
			group := &service.Group{ID: 101, Hydrated: true, Platform: service.PlatformGemini, Status: service.StatusActive}
			rec, c, schedulerCache := runGeminiImageGateEndpoint(t, ep, group, service.PlatformGemini)

			require.Equal(t, http.StatusForbidden, rec.Code)
			require.Contains(t, rec.Body.String(), service.ImageGenerationPermissionMessage())
			_, reachedSelection := c.Get(opsAccountIDKey)
			require.False(t, reachedSelection)
			require.Empty(t, schedulerCache.queriedGroupIDs())
		})
	}
}

func TestGeminiV1BetaModels_ImageGenerationGateDetectsImageConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ep := geminiImageGateEndpoints("gemini-3.1-flash")[3]
	ep.body = `{"contents":[{"role":"user","parts":[{"text":"draw"}]}],"generationConfig":{"imageConfig":{"aspectRatio":"16:9"}}}`
	group := &service.Group{ID: 101, Hydrated: true, Platform: service.PlatformGemini, Status: service.StatusActive}
	rec, c, schedulerCache := runGeminiImageGateEndpoint(t, ep, group, service.PlatformGemini)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), service.ImageGenerationPermissionMessage())
	_, reachedSelection := c.Get(opsAccountIDKey)
	require.False(t, reachedSelection)
	require.Empty(t, schedulerCache.queriedGroupIDs())
}
