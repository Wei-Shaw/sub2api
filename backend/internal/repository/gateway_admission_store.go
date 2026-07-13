package repository

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const defaultGatewayAdmissionLeaseTTL = 15 * time.Minute

var (
	acquireGatewayUserLeaseScript = redis.NewScript(`
		redis.replicate_commands()
		local standardKey = KEYS[1]
		local extraKey = KEYS[2]
		local queueKey = KEYS[3]
		local sequenceKey = KEYS[4]
		local deadlineKey = KEYS[5]
		local arrivalKey = KEYS[6]
		local originalDeadlineKey = KEYS[7]
		local legacyStandardKey = KEYS[8]
		local drainKey = KEYS[9]
		local requestID = ARGV[1]
		local standardLimit = tonumber(ARGV[2])
		local extraLimit = tonumber(ARGV[3])
		local ttlMs = tonumber(ARGV[4])
		local maxWaiting = tonumber(ARGV[5])
		local waitTimeoutMs = tonumber(ARGV[6])
		local legacyTTLSeconds = tonumber(ARGV[7])

		local nowParts = redis.call('TIME')
		local nowMs = tonumber(nowParts[1]) * 1000 + math.floor(tonumber(nowParts[2]) / 1000)
		local expiresAt = nowMs + ttlMs
		local draining = redis.call('EXISTS', drainKey) > 0

		redis.call('ZREMRANGEBYSCORE', standardKey, '-inf', nowMs)
		redis.call('ZREMRANGEBYSCORE', extraKey, '-inf', nowMs)
		redis.call('ZREMRANGEBYSCORE', legacyStandardKey, '-inf', math.floor(nowMs / 1000) - legacyTTLSeconds)
		local legacyStandardCount = redis.call('ZCARD', legacyStandardKey)
		local expiredRequests = redis.call('ZRANGEBYSCORE', deadlineKey, '-inf', nowMs)
		for _, expiredRequestID in ipairs(expiredRequests) do
			redis.call('ZREM', queueKey, expiredRequestID)
			redis.call('ZREM', deadlineKey, expiredRequestID)
			redis.call('ZREM', arrivalKey, expiredRequestID)
		end
		-- Keep an expired request tombstone for one lease TTL so immediate retries
		-- cannot renew their absolute deadline. After that grace period, reclaim
		-- metadata left by a crashed owner once no live lease or queue entry exists.
		local staleOriginalRequests = redis.call(
			'ZRANGEBYSCORE', originalDeadlineKey, '-inf', nowMs - ttlMs, 'LIMIT', 0, 1000
		)
		for _, staleRequestID in ipairs(staleOriginalRequests) do
			if redis.call('ZSCORE', standardKey, staleRequestID) == false and
				redis.call('ZSCORE', extraKey, staleRequestID) == false and
				redis.call('ZSCORE', queueKey, staleRequestID) == false then
				redis.call('ZREM', deadlineKey, staleRequestID)
				redis.call('ZREM', arrivalKey, staleRequestID)
				redis.call('ZREM', originalDeadlineKey, staleRequestID)
			end
		end

		if redis.call('ZSCORE', standardKey, requestID) ~= false then
			redis.call('ZREM', extraKey, requestID)
			redis.call('ZREM', queueKey, requestID)
			redis.call('ZREM', deadlineKey, requestID)
			redis.call('ZADD', standardKey, expiresAt, requestID)
			redis.call('PEXPIRE', standardKey, ttlMs * 2)
			return {1, 1, 0, 0, 0}
		end

		local convertingExtra = false
		if redis.call('ZSCORE', extraKey, requestID) ~= false then
			if draining or extraLimit <= 0 then
				redis.call('ZREM', extraKey, requestID)
				convertingExtra = true
			elseif standardLimit > 0 and legacyStandardCount + redis.call('ZCARD', standardKey) < standardLimit then
				redis.call('ZREM', extraKey, requestID)
				redis.call('ZREM', queueKey, requestID)
				redis.call('ZREM', deadlineKey, requestID)
				redis.call('ZADD', standardKey, expiresAt, requestID)
				redis.call('PEXPIRE', standardKey, ttlMs * 2)
				return {1, 1, 0, 0, 0}
			else
				redis.call('ZADD', extraKey, expiresAt, requestID)
				redis.call('PEXPIRE', extraKey, ttlMs * 2)
				return {1, 2, 0, 0, 0}
			end
		end

		local queued = redis.call('ZSCORE', queueKey, requestID)
		local originalDeadline = redis.call('ZSCORE', originalDeadlineKey, requestID)
		if originalDeadline ~= false and tonumber(originalDeadline) <= nowMs then
			redis.call('ZREM', queueKey, requestID)
			redis.call('ZREM', deadlineKey, requestID)
			redis.call('ZREM', arrivalKey, requestID)
			return {0, 0, 0, 0, 1}
		end
		if not convertingExtra and queued == false and draining then
			return {0, 0, 0, 1, 0}
		end

		if standardLimit <= 0 then
			redis.call('ZREM', queueKey, requestID)
			redis.call('ZREM', deadlineKey, requestID)
			redis.call('ZREM', arrivalKey, requestID)
			redis.call('ZREM', originalDeadlineKey, requestID)
			return {1, 1, 0, 0, 0}
		end

		if queued == false then
			if not convertingExtra and maxWaiting > 0 and redis.call('ZCARD', queueKey) >= maxWaiting then
				return {0, 0, 1, 0, 0}
			end
			local sequence = redis.call('ZSCORE', arrivalKey, requestID)
			if sequence == false then
				sequence = redis.call('INCR', sequenceKey)
				redis.call('ZADD', arrivalKey, sequence, requestID)
			end
			redis.call('ZADD', queueKey, sequence, requestID)
		end
		if redis.call('ZSCORE', deadlineKey, requestID) == false then
			if originalDeadline == false then
				originalDeadline = nowMs + waitTimeoutMs
				redis.call('ZADD', originalDeadlineKey, originalDeadline, requestID)
			end
			redis.call('ZADD', deadlineKey, originalDeadline, requestID)
		end
		redis.call('PEXPIRE', queueKey, ttlMs * 2)
		redis.call('PEXPIRE', sequenceKey, ttlMs * 2)
		redis.call('PEXPIRE', deadlineKey, ttlMs * 2)
		redis.call('PEXPIRE', arrivalKey, ttlMs * 2)
		redis.call('PEXPIRE', originalDeadlineKey, ttlMs * 2)
		local queueHead = redis.call('ZRANGE', queueKey, 0, 0)[1]
		if queueHead ~= requestID then
			return {0, 0, 0, 0, 0}
		end

		if standardLimit > 0 and legacyStandardCount + redis.call('ZCARD', standardKey) < standardLimit then
			redis.call('ZADD', standardKey, expiresAt, requestID)
			redis.call('ZREM', queueKey, requestID)
			redis.call('ZREM', deadlineKey, requestID)
			redis.call('PEXPIRE', standardKey, ttlMs * 2)
			return {1, 1, 0, 0, 0}
		end

		if not draining and extraLimit > 0 and redis.call('ZCARD', extraKey) < extraLimit then
			redis.call('ZADD', extraKey, expiresAt, requestID)
			redis.call('ZREM', queueKey, requestID)
			redis.call('ZREM', deadlineKey, requestID)
			redis.call('PEXPIRE', extraKey, ttlMs * 2)
			return {1, 2, 0, 0, 0}
		end

		return {0, 0, 0, 0, 0}
	`)

	releaseGatewayUserLeaseScript = redis.NewScript(`
		redis.call('ZREM', KEYS[1], ARGV[1])
		redis.call('ZREM', KEYS[2], ARGV[1])
		redis.call('ZREM', KEYS[3], ARGV[1])
		redis.call('ZREM', KEYS[5], ARGV[1])
		redis.call('ZREM', KEYS[6], ARGV[1])
		redis.call('ZREM', KEYS[7], ARGV[1])
		return 1
	`)

	renewGatewayUserLeaseScript = redis.NewScript(`
		redis.replicate_commands()
		local requestID = ARGV[1]
		local class = tonumber(ARGV[2])
		local ttlMs = tonumber(ARGV[3])
		local key = KEYS[class]

		if redis.call('ZSCORE', key, requestID) == false then
			return 0
		end

		local nowParts = redis.call('TIME')
		local nowMs = tonumber(nowParts[1]) * 1000 + math.floor(tonumber(nowParts[2]) / 1000)
		redis.call('ZADD', key, nowMs + ttlMs, requestID)
		redis.call('PEXPIRE', key, ttlMs * 2)
		return 1
	`)

	acquireGatewayTargetLeaseScript = redis.NewScript(`
		redis.replicate_commands()
		local platformKey = KEYS[1]
		local accountKey = KEYS[2]
		local standardQueueKey = KEYS[3]
		local extraQueueKey = KEYS[4]
		local sequenceKey = KEYS[5]
		local deadlineKey = KEYS[6]
		local drainKey = KEYS[7]
		local requestID = ARGV[1]
		local class = tonumber(ARGV[2])
		local platformCapacity = tonumber(ARGV[3])
		local reservedSlots = tonumber(ARGV[4])
		local accountLimit = tonumber(ARGV[5])
		local ttlMs = tonumber(ARGV[6])
		local ttlSeconds = math.max(math.ceil(ttlMs / 1000), 1)
		local waitTimeoutMs = tonumber(ARGV[7])
		local unlimited = tonumber(ARGV[8]) == 1

		-- platformKey is private and stores expires-at milliseconds. accountKey is
		-- shared with concurrencyCache and must keep last-seen Unix seconds.
		local nowParts = redis.call('TIME')
		local nowSeconds = tonumber(nowParts[1])
		local nowPreciseSeconds = nowSeconds + tonumber(nowParts[2]) / 1000000
		local nowMs = nowSeconds * 1000 + math.floor(tonumber(nowParts[2]) / 1000)
		local expiresAt = nowMs + ttlMs
		redis.call('ZREMRANGEBYSCORE', platformKey, '-inf', nowMs)
		redis.call('ZREMRANGEBYSCORE', accountKey, '-inf', nowSeconds - ttlSeconds)
		if class == 2 and redis.call('EXISTS', drainKey) > 0 then
			return 3
		end

		local hasPlatformLease = redis.call('ZSCORE', platformKey, requestID) ~= false
		local hasAccountLease = redis.call('ZSCORE', accountKey, requestID) ~= false
		if hasPlatformLease and hasAccountLease then
			redis.call('ZREM', standardQueueKey, requestID)
			redis.call('ZREM', extraQueueKey, requestID)
			redis.call('ZREM', deadlineKey, requestID)
			redis.call('ZADD', platformKey, expiresAt, requestID)
			redis.call('ZADD', accountKey, nowPreciseSeconds, requestID)
			redis.call('PEXPIRE', platformKey, ttlMs * 2)
			redis.call('EXPIRE', accountKey, ttlSeconds)
			return 1
		end

		if hasPlatformLease then
			redis.call('ZREM', platformKey, requestID)
		end
		if hasAccountLease then
			redis.call('ZREM', accountKey, requestID)
		end

		local requestDeadline = redis.call('ZSCORE', deadlineKey, requestID)
		if requestDeadline ~= false and tonumber(requestDeadline) <= nowMs then
			redis.call('ZREM', standardQueueKey, requestID)
			redis.call('ZREM', extraQueueKey, requestID)
			redis.call('ZREM', deadlineKey, requestID)
			return 2
		end

		local expiredRequests = redis.call('ZRANGEBYSCORE', deadlineKey, '-inf', nowMs)
		for _, expiredRequestID in ipairs(expiredRequests) do
			redis.call('ZREM', standardQueueKey, expiredRequestID)
			redis.call('ZREM', extraQueueKey, expiredRequestID)
			redis.call('ZREM', deadlineKey, expiredRequestID)
		end

		local standardScore = redis.call('ZSCORE', standardQueueKey, requestID)
		local extraScore = redis.call('ZSCORE', extraQueueKey, requestID)
		if class == 1 then
			if standardScore == false then
				if extraScore ~= false then
					standardScore = extraScore
					redis.call('ZREM', extraQueueKey, requestID)
				else
					standardScore = redis.call('INCR', sequenceKey)
				end
				redis.call('ZADD', standardQueueKey, standardScore, requestID)
			end
		else
			if extraScore == false then
				if standardScore ~= false then
					extraScore = standardScore
					redis.call('ZREM', standardQueueKey, requestID)
				else
					extraScore = redis.call('INCR', sequenceKey)
				end
				redis.call('ZADD', extraQueueKey, extraScore, requestID)
			end
		end
		if redis.call('ZSCORE', deadlineKey, requestID) == false then
			redis.call('ZADD', deadlineKey, nowMs + waitTimeoutMs, requestID)
		end
		redis.call('PEXPIRE', standardQueueKey, ttlMs * 2)
		redis.call('PEXPIRE', extraQueueKey, ttlMs * 2)
		redis.call('PEXPIRE', sequenceKey, ttlMs * 2)
		redis.call('PEXPIRE', deadlineKey, ttlMs * 2)

		if class == 1 then
			local standardHead = redis.call('ZRANGE', standardQueueKey, 0, 0)[1]
			if standardHead ~= requestID then
				return 0
			end
		else
			if redis.call('ZCARD', standardQueueKey) > 0 then
				return 0
			end
			local extraHead = redis.call('ZRANGE', extraQueueKey, 0, 0)[1]
			if extraHead ~= requestID then
				return 0
			end
		end
		if unlimited then
			redis.call('ZREM', standardQueueKey, requestID)
			redis.call('ZREM', extraQueueKey, requestID)
			redis.call('ZREM', deadlineKey, requestID)
			return 1
		end

		local platformLimit = platformCapacity
		if class == 2 then
			platformLimit = platformCapacity - reservedSlots
		end
		if platformLimit < 0 then
			platformLimit = 0
		end

		if redis.call('ZCARD', platformKey) >= platformLimit then
			return 0
		end
		if redis.call('ZCARD', accountKey) >= accountLimit then
			return 0
		end

		redis.call('ZADD', platformKey, expiresAt, requestID)
		redis.call('ZADD', accountKey, nowPreciseSeconds, requestID)
		redis.call('ZREM', standardQueueKey, requestID)
		redis.call('ZREM', extraQueueKey, requestID)
		redis.call('ZREM', deadlineKey, requestID)
		redis.call('PEXPIRE', platformKey, ttlMs * 2)
		redis.call('EXPIRE', accountKey, ttlSeconds)
		return 1
	`)

	beginGatewayTargetDispatchScript = redis.NewScript(`
		redis.replicate_commands()
		local platformKey = KEYS[1]
		local accountKey = KEYS[2]
		local drainKey = KEYS[3]
		local requestID = ARGV[1]
		local class = tonumber(ARGV[2])
		local unlimited = tonumber(ARGV[3]) == 1
		local ttlMs = tonumber(ARGV[4])
		local ttlSeconds = math.max(math.ceil(ttlMs / 1000), 1)

		if class == 2 and redis.call('EXISTS', drainKey) > 0 then
			return 2
		end
		if unlimited then
			return 1
		end

		local nowParts = redis.call('TIME')
		local nowSeconds = tonumber(nowParts[1])
		local nowPreciseSeconds = nowSeconds + tonumber(nowParts[2]) / 1000000
		local nowMs = nowSeconds * 1000 + math.floor(tonumber(nowParts[2]) / 1000)
		local platformExpiry = redis.call('ZSCORE', platformKey, requestID)
		local accountLastSeen = redis.call('ZSCORE', accountKey, requestID)
		if platformExpiry == false or accountLastSeen == false or
			tonumber(platformExpiry) <= nowMs or tonumber(accountLastSeen) <= nowSeconds - ttlSeconds then
			redis.call('ZREM', platformKey, requestID)
			redis.call('ZREM', accountKey, requestID)
			return 0
		end

		redis.call('ZADD', platformKey, nowMs + ttlMs, requestID)
		redis.call('ZADD', accountKey, nowPreciseSeconds, requestID)
		redis.call('PEXPIRE', platformKey, ttlMs * 2)
		redis.call('EXPIRE', accountKey, ttlSeconds)
		return 1
	`)

	releaseGatewayTargetLeaseScript = redis.NewScript(`
		redis.call('ZREM', KEYS[1], ARGV[1])
		redis.call('ZREM', KEYS[2], ARGV[1])
		redis.call('ZREM', KEYS[3], ARGV[1])
		redis.call('ZREM', KEYS[4], ARGV[1])
		redis.call('ZREM', KEYS[6], ARGV[1])
		return 1
	`)

	renewGatewayTargetLeaseScript = redis.NewScript(`
		redis.replicate_commands()
		local platformKey = KEYS[1]
		local accountKey = KEYS[2]
		local requestID = ARGV[1]
		local ttlMs = tonumber(ARGV[2])
		local ttlSeconds = math.max(math.ceil(ttlMs / 1000), 1)

		-- Keep the shared account score compatible with legacy concurrencyCache.
		local nowParts = redis.call('TIME')
		local nowSeconds = tonumber(nowParts[1])
		local nowPreciseSeconds = nowSeconds + tonumber(nowParts[2]) / 1000000
		local nowMs = nowSeconds * 1000 + math.floor(tonumber(nowParts[2]) / 1000)
		local platformExpiry = redis.call('ZSCORE', platformKey, requestID)
		local accountLastSeen = redis.call('ZSCORE', accountKey, requestID)
		if platformExpiry == false or accountLastSeen == false or
			tonumber(platformExpiry) <= nowMs or tonumber(accountLastSeen) <= nowSeconds - ttlSeconds then
			redis.call('ZREM', platformKey, requestID)
			redis.call('ZREM', accountKey, requestID)
			return 0
		end

		redis.call('ZADD', platformKey, nowMs + ttlMs, requestID)
		redis.call('ZADD', accountKey, nowPreciseSeconds, requestID)
		redis.call('PEXPIRE', platformKey, ttlMs * 2)
		redis.call('EXPIRE', accountKey, ttlSeconds)
		return 1
	`)
)

