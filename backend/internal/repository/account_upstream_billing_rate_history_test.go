package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

var upstreamBillingRateHistoryTestColumns = []string{
	"id", "detected_at", "group_rate_multiplier", "user_rate_multiplier",
	"peak_rate_enabled", "peak_start", "peak_end", "peak_timezone",
	"peak_rate_multiplier", "resolved_rate_multiplier", "effective_rate_multiplier",
}

func rateHistorySnapshot(receivedAt time.Time, peakEnabled bool) *service.UpstreamBillingProbeSnapshot {
	data := map[string]any{
		"billing_scope":             "token",
		"group_rate_multiplier":     1.0,
		"resolved_rate_multiplier":  1.0,
		"peak_rate_enabled":         peakEnabled,
		"effective_rate_multiplier": 1.0,
	}
	if peakEnabled {
		data["peak_start"] = "09:00"
		data["peak_end"] = "18:00"
		data["timezone"] = "Asia/Shanghai"
		data["peak_rate_multiplier"] = 1.0
	}
	return &service.UpstreamBillingProbeSnapshot{
		Status:     service.UpstreamBillingProbeStatusOK,
		Data:       data,
		ReceivedAt: &receivedAt,
	}
}

func addRateHistoryRow(rows *sqlmock.Rows, id int64, detectedAt time.Time) *sqlmock.Rows {
	return rows.AddRow(id, detectedAt, 1.0, nil, false, nil, nil, nil, nil, 1.0, 1.0)
}

