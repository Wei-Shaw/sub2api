//go:build unit

package inbox

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("启动 miniredis 失败: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb, mr
}

// TestLeaderGuard_SingleAcquirePerCycle 验证同一周期内只有第一次 TryAcquire 成功，
// 后续（模拟其它副本）失败，直到锁 TTL 过期后重新可抢。
func TestLeaderGuard_SingleAcquirePerCycle(t *testing.T) {
	rdb, mr := newTestRedis(t)
	ctx := context.Background()
	g := NewCleanupLeaderGuard(rdb)

	if !g.TryAcquire(ctx) {
		t.Fatal("首次 TryAcquire 应成功")
	}
	if g.TryAcquire(ctx) {
		t.Fatal("同一周期内第二次 TryAcquire 应失败（锁被持有）")
	}

	// 快进超过 TTL，锁自动过期，重新可抢。
	mr.FastForward(cleanupLockTTL + 1)
	if !g.TryAcquire(ctx) {
		t.Fatal("锁过期后 TryAcquire 应重新成功")
	}
}

// TestLeaderGuard_NilRedisAlwaysAcquires 验证无 Redis（单副本）时守卫恒放行。
func TestLeaderGuard_NilRedisAlwaysAcquires(t *testing.T) {
	g := NewCleanupLeaderGuard(nil)
	for i := 0; i < 3; i++ {
		if !g.TryAcquire(context.Background()) {
			t.Fatalf("nil redis 时第 %d 次 TryAcquire 应恒为 true", i)
		}
	}
}

// TestLeaderGuard_RedisErrorFailsClosed 验证 Redis 不可用时 fail-closed（不执行）。
func TestLeaderGuard_RedisErrorFailsClosed(t *testing.T) {
	rdb, mr := newTestRedis(t)
	g := NewCleanupLeaderGuard(rdb)
	mr.Close() // 关闭后所有命令报错

	if g.TryAcquire(context.Background()) {
		t.Fatal("Redis 出错时 TryAcquire 应返回 false（fail-closed）")
	}
}
