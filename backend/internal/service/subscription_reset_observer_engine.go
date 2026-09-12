package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	subscriptionResetJitter       = 2 * time.Second
	subscriptionResetPeriod       = 7 * 24 * time.Hour
	subscriptionResetEarlyHigh    = 20.0
	subscriptionResetEarlyLow     = 5.0
	subscriptionResetRepeatDelay  = 30 * time.Second
	subscriptionResetRecentEvents = 32
)

func subscriptionResetFreshness(p *SubscriptionResetPolicy) time.Duration {
	freshness := 2 * time.Duration(p.AggregationMinutes) * time.Minute
	if freshness < 15*time.Minute {
		return 15 * time.Minute
	}
	return freshness
}

func subscriptionResetHash(parts ...any) string {
	encoded, _ := json.Marshal(parts)
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:])
}

func subscriptionResetSubjectKey(subject *SubscriptionResetQuotaSubject) string {
	if subject == nil || strings.TrimSpace(subject.UserID) == "" || strings.TrimSpace(subject.AccountID) == "" || strings.TrimSpace(subject.Scope) != "codex:global" {
		return ""
	}
	return subscriptionResetHash(strings.TrimSpace(subject.UserID), strings.TrimSpace(subject.AccountID), strings.TrimSpace(subject.Scope))
}

