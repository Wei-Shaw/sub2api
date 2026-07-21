package service

import (
	"context"
	"fmt"
	"math"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	UpstreamBillingRateHistoryMaxEvents = 500

	upstreamBillingRateHistoryRetention     = 365 * 24 * time.Hour
	upstreamBillingRateHistoryCleanupDelay  = 5 * time.Minute
	upstreamBillingRateHistoryCleanupPeriod = 24 * time.Hour
	upstreamBillingRateHistoryCleanupBatch  = 5000
)

var ErrUpstreamBillingRateHistoryRangeInvalid = infraerrors.BadRequest(
	"INVALID_UPSTREAM_BILLING_RATE_HISTORY_RANGE",
	"days must be one of 7, 30, 90, or 365 and limit must be between 1 and 500",
)

type UpstreamBillingRateHistoryEvent struct {
	ID                      int64      `json:"id"`
	DetectedAt              time.Time  `json:"detected_at"`
	IntervalEnd             *time.Time `json:"interval_end"`
	CarriedIn               bool       `json:"carried_in"`
	GroupRateMultiplier     float64    `json:"group_rate_multiplier"`
	UserRateMultiplier      *float64   `json:"user_rate_multiplier"`
	PeakRateEnabled         bool       `json:"peak_rate_enabled"`
	PeakStart               *string    `json:"peak_start"`
	PeakEnd                 *string    `json:"peak_end"`
	PeakTimezone            *string    `json:"peak_timezone"`
	PeakRateMultiplier      *float64   `json:"peak_rate_multiplier"`
	ResolvedRateMultiplier  float64    `json:"resolved_rate_multiplier"`
	EffectiveRateMultiplier float64    `json:"effective_rate_multiplier"`
}

type UpstreamBillingRateHistory struct {
	AccountID int64                             `json:"account_id"`
	RangeDays int                               `json:"range_days"`
	Truncated bool                              `json:"truncated"`
	Events    []UpstreamBillingRateHistoryEvent `json:"events"`
}

type upstreamBillingRateHistoryReader interface {
	ListUpstreamBillingRateHistory(
		context.Context,
		int64,
		time.Time,
		int,
	) ([]UpstreamBillingRateHistoryEvent, bool, error)
}

type upstreamBillingRateHistoryPruner interface {
	DeleteUpstreamBillingRateHistoryBefore(context.Context, time.Time, int) (int64, error)
}

