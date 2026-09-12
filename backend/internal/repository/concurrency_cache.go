package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

// 并发控制缓存常量定义
//
// 性能优化说明：
// 原实现使用 SCAN 命令遍历独立的槽位键（concurrency:account:{id}:{requestID}），
// 在高并发场景下 SCAN 需要多次往返，且遍历大量键时性能下降明显。
//
// 新实现改用 Redis 有序集合（Sorted Set）：
// 1. 每个账号/用户只有一个键，成员为 requestID，分数为时间戳
// 2. 使用 ZCARD 原子获取并发数，时间复杂度 O(1)
// 3. 使用 ZREMRANGEBYSCORE 清理过期槽位，避免手动管理 TTL
// 4. 单次 Redis 调用完成计数，减少网络往返
const (
	// 并发槽位键前缀（有序集合）
	// 格式: concurrency:account:{accountID}
	accountSlotKeyPrefix = "concurrency:account:"
	// 格式: concurrency:user:{userID}
	userSlotKeyPrefix = "concurrency:user:"
	// 格式: concurrency:api_key:{apiKeyID}
	apiKeySlotKeyPrefix = "concurrency:api_key:"
	// Key 级等待 ZSET：member=attemptID，score=Redis 毫秒截止时间。
	apiKeyWaitKeyPrefix = "concurrency:wait:api_key:"
	// Key 级等待阶段结束标记：member=attemptID，score=固定截止毫秒。
	// 用于阻止取消/成功后迟到的 ENTER/POLL 重建状态。
	apiKeyWaitClosedKeyPrefix = "concurrency:wait_closed:api_key:"
	// 等待/结束标记的清理余量（秒）。
	apiKeyQueueCleanupGraceSeconds = 5
	// Live 精确交接的中止栅栏：member=leaseID，score=拒绝迟到交接的毫秒期限。
	liveTransferFenceKeyPrefix = "concurrency:live_transfer_closed:api_key:"
	liveAccountSlotKeyPrefix   = "concurrency:live:account:"
	liveUserSlotKeyPrefix      = "concurrency:live:user:"
	liveAPIKeySlotKeyPrefix    = "concurrency:live:api_key:"
	// API-key-scoped client WebSocket ingress leases use a shorter TTL than
	// ordinary request slots, because idle ingress sessions do not hold a turn slot.
	openAIWSIngressLeaseKeyPrefix  = "concurrency:openai_ws_ingress:api_key:"
	openAIWSIngressLeaseTTLSeconds = 60
	liveLeaseTTLSeconds            = 60
	// 等待队列计数器格式: concurrency:wait:{userID}
	waitQueueKeyPrefix = "concurrency:wait:"
	// 账号级等待队列计数器格式: wait:account:{accountID}
	accountWaitKeyPrefix = "wait:account:"

	// 默认槽位过期时间（分钟），可通过配置覆盖
	defaultSlotTTLMinutes = 15

	// 活跃索引用来替代后台任务全量 SCAN 槽位键。
	// member 是账号/用户 ID，score 是“预计仍需关注到”的 Redis Unix 秒时间戳。
	accountActiveIndexKey = "concurrency:account:active_index" // ZSET member=accountID, score=expireAtUnixSeconds
	userActiveIndexKey    = "concurrency:user:active_index"    // ZSET member=userID, score=expireAtUnixSeconds

	// 后台清理只按批处理索引候选，避免单次任务占用 Redis 太久。
	activeIndexCleanupBatchSize  = 1000
	activeIndexPipelineChunkSize = 500

	// 一次性迁移 marker：活跃索引机制上线前遗留的等待计数键无法被索引发现，
	// 且有流量时 TTL 会被不断刷新，必须清扫一次。marker 存在即代表已完成。
	legacyWaitSweepMarkerKey = "concurrency:startup:legacy_wait_sweep:v1"
)

