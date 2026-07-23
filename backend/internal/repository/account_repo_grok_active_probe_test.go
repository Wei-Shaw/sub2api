package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func TestListGrokActiveProbeCandidatesUsesLightweightProjection(t *testing.T) {
	var capturedSQL string
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureEntQueryMatcher{actual: &capturedSQL}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	driver := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	repo := newAccountRepositoryWithSQL(client, db, nil)

	mock.ExpectQuery("grok active probe projection").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	accounts, err := repo.ListGrokActiveProbeCandidates(context.Background())
	require.NoError(t, err)
	require.Empty(t, accounts)
	require.NoError(t, mock.ExpectationsWereMet())

	normalized := normalizeSQLWhitespace(capturedSQL)
	selectClause, _, found := strings.Cut(normalized, " FROM ")
	require.True(t, found, "unexpected projection SQL: %s", normalized)
	for _, required := range []string{
		"id", "platform", "type", "extra", "expires_at", "auto_pause_on_expired",
		"schedulable", "rate_limit_reset_at", "overload_until", "temp_unschedulable_until", "status",
	} {
		require.Contains(t, selectClause, `"`+required+`"`)
	}
	for _, excluded := range []string{"credentials", "name", "proxy_id", "proxy_fallback_origin_id"} {
		require.NotContains(t, selectClause, excluded)
	}
	require.NotContains(t, normalized, "account_groups")
	require.NotContains(t, normalized, "proxies")
	require.Contains(t, normalized, `"platform" = $1`)
	require.Contains(t, normalized, `"status" = $2`)
}
