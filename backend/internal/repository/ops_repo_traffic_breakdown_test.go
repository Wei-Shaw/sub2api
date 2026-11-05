package repository

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func directionalTrafficFixture(scale int64) opsTrafficAggregate {
	direction := func(original, actual int64) *opsTrafficDirectionAggregate {
		return &opsTrafficDirectionAggregate{
			OriginalBytes: trafficInt64(original * scale), ActualBytes: trafficInt64(actual * scale),
			OriginalSampleCount: 1, ActualSampleCount: 1,
		}
	}
	return opsTrafficAggregate{
		RequestCount: 1,
		Upstream: opsTrafficLinkAggregate{
			OriginalBytes: trafficInt64(30 * scale), ActualBytes: trafficInt64(15 * scale),
			OriginalSampleCount: 2, ActualSampleCount: 2, PairedSampleCount: 2,
			PairedOriginalBytes: trafficInt64(30 * scale), PairedActualBytes: trafficInt64(15 * scale),
			Request: direction(10, 5), Response: direction(20, 10),
		},
		Downstream: opsTrafficLinkAggregate{
			OriginalBytes: trafficInt64(70 * scale), ActualBytes: trafficInt64(35 * scale),
			OriginalSampleCount: 2, ActualSampleCount: 2, PairedSampleCount: 2,
			PairedOriginalBytes: trafficInt64(70 * scale), PairedActualBytes: trafficInt64(35 * scale),
			Request: direction(30, 15), Response: direction(40, 20),
		},
	}
}

func TestOpsTrafficDirectionalMergeKeepsFourDistinctStreams(t *testing.T) {
	head, stable, tail := directionalTrafficFixture(1), directionalTrafficFixture(2), directionalTrafficFixture(3)
	merged := aggregateHourlyRows([]opsHourlyMetricsRow{{traffic: &stable}}).traffic
	merged.add(head)
	merged.add(tail)
	got := merged.summary()
	require.EqualValues(t, 3, got.RequestCount)
	for _, test := range []struct {
		name             string
		got              *opsTrafficDirectionAggregate
		original, actual int64
	}{
		{"upstream request", merged.Upstream.Request, 60, 30},
		{"upstream response", merged.Upstream.Response, 120, 60},
		{"downstream request", merged.Downstream.Request, 180, 90},
		{"downstream response", merged.Downstream.Response, 240, 120},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.NotNil(t, test.got)
			require.Equal(t, test.original, *test.got.OriginalBytes)
			require.Equal(t, test.actual, *test.got.ActualBytes)
			require.EqualValues(t, 3, test.got.OriginalSampleCount)
			require.EqualValues(t, 3, test.got.ActualSampleCount)
		})
	}
	require.EqualValues(t, 90, *got.Upstream.ActualBytes)
	require.EqualValues(t, 210, *got.Downstream.ActualBytes)
	require.Equal(t, 50.0, *got.Upstream.CompressionPercent)
	require.Equal(t, 50.0, *got.Downstream.CompressionPercent)
	require.EqualValues(t, 30, *got.Upstream.Request.ActualBytes)
	require.EqualValues(t, 60, *got.Upstream.Response.ActualBytes)

	// A shallow aggregate copy must not mutate nested source data on merge.
	copy := stable
	copy.add(head)
	require.EqualValues(t, 20, *stable.Upstream.Request.OriginalBytes)
	require.EqualValues(t, 10, *stable.Upstream.Request.ActualBytes)
	require.EqualValues(t, 1, stable.Upstream.Request.OriginalSampleCount)
	require.EqualValues(t, 10, *head.Upstream.Request.OriginalBytes)
}

func TestOpsTrafficDirectionalUnknownAndMeasuredZero(t *testing.T) {
	value := directionalTrafficFixture(1)
	value.Upstream = opsTrafficLinkAggregate{
		ActualBytes: trafficInt64(0), ActualSampleCount: 1,
		Request:  &opsTrafficDirectionAggregate{ActualBytes: trafficInt64(0), ActualSampleCount: 1},
		Response: &opsTrafficDirectionAggregate{},
	}
	got := value.summary()
	require.Nil(t, got.Upstream.OriginalBytes)
	require.EqualValues(t, 0, *got.Upstream.ActualBytes)
	require.Nil(t, got.Upstream.CompressionPercent)
	require.NotNil(t, got.Upstream.Request)
	require.Nil(t, got.Upstream.Request.OriginalBytes)
	require.EqualValues(t, 0, *got.Upstream.Request.ActualBytes)
	require.EqualValues(t, 0, got.Upstream.Request.OriginalSampleCount)
	require.EqualValues(t, 1, got.Upstream.Request.ActualSampleCount)
	require.NotNil(t, got.Upstream.Response)
	require.Nil(t, got.Upstream.Response.ActualBytes)

	payload, err := json.Marshal(got.Upstream.Request)
	require.NoError(t, err)
	require.JSONEq(t, `{"original_bytes":null,"actual_bytes":0,"original_sample_count":0,"actual_sample_count":1,"compression_percent":null}`, string(payload))
}

func TestOpsTrafficLegacyDirectionalDataIsNotInventedDuringMerge(t *testing.T) {
	legacy := opsTrafficAggregate{RequestCount: 2, Upstream: opsTrafficLinkAggregate{
		OriginalBytes: trafficInt64(200), ActualBytes: trafficInt64(100),
		OriginalSampleCount: 4, ActualSampleCount: 4, PairedSampleCount: 4,
		PairedOriginalBytes: trafficInt64(200), PairedActualBytes: trafficInt64(100),
	}}
	for _, legacyFirst := range []bool{false, true} {
		known := directionalTrafficFixture(1)
		merged := known
		if legacyFirst {
			merged = legacy
			merged.add(known)
		} else {
			merged.add(legacy)
		}
		got := merged.summary()
		require.EqualValues(t, 3, got.RequestCount)
		require.EqualValues(t, 230, *got.Upstream.OriginalBytes)
		require.EqualValues(t, 115, *got.Upstream.ActualBytes)
		require.Equal(t, 50.0, *got.Upstream.CompressionPercent)
		require.Nil(t, got.Upstream.Request)
		require.Nil(t, got.Upstream.Response)
		require.Nil(t, got.Downstream.Request)
		require.Nil(t, got.Downstream.Response)

		// Missing directional information stays sticky through another reducer.
		outer := directionalTrafficFixture(1)
		outer.add(merged)
		require.Nil(t, outer.summary().Upstream.Request)
	}
}

func TestOpsTrafficEmptyLegacyBucketDoesNotPoisonKnownBreakdown(t *testing.T) {
	var empty opsTrafficAggregate
	emptyResult := empty.summary()
	require.NotNil(t, emptyResult.Upstream.Request)
	require.Nil(t, emptyResult.Upstream.Request.OriginalBytes)
	require.Nil(t, emptyResult.Upstream.Request.ActualBytes)
	require.Zero(t, emptyResult.Upstream.Request.ActualSampleCount)

	known := directionalTrafficFixture(1)
	want := known.summary()
	known.add(empty)
	require.Equal(t, want, known.summary())
	empty.add(known)
	require.Equal(t, want, empty.summary())
}
