package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

const (
	upstreamGroupCredentialKey          = "upstream_group"
	upstreamAccessTokenCredentialKey    = "upstream_access_token"
	upstreamRateMultiplierCredentialKey = "upstream_rate_multiplier"
	newAPIPricingPath                   = "/api/pricing"
	newAPIUserSelfPath                  = "/api/user/self"
	newAPIUserGroupsPath                = "/api/user/self/groups"
	newAPITokenListPath                 = "/api/token/"
	newAPITokenUsagePath                = "/api/usage/token/"
)

var (
	errNewAPIPricingUnsupported   = errors.New("upstream is not a new-api pricing endpoint")
	errNewAPIUpstreamGroupMissing = errors.New("new-api upstream group is not configured or not found")
)

type newAPIPricingResponse struct {
	Success    bool               `json:"success"`
	GroupRatio map[string]float64 `json:"group_ratio"`
}

type newAPIUserSelfResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Group string `json:"group"`
	} `json:"data"`
}

// buildNewAPISiteAPIURL maps an OpenAI-style base (often .../v1) to a NewAPI
// site-root path such as /api/pricing. buildOpenAIEndpointURL would incorrectly
// produce .../v1/api/... when the base already ends with a version segment.
func buildNewAPISiteAPIURL(normalizedBaseURL, apiPath string) string {
	raw := strings.TrimSpace(normalizedBaseURL)
	apiPath = "/" + strings.TrimLeft(strings.TrimSpace(apiPath), "/")
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		base := strings.TrimRight(raw, "/")
		if openAIBaseURLHasVersionSuffix(base) {
			if idx := strings.LastIndex(base, "/"); idx >= 0 {
				base = base[:idx]
			}
		}
		return strings.TrimRight(base, "/") + apiPath
	}
	path := strings.TrimRight(parsed.Path, "/")
	if openAIBaseURLHasVersionSuffix(path) {
		if idx := strings.LastIndex(path, "/"); idx >= 0 {
			path = path[:idx]
		} else {
			path = ""
		}
	}
	parsed.Path = path + apiPath
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func buildNewAPIPricingURL(normalizedBaseURL string) string {
	return buildNewAPISiteAPIURL(normalizedBaseURL, newAPIPricingPath)
}

// probeNewAPIPricing resolves the account's NewAPI billing multiplier.
//
// Priority:
//  1. credentials.upstream_rate_multiplier (explicit override)
//  2. NewAPI dashboard access token → /api/user/self/groups (user-effective ratios,
//     including group_group_ratio overrides shown in the console)
//  3. Authenticated /api/pricing (same overrides applied server-side)
//  4. Public /api/pricing group_ratio (group defaults; sk- cannot see personal rates)
//
// Group selection for (2)-(4): credentials.upstream_group, then /api/user/self
// group (when access token present), then account.Platform.
func (s *UpstreamBillingProbeService) probeNewAPIPricing(
	ctx context.Context,
	account *Account,
	normalizedBaseURL string,
	proxyURL string,
	now time.Time,
) (map[string]any, int, error) {
	if s == nil || s.accountTestService == nil || s.accountTestService.httpUpstream == nil || account == nil {
		return nil, 0, errNewAPIPricingUnsupported
	}

	if rate, ok := parseUpstreamRateMultiplierCredential(account); ok {
		groupName := strings.TrimSpace(account.GetCredential(upstreamGroupCredentialKey))
		if groupName == "" {
			groupName = strings.TrimSpace(account.Platform)
		}
		if groupName == "" {
			groupName = "custom"
		}
		return buildNewAPIBillingProbeData(groupName, rate, "newapi.credential", now), http.StatusOK, nil
	}

	accessToken := strings.TrimSpace(account.GetCredential(upstreamAccessTokenCredentialKey))
	var userGroup string
	var tokenGroup string
	if accessToken != "" {
		if ug, _, selfErr := s.fetchNewAPIUserSelfGroup(ctx, account, normalizedBaseURL, proxyURL, accessToken); selfErr == nil {
			userGroup = ug
		}
		if tg, _, tgErr := s.fetchNewAPITokenGroup(ctx, account, normalizedBaseURL, proxyURL, accessToken); tgErr == nil {
			tokenGroup = tg
		}
		if groups, status, err := s.fetchNewAPIUserGroups(ctx, account, normalizedBaseURL, proxyURL, accessToken); err == nil {
			groupName, rate, ok := resolveNewAPIUpstreamGroup(account, groups, userGroupCandidates(account, tokenGroup, userGroup)...)
			if ok {
				return buildNewAPIBillingProbeData(groupName, rate, "newapi.user_groups", now), status, nil
			}
			return nil, status, errNewAPIUpstreamGroupMissing
		}
		// Fall through to authenticated /api/pricing when self/groups is unavailable.
	}

	pricingURL := buildNewAPIPricingURL(normalizedBaseURL)
	status, body, err := s.doNewAPIJSONGet(ctx, account, proxyURL, pricingURL, accessToken)
	if err != nil {
		return nil, status, err
	}
	data, err := parseNewAPIPricingProbeResponse(body, account, now, tokenGroup, userGroup)
	if err != nil {
		if errors.Is(err, errNewAPIPricingUnsupported) || errors.Is(err, errNewAPIUpstreamGroupMissing) {
			return nil, status, err
		}
		return nil, status, fmt.Errorf("invalid_response")
	}
	return data, status, nil
}