type gatewayAdmissionStore struct {
	rdb             *redis.Client
	leaseTTL        time.Duration
	leaseTTLMS      int64
	leaseTTLSeconds int64
}

func NewGatewayAdmissionStore(rdb *redis.Client, leaseTTL time.Duration) service.GatewayAdmissionStore {
	if leaseTTL <= 0 {
		leaseTTL = defaultGatewayAdmissionLeaseTTL
	}
	leaseTTLMS := leaseTTL.Milliseconds()
	if leaseTTLMS < 1 {
		leaseTTLMS = 1
	}
	leaseTTLSeconds := (leaseTTLMS + 999) / 1000
	return &gatewayAdmissionStore{
		rdb:             rdb,
		leaseTTL:        leaseTTL,
		leaseTTLMS:      leaseTTLMS,
		leaseTTLSeconds: leaseTTLSeconds,
	}
}

func (s *gatewayAdmissionStore) GatewayAdmissionLeaseTTL() time.Duration {
	if s == nil {
		return 0
	}
	return s.leaseTTL
}

func (s *gatewayAdmissionStore) TryAcquireUserLease(ctx context.Context, request service.UserLeaseRequest) (service.UserLeaseResult, error) {
	if s == nil || s.rdb == nil {
		return service.UserLeaseResult{}, fmt.Errorf("gateway admission store is unavailable")
	}
	if request.UserID <= 0 || request.RequestID == "" {
		return service.UserLeaseResult{}, fmt.Errorf("invalid gateway admission user lease request")
	}
	standardLimit := request.StandardLimit
	extraLimit := max(request.ExtraLimit, 0)
	waitTimeoutMS := request.WaitTimeout.Milliseconds()
	if waitTimeoutMS <= 0 {
		waitTimeoutMS = s.leaseTTLMS
	}
	values, err := acquireGatewayUserLeaseScript.Run(
		ctx,
		s.rdb,
		gatewayAdmissionUserLeaseKeys(request.UserID),
		request.RequestID,
		standardLimit,
		extraLimit,
		s.leaseTTLMS,
		max(request.MaxWaiting, 0),
		waitTimeoutMS,
		s.leaseTTLSeconds,
	).Int64Slice()
	if err != nil {
		return service.UserLeaseResult{}, fmt.Errorf("acquire gateway admission user lease: %w", err)
	}
	if len(values) != 5 {
		return service.UserLeaseResult{}, fmt.Errorf("acquire gateway admission user lease: unexpected result length %d", len(values))
	}

	result := service.UserLeaseResult{
		Acquired:  values[0] == 1,
		Unlimited: values[0] == 1 && request.StandardLimit <= 0,
		QueueFull: values[2] == 1,
		Draining:  values[3] == 1,
		Expired:   values[4] == 1,
	}
	switch values[1] {
	case 1:
		result.Class = service.AdmissionClassStandard
	case 2:
		result.Class = service.AdmissionClassExtra
	}
	return result, nil
}

