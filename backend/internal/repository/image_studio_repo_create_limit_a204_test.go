//go:build unit

package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func a204NewQueuedJob() *service.ImageStudioJob {
	return &service.ImageStudioJob{UserID: 42, APIKeyID: 1, Status: service.ImageStudioStatusQueued, Kind: service.ImageStudioKindGenerate, Model: "gpt-image-2", Prompt: "pelican", Params: []byte(`{"n":1}`)}
}

func TestImageStudioCreateJobWithinLimitCountsAndInsertsUnderUserLockA204(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	createdAt := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock($1)")).
		WithArgs(advisoryLockHash("image_studio_submit:42")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM image_studio_jobs WHERE user_id = $1 AND status IN ('queued', 'running')")).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO image_studio_jobs")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(9), createdAt))
	mock.ExpectCommit()

	job := a204NewQueuedJob()
	require.NoError(t, NewImageStudioRepository(db).CreateJobWithinLimit(context.Background(), job, 2))
	require.Equal(t, int64(9), job.ID)
	require.Equal(t, createdAt, job.CreatedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestImageStudioCreateJobWithinLimitRejectsAtCapA204(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock($1)")).
		WithArgs(advisoryLockHash("image_studio_submit:42")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM image_studio_jobs")).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectRollback()

	job := a204NewQueuedJob()
	err = NewImageStudioRepository(db).CreateJobWithinLimit(context.Background(), job, 2)
	require.ErrorIs(t, err, service.ErrImageStudioBusy)
	require.Zero(t, job.ID)
	require.NoError(t, mock.ExpectationsWereMet(), "no INSERT may run once the user is at the cap")
}
