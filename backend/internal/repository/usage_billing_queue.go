package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	usageBillingStreamKey          = "billing:usage:stream"
	usageBillingConsumerGroup      = "billing-db-writers"
	usageBillingDeadLetterStream   = "billing:usage:dead"
	usageBillingPendingPrefix      = "billing:usage:pending:"
	usageBillingEnqueueDedupPrefix = "billing:usage:queued:"
	usageBillingMutationPrefix     = "billing:usage:mutation:"
	usageBillingDBSlotsKey         = "billing:usage:db-slots"
	usageBillingConsumerRetryDelay = time.Second
	usageBillingCacheTTLSeconds    = int(billingCacheTTL / time.Second)
	usageBillingDeadLetterMaxLen   = int64(10000)
)

var usageBillingEnqueueScript = redis.NewScript(`
	if redis.call('EXISTS', KEYS[2]) == 1 then
		if redis.call('GET', KEYS[2]) == ARGV[7] then
			return ''
		end
		return redis.error_reply('usage billing request fingerprint conflict')
	end

	local balance_mode = 'none'
	local subscription_mode = 'none'
	local balance_cost = tonumber(ARGV[2]) or 0
	local subscription_cost = tonumber(ARGV[3]) or 0
	local api_quota_cost = tonumber(ARGV[4]) or 0
	local rate_limit_cost = tonumber(ARGV[5]) or 0

	local balance_exists = redis.call('EXISTS', KEYS[7]) == 1
	local subscription_exists = redis.call('EXISTS', KEYS[8]) == 1
	if balance_exists and tonumber(redis.call('GET', KEYS[7])) == nil then
		return redis.error_reply('invalid cached billing balance')
	end
	if subscription_exists and redis.call('TYPE', KEYS[8]).ok ~= 'hash' then
		return redis.error_reply('invalid cached billing subscription')
	end
	for _, key in ipairs({KEYS[3], KEYS[4], KEYS[5], KEYS[6]}) do
		if redis.call('EXISTS', key) == 1 and tonumber(redis.call('GET', key)) == nil then
			return redis.error_reply('invalid pending billing value')
		end
	end

	local id = redis.call('XADD', KEYS[1], '*',
		'payload', ARGV[1],
		'balance_mode', balance_exists and 'cache' or (balance_cost > 0 and 'pending' or 'none'),
		'subscription_mode', subscription_exists and 'cache' or (subscription_cost > 0 and 'pending' or 'none'))

	if balance_cost > 0 then
		redis.call('INCR', KEYS[10])
		redis.call('EXPIRE', KEYS[10], ARGV[8])
		if balance_exists then
			redis.call('INCRBYFLOAT', KEYS[7], -balance_cost)
			redis.call('EXPIRE', KEYS[7], ARGV[6])
			balance_mode = 'cache'
		else
			redis.call('INCRBYFLOAT', KEYS[3], balance_cost)
			balance_mode = 'pending'
		end
	end

	if subscription_cost > 0 then
		redis.call('INCR', KEYS[11])
		redis.call('EXPIRE', KEYS[11], ARGV[8])
		if subscription_exists then
			redis.call('HINCRBYFLOAT', KEYS[8], 'daily_usage', subscription_cost)
			redis.call('HINCRBYFLOAT', KEYS[8], 'weekly_usage', subscription_cost)
			redis.call('HINCRBYFLOAT', KEYS[8], 'monthly_usage', subscription_cost)
			redis.call('EXPIRE', KEYS[8], ARGV[6])
			subscription_mode = 'cache'
		else
			redis.call('INCRBYFLOAT', KEYS[4], subscription_cost)
			subscription_mode = 'pending'
		end
	end

	if api_quota_cost > 0 then
		redis.call('INCRBYFLOAT', KEYS[5], api_quota_cost)
	end
	if rate_limit_cost > 0 then
		redis.call('INCR', KEYS[12])
		redis.call('EXPIRE', KEYS[12], ARGV[8])
		redis.call('INCRBYFLOAT', KEYS[6], rate_limit_cost)
	end

	-- Keep the dedup key for the full lifetime of the pending stream entry.
	-- The completion script starts the retention TTL only after PostgreSQL has
	-- accepted (or permanently rejected) the command.
	redis.call('SET', KEYS[2], ARGV[7])
	return id
`)

