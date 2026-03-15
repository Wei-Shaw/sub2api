package repository

import (
	"context"
	_ "embed"
	"fmt"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	stickySessionPrefix         = "sticky_session:"
	clientAffinityPrefix        = "client_affinity:"
	clientAffinityReversePrefix = "client_affinity_rev:"
)

var (
	//go:embed lua/get_affinity.lua
	getAffinityLua string
	//go:embed lua/update_affinity.lua
	updateAffinityLua string
	//go:embed lua/get_affinity_count.lua
	getAffinityCountLua string
	//go:embed lua/get_affinity_clients.lua
	getAffinityClientsLua string
	//go:embed lua/get_affinity_clients_with_scores.lua
	getAffinityClientsWithScoresLua string
	//go:embed lua/clear_account_affinity.lua
	clearAccountAffinityLua string

	getAffinityScript                  = redis.NewScript(getAffinityLua)
	updateAffinityScript               = redis.NewScript(updateAffinityLua)
	getAffinityCountScript             = redis.NewScript(getAffinityCountLua)
	getAffinityClientsScript           = redis.NewScript(getAffinityClientsLua)
	getAffinityClientsWithScoresScript = redis.NewScript(getAffinityClientsWithScoresLua)
	clearAccountAffinityScript         = redis.NewScript(clearAccountAffinityLua)
)

type gatewayCache struct {
	rdb *redis.Client
}

func NewGatewayCache(rdb *redis.Client) service.GatewayCache {
	return &gatewayCache{rdb: rdb}
}

// ensureScriptLoaded 确保 Lua 脚本已加载到 Redis 服务器的脚本缓存中。
// Pipeline 中的 Script.Run 只发送 EVALSHA，如果 Redis 重启过导致脚本缓存丢失，
// EVALSHA 会返回 NOSCRIPT 错误。此方法提前加载脚本以避免该问题。
func ensureScriptLoaded(ctx context.Context, rdb *redis.Client, script *redis.Script) {
	exists, err := script.Exists(ctx, rdb).Result()
	if err != nil || len(exists) == 0 || !exists[0] {
		_ = script.Load(ctx, rdb).Err()
	}
}

// buildSessionKey 构建 session key，包含 groupID 实现分组隔离
// 格式: sticky_session:{groupID}:{sessionHash}
func buildSessionKey(groupID int64, sessionHash string) string {
	return fmt.Sprintf("%s%d:%s", stickySessionPrefix, groupID, sessionHash)
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
func (c *gatewayCache) DeleteSessionAccountID(ctx context.Context, groupID int64, sessionHash string) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Del(ctx, key).Err()
}

// buildAffinityKey 构建正向亲和 key（client → accounts）
// 格式: client_affinity:{groupID}:{clientID}
func buildAffinityKey(groupID int64, clientID string) string {
	return fmt.Sprintf("%s%d:%s", clientAffinityPrefix, groupID, clientID)
}

// buildAffinityReverseKey 构建反向亲和 key（account → clients）
// 格式: client_affinity_rev:{groupID}:{accountID}
func buildAffinityReverseKey(groupID int64, accountID int64) string {
	return fmt.Sprintf("%s%d:%d", clientAffinityReversePrefix, groupID, accountID)
}

func (c *gatewayCache) GetClientAffinityAccounts(ctx context.Context, groupID int64, clientID string, ttl time.Duration) ([]int64, error) {
	key := buildAffinityKey(groupID, clientID)
	now := time.Now().Unix()
	expireThreshold := now - int64(ttl.Seconds())

	result, err := getAffinityScript.Run(ctx, c.rdb, []string{key}, expireThreshold).StringSlice()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	accountIDs := make([]int64, 0, len(result))
	for _, s := range result {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			continue
		}
		accountIDs = append(accountIDs, id)
	}
	return accountIDs, nil
}

func (c *gatewayCache) UpdateClientAffinity(ctx context.Context, groupID int64, clientID string, accountID int64, ttl time.Duration) error {
	fwdKey := buildAffinityKey(groupID, clientID)
	revKey := buildAffinityReverseKey(groupID, accountID)
	now := time.Now().Unix()
	ttlSeconds := int64(ttl.Seconds())
	expireThreshold := now - ttlSeconds

	return updateAffinityScript.Run(ctx, c.rdb, []string{fwdKey, revKey},
		now, ttlSeconds, accountID, expireThreshold, clientID,
	).Err()
}

// GetAccountAffinityCountBatch 批量获取账号的亲和客户端数量（惰性清理过期成员）
func (c *gatewayCache) GetAccountAffinityCountBatch(ctx context.Context, groupID int64, accountIDs []int64, ttl time.Duration) (map[int64]int64, error) {
	if len(accountIDs) == 0 {
		return map[int64]int64{}, nil
	}

	now := time.Now().Unix()
	expireThreshold := now - int64(ttl.Seconds())

	ensureScriptLoaded(ctx, c.rdb, getAffinityCountScript)

	pipe := c.rdb.Pipeline()
	cmds := make([]*redis.Cmd, len(accountIDs))
	for i, accID := range accountIDs {
		key := buildAffinityReverseKey(groupID, accID)
		cmds[i] = getAffinityCountScript.Run(ctx, pipe, []string{key}, expireThreshold)
	}
	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, err
	}

	result := make(map[int64]int64, len(accountIDs))
	for i, accID := range accountIDs {
		count, _ := cmds[i].Int64()
		result[accID] = count
	}
	return result, nil
}

