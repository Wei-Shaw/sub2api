package securityaudit

import (
	"context"
	"strings"

	"github.com/redis/go-redis/v9"
)

const promptAuditFlaggedHashSetKey = "prompt_audit:flagged_hashes"

// PromptAuditHashCache stores only prompt digests. Raw prompt text never enters
// Redis, and the namespace is separate from content moderation hashes.
type PromptAuditHashCache interface {
	RecordFlaggedPromptHash(context.Context, string) error
	HasFlaggedPromptHash(context.Context, string) (bool, error)
	DeleteFlaggedPromptHash(context.Context, string) (bool, error)
	ClearFlaggedPromptHashes(context.Context) (int64, error)
	CountFlaggedPromptHashes(context.Context) (int64, error)
}

type redisPromptAuditHashCache struct {
	rdb *redis.Client
}

func NewRedisPromptAuditHashCache(rdb *redis.Client) PromptAuditHashCache {
	return &redisPromptAuditHashCache{rdb: rdb}
}

func (c *redisPromptAuditHashCache) RecordFlaggedPromptHash(ctx context.Context, promptHash string) error {
	promptHash = strings.TrimSpace(promptHash)
	if c == nil || c.rdb == nil || promptHash == "" {
		return nil
	}
	return c.rdb.SAdd(ctx, promptAuditFlaggedHashSetKey, promptHash).Err()
}

func (c *redisPromptAuditHashCache) HasFlaggedPromptHash(ctx context.Context, promptHash string) (bool, error) {
	promptHash = strings.TrimSpace(promptHash)
	if c == nil || c.rdb == nil || promptHash == "" {
		return false, nil
	}
	return c.rdb.SIsMember(ctx, promptAuditFlaggedHashSetKey, promptHash).Result()
}

func (c *redisPromptAuditHashCache) DeleteFlaggedPromptHash(ctx context.Context, promptHash string) (bool, error) {
	promptHash = strings.TrimSpace(promptHash)
	if c == nil || c.rdb == nil || promptHash == "" {
		return false, nil
	}
	deleted, err := c.rdb.SRem(ctx, promptAuditFlaggedHashSetKey, promptHash).Result()
	if err != nil {
		return false, err
	}
	return deleted > 0, nil
}

func (c *redisPromptAuditHashCache) ClearFlaggedPromptHashes(ctx context.Context) (int64, error) {
	if c == nil || c.rdb == nil {
		return 0, nil
	}
	count, err := c.rdb.SCard(ctx, promptAuditFlaggedHashSetKey).Result()
	if err != nil {
		return 0, err
	}
	if count == 0 {
		return 0, nil
	}
	if err := c.rdb.Del(ctx, promptAuditFlaggedHashSetKey).Err(); err != nil {
		return 0, err
	}
	return count, nil
}

func (c *redisPromptAuditHashCache) CountFlaggedPromptHashes(ctx context.Context) (int64, error) {
	if c == nil || c.rdb == nil {
		return 0, nil
	}
	return c.rdb.SCard(ctx, promptAuditFlaggedHashSetKey).Result()
}
