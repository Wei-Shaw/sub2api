package service

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"strconv"
	"sync"
	"time"
)

const (
	ingesterTickInterval         = 15 * time.Second
	ingesterObservationGrace     = 30 * time.Second
	ingesterFinalizeGrace        = 5 * time.Minute
	ingesterSweepLimit           = 500
	ingesterResetEpsilon         = 2 * time.Second
	windowHistoryRetentionDays   = 90
	windowStaleOpenRetentionDays = 14
	minimumEstimateUtilization   = 5.0
)

type AccountWindowUsageIngester struct {
	windowRepo AccountWindowUsageRepository
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.Mutex
	started    bool
	stopped    bool
	wg         sync.WaitGroup
	now        func() time.Time
}

func NewAccountWindowUsageIngester(repo AccountWindowUsageRepository) *AccountWindowUsageIngester {
	ctx, cancel := context.WithCancel(context.Background())
	return &AccountWindowUsageIngester{windowRepo: repo, ctx: ctx, cancel: cancel, now: time.Now}
}
func (g *AccountWindowUsageIngester) Start() {
	if g == nil || g.windowRepo == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.started || g.stopped {
		return
	}
	g.started = true
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		ticker := time.NewTicker(ingesterTickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-g.ctx.Done():
				return
			case <-ticker.C:
				g.runOnce(g.ctx)
			}
		}
	}()
}
func (g *AccountWindowUsageIngester) Stop() {
	if g == nil {
		return
	}
	g.mu.Lock()
	g.stopped = true
	g.cancel()
	g.mu.Unlock()
	g.wg.Wait()
}
func (g *AccountWindowUsageIngester) runOnce(ctx context.Context) {
	defer func() {
		if p := recover(); p != nil {
			slog.Error("account_window_usage: tick panic", "panic", p)
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, ingesterTickInterval)
	defer cancel()
	now := g.now()
	for i := 0; i < ingesterSweepLimit; i++ {
		ok, err := g.windowRepo.ConsumeNextObservation(ctx, now.Add(-ingesterObservationGrace), g.ApplyObservation)
		if err != nil {
			slog.Warn("account_window_usage: observation remains pending", "error", err)
			break
		}
		if !ok {
			break
		}
	}
	for i := 0; i < ingesterSweepLimit; i++ {
		ok, err := g.windowRepo.ReconcileNextWindow(ctx, now.Add(-ingesterFinalizeGrace), func(ctx context.Context, row *AccountWindowUsageRecord) error {
			if err := g.refreshStats(ctx, row, row.WindowEnd); err != nil {
				return err
			}
			if row.FinalizedAt == nil {
				row.FinalizedAt = &now
				reason := "expired"
				row.EndReason = &reason
			}
			row.StatsFinalizedAt = &now
			row.QualityFlags = withoutFlag(row.QualityFlags, "pending_usage")
			return g.windowRepo.SaveWindow(ctx, row)
		})
		if err != nil {
			slog.Warn("account_window_usage: reconciliation remains pending", "error", err)
			break
		}
		if !ok {
			break
		}
	}
}
func (g *AccountWindowUsageIngester) RunDailyMaintenance(ctx context.Context) {
	if g == nil || g.windowRepo == nil {
		return
	}
	now := g.now()
	if err := g.windowRepo.PruneHistory(ctx, now.AddDate(0, 0, -windowHistoryRetentionDays), now.AddDate(0, 0, -windowStaleOpenRetentionDays)); err != nil {
		slog.Warn("account_window_usage: retention cleanup failed", "error", err)
	}
}

// ApplyObservation is called only inside the journal transaction. Unlike polling
// accounts.extra, this keeps observations that are overwritten before a tick.
func (g *AccountWindowUsageIngester) ApplyObservation(ctx context.Context, obs *AccountQuotaObservation) error {
	if obs == nil {
		return nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(obs.Payload, &payload); err != nil {
		return nil
	}
	// Reset credits report a count, not a window bitmask. The marker is durable in
	// the journal; infer its scope only from each window's subsequent observation.
	for _, kind := range []string{"5h", "7d"} {
		prefix := "codex_" + kind
		used, ok := historyNumber(payload[prefix+"_used_percent"])
		if !ok || used < 0 {
			continue
		}
		var resetString string
		if json.Unmarshal(payload[prefix+"_reset_at"], &resetString) != nil {
			continue
		}
		reset, err := time.Parse(time.RFC3339Nano, resetString)
		if err != nil {
			continue
		}
		minutes, hasMinutes := historyNumber(payload[prefix+"_window_minutes"])
		expected := int(windowTypeDuration[kind] / time.Minute)
		// Missing duration uses the established canonical key semantics; an explicit
		// other duration is never silently labelled 5h/7d (e.g. free monthly windows).
		if hasMinutes && minutes != float64(expected) {
			continue
		}
		if err := g.applyWindow(ctx, obs, kind, used, reset, expected); err != nil {
			return err
		}
	}
	return nil
}
func historyNumber(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return 0, false
	}
	var n float64
	if json.Unmarshal(raw, &n) != nil {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return 0, false
		}
		var err error
		n, err = strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, false
		}
	}
	return n, !math.IsNaN(n) && !math.IsInf(n, 0)
}
func (g *AccountWindowUsageIngester) applyWindow(ctx context.Context, obs *AccountQuotaObservation, kind string, used float64, reset time.Time, minutes int) error {
	if reset.Before(obs.ObservedAt.Add(-ingesterResetEpsilon)) {
		return nil
	}
	row, err := g.windowRepo.GetOpenWindow(ctx, obs.AccountID, kind)
	if err != nil {
		return err
	}
	// A different replica can commit an older terminal observation after the
	// next period has already opened. Route by the actual reset identity and
	// observation time, rather than dropping it or merging it into the new row.
	if row == nil || (row.LastSampleAt != nil && obs.ObservedAt.Before(*row.LastSampleAt)) || absDuration(reset.Sub(row.ResetAt)) > ingesterResetEpsilon {
		closed, err := g.windowRepo.GetClosedWindow(ctx, obs.AccountID, kind, reset, obs.ObservedAt)
		if err != nil {
			return err
		}
		if closed != nil {
			return g.applyLateClosedObservation(ctx, closed, obs, used)
		}
	}
	if row != nil && absDuration(reset.Sub(row.ResetAt)) <= ingesterResetEpsilon && duplicateWindowObservation(row, obs, used) {
		return nil
	}
	if row != nil && (obs.ID <= row.LastObservationID || (row.LastSampleAt != nil && obs.ObservedAt.Before(*row.LastSampleAt))) {
		return nil
	}
	if row != nil {
		oldReset := row.ResetAt
		different := absDuration(reset.Sub(oldReset)) > ingesterResetEpsilon
		markerSince := row.FirstObservedAt
		if row.LastSampleAt != nil {
			markerSince = *row.LastSampleAt
		}
		marker, err := g.windowRepo.LatestResetMarker(ctx, obs.AccountID, markerSince, obs.ObservedAt)
		if err != nil {
			return err
		}
		switch {
		case marker != nil && marker.Before(oldReset) && (different || used+5 < row.LastUsedPercent):
			// The reset marker provides the cut point; the new observation identifies
			// the affected window. Freeze its previous estimate before changing bounds.
			end := *marker
			if end.Before(row.WindowStart) {
				end = row.WindowStart
			}
			if end.After(row.WindowStart) {
				row.WindowEnd = end
				reason := "early_reset"
				row.EndReason = &reason
				row.FinalizedAt = &obs.ObservedAt
				addFlag(row, "pending_usage")
				if err := g.windowRepo.SaveWindow(ctx, row); err != nil {
					return err
				}
				row = nil
			}
		case different && !obs.ObservedAt.Before(oldReset.Add(-ingesterResetEpsilon)):
			reason := "expired"
			row.EndReason = &reason
			row.FinalizedAt = &obs.ObservedAt
			addFlag(row, "pending_usage")
			if err := g.windowRepo.SaveWindow(ctx, row); err != nil {
				return err
			}
			row = nil
		case different:
			// A boundary move before expiry can mean an early reset or a provider
			// correction. Without a reset marker, do not blend two windows' costs.
			row.WindowEnd = obs.ObservedAt
			if !row.WindowEnd.After(row.WindowStart) {
				return nil
			}
			reason := "window_changed"
			row.EndReason = &reason
			row.FinalizedAt = &obs.ObservedAt
			addFlag(row, "ambiguous_reset")
			addFlag(row, "pending_usage")
			if err := g.windowRepo.SaveWindow(ctx, row); err != nil {
				return err
			}
			row = nil
		case marker != nil:
			// A used reset credit did not conclusively change this particular window.
			addFlag(row, "ambiguous_reset")
		}
	}
	if row == nil {
		previous, err := g.windowRepo.GetLatestWindow(ctx, obs.AccountID, kind)
		if err != nil {
			return err
		}
		// Never re-open a sealed window because an out-of-order/duplicate sample
		// arrived after reconciliation. Past observations remain journaled evidence.
		if previous != nil && (obs.ID <= previous.LastObservationID || obs.ObservedAt.Before(previous.WindowEnd) || (absDuration(previous.ResetAt.Sub(reset)) <= ingesterResetEpsilon && previous.EndReason != nil && *previous.EndReason == "expired")) {
			return nil
		}
		start := reset.Add(-time.Duration(minutes) * time.Minute)
		row = &AccountWindowUsageRecord{AccountID: obs.AccountID, WindowType: kind, ResetAt: reset, DurationMinutes: minutes, AccountWindowUsageEntry: AccountWindowUsageEntry{WindowStart: start, WindowEnd: reset, FirstObservedAt: obs.ObservedAt, QualityFlags: []string{}}}
		if previous != nil && previous.WindowEnd.After(start) {
			row.WindowStart = previous.WindowEnd
			if previous.EndReason != nil && *previous.EndReason != "early_reset" {
				addFlag(row, "ambiguous_reset")
			}
		}
		if !row.WindowEnd.After(row.WindowStart) {
			return nil
		}
		// We can observe a low initial percentage, but cannot prove all prior local
		// traffic exists. Keep the amount visible and disclose partial capture.
		if obs.ObservedAt.After(row.WindowStart.Add(time.Minute)) {
			addFlag(row, "partial_start")
		}
		if previous != nil && row.WindowStart.Sub(previous.WindowEnd) > time.Minute {
			addFlag(row, "observation_gap")
		}
	}
	row.SampleCount++
	row.LastObservationID = obs.ID
	row.LastSampleAt = &obs.ObservedAt
	row.LastUsedPercent = used
	if used > row.PeakUsedPercent {
		row.PeakUsedPercent = used
	}
	statsEnd := obs.ObservedAt
	if statsEnd.After(row.WindowEnd) {
		statsEnd = row.WindowEnd
	}
	if err := g.refreshStats(ctx, row, statsEnd); err != nil {
		return err
	}
	g.updateEstimate(row, used, obs.ObservedAt)
	return g.windowRepo.SaveWindow(ctx, row)
}
func (g *AccountWindowUsageIngester) updateEstimate(row *AccountWindowUsageRecord, used float64, observedAt time.Time) {
	row.QualityFlags = withoutFlag(row.QualityFlags, "low_utilization")
	if used < minimumEstimateUtilization {
		addFlag(row, "low_utilization")
	} else if used <= 100 && row.APIReferenceCost != nil && row.MissingPricingRequests == 0 && row.PricedRequests > 0 && !historyCoverageIncomplete(row) {
		// Only complete observed boundaries and fully priced local records may
		// produce a new estimate. An older usable estimate keeps its own basis.
		estimate := *row.APIReferenceCost * 100 / used
		if !math.IsNaN(estimate) && !math.IsInf(estimate, 0) {
			basis := *row.APIReferenceCost
			row.EstimatedReferenceLimit = &estimate
			row.EstimateReferenceCost = &basis
			v := used
			row.EstimateUsedPercent = &v
			at := observedAt
			row.EstimateObservedAt = &at
		}
	}
}
func (g *AccountWindowUsageIngester) applyLateClosedObservation(ctx context.Context, row *AccountWindowUsageRecord, obs *AccountQuotaObservation, used float64) error {
	if duplicateWindowObservation(row, obs, used) {
		return nil
	}
	if obs.ID <= row.LastObservationID || (row.LastSampleAt != nil && obs.ObservedAt.Before(*row.LastSampleAt)) {
		return nil
	}
	row.LastObservationID = obs.ID
	row.LastSampleAt = &obs.ObservedAt
	row.SampleCount++
	row.LastUsedPercent = used
	if used > row.PeakUsedPercent {
		row.PeakUsedPercent = used
	}
	// Calculate the late sample's numerator at its original time. Do not replace
	// the separately finalized full-period accumulated amount with this subtotal.
	basis := *row
	end := obs.ObservedAt
	if end.After(row.WindowEnd) {
		end = row.WindowEnd
	}
	if err := g.refreshStats(ctx, &basis, end); err != nil {
		return err
	}
	g.updateEstimate(&basis, used, obs.ObservedAt)
	row.EstimatedReferenceLimit = basis.EstimatedReferenceLimit
	row.EstimateReferenceCost = basis.EstimateReferenceCost
	row.EstimateUsedPercent = basis.EstimateUsedPercent
	row.EstimateObservedAt = basis.EstimateObservedAt
	row.QualityFlags = withoutFlag(row.QualityFlags, "low_utilization")
	if used < minimumEstimateUtilization {
		addFlag(row, "low_utilization")
	}
	return g.windowRepo.SaveWindow(ctx, row)
}

