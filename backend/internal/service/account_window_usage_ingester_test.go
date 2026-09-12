//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type historyTestRepo struct {
	AccountWindowUsageRepository
	rows         []*AccountWindowUsageRecord
	stats        AccountWindowReferenceStats
	aggregateErr error
	marker       *time.Time
}

func (r *historyTestRepo) GetOpenWindow(_ context.Context, id int64, kind string) (*AccountWindowUsageRecord, error) {
	for i := len(r.rows) - 1; i >= 0; i-- {
		row := r.rows[i]
		if row.AccountID == id && row.WindowType == kind && row.FinalizedAt == nil {
			return cloneHistoryRow(row), nil
		}
	}
	return nil, nil
}
func (r *historyTestRepo) GetLatestWindow(_ context.Context, id int64, kind string) (*AccountWindowUsageRecord, error) {
	for i := len(r.rows) - 1; i >= 0; i-- {
		row := r.rows[i]
		if row.AccountID == id && row.WindowType == kind {
			return cloneHistoryRow(row), nil
		}
	}
	return nil, nil
}
func (r *historyTestRepo) GetClosedWindow(_ context.Context, id int64, kind string, reset, at time.Time) (*AccountWindowUsageRecord, error) {
	for i := len(r.rows) - 1; i >= 0; i-- {
		row := r.rows[i]
		if row.AccountID == id && row.WindowType == kind && row.FinalizedAt != nil && absDuration(row.ResetAt.Sub(reset)) <= ingesterResetEpsilon && !at.Before(row.WindowStart.Add(-ingesterResetEpsilon)) && !at.After(row.WindowEnd.Add(ingesterResetEpsilon)) {
			return cloneHistoryRow(row), nil
		}
	}
	return nil, nil
}

func (r *historyTestRepo) SaveWindow(_ context.Context, row *AccountWindowUsageRecord) error {
	if row.ID == 0 {
		row.ID = int64(len(r.rows) + 1)
		r.rows = append(r.rows, cloneHistoryRow(row))
		return nil
	}
	r.rows[row.ID-1] = cloneHistoryRow(row)
	return nil
}
func (r *historyTestRepo) LatestResetMarker(_ context.Context, _ int64, since, until time.Time) (*time.Time, error) {
	if r.marker != nil && r.marker.After(since) && !r.marker.After(until) {
		return r.marker, nil
	}
	return nil, nil
}
func (r *historyTestRepo) AggregateReferenceUsage(context.Context, int64, time.Time, time.Time) (*AccountWindowReferenceStats, error) {
	if r.aggregateErr != nil {
		return nil, r.aggregateErr
	}
	v := r.stats
	return &v, nil
}
func cloneHistoryRow(row *AccountWindowUsageRecord) *AccountWindowUsageRecord {
	b, _ := json.Marshal(row)
	var c AccountWindowUsageRecord
	_ = json.Unmarshal(b, &c)
	return &c
}
func historyMoney(v float64) *float64 { return &v }
func historyObservation(id int64, at, reset time.Time, used float64) *AccountQuotaObservation {
	payload := fmt.Sprintf(`{"codex_5h_used_percent":%g,"codex_5h_reset_at":%q,"codex_5h_window_minutes":300}`, used, reset.Format(time.RFC3339Nano))
	return &AccountQuotaObservation{ID: id, AccountID: 1, ObservedAt: at, Payload: json.RawMessage(payload)}
}
func historyFixture() (*historyTestRepo, *AccountWindowUsageIngester, time.Time) {
	r := &historyTestRepo{stats: AccountWindowReferenceStats{Requests: 10, TokensTotal: 1000, ReferenceCost: historyMoney(20), PricedRequests: 10}}
	g := NewAccountWindowUsageIngester(r)
	return r, g, time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
}