var usageBillingCompleteScript = redis.NewScript(`
	local acked = redis.call('XACK', KEYS[1], ARGV[1], ARGV[2])
	if acked == 0 then
		return 0
	end

	local function subtract_pending(key, amount)
		if amount <= 0 then
			return
		end
		local current = tonumber(redis.call('GET', key) or '0')
		local updated = current - amount
		if updated <= 0.0000000001 then
			redis.call('DEL', key)
		else
			redis.call('SET', key, tostring(updated))
		end
	end

	local balance_cost = tonumber(ARGV[5]) or 0
	local subscription_cost = tonumber(ARGV[6]) or 0
	local api_quota_cost = tonumber(ARGV[7]) or 0
	local rate_limit_cost = tonumber(ARGV[8]) or 0
	local applied = ARGV[9] == '1'

	if ARGV[3] == 'pending' then
		subtract_pending(KEYS[3], balance_cost)
		redis.call('DEL', KEYS[7])
	end
	if balance_cost > 0 then
		redis.call('INCR', KEYS[10])
		redis.call('EXPIRE', KEYS[10], ARGV[10])
	end
	if ARGV[4] == 'pending' then
		subtract_pending(KEYS[4], subscription_cost)
		redis.call('DEL', KEYS[8])
	end
	if subscription_cost > 0 then
		redis.call('INCR', KEYS[11])
		redis.call('EXPIRE', KEYS[11], ARGV[10])
	end
	subtract_pending(KEYS[5], api_quota_cost)
	subtract_pending(KEYS[6], rate_limit_cost)
	if rate_limit_cost > 0 then
		redis.call('INCR', KEYS[12])
		redis.call('EXPIRE', KEYS[12], ARGV[10])
		redis.call('DEL', KEYS[9])
	end
	if not applied then
		-- The optimistic hot-cache mutation belongs only to a newly applied DB
		-- command. A duplicate or permanent DB rejection must force a reload.
		if balance_cost > 0 then
			redis.call('DEL', KEYS[7])
		end
		if subscription_cost > 0 then
			redis.call('DEL', KEYS[8])
		end
	end

	redis.call('EXPIRE', KEYS[2], ARGV[10])
	redis.call('XDEL', KEYS[1], ARGV[2])
	return 1
`)

var usageBillingAcquireDBSlotScript = redis.NewScript(`
	local redis_time = redis.call('TIME')
	local now_ms = tonumber(redis_time[1]) * 1000 + math.floor(tonumber(redis_time[2]) / 1000)
	redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now_ms)
	if redis.call('ZCARD', KEYS[1]) >= tonumber(ARGV[1]) then
		return 0
	end
	redis.call('ZADD', KEYS[1], now_ms + tonumber(ARGV[2]), ARGV[3])
	return 1
`)

var usageBillingInvalidateCachesScript = redis.NewScript(`
	local balance_cost = tonumber(ARGV[1]) or 0
	local subscription_cost = tonumber(ARGV[2]) or 0
	local rate_limit_cost = tonumber(ARGV[3]) or 0
	local mutation_ttl = ARGV[4]
	if balance_cost > 0 then
		redis.call('DEL', KEYS[7])
		redis.call('INCR', KEYS[10])
		redis.call('EXPIRE', KEYS[10], mutation_ttl)
	end
	if subscription_cost > 0 then
		redis.call('DEL', KEYS[8])
		redis.call('INCR', KEYS[11])
		redis.call('EXPIRE', KEYS[11], mutation_ttl)
	end
	if rate_limit_cost > 0 then
		redis.call('DEL', KEYS[9])
		redis.call('INCR', KEYS[12])
		redis.call('EXPIRE', KEYS[12], mutation_ttl)
	end
	return 1
`)

type queuedUsageBillingRepository struct {
	direct                 service.UsageBillingRepository
	rdb                    *redis.Client
	consumerCount          int
	globalDBMaxConcurrency int
	readBatchSize          int64
	readBlock              time.Duration
	claimIdle              time.Duration
	commandTimeout         time.Duration
	dedupRetention         time.Duration
	fallbackDBSemaphore    chan struct{}
	consumerPrefix         string
	cancel                 context.CancelFunc
	wg                     sync.WaitGroup
	stopped                atomic.Bool
}

