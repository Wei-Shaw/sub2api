package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const stickySessionPrefix = "sticky_session:"

const (
	openAIWSSessionTurnStatePrefix    = "openai_ws:turn_state:"
	openAIWSResponseConnBindingPrefix = "openai_ws:response_conn:"
	openAIWSSessionConnBindingPrefix  = "openai_ws:session_conn:"
)

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

func buildScopedCacheKey(prefix string, groupID int64, key string) string {
	return fmt.Sprintf("%s%d:%s", prefix, groupID, key)
}

func buildGlobalCacheKey(prefix string, key string) string {
	return prefix + key
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

func (c *gatewayCache) SetOpenAIWSSessionTurnState(ctx context.Context, groupID int64, sessionHash, turnState string, ttl time.Duration) error {
	return c.rdb.Set(ctx, buildScopedCacheKey(openAIWSSessionTurnStatePrefix, groupID, sessionHash), turnState, ttl).Err()
}

func (c *gatewayCache) GetOpenAIWSSessionTurnState(ctx context.Context, groupID int64, sessionHash string) (string, error) {
	return c.rdb.Get(ctx, buildScopedCacheKey(openAIWSSessionTurnStatePrefix, groupID, sessionHash)).Result()
}

func (c *gatewayCache) DeleteOpenAIWSSessionTurnState(ctx context.Context, groupID int64, sessionHash string) error {
	return c.rdb.Del(ctx, buildScopedCacheKey(openAIWSSessionTurnStatePrefix, groupID, sessionHash)).Err()
}

func (c *gatewayCache) SetOpenAIWSResponseConnBinding(ctx context.Context, responseID, binding string, ttl time.Duration) error {
	return c.rdb.Set(ctx, buildGlobalCacheKey(openAIWSResponseConnBindingPrefix, responseID), binding, ttl).Err()
}

func (c *gatewayCache) GetOpenAIWSResponseConnBinding(ctx context.Context, responseID string) (string, error) {
	return c.rdb.Get(ctx, buildGlobalCacheKey(openAIWSResponseConnBindingPrefix, responseID)).Result()
}

func (c *gatewayCache) DeleteOpenAIWSResponseConnBinding(ctx context.Context, responseID string) error {
	return c.rdb.Del(ctx, buildGlobalCacheKey(openAIWSResponseConnBindingPrefix, responseID)).Err()
}

func (c *gatewayCache) SetOpenAIWSSessionConnBinding(ctx context.Context, groupID int64, sessionHash, binding string, ttl time.Duration) error {
	return c.rdb.Set(ctx, buildScopedCacheKey(openAIWSSessionConnBindingPrefix, groupID, sessionHash), binding, ttl).Err()
}

func (c *gatewayCache) GetOpenAIWSSessionConnBinding(ctx context.Context, groupID int64, sessionHash string) (string, error) {
	return c.rdb.Get(ctx, buildScopedCacheKey(openAIWSSessionConnBindingPrefix, groupID, sessionHash)).Result()
}

func (c *gatewayCache) DeleteOpenAIWSSessionConnBinding(ctx context.Context, groupID int64, sessionHash string) error {
	return c.rdb.Del(ctx, buildScopedCacheKey(openAIWSSessionConnBindingPrefix, groupID, sessionHash)).Err()
}
