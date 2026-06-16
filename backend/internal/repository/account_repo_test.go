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
	REDACTEDREDACTED,
			Extra:       map[string]any{REDACTED,
			Status:      service.StatusActive,
			Schedulable: true,
	REDACTED)
REDACTED

	got, err := repo.accountsToService(context.Background(), accounts)
REDACTED
	require.Len(t, got, len(accounts))
REDACTED

func newParameterLimitAccountRepo(t *testing.T) *accountRepository {
REDACTED

	registerParameterLimitTestDriverOnce.Do(func() {
		sql.Register(parameterLimitTestDriverName, parameterLimitDriver{REDACTED)
REDACTED)

	db, err := sql.Open(parameterLimitTestDriverName, "")
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)

	drv := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(drv))
	t.Cleanup(func() { _ = client.Close() REDACTED)

	return newAccountRepositoryWithSQL(client, nil, nil)
REDACTED

type parameterLimitDriver struct{REDACTED

func (parameterLimitDriver) Open(string) (driver.Conn, error) {
	return parameterLimitConn{REDACTED, nil
REDACTED

type parameterLimitConn struct{REDACTED

func (parameterLimitConn) Prepare(query string) (driver.Stmt, error) {
	return parameterLimitStmt{query: queryREDACTED, nil
REDACTED

func (parameterLimitConn) Close() error {
	return nil
REDACTED

func (parameterLimitConn) Begin() (driver.Tx, error) {
	return parameterLimitTx{REDACTED, nil
REDACTED

func (parameterLimitConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return queryWithParameterLimit(query, args)
REDACTED

type parameterLimitStmt struct {
	query string
REDACTED

func (s parameterLimitStmt) Close() error {
	return nil
REDACTED

func (s parameterLimitStmt) NumInput() int {
	return -1
REDACTED

func (s parameterLimitStmt) Exec(args []driver.Value) (driver.Result, error) {
	return driver.RowsAffected(0), parameterLimitError(len(args))
REDACTED

func (s parameterLimitStmt) Query(args []driver.Value) (driver.Rows, error) {
	namedArgs := make([]driver.NamedValue, len(args))
	for i, arg := range args {
		namedArgs[i] = driver.NamedValue{Ordinal: i + 1, Value: argREDACTED
REDACTED
	return queryWithParameterLimit(s.query, namedArgs)
REDACTED

type parameterLimitTx struct{REDACTED

func (parameterLimitTx) Commit() error {
	return nil
REDACTED

func (parameterLimitTx) Rollback() error {
	return nil
REDACTED

func queryWithParameterLimit(query string, args []driver.NamedValue) (driver.Rows, error) {
	if err := parameterLimitError(len(args)); err != nil {
		return nil, err
REDACTED
	return parameterLimitRows{columns: columnsForParameterLimitQuery(query)REDACTED, nil
REDACTED

func parameterLimitError(paramCount int) error {
	if paramCount <= 65535 {
		return nil
REDACTED
	return fmt.Errorf("pq: got %d parameters but PostgreSQL only supports 65535 parameters", paramCount)
REDACTED

func columnsForParameterLimitQuery(query string) []string {
	if query == "" {
		return nil
REDACTED
	return []string{"account_id", "group_id", "priority", "created_at"REDACTED
REDACTED

type parameterLimitRows struct {
	columns []string
REDACTED

func (r parameterLimitRows) Columns() []string {
	return r.columns
REDACTED

func (parameterLimitRows) Close() error {
	return nil
REDACTED

func (parameterLimitRows) Next([]driver.Value) error {
	return io.EOF
REDACTED