var (
	// acquireScript 使用有序集合计数并在未达上限时添加槽位
	// 使用 Redis TIME 命令获取服务器时间，避免多实例时钟不同步问题
	// KEYS[1] = 普通槽位键，KEYS[2] = 对应 Live 槽位键
	// ARGV[1] = maxConcurrency
	// ARGV[2] = TTL（秒）
	// ARGV[3] = requestID
	// 返回 {是否成功, Redis 当前秒}，Go 侧复用同一时间源写活跃索引，省去额外 TIME 往返。
	acquireScript = redis.NewScript(`
		-- Redis 3.2-4.x compat: opt into effects replication so redis.call('TIME')
		-- replicates correctly. No-op on Redis 5.0+ (effects replication is default).
		redis.replicate_commands()
		local key = KEYS[1]
		local liveKey = KEYS[2]
		local maxConcurrency = tonumber(ARGV[1])
		local ttl = tonumber(ARGV[2])
		local requestID = ARGV[3]

		-- 使用 Redis 服务器时间，确保多实例时钟一致
		local timeResult = redis.call('TIME')
		local now = tonumber(timeResult[1])
		local expireBefore = now - ttl

		-- 清理过期槽位
		redis.call('ZREMRANGEBYSCORE', key, '-inf', expireBefore)
		redis.call('ZREMRANGEBYSCORE', liveKey, '-inf', now - 60)

		-- 检查是否已存在（支持重试场景刷新时间戳）
		local exists = redis.call('ZSCORE', key, requestID)
		if exists ~= false then
			redis.call('ZADD', key, now, requestID)
			redis.call('EXPIRE', key, ttl)
			return {1, now}
		end

		-- 检查是否达到并发上限
		local count = redis.call('ZCARD', key) + redis.call('ZCARD', liveKey)
		if count < maxConcurrency then
			redis.call('ZADD', key, now, requestID)
			redis.call('EXPIRE', key, ttl)
			return {1, now}
		end

		return {0, now}
	`)

	// getCountScript 统计有序集合中的槽位数量并清理过期条目
	// 使用 Redis TIME 命令获取服务器时间
	// KEYS[1] = 普通槽位键，KEYS[2] = 对应 Live 槽位键
	// ARGV[1] = TTL（秒）
	getCountScript = redis.NewScript(`
		-- Redis 3.2-4.x compat: opt into effects replication so redis.call('TIME')
		-- replicates correctly. No-op on Redis 5.0+ (effects replication is default).
		redis.replicate_commands()
		local key = KEYS[1]
		local liveKey = KEYS[2]
		local ttl = tonumber(ARGV[1])

		-- 使用 Redis 服务器时间
		local timeResult = redis.call('TIME')
		local now = tonumber(timeResult[1])
		local expireBefore = now - ttl

		redis.call('ZREMRANGEBYSCORE', key, '-inf', expireBefore)
		redis.call('ZREMRANGEBYSCORE', liveKey, '-inf', now - 60)
		return redis.call('ZCARD', key) + redis.call('ZCARD', liveKey)
	`)

	acquireLiveLeaseScript = redis.NewScript(`
		redis.replicate_commands()
		local accountRegular = KEYS[1]
		local accountLive = KEYS[2]
		local userRegular = KEYS[3]
		local userLive = KEYS[4]
		local apiLive = KEYS[5]
		local apiRegular = KEYS[6]
		local accountMax = tonumber(ARGV[1])
		local userMax = tonumber(ARGV[2])
		local ttl = tonumber(ARGV[3])
		local leaseID = ARGV[4]
		local replacing = tonumber(ARGV[5])
		local apiMax = tonumber(ARGV[6])
		local sourceKeyMember = ARGV[8]
		local now = tonumber(redis.call('TIME')[1])
		local liveExpireBefore = now - ttl
		redis.call('ZREMRANGEBYSCORE', accountLive, '-inf', liveExpireBefore)
		redis.call('ZREMRANGEBYSCORE', userLive, '-inf', liveExpireBefore)
		redis.call('ZREMRANGEBYSCORE', apiLive, '-inf', liveExpireBefore)
		redis.call('ZREMRANGEBYSCORE', apiRegular, '-inf', now - tonumber(ARGV[7]))
		if redis.call('ZSCORE', accountLive, leaseID) ~= false then
			return 1
		end
		local accountCount = redis.call('ZCARD', accountRegular) + redis.call('ZCARD', accountLive)
		local userCount = redis.call('ZCARD', userRegular) + redis.call('ZCARD', userLive)
		local allowance = 0
		if replacing == 1 then allowance = 1 end
		if accountMax > 0 and accountCount >= accountMax + allowance then return 0 end
		if userMax > 0 and userCount >= userMax + allowance then return 0 end
		local apiCount = redis.call('ZCARD', apiRegular) + redis.call('ZCARD', apiLive)
		if apiMax > 0 then
			if sourceKeyMember ~= '' then
				if redis.call('ZSCORE', apiLive, leaseID) == false and redis.call('ZSCORE', apiRegular, sourceKeyMember) == false then
					-- The reserved key member was lost before handoff; fail closed
					-- instead of double-counting or continuing without ownership.
					return 3
				end
				if redis.call('ZSCORE', apiRegular, sourceKeyMember) ~= false then
					-- Atomic capacity transfer: the regular reservation becomes the
					-- Live member without a second allowance for the same dimension.
					redis.call('ZREM', apiRegular, sourceKeyMember)
					apiCount = apiCount - 1
				end
			end
			if apiCount >= apiMax then return 2 end
		end
		redis.call('ZADD', accountLive, now, leaseID)
		redis.call('ZADD', userLive, now, leaseID)
		redis.call('ZADD', apiLive, now, leaseID)
		redis.call('EXPIRE', accountLive, ttl)
		redis.call('EXPIRE', userLive, ttl)
		redis.call('EXPIRE', apiLive, ttl)
		return 1
	`)

	// transferLiveLeaseScript 是一次原子的精确三维交接：把本次句柄持有的普通
	// account/user/key member 移入同一个 Live lease，不依赖 allowance，也不会对
	// 同一维度重复计数。任一必需来源缺失即失败关闭；同一 leaseID 的三维成员
	// 已齐备时视为幂等重放。
	//
	// KEYS: 1=accountRegular, 2=accountLive, 3=userRegular, 4=userLive,
	//       5=apiRegular, 6=apiLive, 7=fence
	// ARGV: 1=accountMax, 2=userMax, 3=apiMax, 4=accountSource, 5=userSource,
	//       6=keySource, 7=leaseID, 8=liveTTL, 9=slotTTL
	// 返回: 1=LIVE, 2=Key 满, 3=Key 来源丢失, 4=账号满, 5=账号来源丢失,
	//       6=用户满, 7=用户来源丢失, 8=已中止, 9=部分 Live 状态冲突
	transferLiveLeaseScript = redis.NewScript(`
		redis.replicate_commands()
		local accountRegular = KEYS[1]
		local accountLive = KEYS[2]
		local userRegular = KEYS[3]
		local userLive = KEYS[4]
		local apiRegular = KEYS[5]
		local apiLive = KEYS[6]
		local fence = KEYS[7]
		local accountMax = tonumber(ARGV[1])
		local userMax = tonumber(ARGV[2])
		local apiMax = tonumber(ARGV[3])
		local accountSource = ARGV[4]
		local userSource = ARGV[5]
		local keySource = ARGV[6]
		local leaseID = ARGV[7]
		local ttl = tonumber(ARGV[8])
		local slotTTL = tonumber(ARGV[9])
		local t = redis.call('TIME')
		local nowMs = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
		local now = math.floor(nowMs / 1000)
		redis.call('ZREMRANGEBYSCORE', accountLive, '-inf', now - ttl)
		redis.call('ZREMRANGEBYSCORE', userLive, '-inf', now - ttl)
		redis.call('ZREMRANGEBYSCORE', apiLive, '-inf', now - ttl)
		redis.call('ZREMRANGEBYSCORE', apiRegular, '-inf', now - slotTTL)
		redis.call('ZREMRANGEBYSCORE', fence, '-inf', nowMs)

		if redis.call('ZSCORE', fence, leaseID) ~= false then
			return 8
		end
		local accountLiveExists = redis.call('ZSCORE', accountLive, leaseID) ~= false
		local userLiveExists = redis.call('ZSCORE', userLive, leaseID) ~= false
		local apiLiveExists = redis.call('ZSCORE', apiLive, leaseID) ~= false
		if accountLiveExists and userLiveExists and apiLiveExists then
			return 1
		end
		if accountLiveExists or userLiveExists or apiLiveExists then
			return 9
		end

		if accountMax > 0 then
			if accountSource == '' or redis.call('ZSCORE', accountRegular, accountSource) == false then
				return 5
			end
		end
		if userMax > 0 then
			if userSource == '' or redis.call('ZSCORE', userRegular, userSource) == false then
				return 7
			end
		end
		if apiMax > 0 then
			if keySource == '' or redis.call('ZSCORE', apiRegular, keySource) == false then
				return 3
			end
		end

		local accountCount = redis.call('ZCARD', accountRegular) + redis.call('ZCARD', accountLive)
		local userCount = redis.call('ZCARD', userRegular) + redis.call('ZCARD', userLive)
		local apiCount = redis.call('ZCARD', apiRegular) + redis.call('ZCARD', apiLive)
		if accountMax > 0 then
			accountCount = accountCount - 1
			if accountCount >= accountMax then
				return 4
			end
		end
		if userMax > 0 then
			userCount = userCount - 1
			if userCount >= userMax then
				return 6
			end
		end
		if apiMax > 0 then
			apiCount = apiCount - 1
			if apiCount >= apiMax then
				return 2
			end
		end

		if accountMax > 0 then
			redis.call('ZREM', accountRegular, accountSource)
		end
		if userMax > 0 then
			redis.call('ZREM', userRegular, userSource)
		end
		-- A stats-only key member is moved as well, so the unlimited tracking
		-- worker is not left behind as an orphan alongside the Live member.
		if keySource ~= '' then
			redis.call('ZREM', apiRegular, keySource)
		end
		redis.call('ZADD', accountLive, now, leaseID)
		redis.call('ZADD', userLive, now, leaseID)
		redis.call('ZADD', apiLive, now, leaseID)
		redis.call('EXPIRE', accountLive, ttl)
		redis.call('EXPIRE', userLive, ttl)
		redis.call('EXPIRE', apiLive, ttl)
		return 1
	`)

	// migrateLiveLeaseAccountScript 保留同一 Live 租约的 Key/user 成员，只把失败
	// 账号的 Live 成员原子替换为新账号的普通预留。任一既有联合成员缺失或新账号
	// 来源缺失都失败关闭；不重新入队 Key，也不释放 user 所有权。
	//
	// KEYS: 1=newAccountRegular, 2=newAccountLive, 3=oldAccountLive, 4=userLive,
	//       5=apiLive
	// ARGV: 1=accountMax, 2=accountSource, 3=leaseID, 4=liveTTL, 5=slotTTL
	// 返回: 1=LIVE, 2=联合租约丢失, 3=新账号来源丢失, 4=账号满
	migrateLiveLeaseAccountScript = redis.NewScript(`
		redis.replicate_commands()
		local t = redis.call('TIME')
		local nowMs = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
		local now = math.floor(nowMs / 1000)
		local accountMax = tonumber(ARGV[1])
		local accountSource = ARGV[2]
		local leaseID = ARGV[3]
		local ttl = tonumber(ARGV[4])
		local slotTTL = tonumber(ARGV[5])
		redis.call('ZREMRANGEBYSCORE', KEYS[2], '-inf', now - ttl)
		redis.call('ZREMRANGEBYSCORE', KEYS[4], '-inf', now - ttl)
		redis.call('ZREMRANGEBYSCORE', KEYS[5], '-inf', now - ttl)
		redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now - slotTTL)

		if redis.call('ZSCORE', KEYS[4], leaseID) == false or redis.call('ZSCORE', KEYS[5], leaseID) == false then
			return 2
		end
		if accountMax > 0 then
			if accountSource == '' or redis.call('ZSCORE', KEYS[1], accountSource) == false then
				return 3
			end
			local accountCount = redis.call('ZCARD', KEYS[1]) + redis.call('ZCARD', KEYS[2])
			accountCount = accountCount - 1
			if accountCount >= accountMax then
				return 4
			end
		end
		if accountMax > 0 then
			redis.call('ZREM', KEYS[1], accountSource)
		end
		redis.call('ZREM', KEYS[3], leaseID)
		redis.call('ZADD', KEYS[2], now, leaseID)
		redis.call('ZADD', KEYS[4], now, leaseID)
		redis.call('ZADD', KEYS[5], now, leaseID)
		redis.call('EXPIRE', KEYS[2], ttl)
		redis.call('EXPIRE', KEYS[4], ttl)
		redis.call('EXPIRE', KEYS[5], ttl)
		return 1
	`)

	// abortLiveLeaseTransferScript 用独立的栅栏期限封住本 leaseID 的迟到交接，
	// 再精确删除本次三维来源与 Live 成员；它不会触碰其他 lease 或 attempt。
	// 栅栏先于删除写入，Redis 原子执行的同时保证：若清除先到，迟到交接被拒；
	// 若交接先到，本脚本会把它删除。
	//
	// KEYS: 1=accountRegular, 2=userRegular, 3=apiRegular, 4=accountLive,
	//       5=userLive, 6=apiLive, 7=fence
	// ARGV: 1=accountSource, 2=userSource, 3=keySource, 4=leaseID, 5=fenceTTL
	abortLiveLeaseTransferScript = redis.NewScript(`
		redis.replicate_commands()
		local t = redis.call('TIME')
		local nowMs = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
		local fenceTTL = tonumber(ARGV[5])
		local fenceUntil = nowMs + fenceTTL * 1000
		redis.call('ZADD', KEYS[7], fenceUntil, ARGV[4])
		redis.call('EXPIRE', KEYS[7], fenceTTL)
		if ARGV[1] ~= '' then redis.call('ZREM', KEYS[1], ARGV[1]) end
		if ARGV[2] ~= '' then redis.call('ZREM', KEYS[2], ARGV[2]) end
		if ARGV[3] ~= '' then redis.call('ZREM', KEYS[3], ARGV[3]) end
		redis.call('ZREM', KEYS[4], ARGV[4])
		redis.call('ZREM', KEYS[5], ARGV[4])
		redis.call('ZREM', KEYS[6], ARGV[4])
		return 1
	`)

	refreshLiveLeaseScript = redis.NewScript(`
		redis.replicate_commands()
		local ttl = tonumber(ARGV[1])
		local leaseID = ARGV[2]
		local now = tonumber(redis.call('TIME')[1])
		local expireBefore = now - ttl
		for _, key in ipairs(KEYS) do
			redis.call('ZREMRANGEBYSCORE', key, '-inf', expireBefore)
			if redis.call('ZSCORE', key, leaseID) == false then return 0 end
		end
		for _, key in ipairs(KEYS) do
			redis.call('ZADD', key, now, leaseID)
			redis.call('EXPIRE', key, ttl)
		end
		return 1
	`)

	// trackSlotScript 记录 stats-only 槽位，不做并发上限判断。
	// KEYS[1] = 有序集合键
	// ARGV[1] = TTL（秒）
	// ARGV[2] = requestID
	trackSlotScript = redis.NewScript(`
		-- Redis 3.2-4.x compat: opt into effects replication so redis.call('TIME')
		-- replicates correctly. No-op on Redis 5.0+ (effects replication is default).
		redis.replicate_commands()
		local key = KEYS[1]
		local ttl = tonumber(ARGV[1])
		local requestID = ARGV[2]

		local timeResult = redis.call('TIME')
		local now = tonumber(timeResult[1])
		local expireBefore = now - ttl

		redis.call('ZREMRANGEBYSCORE', key, '-inf', expireBefore)
		redis.call('ZADD', key, now, requestID)
		redis.call('EXPIRE', key, ttl)
		return 1
	`)

	// acquireOpenAIWSIngressLeaseScript atomically reaps crashed members and
	// acquires or refreshes one API-key-scoped ingress lease using Redis TIME.
	acquireOpenAIWSIngressLeaseScript = redis.NewScript(`
		redis.replicate_commands()
		local key = KEYS[1]
		local maxConnections = tonumber(ARGV[1])
		local ttl = tonumber(ARGV[2])
		local leaseID = ARGV[3]
		local now = tonumber(redis.call('TIME')[1])
		local expireBefore = now - ttl
		redis.call('ZREMRANGEBYSCORE', key, '-inf', expireBefore)
		if redis.call('ZSCORE', key, leaseID) ~= false then
			redis.call('ZADD', key, now, leaseID)
			redis.call('EXPIRE', key, ttl)
			return 1
		end
		if redis.call('ZCARD', key) < maxConnections then
			redis.call('ZADD', key, now, leaseID)
			redis.call('EXPIRE', key, ttl)
			return 1
		end
		return 0
	`)

	// refreshOpenAIWSIngressLeaseScript does not recreate a missing member: a
	// process that lost its lease must terminate its local WebSocket instead of
	// silently continuing beyond the distributed cap.
	refreshOpenAIWSIngressLeaseScript = redis.NewScript(`
		redis.replicate_commands()
		local key = KEYS[1]
		local ttl = tonumber(ARGV[1])
		local leaseID = ARGV[2]
		local now = tonumber(redis.call('TIME')[1])
		local expireBefore = now - ttl
		redis.call('ZREMRANGEBYSCORE', key, '-inf', expireBefore)
		if redis.call('ZSCORE', key, leaseID) == false then
			return 0
		end
		redis.call('ZADD', key, now, leaseID)
		redis.call('EXPIRE', key, ttl)
		return 1
	`)

	// incrementWaitScript - refreshes TTL on each increment to keep queue depth accurate
	// KEYS[1] = wait queue key
	// ARGV[1] = maxWait
	// ARGV[2] = TTL in seconds
	// 返回 {是否成功, Redis 当前秒}，供 Go 侧免额外 TIME 往返写活跃索引。
	incrementWaitScript = redis.NewScript(`
		-- Redis 3.2-4.x compat: opt into effects replication so redis.call('TIME')
		-- replicates correctly. No-op on Redis 5.0+ (effects replication is default).
		redis.replicate_commands()
		local current = redis.call('GET', KEYS[1])
		if current == false then
			current = 0
		else
			current = tonumber(current)
		end
		local now = tonumber(redis.call('TIME')[1])

		if current >= tonumber(ARGV[1]) then
			return {0, now}
		end

		redis.call('INCR', KEYS[1])

		-- Refresh TTL so long-running traffic doesn't expire active queue counters.
		redis.call('EXPIRE', KEYS[1], ARGV[2])

		return {1, now}
	`)

	// incrementAccountWaitScript - account-level wait queue count (refresh TTL on each increment)
	// 返回值同 incrementWaitScript：{是否成功, Redis 当前秒}。
	incrementAccountWaitScript = redis.NewScript(`
		-- Redis 3.2-4.x compat: opt into effects replication so redis.call('TIME')
		-- replicates correctly. No-op on Redis 5.0+ (effects replication is default).
		redis.replicate_commands()
		local current = redis.call('GET', KEYS[1])
		if current == false then
			current = 0
		else
			current = tonumber(current)
		end
		local now = tonumber(redis.call('TIME')[1])

		if current >= tonumber(ARGV[1]) then
			return {0, now}
		end

		redis.call('INCR', KEYS[1])

		-- Refresh TTL so long-running traffic doesn't expire active queue counters.
		redis.call('EXPIRE', KEYS[1], ARGV[2])

		return {1, now}
	`)

	// decrementWaitScript - same as before
	decrementWaitScript = redis.NewScript(`
			local current = redis.call('GET', KEYS[1])
			if current ~= false and tonumber(current) > 0 then
				redis.call('DECR', KEYS[1])
			end
			return 1
		`)

	// cleanupExpiredSlotsScript 清理单个账号/用户有序集合中过期槽位
	// KEYS[1] = 有序集合键
	// ARGV[1] = TTL（秒）
	cleanupExpiredSlotsScript = redis.NewScript(`
		-- Redis 3.2-4.x compat: opt into effects replication so redis.call('TIME')
		-- replicates correctly. No-op on Redis 5.0+ (effects replication is default).
		redis.replicate_commands()
		local key = KEYS[1]
		local ttl = tonumber(ARGV[1])
		local timeResult = redis.call('TIME')
		local now = tonumber(timeResult[1])
		local expireBefore = now - ttl
		redis.call('ZREMRANGEBYSCORE', key, '-inf', expireBefore)
		if redis.call('ZCARD', key) == 0 then
			redis.call('DEL', key)
		else
			redis.call('EXPIRE', key, ttl)
		end
		return 1
	`)

	// startupCleanupSlotScript 清理单个槽位 key 中非当前进程前缀的成员，避免 Redis Cluster CROSSSLOT。
	// KEYS[1] 是有序集合键，ARGV[1] 是当前进程前缀，ARGV[2] 是槽位 TTL。
	// 返回 {清除数量, 剩余成员数}，Go 侧据剩余数决定索引 member 去留，无需再回读槽位。
	startupCleanupSlotScript = redis.NewScript(`
		local key = KEYS[1]
		local activePrefix = ARGV[1]
		local slotTTL = tonumber(ARGV[2])
		local removed = 0
		local members = redis.call('ZRANGE', key, 0, -1)
		for _, member in ipairs(members) do
			if string.sub(member, 1, string.len(activePrefix)) ~= activePrefix then
				removed = removed + redis.call('ZREM', key, member)
			end
		end
		local remaining = redis.call('ZCARD', key)
		if remaining == 0 then
			redis.call('DEL', key)
		else
			redis.call('EXPIRE', key, slotTTL)
		end
		return {removed, remaining}
	`)
)

