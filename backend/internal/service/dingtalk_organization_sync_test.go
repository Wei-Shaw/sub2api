package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

func TestDingTalkSyncReconcilesOnlyAfterDirectoryCommit(t *testing.T) {
	for _, failReconciliation := range []bool{false, true} {
		t.Run(fmt.Sprint(failReconciliation), func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			mock.ExpectBegin()
			mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("a").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("SELECT job_id=").WithArgs("a", "job").WillReturnRows(sqlmock.NewRows([]string{"current"}).AddRow(true))
			mock.ExpectExec("DELETE FROM dingtalk_departments").WithArgs("a").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("INSERT INTO dingtalk_departments").WithArgs("a", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("INSERT INTO dingtalk_directory_snapshots").WithArgs("a").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			s := NewDingTalkOrganizationService(db, nil)
			called := false
			s.SetAfterSync(func(ctx context.Context, app string, members []DingTalkDirectoryMember) error {
				// All expectations up to commit must have completed before revocation.
				require.NoError(t, mock.ExpectationsWereMet())
				called = true
				require.Equal(t, "a", app)
				require.Empty(t, members)
				status, message, departments := "succeeded", "", 1
				var result error
				if failReconciliation {
					status, message, departments = "failed", "Sync failed; check application permissions and retry", 0
					result = errors.New("upstream throttled")
				}
				mock.ExpectExec("UPDATE dingtalk_sync_jobs").WithArgs("a", "job", status, message, departments, 0).WillReturnResult(sqlmock.NewResult(0, 1))
				return result
			})
			s.runSync("a", "job", func(context.Context) ([]DingTalkDepartment, []DingTalkDirectoryMember, error) {
				return []DingTalkDepartment{{ID: 1, Name: "Root"}}, nil, nil
			})
			require.True(t, called)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDingTalkFailedDirectorySkipsReconciliation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectExec("UPDATE dingtalk_sync_jobs").WithArgs("a", "job", "failed", sqlmock.AnyArg(), 0, 0).WillReturnResult(sqlmock.NewResult(0, 1))
	s := NewDingTalkOrganizationService(db, nil)
	s.SetAfterSync(func(context.Context, string, []DingTalkDirectoryMember) error {
		t.Fatal("partial directory must not revoke API keys")
		return nil
	})
	s.runSync("a", "job", func(context.Context) ([]DingTalkDepartment, []DingTalkDirectoryMember, error) {
		return []DingTalkDepartment{{ID: 1, Name: "Root"}}, nil, errors.New("member page unavailable")
	})
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDingTalkSyncStopCancelsWorker(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery("INSERT INTO dingtalk_sync_jobs").WithArgs("a", sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"started_at"}).AddRow(time.Now()))
	mock.ExpectExec("UPDATE dingtalk_sync_jobs").WithArgs("a", sqlmock.AnyArg(), "failed", sqlmock.AnyArg(), 0, 0).WillReturnResult(sqlmock.NewResult(0, 1))
	s := NewDingTalkOrganizationService(db, nil)
	started := make(chan struct{})
	_, err = s.StartSync(context.Background(), "a", func(ctx context.Context) ([]DingTalkDepartment, []DingTalkDirectoryMember, error) {
		close(started)
		<-ctx.Done()
		return nil, nil, ctx.Err()
	})
	require.NoError(t, err)
	<-started
	s.Stop()
	_, err = s.StartSync(context.Background(), "a", nil)
	require.ErrorIs(t, err, ErrServiceUnavailable)
	require.NoError(t, mock.ExpectationsWereMet())
}
