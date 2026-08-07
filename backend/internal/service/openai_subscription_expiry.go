package service

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/imroc/req/v3"
)

const (
	// OpenAISubscriptionExpirySnapshotKey is intentionally separate from
	// credentials.subscription_expires_at. OAuth refreshes may rewrite the
	// credentials map, but an explicit admin query must remain a stable row
	// snapshot until the next explicit query.
	OpenAISubscriptionExpirySnapshotKey = "openai_subscription_expiry_snapshot"

	OpenAISubscriptionExpiryStatusAvailable   = "available"
	OpenAISubscriptionExpiryStatusUnavailable = "unavailable"

	OpenAISubscriptionExpirySourceSubscriptions = "subscriptions"
	OpenAISubscriptionExpirySourceAccountsCheck = "accounts_check"
	OpenAISubscriptionExpirySourceUnavailable   = "unavailable"

	OpenAISubscriptionExpiryEffectiveSourceUpstream = "upstream"
	OpenAISubscriptionExpiryEffectiveSourceManual   = "manual"
	OpenAISubscriptionExpiryEffectiveSourceNone     = "none"

	openAISubscriptionExpiryUpstreamTimeout = 20 * time.Second
)

// OpenAISubscriptionExpirySnapshot is the result of one explicit upstream
// subscription-expiry query. CheckedAt, and any non-empty expiry, are RFC3339
// timestamps in UTC. WillRenew is deliberately not omitempty so false remains
// distinguishable from a missing field in the persisted JSON object.
type OpenAISubscriptionExpirySnapshot struct {
	Status    string `json:"status"`
	ExpiresAt string `json:"expires_at"`
	CheckedAt string `json:"checked_at"`
	Source    string `json:"source"`
	PlanType  string `json:"plan_type"`
	WillRenew bool   `json:"will_renew"`
}

// OpenAISubscriptionExpiryResult is returned after a single explicit query.
// Snapshot is always present on a successful result, including an
// unavailable snapshot. EffectiveExpiresAt may come from accounts.expires_at
// when the upstream query proved that no real expiry was available.
type OpenAISubscriptionExpiryResult struct {
	AccountID          int64                            `json:"account_id"`
	Snapshot           OpenAISubscriptionExpirySnapshot `json:"snapshot"`
	EffectiveExpiresAt string                           `json:"effective_expires_at,omitempty"`
	EffectiveSource    string                           `json:"effective_source"`
}

type openAISubscriptionResponse struct {
	PlanType    string `json:"plan_type"`
	ActiveUntil string `json:"active_until"`
	WillRenew   *bool  `json:"will_renew"`
}

type openAIAccountsCheckResponse struct {
	Accounts map[string]map[string]any `json:"accounts"`
}

type openAIAccountsCheckResult struct {
	ExpiresAt string
	PlanType  string
	WillRenew *bool
}

