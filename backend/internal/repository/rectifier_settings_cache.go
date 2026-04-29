package repository

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	rectifierSettingsCacheKey = "rectifier:settings"
	// rectifierSettingsBaseTTL 基础 TTL，加上 ±jitter 给出实际过期窗口。
	// 故意不在读路径刷新——保证 value 从写入开始最多活一个 TTL 窗口，之后强制穿透 DB。
	rectifierSettingsBaseTTL = 60 * time.Second
	// rectifierSettingsJitter ±10s 随机抖动，防多实例同时过期穿透 DB。
	rectifierSettingsJitter = 10 * time.Second
)

type rectifierSettingsCache struct {
	rdb *redis.Client
}

// NewRectifierSettingsCache 基于 Redis 的 rectifier 设置缓存。
func NewRectifierSettingsCache(rdb *redis.Client) service.RectifierSettingsCache {
	return &rectifierSettingsCache{rdb: rdb}
}

// Get 读缓存。命中返回 (settings, true, nil)；未命中返回 (nil, false, nil)；
// Redis 错误返回 (nil, false, err)，上层应 fall through 到 DB。
func (c *rectifierSettingsCache) Get(ctx context.Context) (*service.RectifierSettings, bool, error) {
	data, err := c.rdb.Get(ctx, rectifierSettingsCacheKey).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var settings service.RectifierSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, false, err
	}
	return &settings, true, nil
}

// Set 写缓存。TTL = rectifierSettingsBaseTTL ± rectifierSettingsJitter 的随机值。
// 用 Set 而非 SetNX——缓存 miss 时读路径回填，最新值覆盖旧值是正确语义。
func (c *rectifierSettingsCache) Set(ctx context.Context, settings *service.RectifierSettings) error {
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, rectifierSettingsCacheKey, data, jitteredRectifierSettingsTTL()).Err()
}

// Invalidate 删 key。供写路径 (SetRectifierSettings) 主动失效用。
func (c *rectifierSettingsCache) Invalidate(ctx context.Context) error {
	return c.rdb.Del(ctx, rectifierSettingsCacheKey).Err()
}

// jitteredRectifierSettingsTTL 返回 base ± jitter 的随机 TTL。
// rand/v2 自带 goroutine-safe 全局源，无需 seeding。
func jitteredRectifierSettingsTTL() time.Duration {
	// rand.Int64N(2*jitter+1) 范围 [0, 2*jitter]，减 jitter 后范围 [-jitter, +jitter]
	offset := rand.Int64N(int64(2*rectifierSettingsJitter) + 1)
	return rectifierSettingsBaseTTL + time.Duration(offset) - rectifierSettingsJitter
}