func TestWindowHistorySameTimestampSamplesAndOutOfOrder(t *testing.T) {
	r, g, at := historyFixture()
	ctx := context.Background()
	reset := at.Add(5 * time.Hour)
	require.NoError(t, g.ApplyObservation(ctx, historyObservation(1, at, reset, 10)))
	require.NoError(t, g.ApplyObservation(ctx, historyObservation(2, at, reset, 20)))
	require.Len(t, r.rows, 1)
	require.Equal(t, 2, r.rows[0].SampleCount)
	require.Equal(t, 20.0, r.rows[0].LastUsedPercent)
	require.NoError(t, g.ApplyObservation(ctx, historyObservation(1, at, reset, 90)))
	require.Equal(t, 20.0, r.rows[0].LastUsedPercent)
	require.Equal(t, 2, r.rows[0].SampleCount)
	require.NoError(t, g.ApplyObservation(ctx, historyObservation(3, at.Add(-time.Second), reset, 99)))
	require.Equal(t, 20.0, r.rows[0].LastUsedPercent)
}
func TestWindowHistoryNaturalBoundaryRetainsOldEstimate(t *testing.T) {
	r, g, at := historyFixture()
	ctx := context.Background()
	reset := at.Add(5 * time.Hour)
	require.NoError(t, g.ApplyObservation(ctx, historyObservation(1, at, reset, 50)))
	require.InDelta(t, 40, *r.rows[0].EstimatedReferenceLimit, 1e-8)
	r.stats.ReferenceCost = historyMoney(39)
	require.NoError(t, g.ApplyObservation(ctx, historyObservation(2, reset.Add(-time.Second), reset, 100)))
	require.NoError(t, g.ApplyObservation(ctx, historyObservation(3, reset.Add(time.Second), reset.Add(5*time.Hour), 0)))
	require.Len(t, r.rows, 2)
	require.NotNil(t, r.rows[0].FinalizedAt)
	require.Equal(t, "expired", *r.rows[0].EndReason)
	require.InDelta(t, 39, *r.rows[0].EstimatedReferenceLimit, 1e-8)
	require.Nil(t, r.rows[1].EstimatedReferenceLimit)
	require.Contains(t, r.rows[1].QualityFlags, "low_utilization")
}
func TestWindowHistoryResetMarkerDoesNotAssumeWindowCount(t *testing.T) {
	r, g, at := historyFixture()
	ctx := context.Background()
	reset := at.Add(5 * time.Hour)
	require.NoError(t, g.ApplyObservation(ctx, historyObservation(1, at, reset, 70)))
	marker := at.Add(time.Minute)
	r.marker = &marker
	// An unaffected window retains its identity and percentage, even after a card.
	require.NoError(t, g.ApplyObservation(ctx, historyObservation(2, marker.Add(time.Second), reset, 70)))
	require.Len(t, r.rows, 1)
	require.Nil(t, r.rows[0].FinalizedAt)
	// A separate later reset marker and changed window confirms the affected scope.
	marker = at.Add(2 * time.Minute)
	r.marker = &marker
	require.NoError(t, g.ApplyObservation(ctx, historyObservation(3, marker.Add(time.Second), marker.Add(5*time.Hour), 0)))
	require.Len(t, r.rows, 2)
	require.Equal(t, "early_reset", *r.rows[0].EndReason)
	require.Equal(t, marker, r.rows[0].WindowEnd)
	require.Equal(t, marker, r.rows[1].WindowStart)
	require.NotNil(t, r.rows[0].EstimatedReferenceLimit)
}
func TestWindowHistoryUnknownBoundaryChangeIsNotSilentlyMerged(t *testing.T) {
	r, g, at := historyFixture()
	ctx := context.Background()
	reset := at.Add(time.Hour)
	require.NoError(t, g.ApplyObservation(ctx, historyObservation(1, at, reset, 90)))
	next := at.Add(time.Minute)
	require.NoError(t, g.ApplyObservation(ctx, historyObservation(2, next, reset.Add(time.Hour), 2)))
	require.Len(t, r.rows, 2)
	require.Equal(t, "window_changed", *r.rows[0].EndReason)
	require.Contains(t, r.rows[0].QualityFlags, "ambiguous_reset")
	require.Equal(t, 90.0, r.rows[0].LastUsedPercent)
}
func TestWindowHistoryMissingPricePreservesLastUsableEstimate(t *testing.T) {
	r, g, at := historyFixture()
	ctx := context.Background()
	reset := at.Add(5 * time.Hour)
	require.NoError(t, g.ApplyObservation(ctx, historyObservation(1, at, reset, 25)))
	require.InDelta(t, 80, *r.rows[0].EstimatedReferenceLimit, 1e-8)
	r.stats.ReferenceCost = historyMoney(40)
	r.stats.MissingPricingRequests = 1
	require.NoError(t, g.ApplyObservation(ctx, historyObservation(2, at.Add(time.Minute), reset, 75)))
	require.Equal(t, 40.0, *r.rows[0].APIReferenceCost)
	require.Equal(t, 80.0, *r.rows[0].EstimatedReferenceLimit)
	require.Equal(t, 25.0, *r.rows[0].EstimateUsedPercent)
	require.Contains(t, r.rows[0].QualityFlags, "missing_pricing")
}
func TestWindowHistoryMalformedAndUnsupportedPayloads(t *testing.T) {
	r, g, at := historyFixture()
	ctx := context.Background()
	reset := at.Add(5 * time.Hour)
	for _, payload := range []string{`{`, `{"codex_5h_used_percent":25}`, fmt.Sprintf(`{"codex_5h_used_percent":25,"codex_5h_reset_at":%q,"codex_5h_window_minutes":43200}`, reset.Format(time.RFC3339)), `{"codex_5h_used_percent":"NaN","codex_5h_reset_at":"2026-08-21T00:00:00Z"}`, `{"codex_history_reset_at":"2026-08-20T12:00:00Z","codex_history_reset_windows":1}`} {
		require.NoError(t, g.ApplyObservation(ctx, &AccountQuotaObservation{ID: 1, AccountID: 1, ObservedAt: at, Payload: json.RawMessage(payload)}))
	}
	require.Empty(t, r.rows)
}
func TestWindowHistoryAggregationFailureDoesNotSaveEstimate(t *testing.T) {
	r, g, at := historyFixture()
	r.aggregateErr = errors.New("temporary DB failure")
	err := g.ApplyObservation(context.Background(), historyObservation(1, at, at.Add(5*time.Hour), 50))
	require.Error(t, err)
	require.Empty(t, r.rows)
}
func TestWindowHistoryPartialStartAndUnknownPrice(t *testing.T) {
	r, g, at := historyFixture()
	r.stats.ReferenceCost = nil
	r.stats.PricedRequests = 0
	r.stats.MissingPricingRequests = 10
	require.NoError(t, g.ApplyObservation(context.Background(), historyObservation(1, at, at.Add(time.Hour), 50)))
	require.Nil(t, r.rows[0].EstimatedReferenceLimit)
	require.Nil(t, r.rows[0].APIReferenceCost)
	require.Contains(t, r.rows[0].QualityFlags, "partial_start")
	require.Contains(t, r.rows[0].QualityFlags, "missing_pricing")
}