func (s *gatewayAdmissionStore) ReleaseUserLease(ctx context.Context, userID int64, requestID string) error {
	if s == nil || s.rdb == nil {
		return fmt.Errorf("gateway admission store is unavailable")
	}
	if userID <= 0 || requestID == "" {
		return nil
	}
	if err := releaseGatewayUserLeaseScript.Run(
		ctx,
		s.rdb,
		gatewayAdmissionUserLeaseKeys(userID),
		requestID,
	).Err(); err != nil {
		return fmt.Errorf("release gateway admission user lease: %w", err)
	}
	return nil
}

func (s *gatewayAdmissionStore) RenewUserLease(ctx context.Context, userID int64, requestID string, class service.AdmissionClass) (bool, error) {
	if s == nil || s.rdb == nil {
		return false, fmt.Errorf("gateway admission store is unavailable")
	}
	if userID <= 0 || requestID == "" {
		return false, nil
	}
	classCode, err := gatewayAdmissionClassCode(class)
	if err != nil {
		return false, err
	}
	renewed, err := renewGatewayUserLeaseScript.Run(
		ctx,
		s.rdb,
		gatewayAdmissionUserLeaseKeys(userID),
		requestID,
		classCode,
		s.leaseTTLMS,
	).Int64()
	if err != nil {
		return false, fmt.Errorf("renew gateway admission user lease: %w", err)
	}
	return renewed == 1, nil
}

