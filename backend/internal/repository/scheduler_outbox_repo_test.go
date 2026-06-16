package repository

import (
	"context"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestSchedulerOutboxRepositoryDeleteConsumedUpToUsesBoundedCTE(t *testing.T) {
	db, mock, err := sqlmock.New()
REDACTED
	defer func() { _ = db.Close() REDACTED()

	repo := &schedulerOutboxRepository{db: dbREDACTED
	const expectedSQL = `
		WITH doomed AS (
			SELECT id
			FROM scheduler_outbox
			WHERE id <= $1
				AND created_at < NOW() - INTERVAL '10 seconds'
			ORDER BY id ASC
			LIMIT $2
		)
		DELETE FROM scheduler_outbox o
		USING doomed d
		WHERE o.id = d.id
	`
	mock.ExpectExec(regexp.QuoteMeta(expectedSQL)).
		WithArgs(int64(42), 5000).
		WillReturnResult(sqlmock.NewResult(0, 17))

	deleted, err := repo.DeleteConsumedUpTo(context.Background(), 42, 5000)

REDACTED
	require.EqualValues(t, 17, deleted)
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestSchedulerOutboxRepositoryDeleteConsumedUpToSkipsNonPositiveWatermark(t *testing.T) {
	db, mock, err := sqlmock.New()
REDACTED
	defer func() { _ = db.Close() REDACTED()

	repo := &schedulerOutboxRepository{db: dbREDACTED

	deleted, err := repo.DeleteConsumedUpTo(context.Background(), 0, 5000)

REDACTED
	require.EqualValues(t, 0, deleted)
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestSchedulerOutboxRepositoryTryAcquireCleanupLock(t *testing.T) {
	db, mock, err := sqlmock.New()
REDACTED
	defer func() { _ = db.Close() REDACTED()

	repo := &schedulerOutboxRepository{db: dbREDACTED
	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_lock(hashtext('scheduler_outbox_cleanup'))")).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"REDACTED).AddRow(true))
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_unlock(hashtext('scheduler_outbox_cleanup'))")).
		WillReturnResult(sqlmock.NewResult(0, 1))

	lease, acquired, err := repo.TryAcquireCleanupLock(context.Background())
REDACTED
	require.True(t, acquired)
	require.NotNil(t, lease)

	lease.Release()

	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestSchedulerOutboxRepositoryTryAcquireCleanupLockUnavailable(t *testing.T) {
	db, mock, err := sqlmock.New()
REDACTED
	defer func() { _ = db.Close() REDACTED()

	repo := &schedulerOutboxRepository{db: dbREDACTED
	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_lock(hashtext('scheduler_outbox_cleanup'))")).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"REDACTED).AddRow(false))

	lease, acquired, err := repo.TryAcquireCleanupLock(context.Background())
REDACTED
	require.False(t, acquired)
	require.Nil(t, lease)

	require.NoError(t, mock.ExpectationsWereMet())
REDACTED