// ProvideUsageBillingRepository enables the Redis-backed production path while
// NewUsageBillingRepository remains the direct repository used by integration tests.
func ProvideUsageBillingRepository(_ *dbent.Client, sqlDB *sql.DB, rdb *redis.Client, cfg *config.Config) service.UsageBillingRepository {
	direct := &usageBillingRepository{db: sqlDB}
	if cfg == nil || !cfg.Billing.Queue.Enabled || rdb == nil {
		return direct
	}
	queueCfg := cfg.Billing.Queue
	repo := &queuedUsageBillingRepository{
		direct:                 direct,
		rdb:                    rdb,
		consumerCount:          max(1, queueCfg.ConsumerCount),
		globalDBMaxConcurrency: max(1, queueCfg.GlobalDBMaxConcurrency),
		readBatchSize:          int64(max(1, queueCfg.ReadBatchSize)),
		readBlock:              time.Duration(max(1, queueCfg.ReadBlockMilliseconds)) * time.Millisecond,
		claimIdle:              time.Duration(max(1, queueCfg.ClaimIdleSeconds)) * time.Second,
		commandTimeout:         time.Duration(max(1, queueCfg.CommandTimeoutSeconds)) * time.Second,
		dedupRetention:         time.Duration(max(1, queueCfg.DedupRetentionSeconds)) * time.Second,
		fallbackDBSemaphore:    make(chan struct{}, max(1, queueCfg.FallbackDBMaxConcurrency)),
		consumerPrefix:         usageBillingConsumerNamePrefix(),
	}
	repo.start()
	return repo
}

func (r *queuedUsageBillingRepository) Apply(ctx context.Context, cmd *service.UsageBillingCommand) (*service.UsageBillingApplyResult, error) {
	if cmd == nil {
		return &service.UsageBillingApplyResult{}, nil
	}
	cmd.Normalize()
	if cmd.RequestID == "" {
		return nil, service.ErrUsageBillingRequestIDRequired
	}
	payload, err := json.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("marshal usage billing command: %w", err)
	}

	enqueued, err := r.enqueue(ctx, cmd, payload)
	if err == nil {
		return &service.UsageBillingApplyResult{Applied: enqueued, Deferred: enqueued}, nil
	}
	if errors.Is(err, service.ErrUsageBillingRequestConflict) {
		return nil, err
	}
	slog.Error("usage billing Redis enqueue failed; using bounded DB fallback",
		"request_id", cmd.RequestID,
		"api_key_id", cmd.APIKeyID,
		"error", err,
	)
	result, fallbackErr := r.applyDirectBounded(ctx, cmd)
	if fallbackErr != nil {
		return nil, fallbackErr
	}
	// The enqueue error may be an ambiguous network result: the Lua script may
	// already have updated Redis and appended the event. Invalidate base caches
	// and suppress service-layer deltas so neither outcome can double-apply.
	r.invalidateUncertainBillingCaches(ctx, cmd)
	if result != nil && result.Applied {
		result.Deferred = true
	}
	return result, nil
}

func (r *queuedUsageBillingRepository) ReserveBatchImageBalance(ctx context.Context, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	return r.direct.ReserveBatchImageBalance(ctx, cmd)
}

func (r *queuedUsageBillingRepository) CaptureBatchImageBalance(ctx context.Context, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	return r.direct.CaptureBatchImageBalance(ctx, cmd)
}

func (r *queuedUsageBillingRepository) ReleaseBatchImageBalance(ctx context.Context, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	return r.direct.ReleaseBatchImageBalance(ctx, cmd)
}

func (r *queuedUsageBillingRepository) enqueue(ctx context.Context, cmd *service.UsageBillingCommand, payload []byte) (bool, error) {
	keys := usageBillingRedisKeys(cmd)
	result, err := usageBillingEnqueueScript.Run(ctx, r.rdb, keys,
		string(payload),
		cmd.BalanceCost,
		cmd.SubscriptionCost,
		cmd.APIKeyQuotaCost,
		cmd.APIKeyRateLimitCost,
		usageBillingCacheTTLSeconds,
		cmd.RequestFingerprint,
		int64(r.dedupRetention/time.Second),
	).Text()
	if errors.Is(err, redis.Nil) || (err == nil && result == "") {
		return false, nil
	}
	if err != nil {
		if strings.Contains(err.Error(), "usage billing request fingerprint conflict") {
			return false, service.ErrUsageBillingRequestConflict
		}
		return false, err
	}
	return true, nil
}

