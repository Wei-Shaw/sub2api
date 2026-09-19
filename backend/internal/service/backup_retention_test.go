//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func archiveSchedule(days ...int) *BackupScheduleConfig {
	return &BackupScheduleConfig{CronExpr: "CRON_TZ=UTC 0 4 * * *", RetainDays: 14, RetainCount: 10,
		MonthlyArchive: &BackupMonthlyArchiveConfig{Enabled: true, Days: days}}
}

func archiveRecord(id, started string) *BackupRecord {
	return &BackupRecord{ID: id, Status: "completed", TriggeredBy: "scheduled", StartedAt: started, ExpiresAt: "2099-01-01T00:00:00Z"}
}

func TestBackupRetention_ValidationAndRoundTrip(t *testing.T) {
	for _, cfg := range []BackupScheduleConfig{
		{RetainDays: -1}, {RetainCount: -1},
		{MonthlyArchive: &BackupMonthlyArchiveConfig{Enabled: true}},
		{MonthlyArchive: &BackupMonthlyArchiveConfig{Days: []int{0}}},
		{MonthlyArchive: &BackupMonthlyArchiveConfig{Days: []int{32}}},
		{MonthlyArchive: &BackupMonthlyArchiveConfig{RetainCount: -1}},
	} {
		require.Error(t, validateBackupRetention(&cfg))
	}
	repo := newMockSettingRepo()
	svc := newTestBackupService(repo, &mockDumper{}, newMockObjectStore())
	cfg := archiveSchedule(15, 1, 15)
	cfg.RetainDays, cfg.RetainCount = 0, 0
	cfg.MonthlyArchive.IncludeMonthEnd = true
	cfg.MonthlyArchive.RetainCount = 17
	saved, err := svc.UpdateSchedule(context.Background(), *cfg)
	require.NoError(t, err)
	require.Equal(t, []int{1, 15}, saved.MonthlyArchive.Days)
	loaded, err := svc.GetSchedule(context.Background())
	require.NoError(t, err)
	require.Equal(t, saved, loaded)
	require.Zero(t, loaded.RetainDays)
	require.Zero(t, loaded.RetainCount)
}

func TestBackupRetention_MonthEndAndLeapYears(t *testing.T) {
	for _, tc := range []struct{ date, end string }{
		{"2026-02-01", "2026-02-28"}, {"2028-02-01", "2028-02-29"}, {"2026-04-01", "2026-04-30"},
	} {
		t.Run(tc.date, func(t *testing.T) {
			at, err := time.Parse(time.DateOnly, tc.date)
			require.NoError(t, err)
			cfg := &BackupMonthlyArchiveConfig{Days: []int{30, 31}, IncludeMonthEnd: true}
			require.Equal(t, []string{tc.end}, monthlyArchiveDates(at, cfg))
		})
	}
}

func TestBackupRetention_SuccessOnlyFallbackAndMultipleDates(t *testing.T) {
	repo := newMockSettingRepo()
	svc := newTestBackupService(repo, &mockDumper{}, newMockObjectStore())
	cfg := archiveSchedule(1, 15)
	ctx := context.Background()
	failed := archiveRecord("failed", "2026-09-01T04:00:00Z")
	failed.Status = "failed"
	require.NoError(t, svc.saveRecordWithArchive(ctx, failed, cfg))
	manual := archiveRecord("manual", "2026-09-01T05:00:00Z")
	manual.TriggeredBy = "manual"
	require.NoError(t, svc.saveRecordWithArchive(ctx, manual, cfg))
	require.Nil(t, failed.MonthlyArchive)
	require.Nil(t, manual.MonthlyArchive)
	first := archiveRecord("first", "2026-09-02T04:00:00Z")
	require.NoError(t, svc.saveRecordWithArchive(ctx, first, cfg))
	require.Equal(t, []string{"2026-09-01"}, first.MonthlyArchive.Dates)
	require.Empty(t, first.ExpiresAt)
	again := archiveRecord("again", "2026-09-02T06:00:00Z")
	require.NoError(t, svc.saveRecordWithArchive(ctx, again, cfg))
	require.Nil(t, again.MonthlyArchive)
	next := archiveRecord("next", "2026-09-16T04:00:00Z")
	require.NoError(t, svc.saveRecordWithArchive(ctx, next, cfg))
	require.Equal(t, []string{"2026-09-15"}, next.MonthlyArchive.Dates)
	// A later month does not manufacture archives for the missed previous month.
	newMonth := archiveRecord("oct", "2026-10-20T04:00:00Z")
	require.NoError(t, svc.saveRecordWithArchive(ctx, newMonth, cfg))
	require.Equal(t, []string{"2026-10-01", "2026-10-15"}, newMonth.MonthlyArchive.Dates)
}