// QuerySubscriptionExpiry performs one user-triggered subscription lookup
// for accountID and persists the resulting snapshot on that same account row.
// Spark shadows resolve credentials and proxy through their parent in
// prepareUpstreamCall, but the manual effective-expiry fallback and cache write
// deliberately use the clicked row.
func (s *OpenAIQuotaService) QuerySubscriptionExpiry(ctx context.Context, accountID int64) (*OpenAISubscriptionExpiryResult, error) {
	if s == nil || s.accountRepo == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_SUBSCRIPTION_EXPIRY_NOT_CONFIGURED", "openai subscription expiry service is not configured")
	}

	clickedAccount, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || clickedAccount == nil {
		return nil, infraerrors.New(http.StatusNotFound, "OPENAI_SUBSCRIPTION_EXPIRY_ACCOUNT_NOT_FOUND", "account not found")
	}
	if clickedAccount.Platform != PlatformOpenAI {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_SUBSCRIPTION_EXPIRY_INVALID_PLATFORM", "account is not an OpenAI account")
	}
	if clickedAccount.Type != AccountTypeOAuth {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_SUBSCRIPTION_EXPIRY_INVALID_TYPE", "account is not an OAuth account")
	}

	// prepareUpstreamCall performs the canonical shadow resolution, token
	// acquisition, account-id validation, and proxy lookup used by quota calls.
	accessToken, chatGPTAccountID, proxyURL, fedRAMP, err := s.prepareUpstreamCall(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(accessToken) == "" {
		// Agent-identity accounts intentionally do not carry an OAuth bearer
		// token. This endpoint is only for OpenAI OAuth subscription accounts.
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_SUBSCRIPTION_EXPIRY_TOKEN_UNAVAILABLE", "openai OAuth access token is unavailable")
	}

	client, err := s.privacyClientFactory(proxyURL)
	if err != nil {
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_SUBSCRIPTION_EXPIRY_CLIENT_ERROR", "failed to build upstream client")
	}

	callCtx, cancel := context.WithTimeout(ctx, openAISubscriptionExpiryUpstreamTimeout)
	defer cancel()
	headers := buildCodexCommonHeaders(accessToken, chatGPTAccountID, fedRAMP)
	headers["Origin"] = "https://chatgpt.com"
	headers["Referer"] = "https://chatgpt.com/"
	headers["Accept"] = "application/json"

	subscription, subscriptionsOK := queryOpenAISubscription(callCtx, client, headers, chatGPTAccountID)
	if subscriptionsOK {
		if expiresAt := normalizeOpenAIExpiry(subscription.ActiveUntil); expiresAt != "" {
			return s.persistSubscriptionExpiryResult(ctx, accountID, clickedAccount, OpenAISubscriptionExpirySnapshot{
				Status:    OpenAISubscriptionExpiryStatusAvailable,
				ExpiresAt: expiresAt,
				CheckedAt: time.Now().UTC().Format(time.RFC3339Nano),
				Source:    OpenAISubscriptionExpirySourceSubscriptions,
				PlanType:  strings.TrimSpace(subscription.PlanType),
				WillRenew: boolValue(subscription.WillRenew),
			})
		}
	}

	// active_until is absent (or the subscriptions endpoint failed), so use
	// accounts/check as the compatibility source. A successful, parseable
	// envelope with no entitlement expiry is enough to establish unavailable;
	// a failed/malformed fallback is not and must preserve the old cache.
	accountCheck, accountsCheckOK := queryOpenAIAccountsCheck(callCtx, client, headers)
	if !accountsCheckOK {
		if subscriptionsOK && strings.TrimSpace(subscription.ActiveUntil) == "" {
			return s.persistSubscriptionExpiryResult(ctx, accountID, clickedAccount, OpenAISubscriptionExpirySnapshot{
				Status:    OpenAISubscriptionExpiryStatusUnavailable,
				CheckedAt: time.Now().UTC().Format(time.RFC3339Nano),
				Source:    OpenAISubscriptionExpirySourceUnavailable,
				PlanType:  strings.TrimSpace(subscription.PlanType),
				WillRenew: boolValue(subscription.WillRenew),
			})
		}
		if !subscriptionsOK {
			slog.Warn("openai_subscription_expiry_upstream_failed", "account_id", accountID)
		}
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_SUBSCRIPTION_EXPIRY_UPSTREAM_ERROR", "failed to obtain a trusted subscription expiry from upstream")
	}

	planType := strings.TrimSpace(subscription.PlanType)
	willRenew := boolValue(subscription.WillRenew)
	if planType == "" {
		planType = strings.TrimSpace(accountCheck.PlanType)
	}
	if subscription.WillRenew == nil {
		willRenew = boolValue(accountCheck.WillRenew)
	}

	expiresAt := normalizeOpenAIExpiry(accountCheck.ExpiresAt)
	source := OpenAISubscriptionExpirySourceUnavailable
	status := OpenAISubscriptionExpiryStatusUnavailable
	if expiresAt != "" {
		source = OpenAISubscriptionExpirySourceAccountsCheck
		status = OpenAISubscriptionExpiryStatusAvailable
	}

	return s.persistSubscriptionExpiryResult(ctx, accountID, clickedAccount, OpenAISubscriptionExpirySnapshot{
		Status:    status,
		ExpiresAt: expiresAt,
		CheckedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Source:    source,
		PlanType:  planType,
		WillRenew: willRenew,
	})
}