func (r *queuedUsageBillingRepository) applyDirectBounded(ctx context.Context, cmd *service.UsageBillingCommand) (*service.UsageBillingApplyResult, error) {
	select {
	case r.fallbackDBSemaphore <- struct{}{}:
		defer func() { <-r.fallbackDBSemaphore }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return r.direct.Apply(ctx, cmd)
}

func (r *queuedUsageBillingRepository) start() {
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	for i := 0; i < r.consumerCount; i++ {
		r.wg.Add(1)
		go r.consume(ctx, fmt.Sprintf("%s-%d", r.consumerPrefix, i))
	}
}

func (r *queuedUsageBillingRepository) Stop() {
	if r == nil || !r.stopped.CompareAndSwap(false, true) {
		return
	}
	if r.cancel != nil {
		r.cancel()
	}
	r.wg.Wait()
}

func (r *queuedUsageBillingRepository) consume(ctx context.Context, consumer string) {
	defer r.wg.Done()
	claimCursor := "0-0"
	for ctx.Err() == nil {
		if err := r.ensureConsumerGroup(ctx); err != nil {
			r.logConsumerError("create_group", consumer, err)
			waitUsageBillingRetry(ctx)
			continue
		}

		claimCursor = r.claimPending(ctx, consumer, claimCursor)
		streams, err := r.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    usageBillingConsumerGroup,
			Consumer: consumer,
			Streams:  []string{usageBillingStreamKey, ">"},
			Count:    r.readBatchSize,
			Block:    r.readBlock,
		}).Result()
		if errors.Is(err, redis.Nil) || errors.Is(err, context.Canceled) {
			continue
		}
		if err != nil {
			r.logConsumerError("read", consumer, err)
			waitUsageBillingRetry(ctx)
			continue
		}
		for _, stream := range streams {
			r.processMessages(ctx, consumer, stream.Messages)
		}
	}
}

func (r *queuedUsageBillingRepository) ensureConsumerGroup(ctx context.Context) error {
	err := r.rdb.XGroupCreateMkStream(ctx, usageBillingStreamKey, usageBillingConsumerGroup, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return err
	}
	return nil
}

func (r *queuedUsageBillingRepository) claimPending(ctx context.Context, consumer, start string) string {
	if start == "" {
		start = "0-0"
	}
	messages, next, err := r.rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   usageBillingStreamKey,
		Group:    usageBillingConsumerGroup,
		Consumer: consumer,
		MinIdle:  r.claimIdle,
		Start:    start,
		Count:    r.readBatchSize,
	}).Result()
	if errors.Is(err, redis.Nil) || errors.Is(err, context.Canceled) {
		return start
	}
	if err != nil {
		r.logConsumerError("claim", consumer, err)
		return start
	}
	r.processMessages(ctx, consumer, messages)
	if next == "" {
		return "0-0"
	}
	return next
}

func (r *queuedUsageBillingRepository) processMessages(ctx context.Context, consumer string, messages []redis.XMessage) {
	for _, message := range messages {
		payload, ok := message.Values["payload"].(string)
		if !ok || strings.TrimSpace(payload) == "" {
			r.deadLetterAndAck(ctx, message, nil, "missing payload")
			continue
		}
		var cmd service.UsageBillingCommand
		if err := json.Unmarshal([]byte(payload), &cmd); err != nil {
			r.deadLetterAndAck(ctx, message, nil, "invalid payload: "+err.Error())
			continue
		}
		cmd.Normalize()

		leaseMember := consumer + ":" + message.ID
		if err := r.acquireGlobalDBSlot(ctx, leaseMember); err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				r.logConsumerError("acquire_db_slot", consumer, err)
			}
			continue
		}
		applyCtx, cancel := context.WithTimeout(ctx, r.commandTimeout)
		result, err := r.direct.Apply(applyCtx, &cmd)
		r.releaseGlobalDBSlot(leaseMember)
		cancel()
		if err != nil {
			if isPermanentUsageBillingError(err) {
				r.deadLetterAndAck(ctx, message, &cmd, err.Error())
				continue
			}
			r.logConsumerError("apply", consumer, err)
			continue
		}

		if cmd.APIKeyQuotaCost > 0 && cmd.APIKeyAuthCacheKey != "" {
			if err := r.invalidateAPIKeyAuthCache(ctx, cmd.APIKeyAuthCacheKey); err != nil {
				r.logConsumerError("invalidate_api_key", consumer, err)
				continue
			}
		}
		applied := result != nil && result.Applied
		if err := r.complete(ctx, message, &cmd, applied); err != nil {
			r.logConsumerError("ack", consumer, err)
			continue
		}
	}
}

