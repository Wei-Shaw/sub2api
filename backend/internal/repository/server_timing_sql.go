package repository

import (
	"context"
	"database/sql/driver"
	"errors"
	"io"
	"reflect"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
)

type serverTimingConnector struct {
	base driver.Connector
REDACTED

func newServerTimingConnector(base driver.Connector) driver.Connector {
	return &serverTimingConnector{base: baseREDACTED
REDACTED

func (c *serverTimingConnector) Connect(ctx context.Context) (driver.Conn, error) {
	startedAt := time.Now()
	conn, err := c.base.Connect(ctx)
	servertiming.RecordInterval(ctx, servertiming.MetricDatabase, startedAt, time.Now())
	if err != nil {
		return nil, err
REDACTED
	return &serverTimingConn{Conn: connREDACTED, nil
REDACTED

func (c *serverTimingConnector) Driver() driver.Driver {
	return c.base.Driver()
REDACTED

type serverTimingConn struct {
	driver.Conn
REDACTED

func (c *serverTimingConn) Prepare(query string) (driver.Stmt, error) {
	stmt, err := c.Conn.Prepare(query)
	if err != nil {
		return nil, err
REDACTED
	return &serverTimingStmt{Stmt: stmtREDACTED, nil
REDACTED

func (c *serverTimingConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	startedAt := time.Now()
	var (
		stmt driver.Stmt
		err  error
	)
	if preparer, ok := c.Conn.(driver.ConnPrepareContext); ok {
		stmt, err = preparer.PrepareContext(ctx, query)
REDACTED else {
		stmt, err = c.Conn.Prepare(query)
REDACTED
	servertiming.Record(ctx, servertiming.MetricDatabase, startedAt, time.Now(), 1)
	if err != nil {
		return nil, err
REDACTED
	return &serverTimingStmt{Stmt: stmtREDACTED, nil
REDACTED

func (c *serverTimingConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	execer, ok := c.Conn.(driver.ExecerContext)
	if !ok {
		return nil, driver.ErrSkip
REDACTED
	startedAt := time.Now()
	result, err := execer.ExecContext(ctx, query, args)
	servertiming.Record(ctx, servertiming.MetricDatabase, startedAt, time.Now(), 1)
	return result, err
REDACTED

func (c *serverTimingConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	queryer, ok := c.Conn.(driver.QueryerContext)
	if !ok {
		return nil, driver.ErrSkip
REDACTED
	startedAt := time.Now()
	rows, err := queryer.QueryContext(ctx, query, args)
	servertiming.Record(ctx, servertiming.MetricDatabase, startedAt, time.Now(), 1)
	if err != nil || rows == nil {
		return rows, err
REDACTED
	return newServerTimingRows(ctx, rows), nil
REDACTED

func (c *serverTimingConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	startedAt := time.Now()
	var (
		tx  driver.Tx
		err error
	)
	if beginner, ok := c.Conn.(driver.ConnBeginTx); ok {
		tx, err = beginner.BeginTx(ctx, opts)
REDACTED else {
		if opts.Isolation != driver.IsolationLevel(0) {
			return nil, errors.New("driver does not support non-default isolation")
	REDACTED
		if opts.ReadOnly {
			return nil, errors.New("driver does not support read-only transactions")
	REDACTED
		tx, err = c.Conn.Begin()
REDACTED
	servertiming.RecordInterval(ctx, servertiming.MetricDatabase, startedAt, time.Now())
	if err != nil || tx == nil {
		return tx, err
REDACTED
	return &serverTimingTx{Tx: tx, ctx: ctxREDACTED, nil
REDACTED

func (c *serverTimingConn) Ping(ctx context.Context) error {
	if pinger, ok := c.Conn.(driver.Pinger); ok {
		startedAt := time.Now()
		err := pinger.Ping(ctx)
		servertiming.RecordInterval(ctx, servertiming.MetricDatabase, startedAt, time.Now())
		return err
REDACTED
	return nil
REDACTED

func (c *serverTimingConn) ResetSession(ctx context.Context) error {
	if resetter, ok := c.Conn.(driver.SessionResetter); ok {
		startedAt := time.Now()
		err := resetter.ResetSession(ctx)
		servertiming.RecordInterval(ctx, servertiming.MetricDatabase, startedAt, time.Now())
		return err
REDACTED
	return nil
REDACTED

func (c *serverTimingConn) IsValid() bool {
	if validator, ok := c.Conn.(driver.Validator); ok {
		return validator.IsValid()
REDACTED
	return true
REDACTED

func (c *serverTimingConn) CheckNamedValue(value *driver.NamedValue) error {
	if checker, ok := c.Conn.(driver.NamedValueChecker); ok {
		return checker.CheckNamedValue(value)
REDACTED
	return driver.ErrSkip
REDACTED

type serverTimingStmt struct {
	driver.Stmt
REDACTED

func (s *serverTimingStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	startedAt := time.Now()
	var (
		result driver.Result
		err    error
	)
	if execer, ok := s.Stmt.(driver.StmtExecContext); ok {
		result, err = execer.ExecContext(ctx, args)
REDACTED else {
		var values []driver.Value
		values, err = namedValues(args)
		if err == nil {
			result, err = s.Stmt.Exec(values)
	REDACTED
REDACTED
	servertiming.Record(ctx, servertiming.MetricDatabase, startedAt, time.Now(), 1)
	return result, err
REDACTED

func (s *serverTimingStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	startedAt := time.Now()
	var (
		rows driver.Rows
		err  error
	)
	if queryer, ok := s.Stmt.(driver.StmtQueryContext); ok {
		rows, err = queryer.QueryContext(ctx, args)
REDACTED else {
		var values []driver.Value
		values, err = namedValues(args)
		if err == nil {
			rows, err = s.Stmt.Query(values)
	REDACTED
REDACTED
	servertiming.Record(ctx, servertiming.MetricDatabase, startedAt, time.Now(), 1)
	if err != nil || rows == nil {
		return rows, err
REDACTED
	return newServerTimingRows(ctx, rows), nil
REDACTED

func (s *serverTimingStmt) CheckNamedValue(value *driver.NamedValue) error {
	if checker, ok := s.Stmt.(driver.NamedValueChecker); ok {
		return checker.CheckNamedValue(value)
REDACTED
	return driver.ErrSkip
REDACTED

func (s *serverTimingStmt) ColumnConverter(index int) driver.ValueConverter {
	if converter, ok := s.Stmt.(driver.ColumnConverter); ok {
		return converter.ColumnConverter(index)
REDACTED
	return driver.DefaultParameterConverter
REDACTED

func namedValues(args []driver.NamedValue) ([]driver.Value, error) {
	values := make([]driver.Value, len(args))
	for i, arg := range args {
		if arg.Name != "" {
			return nil, errors.New("named parameters are not supported")
	REDACTED
		values[i] = arg.Value
REDACTED
	return values, nil
REDACTED

type serverTimingRows struct {
	driver.Rows
	ctx context.Context
REDACTED

func newServerTimingRows(ctx context.Context, rows driver.Rows) *serverTimingRows {
	return &serverTimingRows{Rows: rows, ctx: ctxREDACTED
REDACTED

func (r *serverTimingRows) Close() error {
	startedAt := time.Now()
	err := r.Rows.Close()
	servertiming.RecordInterval(r.ctx, servertiming.MetricDatabase, startedAt, time.Now())
	return err
REDACTED

func (r *serverTimingRows) Next(dest []driver.Value) error {
	startedAt := time.Now()
	err := r.Rows.Next(dest)
	servertiming.RecordInterval(r.ctx, servertiming.MetricDatabase, startedAt, time.Now())
	return err
REDACTED

func (r *serverTimingRows) HasNextResultSet() bool {
	if rows, ok := r.Rows.(driver.RowsNextResultSet); ok {
		return rows.HasNextResultSet()
REDACTED
	return false
REDACTED

func (r *serverTimingRows) NextResultSet() error {
	rows, ok := r.Rows.(driver.RowsNextResultSet)
	if !ok {
		return io.EOF
REDACTED
	startedAt := time.Now()
	err := rows.NextResultSet()
	servertiming.RecordInterval(r.ctx, servertiming.MetricDatabase, startedAt, time.Now())
	return err
REDACTED

func (r *serverTimingRows) ColumnTypeScanType(index int) reflect.Type {
	if rows, ok := r.Rows.(driver.RowsColumnTypeScanType); ok {
		return rows.ColumnTypeScanType(index)
REDACTED
	return reflect.TypeOf(new(any)).Elem()
REDACTED

func (r *serverTimingRows) ColumnTypeDatabaseTypeName(index int) string {
	if rows, ok := r.Rows.(driver.RowsColumnTypeDatabaseTypeName); ok {
		return rows.ColumnTypeDatabaseTypeName(index)
REDACTED
	return ""
REDACTED

func (r *serverTimingRows) ColumnTypeLength(index int) (int64, bool) {
	if rows, ok := r.Rows.(driver.RowsColumnTypeLength); ok {
		return rows.ColumnTypeLength(index)
REDACTED
	return 0, false
REDACTED

func (r *serverTimingRows) ColumnTypeNullable(index int) (bool, bool) {
	if rows, ok := r.Rows.(driver.RowsColumnTypeNullable); ok {
		return rows.ColumnTypeNullable(index)
REDACTED
	return false, false
REDACTED

func (r *serverTimingRows) ColumnTypePrecisionScale(index int) (int64, int64, bool) {
	if rows, ok := r.Rows.(driver.RowsColumnTypePrecisionScale); ok {
		return rows.ColumnTypePrecisionScale(index)
REDACTED
	return 0, 0, false
REDACTED

type serverTimingTx struct {
	driver.Tx
	ctx context.Context
REDACTED

func (t *serverTimingTx) Commit() error {
	startedAt := time.Now()
	err := t.Tx.Commit()
	servertiming.RecordInterval(t.ctx, servertiming.MetricDatabase, startedAt, time.Now())
	return err
REDACTED

func (t *serverTimingTx) Rollback() error {
	startedAt := time.Now()
	err := t.Tx.Rollback()
	servertiming.RecordInterval(t.ctx, servertiming.MetricDatabase, startedAt, time.Now())
	return err
REDACTED
