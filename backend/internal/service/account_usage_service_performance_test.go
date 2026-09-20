package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type performanceLogRepoStub struct {
	UsageLogRepository
	read func(context.Context, []int64, time.Time, time.Time) (map[int64]*AccountPerformanceStats, error)
}

func (r *performanceLogRepoStub) GetAccountPerformanceStatsBatch(ctx context.Context, ids []int64, start, end time.Time) (map[int64]*AccountPerformanceStats, error) {
	return r.read(ctx, ids, start, end)
}

func TestAccountPerformanceNormalizeIDs(t *testing.T) {
	for _, tc := range []struct {
		name string
		ids  []int64
		want []int64
		err  bool
	}{
		{name: "empty", want: []int64{}},
		{name: "sorted unique", ids: []int64{5, 2, 5, 1}, want: []int64{1, 2, 5}},
		{name: "zero", ids: []int64{1, 0}, err: true},
		{name: "negative", ids: []int64{-1}, err: true},
		{name: "oversized", ids: make([]int64, MaxAccountPerformanceBatchSize+1), err: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := append([]int64(nil), tc.ids...)
			got, err := NormalizeAccountPerformanceIDs(tc.ids)
			if tc.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, got)
			}
			require.Equal(t, original, tc.ids)
		})
	}
	ids := make([]int64, MaxAccountPerformanceBatchSize)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	got, err := NormalizeAccountPerformanceIDs(ids)
	require.NoError(t, err)
	require.Len(t, got, MaxAccountPerformanceBatchSize)
}

func TestAccountPerformanceServicePassiveBatch(t *testing.T) {
	ctx := context.WithValue(context.Background(), struct{}{}, "request context")
	var start, end time.Time
	calls := 0
	value := 12.3456789
	recorded := &AccountPerformanceStats{RequestCount: 2, TTFTMs: &value, TTFTSamples: 1}
	repo := &performanceLogRepoStub{read: func(gotCtx context.Context, ids []int64, a, b time.Time) (map[int64]*AccountPerformanceStats, error) {
		calls++
		require.Equal(t, ctx, gotCtx)
		require.Equal(t, []int64{2, 9}, ids)
		start, end = a, b
		return map[int64]*AccountPerformanceStats{2: recorded, 100: recorded}, nil
	}}
	// All other dependencies are nil: upstream, quotas, accounts and recording
	// paths would panic if this passive read accidentally touched them.
	svc := &AccountUsageService{usageLogRepo: repo}
	before := time.Now().UTC().Truncate(time.Microsecond)
	result, err := svc.GetPerformanceStatsBatch(ctx, []int64{9, 2, 9})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Len(t, result.Stats, 2)
	require.Equal(t, recorded, result.Stats[2])
	require.Equal(t, &AccountPerformanceStats{}, result.Stats[9])
	require.Equal(t, start, result.WindowStart)
	require.Equal(t, end, result.WindowEnd)
	require.Equal(t, time.Hour, end.Sub(start))
	require.False(t, end.Before(before))
	require.False(t, end.After(time.Now()))
}

func TestAccountPerformanceServiceErrorsAndEmpty(t *testing.T) {
	expected := errors.New("database unavailable")
	calls := 0
	svc := &AccountUsageService{usageLogRepo: &performanceLogRepoStub{
		read: func(context.Context, []int64, time.Time, time.Time) (map[int64]*AccountPerformanceStats, error) {
			calls++
			return nil, expected
		},
	}}
	result, err := svc.GetPerformanceStatsBatch(context.Background(), []int64{1})
	require.Nil(t, result)
	require.ErrorIs(t, err, expected)
	require.Equal(t, 1, calls)
	_, err = svc.GetPerformanceStatsBatch(context.Background(), []int64{0})
	require.Error(t, err)
	require.Equal(t, 1, calls)

	svc = &AccountUsageService{usageLogRepo: &usageBatchLogRepoStub{}}
	_, err = svc.GetPerformanceStatsBatch(context.Background(), []int64{1})
	require.ErrorContains(t, err, "batch reader is unavailable")
	result, err = svc.GetPerformanceStatsBatch(context.Background(), nil)
	require.NoError(t, err)
	require.NotNil(t, result.Stats)
	require.Empty(t, result.Stats)
	require.Equal(t, time.Hour, result.WindowEnd.Sub(result.WindowStart))
}
