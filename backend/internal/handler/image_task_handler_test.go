package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

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

func TestAsyncImageHandlerSubmitAndPoll(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
	tasks := service.NewImageTaskServiceWithUploader(store, nil, time.Hour, time.Minute)
	release := make(chan struct{})
	h := &AsyncImageHandler{tasks: tasks}
	h.execute = func(_ string, c *gin.Context) {
		<-release
		c.JSON(http.StatusOK, gin.H{"created": 123, "data": []gin.H{{"url": "https://example.test/image.png"}}})
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
	require.Contains(t, pollWriter.Body.String(), "https://example.test/image.png")
}

// The background task runs after Submit has released the ingress reservation;
// it must still read its body and buffer under the shared budget, then return
// every byte it held.
func TestAsyncImageHandlerTaskUsesOwnBodyReservation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const budgetLimit = 1 << 20
	budget := pkghttputil.NewBodyMemoryBudget(budgetLimit)
	store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
	tasks := service.NewImageTaskServiceWithUploader(store, nil, time.Hour, time.Minute)
	requestBody := `{"model":"gpt-image-1","prompt":"cat"}`
	submitted := make(chan struct{})
	h := &AsyncImageHandler{tasks: tasks}
	h.execute = func(_ string, c *gin.Context) {
		<-submitted
		body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
		if err != nil || string(body) != requestBody {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"message": "read body failed"}})
			return
		}
		if err := pkghttputil.ReserveRequestBodyMemory(c.Request.Context(), 4096); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"message": err.Error()}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"created": 123, "data": []gin.H{{"url": "https://example.test/image.png"}}})
	}

	router := gin.New()
	router.Use(middleware2.RequestBodyLimit(1<<20, budget))
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

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations/async", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusAccepted, w.Code)
	var accepted struct {
		TaskID string `json:"task_id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &accepted))
	close(submitted) // Submit and its ingress release have completed.

	owner := service.ImageTaskOwner{UserID: 7, APIKeyID: 9}
	require.Eventually(t, func() bool {
		got, err := tasks.Get(context.Background(), owner, accepted.TaskID)
		return err == nil && got.Status != service.ImageTaskStatusProcessing
	}, time.Second, 10*time.Millisecond)
	got, err := tasks.Get(context.Background(), owner, accepted.TaskID)
	require.NoError(t, err)
	require.Equal(t, service.ImageTaskStatusCompleted, got.Status, "task error: %s", string(got.Error))

	require.Eventually(t, func() bool {
		ctx, release := pkghttputil.WithRequestBodyMemoryBudget(context.Background(), budget)
		defer release()
		return pkghttputil.ReserveRequestBodyMemory(ctx, budgetLimit) == nil
	}, time.Second, 10*time.Millisecond, "task must release its reservation")
}

// The retained body is charged before a task is created, so exhaustion is
// reported as retryable overload instead of an accepted task that cannot run.
// A body the request already holds moves to the task instead of being charged
// twice.
func TestAsyncImageHandlerSubmitRejectsWhenBodyCannotBeRetained(t *testing.T) {
	requestBody := `{"model":"gpt-image-1","prompt":"cat"}`
	for _, tt := range []struct {
		name          string
		chargeRequest bool
		want          int
	}{
		{name: "budget held by other requests", want: http.StatusServiceUnavailable},
		{name: "body already charged to the request", chargeRequest: true, want: http.StatusAccepted},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
			tasks := service.NewImageTaskServiceWithUploader(store, nil, time.Hour, time.Minute)
			h := &AsyncImageHandler{tasks: tasks}
			ran := make(chan struct{})
			h.execute = func(_ string, c *gin.Context) {
				if !tt.chargeRequest {
					t.Error("task must not run")
				}
				close(ran)
			}

			budget := pkghttputil.NewBodyMemoryBudget(int64(len(requestBody)))
			router := gin.New()
			router.Use(func(c *gin.Context) {
				groupID := int64(3)
				c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
					ID:      9,
					UserID:  7,
					GroupID: &groupID,
					Group:   &service.Group{ID: groupID, Platform: service.PlatformOpenAI, AllowImageGeneration: true},
				})
				ctx, release := pkghttputil.WithRequestBodyMemoryBudget(c.Request.Context(), budget)
				defer release()
				c.Request = c.Request.WithContext(ctx)
				c.Request.Body = pkghttputil.NewPrereadBody([]byte(requestBody))
				if tt.chargeRequest {
					require.NoError(t, pkghttputil.ReserveRequestBodyMemory(ctx, int64(len(requestBody))))
				} else {
					// Another in-flight request holds all remaining capacity.
					other, releaseOther := pkghttputil.WithRequestBodyMemoryBudget(context.Background(), budget)
					defer releaseOther()
					require.NoError(t, pkghttputil.ReserveRequestBodyMemory(other, int64(len(requestBody))))
				}
				c.Next()
			})
			router.POST("/v1/images/generations/async", h.Submit)

			req := httptest.NewRequest(http.MethodPost, "/v1/images/generations/async", strings.NewReader(requestBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, tt.want, w.Code)
			if !tt.chargeRequest {
				require.Equal(t, "1", w.Header().Get("Retry-After"))
				require.Empty(t, store.tasks)
				return
			}
			select {
			case <-ran:
			case <-time.After(time.Second):
				t.Fatal("accepted task did not run")
			}
			require.Eventually(t, func() bool {
				ctx, release := pkghttputil.WithRequestBodyMemoryBudget(context.Background(), budget)
				defer release()
				return pkghttputil.ReserveRequestBodyMemory(ctx, int64(len(requestBody))) == nil
			}, time.Second, 10*time.Millisecond, "task must release the body it took over")
		})
	}
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
