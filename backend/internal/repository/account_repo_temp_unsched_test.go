package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"regexp"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountRepository_SetTempUnschedulable_NoRowsAffectedDoesNotWriteOutbox(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(0)}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)
	until := time.Now().Add(10 * time.Minute)

	err := repo.SetTempUnschedulable(context.Background(), 42, until, "retry")
	require.NoError(t, err)
	require.Len(t, exec.execQueries, 1)
	require.Contains(t, exec.execQueries[0], "UPDATE accounts")
	require.NotContains(t, strings.Join(exec.execQueries, "\n"), "scheduler_outbox")
}

func TestAccountRepository_GrokCredentialConditionalMutationsAreEligibleAndAtomicallyPropagated(t *testing.T) {
	proxyID := int64(77)
	snapshot := service.GrokCredentialMutationSnapshot{
		CredentialsJSON: `{"access_token":"access","refresh_token":"refresh","_token_version":123}`,
		ProxyID:         &proxyID,
	}

	t.Run("permanent", func(t *testing.T) {
		exec := &recordingSQLExecutor{result: rowsAffectedResult(0)}
		repo := newAccountRepositoryWithSQL(nil, exec, nil)

		updated, err := repo.SetGrokCredentialErrorIfMatch(context.Background(), 42, snapshot, "revoked")

		require.NoError(t, err)
		require.False(t, updated)
		require.Len(t, exec.execQueries, 1)
		normalized := normalizeSQLWhitespace(exec.execQueries[0])
		require.Contains(t, normalized, "WITH updated AS ( UPDATE accounts AS a")
		require.Contains(t, normalized, "a.schedulable IS TRUE")
		require.Contains(t, normalized, "a.temp_unschedulable_until IS NULL OR a.temp_unschedulable_until <= NOW()")
		require.Contains(t, normalized, "a.rate_limit_reset_at IS NULL OR a.rate_limit_reset_at <= NOW()")
		require.Contains(t, normalized, "a.overload_until IS NULL OR a.overload_until <= NOW()")
		require.Contains(t, normalized, "a.credentials = $7::jsonb")
		require.Contains(t, normalized, "a.proxy_id IS NOT DISTINCT FROM $8")
		require.Contains(t, normalized, "NOT EXISTS ( SELECT 1 FROM proxies p")
		require.Contains(t, normalized, "INSERT INTO scheduler_outbox")
		require.Len(t, exec.execArgs[0], 10)
		require.Equal(t, snapshot.CredentialsJSON, exec.execArgs[0][6])
		require.Equal(t, &proxyID, exec.execArgs[0][7])
		require.Equal(t, string(service.GrokCredentialReasonProxyInvalid), exec.execArgs[0][8])
		require.Equal(t, service.SchedulerOutboxEventAccountChanged, exec.execArgs[0][9])
	})

	t.Run("transient", func(t *testing.T) {
		exec := &recordingSQLExecutor{result: rowsAffectedResult(0)}
		repo := newAccountRepositoryWithSQL(nil, exec, nil)

		updated, err := repo.SetGrokCredentialTempUnschedulableIfMatch(
			context.Background(), 42, snapshot, time.Now().Add(time.Minute), "temporary",
		)

		require.NoError(t, err)
		require.False(t, updated)
		require.Len(t, exec.execQueries, 1)
		normalized := normalizeSQLWhitespace(exec.execQueries[0])
		require.Contains(t, normalized, "WITH updated AS ( UPDATE accounts AS a")
		require.Contains(t, normalized, "a.schedulable IS TRUE")
		require.Contains(t, normalized, "a.temp_unschedulable_until IS NULL OR a.temp_unschedulable_until <= NOW()")
		require.Contains(t, normalized, "a.rate_limit_reset_at IS NULL OR a.rate_limit_reset_at <= NOW()")
		require.Contains(t, normalized, "a.overload_until IS NULL OR a.overload_until <= NOW()")
		require.Contains(t, normalized, "a.credentials = $7::jsonb")
		require.Contains(t, normalized, "a.proxy_id IS NOT DISTINCT FROM $8")
		require.Contains(t, normalized, "INSERT INTO scheduler_outbox")
		require.Len(t, exec.execArgs[0], 9)
		require.Equal(t, snapshot.CredentialsJSON, exec.execArgs[0][6])
		require.Equal(t, &proxyID, exec.execArgs[0][7])
		require.Equal(t, service.SchedulerOutboxEventAccountChanged, exec.execArgs[0][8])
	})
}

