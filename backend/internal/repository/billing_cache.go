package repository

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	billingBalanceKeyPrefix   = "billing:balance:"
	billingSubKeyPrefix       = "billing:sub:"
	billingRateLimitKeyPrefix = "apikey:rate:"
	billingCacheTTL           = 5 * time.Minute
	billingCacheJitter        = 30 * time.Second
	rateLimitCacheTTL         = 7 * 24 * time.Hour // 7 days matches the longest window

	// Rate limit window durations — must match service.RateLimitWindow* constants.
	rateLimitWindow5h = 5 * time.Hour
	rateLimitWindow1d = 24 * time.Hour
	rateLimitWindow7d = 7 * 24 * time.Hour
)

// jitteredTTL 返回带随机抖动的 TTL，防止缓存雪崩
func jitteredTTL() time.Duration {
	// 只做“减法抖动”，确保实际 TTL 不会超过 billingCacheTTL（避免上界预期被打破）。
	if billingCacheJitter <= 0 {
		return billingCacheTTL
	}
	jitter := time.Duration(rand.IntN(int(billingCacheJitter)))
	return billingCacheTTL - jitter
}

// billingBalanceKey generates the Redis key for user balance cache.
func billingBalanceKey(userID int64) string {
	return fmt.Sprintf("%s%d", billingBalanceKeyPrefix, userID)
}

// billingSubKey generates the Redis key for subscription cache.
func billingSubKey(userID, groupID int64) string {
	return fmt.Sprintf("%s%d:%d", billingSubKeyPrefix, userID, groupID)
}

const (
	subFieldStatus       = "status"
	subFieldExpiresAt    = "expires_at"
	subFieldDailyUsage   = "daily_usage"
	subFieldWeeklyUsage  = "weekly_usage"
	subFieldMonthlyUsage = "monthly_usage"
	subFieldVersion      = "version"
)

// billingRateLimitKey generates the Redis key for API key rate limit cache.
func billingRateLimitKey(keyID int64) string {
	return fmt.Sprintf("%s%d", billingRateLimitKeyPrefix, keyID)
}

const (
	rateLimitFieldUsage5h  = "usage_5h"
	rateLimitFieldUsage1d  = "usage_1d"
	rateLimitFieldUsage7d  = "usage_7d"
	rateLimitFieldWindow5h = "window_5h"
	rateLimitFieldWindow1d = "window_1d"
	rateLimitFieldWindow7d = "window_7d"
)

var (
	deductBalanceScript = redis.NewScript(`
		local current = redis.call('GET', KEYS[1])
		if current == false then
			return 0
		end
		local newVal = tonumber(current) - tonumber(ARGV[1])
		redis.call('SET', KEYS[1], newVal)
		redis.call('EXPIRE', KEYS[1], ARGV[2])
		return 1
	`)

	updateSubUsageScript = redis.NewScript(`
		local exists = redis.call('EXISTS', KEYS[1])
		if exists == 0 then
			return 0
		end
		local cost = tonumber(ARGV[1])
		redis.call('HINCRBYFLOAT', KEYS[1], 'daily_usage', cost)
		redis.call('HINCRBYFLOAT', KEYS[1], 'weekly_usage', cost)
		redis.call('HINCRBYFLOAT', KEYS[1], 'monthly_usage', cost)
		redis.call('EXPIRE', KEYS[1], ARGV[2])
		return 1
	`)

	// updateRateLimitUsageScript atomically increments all three rate limit usage counters
	// with window expiration checking. If a window has expired, its usage is reset to cost
	// (instead of accumulated) and the window timestamp is updated, matching the DB-side
	// IncrementRateLimitUsage semantics.
	//
	// ARGV: [1]=cost, [2]=ttl_seconds, [3]=now_unix, [4]=window_5h_seconds, [5]=window_1d_seconds, [6]=window_7d_seconds
	updateRateLimitUsageScript = redis.NewScript(`
		local exists = redis.call('EXISTS', KEYS[1])
		if exists == 0 then
			return 0
		end
		local cost = tonumber(ARGV[1])
		local now = tonumber(ARGV[3])
		local win5h = tonumber(ARGV[4])
		local win1d = tonumber(ARGV[5])
		local win7d = tonumber(ARGV[6])

		-- Helper: check if window is expired and update usage + window accordingly
		-- Returns nothing, modifies the hash in-place.
		local function update_window(usage_field, window_field, window_duration)
			local w = tonumber(redis.call('HGET', KEYS[1], window_field) or 0)
			if w == 0 or (now - w) >= window_duration then
				-- Window expired or never started: reset usage to cost, start new window
				redis.call('HSET', KEYS[1], usage_field, tostring(cost))
				redis.call('HSET', KEYS[1], window_field, tostring(now))
			else
				-- Window still valid: accumulate
				redis.call('HINCRBYFLOAT', KEYS[1], usage_field, cost)
			end
		end

		update_window('usage_5h', 'window_5h', win5h)
		update_window('usage_1d', 'window_1d', win1d)
		update_window('usage_7d', 'window_7d', win7d)
		redis.call('EXPIRE', KEYS[1], ARGV[2])
		return 1
	`)
)

