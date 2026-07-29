package repository

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	crActiveKeysKey = "cr:active:keys"
	crActiveUsersKey = "cr:active:users"

	crMinuteTTL   = 15 * time.Minute
	crOneHourTTL  = 2 * time.Hour
	crTwenty6hTTL = 26 * time.Hour
	crEvidenceTTL = 48 * time.Hour
	crMismatchTTL = 30 * time.Minute
	crExemptTTL   = 24 * time.Hour

	crUAWindowSeconds = int64(3600)
	crEvidenceIPCap   = 200
	crEvidenceUACap   = 50
)

type connectionSignalCache struct {
	rdb *redis.Client
}

// NewConnectionSignalCache returns the Redis implementation of ConnectionSignalCache.
func NewConnectionSignalCache(rdb *redis.Client) service.ConnectionSignalCache {
	return &connectionSignalCache{rdb: rdb}
}

func (c *connectionSignalCache) redisNow(ctx context.Context) (time.Time, error) {
	t, err := c.rdb.Time(ctx).Result()
	if err != nil {
		return time.Time{}, fmt.Errorf("redis TIME: %w", err)
	}
	return t, nil
}

func crKeyIPsWin(keyID, win int64) string {
	return fmt.Sprintf("cr:k:%d:ips:%d", keyID, win)
}
func crKeyUAsWin(keyID, win int64) string {
	return fmt.Sprintf("cr:k:%d:uas:%d", keyID, win)
}
func crKeyCntWin(keyID, win int64) string {
	return fmt.Sprintf("cr:k:%d:cnt:%d", keyID, win)
}
func crKeyIPs1h(keyID int64) string  { return fmt.Sprintf("cr:k:%d:ips:1h", keyID) }
func crKeyIPs24h(keyID int64) string { return fmt.Sprintf("cr:k:%d:ips:24h", keyID) }
func crKeyUAs1h(keyID int64) string  { return fmt.Sprintf("cr:k:%d:uas:1h", keyID) }
func crKeyOwner(keyID int64) string  { return fmt.Sprintf("cr:k:%d:owner", keyID) }
func crKeyPrefix(keyID int64) string { return fmt.Sprintf("cr:k:%d:prefix", keyID) }
func crKeyIPSet(keyID int64) string  { return fmt.Sprintf("cr:k:%d:ipset", keyID) }
func crKeyUASet(keyID int64) string  { return fmt.Sprintf("cr:k:%d:uaset", keyID) }
func crUserKeys1h(userID int64) string {
	return fmt.Sprintf("cr:u:%d:keys:1h", userID)
}
func crUserIPs1h(userID int64) string {
	return fmt.Sprintf("cr:u:%d:ips:1h", userID)
}
func crUserMismatch(userID, win int64) string {
	return fmt.Sprintf("cr:u:%d:sb_mismatch:%d", userID, win)
}
func crExemptKey(scope string, id int64) string {
	return fmt.Sprintf("cr:exempt:%s:%d", scope, id)
}

