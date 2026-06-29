package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	kiropkg "github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/google/uuid"
)

const (
	kiroUsageOrigin       = "AI_EDITOR"
	kiroUsageResourceType = "AGENTIC_REQUEST"

	kiroDefaultRegion = "us-east-1"
)

var resolveKiroRuntimeEndpoint = kiroRuntimeEndpoint

type kiroUsageLimitsResponse struct {
	NextDateReset        any                      `json:"nextDateReset"`
	OverageConfiguration kiroOverageConfiguration `json:"overageConfiguration"`
	SubscriptionInfo     kiroSubscriptionInfo     `json:"subscriptionInfo"`
	UsageBreakdownList   []kiroUsageBreakdown     `json:"usageBreakdownList"`
}

type kiroOverageConfiguration struct {
	OverageStatus string `json:"overageStatus"`
}

type kiroSubscriptionInfo struct {
	SubscriptionTitle string `json:"subscriptionTitle"`
	Type              string `json:"type"`
}

type kiroUsageBreakdown struct {
	Currency                     string             `json:"currency"`
	CurrentOverages              *float64           `json:"currentOverages"`
	CurrentOveragesWithPrecision *float64           `json:"currentOveragesWithPrecision"`
	CurrentUsage                 *float64           `json:"currentUsage"`
	CurrentUsageWithPrecision    *float64           `json:"currentUsageWithPrecision"`
	DisplayName                  string             `json:"displayName"`
	DisplayNamePlural            string             `json:"displayNamePlural"`
	FreeTrialInfo                *kiroFreeTrialInfo `json:"freeTrialInfo"`
	NextDateReset                any                `json:"nextDateReset"`
	OverageCharges               *float64           `json:"overageCharges"`
	ResourceType                 string             `json:"resourceType"`
	UsageLimit                   *float64           `json:"usageLimit"`
	UsageLimitWithPrecision      *float64           `json:"usageLimitWithPrecision"`
}

type kiroFreeTrialInfo struct {
	CurrentUsage              *float64 `json:"currentUsage"`
	CurrentUsageWithPrecision *float64 `json:"currentUsageWithPrecision"`
	FreeTrialExpiry           any      `json:"freeTrialExpiry"`
	FreeTrialStatus           string   `json:"freeTrialStatus"`
	UsageLimit                *float64 `json:"usageLimit"`
	UsageLimitWithPrecision   *float64 `json:"usageLimitWithPrecision"`
}

type kiroUsageHTTPError struct {
	StatusCode int
	Body       string
}

func (e *kiroUsageHTTPError) Error() string {
	if e == nil {
		return "kiro usage request failed"
	}
	if strings.TrimSpace(e.Body) == "" {
		return fmt.Sprintf("kiro usage request failed (status %d)", e.StatusCode)
	}
	return fmt.Sprintf("kiro usage request failed (status %d): %s", e.StatusCode, e.Body)
}

func (s *AccountUsageService) getKiroUsage(ctx context.Context, account *Account, source string, forceRefresh bool) (*UsageInfo, error) {
	now := time.Now()
	if account == nil {
		return &UsageInfo{
			Source:    source,
			UpdatedAt: &now,
			Error:     "account is nil",
			ErrorCode: errorCodeNetworkError,
		}, nil
	}
	if account.Platform != PlatformKiro || account.Type != AccountTypeOAuth {
		return &UsageInfo{
			Source:    source,
			UpdatedAt: &now,
		}, nil
	}

	cached, hasCached := s.getCachedKiroUsage(account.ID)
	if hasCached && (cached.ErrorCode != "" || cached.Error != "") {
		cached.Source = source
		s.attachKiroRuntimeState(ctx, account, cached)
		return cached, nil
	}
	if !forceRefresh && hasCached {
		cached.Source = source
		s.attachKiroRuntimeState(ctx, account, cached)
		return cached, nil
	}

	flightKey := fmt.Sprintf("kiro-usage:%d", account.ID)
	result, fetchErr, _ := s.cache.kiroUsageFlight.Do(flightKey, func() (any, error) {
		if !forceRefresh {
			if usage, ok := s.getCachedKiroUsage(account.ID); ok {
				return usage, nil
			}
		}
		usage, err := s.fetchAndCacheKiroUsage(ctx, account, source)
		if err != nil {
			return nil, err
		}
		return usage, nil
	})
	if fetchErr == nil {
		if usage, ok := result.(*UsageInfo); ok && usage != nil {
			usage.Source = source
			s.attachKiroRuntimeState(ctx, account, usage)
			if source == "active" {
				s.tryClearRecoverableAccountError(ctx, account)
			}
			return usage, nil
		}
	}

	degraded := buildKiroDegradedUsage(fetchErr)
	degraded.Source = source
	if hasCached {
		cached.Error = degraded.Error
		cached.ErrorCode = degraded.ErrorCode
		cached.NeedsReauth = degraded.NeedsReauth
		cached.KiroQuotaState = degraded.KiroQuotaState
		cached.KiroQuotaReason = degraded.KiroQuotaReason
		cached.KiroQuotaResetAt = degraded.KiroQuotaResetAt
		cached.Source = source
		s.attachKiroRuntimeState(ctx, account, cached)
		return cached, nil
	}
	s.storeKiroUsageSnapshot(account.ID, degraded)
	s.attachKiroRuntimeState(ctx, account, degraded)
	return degraded, nil
}

