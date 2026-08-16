package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type asyncImageMemoryStore struct {
	mu    sync.RWMutex
	tasks map[string]*service.ImageTaskRecord
}

type asyncImageBalanceUserRepoStub struct {
	service.UserRepository
	balance float64
}

func (s *asyncImageBalanceUserRepoStub) GetByID(_ context.Context, id int64) (*service.User, error) {
	return &service.User{ID: id, Balance: s.balance}, nil
}

type asyncImageRiskRecordingStore struct {
	handlerBillingRiskBudgetStore
	acquired chan service.BillingRiskAcquireRequest
}

func (s *asyncImageRiskRecordingStore) Acquire(ctx context.Context, request service.BillingRiskAcquireRequest) (*service.BillingRiskAcquireResult, error) {
	s.acquired <- request
	return s.handlerBillingRiskBudgetStore.Acquire(ctx, request)
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

func TestAsyncImageExecutionContextReusesUnifiedBillingRiskLease(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations/async", strings.NewReader(`{"model":"gpt-image-1","prompt":"test"}`))

	store := &handlerBillingRiskBudgetStore{}
	preAcquired := newBillingRiskLease(newHandlerBillingRiskPermit(t, store))
	defer preAcquired.Close(context.Background())
	taskCtx, _, cancel, _ := newAsyncImageContext(c, []byte(`{"model":"gpt-image-1","prompt":"test"}`), time.Minute, preAcquired)
	defer cancel()

	lease, err := acquireBillingRiskLease(taskCtx.Request.Context(), nil, service.BillingRiskAdmissionInput{})

	require.NoError(t, err)
	require.Same(t, preAcquired, lease)
	require.Equal(t, 1, store.leaseCount(), "内部同步网关不得获取第二份统一风险许可")
}

func TestAsyncImageHandlerSubmitRegistersAndReleasesUnifiedBillingRiskLease(t *testing.T) {
	store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
	tasks := service.NewImageTaskServiceWithUploader(store, nil, time.Hour, time.Minute)
	riskStore := &handlerBillingRiskBudgetStore{}
	settings := newEnabledBillingRiskSettingService(t)
	admission := service.NewBillingRiskAdmissionService(service.NewBillingRiskGuard(riskStore, settings), nil, nil, &config.Config{})
	h := &AsyncImageHandler{
		tasks:  tasks,
		openAI: &OpenAIGatewayHandler{billingRiskAdmission: admission},
	}
	release := make(chan struct{})
	h.execute = func(_ string, c *gin.Context) {
		<-release
		c.JSON(http.StatusOK, gin.H{"data": []gin.H{{"url": "https://example.test/image.png"}}})
	}
	router := asyncImageRiskTestRouter(h)

	response := submitAsyncImageRiskTestRequest(router)
	require.Equal(t, http.StatusAccepted, response.Code)
	require.Equal(t, 1, riskStore.leaseCount(), "异步任务提交后必须进入统一风险预算")

	close(release)
	require.Eventually(t, func() bool {
		return riskStore.leaseCount() == 0
	}, time.Second, 10*time.Millisecond, "未移交账务的异步任务许可必须在任务结束后释放")
}

func TestAsyncImageHandlerRefreshesBillingBalanceBeforeUnifiedRiskLease(t *testing.T) {
	store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
	tasks := service.NewImageTaskServiceWithUploader(store, nil, time.Hour, time.Minute)
	riskStore := &asyncImageRiskRecordingStore{acquired: make(chan service.BillingRiskAcquireRequest, 1)}
	settings := newEnabledBillingRiskSettingService(t)
	cfg := &config.Config{}
	admission := service.NewBillingRiskAdmissionService(service.NewBillingRiskGuard(riskStore, settings), nil, nil, cfg)
	billingCache := service.NewBillingCacheService(nil, &asyncImageBalanceUserRepoStub{balance: 0.2}, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCache.Stop)
	h := &AsyncImageHandler{
		tasks: tasks,
		openAI: &OpenAIGatewayHandler{
			billingCacheService:  billingCache,
			billingRiskAdmission: admission,
		},
	}
	release := make(chan struct{})
	h.execute = func(_ string, c *gin.Context) {
		<-release
		c.JSON(http.StatusOK, gin.H{"data": []gin.H{{"url": "https://example.test/image.png"}}})
	}

	response := submitAsyncImageRiskTestRequest(asyncImageRiskTestRouterWithBalance(h, 20))
	require.Equal(t, http.StatusAccepted, response.Code)
	request := <-riskStore.acquired
	require.Equal(t, int64(200_000), request.BalanceMicros, "统一风险准入必须使用当前 Billing 余额，而不是认证缓存旧值")

	close(release)
	require.Eventually(t, func() bool {
		return riskStore.leaseCount() == 0
	}, time.Second, 10*time.Millisecond)
}

func TestAsyncImageHandlerDefersCollectedUsageUntilTaskCompletes(t *testing.T) {
	store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
	tasks := service.NewImageTaskServiceWithUploader(store, nil, time.Hour, time.Minute)
	riskStore := &handlerBillingRiskBudgetStore{}
	settings := newEnabledBillingRiskSettingService(t)
	admission := service.NewBillingRiskAdmissionService(service.NewBillingRiskGuard(riskStore, settings), nil, nil, &config.Config{})
	h := &AsyncImageHandler{
		tasks:  tasks,
		openAI: &OpenAIGatewayHandler{billingRiskAdmission: admission},
	}
	collected := make(chan struct{})
	finish := make(chan struct{})
	var charged atomic.Bool
	h.execute = func(_ string, c *gin.Context) {
		lease := billingRiskLeaseFromContext(c.Request.Context())
		require.NotNil(t, lease)
		permit := lease.Handoff()
		require.NotNil(t, permit)
		h.openAI.submitBillingRiskUsageRecordTask(c.Request.Context(), permit, nil, func(ctx context.Context) {
			charged.Store(true)
			require.NoError(t, permit.Release(ctx))
		})
		close(collected)
		<-finish
		c.JSON(http.StatusOK, gin.H{"data": []gin.H{{"url": "https://example.test/image.png"}}})
	}

	response := submitAsyncImageRiskTestRequest(asyncImageRiskTestRouter(h))
	require.Equal(t, http.StatusAccepted, response.Code)
	<-collected
	require.False(t, charged.Load(), "任务仍在执行时不能触发该图片任务自身扣费")

	close(finish)
	require.Eventually(t, charged.Load, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		return riskStore.leaseCount() == 0
	}, time.Second, 10*time.Millisecond)
}

func asyncImageRiskTestRouter(h *AsyncImageHandler) *gin.Engine {
	return asyncImageRiskTestRouterWithBalance(h, 1)
}

func asyncImageRiskTestRouterWithBalance(h *AsyncImageHandler, balance float64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		groupID := int64(3)
		group := &service.Group{ID: groupID, Platform: service.PlatformOpenAI, AllowImageGeneration: true, RateMultiplier: 1}
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
			ID:      9,
			UserID:  7,
			GroupID: &groupID,
			Group:   group,
			User:    &service.User{ID: 7, Balance: balance},
		})
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 7, Concurrency: 1})
		c.Next()
	})
	router.POST("/v1/images/generations/async", h.Submit)
	return router
}

func submitAsyncImageRiskTestRequest(router http.Handler) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations/async", strings.NewReader(`{"model":"gpt-image-1","prompt":"cat"}`))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
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
