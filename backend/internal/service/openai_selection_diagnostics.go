package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type OpenAISelectionError struct {
	Phase  string
	Cause  string
	Detail string
	Err    error
}

func (e *OpenAISelectionError) Error() string {
	if e == nil {
		return ""
	}

	base := "openai account selection failed"
	if phase := strings.TrimSpace(e.Phase); phase != "" {
		base += " at " + phase
	}

	cause := strings.TrimSpace(e.Cause)
	if cause == "" && e.Err != nil {
		cause = strings.TrimSpace(e.Err.Error())
	}
	detail := strings.TrimSpace(e.Detail)

	switch {
	case cause != "" && detail != "":
		return fmt.Sprintf("%s: %s (%s)", base, cause, detail)
	case cause != "":
		return fmt.Sprintf("%s: %s", base, cause)
	case detail != "":
		return fmt.Sprintf("%s (%s)", base, detail)
	default:
		return base
	}
}

func (e *OpenAISelectionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type openAISelectionFilterStats struct {
	Total                 int
	Eligible              int
	Excluded              int
	Unschedulable         int
	ModelUnsupported      int
	ChannelRestricted     int
	CompactUnsupported    int
	TransportIncompatible int
}

type openAISelectionAttemptStats struct {
	PrefilterCandidates       int
	LoadBelowThreshold        int
	Saturated                 int
	FreshLookupRejected       int
	RecheckRejected           int
	ChannelRestrictedAfterDB  int
	CompactUnsupportedAfterDB int
	TransportRejectedAfterDB  int
	SlotAcquireBusy           int
	SlotAcquireErrors         int
}

func newOpenAISelectionError(phase, cause, detail string, err error) error {
	return &OpenAISelectionError{
		Phase:  strings.TrimSpace(phase),
		Cause:  strings.TrimSpace(cause),
		Detail: strings.TrimSpace(detail),
		Err:    err,
	}
}

func newOpenAINoAvailableAccountsError(phase, cause, detail string, compactBlocked bool) error {
	baseErr := ErrNoAvailableAccounts
	if compactBlocked {
		baseErr = ErrNoAvailableCompactAccounts
	}
	return newOpenAISelectionError(phase, cause, detail, baseErr)
}

func isOpenAISelectionAbortError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func formatOpenAISelectionScope(groupID *int64, requestedModel string, extra ...string) string {
	parts := make([]string, 0, 3+len(extra))
	parts = append(parts, fmt.Sprintf("group_id=%d", derefGroupID(groupID)))
	if model := strings.TrimSpace(requestedModel); model != "" {
		parts = append(parts, "model="+model)
	}
	for _, item := range extra {
		item = strings.TrimSpace(item)
		if item != "" {
			parts = append(parts, item)
		}
	}
	return strings.Join(parts, " ")
}

func summarizeOpenAISelectionFilterStats(stats openAISelectionFilterStats) string {
	return fmt.Sprintf(
		"total=%d eligible=%d excluded=%d unschedulable=%d model_unsupported=%d channel_restricted=%d compact_unsupported=%d transport_incompatible=%d",
		stats.Total,
		stats.Eligible,
		stats.Excluded,
		stats.Unschedulable,
		stats.ModelUnsupported,
		stats.ChannelRestricted,
		stats.CompactUnsupported,
		stats.TransportIncompatible,
	)
}

func summarizeOpenAISelectionAttemptStats(stats openAISelectionAttemptStats) string {
	return fmt.Sprintf(
		"prefilter_candidates=%d load_below_threshold=%d saturated=%d fresh_lookup_rejected=%d recheck_rejected=%d channel_restricted_after_db=%d compact_unsupported_after_db=%d transport_rejected_after_db=%d slot_acquire_busy=%d slot_acquire_errors=%d",
		stats.PrefilterCandidates,
		stats.LoadBelowThreshold,
		stats.Saturated,
		stats.FreshLookupRejected,
		stats.RecheckRejected,
		stats.ChannelRestrictedAfterDB,
		stats.CompactUnsupportedAfterDB,
		stats.TransportRejectedAfterDB,
		stats.SlotAcquireBusy,
		stats.SlotAcquireErrors,
	)
}

func (s *OpenAIGatewayService) collectOpenAISelectionFilterStats(
	ctx context.Context,
	groupID *int64,
	accounts []Account,
	requestedModel string,
	excludedIDs map[int64]struct{},
	requiredTransport OpenAIUpstreamTransport,
	requireCompact bool,
) openAISelectionFilterStats {
	stats := openAISelectionFilterStats{Total: len(accounts)}
	needsUpstreamCheck := groupID != nil && s.needsUpstreamChannelRestrictionCheck(ctx, groupID)

	for i := range accounts {
		acc := &accounts[i]
		if acc == nil {
			stats.Unschedulable++
			continue
		}
		if excludedIDs != nil {
			if _, excluded := excludedIDs[acc.ID]; excluded {
				stats.Excluded++
				continue
			}
		}
		if !acc.IsSchedulable() || !acc.IsOpenAI() {
			stats.Unschedulable++
			continue
		}
		if requestedModel != "" && !acc.IsModelSupported(requestedModel) {
			stats.ModelUnsupported++
			continue
		}
		if requiredTransport != OpenAIUpstreamTransportAny && !s.isOpenAIAccountTransportCompatible(acc, requiredTransport) {
			stats.TransportIncompatible++
			continue
		}
		if needsUpstreamCheck && s.isUpstreamModelRestrictedByChannel(ctx, *groupID, acc, requestedModel, requireCompact) {
			stats.ChannelRestricted++
			continue
		}
		if requireCompact && openAICompactSupportTier(acc) == 0 {
			stats.CompactUnsupported++
			continue
		}
		stats.Eligible++
	}

	return stats
}