func (s *gatewayAdmissionStore) TryAcquireTargetLease(ctx context.Context, request service.TargetLeaseRequest) (service.TargetLeaseResult, error) {
	if s == nil || s.rdb == nil {
		return service.TargetLeaseResult{}, fmt.Errorf("gateway admission store is unavailable")
	}
	if request.RequestID == "" || request.Platform == "" || request.AccountID <= 0 {
		return service.TargetLeaseResult{}, fmt.Errorf("invalid gateway admission target lease request")
	}
	if !request.Unlimited && (request.AccountLimit <= 0 || request.PlatformCapacity <= 0) {
		return service.TargetLeaseResult{}, nil
	}

	classCode, err := gatewayAdmissionClassCode(request.Class)
	if err != nil {
		return service.TargetLeaseResult{}, err
	}

	reservedSlots := 0
	if !request.Unlimited {
		reservedSlots = min(max(request.ReservedSlots, 0), request.PlatformCapacity)
	}
	waitTimeoutMS := request.WaitTimeout.Milliseconds()
	if waitTimeoutMS < 1 {
		waitTimeoutMS = s.leaseTTLMS
	}
	unlimitedFlag := 0
	if request.Unlimited {
		unlimitedFlag = 1
	}
	decision, err := acquireGatewayTargetLeaseScript.Run(
		ctx,
		s.rdb,
		gatewayAdmissionTargetLeaseKeys(request.Platform, request.AccountID),
		request.RequestID,
		classCode,
		request.PlatformCapacity,
		reservedSlots,
		request.AccountLimit,
		s.leaseTTLMS,
		waitTimeoutMS,
		unlimitedFlag,
	).Int64()
	if err != nil {
		return service.TargetLeaseResult{}, fmt.Errorf("acquire gateway admission target lease: %w", err)
	}
	return service.TargetLeaseResult{
		Acquired: decision == 1,
		Expired:  decision == 2,
		Draining: decision == 3,
	}, nil
}