// GetAccountAffinityClientsBatch 批量获取每个账号跨所有分组的亲和客户端列表（去重）。
// accountGroups: map[accountID][]groupID，对每个 (groupID, accountID) 组合查询反向索引。
func (c *gatewayCache) GetAccountAffinityClientsBatch(ctx context.Context, accountGroups map[int64][]int64, ttl time.Duration) (map[int64][]string, error) {
	if len(accountGroups) == 0 {
		return map[int64][]string{}, nil
	}

	now := time.Now().Unix()
	expireThreshold := now - int64(ttl.Seconds())

	// 构建所有 (accountID, groupID) 组合的查询
	type queryItem struct {
		accountID int64
		groupID   int64
	}
	var queries []queryItem
	for accID, groupIDs := range accountGroups {
		for _, gID := range groupIDs {
			queries = append(queries, queryItem{accountID: accID, groupID: gID})
		}
	}

	ensureScriptLoaded(ctx, c.rdb, getAffinityClientsScript)

	pipe := c.rdb.Pipeline()
	cmds := make([]*redis.Cmd, len(queries))
	for i, q := range queries {
		key := buildAffinityReverseKey(q.groupID, q.accountID)
		cmds[i] = getAffinityClientsScript.Run(ctx, pipe, []string{key}, expireThreshold)
	}
	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, err
	}

	// 合并结果：同一个 accountID 跨多个 group 的 clientID 去重
	result := make(map[int64][]string, len(accountGroups))
	seen := make(map[int64]map[string]struct{}, len(accountGroups))
	for i, q := range queries {
		clients, _ := cmds[i].StringSlice()
		if len(clients) == 0 {
			continue
		}
		if seen[q.accountID] == nil {
			seen[q.accountID] = make(map[string]struct{})
		}
		for _, clientID := range clients {
			if _, exists := seen[q.accountID][clientID]; !exists {
				seen[q.accountID][clientID] = struct{}{}
				result[q.accountID] = append(result[q.accountID], clientID)
			}
		}
	}
	return result, nil
}

// GetAccountAffinityClientsWithScores 获取单个账号跨所有分组的亲和客户端列表（含最后活跃时间戳，去重取最近）。
func (c *gatewayCache) GetAccountAffinityClientsWithScores(
	ctx context.Context,
	accountID int64,
	groupIDs []int64,
	ttl time.Duration,
) ([]service.AffinityClient, error) {
	if len(groupIDs) == 0 {
		return nil, nil
	}

	now := time.Now().Unix()
	expireThreshold := now - int64(ttl.Seconds())

	ensureScriptLoaded(ctx, c.rdb, getAffinityClientsWithScoresScript)

	pipe := c.rdb.Pipeline()
	cmds := make([]*redis.Cmd, len(groupIDs))
	for i, gID := range groupIDs {
		key := buildAffinityReverseKey(gID, accountID)
		cmds[i] = getAffinityClientsWithScoresScript.Run(ctx, pipe, []string{key}, expireThreshold)
	}
	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, err
	}

	// 合并跨组结果，同一 clientID 取最近的 lastActive
	seen := make(map[string]int64) // clientID → max timestamp
	for _, cmd := range cmds {
		vals, _ := cmd.StringSlice()
		// vals 格式: [clientID1, score1, clientID2, score2, ...]
		for j := 0; j+1 < len(vals); j += 2 {
			clientID := vals[j]
			ts, _ := strconv.ParseInt(vals[j+1], 10, 64)
			if existing, ok := seen[clientID]; !ok || ts > existing {
				seen[clientID] = ts
			}
		}
	}

	result := make([]service.AffinityClient, 0, len(seen))
	for clientID, ts := range seen {
		result = append(result, service.AffinityClient{
			ClientID:   clientID,
			LastActive: time.Unix(ts, 0),
		})
	}

	// 按最后活跃时间降序排序
	service.SortAffinityClients(result)

	return result, nil
}

// ClearAccountAffinity 清除指定账号在所有分组的亲和记录（正向+反向索引）。
// 对每个 groupID 执行 Lua 脚本：读取反向索引获取所有客户端，
// 从每个客户端的正向索引中移除该账号，然后删除反向索引。
func (c *gatewayCache) ClearAccountAffinity(ctx context.Context, accountID int64, groupIDs []int64) error {
	if len(groupIDs) == 0 {
		return nil
	}

	ensureScriptLoaded(ctx, c.rdb, clearAccountAffinityScript)

	pipe := c.rdb.Pipeline()
	for _, gID := range groupIDs {
		revKey := buildAffinityReverseKey(gID, accountID)
		clearAccountAffinityScript.Run(ctx, pipe, []string{revKey}, gID, accountID)
	}
	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return err
	}
	return nil
}