type concurrencyCache struct {
	rdb                 *redis.Client
	slotTTLSeconds      int // 槽位过期时间（秒）
	waitQueueTTLSeconds int // 等待队列过期时间（秒）
}

// NewConcurrencyCache 创建并发控制缓存
// slotTTLMinutes: 槽位过期时间（分钟），0 或负数使用默认值 15 分钟
// waitQueueTTLSeconds: 等待队列过期时间（秒），0 或负数使用 slot TTL
func NewConcurrencyCache(rdb *redis.Client, slotTTLMinutes int, waitQueueTTLSeconds int) service.ConcurrencyCache {
	if slotTTLMinutes <= 0 {
		slotTTLMinutes = defaultSlotTTLMinutes
	}
	if waitQueueTTLSeconds <= 0 {
		waitQueueTTLSeconds = slotTTLMinutes * 60
	}
	return &concurrencyCache{
		rdb:                 rdb,
		slotTTLSeconds:      slotTTLMinutes * 60,
		waitQueueTTLSeconds: waitQueueTTLSeconds,
	}
}

// Helper functions for key generation
func accountSlotKey(accountID int64) string {
	return fmt.Sprintf("%s%d", accountSlotKeyPrefix, accountID)
}

func userSlotKey(userID int64) string {
	return fmt.Sprintf("%s%d", userSlotKeyPrefix, userID)
}

func apiKeySlotKey(apiKeyID int64) string {
	return fmt.Sprintf("%s%d", apiKeySlotKeyPrefix, apiKeyID)
}

func liveAccountSlotKey(accountID int64) string {
	return fmt.Sprintf("%s%d", liveAccountSlotKeyPrefix, accountID)
}

func liveUserSlotKey(userID int64) string {
	return fmt.Sprintf("%s%d", liveUserSlotKeyPrefix, userID)
}

func liveAPIKeySlotKey(apiKeyID int64) string {
	return fmt.Sprintf("%s%d", liveAPIKeySlotKeyPrefix, apiKeyID)
}

func openAIWSIngressLeaseKey(apiKeyID int64) string {
	return fmt.Sprintf("%s%d", openAIWSIngressLeaseKeyPrefix, apiKeyID)
}

func apiKeyWaitKey(apiKeyID int64) string {
	return fmt.Sprintf("%s%d", apiKeyWaitKeyPrefix, apiKeyID)
}

func apiKeyWaitClosedKey(apiKeyID int64) string {
	return fmt.Sprintf("%s%d", apiKeyWaitClosedKeyPrefix, apiKeyID)
}

// liveTransferFenceKey fences aborted Live handoffs: member=leaseID,
// score=milliseconds until which a late transfer command must be refused.
func liveTransferFenceKey(apiKeyID int64) string {
	return fmt.Sprintf("%s%d", liveTransferFenceKeyPrefix, apiKeyID)
}

func waitQueueKey(userID int64) string {
	return fmt.Sprintf("%s%d", waitQueueKeyPrefix, userID)
}

func accountWaitKey(accountID int64) string {
	return fmt.Sprintf("%s%d", accountWaitKeyPrefix, accountID)
}

// redisUnixSeconds 统一使用 Redis 服务器时间，避免多实例本地时钟漂移导致索引提前/延后过期。
func (c *concurrencyCache) redisUnixSeconds(ctx context.Context) (int64, error) {
	now, err := c.rdb.Time(ctx).Result()
	if err != nil {
		return 0, fmt.Errorf("redis TIME: %w", err)
	}
	return now.Unix(), nil
}

// slotIndexSpec 描述一个活跃索引及其对应的槽位/等待键构造方式。
// 用具名字段避免把 slotKey/waitKey 两个同签名函数按位置传参时写反。
type slotIndexSpec struct {
	indexKey string
	slotKey  func(int64) string
	waitKey  func(int64) string
}

var (
	accountSlotIndex = slotIndexSpec{indexKey: accountActiveIndexKey, slotKey: accountSlotKey, waitKey: accountWaitKey}
	userSlotIndex    = slotIndexSpec{indexKey: userActiveIndexKey, slotKey: userSlotKey, waitKey: waitQueueKey}
)

// touchActiveIndexAt 是写路径上的轻量标记：主操作已成功时，尽力把 ID 放入活跃索引，
// score 为给定的绝对过期时间（Redis Unix 秒）。索引失败不影响并发槽位/等待队列本身，
// 后续释放或清理会再次校正，因此只记日志不上抛。
func (c *concurrencyCache) touchActiveIndexAt(ctx context.Context, indexKey string, id int64, expireAt int64) {
	if c == nil || c.rdb == nil || id <= 0 || expireAt <= 0 {
		return
	}
	if err := c.rdb.ZAdd(ctx, indexKey, redis.Z{
		Score:  float64(expireAt),
		Member: strconv.FormatInt(id, 10),
	}).Err(); err != nil {
		logger.LegacyPrintf("repository.concurrency", "Warning: touch active index %s for %d failed: %v", indexKey, id, err)
	}
}

func (c *concurrencyCache) refreshAccountActiveIndex(ctx context.Context, accountID int64) {
	c.refreshActiveIndex(ctx, accountActiveIndexKey, accountID, accountSlotKey(accountID), accountWaitKey(accountID))
}

func (c *concurrencyCache) refreshUserActiveIndex(ctx context.Context, userID int64) {
	c.refreshActiveIndex(ctx, userActiveIndexKey, userID, userSlotKey(userID), waitQueueKey(userID))
}

