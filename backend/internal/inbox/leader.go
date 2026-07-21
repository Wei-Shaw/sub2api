package inbox

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/redis/go-redis/v9"
)

const (
	// cleanupLockKey 是清理任务的分布式锁 key。
	cleanupLockKey = "sub2api:inbox:cleanup:lock"
	// cleanupLockTTL 是锁的存活时间：远大于单轮清理耗时、远小于清理周期（1h），
	// 保证同一周期内仅一个副本执行，且到期自动释放，无需显式解锁——从而避免"执行
	// 中途显式 DEL 释放锁 → 其它副本抢到 → 重复执行"的竞态窗口。
	cleanupLockTTL = 5 * time.Minute
)

// LeaderGuard 基于 Redis `SET NX EX` 的"单周期领导者"守卫。多副本部署下用于保证
// 周期性任务（如信箱数据保留清理）每个周期仅有一个副本执行。
type LeaderGuard struct {
	rdb *redis.Client
	key string
	ttl time.Duration
}

// NewCleanupLeaderGuard 构造清理任务的领导者守卫。rdb 为 nil 时 TryAcquire 恒为 true
// （单副本 / 无 Redis 场景直接执行）。
func NewCleanupLeaderGuard(rdb *redis.Client) *LeaderGuard {
	return &LeaderGuard{rdb: rdb, key: cleanupLockKey, ttl: cleanupLockTTL}
}

// TryAcquire 尝试获取本周期的执行权（`SET key 1 NX EX ttl`）：
//   - rdb 为 nil：返回 true（无分布式协调需求，直接执行）；
//   - 抢到锁：返回 true；
//   - 未抢到（其它副本本周期已持有）：返回 false；
//   - Redis 出错：返回 false（fail-closed，避免 Redis 抖动时所有副本一起执行清理）。
func (g *LeaderGuard) TryAcquire(ctx context.Context) bool {
	if g == nil || g.rdb == nil {
		return true
	}
	ok, err := g.rdb.SetNX(ctx, g.key, "1", g.ttl).Result()
	if err != nil {
		logger.LegacyPrintf("inbox.cleanup", "[Inbox] acquire cleanup lock failed: %v", err)
		return false
	}
	return ok
}
