package service

import (
	"math"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

// AntigravityQuotaGroup retains upstream pool boundaries, including account-
// dependent Argon pools and future groups. Never aggregate unrelated models.
type AntigravityQuotaGroup struct {
	Name        string                   `json:"name"`
	Description string                   `json:"description,omitempty"`
	Buckets     []AntigravityQuotaBucket `json:"buckets"`
}

type AntigravityQuotaBucket struct {
	ID          string     `json:"id"`
	Name        string     `json:"name,omitempty"`
	Window      string     `json:"window"`
	Utilization *float64   `json:"utilization"`
	ResetsAt    *time.Time `json:"resets_at"`
	Disabled    bool       `json:"disabled,omitempty"`
}

func antigravityQuotaGroups(summary *antigravity.QuotaSummary) []AntigravityQuotaGroup {
	if summary == nil {
		return nil
	}
	groups := make([]AntigravityQuotaGroup, 0, len(summary.Groups))
	for _, source := range summary.Groups {
		group := AntigravityQuotaGroup{
			Name: source.DisplayName, Description: source.Description,
			Buckets: make([]AntigravityQuotaBucket, 0, len(source.Buckets)),
		}
		for _, b := range source.Buckets {
			if strings.TrimSpace(b.ID) == "" {
				continue
			}
			bucket := AntigravityQuotaBucket{
				ID: b.ID, Name: b.DisplayName, Window: quotaBucketWindow(b), Disabled: b.Disabled,
			}
			if reset, err := time.Parse(time.RFC3339, b.ResetTime); err == nil {
				bucket.ResetsAt = &reset
			}
			if b.RemainingFraction != nil && !b.Disabled {
				fraction := *b.RemainingFraction
				if !math.IsNaN(fraction) && !math.IsInf(fraction, 0) && fraction >= 0 && fraction <= 1 {
					used := (1 - fraction) * 100
					bucket.Utilization = &used
				}
			}
			// Missing fractions remain unknown; disabled and empty buckets must
			// stay visible instead of becoming an invented 0% or disappearing.
			group.Buckets = append(group.Buckets, bucket)
		}
		if len(group.Buckets) > 0 {
			groups = append(groups, group)
		}
	}
	return groups
}

func quotaBucketWindow(bucket antigravity.QuotaBucket) string {
	if bucket.Window != "" {
		return bucket.Window
	}
	switch bucket.ID {
	case "gemini-5h", "3p-5h":
		return "5h"
	case "gemini-weekly", "3p-weekly", "gemini-argon-wtu-weekly":
		return "weekly"
	default:
		return "unknown"
	}
}

func antigravityForbiddenUsage(err *antigravity.ForbiddenError) *UsageInfo {
	now := time.Now()
	kind := classifyForbiddenType(err.Body)
	return &UsageInfo{
		UpdatedAt: &now, IsForbidden: true, ForbiddenReason: err.Body,
		ForbiddenType: kind, ValidationURL: extractValidationURL(err.Body),
		NeedsVerify: kind == forbiddenTypeValidation,
		IsBanned:    kind == forbiddenTypeViolation, ErrorCode: errorCodeForbidden,
	}
}

// Retain an explicitly stale snapshot on transient errors, never as fresh quota.
// Authentication failures must not keep displaying a previously valid account.
func retainAntigravityQuotaSnapshot(previous, current *UsageInfo) {
	if previous == nil || current == nil || current.NeedsReauth || current.IsForbidden {
		return
	}
	if current.SubscriptionTierRaw != "" && previous.SubscriptionTierRaw != "" &&
		current.SubscriptionTierRaw != previous.SubscriptionTierRaw {
		return // A confirmed tier change invalidates the old quota snapshot.
	}
	if len(previous.AntigravityQuotaGroups) == 0 && len(previous.AntigravityQuota) == 0 {
		return
	}
	current.AntigravityQuotaGroups = previous.AntigravityQuotaGroups
	if len(current.AntigravityQuota) == 0 {
		current.AntigravityQuota = previous.AntigravityQuota
		current.AntigravityQuotaDetails = previous.AntigravityQuotaDetails
	}
	current.AntigravityQuotaSource = previous.AntigravityQuotaSource
	current.AntigravityQuotaStale = true
	current.UpdatedAt = previous.UpdatedAt
	current.SubscriptionTier = previous.SubscriptionTier
	current.SubscriptionTierRaw = previous.SubscriptionTierRaw
}
