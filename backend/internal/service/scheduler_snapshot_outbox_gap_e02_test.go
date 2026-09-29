//go:build unit

package service

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// e02GapCache 记录每个账号 last-used 更新被应用的次数。
type e02GapCache struct {
	SchedulerCache

	watermark     int64
	setWatermarks []int64
	applied       map[int64]int
}

func (c *e02GapCache) GetOutboxWatermark(context.Context) (int64, error) {
	return c.watermark, nil
}

func (c *e02GapCache) SetOutboxWatermark(_ context.Context, id int64) error {
	c.watermark = id
	c.setWatermarks = append(c.setWatermarks, id)
	return nil
}

func (c *e02GapCache) UpdateLastUsed(_ context.Context, updates map[int64]time.Time) error {
	for id := range updates {
		c.applied[id]++
	}
	return nil
}

// e02GapRepo 模拟只返回已提交行的 outbox：WHERE id > afterID ORDER BY id LIMIT n。
type e02GapRepo struct {
	events      []SchedulerOutboxEvent
	deleteCalls []int64
}

func (r *e02GapRepo) ListAfterAndReleaseDedup(_ context.Context, afterID int64, limit int) ([]SchedulerOutboxEvent, error) {
	out := make([]SchedulerOutboxEvent, 0, len(r.events))
	for _, event := range r.events {
		if event.ID <= afterID {
			continue
		}
		out = append(out, event)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (r *e02GapRepo) FirstCreatedAtAfter(_ context.Context, afterID int64) (time.Time, bool, error) {
	for _, event := range r.events {
		if event.ID > afterID {
			return event.CreatedAt, true, nil
		}
	}
	return time.Time{}, false, nil
}

func (r *e02GapRepo) MaxID(context.Context) (int64, error) {
	var maxID int64
	for _, event := range r.events {
		if event.ID > maxID {
			maxID = event.ID
		}
	}
	return maxID, nil
}

func (r *e02GapRepo) DeleteConsumedUpTo(_ context.Context, watermark int64, _ int) (int64, error) {
	r.deleteCalls = append(r.deleteCalls, watermark)
	return 0, nil
}

type e02GapLease struct{}

func (e02GapLease) Release() {}

func (r *e02GapRepo) TryAcquireCleanupLock(context.Context) (SchedulerOutboxCleanupLease, bool, error) {
	return e02GapLease{}, true, nil
}

func e02LastUsedEvent(id, accountID int64) SchedulerOutboxEvent {
	return SchedulerOutboxEvent{
		ID:        id,
		EventType: SchedulerOutboxEventAccountLastUsed,
		Payload: map[string]any{
			"last_used": map[string]any{strconv.FormatInt(accountID, 10): float64(1700000000)},
		},
		CreatedAt: time.Now(),
	}
}

func e02NewService(cache *e02GapCache, repo *e02GapRepo) *SchedulerSnapshotService {
	return NewSchedulerSnapshotService(cache, repo, nil, nil, nil)
}

func TestSchedulerOutboxE02LateCommittedGapEventIsConsumed(t *testing.T) {
	cache := &e02GapCache{watermark: 9, applied: make(map[int64]int)}
	// id=10 属于尚未提交的事务，此时只能看到 id=11。
	repo := &e02GapRepo{events: []SchedulerOutboxEvent{e02LastUsedEvent(11, 8)}}
	svc := e02NewService(cache, repo)

	svc.pollOutbox()
	require.Equal(t, 1, cache.applied[8], "缺口之后的事件应立即处理")
	require.Equal(t, int64(9), cache.watermark, "水位应停在缺口之前")
	require.Empty(t, cache.setWatermarks)

	// 事务提交后 id=10 可见。
	repo.events = []SchedulerOutboxEvent{e02LastUsedEvent(10, 7), e02LastUsedEvent(11, 8)}
	svc.pollOutbox()

	require.Equal(t, 1, cache.applied[7], "晚提交的 id=10 不能因水位越过它而丢失")
	require.Equal(t, 1, cache.applied[8], "缺口等待期间已处理的事件不得重复处理")
	require.Equal(t, int64(11), cache.watermark)
}

func TestSchedulerOutboxE02FreshStartAdvancesPastGaps(t *testing.T) {
	cache := &e02GapCache{applied: make(map[int64]int)}
	repo := &e02GapRepo{events: []SchedulerOutboxEvent{e02LastUsedEvent(5, 1), e02LastUsedEvent(7, 2)}}
	svc := e02NewService(cache, repo)

	svc.pollOutbox()

	require.Equal(t, int64(7), cache.watermark, "watermark=0 时保持原有行为")
	require.Equal(t, []int64{7}, cache.setWatermarks)
	require.Equal(t, 1, cache.applied[1])
	require.Equal(t, 1, cache.applied[2])
}

func TestSchedulerOutboxE02HeldGapDoesNotStarveNewEvents(t *testing.T) {
	cache := &e02GapCache{watermark: 9, applied: make(map[int64]int)}
	events := make([]SchedulerOutboxEvent, 0, 201)
	// id=10 缺失，随后是一整页（200 条）事件。
	for id := int64(11); id <= 210; id++ {
		events = append(events, e02LastUsedEvent(id, id))
	}
	repo := &e02GapRepo{events: events}
	svc := e02NewService(cache, repo)

	svc.pollOutbox()
	require.Equal(t, 1, cache.applied[210])

	repo.events = append(repo.events, e02LastUsedEvent(211, 211))
	svc.pollOutbox()

	require.Equal(t, 1, cache.applied[211], "缺口保持期间新事件不能被已处理的整页事件饿死")
	for id := int64(11); id <= 210; id++ {
		require.Equal(t, 1, cache.applied[id], "id=%d 只能处理一次", id)
	}
}

func TestSchedulerOutboxE02ExpiredGapIsSkipped(t *testing.T) {
	cache := &e02GapCache{watermark: 9, applied: make(map[int64]int)}
	repo := &e02GapRepo{events: []SchedulerOutboxEvent{e02LastUsedEvent(11, 8)}}
	svc := e02NewService(cache, repo)

	svc.pollOutbox()
	require.Equal(t, int64(9), cache.watermark)

	// 缺口超过等待期（回滚或 ON CONFLICT 消耗的 id）后不能一直卡住水位。
	svc.outboxGapSeenAt[10] = time.Now().Add(-outboxGapGrace - time.Second)
	svc.pollOutbox()

	require.Equal(t, int64(11), cache.watermark)
	require.Equal(t, 1, cache.applied[8], "跳过缺口时不得重复处理已处理的事件")
	require.Equal(t, []int64{9, 11}, repo.deleteCalls, "清理只能覆盖安全水位以内的行")

	// 下一轮按新水位裁剪状态，不再回看已越过的缺口。
	repo.events = append(repo.events, e02LastUsedEvent(12, 9))
	svc.pollOutbox()
	require.Equal(t, int64(12), cache.watermark)
	require.Equal(t, 1, cache.applied[9])
	require.Empty(t, svc.outboxHandledAhead)
	require.Empty(t, svc.outboxGapSeenAt)
}