type billingCache struct {
	rdb *redis.Client
}

func NewBillingCache(rdb *redis.Client) service.BillingCache {
	return &billingCache{rdb: rdb}
}

func (c *billingCache) GetUserBalance(ctx context.Context, userID int64) (float64, error) {
	key := billingBalanceKey(userID)
	val, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(val, 64)
}

func (c *billingCache) SetUserBalance(ctx context.Context, userID int64, balance float64) error {
	key := billingBalanceKey(userID)
	return c.rdb.Set(ctx, key, balance, jitteredTTL()).Err()
}

func (c *billingCache) DeductUserBalance(ctx context.Context, userID int64, amount float64) error {
	key := billingBalanceKey(userID)
	_, err := deductBalanceScript.Run(ctx, c.rdb, []string{key}, amount, int(jitteredTTL().Seconds())).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		log.Printf("Warning: deduct balance cache failed for user %d: %v", userID, err)
		return err
	}
	return nil
}

func (c *billingCache) InvalidateUserBalance(ctx context.Context, userID int64) error {
	key := billingBalanceKey(userID)
	return c.rdb.Del(ctx, key).Err()
}

func (c *billingCache) GetSubscriptionCache(ctx context.Context, userID, groupID int64) (*service.SubscriptionCacheData, error) {
	key := billingSubKey(userID, groupID)
	result, err := c.rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, redis.Nil
	}
	return c.parseSubscriptionCache(result)
}

func (c *billingCache) parseSubscriptionCache(data map[string]string) (*service.SubscriptionCacheData, error) {
	result := &service.SubscriptionCacheData{}

	result.Status = data[subFieldStatus]
	if result.Status == "" {
		return nil, errors.New("invalid cache: missing status")
	}

	if expiresStr, ok := data[subFieldExpiresAt]; ok {
		expiresAt, err := strconv.ParseInt(expiresStr, 10, 64)
		if err == nil {
			result.ExpiresAt = time.Unix(expiresAt, 0)
		}
	}

	if dailyStr, ok := data[subFieldDailyUsage]; ok {
		result.DailyUsage, _ = strconv.ParseFloat(dailyStr, 64)
	}

	if weeklyStr, ok := data[subFieldWeeklyUsage]; ok {
		result.WeeklyUsage, _ = strconv.ParseFloat(weeklyStr, 64)
	}

	if monthlyStr, ok := data[subFieldMonthlyUsage]; ok {
		result.MonthlyUsage, _ = strconv.ParseFloat(monthlyStr, 64)
	}

	if versionStr, ok := data[subFieldVersion]; ok {
		result.Version, _ = strconv.ParseInt(versionStr, 10, 64)
	}

	return result, nil
}

func (c *billingCache) SetSubscriptionCache(ctx context.Context, userID, groupID int64, data *service.SubscriptionCacheData) error {
	if data == nil {
		return nil
	}

	key := billingSubKey(userID, groupID)

	fields := map[string]any{
		subFieldStatus:       data.Status,
		subFieldExpiresAt:    data.ExpiresAt.Unix(),
		subFieldDailyUsage:   data.DailyUsage,
		subFieldWeeklyUsage:  data.WeeklyUsage,
		subFieldMonthlyUsage: data.MonthlyUsage,
		subFieldVersion:      data.Version,
	}

	pipe := c.rdb.Pipeline()
	pipe.HSet(ctx, key, fields)
	pipe.Expire(ctx, key, jitteredTTL())
	_, err := pipe.Exec(ctx)
	return err
}

func (c *billingCache) UpdateSubscriptionUsage(ctx context.Context, userID, groupID int64, cost float64) error {
	key := billingSubKey(userID, groupID)
	_, err := updateSubUsageScript.Run(ctx, c.rdb, []string{key}, cost, int(jitteredTTL().Seconds())).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		log.Printf("Warning: update subscription usage cache failed for user %d group %d: %v", userID, groupID, err)
		return err
	}
	return nil
}

func (c *billingCache) InvalidateSubscriptionCache(ctx context.Context, userID, groupID int64) error {
	key := billingSubKey(userID, groupID)
	return c.rdb.Del(ctx, key).Err()
}

