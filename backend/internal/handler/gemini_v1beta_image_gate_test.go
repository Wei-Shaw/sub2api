package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGeminiV1BetaModels_ImageGenerationGateAppliesToBothNativeActions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, action := range []string{"generateContent", "streamGenerateContent"} {
		t.Run(action, func(t *testing.T) {
			body := []byte(`{"contents":[{"role":"user","parts":[{"text":"draw"}]}],"generationConfig":{"responseModalities":["TEXT","IMAGE"]}}`)
			req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-3.1-flash:"+action, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = req
			groupID := int64(101)
			c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
				ID: 1, GroupID: &groupID,
				Group: &service.Group{ID: groupID, Platform: service.PlatformGemini, AllowImageGeneration: false},
				User:  &service.User{ID: 7},
			})
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7, Concurrency: 1})
			c.Params = gin.Params{{Key: "modelAction", Value: "gemini-3.1-flash:" + action}}

			(&GatewayHandler{}).GeminiV1BetaModels(c)

			require.Equal(t, http.StatusForbidden, rec.Code)
			require.Contains(t, rec.Body.String(), service.ImageGenerationPermissionMessage())
		})
	}
}