func TestAppendUpstreamBillingRateHistoryStoresOnlyChanges(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	receivedAt := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	latestQuery := `(?s)` + regexp.QuoteMeta("FROM account_upstream_billing_rate_events") + `.*` + regexp.QuoteMeta("LIMIT 1 FOR UPDATE")

	mock.ExpectQuery(latestQuery).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows(upstreamBillingRateHistoryTestColumns))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO account_upstream_billing_rate_events")).
		WithArgs(int64(7), receivedAt, 1.0, nil, false, nil, nil, nil, nil, 1.0, 1.0).
		WillReturnResult(sqlmock.NewResult(1, 1))
	require.NoError(t, appendUpstreamBillingRateHistoryEvent(
		context.Background(), db, 7, rateHistorySnapshot(receivedAt, false),
	))

	mock.ExpectQuery(latestQuery).
		WithArgs(int64(7)).
		WillReturnRows(addRateHistoryRow(sqlmock.NewRows(upstreamBillingRateHistoryTestColumns), 1, receivedAt))
	require.NoError(t, appendUpstreamBillingRateHistoryEvent(
		context.Background(), db, 7, rateHistorySnapshot(receivedAt.Add(time.Hour), false),
	))

	mock.ExpectQuery(latestQuery).
		WithArgs(int64(7)).
		WillReturnRows(addRateHistoryRow(sqlmock.NewRows(upstreamBillingRateHistoryTestColumns), 1, receivedAt))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO account_upstream_billing_rate_events")).
		WithArgs(int64(7), receivedAt.Add(2*time.Hour), 1.0, nil, true, "09:00", "18:00", "Asia/Shanghai", 1.0, 1.0, 1.0).
		WillReturnResult(sqlmock.NewResult(2, 1))
	require.NoError(t, appendUpstreamBillingRateHistoryEvent(
		context.Background(), db, 7, rateHistorySnapshot(receivedAt.Add(2*time.Hour), true),
	))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListUpstreamBillingRateHistoryIncludesPredecessorAndTruncatesNewest(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := newAccountRepositoryWithSQL(nil, db, nil)
	since := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	first := since.Add(24 * time.Hour)
	second := since.Add(48 * time.Hour)
	predecessor := since.Add(-24 * time.Hour)
	listQuery := `(?s)` + regexp.QuoteMeta("WHERE account_id = $1 AND detected_at >= $2") + `.*` + regexp.QuoteMeta("LIMIT $3")
	predecessorQuery := `(?s)` + regexp.QuoteMeta("WHERE account_id = $1 AND detected_at < $2") + `.*` + regexp.QuoteMeta("LIMIT 1")

	rows := sqlmock.NewRows(upstreamBillingRateHistoryTestColumns)
	addRateHistoryRow(rows, 2, second)
	addRateHistoryRow(rows, 1, first)
	mock.ExpectQuery(listQuery).WithArgs(int64(7), since, 4).WillReturnRows(rows)
	mock.ExpectQuery(predecessorQuery).
		WithArgs(int64(7), since).
		WillReturnRows(addRateHistoryRow(sqlmock.NewRows(upstreamBillingRateHistoryTestColumns), 9, predecessor))
	events, truncated, err := repo.ListUpstreamBillingRateHistory(context.Background(), 7, since, 3)
	require.NoError(t, err)
	require.False(t, truncated)
	require.Equal(t, []int64{9, 1, 2}, []int64{events[0].ID, events[1].ID, events[2].ID})

	rows = sqlmock.NewRows(upstreamBillingRateHistoryTestColumns)
	addRateHistoryRow(rows, 3, since.Add(72*time.Hour))
	addRateHistoryRow(rows, 2, second)
	addRateHistoryRow(rows, 1, first)
	mock.ExpectQuery(listQuery).WithArgs(int64(7), since, 3).WillReturnRows(rows)
	events, truncated, err = repo.ListUpstreamBillingRateHistory(context.Background(), 7, since, 2)
	require.NoError(t, err)
	require.True(t, truncated)
	require.Equal(t, []int64{2, 3}, []int64{events[0].ID, events[1].ID})
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateCredentialsDeletesHistoryOnlyWhenBaseURLChanges(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })
	payload := `{"api_key":"sk-new","base_url":"https://new.example/v1"}`

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)`+regexp.QuoteMeta("SELECT COALESCE(credentials ->> 'base_url', '') IS DISTINCT FROM")+`.*`+regexp.QuoteMeta("FOR NO KEY UPDATE")).
		WithArgs(int64(27), payload).
		WillReturnRows(sqlmock.NewRows([]string{"base_url_changed"}).AddRow(true))
	mock.ExpectExec(`(?s)UPDATE accounts.*credentials IS DISTINCT FROM \$1::jsonb.*- 'upstream_billing_probe'`).
		WithArgs(payload, int64(27)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM account_upstream_billing_rate_events")).
		WithArgs(`{27}`).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).
		WithArgs(service.SchedulerOutboxEventAccountChanged, int64(27), nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := newAccountRepositoryWithSQL(client, db, nil)
	err = repo.UpdateCredentials(context.Background(), 27, map[string]any{
		"api_key": "sk-new", "base_url": "https://new.example/v1",
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBulkUpdateDeletesHistoryForChangedBaseURLsOnly(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })
	payload := []byte(`{"base_url":"https://new.example/v1"}`)

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)`+regexp.QuoteMeta("SELECT id,")+`.*`+regexp.QuoteMeta("FOR NO KEY UPDATE")).
		WithArgs(`{27,28}`, payload).
		WillReturnRows(sqlmock.NewRows([]string{"id", "base_url_changed"}).AddRow(int64(27), true).AddRow(int64(28), false))
	mock.ExpectExec(`(?s)`+regexp.QuoteMeta("UPDATE accounts SET credentials = COALESCE(credentials, '{}'::jsonb) || $1::jsonb")+`.*`+regexp.QuoteMeta("WHERE id = ANY($2)")).
		WithArgs(payload, `{27,28}`).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM account_upstream_billing_rate_events")).
		WithArgs(`{27}`).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)")).
		WithArgs(service.SchedulerOutboxEventAccountBulkChanged, nil, nil, accountIDsPayloadMatcher{want: []int64{27, 28}}).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := newAccountRepositoryWithSQL(client, db, nil)
	rows, err := repo.BulkUpdate(context.Background(), []int64{27, 28}, service.AccountBulkUpdate{
		Credentials: map[string]any{"base_url": "https://new.example/v1"},
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), rows)
	require.NoError(t, mock.ExpectationsWereMet())
}