// EmitAlwaysOn implements Tier A always-on pipeline (≤19 cmds with per-request UA trim).
func (c *connectionSignalCache) EmitAlwaysOn(ctx context.Context, sig service.ConnectionSignal, maxActive int, pruneEveryN int, emitSeq uint64) (int, error) {
	if c == nil || c.rdb == nil {
		return 0, nil
	}
	if sig.APIKeyID <= 0 || sig.UserID <= 0 {
		return 0, nil
	}
	if sig.IP == "" {
		return 0, nil
	}
	now := sig.NowUnix
	if now <= 0 {
		t, err := c.redisNow(ctx)
		if err != nil {
			return 0, err
		}
		now = t.Unix()
	}
	win := now / 60
	uaHash := sig.UAHash
	if uaHash == "" {
		uaHash = "empty"
	}
	if maxActive <= 0 {
		maxActive = 50000
	}
	if pruneEveryN <= 0 {
		pruneEveryN = 32
	}

	pipe := c.rdb.Pipeline()
	cmds := 0

	// active tracking
	pipe.ZAdd(ctx, crActiveKeysKey, redis.Z{Score: float64(now), Member: sig.APIKeyID})
	pipe.ZAdd(ctx, crActiveUsersKey, redis.Z{Score: float64(now), Member: sig.UserID})
	cmds += 2

	// minute windows
	pipe.SAdd(ctx, crKeyIPsWin(sig.APIKeyID, win), sig.IP)
	pipe.Expire(ctx, crKeyIPsWin(sig.APIKeyID, win), crMinuteTTL)
	pipe.SAdd(ctx, crKeyUAsWin(sig.APIKeyID, win), uaHash)
	pipe.Expire(ctx, crKeyUAsWin(sig.APIKeyID, win), crMinuteTTL)
	pipe.Incr(ctx, crKeyCntWin(sig.APIKeyID, win))
	pipe.Expire(ctx, crKeyCntWin(sig.APIKeyID, win), crMinuteTTL)
	cmds += 6

	// HLL 1h / 24h
	pipe.PFAdd(ctx, crKeyIPs1h(sig.APIKeyID), sig.IP)
	pipe.Expire(ctx, crKeyIPs1h(sig.APIKeyID), crOneHourTTL)
	pipe.PFAdd(ctx, crKeyIPs24h(sig.APIKeyID), sig.IP)
	pipe.Expire(ctx, crKeyIPs24h(sig.APIKeyID), crTwenty6hTTL)
	cmds += 4

	// R2 authority: sliding 1h ZSET
	pipe.ZAdd(ctx, crKeyUAs1h(sig.APIKeyID), redis.Z{Score: float64(now), Member: uaHash})
	pipe.ZRemRangeByScore(ctx, crKeyUAs1h(sig.APIKeyID), "-inf", strconv.FormatInt(now-crUAWindowSeconds, 10))
	pipe.Expire(ctx, crKeyUAs1h(sig.APIKeyID), crOneHourTTL)
	cmds += 3

	// key → user mapping for worker scoring (48h)
	pipe.Set(ctx, crKeyOwner(sig.APIKeyID), sig.UserID, crEvidenceTTL)
	cmds++
	if sig.KeyPrefix != "" {
		pipe.Set(ctx, crKeyPrefix(sig.APIKeyID), sig.KeyPrefix, crEvidenceTTL)
		cmds++
	}

	// user always-on
	pipe.SAdd(ctx, crUserKeys1h(sig.UserID), sig.APIKeyID)
	pipe.Expire(ctx, crUserKeys1h(sig.UserID), crOneHourTTL)
	pipe.PFAdd(ctx, crUserIPs1h(sig.UserID), sig.IP)
	pipe.Expire(ctx, crUserIPs1h(sig.UserID), crOneHourTTL)
	cmds += 4

	// occasional active prune
	if pruneEveryN > 0 && emitSeq%uint64(pruneEveryN) == 0 {
		cutoff := strconv.FormatInt(now-86400, 10)
		pipe.ZRemRangeByScore(ctx, crActiveKeysKey, "-inf", cutoff)
		pipe.ZRemRangeByScore(ctx, crActiveUsersKey, "-inf", cutoff)
		cmds += 2
		// hard cap: keep highest scores (most recent)
		// ZREMRANGEBYRANK 0 -(max+1) removes oldest when over cap
		// go-redis: keep last maxActive → remove rank 0 .. -(maxActive+1)
		pipe.ZRemRangeByRank(ctx, crActiveKeysKey, 0, int64(-(maxActive + 1)))
		pipe.ZRemRangeByRank(ctx, crActiveUsersKey, 0, int64(-(maxActive + 1)))
		cmds += 2
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return cmds, fmt.Errorf("connection signal emit: %w", err)
	}
	return cmds, nil
}

