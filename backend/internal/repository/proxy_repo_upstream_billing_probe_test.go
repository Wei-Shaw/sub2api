package repository

import (
	"context"
	"errors"
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

func TestProxyUpdateInvalidatesBoundProbeSnapshotsAndEnqueuesOutboxAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)` + regexp.QuoteMeta("SELECT protocol, host, port") + `.*` + regexp.QuoteMeta("FOR NO KEY UPDATE")).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"protocol", "host", "port", "username", "password", "status"}).
			AddRow("http", "old.example", 8080, "user", "pass", service.StatusActive))
	mock.ExpectExec(`(?s)UPDATE "proxies" SET`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "proxies" SET "backup_proxy_id" = NULL WHERE "backup_proxy_id" = \$1`).
		WithArgs(int64(9)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	expectProxyUpdateReload(mock, 9, "new.example", "user", "pass")
	mock.ExpectQuery(`(?s)UPDATE accounts.*platform = 'openai'.*type = 'apikey'.*extra \? 'upstream_billing_probe'.*extra -> 'upstream_billing_probe' <> 'null'::jsonb.*RETURNING id`).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(17)).AddRow(int64(18)))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)")).
		WithArgs(service.SchedulerOutboxEventAccountBulkChanged, nil, nil, accountIDsPayloadMatcher{want: []int64{17, 18}}).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := newProxyRepositoryWithSQL(client, db)
	proxy := &service.Proxy{
		ID:       9,
		Name:     "proxy",
		Protocol: "http",
		Host:     "new.example",
		Port:     8080,
		Username: "user",
		Password: "pass",
		Status:   service.StatusActive,
	}

	err = repo.Update(context.Background(), proxy)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProxyUpdateRollsBackWhenProbeInvalidationOutboxFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)` + regexp.QuoteMeta("SELECT protocol, host, port") + `.*` + regexp.QuoteMeta("FOR NO KEY UPDATE")).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"protocol", "host", "port", "username", "password", "status"}).
			AddRow("http", "old.example", 8080, "", "", service.StatusActive))
	mock.ExpectExec(`(?s)UPDATE "proxies" SET`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "proxies" SET "backup_proxy_id" = NULL WHERE "backup_proxy_id" = \$1`).
		WithArgs(int64(9)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	expectProxyUpdateReload(mock, 9, "new.example", "", "")
	mock.ExpectQuery(`(?s)UPDATE accounts.*platform = 'openai'.*type = 'apikey'.*extra \? 'upstream_billing_probe'.*extra -> 'upstream_billing_probe' <> 'null'::jsonb.*RETURNING id`).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(17)))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)")).
		WillReturnError(errors.New("outbox failed"))
	mock.ExpectRollback()

	repo := newProxyRepositoryWithSQL(client, db)
	proxy := &service.Proxy{ID: 9, Name: "proxy", Protocol: "http", Host: "new.example", Port: 8080, Status: service.StatusActive}

	err = repo.Update(context.Background(), proxy)

	require.EqualError(t, err, "outbox failed")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProxyUpdateSkipsProbeInvalidationForNonIdentityChange(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)` + regexp.QuoteMeta("SELECT protocol, host, port") + `.*` + regexp.QuoteMeta("FOR NO KEY UPDATE")).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"protocol", "host", "port", "username", "password", "status"}).
			AddRow("http", "same.example", 8080, "", "", service.StatusActive))
	mock.ExpectExec(`(?s)UPDATE "proxies" SET`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "proxies" SET "backup_proxy_id" = NULL WHERE "backup_proxy_id" = \$1`).
		WithArgs(int64(9)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	expectProxyUpdateReload(mock, 9, "same.example", "", "")
	mock.ExpectCommit()

	repo := newProxyRepositoryWithSQL(client, db)
	proxy := &service.Proxy{ID: 9, Name: "renamed", Protocol: "http", Host: "same.example", Port: 8080, Status: service.StatusActive}

	err = repo.Update(context.Background(), proxy)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func expectProxyUpdateReload(mock sqlmock.Sqlmock, id int64, host, username, password string) {
	now := time.Now()
	mock.ExpectQuery(`(?s)SELECT .* FROM "proxies" WHERE "id" = \$1`).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "name", "protocol", "host", "port",
			"username", "password", "status", "expires_at", "fallback_mode", "backup_proxy_id", "expiry_warn_days",
		}).AddRow(
			id, now, now, nil, "proxy", "http", host, 8080,
			username, password, service.StatusActive, nil, service.FallbackModeNone, nil, 0,
		))
}

func TestEnqueueProxyAccountChangesChunksLargePayloads(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	accountIDs := make([]int64, 1001)
	for i := range accountIDs {
		accountIDs[i] = int64(i + 1)
	}
	for start := 0; start < len(accountIDs); start += proxyProbeOutboxAccountChunkSize {
		end := start + proxyProbeOutboxAccountChunkSize
		if end > len(accountIDs) {
			end = len(accountIDs)
		}
		mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)")).
			WithArgs(service.SchedulerOutboxEventAccountBulkChanged, nil, nil, accountIDsPayloadMatcher{want: accountIDs[start:end]}).
			WillReturnResult(sqlmock.NewResult(1, 1))
	}

	err = enqueueProxyProbeAccountChanges(context.Background(), db, accountIDs)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
