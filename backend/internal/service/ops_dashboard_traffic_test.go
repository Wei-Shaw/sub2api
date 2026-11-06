package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type opsTrafficOverviewRepo struct {
	opsRepoMock
	overview *OpsDashboardOverview
}

func (r *opsTrafficOverviewRepo) GetDashboardOverview(context.Context, *OpsDashboardFilter) (*OpsDashboardOverview, error) {
	return r.overview, nil
}

func TestOpsTrafficOverviewAttachesInstanceIngestionHealth(t *testing.T) {
	ledger := &opsTrafficLedger{}
	ledger.pending.Store(2)
	ledger.failed.Store(3)
	ledger.flushed.Store(4)
	svc := &OpsService{
		opsRepo: &opsTrafficOverviewRepo{overview: &OpsDashboardOverview{Traffic: &OpsTrafficSummary{
			Source: "http_requests", RequestCount: 10,
		}}},
		trafficLedger: ledger,
	}
	start := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	groupID := int64(42)
	got, err := svc.GetDashboardOverview(context.Background(), &OpsDashboardFilter{
		StartTime: start, EndTime: start.Add(time.Hour), Platform: "openai", GroupID: &groupID, QueryMode: OpsQueryModeRaw,
	})
	require.NoError(t, err)
	require.Equal(t, &OpsTrafficLedgerHealth{Scope: "instance_lifetime", Pending: 2, Failed: 3, Flushed: 4}, got.Traffic.Ingestion)
	require.EqualValues(t, 10, got.Traffic.RequestCount, "ingestion is not part of the time-filtered population")
}

func TestOpsTrafficOverviewJSONKeepsUnknownDistinctFromMeasuredZero(t *testing.T) {
	zero := int64(0)
	negative := -25.0
	value := OpsTrafficSummary{
		Source: "http_requests", RequestCount: 1,
		Downstream: OpsTrafficLink{
			OriginalBytes: &zero, ActualBytes: &zero, OriginalSampleCount: 1, ActualSampleCount: 1, PairedSampleCount: 1,
			Request:  &OpsTrafficDirection{OriginalBytes: &zero, ActualBytes: &zero, OriginalSampleCount: 1, ActualSampleCount: 1, CompressionPercent: &negative},
			Response: &OpsTrafficDirection{},
		},
	}
	data, err := json.Marshal(value)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))
	up, ok := decoded["upstream"].(map[string]any)
	require.True(t, ok)
	down, ok := decoded["downstream"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, up, "original_bytes")
	require.Nil(t, up["original_bytes"])
	require.Equal(t, float64(0), down["original_bytes"])
	require.Nil(t, down["compression_percent"])
	require.Contains(t, up, "request")
	require.Nil(t, up["request"], "missing legacy breakdown is explicit null, not an invented split")
	require.Contains(t, up, "response")
	require.Nil(t, up["response"])
	request, ok := down["request"].(map[string]any)
	require.True(t, ok)
	response, ok := down["response"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(0), request["original_bytes"])
	require.Equal(t, float64(0), request["actual_bytes"])
	require.Equal(t, float64(1), request["original_sample_count"])
	require.Equal(t, float64(1), request["actual_sample_count"])
	require.Equal(t, float64(-25), request["compression_percent"], "a directional ratio is exposed per direction")
	require.Nil(t, response["original_bytes"])
	require.Nil(t, response["actual_bytes"])
	require.Equal(t, float64(0), response["actual_sample_count"])
	require.Contains(t, response, "compression_percent")
	require.Nil(t, response["compression_percent"], "an unknown directional ratio stays null, never zero")
	require.Contains(t, decoded, "ingestion")
	require.Nil(t, decoded["ingestion"])
	require.NotContains(t, decoded, "usage_count")
}
