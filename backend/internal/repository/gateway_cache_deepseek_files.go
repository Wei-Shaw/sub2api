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
	return []string{legacy + ":" + strconv.FormatInt(accountID, 10), index + ":" + id, index, legacy, index + ":order:", index + ":expiry:"}
}

type indexedDeepseekFile struct {
	service.DeepseekFile
	Order string `json:"order"`
}

func fileIndexMember(f service.DeepseekFile) string {
	return f.CreatedAt.UTC().Format("20060102150405.000000000") + ":" + f.ID
}

// Scores on the lifetime index are binding deadlines. The order indexes use
// equal scores and fixed-width UTC timestamps so lex ranges handle timestamp ties.
const expireDeepseekFileIndex = `
local last = redis.call('ZREVRANGE', KEYS[3], 0, 0, 'WITHSCORES')
for _, key in ipairs({KEYS[3], KEYS[5], KEYS[5]..'user_data', KEYS[5]..'text_recognition', KEYS[5]..'rich_layout_recognition', KEYS[6]}) do
  if not last[2] then redis.call('DEL', key)
  elseif last[2] == 'inf' or last[2] == '+inf' then redis.call('PERSIST', key)
  else redis.call('EXPIREAT', key, last[2]) end
end
return 1
`

const removeOldOrder = `
local previous = redis.call('GET', KEYS[2])
if previous then
  local f = cjson.decode(previous)
  if f.account_id ~= ARGV[2] then return 0 end
  if f.order then
    if ARGV[9] and f.Origin ~= '' and f.Origin ~= ARGV[9] then return 0 end
    redis.call('ZREM', KEYS[6], f.order..'|'..f.Purpose)
    redis.call('ZREM', KEYS[5], f.order)
    if f.Purpose ~= '' then redis.call('ZREM', KEYS[5]..f.Purpose, f.order) end
  end
end
`

// Reclaim order members even when temporary and permanent files share an index.
// Each write or list does a bounded amount of housekeeping.
const pruneDeepseekOrder = `
local function pruneExpired(orderIndex, expiryIndex, now)
  local expired = redis.call('ZRANGEBYSCORE', expiryIndex, '-inf', now, 'LIMIT', 0, 128)
  for _, value in ipairs(expired) do
    local split = string.find(value, '|', 1, true)
    local member, purpose = string.sub(value, 1, split-1), string.sub(value, split+1)
    redis.call('ZREM', orderIndex, member)
    if purpose ~= '' then redis.call('ZREM', orderIndex..purpose, member) end
    redis.call('ZREM', expiryIndex, value)
  end
end
`

var pruneDeepseekFilePage = redis.NewScript(pruneDeepseekOrder + "pruneExpired(KEYS[1], KEYS[2], ARGV[1]); return 1")

var saveDeepseekFile = redis.NewScript(`
local owner = redis.call('GET', KEYS[1])
local legacy = redis.call('GET', KEYS[4])
if (owner and owner ~= ARGV[1]) or (legacy and legacy ~= ARGV[1]) then return 0 end

` + removeOldOrder + pruneDeepseekOrder + `
pruneExpired(KEYS[5], KEYS[6], ARGV[6])
redis.call('SET', KEYS[1], ARGV[1])
redis.call('SET', KEYS[2], ARGV[3])
if legacy then redis.call('DEL', KEYS[4]) end
if ARGV[4] ~= '+inf' then
  redis.call('EXPIREAT', KEYS[1], ARGV[4])
  redis.call('EXPIREAT', KEYS[2], ARGV[4])
  redis.call('ZADD', KEYS[6], ARGV[10], ARGV[7]..'|'..ARGV[8])
end
redis.call('ZADD', KEYS[3], ARGV[4], ARGV[5])
redis.call('ZREMRANGEBYSCORE', KEYS[3], '-inf', ARGV[6])
redis.call('ZADD', KEYS[5], 0, ARGV[7])
if ARGV[8] ~= '' then redis.call('ZADD', KEYS[5]..ARGV[8], 0, ARGV[7]) end
` + expireDeepseekFileIndex)

func (c *gatewayCache) SaveDeepseekFile(ctx context.Context, userID int64, file service.DeepseekFile) error {
	if userID <= 0 || file.AccountID <= 0 || file.ID == "" || file.Origin == "" {
		return errors.New("invalid DeepSeek file owner or origin")
	}
	member := fileIndexMember(file)
	data, err := json.Marshal(indexedDeepseekFile{file, member})
	if err != nil {
		return err
	}
	deadline := "+inf"
	if file.ExpiresAt != 0 {
		deadline = strconv.FormatInt(time.Unix(file.ExpiresAt, 0).Add(24*time.Hour).Unix(), 10)
	}
	ok, err := saveDeepseekFile.Run(ctx, c.rdb, deepseekFileKeys(userID, file.AccountID, file.ID),
		deepseekFileOwner(userID, file.AccountID), file.AccountID, data, deadline, file.ID, time.Now().Unix(),
		member, file.Purpose, file.Origin, file.ExpiresAt).Int()
	if err == nil && ok != 1 {
		return errors.New("DeepSeek file binding changed")
	}
	return err
}