func TestAccountRepository_GrokCredentialCommitCarriesOutboxAcrossCallerCancellation(t *testing.T) {
	snapshot := service.GrokCredentialMutationSnapshot{CredentialsJSON: `{"access_token":"access","refresh_token":"refresh"}`}
	tests := []struct {
		name   string
		mutate func(context.Context, *accountRepository) (bool, error)
	}{
		{
			name: "permanent",
			mutate: func(ctx context.Context, repo *accountRepository) (bool, error) {
				return repo.SetGrokCredentialErrorIfMatch(ctx, 42, snapshot, string(service.GrokCredentialReasonRevoked))
			},
		},
		{
			name: "transient",
			mutate: func(ctx context.Context, repo *accountRepository) (bool, error) {
				return repo.SetGrokCredentialTempUnschedulableIfMatch(ctx, 42, snapshot, time.Now().Add(time.Minute), string(service.GrokCredentialReasonRefreshTransient))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			exec := &recordingSQLExecutor{result: rowsAffectedResult(1), afterExec: cancel}
			repo := newAccountRepositoryWithSQL(nil, exec, nil)

			updated, err := tt.mutate(ctx, repo)

			require.NoError(t, err)
			require.True(t, updated)
			require.ErrorIs(t, ctx.Err(), context.Canceled)
			require.Len(t, exec.execQueries, 1, "state update and scheduler outbox must share one atomic SQL statement")
			require.Contains(t, normalizeSQLWhitespace(exec.execQueries[0]), "INSERT INTO scheduler_outbox")
		})
	}
}

func TestAccountRepository_SetGrokOAuthErrorIfCredentialsUnchanged_RequiresActiveExactCredentialMatch(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(0)}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)

	applied, err := repo.SetGrokOAuthErrorIfCredentialsUnchanged(
		context.Background(),
		42,
		map[string]any{"access_token": "observed", "_token_version": int64(7)},
		"missing refresh token",
	)

	require.NoError(t, err)
	require.False(t, applied)
	require.Len(t, exec.execQueries, 1, "the account mutation and conditional outbox insert must be one statement")
	normalized := normalizeSQLWhitespace(exec.execQueries[0])
	require.Contains(t, normalized, "WITH updated AS")
	require.Contains(t, normalized, "INSERT INTO scheduler_outbox")
	require.Contains(t, normalized, "FROM updated")
	require.Contains(t, normalized, "platform = $4")
	require.Contains(t, normalized, "type = $5")
	require.Contains(t, normalized, "status = $6")
	require.Contains(t, normalized, "credentials = $7::jsonb")
	require.Contains(t, normalized, "NULLIF(BTRIM(a.credentials->>'refresh_token'), '') IS NULL")
	require.Len(t, exec.execArgs, 1)
	require.Equal(t, service.StatusActive, exec.execArgs[0][5])
	require.Contains(t, exec.execArgs[0][6], `"_token_version":7`)
}

func TestAccountRepository_SetGrokOAuthErrorIfCredentialsUnchanged_AppliedWritesOutbox(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(1)}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)

	applied, err := repo.SetGrokOAuthErrorIfCredentialsUnchanged(
		context.Background(),
		42,
		map[string]any{"access_token": "observed"},
		"missing refresh token",
	)

	require.NoError(t, err)
	require.True(t, applied)
	require.Len(t, exec.execQueries, 1)
	normalized := normalizeSQLWhitespace(exec.execQueries[0])
	require.Contains(t, normalized, "WITH updated AS")
	require.Contains(t, normalized, "INSERT INTO scheduler_outbox")
	require.Contains(t, normalized, "SELECT $8, updated.id, NULL, NULL FROM updated")
}