func (c *connectionSignalCache) EmitEvidence(ctx context.Context, sig service.ConnectionSignal) error {
	if c == nil || c.rdb == nil || sig.APIKeyID <= 0 {
		return nil
	}
	if sig.IP == "" {
		return nil
	}
	now := sig.NowUnix
	if now <= 0 {
		t, err := c.redisNow(ctx)
		if err != nil {
			return err
		}
		now = t.Unix()
	}
	uaHash := sig.UAHash
	if uaHash == "" {
		uaHash = "empty"
	}
	pipe := c.rdb.Pipeline()
	pipe.ZAdd(ctx, crKeyIPSet(sig.APIKeyID), redis.Z{Score: float64(now), Member: sig.IP})
	pipe.Expire(ctx, crKeyIPSet(sig.APIKeyID), crEvidenceTTL)
	pipe.ZAdd(ctx, crKeyUASet(sig.APIKeyID), redis.Z{Score: float64(now), Member: uaHash})
	pipe.Expire(ctx, crKeyUASet(sig.APIKeyID), crEvidenceTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("connection signal evidence: %w", err)
	}
	return nil
}

func (c *connectionSignalCache) IncrSessionMismatch(ctx context.Context, userID int64) error {
	if c == nil || c.rdb == nil || userID <= 0 {
		return nil
	}
	t, err := c.redisNow(ctx)
	if err != nil {
		return err
	}
	win := t.Unix() / 60
	key := crUserMismatch(userID, win)
	pipe := c.rdb.TxPipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, crMismatchTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("session mismatch incr: %w", err)
	}
	return nil
}

func (c *connectionSignalCache) PruneActive(ctx context.Context, maxActive int, olderThan time.Duration) error {
	if c == nil || c.rdb == nil {
		return nil
	}
	if maxActive <= 0 {
		maxActive = 50000
	}
	if olderThan <= 0 {
		olderThan = 24 * time.Hour
	}
	t, err := c.redisNow(ctx)
	if err != nil {
		return err
	}
	cutoff := strconv.FormatInt(t.Add(-olderThan).Unix(), 10)
	pipe := c.rdb.Pipeline()
	pipe.ZRemRangeByScore(ctx, crActiveKeysKey, "-inf", cutoff)
	pipe.ZRemRangeByScore(ctx, crActiveUsersKey, "-inf", cutoff)
	pipe.ZRemRangeByRank(ctx, crActiveKeysKey, 0, int64(-(maxActive + 1)))
	pipe.ZRemRangeByRank(ctx, crActiveUsersKey, 0, int64(-(maxActive + 1)))
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("prune active: %w", err)
	}
	return nil
}

func (c *connectionSignalCache) ActiveCards(ctx context.Context) (int64, int64, error) {
	if c == nil || c.rdb == nil {
		return 0, 0, nil
	}
	pipe := c.rdb.Pipeline()
	k := pipe.ZCard(ctx, crActiveKeysKey)
	u := pipe.ZCard(ctx, crActiveUsersKey)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, 0, err
	}
	return k.Val(), u.Val(), nil
}

func (c *connectionSignalCache) TrimUAWindow(ctx context.Context, keyID int64, nowUnix int64) error {
	if c == nil || c.rdb == nil || keyID <= 0 {
		return nil
	}
	if nowUnix <= 0 {
		t, err := c.redisNow(ctx)
		if err != nil {
			return err
		}
		nowUnix = t.Unix()
	}
	return c.rdb.ZRemRangeByScore(ctx, crKeyUAs1h(keyID), "-inf", strconv.FormatInt(nowUnix-crUAWindowSeconds, 10)).Err()
}

