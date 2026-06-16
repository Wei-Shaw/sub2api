//go:build integration

package repository

import (
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// ServiceQuotaLimiterSnapshotSuite 覆盖只读快照路径：rolling / fixed / concurrency
// 三种 mode + 单 key Snapshot + Pipeline SnapshotMany。
//
// 与 ServiceQuotaLimiterSuite 隔开：那个 suite 集中校验 Acquire 原子性，本 suite
// 关注 Snapshot 的零写副作用与 missing-key 语义，避免互相污染。
type ServiceQuotaLimiterSnapshotSuite struct {
	IntegrationRedisSuite
	limiter service.ServiceQuotaLimiter
}

func TestServiceQuotaLimiterSnapshotSuite(t *testing.T) {
	suite.Run(t, new(ServiceQuotaLimiterSnapshotSuite))
}

func (s *ServiceQuotaLimiterSnapshotSuite) SetupTest() {
	s.IntegrationRedisSuite.SetupTest()
	s.limiter = NewServiceQuotaLimiter(s.rdb)
}

// Test_Snapshot_Rolling_ResetAtUnixMs 验证 rolling 快照返回的 ResetAtUnixMs ≈ now+window。
// 拆出来避免 Test_Snapshot_Rolling_RespectsWindow 同时校验过多内容。
func (s *ServiceQuotaLimiterSnapshotSuite) Test_Snapshot_Rolling_ResetAtUnixMs() {
	const (
		key    = "svcquota:test:snapshot:rolling:reset"
		window = 30 * time.Second
	)
	_, err := s.limiter.Increment(s.ctx, key, 1, window, service.ServiceQuotaWindowRolling)
	s.RequireNoError(err)
	beforeMs := time.Now().UnixMilli()
	snap, err := s.limiter.Snapshot(s.ctx, key, window, service.ServiceQuotaWindowRolling)
	s.RequireNoError(err)
	require.True(s.T(), snap.Exists)
	require.InDelta(s.T(),
		float64(beforeMs+window.Milliseconds()),
		float64(snap.ResetAtUnixMs),
		1500, "rolling ResetAtUnixMs 应 ≈ now + window，容差 1.5s")
}

// Test_Snapshot_Fixed_ResetAtUnixMs 验证 fixed 快照返回的 ResetAtUnixMs 对齐到下一个 window 整点。
// 修复后的 fixed window TTL = window - (now % window)，所以 ResetAtUnixMs ≈ ceil(now/window)*window。
func (s *ServiceQuotaLimiterSnapshotSuite) Test_Snapshot_Fixed_ResetAtUnixMs() {
	const (
		key    = "svcquota:test:snapshot:fixed:reset"
		window = 60 * time.Second
	)
	_, err := s.limiter.Increment(s.ctx, key, 1, window, service.ServiceQuotaWindowFixed)
	s.RequireNoError(err)
	beforeMs := time.Now().UnixMilli()
	snap, err := s.limiter.Snapshot(s.ctx, key, window, service.ServiceQuotaWindowFixed)
	s.RequireNoError(err)
	require.True(s.T(), snap.Exists)
	windowMs := window.Milliseconds()
	expectedResetMs := beforeMs + windowMs - (beforeMs % windowMs)
	require.InDelta(s.T(),
		float64(expectedResetMs),
		float64(snap.ResetAtUnixMs),
		1500, "fixed ResetAtUnixMs 应 ≈ 下一个 window 整点，容差 1.5s")
}

// Test_Snapshot_Concurrency_ResetAtUnixMs 验证 concurrency 快照 ResetAtUnixMs 始终为 0。
func (s *ServiceQuotaLimiterSnapshotSuite) Test_Snapshot_Concurrency_ResetAtUnixMs() {
	const key = "svcquota:test:snapshot:concurrency:reset"
	ok, err := s.limiter.Acquire(s.ctx, key, "m1", 10)
	s.RequireNoError(err)
	require.True(s.T(), ok)
	snap, err := s.limiter.Snapshot(s.ctx, key, 0, "")
	s.RequireNoError(err)
	require.True(s.T(), snap.Exists)
	require.Zero(s.T(), snap.ResetAtUnixMs, "concurrency 快照不展示倒计时 → ResetAtUnixMs=0")
}

// Test_Snapshot_Missing_ResetAtUnixMs 验证 key 不存在时 ResetAtUnixMs=0。
func (s *ServiceQuotaLimiterSnapshotSuite) Test_Snapshot_Missing_ResetAtUnixMs() {
	for _, mode := range []string{service.ServiceQuotaWindowFixed, service.ServiceQuotaWindowRolling} {
		snap, err := s.limiter.Snapshot(s.ctx, "svcquota:test:snapshot:missing:"+mode, time.Minute, mode)
		s.RequireNoError(err)
		require.False(s.T(), snap.Exists, "mode=%s missing key Exists=false", mode)
		require.Zero(s.T(), snap.ResetAtUnixMs, "mode=%s missing key ResetAtUnixMs=0", mode)
	}
}

// Test_Snapshot_Rolling_RespectsWindow 验证 rolling 快照只统计 window 内的 member，
// 且多次 Snapshot 不修改 ZSET（与 Current 相比少了 ZRemRangeByScore 副作用）。
func (s *ServiceQuotaLimiterSnapshotSuite) Test_Snapshot_Rolling_RespectsWindow() {
	const (
		key    = "svcquota:test:snapshot:rolling"
		window = 60 * time.Second
	)

	// 用 Increment 写两条 in-window member，再手动塞一条 out-of-window 的 member。
	_, err := s.limiter.Increment(s.ctx, key, 5, window, service.ServiceQuotaWindowRolling)
	s.RequireNoError(err)
	_, err = s.limiter.Increment(s.ctx, key, 7, window, service.ServiceQuotaWindowRolling)
	s.RequireNoError(err)

	// 直接通过 rdb 注入一条 score 远在 cutoff 之前的 member，
	// 模拟历史遗留数据。Snapshot 必须把它过滤掉。
	staleScore := time.Now().Add(-2 * window).UnixMilli()
	staleMember := strconv.FormatInt(staleScore, 10) + ":0:13"
	s.RequireNoError(s.rdb.ZAdd(s.ctx, key, redis.Z{Score: float64(staleScore), Member: staleMember}).Err())

	beforeCount, err := s.rdb.ZCard(s.ctx, key).Result()
	s.RequireNoError(err)

	snap, err := s.limiter.Snapshot(s.ctx, key, window, service.ServiceQuotaWindowRolling)
	s.RequireNoError(err)
	require.True(s.T(), snap.Exists, "rolling key with members must report Exists=true")
	require.InDelta(s.T(), 12.0, snap.Current, 1e-9, "Snapshot 必须只累加 in-window 的 delta")

	// 再次调用确认零写副作用：ZSET 元素数和原来相等（包括 stale member）。
	snap2, err := s.limiter.Snapshot(s.ctx, key, window, service.ServiceQuotaWindowRolling)
	s.RequireNoError(err)
	require.InDelta(s.T(), 12.0, snap2.Current, 1e-9)

	afterCount, err := s.rdb.ZCard(s.ctx, key).Result()
	s.RequireNoError(err)
	require.Equal(s.T(), beforeCount, afterCount, "Snapshot 不应清理过期 member（监控只读）")
}

// Test_Snapshot_Fixed_KeyMissing 验证 fixed key 不存在 → {0,false}, nil（监控页据此显示"未启用"）。
func (s *ServiceQuotaLimiterSnapshotSuite) Test_Snapshot_Fixed_KeyMissing() {
	snap, err := s.limiter.Snapshot(s.ctx, "svcquota:test:snapshot:missing", time.Minute, service.ServiceQuotaWindowFixed)
	s.RequireNoError(err)
	require.False(s.T(), snap.Exists)
	require.InDelta(s.T(), 0.0, snap.Current, 1e-9)
}

// Test_Snapshot_Fixed_ExistingKey 验证 fixed 写入后 Snapshot 能读到精确浮点。
func (s *ServiceQuotaLimiterSnapshotSuite) Test_Snapshot_Fixed_ExistingKey() {
	const key = "svcquota:test:snapshot:fixed"
	_, err := s.limiter.Increment(s.ctx, key, 15.5, time.Minute, service.ServiceQuotaWindowFixed)
	s.RequireNoError(err)

	snap, err := s.limiter.Snapshot(s.ctx, key, time.Minute, service.ServiceQuotaWindowFixed)
	s.RequireNoError(err)
	require.True(s.T(), snap.Exists)
	require.InDelta(s.T(), 15.5, snap.Current, 1e-9)
}

// Test_Snapshot_Concurrency_RespectsLeakWindow 写 5 个 member，2 个 score 远早于
// leak window cutoff，Snapshot 应只数到 3 个活跃槽位。
func (s *ServiceQuotaLimiterSnapshotSuite) Test_Snapshot_Concurrency_RespectsLeakWindow() {
	const key = "svcquota:test:snapshot:concurrency"

	now := time.Now().Unix()
	staleBase := now - int64(concurrencyAcquireLeakWindow) - 60

	// 3 条 fresh + 2 条 stale，分散写入。
	for i := 0; i < 3; i++ {
		ok, err := s.limiter.Acquire(s.ctx, key, "fresh-"+strconv.Itoa(i), 100)
		s.RequireNoError(err)
		require.True(s.T(), ok)
	}
	for i := 0; i < 2; i++ {
		score := float64(staleBase - int64(i))
		s.RequireNoError(s.rdb.ZAdd(s.ctx, key, redis.Z{Score: score, Member: "stale-" + strconv.Itoa(i)}).Err())
	}

	snap, err := s.limiter.Snapshot(s.ctx, key, 0, "")
	s.RequireNoError(err)
	require.True(s.T(), snap.Exists)
	require.InDelta(s.T(), 3.0, snap.Current, 1e-9, "concurrency Snapshot 必须只数 leak window 内的活跃槽位")
}

// Test_SnapshotMany_Pipeline_MixedModes 构造 9 个 key（3 rolling + 3 fixed + 3 concurrency，
// 各包含若干不存在），SnapshotMany 一次返回 9 个，与单读结果完全一致。
func (s *ServiceQuotaLimiterSnapshotSuite) Test_SnapshotMany_Pipeline_MixedModes() {
	keys, expected := s.setupMixedSnapshotFixtures()

	got, err := s.limiter.SnapshotMany(s.ctx, keys)
	s.RequireNoError(err)
	require.Len(s.T(), got, len(keys))

	for i := range keys {
		require.Equalf(s.T(), expected[i].Exists, got[i].Exists, "keys[%d]=%s Exists 不一致", i, keys[i].Key)
		require.InDeltaf(s.T(), expected[i].Current, got[i].Current, 1e-9, "keys[%d]=%s Current 不一致", i, keys[i].Key)
	}
}

// Test_SnapshotMany_RedisNilNotErr 验证全部 key 不存在时 SnapshotMany 不返回 err，
// 而是为每个 key 返回 {0,false}，与 Snapshot 单点保持一致。
func (s *ServiceQuotaLimiterSnapshotSuite) Test_SnapshotMany_RedisNilNotErr() {
	keys := []service.SnapshotKey{
		{Key: "svcquota:test:snapmany:missing:rolling", Window: time.Minute, Mode: service.ServiceQuotaWindowRolling},
		{Key: "svcquota:test:snapmany:missing:fixed", Window: time.Minute, Mode: service.ServiceQuotaWindowFixed},
		{Key: "svcquota:test:snapmany:missing:concurrency", IsConcurrency: true},
	}

	got, err := s.limiter.SnapshotMany(s.ctx, keys)
	s.RequireNoError(err)
	require.Len(s.T(), got, len(keys))
	for i, snap := range got {
		require.Falsef(s.T(), snap.Exists, "keys[%d]=%s 应返回 Exists=false", i, keys[i].Key)
		require.InDeltaf(s.T(), 0.0, snap.Current, 1e-9, "keys[%d]=%s 应返回 Current=0", i, keys[i].Key)
	}
}

// setupMixedSnapshotFixtures 写入 mixed-mode 数据并返回 SnapshotMany 入参 + 期望值。
// 拆出来避免 Test_SnapshotMany_Pipeline_MixedModes 函数过长。
func (s *ServiceQuotaLimiterSnapshotSuite) setupMixedSnapshotFixtures() ([]service.SnapshotKey, []service.LimiterSnapshot) {
	const window = 30 * time.Second

	// rolling: 1 个有数据 + 1 个空 + 1 个有数据
	rollingHit := "svcquota:test:snapmany:rolling:a"
	rollingMiss := "svcquota:test:snapmany:rolling:b"
	rollingHit2 := "svcquota:test:snapmany:rolling:c"
	_, err := s.limiter.Increment(s.ctx, rollingHit, 3, window, service.ServiceQuotaWindowRolling)
	s.RequireNoError(err)
	_, err = s.limiter.Increment(s.ctx, rollingHit2, 9, window, service.ServiceQuotaWindowRolling)
	s.RequireNoError(err)

	// fixed: 1 个有数据 + 1 个空 + 1 个有数据
	fixedHit := "svcquota:test:snapmany:fixed:a"
	fixedMiss := "svcquota:test:snapmany:fixed:b"
	fixedHit2 := "svcquota:test:snapmany:fixed:c"
	_, err = s.limiter.Increment(s.ctx, fixedHit, 11, window, service.ServiceQuotaWindowFixed)
	s.RequireNoError(err)
	_, err = s.limiter.Increment(s.ctx, fixedHit2, 7.25, window, service.ServiceQuotaWindowFixed)
	s.RequireNoError(err)

	// concurrency: 1 个有数据 + 1 个空 + 1 个有数据
	concHit := "svcquota:test:snapmany:concurrency:a"
	concMiss := "svcquota:test:snapmany:concurrency:b"
	concHit2 := "svcquota:test:snapmany:concurrency:c"
	for i := 0; i < 2; i++ {
		ok, err := s.limiter.Acquire(s.ctx, concHit, "m"+strconv.Itoa(i), 50)
		s.RequireNoError(err)
		require.True(s.T(), ok)
	}
	ok, err := s.limiter.Acquire(s.ctx, concHit2, "m0", 50)
	s.RequireNoError(err)
	require.True(s.T(), ok)

	keys := []service.SnapshotKey{
		{Key: rollingHit, Window: window, Mode: service.ServiceQuotaWindowRolling},
		{Key: rollingMiss, Window: window, Mode: service.ServiceQuotaWindowRolling},
		{Key: rollingHit2, Window: window, Mode: service.ServiceQuotaWindowRolling},
		{Key: fixedHit, Window: window, Mode: service.ServiceQuotaWindowFixed},
		{Key: fixedMiss, Window: window, Mode: service.ServiceQuotaWindowFixed},
		{Key: fixedHit2, Window: window, Mode: service.ServiceQuotaWindowFixed},
		{Key: concHit, IsConcurrency: true},
		{Key: concMiss, IsConcurrency: true},
		{Key: concHit2, IsConcurrency: true},
	}

	// 对照值用单 key Snapshot 逐个读一遍，等价于 SnapshotMany 应该返回的结果。
	// 注意：concurrency 走 ZCount，fixed/rolling 走 GET/ZRange——必须按 sk 字段
	// 路由，不能统一调 mode。
	expected := make([]service.LimiterSnapshot, len(keys))
	for i, sk := range keys {
		var (
			snap service.LimiterSnapshot
			err  error
		)
		switch {
		case sk.IsConcurrency:
			snap, err = s.limiter.Snapshot(s.ctx, sk.Key, 0, "")
		default:
			snap, err = s.limiter.Snapshot(s.ctx, sk.Key, sk.Window, sk.Mode)
		}
		s.RequireNoError(err)
		expected[i] = snap
	}
	return keys, expected
}
