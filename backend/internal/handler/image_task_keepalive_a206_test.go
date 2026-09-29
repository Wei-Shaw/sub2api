//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// a206LateModerationError simulates an Images handler with the JSON keepalive
// enabled whose upstream fails after at least one heartbeat interval.
func a206LateModerationError(c *gin.Context) {
	stop := service.StartOpenAIImagesJSONKeepalive(c, 5*time.Millisecond)
	defer stop()
	time.Sleep(60 * time.Millisecond)
	c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
		"type":    "invalid_request_error",
		"code":    "moderation_blocked",
		"message": "blocked",
	}})
}

func TestA206AsyncImageTaskKeepaliveLateErrorMarksFailed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
	tasks := service.NewImageTaskServiceWithUploader(store, nil, time.Hour, time.Minute)
	h := &AsyncImageHandler{tasks: tasks}
	h.execute = func(_ string, c *gin.Context) { a206LateModerationError(c) }

	router := gin.New()
	router.Use(func(c *gin.Context) {
		groupID := int64(3)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
			ID:      9,
			UserID:  7,
			GroupID: &groupID,
			Group:   &service.Group{ID: groupID, Platform: service.PlatformOpenAI, AllowImageGeneration: true},
		})
		c.Next()
	})
	router.POST("/v1/images/generations/async", h.Submit)

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations/async", strings.NewReader(`{"model":"gpt-image-1","prompt":"cat"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusAccepted, w.Code)

	var accepted struct {
		TaskID string `json:"task_id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &accepted))

	owner := service.ImageTaskOwner{UserID: 7, APIKeyID: 9}
	var got *service.ImageTask
	require.Eventually(t, func() bool {
		task, err := tasks.Get(context.Background(), owner, accepted.TaskID)
		if err != nil || task.Status == service.ImageTaskStatusProcessing {
			return false
		}
		got = task
		return true
	}, 2*time.Second, 10*time.Millisecond)

	require.Equal(t, service.ImageTaskStatusFailed, got.Status, "a late upstream error must not be stored as completed")
	require.Equal(t, http.StatusBadRequest, got.HTTPStatus)
	require.Contains(t, string(got.Error), "moderation_blocked")
	require.Empty(t, got.Result)
}

func TestA206ImageStudioExecutorKeepaliveLateErrorKeepsStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := gin.New()
	gateway.POST("/v1/images/generations", a206LateModerationError)
	h := NewImageStudioHandler(nil)
	h.SetGateway(gateway)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/image-studio/jobs", nil)

	status, body := h.executor(c)(context.Background(), "/v1/images/generations", "sk-own", []byte(`{"prompt":"x"}`))

	require.Equal(t, http.StatusBadRequest, status, "the studio replay must see the real upstream status")
	require.Contains(t, string(body), "moderation_blocked")
}
