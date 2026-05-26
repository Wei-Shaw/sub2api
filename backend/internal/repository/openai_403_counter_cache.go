package repository

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const openAI403CounterPrefix = "openai_403_count:account:"

var openAI403CounterIncrScript = redis.NewScript(`
	local key = KEYS[1]
	local ttl = tonumber(ARGV[1])

	local count = redis.call('INCR', key)
	if count == 1 then
		redis.call('EXPIRE', key, ttl)
	end

	return count
`)

type openAI403CounterCache struct {
	rdb *redis.Client
}

func NewOpenAI403CounterCache(rdb *redis.Client) service.OpenAI403CounterCache {
	return &openAI403CounterCache{rdb: rdb}
}

func (c *openAI403CounterCache) IncrementOpenAI403Count(ctx context.Context, accountID int64, windowMinutes int) (int64, error) {
	key := fmt.Sprintf("%s%d", openAI403CounterPrefix, accountID)

	ttlSeconds := windowMinutes * 60
	if ttlSeconds < 60 {
		ttlSeconds = 60
	}

	result, err := openAI403CounterIncrScript.Run(ctx, c.rdb, []string{key}, ttlSeconds).Int64()
	if err != nil {
		return 0, fmt.Errorf("increment openai 403 count: %w", err)
	}
	return result, nil
}

func (c *openAI403CounterCache) ResetOpenAI403Count(ctx context.Context, accountID int64) error {
	key := fmt.Sprintf("%s%d", openAI403CounterPrefix, accountID)
	return c.rdb.Del(ctx, key).Err()
}
