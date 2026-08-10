package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type asyncImagePrivateStorage struct {
	data        map[string][]byte
	contentType map[string]string
}

func (s *asyncImagePrivateStorage) Save(_ context.Context, key, contentType string, data []byte) error {
	s.data[key] = append([]byte(nil), data...)
	s.contentType[key] = contentType
	return nil
}

func (s *asyncImagePrivateStorage) Load(_ context.Context, key string, _ int64) ([]byte, string, error) {
	data, ok := s.data[key]
	if !ok {
		return nil, "", errors.New("not found")
	}
	return append([]byte(nil), data...), s.contentType[key], nil
}

func (s *asyncImagePrivateStorage) Size(_ context.Context, key string) (int64, error) {
	data, ok := s.data[key]
	if !ok {
		return 0, errors.New("object not found")
	}
	return int64(len(data)), nil
}

func (s *asyncImagePrivateStorage) Delete(_ context.Context, key string) error {
	delete(s.data, key)
	delete(s.contentType, key)
	return nil
}

type asyncImageMemoryStore struct {
	mu    sync.RWMutex
	tasks map[string]*service.ImageTaskRecord
}

func (s *asyncImageMemoryStore) Save(_ context.Context, task *service.ImageTaskRecord, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := *task
	copy.Result = append(json.RawMessage(nil), task.Result...)
	copy.Error = append(json.RawMessage(nil), task.Error...)
	s.tasks[task.ID] = &copy
	return nil
}

func (s *asyncImageMemoryStore) Get(_ context.Context, id string) (*service.ImageTaskRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	task := s.tasks[id]
	if task == nil {
		return nil, service.ErrImageTaskNotFound
	}
	copy := *task
	copy.Result = append(json.RawMessage(nil), task.Result...)
	copy.Error = append(json.RawMessage(nil), task.Error...)
	return &copy, nil
}

func (s *asyncImageMemoryStore) ListByUser(_ context.Context, userID int64, limit int) ([]*service.ImageTaskRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*service.ImageTaskRecord, 0)
	for _, task := range s.tasks {
		if task.UserID != userID {
			continue
		}
		copy := *task
		out = append(out, &copy)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (s *asyncImageMemoryStore) ListForAdmin(_ context.Context, _ int64, _ int) ([]*service.ImageTaskRecord, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*service.ImageTaskRecord, 0, len(s.tasks))
	for _, task := range s.tasks {
		copy := *task
		out = append(out, &copy)
	}
	return out, len(out), nil
}

func (s *asyncImageMemoryStore) AdminStorageStats(context.Context) (int, int64, error) {
	return 0, 0, nil
}

func (s *asyncImageMemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tasks, id)
	return nil
}

func (s *asyncImageMemoryStore) ScheduleCleanup(context.Context, service.ImageTaskCleanup) error {
	return nil
}

func (s *asyncImageMemoryStore) GetCleanup(context.Context, string) (*service.ImageTaskCleanup, error) {
	return nil, service.ErrImageTaskNotFound
}

func (s *asyncImageMemoryStore) ListDueCleanup(context.Context, time.Time, int) ([]service.ImageTaskCleanup, error) {
	return nil, nil
}

func (s *asyncImageMemoryStore) DeleteCleanup(context.Context, string) error { return nil }

func (s *asyncImageMemoryStore) TryLock(context.Context, string, string, time.Duration) (bool, error) {
	return true, nil
}

func (s *asyncImageMemoryStore) Unlock(context.Context, string, string) error { return nil }

