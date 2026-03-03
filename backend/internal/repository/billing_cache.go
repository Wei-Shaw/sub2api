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
	billingBalanceKeyPrefix  = "billing:balance:"
	billingSubKeyPrefix      = "billing:sub:"
	billingRateLimitKeyPrefix = "apikey:rate:"
	billingCacheTTL          = 5 * time.Minute
	billingCacheJitter       = 30 * time.Second
	rateLimitCacheTTL        = 7 * 24 * time.Hour // 7 days matches the longest window
)

// jitteredTTL 返回带随机抖动的 TTL，防止缓存雪崩
func jitteredTTL() time.Duration {
	// 只做“减法抖动”，确保实际 TTL 不会超过 billingCacheTTL（避免上界预期被打破）。
	if billingCacheJitter <= 0 {
		return billingCacheTTL
REDACTED
	jitter := time.Duration(rand.IntN(int(billingCacheJitter)))
	return billingCacheTTL - jitter
REDACTED

// billingBalanceKey generates the Redis key for user balance cache.
func billingBalanceKey(userID int64) string {
	return fmt.Sprintf("%s%d", billingBalanceKeyPrefix, userID)
REDACTED

// billingSubKey generates the Redis key for subscription cache.
func billingSubKey(userID, groupID int64) string {
	return fmt.Sprintf("%s%d:%d", billingSubKeyPrefix, userID, groupID)
REDACTED

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
REDACTED

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

	// updateRateLimitUsageScript atomically increments all three rate limit usage counters.
	// Returns 0 if the key doesn't exist (cache miss), 1 on success.
	updateRateLimitUsageScript = redis.NewScript(`
		local exists = redis.call('EXISTS', KEYS[1])
		if exists == 0 then
			return 0
		end
		local cost = tonumber(ARGV[1])
		redis.call('HINCRBYFLOAT', KEYS[1], 'usage_5h', cost)
		redis.call('HINCRBYFLOAT', KEYS[1], 'usage_1d', cost)
		redis.call('HINCRBYFLOAT', KEYS[1], 'usage_7d', cost)
		redis.call('EXPIRE', KEYS[1], ARGV[2])
		return 1
	`)
)

type billingCache struct {
	rdb *redis.Client
REDACTED

func NewBillingCache(rdb *redis.Client) service.BillingCache {
	return &billingCache{rdb: rdbREDACTED
REDACTED

func (c *billingCache) GetUserBalance(ctx context.Context, userID int64) (float64, error) {
	key := billingBalanceKey(userID)
	val, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		return 0, err
REDACTED
	return strconv.ParseFloat(val, 64)
REDACTED

func (c *billingCache) SetUserBalance(ctx context.Context, userID int64, balance float64) error {
	key := billingBalanceKey(userID)
	return c.rdb.Set(ctx, key, balance, jitteredTTL()).Err()
REDACTED

func (c *billingCache) DeductUserBalance(ctx context.Context, userID int64, amount float64) error {
	key := billingBalanceKey(userID)
	_, err := deductBalanceScript.Run(ctx, c.rdb, []string{keyREDACTED, amount, int(jitteredTTL().Seconds())).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		log.Printf("Warning: deduct balance cache failed for user %d: %v", userID, err)
		return err
REDACTED
	return nil
REDACTED

func (c *billingCache) InvalidateUserBalance(ctx context.Context, userID int64) error {
	key := billingBalanceKey(userID)
	return c.rdb.Del(ctx, key).Err()
REDACTED

func (c *billingCache) GetSubscriptionCache(ctx context.Context, userID, groupID int64) (*service.SubscriptionCacheData, error) {
	key := billingSubKey(userID, groupID)
	result, err := c.rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, err
REDACTED
	if len(result) == 0 {
		return nil, redis.Nil
REDACTED
	return c.parseSubscriptionCache(result)
REDACTED

func (c *billingCache) parseSubscriptionCache(data map[string]string) (*service.SubscriptionCacheData, error) {
	result := &service.SubscriptionCacheData{REDACTED

	result.Status = data[subFieldStatus]
	if result.Status == "" {
		return nil, errors.New("invalid cache: missing status")
REDACTED

	if expiresStr, ok := data[subFieldExpiresAt]; ok {
		expiresAt, err := strconv.ParseInt(expiresStr, 10, 64)
		if err == nil {
			result.ExpiresAt = time.Unix(expiresAt, 0)
	REDACTED
REDACTED

	if dailyStr, ok := data[subFieldDailyUsage]; ok {
		result.DailyUsage, _ = strconv.ParseFloat(dailyStr, 64)
REDACTED

	if weeklyStr, ok := data[subFieldWeeklyUsage]; ok {
		result.WeeklyUsage, _ = strconv.ParseFloat(weeklyStr, 64)
REDACTED

	if monthlyStr, ok := data[subFieldMonthlyUsage]; ok {
		result.MonthlyUsage, _ = strconv.ParseFloat(monthlyStr, 64)
REDACTED

	if versionStr, ok := data[subFieldVersion]; ok {
		result.Version, _ = strconv.ParseInt(versionStr, 10, 64)
REDACTED

	return result, nil
REDACTED

func (c *billingCache) SetSubscriptionCache(ctx context.Context, userID, groupID int64, data *service.SubscriptionCacheData) error {
	if data == nil {
		return nil
REDACTED

	key := billingSubKey(userID, groupID)

	fields := map[string]any{
		subFieldStatus:       data.Status,
		subFieldExpiresAt:    data.ExpiresAt.Unix(),
		subFieldDailyUsage:   data.DailyUsage,
		subFieldWeeklyUsage:  data.WeeklyUsage,
		subFieldMonthlyUsage: data.MonthlyUsage,
		subFieldVersion:      data.Version,
REDACTED

	pipe := c.rdb.Pipeline()
	pipe.HSet(ctx, key, fields)
	pipe.Expire(ctx, key, jitteredTTL())
	_, err := pipe.Exec(ctx)
	return err
REDACTED

func (c *billingCache) UpdateSubscriptionUsage(ctx context.Context, userID, groupID int64, cost float64) error {
	key := billingSubKey(userID, groupID)
	_, err := updateSubUsageScript.Run(ctx, c.rdb, []string{keyREDACTED, cost, int(jitteredTTL().Seconds())).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		log.Printf("Warning: update subscription usage cache failed for user %d group %d: %v", userID, groupID, err)
		return err
REDACTED
	return nil
REDACTED

func (c *billingCache) InvalidateSubscriptionCache(ctx context.Context, userID, groupID int64) error {
	key := billingSubKey(userID, groupID)
	return c.rdb.Del(ctx, key).Err()
REDACTED

func (c *billingCache) GetAPIKeyRateLimit(ctx context.Context, keyID int64) (*service.APIKeyRateLimitCacheData, error) {
	key := billingRateLimitKey(keyID)
	result, err := c.rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, err
REDACTED
	if len(result) == 0 {
		return nil, redis.Nil
REDACTED
	data := &service.APIKeyRateLimitCacheData{REDACTED
	if v, ok := result[rateLimitFieldUsage5h]; ok {
		data.Usage5h, _ = strconv.ParseFloat(v, 64)
REDACTED
	if v, ok := result[rateLimitFieldUsage1d]; ok {
		data.Usage1d, _ = strconv.ParseFloat(v, 64)
REDACTED
	if v, ok := result[rateLimitFieldUsage7d]; ok {
		data.Usage7d, _ = strconv.ParseFloat(v, 64)
REDACTED
	if v, ok := result[rateLimitFieldWindow5h]; ok {
		data.Window5h, _ = strconv.ParseInt(v, 10, 64)
REDACTED
	if v, ok := result[rateLimitFieldWindow1d]; ok {
		data.Window1d, _ = strconv.ParseInt(v, 10, 64)
REDACTED
	if v, ok := result[rateLimitFieldWindow7d]; ok {
		data.Window7d, _ = strconv.ParseInt(v, 10, 64)
REDACTED
	return data, nil
REDACTED

func (c *billingCache) SetAPIKeyRateLimit(ctx context.Context, keyID int64, data *service.APIKeyRateLimitCacheData) error {
	if data == nil {
		return nil
REDACTED
	key := billingRateLimitKey(keyID)
	fields := map[string]any{
		rateLimitFieldUsage5h:  data.Usage5h,
		rateLimitFieldUsage1d:  data.Usage1d,
		rateLimitFieldUsage7d:  data.Usage7d,
		rateLimitFieldWindow5h: data.Window5h,
		rateLimitFieldWindow1d: data.Window1d,
		rateLimitFieldWindow7d: data.Window7d,
REDACTED
	pipe := c.rdb.Pipeline()
	pipe.HSet(ctx, key, fields)
	pipe.Expire(ctx, key, rateLimitCacheTTL)
	_, err := pipe.Exec(ctx)
	return err
REDACTED

func (c *billingCache) UpdateAPIKeyRateLimitUsage(ctx context.Context, keyID int64, cost float64) error {
	key := billingRateLimitKey(keyID)
	_, err := updateRateLimitUsageScript.Run(ctx, c.rdb, []string{keyREDACTED, cost, int(rateLimitCacheTTL.Seconds())).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		log.Printf("Warning: update rate limit usage cache failed for api key %d: %v", keyID, err)
		return err
REDACTED
	return nil
REDACTED

func (c *billingCache) InvalidateAPIKeyRateLimit(ctx context.Context, keyID int64) error {
	key := billingRateLimitKey(keyID)
	return c.rdb.Del(ctx, key).Err()
REDACTED
