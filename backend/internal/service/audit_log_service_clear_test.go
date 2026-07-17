package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type auditLogClearRepoStub struct {
	mu        sync.Mutex
	logs      []*AuditLog
	batchErr  error
	clearErr  error
	clearRuns int
}

func (r *auditLogClearRepoStub) BatchInsert(_ context.Context, logs []*AuditLog) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.batchErr != nil {
		return 0, r.batchErr
	}
	r.logs = append(r.logs, logs...)
	return int64(len(logs)), nil
}

func (r *auditLogClearRepoStub) ClearAll(_ context.Context, trace *AuditLog) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clearRuns++
	if r.clearErr != nil {
		return 0, r.clearErr
	}
	deleted := int64(len(r.logs))
	traceCopy := *trace
	traceCopy.Extra = cloneAuditLogExtraForTest(trace.Extra)
	traceCopy.Extra["deleted_rows"] = deleted
	r.logs = []*AuditLog{&traceCopy}
	return deleted, nil
}

func (r *auditLogClearRepoStub) List(context.Context, *AuditLogFilter) (*AuditLogList, error) {
	return nil, nil
}

func (r *auditLogClearRepoStub) GetByID(context.Context, int64) (*AuditLog, error) {
	return nil, ErrAuditLogNotFound
}

func (r *auditLogClearRepoStub) DeleteBefore(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}

func (r *auditLogClearRepoStub) snapshot() []*AuditLog {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*AuditLog(nil), r.logs...)
}

func cloneAuditLogExtraForTest(extra map[string]any) map[string]any {
	cloned := make(map[string]any, len(extra)+1)
	for key, value := range extra {
		cloned[key] = value
	}
	return cloned
}

func TestAuditLogServiceClearAllOrdersQueuedRecords(t *testing.T) {
	repo := &auditLogClearRepoStub{}
	svc := NewAuditLogService(repo, nil)
	svc.Start()
	svc.Record(&AuditLog{Action: "before.clear"})

	deleted, err := svc.ClearAll(context.Background(), &AuditLog{ActorEmail: "admin@example.test"})
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)

	svc.Record(&AuditLog{Action: "after.clear"})
	svc.Stop()

	logs := repo.snapshot()
	require.Len(t, logs, 2)
	require.Equal(t, AuditActionAuditLogClear, logs[0].Action)
	require.Equal(t, int64(1), logs[0].Extra["deleted_rows"])
	require.Equal(t, "after.clear", logs[1].Action)
}

func TestAuditLogServiceClearAllStopsWhenPendingFlushFails(t *testing.T) {
	repo := &auditLogClearRepoStub{batchErr: errors.New("database unavailable")}
	svc := NewAuditLogService(repo, nil)
	svc.Start()
	svc.Record(&AuditLog{Action: "before.clear"})

	_, err := svc.ClearAll(context.Background(), &AuditLog{})

	require.Error(t, err)
	require.Contains(t, err.Error(), "flush pending audit logs")
	require.Equal(t, 0, repo.clearRuns)
	svc.Stop()
}

func TestAuditLogServiceClearAllRequiresStartedServiceAndTrace(t *testing.T) {
	svc := NewAuditLogService(&auditLogClearRepoStub{}, nil)

	_, err := svc.ClearAll(context.Background(), &AuditLog{})
	require.EqualError(t, err, "audit log service is not started")

	svc.Start()
	_, err = svc.ClearAll(context.Background(), nil)
	require.EqualError(t, err, "audit clear trace is required")
	svc.Stop()

	_, err = svc.ClearAll(context.Background(), &AuditLog{})
	require.EqualError(t, err, "audit log service is stopping")
}
