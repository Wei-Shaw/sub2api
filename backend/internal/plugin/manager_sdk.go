package plugin

import (
	"errors"
	"fmt"
	"net"

	"google.golang.org/grpc"

	"github.com/Wei-Shaw/sub2api/internal/pkg/leaderlock"
	pluginsdkroot "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

// =============================================================
// SDK gRPC server lifecycle (启动监听 / 注册 services / 停止)
//
// 这一组方法只负责"插件 ↔ host"的 gRPC 通道生命周期。所有 service
// 注册、interceptor 配置都在这里集中, 与 plugin 进程的运行无关 —
// SDK gRPC server 是 host 全局单例, 不随 plugin Enable/Disable 变化。
// =============================================================

// startSDKServer 监听 SDK gRPC 端口并启动 grpc.Server。
//
// 这里同时初始化 V5 W2 引入的 JobSchedulerServer:scheduler 与 SDKServer
// 共享同一个 Redis client(通过 leaderlock helper 复用 OpsCleanupService 的
// 锁实现),并把 caller 解析委托给 SDKServer.resolveCaller,确保 plugin
// metadata 头与 RedisProxy/EventBus 等服务用同一套身份识别。
//
// gRPC server 上挂载 RequirePluginIdentity{Unary,Stream} 拦截器,把每个
// SDK RPC 的 caller 解析与匿名拒绝集中在一处。下游 handler 只需调用
// CallerFromContext(ctx) 取已校验的 plugin name,不再各自走 metadata 抓取
// + knownPluginsRegistry 校验那一套样板。任何匿名调用都会在 handler 之前
// 就被 PermissionDenied 短路。
//
// T1 + T5: 拦截器顺序 = traceparent 在前 + identity 在后, 让 PermissionDenied
// 日志也带上 trace id, 便于审计被拒绝的调用。
func (m *PluginManager) startSDKServer() error {
	ln, err := net.Listen("tcp", m.cfg.SDKListenAddr)
	if err != nil {
		return fmt.Errorf("listen sdk addr %s: %w", m.cfg.SDKListenAddr, err)
	}
	m.sdkLn = ln
	m.sdkAddr = ln.Addr().String()

	jobScheduler := m.attachJobScheduler()
	srv := m.buildGRPCServer()
	m.registerExtensionServices(srv)
	m.sdkGRPC = srv

	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			m.logger.Error("sdk grpc server stopped", "error", err)
		}
	}()
	m.logger.Info("sdk grpc server listening",
		"addr", m.sdkAddr,
		"job_scheduler_attached", jobScheduler != nil,
	)
	return nil
}

// attachJobScheduler 构造并挂载 V5 W2 JobScheduler。m.jobHistory 可为 nil
// (admin handler 还没注入) — scheduler 内部会容忍并降级为 log-only。
func (m *PluginManager) attachJobScheduler() *JobSchedulerServer {
	leaderProvider := m.buildLeaderLockProvider()
	jobScheduler := NewJobSchedulerServer(
		leaderProvider,
		m.jobHistory,
		m.logger,
	)
	m.sdkServer.AttachJobScheduler(jobScheduler)
	m.jobScheduler = jobScheduler
	return jobScheduler
}

// buildGRPCServer 构造一台带"trace + 身份验证"两层拦截器的 grpc.Server,
// 并把核心 SDK services 挂上。Settings/Events 的 service 在 registerExtensionServices
// 中按需 register。
func (m *PluginManager) buildGRPCServer() *grpc.Server {
	resolver := CallerResolver(m.sdkServer.resolveCaller)
	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			pluginsdkroot.TraceparentUnaryServer(),
			RequirePluginIdentityUnary(resolver),
		),
		grpc.ChainStreamInterceptor(
			pluginsdkroot.TraceparentStreamServer(),
			RequirePluginIdentityStream(resolver),
		),
	)
	m.sdkServer.RegisterServices(srv)
	return srv
}

// registerExtensionServices 按可选 extension (Settings / Events / Host) 挂载
// 额外的 gRPC service。若对应 service 未配置 (SetSettings/SetEvents/
// SetHostPricingResolver 没调用), 不注册任何东西 — 插件这边收到的会是
// codes.Unimplemented, 这是预期的"未启用"语义。
func (m *PluginManager) registerExtensionServices(srv *grpc.Server) {
	if m.settingsServer != nil {
		m.settingsServer.Register(srv)
	}
	if m.eventsServer != nil {
		m.eventsServer.Register(srv)
	}
	if m.hostService != nil {
		m.hostService.RegisterServices(srv)
	}
}

// buildLeaderLockProvider mints a leaderlock.Provider sharing the manager's
// Redis client (when present) with a stable per-process instance ID. We pull
// the Redis client off SDKServer because the manager no longer keeps a
// reference of its own — see NewPluginManager.
func (m *PluginManager) buildLeaderLockProvider() leaderlock.Provider {
	return leaderlock.New(m.sdkServer.redis, m.db, leaderlock.Config{
		InstanceID: m.instanceID,
		TTL:        jobLeaderLockTTL,
		// SingleInstance is intentionally false: even in simple mode we
		// take the Redis lease so subsequent multi-replica deploys do not
		// silently double-fire jobs. The SetNX cost is negligible.
		Logger: m.logger.With("component", "plugin_job_leader_lock"),
	})
}
