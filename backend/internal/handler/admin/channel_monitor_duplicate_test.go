//go:build unit

package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type duplicateChannelMonitorHandlerRepoStub struct {
	service.ChannelMonitorRepository
	source      *service.ChannelMonitor
	byOperation map[string]*service.ChannelMonitor
	createCalls int
}

func (r *duplicateChannelMonitorHandlerRepoStub) GetByID(_ context.Context, id int64) (*service.ChannelMonitor, error) {
	if r.source == nil || r.source.ID != id {
		return nil, service.ErrChannelMonitorNotFound
	}
	return r.source, nil
}

func (r *duplicateChannelMonitorHandlerRepoStub) Create(_ context.Context, monitor *service.ChannelMonitor) error {
	r.createCalls++
	monitor.ID = int64(100 + r.createCalls)
	stored := *monitor
	if stored.DuplicateOperationID != "" {
		if r.byOperation == nil {
			r.byOperation = make(map[string]*service.ChannelMonitor)
		}
		r.byOperation[stored.DuplicateOperationID] = &stored
	}
	return nil
}

func (r *duplicateChannelMonitorHandlerRepoStub) FindByDuplicateOperationID(_ context.Context, operationID string) (*service.ChannelMonitor, error) {
	monitor := r.byOperation[operationID]
	if monitor == nil {
		return nil, nil
	}
	cloned := *monitor
	return &cloned, nil
}

type duplicateChannelMonitorHandlerEncryptor struct{}

func (duplicateChannelMonitorHandlerEncryptor) Encrypt(plaintext string) (string, error) {
	return "ENC:" + plaintext, nil
}

func (duplicateChannelMonitorHandlerEncryptor) Decrypt(ciphertext string) (string, error) {
	return strings.TrimPrefix(ciphertext, "ENC:"), nil
}

func setupDuplicateChannelMonitorRouter(t *testing.T) (*gin.Engine, *duplicateChannelMonitorHandlerRepoStub) {
	t.Helper()
	previousCoordinator := service.DefaultIdempotencyCoordinator()
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(
		newMemoryIdempotencyRepoStub(),
		service.DefaultIdempotencyConfig(),
	))
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(previousCoordinator) })

	repo := &duplicateChannelMonitorHandlerRepoStub{
		source: &service.ChannelMonitor{
			ID:               42,
			Name:             "primary",
			Provider:         service.MonitorProviderOpenAI,
			APIMode:          service.MonitorAPIModeResponses,
			Endpoint:         "https://api.example.com",
			APIKey:           "ENC:top-secret",
			PrimaryModel:     "gpt-5.4-mini",
			ExtraModels:      []string{"gpt-5.4"},
			Enabled:          true,
			IntervalSeconds:  60,
			BodyOverrideMode: service.MonitorBodyOverrideModeOff,
		},
	}
	monitorService := service.NewChannelMonitorService(repo, duplicateChannelMonitorHandlerEncryptor{})
	handler := NewChannelMonitorHandler(monitorService)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 77})
		c.Next()
	})
	router.POST("/api/v1/admin/channel-monitors/:id/duplicate", handler.Duplicate)
	return router, repo
}

func TestDuplicateChannelMonitorHandlerRedactsKeyAndReplaysRetry(t *testing.T) {
	router, repo := setupDuplicateChannelMonitorRouter(t)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/channel-monitors/42/duplicate", nil)
	request.Header.Set("Idempotency-Key", "duplicate-channel-monitor-42")
	first := httptest.NewRecorder()
	router.ServeHTTP(first, request)

	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, 1, repo.createCalls)
	require.Contains(t, first.Body.String(), `"name":"primary (Copy)"`)
	require.Contains(t, first.Body.String(), `"api_key_masked":"top-***"`)
	require.Contains(t, first.Body.String(), `"created_by":77`)
	require.Contains(t, first.Body.String(), `"enabled":false`)
	require.NotContains(t, first.Body.String(), "top-secret")

	retryRequest := httptest.NewRequest(http.MethodPost, "/api/v1/admin/channel-monitors/42/duplicate", nil)
	retryRequest.Header.Set("Idempotency-Key", "duplicate-channel-monitor-42")
	retry := httptest.NewRecorder()
	router.ServeHTTP(retry, retryRequest)

	require.Equal(t, http.StatusOK, retry.Code)
	require.Equal(t, "true", retry.Header().Get("X-Idempotency-Replayed"))
	require.Equal(t, 1, repo.createCalls)
	require.JSONEq(t, first.Body.String(), retry.Body.String())
}

func TestDuplicateChannelMonitorHandlerRejectsInvalidID(t *testing.T) {
	router, repo := setupDuplicateChannelMonitorRouter(t)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/channel-monitors/not-a-number/duplicate", nil)
	request.Header.Set("Idempotency-Key", "duplicate-channel-monitor-invalid")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Zero(t, repo.createCalls)
}

func TestDuplicateChannelMonitorHandlerRecoversAfterMarkSucceededFailure(t *testing.T) {
	router, repo := setupDuplicateChannelMonitorRouter(t)
	idempotencyRepo := &failOnceMarkSucceededRepo{
		memoryIdempotencyRepoStub: newMemoryIdempotencyRepoStub(),
		failNext:                  true,
	}
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(
		idempotencyRepo,
		service.DefaultIdempotencyConfig(),
	))

	call := func() *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/channel-monitors/42/duplicate", nil)
		request.Header.Set("Idempotency-Key", "duplicate-channel-monitor-recovery")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder
	}

	first := call()
	second := call()

	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, http.StatusOK, second.Code)
	require.Equal(t, "true", first.Header().Get("X-Idempotency-Recovered"))
	require.Equal(t, "true", second.Header().Get("X-Idempotency-Recovered"))
	require.Equal(t, 1, repo.createCalls, "ambiguous retries must not repeat the create side effect")
	require.Contains(t, second.Body.String(), `"id":101`)
	require.Contains(t, second.Body.String(), `"api_key_masked":"top-***"`)
	require.NotContains(t, second.Body.String(), "top-secret")
}