func (s *AccountUsageService) fetchAndCacheKiroUsage(ctx context.Context, account *Account, source string) (*UsageInfo, error) {
	token, err := s.getKiroUsageAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("no access token available")
	}

	region := kiroAPIRegion(account)
	profileArn := strings.TrimSpace(account.GetCredential("profile_arn"))

	resp, err := s.requestKiroUsageLimits(ctx, account, region, profileArn, token)
	if err != nil {
		if s.shouldRetryKiroUsageWithRefresh(err) {
			refreshedToken, refreshErr := s.kiroTokenProvider.ForceRefreshAccessToken(ctx, account)
			if refreshErr == nil && strings.TrimSpace(refreshedToken) != "" {
				resp, err = s.requestKiroUsageLimits(ctx, account, region, profileArn, strings.TrimSpace(refreshedToken))
				if err == nil {
					usage := mapKiroUsageToInfo(resp)
					usage.Source = source
					s.storeKiroUsageSnapshot(account.ID, usage)
					return usage, nil
				}
				return nil, err
			}
		}
		return nil, err
	}

	usage := mapKiroUsageToInfo(resp)
	usage.Source = source
	s.storeKiroUsageSnapshot(account.ID, usage)
	return usage, nil
}

func (s *AccountUsageService) getKiroUsageAccessToken(ctx context.Context, account *Account) (string, error) {
	if s != nil && s.kiroTokenProvider != nil {
		return s.kiroTokenProvider.GetAccessToken(ctx, account)
	}
	return strings.TrimSpace(account.GetCredential("access_token")), nil
}

func (s *AccountUsageService) shouldRetryKiroUsageWithRefresh(err error) bool {
	if s == nil || s.kiroTokenProvider == nil || err == nil {
		return false
	}
	return classifyKiroError(err).Category == kiroErrorAuthError
}

func (s *AccountUsageService) storeKiroUsageSnapshot(accountID int64, usage *UsageInfo) {
	if s == nil || s.cache == nil || accountID <= 0 || usage == nil {
		return
	}
	now := time.Now()
	if usage.UpdatedAt == nil {
		usage.UpdatedAt = &now
	}
	s.cache.kiroUsageCache.Store(accountID, &kiroUsageCache{
		usageInfo: cloneUsageInfo(usage),
		timestamp: now,
	})
}

func (s *AccountUsageService) getCachedKiroUsage(accountID int64) (*UsageInfo, bool) {
	if s == nil || s.cache == nil || accountID <= 0 {
		return nil, false
	}
	cached, ok := s.cache.kiroUsageCache.Load(accountID)
	if !ok {
		return nil, false
	}
	cache, ok := cached.(*kiroUsageCache)
	if !ok || cache == nil || cache.usageInfo == nil {
		return nil, false
	}
	if time.Since(cache.timestamp) >= kiroCacheTTL(cache.usageInfo) {
		return nil, false
	}
	return cloneUsageInfo(cache.usageInfo), true
}

func kiroCacheTTL(info *UsageInfo) time.Duration {
	if info == nil {
		return kiroUsageErrorTTL
	}
	if info.ErrorCode != "" || info.Error != "" {
		return kiroUsageErrorTTL
	}
	return apiCacheTTL
}