// refreshActiveIndex 以 Redis 中的真实槽位/等待数为准重建索引状态。
// 释放槽位、等待计数减少、清理过期成员后都会调用它，防止索引残留。
// 索引维护是 best-effort：失败只记日志，不影响主流程。
func (c *concurrencyCache) refreshActiveIndex(ctx context.Context, indexKey string, id int64, slotKey, waitKey string) {
	if c == nil || c.rdb == nil || id <= 0 {
		return
	}
	now, err := c.redisUnixSeconds(ctx)
	if err != nil {
		logger.LegacyPrintf("repository.concurrency", "Warning: refresh active index %s for %d failed: %v", indexKey, id, err)
		return
	}

	load, err := c.readActiveLoadForKey(ctx, id, slotKey, waitKey, now)
	if err != nil {
		logger.LegacyPrintf("repository.concurrency", "Warning: refresh active index %s for %d failed: %v", indexKey, id, err)
		return
	}
	member := strconv.FormatInt(id, 10)
	if load.slotCount == 0 && load.waitCount <= 0 {
		if err := c.rdb.ZRem(ctx, indexKey, member).Err(); err != nil {
			logger.LegacyPrintf("repository.concurrency", "Warning: remove active index member %s from %s failed: %v", member, indexKey, err)
		}
		return
	}

	ttlSeconds := c.activeIndexTTL(load.slotCount, load.waitCount)
	if ttlSeconds <= 0 {
		return
	}
	c.touchActiveIndexAt(ctx, indexKey, id, now+int64(ttlSeconds))
}

type activeIndexLoad struct {
	id        int64
	member    string
	slotCount int
	waitCount int
}

// activeIndexTTL 取槽位 TTL 与等待队列 TTL 中仍然需要关注的较大值。
// 只要并发槽位或等待计数还有负载，就保留索引；两者都为 0 时调用方会删除索引。
func (c *concurrencyCache) activeIndexTTL(slotCount int, waitCount int) int {
	ttlSeconds := 0
	if slotCount > 0 {
		ttlSeconds = c.slotTTLSeconds
	}
	if waitCount > 0 && c.waitQueueTTLSeconds > ttlSeconds {
		ttlSeconds = c.waitQueueTTLSeconds
	}
	return ttlSeconds
}

// readActiveLoadForKey 读取单个 ID 的当前负载，并顺手清理该槽位集合中的过期成员。
func (c *concurrencyCache) readActiveLoadForKey(ctx context.Context, id int64, slotKey, waitKey string, now int64) (activeIndexLoad, error) {
	cutoffTime := now - int64(c.slotTTLSeconds)
	pipe := c.rdb.Pipeline()
	pipe.ZRemRangeByScore(ctx, slotKey, "-inf", strconv.FormatInt(cutoffTime, 10))
	zcardCmd := pipe.ZCard(ctx, slotKey)
	getCmd := pipe.Get(ctx, waitKey)
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return activeIndexLoad{}, fmt.Errorf("pipeline exec: %w", err)
	}

	waitCount := 0
	if v, err := getCmd.Int(); err == nil && v > 0 {
		waitCount = v
	}
	return activeIndexLoad{
		id:        id,
		member:    strconv.FormatInt(id, 10),
		slotCount: int(zcardCmd.Val()),
		waitCount: waitCount,
	}, nil
}

// readIndexLoads 批量读取索引候选的真实负载（账号/用户通用）。
// 分块 Pipeline 可以减少 Redis 往返，同时避免一次 Pipeline 塞入过多命令。
func (c *concurrencyCache) readIndexLoads(ctx context.Context, spec slotIndexSpec, members []string, now int64) ([]activeIndexLoad, []string, error) {
	loads := make([]activeIndexLoad, 0, len(members))
	staleMembers := make([]string, 0)
	candidates := make([]activeIndexLoad, 0, len(members))
	for _, member := range members {
		id, err := strconv.ParseInt(member, 10, 64)
		if err != nil || id <= 0 {
			staleMembers = append(staleMembers, member)
			continue
		}
		candidates = append(candidates, activeIndexLoad{id: id, member: member})
	}

	cutoffTime := now - int64(c.slotTTLSeconds)
	for start := 0; start < len(candidates); start += activeIndexPipelineChunkSize {
		end := start + activeIndexPipelineChunkSize
		if end > len(candidates) {
			end = len(candidates)
		}
		chunk := candidates[start:end]

		pipe := c.rdb.Pipeline()
		type loadCmd struct {
			activeIndexLoad
			zcardCmd *redis.IntCmd
			getCmd   *redis.StringCmd
		}
		cmds := make([]loadCmd, 0, len(chunk))
		for _, candidate := range chunk {
			slotKey := spec.slotKey(candidate.id)
			waitKey := spec.waitKey(candidate.id)
			pipe.ZRemRangeByScore(ctx, slotKey, "-inf", strconv.FormatInt(cutoffTime, 10))
			cmds = append(cmds, loadCmd{
				activeIndexLoad: candidate,
				zcardCmd:        pipe.ZCard(ctx, slotKey),
				getCmd:          pipe.Get(ctx, waitKey),
			})
		}
		if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
			return nil, nil, fmt.Errorf("pipeline exec: %w", err)
		}
		for _, cmd := range cmds {
			waitCount := 0
			if v, err := cmd.getCmd.Int(); err == nil && v > 0 {
				waitCount = v
			}
			loads = append(loads, activeIndexLoad{
				id:        cmd.id,
				member:    cmd.member,
				slotCount: int(cmd.zcardCmd.Val()),
				waitCount: waitCount,
			})
		}
	}

	return loads, staleMembers, nil
}

// removeActiveIndexMembers 清理无效 member；这是辅助索引的维护动作，调用方无需因为失败中断主流程。
func (c *concurrencyCache) removeActiveIndexMembers(ctx context.Context, indexKey string, members []string) {
	if len(members) == 0 {
		return
	}
	args := make([]any, 0, len(members))
	for _, member := range members {
		args = append(args, member)
	}
	if err := c.rdb.ZRem(ctx, indexKey, args...).Err(); err != nil {
		logger.LegacyPrintf("repository.concurrency", "Warning: remove %d active index members from %s failed: %v", len(members), indexKey, err)
	}
}

// runScriptInt64Pair 执行返回两元素整数数组的 Lua 脚本并解析（如 {result, now}、{removed, remaining}）。
func runScriptInt64Pair(ctx context.Context, rdb *redis.Client, script *redis.Script, keys []string, args ...any) (int64, int64, error) {
	raw, err := script.Run(ctx, rdb, keys, args...).Result()
	if err != nil {
		return 0, 0, err
	}
	first, err := redisScriptInt64At(raw, 0)
	if err != nil {
		return 0, 0, fmt.Errorf("parse script value 0: %w", err)
	}
	second, err := redisScriptInt64At(raw, 1)
	if err != nil {
		return 0, 0, fmt.Errorf("parse script value 1: %w", err)
	}
	return first, second, nil
}

// Account slot operations

func (c *concurrencyCache) AcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error) {
	key := accountSlotKey(accountID)
	// 时间戳在 Lua 脚本内使用 Redis TIME 命令获取，确保多实例时钟一致
	result, now, err := runScriptInt64Pair(ctx, c.rdb, acquireScript, []string{key, liveAccountSlotKey(accountID)}, maxConcurrency, c.slotTTLSeconds, requestID)
	if err != nil {
		return false, err
	}
	if result == 1 {
		// 成功占槽后标记活跃账号，后台清理即可从索引定位候选账号。
		c.touchActiveIndexAt(ctx, accountActiveIndexKey, accountID, now+int64(c.slotTTLSeconds))
	}
	return result == 1, nil
}

func (c *concurrencyCache) ReleaseAccountSlot(ctx context.Context, accountID int64, requestID string) error {
	key := accountSlotKey(accountID)
	if err := c.rdb.ZRem(ctx, key, requestID).Err(); err != nil {
		return err
	}
	// 释放后用真实负载刷新索引；若没有槽位和等待计数，会移除索引 member。
	c.refreshAccountActiveIndex(ctx, accountID)
	return nil
}

func (c *concurrencyCache) GetAccountConcurrency(ctx context.Context, accountID int64) (int, error) {
	key := accountSlotKey(accountID)
	// 时间戳在 Lua 脚本内使用 Redis TIME 命令获取
	result, err := getCountScript.Run(ctx, c.rdb, []string{key, liveAccountSlotKey(accountID)}, c.slotTTLSeconds).Int()
	if err != nil {
		return 0, err
	}
	return result, nil
}

func (c *concurrencyCache) GetAccountConcurrencyBatch(ctx context.Context, accountIDs []int64) (map[int64]int, error) {
	if len(accountIDs) == 0 {
		return map[int64]int{}, nil
	}

	now, err := c.rdb.Time(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("redis TIME: %w", err)
	}
	cutoffTime := now.Unix() - int64(c.slotTTLSeconds)

	pipe := c.rdb.Pipeline()
	type accountCmd struct {
		accountID int64
		zcardCmd  *redis.IntCmd
		liveCmd   *redis.IntCmd
	}
	cmds := make([]accountCmd, 0, len(accountIDs))
	for _, accountID := range accountIDs {
		slotKey := accountSlotKeyPrefix + strconv.FormatInt(accountID, 10)
		liveKey := liveAccountSlotKeyPrefix + strconv.FormatInt(accountID, 10)
		pipe.ZRemRangeByScore(ctx, slotKey, "-inf", strconv.FormatInt(cutoffTime, 10))
		pipe.ZRemRangeByScore(ctx, liveKey, "-inf", strconv.FormatInt(now.Unix()-liveLeaseTTLSeconds, 10))
		cmds = append(cmds, accountCmd{
			accountID: accountID,
			zcardCmd:  pipe.ZCard(ctx, slotKey),
			liveCmd:   pipe.ZCard(ctx, liveKey),
		})
	}

	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("pipeline exec: %w", err)
	}

	result := make(map[int64]int, len(accountIDs))
	for _, cmd := range cmds {
		result[cmd.accountID] = int(cmd.zcardCmd.Val() + cmd.liveCmd.Val())
	}
	return result, nil
}

// User slot operations

