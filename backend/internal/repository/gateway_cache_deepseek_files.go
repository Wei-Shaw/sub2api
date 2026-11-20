package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

var _ service.DeepseekFileStore = (*gatewayCache)(nil)

// Preserve known ownership from earlier uploads. The old two-field value lacked
// a gateway user and is deliberately never accepted as proof of ownership.
const deepseekFileOwnerPrefix = "deepseek_file_owner:"

func deepseekFileOwner(userID, accountID int64) string {
	return fmt.Sprintf("%d|%d|%s", userID, accountID, service.PlatformDeepseek)
}

func deepseekUserFileIndex(userID int64) string {
	return "deepseek_files:" + strconv.FormatInt(userID, 10)
}

func deepseekFileKeys(userID, accountID int64, id string) []string {
	index := deepseekUserFileIndex(userID)
	legacy := deepseekFileOwnerPrefix + id
	return []string{legacy + ":" + strconv.FormatInt(accountID, 10), index + ":" + id, index, legacy}
}

// Keep the index until the last temporary file expires. Permanent files have an infinite score.
const expireDeepseekFileIndex = `
local last = redis.call('ZREVRANGE', KEYS[3], 0, 0, 'WITHSCORES')
if last[2] == 'inf' or last[2] == '+inf' then
  redis.call('PERSIST', KEYS[3])
elseif last[2] then
  redis.call('EXPIREAT', KEYS[3], last[2])
end
return 1
`

var saveDeepseekFile = redis.NewScript(`
local owner = redis.call('GET', KEYS[1])
local legacy = redis.call('GET', KEYS[4])
if (owner and owner ~= ARGV[1]) or (legacy and legacy ~= ARGV[1]) then return 0 end
local previous = redis.call('GET', KEYS[2])
if previous and cjson.decode(previous).account_id ~= ARGV[6] then return 0 end
redis.call('SET', KEYS[1], ARGV[1])
redis.call('SET', KEYS[2], ARGV[2])
if legacy then redis.call('DEL', KEYS[4]) end
local deadline = ARGV[4]
if deadline ~= '+inf' then
  redis.call('EXPIREAT', KEYS[1], deadline)
  redis.call('EXPIREAT', KEYS[2], deadline)
end
redis.call('ZADD', KEYS[3], deadline, ARGV[3])
redis.call('ZREMRANGEBYSCORE', KEYS[3], '-inf', ARGV[5])
` + expireDeepseekFileIndex)

func (c *gatewayCache) SaveDeepseekFile(ctx context.Context, userID int64, file service.DeepseekFile) error {
	if userID <= 0 || file.AccountID <= 0 || file.ID == "" {
		return errors.New("invalid DeepSeek file owner")
	}
	data, err := json.Marshal(file)
	if err != nil {
		return err
	}
	deadline := "+inf"
	if file.ExpiresAt != 0 {
		deadline = strconv.FormatInt(time.Unix(file.ExpiresAt, 0).Add(24*time.Hour).Unix(), 10)
	}
	ok, err := saveDeepseekFile.Run(ctx, c.rdb, deepseekFileKeys(userID, file.AccountID, file.ID),
		deepseekFileOwner(userID, file.AccountID), data, file.ID, deadline, time.Now().Unix(), file.AccountID).Int()
	if err == nil && ok != 1 {
		return errors.New("DeepSeek file ID already bound to another owner")
	}
	return err
}

func (c *gatewayCache) DeepseekFileAccounts(ctx context.Context, userID int64, ids []string) (map[string]int64, error) {
	accounts := make(map[string]int64, len(ids))
	if userID <= 0 || len(ids) == 0 {
		return accounts, nil
	}
	// Read user-scoped records and any pre-index bindings in a single Redis round trip.
	index := deepseekUserFileIndex(userID)
	keys := make([]string, 0, 2*len(ids))
	for _, id := range ids {
		keys = append(keys, index+":"+id, deepseekFileOwnerPrefix+id)
	}
	values, err := c.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	owner := strconv.FormatInt(userID, 10)
	for i, id := range ids {
		if raw, ok := values[2*i].(string); ok {
			var file service.DeepseekFile
			if err := json.Unmarshal([]byte(raw), &file); err != nil {
				return nil, err
			}
			accounts[id] = file.AccountID
			continue
		}
		raw, _ := values[2*i+1].(string)
		fields := strings.SplitN(raw, "|", 3)
		if len(fields) == 3 && fields[0] == owner && fields[2] == service.PlatformDeepseek {
			if accountID, err := strconv.ParseInt(fields[1], 10, 64); err == nil && accountID > 0 {
				accounts[id] = accountID
			}
		}
	}
	return accounts, nil
}

func (c *gatewayCache) ListDeepseekFiles(ctx context.Context, userID int64) ([]service.DeepseekFile, error) {
	index := deepseekUserFileIndex(userID)
	pipe := c.rdb.TxPipeline()
	pipe.ZRemRangeByScore(ctx, index, "-inf", strconv.FormatInt(time.Now().Unix(), 10))
	members := pipe.ZRange(ctx, index, 0, -1)
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}
	keys := members.Val()
	if len(keys) == 0 {
		return nil, nil
	}
	for i, id := range keys {
		keys[i] = index + ":" + id
	}
	values, err := c.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	files := make([]service.DeepseekFile, 0, len(values))
	for _, value := range values {
		if raw, ok := value.(string); ok {
			var file service.DeepseekFile
			if err := json.Unmarshal([]byte(raw), &file); err != nil {
				return nil, err
			}
			files = append(files, file)
		}
	}
	return files, nil
}

var deleteDeepseekFile = redis.NewScript(`
local owner = redis.call('GET', KEYS[1])
if owner and owner ~= ARGV[1] then return 0 end
local current = redis.call('GET', KEYS[2])
if current and cjson.decode(current).account_id ~= ARGV[3] then return 0 end
if redis.call('GET', KEYS[4]) == ARGV[1] then redis.call('DEL', KEYS[4]) end
redis.call('DEL', KEYS[1], KEYS[2])
redis.call('ZREM', KEYS[3], ARGV[2])
` + expireDeepseekFileIndex)

func (c *gatewayCache) DeleteDeepseekFile(ctx context.Context, userID, accountID int64, id string) error {
	return deleteDeepseekFile.Run(ctx, c.rdb, deepseekFileKeys(userID, accountID, id),
		deepseekFileOwner(userID, accountID), id, accountID).Err()
}