func TestAccountRepository_SetGrokOAuthRefreshErrorIfCredentialsUnchanged_UsesAttemptCredentialsAndProxy(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(0)}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)
	proxyID := int64(17)

	applied, err := repo.SetGrokOAuthRefreshErrorIfCredentialsUnchanged(
		context.Background(),
		42,
		map[string]any{"refresh_token": "attempted", "_token_version": int64(7)},
		&proxyID,
		"revoked",
	)

	require.NoError(t, err)
	require.False(t, applied)
	require.Len(t, exec.execQueries, 1)
	normalized := normalizeSQLWhitespace(exec.execQueries[0])
	require.Contains(t, normalized, "credentials = $7::jsonb")
	require.Contains(t, normalized, "proxy_id IS NOT DISTINCT FROM $8")
	require.NotContains(t, normalized, "credentials->>'refresh_token'",
		"background invalid_grant CAS must accept the attempted refresh token; only reconciliation requires it missing")
	require.Equal(t, &proxyID, exec.execArgs[0][7])
	require.Contains(t, normalized, "INSERT INTO scheduler_outbox")
	require.Len(t, exec.execArgs[0], 9)
}

func TestAccountRepository_SetGrokOAuthRefreshTempUnschedulableIfCredentialsUnchanged_UsesAttemptCredentialsAndProxy(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(0)}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)
	proxyID := int64(19)

	applied, err := repo.SetGrokOAuthRefreshTempUnschedulableIfCredentialsUnchanged(
		context.Background(),
		42,
		map[string]any{"refresh_token": "attempted", "_token_version": int64(8)},
		&proxyID,
		time.Now().Add(10*time.Minute),
		"retry exhausted",
	)

	require.NoError(t, err)
	require.False(t, applied)
	require.Len(t, exec.execQueries, 1)
	normalized := normalizeSQLWhitespace(exec.execQueries[0])
	require.Contains(t, normalized, "credentials = $7::jsonb")
	require.Contains(t, normalized, "proxy_id IS NOT DISTINCT FROM $8")
	require.Contains(t, normalized, "a.temp_unschedulable_until IS NULL OR a.temp_unschedulable_until < $1")
	require.Len(t, exec.execArgs[0], 9)
	require.Equal(t, &proxyID, exec.execArgs[0][7])
	require.Contains(t, normalized, "INSERT INTO scheduler_outbox")
}

func TestAccountRepository_UpdateGrokOAuthCredentialsIfUnchanged_UsesExactAttemptStateAndAtomicOutbox(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(1)}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)
	proxyID := int64(29)

	applied, err := repo.UpdateGrokOAuthCredentialsIfUnchanged(
		context.Background(),
		42,
		map[string]any{"refresh_token": "attempted", "_token_version": int64(9)},
		&proxyID,
		map[string]any{"refresh_token": "rotated", "_token_version": int64(10)},
	)

	require.NoError(t, err)
	require.True(t, applied)
	require.Len(t, exec.execQueries, 1)
	normalized := normalizeSQLWhitespace(exec.execQueries[0])
	require.Contains(t, normalized, "WITH updated AS")
	require.Contains(t, normalized, "credentials = $1::jsonb")
	require.Contains(t, normalized, "credentials = $5::jsonb")
	require.Contains(t, normalized, "proxy_id IS NOT DISTINCT FROM $6")
	require.Contains(t, normalized, "INSERT INTO scheduler_outbox")
	require.Len(t, exec.execArgs[0], 7)
	require.Equal(t, &proxyID, exec.execArgs[0][5])
}

