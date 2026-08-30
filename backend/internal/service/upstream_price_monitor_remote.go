package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

const upstreamPriceMonitorMaxResponseBytes = 1 << 20

type x5m5xUpstreamPriceRemoteFetcher struct {
	transport *AccountTestService
	now       func() time.Time
	location  *time.Location
}

// NewX5M5XUpstreamPriceRemoteFetcher builds a key-only remote reader using the
// same proxy, header override, TLS profile, and concurrency transport as normal
// production account traffic.
func NewX5M5XUpstreamPriceRemoteFetcher(accountTestService *AccountTestService) UpstreamPriceRemoteFetcher {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	return &x5m5xUpstreamPriceRemoteFetcher{transport: accountTestService, now: time.Now, location: location}
}

func (f *x5m5xUpstreamPriceRemoteFetcher) FetchUsage(ctx context.Context, account *Account) (*domain.UpstreamPriceRemoteUsageSnapshot, error) {
	if f == nil || f.transport == nil || f.transport.httpUpstream == nil {
		return nil, ErrUpstreamPriceMonitorUnavailable
	}
	capturedAt := f.now().UTC()
	ledgerDate := capturedAt.In(f.location).Format("2006-01-02")
	query := url.Values{"start_date": {ledgerDate}, "end_date": {ledgerDate}}
	body, err := f.get(ctx, account, "/v1/usage", query)
	if err != nil {
		return nil, err
	}
	models, err := parseX5M5XUsageModelStats(body)
	if err != nil {
		return nil, err
	}
	return &domain.UpstreamPriceRemoteUsageSnapshot{
		AccountID: account.ID, LedgerDate: ledgerDate, CapturedAt: capturedAt, Models: models,
	}, nil
}

func (f *x5m5xUpstreamPriceRemoteFetcher) FetchBilling(ctx context.Context, account *Account) (*domain.UpstreamPriceBillingSnapshot, error) {
	if f == nil || f.transport == nil || f.transport.httpUpstream == nil {
		return nil, ErrUpstreamPriceMonitorUnavailable
	}
	body, err := f.get(ctx, account, "/v1/sub2api/billing", nil)
	if err != nil {
		return nil, err
	}
	data, err := parseUpstreamBillingProbeResponse(body)
	if err != nil {
		return nil, fmt.Errorf("parse x5m5x billing context: %w", err)
	}
	resolved, ok := resolveAccountExtraNumber(data, "resolved_rate_multiplier")
	if !ok {
		return nil, errors.New("x5m5x billing response has no resolved multiplier")
	}
	effective, ok := resolveAccountExtraNumber(data, "effective_rate_multiplier")
	if !ok {
		return nil, errors.New("x5m5x billing response has no effective multiplier")
	}
	observedRaw, _ := data["observed_at"].(string)
	observedAt, err := time.Parse(time.RFC3339Nano, observedRaw)
	if err != nil {
		return nil, errors.New("x5m5x billing response has invalid observed_at")
	}
	peakEnabled, _ := data["peak_rate_enabled"].(bool)
	appliedPeak := 1.0
	if value, found := resolveAccountExtraNumber(data, "applied_peak_multiplier"); found {
		appliedPeak = value
	}
	out := &domain.UpstreamPriceBillingSnapshot{
		ResolvedRateMultiplier:  resolved,
		EffectiveRateMultiplier: effective,
		PeakRateEnabled:         peakEnabled,
		AppliedPeakMultiplier:   appliedPeak,
		ObservedAt:              observedAt.UTC(),
	}
	if peakEnabled {
		out.PeakStart, _ = data["peak_start"].(string)
		out.PeakEnd, _ = data["peak_end"].(string)
		out.Timezone, _ = data["timezone"].(string)
		if value, found := resolveAccountExtraNumber(data, "peak_rate_multiplier"); found {
			out.PeakRateMultiplier = &value
		}
	}
	return out, nil
}

func (f *x5m5xUpstreamPriceRemoteFetcher) FetchModels(ctx context.Context, account *Account) ([]string, error) {
	body, err := f.get(ctx, account, "/v1/models", nil)
	if err != nil {
		return nil, err
	}
	models, err := parseX5M5XModelList(body)
	if err == nil && len(models) == 0 {
		return nil, errors.New("x5m5x model list is empty")
	}
	return models, err
}

func parseX5M5XModelList(body []byte) ([]string, error) {
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse x5m5x model list: %w", err)
	}
	rows := payload
	if object, ok := payload.(map[string]any); ok {
		rows = object["data"]
	}
	items, ok := rows.([]any)
	if !ok {
		return nil, errors.New("x5m5x model list has an unexpected shape")
	}
	seen := make(map[string]string, len(items))
	for index, item := range items {
		name := ""
		switch value := item.(type) {
		case string:
			name = value
		case map[string]any:
			name, _ = value["id"].(string)
		default:
			return nil, fmt.Errorf("x5m5x model list row %d is invalid", index)
		}
		name = strings.TrimSpace(name)
		if name == "" || len(name) > 255 || strings.ContainsAny(name, " \t\r\n,") {
			return nil, fmt.Errorf("x5m5x model list row %d has an invalid id", index)
		}
		key := strings.ToLower(name)
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("x5m5x model list contains duplicate id %q", name)
		}
		seen[key] = name
	}
	models := make([]string, 0, len(seen))
	for _, model := range seen {
		models = append(models, model)
	}
	sort.Slice(models, func(i, j int) bool { return strings.ToLower(models[i]) < strings.ToLower(models[j]) })
	return models, nil
}

