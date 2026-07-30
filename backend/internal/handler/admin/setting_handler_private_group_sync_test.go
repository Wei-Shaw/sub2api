//go:build unit

package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type syncProvisionerStub struct {
	result *service.SyncResult
	err    error
	calls  int
}

func (s *syncProvisionerStub) ProvisionPrivatePlatformGroups(context.Context, int64) (*service.ProvisionResult, error) {
	return &service.ProvisionResult{}, nil
}
func (s *syncProvisionerStub) RevokePrivatePlatformGroups(context.Context, int64) (*service.RevokeResult, error) {
	return &service.RevokeResult{}, nil
}
func (s *syncProvisionerStub) SyncPrivateSubscriptionExpiresAt(context.Context) (*service.SyncResult, error) {
	s.calls++
	return s.result, s.err
}
func (s *syncProvisionerStub) AfterCommit(context.Context, *service.ProvisionResult)     {}
func (s *syncProvisionerStub) AfterRevokeCommit(context.Context, *service.RevokeResult) {}

type forceAuditRepo struct {
	inserted []*service.AuditLog
}

func (r *forceAuditRepo) BatchInsert(context.Context, []*service.AuditLog) (int64, error) {
	return 0, nil
}
func (r *forceAuditRepo) Insert(_ context.Context, log *service.AuditLog) error {
	cp := *log
	r.inserted = append(r.inserted, &cp)
	return nil
}
func (r *forceAuditRepo) List(context.Context, *service.AuditLogFilter) (*service.AuditLogList, error) {
	return &service.AuditLogList{}, nil
}
func (r *forceAuditRepo) GetByID(context.Context, int64) (*service.AuditLog, error) {
	return nil, service.ErrAuditLogNotFound
}
func (r *forceAuditRepo) Count(context.Context) (int64, error)       { return 0, nil }
func (r *forceAuditRepo) TruncateAll(context.Context) error          { return nil }
func (r *forceAuditRepo) DeleteBefore(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}

func TestSyncPrivateGroupExpires_RequiresConfirm(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prov := &syncProvisionerStub{}
	h := NewSettingHandler(nil, nil, nil, nil, nil, nil, nil)
	h.SetPrivateGroupSyncDeps(prov, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body, _ := json.Marshal(map[string]any{"confirm": false})
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/private-group-expires/sync", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.SyncPrivateGroupExpires(c)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Equal(t, 0, prov.calls)
}

func TestSyncPrivateGroupExpires_SuccessForceAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	expires := time.Date(2099, 6, 15, 23, 59, 59, 0, time.FixedZone("CST", 8*3600))
	prov := &syncProvisionerStub{result: &service.SyncResult{
		Updated:   7,
		ExpiresAt: expires,
		Status:    service.SubscriptionStatusActive,
	}}
	repo := &forceAuditRepo{}
	audit := service.NewAuditLogService(repo, nil)
	h := NewSettingHandler(nil, nil, nil, nil, nil, nil, nil)
	h.SetPrivateGroupSyncDeps(prov, audit)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body, _ := json.Marshal(map[string]any{"confirm": true})
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/private-group-expires/sync", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.SyncPrivateGroupExpires(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 1, prov.calls)
	require.Len(t, repo.inserted, 1)
	require.Equal(t, service.AuditActionPrivateGroupExpiresSync, repo.inserted[0].Action)
	require.EqualValues(t, 7, repo.inserted[0].Extra["updated"])
	require.Equal(t, service.SubscriptionStatusActive, repo.inserted[0].Extra["status"])
}

func TestSyncPrivateGroupExpires_UnconfiguredDate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prov := &syncProvisionerStub{err: service.ErrPrivateGroupExpiresDateNotConfigured}
	h := NewSettingHandler(nil, nil, nil, nil, nil, nil, nil)
	h.SetPrivateGroupSyncDeps(prov, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body, _ := json.Marshal(map[string]any{"confirm": true})
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/private-group-expires/sync", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.SyncPrivateGroupExpires(c)
	require.Equal(t, http.StatusBadRequest, w.Code)
}