func queryOpenAISubscription(ctx context.Context, client *req.Client, headers map[string]string, accountID string) (openAISubscriptionResponse, bool) {
	var payload openAISubscriptionResponse
	resp, err := client.R().
		SetContext(ctx).
		SetHeaders(headers).
		SetSuccessResult(&payload).
		SetQueryParam("account_id", accountID).
		Get(chatGPTSubscriptionsURL)
	if err != nil {
		slog.Debug("openai_subscription_expiry_subscriptions_request_error", "error", err)
		return openAISubscriptionResponse{}, false
	}
	if !resp.IsSuccessState() {
		slog.Debug("openai_subscription_expiry_subscriptions_failed", "status", resp.StatusCode)
		return openAISubscriptionResponse{}, false
	}
	return payload, true
}

func queryOpenAIAccountsCheck(ctx context.Context, client *req.Client, headers map[string]string) (openAIAccountsCheckResult, bool) {
	var payload openAIAccountsCheckResponse
	resp, err := client.R().
		SetContext(ctx).
		SetHeaders(headers).
		SetSuccessResult(&payload).
		Get(chatGPTAccountsCheckURL)
	if err != nil {
		slog.Debug("openai_subscription_expiry_accounts_check_request_error", "error", err)
		return openAIAccountsCheckResult{}, false
	}
	if !resp.IsSuccessState() {
		slog.Debug("openai_subscription_expiry_accounts_check_failed", "status", resp.StatusCode)
		return openAIAccountsCheckResult{}, false
	}
	if payload.Accounts == nil {
		return openAIAccountsCheckResult{}, false
	}

	account, ok := payload.Accounts[strings.TrimSpace(headers["chatgpt-account-id"])]
	if !ok {
		// A legacy response may contain exactly one account while omitting the
		// organization key used by the credential. It is safe to use that one
		// candidate. Multiple unmatched accounts are not a trusted "unavailable"
		// result because we cannot tell which workspace belongs to this row.
		if len(payload.Accounts) != 1 {
			return openAIAccountsCheckResult{}, false
		}
		for _, candidate := range payload.Accounts {
			account = candidate
		}
	}

	return openAIAccountsCheckResult{
		ExpiresAt: extractEntitlementExpiresAt(account),
		PlanType:  extractPlanType(account),
		WillRenew: extractWillRenew(account),
	}, true
}

func (s *OpenAIQuotaService) persistSubscriptionExpiryResult(ctx context.Context, accountID int64, clickedAccount *Account, snapshot OpenAISubscriptionExpirySnapshot) (*OpenAISubscriptionExpiryResult, error) {
	if err := s.accountRepo.UpdateExtra(ctx, accountID, map[string]any{
		OpenAISubscriptionExpirySnapshotKey: snapshot,
	}); err != nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_SUBSCRIPTION_EXPIRY_CACHE_WRITE_FAILED", "failed to cache subscription expiry snapshot").WithCause(err)
	}

	result := &OpenAISubscriptionExpiryResult{
		AccountID:       accountID,
		Snapshot:        snapshot,
		EffectiveSource: OpenAISubscriptionExpiryEffectiveSourceNone,
	}
	if snapshot.Status == OpenAISubscriptionExpiryStatusAvailable && snapshot.ExpiresAt != "" {
		result.EffectiveExpiresAt = snapshot.ExpiresAt
		result.EffectiveSource = OpenAISubscriptionExpiryEffectiveSourceUpstream
	} else if clickedAccount != nil && clickedAccount.ExpiresAt != nil {
		result.EffectiveExpiresAt = clickedAccount.ExpiresAt.UTC().Format(time.RFC3339Nano)
		result.EffectiveSource = OpenAISubscriptionExpiryEffectiveSourceManual
	}
	return result, nil
}

func normalizeOpenAIExpiry(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func extractWillRenew(account map[string]any) *bool {
	if account == nil {
		return nil
	}
	for _, candidate := range []map[string]any{
		account,
		mapValue(account, "account"),
		mapValue(account, "entitlement"),
	} {
		if candidate == nil {
			continue
		}
		if value, ok := candidate["will_renew"].(bool); ok {
			return &value
		}
	}
	return nil
}

func mapValue(values map[string]any, key string) map[string]any {
	value, _ := values[key].(map[string]any)
	return value
}
