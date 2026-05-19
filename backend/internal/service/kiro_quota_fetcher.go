package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
)

// KiroQuotaFetcher implements QuotaFetcher for Kiro accounts.
// Mirrors AntigravityQuotaFetcher's shape: it knows how to call Kiro's
// /getUsageLimits endpoint and project the response into the shared
// QuotaResult struct that AccountUsageService consumes.
//
// Raw fields land under account.extra (usage/subscription) so the admin
// UI can render them; UsageInfo is populated with the bare-minimum
// SubscriptionTier so the standard "tier" badge shows correctly.
type KiroQuotaFetcher struct {
	proxyRepo ProxyRepository
}

// NewKiroQuotaFetcher constructs the fetcher.
func NewKiroQuotaFetcher(proxyRepo ProxyRepository) *KiroQuotaFetcher {
	return &KiroQuotaFetcher{proxyRepo: proxyRepo}
}

// CanFetch reports whether this account can be queried — must be a Kiro
// account with a usable access token.
func (f *KiroQuotaFetcher) CanFetch(account *Account) bool {
	if account == nil || account.Platform != PlatformKiro {
		return false
	}
	return account.GetCredential("access_token") != ""
}

// FetchQuota calls Kiro's /getUsageLimits and returns the shaped result.
// proxyURL is the resolved outbound proxy (empty for direct).
//
// On a 4xx/5xx the error propagates; the caller decides whether to mark
// the account as failed. We never partial-write into account.Extra here —
// the caller (AccountUsageService) is responsible for that persistence.
func (f *KiroQuotaFetcher) FetchQuota(ctx context.Context, account *Account, proxyURL string) (*QuotaResult, error) {
	accessToken := account.GetCredential("access_token")
	if accessToken == "" {
		return nil, fmt.Errorf("kiro quota: empty access token")
	}

	limits, err := kiro.FetchUsageLimits(accessToken, proxyURL)
	if err != nil {
		return nil, fmt.Errorf("kiro quota: %w", err)
	}

	raw := buildKiroQuotaRaw(limits)
	info := buildKiroUsageInfo(limits)
	return &QuotaResult{
		UsageInfo: info,
		Raw:       raw,
	}, nil
}

// GetProxyURL resolves the outbound proxy URL for an account. Used by
// AccountUsageService before invoking FetchQuota.
func (f *KiroQuotaFetcher) GetProxyURL(ctx context.Context, account *Account) string {
	if f.proxyRepo == nil || account == nil || account.ProxyID == nil {
		return ""
	}
	p, err := f.proxyRepo.GetByID(ctx, *account.ProxyID)
	if err != nil || p == nil {
		return ""
	}
	return p.URL()
}

// buildKiroQuotaRaw produces the JSONB blob persisted on account.extra.
// Schema matches the design spec:
//
//	{
//	  "usage": { "current", "limit", "percent", "next_reset_date",
//	             "trial_current", "trial_limit", "trial_status",
//	             "last_refresh" },
//	  "subscription": { "type", "title", "name", "status" }
//	}
func buildKiroQuotaRaw(u *kiro.UsageLimits) map[string]any {
	if u == nil {
		return nil
	}
	raw := map[string]any{
		"last_refresh": time.Now().Unix(),
	}

	if u.SubscriptionInfo != nil {
		raw["subscription"] = map[string]any{
			"type":   u.SubscriptionInfo.SubscriptionType,
			"name":   u.SubscriptionInfo.SubscriptionName,
			"title":  u.SubscriptionInfo.SubscriptionTitle,
			"status": u.SubscriptionInfo.Status,
		}
	}

	if agentic := u.AgenticUsage(); agentic != nil {
		usage := map[string]any{
			"current":  agentic.CurrentUsage,
			"limit":    agentic.UsageLimit,
			"currency": agentic.Currency,
			"unit":     agentic.Unit,
		}
		if agentic.UsageLimit > 0 {
			usage["percent"] = agentic.CurrentUsage / agentic.UsageLimit
		}
		if next := string(u.NextDateReset); next != "" {
			usage["next_reset_date"] = next
		}
		if agentic.FreeTrialInfo != nil {
			usage["trial_current"] = agentic.FreeTrialInfo.CurrentUsage
			usage["trial_limit"] = agentic.FreeTrialInfo.UsageLimit
			usage["trial_status"] = agentic.FreeTrialInfo.FreeTrialStatus
			if expiry := string(agentic.FreeTrialInfo.FreeTrialExpiry); expiry != "" {
				usage["trial_expires_at"] = expiry
			}
		}
		raw["usage"] = usage
	}
	return raw
}

// buildKiroUsageInfo builds the shared UsageInfo struct so the standard
// admin UI badges (subscription tier) light up. Other fields are left
// nil — Kiro's quota model doesn't map onto the existing five-hour /
// seven-day windows.
func buildKiroUsageInfo(u *kiro.UsageLimits) *UsageInfo {
	if u == nil {
		return nil
	}
	now := time.Now()
	info := &UsageInfo{
		Source:    "active",
		UpdatedAt: &now,
	}
	if u.SubscriptionInfo != nil {
		info.SubscriptionTier = kiroSubscriptionTier(u.SubscriptionInfo.SubscriptionType)
		info.SubscriptionTierRaw = u.SubscriptionInfo.SubscriptionType
	}
	return info
}

// kiroSubscriptionTier normalises Kiro's subscription_type string into
// the FREE/PRO/ULTRA/UNKNOWN vocabulary the admin UI already understands.
func kiroSubscriptionTier(raw string) string {
	switch raw {
	case "FREE":
		return "FREE"
	case "PRO", "PRO_PLUS":
		return "PRO"
	case "POWER":
		return "ULTRA"
	case "":
		return "UNKNOWN"
	}
	return "UNKNOWN"
}
