package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestChatCompletionsRejectsAntigravityGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/chat/completions",
		strings.NewReader(`{
			"model": "gemini-3.1-pro-low",
			"messages": [{"role": "user", "content": "hi"}],
			"stream": false
		}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")

	groupID := int64(1)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID:      1,
		GroupID: &groupID,
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformAntigravity,
		},
		User: &service.User{ID: 1},
	})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{
		UserID:      1,
		Concurrency: 1,
	})

	(&GatewayHandler{}).ChatCompletions(c)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(),
		"Antigravity groups do not support /v1/chat/completions")
	require.Contains(t, recorder.Body.String(), "/v1/messages")
	require.Contains(t, recorder.Body.String(), "/antigravity/v1beta/models/")
}
