package repository

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

// 进程存活心跳让启动清理区分「已死进程的残留」和「同一 Redis 上其他仍在服务的进程」。
// 多个进程（蓝绿两槽、worker）共用一个 Redis 时，新进程启动只能清掉前者。
const (
	// 格式: ZSET member="<请求前缀>@<实例身份>"，score=最近一次心跳的 Redis Unix 秒。
	processHeartbeatKey = "concurrency:process:heartbeat"
	// 超过这个窗口没有心跳的前缀视为已死。service 侧心跳间隔必须远小于它。
	processHeartbeatWindowSeconds = 60
)

var _ service.ProcessLivenessCache = (*concurrencyCache)(nil)

// processHeartbeatScript 记录本进程心跳。
// 同一实例身份同一时刻只可能跑一个进程，因此同实例的其他前缀是崩溃/重启前的上一代，直接移除；
// 这样容器崩溃后立即重启时，上一代的残留不必等心跳窗口过去就能在启动时清掉。
// KEYS[1] = 心跳键，ARGV[1] = 请求前缀，ARGV[2] = 实例身份（可为空），ARGV[3] = 存活窗口（秒）
var processHeartbeatScript = redis.NewScript(`
	redis.replicate_commands()
	local key = KEYS[1]
	local prefix = ARGV[1]
	local instance = ARGV[2]
	local window = tonumber(ARGV[3])
	local now = tonumber(redis.call('TIME')[1])
	redis.call('ZREMRANGEBYSCORE', key, '-inf', '(' .. (now - window))
	local self = prefix .. '@' .. instance
	if instance ~= '' then
		for _, member in ipairs(redis.call('ZRANGE', key, 0, -1)) do
			local at = string.find(member, '@', 1, true)
			if member ~= self and at ~= nil and string.sub(member, at + 1) == instance then
				redis.call('ZREM', key, member)
			end
		end
	end
	redis.call('ZADD', key, now, self)
	redis.call('EXPIRE', key, window * 2)
	return 1
`)

// HeartbeatProcess 记录 requestPrefix 所属进程仍存活。instanceID 是跨进程重启保持不变、
// 但同时运行的进程之间互不相同的身份（容器主机名）；为空时不做同实例上一代的识别。
func (c *concurrencyCache) HeartbeatProcess(ctx context.Context, requestPrefix, instanceID string) error {
	if requestPrefix == "" {
		return nil
	}
	return processHeartbeatScript.Run(ctx, c.rdb, []string{processHeartbeatKey}, requestPrefix, instanceID, processHeartbeatWindowSeconds).Err()
}

// livePeerProcessPrefixes 返回心跳窗口内仍存活、且不是 activeRequestPrefix 的进程前缀。
func (c *concurrencyCache) livePeerProcessPrefixes(ctx context.Context, activeRequestPrefix string, now int64) ([]string, error) {
	members, err := c.rdb.ZRangeArgs(ctx, redis.ZRangeArgs{
		Key:     processHeartbeatKey,
		Start:   strconv.FormatInt(now-processHeartbeatWindowSeconds, 10),
		Stop:    "+inf",
		ByScore: true,
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("read process heartbeats: %w", err)
	}
	peers := make([]string, 0, len(members))
	for _, member := range members {
		prefix, _, _ := strings.Cut(member, "@")
		if prefix == "" || prefix == activeRequestPrefix {
			continue
		}
		peers = append(peers, prefix)
	}
	return peers, nil
}
