package repository

import (
	"context"
	"database/sql/driver"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
)

const fakeDriverDelay = 2 * time.Millisecond

type timingFakeDriver struct{REDACTED

func (timingFakeDriver) Open(string) (driver.Conn, error) { return newTimingFakeConn(), nil REDACTED

type timingFakeConnector struct {
	conn driver.Conn
REDACTED

func (c timingFakeConnector) Connect(context.Context) (driver.Conn, error) {
	time.Sleep(fakeDriverDelay)
	return c.conn, nil
REDACTED

func (timingFakeConnector) Driver() driver.Driver { return timingFakeDriver{REDACTED REDACTED

type timingFakeConn struct{REDACTED

func newTimingFakeConn() *timingFakeConn { return &timingFakeConn{REDACTED REDACTED

func (c *timingFakeConn) Prepare(string) (driver.Stmt, error) {
	time.Sleep(fakeDriverDelay)
	return &timingFakeStmt{REDACTED, nil
REDACTED

func (c *timingFakeConn) PrepareContext(context.Context, string) (driver.Stmt, error) {
	time.Sleep(fakeDriverDelay)
	return &timingFakeStmt{REDACTED, nil
REDACTED

func (c *timingFakeConn) Close() error { return nil REDACTED

func (c *timingFakeConn) Begin() (driver.Tx, error) {
	time.Sleep(fakeDriverDelay)
	return &timingFakeTx{REDACTED, nil
REDACTED

func (c *timingFakeConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	time.Sleep(fakeDriverDelay)
	return &timingFakeTx{REDACTED, nil
REDACTED

func (c *timingFakeConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	time.Sleep(fakeDriverDelay)
	return driver.RowsAffected(1), nil
REDACTED

func (c *timingFakeConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	time.Sleep(fakeDriverDelay)
	return &timingFakeRows{values: [][]driver.Value{{"value"REDACTEDREDACTEDREDACTED, nil
REDACTED

func (c *timingFakeConn) Ping(context.Context) error {
	time.Sleep(fakeDriverDelay)
	return nil
REDACTED

func (c *timingFakeConn) ResetSession(context.Context) error {
	time.Sleep(fakeDriverDelay)
	return nil
REDACTED

type timingFakeStmt struct{REDACTED

func (s *timingFakeStmt) Close() error  { return nil REDACTED
func (s *timingFakeStmt) NumInput() int { return -1 REDACTED

func (s *timingFakeStmt) Exec([]driver.Value) (driver.Result, error) {
	time.Sleep(fakeDriverDelay)
	return driver.RowsAffected(1), nil
REDACTED

func (s *timingFakeStmt) Query([]driver.Value) (driver.Rows, error) {
	time.Sleep(fakeDriverDelay)
	return &timingFakeRows{values: [][]driver.Value{{"value"REDACTEDREDACTEDREDACTED, nil
REDACTED

func (s *timingFakeStmt) ExecContext(context.Context, []driver.NamedValue) (driver.Result, error) {
	time.Sleep(fakeDriverDelay)
	return driver.RowsAffected(1), nil
REDACTED

func (s *timingFakeStmt) QueryContext(context.Context, []driver.NamedValue) (driver.Rows, error) {
	time.Sleep(fakeDriverDelay)
	return &timingFakeRows{values: [][]driver.Value{{"value"REDACTEDREDACTEDREDACTED, nil
REDACTED

type timingFakeRows struct {
	values [][]driver.Value
	index  int
REDACTED

func (r *timingFakeRows) Columns() []string { return []string{"value"REDACTED REDACTED

func (r *timingFakeRows) Close() error {
	time.Sleep(fakeDriverDelay)
	return nil
REDACTED

func (r *timingFakeRows) Next(dest []driver.Value) error {
	time.Sleep(fakeDriverDelay)
	if r.index >= len(r.values) {
		return io.EOF
REDACTED
	copy(dest, r.values[r.index])
	r.index++
	return nil
REDACTED

type timingFakeTx struct{REDACTED

func (t *timingFakeTx) Commit() error {
	time.Sleep(fakeDriverDelay)
	return nil
REDACTED

func (t *timingFakeTx) Rollback() error {
	time.Sleep(fakeDriverDelay)
	return nil
REDACTED

func metricDuration(t *testing.T, header, metric string) float64 {
REDACTED
	re := regexp.MustCompile(`(?:^|, )` + regexp.QuoteMeta(metric) + `;dur=([0-9]+(?:\.[0-9]+)?)`)
	match := re.FindStringSubmatch(header)
	if len(match) != 2 {
		t.Fatalf("metric %q missing from header %q", metric, header)
REDACTED
	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		t.Fatalf("parse %s duration: %v", metric, err)
REDACTED
	return value
REDACTED

func TestServerTimingConnectorRecordsDriverCallsWithoutRowLifetime(t *testing.T) {
	startedAt := time.Now()
	collector := servertiming.New(startedAt)
	ctx := servertiming.WithCollector(context.Background(), collector)

	wrapped := newServerTimingConnector(timingFakeConnector{conn: newTimingFakeConn()REDACTED)
	rawConn, err := wrapped.Connect(ctx)
	if err != nil {
		t.Fatal(err)
REDACTED
	conn, ok := rawConn.(*serverTimingConn)
	if !ok {
		t.Fatalf("Connect() returned %T, want *serverTimingConn", rawConn)
REDACTED

	if _, err := conn.ExecContext(ctx, "sensitive update", nil); err != nil {
		t.Fatal(err)
REDACTED
	rows, err := conn.QueryContext(ctx, "sensitive select", nil)
	if err != nil {
		t.Fatal(err)
REDACTED
	values := make([]driver.Value, 1)
	if err := rows.Next(values); err != nil {
		t.Fatal(err)
REDACTED

	// Application work between row reads must remain app time.
	time.Sleep(30 * time.Millisecond)
	if err := rows.Next(values); err != io.EOF {
		t.Fatalf("rows.Next() = %v, want EOF", err)
REDACTED
	if err := rows.Close(); err != nil {
		t.Fatal(err)
REDACTED

	header := collector.HeaderValue(time.Now(), "bypass")
	if !strings.Contains(header, `queries=2`) {
		t.Fatalf("header %q does not report two SQL operations", header)
REDACTED
	if strings.Contains(header, "sensitive") {
		t.Fatalf("SQL text leaked into header: %q", header)
REDACTED
	if app, db := metricDuration(t, header, "app"), metricDuration(t, header, "db"); app <= db {
		t.Fatalf("row processing gap was counted as DB time: app=%.1fms db=%.1fms header=%q", app, db, header)
REDACTED
REDACTED

func TestServerTimingPreparedStatementsAndTransactions(t *testing.T) {
	collector := servertiming.New(time.Now())
	ctx := servertiming.WithCollector(context.Background(), collector)
	conn := &serverTimingConn{Conn: newTimingFakeConn()REDACTED

	stmt, err := conn.PrepareContext(ctx, "prepare sensitive statement")
	if err != nil {
		t.Fatal(err)
REDACTED
	timedStmt, ok := stmt.(*serverTimingStmt)
	if !ok {
		t.Fatalf("PrepareContext() returned %T, want *serverTimingStmt", stmt)
REDACTED
	if _, err := timedStmt.ExecContext(ctx, nil); err != nil {
		t.Fatal(err)
REDACTED
	rows, err := timedStmt.QueryContext(ctx, nil)
	if err != nil {
		t.Fatal(err)
REDACTED
	if err := rows.Close(); err != nil {
		t.Fatal(err)
REDACTED

	tx, err := conn.BeginTx(ctx, driver.TxOptions{REDACTED)
	if err != nil {
		t.Fatal(err)
REDACTED
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
REDACTED
	if err := conn.Ping(ctx); err != nil {
		t.Fatal(err)
REDACTED
	if err := conn.ResetSession(ctx); err != nil {
		t.Fatal(err)
REDACTED

	header := collector.HeaderValue(time.Now(), "bypass")
	if !strings.Contains(header, `queries=3`) {
		t.Fatalf("header %q does not report prepare, exec, and query operations", header)
REDACTED
	if metricDuration(t, header, "db") <= 0 {
		t.Fatalf("DB duration was not recorded: %q", header)
REDACTED
REDACTED

func TestNamedValuesRejectNamedParameters(t *testing.T) {
	if _, err := namedValues([]driver.NamedValue{{Name: "secret", Value: 1REDACTEDREDACTED); err == nil {
		t.Fatal("namedValues accepted a named parameter")
REDACTED
	values, err := namedValues([]driver.NamedValue{{Ordinal: 1, Value: "value"REDACTEDREDACTED)
	if err != nil {
		t.Fatal(err)
REDACTED
	if len(values) != 1 || values[0] != "value" {
		t.Fatalf("namedValues() = %#v", values)
REDACTED
REDACTED
