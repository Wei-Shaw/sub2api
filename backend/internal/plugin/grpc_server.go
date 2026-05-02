package plugin

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// txAutoRollbackTimeout 是事务的自动回滚超时,防止插件忘记 Commit/Rollback 导致 sql 事务长期占用连接。
const txAutoRollbackTimeout = 30 * time.Second

// txCleanupInterval 是事务清理 goroutine 的轮询间隔。
const txCleanupInterval = 10 * time.Second

// SDKServer 实现了 SQLProxy / RedisProxy 两个 gRPC 服务,
// 并通过内嵌 eventBusAdapter 暴露 EventBus 服务。
// 这是核心暴露给插件的 SDK 入口。
//
// 之所以把 EventBus 拆成单独的 adapter,是因为 RedisProxy.Publish 和 EventBus.Publish
// 同名但参数类型不同,Go 不允许同名方法,只能用 wrapper 解决签名冲突。
type SDKServer struct {
	pluginsdk.UnimplementedSQLProxyServer
	pluginsdk.UnimplementedRedisProxyServer

	db    *sql.DB
	redis *redis.Client

	txMu sync.Mutex
	txs  map[string]*activeTx

	// capabilities tracks which plugin holds which privileged SDK features
	// (e.g. redis_raw_keys). Populated by the manager via RegisterPlugin
	// once GetManifest has been processed.
	capabilities *pluginCapabilityRegistry

	// knownPlugins is the set of plugin names the manager has registered.
	// It exists so the metadata-based caller identity check can reject
	// arbitrary names supplied by something that is not actually a plugin.
	knownPlugins *knownPluginsRegistry

	// secrets handles the SecretEncryption gRPC service (V5 W5). Nil when
	// the host does not have a master key configured — gRPC will refuse
	// the SecretEncryption methods with Unimplemented in that case rather
	// than crashing on a nil dereference.
	secrets *SecretEncryptionServer

	// jobScheduler is the optional V5 W2 JobScheduler service. The manager
	// attaches it after construction (see PluginManager.startSDKServer) so
	// SDKServer stays dependency-free of the concrete scheduler type.
	// RegisterServices only registers the gRPC service when this is non-nil.
	jobScheduler *JobSchedulerServer

	// sqlGate enforces the per-plugin table allow-list (P12·B-1) on every
	// SQLProxy entry point. Always non-nil; gates on an empty plugin name
	// no-op so internal / non-plugin callers retain full SQL access.
	sqlGate *sqlGate
	stop    context.CancelFunc
}

type activeTx struct {
	tx        *sql.Tx
	startedAt time.Time
}

// NewSDKServer 构造 SDK 服务实例,并启动事务清理 goroutine。
// 调用方应在程序退出前调用 Stop() 释放资源。
//
// secretMasterKeyHex 是 V5 W5 SecretEncryption 服务的主密钥（32 字节 hex）。
// 传空串表示禁用 SecretEncryption 服务（gRPC 端不会注册 service，调用插件会得到
// Unimplemented），通常在自动化测试或显式关闭加密能力时使用。
func NewSDKServer(db *sql.DB, rdb *redis.Client, secretMasterKeyHex string) (*SDKServer, error) {
	caps := newPluginCapabilityRegistry()
	s := &SDKServer{
		db:           db,
		redis:        rdb,
		txs:          make(map[string]*activeTx),
		capabilities: caps,
		knownPlugins: newKnownPluginsRegistry(),
		sqlGate:      newSQLGate(caps),
	}
	if secretMasterKeyHex != "" {
		// T51: caller identity now flows exclusively through ctx — the
		// interceptor stamps it for production traffic, tests use
		// WithCaller(ctx, name). The constructor no longer takes a
		// resolver parameter.
		secrets, err := NewSecretEncryptionServer(secretMasterKeyHex, slog.Default().With("component", "plugin_secret_encryption"))
		if err != nil {
			return nil, fmt.Errorf("init secret encryption server: %w", err)
		}
		s.secrets = secrets
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.stop = cancel
	go s.cleanupLoop(ctx)
	return s, nil
}

// Stop 取消后台 goroutine 并回滚所有未完成事务。
func (s *SDKServer) Stop() {
	if s.stop != nil {
		s.stop()
	}
	s.txMu.Lock()
	defer s.txMu.Unlock()
	for id, t := range s.txs {
		_ = t.tx.Rollback()
		delete(s.txs, id)
	}
}

// RegisterServices 把所有 SDK 服务注册到 grpcServer。
//
// JobScheduler 是 V5 W2 引入的可选服务,只有 manager 在 startSDKServer 中
// 通过 AttachJobScheduler 注入实例后才会注册。这样旧部署或未声明
// CapabilityJobScheduler 的插件不会因为缺少 scheduler 实现而启动失败。
func (s *SDKServer) RegisterServices(grpcServer *grpc.Server) {
	pluginsdk.RegisterSQLProxyServer(grpcServer, s)
	pluginsdk.RegisterRedisProxyServer(grpcServer, s)
	pluginsdk.RegisterEventBusServer(grpcServer, &eventBusAdapter{srv: s})
	pluginsdk.RegisterLogProxyServer(grpcServer, NewLogProxyServer(s))
	if s.secrets != nil {
		s.secrets.RegisterServices(grpcServer)
	}
	if s.jobScheduler != nil {
		pluginsdk.RegisterJobSchedulerServer(grpcServer, s.jobScheduler)
	}
}

// AttachJobScheduler binds a JobSchedulerServer onto this SDKServer. Must be
// called BEFORE RegisterServices so the gRPC dispatcher learns about the
// service. Calling twice replaces the previous scheduler — the manager only
// does this in tests; production builds set it once at startup.
func (s *SDKServer) AttachJobScheduler(js *JobSchedulerServer) {
	s.jobScheduler = js
}

// JobScheduler returns the attached scheduler, or nil if none has been
// installed. Exposed so admin handlers (W2.4) can call ManualFire.
func (s *SDKServer) JobScheduler() *JobSchedulerServer {
	return s.jobScheduler
}

// cleanupLoop 周期性回滚超时事务。
func (s *SDKServer) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(txCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.rollbackTimedOutTx()
		}
	}
}

