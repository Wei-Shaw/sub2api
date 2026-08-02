package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const proxyHealthKeyPrefix = "proxy:health:"

func proxyHealthKey(proxyID int64) string {
	return fmt.Sprintf("%s%d", proxyHealthKeyPrefix, proxyID)
}

// proxyHealthCASScript writes ARGV[2] only when the stored JSON version equals
// ARGV[1] (or the key is missing and expected version is 0).
var proxyHealthCASScript = redis.NewScript(`
local expected = tonumber(ARGV[1])
local cur = redis.call("GET", KEYS[1])
if cur == false then
  if expected ~= 0 then
    return 0
  end
  redis.call("SET", KEYS[1], ARGV[2])
  return 1
end
local ok, obj = pcall(cjson.decode, cur)
if not ok or type(obj) ~= "table" then
  if expected ~= 0 then
    return 0
  end
  redis.call("SET", KEYS[1], ARGV[2])
  return 1
end
local ver = tonumber(obj["version"] or 0)
if ver ~= expected then
  return 0
end
redis.call("SET", KEYS[1], ARGV[2])
return 1
`)

type proxyHealthCache struct {
	rdb *redis.Client
}

// NewProxyHealthCache constructs a Redis-backed ProxyHealthCache.
func NewProxyHealthCache(rdb *redis.Client) service.ProxyHealthCache {
	return &proxyHealthCache{rdb: rdb}
}

func (c *proxyHealthCache) GetProxyHealth(ctx context.Context, proxyID int64) (*service.ProxyHealthMeta, error) {
	if c == nil || c.rdb == nil || proxyID <= 0 {
		return nil, nil
	}
	raw, err := c.rdb.Get(ctx, proxyHealthKey(proxyID)).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var meta service.ProxyHealthMeta
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func (c *proxyHealthCache) SetProxyHealth(ctx context.Context, proxyID int64, meta *service.ProxyHealthMeta) error {
	if c == nil || c.rdb == nil || meta == nil || proxyID <= 0 {
		return nil
	}
	payload, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	// No TTL: health counters must survive process restarts until thresholds fire.
	return c.rdb.Set(ctx, proxyHealthKey(proxyID), payload, 0).Err()
}

func (c *proxyHealthCache) CompareAndSetProxyHealth(ctx context.Context, proxyID int64, expectedVersion int64, meta *service.ProxyHealthMeta) (bool, error) {
	if c == nil || c.rdb == nil || meta == nil || proxyID <= 0 {
		return true, nil
	}
	payload, err := json.Marshal(meta)
	if err != nil {
		return false, err
	}
	res, err := proxyHealthCASScript.Run(ctx, c.rdb, []string{proxyHealthKey(proxyID)}, expectedVersion, string(payload)).Int()
	if err != nil {
		return false, err
	}
	return res == 1, nil
}