func (c *concurrencyCache) AcquireUserSlot(ctx context.Context, userID int64, maxConcurrency int, requestID string) (bool, error) {
	key := userSlotKey(userID)
	// 时间戳在 Lua 脚本内使用 Redis TIME 命令获取，确保多实例时钟一致
	result, now, err := runScriptInt64Pair(ctx, c.rdb, acquireScript, []string{key, liveUserSlotKey(userID)}, maxConcurrency, c.slotTTLSeconds, requestID)
	if err != nil {
		return false, err
	}
	if result == 1 {
		// 成功占槽后标记活跃用户，避免启动清理依赖全量 SCAN。
		c.touchActiveIndexAt(ctx, userActiveIndexKey, userID, now+int64(c.slotTTLSeconds))
	}
	return result == 1, nil
}

func (c *concurrencyCache) ReleaseUserSlot(ctx context.Context, userID int64, requestID string) error {
	key := userSlotKey(userID)
	if err := c.rdb.ZRem(ctx, key, requestID).Err(); err != nil {
		return err
	}
	// 释放后按 Redis 中剩余负载修正索引状态。
	c.refreshUserActiveIndex(ctx, userID)
	return nil
}

func (c *concurrencyCache) GetUserConcurrency(ctx context.Context, userID int64) (int, error) {
	key := userSlotKey(userID)
	// 时间戳在 Lua 脚本内使用 Redis TIME 命令获取
	result, err := getCountScript.Run(ctx, c.rdb, []string{key, liveUserSlotKey(userID)}, c.slotTTLSeconds).Int()
	if err != nil {
		return 0, err
	}
	return result, nil
}

// Admission and statistics share the same members. Redis TIME keeps pruning
// consistent across gateway instances; retrying the same member is idempotent.
// KEYS: 1=regular, 2=live, 3=waiting, 4=closed
// ARGV: 1=ttlSeconds, 2=liveTTLSeconds, 3=member, 4=limit
// Returns {code, nowMs}: code 1=ACQUIRED, 0=BUSY, 4=already closed.
var acquireAPIKeySlotScript = redis.NewScript(`
	redis.replicate_commands()
	local t = redis.call('TIME')
	local nowMs = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
	local now = math.floor(nowMs / 1000)
	local ttl = tonumber(ARGV[1])
	local member = ARGV[3]
	local limit = tonumber(ARGV[4])
	redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now - ttl)
	redis.call('ZREMRANGEBYSCORE', KEYS[2], '-inf', now - tonumber(ARGV[2]))
	if redis.call('ZSCORE', KEYS[1], member) ~= false then
		return {1, nowMs}
	end
	if redis.call('ZSCORE', KEYS[4], member) ~= false then
		return {4, nowMs}
	end
	-- A late fast attempt must not bypass the queue state machine.
	if redis.call('ZSCORE', KEYS[3], member) ~= false then
		return {0, nowMs}
	end
	local count = redis.call('ZCARD', KEYS[1]) + redis.call('ZCARD', KEYS[2])
	if limit > 0 and count >= limit then
		return {0, nowMs}
	end
	redis.call('ZADD', KEYS[1], now, member)
	redis.call('EXPIRE', KEYS[1], ttl)
	return {1, nowMs}
`)

func (c *concurrencyCache) AcquireAPIKeySlot(ctx context.Context, apiKeyID int64, maxConcurrency int, requestID string) (bool, error) {
	acquired, _, err := c.AcquireAPIKeySlotWithTime(ctx, apiKeyID, maxConcurrency, requestID)
	return acquired, err
}

func (c *concurrencyCache) AcquireAPIKeySlotWithTime(ctx context.Context, apiKeyID int64, maxConcurrency int, requestID string) (bool, int64, error) {
	code, nowMs, err := runScriptInt64Pair(
		ctx,
		c.rdb,
		acquireAPIKeySlotScript,
		[]string{apiKeySlotKey(apiKeyID), liveAPIKeySlotKey(apiKeyID), apiKeyWaitKey(apiKeyID), apiKeyWaitClosedKey(apiKeyID)},
		c.slotTTLSeconds,
		liveLeaseTTLSeconds,
		requestID,
		maxConcurrency,
	)
	if err != nil {
		return false, 0, err
	}
	return code == int64(service.APIKeyQueueOutcomeAcquired), nowMs, nil
}

// advanceAPIKeyQueueScript is the atomic ENTER/POLL state machine. All time
// checks use Redis TIME; the attempt's deadline is fixed by the caller and
// never extended here.
//
// KEYS: 1=regular, 2=live, 3=waiting, 4=closed
// ARGV: 1=mode(1=ENTER,2=POLL), 2=attemptID, 3=fixedDeadlineMs, 4=keyLimit,
//
//	5=maxWaiting, 6=slotTTLSeconds, 7=liveTTLSeconds, 8=cleanupGraceSeconds
var advanceAPIKeyQueueScript = redis.NewScript(`
	redis.replicate_commands()
	local t = redis.call('TIME')
	local nowMs = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
	local now = math.floor(nowMs / 1000)
	local mode = tonumber(ARGV[1])
	local attemptID = ARGV[2]
	local deadlineMs = tonumber(ARGV[3])
	local limit = tonumber(ARGV[4])
	local maxWaiting = tonumber(ARGV[5])
	local ttl = tonumber(ARGV[6])
	local grace = tonumber(ARGV[8])

	redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now - ttl)
	redis.call('ZREMRANGEBYSCORE', KEYS[2], '-inf', now - tonumber(ARGV[7]))
	redis.call('ZREMRANGEBYSCORE', KEYS[3], '-inf', nowMs)
	redis.call('ZREMRANGEBYSCORE', KEYS[4], '-inf', nowMs)

	if nowMs >= deadlineMs then
		redis.call('ZREM', KEYS[3], attemptID)
		return 3
	end
	if limit <= 0 then
		return 6
	end
	if redis.call('ZSCORE', KEYS[1], attemptID) ~= false then
		redis.call('ZREM', KEYS[3], attemptID)
		return 1
	end
	if redis.call('ZSCORE', KEYS[4], attemptID) ~= false then
		return 4
	end
	local queued = redis.call('ZSCORE', KEYS[3], attemptID)
	if queued ~= false and tonumber(queued) ~= deadlineMs then
		return 5
	end
	if mode == 2 and queued == false then
		return 5
	end
	local active = redis.call('ZCARD', KEYS[1]) + redis.call('ZCARD', KEYS[2])
	if active < limit then
		redis.call('ZADD', KEYS[1], now, attemptID)
		redis.call('EXPIRE', KEYS[1], ttl)
		redis.call('ZREM', KEYS[3], attemptID)
		redis.call('ZADD', KEYS[4], deadlineMs, attemptID)
		-- Keep every still-valid fence alive: the key TTL must cover the
		-- furthest member deadline, not just this attempt's.
		local maxClosed = redis.call('ZREVRANGE', KEYS[4], 0, 0, 'WITHSCORES')
		local closedUntil = deadlineMs
		if maxClosed[2] ~= nil then closedUntil = tonumber(maxClosed[2]) end
		local closedTTL = math.ceil((closedUntil - nowMs) / 1000) + grace
		if closedTTL < 1 then closedTTL = 1 end
		redis.call('EXPIRE', KEYS[4], closedTTL)
		return 1
	end
	if maxWaiting == 0 then
		return 8
	end
	if queued == false then
		if redis.call('ZCARD', KEYS[3]) >= maxWaiting then
			return 7
		end
		redis.call('ZADD', KEYS[3], deadlineMs, attemptID)
	end
	local maxMember = redis.call('ZREVRANGE', KEYS[3], 0, 0, 'WITHSCORES')
	if maxMember[2] ~= nil then
		local waitTTL = math.ceil((tonumber(maxMember[2]) - nowMs) / 1000) + grace
		if waitTTL < 1 then waitTTL = 1 end
		redis.call('EXPIRE', KEYS[3], waitTTL)
	end
	return 2
`)

// abortAPIKeyQueueScript fences late ENTER/POLL commands and removes this exact
// attempt from waiting and regular members. It never touches other attempts.
// removeRegular=0 is used after a confirmed ACQUIRED: the waiting stage is
// closed, but the regular member belongs to the new reservation.
//
// KEYS: 1=regular, 2=waiting, 3=closed
// ARGV: 1=attemptID, 2=fixedDeadlineMs, 3=cleanupGraceSeconds, 4=removeRegular(0/1)
var abortAPIKeyQueueScript = redis.NewScript(`
	redis.replicate_commands()
	local t = redis.call('TIME')
	local nowMs = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
	local attemptID = ARGV[1]
	local deadlineMs = tonumber(ARGV[2])
	if nowMs < deadlineMs then
		redis.call('ZADD', KEYS[3], deadlineMs, attemptID)
		local maxClosed = redis.call('ZREVRANGE', KEYS[3], 0, 0, 'WITHSCORES')
		local closedUntil = deadlineMs
		if maxClosed[2] ~= nil then closedUntil = tonumber(maxClosed[2]) end
		local closedTTL = math.ceil((closedUntil - nowMs) / 1000) + tonumber(ARGV[3])
		if closedTTL < 1 then closedTTL = 1 end
		redis.call('EXPIRE', KEYS[3], closedTTL)
	end
	redis.call('ZREM', KEYS[2], attemptID)
	if tonumber(ARGV[4]) == 1 then
		redis.call('ZREM', KEYS[1], attemptID)
	end
	return 1
`)

// apiKeyQueueStatsScript reads active and waiting counts after pruning expired
// members with Redis TIME, so a transfer cannot be shown as both waiting and
// active.
//
// KEYS: 1=regular, 2=live, 3=waiting, 4=closed
// ARGV: 1=slotTTLSeconds, 2=liveTTLSeconds
var apiKeyQueueStatsScript = redis.NewScript(`
	redis.replicate_commands()
	local t = redis.call('TIME')
	local nowMs = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
	local now = math.floor(nowMs / 1000)
	redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now - tonumber(ARGV[1]))
	redis.call('ZREMRANGEBYSCORE', KEYS[2], '-inf', now - tonumber(ARGV[2]))
	redis.call('ZREMRANGEBYSCORE', KEYS[3], '-inf', nowMs)
	redis.call('ZREMRANGEBYSCORE', KEYS[4], '-inf', nowMs)
	return {redis.call('ZCARD', KEYS[1]) + redis.call('ZCARD', KEYS[2]), redis.call('ZCARD', KEYS[3])}
`)