func TestBackupRetention_ScheduleTimezoneAndStartDate(t *testing.T) {
	repo := newMockSettingRepo()
	svc := newTestBackupService(repo, &mockDumper{}, newMockObjectStore())
	cfg := archiveSchedule(1, 31)
	cfg.CronExpr = "CRON_TZ=Asia/Tokyo 0 4 * * *"
	first := archiveRecord("tokyo", "2026-08-31T16:00:00Z")
	require.NoError(t, svc.saveRecordWithArchive(context.Background(), first, cfg))
	require.Equal(t, []string{"2026-09-01"}, first.MonthlyArchive.Dates)
	last := archiveRecord("cross-midnight", "2026-09-30T14:59:00Z")
	last.FinishedAt = "2026-09-30T15:10:00Z"
	require.NoError(t, svc.saveRecordWithArchive(context.Background(), last, cfg))
	require.Equal(t, []string{"2026-09-30"}, last.MonthlyArchive.Dates)
}

func TestBackupRetention_CheckpointSurvivesRecordRemovalAndRestart(t *testing.T) {
	repo := newMockSettingRepo()
	seedS3Config(t, repo)
	store := newMockObjectStore()
	svc := newTestBackupService(repo, &mockDumper{}, store)
	cfg := archiveSchedule(1, 15)
	ctx := context.Background()
	record := archiveRecord("archive", "2026-09-16T04:00:00Z")
	require.NoError(t, svc.saveRecordWithArchive(ctx, record, cfg))
	require.ErrorIs(t, svc.DeleteBackup(ctx, record.ID), ErrBackupArchiveProtected)
	require.NoError(t, svc.DeleteArchivedBackup(ctx, record.ID))
	svc = newTestBackupService(repo, &mockDumper{}, store)
	retry := archiveRecord("retry", "2026-09-17T04:00:00Z")
	require.NoError(t, svc.saveRecordWithArchive(ctx, retry, cfg))
	require.Nil(t, retry.MonthlyArchive)
}

func TestBackupRetention_CleanupSeparatePoolsAndPermanentProtection(t *testing.T) {
	repo := newMockSettingRepo()
	seedS3Config(t, repo)
	store := newMockObjectStore()
	svc := newTestBackupService(repo, &mockDumper{}, store)
	ctx := context.Background()
	now := time.Now().UTC()
	for i, id := range []string{"running", "failed", "ordinary-new", "archive-new", "permanent", "ordinary-old", "archive-old", "restoring"} {
		r := &BackupRecord{ID: id, Status: "completed", StartedAt: now.Add(-time.Duration(i) * time.Hour).Format(time.RFC3339), S3Key: id}
		switch id {
		case "running", "failed":
			r.Status = id
		case "archive-new", "archive-old":
			r.MonthlyArchive = &BackupMonthlyArchive{RetainCount: 12}
		case "permanent":
			r.MonthlyArchive = &BackupMonthlyArchive{}
		case "restoring":
			r.RestoreStatus = "running"
		}
		store.objects[id] = []byte(id)
		require.NoError(t, svc.saveRecord(ctx, r))
	}
	cfg := &BackupScheduleConfig{RetainDays: 1, RetainCount: 1, MonthlyArchive: &BackupMonthlyArchiveConfig{RetainCount: 1}}
	require.NoError(t, svc.cleanupOldBackups(ctx, cfg))
	for _, id := range []string{"running", "failed", "ordinary-new", "archive-new", "permanent", "restoring"} {
		_, err := svc.GetBackupRecord(ctx, id)
		require.NoError(t, err, id)
	}
	for _, id := range []string{"ordinary-old", "archive-old"} {
		_, err := svc.GetBackupRecord(ctx, id)
		require.ErrorIs(t, err, ErrBackupNotFound)
		require.NotContains(t, store.objects, id)
	}
	finite, err := svc.GetBackupRecord(ctx, "archive-new")
	require.NoError(t, err)
	require.Equal(t, 1, finite.MonthlyArchive.RetainCount)
	// Switching the rule off or to permanent does not downgrade prior permanent archives.
	cfg.MonthlyArchive = &BackupMonthlyArchiveConfig{}
	cfg.RetainDays = 0
	require.NoError(t, svc.cleanupOldBackups(ctx, cfg))
	_, err = svc.GetBackupRecord(ctx, "permanent")
	require.NoError(t, err)
}

func TestBackupRetention_Over100ArchivesRemainDiscoverable(t *testing.T) {
	repo := newMockSettingRepo()
	svc := newTestBackupService(repo, &mockDumper{}, newMockObjectStore())
	ctx := context.Background()
	for i := range 105 {
		record := archiveRecord(fmt.Sprint(i), "2020-01-01T00:00:00Z")
		record.MonthlyArchive = &BackupMonthlyArchive{}
		require.NoError(t, svc.saveRecord(ctx, record))
	}
	require.NoError(t, svc.cleanupOldBackups(ctx, &BackupScheduleConfig{RetainDays: 1, RetainCount: 1}))
	records, err := svc.ListBackups(ctx)
	require.NoError(t, err)
	require.Len(t, records, 105)
}

