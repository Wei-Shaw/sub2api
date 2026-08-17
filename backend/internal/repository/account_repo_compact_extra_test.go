package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
)

func TestShouldEnqueueSchedulerOutboxForExtraUpdates_CompactCapabilityKeysAreRelevant(t *testing.T) {
	updates := map[string]any{
		"openai_compact_supported":  true,
		"openai_compact_checked_at": "2026-04-10T10:00:00Z",
	}

	if !shouldEnqueueSchedulerOutboxForExtraUpdates(updates) {
		t.Fatalf("expected compact capability updates to enqueue scheduler outbox")
	}
}

func TestUpdateExtraCompactProbeUnknownDeletesVerdictWithMonotonicGuard(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })

	const (
		accountID  = int64(91)
		observedAt = int64(1771236000123456789)
	)
	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE accounts SET extra = CASE WHEN .*openai_compact_probe_observed_at_unix_nano.* <= \$3::numeric THEN .*\|\| \$4::jsonb.* - \$5::text\[\].* ELSE .* END, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(`{}`, accountID, observedAt, sqlmock.AnyArg(), pq.Array([]string{"openai_compact_supported"})).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO scheduler_outbox`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := newAccountRepositoryWithSQL(client, db, nil)
	err = repo.UpdateExtra(context.Background(), accountID, map[string]any{
		"openai_compact_supported":                           nil,
		service.OpenAICompactProbeObservedAtUnixNanoExtraKey: observedAt,
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPartitionCompactProbeExtraUpdates(t *testing.T) {
	updates := map[string]any{
		"privacy_mode":                                       "strict",
		"openai_compact_supported":                           true,
		"openai_compact_last_error":                          "",
		service.OpenAICompactProbeObservedAtUnixNanoExtraKey: int64(200),
	}

	common, group := partitionCompactProbeExtraUpdates(updates)
	require.Equal(t, map[string]any{"privacy_mode": "strict"}, common)
	require.NotNil(t, group)
	require.Equal(t, int64(200), group.observedAt)
	require.Contains(t, group.updates, "openai_compact_supported")
	require.Contains(t, group.updates, "openai_compact_last_error")
	require.NotContains(t, group.updates, "privacy_mode")
}

func TestShouldEnqueueSchedulerOutboxForExtraUpdates_OpenAIResponsesCapabilityKeysAreRelevant(t *testing.T) {
	updates := map[string]any{
		"openai_responses_mode":      "force_chat_completions",
		"openai_responses_supported": false,
	}

	if !shouldEnqueueSchedulerOutboxForExtraUpdates(updates) {
		t.Fatalf("expected responses capability updates to enqueue scheduler outbox")
	}
}