func (r *queuedUsageBillingRepository) acquireGlobalDBSlot(ctx context.Context, member string) error {
	lease := max(r.commandTimeout*2, r.claimIdle)
	for {
		acquired, err := usageBillingAcquireDBSlotScript.Run(ctx, r.rdb, []string{usageBillingDBSlotsKey},
			r.globalDBMaxConcurrency,
			lease.Milliseconds(),
			member,
		).Int()
		if err != nil {
			return err
		}
		if acquired == 1 {
			return nil
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (r *queuedUsageBillingRepository) releaseGlobalDBSlot(member string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.rdb.ZRem(ctx, usageBillingDBSlotsKey, member).Err(); err != nil {
		r.logConsumerError("release_db_slot", "", err)
	}
}

func (r *queuedUsageBillingRepository) complete(ctx context.Context, message redis.XMessage, cmd *service.UsageBillingCommand, applied bool) error {
	keys := usageBillingRedisKeys(cmd)
	appliedFlag := 0
	if applied {
		appliedFlag = 1
	}
	_, err := usageBillingCompleteScript.Run(ctx, r.rdb, keys,
		usageBillingConsumerGroup,
		message.ID,
		usageBillingMessageString(message, "balance_mode"),
		usageBillingMessageString(message, "subscription_mode"),
		cmd.BalanceCost,
		cmd.SubscriptionCost,
		cmd.APIKeyQuotaCost,
		cmd.APIKeyRateLimitCost,
		appliedFlag,
		int64(r.dedupRetention/time.Second),
	).Result()
	return err
}

func (r *queuedUsageBillingRepository) deadLetterAndAck(ctx context.Context, message redis.XMessage, cmd *service.UsageBillingCommand, reason string) {
	if err := r.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: usageBillingDeadLetterStream,
		MaxLen: usageBillingDeadLetterMaxLen,
		Approx: true,
		Values: map[string]any{"source_id": message.ID, "payload": message.Values["payload"], "reason": reason},
	}).Err(); err != nil {
		r.logConsumerError("dead_letter", "", err)
		return
	}
	if cmd == nil {
		_, _ = r.rdb.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.XAck(ctx, usageBillingStreamKey, usageBillingConsumerGroup, message.ID)
			pipe.XDel(ctx, usageBillingStreamKey, message.ID)
			return nil
		})
		return
	}
	if err := r.complete(ctx, message, cmd, false); err != nil {
		r.logConsumerError("dead_letter_ack", "", err)
	}
}

func (r *queuedUsageBillingRepository) invalidateAPIKeyAuthCache(ctx context.Context, cacheKey string) error {
	pipe := r.rdb.Pipeline()
	pipe.Del(ctx, apiKeyAuthCacheKey(cacheKey))
	pipe.Publish(ctx, authCacheInvalidateChannel, cacheKey)
	_, err := pipe.Exec(ctx)
	return err
}

func (r *queuedUsageBillingRepository) invalidateUncertainBillingCaches(ctx context.Context, cmd *service.UsageBillingCommand) {
	if cmd == nil {
		return
	}
	if _, err := usageBillingInvalidateCachesScript.Run(ctx, r.rdb, usageBillingRedisKeys(cmd),
		cmd.BalanceCost,
		cmd.SubscriptionCost,
		cmd.APIKeyRateLimitCost,
		int64(r.dedupRetention/time.Second),
	).Result(); err != nil {
		r.logConsumerError("invalidate_uncertain_cache", "", err)
	}
	if cmd.APIKeyQuotaCost > 0 && cmd.APIKeyAuthCacheKey != "" {
		if err := r.invalidateAPIKeyAuthCache(ctx, cmd.APIKeyAuthCacheKey); err != nil {
			r.logConsumerError("invalidate_uncertain_api_key", "", err)
		}
	}
}