func (g *AccountWindowUsageIngester) refreshStats(ctx context.Context, row *AccountWindowUsageRecord, end time.Time) error {
	stats, err := g.windowRepo.AggregateReferenceUsage(ctx, row.AccountID, row.WindowStart, end)
	if err != nil {
		return err
	}
	row.Requests = stats.Requests
	row.TokensTotal = stats.TokensTotal
	row.APIReferenceCost = stats.ReferenceCost
	row.PricedRequests = stats.PricedRequests
	row.MissingPricingRequests = stats.MissingPricingRequests
	row.QualityFlags = withoutFlag(row.QualityFlags, "missing_pricing")
	if stats.MissingPricingRequests > 0 {
		addFlag(row, "missing_pricing")
	}
	return nil
}
func addFlag(row *AccountWindowUsageRecord, flag string) {
	for _, v := range row.QualityFlags {
		if v == flag {
			return
		}
	}
	row.QualityFlags = append(row.QualityFlags, flag)
}
func withoutFlag(flags []string, flag string) []string {
	out := make([]string, 0, len(flags))
	for _, v := range flags {
		if v != flag {
			out = append(out, v)
		}
	}
	return out
}
func absDuration(v time.Duration) time.Duration {
	if v < 0 {
		return -v
	}
	return v
}

func historyCoverageIncomplete(row *AccountWindowUsageRecord) bool {
	for _, flag := range row.QualityFlags {
		switch flag {
		case "partial_start", "ambiguous_reset", "observation_gap":
			return true
		}
	}
	return false
}

// A producer can retry after the database committed but its acknowledgement was
// lost. Distinct journal ids with the same window sample are equivalent; a
// different percentage at the same timestamp remains a distinct observation.
// The caller has already matched the window reset identity.
func duplicateWindowObservation(row *AccountWindowUsageRecord, obs *AccountQuotaObservation, used float64) bool {
	return row.LastSampleAt != nil && row.LastSampleAt.Equal(obs.ObservedAt) && row.LastUsedPercent == used
}
