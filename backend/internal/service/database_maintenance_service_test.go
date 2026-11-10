package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type maintenanceRepoStub struct {
	stats    *DatabaseStats
	job      *DatabaseMaintenanceJob
	session  *maintenanceSessionStub
	acquired int
}

func (r *maintenanceRepoStub) Stats(context.Context) (*DatabaseStats, error) { return r.stats, nil }
func (r *maintenanceRepoStub) LatestJob(context.Context) (*DatabaseMaintenanceJob, error) {
	return r.job, nil
}
func (r *maintenanceRepoStub) Acquire(context.Context) (DatabaseMaintenanceSession, error) {
	r.acquired++
	return r.session, nil
}

type maintenanceSessionStub struct {
	recomputed  bool
	mu          sync.Mutex
	saved       []DatabaseMaintenanceJob
	batches     []int64
	batchErr    error
	closed      chan struct{}
	entered     chan struct{}
	proceed     chan struct{}
	vacuumTable string
	vacuumFull  bool
}

func (s *maintenanceSessionStub) SaveJob(_ context.Context, job *DatabaseMaintenanceJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saved = append(s.saved, *job)
	return nil
}
func (s *maintenanceSessionStub) CleanupBatch(ctx context.Context, _ string, _ time.Time) (int64, error) {
	if s.entered != nil {
		close(s.entered)
		s.entered = nil
		<-s.proceed
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if len(s.batches) == 0 {
		return 0, s.batchErr
	}
	n := s.batches[0]
	s.batches = s.batches[1:]
	return n, nil
}
func (s *maintenanceSessionStub) RecomputeUsage(context.Context, time.Time) error {
	s.recomputed = true
	return nil
}
func (s *maintenanceSessionStub) Vacuum(_ context.Context, table string, full bool) error {
	s.vacuumTable = table
	s.vacuumFull = full
	return nil
}
func (s *maintenanceSessionStub) Close() { close(s.closed) }

func newMaintenanceTestService() (*DatabaseMaintenanceService, *maintenanceRepoStub, *maintenanceSessionStub) {
	session := &maintenanceSessionStub{closed: make(chan struct{})}
	repo := &maintenanceRepoStub{stats: &DatabaseStats{Tables: []DatabaseTableStats{{Name: "usage_logs"}, {Name: "users"}, {Name: "ops_system_logs"}}}, session: session}
	return NewDatabaseMaintenanceService(repo), repo, session
}

func TestDatabaseMaintenanceRejectsUnsafeRequests(t *testing.T) {
	cases := []DatabaseMaintenanceRequest{
		{Operation: "drop", Tables: []string{"usage_logs"}, Confirm: true},
		{Operation: "cleanup", Tables: []string{"users"}, RetentionDays: 30, Confirm: true},
		{Operation: "cleanup", Tables: []string{"usage_logs"}, RetentionDays: 0, Confirm: true},
		{Operation: "cleanup", Tables: []string{"usage_logs"}, RetentionDays: -1, Confirm: true},
		{Operation: "cleanup", Tables: []string{"usage_logs"}, RetentionDays: 36501, Confirm: true},
		{Operation: "vacuum", Tables: []string{"usage_logs; DROP TABLE users"}, Confirm: true},
		{Operation: "vacuum", Tables: []string{"usage_logs"}},
		{Operation: "vacuum", Confirm: true},
	}
	for _, req := range cases {
		svc, repo, _ := newMaintenanceTestService()
		_, err := svc.Start(context.Background(), req, 1)
		require.Error(t, err, "%+v", req)
		require.Zero(t, repo.acquired)
	}
	svc, repo, _ := newMaintenanceTestService()
	_, err := svc.Start(context.Background(), DatabaseMaintenanceRequest{Operation: "vacuum", Tables: []string{"users"}, Confirm: true}, 0)
	require.Error(t, err)
	require.Zero(t, repo.acquired)
}

func TestDatabaseMaintenanceContinuesAfterRequestCancellation(t *testing.T) {
	svc, _, session := newMaintenanceTestService()
	session.batches = []int64{5000, 17, 0}
	entered := make(chan struct{})
	session.entered, session.proceed = entered, make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	job, err := svc.Start(ctx, DatabaseMaintenanceRequest{Operation: "cleanup", Tables: []string{"usage_logs", "usage_logs"}, RetentionDays: 30, Confirm: true}, 7)
	require.NoError(t, err)
	<-entered
	cancel()
	close(session.proceed)
	select {
	case <-session.closed:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not finish")
	}
	require.Equal(t, "running", job.Status)
	require.Equal(t, []string{"usage_logs"}, job.Tables)
	require.WithinDuration(t, time.Now().UTC().AddDate(0, 0, -30), *job.Cutoff, time.Second)
	last := session.saved[len(session.saved)-1]
	require.Equal(t, "succeeded", last.Status)
	require.Equal(t, int64(5017), last.DeletedRows)
	require.True(t, session.recomputed)
	require.Equal(t, 1, last.CompletedTables)
	require.Equal(t, int64(7), last.CreatedBy)
	require.NotNil(t, last.FinishedAt)
}

func TestDatabaseMaintenancePreservesPartialProgressOnFailure(t *testing.T) {
	svc, _, session := newMaintenanceTestService()
	session.batches, session.batchErr = []int64{5000}, errors.New("disk failure")
	_, err := svc.Start(context.Background(), DatabaseMaintenanceRequest{Operation: "cleanup", Tables: []string{"usage_logs"}, RetentionDays: 7, Confirm: true}, 1)
	require.NoError(t, err)
	select {
	case <-session.closed:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not finish")
	}
	last := session.saved[len(session.saved)-1]
	require.Equal(t, "failed", last.Status)
	require.Equal(t, int64(5000), last.DeletedRows)
	require.Zero(t, last.CompletedTables)
	require.Contains(t, last.Error, "disk failure")
	require.True(t, session.recomputed)
}

func TestDatabaseMaintenanceFullVacuum(t *testing.T) {
	svc, _, session := newMaintenanceTestService()
	_, err := svc.Start(context.Background(), DatabaseMaintenanceRequest{Operation: "vacuum_full", Tables: []string{"users"}, Confirm: true}, 1)
	require.NoError(t, err)
	select {
	case <-session.closed:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not finish")
	}
	require.True(t, session.vacuumFull)
	require.Equal(t, "users", session.vacuumTable)
	require.Equal(t, "succeeded", session.saved[len(session.saved)-1].Status)
}

func TestDatabaseMaintenanceReportsInterruptedJob(t *testing.T) {
	svc, repo, _ := newMaintenanceTestService()
	repo.job = &DatabaseMaintenanceJob{Status: "running", StartedAt: time.Now().Add(-time.Hour), DeletedRows: 123}
	job, err := svc.LatestJob(context.Background())
	require.NoError(t, err)
	require.Equal(t, "failed", job.Status)
	require.Equal(t, int64(123), job.DeletedRows)
	require.Contains(t, job.Error, "interrupted")
}
