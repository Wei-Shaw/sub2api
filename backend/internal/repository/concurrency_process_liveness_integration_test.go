//go:build integration

package repository

import (
	"github.com/stretchr/testify/require"
)

// 真实 Redis 上验证蓝绿发布场景：存活同伴的在途槽位与排队计数保留，已死进程的残留照旧清掉。
func (s *ConcurrencyCacheSuite) TestCleanupStaleProcessSlots_KeepsLivePeerSlots() {
	require.NoError(s.T(), s.rdb.Set(s.ctx, legacyWaitSweepMarkerKey, "1", 0).Err())
	// 共用带用例键前缀钩子的客户端；进程身份由请求前缀区分，不依赖连接。
	blue := NewConcurrencyCache(s.rdb, testSlotTTLMinutes, int(testSlotTTL.Seconds())).(*concurrencyCache)
	green := s.rawCache

	acquired, err := s.rawCache.AcquireAccountSlot(s.ctx, 4901, 1, "rdead-1")
	require.NoError(s.T(), err)
	require.True(s.T(), acquired)
	_, err = s.rawCache.IncrementAccountWaitCount(s.ctx, 4901, 5)
	require.NoError(s.T(), err)

	require.NoError(s.T(), blue.HeartbeatProcess(s.ctx, "rblue", "sub2api-blue"))
	acquired, err = blue.AcquireAccountSlot(s.ctx, 4902, 1, "rblue-1")
	require.NoError(s.T(), err)
	require.True(s.T(), acquired)
	acquired, err = blue.AcquireUserSlot(s.ctx, 4903, 1, "rblue-2")
	require.NoError(s.T(), err)
	require.True(s.T(), acquired)
	_, err = blue.IncrementAccountWaitCount(s.ctx, 4902, 5)
	require.NoError(s.T(), err)

	require.NoError(s.T(), green.HeartbeatProcess(s.ctx, "rgreen", "sub2api-green"))
	require.NoError(s.T(), green.CleanupStaleProcessSlots(s.ctx, "rgreen"))

	exists, err := s.rdb.Exists(s.ctx, accountSlotKey(4901), accountWaitKey(4901)).Result()
	require.NoError(s.T(), err)
	require.Zero(s.T(), exists, "dead process residue and its wait counter should be purged")

	members, err := s.rdb.ZRange(s.ctx, accountSlotKey(4902), 0, -1).Result()
	require.NoError(s.T(), err)
	require.Equal(s.T(), []string{"rblue-1"}, members)
	members, err = s.rdb.ZRange(s.ctx, userSlotKey(4903), 0, -1).Result()
	require.NoError(s.T(), err)
	require.Equal(s.T(), []string{"rblue-2"}, members)
	waiting, err := s.rdb.Get(s.ctx, accountWaitKey(4902)).Int()
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, waiting)

	acquired, err = green.AcquireAccountSlot(s.ctx, 4902, 1, "rgreen-1")
	require.NoError(s.T(), err)
	require.False(s.T(), acquired, "account limit must still count the live peer's in-flight request")
}
