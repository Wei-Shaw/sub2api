package inbox

import (
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
)

// ProviderSet 是信箱模块的 wire provider 集合。
//
// 依赖说明：*sql.DB 与 *redis.Client 由 repository 层提供；AttributeProvider、Metrics、
// Config、WSAuthenticator、UserIDFunc、OriginChecker 这些"应用侧"依赖由 server 层提供
// （它们依赖 service/middleware，放在 server 包以避免 inbox 反向依赖 service 形成环）。
var ProviderSet = wire.NewSet(
	NewRepository,
	NewSeqAllocator,
	wire.Bind(new(SeqSource), new(*SeqAllocator)),
	NewService,
	NewHub,
	ProvideCoordinator,
	wire.Bind(new(Notifier), new(*Coordinator)),
	NewPublisherWithMetrics,
	ProvideCleaner,
	NewHandler,
	NewWSHandler,
)

// ProvideCoordinator 构造协调器并把它注入 Hub 作为跨实例踢出广播器（解决 Hub↔
// Coordinator 的构造循环：Hub 先建、Coordinator 持有 Hub、再回填 kicker）。
func ProvideCoordinator(rdb *redis.Client, hub *Hub) *Coordinator {
	c := NewCoordinator(rdb, hub)
	hub.SetKicker(c)
	return c
}

// ProvideCleaner 从 Config 构造清理器（把非注入友好的 retention/batch 参数收敛到 Config）。
func ProvideCleaner(repo Repository, cfg Config) *Cleaner {
	return NewCleaner(repo, cfg.Retention, 0)
}