func (c *gatewayCache) LookupDeepseekFiles(ctx context.Context, userID int64, ids []string) (map[string]service.DeepseekFile, error) {
	files := make(map[string]service.DeepseekFile, len(ids))
	if userID <= 0 || len(ids) == 0 {
		return files, nil
	}
	index := deepseekUserFileIndex(userID)
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = index + ":" + id
	}
	values, err := c.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	for i, value := range values {
		if raw, ok := value.(string); ok {
			var f service.DeepseekFile
			if err := json.Unmarshal([]byte(raw), &f); err != nil {
				return nil, err
			}
			if f.Origin != "" {
				files[ids[i]] = f
			}
		}
	}
	return files, nil
}

func (c *gatewayCache) ListDeepseekFiles(ctx context.Context, userID int64, page service.DeepseekFilePage) ([]service.DeepseekFile, error) {
	if page.Limit < 1 || page.Limit > 1000 {
		return nil, errors.New("invalid file page size")
	}
	index := deepseekUserFileIndex(userID)
	order := index + ":order:" + page.Purpose
	now := time.Now().Unix()
	keys := []string{index + ":order:", index + ":expiry:"}
	if err := pruneDeepseekFilePage.Run(ctx, c.rdb, keys, now).Err(); err != nil {
		return nil, err
	}
	bound := ""
	if page.Cursor != "" {
		raw, err := c.rdb.Get(ctx, index+":"+page.Cursor).Bytes()
		if err != nil && err != redis.Nil {
			return nil, err
		}
		var cursor indexedDeepseekFile
		if err == redis.Nil {
			return nil, service.ErrDeepseekFileCursor
		}
		if err := json.Unmarshal(raw, &cursor); err != nil {
			return nil, err
		}
		if cursor.Order == "" || (cursor.ExpiresAt != 0 && cursor.ExpiresAt <= now) {
			return nil, service.ErrDeepseekFileCursor
		}
		bound = cursor.Order
	}
	files := make([]service.DeepseekFile, 0, page.Limit+1)
	reverse := page.Descending != page.Before
	for len(files) <= page.Limit {
		n := page.Limit + 1 - len(files)
		opts := &redis.ZRangeBy{Min: "-", Max: "+", Count: int64(n)}
		if bound != "" {
			if reverse {
				opts.Max = "(" + bound
			} else {
				opts.Min = "(" + bound
			}
		}
		var members []string
		var err error
		if reverse {
			members, err = c.rdb.ZRevRangeByLex(ctx, order, opts).Result()
		} else {
			members, err = c.rdb.ZRangeByLex(ctx, order, opts).Result()
		}
		if err != nil {
			return nil, err
		}
		if len(members) == 0 {
			break
		}
		keys := make([]string, len(members))
		for i, member := range members {
			_, id, _ := strings.Cut(member, ":")
			keys[i] = index + ":" + id
		}
		values, err := c.rdb.MGet(ctx, keys...).Result()
		if err != nil {
			return nil, err
		}
		for i, v := range values {
			raw, ok := v.(string)
			if !ok {
				continue
			}
			var f indexedDeepseekFile
			if err := json.Unmarshal([]byte(raw), &f); err != nil {
				return nil, err
			}
			if f.Order == members[i] && (page.Purpose == "" || page.Purpose == f.Purpose) && (f.ExpiresAt == 0 || f.ExpiresAt > now) {
				files = append(files, f.DeepseekFile)
			}
		}
		bound = members[len(members)-1]
		if len(members) < n {
			break
		}
	}
	return files, nil
}

var deleteDeepseekFile = redis.NewScript(`
local owner = redis.call('GET', KEYS[1])
if owner and owner ~= ARGV[1] then return 0 end
` + removeOldOrder + `
if redis.call('GET', KEYS[4]) == ARGV[1] then redis.call('DEL', KEYS[4]) end
redis.call('DEL', KEYS[1], KEYS[2])
redis.call('ZREM', KEYS[3], ARGV[3])
` + expireDeepseekFileIndex)

func (c *gatewayCache) DeleteDeepseekFile(ctx context.Context, userID, accountID int64, id string) error {
	ok, err := deleteDeepseekFile.Run(ctx, c.rdb, deepseekFileKeys(userID, accountID, id), deepseekFileOwner(userID, accountID), accountID, id).Int()
	if err == nil && ok == 0 {
		return errors.New("DeepSeek file binding changed")
	}
	return err
}