func (c *billingCache) GetAPIKeyRateLimit(ctx context.Context, keyID int64) (*service.APIKeyRateLimitCacheData, error) {
	key := billingRateLimitKey(keyID)
	result, err := c.rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, redis.Nil
	}
	data := &service.APIKeyRateLimitCacheData{}
	if v, ok := result[rateLimitFieldUsage5h]; ok {
		data.Usage5h, _ = strconv.ParseFloat(v, 64)
	}
	if v, ok := result[rateLimitFieldUsage1d]; ok {
		data.Usage1d, _ = strconv.ParseFloat(v, 64)
	}
	if v, ok := result[rateLimitFieldUsage7d]; ok {
		data.Usage7d, _ = strconv.ParseFloat(v, 64)
	}
	if v, ok := result[rateLimitFieldWindow5h]; ok {
		data.Window5h, _ = strconv.ParseInt(v, 10, 64)
	}
	if v, ok := result[rateLimitFieldWindow1d]; ok {
		data.Window1d, _ = strconv.ParseInt(v, 10, 64)
	}
	if v, ok := result[rateLimitFieldWindow7d]; ok {
		data.Window7d, _ = strconv.ParseInt(v, 10, 64)
	}
	return data, nil
}

func (c *billingCache) SetAPIKeyRateLimit(ctx context.Context, keyID int64, data *service.APIKeyRateLimitCacheData) error {
	if data == nil {
		return nil
	}
	key := billingRateLimitKey(keyID)
	fields := map[string]any{
		rateLimitFieldUsage5h:  data.Usage5h,
		rateLimitFieldUsage1d:  data.Usage1d,
		rateLimitFieldUsage7d:  data.Usage7d,
		rateLimitFieldWindow5h: data.Window5h,
		rateLimitFieldWindow1d: data.Window1d,
		rateLimitFieldWindow7d: data.Window7d,
	}
	pipe := c.rdb.Pipeline()
	pipe.HSet(ctx, key, fields)
	pipe.Expire(ctx, key, rateLimitCacheTTL)
	_, err := pipe.Exec(ctx)
	return err
}

func (c *billingCache) UpdateAPIKeyRateLimitUsage(ctx context.Context, keyID int64, cost float64) error {
	key := billingRateLimitKey(keyID)
	now := time.Now().Unix()
	_, err := updateRateLimitUsageScript.Run(ctx, c.rdb, []string{key},
		cost,
		int(rateLimitCacheTTL.Seconds()),
		now,
		int(rateLimitWindow5h.Seconds()),
		int(rateLimitWindow1d.Seconds()),
		int(rateLimitWindow7d.Seconds()),
	).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		log.Printf("Warning: update rate limit usage cache failed for api key %d: %v", keyID, err)
		return err
	}
	return nil
}

func (c *billingCache) InvalidateAPIKeyRateLimit(ctx context.Context, keyID int64) error {
	key := billingRateLimitKey(keyID)
	return c.rdb.Del(ctx, key).Err()
}

// ============================================
// user × platform quota 缓存
// ============================================

// userPlatformQuotaCacheKey 构造 Redis key
func userPlatformQuotaCacheKey(userID int64, platform string) string {
	return fmt.Sprintf("billing:user_platform_quota:%d:%s", userID, platform)
}

func (c *billingCache) GetUserPlatformQuotaCache(ctx context.Context, userID int64, platform string) (*service.UserPlatformQuotaCacheEntry, bool, error) {
	key := userPlatformQuotaCacheKey(userID, platform)
	fields := []string{
		"daily_usage", "weekly_usage", "monthly_usage", "version", "schema_version",
		"daily_limit", "weekly_limit", "monthly_limit",
		"daily_window_start", "weekly_window_start", "monthly_window_start",
	}
	vals, err := c.rdb.HMGet(ctx, key, fields...).Result()
	if err != nil {
		return nil, false, err
	}
	// 前4个全为nil → key 不存在
	if vals[0] == nil && vals[1] == nil && vals[2] == nil && vals[3] == nil {
		return nil, false, nil
	}
	parseFloat := func(v any) float64 {
		if v == nil {
			return 0
		}
		s, ok := v.(string)
		if !ok {
			return 0
		}
		f, _ := strconv.ParseFloat(s, 64)
		return f
	}
	parseFloatPtr := func(v any) *float64 {
		if v == nil {
			return nil
		}
		s, ok := v.(string)
		if !ok || s == "" {
			return nil
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil
		}
		return &f
	}
	parseTimePtr := func(v any) *time.Time {
		if v == nil {
			return nil
		}
		s, ok := v.(string)
		if !ok || s == "" {
			return nil
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil
		}
		t := time.Unix(n, 0).UTC()
		return &t
	}
	parseInt64 := func(v any) int64 {
		if v == nil {
			return 0
		}
		s, ok := v.(string)
		if !ok {
			return 0
		}
		n, _ := strconv.ParseInt(s, 10, 64)
		return n
	}
	return &service.UserPlatformQuotaCacheEntry{
		DailyUsageUSD:      parseFloat(vals[0]),
		WeeklyUsageUSD:     parseFloat(vals[1]),
		MonthlyUsageUSD:    parseFloat(vals[2]),
		Version:            parseInt64(vals[3]),
		SchemaVersion:      parseInt64(vals[4]),
		DailyLimitUSD:      parseFloatPtr(vals[5]),
		WeeklyLimitUSD:     parseFloatPtr(vals[6]),
		MonthlyLimitUSD:    parseFloatPtr(vals[7]),
		DailyWindowStart:   parseTimePtr(vals[8]),
		WeeklyWindowStart:  parseTimePtr(vals[9]),
		MonthlyWindowStart: parseTimePtr(vals[10]),
	}, true, nil
}

