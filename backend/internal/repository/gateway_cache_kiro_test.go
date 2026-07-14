//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// newKiroCacheTestNodes 返回两个共享同一 miniredis 的 gatewayCache 实例，
// 模拟多机部署下的两个 sub2api 节点（各自独立进程，但共享 Redis）。
func newKiroCacheTestNodes(t *testing.T) (*gatewayCache, *gatewayCache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	nodeA := &gatewayCache{rdb: redis.NewClient(&redis.Options{Addr: mr.Addr()})}
	nodeB := &gatewayCache{rdb: redis.NewClient(&redis.Options{Addr: mr.Addr()})}
	return nodeA, nodeB, mr
}

const kiroTestCacheKey uint64 = 0x1234abcd

// TestKiroCacheStoreProbeCrossInstance 验证核心目标：节点 A store 的前缀指纹，
// 节点 B 能 probe 命中——这正是多机部署下缓存模拟生效的前提。
func TestKiroCacheStoreProbeCrossInstance(t *testing.T) {
	nodeA, nodeB, _ := newKiroCacheTestNodes(t)
	ctx := context.Background()

	fps := []string{"fp_sys", "fp_msg1", "fp_msg2"}
	ttls := []time.Duration{5 * time.Minute, 5 * time.Minute, 5 * time.Minute}
	if err := nodeA.KiroCacheStore(ctx, kiroTestCacheKey, fps, ttls); err != nil {
		t.Fatalf("nodeA store: %v", err)
	}

	// 节点 B probe（newest-first：最后一个断点在最前）。
	idx, _, err := nodeB.KiroCacheProbe(ctx, kiroTestCacheKey, []string{"fp_msg2", "fp_msg1", "fp_sys"})
	if err != nil {
		t.Fatalf("nodeB probe: %v", err)
	}
	if idx != 1 {
		t.Fatalf("cross-instance hit expected at newest candidate (idx=1), got %d", idx)
	}
}

// TestKiroCacheProbeNewestFirstOrder 验证 probe 返回第一个存活候选的 1-based 下标，
// 且遵循调用方传入的 newest-first 顺序。
func TestKiroCacheProbeNewestFirstOrder(t *testing.T) {
	node, _, _ := newKiroCacheTestNodes(t)
	ctx := context.Background()

	// 只 store 了 fp_sys 与 fp_msg1，没有 fp_msg2。
	if err := node.KiroCacheStore(ctx, kiroTestCacheKey, []string{"fp_sys", "fp_msg1"}, []time.Duration{time.Minute, time.Minute}); err != nil {
		t.Fatalf("store: %v", err)
	}

	// 候选顺序 newest-first：fp_msg2(未存)→fp_msg1(命中)→fp_sys。
	idx, _, err := node.KiroCacheProbe(ctx, kiroTestCacheKey, []string{"fp_msg2", "fp_msg1", "fp_sys"})
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if idx != 2 {
		t.Fatalf("expected first live candidate at idx=2 (fp_msg1), got %d", idx)
	}
}

// TestKiroCacheProbeMiss 验证无任何存活指纹时返回 0。
func TestKiroCacheProbeMiss(t *testing.T) {
	node, _, _ := newKiroCacheTestNodes(t)
	ctx := context.Background()

	idx, _, err := node.KiroCacheProbe(ctx, kiroTestCacheKey, []string{"nope1", "nope2"})
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if idx != 0 {
		t.Fatalf("expected miss (0), got %d", idx)
	}

	// 空候选也应安全返回 0。
	if idx, _, err := node.KiroCacheProbe(ctx, kiroTestCacheKey, nil); err != nil || idx != 0 {
		t.Fatalf("empty candidates: idx=%d err=%v", idx, err)
	}
}

