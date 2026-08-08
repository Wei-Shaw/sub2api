//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// These tests pin the multi-instance contract for the three background jobs that
// previously relied on a per-process guard only: when a peer instance holds the
// leader lock the job must skip the cycle entirely, and once the peer releases it
// the job must run again on the next invocation.

// --- ScheduledTestRunnerService ---------------------------------------------

type leaderLockPlanRepo struct {
	listDueCalls atomic.Int64
}

func (r *leaderLockPlanRepo) Create(context.Context, *ScheduledTestPlan) (*ScheduledTestPlan, error) {
	return nil, nil
}
func (r *leaderLockPlanRepo) GetByID(context.Context, int64) (*ScheduledTestPlan, error) {
	return nil, nil
}
func (r *leaderLockPlanRepo) ListByAccountID(context.Context, int64) ([]*ScheduledTestPlan, error) {
	return nil, nil
}
func (r *leaderLockPlanRepo) ListDue(context.Context, time.Time) ([]*ScheduledTestPlan, error) {
	r.listDueCalls.Add(1)
	return nil, nil
}
func (r *leaderLockPlanRepo) Update(context.Context, *ScheduledTestPlan) (*ScheduledTestPlan, error) {
	return nil, nil
}
func (r *leaderLockPlanRepo) Delete(context.Context, int64) error { return nil }
func (r *leaderLockPlanRepo) UpdateAfterRun(context.Context, int64, time.Time, time.Time) error {
	return nil
}

func TestScheduledTestRunnerSkipsWhenPeerHoldsLeaderLock(t *testing.T) {
	repo := &leaderLockPlanRepo{}
	svc := NewScheduledTestRunnerService(repo, nil, nil, nil, nil)
	cache := &fakeLeaderLockCache{}
	svc.SetLeaderLock(cache, nil)

	_, acquired := tryAcquireSingletonLeaderLock(context.Background(), cache, nil, scheduledTestRunnerLeaderLockKey, "peer", time.Minute)
	require.True(t, acquired)

	svc.runDueOnce(context.Background())
	require.Zero(t, repo.listDueCalls.Load(), "peer holds the lock, due plans must not be scanned")

	require.NoError(t, cache.ReleaseLeaderLock(context.Background(), scheduledTestRunnerLeaderLockKey, "peer"))

	svc.runDueOnce(context.Background())
	require.Equal(t, int64(1), repo.listDueCalls.Load(), "lock is free, the scan must run")
}

// --- BackupService -----------------------------------------------------------

type countingDumper struct {
	calls atomic.Int64
}

func (d *countingDumper) Dump(context.Context) (io.ReadCloser, error) {
	d.calls.Add(1)
	return io.NopCloser(bytes.NewReader([]byte("dump"))), nil
}
func (d *countingDumper) Restore(context.Context, io.Reader) error { return nil }

func TestScheduledBackupSkipsWhenPeerHoldsLeaderLock(t *testing.T) {
	repo := newMockSettingRepo()
	seedS3Config(t, repo)
	dumper := &countingDumper{}
	svc := newTestBackupService(repo, dumper, newMockObjectStore())
	cache := &fakeLeaderLockCache{}
	svc.SetLeaderLock(cache, nil)

	_, acquired := tryAcquireSingletonLeaderLock(context.Background(), cache, nil, backupScheduledLeaderLockKey, "peer", time.Minute)
	require.True(t, acquired)

	svc.runScheduledBackup()
	require.Zero(t, dumper.calls.Load(), "peer holds the lock, no second pg_dump may start")

	require.NoError(t, cache.ReleaseLeaderLock(context.Background(), backupScheduledLeaderLockKey, "peer"))

	svc.runScheduledBackup()
	require.Equal(t, int64(1), dumper.calls.Load(), "lock is free, the scheduled backup must run")
}

// --- ChannelMonitorRunner ----------------------------------------------------

func TestChannelMonitorRunOneSkipsWhenPeerHoldsLeaderLock(t *testing.T) {
	const monitorID int64 = 7
	stub := &stubMonitorSvc{}
	runner := newRunnerForTest(stub)
	cache := &fakeLeaderLockCache{}
	runner.SetLeaderLock(cache, nil)

	key := channelMonitorLeaderLockKeyPrefix + "7:leader"
	_, acquired := tryAcquireSingletonLeaderLock(context.Background(), cache, nil, key, "peer", time.Minute)
	require.True(t, acquired)

	runner.runOne(monitorID, "m")
	require.Zero(t, stub.runCount.Load(), "peer holds the lock, the monitor must not be probed twice")

	require.NoError(t, cache.ReleaseLeaderLock(context.Background(), key, "peer"))

	runner.runOne(monitorID, "m")
	require.Equal(t, int64(1), stub.runCount.Load(), "lock is free, the check must run")
}

// Locking per monitor (rather than electing one runner for all monitors) is what
// keeps checks spread across instances; this pins that two different monitors can
// be claimed by two different instances at the same time.
func TestChannelMonitorLeaderLockIsPerMonitor(t *testing.T) {
	cache := &fakeLeaderLockCache{}
	ctx := context.Background()

	_, ok := tryAcquireSingletonLeaderLock(ctx, cache, nil, channelMonitorLeaderLockKeyPrefix+"1:leader", "instance-a", time.Minute)
	require.True(t, ok)

	_, ok = tryAcquireSingletonLeaderLock(ctx, cache, nil, channelMonitorLeaderLockKeyPrefix+"2:leader", "instance-b", time.Minute)
	require.True(t, ok, "a different monitor must be claimable by a different instance")

	_, ok = tryAcquireSingletonLeaderLock(ctx, cache, nil, channelMonitorLeaderLockKeyPrefix+"1:leader", "instance-b", time.Minute)
	require.False(t, ok, "the same monitor must not be claimed twice")
}