func (s *gatewayAdmissionStore) ReleaseTargetLease(ctx context.Context, platform string, accountID int64, requestID string) error {
	if s == nil || s.rdb == nil {
		return fmt.Errorf("gateway admission store is unavailable")
	}
	if platform == "" || accountID <= 0 || requestID == "" {
		return nil
	}
	if err := releaseGatewayTargetLeaseScript.Run(
		ctx,
		s.rdb,
		gatewayAdmissionTargetLeaseKeys(platform, accountID),
		requestID,
	).Err(); err != nil {
		return fmt.Errorf("release gateway admission target lease: %w", err)
	}
	return nil
}

// BeginTargetDispatch establishes the Redis ordering point between dispatch and the global drain.
func (s *gatewayAdmissionStore) BeginTargetDispatch(ctx context.Context, request service.TargetDispatchRequest) (service.TargetDispatchResult, error) {
	if s == nil || s.rdb == nil {
		return service.TargetDispatchResult{}, fmt.Errorf("gateway admission store is unavailable")
	}
	if request.RequestID == "" || request.Platform == "" || request.AccountID <= 0 {
		return service.TargetDispatchResult{}, fmt.Errorf("invalid gateway admission target dispatch request")
	}
	classCode, err := gatewayAdmissionClassCode(request.Class)
	if err != nil {
		return service.TargetDispatchResult{}, err
	}
	keys := gatewayAdmissionTargetLeaseKeys(request.Platform, request.AccountID)
	unlimitedFlag := 0
	if request.Unlimited {
		unlimitedFlag = 1
	}
	decision, err := beginGatewayTargetDispatchScript.Run(
		ctx,
		s.rdb,
		[]string{keys[0], keys[1], keys[6]},
		request.RequestID,
		classCode,
		unlimitedFlag,
		s.leaseTTLMS,
	).Int64()
	if err != nil {
		return service.TargetDispatchResult{}, fmt.Errorf("begin gateway admission target dispatch: %w", err)
	}
	return service.TargetDispatchResult{
		Started:  decision == 1,
		Draining: decision == 2,
	}, nil
}

