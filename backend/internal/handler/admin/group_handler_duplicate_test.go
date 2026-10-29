//go:build unit

package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type duplicateGroupAdminServiceStub struct {
	service.AdminService
	group        *service.Group
	calls        int
	recoverCalls int
	groupID      int64
	actorScope   string
	operationKey string
	recoverScope string
	recoverKey   string
	created      bool
}

func (s *duplicateGroupAdminServiceStub) DuplicateGroup(_ context.Context, groupID int64, actorScope, operationKey string) (*service.Group, error) {
	s.calls++
	s.groupID = groupID
	s.actorScope = actorScope
	s.operationKey = operationKey
	s.created = true
	return s.group, nil
}

func (s *duplicateGroupAdminServiceStub) RecoverDuplicateGroup(_ context.Context, _ int64, actorScope, operationKey string) (*service.Group, error) {
	s.recoverCalls++
	s.recoverScope = actorScope
	s.recoverKey = operationKey
	if !s.created {
		return nil, nil
	}
	return s.group, nil
}

func setupDuplicateGroupRouter(t *testing.T, svc service.AdminService) *gin.Engine {
	t.Helper()
	previousCoordinator := service.DefaultIdempotencyCoordinator()
	service.SetDefaultIdempotencyCoordinator(nil)
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(previousCoordinator) })

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 77})
		c.Next()
	})
	handler := NewGroupHandler(svc, nil, nil)
	router.POST("/api/v1/admin/groups/:id/duplicate", handler.Duplicate)
	return router
}

func duplicateGroupHandlerFixture() *service.Group {
	return &service.Group{
		ID:                   43,
		Name:                 "primary (Copy)",
		Platform:             service.PlatformAnthropic,
		Status:               "inactive",
		RateMultiplier:       1,
		AccountCount:         3,
		ActiveAccountCount:   2,
		DuplicateOperationID: "internal-operation-must-not-leak",
		ModelRouting:         map[string][]int64{"claude-*": {7}},
	}
}

func TestDuplicateGroupHandlerReturnsAdminDTOWithoutOperationMetadata(t *testing.T) {
	svc := &duplicateGroupAdminServiceStub{group: duplicateGroupHandlerFixture()}
	router := setupDuplicateGroupRouter(t, svc)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/groups/42/duplicate", nil)

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 1, svc.calls)
	require.Contains(t, recorder.Body.String(), `"name":"primary (Copy)"`)
	require.Contains(t, recorder.Body.String(), `"status":"inactive"`)
	require.Contains(t, recorder.Body.String(), `"account_count":3`)
	require.NotContains(t, recorder.Body.String(), "duplicate_operation_id")
	require.NotContains(t, recorder.Body.String(), "internal-operation-must-not-leak")
}

func TestDuplicateGroupHandlerRejectsInvalidID(t *testing.T) {
	for _, id := range []string{"not-a-number", "0", "-1"} {
		t.Run(id, func(t *testing.T) {
			svc := &duplicateGroupAdminServiceStub{}
			router := setupDuplicateGroupRouter(t, svc)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/groups/"+id+"/duplicate", nil)

			router.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Zero(t, svc.calls)
		})
	}
}

func TestDuplicateGroupHandlerReplaysSameIdempotencyKey(t *testing.T) {
	svc := &duplicateGroupAdminServiceStub{group: duplicateGroupHandlerFixture()}
	router := setupDuplicateGroupRouter(t, svc)
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(newMemoryIdempotencyRepoStub(), service.DefaultIdempotencyConfig()))

	call := func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/groups/42/duplicate", nil)
		request.Header.Set("Idempotency-Key", "duplicate-group-42")
		router.ServeHTTP(recorder, request)
		return recorder
	}

	first := call()
	second := call()

	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, http.StatusOK, second.Code)
	require.Equal(t, 1, svc.calls)
	require.Equal(t, int64(42), svc.groupID)
	require.Equal(t, "admin:77", svc.actorScope)
	require.Equal(t, "duplicate-group-42", svc.operationKey)
	require.Equal(t, "true", second.Header().Get("X-Idempotency-Replayed"))
}

func TestDuplicateGroupHandlerRecoversAfterMarkSucceededFailure(t *testing.T) {
	svc := &duplicateGroupAdminServiceStub{group: duplicateGroupHandlerFixture()}
	router := setupDuplicateGroupRouter(t, svc)
	repo := &failOnceMarkSucceededRepo{memoryIdempotencyRepoStub: newMemoryIdempotencyRepoStub(), failNext: true}
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(repo, service.DefaultIdempotencyConfig()))

	call := func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/groups/42/duplicate", nil)
		request.Header.Set("Idempotency-Key", "duplicate-group-42-recovery")
		router.ServeHTTP(recorder, request)
		return recorder
	}

	first := call()
	second := call()

	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, http.StatusOK, second.Code)
	require.Equal(t, "true", first.Header().Get("X-Idempotency-Recovered"))
	require.Equal(t, "true", second.Header().Get("X-Idempotency-Recovered"))
	require.Equal(t, 1, svc.calls, "ambiguous retries must not repeat the create side effect")
	require.Equal(t, 2, svc.recoverCalls)
	require.Equal(t, "admin:77", svc.recoverScope)
	require.Equal(t, "duplicate-group-42-recovery", svc.recoverKey)
}