func cloneUsageInfo(info *UsageInfo) *UsageInfo {
	if info == nil {
		return nil
	}
	cloned := *info
	return &cloned
}

func (s *AccountUsageService) requestKiroUsageLimits(ctx context.Context, account *Account, region, profileArn, token string) (*kiroUsageLimitsResponse, error) {
	endpoint := resolveKiroRuntimeEndpoint(region)
	reqURL, err := url.Parse(endpoint + "/getUsageLimits")
	if err != nil {
		return nil, fmt.Errorf("build kiro usage url failed: %w", err)
	}
	q := reqURL.Query()
	q.Set("origin", kiroUsageOrigin)
	if profileArn = strings.TrimSpace(profileArn); profileArn != "" {
		q.Set("profileArn", profileArn)
	}
	q.Set("resourceType", kiroUsageResourceType)
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create kiro usage request failed: %w", err)
	}
	s.applyKiroRuntimeHeaders(req, account, token)

	client, err := httpclient.GetClient(httpclient.Options{
		ProxyURL:           accountProxyURL(account),
		Timeout:            30 * time.Second,
		ValidateResolvedIP: true,
		AllowPrivateHosts:  isLoopbackEndpoint(endpoint),
	})
	if err != nil {
		return nil, fmt.Errorf("create kiro usage client failed: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kiro usage request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read kiro usage response failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &kiroUsageHTTPError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}

	var parsed kiroUsageLimitsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode kiro usage response failed: %w", err)
	}
	return &parsed, nil
}

func (s *AccountUsageService) applyKiroRuntimeHeaders(req *http.Request, account *Account, token string) {
	if req == nil {
		return
	}
	accountKey := buildKiroAccountKey(account)
	machineID := buildKiroMachineID(account)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	req.Header.Set("User-Agent", kiropkg.BuildRuntimeUserAgent(accountKey, machineID))
	req.Header.Set("X-Amz-User-Agent", kiropkg.BuildRuntimeAmzUserAgent(accountKey, machineID))
	req.Header.Set("x-amzn-kiro-agent-mode", "vibe")
	req.Header.Set("x-amzn-codewhisperer-optout", "true")
	req.Header.Set("Amz-Sdk-Request", "attempt=1; max=3")
	req.Header.Set("Amz-Sdk-Invocation-Id", uuid.NewString())

	if account == nil {
		return
	}
	applyKiroConditionalHeaders(req, account)
}

func accountProxyURL(account *Account) string {
	if account == nil || account.ProxyID == nil || account.Proxy == nil {
		return ""
	}
	return account.Proxy.URL()
}

func kiroRuntimeEndpoint(region string) string {
	region = strings.TrimSpace(region)
	if region == "" {
		region = kiroDefaultRegion
	}
	switch region {
	case "us-east-1":
		return "https://q.us-east-1.amazonaws.com"
	case "eu-central-1":
		return "https://q.eu-central-1.amazonaws.com"
	case "us-gov-east-1":
		return "https://q-fips.us-gov-east-1.amazonaws.com"
	case "us-gov-west-1":
		return "https://q-fips.us-gov-west-1.amazonaws.com"
	case "us-iso-east-1":
		return "https://q.us-iso-east-1.c2s.ic.gov"
	case "us-isob-east-1":
		return "https://q.us-isob-east-1.sc2s.sgov.gov"
	case "us-isof-south-1":
		return "https://q.us-isof-south-1.csp.hci.ic.gov"
	case "us-isof-east-1":
		return "https://q.us-isof-east-1.csp.hci.ic.gov"
	default:
		if strings.HasPrefix(region, "us-gov-") {
			return "https://q-fips." + region + ".amazonaws.com"
		}
		return "https://q." + region + ".amazonaws.com"
	}
}

