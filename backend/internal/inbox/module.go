package inbox

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

// Module 组装信箱的所有组件，供应用在启动时一次性构造并接线，避免侵入全局 wire 代码生成。
type Module struct {
	Repo      Repository
	Seq       *SeqAllocator
	Publisher Publisher
	Service   *Service
	Hub       *Hub
	Coord     *Coordinator
	Cleaner   *Cleaner
	Handler   *Handler
	WS        *WSHandler
}

// ModuleDeps 是构造 Module 所需的外部依赖。
type ModuleDeps struct {
	// DB / Redis 为必需的基础设施。
	DB    *sql.DB
	Redis *redis.Client
	// Attrs 提供广播 targeting 求值所需的用户属性；nil 时退化为 noop（仅 all_users 命中）。
	Attrs AttributeProvider
	// Metrics 可观测性打点；nil 时用 noop。
	Metrics Metrics
	// WSAuth 从 WS 握手请求解析用户身份；WS 端点必需。
	WSAuth WSAuthenticator
	// GetUserID 从 gin 上下文取当前用户 id；REST 端点必需。
	GetUserID UserIDFunc
	// CheckOrigin 校验 WS 握手 Origin；nil 用 gorilla 默认（同源）。
	CheckOrigin func(*http.Request) bool
	// Config 服务参数（保留期 / 分页等），零值回退默认。
	Config Config
}

// NewModule 构造并接线信箱模块。发布 SDK 的 Notifier 由 Coordinator 承担（Redis pub/sub
// 跨节点 fan-out）。
func NewModule(deps ModuleDeps) *Module {
	repo := NewRepository(deps.DB)
	seq := NewSeqAllocator(deps.Redis)

	attrs := deps.Attrs
	if attrs == nil {
		attrs = NewNoopAttributeProvider()
	}
	metrics := deps.Metrics
	if metrics == nil {
		metrics = NewNoopMetrics()
	}

	svc := NewService(repo, seq, attrs, metrics, deps.Config)
	hub := NewHub(svc, attrs, metrics)
	coord := NewCoordinator(deps.Redis, hub)
	hub.SetKicker(coord) // 跨实例踢出广播器回填
	pub := NewPublisherWithMetrics(seq, repo, coord, metrics)
	cleaner := NewCleaner(repo, deps.Config.Retention, 0)
	handler := NewHandler(svc, pub, deps.GetUserID)
	ws := NewWSHandler(hub, svc, deps.WSAuth, metrics, deps.CheckOrigin)

	return &Module{
		Repo:      repo,
		Seq:       seq,
		Publisher: pub,
		Service:   svc,
		Hub:       hub,
		Coord:     coord,
		Cleaner:   cleaner,
		Handler:   handler,
		WS:        ws,
	}
}

// StartBackground 启动常驻后台任务：Redis pub/sub 订阅与周期清理。均在独立 goroutine
// 运行，随 ctx 取消而退出。cleanupInterval<=0 时默认 1 小时。isLeader 为 nil 时本节点
// 总是执行清理（单副本部署）。
func (m *Module) StartBackground(ctx context.Context, cleanupInterval time.Duration, isLeader func(context.Context) bool) {
	go m.Coord.Run(ctx)
	go m.Cleaner.Start(ctx, cleanupInterval, isLeader)
}