func (c *connectionSignalCache) ReadKeyWindowMetrics(ctx context.Context, keyID, userID int64, nowUnix int64) (*service.ConnectionRiskSubjectMetrics, error) {
	if c == nil || c.rdb == nil {
		return &service.ConnectionRiskSubjectMetrics{}, nil
	}
	if nowUnix <= 0 {
		t, err := c.redisNow(ctx)
		if err != nil {
			return nil, err
		}
		nowUnix = t.Unix()
	}
	_ = c.TrimUAWindow(ctx, keyID, nowUnix)

	win := nowUnix / 60
	m := &service.ConnectionRiskSubjectMetrics{
		APIKeyID: keyID,
		UserID:   userID,
		NowUnix:  nowUnix,
	}
	if prefix, err := c.GetKeyPrefix(ctx, keyID); err == nil {
		m.APIKeyPrefix = prefix
	}

	// Collect last 5 minute IP sets + counts
	ipKeys := make([]string, 0, 5)
	cntKeys := make([]string, 0, 5)
	for i := int64(0); i < 5; i++ {
		w := win - i
		ipKeys = append(ipKeys, crKeyIPsWin(keyID, w))
		cntKeys = append(cntKeys, crKeyCntWin(keyID, w))
	}

	pipe := c.rdb.Pipeline()
	sunion := pipe.SUnion(ctx, ipKeys...)
	cntCmds := make([]*redis.StringCmd, len(cntKeys))
	for i, k := range cntKeys {
		cntCmds[i] = pipe.Get(ctx, k)
	}
	curIP := pipe.SCard(ctx, crKeyIPsWin(keyID, win))
	ua1h := pipe.ZCount(ctx, crKeyUAs1h(keyID), strconv.FormatInt(nowUnix-crUAWindowSeconds, 10), "+inf")
	hll1h := pipe.PFCount(ctx, crKeyIPs1h(keyID))
	hll24h := pipe.PFCount(ctx, crKeyIPs24h(keyID))
	userKeys := pipe.SCard(ctx, crUserKeys1h(userID))
	userHLL := pipe.PFCount(ctx, crUserIPs1h(userID))
	// R7: last 15 minute mismatch counters
	mismatchCmds := make([]*redis.StringCmd, 15)
	for i := int64(0); i < 15; i++ {
		mismatchCmds[i] = pipe.Get(ctx, crUserMismatch(userID, win-i))
	}
	// evidence samples (optional)
	sampleIPs := pipe.ZRevRange(ctx, crKeyIPSet(keyID), 0, 19)
	sampleUAs := pipe.ZRevRange(ctx, crKeyUASet(keyID), 0, 19)

	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		// Pipeline may return redis.Nil for missing GET keys; ignore those.
		// go-redis Pipeline.Exec returns first non-Nil error; GET miss is redis.Nil per cmd.
	}

	if ips, err := sunion.Result(); err == nil {
		m.DistinctIP5m = len(ips)
		if len(ips) > 20 {
			m.SampleIPs = ips[:20]
		} else {
			m.SampleIPs = ips
		}
	}
	req5 := 0
	for _, cmd := range cntCmds {
		if v, err := cmd.Int(); err == nil {
			req5 += v
		}
	}
	m.ReqCount5m = req5
	if v, err := curIP.Result(); err == nil {
		m.DistinctIPCurrentMin = int(v)
	}
	if v, err := cntCmds[0].Int(); err == nil {
		m.ReqCountCurrentMin = v
	}
	if v, err := ua1h.Result(); err == nil {
		m.UACount1h = int(v)
	}
	if v, err := hll1h.Result(); err == nil {
		m.IPHll1h = v
	}
	if v, err := hll24h.Result(); err == nil {
		m.IPHll24h = v
	}
	if v, err := userKeys.Result(); err == nil {
		m.UserKeys1h = int(v)
	}
	if v, err := userHLL.Result(); err == nil {
		m.UserIPHLL1h = v
	}
	mismatch := 0
	for _, cmd := range mismatchCmds {
		if v, err := cmd.Int(); err == nil {
			mismatch += v
		}
	}
	m.SBMismatch15m = mismatch
	if ips, err := sampleIPs.Result(); err == nil && len(ips) > 0 {
		// Prefer evidence samples when available for UI, keep union samples as fallback
		m.SampleIPs = ips
	}
	if uas, err := sampleUAs.Result(); err == nil {
		m.SampleUAHashes = uas
	}

	// Worker-side evidence cap (non hot-path)
	_ = c.rdb.ZRemRangeByRank(ctx, crKeyIPSet(keyID), 0, int64(-(crEvidenceIPCap + 1))).Err()
	_ = c.rdb.ZRemRangeByRank(ctx, crKeyUASet(keyID), 0, int64(-(crEvidenceUACap + 1))).Err()

	return m, nil
}