func isLoopbackEndpoint(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func mapKiroUsageToInfo(resp *kiroUsageLimitsResponse) *UsageInfo {
	now := time.Now()
	if resp == nil {
		return &UsageInfo{UpdatedAt: &now}
	}
	info := &UsageInfo{
		UpdatedAt:            &now,
		KiroSubscriptionName: strings.TrimSpace(resp.SubscriptionInfo.SubscriptionTitle),
		KiroSubscriptionType: strings.TrimSpace(resp.SubscriptionInfo.Type),
		KiroOveragesEnabled:  strings.EqualFold(strings.TrimSpace(resp.OverageConfiguration.OverageStatus), "ENABLED"),
	}

	resetAt := parseKiroTimestamp(resp.NextDateReset)
	if credit := selectKiroCreditBreakdown(resp.UsageBreakdownList); credit != nil {
		if breakdownReset := parseKiroTimestamp(credit.NextDateReset); breakdownReset != nil {
			resetAt = breakdownReset
		}
		info.KiroCredit = &KiroCreditProgress{
			CurrentUsage:   selectKiroFloat(credit.CurrentUsageWithPrecision, credit.CurrentUsage),
			UsageLimit:     selectKiroFloat(credit.UsageLimitWithPrecision, credit.UsageLimit),
			PercentageUsed: percentageOrZero(selectKiroFloat(credit.CurrentUsageWithPrecision, credit.CurrentUsage), selectKiroFloat(credit.UsageLimitWithPrecision, credit.UsageLimit)),
		}
		info.KiroOverage = &KiroOverageInfo{
			CurrentOverages: selectKiroFloat(credit.CurrentOveragesWithPrecision, credit.CurrentOverages),
			OverageCharges:  selectKiroFloat(credit.OverageCharges, nil),
			CurrencyCode:    strings.TrimSpace(credit.Currency),
			CurrencySymbol:  kiroCurrencySymbol(strings.TrimSpace(credit.Currency)),
		}
		if ft := credit.FreeTrialInfo; ft != nil && strings.EqualFold(strings.TrimSpace(ft.FreeTrialStatus), "ACTIVE") {
			expiry := parseKiroTimestamp(ft.FreeTrialExpiry)
			daysRemaining := 0
			if expiry != nil {
				daysRemaining = int(time.Until(*expiry).Hours() / 24)
				if time.Until(*expiry)%(24*time.Hour) != 0 {
					daysRemaining++
				}
				if daysRemaining < 0 {
					daysRemaining = 0
				}
			}
			current := selectKiroFloat(ft.CurrentUsageWithPrecision, ft.CurrentUsage)
			limit := selectKiroFloat(ft.UsageLimitWithPrecision, ft.UsageLimit)
			info.KiroBonus = &KiroCreditProgress{
				CurrentUsage:   current,
				UsageLimit:     limit,
				PercentageUsed: percentageOrZero(current, limit),
				DaysRemaining:  daysRemaining,
				ExpiryDate:     expiry,
			}
		}
	}
	info.KiroResetAt = resetAt
	setKiroQuotaStateFromUsage(info)
	return info
}

func selectKiroCreditBreakdown(items []kiroUsageBreakdown) *kiroUsageBreakdown {
	for i := range items {
		if strings.EqualFold(strings.TrimSpace(items[i].ResourceType), "CREDIT") {
			return &items[i]
		}
	}
	if len(items) == 0 {
		return nil
	}
	return &items[0]
}

func selectKiroFloat(preferred *float64, fallback *float64) float64 {
	switch {
	case preferred != nil:
		return *preferred
	case fallback != nil:
		return *fallback
	default:
		return 0
	}
}

func percentageOrZero(current, limit float64) float64 {
	if limit <= 0 {
		return 0
	}
	return current / limit * 100
}

func parseKiroTimestamp(raw any) *time.Time {
	if raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return nil
		}
		if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
			return &parsed
		}
		if i, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return unixishToTime(i)
		}
		if f, err := strconv.ParseFloat(trimmed, 64); err == nil {
			return unixishFloatToTime(f)
		}
	case float64:
		return unixishFloatToTime(v)
	case int64:
		return unixishToTime(v)
	case int:
		return unixishToTime(int64(v))
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return unixishToTime(i)
		}
		if f, err := v.Float64(); err == nil {
			return unixishFloatToTime(f)
		}
	}
	return nil
}

func unixishFloatToTime(v float64) *time.Time {
	if v <= 0 {
		return nil
	}
	if v >= 1e12 {
		t := time.UnixMilli(int64(v))
		return &t
	}
	t := time.Unix(int64(v), 0)
	return &t
}

func unixishToTime(v int64) *time.Time {
	if v <= 0 {
		return nil
	}
	if v >= 1e12 {
		t := time.UnixMilli(v)
		return &t
	}
	t := time.Unix(v, 0)
	return &t
}

func kiroCurrencySymbol(code string) string {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "USD":
		return "$"
	default:
		return ""
	}
}