type failingArchiveRepo struct {
	*mockSettingRepo
	fail bool
}

func (r *failingArchiveRepo) SetMultiple(ctx context.Context, values map[string]string) error {
	if r.fail {
		return errors.New("injected atomic save failure")
	}
	return r.mockSettingRepo.SetMultiple(ctx, values)
}

func TestBackupRetention_PersistenceFailuresFailClosed(t *testing.T) {
	repo := newMockSettingRepo()
	svc := newTestBackupService(repo, &mockDumper{}, newMockObjectStore())
	ctx := context.Background()
	failing := &failingArchiveRepo{mockSettingRepo: repo, fail: true}
	svc.settingRepo = failing
	record := archiveRecord("new", "2026-09-01T04:00:00Z")
	cfg := archiveSchedule(1)
	require.Error(t, svc.saveRecordWithArchive(ctx, record, cfg))
	require.Nil(t, record.MonthlyArchive, "failed commit must not mutate the caller's retry state")
	require.Empty(t, repo.data[settingKeyBackupArchiveCheckpoint])
	failing.fail = false
	require.NoError(t, svc.saveRecordWithArchive(ctx, record, cfg))
	require.NotNil(t, record.MonthlyArchive)
	oldRecords := repo.data[settingKeyBackupRecords]
	repo.getValueErr = errors.New("read failed")
	require.Error(t, svc.saveRecord(ctx, archiveRecord("other", "2026-09-02T04:00:00Z")))
	require.Equal(t, oldRecords, repo.data[settingKeyBackupRecords])
	require.Error(t, svc.cleanupOldBackups(ctx, cfg))
	require.Error(t, func() error { _, err := svc.GetSchedule(ctx); return err }())
	repo.getValueErr = nil
	repo.data[settingKeyBackupRecords] = "corrupted"
	require.ErrorIs(t, svc.saveRecord(ctx, record), ErrBackupRecordsCorrupt)
	require.Equal(t, "corrupted", repo.data[settingKeyBackupRecords])
	repo.data[settingKeyBackupRecords] = oldRecords
	repo.data[settingKeyBackupArchiveCheckpoint] = "bad date"
	require.Error(t, svc.saveRecordWithArchive(ctx, archiveRecord("later", "2026-09-02T04:00:00Z"), cfg))
	require.Equal(t, oldRecords, repo.data[settingKeyBackupRecords])
}

func TestBackupRetention_ScheduledExecutionAndZeroDays(t *testing.T) {
	repo := newMockSettingRepo()
	seedS3Config(t, repo)
	svc := newTestBackupService(repo, &mockDumper{dumpData: []byte("backup")}, newMockObjectStore())
	cfg := archiveSchedule(time.Now().UTC().Day())
	cfg.RetainDays = 0
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, repo.Set(context.Background(), settingKeyBackupSchedule, string(raw)))
	svc.runScheduledBackup()
	records, err := svc.ListBackups(context.Background())
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.NotNil(t, records[0].MonthlyArchive)
	require.Empty(t, records[0].ExpiresAt)
	// The following successful backup is ordinary and must still honor zero days.
	svc.runScheduledBackup()
	records, err = svc.ListBackups(context.Background())
	require.NoError(t, err)
	require.Len(t, records, 2)
	for _, record := range records {
		require.Empty(t, record.ExpiresAt)
	}
}

func TestBackupRetention_CrossInstanceRecordUpdates(t *testing.T) {
	repo := newMockSettingRepo()
	cache := &fakeLeaderLockCache{}
	services := []*BackupService{
		newTestBackupService(repo, &mockDumper{}, newMockObjectStore()),
		newTestBackupService(repo, &mockDumper{}, newMockObjectStore()),
	}
	for _, svc := range services {
		svc.SetLeaderLock(cache, nil)
	}
	var wg sync.WaitGroup
	errors := make(chan error, 24)
	for i := range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errors <- services[i%2].saveRecord(context.Background(), archiveRecord(fmt.Sprint(i), "2026-09-01T00:00:00Z"))
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	records, err := services[0].ListBackups(context.Background())
	require.NoError(t, err)
	require.Len(t, records, 24)
}

func TestBackupRetention_RecordLockWithSingleConnectionPool(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	db.SetMaxOpenConns(1)
	repo := newMockSettingRepo()
	svc := newTestBackupService(repo, &mockDumper{}, newMockObjectStore())
	svc.SetLeaderLock(&fakeLeaderLockCache{}, db)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	record := archiveRecord("single-connection", "2026-09-01T00:00:00Z")
	require.NoError(t, svc.saveRecord(ctx, record), "Redis locking must not occupy a database connection")
	require.NoError(t, mock.ExpectationsWereMet())
	svc.SetLeaderLock(nil, db)
	require.ErrorContains(t, svc.saveRecord(ctx, record), "at least two database connections")
}