func TestWindowHistoryPartialCoverageSuppressesEstimateEvenWithPrices(t *testing.T) {
	r, g, at := historyFixture()
	require.NoError(t, g.ApplyObservation(context.Background(), historyObservation(1, at, at.Add(time.Hour), 50)))
	require.Equal(t, 20.0, *r.rows[0].APIReferenceCost)
	require.Nil(t, r.rows[0].EstimatedReferenceLimit)
	require.Contains(t, r.rows[0].QualityFlags, "partial_start")
}
func TestWindowHistoryExactNaturalBoundaryStartsNextPeriod(t *testing.T) {
	r, g, at := historyFixture()
	ctx := context.Background()
	reset := at.Add(5 * time.Hour)
	require.NoError(t, g.ApplyObservation(ctx, historyObservation(1, at, reset, 50)))
	require.NoError(t, g.ApplyObservation(ctx, historyObservation(2, reset, reset.Add(5*time.Hour), 0)))
	require.Len(t, r.rows, 2)
	require.Equal(t, reset, r.rows[1].WindowStart)
}
func TestWindowHistoryStalePastResetCannotOpenActiveRow(t *testing.T) {
	r, g, at := historyFixture()
	require.NoError(t, g.ApplyObservation(context.Background(), historyObservation(1, at, at.Add(-time.Hour), 99)))
	require.Empty(t, r.rows)
}
func TestWindowHistoryAmbiguousResetPreservesPreviousBasisOnly(t *testing.T) {
	r, g, at := historyFixture()
	ctx := context.Background()
	reset := at.Add(5 * time.Hour)
	require.NoError(t, g.ApplyObservation(ctx, historyObservation(1, at, reset, 25)))
	marker := at.Add(time.Minute)
	r.marker = &marker
	r.stats.ReferenceCost = historyMoney(60)
	require.NoError(t, g.ApplyObservation(ctx, historyObservation(2, marker.Add(time.Second), reset, 50)))
	require.Equal(t, 80.0, *r.rows[0].EstimatedReferenceLimit)
	require.Equal(t, 25.0, *r.rows[0].EstimateUsedPercent)
	require.Contains(t, r.rows[0].QualityFlags, "ambiguous_reset")
}
func TestWindowHistoryGapSuppressesNewEstimate(t *testing.T) {
	r, g, at := historyFixture()
	ctx := context.Background()
	reset := at.Add(5 * time.Hour)
	require.NoError(t, g.ApplyObservation(ctx, historyObservation(1, at, reset, 25)))
	nextStart := reset.Add(time.Hour)
	require.NoError(t, g.ApplyObservation(ctx, historyObservation(2, nextStart, nextStart.Add(5*time.Hour), 10)))
	require.Len(t, r.rows, 2)
	require.Contains(t, r.rows[1].QualityFlags, "observation_gap")
	require.Nil(t, r.rows[1].EstimatedReferenceLimit)
}

