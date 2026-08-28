package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

// zhipuMCPSessionPrefix 智谱 MCP session 粘表的统一 key 前缀。
// 与 sticky_session: 分开命名空间，避免与模型转发的粘性会话清理逻辑互相影响。
const zhipuMCPSessionPrefix = "zhipu_mcp:session:"

// zhipuMCPCache service.ZhipuMCPSessionStore 的 Redis 实现。
// sessionID 的长度/字符校验由 service 层（sanitizeZhipuMCPSessionID）在拼 key 前完成。
type zhipuMCPCache struct {
	rdb *redis.Client
}

var _ service.ZhipuMCPSessionStore = (*zhipuMCPCache)(nil)

// NewZhipuMCPCache 创建智谱 MCP session 粘表缓存。
func NewZhipuMCPCache(rdb *redis.Client) service.ZhipuMCPSessionStore {
	return &zhipuMCPCache{rdb: rdb}
}

func buildZhipuMCPSessionKey(sessionID string) string {
	return fmt.Sprintf("%s%s", zhipuMCPSessionPrefix, sessionID)
}

func (c *zhipuMCPCache) SetZhipuMCPSession(ctx context.Context, sessionID string, accountID int64, ttl time.Duration) error {
	return c.rdb.Set(ctx, buildZhipuMCPSessionKey(sessionID), accountID, ttl).Err()
}

func (c *zhipuMCPCache) GetZhipuMCPSession(ctx context.Context, sessionID string) (int64, error) {
	accountID, err := c.rdb.Get(ctx, buildZhipuMCPSessionKey(sessionID)).Int64()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, service.ErrZhipuMCPSessionNotFound
		}
		return 0, err
	}
	return accountID, nil
}

func (c *zhipuMCPCache) DeleteZhipuMCPSession(ctx context.Context, sessionID string) error {
	return c.rdb.Del(ctx, buildZhipuMCPSessionKey(sessionID)).Err()
}