// fetchNewAPITokenGroup resolves the NewAPI token's billing group for this
// account's sk- key: usage/token (sk-) → token name, then /api/token/ (access
// token) → matching token.group. Token names like "cs" are not group names.
func (s *UpstreamBillingProbeService) fetchNewAPITokenGroup(
	ctx context.Context,
	account *Account,
	normalizedBaseURL string,
	proxyURL string,
	accessToken string,
) (string, int, error) {
	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(account.GetCNAPIKey())
	}
	if apiKey == "" {
		return "", 0, errNewAPIUpstreamGroupMissing
	}
	usageStatus, usageBody, usageErr := s.doNewAPIJSONGet(ctx, account, proxyURL, buildNewAPISiteAPIURL(normalizedBaseURL, newAPITokenUsagePath), apiKey)
	if usageErr != nil {
		return "", usageStatus, usageErr
	}
	tokenName := parseNewAPITokenUsageName(usageBody)
	if tokenName == "" {
		return "", usageStatus, errNewAPIUpstreamGroupMissing
	}
	listStatus, listBody, listErr := s.doNewAPIJSONGet(ctx, account, proxyURL, buildNewAPISiteAPIURL(normalizedBaseURL, newAPITokenListPath), accessToken)
	if listErr != nil {
		return "", listStatus, listErr
	}
	group, ok := matchNewAPITokenGroup(listBody, tokenName, apiKey)
	if !ok {
		return "", listStatus, errNewAPIUpstreamGroupMissing
	}
	return group, listStatus, nil
}

func (s *UpstreamBillingProbeService) fetchNewAPIUserSelfGroup(
	ctx context.Context,
	account *Account,
	normalizedBaseURL string,
	proxyURL string,
	accessToken string,
) (string, int, error) {
	status, body, err := s.doNewAPIJSONGet(ctx, account, proxyURL, buildNewAPISiteAPIURL(normalizedBaseURL, newAPIUserSelfPath), accessToken)
	if err != nil {
		return "", status, err
	}
	var payload newAPIUserSelfResponse
	if json.Unmarshal(body, &payload) != nil || !payload.Success {
		return "", status, errNewAPIPricingUnsupported
	}
	group := strings.TrimSpace(payload.Data.Group)
	if group == "" {
		return "", status, errNewAPIUpstreamGroupMissing
	}
	return group, status, nil
}

