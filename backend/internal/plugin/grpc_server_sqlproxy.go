package plugin

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// ============================================================
// SQLProxy 实现
// ============================================================

func (s *SDKServer) Query(ctx context.Context, req *pluginsdk.SQLRequest) (*pluginsdk.SQLResponse, error) {
	if s.db == nil {
		return nil, errService("sql db")
	}
	pluginName, _ := CallerFromContext(ctx)
	if err := s.sqlGate.Authorize(pluginName, req.GetQuery()); err != nil {
		return nil, sqlGateStatus(err)
	}
	args := convertSQLValues(req.GetArgs())
	rows, err := s.db.QueryContext(ctx, req.GetQuery(), args...)
	if err != nil {
		return nil, errInternal(err, "plugin sql query")
	}
	defer func() { _ = rows.Close() }()
	return scanRowsToResponse(rows)
}

func (s *SDKServer) Exec(ctx context.Context, req *pluginsdk.SQLRequest) (*pluginsdk.ExecResponse, error) {
	if s.db == nil {
		return nil, errService("sql db")
	}
	pluginName, _ := CallerFromContext(ctx)
	if err := s.sqlGate.Authorize(pluginName, req.GetQuery()); err != nil {
		return nil, sqlGateStatus(err)
	}
	args := convertSQLValues(req.GetArgs())
	res, err := s.db.ExecContext(ctx, req.GetQuery(), args...)
	if err != nil {
		return nil, errInternal(err, "plugin sql exec")
	}
	return execResultToResponse(res), nil
}

func (s *SDKServer) BeginTx(ctx context.Context, req *pluginsdk.BeginTxRequest) (*pluginsdk.TxResponse, error) {
	if s.db == nil {
		return nil, errService("sql db")
	}
	isolation, err := parseIsolationLevel(req.GetIsolationLevel())
	if err != nil {
		return nil, err
	}
	opts := &sql.TxOptions{ReadOnly: req.GetReadOnly(), Isolation: isolation}

	pluginName, _ := CallerFromContext(ctx)
	if err := s.checkActiveTxLimit(pluginName); err != nil {
		return nil, err
	}

	// context.WithoutCancel: gRPC per-RPC context 在 handler 返回后被
	// transport 层 cancel（finishStream → s.cancel()）。database/sql.Tx
	// 的 awaitDone goroutine 会监听 ctx.Done() 并自动 rollback。事务的
	// 生命周期跨越 BeginTx → TxQuery/TxExec → CommitTx 多个 RPC，必须
	// 用 WithoutCancel 使 ctx 脱离 RPC 生命周期。事务超时保护由
	// rollbackTimedOutTx（30s 清理）兜底。
	tx, err := s.db.BeginTx(context.WithoutCancel(ctx), opts)
	if err != nil {
		return nil, errInternal(err, "plugin begin tx")
	}
	id := uuid.NewString()
	s.txMu.Lock()
	s.txs[id] = &activeTx{tx: tx, owner: pluginName, startedAt: time.Now()}
	s.txMu.Unlock()
	return &pluginsdk.TxResponse{TxId: id}, nil
}

func (s *SDKServer) TxQuery(ctx context.Context, req *pluginsdk.TxSQLRequest) (*pluginsdk.SQLResponse, error) {
	tx, err := s.lookupTx(ctx, req.GetTxId())
	if err != nil {
		return nil, err
	}
	pluginName, _ := CallerFromContext(ctx)
	if err := s.sqlGate.Authorize(pluginName, req.GetQuery()); err != nil {
		return nil, sqlGateStatus(err)
	}
	args := convertSQLValues(req.GetArgs())
	rows, err := tx.QueryContext(ctx, req.GetQuery(), args...)
	if err != nil {
		return nil, errInternal(err, "plugin tx query")
	}
	defer func() { _ = rows.Close() }()
	return scanRowsToResponse(rows)
}

