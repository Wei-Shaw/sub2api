package driver

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// DriverName is the canonical name registered with database/sql.
const DriverName = "plugin-grpc"

// txFinishTimeout caps how long Commit/Rollback may run after BeginTx's
// caller ctx has been cancelled. database/sql does not pass a ctx to
// Tx.Commit / Tx.Rollback, so we derive one from the BeginTx ctx (without
// cancel propagation) and add this timeout so a stuck core RPC cannot leak
// goroutines forever.
const txFinishTimeout = 30 * time.Second

var registered atomic.Bool

// Register installs a database/sql driver named DriverName backed by the
// provided SQLProxyClient. Subsequent calls reconfigure the underlying
// client without re-registering, so the SDK can call Register exactly once
// per process and still swap out the gRPC channel during reconnects.
//
// After Register, plugins may open a *sql.DB with sql.Open(DriverName, "").
// The DSN is ignored - the driver always proxies to the supplied client.
func Register(client pb.SQLProxyClient) {
	getActiveClient().set(client)
	if registered.CompareAndSwap(false, true) {
		sql.Register(DriverName, &grpcDriver{})
	}
}

// activeClient is the indirection that lets Register swap out the underlying
// gRPC client without re-registering with database/sql.
type activeClient struct {
	v atomic.Value
}

var globalActive activeClient

func getActiveClient() *activeClient { return &globalActive }

func (a *activeClient) set(c pb.SQLProxyClient) { a.v.Store(clientHolder{c}) }

func (a *activeClient) get() pb.SQLProxyClient {
	h, ok := a.v.Load().(clientHolder)
	if !ok {
		return nil
	}
	return h.c
}

// clientHolder wraps the interface value because atomic.Value requires a
// concrete type that never changes between Stores.
type clientHolder struct{ c pb.SQLProxyClient }

// grpcDriver is the database/sql driver entry point.
type grpcDriver struct{}

// Open returns a new connection. Because the underlying transport is gRPC,
// every "connection" shares the same channel; database/sql still pools the
// returned Conns and invokes Close when one is evicted.
func (d *grpcDriver) Open(_ string) (driver.Conn, error) {
	c := getActiveClient().get()
	if c == nil {
		return nil, fmt.Errorf("plugin-sdk: SQL driver used before Register")
	}
	return &grpcConn{client: c}, nil
}

// grpcConn implements driver.Conn plus the *Context interfaces so
// database/sql can pass a context.Context all the way down. Transaction
// state lives on the conn because database/sql pins every Exec/Query of
// an open *sql.Tx to the same driver.Conn, so the routing decision (tx
// vs non-tx) only needs one state field per conn.
type grpcConn struct {
	client pb.SQLProxyClient
	closed atomic.Bool

	// txMu guards txID. database/sql serialises calls on a single conn
	// in practice, but holding an explicit lock keeps the contract
	// obvious for any future async wrapper.
	txMu sync.Mutex
	txID string
}

// activeTxID returns the in-flight transaction id, or "" when no tx is
// open. Centralising the read keeps the lock scope tight.
func (c *grpcConn) activeTxID() string {
	c.txMu.Lock()
	defer c.txMu.Unlock()
	return c.txID
}

// setTxID stores the BeginTx id, or clears it after Commit/Rollback.
func (c *grpcConn) setTxID(id string) {
	c.txMu.Lock()
	c.txID = id
	c.txMu.Unlock()
}

// Prepare returns a statement that defers its real work until Exec/Query.
// We do not have a server-side prepare, so the implementation simply
// remembers the query string.
func (c *grpcConn) Prepare(query string) (driver.Stmt, error) {
	return c.PrepareContext(context.Background(), query)
}

// PrepareContext satisfies driver.ConnPrepareContext.
func (c *grpcConn) PrepareContext(_ context.Context, query string) (driver.Stmt, error) {
	if c.closed.Load() {
		return nil, driver.ErrBadConn
	}
	return &grpcStmt{conn: c, query: query}, nil
}

// Close releases the local Conn handle. The shared gRPC channel is owned by
// the SDK, so we never tear it down here.
func (c *grpcConn) Close() error {
	c.closed.Store(true)
	return nil
}

// IsValid satisfies driver.Validator, letting database/sql's connection pool
// discard connections that have been closed without an expensive round-trip.
func (c *grpcConn) IsValid() bool {
	return !c.closed.Load()
}

// ResetSession satisfies driver.SessionResetter, letting database/sql clean
// up stale transaction state before returning a connection to the pool.
func (c *grpcConn) ResetSession(_ context.Context) error {
	if c.closed.Load() {
		return driver.ErrBadConn
	}
	c.txMu.Lock()
	orphanedTx := c.txID
	c.txID = ""
	c.txMu.Unlock()
	if orphanedTx != "" {
		ctx, cancel := context.WithTimeout(context.Background(), txFinishTimeout)
		defer cancel()
		_, _ = c.client.RollbackTx(ctx, &pb.TxIDRequest{TxId: orphanedTx})
	}
	return nil
}