func (c *concurrencyCache) AdvanceAPIKeyQueue(ctx context.Context, apiKeyID int64, requestID string, mode service.APIKeyQueueMode, deadlineMs int64, keyLimit int, maxWaiting int) (service.APIKeyQueueOutcome, error) {
	code, err := advanceAPIKeyQueueScript.Run(
		ctx,
		c.rdb,
		[]string{apiKeySlotKey(apiKeyID), liveAPIKeySlotKey(apiKeyID), apiKeyWaitKey(apiKeyID), apiKeyWaitClosedKey(apiKeyID)},
		int(mode),
		requestID,
		deadlineMs,
		keyLimit,
		maxWaiting,
		c.slotTTLSeconds,
		liveLeaseTTLSeconds,
		apiKeyQueueCleanupGraceSeconds,
	).Int()
	if err != nil {
		return 0, err
	}
	return service.APIKeyQueueOutcome(code), nil
}

func (c *concurrencyCache) AbortAPIKeyQueueAttempt(ctx context.Context, apiKeyID int64, requestID string, deadlineMs int64, removeRegular bool) error {
	remove := 0
	if removeRegular {
		remove = 1
	}
	return abortAPIKeyQueueScript.Run(
		ctx,
		c.rdb,
		[]string{apiKeySlotKey(apiKeyID), apiKeyWaitKey(apiKeyID), apiKeyWaitClosedKey(apiKeyID)},
		requestID,
		deadlineMs,
		apiKeyQueueCleanupGraceSeconds,
		remove,
	).Err()
}

func (c *concurrencyCache) GetAPIKeyQueueStats(ctx context.Context, apiKeyID int64) (int, int, error) {
	active, waiting, err := runScriptInt64Pair(
		ctx,
		c.rdb,
		apiKeyQueueStatsScript,
		[]string{apiKeySlotKey(apiKeyID), liveAPIKeySlotKey(apiKeyID), apiKeyWaitKey(apiKeyID), apiKeyWaitClosedKey(apiKeyID)},
		c.slotTTLSeconds,
		liveLeaseTTLSeconds,
	)
	if err != nil {
		return 0, 0, err
	}
	return int(active), int(waiting), nil
}

func (c *concurrencyCache) TrackAPIKeySlot(ctx context.Context, apiKeyID int64, requestID string) error {
	key := apiKeySlotKey(apiKeyID)
	_, err := trackSlotScript.Run(ctx, c.rdb, []string{key}, c.slotTTLSeconds, requestID).Result()
	return err
}

func (c *concurrencyCache) ReleaseAPIKeySlot(ctx context.Context, apiKeyID int64, requestID string) error {
	key := apiKeySlotKey(apiKeyID)
	return c.rdb.ZRem(ctx, key, requestID).Err()
}

func (c *concurrencyCache) APIKeySlotRefreshInterval() time.Duration {
	return time.Duration(c.slotTTLSeconds) * time.Second / 3
}

func (c *concurrencyCache) APIKeySlotTTL() time.Duration {
	return time.Duration(c.slotTTLSeconds) * time.Second
}

// Refresh only an existing member, atomically with its key TTL. In particular,
// admin deletion must not be undone by a late heartbeat.
var refreshAPIKeySlotScript = redis.NewScript(`
	redis.replicate_commands()
	if redis.call('ZSCORE', KEYS[1], ARGV[2]) == false then
		return 0
	end
	local now = redis.call('TIME')
	redis.call('ZADD', KEYS[1], 'XX', tonumber(now[1]), ARGV[2])
	redis.call('EXPIRE', KEYS[1], tonumber(ARGV[1]))
	return 1
`)

func (c *concurrencyCache) RefreshAPIKeySlot(ctx context.Context, apiKeyID int64, requestID string) (bool, error) {
	n, err := refreshAPIKeySlotScript.Run(ctx, c.rdb, []string{apiKeySlotKey(apiKeyID)}, c.slotTTLSeconds, requestID).Int()
	return n == 1, err
}