func (r *queuedUsageBillingRepository) logConsumerError(stage, consumer string, err error) {
	slog.Error("usage billing Redis consumer error", "stage", stage, "consumer", consumer, "error", err)
}

func usageBillingRedisKeys(cmd *service.UsageBillingCommand) []string {
	userID, groupID, apiKeyID := int64(0), int64(0), int64(0)
	requestID := "invalid"
	if cmd != nil {
		userID, groupID, apiKeyID = cmd.UserID, cmd.GroupID, cmd.APIKeyID
		requestID = cmd.RequestID
	}
	return []string{
		usageBillingStreamKey,
		usageBillingEnqueueDedupKey(requestID, apiKeyID),
		usageBillingPendingBalanceKey(userID),
		usageBillingPendingSubscriptionKey(userID, groupID),
		usageBillingPendingAPIKeyQuotaKey(apiKeyID),
		usageBillingPendingAPIKeyRateLimitKey(apiKeyID),
		billingBalanceKey(userID),
		billingSubKey(userID, groupID),
		billingRateLimitKey(apiKeyID),
		usageBillingBalanceMutationKey(userID),
		usageBillingSubscriptionMutationKey(userID, groupID),
		usageBillingAPIKeyRateLimitMutationKey(apiKeyID),
	}
}

func usageBillingEnqueueDedupKey(requestID string, apiKeyID int64) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(requestID) + "\x00" + strconv.FormatInt(apiKeyID, 10)))
	return usageBillingEnqueueDedupPrefix + hex.EncodeToString(sum[:])
}

func usageBillingPendingBalanceKey(userID int64) string {
	return usageBillingPendingPrefix + "balance:" + strconv.FormatInt(userID, 10)
}

func usageBillingPendingSubscriptionKey(userID, groupID int64) string {
	return usageBillingPendingPrefix + "subscription:" + strconv.FormatInt(userID, 10) + ":" + strconv.FormatInt(groupID, 10)
}

func usageBillingPendingAPIKeyQuotaKey(apiKeyID int64) string {
	return usageBillingPendingPrefix + "api-quota:" + strconv.FormatInt(apiKeyID, 10)
}

func usageBillingPendingAPIKeyRateLimitKey(apiKeyID int64) string {
	return usageBillingPendingPrefix + "api-rate:" + strconv.FormatInt(apiKeyID, 10)
}

func usageBillingBalanceMutationKey(userID int64) string {
	return usageBillingMutationPrefix + "balance:" + strconv.FormatInt(userID, 10)
}

func usageBillingSubscriptionMutationKey(userID, groupID int64) string {
	return usageBillingMutationPrefix + "subscription:" + strconv.FormatInt(userID, 10) + ":" + strconv.FormatInt(groupID, 10)
}

func usageBillingAPIKeyRateLimitMutationKey(apiKeyID int64) string {
	return usageBillingMutationPrefix + "api-rate:" + strconv.FormatInt(apiKeyID, 10)
}

func usageBillingMessageString(message redis.XMessage, key string) string {
	value, _ := message.Values[key].(string)
	return value
}

func usageBillingConsumerNamePrefix() string {
	host, _ := os.Hostname()
	if host == "" {
		host = "unknown"
	}
	return fmt.Sprintf("%s-%d-%d", host, os.Getpid(), time.Now().UnixNano())
}

func waitUsageBillingRetry(ctx context.Context) {
	timer := time.NewTimer(usageBillingConsumerRetryDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func isPermanentUsageBillingError(err error) bool {
	return errors.Is(err, service.ErrUsageBillingRequestConflict) ||
		errors.Is(err, service.ErrUsageBillingRequestIDRequired) ||
		errors.Is(err, service.ErrUserNotFound) ||
		errors.Is(err, service.ErrAPIKeyNotFound) ||
		errors.Is(err, service.ErrAccountNotFound) ||
		errors.Is(err, service.ErrSubscriptionNotFound)
}
