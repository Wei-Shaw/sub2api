package inbox

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	// defaultCleanupBatch 是单批删除的行数上限，避免长事务与锁放大。
	defaultCleanupBatch = 10000
	// maxCleanupIterations 是单次清理的最大批次数，防御异常情况下的无限循环。
	maxCleanupIterations = 1000
)

// Cleaner 周期性删除超过保留期的消息。多副本部署时应由 leader 守卫保证仅一个节点执行。
type Cleaner struct {
	repo      Repository
	retention time.Duration
	batch     int
	now       func() time.Time
}

// NewCleaner 构造清理器。retention<=0 用默认 30 天；batch<=0 用默认批大小。
func NewCleaner(repo Repository, retention time.Duration, batch int) *Cleaner {
	if retention <= 0 {
		retention = defaultRetention
	}
	if batch <= 0 {
		batch = defaultCleanupBatch
	}
	return &Cleaner{repo: repo, retention: retention, batch: batch, now: time.Now}
}

// RunOnce 执行一轮清理，返回删除的单播 / 广播行数。分批循环直至无更多过期行。
func (c *Cleaner) RunOnce(ctx context.Context) (directDeleted, broadcastDeleted int64, err error) {
	cutoff := c.now().Add(-c.retention)

	directDeleted, err = c.batchDelete(ctx, func(ctx context.Context, limit int) (int64, error) {
		return c.repo.DeleteExpiredDirect(ctx, cutoff, limit)
	})
	if err != nil {
		return directDeleted, 0, err
	}
	broadcastDeleted, err = c.batchDelete(ctx, func(ctx context.Context, limit int) (int64, error) {
		return c.repo.DeleteExpiredBroadcasts(ctx, cutoff, limit)
	})
	return directDeleted, broadcastDeleted, err
}

// batchDelete 反复调用 delete 直至单批删除数小于批大小（无更多过期行）或触及迭代上限。
func (c *Cleaner) batchDelete(ctx context.Context, delete func(context.Context, int) (int64, error)) (int64, error) {
	var total int64
	for i := 0; i < maxCleanupIterations; i++ {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, err := delete(ctx, c.batch)
		if err != nil {
			return total, err
		}
		total += n
		if n < int64(c.batch) {
			break
		}
	}
	return total, nil
}

// Start 以 interval 为周期运行清理，直到 ctx 取消。每轮先通过 isLeader 判定是否由本
// 节点执行（多副本下避免重复删除）。isLeader 为 nil 时默认本节点执行。阻塞运行，应在
// 独立 goroutine 中启动。
func (c *Cleaner) Start(ctx context.Context, interval time.Duration, isLeader func(context.Context) bool) {
	if interval <= 0 {
		interval = time.Hour
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if isLeader != nil && !isLeader(ctx) {
				continue
			}
			d, b, err := c.RunOnce(ctx)
			if err != nil {
				logger.LegacyPrintf("inbox.cleanup", "[Inbox] cleanup run failed: %v", err)
				continue
			}
			if d > 0 || b > 0 {
				logger.LegacyPrintf("inbox.cleanup", "[Inbox] cleanup deleted direct=%d broadcast=%d", d, b)
			}
		}
	}
}