func buildKiroDegradedUsage(err error) *UsageInfo {
	now := time.Now()
	info := &UsageInfo{
		UpdatedAt: &now,
		Error:     "usage API error",
		ErrorCode: errorCodeNetworkError,
	}
	if err == nil {
		return info
	}

	info.Error = fmt.Sprintf("usage API error: %v", err)

	classification := classifyKiroError(err)
	switch classification.Category {
	case kiroErrorAuthError:
		info.ErrorCode = errorCodeUnauthenticated
		info.NeedsReauth = true
	case kiroErrorRateLimited:
		info.ErrorCode = errorCodeRateLimited
	case kiroErrorQuotaExhausted:
		info.ErrorCode = errorCodeNetworkError
		info.KiroQuotaState = kiroQuotaStateCreditsExhausted
		info.KiroQuotaReason = classification.Message
	case kiroErrorOverageExhausted:
		info.ErrorCode = errorCodeNetworkError
		info.KiroQuotaState = kiroQuotaStateOverageExhausted
		info.KiroQuotaReason = classification.Message
	case kiroErrorSuspended, kiroErrorUsageForbidden, kiroErrorProfileError:
		info.ErrorCode = errorCodeForbidden
	default:
		info.ErrorCode = errorCodeNetworkError
	}
	return info
}

func (s *AccountUsageService) attachKiroRuntimeState(ctx context.Context, account *Account, usage *UsageInfo) {
	if s == nil || usage == nil || account == nil || account.Platform != PlatformKiro || s.kiroCooldownStore == nil {
		return
	}
	usage.KiroRuntimeState = ""
	usage.KiroRuntimeReason = ""
	usage.KiroRuntimeResetAt = nil
	state, err := s.kiroCooldownStore.GetState(ctx, buildKiroAccountKey(account))
	if err != nil || state == nil {
		return
	}
	usage.KiroRuntimeState, usage.KiroRuntimeReason, usage.KiroRuntimeResetAt = kiroRuntimeStateSnapshot(state)
}

func (s *AccountUsageService) EnrichAccountWithKiroRuntimeState(ctx context.Context, account *Account) {
	if s == nil || account == nil || account.Platform != PlatformKiro || account.Type != AccountTypeOAuth {
		return
	}
	account.KiroQuotaState = ""
	account.KiroQuotaReason = ""
	account.KiroQuotaResetAt = nil
	account.KiroRuntimeState = ""
	account.KiroRuntimeReason = ""
	account.KiroRuntimeResetAt = nil
	if usage, ok := s.getCachedKiroUsage(account.ID); ok {
		account.KiroQuotaState = usage.KiroQuotaState
		account.KiroQuotaReason = usage.KiroQuotaReason
		account.KiroQuotaResetAt = usage.KiroQuotaResetAt
	}
	if s.kiroCooldownStore == nil {
		return
	}
	state, err := s.kiroCooldownStore.GetState(ctx, buildKiroAccountKey(account))
	if err != nil || state == nil {
		return
	}
	account.KiroRuntimeState, account.KiroRuntimeReason, account.KiroRuntimeResetAt = kiroRuntimeStateSnapshot(state)
}

func setKiroQuotaStateFromUsage(info *UsageInfo) {
	if info == nil {
		return
	}
	info.KiroQuotaState = ""
	info.KiroQuotaReason = ""
	info.KiroQuotaResetAt = nil

	creditExhausted := false
	if info.KiroCredit != nil && info.KiroCredit.UsageLimit > 0 {
		creditExhausted = info.KiroCredit.CurrentUsage >= info.KiroCredit.UsageLimit
	}
	overageActive := info.KiroOverage != nil &&
		(info.KiroOverage.CurrentOverages > 0 || info.KiroOverage.OverageCharges > 0)

	switch {
	case info.KiroOveragesEnabled && (overageActive || creditExhausted):
		info.KiroQuotaState = kiroQuotaStateOverageActive
		info.KiroQuotaReason = "overages_enabled"
		info.KiroQuotaResetAt = info.KiroResetAt
	case creditExhausted:
		info.KiroQuotaState = kiroQuotaStateCreditsExhausted
		info.KiroQuotaReason = "credits_exhausted"
		info.KiroQuotaResetAt = info.KiroResetAt
	default:
		info.KiroQuotaState = kiroQuotaStateNormal
	}
}