func (c *billingCache) SetUserPlatformQuotaCache(ctx context.Context, userID int64, platform string, entry *service.UserPlatformQuotaCacheEntry, ttl time.Duration) error {
	if entry == nil {
		return nil
	}
	key := userPlatformQuotaCacheKey(userID, platform)
	pipe := c.rdb.TxPipeline()

	// 浮点可空字段：nil → 空字符串（读取时 parseFloatPtr 返回 nil，表示无限额）
	fmtFloatPtr := func(p *float64) string {
		if p == nil {
			return ""
		}
		return strconv.FormatFloat(*p, 'f', -1, 64)
	}
	// time.Time 可空字段：nil → 空字符串；有值 → unix 秒
	fmtTimePtr := func(p *time.Time) string {
		if p == nil {
			return ""
		}
		return strconv.FormatInt(p.Unix(), 10)
	}

	pipe.HSet(ctx, key,
		"daily_usage", entry.DailyUsageUSD,
		"weekly_usage", entry.WeeklyUsageUSD,
		"monthly_usage", entry.MonthlyUsageUSD,
		"version", entry.Version,
		"schema_version", entry.SchemaVersion,
		"daily_limit", fmtFloatPtr(entry.DailyLimitUSD),
		"weekly_limit", fmtFloatPtr(entry.WeeklyLimitUSD),
		"monthly_limit", fmtFloatPtr(entry.MonthlyLimitUSD),
		"daily_window_start", fmtTimePtr(entry.DailyWindowStart),
		"weekly_window_start", fmtTimePtr(entry.WeeklyWindowStart),
		"monthly_window_start", fmtTimePtr(entry.MonthlyWindowStart),
	)
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (c *billingCache) DeleteUserPlatformQuotaCache(ctx context.Context, userID int64, platform string) error {
	return c.rdb.Del(ctx, userPlatformQuotaCacheKey(userID, platform)).Err()
}

// updateUserPlatformQuotaUsageScript 缓存累加：EXISTS + schema_version 双重守卫。
// 旧版 entry（schema_version != ARGV[3]，包括缺字段的 0 值）不参与累加，由上层走 DB fallback 后
// SetCache 重建为新版 entry —— 若此处仍累加，上层覆盖时会丢失这部分增量，导致 Redis usage 比真实偏小。
// key 不存在同样跳过（由下次 SetCache 重建）。
// KEYS[1] = hash key
// ARGV[1] = cost (string float)
// ARGV[2] = ttl seconds
// ARGV[3] = expected schema_version (Go 侧 UserPlatformQuotaCacheSchemaV1)
const updateUserPlatformQuotaUsageScript = `
if redis.call("EXISTS", KEYS[1]) == 0 then
    return 0
end
local ver = redis.call("HGET", KEYS[1], "schema_version")
if ver == false or tonumber(ver) ~= tonumber(ARGV[3]) then
    return 0
end
redis.call("HINCRBYFLOAT", KEYS[1], "daily_usage", ARGV[1])
redis.call("HINCRBYFLOAT", KEYS[1], "weekly_usage", ARGV[1])
redis.call("HINCRBYFLOAT", KEYS[1], "monthly_usage", ARGV[1])
redis.call("HINCRBY", KEYS[1], "version", 1)
redis.call("EXPIRE", KEYS[1], ARGV[2])
return 1
`

func (c *billingCache) IncrUserPlatformQuotaUsageCache(ctx context.Context, userID int64, platform string, cost float64, ttl time.Duration) error {
	key := userPlatformQuotaCacheKey(userID, platform)
	_, err := c.rdb.Eval(ctx, updateUserPlatformQuotaUsageScript, []string{key},
		strconv.FormatFloat(cost, 'f', -1, 64),
		int(ttl.Seconds()),
		service.UserPlatformQuotaCacheSchemaV1,
	).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return err
	}
	return nil
}
