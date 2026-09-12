//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMinuteBackfillRanges(t *testing.T) {
	safeEnd := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	target := safeEnd.AddDate(0, 0, -30)

	tests := []struct {
		name     string
		earliest *time.Time
		latest   *time.Time
		want     [][2]time.Time
	}{
		{
			name: "表为空时一次覆盖整个保留窗口",
			want: [][2]time.Time{{target, safeEnd}},
		},
		{
			name:     "已覆盖整个保留窗口且水位跟得上时无需回填",
			earliest: opsPtrTime(target.Add(-time.Hour)),
			latest:   opsPtrTime(safeEnd.Add(-2 * time.Minute)),
		},
		{
			name:     "只缺低端：补到已覆盖的最早桶为止",
			earliest: opsPtrTime(target.Add(72 * time.Hour)),
			latest:   opsPtrTime(safeEnd.Add(-2 * time.Minute)),
			want:     [][2]time.Time{{target, target.Add(72 * time.Hour)}},
		},
		{
			// 增量循环每轮只回看 opsAggMinuteOverlap，停机更久就够不到了。
			name:     "只缺高端：停机超过 overlap 的部分必须补",
			earliest: opsPtrTime(target.Add(-time.Hour)),
			latest:   opsPtrTime(safeEnd.Add(-3 * time.Hour)),
			want:     [][2]time.Time{{safeEnd.Add(-3 * time.Hour).Add(time.Minute), safeEnd}},
		},
		{
			name:     "停机时间在 overlap 之内则交给增量循环，不重复回填",
			earliest: opsPtrTime(target.Add(-time.Hour)),
			latest:   opsPtrTime(safeEnd.Add(-opsAggMinuteOverlap)),
		},
		{
			name:     "两头都缺",
			earliest: opsPtrTime(target.Add(48 * time.Hour)),
			latest:   opsPtrTime(safeEnd.Add(-5 * time.Hour)),
			want: [][2]time.Time{
				{target, target.Add(48 * time.Hour)},
				{safeEnd.Add(-5 * time.Hour).Add(time.Minute), safeEnd},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &OpsAggregationService{opsRepo: &opsRepoMock{
				GetEarliestMinuteBucketStartFn: bucketFn(tc.earliest),
				GetLatestMinuteBucketStartFn:   bucketFn(tc.latest),
			}}
			got, err := svc.minuteBackfillRanges(context.Background(), target, safeEnd)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestMinuteBackfillRangesEmptyWindow(t *testing.T) {
	at := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	svc := &OpsAggregationService{opsRepo: &opsRepoMock{}}

	got, err := svc.minuteBackfillRanges(context.Background(), at, at)
	require.NoError(t, err)
	require.Empty(t, got)
}

// 读覆盖范围失败时必须上抛：静默返回空会被当成"已经补齐"，缺口永远补不上。
func TestMinuteBackfillRangesPropagatesError(t *testing.T) {
	safeEnd := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	target := safeEnd.AddDate(0, 0, -30)
	boom := errors.New("boom")

	svc := &OpsAggregationService{opsRepo: &opsRepoMock{
		GetEarliestMinuteBucketStartFn: func(context.Context) (time.Time, bool, error) {
			return time.Time{}, false, boom
		},
	}}
	_, err := svc.minuteBackfillRanges(context.Background(), target, safeEnd)
	require.ErrorIs(t, err, boom)

	svc = &OpsAggregationService{opsRepo: &opsRepoMock{
		GetEarliestMinuteBucketStartFn: bucketFn(opsPtrTime(target.Add(-time.Hour))),
		GetLatestMinuteBucketStartFn: func(context.Context) (time.Time, bool, error) {
			return time.Time{}, false, boom
		},
	}}
	_, err = svc.minuteBackfillRanges(context.Background(), target, safeEnd)
	require.ErrorIs(t, err, boom)
}

func opsPtrTime(t time.Time) *time.Time { return &t }

func bucketFn(at *time.Time) func(context.Context) (time.Time, bool, error) {
	if at == nil {
		return nil
	}
	return func(context.Context) (time.Time, bool, error) { return *at, true, nil }
}
