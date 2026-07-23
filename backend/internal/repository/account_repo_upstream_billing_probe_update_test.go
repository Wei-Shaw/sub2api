package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func TestLockAndMergeAccountProbeExtraUsesCurrentDatabaseSnapshot(t *testing.T) {
	tests := []struct {
		name              string
		identityUnchanged bool
		databaseEnabled   any
		databaseSnapshot  any
		inputExtra        map[string]any
		wantSnapshot      any
		wantEnabled       any
REDACTED{
		{
			name:              "ordinary edit preserves current enable flag and snapshot created after account load",
			identityUnchanged: true,
			databaseEnabled:   []byte(`true`),
			databaseSnapshot:  []byte(`{"status":"ok"REDACTED`),
			inputExtra:        map[string]any{service.UpstreamBillingProbeEnabledExtraKey: falseREDACTED,
			wantSnapshot:      map[string]any{"status": "ok"REDACTED,
			wantEnabled:       true,
	REDACTED,
		{
			name:              "identity change clears stale snapshot",
			identityUnchanged: false,
			databaseEnabled:   []byte(`true`),
			databaseSnapshot:  []byte(`{"status":"ok"REDACTED`),
			inputExtra: map[string]any{
				service.UpstreamBillingProbeEnabledExtraKey: true,
				service.UpstreamBillingProbeExtraKey:        map[string]any{"status": "stale"REDACTED,
		REDACTED,
			wantEnabled: true,
	REDACTED,
		{
			name:              "current explicit disable clears snapshot",
			identityUnchanged: true,
			databaseEnabled:   []byte(`false`),
			databaseSnapshot:  []byte(`{"status":"ok"REDACTED`),
			inputExtra: map[string]any{
				service.UpstreamBillingProbeEnabledExtraKey: true,
				service.UpstreamBillingProbeExtraKey:        map[string]any{"status": "stale"REDACTED,
		REDACTED,
			wantEnabled: false,
	REDACTED,
		{
			name:              "missing database snapshot is not resurrected from stale input",
			identityUnchanged: true,
			databaseEnabled:   []byte(`true`),
			databaseSnapshot:  nil,
			inputExtra: map[string]any{
				service.UpstreamBillingProbeEnabledExtraKey: true,
				service.UpstreamBillingProbeExtraKey:        map[string]any{"status": "stale"REDACTED,
		REDACTED,
			wantEnabled: true,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
		REDACTED
			t.Cleanup(func() { _ = db.Close() REDACTED)
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			t.Cleanup(func() { _ = client.Close() REDACTED)

			mock.ExpectQuery(`(?s)`+regexp.QuoteMeta("SELECT")+`.*`+regexp.QuoteMeta("FOR NO KEY UPDATE")).
				WithArgs(int64(27), service.PlatformOpenAI, service.AccountTypeAPIKey, `{"api_key":"sk-test"REDACTED`, nil).
				WillReturnRows(sqlmock.NewRows([]string{"identity_unchanged", "ollama_group_unchanged", "ollama_proxy_unchanged", "enabled", "snapshot", "ollama_session", "ollama_auto", "ollama_snapshot"REDACTED).
					AddRow(tt.identityUnchanged, false, true, tt.databaseEnabled, tt.databaseSnapshot, nil, nil, nil))

			account := &service.Account{
				ID:          27,
				Platform:    service.PlatformOpenAI,
				Type:        service.AccountTypeAPIKey,
		REDACTED"api_key": "sk-test"REDACTED,
				Extra:       tt.inputExtra,
		REDACTED
			got, err := lockAndMergeAccountProbeExtra(context.Background(), client, account, nil)
		REDACTED
			if tt.wantSnapshot == nil {
				require.NotContains(t, got, service.UpstreamBillingProbeExtraKey)
		REDACTED else {
				require.Equal(t, tt.wantSnapshot, got[service.UpstreamBillingProbeExtraKey])
		REDACTED
			require.Equal(t, tt.wantEnabled, got[service.UpstreamBillingProbeEnabledExtraKey])
			require.NoError(t, mock.ExpectationsWereMet())
	REDACTED)
REDACTED
REDACTED

func TestLockAndMergeAccountProbeExtraProtectsOllamaManagedFields(t *testing.T) {
	for _, identityUnchanged := range []bool{true, falseREDACTED {
		t.Run(map[bool]string{true: "same identity keeps snapshot", false: "changed identity clears snapshot"REDACTED[identityUnchanged], func(t *testing.T) {
			db, mock, err := sqlmock.New()
		REDACTED
			t.Cleanup(func() { _ = db.Close() REDACTED)
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			t.Cleanup(func() { _ = client.Close() REDACTED)

			mock.ExpectQuery(`(?s)`+regexp.QuoteMeta("SELECT")+`.*`+regexp.QuoteMeta("FOR NO KEY UPDATE")).
				WithArgs(int64(29), service.PlatformAnthropic, service.AccountTypeAPIKey, `{"api_key":"key","base_url":"https://ollama.com"REDACTED`, nil).
				WillReturnRows(sqlmock.NewRows([]string{"identity_unchanged", "ollama_group_unchanged", "ollama_proxy_unchanged", "enabled", "snapshot", "ollama_session", "ollama_auto", "ollama_snapshot"REDACTED).
					AddRow(identityUnchanged, identityUnchanged, true, nil, nil, []byte(`"local-ciphertext"`), []byte(`true`), []byte(`{"status":"ok"REDACTED`)))

			account := &service.Account{
				ID: 29, Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey,
		REDACTED"api_key": "key", "base_url": "https://ollama.com"REDACTED,
				Extra: map[string]any{
					service.OllamaCloudUsageSessionExtraKey:     "forged-ciphertext",
					service.OllamaCloudUsageAutoRefreshExtraKey: false,
					service.OllamaCloudUsageSnapshotExtraKey:    map[string]any{"status": "forged"REDACTED,
			REDACTED,
		REDACTED
			got, err := lockAndMergeAccountProbeExtra(context.Background(), client, account, nil)
		REDACTED
			if identityUnchanged {
				require.Equal(t, "local-ciphertext", got[service.OllamaCloudUsageSessionExtraKey])
				require.Equal(t, true, got[service.OllamaCloudUsageAutoRefreshExtraKey])
				require.Equal(t, map[string]any{"status": "ok"REDACTED, got[service.OllamaCloudUsageSnapshotExtraKey])
		REDACTED else {
				require.NotContains(t, got, service.OllamaCloudUsageSessionExtraKey)
				require.NotContains(t, got, service.OllamaCloudUsageAutoRefreshExtraKey)
				require.NotContains(t, got, service.OllamaCloudUsageSnapshotExtraKey)
		REDACTED
			require.NoError(t, mock.ExpectationsWereMet())
	REDACTED)
REDACTED
REDACTED

func TestUpdateExtraExplicitProbeDisableRemovesSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New()
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() REDACTED)

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE accounts SET extra = .* - 'upstream_billing_probe'`).
		WithArgs(`{"upstream_billing_probe_enabled":falseREDACTED`, int64(27)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).
		WithArgs(service.SchedulerOutboxEventAccountChanged, int64(27), nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	repo := newAccountRepositoryWithSQL(client, db, nil)

	err = repo.UpdateExtra(context.Background(), 27, map[string]any{service.UpstreamBillingProbeEnabledExtraKey: falseREDACTED)

REDACTED
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestUpdateExtraNilProbeRemovesKeyInsteadOfWritingJSONNull(t *testing.T) {
	db, mock, err := sqlmock.New()
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() REDACTED)

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE accounts SET extra = .* - 'upstream_billing_probe'`).
		WithArgs(`{"upstream_billing_probe":nullREDACTED`, int64(27)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).
		WithArgs(service.SchedulerOutboxEventAccountChanged, int64(27), nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	repo := newAccountRepositoryWithSQL(client, db, nil)

	err = repo.UpdateExtra(context.Background(), 27, map[string]any{service.UpstreamBillingProbeExtraKey: nilREDACTED)

REDACTED
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestBulkUpdateNilProbeRemovesKeyInsteadOfWritingJSONNull(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(1)REDACTED
	repo := newAccountRepositoryWithSQL(nil, exec, nil)

	_, err := repo.BulkUpdate(context.Background(), []int64{27REDACTED, service.AccountBulkUpdate{
		Extra: map[string]any{service.UpstreamBillingProbeExtraKey: nilREDACTED,
REDACTED)

REDACTED
	require.NotEmpty(t, exec.execQueries)
	require.Contains(t, normalizeSQLWhitespace(exec.execQueries[0]), "- 'upstream_billing_probe'")
REDACTED

func TestBulkUpdateDisablingProbeRemovesSnapshot(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(1)REDACTED
	repo := newAccountRepositoryWithSQL(nil, exec, nil)

	_, err := repo.BulkUpdate(context.Background(), []int64{27REDACTED, service.AccountBulkUpdate{
		Extra: map[string]any{service.UpstreamBillingProbeEnabledExtraKey: falseREDACTED,
REDACTED)

REDACTED
	require.NotEmpty(t, exec.execQueries)
	require.Contains(t, normalizeSQLWhitespace(exec.execQueries[0]), "- 'upstream_billing_probe'")
	payload, ok := exec.execArgs[0][0].([]byte)
	require.True(t, ok)
	require.Equal(t, `{"upstream_billing_probe_enabled":falseREDACTED`, string(payload))
REDACTED

func TestBulkUpdateProbeEligibilityMismatchRollsBack(t *testing.T) {
	db, mock, err := sqlmock.New()
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() REDACTED)

	enabled := true
	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE accounts SET extra = .* WHERE id = ANY\(\$2\) AND deleted_at IS NULL AND platform = \$3 AND type = \$4`).
		WithArgs(sqlmock.AnyArg(), `{27,28REDACTED`, service.PlatformOpenAI, service.AccountTypeAPIKey).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	repo := newAccountRepositoryWithSQL(client, db, nil)
	rows, err := repo.BulkUpdate(context.Background(), []int64{27, 28REDACTED, service.AccountBulkUpdate{
		ProbeEnabled: &enabled,
REDACTED)

	require.ErrorIs(t, err, service.ErrUpstreamBillingProbeAccountInvalid)
	require.Zero(t, rows)
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestUpdateCredentialsAtomicallyClearsProbeForOpenAIAPIKeyIdentityChange(t *testing.T) {
	db, mock, err := sqlmock.New()
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() REDACTED)

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE accounts.*credentials IS DISTINCT FROM \$1::jsonb.*- 'upstream_billing_probe'`).
		WithArgs(`{"api_key":"sk-new"REDACTED`, int64(27)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).
		WithArgs(service.SchedulerOutboxEventAccountChanged, int64(27), nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	repo := newAccountRepositoryWithSQL(client, db, nil)

	err = repo.UpdateCredentials(context.Background(), 27, map[string]any{"api_key": "sk-new"REDACTED)

REDACTED
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestUpdateWithUpstreamBillingProbeEnabledRollsBackWhenOutboxFails(t *testing.T) {
	db, mock, err := sqlmock.New()
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() REDACTED)

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)`+regexp.QuoteMeta("SELECT")+`.*`+regexp.QuoteMeta("FOR NO KEY UPDATE")).
		WithArgs(int64(27), service.PlatformOpenAI, service.AccountTypeAPIKey, `{"api_key":"sk-test"REDACTED`, nil).
		WillReturnRows(sqlmock.NewRows([]string{"identity_unchanged", "ollama_group_unchanged", "ollama_proxy_unchanged", "enabled", "snapshot", "ollama_session", "ollama_auto", "ollama_snapshot"REDACTED).
			AddRow(true, false, true, []byte(`true`), []byte(`{"status":"ok"REDACTED`), nil, nil, nil))
	mock.ExpectExec(`(?s)UPDATE .*accounts.*SET.*WHERE .*id.*`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT .* FROM "accounts" WHERE "id" = \$1`).
		WithArgs(int64(27)).
		WillReturnRows(updatedAccountRows(27, `{"upstream_billing_probe_enabled":falseREDACTED`))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).WillReturnError(errors.New("outbox failed"))
	mock.ExpectRollback()

	repo := newAccountRepositoryWithSQL(client, db, nil)
	account := &service.Account{
		ID:          27,
		Name:        "test",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
REDACTED"api_key": "sk-test"REDACTED,
		Extra: map[string]any{
			service.UpstreamBillingProbeExtraKey: map[string]any{"status": "stale"REDACTED,
	REDACTED,
		Concurrency: 1,
		Priority:    1,
		Status:      service.StatusActive,
		Schedulable: true,
REDACTED

	err = repo.UpdateWithUpstreamBillingProbeEnabled(context.Background(), account, false)

	require.EqualError(t, err, "outbox failed")
	require.Equal(t, false, account.Extra[service.UpstreamBillingProbeEnabledExtraKey])
	require.NotContains(t, account.Extra, service.UpstreamBillingProbeExtraKey)
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestUpdateExtraRollsBackWhenOutboxFails(t *testing.T) {
	db, mock, err := sqlmock.New()
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() REDACTED)

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE accounts SET extra = .* - 'upstream_billing_probe'`).
		WithArgs(`{"upstream_billing_probe_enabled":falseREDACTED`, int64(27)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).WillReturnError(errors.New("outbox failed"))
	mock.ExpectRollback()

	repo := newAccountRepositoryWithSQL(client, db, nil)
	err = repo.UpdateExtra(context.Background(), 27, map[string]any{service.UpstreamBillingProbeEnabledExtraKey: falseREDACTED)

	require.EqualError(t, err, "outbox failed")
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestUpdateCredentialsRollsBackWhenOutboxFails(t *testing.T) {
	db, mock, err := sqlmock.New()
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() REDACTED)

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE accounts.*credentials IS DISTINCT FROM \$1::jsonb.*- 'upstream_billing_probe'`).
		WithArgs(`{"api_key":"sk-new"REDACTED`, int64(27)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).WillReturnError(errors.New("outbox failed"))
	mock.ExpectRollback()

	repo := newAccountRepositoryWithSQL(client, db, nil)
	err = repo.UpdateCredentials(context.Background(), 27, map[string]any{"api_key": "sk-new"REDACTED)

	require.EqualError(t, err, "outbox failed")
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestBulkUpdateRollsBackWhenOutboxFails(t *testing.T) {
	db, mock, err := sqlmock.New()
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() REDACTED)

	name := "renamed"
	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE accounts SET name = \$1.*WHERE id = ANY\(\$2\)`).
		WithArgs(name, `{27,28REDACTED`).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).WillReturnError(errors.New("outbox failed"))
	mock.ExpectRollback()

	repo := newAccountRepositoryWithSQL(client, db, nil)
	rows, err := repo.BulkUpdate(context.Background(), []int64{27, 28REDACTED, service.AccountBulkUpdate{Name: &nameREDACTED)

	require.EqualError(t, err, "outbox failed")
	require.Zero(t, rows)
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func updatedAccountRows(id int64, extra string) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows(dbaccount.Columns).AddRow(
		id, now, now, nil, "test", nil, service.PlatformOpenAI, service.AccountTypeAPIKey,
		[]byte(`{"api_key":"sk-test"REDACTED`), []byte(extra), nil, nil, 1, nil, 1, 1.0,
		service.StatusActive, nil, nil, nil, false, true, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, service.QuotaDimensionGlobal,
	)
REDACTED