func (s *SDKServer) TxExec(ctx context.Context, req *pluginsdk.TxSQLRequest) (*pluginsdk.ExecResponse, error) {
	tx, err := s.lookupTx(ctx, req.GetTxId())
	if err != nil {
		return nil, err
	}
	pluginName, _ := CallerFromContext(ctx)
	if err := s.sqlGate.Authorize(pluginName, req.GetQuery()); err != nil {
		return nil, sqlGateStatus(err)
	}
	args := convertSQLValues(req.GetArgs())
	res, err := tx.ExecContext(ctx, req.GetQuery(), args...)
	if err != nil {
		return nil, errInternal(err, "plugin tx exec")
	}
	return execResultToResponse(res), nil
}

func (s *SDKServer) CommitTx(ctx context.Context, req *pluginsdk.TxIDRequest) (*emptypb.Empty, error) {
	tx, err := s.popTx(ctx, req.GetTxId())
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, errInternal(err, "plugin commit tx")
	}
	return &emptypb.Empty{}, nil
}

func (s *SDKServer) RollbackTx(ctx context.Context, req *pluginsdk.TxIDRequest) (*emptypb.Empty, error) {
	tx, err := s.popTx(ctx, req.GetTxId())
	if err != nil {
		return nil, err
	}
	if err := tx.Rollback(); err != nil {
		return nil, errInternal(err, "plugin rollback tx")
	}
	return &emptypb.Empty{}, nil
}

func (s *SDKServer) lookupTx(ctx context.Context, id string) (*sql.Tx, error) {
	s.txMu.Lock()
	defer s.txMu.Unlock()
	t, ok := s.txs[id]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "unknown tx_id: %s", id)
	}
	if caller, _ := CallerFromContext(ctx); caller != t.owner {
		return nil, status.Errorf(codes.PermissionDenied,
			"tx %s belongs to %s, not %s", id, t.owner, caller)
	}
	return t.tx, nil
}

func (s *SDKServer) popTx(ctx context.Context, id string) (*sql.Tx, error) {
	s.txMu.Lock()
	defer s.txMu.Unlock()
	t, ok := s.txs[id]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "unknown tx_id: %s", id)
	}
	if caller, _ := CallerFromContext(ctx); caller != t.owner {
		return nil, status.Errorf(codes.PermissionDenied,
			"tx %s belongs to %s, not %s", id, t.owner, caller)
	}
	delete(s.txs, id)
	return t.tx, nil
}

// parseIsolationLevel converts a proto-level isolation string to sql.IsolationLevel.
func parseIsolationLevel(level string) (sql.IsolationLevel, error) {
	switch level {
	case "read_committed":
		return sql.LevelReadCommitted, nil
	case "repeatable_read":
		return sql.LevelRepeatableRead, nil
	case "serializable":
		return sql.LevelSerializable, nil
	case "":
		return sql.LevelDefault, nil
	default:
		return 0, status.Errorf(codes.InvalidArgument, "unsupported isolation level: %s", level)
	}
}

// checkActiveTxLimit enforces maxActiveTxPerPlugin for the given plugin.
func (s *SDKServer) checkActiveTxLimit(pluginName string) error {
	s.txMu.Lock()
	defer s.txMu.Unlock()
	count := 0
	for _, t := range s.txs {
		if t.owner == pluginName {
			count++
		}
	}
	if count >= maxActiveTxPerPlugin {
		return status.Errorf(codes.ResourceExhausted,
			"plugin %s has %d active transactions (max %d)",
			pluginName, count, maxActiveTxPerPlugin)
	}
	return nil
}

// collectColumnTypeNames extracts database type names from *sql.Rows.
// Returns a zero-length slice on unsupported drivers (ColumnTypes returns error).
func collectColumnTypeNames(rows *sql.Rows, numCols int) []string {
	colTypes, _ := rows.ColumnTypes()
	typeNames := make([]string, numCols)
	for i, ct := range colTypes {
		typeNames[i] = ct.DatabaseTypeName()
	}
	return typeNames
}

// ============================================================
// SQL 类型转换辅助
// ============================================================

func convertSQLValues(values []*pluginsdk.SQLValue) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = sqlValueToInterface(v)
	}
	return out
}

