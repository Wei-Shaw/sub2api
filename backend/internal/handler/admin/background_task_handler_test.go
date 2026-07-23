package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type backgroundTaskHandlerRepoStub struct {
	service.BackgroundTaskRepository
	listFilter        service.BackgroundTaskListFilter
	list              *service.BackgroundTaskListResult
	cancelErr         error
	retryTask         *service.BackgroundTaskRun
	creationLookupKey string
	creationTask      *service.BackgroundTaskRun
}

func (r *backgroundTaskHandlerRepoStub) List(_ context.Context, filter service.BackgroundTaskListFilter) (*service.BackgroundTaskListResult, error) {
	r.listFilter = filter
	return r.list, nil
}

func (r *backgroundTaskHandlerRepoStub) Cancel(_ context.Context, _, _ int64, _ time.Time) (*service.BackgroundTaskRun, error) {
	if r.cancelErr != nil {
		return nil, r.cancelErr
	}
	return nil, service.ErrBackgroundTaskNotFound
}

func (r *backgroundTaskHandlerRepoStub) RequeueIndeterminate(_ context.Context, _ int64, _ time.Time) (*service.BackgroundTaskRun, error) {
	return r.retryTask, nil
}

func (r *backgroundTaskHandlerRepoStub) GetByCreationRequestKey(_ context.Context, key string) (*service.BackgroundTaskRun, error) {
	r.creationLookupKey = key
	if r.creationTask != nil {
		return r.creationTask, nil
	}
	return nil, service.ErrBackgroundTaskNotFound
}

func backgroundTaskHandlerRouter(repo service.BackgroundTaskRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 99})
		c.Next()
	})
	handler := NewBackgroundTaskHandler(service.NewBackgroundTaskService(repo, nil, nil, nil))
	router.POST("/accounts/:id/quota-reset-tasks", handler.CreateOpenAIQuotaReset)
	router.GET("/tasks", handler.List)
	router.POST("/tasks/:id/cancel", handler.Cancel)
	router.POST("/tasks/:id/retry", handler.Retry)
	return router
}

func TestBackgroundTaskHandlerListFiltersAndRedactsPrivateFields(t *testing.T) {
	requestID := "private-redeem-request-id"
	creationRequestKey := "private-creation-request-key"
	actorID := int64(99)
	repo := &backgroundTaskHandlerRepoStub{list: &service.BackgroundTaskListResult{
		Items: []*service.BackgroundTaskRun{{
			ID: 7, TaskType: service.BackgroundTaskTypeOpenAIQuotaReset,
			ResourceType: "openai_account", ResourceID: "42",
			Payload:            json.RawMessage(`{"credit_id":"private-credit-id","oauth":"private-token"}`),
			Display:            json.RawMessage(`{"account_id":42,"account_name":"safe-account","credit_expires_at":"2030-01-01T00:00:00Z"}`),
			RunAt:              time.Date(2029, 12, 31, 23, 0, 0, 0, time.UTC),
			Status:             service.BackgroundTaskStatusIndeterminate,
			IdempotencyKey:     &requestID,
			CreationRequestKey: &creationRequestKey,
			CreatedBy:          actorID,
			CanceledBy:         &actorID,
		}},
		Total: 1, Page: 2, PageSize: 5,
	}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/tasks?task_type=openai_quota_reset&status=indeterminate&resource_type=openai_account&resource_id=42&page=2&page_size=5", nil)
	backgroundTaskHandlerRouter(repo).ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, service.BackgroundTaskTypeOpenAIQuotaReset, repo.listFilter.TaskType)
	require.Equal(t, service.BackgroundTaskStatusIndeterminate, repo.listFilter.Status)
	require.Equal(t, "openai_account", repo.listFilter.ResourceType)
	require.Equal(t, "42", repo.listFilter.ResourceID)
	require.Equal(t, 2, repo.listFilter.Page)
	require.Equal(t, 5, repo.listFilter.PageSize)
	body := recorder.Body.String()
	require.Contains(t, body, "safe-account")
	require.NotContains(t, body, "private-credit-id")
	require.NotContains(t, body, requestID)
	require.NotContains(t, body, creationRequestKey)
	require.NotContains(t, body, "private-token")
	require.NotContains(t, body, "created_by")
	require.NotContains(t, body, "canceled_by")
}

func TestBackgroundTaskHandlerCreateRequiresBoundedIdempotencyKey(t *testing.T) {
	router := backgroundTaskHandlerRouter(&backgroundTaskHandlerRepoStub{})
	body := `{"expected_expires_at":"2030-01-01T00:00:00Z","lead_time_minutes":60}`

	for _, key := range []string{"", "   ", strings.Repeat("k", service.BackgroundTaskCreationRequestKeyMaxLength+1)} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/accounts/42/quota-reset-tasks", strings.NewReader(body))
		if key != "" {
			request.Header.Set("Idempotency-Key", key)
		}
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusBadRequest, recorder.Code, "key %q", key)
	}
}

func TestBackgroundTaskHandlerCreatePassesTrimmedIdempotencyKey(t *testing.T) {
	repo := &backgroundTaskHandlerRepoStub{creationTask: &service.BackgroundTaskRun{
		ID: 12, TaskType: service.BackgroundTaskTypeOpenAIQuotaReset,
		ResourceType: "openai_account", ResourceID: "42",
		Status: service.BackgroundTaskStatusPending,
	}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/accounts/42/quota-reset-tasks",
		strings.NewReader(`{"expected_expires_at":"2030-01-01T00:00:00Z","lead_time_minutes":60}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "  reset-create-42  ")
	backgroundTaskHandlerRouter(repo).ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "reset-create-42", repo.creationLookupKey)
	require.Contains(t, recorder.Body.String(), `"created":false`)
}

func TestBackgroundTaskHandlerCancelConflictAndRetryOriginalTask(t *testing.T) {
	repo := &backgroundTaskHandlerRepoStub{cancelErr: service.ErrBackgroundTaskCannotCancel}
	router := backgroundTaskHandlerRouter(repo)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/tasks/7/cancel", nil))
	require.Equal(t, http.StatusConflict, recorder.Code)

	repo.retryTask = &service.BackgroundTaskRun{
		ID: 7, TaskType: service.BackgroundTaskTypeOpenAIQuotaReset,
		ResourceType: "openai_account", ResourceID: "42",
		RunAt: time.Now(), Status: service.BackgroundTaskStatusPending,
	}
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/tasks/7/retry", strings.NewReader("")))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"id":7`)
}
