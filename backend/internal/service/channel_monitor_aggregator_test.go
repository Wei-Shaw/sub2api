//go:build unit

package service

import (
	"context"
	"testing"
	"time"
)

func TestMergeModelDetailsIncludesTimeline(t *testing.T) {
	now := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	latency := 284
	ping := 42

	monitor := &ChannelMonitor{
		PrimaryModel: "gpt-5.5",
		ExtraModels:  []string{"gpt-5.4"},
	}
	latest := []*ChannelMonitorLatest{
		{Model: "gpt-5.5", Status: MonitorStatusOperational, LatencyMs: &latency},
	}
	availMap := map[int]map[string]*ChannelMonitorAvailability{
		monitorAvailability7Days: {
			"gpt-5.5": {AvailabilityPct: 99.5, AvgLatencyMs: &latency},
		},
		monitorAvailability15Days: {
			"gpt-5.5": {AvailabilityPct: 98.5},
		},
		monitorAvailability30Days: {
			"gpt-5.5": {AvailabilityPct: 97.5},
		},
	}
	timelineByModel := map[string][]*ChannelMonitorHistoryEntry{
		"gpt-5.5": {
			{
				Model:         "gpt-5.5",
				Status:        MonitorStatusOperational,
				LatencyMs:     &latency,
				PingLatencyMs: &ping,
				CheckedAt:     now,
			},
		},
	}

	models := mergeModelDetails(monitor, latest, availMap, timelineByModel)

	if len(models) != 2 {
		t.Fatalf("expected 2 model details, got %d", len(models))
	}
	if got := len(models[0].Timeline); got != 1 {
		t.Fatalf("expected primary model timeline, got %d points", got)
	}
	if models[0].Timeline[0].Status != MonitorStatusOperational {
		t.Fatalf("unexpected timeline status: %q", models[0].Timeline[0].Status)
	}
	if got := len(models[1].Timeline); got != 0 {
		t.Fatalf("expected empty timeline for model without history, got %d", got)
	}
}

func TestGetUserDetailLoadsTimelineForEachConfiguredModel(t *testing.T) {
	repo := &stubChannelMonitorRepo{
		monitor: &ChannelMonitor{
			ID:           7,
			Name:         "west",
			Provider:     MonitorProviderOpenAI,
			GroupName:    "default",
			PrimaryModel: "gpt-5.5",
			ExtraModels:  []string{"gpt-5.4"},
			Enabled:      true,
		},
	}
	svc := NewChannelMonitorService(repo, nil)

	_, err := svc.GetUserDetail(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetUserDetail returned error: %v", err)
	}

	if got := repo.historyRequests; got != 2 {
		t.Fatalf("expected history lookup for primary and extra models, got %d", got)
	}
	if repo.historyLimit != monitorTimelineMaxPoints {
		t.Fatalf("expected history limit %d, got %d", monitorTimelineMaxPoints, repo.historyLimit)
	}
}

type stubChannelMonitorRepo struct {
	monitor         *ChannelMonitor
	historyRequests int
	historyLimit    int
}

func (r *stubChannelMonitorRepo) Create(context.Context, *ChannelMonitor) error { return nil }
func (r *stubChannelMonitorRepo) GetByID(_ context.Context, _ int64) (*ChannelMonitor, error) {
	return r.monitor, nil
}
func (r *stubChannelMonitorRepo) Update(context.Context, *ChannelMonitor) error { return nil }
func (r *stubChannelMonitorRepo) Delete(context.Context, int64) error           { return nil }
func (r *stubChannelMonitorRepo) List(context.Context, ChannelMonitorListParams) ([]*ChannelMonitor, int64, error) {
	return nil, 0, nil
}
func (r *stubChannelMonitorRepo) ListEnabled(context.Context) ([]*ChannelMonitor, error) {
	return nil, nil
}
func (r *stubChannelMonitorRepo) MarkChecked(context.Context, int64, time.Time) error {
	return nil
}
func (r *stubChannelMonitorRepo) InsertHistoryBatch(context.Context, []*ChannelMonitorHistoryRow) error {
	return nil
}
func (r *stubChannelMonitorRepo) DeleteHistoryBefore(context.Context, time.Time) (int64, error) {
	return 0, nil
}
func (r *stubChannelMonitorRepo) ListHistory(_ context.Context, _ int64, _ string, limit int) ([]*ChannelMonitorHistoryEntry, error) {
	r.historyRequests++
	r.historyLimit = limit
	return []*ChannelMonitorHistoryEntry{}, nil
}
func (r *stubChannelMonitorRepo) ListLatestPerModel(context.Context, int64) ([]*ChannelMonitorLatest, error) {
	return []*ChannelMonitorLatest{}, nil
}
func (r *stubChannelMonitorRepo) ComputeAvailability(context.Context, int64, int) ([]*ChannelMonitorAvailability, error) {
	return []*ChannelMonitorAvailability{}, nil
}
func (r *stubChannelMonitorRepo) ListLatestForMonitorIDs(context.Context, []int64) (map[int64][]*ChannelMonitorLatest, error) {
	return map[int64][]*ChannelMonitorLatest{}, nil
}
func (r *stubChannelMonitorRepo) ComputeAvailabilityForMonitors(context.Context, []int64, int) (map[int64][]*ChannelMonitorAvailability, error) {
	return map[int64][]*ChannelMonitorAvailability{}, nil
}
func (r *stubChannelMonitorRepo) ListRecentHistoryForMonitors(context.Context, []int64, map[int64]string, int) (map[int64][]*ChannelMonitorHistoryEntry, error) {
	return map[int64][]*ChannelMonitorHistoryEntry{}, nil
}
func (r *stubChannelMonitorRepo) UpsertDailyRollupsFor(context.Context, time.Time) (int64, error) {
	return 0, nil
}
func (r *stubChannelMonitorRepo) DeleteRollupsBefore(context.Context, time.Time) (int64, error) {
	return 0, nil
}
func (r *stubChannelMonitorRepo) LoadAggregationWatermark(context.Context) (*time.Time, error) {
	return nil, nil
}
func (r *stubChannelMonitorRepo) UpdateAggregationWatermark(context.Context, time.Time) error {
	return nil
}
