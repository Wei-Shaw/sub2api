package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// profitExclusionStateStub 是一个内存版键值表，可注入读失败。
type profitExclusionStateStub struct {
	values   map[string]string
	getErr   error
	setErr   error
	getCalls int
}

func newProfitExclusionStateStub() *profitExclusionStateStub {
	return &profitExclusionStateStub{values: map[string]string{}}
}

func (s *profitExclusionStateStub) Get(_ context.Context, key string) (string, error) {
	s.getCalls++
	if s.getErr != nil {
		return "", s.getErr
	}
	return s.values[key], nil
}

func (s *profitExclusionStateStub) Set(_ context.Context, key, value string) error {
	if s.setErr != nil {
		return s.setErr
	}
	s.values[key] = value
	return nil
}

func (s *profitExclusionStateStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		out[k] = s.values[k]
	}
	return out, nil
}

func TestNormalizeProfitExcludedUserIDs(t *testing.T) {
	cases := []struct {
		name string
		in   []int64
		want []int64
	}{
		{name: "nil 得到空切片而不是 nil", in: nil, want: []int64{}},
		{name: "去重并升序", in: []int64{9, 3, 9, 1}, want: []int64{1, 3, 9}},
		{name: "丢弃 0 与负数", in: []int64{0, -5, 7}, want: []int64{7}},
		{name: "全是非法值时得到空切片", in: []int64{0, -1}, want: []int64{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeProfitExcludedUserIDs(tc.in)
			require.Equal(t, tc.want, got)
			require.NotNil(t, got, "返回值必须非 nil，前端会直接遍历")
		})
	}
}

// TestUsageProfitExclusionService_EmptyWhenKeyMissing 验证「没配过」是正常状态。
func TestUsageProfitExclusionService_EmptyWhenKeyMissing(t *testing.T) {
	svc := NewUsageProfitExclusionService(newProfitExclusionStateStub())

	ids, err := svc.ExcludedUserIDs(context.Background())
	require.NoError(t, err)
	require.Empty(t, ids)
}

func TestUsageProfitExclusionService_ReadsAndNormalizesStoredValue(t *testing.T) {
	state := newProfitExclusionStateStub()
	state.values[UsageProfitExcludeStateKey] = "[9,3,3]"
	svc := NewUsageProfitExclusionService(state)

	ids, err := svc.ExcludedUserIDs(context.Background())
	require.NoError(t, err)
	require.Equal(t, []int64{3, 9}, ids)
}

// TestUsageProfitExclusionService_UpdateWritesAndInvalidatesCache 验证写后立即生效。
//
// 「改完名单要等缓存过期才起作用」是很容易被当成 bug 的行为，所以 Update 里
// 直接更新缓存而不是只删缓存。
func TestUsageProfitExclusionService_UpdateWritesAndInvalidatesCache(t *testing.T) {
	state := newProfitExclusionStateStub()
	svc := NewUsageProfitExclusionService(state)
	ctx := context.Background()

	// 先读一次把空名单灌进缓存。
	_, err := svc.ExcludedUserIDs(ctx)
	require.NoError(t, err)

	view, err := svc.Update(ctx, []int64{5, 2, 5, 0})
	require.NoError(t, err)
	require.Equal(t, []int64{2, 5}, view.UserIDs)

	// 立刻再读，必须已经是新值（不能等 TTL）。
	ids, err := svc.ExcludedUserIDs(ctx)
	require.NoError(t, err)
	require.Equal(t, []int64{2, 5}, ids)

	// 落库内容也必须是规范化后的 JSON 数组。
	require.JSONEq(t, "[2,5]", state.values[UsageProfitExcludeStateKey])
}

func TestUsageProfitExclusionService_CachesWithinTTL(t *testing.T) {
	state := newProfitExclusionStateStub()
	svc := NewUsageProfitExclusionService(state)
	svc.ttl = time.Hour
	ctx := context.Background()

	_, err := svc.ExcludedUserIDs(ctx)
	require.NoError(t, err)
	_, err = svc.ExcludedUserIDs(ctx)
	require.NoError(t, err)

	require.Equal(t, 1, state.getCalls, "TTL 内的重复读取不应打库")
}

// TestUsageProfitExclusionService_FailsLoudBeforeFirstLoad 是本文件最重要的一条。
//
// 名单读不到时**绝不能**降级成「按空名单继续算」：空名单会把内部人员那些
// 手工调整出来的虚假收入算进毛利，图上只是一个偏高的利润，完全看不出异常。
// 这条测试锁死「宁可报错，也不发一个悄悄偏高的经营数字」。
func TestUsageProfitExclusionService_FailsLoudBeforeFirstLoad(t *testing.T) {
	state := newProfitExclusionStateStub()
	state.getErr = errors.New("db down")
	svc := NewUsageProfitExclusionService(state)

	ids, err := svc.ExcludedUserIDs(context.Background())
	require.Error(t, err)
	require.Nil(t, ids)
}

// TestUsageProfitExclusionService_KeepsStaleValueAfterLoad 验证已生效过的口径不因一次抖动退化。
func TestUsageProfitExclusionService_KeepsStaleValueAfterLoad(t *testing.T) {
	state := newProfitExclusionStateStub()
	state.values[UsageProfitExcludeStateKey] = "[7]"
	svc := NewUsageProfitExclusionService(state)
	svc.ttl = 0 // 让每次读取都尝试刷新
	ctx := context.Background()

	ids, err := svc.ExcludedUserIDs(ctx)
	require.NoError(t, err)
	require.Equal(t, []int64{7}, ids)

	state.getErr = errors.New("db hiccup")
	ids, err = svc.ExcludedUserIDs(ctx)
	require.NoError(t, err, "曾经读到过时，读失败应退回上一次的值而不是让整页报错")
	require.Equal(t, []int64{7}, ids)
}

// TestUsageProfitExclusionService_InvalidStoredValueFails 验证手工写坏的值会明确报错。
func TestUsageProfitExclusionService_InvalidStoredValueFails(t *testing.T) {
	state := newProfitExclusionStateStub()
	state.values[UsageProfitExcludeStateKey] = "not-json"
	svc := NewUsageProfitExclusionService(state)

	_, err := svc.ExcludedUserIDs(context.Background())
	require.Error(t, err)
}

// TestUsageProfitExclusionService_EffectiveDoesNotLeakCacheSlice 验证对外视图是副本。
//
// 直接把缓存切片发出去，调用方一次 append 就能改掉后续所有请求看到的口径。
func TestUsageProfitExclusionService_EffectiveDoesNotLeakCacheSlice(t *testing.T) {
	state := newProfitExclusionStateStub()
	state.values[UsageProfitExcludeStateKey] = "[1,2]"
	svc := NewUsageProfitExclusionService(state)
	ctx := context.Background()

	view, err := svc.Effective(ctx)
	require.NoError(t, err)
	view.UserIDs[0] = 999

	again, err := svc.Effective(ctx)
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2}, again.UserIDs)
}

// TestUsageProfitExclusionService_StoreUnavailable 验证存储缺席是明确错误。
func TestUsageProfitExclusionService_StoreUnavailable(t *testing.T) {
	svc := NewUsageProfitExclusionService(nil)

	_, err := svc.ExcludedUserIDs(context.Background())
	require.ErrorIs(t, err, ErrUsageProfitExclusionStoreUnavailable)

	_, err = svc.Update(context.Background(), []int64{1})
	require.ErrorIs(t, err, ErrUsageProfitExclusionStoreUnavailable)
}
