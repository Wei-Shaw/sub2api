//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type systemHandlerUpdateServiceStub struct {
	performErr            error
	updateInfo            *service.UpdateInfo
	checkErr              error
	checkForces           []bool
	performCall           int
	performCtxErr         error
	performHasDeadline    bool
	rollbackCall          int
	rollbackToCall        int
	rollbackToCtxErr      error
	rollbackToHasDeadline bool
	rollbackToVersions    []string
	rollbackToErr         error
	rollbackVersions      []service.RollbackVersion
	rollbackVersionsErr   error
	rollbackVersionsCall  int
	deploymentMode        string
	managedJob            *service.DeploymentJob
	managedUpdateCalls    int
	managedRollbackCalls  int
	managedRollbackTarget string
}

func (s *systemHandlerUpdateServiceStub) CheckUpdate(_ context.Context, force bool) (*service.UpdateInfo, error) {
	s.checkForces = append(s.checkForces, force)
	return s.updateInfo, s.checkErr
}

func (s *systemHandlerUpdateServiceStub) PerformUpdate(ctx context.Context) error {
	s.performCall++
	s.performCtxErr = ctx.Err()
	_, s.performHasDeadline = ctx.Deadline()
	return s.performErr
}

func (s *systemHandlerUpdateServiceStub) Rollback() error {
	s.rollbackCall++
	return nil
}

func (s *systemHandlerUpdateServiceStub) ListRollbackVersions(context.Context) ([]service.RollbackVersion, error) {
	s.rollbackVersionsCall++
	return s.rollbackVersions, s.rollbackVersionsErr
}

func (s *systemHandlerUpdateServiceStub) RollbackToVersion(ctx context.Context, version string) error {
	s.rollbackToCall++
	s.rollbackToCtxErr = ctx.Err()
	_, s.rollbackToHasDeadline = ctx.Deadline()
	s.rollbackToVersions = append(s.rollbackToVersions, version)
	return s.rollbackToErr
}

func (s *systemHandlerUpdateServiceStub) DeploymentMode() string { return s.deploymentMode }

func (s *systemHandlerUpdateServiceStub) StartManagedUpdate(context.Context, string) (*service.DeploymentJob, error) {
	s.managedUpdateCalls++
	return s.managedJob, nil
}

func (s *systemHandlerUpdateServiceStub) StartManagedRollback(_ context.Context, version, _ string) (*service.DeploymentJob, error) {
	s.managedRollbackCalls++
	s.managedRollbackTarget = version
	return s.managedJob, nil
}

func (s *systemHandlerUpdateServiceStub) DeploymentJob(context.Context, string) (*service.DeploymentJob, error) {
	return s.managedJob, nil
}

func (s *systemHandlerUpdateServiceStub) CurrentDeploymentJob(context.Context) (*service.DeploymentJob, error) {
	return s.managedJob, nil
}

type systemUpdateResponseEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Message         string                 `json:"message"`
		AlreadyUpToDate bool                   `json:"already_up_to_date"`
		CurrentVersion  string                 `json:"current_version"`
		LatestVersion   string                 `json:"latest_version"`
		OperationID     string                 `json:"operation_id"`
		Job             *service.DeploymentJob `json:"job"`
	} `json:"data"`
}

func TestSystemHandlerStartsManagedDockerUpdateWithoutReplacingBinary(t *testing.T) {
	updateSvc := &systemHandlerUpdateServiceStub{
		deploymentMode: service.DeploymentModeDockerManaged,
		managedJob: &service.DeploymentJob{
			ID:            "sysop-managed-update",
			Action:        "update",
			TargetVersion: "0.1.165-ts.1",
			Status:        "running",
			Stage:         "pulling",
		},
	}
	repo := newMemoryIdempotencyRepoStub()
	router := newSystemHandlerTestRouter(t, updateSvc, repo)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/system/update", nil)
	req.Header.Set("Idempotency-Key", "managed-update")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, updateSvc.managedUpdateCalls)
	require.Zero(t, updateSvc.performCall)
	var body systemUpdateResponseEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.NotNil(t, body.Data.Job)
	require.Equal(t, "sysop-managed-update", body.Data.Job.ID)
	requireSystemLockStatus(t, repo, service.IdempotencyStatusSucceeded)
}

func TestSystemHandlerStartsManagedDockerRollback(t *testing.T) {
	updateSvc := &systemHandlerUpdateServiceStub{
		deploymentMode: service.DeploymentModeDockerManaged,
		managedJob: &service.DeploymentJob{
			ID:            "sysop-managed-rollback",
			Action:        "rollback",
			TargetVersion: "0.1.163-ts.1",
			Status:        "running",
			Stage:         "pulling",
		},
	}
	repo := newMemoryIdempotencyRepoStub()
	router := newSystemHandlerTestRouter(t, updateSvc, repo)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/system/rollback", strings.NewReader(`{"version":"0.1.163-ts.1"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "managed-rollback")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, updateSvc.managedRollbackCalls)
	require.Equal(t, "0.1.163-ts.1", updateSvc.managedRollbackTarget)
	require.Zero(t, updateSvc.rollbackToCall)
}

type systemUpdateErrorEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Reason  string `json:"reason"`
}

func newSystemHandlerTestRouter(t *testing.T, updateSvc *systemHandlerUpdateServiceStub, repo *memoryIdempotencyRepoStub) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	service.SetDefaultIdempotencyCoordinator(nil)
	t.Cleanup(func() {
		service.SetDefaultIdempotencyCoordinator(nil)
	})

	lockSvc := service.NewSystemOperationLockService(repo, service.IdempotencyConfig{
		ProcessingTimeout:  time.Second,
		SystemOperationTTL: time.Minute,
	})
	handler := NewSystemHandler(updateSvc, lockSvc)

	router := gin.New()
	router.POST("/api/v1/admin/system/update", handler.PerformUpdate)
	router.POST("/api/v1/admin/system/rollback", handler.Rollback)
	router.POST("/api/v1/admin/system/restart", handler.RestartService)
	router.GET("/api/v1/admin/system/rollback-versions", handler.GetRollbackVersions)
	return router
}

func TestSystemHandlerRejectsManagedDockerRestartBeforeObserveOnlyIdempotency(t *testing.T) {
	updateSvc := &systemHandlerUpdateServiceStub{
		deploymentMode: service.DeploymentModeDockerManaged,
	}
	repo := newMemoryIdempotencyRepoStub()
	router := newSystemHandlerTestRouter(t, updateSvc, repo)
	cfg := service.DefaultIdempotencyConfig()
	cfg.ObserveOnly = true
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(repo, cfg))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/system/restart", nil)
	// Deliberately omit Idempotency-Key: observe-only mode must not allow the
	// unsafe restart path to run before or during a host deployment.
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	var body systemUpdateErrorEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "DOCKER_MANAGED_RESTART_UNSUPPORTED", body.Reason)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Empty(t, repo.data, "rejected restart must not claim an idempotency record or system lock")
}

func requireSystemLockStatus(t *testing.T, repo *memoryIdempotencyRepoStub, wantStatus string) {
	t.Helper()
	repo.mu.Lock()
	defer repo.mu.Unlock()

	for _, record := range repo.data {
		if record.Status == wantStatus {
			return
		}
	}
	t.Fatalf("system lock status %q not found in records: %#v", wantStatus, repo.data)
}

func TestSystemHandlerPerformUpdateAlreadyUpToDateReturnsOK(t *testing.T) {
	updateSvc := &systemHandlerUpdateServiceStub{
		performErr: service.ErrNoUpdateAvailable,
		updateInfo: &service.UpdateInfo{
			CurrentVersion: "0.1.132",
			LatestVersion:  "0.1.132",
			HasUpdate:      false,
		},
	}
	repo := newMemoryIdempotencyRepoStub()
	router := newSystemHandlerTestRouter(t, updateSvc, repo)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/system/update", nil)
	req.Header.Set("Idempotency-Key", "already-up-to-date")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, updateSvc.performCall)
	require.Equal(t, []bool{false}, updateSvc.checkForces)
	requireSystemLockStatus(t, repo, service.IdempotencyStatusSucceeded)

	var body systemUpdateResponseEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 0, body.Code)
	require.Equal(t, "success", body.Message)
	require.Equal(t, "Already up to date", body.Data.Message)
	require.True(t, body.Data.AlreadyUpToDate)
	require.Equal(t, "0.1.132", body.Data.CurrentVersion)
	require.Equal(t, "0.1.132", body.Data.LatestVersion)
	require.NotEmpty(t, body.Data.OperationID)
}

func TestSystemHandlerPerformUpdateFailureStillReturnsInternalError(t *testing.T) {
	updateSvc := &systemHandlerUpdateServiceStub{
		performErr: errors.New("download failed"),
	}
	repo := newMemoryIdempotencyRepoStub()
	router := newSystemHandlerTestRouter(t, updateSvc, repo)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/system/update", nil)
	req.Header.Set("Idempotency-Key", "real-failure")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, 1, updateSvc.performCall)
	require.Empty(t, updateSvc.checkForces)
	requireSystemLockStatus(t, repo, service.IdempotencyStatusFailedRetryable)

	var body systemUpdateErrorEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, http.StatusInternalServerError, body.Code)
	require.Equal(t, "internal error", body.Message)
}