func (s *UpstreamBillingProbeService) fetchNewAPIUserGroups(
	ctx context.Context,
	account *Account,
	normalizedBaseURL string,
	proxyURL string,
	accessToken string,
) (map[string]float64, int, error) {
	status, body, err := s.doNewAPIJSONGet(ctx, account, proxyURL, buildNewAPISiteAPIURL(normalizedBaseURL, newAPIUserGroupsPath), accessToken)
	if err != nil {
		return nil, status, err
	}
	ratios, ok := parseNewAPIUserGroupsResponse(body)
	if !ok {
		return nil, status, errNewAPIPricingUnsupported
	}
	return ratios, status, nil
}

func (s *UpstreamBillingProbeService) doNewAPIJSONGet(
	ctx context.Context,
	account *Account,
	proxyURL string,
	requestURL string,
	accessToken string,
) (int, []byte, error) {
	probeCtx, cancel := context.WithTimeout(ctx, upstreamBillingProbeRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, requestURL, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("request_build_failed")
	}
	reqCtx := WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileDefault)
	req = req.WithContext(WithHTTPUpstreamRedirectsDisabled(reqCtx))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Sub2API-UpstreamBillingProbe/1.0")
	if token := strings.TrimSpace(accessToken); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	account.ApplyHeaderOverrides(req.Header)

	var tlsProfile *tlsfingerprint.Profile
	if s.accountTestService.tlsFPProfileService != nil {
		tlsProfile = s.accountTestService.tlsFPProfileService.ResolveTLSProfile(account)
	}
	resp, err := s.accountTestService.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, tlsProfile)
	if err != nil {
		return 0, nil, fmt.Errorf("request_failed")
	}
	if resp == nil || resp.Body == nil {
		return 0, nil, fmt.Errorf("empty_response")
	}
	defer func() { _ = resp.Body.Close() }()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, upstreamBillingProbeMaxBodyBytes+1))
	if readErr != nil {
		return resp.StatusCode, nil, fmt.Errorf("response_read_failed")
	}
	if len(body) > upstreamBillingProbeMaxBodyBytes {
		return resp.StatusCode, nil, fmt.Errorf("response_too_large")
	}
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
		return resp.StatusCode, body, errNewAPIPricingUnsupported
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return resp.StatusCode, body, fmt.Errorf("auth_failed")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, body, fmt.Errorf("http_error")
	}
	return resp.StatusCode, body, nil
}

func parseNewAPIUserGroupsResponse(body []byte) (map[string]float64, bool) {
	var payload struct {
		Success bool                      `json:"success"`
		Data    map[string]map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Data == nil || len(payload.Data) == 0 {
		return nil, false
	}
	ratios := make(map[string]float64, len(payload.Data))
	for name, info := range payload.Data {
		if info == nil {
			continue
		}
		rate, ok := coerceJSONFloat(info["ratio"])
		if !ok || rate < 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
			continue
		}
		ratios[name] = rate
	}
	if len(ratios) == 0 {
		return nil, false
	}
	return ratios, true
}

func parseNewAPIPricingProbeResponse(body []byte, account *Account, now time.Time, extraGroups ...string) (map[string]any, error) {
	var payload newAPIPricingResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, errNewAPIPricingUnsupported
	}
	if payload.GroupRatio == nil || len(payload.GroupRatio) == 0 {
		return nil, errNewAPIPricingUnsupported
	}

	groupName, rate, ok := resolveNewAPIUpstreamGroup(account, payload.GroupRatio, userGroupCandidates(account, extraGroups...)...)
	if !ok {
		return nil, errNewAPIUpstreamGroupMissing
	}
	if rate < 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return nil, fmt.Errorf("invalid_response")
	}
	source := "newapi.pricing"
	if account != nil && strings.TrimSpace(account.GetCredential(upstreamAccessTokenCredentialKey)) != "" {
		source = "newapi.pricing.authenticated"
	}
	return buildNewAPIBillingProbeData(groupName, rate, source, now), nil
}

func buildNewAPIBillingProbeData(groupName string, rate float64, source string, now time.Time) map[string]any {
	observedAt := now.UTC().Format(time.RFC3339Nano)
	return map[string]any{
		"object":                    "sub2api.key_billing",
		"schema_version":            1,
		"billing_scope":             "token",
		"billing_source":            source,
		"group_name":                groupName,
		"group_rate_multiplier":     rate,
		"resolved_rate_multiplier":  rate,
		"peak_rate_enabled":         false,
		"effective_rate_multiplier": rate,
		"observed_at":               observedAt,
	}
}