// TestKiroCacheProbeRefreshesTTL 验证命中 probe 会把该 key 滚动续期到其存储的 TTL
// （对齐内存版 compute 的 entry.expiresAt = now + entry.ttl）。
func TestKiroCacheProbeRefreshesTTL(t *testing.T) {
	node, _, mr := newKiroCacheTestNodes(t)
	ctx := context.Background()

	ttl := 5 * time.Minute
	if err := node.KiroCacheStore(ctx, kiroTestCacheKey, []string{"fp"}, []time.Duration{ttl}); err != nil {
		t.Fatalf("store: %v", err)
	}
	key := kiroCacheEmulKey(kiroTestCacheKey, "fp")

	// 快进 4 分钟，key 仍存活但剩 ~1 分钟。
	mr.FastForward(4 * time.Minute)
	if !mr.Exists(key) {
		t.Fatalf("key should still be alive before probe")
	}
	_, remainTTLms, err := node.KiroCacheProbe(ctx, kiroTestCacheKey, []string{"fp"})
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	// probe 应回传「续期前」的剩余 TTL（此时约 1 分钟），用于排障续期是否生效。
	if remainTTLms <= 0 || remainTTLms > int64((5*time.Minute).Milliseconds()) {
		t.Fatalf("expected pre-refresh remaining TTL in (0, 5m], got %dms", remainTTLms)
	}
	if remainTTLms > int64((2 * time.Minute).Milliseconds()) {
		t.Fatalf("pre-refresh remaining TTL should be ~1m (after 4m fast-forward), got %dms", remainTTLms)
	}
	// 续期后剩余 TTL 应回到接近 5 分钟（远大于续期前的 ~1 分钟）。
	got := mr.TTL(key)
	if got < 4*time.Minute {
		t.Fatalf("probe should roll TTL back to ~5m, got %v", got)
	}
}

// TestKiroCacheStoreMaxTTLMerge 验证 store 对已存在 key 做 max-TTL 合并：
// 后到的较短 TTL 不会缩短已有的较长过期（对齐内存版 update 的 max 语义）。
func TestKiroCacheStoreMaxTTLMerge(t *testing.T) {
	node, _, mr := newKiroCacheTestNodes(t)
	ctx := context.Background()
	key := kiroCacheEmulKey(kiroTestCacheKey, "fp")

	// 先写 1h。
	if err := node.KiroCacheStore(ctx, kiroTestCacheKey, []string{"fp"}, []time.Duration{time.Hour}); err != nil {
		t.Fatalf("store 1h: %v", err)
	}
	// 再写 5m —— 不应把过期缩短到 5m。
	if err := node.KiroCacheStore(ctx, kiroTestCacheKey, []string{"fp"}, []time.Duration{5 * time.Minute}); err != nil {
		t.Fatalf("store 5m: %v", err)
	}
	if got := mr.TTL(key); got <= 5*time.Minute {
		t.Fatalf("max-TTL merge: expected remaining >5m (kept 1h), got %v", got)
	}

	// 反向：已存在 5m，写入 1h 应延长到 1h。
	key2 := kiroCacheEmulKey(kiroTestCacheKey, "fp2")
	if err := node.KiroCacheStore(ctx, kiroTestCacheKey, []string{"fp2"}, []time.Duration{5 * time.Minute}); err != nil {
		t.Fatalf("store fp2 5m: %v", err)
	}
	if err := node.KiroCacheStore(ctx, kiroTestCacheKey, []string{"fp2"}, []time.Duration{time.Hour}); err != nil {
		t.Fatalf("store fp2 1h: %v", err)
	}
	if got := mr.TTL(key2); got <= 5*time.Minute {
		t.Fatalf("max-TTL merge: expected extended to ~1h, got %v", got)
	}
}

// TestKiroCacheExpiry 验证过期由 Redis 原生管理：TTL 过后 probe 落空。
func TestKiroCacheExpiry(t *testing.T) {
	node, _, mr := newKiroCacheTestNodes(t)
	ctx := context.Background()

	if err := node.KiroCacheStore(ctx, kiroTestCacheKey, []string{"fp"}, []time.Duration{5 * time.Minute}); err != nil {
		t.Fatalf("store: %v", err)
	}
	mr.FastForward(6 * time.Minute)
	idx, _, err := node.KiroCacheProbe(ctx, kiroTestCacheKey, []string{"fp"})
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if idx != 0 {
		t.Fatalf("expired entry should miss, got idx=%d", idx)
	}
}

// TestKiroCacheCredentialIsolation 验证不同 credentialKey 互不干扰。
func TestKiroCacheCredentialIsolation(t *testing.T) {
	node, _, _ := newKiroCacheTestNodes(t)
	ctx := context.Background()

	if err := node.KiroCacheStore(ctx, 111, []string{"fp"}, []time.Duration{time.Minute}); err != nil {
		t.Fatalf("store cred 111: %v", err)
	}
	if idx, _, err := node.KiroCacheProbe(ctx, 222, []string{"fp"}); err != nil || idx != 0 {
		t.Fatalf("different credential must not hit: idx=%d err=%v", idx, err)
	}
	if idx, _, err := node.KiroCacheProbe(ctx, 111, []string{"fp"}); err != nil || idx != 1 {
		t.Fatalf("same credential should hit: idx=%d err=%v", idx, err)
	}
}