func (s *SDKServer) rollbackTimedOutTx() {
	cutoff := time.Now().Add(-txAutoRollbackTimeout)

	s.txMu.Lock()
	expired := make([]string, 0)
	for id, t := range s.txs {
		if t.startedAt.Before(cutoff) {
			expired = append(expired, id)
		}
	}
	s.txMu.Unlock()

	for _, id := range expired {
		s.txMu.Lock()
		t, ok := s.txs[id]
		if ok {
			delete(s.txs, id)
		}
		s.txMu.Unlock()
		if !ok {
			continue
		}
		if err := t.tx.Rollback(); err != nil {
			slog.Warn("rollback timed-out plugin tx", "tx_id", id, "error", err)
		} else {
			slog.Warn("rolled back timed-out plugin tx", "tx_id", id, "age", time.Since(t.startedAt).String())
		}
	}
}

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
	opts := &sql.TxOptions{ReadOnly: req.GetReadOnly()}
	switch req.GetIsolationLevel() {
	case "read_committed":
		opts.Isolation = sql.LevelReadCommitted
	case "repeatable_read":
		opts.Isolation = sql.LevelRepeatableRead
	case "serializable":
		opts.Isolation = sql.LevelSerializable
	case "":
		// 使用驱动默认
	default:
		return nil, status.Errorf(codes.InvalidArgument, "unsupported isolation level: %s", req.GetIsolationLevel())
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
	s.txs[id] = &activeTx{tx: tx, startedAt: time.Now()}
	s.txMu.Unlock()
	return &pluginsdk.TxResponse{TxId: id}, nil
}

func (s *SDKServer) TxQuery(ctx context.Context, req *pluginsdk.TxSQLRequest) (*pluginsdk.SQLResponse, error) {
	tx, err := s.lookupTx(req.GetTxId())
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
	tx, err := s.lookupTx(req.GetTxId())
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

func (s *SDKServer) CommitTx(_ context.Context, req *pluginsdk.TxIDRequest) (*emptypb.Empty, error) {
	tx, err := s.popTx(req.GetTxId())
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, errInternal(err, "plugin commit tx")
	}
	return &emptypb.Empty{}, nil
}

func (s *SDKServer) RollbackTx(_ context.Context, req *pluginsdk.TxIDRequest) (*emptypb.Empty, error) {
	tx, err := s.popTx(req.GetTxId())
	if err != nil {
		return nil, err
	}
	if err := tx.Rollback(); err != nil {
		return nil, errInternal(err, "plugin rollback tx")
	}
	return &emptypb.Empty{}, nil
}

func (s *SDKServer) lookupTx(id string) (*sql.Tx, error) {
	s.txMu.Lock()
	defer s.txMu.Unlock()
	t, ok := s.txs[id]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "unknown tx_id: %s", id)
	}
	return t.tx, nil
}

func (s *SDKServer) popTx(id string) (*sql.Tx, error) {
	s.txMu.Lock()
	defer s.txMu.Unlock()
	t, ok := s.txs[id]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "unknown tx_id: %s", id)
	}
	delete(s.txs, id)
	return t.tx, nil
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

	// Retrieve database type names so the plugin driver can implement
	// ColumnTypeDatabaseTypeName / ColumnTypeScanType for ent's unknown-
	// column branch. Errors are ignored — some drivers do not support
	// ColumnTypes and the proxy should still work without type metadata.
	colTypes, _ := rows.ColumnTypes()
	typeNames := make([]string, len(cols))
	if colTypes != nil {
		for i, ct := range colTypes {
			typeNames[i] = ct.DatabaseTypeName()
		}
	}

	resp := &pluginsdk.SQLResponse{Columns: cols, ColumnTypes: typeNames}
	for rows.Next() {
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

