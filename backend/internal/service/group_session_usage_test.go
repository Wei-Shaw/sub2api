package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type groupSessionUsageSessionCacheStub struct {
	SessionLimitCache
	active    map[int64]map[string]struct{}
	err       error
	requested []int64
	timeouts  map[int64]time.Duration
}

func (s *groupSessionUsageSessionCacheStub) GetActiveSessionCountBatch(_ context.Context, accountIDs []int64, _ map[int64]time.Duration) (map[int64]int, error) {
	out := make(map[int64]int, len(accountIDs))
	for _, id := range accountIDs {
		out[id] = len(s.active[id])
	}
	return out, nil
}

func (s *groupSessionUsageSessionCacheStub) GetActiveSessionsBatch(_ context.Context, accountIDs []int64, idleTimeouts map[int64]time.Duration) (map[int64]map[string]struct{}, error) {
	s.requested = append([]int64(nil), accountIDs...)
	s.timeouts = make(map[int64]time.Duration, len(idleTimeouts))
	for id, timeout := range idleTimeouts {
		s.timeouts[id] = timeout
	}
	if s.err != nil {
		return nil, s.err
	}
	out := make(map[int64]map[string]struct{}, len(accountIDs))
	for _, id := range accountIDs {
		if sessions, ok := s.active[id]; ok {
			out[id] = sessions
		}
	}
	return out, nil
}

type groupSessionUsageStickyCacheStub struct {
	GatewayCache
	bindings  map[string]int64
	err       error
	requested []string
	groupIDs  []int64
}

func (s *groupSessionUsageStickyCacheStub) GetSessionAccountIDBatch(_ context.Context, groupID int64, sessionHashes []string) (map[string]int64, error) {
	s.groupIDs = append(s.groupIDs, groupID)
	s.requested = append([]string(nil), sessionHashes...)
	if s.err != nil {
		return nil, s.err
	}
	out := make(map[string]int64)
	for _, sessionHash := range sessionHashes {
		if accountID, ok := s.bindings[sessionHash]; ok {
			out[sessionHash] = accountID
		}
	}
	return out, nil
}

func sessionLimitedAccount(id int64, maxSessions, idleMinutes int) Account {
	return Account{
		ID: id,
		Extra: map[string]any{
			"max_sessions":                 maxSessions,
			"session_idle_timeout_minutes": idleMinutes,
		},
	}
}

func TestSessionLimitedAccountsSkipsAccountsWithoutLimit(t *testing.T) {
	accounts := []Account{
		sessionLimitedAccount(1, 3, 7),
		{ID: 2},
		sessionLimitedAccount(3, 1, 0),
	}

	ids, timeouts := sessionLimitedAccounts(accounts)

	require.Equal(t, []int64{1, 3}, ids)
	require.Equal(t, 7*time.Minute, timeouts[1])
	// 账号未配置 idle timeout 时回退到账号级默认 5 分钟，不引入分组级 timeout。
	require.Equal(t, 5*time.Minute, timeouts[3])
	require.NotContains(t, timeouts, int64(2))
}

func TestComputeGroupSessionUsageCountsOnlyStickyOwnedActiveSessions(t *testing.T) {
	sessionCache := &groupSessionUsageSessionCacheStub{
		active: map[int64]map[string]struct{}{
			1: {"s1": {}, "s2": {}},
			2: {"s3": {}},
		},
	}
	sticky := &groupSessionUsageStickyCacheStub{
		bindings: map[string]int64{
			"s1": 1,
			"s2": 2, // sticky 指向 account 2，但 s2 在 account 2 上并不活跃
			"s3": 2,
		},
	}

	usage := computeGroupSessionUsage(
		context.Background(),
		sessionCache,
		sticky,
		10,
		[]int64{1, 2},
		map[int64]time.Duration{1: 7 * time.Minute, 2: 9 * time.Minute},
		"s1",
	)

	require.True(t, usage.Computed)
	require.Equal(t, 2, usage.Used)
	require.True(t, usage.ContainsSession)
	require.Equal(t, []int64{10}, sticky.groupIDs)
	require.ElementsMatch(t, []string{"s1", "s2", "s3"}, sticky.requested)
}

func TestComputeGroupSessionUsageIgnoresSessionsOwnedByOtherGroups(t *testing.T) {
	sessionCache := &groupSessionUsageSessionCacheStub{
		active: map[int64]map[string]struct{}{
			1: {"shared": {}, "mine": {}},
		},
	}
	// 共享账号上 "shared" 由别的分组建立，因此本分组没有 sticky 绑定。
	sticky := &groupSessionUsageStickyCacheStub{bindings: map[string]int64{"mine": 1}}

	usage := computeGroupSessionUsage(
		context.Background(),
		sessionCache,
		sticky,
		10,
		[]int64{1},
		map[int64]time.Duration{1: 5 * time.Minute},
		"",
	)

	require.True(t, usage.Computed)
	require.Equal(t, 1, usage.Used)
	require.False(t, usage.ContainsSession)
}

func TestComputeGroupSessionUsageIgnoresBindingsOutsideGroupAccounts(t *testing.T) {
	sessionCache := &groupSessionUsageSessionCacheStub{
		active: map[int64]map[string]struct{}{1: {"s1": {}}},
	}
	// sticky 指向已被移出分组的账号 99。
	sticky := &groupSessionUsageStickyCacheStub{bindings: map[string]int64{"s1": 99}}

	usage := computeGroupSessionUsage(
		context.Background(),
		sessionCache,
		sticky,
		10,
		[]int64{1},
		map[int64]time.Duration{1: 5 * time.Minute},
		"",
	)

	require.True(t, usage.Computed)
	require.Equal(t, 0, usage.Used)
}