func (c *concurrencyCache) AcquireOpenAIWSIngressLease(ctx context.Context, apiKeyID int64, maxConnections int, leaseID string) (bool, error) {
	if c == nil || c.rdb == nil || apiKeyID <= 0 || maxConnections <= 0 || leaseID == "" {
		return false, nil
	}
	result, err := acquireOpenAIWSIngressLeaseScript.Run(
		ctx,
		c.rdb,
		[]string{openAIWSIngressLeaseKey(apiKeyID)},
		maxConnections,
		openAIWSIngressLeaseTTLSeconds,
		leaseID,
	).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func (c *concurrencyCache) RefreshOpenAIWSIngressLease(ctx context.Context, apiKeyID int64, leaseID string) (bool, error) {
	if c == nil || c.rdb == nil || apiKeyID <= 0 || leaseID == "" {
		return false, nil
	}
	result, err := refreshOpenAIWSIngressLeaseScript.Run(
		ctx,
		c.rdb,
		[]string{openAIWSIngressLeaseKey(apiKeyID)},
		openAIWSIngressLeaseTTLSeconds,
		leaseID,
	).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func (c *concurrencyCache) ReleaseOpenAIWSIngressLease(ctx context.Context, apiKeyID int64, leaseID string) error {
	if c == nil || c.rdb == nil || apiKeyID <= 0 || leaseID == "" {
		return nil
	}
	return c.rdb.ZRem(ctx, openAIWSIngressLeaseKey(apiKeyID), leaseID).Err()
}

func (c *concurrencyCache) AcquireLiveLease(
	ctx context.Context,
	accountID int64,
	accountMax int,
	userID int64,
	userMax int,
	apiKeyID int64,
	apiKeyMax int,
	leaseID string,
	replacingRegularSlots bool,
) (bool, error) {
	if c == nil || c.rdb == nil || accountID <= 0 || userID <= 0 || apiKeyID <= 0 || leaseID == "" {
		return false, nil
	}
	return c.acquireLiveLease(ctx, accountID, accountMax, userID, userMax, apiKeyID, apiKeyMax, "", leaseID, replacingRegularSlots)
}

// AcquireLiveLeaseTransferring atomically moves the exact regular account,
// user and key reservations into the joint Live lease, so no dimension is
// counted twice. A missing required source or an aborted handoff fails closed
// instead of inventing capacity.
func (c *concurrencyCache) AcquireLiveLeaseTransferring(ctx context.Context, request service.LiveLeaseTransferRequest) (bool, error) {
	if c == nil || c.rdb == nil || request.AccountID <= 0 || request.UserID <= 0 || request.APIKeyID <= 0 || request.LeaseID == "" {
		return false, nil
	}
	result, err := transferLiveLeaseScript.Run(ctx, c.rdb, []string{
		accountSlotKey(request.AccountID),
		liveAccountSlotKey(request.AccountID),
		userSlotKey(request.UserID),
		liveUserSlotKey(request.UserID),
		apiKeySlotKey(request.APIKeyID),
		liveAPIKeySlotKey(request.APIKeyID),
		liveTransferFenceKey(request.APIKeyID),
	},
		request.AccountMax,
		request.UserMax,
		request.APIKeyMax,
		request.AccountRequestID,
		request.UserRequestID,
		request.KeyRequestID,
		request.LeaseID,
		liveLeaseTTLSeconds,
		c.slotTTLSeconds,
	).Int()
	if err != nil {
		// The script may have executed even if the reply was lost; the caller
		// must abort with the exact identities instead of assuming failure.
		return false, fmt.Errorf("%w: %v", service.ErrLiveLeaseTransferUncertain, err)
	}
	switch result {
	case 1:
		return true, nil
	case 2:
		return false, service.ErrAPIKeyConcurrencyLimit
	case 3:
		return false, service.ErrAPIKeyReservationLost
	case 4, 6:
		return false, service.ErrLiveConcurrencyFull
	case 5, 7:
		return false, service.ErrLiveLeaseSourceLost
	case 8:
		return false, service.ErrLiveLeaseTransferFenced
	case 9:
		return false, fmt.Errorf("%w: partial Live lease state for %s", service.ErrLiveUnavailable, request.LeaseID)
	default:
		return false, fmt.Errorf("%w: unexpected Live transfer result %d", service.ErrLiveUnavailable, result)
	}
}

// MigrateLiveLeaseAccount keeps the joint Key/user Live members and atomically
// replaces the failed account member with a new ordinary account reservation.
// A missing joint member or account source fails closed; the new ordinary
// member is removed only on success, so the caller's release path stays exact.
func (c *concurrencyCache) MigrateLiveLeaseAccount(ctx context.Context, request service.LiveLeaseTransferRequest) (bool, error) {
	if c == nil || c.rdb == nil || request.AccountID <= 0 || request.UserID <= 0 || request.APIKeyID <= 0 || request.LeaseID == "" {
		return false, nil
	}
	result, err := migrateLiveLeaseAccountScript.Run(ctx, c.rdb, []string{
		accountSlotKey(request.AccountID),
		liveAccountSlotKey(request.AccountID),
		liveAccountSlotKey(request.ReplacedAccountID),
		liveUserSlotKey(request.UserID),
		liveAPIKeySlotKey(request.APIKeyID),
	},
		request.AccountMax,
		request.AccountRequestID,
		request.LeaseID,
		liveLeaseTTLSeconds,
		c.slotTTLSeconds,
	).Int()
	if err != nil {
		return false, fmt.Errorf("%w: %v", service.ErrLiveLeaseTransferUncertain, err)
	}
	switch result {
	case 1:
		return true, nil
	case 2, 3:
		return false, service.ErrLiveLeaseSourceLost
	case 4:
		return false, service.ErrLiveConcurrencyFull
	default:
		return false, fmt.Errorf("%w: unexpected Live migration result %d", service.ErrLiveUnavailable, result)
	}
}

// ReleaseLiveLeaseAccount removes only this account's Live member so the joint
// Key/user members survive across SDP retry attempts.
func (c *concurrencyCache) ReleaseLiveLeaseAccount(ctx context.Context, accountID int64, leaseID string) error {
	if c == nil || c.rdb == nil || accountID <= 0 || leaseID == "" {
		return nil
	}
	return c.rdb.ZRem(ctx, liveAccountSlotKey(accountID), leaseID).Err()
}

// AbortLiveLeaseTransfer fences a handoff whose acknowledgement was lost and
// removes exactly this transfer's members. The fence deadline is independent
// of the Key wait deadline: it only has to outlive the in-flight transfer
// command, after which the deleted sources fail closed on any late replay.
func (c *concurrencyCache) AbortLiveLeaseTransfer(ctx context.Context, request service.LiveLeaseTransferRequest) error {
	if c == nil || c.rdb == nil || request.LeaseID == "" {
		return nil
	}
	return abortLiveLeaseTransferScript.Run(ctx, c.rdb, []string{
		accountSlotKey(request.AccountID),
		userSlotKey(request.UserID),
		apiKeySlotKey(request.APIKeyID),
		liveAccountSlotKey(request.AccountID),
		liveUserSlotKey(request.UserID),
		liveAPIKeySlotKey(request.APIKeyID),
		liveTransferFenceKey(request.APIKeyID),
	},
		request.AccountRequestID,
		request.UserRequestID,
		request.KeyRequestID,
		request.LeaseID,
		liveLeaseTTLSeconds,
	).Err()
}

func (c *concurrencyCache) acquireLiveLease(
	ctx context.Context,
	accountID int64,
	accountMax int,
	userID int64,
	userMax int,
	apiKeyID int64,
	apiKeyMax int,
	keyRequestID string,
	leaseID string,
	replacingRegularSlots bool,
) (bool, error) {
	replacing := 0
	if replacingRegularSlots {
		replacing = 1
	}
	result, err := acquireLiveLeaseScript.Run(ctx, c.rdb, []string{
		accountSlotKey(accountID),
		liveAccountSlotKey(accountID),
		userSlotKey(userID),
		liveUserSlotKey(userID),
		liveAPIKeySlotKey(apiKeyID),
		apiKeySlotKey(apiKeyID),
	}, accountMax, userMax, liveLeaseTTLSeconds, leaseID, replacing, apiKeyMax, c.slotTTLSeconds, keyRequestID).Int()
	if err == nil && result == 2 {
		return false, service.ErrAPIKeyConcurrencyLimit
	}
	if err == nil && result == 3 {
		return false, service.ErrAPIKeyReservationLost
	}
	return result == 1, err
}

func (c *concurrencyCache) RefreshLiveLease(ctx context.Context, accountID, userID, apiKeyID int64, leaseID string) (bool, error) {
	if c == nil || c.rdb == nil || leaseID == "" {
		return false, nil
	}
	result, err := refreshLiveLeaseScript.Run(ctx, c.rdb, []string{
		liveAccountSlotKey(accountID),
		liveUserSlotKey(userID),
		liveAPIKeySlotKey(apiKeyID),
	}, liveLeaseTTLSeconds, leaseID).Int()
	return result == 1, err
}

func (c *concurrencyCache) ReleaseLiveLease(ctx context.Context, accountID, userID, apiKeyID int64, leaseID string) error {
	if c == nil || c.rdb == nil || leaseID == "" {
		return nil
	}
	pipe := c.rdb.TxPipeline()
	pipe.ZRem(ctx, liveAccountSlotKey(accountID), leaseID)
	pipe.ZRem(ctx, liveUserSlotKey(userID), leaseID)
	pipe.ZRem(ctx, liveAPIKeySlotKey(apiKeyID), leaseID)
	_, err := pipe.Exec(ctx)
	return err
}

func (c *concurrencyCache) GetAPIKeyConcurrencyBatch(ctx context.Context, apiKeyIDs []int64) (map[int64]int, error) {
	if len(apiKeyIDs) == 0 {
		return map[int64]int{}, nil
	}

	now, err := c.rdb.Time(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("redis TIME: %w", err)
	}
	cutoffTime := now.Unix() - int64(c.slotTTLSeconds)

	pipe := c.rdb.Pipeline()
	type apiKeyCmd struct {
		apiKeyID int64
		zcardCmd *redis.IntCmd
		liveCmd  *redis.IntCmd
	}
	cmds := make([]apiKeyCmd, 0, len(apiKeyIDs))
	for _, apiKeyID := range apiKeyIDs {
		slotKey := apiKeySlotKeyPrefix + strconv.FormatInt(apiKeyID, 10)
		liveKey := liveAPIKeySlotKeyPrefix + strconv.FormatInt(apiKeyID, 10)
		pipe.ZRemRangeByScore(ctx, slotKey, "-inf", strconv.FormatInt(cutoffTime, 10))
		pipe.ZRemRangeByScore(ctx, liveKey, "-inf", strconv.FormatInt(now.Unix()-liveLeaseTTLSeconds, 10))
		cmds = append(cmds, apiKeyCmd{
			apiKeyID: apiKeyID,
			zcardCmd: pipe.ZCard(ctx, slotKey),
			liveCmd:  pipe.ZCard(ctx, liveKey),
		})
	}

	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("pipeline exec: %w", err)
	}

	result := make(map[int64]int, len(apiKeyIDs))
	for _, cmd := range cmds {
		result[cmd.apiKeyID] = int(cmd.zcardCmd.Val() + cmd.liveCmd.Val())
	}
	return result, nil
}

// Wait queue operations

func (c *concurrencyCache) IncrementWaitCount(ctx context.Context, userID int64, maxWait int) (bool, error) {
	key := waitQueueKey(userID)
	result, now, err := runScriptInt64Pair(ctx, c.rdb, incrementWaitScript, []string{key}, maxWait, c.waitQueueTTLSeconds)
	if err != nil {
		return false, err
	}
	if result == 1 {
		// 等待队列也会让用户保持“活跃”，否则槽位为 0 时后台任务可能漏看等待计数。
		c.touchActiveIndexAt(ctx, userActiveIndexKey, userID, now+int64(c.waitQueueTTLSeconds))
	}
	return result == 1, nil
}

func (c *concurrencyCache) DecrementWaitCount(ctx context.Context, userID int64) error {
	key := waitQueueKey(userID)
	_, err := decrementWaitScript.Run(ctx, c.rdb, []string{key}).Result()
	if err == nil {
		// 等待数减少后重新判断是否还需要保留索引。
		c.refreshUserActiveIndex(ctx, userID)
	}
	return err
}

// Account wait queue operations

func (c *concurrencyCache) IncrementAccountWaitCount(ctx context.Context, accountID int64, maxWait int) (bool, error) {
	key := accountWaitKey(accountID)
	result, now, err := runScriptInt64Pair(ctx, c.rdb, incrementAccountWaitScript, []string{key}, maxWait, c.waitQueueTTLSeconds)
	if err != nil {
		return false, err
	}
	if result == 1 {
		// 账号级等待队列同样写入账号活跃索引，供负载查询和清理任务使用。
		c.touchActiveIndexAt(ctx, accountActiveIndexKey, accountID, now+int64(c.waitQueueTTLSeconds))
	}
	return result == 1, nil
}

func (c *concurrencyCache) DecrementAccountWaitCount(ctx context.Context, accountID int64) error {
	key := accountWaitKey(accountID)
	_, err := decrementWaitScript.Run(ctx, c.rdb, []string{key}).Result()
	if err == nil {
		// 等待计数归零后索引需要同步删除，避免后台任务反复处理空账号。
		c.refreshAccountActiveIndex(ctx, accountID)
	}
	return err
}

func (c *concurrencyCache) GetAccountWaitingCount(ctx context.Context, accountID int64) (int, error) {
	key := accountWaitKey(accountID)
	val, err := c.rdb.Get(ctx, key).Int()
	if err != nil && !errors.Is(err, redis.Nil) {
		return 0, err
	}
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return val, nil
}

func (c *concurrencyCache) GetAccountsLoadBatch(ctx context.Context, accounts []service.AccountWithConcurrency) (map[int64]*service.AccountLoadInfo, error) {
	if len(accounts) == 0 {
		return map[int64]*service.AccountLoadInfo{}, nil
	}

	// 使用 Pipeline 替代 Lua 脚本，兼容 Redis Cluster（Lua 内动态拼 key 会 CROSSSLOT）。
	// 每个账号执行 3 个命令：ZREMRANGEBYSCORE（清理过期）、ZCARD（并发数）、GET（等待数）。
	now, err := c.rdb.Time(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("redis TIME: %w", err)
	}
	cutoffTime := now.Unix() - int64(c.slotTTLSeconds)

	pipe := c.rdb.Pipeline()

	type accountCmds struct {
		id             int64
		maxConcurrency int
		zcardCmd       *redis.IntCmd
		liveCmd        *redis.IntCmd
		getCmd         *redis.StringCmd
	}
	cmds := make([]accountCmds, 0, len(accounts))
	for _, acc := range accounts {
		slotKey := accountSlotKeyPrefix + strconv.FormatInt(acc.ID, 10)
		liveKey := liveAccountSlotKeyPrefix + strconv.FormatInt(acc.ID, 10)
		waitKey := accountWaitKeyPrefix + strconv.FormatInt(acc.ID, 10)
		pipe.ZRemRangeByScore(ctx, slotKey, "-inf", strconv.FormatInt(cutoffTime, 10))
		pipe.ZRemRangeByScore(ctx, liveKey, "-inf", strconv.FormatInt(now.Unix()-liveLeaseTTLSeconds, 10))
		ac := accountCmds{
			id:             acc.ID,
			maxConcurrency: acc.MaxConcurrency,
			zcardCmd:       pipe.ZCard(ctx, slotKey),
			liveCmd:        pipe.ZCard(ctx, liveKey),
			getCmd:         pipe.Get(ctx, waitKey),
		}
		cmds = append(cmds, ac)
	}

	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("pipeline exec: %w", err)
	}

	loadMap := make(map[int64]*service.AccountLoadInfo, len(accounts))
	for _, ac := range cmds {
		currentConcurrency := int(ac.zcardCmd.Val() + ac.liveCmd.Val())
		waitingCount := 0
		if v, err := ac.getCmd.Int(); err == nil {
			waitingCount = v
		}
		loadRate := 0
		if ac.maxConcurrency > 0 {
			loadRate = (currentConcurrency + waitingCount) * 100 / ac.maxConcurrency
		}
		loadMap[ac.id] = &service.AccountLoadInfo{
			AccountID:          ac.id,
			CurrentConcurrency: currentConcurrency,
			WaitingCount:       waitingCount,
			LoadRate:           loadRate,
		}
	}

	return loadMap, nil
}

