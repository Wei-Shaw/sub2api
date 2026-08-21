package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/fal"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestImageBuildSubmitInputUsesEffectiveGroupImageRate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/images/generations", nil)

	groupID := int64(2)
	accountRate := 6.0
	apiKey := &service.APIKey{
		ID:      11,
		GroupID: &groupID,
		Group:   &service.Group{ID: groupID, RateMultiplier: 0.2},
	}
	account := &service.Account{ID: 3, Platform: service.PlatformFal, RateMultiplier: &accountRate}
	h := &ImageGatewayHandler{imagesService: &service.OpenAIGatewayService{}}

	input := h.buildSubmitInput(
		c, apiKey, middleware2.AuthSubject{UserID: 1}, service.AsyncMediaFacadeOpenAI,
		"gpt-image-2", account, fal.ImageGenInput{Prompt: "test"},
	)

	require.InDelta(t, 0.2, input.RateMultiplier, 1e-12)
	require.True(t, input.RateMultiplierSet)
}
