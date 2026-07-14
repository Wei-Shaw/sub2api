package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

const (
	grokQuotaUpstreamTimeout = 20 * time.Second
	grokBillingExtraKey      = "grok_billing_snapshot"
	grokBillingUpdatedAtKey  = "grok_billing_updated_at"
	// Free tier on cli-chat-proxy accepts grok-4.5 and returns rate-limit headers
	// (observed as ~21 requests / 2M tokens). api.x.ai returns 402 for free.
	grokFreeRateLimitProbeModel = "grok-4.5"
)

// GrokQuotaProbeResult is returned by active quota probes.
// Source is "billing_probe" for Grok CLI weekly/monthly billing endpoints
// (preferred), which does not burn Responses tokens.
type GrokQuotaProbeResult struct {
	Source          string              `json:"source"`
	Model           string              `json:"model,omitempty"`
	Billing         *xai.BillingSummary `json:"billing,omitempty"`
	Snapshot        *xai.QuotaSnapshot  `json:"snapshot,omitempty"`
	StatusCode      int                 `json:"status_code,omitempty"`
	HeadersObserved bool                `json:"headers_observed"`
	BillingObserved bool                `json:"billing_observed"`
	ResetSupported  bool                `json:"reset_supported"`
	FetchedAt       int64               `json:"fetched_at"`
}

