package repository

import (
	"context"
	"fmt"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

func accountContinuationBurstKey(accountID int64) string {
	return fmt.Sprintf("admission:account:burst:%d", accountID)
}

func accountSlotReuseKey(accountID int64, requestID string) string {
	return fmt.Sprintf("admission:account:reuse:%d:%s", accountID, requestID)
}

// 获槽、保槽复用和轮转记账共用原子操作。复用 token 仅用于重放同一次 Redis 操作，纯续租不经过这里。
var accountAdmissionScript = redis.NewScript(`
	redis.replicate_commands()
	local slot, live, waiting, continuation, burst, reuse = unpack(KEYS)
	local maxConcurrency = tonumber(ARGV[1])
	local ttl = tonumber(ARGV[2])
	local requestID = ARGV[3]
	local class = ARGV[4]
	local limit = tonumber(ARGV[5])
	local admissionID = ARGV[6]
	local now = tonumber(redis.call('TIME')[1])
	redis.call('ZREMRANGEBYSCORE', slot, '-inf', now - ttl)
	redis.call('ZREMRANGEBYSCORE', live, '-inf', now - 60)
	local exists = redis.call('ZSCORE', slot, requestID) ~= false
	if admissionID ~= '' then
		if not exists then return {0, now} end
		if redis.call('GET', reuse) == admissionID then return {1, now} end
	elseif exists then
		redis.call('ZADD', slot, now, requestID)
		redis.call('EXPIRE', slot, ttl)
		return {1, now}
	end
	local newWaiting = tonumber(redis.call('GET', waiting) or '0')
	local contWaiting = tonumber(redis.call('GET', continuation) or '0')
	local count = tonumber(redis.call('GET', burst) or '0')
	if newWaiting <= 0 or limit <= 0 then
		redis.call('DEL', burst)
		count = 0
	end
	local newTurn = limit > 0 and newWaiting > 0 and count >= limit
	if class == 'continuation' and newTurn then
		if admissionID ~= '' then return {-1, now} end
		return {0, now}
	end
	if class == 'new_session' and contWaiting > 0 and not newTurn then return {0, now} end
	if admissionID == '' and redis.call('ZCARD', slot) + redis.call('ZCARD', live) >= maxConcurrency then
		return {0, now}
	end
	redis.call('ZADD', slot, now, requestID)
	redis.call('EXPIRE', slot, ttl)
	if admissionID ~= '' then
		redis.call('SET', reuse, admissionID, 'EX', ttl)
	end
	if class == 'new_session' then
		redis.call('DEL', burst)
	elseif class == 'continuation' and limit > 0 and newWaiting > 0 then
		local waitTTL = redis.call('PTTL', waiting)
		if waitTTL <= 0 then waitTTL = tonumber(ARGV[7]) * 1000 end
		redis.call('SET', burst, count + 1, 'PX', waitTTL)
	end
	return {1, now}
`)

var releaseAccountAdmissionScript = redis.NewScript(`
	redis.call('ZREM', KEYS[1], ARGV[1])
	redis.call('DEL', KEYS[2])
	return 1
`)

func (c *concurrencyCache) AcquireAccountSlotForClass(ctx context.Context, accountID int64, maxConcurrency int, requestID string, class service.AccountWaitClass, burstLimit int) (bool, error) {
	return c.admitAccountSlot(ctx, accountID, maxConcurrency, requestID, class, burstLimit, "")
}

func (c *concurrencyCache) ReuseAccountSlot(ctx context.Context, accountID int64, requestID, admissionID string, burstLimit int) (bool, error) {
	return c.admitAccountSlot(ctx, accountID, 0, requestID, service.AccountWaitClassContinuation, burstLimit, admissionID)
}

func (c *concurrencyCache) admitAccountSlot(ctx context.Context, accountID int64, maxConcurrency int, requestID string, class service.AccountWaitClass, burstLimit int, admissionID string) (bool, error) {
	keys := []string{accountSlotKey(accountID), liveAccountSlotKey(accountID), accountWaitKey(accountID), accountContinuationWaitKey(accountID), accountContinuationBurstKey(accountID), accountSlotReuseKey(accountID, requestID)}
	result, now, err := runScriptInt64Pair(ctx, c.rdb, accountAdmissionScript, keys, maxConcurrency, c.slotTTLSeconds, requestID, class.String(), burstLimit, admissionID, c.waitQueueTTLSeconds)
	if err != nil {
		return false, err
	}
	if result == -1 {
		return false, service.ErrAccountSlotYieldToNewSession
	}
	if result == 1 {
		c.touchActiveIndexAt(ctx, accountActiveIndexKey, accountID, now+int64(c.slotTTLSeconds))
	}
	return result == 1, nil
}

func (c *concurrencyCache) GetAccountAdmissionState(ctx context.Context, accountID int64) (*service.AccountAdmissionState, error) {
	values, err := c.rdb.MGet(ctx, accountWaitKey(accountID), accountContinuationWaitKey(accountID), accountContinuationBurstKey(accountID)).Result()
	if err != nil {
		return nil, err
	}
	counts := [3]int{}
	for i, value := range values {
		if value == nil {
			continue
		}
		counts[i], err = strconv.Atoi(fmt.Sprint(value))
		if err != nil {
			return nil, err
		}
	}
	return &service.AccountAdmissionState{NewWaiting: counts[0], ContinuationWaiting: counts[1], ContinuationBurst: counts[2]}, nil
}
