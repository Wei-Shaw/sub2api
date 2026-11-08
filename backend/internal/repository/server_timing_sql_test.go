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

type timingFakeDriver struct{}

func (timingFakeDriver) Open(string) (driver.Conn, error) { return newTimingFakeConn(), nil }

type timingFakeConnector struct {
	conn driver.Conn
}

func (c timingFakeConnector) Connect(context.Context) (driver.Conn, error) {
	time.Sleep(fakeDriverDelay)
	return c.conn, nil
}

func (timingFakeConnector) Driver() driver.Driver { return timingFakeDriver{} }

type timingFakeConn struct{}

func newTimingFakeConn() *timingFakeConn { return &timingFakeConn{} }

func (c *timingFakeConn) Prepare(string) (driver.Stmt, error) {
	time.Sleep(fakeDriverDelay)
	return &timingFakeStmt{}, nil
}

func (c *timingFakeConn) PrepareContext(context.Context, string) (driver.Stmt, error) {
	time.Sleep(fakeDriverDelay)
	return &timingFakeStmt{}, nil
}

func (c *timingFakeConn) Close() error { return nil }

func (c *timingFakeConn) Begin() (driver.Tx, error) {
	time.Sleep(fakeDriverDelay)
	return &timingFakeTx{}, nil
}

func (c *timingFakeConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	time.Sleep(fakeDriverDelay)
	return &timingFakeTx{}, nil
}

func (c *timingFakeConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	time.Sleep(fakeDriverDelay)
	return driver.RowsAffected(1), nil
}

func (c *timingFakeConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	time.Sleep(fakeDriverDelay)
	return &timingFakeRows{values: [][]driver.Value{{"value"}}}, nil
}

func (c *timingFakeConn) Ping(context.Context) error {
	time.Sleep(fakeDriverDelay)
	return nil
}

func (c *timingFakeConn) ResetSession(context.Context) error {
	time.Sleep(fakeDriverDelay)
	return nil
}

type timingFakeStmt struct{}

func (s *timingFakeStmt) Close() error  { return nil }
func (s *timingFakeStmt) NumInput() int { return -1 }

func (s *timingFakeStmt) Exec([]driver.Value) (driver.Result, error) {
	time.Sleep(fakeDriverDelay)
	return driver.RowsAffected(1), nil
}

func (s *timingFakeStmt) Query([]driver.Value) (driver.Rows, error) {
	time.Sleep(fakeDriverDelay)
	return &timingFakeRows{values: [][]driver.Value{{"value"}}}, nil
}

func (s *timingFakeStmt) ExecContext(context.Context, []driver.NamedValue) (driver.Result, error) {
	time.Sleep(fakeDriverDelay)
	return driver.RowsAffected(1), nil
}

func (s *timingFakeStmt) QueryContext(context.Context, []driver.NamedValue) (driver.Rows, error) {
	time.Sleep(fakeDriverDelay)
	return &timingFakeRows{values: [][]driver.Value{{"value"}}}, nil
}

type timingFakeRows struct {
	values [][]driver.Value
	index  int
}

func (r *timingFakeRows) Columns() []string { return []string{"value"} }

func (r *timingFakeRows) Close() error {
	time.Sleep(fakeDriverDelay)
	return nil
}

func (r *timingFakeRows) Next(dest []driver.Value) error {
	time.Sleep(fakeDriverDelay)
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

type timingFakeTx struct{}

func (t *timingFakeTx) Commit() error {
	time.Sleep(fakeDriverDelay)
	return nil
}

func (t *timingFakeTx) Rollback() error {
	time.Sleep(fakeDriverDelay)
	return nil
}

func metricDuration(t *testing.T, header, metric string) float64 {
	t.Helper()
	re := regexp.MustCompile(`(?:^|, )` + regexp.QuoteMeta(metric) + `;dur=([0-9]+(?:\.[0-9]+)?)`)
	match := re.FindStringSubmatch(header)
	if len(match) != 2 {
		t.Fatalf("metric %q missing from header %q", metric, header)
	}
	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		t.Fatalf("parse %s duration: %v", metric, err)
	}
	return value
}

func TestServerTimingConnectorRecordsDriverCallsWithoutRowLifetime(t *testing.T) {
	startedAt := time.Now()
	collector := servertiming.New(startedAt)
	ctx := servertiming.WithCollector(context.Background(), collector)

	wrapped := newServerTimingConnector(timingFakeConnector{conn: newTimingFakeConn()})
	rawConn, err := wrapped.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	conn, ok := rawConn.(*serverTimingConn)
	if !ok {
		t.Fatalf("Connect() returned %T, want *serverTimingConn", rawConn)
	}

	if _, err := conn.ExecContext(ctx, "sensitive update", nil); err != nil {
		t.Fatal(err)
	}
	rows, err := conn.QueryContext(ctx, "sensitive select", nil)
	if err != nil {
		t.Fatal(err)
	}
	values := make([]driver.Value, 1)
	if err := rows.Next(values); err != nil {
		t.Fatal(err)
	}

	// Application work between row reads must remain app time.
	time.Sleep(30 * time.Millisecond)
	if err := rows.Next(values); err != io.EOF {
		t.Fatalf("rows.Next() = %v, want EOF", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}

	header := collector.HeaderValue(time.Now(), "bypass")
	if !strings.Contains(header, `queries=2`) {
		t.Fatalf("header %q does not report two SQL operations", header)
	}
	if strings.Contains(header, "sensitive") {
		t.Fatalf("SQL text leaked into header: %q", header)
	}
	if app, db := metricDuration(t, header, "app"), metricDuration(t, header, "db"); app <= db {
		t.Fatalf("row processing gap was counted as DB time: app=%.1fms db=%.1fms header=%q", app, db, header)
	}
}

func TestServerTimingPreparedStatementsAndTransactions(t *testing.T) {
	collector := servertiming.New(time.Now())
	ctx := servertiming.WithCollector(context.Background(), collector)
	conn := &serverTimingConn{Conn: newTimingFakeConn()}

	stmt, err := conn.PrepareContext(ctx, "prepare sensitive statement")
	if err != nil {
		t.Fatal(err)
	}
	timedStmt, ok := stmt.(*serverTimingStmt)
	if !ok {
		t.Fatalf("PrepareContext() returned %T, want *serverTimingStmt", stmt)
	}
	if _, err := timedStmt.ExecContext(ctx, nil); err != nil {
		t.Fatal(err)
	}
	rows, err := timedStmt.QueryContext(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}

	tx, err := conn.BeginTx(ctx, driver.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := conn.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	if err := conn.ResetSession(ctx); err != nil {
		t.Fatal(err)
	}

	header := collector.HeaderValue(time.Now(), "bypass")
	if !strings.Contains(header, `queries=3`) {
		t.Fatalf("header %q does not report prepare, exec, and query operations", header)
	}
	if metricDuration(t, header, "db") <= 0 {
		t.Fatalf("DB duration was not recorded: %q", header)
	}
}

func TestNamedValuesRejectNamedParameters(t *testing.T) {
	if _, err := namedValues([]driver.NamedValue{{Name: "secret", Value: 1}}); err == nil {
		t.Fatal("namedValues accepted a named parameter")
	}
	values, err := namedValues([]driver.NamedValue{{Ordinal: 1, Value: "value"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0] != "value" {
		t.Fatalf("namedValues() = %#v", values)
	}
}
