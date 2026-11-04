//go:build unit

package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type backupDelayedReadRepo struct {
	*mockSettingRepo
	fired  atomic.Bool
	loaded chan struct{}
	resume chan struct{}
}

func (r *backupDelayedReadRepo) GetValue(ctx context.Context, key string) (string, error) {
	value, err := r.mockSettingRepo.GetValue(ctx, key)
	if key == settingKeyBackupRecords && r.fired.CompareAndSwap(false, true) {
		close(r.loaded)
		<-r.resume
	}
	return value, err
}
func TestBackupRecovery_RestartPreservesPeerArchive(t *testing.T) {
	ctx := context.Background()
	repo := newMockSettingRepo()
	seedS3Config(t, repo)
	store := newMockObjectStore()
	writer := newTestBackupService(repo, &mockDumper{}, store)
	started := time.Now().UTC()
	record := archiveRecord("now-permanent", started.Format(time.RFC3339))
	record.Status = "running"
	record.S3Key = "now-permanent.sql.gz"
	store.objects[record.S3Key] = []byte("important archive")
	require.NoError(t, writer.saveRecord(ctx, record))

	restarting := newTestBackupService(repo, &mockDumper{}, store)
	delayedRepo := &backupDelayedReadRepo{mockSettingRepo: repo, loaded: make(chan struct{}), resume: make(chan struct{})}
	restarting.settingRepo = delayedRepo
	cache := &fakeLeaderLockCache{}
	writer.SetLeaderLock(cache, nil)
	restarting.SetLeaderLock(cache, nil)
	done := make(chan struct{})
	go func() { restarting.recoverStaleRecords(); close(done) }()
	<-delayedRepo.loaded
	record.Status = "completed"
	// Recovery must own the writer lock across its read and decision. The peer
	// completes only after recovery releases it; the young operation is active.
	require.Equal(t, restarting.instanceID, cache.heldBy("backup:records:writer"))
	written := make(chan error, 1)
	go func() { written <- writer.saveRecordWithArchive(ctx, record, archiveSchedule(started.Day())) }()
	close(delayedRepo.resume)
	<-done
	require.NoError(t, <-written)
	require.NotNil(t, record.MonthlyArchive)
	restarting.recoverStaleRecords()
	saved, err := writer.GetBackupRecord(ctx, record.ID)
	require.NoError(t, err)
	require.NotNil(t, saved.MonthlyArchive)
	require.Equal(t, "completed", saved.Status, "completed permanent archive was overwritten by stale restart snapshot")
	require.Contains(t, store.objects, record.S3Key, "restart must not delete completed permanent archive")
}

func TestBackupRecovery_RestoreRejectsRemovedArchive(t *testing.T) {
	ctx := context.Background()
	repo := newMockSettingRepo()
	seedS3Config(t, repo)
	store := newMockObjectStore()
	cleaner := newTestBackupService(repo, &mockDumper{}, store)
	for i, id := range []string{"old", "new"} {
		record := archiveRecord(id, []string{"2026-09-01T04:00:00Z", "2026-09-15T04:00:00Z"}[i])
		record.MonthlyArchive = &BackupMonthlyArchive{RetainCount: 1}
		record.S3Key = id
		store.objects[id] = []byte("payload")
		require.NoError(t, cleaner.saveRecord(ctx, record))
	}
	restoring := newTestBackupService(repo, &mockDumper{}, store)
	delayedRepo := &backupDelayedReadRepo{mockSettingRepo: repo, loaded: make(chan struct{}), resume: make(chan struct{})}
	restoring.settingRepo = delayedRepo
	cache := &fakeLeaderLockCache{}
	cleaner.SetLeaderLock(cache, nil)
	restoring.SetLeaderLock(cache, nil)
	result := make(chan error)
	go func() { _, err := restoring.StartRestore(ctx, "old"); result <- err }()
	<-delayedRepo.loaded
	cfg := archiveSchedule(1, 15)
	cfg.MonthlyArchive.RetainCount = 1
	require.NoError(t, cleaner.cleanupOldBackups(ctx, cfg))
	require.NotContains(t, store.objects, "old")
	close(delayedRepo.resume)
	err := <-result
	restoring.wg.Wait()
	saved, getErr := cleaner.GetBackupRecord(ctx, "old")
	require.Nil(t, saved)
	require.ErrorIs(t, err, ErrBackupNotFound, "restore must not start against a deleted archive")
	require.ErrorIs(t, getErr, ErrBackupNotFound, "cleanup-deleted archive must not be resurrected")
}

type backupFailRecordWriteRepo struct{ *mockSettingRepo }

func (r *backupFailRecordWriteRepo) Set(ctx context.Context, key, value string) error {
	if key == settingKeyBackupRecords {
		return context.DeadlineExceeded
	}
	return r.mockSettingRepo.Set(ctx, key, value)
}
func TestBackupRecovery_RestoreRequiresDurableProtection(t *testing.T) {
	ctx := context.Background()
	repo := newMockSettingRepo()
	seedS3Config(t, repo)
	store := newMockObjectStore()
	svc := newTestBackupService(repo, &mockDumper{}, store)
	record := archiveRecord("finite-archive", "2026-09-01T04:00:00Z")
	record.MonthlyArchive = &BackupMonthlyArchive{RetainCount: 1}
	record.S3Key = record.ID
	require.NoError(t, svc.saveRecord(ctx, record))
	svc.settingRepo = &backupFailRecordWriteRepo{repo}
	_, err := svc.StartRestore(ctx, record.ID)
	svc.wg.Wait()
	require.Error(t, err, "failed restore-running marker must prevent restore from starting")
}