func (c *concurrencyCache) GetUsersLoadBatch(ctx context.Context, users []service.UserWithConcurrency) (map[int64]*service.UserLoadInfo, error) {
	if len(users) == 0 {
		return map[int64]*service.UserLoadInfo{}, nil
	}

	// 使用 Pipeline 替代 Lua 脚本，兼容 Redis Cluster。
	now, err := c.rdb.Time(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("redis TIME: %w", err)
	}
	cutoffTime := now.Unix() - int64(c.slotTTLSeconds)

	pipe := c.rdb.Pipeline()

	type userCmds struct {
		id             int64
		maxConcurrency int
		zcardCmd       *redis.IntCmd
		liveCmd        *redis.IntCmd
		getCmd         *redis.StringCmd
	}
	cmds := make([]userCmds, 0, len(users))
	for _, u := range users {
		slotKey := userSlotKeyPrefix + strconv.FormatInt(u.ID, 10)
		liveKey := liveUserSlotKeyPrefix + strconv.FormatInt(u.ID, 10)
		waitKey := waitQueueKeyPrefix + strconv.FormatInt(u.ID, 10)
		pipe.ZRemRangeByScore(ctx, slotKey, "-inf", strconv.FormatInt(cutoffTime, 10))
		pipe.ZRemRangeByScore(ctx, liveKey, "-inf", strconv.FormatInt(now.Unix()-liveLeaseTTLSeconds, 10))
		uc := userCmds{
			id:             u.ID,
			maxConcurrency: u.MaxConcurrency,
			zcardCmd:       pipe.ZCard(ctx, slotKey),
			liveCmd:        pipe.ZCard(ctx, liveKey),
			getCmd:         pipe.Get(ctx, waitKey),
		}
		cmds = append(cmds, uc)
	}

	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("pipeline exec: %w", err)
	}

	loadMap := make(map[int64]*service.UserLoadInfo, len(users))
	for _, uc := range cmds {
		currentConcurrency := int(uc.zcardCmd.Val() + uc.liveCmd.Val())
		waitingCount := 0
		if v, err := uc.getCmd.Int(); err == nil {
			waitingCount = v
		}
		loadRate := 0
		if uc.maxConcurrency > 0 {
			loadRate = (currentConcurrency + waitingCount) * 100 / uc.maxConcurrency
		}
		loadMap[uc.id] = &service.UserLoadInfo{
			UserID:             uc.id,
			CurrentConcurrency: currentConcurrency,
			WaitingCount:       waitingCount,
			LoadRate:           loadRate,
		}
	}

	return loadMap, nil
}

func (c *concurrencyCache) CleanupExpiredAccountSlots(ctx context.Context, accountID int64) error {
	key := accountSlotKey(accountID)
	_, err := cleanupExpiredSlotsScript.Run(ctx, c.rdb, []string{key}, c.slotTTLSeconds).Result()
	if err == nil {
		// 单账号清理后同步索引，保持后台批量清理的候选集准确。
		c.refreshAccountActiveIndex(ctx, accountID)
	}
	return err
}

// CleanupExpiredAccountSlotKeys 处理账号与用户两个活跃索引中已到期的候选。
// （方法名中的 Account 是历史遗留，保留以避免接口变更；实际同时回收两个索引，
// 否则 user 索引的过期成员没有任何清理路径，会无界累积。）
func (c *concurrencyCache) CleanupExpiredAccountSlotKeys(ctx context.Context) error {
	if err := c.reconcileExpiredIndexCandidates(ctx, accountSlotIndex); err != nil {
		return err
	}
	return c.reconcileExpiredIndexCandidates(ctx, userSlotIndex)
}

// reconcileExpiredIndexCandidates 处理单个活跃索引中 score 已到期的候选：
// 无真实负载则移除 member；仍有负载则按真实负载批量刷新 score。
func (c *concurrencyCache) reconcileExpiredIndexCandidates(ctx context.Context, spec slotIndexSpec) error {
	now, err := c.redisUnixSeconds(ctx)
	if err != nil {
		return err
	}
	members, err := c.rdb.ZRangeArgs(ctx, redis.ZRangeArgs{
		Key:     spec.indexKey,
		Start:   "-inf",
		Stop:    strconv.FormatInt(now, 10),
		ByScore: true,
		Count:   activeIndexCleanupBatchSize,
	}).Result()
	if err != nil {
		return fmt.Errorf("read expired index %s: %w", spec.indexKey, err)
	}

	loads, staleMembers, err := c.readIndexLoads(ctx, spec, members, now)
	if err != nil {
		return err
	}
	refreshed := make([]redis.Z, 0, len(loads))
	for _, load := range loads {
		if load.slotCount == 0 && load.waitCount <= 0 {
			// 真实槽位和等待数都为空，说明这个索引 member 已经完成使命。
			staleMembers = append(staleMembers, load.member)
			continue
		}
		refreshed = append(refreshed, redis.Z{
			Score:  float64(now + int64(c.activeIndexTTL(load.slotCount, load.waitCount))),
			Member: load.member,
		})
	}
	if len(refreshed) > 0 {
		if err := c.rdb.ZAdd(ctx, spec.indexKey, refreshed...).Err(); err != nil {
			logger.LegacyPrintf("repository.concurrency", "Warning: refresh %d active index members in %s failed: %v", len(refreshed), spec.indexKey, err)
		}
	}
	c.removeActiveIndexMembers(ctx, spec.indexKey, staleMembers)
	return nil
}

// CleanupStaleProcessSlots 启动时清理非当前进程前缀的槽位。
// 清理范围来自活跃索引（含 score 已过期的成员——它们往往正是崩溃进程留下的残留），
// 避免在 Redis 上 SCAN 全部 concurrency:* 键；另有一次性迁移清扫兜底索引机制上线前的遗留等待计数。
// API Key 槽位（concurrency:api_key:*）是 stats-only 数据：每次 Track/读取都会按分数
// 裁剪过期成员，key 自带 TTL，可在一个 slot TTL 内自愈，因此不参与启动清理。
func (c *concurrencyCache) CleanupStaleProcessSlots(ctx context.Context, activeRequestPrefix string) error {
	if activeRequestPrefix == "" {
		return nil
	}
	if err := c.sweepLegacyWaitKeysOnce(ctx); err != nil {
		return err
	}
	now, err := c.redisUnixSeconds(ctx)
	if err != nil {
		return err
	}

	accountMembers, err := c.allIndexMembers(ctx, accountActiveIndexKey)
	if err != nil {
		return err
	}
	if err := c.cleanupStaleProcessSlotsForIndex(ctx, accountSlotIndex, accountMembers, activeRequestPrefix, now); err != nil {
		return err
	}

	userMembers, err := c.allIndexMembers(ctx, userActiveIndexKey)
	if err != nil {
		return err
	}
	return c.cleanupStaleProcessSlotsForIndex(ctx, userSlotIndex, userMembers, activeRequestPrefix, now)
}

// sweepLegacyWaitKeysOnce 一次性清扫活跃索引机制上线前遗留的等待计数键。
// 等待计数在有流量时会不断刷新 TTL、无法自然过期，而索引不认识旧键，
// 因此这里例外地做一次 SCAN，用 marker 键保证整个 Redis 数据生命周期内只执行一次。
// 先清扫后写 marker：清扫失败时下次启动会重试；并发实例重复清扫是幂等的。
func (c *concurrencyCache) sweepLegacyWaitKeysOnce(ctx context.Context) error {
	exists, err := c.rdb.Exists(ctx, legacyWaitSweepMarkerKey).Result()
	if err != nil {
		return fmt.Errorf("check legacy wait sweep marker: %w", err)
	}
	if exists > 0 {
		return nil
	}
	for _, pattern := range []string{accountWaitKeyPrefix + "*", waitQueueKeyPrefix + "*"} {
		var cursor uint64
		for {
			keys, next, err := c.rdb.Scan(ctx, cursor, pattern, 200).Result()
			if err != nil {
				return fmt.Errorf("scan legacy wait keys %s: %w", pattern, err)
			}
			if len(keys) > 0 {
				if err := c.rdb.Del(ctx, keys...).Err(); err != nil {
					return fmt.Errorf("delete legacy wait keys: %w", err)
				}
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
	}
	if err := c.rdb.Set(ctx, legacyWaitSweepMarkerKey, "1", 0).Err(); err != nil {
		return fmt.Errorf("set legacy wait sweep marker: %w", err)
	}
	return nil
}

// allIndexMembers 返回索引中全部 member（含 score 已过期的）。
// 启动清理必须覆盖过期成员：长时间停机后 score 过期的候选恰恰最可能持有死进程残留。
func (c *concurrencyCache) allIndexMembers(ctx context.Context, indexKey string) ([]string, error) {
	members, err := c.rdb.ZRange(ctx, indexKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("read active index %s: %w", indexKey, err)
	}
	return members, nil
}

// cleanupStaleProcessSlotsForIndex 逐个处理索引中的账号/用户。
// Lua 脚本一次只碰一个槽位 key，兼容 Redis Cluster，随后删除重启后已失效的等待计数；
// 索引 member 的去留由脚本返回的剩余槽位数决定，最后批量写回。
func (c *concurrencyCache) cleanupStaleProcessSlotsForIndex(
	ctx context.Context,
	spec slotIndexSpec,
	members []string,
	activeRequestPrefix string,
	now int64,
) error {
	staleMembers := make([]string, 0)
	refreshed := make([]redis.Z, 0)
	for _, member := range members {
		id, err := strconv.ParseInt(member, 10, 64)
		if err != nil || id <= 0 {
			staleMembers = append(staleMembers, member)
			continue
		}

		_, remaining, err := runScriptInt64Pair(ctx, c.rdb, startupCleanupSlotScript, []string{spec.slotKey(id)}, activeRequestPrefix, c.slotTTLSeconds)
		if err != nil {
			return fmt.Errorf("cleanup stale process slots %s: %w", spec.slotKey(id), err)
		}
		// 等待计数属于已死进程，直接删除；剩余槽位（当前进程前缀）决定索引 member 去留。
		if err := c.rdb.Del(ctx, spec.waitKey(id)).Err(); err != nil {
			return fmt.Errorf("delete stale wait key %s: %w", spec.waitKey(id), err)
		}
		if remaining > 0 {
			refreshed = append(refreshed, redis.Z{
				Score:  float64(now + int64(c.slotTTLSeconds)),
				Member: member,
			})
		} else {
			staleMembers = append(staleMembers, member)
		}
	}
	if len(refreshed) > 0 {
		if err := c.rdb.ZAdd(ctx, spec.indexKey, refreshed...).Err(); err != nil {
			logger.LegacyPrintf("repository.concurrency", "Warning: refresh %d active index members in %s failed: %v", len(refreshed), spec.indexKey, err)
		}
	}
	c.removeActiveIndexMembers(ctx, spec.indexKey, staleMembers)
	return nil
}
