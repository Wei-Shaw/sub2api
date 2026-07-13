package service

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

const grokQuotaSnapshotExtraKey = "grok_usage_snapshot"

type GrokQuotaFetcher struct{}

func NewGrokQuotaFetcher() *GrokQuotaFetcher {
	return &GrokQuotaFetcher{}
}

func (f *GrokQuotaFetcher) BuildUsageInfo(account *Account) *UsageInfo {
	now := time.Now()
	usage := &UsageInfo{
		Source:    "passive",
		UpdatedAt: &now,
	}
	if account == nil {
		usage.ErrorCode = "quota_unknown"
		usage.Error = "Grok quota is unknown until billing is probed or an upstream response includes xAI rate-limit headers"
		return usage
	}

	billing, billingAt, billingOK := grokBillingFromExtra(account.Extra)
	if billingOK {
		usage.GrokBilling = billing
		usage.GrokBillingUpdatedAt = billingAt
		if billing.PlanLabel != "" {
			usage.GrokBillingPlanLabel = billing.PlanLabel
			// Surface plan label as subscription tier for badge UIs that already
			// render subscription_tier.
			if usage.SubscriptionTier == "" {
				usage.SubscriptionTier = billing.PlanLabel
				usage.SubscriptionTierRaw = billing.PlanLabel
			}
		}
		if parsedAt, err := time.Parse(time.RFC3339, billingAt); err == nil {
			usage.UpdatedAt = &parsedAt
		}
		usage.GrokQuotaSnapshotState = "billing_observed"
		usage.ErrorCode = ""
		usage.Error = ""
	}

	snapshot, err := grokQuotaSnapshotFromExtra(account.Extra)
	if err != nil || snapshot == nil {
		if !billingOK {
			usage.ErrorCode = "quota_unknown"
			usage.Error = "Grok quota is unknown until billing is probed or an upstream response includes xAI rate-limit headers"
		}
		return usage
	}

	if parsedAt, err := time.Parse(time.RFC3339, snapshot.UpdatedAt); err == nil {
		// Prefer the more recent of billing vs rate-limit snapshot.
		if usage.UpdatedAt == nil || parsedAt.After(*usage.UpdatedAt) {
			usage.UpdatedAt = &parsedAt
		}
	}
	usage.GrokRequestQuota = snapshot.Requests
	usage.GrokTokenQuota = snapshot.Tokens
	usage.GrokRetryAfterSeconds = snapshot.RetryAfterSeconds
	if snapshot.SubscriptionTier != "" {
		usage.SubscriptionTier = snapshot.SubscriptionTier
		usage.SubscriptionTierRaw = snapshot.SubscriptionTier
	}
	usage.GrokEntitlementStatus = snapshot.EntitlementStatus
	usage.GrokLastQuotaProbeAt = snapshot.LastProbeAt
	usage.GrokLastHeadersSeenAt = snapshot.LastHeadersSeenAt
	usage.GrokLastStatusCode = snapshot.StatusCode
	if snapshot.HasObservedHeaders() {
		if !billingOK {
			usage.GrokQuotaSnapshotState = "observed"
		}
	} else if !billingOK {
		usage.GrokQuotaSnapshotState = "no_headers"
		usage.ErrorCode = "quota_unknown"
		usage.Error = "No xAI quota headers observed on the latest Grok probe"
	}

	switch snapshot.StatusCode {
	case 401:
		usage.NeedsReauth = true
		usage.ErrorCode = "unauthenticated"
	case 403:
		usage.IsForbidden = true
		usage.ForbiddenType = "forbidden"
		usage.ErrorCode = "forbidden"
		if usage.GrokEntitlementStatus == "" {
			usage.GrokEntitlementStatus = "forbidden"
		}
	case 429:
		usage.ErrorCode = "rate_limited"
	}
	return usage
}

func grokQuotaSnapshotFromExtra(extra map[string]any) (*xai.QuotaSnapshot, error) {
	if extra == nil {
		return nil, nil
	}
	raw, ok := extra[grokQuotaSnapshotExtraKey]
	if !ok || raw == nil {
		return nil, nil
	}
	switch snapshot := raw.(type) {
	case *xai.QuotaSnapshot:
		return snapshot, nil
	case xai.QuotaSnapshot:
		return &snapshot, nil
	case map[string]any:
		data, err := json.Marshal(snapshot)
		if err != nil {
			return nil, err
		}
		var out xai.QuotaSnapshot
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, err
		}
		return &out, nil
	default:
		data, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("marshal grok quota snapshot: %w", err)
		}
		var out xai.QuotaSnapshot
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, err
		}
		return &out, nil
	}
}

func grokBillingFromExtra(extra map[string]any) (*xai.BillingSummary, string, bool) {
	if extra == nil {
		return nil, "", false
	}
	updatedAt, _ := extra[grokBillingUpdatedAtKey].(string)
	raw, ok := extra[grokBillingExtraKey]
	if !ok || raw == nil {
		return nil, updatedAt, false
	}
	billing, err := decodeGrokBilling(raw)
	if err != nil || billing == nil || !billing.HasData() {
		return nil, updatedAt, false
	}
	if updatedAt == "" {
		updatedAt = billing.UpdatedAt
	}
	return billing, updatedAt, true
}

func decodeGrokBilling(raw any) (*xai.BillingSummary, error) {
	switch v := raw.(type) {
	case *xai.BillingSummary:
		return v, nil
	case xai.BillingSummary:
		return &v, nil
	case map[string]any:
		data, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		var out xai.BillingSummary
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, err
		}
		return &out, nil
	default:
		data, err := json.Marshal(raw)
		if err != nil {
			return nil, err
		}
		var out xai.BillingSummary
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, err
		}
		return &out, nil
	}
}
