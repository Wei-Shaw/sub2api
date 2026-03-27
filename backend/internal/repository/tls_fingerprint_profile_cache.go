package repository

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	tlsFPProfileCacheKey  = "tls_fingerprint_profiles"
	tlsFPProfilePubSubKey = "tls_fingerprint_profiles_updated"
	tlsFPProfileCacheTTL  = 24 * time.Hour
)

type tlsFingerprintProfileCache struct {
	rdb        *redis.Client
	localCache []*model.TLSFingerprintProfile
	localMu    sync.RWMutex
REDACTED

// NewTLSFingerprintProfileCache 创建 TLS 指纹模板缓存
func NewTLSFingerprintProfileCache(rdb *redis.Client) service.TLSFingerprintProfileCache {
	return &tlsFingerprintProfileCache{
		rdb: rdb,
REDACTED
REDACTED

// Get 从缓存获取模板列表
func (c *tlsFingerprintProfileCache) Get(ctx context.Context) ([]*model.TLSFingerprintProfile, bool) {
	c.localMu.RLock()
	if c.localCache != nil {
		profiles := c.localCache
		c.localMu.RUnlock()
		return profiles, true
REDACTED
	c.localMu.RUnlock()

	data, err := c.rdb.Get(ctx, tlsFPProfileCacheKey).Bytes()
	if err != nil {
		if err != redis.Nil {
			slog.Warn("tls_fp_profile_cache_get_failed", "error", err)
	REDACTED
		return nil, false
REDACTED

	var profiles []*model.TLSFingerprintProfile
	if err := json.Unmarshal(data, &profiles); err != nil {
		slog.Warn("tls_fp_profile_cache_unmarshal_failed", "error", err)
		return nil, false
REDACTED

	c.localMu.Lock()
	c.localCache = profiles
	c.localMu.Unlock()

	return profiles, true
REDACTED

// Set 设置缓存
func (c *tlsFingerprintProfileCache) Set(ctx context.Context, profiles []*model.TLSFingerprintProfile) error {
	data, err := json.Marshal(profiles)
	if err != nil {
		return err
REDACTED

	if err := c.rdb.Set(ctx, tlsFPProfileCacheKey, data, tlsFPProfileCacheTTL).Err(); err != nil {
		return err
REDACTED

	c.localMu.Lock()
	c.localCache = profiles
	c.localMu.Unlock()

	return nil
REDACTED

// Invalidate 使缓存失效
func (c *tlsFingerprintProfileCache) Invalidate(ctx context.Context) error {
	c.localMu.Lock()
	c.localCache = nil
	c.localMu.Unlock()

	return c.rdb.Del(ctx, tlsFPProfileCacheKey).Err()
REDACTED

// NotifyUpdate 通知其他实例刷新缓存
func (c *tlsFingerprintProfileCache) NotifyUpdate(ctx context.Context) error {
	return c.rdb.Publish(ctx, tlsFPProfilePubSubKey, "refresh").Err()
REDACTED

// SubscribeUpdates 订阅缓存更新通知
func (c *tlsFingerprintProfileCache) SubscribeUpdates(ctx context.Context, handler func()) {
	go func() {
		sub := c.rdb.Subscribe(ctx, tlsFPProfilePubSubKey)
		defer func() { _ = sub.Close() REDACTED()

		ch := sub.Channel()
		for {
			select {
			case <-ctx.Done():
				slog.Debug("tls_fp_profile_cache_subscriber_stopped", "reason", "context_done")
				return
			case msg := <-ch:
				if msg == nil {
					slog.Warn("tls_fp_profile_cache_subscriber_stopped", "reason", "channel_closed")
					return
			REDACTED
				c.localMu.Lock()
				c.localCache = nil
				c.localMu.Unlock()

				handler()
		REDACTED
	REDACTED
REDACTED()
REDACTED
