package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	stickySessionPrefix = "sticky_session:"
)

type gatewayCache struct {
	rdb *redis.Client
REDACTED

func NewGatewayCache(rdb *redis.Client) service.GatewayCache {
	return &gatewayCache{rdb: rdbREDACTED
REDACTED

// buildSessionKey 构建 session key，包含 groupID 实现分组隔离
// 格式: sticky_session:{groupIDREDACTED:{sessionHashREDACTED
func buildSessionKey(groupID int64, sessionHash string) string {
	return fmt.Sprintf("%s%d:%s", stickySessionPrefix, groupID, sessionHash)
REDACTED

func (c *gatewayCache) GetSessionAccountID(ctx context.Context, groupID int64, sessionHash string) (int64, error) {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Get(ctx, key).Int64()
REDACTED

func (c *gatewayCache) SetSessionAccountID(ctx context.Context, groupID int64, sessionHash string, accountID int64, ttl time.Duration) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Set(ctx, key, accountID, ttl).Err()
REDACTED

func (c *gatewayCache) RefreshSessionTTL(ctx context.Context, groupID int64, sessionHash string, ttl time.Duration) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Expire(ctx, key, ttl).Err()
REDACTED

// DeleteSessionAccountID 删除粘性会话与账号的绑定关系。
func (c *gatewayCache) DeleteSessionAccountID(ctx context.Context, groupID int64, sessionHash string) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Del(ctx, key).Err()
REDACTED
