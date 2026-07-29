package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func TestListModelAvailabilityCandidates_GroupQueryIgnoresTransientState(t *testing.T) {
	var capturedSQL string
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureEntQueryMatcher{actual: &capturedSQL}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	driver := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	repo := newAccountRepositoryWithSQL(client, db, nil)

	mock.ExpectQuery("model availability candidates").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	groupID := int64(42)
	accounts, err := repo.ListModelAvailabilityCandidates(
		context.Background(),
		&groupID,
		[]string{service.PlatformAnthropic},
		false,
	)
	require.NoError(t, err)
	require.Empty(t, accounts)
	require.NoError(t, mock.ExpectationsWereMet())

	normalized := normalizeSQLWhitespace(capturedSQL)
	_, whereClause, found := strings.Cut(normalized, " WHERE ")
	require.True(t, found, "expected WHERE clause in query: %s", normalized)
	whereClause, _, _ = strings.Cut(whereClause, " ORDER BY ")
	for _, configuredPredicate := range []string{"group_id", "status", "schedulable", "platform"} {
		require.Contains(t, whereClause, configuredPredicate)
	}
	for _, transientPredicate := range []string{
		"rate_limit_reset_at",
		"overload_until",
		"temp_unschedulable_until",
		"expires_at",
		"auto_pause_on_expired",
	} {
		require.NotContains(t, whereClause, transientPredicate, "configured-state diagnosis must not filter transient predicate %q", transientPredicate)
	}
}