func sqlValueToInterface(v *pluginsdk.SQLValue) any {
	if v == nil {
		return nil
	}
	switch payload := v.GetValue().(type) {
	case *pluginsdk.SQLValue_Null:
		return nil
	case *pluginsdk.SQLValue_IntValue:
		return payload.IntValue
	case *pluginsdk.SQLValue_FloatValue:
		return payload.FloatValue
	case *pluginsdk.SQLValue_StringValue:
		return payload.StringValue
	case *pluginsdk.SQLValue_BytesValue:
		return payload.BytesValue
	case *pluginsdk.SQLValue_BoolValue:
		return payload.BoolValue
	case *pluginsdk.SQLValue_TimeValue:
		t, err := time.Parse(time.RFC3339Nano, payload.TimeValue)
		if err != nil {
			return payload.TimeValue
		}
		return t
	default:
		return nil
	}
}

func interfaceToSQLValue(v any) *pluginsdk.SQLValue {
	if v == nil {
		return &pluginsdk.SQLValue{Value: &pluginsdk.SQLValue_Null{Null: true}}
	}
	switch x := v.(type) {
	case bool:
		return &pluginsdk.SQLValue{Value: &pluginsdk.SQLValue_BoolValue{BoolValue: x}}
	case int:
		return &pluginsdk.SQLValue{Value: &pluginsdk.SQLValue_IntValue{IntValue: int64(x)}}
	case int32:
		return &pluginsdk.SQLValue{Value: &pluginsdk.SQLValue_IntValue{IntValue: int64(x)}}
	case int64:
		return &pluginsdk.SQLValue{Value: &pluginsdk.SQLValue_IntValue{IntValue: x}}
	case float32:
		return &pluginsdk.SQLValue{Value: &pluginsdk.SQLValue_FloatValue{FloatValue: float64(x)}}
	case float64:
		return &pluginsdk.SQLValue{Value: &pluginsdk.SQLValue_FloatValue{FloatValue: x}}
	case string:
		return &pluginsdk.SQLValue{Value: &pluginsdk.SQLValue_StringValue{StringValue: x}}
	case []byte:
		return &pluginsdk.SQLValue{Value: &pluginsdk.SQLValue_BytesValue{BytesValue: x}}
	case time.Time:
		return &pluginsdk.SQLValue{Value: &pluginsdk.SQLValue_TimeValue{TimeValue: x.Format(time.RFC3339Nano)}}
	default:
		// 兜底:转字符串,避免 panic
		return &pluginsdk.SQLValue{Value: &pluginsdk.SQLValue_StringValue{StringValue: fmt.Sprintf("%v", x)}}
	}
}

func scanRowsToResponse(rows *sql.Rows) (*pluginsdk.SQLResponse, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, errInternal(err, "plugin sql columns")
	}
	typeNames := collectColumnTypeNames(rows, len(cols))

	resp := &pluginsdk.SQLResponse{Columns: cols, ColumnTypes: typeNames}
	for rows.Next() {
		if len(resp.Rows) >= maxQueryRows {
			return nil, status.Errorf(codes.ResourceExhausted,
				"plugin sql result exceeds %d rows; use pagination", maxQueryRows)
		}
		holders := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range holders {
			ptrs[i] = &holders[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, errInternal(err, "plugin sql scan")
		}
		row := &pluginsdk.SQLRow{Values: make([]*pluginsdk.SQLValue, len(cols))}
		for i, v := range holders {
			row.Values[i] = interfaceToSQLValue(v)
		}
		resp.Rows = append(resp.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, errInternal(err, "plugin sql iterate")
	}
	return resp, nil
}

func execResultToResponse(res sql.Result) *pluginsdk.ExecResponse {
	out := &pluginsdk.ExecResponse{}
	if rows, err := res.RowsAffected(); err == nil {
		out.RowsAffected = rows
	}
	if id, err := res.LastInsertId(); err == nil {
		out.LastInsertId = id
	}
	return out
}
