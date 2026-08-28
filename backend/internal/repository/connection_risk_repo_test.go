//go:build unit

package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestConnectionRiskRepository_UpsertOpenUsesPartialIndexConflictTarget(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	queryErr := errors.New("stop after query verification")
	mock.ExpectQuery(`(?s)^\s*INSERT INTO connection_risk_events .*ON CONFLICT \(dedupe_key\) WHERE status = 'open' AND dedupe_key <> ''\s*DO UPDATE SET.*RETURNING`).
		WithArgs(
			service.ConnectionRiskSubjectAPIKey,
			nil,
			int64(42),
			"sk-test",
			sqlmock.AnyArg(),
			"high",
			float64(80),
			service.ConnectionRiskStatusOpen,
			"title",
			"summary",
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			"k:42:R1:1",
			service.ConnectionRiskActionNone,
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			nil,
			nil,
			sqlmock.AnyArg(),
		).
		WillReturnError(queryErr)

	kid := int64(42)
	now := time.Now().UTC()
	repo := &connectionRiskRepository{db: db}
	_, err = repo.UpsertOpen(context.Background(), &service.ConnectionRiskEvent{
		SubjectType:  service.ConnectionRiskSubjectAPIKey,
		APIKeyID:     &kid,
		APIKeyPrefix: "sk-test",
		Severity:     "high",
		Score:        80,
		Status:       service.ConnectionRiskStatusOpen,
		Title:        "title",
		Summary:      "summary",
		DedupeKey:    "k:42:R1:1",
		ActionTaken:  service.ConnectionRiskActionNone,
		FirstSeenAt:  now,
		LastSeenAt:   now,
	})
	require.ErrorIs(t, err, queryErr)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestConnectionRiskRepository_UpdateActionTaken(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectExec(`(?s)UPDATE connection_risk_events\s+SET action_taken = \$1, updated_at = \$2\s+WHERE id = \$3`).
		WithArgs("throttled", sqlmock.AnyArg(), int64(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	repo := &connectionRiskRepository{db: db}
	require.NoError(t, repo.UpdateActionTaken(context.Background(), 9, "throttled"))
	require.NoError(t, mock.ExpectationsWereMet())
}
