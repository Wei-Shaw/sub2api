package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func TestBulkUpdateEnsuresCodexFingerprintSeedWithPerRowSQL(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(0)REDACTED
	repo := newAccountRepositoryWithSQL(nil, exec, nil)

	_, err := repo.BulkUpdate(context.Background(), []int64{27, 28REDACTED, service.AccountBulkUpdate{
		Extra: map[string]any{
			"codex_fingerprint_mode": "session",
			"codex_fingerprint_seed": "22222222-2222-4222-8222-222222222222",
	REDACTED,
		EnsureCodexFingerprintSeed: true,
REDACTED)

REDACTED
	require.NotEmpty(t, exec.execQueries)
	query := normalizeSQLWhitespace(exec.execQueries[0])
	require.Contains(t, query, "jsonb_set")
	require.Contains(t, query, "gen_random_uuid()::text")
	require.Contains(t, query, "platform = 'openai' AND type = 'oauth'")
	require.Contains(t, query, "to_jsonb(extra ->> 'codex_fingerprint_seed')")
	require.Contains(t, query, codexFingerprintSeedCanonicalPattern)
	require.NotContains(t, query, "22222222-2222-4222-8222-222222222222")
	require.NotEmpty(t, exec.execArgs)
	payload, ok := exec.execArgs[0][0].([]byte)
	require.True(t, ok)
	require.Equal(t, `{"codex_fingerprint_mode":"session"REDACTED`, string(payload))
REDACTED

func TestUpdateExtraEnsuresCodexFingerprintSeedAtomicallyWhenEnabling(t *testing.T) {
	db, mock, err := sqlmock.New()
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() REDACTED)

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE accounts SET extra = .*jsonb_set.*gen_random_uuid\(\)::text.*WHERE id = \$2 AND deleted_at IS NULL`).
		WithArgs(`{"codex_fingerprint_mode":"device"REDACTED`, int64(27)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).
		WithArgs(service.SchedulerOutboxEventAccountChanged, int64(27), nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	repo := newAccountRepositoryWithSQL(client, db, nil)

	err = repo.UpdateExtra(context.Background(), 27, map[string]any{
		"codex_fingerprint_mode": "device",
		"codex_fingerprint_seed": "22222222-2222-4222-8222-222222222222",
REDACTED)

REDACTED
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestBulkUpdateCodexFingerprintSeedRollsBackWhenUpdateFails(t *testing.T) {
	db, mock, err := sqlmock.New()
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() REDACTED)

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE accounts SET extra = .*gen_random_uuid\(\)::text.*WHERE id = ANY\(\$2\)`).
		WithArgs(sqlmock.AnyArg(), `{27,28REDACTED`).
		WillReturnError(errors.New("update failed"))
	mock.ExpectRollback()

	repo := newAccountRepositoryWithSQL(client, db, nil)
	rows, err := repo.BulkUpdate(context.Background(), []int64{27, 28REDACTED, service.AccountBulkUpdate{
		Extra: map[string]any{
			"codex_fingerprint_mode": "session",
	REDACTED,
		EnsureCodexFingerprintSeed: true,
REDACTED)

	require.EqualError(t, err, "update failed")
	require.Zero(t, rows)
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestBulkUpdateCodexFingerprintSeedRollsBackWhenOutboxFails(t *testing.T) {
	db, mock, err := sqlmock.New()
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() REDACTED)

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE accounts SET extra = .*gen_random_uuid\(\)::text.*WHERE id = ANY\(\$2\)`).
		WithArgs(sqlmock.AnyArg(), `{27,28REDACTED`).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).
		WillReturnError(errors.New("outbox failed"))
	mock.ExpectRollback()

	repo := newAccountRepositoryWithSQL(client, db, nil)
	rows, err := repo.BulkUpdate(context.Background(), []int64{27, 28REDACTED, service.AccountBulkUpdate{
		Extra: map[string]any{
			"codex_fingerprint_mode": "full",
	REDACTED,
		EnsureCodexFingerprintSeed: true,
REDACTED)

	require.EqualError(t, err, "outbox failed")
	require.Zero(t, rows)
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED
