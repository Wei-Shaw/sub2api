package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

func openAIStickySuccessCacheKey(groupID int64, sessionHash, model string) string {
	return buildSessionKey(groupID, "openai:success:"+sessionHash+":"+service.DeriveSessionHashFromSeed(model))
}

func (c *gatewayCache) GetOpenAIStickySuccess(ctx context.Context, groupID int64, sessionHash, model string) (service.OpenAIStickySuccessBinding, error) {
	var binding service.OpenAIStickySuccessBinding
	data, err := c.rdb.Get(ctx, openAIStickySuccessCacheKey(groupID, sessionHash, model)).Bytes()
	if errors.Is(err, redis.Nil) {
		return binding, service.ErrStickySessionNotFound
	}
	if err != nil {
		return binding, err
	}
	err = json.Unmarshal(data, &binding)
	return binding, err
}

var compareAndSwapOpenAIStickySuccess = redis.NewScript(`
local current = redis.call('GET', KEYS[1]) or ''
if current ~= ARGV[1] then return 0 end
redis.call('SET', KEYS[1], ARGV[2], 'PX', ARGV[3])
return 1
`)

func (c *gatewayCache) CompareAndSwapOpenAIStickySuccess(ctx context.Context, groupID int64, sessionHash, model string, expected, next service.OpenAIStickySuccessBinding, ttl time.Duration) (bool, error) {
	previous := ""
	if expected.AccountID > 0 {
		data, err := json.Marshal(expected)
		if err != nil {
			return false, err
		}
		previous = string(data)
	}
	data, err := json.Marshal(next)
	if err != nil {
		return false, err
	}
	changed, err := compareAndSwapOpenAIStickySuccess.Run(ctx, c.rdb, []string{openAIStickySuccessCacheKey(groupID, sessionHash, model)}, previous, string(data), ttl.Milliseconds()).Int()
	return changed == 1, err
}