// Begin is the legacy API; we always go through BeginTx.
func (c *grpcConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

// BeginTx starts a transaction on the core via gRPC and returns a Tx that
// remembers the txID for subsequent TxQuery/TxExec calls. The transaction
// id is also stored on grpcConn so QueryContext/ExecContext can route
// every in-flight statement through TxQuery/TxExec.
func (c *grpcConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if c.closed.Load() {
		return nil, driver.ErrBadConn
	}
	if existing := c.activeTxID(); existing != "" {
		// database/sql does not allow nested transactions; protect the
		// invariant explicitly so a buggy caller produces a clear error
		// instead of silently dropping the previous txID.
		return nil, fmt.Errorf("plugin-sdk: BeginTx called with active tx %s", existing)
	}
	isoLevel, err := isolationLevelString(sql.IsolationLevel(opts.Isolation))
	if err != nil {
		return nil, err
	}
	resp, err := c.client.BeginTx(ctx, &pb.BeginTxRequest{
		IsolationLevel: isoLevel,
		ReadOnly:       opts.ReadOnly,
	})
	if err != nil {
		return nil, err
	}
	txID := resp.GetTxId()
	c.setTxID(txID)
	// finishCtx is the parent ctx used by Commit/Rollback. WithoutCancel
	// drops the caller's cancellation so the final RPC does not abort
	// just because the request handler that called BeginTx already
	// returned (database/sql often calls Commit/Rollback after the user
	// callback completes); the per-call timeout is layered on later.
	finishCtx := context.WithoutCancel(ctx)
	return &grpcTx{conn: c, client: c.client, txID: txID, finishCtx: finishCtx}, nil
}

// Ping is a cheap RPC so database/sql can check liveness. We approximate
// it with a no-op SELECT 1 on the gRPC channel because the SQLProxy
// service does not (yet) expose a dedicated Ping RPC.
//
// TODO(plugin-sdk): add SQLProxy.Ping to sdk.proto and route this through
// it so health checks do not depend on the core executing a real query.
func (c *grpcConn) Ping(ctx context.Context) error {
	if c.closed.Load() {
		return driver.ErrBadConn
	}
	_, err := c.client.Query(ctx, &pb.SQLRequest{Query: "SELECT 1"})
	return err
}

// QueryContext satisfies driver.QueryerContext. When a transaction is
// active on this conn the call is routed through SQLProxy.TxQuery so the
// statement actually participates in the transaction; otherwise it falls
// through to the non-tx Query RPC. Both branches share the same arg
// encoding and row materialisation.
func (c *grpcConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if c.closed.Load() {
		return nil, driver.ErrBadConn
	}
	pbArgs, err := namedValuesToProto(args)
	if err != nil {
		return nil, err
	}
	resp, err := c.dispatchQuery(ctx, query, pbArgs)
	if err != nil {
		return nil, err
	}
	return newRows(resp), nil
}

// ExecContext satisfies driver.ExecerContext. Mirrors QueryContext: it
// routes to TxExec when a transaction is open and to Exec otherwise.
func (c *grpcConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if c.closed.Load() {
		return nil, driver.ErrBadConn
	}
	pbArgs, err := namedValuesToProto(args)
	if err != nil {
		return nil, err
	}
	resp, err := c.dispatchExec(ctx, query, pbArgs)
	if err != nil {
		return nil, err
	}
	return &execResult{rowsAffected: resp.GetRowsAffected(), lastID: resp.GetLastInsertId()}, nil
}

// dispatchQuery is the single routing point that decides between the
// transactional and non-transactional Query RPCs. Centralising the
// branch keeps the state-machine invariant (txID empty / no tx) in one
// place and lets QueryContext stay protocol-only.
func (c *grpcConn) dispatchQuery(ctx context.Context, query string, args []*pb.SQLValue) (*pb.SQLResponse, error) {
	if txID := c.activeTxID(); txID != "" {
		return c.client.TxQuery(ctx, &pb.TxSQLRequest{TxId: txID, Query: query, Args: args})
	}
	return c.client.Query(ctx, &pb.SQLRequest{Query: query, Args: args})
}

// dispatchExec is the Exec counterpart of dispatchQuery.
func (c *grpcConn) dispatchExec(ctx context.Context, query string, args []*pb.SQLValue) (*pb.ExecResponse, error) {
	if txID := c.activeTxID(); txID != "" {
		return c.client.TxExec(ctx, &pb.TxSQLRequest{TxId: txID, Query: query, Args: args})
	}
	return c.client.Exec(ctx, &pb.SQLRequest{Query: query, Args: args})
}

// grpcStmt is a thin wrapper that re-routes Exec/Query through the conn.
type grpcStmt struct {
	conn  *grpcConn
	query string
}

func (s *grpcStmt) Close() error  { return nil }
func (s *grpcStmt) NumInput() int { return -1 }

func (s *grpcStmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.ExecContext(context.Background(), valuesToNamed(args))
}
func (s *grpcStmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.QueryContext(context.Background(), valuesToNamed(args))
}
func (s *grpcStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	return s.conn.ExecContext(ctx, s.query, args)
}
func (s *grpcStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	return s.conn.QueryContext(ctx, s.query, args)
}

// grpcTx tracks the server-side transaction id and forwards the final
// commit/rollback back through the proxy. The conn pointer lets us clear
// grpcConn.txID once the transaction terminates so subsequent statements
// on this conn go back to the non-transactional path.
type grpcTx struct {
	conn      *grpcConn
	client    pb.SQLProxyClient
	txID      string
	finishCtx context.Context
	done      atomic.Bool
}

func (t *grpcTx) Commit() error {
	return t.finish(true)
}

func (t *grpcTx) Rollback() error {
	return t.finish(false)
}

// finish issues either CommitTx or RollbackTx with a bounded ctx derived
// from BeginTx's caller ctx. database/sql calls Commit/Rollback without a
// ctx, so we cannot accept one here; using context.Background() instead
// would silently strip whatever deadline / trace id the caller attached
// to BeginTx, which is what the previous implementation did.
func (t *grpcTx) finish(commit bool) error {
	if !t.done.CompareAndSwap(false, true) {
		return sql.ErrTxDone
	}
	parent := t.finishCtx
	if parent == nil {
		// Defensive default for callers that constructed grpcTx without
		// going through BeginTx (tests / mocks). A bare Background still
		// honours txFinishTimeout so the RPC cannot block forever.
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, txFinishTimeout)
	defer cancel()

	var err error
	if commit {
		_, err = t.client.CommitTx(ctx, &pb.TxIDRequest{TxId: t.txID})
	} else {
		_, err = t.client.RollbackTx(ctx, &pb.TxIDRequest{TxId: t.txID})
	}
	// Always clear conn.txID so the conn is reusable; otherwise a failed
	// Commit would leave the conn pinned to a dead transaction and every
	// subsequent statement would route into TxQuery/TxExec on a txID the
	// core has already torn down.
	if t.conn != nil {
		t.conn.setTxID("")
	}
	if err != nil {
		op := "commit"
		if !commit {
			op = "rollback"
		}
		slog.Default().Error("plugin-sdk: tx finish failed",
			"op", op, "tx_id", t.txID, "err", err)
	}
	return err
}

// execResult is a trivial driver.Result implementation.
type execResult struct {
	rowsAffected int64
	lastID       int64
}

func (r *execResult) LastInsertId() (int64, error) { return r.lastID, nil }
func (r *execResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

// grpcRows iterates over the SQLResponse returned by Query. The full result
// set is materialised by the proxy; we just hand rows back one at a time.
type grpcRows struct {
	columns  []string
	colTypes []string // PostgreSQL type names from host ColumnTypes()
	rows     []*pb.SQLRow
	idx      int
}

func newRows(resp *pb.SQLResponse) *grpcRows {
	return &grpcRows{
		columns:  resp.GetColumns(),
		colTypes: resp.GetColumnTypes(),
		rows:     resp.GetRows(),
	}
}

func (r *grpcRows) Columns() []string { return r.columns }
func (r *grpcRows) Close() error      { return nil }

func (r *grpcRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	row := r.rows[r.idx]
	r.idx++
	values := row.GetValues()
	if len(values) != len(dest) {
		return fmt.Errorf("plugin-sdk: column count mismatch: got %d, want %d", len(values), len(dest))
	}
	for i, v := range values {
		dest[i] = sqlValueToDriver(v)
	}
	return nil
}

// Compile-time interface assertions.
var (
	_ driver.Driver             = (*grpcDriver)(nil)
	_ driver.Conn               = (*grpcConn)(nil)
	_ driver.ConnBeginTx        = (*grpcConn)(nil)
	_ driver.ConnPrepareContext = (*grpcConn)(nil)
	_ driver.QueryerContext     = (*grpcConn)(nil)
	_ driver.ExecerContext      = (*grpcConn)(nil)
	_ driver.Pinger             = (*grpcConn)(nil)
	_ driver.Validator          = (*grpcConn)(nil)
	_ driver.SessionResetter    = (*grpcConn)(nil)
	_ driver.Stmt               = (*grpcStmt)(nil)
	_ driver.StmtExecContext    = (*grpcStmt)(nil)
	_ driver.StmtQueryContext   = (*grpcStmt)(nil)
	_ driver.Tx                 = (*grpcTx)(nil)
	_ driver.Rows               = (*grpcRows)(nil)
	_ driver.Result             = (*execResult)(nil)

	// grpcRows satisfies the optional column-type introspection interfaces.
	_ interface{ ColumnTypeDatabaseTypeName(int) string } = (*grpcRows)(nil)
	_ interface{ ColumnTypeScanType(int) reflect.Type }   = (*grpcRows)(nil)
)
