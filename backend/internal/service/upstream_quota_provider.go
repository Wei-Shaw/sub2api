package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

const (
	UpstreamQuotaProviderExtraKey = "upstream_quota_provider"
	UpstreamQuotaSyncedAtExtraKey = "upstream_quota_synced_at"
	UpstreamQuotaManual           = "manual"
	UpstreamQuotaSub2API          = "sub2api"
	UpstreamQuotaNewAPI           = "new-api"

	upstreamQuotaTimeout      = 10 * time.Second
	upstreamQuotaMaxBodyBytes = 64 * 1024
	newAPIQuotaPerUSD         = 500000.0
)

// UpstreamQuota is the provider-neutral representation persisted into the
// existing account quota fields. Keeping this model small lets new providers
// be added without changing quota enforcement or the admin UI.
type UpstreamQuota struct {
	Limit float64 `json:"limit"`
	Used  float64 `json:"used"`
}

// UpstreamQuotaProvider implements one upstream quota protocol (Strategy pattern).
type UpstreamQuotaProvider interface {
	Name() string
	Path() string
	Parse([]byte) (*UpstreamQuota, error)
}

type upstreamQuotaProviderRegistry struct {
	providers map[string]UpstreamQuotaProvider
}

func newUpstreamQuotaProviderRegistry() *upstreamQuotaProviderRegistry {
	providers := []UpstreamQuotaProvider{sub2APIQuotaProvider{}, newAPIQuotaProvider{}}
	registry := &upstreamQuotaProviderRegistry{providers: make(map[string]UpstreamQuotaProvider, len(providers))}
	for _, provider := range providers {
		registry.providers[provider.Name()] = provider
	}
	return registry
}

func (r *upstreamQuotaProviderRegistry) Get(name string) (UpstreamQuotaProvider, bool) {
	provider, ok := r.providers[strings.ToLower(strings.TrimSpace(name))]
	return provider, ok
}

type sub2APIQuotaProvider struct{}

func (sub2APIQuotaProvider) Name() string { return UpstreamQuotaSub2API }
func (sub2APIQuotaProvider) Path() string { return "/v1/usage" }
func (sub2APIQuotaProvider) Parse(body []byte) (*UpstreamQuota, error) {
	var response struct {
		Quota *struct {
			Limit float64 `json:"limit"`
			Used  float64 `json:"used"`
		} `json:"quota"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode sub2api quota: %w", err)
	}
	if response.Quota == nil {
		return nil, fmt.Errorf("sub2api response does not contain a limited API key quota")
	}
	return validateUpstreamQuota(response.Quota.Limit, response.Quota.Used)
}

type newAPIQuotaProvider struct{}

func (newAPIQuotaProvider) Name() string { return UpstreamQuotaNewAPI }
func (newAPIQuotaProvider) Path() string { return "/api/usage/token" }
func (newAPIQuotaProvider) Parse(body []byte) (*UpstreamQuota, error) {
	var response struct {
		Code any `json:"code"`
		Data *struct {
			TotalGranted float64 `json:"total_granted"`
			TotalUsed    float64 `json:"total_used"`
			Unlimited    bool    `json:"unlimited_quota"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode new-api quota: %w", err)
	}
	if response.Data == nil || response.Data.Unlimited {
		return nil, fmt.Errorf("new-api response does not contain a limited token quota")
	}
	return validateUpstreamQuota(response.Data.TotalGranted/newAPIQuotaPerUSD, response.Data.TotalUsed/newAPIQuotaPerUSD)
}

func validateUpstreamQuota(limit, used float64) (*UpstreamQuota, error) {
	if limit <= 0 || used < 0 || math.IsNaN(limit) || math.IsNaN(used) || math.IsInf(limit, 0) || math.IsInf(used, 0) {
		return nil, fmt.Errorf("invalid upstream quota values")
	}
	return &UpstreamQuota{Limit: limit, Used: used}, nil
}

func ValidateUpstreamQuotaProviderConfig(accountType string, extra map[string]any) error {
	if extra == nil {
		return nil
	}
	raw, exists := extra[UpstreamQuotaProviderExtraKey]
	if !exists {
		return nil
	}
	name, ok := raw.(string)
	if !ok {
		return fmt.Errorf("%s must be a string", UpstreamQuotaProviderExtraKey)
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || name == UpstreamQuotaManual {
		extra[UpstreamQuotaProviderExtraKey] = UpstreamQuotaManual
		return nil
	}
	if accountType != AccountTypeAPIKey {
		return fmt.Errorf("upstream quota providers are only supported for API key accounts")
	}
	if _, ok := newUpstreamQuotaProviderRegistry().Get(name); !ok {
		return fmt.Errorf("unsupported upstream quota provider %q", name)
	}
	if parseExtraFloat64(extra["quota_limit"]) <= 0 {
		return fmt.Errorf("quota_limit must be enabled before selecting an upstream quota provider")
	}
	extra[UpstreamQuotaProviderExtraKey] = name
	return nil
}

// RefreshUpstreamQuota fetches and persists a configured API-key account quota.
// Manual accounts deliberately bypass all network work and retain existing values.
func (s *AccountTestService) RefreshUpstreamQuota(ctx context.Context, accountID int64) (*Account, error) {
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if account.Type != AccountTypeAPIKey {
		return nil, fmt.Errorf("upstream quota is only supported for API key accounts")
	}
	providerName := account.getExtraString(UpstreamQuotaProviderExtraKey)
	if providerName == "" || providerName == UpstreamQuotaManual {
		return account, nil
	}
	provider, ok := newUpstreamQuotaProviderRegistry().Get(providerName)
	if !ok {
		return nil, fmt.Errorf("unsupported upstream quota provider %q", providerName)
	}
	if s.httpUpstream == nil {
		return nil, fmt.Errorf("upstream transport is unavailable")
	}
	apiKey := account.GetCredential("api_key")
	if apiKey == "" {
		return nil, fmt.Errorf("API key is missing")
	}
	baseURL := account.GetCredential("base_url")
	if baseURL == "" {
		return nil, fmt.Errorf("base URL is required for upstream quota providers")
	}
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, err
	}

	requestCtx, cancel := context.WithTimeout(ctx, upstreamQuotaTimeout)
	defer cancel()
	requestURL := buildOpenAIEndpointURL(normalizedBaseURL, provider.Path())
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, requestURL, bytes.NewReader(nil))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	account.ApplyHeaderOverrides(req.Header)
	req = req.WithContext(WithHTTPUpstreamRedirectsDisabled(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI)))

	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	var resp *http.Response
	if s.tlsFPProfileService != nil {
		resp, err = s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	} else {
		resp, err = s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	}
	if err != nil {
		return nil, fmt.Errorf("query %s quota: %w", provider.Name(), err)
	}
	if resp == nil || resp.Body == nil {
		return nil, fmt.Errorf("query %s quota: empty response", provider.Name())
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, upstreamQuotaMaxBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s quota: %w", provider.Name(), err)
	}
	if len(body) > upstreamQuotaMaxBodyBytes {
		return nil, fmt.Errorf("%s quota response is too large", provider.Name())
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s quota request returned HTTP %d", provider.Name(), resp.StatusCode)
	}
	quota, err := provider.Parse(body)
	if err != nil {
		return nil, err
	}
	if err := s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{
		"quota_limit":                 quota.Limit,
		"quota_used":                  quota.Used,
		UpstreamQuotaSyncedAtExtraKey: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		return nil, err
	}
	return s.accountRepo.GetByID(ctx, account.ID)
}