func (s *gatewayAdmissionStore) RenewTargetLease(ctx context.Context, platform string, accountID int64, requestID string) (bool, error) {
	if s == nil || s.rdb == nil {
		return false, fmt.Errorf("gateway admission store is unavailable")
	}
	if platform == "" || accountID <= 0 || requestID == "" {
		return false, nil
	}
	keys := gatewayAdmissionTargetLeaseKeys(platform, accountID)
	renewed, err := renewGatewayTargetLeaseScript.Run(
		ctx,
		s.rdb,
		keys[:2],
		requestID,
		s.leaseTTLMS,
	).Int64()
	if err != nil {
		return false, fmt.Errorf("renew gateway admission target lease: %w", err)
	}
	return renewed == 1, nil
}

func gatewayAdmissionClassCode(class service.AdmissionClass) (int64, error) {
	switch class {
	case service.AdmissionClassStandard:
		return 1, nil
	case service.AdmissionClassExtra:
		return 2, nil
	default:
		return 0, fmt.Errorf("invalid gateway admission class %q", class)
	}
}

func gatewayAdmissionUserLeaseKeys(userID int64) []string {
	tag := "{u:" + strconv.FormatInt(userID, 10) + "}"
	prefix := "gateway_admission:" + tag + ":"
	return []string{
		prefix + "lease:standard",
		prefix + "lease:extra",
		prefix + "queue",
		prefix + "queue:sequence",
		prefix + "queue:deadline",
		prefix + "queue:arrival",
		prefix + "queue:original_deadline",
		userSlotKey(userID),
		extraConcurrencyAdmissionDrainKey,
	}
}

func gatewayAdmissionTargetLeaseKeys(platform string, accountID int64) []string {
	platformPrefix := "gateway_admission:{p:" + platform + "}:"
	return []string{
		platformPrefix + "lease:platform",
		accountSlotKeyPrefix + strconv.FormatInt(accountID, 10),
		platformPrefix + "queue:standard",
		platformPrefix + "queue:extra",
		platformPrefix + "queue:sequence",
		platformPrefix + "queue:deadline",
		extraConcurrencyAdmissionDrainKey,
	}
}