// TestSystemHandlerPerformUpdateSurvivesClientDisconnect reproduces #4504:
// the browser or a reverse proxy (axios 30s default, nginx proxy_read_timeout
// 60s) aborts the long-running update request and cancels the request
// context. The download must keep running on a detached, bounded context
// instead of dying with "download failed: context canceled".
func TestSystemHandlerPerformUpdateSurvivesClientDisconnect(t *testing.T) {
	updateSvc := &systemHandlerUpdateServiceStub{}
	repo := newMemoryIdempotencyRepoStub()
	router := newSystemHandlerTestRouter(t, updateSvc, repo)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/system/update", nil)
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	req = req.WithContext(canceledCtx)
	req.Header.Set("Idempotency-Key", "disconnected-update")
	router.ServeHTTP(rec, req)

	require.Equal(t, 1, updateSvc.performCall)
	require.NoError(t, updateSvc.performCtxErr,
		"update must not observe the canceled request context")
	require.True(t, updateSvc.performHasDeadline,
		"detached update context must still be bounded by a deadline")
	requireSystemLockStatus(t, repo, service.IdempotencyStatusSucceeded)
}

func TestSystemHandlerRollbackToVersionSurvivesClientDisconnect(t *testing.T) {
	updateSvc := &systemHandlerUpdateServiceStub{}
	repo := newMemoryIdempotencyRepoStub()
	router := newSystemHandlerTestRouter(t, updateSvc, repo)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/system/rollback",
		strings.NewReader(`{"version":"0.1.146"}`))
	req.Header.Set("Content-Type", "application/json")
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	req = req.WithContext(canceledCtx)
	req.Header.Set("Idempotency-Key", "disconnected-rollback")
	router.ServeHTTP(rec, req)

	require.Equal(t, 1, updateSvc.rollbackToCall)
	require.NoError(t, updateSvc.rollbackToCtxErr,
		"versioned rollback must not observe the canceled request context")
	require.True(t, updateSvc.rollbackToHasDeadline,
		"detached rollback context must still be bounded by a deadline")
	requireSystemLockStatus(t, repo, service.IdempotencyStatusSucceeded)
}

func TestSystemHandlerRollbackWithoutBodyUsesLegacyBackup(t *testing.T) {
	updateSvc := &systemHandlerUpdateServiceStub{}
	repo := newMemoryIdempotencyRepoStub()
	router := newSystemHandlerTestRouter(t, updateSvc, repo)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/system/rollback", nil)
	req.Header.Set("Idempotency-Key", "legacy-rollback")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, updateSvc.rollbackCall)
	require.Equal(t, 0, updateSvc.rollbackToCall)
	requireSystemLockStatus(t, repo, service.IdempotencyStatusSucceeded)
}

func TestSystemHandlerRollbackWithVersionCallsRollbackToVersion(t *testing.T) {
	updateSvc := &systemHandlerUpdateServiceStub{}
	repo := newMemoryIdempotencyRepoStub()
	router := newSystemHandlerTestRouter(t, updateSvc, repo)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/system/rollback",
		strings.NewReader(`{"version":"0.1.146"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "rollback-to-146")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 0, updateSvc.rollbackCall)
	require.Equal(t, 1, updateSvc.rollbackToCall)
	require.Equal(t, []string{"0.1.146"}, updateSvc.rollbackToVersions)
	requireSystemLockStatus(t, repo, service.IdempotencyStatusSucceeded)

	var body systemUpdateResponseEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 0, body.Code)
	require.Equal(t, "Rollback completed. Please restart the service.", body.Data.Message)
}

func TestSystemHandlerRollbackWithDisallowedVersionReturnsBadRequest(t *testing.T) {
	updateSvc := &systemHandlerUpdateServiceStub{
		rollbackToErr: service.ErrRollbackVersionNotAllowed,
	}
	repo := newMemoryIdempotencyRepoStub()
	router := newSystemHandlerTestRouter(t, updateSvc, repo)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/system/rollback",
		strings.NewReader(`{"version":"9.9.9"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "rollback-to-bad")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, 1, updateSvc.rollbackToCall)
}

func TestSystemHandlerGetRollbackVersions(t *testing.T) {
	updateSvc := &systemHandlerUpdateServiceStub{
		rollbackVersions: []service.RollbackVersion{
			{Version: "0.1.146", PublishedAt: "2026-07-07T00:00:00Z", HTMLURL: "https://example.com/v0.1.146"},
			{Version: "0.1.145", PublishedAt: "2026-07-06T00:00:00Z", HTMLURL: "https://example.com/v0.1.145"},
		},
	}
	repo := newMemoryIdempotencyRepoStub()
	router := newSystemHandlerTestRouter(t, updateSvc, repo)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/rollback-versions", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, updateSvc.rollbackVersionsCall)

	var body struct {
		Code int `json:"code"`
		Data struct {
			Versions []service.RollbackVersion `json:"versions"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 0, body.Code)
	require.Len(t, body.Data.Versions, 2)
	require.Equal(t, "0.1.146", body.Data.Versions[0].Version)
}

func TestSystemHandlerGetRollbackVersionsError(t *testing.T) {
	updateSvc := &systemHandlerUpdateServiceStub{
		rollbackVersionsErr: errors.New("github unavailable"),
	}
	repo := newMemoryIdempotencyRepoStub()
	router := newSystemHandlerTestRouter(t, updateSvc, repo)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/rollback-versions", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
}
