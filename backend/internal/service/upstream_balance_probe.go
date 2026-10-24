package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/tidwall/gjson"
)

const (
	BalanceProbeSourceCredentialKey = "balance_probe_source"
	BalanceProbeSourceSub2API       = "sub2api"
	BalanceProbeSourceNewAPI        = "newapi"

	upstreamBalanceExtraBalance   = "upstream_balance"
	upstreamBalanceExtraCurrency  = "upstream_balance_currency"
	upstreamBalanceExtraSource    = "upstream_balance_source"
	upstreamBalanceExtraAvailable = "upstream_balance_available"
	upstreamBalanceExtraUpdated   = "upstream_balance_updated_at"

	upstreamBalanceRequestTimeout = 15 * time.Second
	upstreamBalanceMaxBodyBytes   = 256 * 1024
)

// UpstreamBalanceProbeResult is the admin UI payload for Sub2API/NewAPI balance probes.
type UpstreamBalanceProbeResult struct {
	Source     string  `json:"source"`
	Success    bool    `json:"success"`
	Balance    float64 `json:"balance"`
	Currency   string  `json:"currency,omitempty"`
	Available  bool    `json:"available"`
	StatusCode int     `json:"status_code,omitempty"`
	FetchedAt  int64   `json:"fetched_at"`
	Persisted  bool    `json:"persisted"`
	Error      string  `json:"error,omitempty"`
}

// NormalizeBalanceProbeSource returns a supported source or empty when disabled/invalid.
func NormalizeBalanceProbeSource(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case BalanceProbeSourceSub2API:
		return BalanceProbeSourceSub2API
	case BalanceProbeSourceNewAPI:
		return BalanceProbeSourceNewAPI
	default:
		return ""
	}
}

func AccountBalanceProbeSource(account *Account) string {
	if account == nil {
		return ""
	}
	return NormalizeBalanceProbeSource(account.GetCredential(BalanceProbeSourceCredentialKey))
}

// ProbeUpstreamBalance queries wallet/token balance from the configured upstream system.
func (s *UpstreamBillingProbeService) ProbeUpstreamBalance(ctx context.Context, accountID int64) (*UpstreamBalanceProbeResult, error) {
	if s == nil || s.accountRepo == nil || s.accountTestService == nil || s.accountTestService.httpUpstream == nil {
		return nil, infraerrors.New(http.StatusServiceUnavailable, "UPSTREAM_BALANCE_UNAVAILABLE", "upstream balance probe is unavailable")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil {
		return nil, infraerrors.New(http.StatusNotFound, "ACCOUNT_NOT_FOUND", "account not found")
	}
	if account.Type != AccountTypeAPIKey {
		return nil, infraerrors.New(http.StatusBadRequest, "UPSTREAM_BALANCE_UNSUPPORTED", "only API key accounts support upstream balance probe")
	}
	source := AccountBalanceProbeSource(account)
	if source == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "UPSTREAM_BALANCE_SOURCE_REQUIRED", "set balance_probe_source to sub2api or newapi")
	}

	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(account.GetCNAPIKey())
	}
	if apiKey == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "UPSTREAM_BALANCE_NO_APIKEY", "account api_key is empty")
	}

	baseURL := strings.TrimSpace(account.GetCredential("base_url"))
	if baseURL == "" {
		baseURL = strings.TrimSpace(account.GetOpenAIBaseURL())
	}
	if baseURL == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "UPSTREAM_BALANCE_NO_BASE_URL", "account base_url is empty")
	}

	proxyURL := ""
	if account.ProxyID != nil {
		if account.Proxy != nil {
			proxyURL = account.Proxy.URL()
		}
	}

	var targetURL string
	switch source {
	case BalanceProbeSourceNewAPI:
		targetURL = buildNewAPISiteAPIURL(baseURL, newAPITokenUsagePath)
	case BalanceProbeSourceSub2API:
		targetURL = buildOpenAIEndpointURL(strings.TrimRight(baseURL, "/"), "/v1/usage")
	default:
		return nil, infraerrors.New(http.StatusBadRequest, "UPSTREAM_BALANCE_SOURCE_INVALID", "unsupported balance_probe_source")
	}

	statusCode, body, err := s.doUpstreamBalanceGET(ctx, account, proxyURL, targetURL, apiKey)
	now := time.Now().UTC()
	result := &UpstreamBalanceProbeResult{
		Source:     source,
		FetchedAt:  now.Unix(),
		StatusCode: statusCode,
		Available:  true,
	}
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}
	if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
		result.Error = fmt.Sprintf("Authentication failed (HTTP %d)", statusCode)
		return result, nil
	}
	if statusCode < 200 || statusCode >= 300 {
		result.Error = fmt.Sprintf("API error (HTTP %d): %s", statusCode, truncate(strings.TrimSpace(string(body)), 240))
		return result, nil
	}

	balance, currency, available, ok := parseUpstreamBalanceBody(source, body)
	if !ok {
		result.Error = "Invalid balance response"
		return result, nil
	}
	result.Balance = balance
	result.Currency = currency
	result.Available = available
	result.Success = true

	updates := map[string]any{
		upstreamBalanceExtraBalance:   balance,
		upstreamBalanceExtraCurrency:  currency,
		upstreamBalanceExtraSource:    source,
		upstreamBalanceExtraAvailable: available,
		upstreamBalanceExtraUpdated:   now.Format(time.RFC3339),
	}
	if err := s.accountRepo.UpdateExtra(ctx, account.ID, updates); err != nil {
		slog.Warn("upstream_balance_persist_failed", "account_id", account.ID, "source", source, "error", err)
	} else {
		result.Persisted = true
	}
	if s.balanceNotify != nil {
		s.balanceNotify.CheckAccountBalanceLow(ctx, account, balance, currency)
	}
	return result, nil
}

