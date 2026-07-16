package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"sync"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

const parameterLimitTestDriverName = "sub2api_param_limit_test"

var registerParameterLimitTestDriverOnce sync.Once

func TestAccountsToService_LargeActiveAccountSetDoesNotExceedPostgresParameterLimit(t *testing.T) {
	repo := newParameterLimitAccountRepo(t)

	accounts := make([]*dbent.Account, 0, 65536)
	for i := range 65536 {
		accounts = append(accounts, &dbent.Account{
			ID:          int64(i + 1),
			Name:        "large-active",
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeOAuth,
			Credentials: map[string]any{},
			Extra:       map[string]any{},
			Status:      service.StatusActive,
			Schedulable: true,
		})
	}

	got, err := repo.accountsToService(context.Background(), accounts)
	require.NoError(t, err)
	require.Len(t, got, len(accounts))
}

func newParameterLimitAccountRepo(t *testing.T) *accountRepository {
	t.Helper()

	registerParameterLimitTestDriverOnce.Do(func() {
		sql.Register(parameterLimitTestDriverName, parameterLimitDriver{})
	})

	db, err := sql.Open(parameterLimitTestDriverName, "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	drv := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(drv))
	t.Cleanup(func() { _ = client.Close() })

	return newAccountRepositoryWithSQL(client, nil, nil)
}

type parameterLimitDriver struct{}

func (parameterLimitDriver) Open(string) (driver.Conn, error) {
	return parameterLimitConn{}, nil
}

type parameterLimitConn struct{}

func (parameterLimitConn) Prepare(query string) (driver.Stmt, error) {
	return parameterLimitStmt{query: query}, nil
}

func (parameterLimitConn) Close() error {
	return nil
}

func (parameterLimitConn) Begin() (driver.Tx, error) {
	return parameterLimitTx{}, nil
}

func (parameterLimitConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return queryWithParameterLimit(query, args)
}

type parameterLimitStmt struct {
	query string
}

func (s parameterLimitStmt) Close() error {
	return nil
}

func (s parameterLimitStmt) NumInput() int {
	return -1
}

func (s parameterLimitStmt) Exec(args []driver.Value) (driver.Result, error) {
	return driver.RowsAffected(0), parameterLimitError(len(args))
}

func (s parameterLimitStmt) Query(args []driver.Value) (driver.Rows, error) {
	namedArgs := make([]driver.NamedValue, len(args))
	for i, arg := range args {
		namedArgs[i] = driver.NamedValue{Ordinal: i + 1, Value: arg}
	}
	return queryWithParameterLimit(s.query, namedArgs)
}

type parameterLimitTx struct{}

func (parameterLimitTx) Commit() error {
	return nil
}

func (parameterLimitTx) Rollback() error {
	return nil
}

func queryWithParameterLimit(query string, args []driver.NamedValue) (driver.Rows, error) {
	if err := parameterLimitError(len(args)); err != nil {
		return nil, err
	}
	return parameterLimitRows{columns: columnsForParameterLimitQuery(query)}, nil
}

func parameterLimitError(paramCount int) error {
	if paramCount <= 65535 {
		return nil
	}
	return fmt.Errorf("pq: got %d parameters but PostgreSQL only supports 65535 parameters", paramCount)
}

func columnsForParameterLimitQuery(query string) []string {
	if query == "" {
		return nil
	}
	return []string{"account_id", "group_id", "priority", "created_at"}
}

type parameterLimitRows struct {
	columns []string
}

func (r parameterLimitRows) Columns() []string {
	return r.columns
}

func (parameterLimitRows) Close() error {
	return nil
}

func (parameterLimitRows) Next([]driver.Value) error {
	return io.EOF
}
