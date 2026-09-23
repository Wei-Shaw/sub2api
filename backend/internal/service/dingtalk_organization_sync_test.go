package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestDingTalkSyncDetachesRequestAndRecordsFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery("INSERT INTO dingtalk_sync_jobs").WithArgs("a", sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"started_at"}).AddRow(time.Now()))
	mock.ExpectExec("UPDATE dingtalk_sync_jobs").WithArgs("a", sqlmock.AnyArg(), "failed", sqlmock.AnyArg(), 0, 0).WillReturnResult(sqlmock.NewResult(0, 1))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, release, observed := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	s := NewDingTalkOrganizationService(db, nil)
	job, err := s.StartSync(ctx, "a", func(background context.Context) ([]DingTalkDepartment, []DingTalkDirectoryMember, error) {
		close(started)
		<-release
		observed <- background.Err()
		return nil, nil, errors.New("upstream unavailable")
	})
	require.NoError(t, err)
	require.Equal(t, "running", job.Status)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	cancel()
	close(release)
	require.NoError(t, <-observed)
	require.Eventually(t, func() bool { return mock.ExpectationsWereMet() == nil }, time.Second, time.Millisecond)
}

func TestDingTalkSyncDuplicateReturnsExistingJob(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery("INSERT INTO dingtalk_sync_jobs").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT job_id").WithArgs("a").WillReturnRows(sqlmock.NewRows([]string{"job_id", "status", "started_at", "finished_at", "error", "departments", "members"}).AddRow("existing", "running", time.Now(), nil, "", 0, 0))
	s := NewDingTalkOrganizationService(db, nil)
	job, err := s.StartSync(context.Background(), "a", nil)
	require.NoError(t, err)
	require.Equal(t, "existing", job.JobID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDingTalkSyncPanicBecomesFailedJob(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectExec("UPDATE dingtalk_sync_jobs").WithArgs("a", "job", "failed", sqlmock.AnyArg(), 0, 0).WillReturnResult(sqlmock.NewResult(0, 1))
	NewDingTalkOrganizationService(db, nil).runSync("a", "job", func(context.Context) ([]DingTalkDepartment, []DingTalkDirectoryMember, error) { panic("failure") })
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDingTalkSyncSupersededJobCannotReplaceDirectory(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("a").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT job_id=").WithArgs("a", "old-job").WillReturnRows(sqlmock.NewRows([]string{"current"}).AddRow(false))
	mock.ExpectRollback()
	err = NewDingTalkOrganizationService(db, nil).replaceDirectory(context.Background(), "a", []DingTalkDepartment{{ID: 1, Name: "Old snapshot"}}, nil, "old-job")
	require.ErrorContains(t, err, "superseded")
	require.NoError(t, mock.ExpectationsWereMet())
}
