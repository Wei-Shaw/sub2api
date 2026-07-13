package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const stickySessionPrefix = "sticky_session:"

type gatewayCache struct {
	rdb *redis.Client
}

func NewGatewayCache(rdb *redis.Client) service.GatewayCache {
	return &gatewayCache{rdb: rdb}
}

// buildSessionKey 构建 session key，包含 groupID 实现分组隔离
// 格式: sticky_session:{groupID}:{sessionHash}
func buildSessionKey(groupID int64, sessionHash string) string {
	return fmt.Sprintf("%s%d:%s", stickySessionPrefix, groupID, sessionHash)
}

func (c *gatewayCache) GetSessionAccountID(ctx context.Context, groupID int64, sessionHash string) (int64, error) {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Get(ctx, key).Int64()
}

func (c *gatewayCache) SetSessionAccountID(ctx context.Context, groupID int64, sessionHash string, accountID int64, ttl time.Duration) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Set(ctx, key, accountID, ttl).Err()
}

func (c *gatewayCache) RefreshSessionTTL(ctx context.Context, groupID int64, sessionHash string, ttl time.Duration) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Expire(ctx, key, ttl).Err()
}

// DeleteSessionAccountID 删除粘性会话与账号的绑定关系。
// 当检测到绑定的账号不可用（如状态错误、禁用、不可调度等）时调用，
// 以便下次请求能够重新选择可用账号。
//
// DeleteSessionAccountID removes the sticky session binding for the given session.
// Called when the bound account becomes unavailable (e.g., error status, disabled,
// or unschedulable), allowing subsequent requests to select a new available account.
func (c *gatewayCache) DeleteSessionAccountID(ctx context.Context, groupID int64, sessionHash string) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Del(ctx, key).Err()
}

// Compile-time assertion: gatewayCache must implement CyberSessionBlockStore.
var _ service.CyberSessionBlockStore = (*gatewayCache)(nil)

const cyberSessionBlockPrefix = "cyber_session_block:"

// SetCyberSessionBlocked 把被 cyber_policy 命中的会话写入屏蔽表（TTL 自动过期）。
// 存储值 "1" 作为存在标记（IsCyberSessionBlocked 只检查 key 是否存在，不读值）。
func (c *gatewayCache) SetCyberSessionBlocked(ctx context.Context, key string, ttl time.Duration) error {
	return c.rdb.Set(ctx, cyberSessionBlockPrefix+key, "1", ttl).Err()
}

// IsCyberSessionBlocked 查询会话是否在屏蔽表中。
func (c *gatewayCache) IsCyberSessionBlocked(ctx context.Context, key string) (bool, error) {
	n, err := c.rdb.Exists(ctx, cyberSessionBlockPrefix+key).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// ── Kiro cache-emulation shared store ───────────────────────────────────────
// 为多机部署提供跨实例的 Kiro 前缀缓存命中记账。key 形如
// kiro_cache_emul:{credentialKey}:{fingerprintHex}，value 存该断点的 TTL 毫秒数
// （用于命中时的滚动续期），过期由 Redis 原生管理。语义对齐进程内 tracker：
// probe 命中即把该 key 续期到其存储的 TTL；store 对每个 fingerprint 做 max-TTL 合并。

const kiroCacheEmulPrefix = "kiro_cache_emul:"

// kiroCacheProbeScript 按 KEYS 顺序（调用方传入 newest-first）查找第一个仍存活的
// fingerprint：命中则用其存储的 TTL 值滚动续期，并返回其 1-based 下标；否则返回 0。
var kiroCacheProbeScript = redis.NewScript(`
for i, key in ipairs(KEYS) do
  local ttl = redis.call('GET', key)
  if ttl then
    local ms = tonumber(ttl)
    if ms and ms > 0 then
      redis.call('PEXPIRE', key, ms)
    end
    return i
  end
end
return 0
`)

// kiroCacheStoreScript 对每个 KEYS[i]（TTL 毫秒数为 ARGV[i]）做 max-TTL upsert：
// 存储值取 max(旧存储TTL, 新TTL)，实际过期取 max(旧剩余PTTL, 新TTL)。
var kiroCacheStoreScript = redis.NewScript(`
for i, key in ipairs(KEYS) do
  local incoming = tonumber(ARGV[i])
  if incoming and incoming > 0 then
    local existingTtl = tonumber(redis.call('GET', key)) or 0
    local existingPttl = redis.call('PTTL', key)
    if existingPttl == nil or existingPttl < 0 then existingPttl = 0 end
    local newTtl = incoming
    if existingTtl > newTtl then newTtl = existingTtl end
    local newPttl = incoming
    if existingPttl > newPttl then newPttl = existingPttl end
    redis.call('SET', key, newTtl, 'PX', newPttl)
  end
end
return 1
`)

func kiroCacheEmulKey(cacheKey uint64, fingerprintHex string) string {
	return fmt.Sprintf("%s%d:%s", kiroCacheEmulPrefix, cacheKey, fingerprintHex)
}

// KiroCacheProbe implements service.kiroCacheEmulationStore.
func (c *gatewayCache) KiroCacheProbe(ctx context.Context, cacheKey uint64, fingerprintsNewestFirst []string) (int, error) {
	if len(fingerprintsNewestFirst) == 0 {
		return 0, nil
	}
	keys := make([]string, len(fingerprintsNewestFirst))
	for i, fp := range fingerprintsNewestFirst {
		keys[i] = kiroCacheEmulKey(cacheKey, fp)
	}
	idx, err := kiroCacheProbeScript.Run(ctx, c.rdb, keys).Int()
	if err != nil {
		return 0, err
	}
	return idx, nil
}

// KiroCacheStore implements service.kiroCacheEmulationStore.
func (c *gatewayCache) KiroCacheStore(ctx context.Context, cacheKey uint64, fingerprints []string, ttls []time.Duration) error {
	if len(fingerprints) == 0 || len(fingerprints) != len(ttls) {
		return nil
	}
	keys := make([]string, len(fingerprints))
	args := make([]any, len(fingerprints))
	for i, fp := range fingerprints {
		keys[i] = kiroCacheEmulKey(cacheKey, fp)
		args[i] = ttls[i].Milliseconds()
	}
	return kiroCacheStoreScript.Run(ctx, c.rdb, keys, args...).Err()
}