type GrokQuotaResetResult struct {
	Supported bool   `json:"supported"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

type GrokQuotaService struct {
	accountRepo   AccountRepository
	proxyRepo     ProxyRepository
	tokenProvider *GrokTokenProvider
	httpUpstream  HTTPUpstream
}

func NewGrokQuotaService(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	tokenProvider *GrokTokenProvider,
	httpUpstream HTTPUpstream,
) *GrokQuotaService {
	return &GrokQuotaService{
		accountRepo:   accountRepo,
		proxyRepo:     proxyRepo,
		tokenProvider: tokenProvider,
		httpUpstream:  httpUpstream,
	}
}

// ProbeUsage refreshes Grok subscription quota.
//  1. CLI billing: weekly % + monthly credits (SuperGrok).
//  2. Free tier: billing has monthlyLimit=0 and no weekly usage % — also send a
//     tiny CLI-proxy Responses probe to read rate-limit remaining (e.g. 21 req).
func (s *GrokQuotaService) ProbeUsage(ctx context.Context, accountID int64) (*GrokQuotaProbeResult, error) {
	account, token, proxyURL, err := s.prepareProbe(ctx, accountID)
	if err != nil {
		return nil, err
	}

	callCtx, cancel := context.WithTimeout(ctx, grokQuotaUpstreamTimeout)
	defer cancel()

	userID := resolveGrokBillingUserID(account)
	billing, err := s.fetchGrokBilling(callCtx, account, token, proxyURL, userID)
	if err != nil {
		slog.Warn("grok_billing_probe_failed", "account_id", account.ID, "error", err)
		return nil, infraerrors.Newf(http.StatusBadGateway, "GROK_QUOTA_PROBE_UPSTREAM_ERROR", "Grok billing probe failed: %v", err)
	}
	if billing == nil || !billing.HasData() {
		return nil, infraerrors.New(http.StatusBadGateway, "GROK_QUOTA_PROBE_EMPTY", "Grok billing probe returned empty data")
	}

	billing.Source = "billing_probe"
	if billing.UpdatedAt == "" {
		billing.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	extraUpdates := map[string]any{
		grokBillingExtraKey:     billing,
		grokBillingUpdatedAtKey: billing.UpdatedAt,
	}
	// Persist plan label for account list badges (shown under account name / platform column).
	if plan := strings.TrimSpace(billing.PlanLabel); plan != "" {
		extraUpdates["plan_type"] = plan
	}

	result := &GrokQuotaProbeResult{
		Source:          "billing_probe",
		Billing:         billing,
		BillingObserved: true,
		ResetSupported:  false,
		FetchedAt:       time.Now().Unix(),
	}

	// Free (or missing weekly usage %) accounts: capture CLI rate-limit windows.
	// Live check: free OAuth works on cli-chat-proxy with grok-4.5 → ~21 req / 2M tokens;
	// api.x.ai stays 402 for free.
	if isGrokFreeBilling(billing) || billing.UsagePercent == nil {
		snapshot, probeModel, statusCode, probeErr := s.probeGrokRateLimitHeaders(callCtx, account, token, proxyURL, userID)
		if probeErr != nil {
			slog.Warn("grok_ratelimit_probe_failed", "account_id", account.ID, "error", probeErr)
		} else if snapshot != nil {
			extraUpdates[grokQuotaSnapshotExtraKey] = snapshot
			result.Snapshot = snapshot
			result.HeadersObserved = snapshot.HeadersObserved
			result.StatusCode = statusCode
			result.Model = probeModel
			if isGrokFreeBilling(billing) {
				result.Source = "billing_and_ratelimit_probe"
			}
		}
	}

	_ = s.accountRepo.UpdateExtra(ctx, account.ID, extraUpdates)
	return result, nil
}

func isGrokFreeBilling(b *xai.BillingSummary) bool {
	if b == nil {
		return false
	}
	plan := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(b.PlanLabel), "-", "_"))
	if plan == "grok_free" {
		return true
	}
	if b.MonthlyLimitCents != nil && *b.MonthlyLimitCents == 0 {
		if b.OnDemandCapCents == nil || *b.OnDemandCapCents == 0 {
			return true
		}
	}
	return false
}

// probeGrokRateLimitHeaders sends a minimal Responses call on the account's
// Grok base URL (subscription proxy for OAuth) to observe rate-limit headers.
func (s *GrokQuotaService) probeGrokRateLimitHeaders(
	ctx context.Context,
	account *Account,
	token, proxyURL, userID string,
) (*xai.QuotaSnapshot, string, int, error) {
	model := grokFreeRateLimitProbeModel
	body, err := json.Marshal(map[string]any{
		"model":             model,
		"input":             ".",
		"max_output_tokens": 1,
		"store":             false,
	})
	if err != nil {
		return nil, model, 0, err
	}
	targetURL, err := xai.BuildResponsesURL(account.GetGrokBaseURL())
	if err != nil {
		return nil, model, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, model, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	applyGrokCLIHeaders(req.Header)
	if uid := strings.TrimSpace(userID); uid != "" {
		req.Header.Set("x-userid", uid)
	}

	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, maxInt(account.Concurrency, 1))
	if err != nil {
		return nil, model, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	snapshot := xai.ObserveQuotaHeaders(resp.Header, resp.StatusCode, "active_probe")
	// 402 free on api.x.ai still useful as plan signal even without headers.
	if snapshot == nil {
		snapshot = &xai.QuotaSnapshot{
			StatusCode:        resp.StatusCode,
			ObservationSource: "active_probe",
			UpdatedAt:         time.Now().UTC().Format(time.RFC3339),
		}
	}
	return snapshot, model, resp.StatusCode, nil
}

func (s *GrokQuotaService) ResetQuota(ctx context.Context, accountID int64) (*GrokQuotaResetResult, error) {
	if _, err := s.loadGrokOAuthAccount(ctx, accountID); err != nil {
		return nil, err
	}
	return nil, infraerrors.New(http.StatusNotImplemented, "GROK_QUOTA_RESET_UNSUPPORTED", "xAI does not expose a Grok subscription quota reset endpoint for OAuth accounts")
}

func (s *GrokQuotaService) prepareProbe(ctx context.Context, accountID int64) (*Account, string, string, error) {
	if s == nil || s.tokenProvider == nil || s.httpUpstream == nil {
		return nil, "", "", infraerrors.New(http.StatusInternalServerError, "GROK_QUOTA_NOT_CONFIGURED", "grok quota service is not configured")
	}
	account, err := s.loadGrokOAuthAccount(ctx, accountID)
	if err != nil {
		return nil, "", "", err
	}

	token, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, "", "", infraerrors.Newf(http.StatusBadGateway, "GROK_QUOTA_TOKEN_UNAVAILABLE", "failed to acquire access token: %v", err)
	}
	if strings.TrimSpace(token) == "" {
		return nil, "", "", infraerrors.New(http.StatusBadGateway, "GROK_QUOTA_TOKEN_UNAVAILABLE", "access token is empty")
	}

	return account, token, s.resolveProxyURL(ctx, account), nil
}

func (s *GrokQuotaService) resolveProxyURL(ctx context.Context, account *Account) string {
	if account == nil || account.ProxyID == nil {
		return ""
	}
	switch {
	case account.Proxy != nil:
		return account.Proxy.URL()
	case s != nil && s.proxyRepo != nil:
		if proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID); err == nil && proxy != nil {
			return proxy.URL()
		}
	}
	return ""
}

func (s *GrokQuotaService) loadGrokOAuthAccount(ctx context.Context, accountID int64) (*Account, error) {
	if s == nil || s.accountRepo == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "GROK_QUOTA_NOT_CONFIGURED", "grok quota service is not configured")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusNotFound, "GROK_QUOTA_ACCOUNT_NOT_FOUND", "account not found: %v", err)
	}
	if account == nil {
		return nil, infraerrors.New(http.StatusNotFound, "GROK_QUOTA_ACCOUNT_NOT_FOUND", "account not found")
	}
	if account.Platform != PlatformGrok {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_QUOTA_INVALID_PLATFORM", "account is not a Grok account")
	}
	if account.Type != AccountTypeOAuth {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_QUOTA_INVALID_TYPE", "account is not an OAuth account")
	}
	return account, nil
}

func (s *GrokQuotaService) fetchGrokBilling(
	ctx context.Context,
	account *Account,
	accessToken, proxyURL, userID string,
) (*xai.BillingSummary, error) {
	type result struct {
		summary *xai.BillingSummary
		err     error
		which   string
	}
	ch := make(chan result, 2)
	var wg sync.WaitGroup
	for _, item := range []struct {
		url   string
		which string
	}{
		{xai.BillingWeeklyURL, "weekly"},
		{xai.BillingMonthlyURL, "monthly"},
	} {
		wg.Add(1)
		go func(url, which string) {
			defer wg.Done()
			summary, err := s.fetchGrokBillingOnce(ctx, account, accessToken, proxyURL, userID, url)
			ch <- result{summary: summary, err: err, which: which}
		}(item.url, item.which)
	}
	wg.Wait()
	close(ch)

	var weekly, monthly *xai.BillingSummary
	var weeklyErr, monthlyErr error
	for r := range ch {
		switch r.which {
		case "weekly":
			weekly, weeklyErr = r.summary, r.err
		case "monthly":
			monthly, monthlyErr = r.summary, r.err
		}
	}
	merged := xai.MergeBillingSummaries(weekly, monthly)
	if merged == nil || !merged.HasData() {
		if weeklyErr != nil && monthlyErr != nil {
			return nil, fmt.Errorf("weekly: %v; monthly: %v", weeklyErr, monthlyErr)
		}
		if weeklyErr != nil {
			return nil, weeklyErr
		}
		if monthlyErr != nil {
			return nil, monthlyErr
		}
		return nil, fmt.Errorf("empty billing data")
	}
	return merged, nil
}

func (s *GrokQuotaService) fetchGrokBillingOnce(
	ctx context.Context,
	account *Account,
	accessToken, proxyURL, userID, url string,
) (*xai.BillingSummary, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "*/*")
	applyGrokCLIHeaders(req.Header)
	if uid := strings.TrimSpace(userID); uid != "" {
		req.Header.Set("x-userid", uid)
	}

	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, maxInt(account.Concurrency, 1))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, msg)
	}
	return xai.ParseBillingBody(body)
}

func resolveGrokBillingUserID(account *Account) string {
	if account == nil {
		return ""
	}
	if idToken := strings.TrimSpace(account.GetCredential("id_token")); idToken != "" {
		if sub := xai.SubjectFromIDToken(idToken); sub != "" {
			return sub
		}
	}
	if email := strings.TrimSpace(account.GetCredential("email")); email != "" {
		return email
	}
	return strings.TrimSpace(account.Name)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