func userGroupCandidates(account *Account, extraGroups ...string) []string {
	candidates := make([]string, 0, 4)
	seen := map[string]struct{}{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		candidates = append(candidates, value)
	}
	if account != nil {
		add(account.GetCredential(upstreamGroupCredentialKey))
	}
	for _, group := range extraGroups {
		add(group)
	}
	if account != nil {
		add(account.Platform)
	}
	return candidates
}

func resolveNewAPIUpstreamGroup(account *Account, ratios map[string]float64, candidates ...string) (string, float64, bool) {
	if len(ratios) == 0 {
		return "", 0, false
	}
	if len(candidates) == 0 {
		candidates = userGroupCandidates(account)
	}
	for _, candidate := range candidates {
		if rate, ok := ratios[candidate]; ok {
			return candidate, rate, true
		}
		for name, rate := range ratios {
			if strings.EqualFold(strings.TrimSpace(name), candidate) {
				return name, rate, true
			}
		}
	}
	return "", 0, false
}

func parseNewAPITokenUsageName(body []byte) string {
	var payload struct {
		Data struct {
			Name   string `json:"name"`
			Object string `json:"object"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return ""
	}
	return strings.TrimSpace(payload.Data.Name)
}

func matchNewAPITokenGroup(body []byte, tokenName, apiKey string) (string, bool) {
	tokens := extractNewAPITokenList(body)
	if len(tokens) == 0 {
		return "", false
	}
	tokenName = strings.TrimSpace(tokenName)
	apiKey = strings.TrimSpace(apiKey)
	maskedNeedle := maskNewAPITokenKey(apiKey)

	for _, token := range tokens {
		name := strings.TrimSpace(asString(token["name"]))
		group := strings.TrimSpace(asString(token["group"]))
		if group == "" {
			continue
		}
		if tokenName != "" && strings.EqualFold(name, tokenName) {
			return group, true
		}
		key := strings.TrimSpace(asString(token["key"]))
		if apiKey != "" && (key == apiKey || strings.EqualFold(key, apiKey)) {
			return group, true
		}
		if maskedNeedle != "" && key != "" && strings.EqualFold(key, maskedNeedle) {
			return group, true
		}
	}
	return "", false
}

func extractNewAPITokenList(body []byte) []map[string]any {
	var payload struct {
		Success bool `json:"success"`
		Data    any  `json:"data"`
	}
	if json.Unmarshal(body, &payload) != nil || payload.Data == nil {
		return nil
	}
	switch data := payload.Data.(type) {
	case []any:
		return mapsFromAnyList(data)
	case map[string]any:
		for _, key := range []string{"items", "data", "records", "list"} {
			if raw, ok := data[key].([]any); ok {
				return mapsFromAnyList(raw)
			}
		}
	}
	return nil
}

func mapsFromAnyList(items []any) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// maskNewAPITokenKey mirrors NewAPI's common key masking: keep prefix/suffix
// and replace the middle with asterisks (e.g. L7V1**********dgSF).
func maskNewAPITokenKey(apiKey string) string {
	key := strings.TrimSpace(apiKey)
	key = strings.TrimPrefix(key, "sk-")
	if len(key) < 8 {
		return ""
	}
	return key[:4] + "**********" + key[len(key)-4:]
}

func parseUpstreamRateMultiplierCredential(account *Account) (float64, bool) {
	if account == nil || account.Credentials == nil {
		return 0, false
	}
	value, ok := account.Credentials[upstreamRateMultiplierCredentialKey]
	if !ok || value == nil {
		return 0, false
	}
	rate, ok := coerceJSONFloat(value)
	if !ok || rate < 0 {
		return 0, false
	}
	return rate, true
}

func coerceJSONFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, false
		}
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, false
		}
		return f, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}
