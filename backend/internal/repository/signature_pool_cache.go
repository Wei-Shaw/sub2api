package repository

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service/signature"
	"github.com/redis/go-redis/v9"
)

// signaturePoolKeyPrefix is the Redis key prefix for thinking-signature pools.
// Full key shape: sub2api:sig_pool:<bucket>
// where <bucket> is returned by signature.BucketFor(accountType, accountID).
const signaturePoolKeyPrefix = "sub2api:sig_pool:"

// signaturePoolKeyTTLFactor controls how long the ZSET key itself survives
// without activity. We extend it to (SoftTTL * this factor) so that keys
// outlive their oldest entry by a margin — avoids losing a populated pool
// to Redis eviction during a quiet period shorter than SoftTTL.
const signaturePoolKeyTTLFactor = 24

var signaturePoolAddScript = redis.NewScript(`
	local key = KEYS[1]
	local member = ARGV[1]
	local score = tonumber(ARGV[2])
	local capacity = tonumber(ARGV[3])
	local softTTL = tonumber(ARGV[4])
	local keyTTL = tonumber(ARGV[5])

	-- Insert / refresh this signature.
	redis.call('ZADD', key, score, member)

	-- Lazy expiry: drop entries older than soft TTL.
	if softTTL > 0 then
		local cutoff = score - softTTL
		redis.call('ZREMRANGEBYSCORE', key, '-inf', '(' .. tostring(cutoff))
	end

	-- Trim to capacity (keep newest).
	if capacity > 0 then
		local sz = redis.call('ZCARD', key)
		if sz > capacity then
			redis.call('ZREMRANGEBYRANK', key, 0, sz - capacity - 1)
		end
	end

	-- Refresh key-level TTL so a quiet pool does not vanish unexpectedly.
	if keyTTL > 0 then
		redis.call('EXPIRE', key, keyTTL)
	end

	return 1
`)

type signaturePoolCache struct {
	rdb            *redis.Client
	softTTLSeconds int
}

// NewSignaturePoolCache builds a Redis-backed SignaturePool.
// softTTL is the per-entry soft expiry used for lazy cleanup on Add.
// Passing 0 disables soft expiry (entries live until trimmed by capacity).
func NewSignaturePoolCache(rdb *redis.Client, softTTL time.Duration) signature.SignaturePool {
	secs := int(softTTL.Seconds())
	if secs < 0 {
		secs = 0
	}
	return &signaturePoolCache{
		rdb:            rdb,
		softTTLSeconds: secs,
	}
}

func signaturePoolKey(bucket string) string {
	return signaturePoolKeyPrefix + bucket
}

func (c *signaturePoolCache) Add(ctx context.Context, bucket string, sig string, at time.Time, capacity int) error {
	if bucket == "" || sig == "" {
		return nil
	}
	key := signaturePoolKey(bucket)
	score := at.Unix()
	keyTTL := c.softTTLSeconds * signaturePoolKeyTTLFactor
	if keyTTL <= 0 && c.softTTLSeconds > 0 {
		keyTTL = c.softTTLSeconds // fallback if factor math underflows
	}
	_, err := signaturePoolAddScript.Run(ctx, c.rdb, []string{key},
		sig, strconv.FormatInt(score, 10), capacity, c.softTTLSeconds, keyTTL,
	).Int()
	if err != nil {
		return fmt.Errorf("signature pool add: %w", err)
	}
	return nil
}

func (c *signaturePoolCache) TopN(ctx context.Context, bucket string, n int) ([]string, error) {
	if bucket == "" || n <= 0 {
		return nil, nil
	}
	key := signaturePoolKey(bucket)
	// Lazy expiry: we intentionally do NOT filter by score on read — the user
	// requirement is "avoid having no signature available". Entries past soft
	// TTL are cleaned only on the next Add.
	sigs, err := c.rdb.ZRevRange(ctx, key, 0, int64(n-1)).Result()
	if err != nil {
		return nil, fmt.Errorf("signature pool top-n: %w", err)
	}
	return sigs, nil
}

func (c *signaturePoolCache) Size(ctx context.Context, bucket string) (int64, error) {
	if bucket == "" {
		return 0, nil
	}
	key := signaturePoolKey(bucket)
	return c.rdb.ZCard(ctx, key).Result()
}