func TestAsyncImageHandlerSubmitAndPoll(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
	privateStorage := &asyncImagePrivateStorage{data: make(map[string][]byte), contentType: make(map[string]string)}
	tasks := service.NewImageTaskServiceWithUploader(store, service.NewImageResultUploader(privateStorage, "images/", 0, nil), time.Hour, time.Minute)
	release := make(chan struct{})
	h := &AsyncImageHandler{tasks: tasks}
	h.execute = func(_ string, c *gin.Context) {
		<-release
		png := []byte("\x89PNG\r\n\x1a\npayload")
		c.JSON(http.StatusOK, gin.H{"created": 123, "data": []gin.H{{"b64_json": base64.StdEncoding.EncodeToString(png)}}})
	}

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
	router.GET("/v1/images/tasks/:task_id", h.Get)

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations/async", strings.NewReader(`{"model":"gpt-image-1","prompt":"cat"}`)).WithContext(requestCtx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusAccepted, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.Equal(t, "3", w.Header().Get("Retry-After"))

	var accepted struct {
		TaskID  string `json:"task_id"`
		Status  string `json:"status"`
		PollURL string `json:"poll_url"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &accepted))
	require.Equal(t, service.ImageTaskStatusProcessing, accepted.Status)
	require.Equal(t, "/v1/images/tasks/"+accepted.TaskID, accepted.PollURL)
	require.Equal(t, accepted.PollURL, w.Header().Get("Location"))

	// The detached background request must survive completion of/cancellation
	// from the short submission request.
	cancelRequest()
	close(release)
	require.Eventually(t, func() bool {
		got, err := tasks.Get(context.Background(), service.ImageTaskOwner{UserID: 7, APIKeyID: 9}, accepted.TaskID)
		return err == nil && got.Status == service.ImageTaskStatusCompleted
	}, time.Second, 10*time.Millisecond)

	pollReq := httptest.NewRequest(http.MethodGet, accepted.PollURL, nil)
	pollWriter := httptest.NewRecorder()
	router.ServeHTTP(pollWriter, pollReq)
	require.Equal(t, http.StatusOK, pollWriter.Code)
	require.Equal(t, "no-store", pollWriter.Header().Get("Cache-Control"))
	require.Empty(t, pollWriter.Header().Get("Retry-After"))
	require.NotContains(t, pollWriter.Body.String(), "b64_json")
	require.Contains(t, pollWriter.Body.String(), accepted.PollURL+"/images/0")
}

// When object storage is not configured the feature is fully disabled: the
// endpoints must return 404 without creating a task or writing to Redis.
func TestAsyncImageHandlerDisabledReturns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
	tasks := service.NewImageTaskServiceWithOptions(store, time.Hour, time.Minute) // enabled == false
	h := &AsyncImageHandler{tasks: tasks}

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
	router.GET("/v1/images/tasks/:task_id", h.Get)

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations/async", strings.NewReader(`{"model":"gpt-image-1","prompt":"cat"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "not enabled")

	pollReq := httptest.NewRequest(http.MethodGet, "/v1/images/tasks/imgtask_missing", nil)
	pollWriter := httptest.NewRecorder()
	router.ServeHTTP(pollWriter, pollReq)
	require.Equal(t, http.StatusNotFound, pollWriter.Code)

	// No task was created / persisted.
	require.Empty(t, store.tasks)
}

func TestAsyncImageHandlerGetImageRequiresSubmittingAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
	privateStorage := &asyncImagePrivateStorage{data: make(map[string][]byte), contentType: make(map[string]string)}
	tasks := service.NewImageTaskServiceWithUploader(store, service.NewImageResultUploader(privateStorage, "images/", 0, nil), time.Hour, time.Minute)
	owner := service.ImageTaskOwner{UserID: 7, APIKeyID: 9}
	created, err := tasks.Create(context.Background(), owner)
	require.NoError(t, err)
	png := []byte("\x89PNG\r\n\x1a\npayload")
	b64 := base64.StdEncoding.EncodeToString(png)
	require.NoError(t, tasks.Complete(context.Background(), created.ID, http.StatusOK, json.RawMessage(`{"data":[{"b64_json":"`+b64+`"}]}`)))

	h := &AsyncImageHandler{tasks: tasks}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		keyID, _ := strconv.ParseInt(c.GetHeader("X-Test-Key-ID"), 10, 64)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{ID: keyID, UserID: 7})
		c.Next()
	})
	router.GET("/v1/images/tasks/:task_id/images/:image_index", h.GetImage)
	path := "/v1/images/tasks/" + created.ID + "/images/0"

	foreignRequest := httptest.NewRequest(http.MethodGet, path, nil)
	foreignRequest.Header.Set("X-Test-Key-ID", "10")
	foreignWriter := httptest.NewRecorder()
	router.ServeHTTP(foreignWriter, foreignRequest)
	require.Equal(t, http.StatusNotFound, foreignWriter.Code)
	require.NotContains(t, foreignWriter.Body.String(), "images/")

	ownerRequest := httptest.NewRequest(http.MethodGet, path, nil)
	ownerRequest.Header.Set("X-Test-Key-ID", "9")
	ownerWriter := httptest.NewRecorder()
	router.ServeHTTP(ownerWriter, ownerRequest)
	require.Equal(t, http.StatusOK, ownerWriter.Code)
	require.Equal(t, "private, no-store", ownerWriter.Header().Get("Cache-Control"))
	require.Equal(t, "image/png", ownerWriter.Header().Get("Content-Type"))
	require.Equal(t, png, ownerWriter.Body.Bytes())
}