func (s *UpstreamBillingProbeService) doUpstreamBalanceGET(
	ctx context.Context,
	account *Account,
	proxyURL string,
	targetURL string,
	apiKey string,
) (int, []byte, error) {
	probeCtx, cancel := context.WithTimeout(ctx, upstreamBalanceRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, targetURL, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("request_build_failed")
	}
	reqCtx := WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileDefault)
	req = req.WithContext(WithHTTPUpstreamRedirectsDisabled(reqCtx))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("User-Agent", "Sub2API-UpstreamBalanceProbe/1.0")
	account.ApplyHeaderOverrides(req.Header)

	var tlsProfile *tlsfingerprint.Profile
	if s.accountTestService.tlsFPProfileService != nil {
		tlsProfile = s.accountTestService.tlsFPProfileService.ResolveTLSProfile(account)
	}
	resp, err := s.accountTestService.httpUpstream.DoWithTLS(req, proxyURL, account.ID, maxInt(account.Concurrency, 1), tlsProfile)
	if err != nil {
		return 0, nil, fmt.Errorf("request_failed")
	}
	if resp == nil || resp.Body == nil {
		return 0, nil, fmt.Errorf("empty_response")
	}
	defer func() { _ = resp.Body.Close() }()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, upstreamBalanceMaxBodyBytes+1))
	if readErr != nil {
		return resp.StatusCode, nil, fmt.Errorf("response_read_failed")
	}
	if len(body) > upstreamBalanceMaxBodyBytes {
		return resp.StatusCode, nil, fmt.Errorf("response_too_large")
	}
	return resp.StatusCode, body, nil
}

func parseUpstreamBalanceBody(source string, body []byte) (balance float64, currency string, available bool, ok bool) {
	switch source {
	case BalanceProbeSourceNewAPI:
		entries, avail, parsed := parseNewAPITokenUsageBalance(body)
		if !parsed || len(entries) == 0 {
			return 0, "", false, false
		}
		return entries[0].Balance, entries[0].Currency, avail, true
	case BalanceProbeSourceSub2API:
		return parseSub2APIUsageBalance(body)
	default:
		return 0, "", false, false
	}
}

func parseSub2APIUsageBalance(body []byte) (float64, string, bool, bool) {
	// Prefer top-level remaining/balance (wallet or quota remaining).
	unit := strings.ToUpper(strings.TrimSpace(gjson.GetBytes(body, "unit").String()))
	if unit == "" {
		unit = "USD"
	}
	if rem := gjson.GetBytes(body, "remaining"); rem.Exists() {
		if balance, ok := cnParseF64(rem.Value()); ok {
			return balance, unit, true, true
		}
	}
	if bal := gjson.GetBytes(body, "balance"); bal.Exists() {
		if balance, ok := cnParseF64(bal.Value()); ok {
			return balance, unit, true, true
		}
	}
	if rem := gjson.GetBytes(body, "quota.remaining"); rem.Exists() {
		if balance, ok := cnParseF64(rem.Value()); ok {
			qUnit := strings.ToUpper(strings.TrimSpace(gjson.GetBytes(body, "quota.unit").String()))
			if qUnit == "" {
				qUnit = unit
			}
			return balance, qUnit, true, true
		}
	}
	return 0, "", false, false
}