func (s *UpstreamBillingProbeService) GetAccountRateHistory(
	ctx context.Context,
	accountID int64,
	days int,
	limit int,
) (*UpstreamBillingRateHistory, error) {
	if s == nil || s.accountRepo == nil {
		return nil, ErrUpstreamBillingProbeUnavailable
	}
	if !validUpstreamBillingRateHistoryRange(days, limit) {
		return nil, ErrUpstreamBillingRateHistoryRangeInvalid
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if !isUpstreamBillingProbeAccount(account) {
		return nil, ErrUpstreamBillingProbeAccountInvalid
	}
	reader, ok := s.accountRepo.(upstreamBillingRateHistoryReader)
	if !ok {
		return nil, ErrUpstreamBillingProbeUnavailable
	}

	rangeStart := s.currentTime().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	events, truncated, err := reader.ListUpstreamBillingRateHistory(ctx, accountID, rangeStart, limit)
	if err != nil {
		return nil, err
	}
	if events == nil {
		events = make([]UpstreamBillingRateHistoryEvent, 0)
	}
	for i := range events {
		events[i].CarriedIn = events[i].DetectedAt.Before(rangeStart)
		events[i].IntervalEnd = nil
		if i+1 < len(events) {
			end := events[i+1].DetectedAt
			events[i].IntervalEnd = &end
		}
	}
	return &UpstreamBillingRateHistory{
		AccountID: accountID,
		RangeDays: days,
		Truncated: truncated,
		Events:    events,
	}, nil
}

func validUpstreamBillingRateHistoryRange(days, limit int) bool {
	if limit < 1 || limit > UpstreamBillingRateHistoryMaxEvents {
		return false
	}
	switch days {
	case 7, 30, 90, 365:
		return true
	default:
		return false
	}
}

// BuildUpstreamBillingRateHistoryEvent projects the already-sanitized success
// snapshot into the only fields allowed in history storage.
func BuildUpstreamBillingRateHistoryEvent(snapshot *UpstreamBillingProbeSnapshot) (*UpstreamBillingRateHistoryEvent, error) {
	if snapshot == nil || snapshot.Status != UpstreamBillingProbeStatusOK || snapshot.ReceivedAt == nil || snapshot.ReceivedAt.IsZero() {
		return nil, fmt.Errorf("invalid successful upstream billing snapshot")
	}
	data := snapshot.Data
	if scope, _ := data["billing_scope"].(string); scope != "token" {
		return nil, fmt.Errorf("invalid upstream billing scope")
	}
	group, err := requiredHistoryMultiplier(data, "group_rate_multiplier")
	if err != nil {
		return nil, err
	}
	resolved, err := requiredHistoryMultiplier(data, "resolved_rate_multiplier")
	if err != nil {
		return nil, err
	}
	effective, err := requiredHistoryMultiplier(data, "effective_rate_multiplier")
	if err != nil {
		return nil, err
	}

	event := &UpstreamBillingRateHistoryEvent{
		DetectedAt:              snapshot.ReceivedAt.UTC(),
		GroupRateMultiplier:     group,
		ResolvedRateMultiplier:  resolved,
		EffectiveRateMultiplier: effective,
	}
	if _, exists := data["user_rate_multiplier"]; exists {
		value, ok := resolveAccountExtraNumber(data, "user_rate_multiplier")
		if !ok || !validHistoryMultiplier(value) {
			return nil, fmt.Errorf("invalid user_rate_multiplier")
		}
		event.UserRateMultiplier = &value
	}

	peakEnabled, ok := data["peak_rate_enabled"].(bool)
	if !ok {
		return nil, fmt.Errorf("invalid peak_rate_enabled")
	}
	event.PeakRateEnabled = peakEnabled
	if !peakEnabled {
		return event, nil
	}

	start, startOK := data["peak_start"].(string)
	end, endOK := data["peak_end"].(string)
	timezoneName, timezoneOK := data["timezone"].(string)
	peak, peakOK := resolveAccountExtraNumber(data, "peak_rate_multiplier")
	startMinute, validStart := parseMinutes(start)
	endMinute, validEnd := parseMinutes(end)
	if !startOK || !endOK || !timezoneOK || !peakOK || !validStart || !validEnd ||
		startMinute >= endMinute || !validHistoryMultiplier(peak) {
		return nil, fmt.Errorf("invalid peak billing fields")
	}
	if _, err := time.LoadLocation(timezoneName); err != nil {
		return nil, fmt.Errorf("invalid peak timezone: %w", err)
	}
	event.PeakStart = &start
	event.PeakEnd = &end
	event.PeakTimezone = &timezoneName
	event.PeakRateMultiplier = &peak
	return event, nil
}

func requiredHistoryMultiplier(data map[string]any, key string) (float64, error) {
	value, ok := resolveAccountExtraNumber(data, key)
	if !ok || !validHistoryMultiplier(value) {
		return 0, fmt.Errorf("invalid %s", key)
	}
	return value, nil
}

func validHistoryMultiplier(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

// SameUpstreamBillingRateHistoryValues ignores event identity and time; only a
// normalized declared-rate component change creates a new row.
func SameUpstreamBillingRateHistoryValues(left, right *UpstreamBillingRateHistoryEvent) bool {
	if left == nil || right == nil ||
		left.PeakRateEnabled != right.PeakRateEnabled ||
		!equalBillingMultiplier(left.GroupRateMultiplier, right.GroupRateMultiplier) ||
		!equalOptionalBillingMultiplier(left.UserRateMultiplier, right.UserRateMultiplier) ||
		!equalOptionalBillingMultiplier(left.PeakRateMultiplier, right.PeakRateMultiplier) ||
		!equalOptionalString(left.PeakStart, right.PeakStart) ||
		!equalOptionalString(left.PeakEnd, right.PeakEnd) ||
		!equalOptionalString(left.PeakTimezone, right.PeakTimezone) ||
		!equalBillingMultiplier(left.ResolvedRateMultiplier, right.ResolvedRateMultiplier) ||
		!equalBillingMultiplier(left.EffectiveRateMultiplier, right.EffectiveRateMultiplier) {
		return false
	}
	return true
}

func equalOptionalBillingMultiplier(left, right *float64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return equalBillingMultiplier(*left, *right)
}

func equalOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func (s *UpstreamBillingProbeService) runRateHistoryRetentionLoop() {
	defer s.wg.Done()
	timer := time.NewTimer(upstreamBillingRateHistoryCleanupDelay)
	defer timer.Stop()
	select {
	case <-s.parentCtx.Done():
		return
	case <-timer.C:
	}
	s.cleanupRateHistory(s.parentCtx)

	ticker := time.NewTicker(upstreamBillingRateHistoryCleanupPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-s.parentCtx.Done():
			return
		case <-ticker.C:
			s.cleanupRateHistory(s.parentCtx)
		}
	}
}

func (s *UpstreamBillingProbeService) cleanupRateHistory(ctx context.Context) {
	pruner, ok := s.accountRepo.(upstreamBillingRateHistoryPruner)
	if !ok {
		return
	}
	cutoff := s.currentTime().UTC().Add(-upstreamBillingRateHistoryRetention)
	for {
		deleted, err := pruner.DeleteUpstreamBillingRateHistoryBefore(ctx, cutoff, upstreamBillingRateHistoryCleanupBatch)
		if err != nil {
			logger.LegacyPrintf("service.upstream_billing_probe", "rate_history_cleanup_failed: err=%v", err)
			return
		}
		if deleted < upstreamBillingRateHistoryCleanupBatch {
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}