// AdvanceSubscriptionResetObserver is deterministic: all clocks and provider
// observations are inputs. Its caller persists state and changed audit events in
// the same transaction. Early drops require opt-in and repeated low evidence.
func AdvanceSubscriptionResetObserver(p *SubscriptionResetPolicy, state *SubscriptionResetObserverState, sample *SubscriptionResetSample, now time.Time) ([]*SubscriptionResetEvent, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if state == nil {
		return nil, fmt.Errorf("subscription reset observer state is required")
	}
	if p.Mode == "off" {
		return nil, nil
	}
	changed := map[string]*SubscriptionResetEvent{}
	for _, event := range state.Events {
		if event.Status == "pending" && now.After(event.DeadlineAt) {
			event.Status, event.Reason, event.UpdatedAt = "timed_out", "quorum_not_met", now
			changed[event.ID] = event
		}
	}
	finish := func() ([]*SubscriptionResetEvent, error) {
		out := make([]*SubscriptionResetEvent, 0, len(changed))
		for _, event := range changed {
			out = append(out, event)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		return out, nil
	}
	if sample == nil || sample.ObservedAt.IsZero() || !sample.ObservedAt.After(p.UpdatedAt) || sample.ObservedAt.After(now.Add(subscriptionResetJitter)) {
		return finish()
	}
	selected := false
	for _, id := range p.AccountIDs {
		if id == sample.AccountID {
			selected = true
			break
		}
	}
	if !selected {
		return finish()
	}
	if state.Accounts == nil {
		state.Accounts = map[int64]*SubscriptionResetObserverAccount{}
	}
	account := state.Accounts[sample.AccountID]
	if account == nil {
		account = &SubscriptionResetObserverAccount{SubscriptionResetAccountState: SubscriptionResetAccountState{AccountID: sample.AccountID}}
		state.Accounts[sample.AccountID] = account
	}
	if account.LastObservedAt != nil && !sample.ObservedAt.After(*account.LastObservedAt) {
		return finish()
	}
	at := sample.ObservedAt.UTC()
	account.LastObservedAt = &at
	if state.LastObservedAt == nil || at.After(*state.LastObservedAt) {
		state.LastObservedAt = &at
	}
	account.State, account.Reason, account.LastError = "unknown", "missing_quota_fields", ""
	account.UsedPercent, account.ResetAt, account.WindowMinutes = nil, nil, nil
	if safeError := subscriptionResetSafeReason(sample.Error); safeError != "" {
		account.Reason, account.LastError = safeError, safeError
		return finish()
	}
	if now.Sub(at) > subscriptionResetFreshness(p) {
		account.Reason = "stale_sample"
		return finish()
	}
	key := subscriptionResetSubjectKey(sample.Subject)
	if key == "" {
		account.Reason = "unverified_subject"
		return finish()
	}
	if account.SubjectKey != "" && account.SubjectKey != key {
		account.ReviewReason = "subject_or_plan_changed"
	}
	account.SubjectKey, account.PlanType = key, strings.ToLower(strings.TrimSpace(sample.PlanType))
	if sample.UsedPercent == nil || sample.ResetAt == nil || sample.WindowMinutes == nil {
		return finish()
	}
	used := *sample.UsedPercent
	if math.IsNaN(used) || math.IsInf(used, 0) || used < 0 || used > 100 || *sample.WindowMinutes != 10080 || sample.ResetAt.Before(at.Add(-subscriptionResetJitter)) || sample.ResetAt.After(at.Add(subscriptionResetPeriod+time.Minute)) {
		account.Reason = "invalid_quota_fields"
		return finish()
	}
	reset := sample.ResetAt.UTC()
	minutes := *sample.WindowMinutes
	account.UsedPercent, account.ResetAt, account.WindowMinutes = &used, &reset, &minutes
	previous := account.Baseline
	current := &SubscriptionResetEvidence{ObservedAt: at, SubjectKey: key, Used: used, ResetAt: reset, PlanType: account.PlanType}
	account.State, account.Reason = "observing", "baseline_ready"
	if sample.LocalResetAt != nil && sample.LocalResetAt.After(at.Add(subscriptionResetJitter)) && !sample.LocalResetAt.After(now.Add(subscriptionResetJitter)) {
		invalidateSubscriptionResetEarlyEvidence(state, key, "local_reset_excluded", now, changed)
		account.State, account.Reason, account.Baseline = "local_reset", "local_reset_excluded", nil
		return finish()
	}
	if previous != nil && (previous.SubjectKey != key || previous.PlanType != current.PlanType) {
		account.ReviewReason = "subject_or_plan_changed"
	}
	if account.ReviewReason != "" {
		if previous != nil {
			invalidateSubscriptionResetEarlyEvidence(state, previous.SubjectKey, account.ReviewReason, now, changed)
		}
		account.State, account.Reason = "needs_review", account.ReviewReason
		account.Baseline = current
		return finish()
	}
	if previous == nil || at.Sub(previous.ObservedAt) > subscriptionResetFreshness(p) {
		if previous != nil {
			account.Reason = "baseline_refreshed_after_gap"
		}
		account.Baseline = current
		return finish()
	}
	if sample.LocalResetAt != nil && !sample.LocalResetAt.After(at.Add(subscriptionResetJitter)) && (sample.LocalResetAt.After(previous.ObservedAt) || at.Sub(*sample.LocalResetAt) <= subscriptionResetFreshness(p)) {
		invalidateSubscriptionResetEarlyEvidence(state, key, "local_reset_excluded", now, changed)
		account.State, account.Reason, account.Baseline = "local_reset", "local_reset_excluded", current
		return finish()
	}
	resetDelta := current.ResetAt.Sub(previous.ResetAt)
	if resetDelta < -subscriptionResetJitter {
		account.State, account.Reason = "unknown", "replayed_window"
		// Keep the last accepted baseline: a cached old window must not rewind it.
		return finish()
	}
	if absSubscriptionResetDuration(resetDelta) <= subscriptionResetJitter {
		current.ResetAt = previous.ResetAt
		reset = previous.ResetAt
	}
	account.Baseline = current
	advanceSubscriptionResetEarlyEvidence(p, state, current, now, changed)
	// An opted-in single subject needs two distinct fresh observations of the
	// same new window, separated in time; duplicate imports cannot provide them.
	for _, event := range state.Events {
		if event.Status != "pending" || event.Denominator != 1 || !p.AllowSingleSubject || at.After(event.DeadlineAt) || event.Kind != "natural_reset" {
			continue
		}
		member := &event.Members[0]
		if member.SubjectKey == key && member.NewResetAt != nil && member.LastEvidenceAt != nil && at.Sub(*member.LastEvidenceAt) >= subscriptionResetRepeatDelay && absSubscriptionResetDuration(current.ResetAt.Sub(*member.NewResetAt)) <= subscriptionResetJitter {
			member.ConfirmationSamples++
			member.LastEvidenceAt, member.ConfirmedAt, member.Confirmed = &at, &at, true
			member.State, member.Reason = "confirmed", "repeated_fresh_sample"
			updateSubscriptionResetQuorum(event, p, now)
			changed[event.ID] = event
		}
	}
	kind := ""
	if resetDelta > subscriptionResetJitter {
		nearNextPeriod := absSubscriptionResetDuration(resetDelta-subscriptionResetPeriod) <= time.Duration(p.AggregationMinutes)*time.Minute+subscriptionResetJitter
		if !at.Before(previous.ResetAt.Add(-subscriptionResetJitter)) && nearNextPeriod {
			kind = "natural_reset"
		} else if previous.Used >= subscriptionResetEarlyHigh && current.Used <= subscriptionResetEarlyLow {
			kind = "early_drop"
		} else {
			account.State, account.Reason = "needs_review", "early_reset_threshold_not_met"
		}
	} else if previous.Used >= subscriptionResetEarlyHigh && current.Used <= subscriptionResetEarlyLow {
		kind = "early_drop"
	}
	if kind == "" {
		return finish()
	}
	if kind == "early_drop" {
		account.State, account.Reason = "needs_review", "early_reset_unverified"
		if p.AllowEarlyResets {
			account.State, account.Reason = "observing", "early_repeat_sample_required"
		}
	}
	var event *SubscriptionResetEvent
	previousEventID, ambiguous := "", false
	if kind == "early_drop" {
		event, previousEventID, ambiguous = findSubscriptionResetEarlyEvent(p, state, previous, now)
	} else {
		for i := len(state.Events) - 1; i >= 0; i-- {
			candidate := state.Events[i]
			if candidate.PolicyVersion == p.Version && candidate.Kind == kind && absSubscriptionResetDuration(candidate.SourceResetAt.Sub(previous.ResetAt)) <= time.Duration(p.AggregationMinutes)*time.Minute+subscriptionResetJitter {
				event = candidate
				break
			}
		}
	}
	if event == nil {
		event = newSubscriptionResetEvent(p, state, previous, current, kind, now)
		event.PreviousEventID = previousEventID
		if ambiguous {
			event.Status, event.Reason = "needs_review", "ambiguous_early_batch"
			account.State, account.Reason = "needs_review", "ambiguous_early_batch"
		}
		state.Events = append(state.Events, event)
		trimSubscriptionResetEvents(p, state, now)
	}
	for i := range event.Members {
		member := &event.Members[i]
		if member.SubjectKey != key || member.TransitionID != "" {
			continue
		}
		member.TransitionID = subscriptionResetHash(key, previous.ResetAt, current.ResetAt, previous.ObservedAt, current.ObservedAt)
		oldReset, newReset := previous.ResetAt, current.ResetAt
		oldUsed, newUsed := previous.Used, current.Used
		member.OldResetAt, member.NewResetAt, member.LastEvidenceAt = &oldReset, &newReset, &at
		baselineAt := previous.ObservedAt
		member.FirstEvidenceAt, member.BaselineObservedAt, member.PlanType = &at, &baselineAt, current.PlanType
		member.OldUsedPercent, member.NewUsedPercent = &oldUsed, &newUsed
		member.ConfirmationSamples = 1
		member.State, member.Reason = "needs_review", "early_reset_unverified"
		if kind == "early_drop" && p.AllowEarlyResets && event.Status != "needs_review" {
			member.State, member.Reason = "pending", "repeat_low_sample_required"
		}
		if kind == "natural_reset" {
			member.Confirmed = event.Denominator > 1
			member.State, member.Reason = "pending", "repeat_sample_required"
			if member.Confirmed {
				member.ConfirmedAt = &at
				member.State, member.Reason = "confirmed", "natural_window_advanced"
				if at.After(event.DeadlineAt) {
					member.State, member.Reason = "late_confirmed", "outside_aggregation_window"
				}
			}
		}
	}
	updateSubscriptionResetQuorum(event, p, now)
	changed[event.ID] = event
	return finish()
}

func newSubscriptionResetEvent(p *SubscriptionResetPolicy, state *SubscriptionResetObserverState, previous, current *SubscriptionResetEvidence, kind string, now time.Time) *SubscriptionResetEvent {
	event := &SubscriptionResetEvent{ID: subscriptionResetHash(p.GroupID, p.Version, kind, previous.SubjectKey, previous.ResetAt, current.ResetAt, current.ObservedAt), GroupID: p.GroupID, PolicyVersion: p.Version, Source: p.Source, Kind: kind, Status: "pending", OpenedAt: current.ObservedAt, DeadlineAt: current.ObservedAt.Add(time.Duration(p.AggregationMinutes) * time.Minute), UpdatedAt: now, SourceResetAt: previous.ResetAt, ResetDimensions: append([]string{}, p.ResetDimensions...), Members: []SubscriptionResetEventMember{}}
	bySubject := map[string]int{}
	ids := append([]int64{}, p.AccountIDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		key, reason, memberState := "unknown:"+strconv.FormatInt(id, 10), "unverified_subject", "unknown"
		if account := state.Accounts[id]; account != nil && account.SubjectKey != "" {
			key, reason, memberState = account.SubjectKey, "waiting_for_reset", "pending"
			if account.State == "unknown" || account.State == "local_reset" || account.ReviewReason != "" {
				reason, memberState = account.Reason, "unknown"
			}
		}
		if index, exists := bySubject[key]; exists {
			event.Members[index].AccountIDs = append(event.Members[index].AccountIDs, id)
			continue
		}
		bySubject[key] = len(event.Members)
		event.Members = append(event.Members, SubscriptionResetEventMember{SubjectKey: key, AccountIDs: []int64{id}, State: memberState, Reason: reason})
	}
	event.Denominator = len(event.Members)
	event.RequiredCount = (event.Denominator*p.QuorumPercent + 99) / 100
	if event.Denominator > 1 && event.RequiredCount < 2 {
		event.RequiredCount = 2
	}
	if kind == "early_drop" && !p.AllowEarlyResets {
		event.Status, event.Reason = "needs_review", "early_reset_unverified"
	} else if event.Denominator == 1 && !p.AllowSingleSubject {
		event.Status, event.Reason = "needs_review", "single_subject_requires_opt_in"
	}
	return event
}

func updateSubscriptionResetQuorum(event *SubscriptionResetEvent, p *SubscriptionResetPolicy, now time.Time) {
	event.ConfirmedCount = 0
	for _, member := range event.Members {
		if member.Confirmed {
			event.ConfirmedCount++
		}
	}
	event.UpdatedAt = now
	if event.Status != "pending" {
		return
	}
	if now.After(event.DeadlineAt) {
		event.Status, event.Reason = "timed_out", "quorum_not_met"
		return
	}
	if event.ConfirmedCount >= event.RequiredCount && (event.Denominator > 1 || p.AllowSingleSubject) {
		confirmed := now.UTC()
		event.Status, event.Reason, event.ConfirmedAt = "confirmed", "observation_quorum_met", &confirmed
		event.ConfirmedKind = "natural"
		if event.Kind == "early_drop" {
			event.ConfirmedKind = "early"
		}
	}
}

// Each early batch names its predecessor. Matching the subject's own observed
// transition chain prevents a delayed first reset joining a later batch merely
// because both share the same upstream reset_at value.
func findSubscriptionResetEarlyEvent(p *SubscriptionResetPolicy, state *SubscriptionResetObserverState, previous *SubscriptionResetEvidence, now time.Time) (*SubscriptionResetEvent, string, bool) {
	previousID := ""
	var latestAt time.Time
	ambiguous := false
	for _, event := range state.Events {
		if event.PolicyVersion != p.Version || event.Kind != "early_drop" {
			continue
		}
		for _, member := range event.Members {
			if member.SubjectKey != previous.SubjectKey || member.FirstEvidenceAt == nil || member.NewResetAt == nil || absSubscriptionResetDuration(member.NewResetAt.Sub(previous.ResetAt)) > subscriptionResetJitter {
				continue
			}
			if previous.ObservedAt.Equal(*member.FirstEvidenceAt) {
				ambiguous = true
			}
			if previous.ObservedAt.After(*member.FirstEvidenceAt) && member.FirstEvidenceAt.After(latestAt) {
				previousID, latestAt = event.ID, *member.FirstEvidenceAt
			}
		}
	}
	var found *SubscriptionResetEvent
	if previousID == "" && state.EarlyHistoryTruncatedUntil != nil && !now.After(*state.EarlyHistoryTruncatedUntil) {
		ambiguous = true
	}
	for _, event := range state.Events {
		if event.PolicyVersion != p.Version || event.Kind != "early_drop" || event.PreviousEventID != previousID || absSubscriptionResetDuration(event.SourceResetAt.Sub(previous.ResetAt)) > time.Duration(p.AggregationMinutes)*time.Minute+subscriptionResetJitter {
			continue
		}
		memberExists := false
		for _, member := range event.Members {
			memberExists = memberExists || member.SubjectKey == previous.SubjectKey
		}
		if !memberExists || found != nil {
			ambiguous = true
		}
		found = event
	}
	if ambiguous {
		return nil, previousID, true
	}
	return found, previousID, false
}

func trimSubscriptionResetEvents(p *SubscriptionResetPolicy, state *SubscriptionResetObserverState, now time.Time) {
	if len(state.Events) <= subscriptionResetRecentEvents {
		return
	}
	cut := len(state.Events) - subscriptionResetRecentEvents
	retained := make([]*SubscriptionResetEvent, 0, subscriptionResetRecentEvents+2)
	for i, event := range state.Events {
		// A still-fresh baseline can report this same natural boundary later.
		// Retain its identity even when early review events fill the recent list.
		retention := subscriptionResetFreshness(p) + time.Duration(p.AggregationMinutes)*time.Minute + subscriptionResetJitter
		if i >= cut || (event.Kind == "natural_reset" && !now.After(event.SourceResetAt.Add(retention))) {
			retained = append(retained, event)
			continue
		}
		if event.Kind == "early_drop" {
			until := event.SourceResetAt.Add(retention)
			if state.EarlyHistoryTruncatedUntil == nil || until.After(*state.EarlyHistoryTruncatedUntil) {
				state.EarlyHistoryTruncatedUntil = &until
			}
		}
	}
	state.Events = retained
}

func advanceSubscriptionResetEarlyEvidence(p *SubscriptionResetPolicy, state *SubscriptionResetObserverState, current *SubscriptionResetEvidence, now time.Time, changed map[string]*SubscriptionResetEvent) {
	if !p.AllowEarlyResets {
		return
	}
	for _, event := range state.Events {
		if event.PolicyVersion != p.Version || event.Kind != "early_drop" || event.Status == "needs_review" || event.Status == "config_changed" {
			continue
		}
		for i := range event.Members {
			member := &event.Members[i]
			if member.SubjectKey != current.SubjectKey || member.Confirmed || member.State != "pending" || member.FirstEvidenceAt == nil || member.NewResetAt == nil || !current.ObservedAt.After(*member.FirstEvidenceAt) {
				continue
			}
			if current.Used > subscriptionResetEarlyLow || member.PlanType != current.PlanType || absSubscriptionResetDuration(current.ResetAt.Sub(*member.NewResetAt)) > subscriptionResetJitter || current.ObservedAt.Sub(*member.FirstEvidenceAt) > subscriptionResetFreshness(p) {
				member.State, member.Reason = "needs_review", "early_low_samples_interrupted"
				changed[event.ID] = event
				continue
			}
			if current.ObservedAt.Sub(*member.FirstEvidenceAt) < subscriptionResetRepeatDelay {
				continue
			}
			at := current.ObservedAt
			member.ConfirmationSamples++
			member.LastEvidenceAt, member.ConfirmedAt, member.Confirmed = &at, &at, true
			member.State, member.Reason = "confirmed", "repeated_fresh_low_sample"
			if at.After(event.DeadlineAt) {
				member.State, member.Reason = "late_confirmed", "outside_aggregation_window"
			}
			changed[event.ID] = event
		}
		if changed[event.ID] != nil {
			updateSubscriptionResetQuorum(event, p, now)
		}
	}
}

func invalidateSubscriptionResetEarlyEvidence(state *SubscriptionResetObserverState, subjectKey, reason string, now time.Time, changed map[string]*SubscriptionResetEvent) {
	for _, event := range state.Events {
		if event.Kind != "early_drop" {
			continue
		}
		for i := range event.Members {
			member := &event.Members[i]
			if member.SubjectKey == subjectKey && member.State == "pending" && member.FirstEvidenceAt != nil && !member.Confirmed {
				member.State, member.Reason = "needs_review", reason
				event.UpdatedAt = now
				changed[event.ID] = event
			}
		}
	}
}

func absSubscriptionResetDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}