func (c *connectionSignalCache) TryDedupe(ctx context.Context, dedupeKey string, ttl time.Duration) (bool, error) {
	if c == nil || c.rdb == nil || dedupeKey == "" {
		return true, nil
	}
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	ok, err := c.rdb.SetNX(ctx, "cr:dedupe:"+dedupeKey, "1", ttl).Result()
	return ok, err
}

func (c *connectionSignalCache) IsExempt(ctx context.Context, scope string, id int64) (bool, error) {
	if c == nil || c.rdb == nil {
		return false, nil
	}
	n, err := c.rdb.Exists(ctx, crExemptKey(scope, id)).Result()
	return n > 0, err
}

func (c *connectionSignalCache) SetExempt(ctx context.Context, scope string, id int64, reason string, ttl time.Duration) error {
	if c == nil || c.rdb == nil {
		return nil
	}
	if ttl <= 0 {
		ttl = crExemptTTL
	}
	if reason == "" {
		reason = "exempt"
	}
	return c.rdb.Set(ctx, crExemptKey(scope, id), reason, ttl).Err()
}

func (c *connectionSignalCache) ClearExempt(ctx context.Context, scope string, id int64) error {
	if c == nil || c.rdb == nil {
		return nil
	}
	return c.rdb.Del(ctx, crExemptKey(scope, id)).Err()
}

func (c *connectionSignalCache) ListActiveKeys(ctx context.Context, limit int) ([]int64, error) {
	return c.listActive(ctx, crActiveKeysKey, limit)
}

func (c *connectionSignalCache) ListActiveUsers(ctx context.Context, limit int) ([]int64, error) {
	return c.listActive(ctx, crActiveUsersKey, limit)
}

func (c *connectionSignalCache) GetKeyOwner(ctx context.Context, keyID int64) (int64, error) {
	if c == nil || c.rdb == nil || keyID <= 0 {
		return 0, nil
	}
	val, err := c.rdb.Get(ctx, crKeyOwner(keyID)).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	id, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, nil
	}
	return id, nil
}