func TestBackupRecovery_ActiveAndLegacyRestoresKeepProtection(t *testing.T) {
	ctx := context.Background()
	svc := newTestBackupService(newMockSettingRepo(), &mockDumper{}, newMockObjectStore())
	for _, id := range []string{"active", "legacy"} {
		record := archiveRecord(id, time.Now().AddDate(-1, 0, 0).Format(time.RFC3339))
		record.MonthlyArchive = &BackupMonthlyArchive{RetainCount: 0}
		record.RestoreStatus = "running"
		if id == "active" {
			record.RestoreStartedAt = time.Now().Format(time.RFC3339)
		}
		require.NoError(t, svc.saveRecord(ctx, record))
	}
	svc.recoverStaleRecords()
	for _, id := range []string{"active", "legacy"} {
		record, err := svc.GetBackupRecord(ctx, id)
		require.NoError(t, err)
		require.Equal(t, "running", record.RestoreStatus)
		require.NotEmpty(t, record.RestoreStartedAt)
		require.ErrorIs(t, svc.DeleteArchivedBackup(ctx, id), ErrRestoreInProgress)
	}
}

func TestBackupRecovery_FailedStateWriteNeverDeletesObjects(t *testing.T) {
	ctx := context.Background()
	repo := newMockSettingRepo()
	seedS3Config(t, repo)
	store := newMockObjectStore()
	svc := newTestBackupService(repo, &mockDumper{}, store)
	record := archiveRecord("interrupted", time.Now().Add(-time.Hour).Format(time.RFC3339))
	record.Status, record.S3Key = "running", "interrupted.sql.gz"
	store.objects[record.S3Key] = []byte("payload")
	require.NoError(t, svc.saveRecord(ctx, record))
	svc.settingRepo = &backupFailRecordWriteRepo{repo}
	svc.recoverStaleRecords()
	saved, err := svc.GetBackupRecord(ctx, record.ID)
	require.NoError(t, err)
	require.Equal(t, "running", saved.Status)
	require.Contains(t, store.objects, record.S3Key)
	require.Empty(t, store.deletedKeys)
}

func TestBackupRecovery_RestoreReservationProtectsFromPeerCleanup(t *testing.T) {
	ctx := context.Background()
	repo := newMockSettingRepo()
	seedS3Config(t, repo)
	store := newMockObjectStore()
	svc := newTestBackupService(repo, &mockDumper{}, store)
	peer := newTestBackupService(repo, &mockDumper{}, store)
	cache := &fakeLeaderLockCache{}
	svc.SetLeaderLock(cache, nil)
	peer.SetLeaderLock(cache, nil)
	record := archiveRecord("old", time.Now().AddDate(-1, 0, 0).Format(time.RFC3339))
	record.S3Key = record.ID
	store.objects[record.ID] = []byte("payload")
	require.NoError(t, svc.saveRecord(ctx, record))
	restoring, err := svc.beginBackupRestore(ctx, record.ID)
	require.NoError(t, err)
	require.Equal(t, "running", restoring.RestoreStatus)
	_, err = peer.beginBackupRestore(ctx, record.ID)
	require.ErrorIs(t, err, ErrRestoreInProgress)
	require.NoError(t, peer.cleanupOldBackups(ctx, archiveSchedule(1)))
	require.Contains(t, store.objects, record.ID)
	require.ErrorIs(t, peer.DeleteBackup(ctx, record.ID), ErrRestoreInProgress)
}

func TestBackupRecovery_FailedRestoreCannotRecreateMissingRecord(t *testing.T) {
	ctx := context.Background()
	svc := newTestBackupService(newMockSettingRepo(), &mockDumper{}, newMockObjectStore())
	record := archiveRecord("removed", time.Now().Format(time.RFC3339))
	record.RestoreStatus = "failed"
	require.ErrorIs(t, svc.saveRestoreRecord(ctx, record), ErrBackupNotFound)
	_, err := svc.GetBackupRecord(ctx, record.ID)
	require.ErrorIs(t, err, ErrBackupNotFound)
}

func TestBackupRecovery_LateRestoreResultCannotReleaseNewProtection(t *testing.T) {
	ctx := context.Background()
	svc := newTestBackupService(newMockSettingRepo(), &mockDumper{}, newMockObjectStore())
	record := archiveRecord("restoring", time.Now().AddDate(-1, 0, 0).Format(time.RFC3339))
	record.RestoreStatus = "running"
	record.RestoreStartedAt = time.Now().Add(-time.Hour).Format(time.RFC3339)
	require.NoError(t, svc.saveRecord(ctx, record))
	svc.recoverStaleRecords()
	current, err := svc.beginBackupRestore(ctx, record.ID)
	require.NoError(t, err)
	for _, status := range []string{"failed", "completed"} {
		record.RestoreStatus = status
		require.Error(t, svc.saveRestoreRecord(ctx, record))
		saved, err := svc.GetBackupRecord(ctx, record.ID)
		require.NoError(t, err)
		require.Equal(t, "running", saved.RestoreStatus)
		require.Equal(t, current.RestoreStartedAt, saved.RestoreStartedAt)
		require.ErrorIs(t, svc.DeleteBackup(ctx, record.ID), ErrRestoreInProgress)
	}
}