func (f *x5m5xUpstreamPriceRemoteFetcher) get(ctx context.Context, account *Account, endpoint string, query url.Values) ([]byte, error) {
	if account == nil || account.Type != AccountTypeAPIKey {
		return nil, errors.New("x5m5x price monitor requires an API-key account")
	}
	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	if apiKey == "" {
		return nil, errors.New("x5m5x price monitor account has no API key")
	}
	baseURL := strings.TrimSpace(account.GetOpenAIBaseURL())
	if baseURL == "" {
		baseURL = strings.TrimSpace(account.GetCredential("base_url"))
	}
	normalizedBaseURL, err := f.transport.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, errors.New("x5m5x price monitor account has invalid base URL")
	}
	target := buildOpenAIEndpointURL(normalizedBaseURL, endpoint)
	parsed, err := url.Parse(target)
	if err != nil {
		return nil, errors.New("build x5m5x price monitor request URL")
	}
	if len(query) > 0 {
		parsed.RawQuery = query.Encode()
	}
	requestCtx, cancel := context.WithTimeout(ctx, upstreamBillingProbeRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, parsed.String(), bytes.NewReader(nil))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	account.ApplyHeaderOverrides(req.Header)
	req = req.WithContext(WithHTTPUpstreamRedirectsDisabled(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI)))
	proxyURL := ""
	if account.ProxyID != nil {
		if account.Proxy == nil || account.Proxy.ID != *account.ProxyID {
			return nil, errors.New("x5m5x price monitor account proxy is unavailable")
		}
		proxyURL = account.Proxy.URL()
	}
	var tlsProfile *tlsfingerprint.Profile
	if f.transport.tlsFPProfileService != nil {
		tlsProfile = f.transport.tlsFPProfileService.ResolveTLSProfile(account)
	}
	resp, err := f.transport.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, tlsProfile)
	if err != nil {
		return nil, fmt.Errorf("x5m5x price monitor request failed: %w", err)
	}
	if resp == nil || resp.Body == nil {
		return nil, errors.New("x5m5x price monitor received an empty response")
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, upstreamPriceMonitorMaxResponseBytes+1))
	if err != nil {
		return nil, errors.New("read x5m5x price monitor response")
	}
	if len(body) > upstreamPriceMonitorMaxResponseBytes {
		return nil, errors.New("x5m5x price monitor response is too large")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("x5m5x price monitor HTTP %d", resp.StatusCode)
	}
	return body, nil
}

func parseX5M5XUsageModelStats(body []byte) (map[string]domain.UpstreamPriceUsageCounters, error) {
	type usageRow struct {
		Model               string      `json:"model"`
		Requests            json.Number `json:"requests"`
		InputTokens         json.Number `json:"input_tokens"`
		OutputTokens        json.Number `json:"output_tokens"`
		CacheCreationTokens json.Number `json:"cache_creation_tokens"`
		CacheReadTokens     json.Number `json:"cache_read_tokens"`
		ActualCost          json.Number `json:"actual_cost"`
	}
	var payload struct {
		ModelStats []usageRow `json:"model_stats"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("parse x5m5x usage response: %w", err)
	}
	out := make(map[string]domain.UpstreamPriceUsageCounters, len(payload.ModelStats))
	for _, row := range payload.ModelStats {
		model := strings.TrimSpace(row.Model)
		if model == "" {
			return nil, errors.New("x5m5x usage response contains an empty model")
		}
		key := strings.ToLower(model)
		if _, duplicate := out[key]; duplicate {
			return nil, fmt.Errorf("x5m5x usage response contains duplicate model %q", model)
		}
		requests, err := nonnegativeJSONInt64(row.Requests, "requests")
		if err != nil {
			return nil, err
		}
		input, err := nonnegativeJSONInt64(row.InputTokens, "input_tokens")
		if err != nil {
			return nil, err
		}
		output, err := nonnegativeJSONInt64(row.OutputTokens, "output_tokens")
		if err != nil {
			return nil, err
		}
		cacheCreation, err := nonnegativeJSONInt64(row.CacheCreationTokens, "cache_creation_tokens")
		if err != nil {
			return nil, err
		}
		cacheRead, err := nonnegativeJSONInt64(row.CacheReadTokens, "cache_read_tokens")
		if err != nil {
			return nil, err
		}
		cost := 0.0
		if row.ActualCost != "" {
			cost, err = strconv.ParseFloat(row.ActualCost.String(), 64)
			if err != nil || cost < 0 || math.IsNaN(cost) || math.IsInf(cost, 0) {
				return nil, errors.New("x5m5x usage response contains invalid actual_cost")
			}
		}
		out[key] = domain.UpstreamPriceUsageCounters{
			Requests: requests, InputTokens: input, OutputTokens: output,
			CacheCreationTokens: cacheCreation, CacheReadTokens: cacheRead, ActualCost: cost,
		}
	}
	return out, nil
}

func nonnegativeJSONInt64(value json.Number, field string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value.String(), 10, 64)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("x5m5x usage response contains invalid %s", field)
	}
	return parsed, nil
}
