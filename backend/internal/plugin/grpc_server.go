package plugin

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// txAutoRollbackTimeout 是事务的自动回滚超时,防止插件忘记 Commit/Rollback 导致 sql 事务长期占用连接。
const txAutoRollbackTimeout = 30 * time.Second

// txCleanupInterval 是事务清理 goroutine 的轮询间隔。
const txCleanupInterval = 10 * time.Second

// maxActiveTxPerPlugin 限制每个插件的最大活跃事务数,防止单个插件耗尽连接池。
const maxActiveTxPerPlugin = 16

// maxQueryRows caps the number of rows returned to a plugin per Query/TxQuery call.
const maxQueryRows = 10000

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
	cancel    context.CancelFunc // WithTimeout cancel; called on Commit/Rollback/cleanup
	owner     string
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
		if t.cancel != nil {
			t.cancel()
		}
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

	type expiredEntry struct {
		id string
		t  *activeTx
	}
	s.txMu.Lock()
	var expired []expiredEntry
	for id, t := range s.txs {
		if t.startedAt.Before(cutoff) {
			expired = append(expired, expiredEntry{id: id, t: t})
			delete(s.txs, id)
		}
	}
	s.txMu.Unlock()

	for _, e := range expired {
		if e.t.cancel != nil {
			e.t.cancel()
		}
		if err := e.t.tx.Rollback(); err != nil {
			slog.Warn("rollback timed-out plugin tx", "tx_id", e.id, "error", err)
		} else {
			slog.Warn("rolled back timed-out plugin tx", "tx_id", e.id, "age", time.Since(e.t.startedAt).String())
		}
	}
}