func (c *connectionSignalCache) GetKeyPrefix(ctx context.Context, keyID int64) (string, error) {
	if c == nil || c.rdb == nil || keyID <= 0 {
		return "", nil
	}
	val, err := c.rdb.Get(ctx, crKeyPrefix(keyID)).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

func crThrottleKey(keyID int64) string {
	return fmt.Sprintf("cr:throttle:k:%d", keyID)
}
func crThrottleCnt(keyID, win int64) string {
	return fmt.Sprintf("cr:throttle:cnt:%d:%d", keyID, win)
}
func crBaselineDay(keyID int64, day string) string {
	return fmt.Sprintf("cr:baseline:k:%d:day:%s", keyID, day)
}
func crBaselineHash(keyID int64) string {
	return fmt.Sprintf("cr:baseline:k:%d", keyID)
}

func (c *connectionSignalCache) SetThrottle(ctx context.Context, keyID int64, capRPM int, untilUnix int64) error {
	if c == nil || c.rdb == nil || keyID <= 0 {
		return nil
	}
	ttl := time.Until(time.Unix(untilUnix, 0))
	if ttl <= 0 {
		ttl = time.Hour
	}
	val := fmt.Sprintf("%d:%d", capRPM, untilUnix)
	return c.rdb.Set(ctx, crThrottleKey(keyID), val, ttl).Err()
}

func (c *connectionSignalCache) ClearThrottle(ctx context.Context, keyID int64) error {
	if c == nil || c.rdb == nil {
		return nil
	}
	return c.rdb.Del(ctx, crThrottleKey(keyID)).Err()
}

func (c *connectionSignalCache) GetThrottle(ctx context.Context, keyID int64) (int, int64, bool, error) {
	if c == nil || c.rdb == nil {
		return 0, 0, false, nil
	}
	val, err := c.rdb.Get(ctx, crThrottleKey(keyID)).Result()
	if err == redis.Nil {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	var capRPM int
	var until int64
	if _, err := fmt.Sscanf(val, "%d:%d", &capRPM, &until); err != nil {
		return 0, 0, false, nil
	}
	if until > 0 && time.Now().Unix() > until {
		_ = c.rdb.Del(ctx, crThrottleKey(keyID)).Err()
		return 0, 0, false, nil
	}
	return capRPM, until, true, nil
}

func (c *connectionSignalCache) IncrThrottleCount(ctx context.Context, keyID int64) (int, error) {
	if c == nil || c.rdb == nil {
		return 0, nil
	}
	t, err := c.redisNow(ctx)
	if err != nil {
		return 0, err
	}
	win := t.Unix() / 60
	key := crThrottleCnt(keyID, win)
	pipe := c.rdb.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, 2*time.Minute)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return int(incr.Val()), nil
}

func (c *connectionSignalCache) SnapshotBaselineDay(ctx context.Context, keyID int64, day string, count int64) error {
	if c == nil || c.rdb == nil || keyID <= 0 || day == "" {
		return nil
	}
	return c.rdb.Set(ctx, crBaselineDay(keyID, day), count, 14*24*time.Hour).Err()
}

func (c *connectionSignalCache) LoadBaselineSamples(ctx context.Context, keyID int64, days []string) ([]int64, error) {
	out := make([]int64, 0, len(days))
	if c == nil || c.rdb == nil {
		return out, nil
	}
	for _, day := range days {
		val, err := c.rdb.Get(ctx, crBaselineDay(keyID, day)).Int64()
		if err == redis.Nil {
			continue
		}
		if err != nil {
			return out, err
		}
		out = append(out, val)
	}
	return out, nil
}

func (c *connectionSignalCache) SetBaselineP95(ctx context.Context, keyID int64, p95 float64, sampleDays int) error {
	if c == nil || c.rdb == nil {
		return nil
	}
	pipe := c.rdb.Pipeline()
	pipe.HSet(ctx, crBaselineHash(keyID), "ip_p95", p95, "sample_days", sampleDays, "updated_at", time.Now().Unix())
	pipe.Expire(ctx, crBaselineHash(keyID), 30*24*time.Hour)
	_, err := pipe.Exec(ctx)
	return err
}

func (c *connectionSignalCache) GetBaselineP95(ctx context.Context, keyID int64) (float64, int, bool, error) {
	if c == nil || c.rdb == nil {
		return 0, 0, false, nil
	}
	vals, err := c.rdb.HGetAll(ctx, crBaselineHash(keyID)).Result()
	if err != nil {
		return 0, 0, false, err
	}
	if len(vals) == 0 {
		return 0, 0, false, nil
	}
	p95, _ := strconv.ParseFloat(vals["ip_p95"], 64)
	days, _ := strconv.Atoi(vals["sample_days"])
	if p95 <= 0 || days < 3 {
		return 0, days, false, nil
	}
	return p95, days, true, nil
}

func (c *connectionSignalCache) listActive(ctx context.Context, key string, limit int) ([]int64, error) {
	if c == nil || c.rdb == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 2000
	}
	members, err := c.rdb.ZRevRange(ctx, key, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(members))
	for _, m := range members {
		id, err := strconv.ParseInt(m, 10, 64)
		if err != nil {
			continue
		}
		out = append(out, id)
	}
	return out, nil
}