func TestComputeGroupSessionUsageFailsOpenOnCacheErrors(t *testing.T) {
	t.Run("session cache error", func(t *testing.T) {
		usage := computeGroupSessionUsage(
			context.Background(),
			&groupSessionUsageSessionCacheStub{err: errors.New("redis down")},
			&groupSessionUsageStickyCacheStub{},
			10,
			[]int64{1},
			map[int64]time.Duration{1: 5 * time.Minute},
			"s1",
		)
		require.False(t, usage.Computed, "缓存不可用必须失败开放，交由调用方放行")
	})

	t.Run("sticky cache error", func(t *testing.T) {
		usage := computeGroupSessionUsage(
			context.Background(),
			&groupSessionUsageSessionCacheStub{active: map[int64]map[string]struct{}{1: {"s1": {}}}},
			&groupSessionUsageStickyCacheStub{err: errors.New("redis down")},
			10,
			[]int64{1},
			map[int64]time.Duration{1: 5 * time.Minute},
			"s1",
		)
		require.False(t, usage.Computed)
	})

	t.Run("nil caches", func(t *testing.T) {
		usage := computeGroupSessionUsage(context.Background(), nil, nil, 10, []int64{1}, nil, "s1")
		require.False(t, usage.Computed)
	})
}

func TestComputeGroupSessionUsageWithoutSessionLimitedAccounts(t *testing.T) {
	sessionCache := &groupSessionUsageSessionCacheStub{}
	sticky := &groupSessionUsageStickyCacheStub{}

	usage := computeGroupSessionUsage(context.Background(), sessionCache, sticky, 10, nil, nil, "s1")

	require.True(t, usage.Computed)
	require.Equal(t, 0, usage.Used)
	require.Empty(t, sessionCache.requested)
	require.Empty(t, sticky.groupIDs)
}

func TestCheckGroupSessionCapacity(t *testing.T) {
	newGroup := func(maxSessions int) *Group {
		return &Group{ID: 10, MaxSessions: maxSessions, Platform: PlatformAnthropic, Status: StatusActive, Hydrated: true}
	}
	accounts := []Account{sessionLimitedAccount(1, 5, 5)}

	t.Run("group limit disabled", func(t *testing.T) {
		svc := &GatewayService{
			sessionLimitCache: &groupSessionUsageSessionCacheStub{},
			cache:             &groupSessionUsageStickyCacheStub{},
		}
		_, ok := svc.checkGroupSessionCapacity(context.Background(), newGroup(0), accounts, "s1")
		require.True(t, ok)
	})

	t.Run("empty session id", func(t *testing.T) {
		svc := &GatewayService{
			sessionLimitCache: &groupSessionUsageSessionCacheStub{},
			cache:             &groupSessionUsageStickyCacheStub{},
		}
		_, ok := svc.checkGroupSessionCapacity(context.Background(), newGroup(1), accounts, "")
		require.True(t, ok)
	})

	t.Run("new session at cap is rejected", func(t *testing.T) {
		svc := &GatewayService{
			sessionLimitCache: &groupSessionUsageSessionCacheStub{
				active: map[int64]map[string]struct{}{1: {"a": {}, "b": {}}},
			},
			cache: &groupSessionUsageStickyCacheStub{bindings: map[string]int64{"a": 1, "b": 1}},
		}
		usage, ok := svc.checkGroupSessionCapacity(context.Background(), newGroup(2), accounts, "new")
		require.False(t, ok)
		require.Equal(t, 2, usage.Used)
	})

	t.Run("existing group session is allowed at cap", func(t *testing.T) {
		svc := &GatewayService{
			sessionLimitCache: &groupSessionUsageSessionCacheStub{
				active: map[int64]map[string]struct{}{1: {"a": {}, "b": {}}},
			},
			cache: &groupSessionUsageStickyCacheStub{bindings: map[string]int64{"a": 1, "b": 1}},
		}
		usage, ok := svc.checkGroupSessionCapacity(context.Background(), newGroup(2), accounts, "b")
		require.True(t, ok)
		require.True(t, usage.ContainsSession)
	})

	t.Run("cache error fails open", func(t *testing.T) {
		svc := &GatewayService{
			sessionLimitCache: &groupSessionUsageSessionCacheStub{err: errors.New("redis down")},
			cache:             &groupSessionUsageStickyCacheStub{},
		}
		_, ok := svc.checkGroupSessionCapacity(context.Background(), newGroup(1), accounts, "new")
		require.True(t, ok)
	})
}

func TestGroupSessionCapacityErrorsWrapNoAvailableAccounts(t *testing.T) {
	require.ErrorIs(t, ErrGroupSessionCapacityExceeded, ErrNoAvailableAccounts)
	require.ErrorIs(t, ErrAccountSessionCapacityExceeded, ErrNoAvailableAccounts)
}

func TestGroupGetMaxSessions(t *testing.T) {
	require.Equal(t, 0, (*Group)(nil).GetMaxSessions())
	require.Equal(t, 0, (&Group{MaxSessions: 0}).GetMaxSessions())
	require.Equal(t, 0, (&Group{MaxSessions: -1}).GetMaxSessions())
	require.Equal(t, 12, (&Group{MaxSessions: 12}).GetMaxSessions())
}
