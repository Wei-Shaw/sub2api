//go:build integration

package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestSchedulerOutboxPollingDoesNotSkipAnEarlierIDThatCommitsLate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := integrationDB.ExecContext(ctx, "TRUNCATE scheduler_outbox RESTART IDENTITY")
	require.NoError(t, err)

	earlierTx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = earlierTx.Rollback() })

	earlierID := insertSchedulerOutboxEventInTx(t, ctx, earlierTx, 101)

	laterTx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = laterTx.Rollback() })
	laterID := insertSchedulerOutboxEventInTx(t, ctx, laterTx, 202)
	require.Less(t, earlierID, laterID)
	require.NoError(t, laterTx.Commit(), "the larger sequence ID commits first")

	repo := NewSchedulerOutboxRepository(integrationDB)
	first, err := repo.ListAfterAndReleaseDedup(ctx, 0, 100)
	require.Nil(t, first, "a failed poll must not expose events or advance the caller watermark")
	var pgErr *pq.Error
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, pq.ErrorCode("55P03"), pgErr.Code, "the poll must fail immediately when an insert is still in flight")

	require.NoError(t, earlierTx.Commit(), "the smaller sequence ID commits second")

	second, err := repo.ListAfterAndReleaseDedup(ctx, 0, 100)
	require.NoError(t, err)
	require.Equal(t, []int64{earlierID, laterID}, schedulerOutboxEventIDs(second),
		"advancing the watermark must not make a late-committing lower ID permanently invisible")
}

func insertSchedulerOutboxEventInTx(t *testing.T, ctx context.Context, tx *sql.Tx, accountID int64) int64 {
	t.Helper()

	var id int64
	err := tx.QueryRowContext(ctx, `
		INSERT INTO scheduler_outbox (event_type, account_id, payload)
		VALUES ($1, $2, $3)
		RETURNING id
	`, service.SchedulerOutboxEventAccountLastUsed, accountID, []byte(`{"last_used": {}}`)).Scan(&id)
	require.NoError(t, err)
	return id
}

func schedulerOutboxEventIDs(events []service.SchedulerOutboxEvent) []int64 {
	ids := make([]int64, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.ID)
	}
	return ids
}