func TestWindowHistoryAccountEligibility(t *testing.T) {
	base := func() *Account { return &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth} }
	require.True(t, IsOpenAIQuotaHistoryAccount(base()))
	for _, dimension := range []string{"", "global", " GLOBAL "} {
		a := base()
		a.QuotaDimension = dimension
		require.True(t, IsOpenAIQuotaHistoryAccount(a))
	}
	for _, key := range []string{"auth_mode", "openai_auth_mode"} {
		for _, mode := range []string{"agent_identity", " Agent_Identity ", "personal_access_token", "personalAccessToken"} {
			a := base()
			a.Credentials = map[string]any{key: mode}
			require.False(t, IsOpenAIQuotaHistoryAccount(a))
		}
	}
	a := base()
	a.QuotaDimension = "spark"
	require.False(t, IsOpenAIQuotaHistoryAccount(a))
	a = base()
	parent := int64(1)
	a.ParentAccountID = &parent
	require.False(t, IsOpenAIQuotaHistoryAccount(a))
	a = base()
	a.Type = AccountTypeAPIKey
	require.False(t, IsOpenAIQuotaHistoryAccount(a))
}
func TestWindowHistoryNullPercentageIsMissing(t *testing.T) {
	r, g, at := historyFixture()
	obs := historyObservation(1, at, at.Add(5*time.Hour), 0)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(obs.Payload, &payload))
	payload["codex_5h_used_percent"] = nil
	obs.Payload, _ = json.Marshal(payload)
	require.NoError(t, g.ApplyObservation(context.Background(), obs))
	require.Empty(t, r.rows)
}

func TestWindowHistoryLateTerminalUpdatesOldPeriodOnly(t *testing.T) {
	r, g, at := historyFixture()
	ctx := context.Background()
	reset := at.Add(5 * time.Hour)
	require.NoError(t, g.ApplyObservation(ctx, historyObservation(1, at, reset, 50)))
	require.NoError(t, g.ApplyObservation(ctx, historyObservation(2, reset, reset.Add(5*time.Hour), 0)))
	require.Len(t, r.rows, 2)
	// The cumulative finalized amount is not the numerator at an earlier sample.
	finalized := reset.Add(5 * time.Minute)
	r.rows[0].StatsFinalizedAt = &finalized
	r.rows[0].APIReferenceCost = historyMoney(40)
	r.stats.ReferenceCost = historyMoney(30)
	require.NoError(t, g.ApplyObservation(ctx, historyObservation(3, reset.Add(-time.Second), reset, 100)))
	require.Len(t, r.rows, 2)
	require.Equal(t, 100.0, r.rows[0].LastUsedPercent)
	require.Equal(t, 100.0, r.rows[0].PeakUsedPercent)
	require.Equal(t, 40.0, *r.rows[0].APIReferenceCost)
	require.Equal(t, 30.0, *r.rows[0].EstimateReferenceCost)
	require.Equal(t, 30.0, *r.rows[0].EstimatedReferenceLimit)
	require.Equal(t, 0.0, r.rows[1].LastUsedPercent)
	require.Nil(t, r.rows[1].EstimatedReferenceLimit)
}

func TestWindowHistoryProducerRetryWithNewJournalIDIsIdempotent(t *testing.T) {
	r, g, at := historyFixture()
	ctx := context.Background()
	reset := at.Add(5 * time.Hour)
	require.NoError(t, g.ApplyObservation(ctx, historyObservation(1, at, reset, 10)))
	require.NoError(t, g.ApplyObservation(ctx, historyObservation(2, at, reset, 10)))
	require.Len(t, r.rows, 1)
	require.Equal(t, 1, r.rows[0].SampleCount)
	require.NoError(t, g.ApplyObservation(ctx, historyObservation(3, reset, reset.Add(5*time.Hour), 0)))
	require.NoError(t, g.ApplyObservation(ctx, historyObservation(4, reset, reset.Add(5*time.Hour), 0)))
	require.Len(t, r.rows, 2)
	require.Equal(t, 1, r.rows[1].SampleCount)
	require.NoError(t, g.ApplyObservation(ctx, historyObservation(5, reset.Add(-time.Second), reset, 100)))
	require.NoError(t, g.ApplyObservation(ctx, historyObservation(6, reset.Add(-time.Second), reset, 100)))
	require.Equal(t, 2, r.rows[0].SampleCount)
	require.Equal(t, 100.0, r.rows[0].LastUsedPercent)
}