func TestAccountRepository_ListOAuthRefreshCandidatePage_SQLFilter(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	var capturedSQL string
	var capturedArgs []any
	mock.ExpectQuery("SELECT id").
		WillReturnRows(sqlmock.NewRows([]string{"id"})).
		WillDelayFor(0)

	repo := newAccountRepositoryWithSQL(nil, captureQuerySQL{db: db, captured: &capturedSQL, args: &capturedArgs}, nil)

	page, err := repo.ListOAuthRefreshCandidatePage(context.Background(), service.OAuthRefreshPageOptions{
		Platforms:            []string{service.PlatformAnthropic, service.PlatformOpenAI, service.PlatformGemini, service.PlatformAntigravity, service.PlatformGrok},
		AfterID:              100,
		Limit:                200,
		ActiveOnly:           true,
		IncludeSetupToken:    true,
		RequireRefreshToken:  true,
		ExcludeRetryCooldown: true,
	})
	require.NoError(t, err)
	require.Empty(t, page.Accounts)

	normalized := normalizeSQLWhitespace(capturedSQL)
	require.Contains(t, normalized, "deleted_at IS NULL")
	require.Contains(t, normalized, "status = 'active'")
	// setup-token 的 access_token 同为 8h 短期令牌，必须与 oauth 一起纳入后台刷新候选
	require.Contains(t, normalized, "type IN ('oauth', 'setup-token')")
	require.Contains(t, normalized, "platform = ANY($1)")
	require.NotContains(t, normalized, "platform IN ('anthropic'",
		"candidate platforms must come from the refresher registry instead of a second hard-coded list")
	require.Contains(t, normalized, "credentials ? 'refresh_token'")
	require.Contains(t, normalized, "btrim(credentials->>'refresh_token') <> ''")
	require.Contains(t, normalized, "temp_unschedulable_until > NOW()")
	require.Contains(t, normalized, "temp_unschedulable_reason LIKE 'token refresh retry exhausted:%'")
	require.Contains(t, normalized, "IS NOT TRUE",
		"must use IS NOT TRUE so accounts with NULL temp_unschedulable_until are not silently excluded by PG 3-valued logic")
	require.NotContains(t, normalized, "AND NOT (",
		"plain NOT (...) excludes NULL temp_unschedulable_until rows (the common healthy case)")
	require.Contains(t, normalized, "id > $2")
	require.Contains(t, normalized, "ORDER BY id ASC")
	require.Contains(t, normalized, "LIMIT $3")
	require.NotContains(t, normalized, "credentials->>'expires_at'")
	require.Len(t, capturedArgs, 3)
	require.Equal(t, int64(100), capturedArgs[1])
	require.Equal(t, 200, capturedArgs[2])
	valuer, ok := capturedArgs[0].(interface{ Value() (driver.Value, error) })
	require.True(t, ok)
	platforms, err := valuer.Value()
	require.NoError(t, err)
	require.Contains(t, platforms, service.PlatformGrok)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountRepository_ListOAuthRefreshCandidatePage_ReconciliationExcludesAPIKeys(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	var capturedSQL string
	mock.ExpectQuery("SELECT id").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	repo := newAccountRepositoryWithSQL(nil, captureQuerySQL{db: db, captured: &capturedSQL}, nil)

	page, err := repo.ListOAuthRefreshCandidatePage(context.Background(), service.OAuthRefreshPageOptions{
		Platforms: []string{service.PlatformGrok},
		AfterID:   0,
		Limit:     50,
	})
	require.NoError(t, err)
	require.Empty(t, page.Accounts)

	normalized := normalizeSQLWhitespace(capturedSQL)
	require.Contains(t, normalized, "type = 'oauth'")
	require.NotContains(t, normalized, "type IN ('oauth', 'setup-token')")
	require.NotContains(t, normalized, "type = 'api-key'")
	require.NotContains(t, normalized, "credentials ? 'refresh_token'",
		"reconciliation must be able to find structurally invalid OAuth rows")
	require.Contains(t, normalized, "ORDER BY id ASC")
	require.NoError(t, mock.ExpectationsWereMet())
}

type captureQuerySQL struct {
	db       *sql.DB
	captured *string
	args     *[]any
}

func (c captureQuerySQL) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return c.db.ExecContext(ctx, query, args...)
}

func (c captureQuerySQL) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if c.captured != nil {
		*c.captured = query
	}
	if c.args != nil {
		*c.args = append([]any(nil), args...)
	}
	return c.db.QueryContext(ctx, query, args...)
}

func normalizeSQLWhitespace(sql string) string {
	return strings.Join(regexp.MustCompile(`\s+`).Split(strings.TrimSpace(sql), -1), " ")
}

type rowsAffectedResult int64

func (r rowsAffectedResult) LastInsertId() (int64, error) { return 0, nil }
func (r rowsAffectedResult) RowsAffected() (int64, error) { return int64(r), nil }

type recordingSQLExecutor struct {
	result      sql.Result
	err         error
	afterExec   func()
	execQueries []string
	execArgs    [][]any
}

func (e *recordingSQLExecutor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	e.execQueries = append(e.execQueries, query)
	e.execArgs = append(e.execArgs, append([]any(nil), args...))
	if e.err != nil {
		return nil, e.err
	}
	if e.afterExec != nil {
		e.afterExec()
	}
	return e.result, nil
}

func (e *recordingSQLExecutor) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return nil, sql.ErrNoRows
}
