package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"golang.org/x/sync/singleflight"
)

const (
	AccountQuotaModeManual   = "manual"
	AccountQuotaModeOAuth    = "oauth"
	AccountQuotaModeUpstream = "upstream"

	AccountQuotaModeExtraKey     = "quota_mode"
	AccountQuotaProviderExtraKey = "quota_provider"
	AccountQuotaSnapshotExtraKey = "upstream_quota_snapshot"

	accountQuotaCacheTTL      = 5 * time.Minute
	accountQuotaErrorCacheTTL = time.Minute
	accountQuotaBodyLimit     = 64 << 10
	defaultNewAPIQuotaPerUSD  = 500_000
	newAPIUnlimitedSentinel   = 100_000_000
)

// AccountQuotaMetric is the provider-independent representation consumed by the account list.
type AccountQuotaMetric struct {
	Key         string     `json:"key"`
	Label       string     `json:"label"`
	Period      string     `json:"period,omitempty"`
	Unit        string     `json:"unit"`
	Limit       *float64   `json:"limit,omitempty"`
	Used        *float64   `json:"used,omitempty"`
	Remaining   *float64   `json:"remaining,omitempty"`
	Utilization *float64   `json:"utilization,omitempty"`
	ResetAt     *time.Time `json:"reset_at,omitempty"`
	Unlimited   bool       `json:"unlimited,omitempty"`
}

type AccountQuotaPlan struct {
	Type      string     `json:"type,omitempty"`
	Name      string     `json:"name,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// AccountQuotaResult is the stable response contract for manual, OAuth and upstream-key quotas.
// Usage is retained for OAuth accounts so existing detailed OAuth views can migrate incrementally.
type AccountQuotaResult struct {
	Mode            string               `json:"mode"`
	Provider        string               `json:"provider"`
	Status          string               `json:"status"`
	Source          string               `json:"source"`
	FetchedAt       *time.Time           `json:"fetched_at,omitempty"`
	FreshUntil      *time.Time           `json:"fresh_until,omitempty"`
	Plan            *AccountQuotaPlan    `json:"plan,omitempty"`
	KeyExpiresAt    *time.Time           `json:"key_expires_at,omitempty"`
	Metrics         []AccountQuotaMetric `json:"metrics"`
	Usage           *UsageInfo           `json:"usage,omitempty"`
	SuggestedConfig map[string]any       `json:"suggested_config,omitempty"`
	Warnings        []string             `json:"warnings,omitempty"`
	ErrorCode       string               `json:"error_code,omitempty"`
	Error           string               `json:"error,omitempty"`
}

type AccountQuotaProviderDescriptor struct {
	ID                    string                            `json:"id"`
	Name                  string                            `json:"name"`
	SupportedAccountTypes []string                          `json:"supported_account_types"`
	RequiredCredentials   []string                          `json:"required_credentials"`
	ConfigFields          []AccountQuotaProviderConfigField `json:"config_fields"`
}

type AccountQuotaProviderConfigField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Type        string `json:"type"`
	Required    bool   `json:"required,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
}

type AccountQuotaFetchInput struct {
	Account  *Account
	BaseURL  string
	APIKey   string
	ProxyURL string
	Config   map[string]any
}

// AccountQuotaProvider is the extension point for another upstream key provider.
type AccountQuotaProvider interface {
	Descriptor() AccountQuotaProviderDescriptor
	Fetch(context.Context, AccountQuotaFetchInput) (*AccountQuotaResult, error)
}

type accountQuotaCacheEntry struct {
	result    *AccountQuotaResult
	timestamp time.Time
}

type AccountQuotaService struct {
	accountRepo  AccountRepository
	usageService *AccountUsageService
	providers    map[string]AccountQuotaProvider
	cache        sync.Map
	flight       singleflight.Group
}

func NewAccountQuotaService(accountRepo AccountRepository, usageService *AccountUsageService, httpUpstream HTTPUpstream) *AccountQuotaService {
	s := &AccountQuotaService{
		accountRepo:  accountRepo,
		usageService: usageService,
		providers:    make(map[string]AccountQuotaProvider),
	}
	s.RegisterProvider(&sub2APIQuotaProvider{client: httpUpstream})
	s.RegisterProvider(&newAPIQuotaProvider{client: httpUpstream})
	return s
}

func (s *AccountQuotaService) RegisterProvider(provider AccountQuotaProvider) {
	if s == nil || provider == nil {
		return
	}
	id := strings.ToLower(strings.TrimSpace(provider.Descriptor().ID))
	if id != "" {
		s.providers[id] = provider
	}
}

func (s *AccountQuotaService) Providers() []AccountQuotaProviderDescriptor {
	result := make([]AccountQuotaProviderDescriptor, 0, len(s.providers))
	for _, provider := range s.providers {
		result = append(result, provider.Descriptor())
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

// GetQuota applies the same passive/active/force lifecycle as OAuth usage queries.
func (s *AccountQuotaService) GetQuota(ctx context.Context, accountID int64, source string, force bool) (*AccountQuotaResult, error) {
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" {
		source = "active"
	}
	if source != "active" && source != "passive" {
		return nil, fmt.Errorf("invalid quota source %q", source)
	}

	if account.Type == AccountTypeOAuth || account.Type == AccountTypeSetupToken {
		return s.getOAuthQuota(ctx, account, source, force)
	}
	if accountQuotaMode(account) != AccountQuotaModeUpstream {
		return manualAccountQuota(account), nil
	}

	providerID := strings.ToLower(strings.TrimSpace(account.GetExtraString(AccountQuotaProviderExtraKey)))
	provider, ok := s.providers[providerID]
	if !ok {
		return &AccountQuotaResult{Mode: AccountQuotaModeUpstream, Provider: providerID, Status: "unsupported", Source: source, Metrics: []AccountQuotaMetric{}, ErrorCode: "UNSUPPORTED_QUOTA_PROVIDER", Error: "暂不支持该上游配额供应商"}, nil
	}

	stale := quotaSnapshotFromExtra(account.Extra)
	if stale != nil && !strings.EqualFold(stale.Provider, providerID) {
		stale = nil
	}
	if source == "passive" {
		if stale != nil {
			stale.Source = "passive"
			return stale, nil
		}
		return &AccountQuotaResult{Mode: AccountQuotaModeUpstream, Provider: providerID, Status: "pending", Source: "passive", Metrics: []AccountQuotaMetric{}}, nil
	}

	if !force {
		if cached := s.cached(accountID); cached != nil {
			cached.Source = "cache"
			return cached, nil
		}
	}

	key := strconv.FormatInt(accountID, 10)
	value, _, _ := s.flight.Do(key, func() (any, error) {
		if !force {
			if cached := s.cached(accountID); cached != nil {
				return cached, nil
			}
		}
		input, inputErr := buildAccountQuotaFetchInput(account)
		if inputErr != nil {
			return failedQuotaResult(providerID, stale, inputErr), nil
		}
		result, fetchErr := provider.Fetch(ctx, input)
		if fetchErr != nil {
			failed := failedQuotaResult(providerID, stale, fetchErr)
			s.cache.Store(accountID, &accountQuotaCacheEntry{result: cloneAccountQuotaResult(failed), timestamp: time.Now()})
			return failed, nil
		}
		now := time.Now().UTC()
		freshUntil := now.Add(accountQuotaCacheTTL)
		result.Mode = AccountQuotaModeUpstream
		result.Provider = providerID
		result.Status = "ok"
		result.Source = "active"
		result.FetchedAt = &now
		result.FreshUntil = &freshUntil
		if result.Metrics == nil {
			result.Metrics = []AccountQuotaMetric{}
		}
		s.cache.Store(accountID, &accountQuotaCacheEntry{result: cloneAccountQuotaResult(result), timestamp: time.Now()})
		if snapshot, marshalErr := quotaSnapshotToMap(result); marshalErr == nil {
			updates := map[string]any{AccountQuotaSnapshotExtraKey: snapshot}
			if suggestedConfig, changed := mergeSuggestedQuotaConfig(input.Config, result.SuggestedConfig); changed {
				updates["upstream_quota_config"] = suggestedConfig
			}
			_ = s.accountRepo.UpdateExtra(ctx, accountID, updates)
		}
		return result, nil
	})
	return value.(*AccountQuotaResult), nil
}

func (s *AccountQuotaService) getOAuthQuota(ctx context.Context, account *Account, source string, force bool) (*AccountQuotaResult, error) {
	var usage *UsageInfo
	var err error
	if source == "passive" {
		usage, err = s.usageService.GetPassiveUsage(ctx, account.ID)
	} else {
		usage, err = s.usageService.GetUsage(ctx, account.ID, force)
	}
	if err != nil {
		return nil, err
	}
	result := &AccountQuotaResult{Mode: AccountQuotaModeOAuth, Provider: account.Platform, Status: "ok", Source: source, Usage: usage, Metrics: usageMetrics(usage)}
	if usage != nil {
		result.FetchedAt = usage.UpdatedAt
		if usage.Error != "" {
			result.Status, result.Error, result.ErrorCode = "failed", usage.Error, usage.ErrorCode
		}
	}
	return result, nil
}

func (s *AccountQuotaService) cached(accountID int64) *AccountQuotaResult {
	raw, ok := s.cache.Load(accountID)
	if !ok {
		return nil
	}
	entry, ok := raw.(*accountQuotaCacheEntry)
	if !ok || entry.result == nil {
		return nil
	}
	ttl := accountQuotaCacheTTL
	if entry.result.Status == "failed" || entry.result.Status == "stale" {
		ttl = accountQuotaErrorCacheTTL
	}
	if time.Since(entry.timestamp) >= ttl {
		s.cache.Delete(accountID)
		return nil
	}
	return cloneAccountQuotaResult(entry.result)
}

func accountQuotaMode(account *Account) string {
	if account == nil || account.Type == AccountTypeOAuth || account.Type == AccountTypeSetupToken {
		return AccountQuotaModeOAuth
	}
	if strings.EqualFold(account.GetExtraString(AccountQuotaModeExtraKey), AccountQuotaModeUpstream) {
		return AccountQuotaModeUpstream
	}
	return AccountQuotaModeManual
}

func validateAccountQuotaConfig(account *Account) error {
	if account == nil || account.Extra == nil {
		return nil
	}
	rawMode, exists := account.Extra[AccountQuotaModeExtraKey]
	if !exists || rawMode == nil || rawMode == "" {
		return nil
	}
	mode, ok := rawMode.(string)
	if !ok || (mode != AccountQuotaModeManual && mode != AccountQuotaModeUpstream) {
		return infraerrors.BadRequest("INVALID_QUOTA_MODE", "quota_mode must be manual or upstream")
	}
	if mode != AccountQuotaModeUpstream {
		delete(account.Extra, AccountQuotaProviderExtraKey)
		delete(account.Extra, "upstream_quota_config")
		return nil
	}
	if account.Type != AccountTypeAPIKey {
		return infraerrors.BadRequest("INVALID_UPSTREAM_QUOTA_ACCOUNT_TYPE", "upstream quota is only supported for API key accounts")
	}
	provider, ok := account.Extra[AccountQuotaProviderExtraKey].(string)
	if !ok || strings.TrimSpace(provider) == "" {
		return infraerrors.BadRequest("QUOTA_PROVIDER_REQUIRED", "quota_provider is required for upstream quota")
	}
	account.Extra[AccountQuotaProviderExtraKey] = strings.ToLower(strings.TrimSpace(provider))
	for key := range account.Extra {
		if strings.HasPrefix(key, "quota_") && key != AccountQuotaModeExtraKey && key != AccountQuotaProviderExtraKey {
			delete(account.Extra, key)
		}
	}
	if rawConfig, exists := account.Extra["upstream_quota_config"]; exists && rawConfig != nil {
		config, ok := rawConfig.(map[string]any)
		if !ok {
			return infraerrors.BadRequest("INVALID_UPSTREAM_QUOTA_CONFIG", "upstream_quota_config must be an object")
		}
		if account.Extra[AccountQuotaProviderExtraKey] == "newapi" {
			if rawUserID, exists := config["user_id"]; exists && !isEmptyQuotaConfigValue(rawUserID) {
				userID := identifierValue(rawUserID)
				parsedUserID, parseErr := strconv.ParseInt(userID, 10, 64)
				if parseErr != nil || parsedUserID <= 0 {
					return infraerrors.BadRequest("INVALID_NEWAPI_USER_ID", "NewAPI user_id must be a positive integer")
				}
				config["user_id"] = userID
			}
			if rawRate, exists := config["quota_per_usd"]; exists && !isEmptyQuotaConfigValue(rawRate) {
				rate, rateOK := firstNumber(map[string]any{"value": rawRate}, "value")
				if !rateOK || rate <= 0 {
					return infraerrors.BadRequest("INVALID_NEWAPI_QUOTA_RATE", "NewAPI quota_per_usd must be greater than zero")
				}
				config["quota_per_usd"] = rate
			}
		}
	}
	return nil
}

func manualAccountQuota(account *Account) *AccountQuotaResult {
	result := &AccountQuotaResult{Mode: AccountQuotaModeManual, Provider: "manual", Status: "ok", Source: "local", Metrics: []AccountQuotaMetric{}}
	add := func(key, label, period string, limit, used float64) {
		if limit <= 0 {
			return
		}
		remaining := limit - used
		if remaining < 0 {
			remaining = 0
		}
		utilization := used / limit * 100
		result.Metrics = append(result.Metrics, AccountQuotaMetric{Key: key, Label: label, Period: period, Unit: "USD", Limit: floatPtr(limit), Used: floatPtr(used), Remaining: &remaining, Utilization: &utilization})
	}
	add("total", "总配额", "total", account.GetQuotaLimit(), account.GetQuotaUsed())
	add("daily", "日配额", "day", account.GetQuotaDailyLimit(), account.GetQuotaDailyUsed())
	add("weekly", "周配额", "week", account.GetQuotaWeeklyLimit(), account.GetQuotaWeeklyUsed())
	return result
}

func usageMetrics(usage *UsageInfo) []AccountQuotaMetric {
	if usage == nil {
		return []AccountQuotaMetric{}
	}
	metrics := make([]AccountQuotaMetric, 0, 4)
	add := func(key, label, period string, progress *UsageProgress) {
		if progress == nil {
			return
		}
		util := progress.Utilization
		metrics = append(metrics, AccountQuotaMetric{Key: key, Label: label, Period: period, Unit: "percent", Utilization: &util, ResetAt: progress.ResetsAt})
	}
	add("five_hour", "5 小时", "5h", usage.FiveHour)
	add("seven_day", "7 天", "7d", usage.SevenDay)
	add("seven_day_sonnet", "Sonnet 7 天", "7d", usage.SevenDaySonnet)
	add("seven_day_fable", "Fable 7 天", "7d", usage.SevenDayFable)
	return metrics
}

func buildAccountQuotaFetchInput(account *Account) (AccountQuotaFetchInput, error) {
	baseURL := strings.TrimSpace(account.GetCredential("base_url"))
	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	if baseURL == "" {
		return AccountQuotaFetchInput{}, errors.New("账号未配置上游地址")
	}
	if apiKey == "" {
		return AccountQuotaFetchInput{}, errors.New("账号未配置 API Key")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return AccountQuotaFetchInput{}, errors.New("上游地址无效")
	}
	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	config, _ := mapValue(account.Extra["upstream_quota_config"])
	return AccountQuotaFetchInput{Account: account, BaseURL: strings.TrimRight(baseURL, "/"), APIKey: apiKey, ProxyURL: proxyURL, Config: config}, nil
}

func failedQuotaResult(provider string, stale *AccountQuotaResult, err error) *AccountQuotaResult {
	if stale != nil {
		result := cloneAccountQuotaResult(stale)
		result.Status, result.Source, result.ErrorCode, result.Error = "stale", "cache", "UPSTREAM_QUOTA_FETCH_FAILED", safeQuotaError(err)
		return result
	}
	return &AccountQuotaResult{Mode: AccountQuotaModeUpstream, Provider: provider, Status: "failed", Source: "active", Metrics: []AccountQuotaMetric{}, ErrorCode: "UPSTREAM_QUOTA_FETCH_FAILED", Error: safeQuotaError(err)}
}

func safeQuotaError(err error) string {
	if err == nil {
		return "上游配额查询失败"
	}
	message := strings.TrimSpace(err.Error())
	if message == "" || len(message) > 240 {
		return "上游配额查询失败"
	}
	return message
}

func quotaSnapshotToMap(result *AccountQuotaResult) (map[string]any, error) {
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	var snapshot map[string]any
	err = json.Unmarshal(raw, &snapshot)
	return snapshot, err
}

func quotaSnapshotFromExtra(extra map[string]any) *AccountQuotaResult {
	if extra == nil || extra[AccountQuotaSnapshotExtraKey] == nil {
		return nil
	}
	raw, err := json.Marshal(extra[AccountQuotaSnapshotExtraKey])
	if err != nil {
		return nil
	}
	var result AccountQuotaResult
	if json.Unmarshal(raw, &result) != nil || result.Provider == "" {
		return nil
	}
	return &result
}

func cloneAccountQuotaResult(result *AccountQuotaResult) *AccountQuotaResult {
	if result == nil {
		return nil
	}
	raw, _ := json.Marshal(result)
	var clone AccountQuotaResult
	_ = json.Unmarshal(raw, &clone)
	return &clone
}

func floatPtr(value float64) *float64 { return &value }

type sub2APIQuotaProvider struct{ client HTTPUpstream }

func (p *sub2APIQuotaProvider) Descriptor() AccountQuotaProviderDescriptor {
	return AccountQuotaProviderDescriptor{ID: "sub2api", Name: "Sub2API", SupportedAccountTypes: []string{AccountTypeAPIKey}, RequiredCredentials: []string{"base_url", "api_key"}, ConfigFields: []AccountQuotaProviderConfigField{}}
}

func (p *sub2APIQuotaProvider) Fetch(ctx context.Context, input AccountQuotaFetchInput) (*AccountQuotaResult, error) {
	endpoint := input.BaseURL
	if strings.HasSuffix(strings.ToLower(endpoint), "/v1") {
		endpoint += "/usage"
	} else {
		endpoint += "/v1/usage"
	}
	payload, err := fetchQuotaJSON(ctx, p.client, input, endpoint)
	if err != nil {
		return nil, err
	}
	data := unwrapQuotaData(payload)
	result := &AccountQuotaResult{Metrics: []AccountQuotaMetric{}}
	result.KeyExpiresAt = timeValue(data["expires_at"])
	mode := stringValue(data["mode"])
	if subscription, ok := mapValue(data["subscription"]); ok {
		applyLegacySub2APIResetFallbacks(subscription, time.Now().UTC())
		result.Plan = &AccountQuotaPlan{Type: "subscription", Name: firstString(data, "planName", "plan_name", "name", "plan"), ExpiresAt: firstTime(subscription, "expires_at", "expired_at")}
		appendSub2APISubscriptionMetric(result, subscription, "daily", "日配额", "day")
		appendSub2APISubscriptionMetric(result, subscription, "weekly", "周配额", "week")
		appendSub2APISubscriptionMetric(result, subscription, "monthly", "月配额", "month")
	} else if quota, ok := mapValue(data["quota"]); ok {
		appendQuotaMetric(result, "total", "总配额", "total", quota)
	} else {
		appendQuotaMetric(result, "total", "总配额", "total", data)
	}
	if len(result.Metrics) == 0 {
		if wallet, ok := mapValue(data["wallet"]); ok {
			appendQuotaMetric(result, "wallet", "钱包余额", "total", wallet)
		}
	}
	if rateLimits, ok := data["rate_limits"].([]any); ok {
		for _, raw := range rateLimits {
			if rateLimit, ok := mapValue(raw); ok {
				window := firstString(rateLimit, "window", "period")
				appendQuotaMetric(result, "rate_"+window, strings.ToUpper(window)+" 速率额度", window, rateLimit)
			}
		}
	}
	if result.Plan == nil && mode != "" {
		result.Plan = &AccountQuotaPlan{Type: mode, Name: mode}
	}
	if len(result.Metrics) == 0 {
		return nil, errors.New("Sub2API 返回的数据中没有可识别的配额")
	}
	return result, nil
}

type newAPIQuotaProvider struct{ client HTTPUpstream }

func (p *newAPIQuotaProvider) Descriptor() AccountQuotaProviderDescriptor {
	return AccountQuotaProviderDescriptor{
		ID:                    "newapi",
		Name:                  "NewAPI",
		SupportedAccountTypes: []string{AccountTypeAPIKey},
		RequiredCredentials:   []string{"base_url", "api_key"},
		ConfigFields: []AccountQuotaProviderConfigField{
			{Key: "user_id", Label: "NewAPI User ID", Type: "number", Placeholder: "Optional for legacy versions"},
			{Key: "quota_per_usd", Label: "Quota per USD", Type: "number", Placeholder: "500000"},
		},
	}
}

func (p *newAPIQuotaProvider) Fetch(ctx context.Context, input AccountQuotaFetchInput) (*AccountQuotaResult, error) {
	base := strings.TrimRight(input.BaseURL, "/")
	if strings.HasSuffix(strings.ToLower(base), "/v1") {
		base = base[:len(base)-len("/v1")]
	}
	payload, err := fetchQuotaJSON(ctx, p.client, input, base+"/api/usage/token/")
	if err != nil {
		return nil, err
	}
	data := unwrapQuotaData(payload)
	result := &AccountQuotaResult{Metrics: []AccountQuotaMetric{}}
	display := p.fetchQuotaDisplay(ctx, input, base, data)
	limit, hasLimit := firstNumber(data, "total_granted", "quota", "total_quota")
	used, hasUsed := firstNumber(data, "total_used", "used_quota")
	if !hasUsed {
		if remaining, ok := firstNumber(data, "total_available", "remain_quota", "remaining_quota"); ok && hasLimit {
			used, hasUsed = limit-remaining, true
		}
	}
	remaining, hasRemaining := firstNumber(data, "total_available", "remain_quota", "remaining_quota")
	if !hasRemaining && hasLimit && hasUsed {
		remaining, hasRemaining = limit-used, true
	}
	if hasLimit {
		limit = display.convert(limit)
	}
	if hasUsed {
		used = display.convert(used)
	}
	if hasRemaining {
		remaining = display.convert(remaining)
	}
	metric := AccountQuotaMetric{Key: "total", Label: "Key 配额", Period: "total", Unit: display.unit}
	if unlimited, ok := data["unlimited_quota"].(bool); ok {
		metric.Unlimited = unlimited
	}
	if metric.Unlimited {
		balance, balanceErr := p.fetchAccountBalance(ctx, input, base)
		// The compatibility billing endpoints expose hard_limit_usd and
		// total_usage in USD/cents, regardless of the key quota display mode.
		display.unit = "USD"
		if balanceErr != nil {
			return nil, fmt.Errorf("NewAPI Key 未限制额度，但账号余额获取失败: %w", balanceErr)
		}
		metric = AccountQuotaMetric{Key: "balance", Label: "账号余额", Period: "total", Unit: display.unit, Remaining: &balance}
	} else {
		if hasLimit {
			metric.Limit = floatPtr(limit)
		}
		if hasUsed {
			metric.Used = floatPtr(used)
		}
		if hasRemaining {
			metric.Remaining = floatPtr(remaining)
		}
		if hasLimit && limit > 0 && hasUsed {
			metric.Utilization = floatPtr(used / limit * 100)
		}
	}
	if metric.Limit == nil && metric.Used == nil && metric.Remaining == nil && !metric.Unlimited {
		return nil, errors.New("NewAPI 返回的数据中没有可识别的配额")
	}
	result.Metrics = append(result.Metrics, metric)
	result.KeyExpiresAt = firstTime(data, "expires_at", "expired_at")
	if groupName := firstString(data, "group", "group_name", "token_group"); groupName != "" {
		result.Plan = &AccountQuotaPlan{Type: "group", Name: groupName}
	}
	if userID := firstIdentifier(data, "user_id", "userId", "uid"); userID != "" {
		result.SuggestedConfig = map[string]any{"user_id": userID}
	} else if user, ok := mapValue(data["user"]); ok {
		if userID := firstIdentifier(user, "id", "user_id", "userId"); userID != "" {
			result.SuggestedConfig = map[string]any{"user_id": userID}
		}
	}
	return result, nil
}

func (p *newAPIQuotaProvider) fetchAccountBalance(ctx context.Context, input AccountQuotaFetchInput, base string) (float64, error) {
	subscription, err := fetchQuotaJSON(ctx, p.client, input, base+"/v1/dashboard/billing/subscription")
	if err != nil {
		return 0, err
	}
	subscription = unwrapQuotaData(subscription)
	total, hasTotal := firstNumber(subscription, "hard_limit_usd", "system_hard_limit_usd", "soft_limit_usd")
	if hasTotal && total >= 0 && total != newAPIUnlimitedSentinel {
		return p.fetchBillingBalance(ctx, input, base, total)
	}
	if hasTotal && total == newAPIUnlimitedSentinel {
		return 0, errors.New("NewAPI 计费接口仍返回无限额度，无法仅凭 API Key 查询用户钱包余额")
	}
	return 0, errors.New("NewAPI 未返回可识别的账号余额")
}

func (p *newAPIQuotaProvider) fetchBillingBalance(ctx context.Context, input AccountQuotaFetchInput, base string, total float64) (float64, error) {
	usageEndpoint, err := url.Parse(base + "/v1/dashboard/billing/usage")
	if err != nil {
		return 0, errors.New("NewAPI 余额用量地址无效")
	}
	now := time.Now().UTC()
	query := usageEndpoint.Query()
	query.Set("start_date", "1970-01-01")
	query.Set("end_date", now.AddDate(0, 0, 1).Format("2006-01-02"))
	usageEndpoint.RawQuery = query.Encode()
	usage, err := fetchQuotaJSON(ctx, p.client, input, usageEndpoint.String())
	if err != nil {
		return 0, err
	}
	usage = unwrapQuotaData(usage)
	usedCents, hasUsed := firstNumber(usage, "total_usage")
	if !hasUsed || usedCents < 0 {
		return 0, errors.New("NewAPI 未返回可识别的账号用量")
	}
	balance := total - usedCents/100
	if balance < 0 {
		balance = 0
	}
	return balance, nil
}

func newAPIQuotaPerUSD(config, data map[string]any) float64 {
	if rate, ok := firstNumber(data, "quota_per_usd", "quota_per_unit"); ok && rate > 0 {
		return rate
	}
	if rate, ok := firstNumber(config, "quota_per_usd", "quota_per_unit"); ok && rate > 0 {
		return rate
	}
	return defaultNewAPIQuotaPerUSD
}

type newAPIQuotaDisplay struct {
	mode                 string
	unit                 string
	quotaPerUnit         float64
	usdExchangeRate      float64
	customCurrencyRate   float64
	customCurrencySymbol string
}

func (p *newAPIQuotaProvider) fetchQuotaDisplay(ctx context.Context, input AccountQuotaFetchInput, base string, quotaData map[string]any) newAPIQuotaDisplay {
	statusData := map[string]any{}
	if status, err := fetchQuotaJSON(ctx, p.client, input, base+"/api/status"); err == nil {
		statusData = unwrapQuotaData(status)
	}
	// /api/status is the authoritative display metadata endpoint. The token
	// response is used only as a fallback for older NewAPI versions.
	unit := strings.ToUpper(firstString(statusData, "quota_display_type", "unit", "currency"))
	if unit == "" {
		unit = strings.ToUpper(firstString(quotaData, "unit", "currency", "quota_display_type"))
	}
	if unit == "" {
		if displayCurrency, ok := statusData["display_in_currency"].(bool); ok && !displayCurrency {
			unit = "TOKENS"
		} else {
			unit = "USD"
		}
	}
	display := newAPIQuotaDisplay{
		mode:                 unit,
		unit:                 unit,
		quotaPerUnit:         newAPIQuotaPerUSD(input.Config, statusData),
		usdExchangeRate:      1,
		customCurrencyRate:   1,
		customCurrencySymbol: firstString(statusData, "custom_currency_symbol"),
	}
	if _, statusHasRate := firstNumber(statusData, "quota_per_usd", "quota_per_unit"); !statusHasRate {
		if rate, ok := firstNumber(quotaData, "quota_per_usd", "quota_per_unit"); ok && rate > 0 {
			display.quotaPerUnit = rate
		}
	}
	if rate, ok := firstNumber(statusData, "usd_exchange_rate"); ok && rate > 0 {
		display.usdExchangeRate = rate
	}
	if rate, ok := firstNumber(statusData, "custom_currency_exchange_rate"); ok && rate > 0 {
		display.customCurrencyRate = rate
	}
	if display.unit == "TOKEN" || display.unit == "TOKENS" {
		display.unit = "tokens"
	} else if display.unit == "CUSTOM" && display.customCurrencySymbol != "" {
		display.unit = display.customCurrencySymbol
	}
	return display
}

func (d newAPIQuotaDisplay) convert(value float64) float64 {
	switch strings.ToUpper(d.mode) {
	case "TOKEN", "TOKENS":
		return value
	case "CNY":
		return value / d.quotaPerUnit * d.usdExchangeRate
	case "CUSTOM":
		return value / d.quotaPerUnit * d.customCurrencyRate
	default:
		return value / d.quotaPerUnit
	}
}

func fetchQuotaJSON(ctx context.Context, client HTTPUpstream, input AccountQuotaFetchInput, endpoint string) (map[string]any, error) {
	if client == nil {
		return nil, errors.New("上游 HTTP 客户端不可用")
	}
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	requestContext := WithHTTPUpstreamRedirectsDisabled(reqCtx)
	req, err := newQuotaRequest(requestContext, input, endpoint)
	if err != nil {
		return nil, errors.New("创建配额查询请求失败")
	}
	resp, err := client.Do(req, input.ProxyURL, input.Account.ID, input.Account.Concurrency)
	if err != nil {
		return nil, errors.New("无法连接上游配额接口")
	}
	for redirectCount := 0; isQuotaRedirect(resp.StatusCode); redirectCount++ {
		if redirectCount >= 5 {
			resp.Body.Close()
			return nil, errors.New("上游配额接口重定向次数过多")
		}
		redirectURL, redirectErr := resolveQuotaRedirect(req.URL, resp.Header.Get("Location"))
		resp.Body.Close()
		if redirectErr != nil {
			return nil, redirectErr
		}
		req, err = newQuotaRequest(requestContext, input, redirectURL.String())
		if err != nil {
			return nil, errors.New("创建配额重定向请求失败")
		}
		resp, err = client.Do(req, input.ProxyURL, input.Account.ID, input.Account.Concurrency)
		if err != nil {
			return nil, errors.New("无法连接上游配额重定向接口")
		}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, accountQuotaBodyLimit+1))
	if err != nil || len(body) > accountQuotaBodyLimit {
		return nil, errors.New("读取上游配额响应失败")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("上游配额接口返回 HTTP %d", resp.StatusCode)
	}
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return nil, errors.New("上游配额响应不是有效 JSON")
	}
	if success, ok := payload["success"].(bool); ok && !success {
		return nil, errors.New("上游配额接口返回失败")
	}
	if success, ok := payload["code"].(bool); ok && !success {
		return nil, errors.New("上游配额接口返回失败")
	}
	return payload, nil
}

func newQuotaRequest(ctx context.Context, input AccountQuotaFetchInput, endpoint string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+input.APIKey)
	req.Header.Set("Accept", "application/json")
	if userID := identifierValue(input.Config["user_id"]); userID != "" {
		// Older NewAPI user-auth middleware requires this header. Current
		// TokenAuthReadOnly versions derive the user directly from the key.
		req.Header.Set("New-Api-User", userID)
	}
	return req, nil
}

func isQuotaRedirect(statusCode int) bool {
	return statusCode == http.StatusMovedPermanently ||
		statusCode == http.StatusFound ||
		statusCode == http.StatusSeeOther ||
		statusCode == http.StatusTemporaryRedirect ||
		statusCode == http.StatusPermanentRedirect
}

func resolveQuotaRedirect(source *url.URL, location string) (*url.URL, error) {
	if source == nil || strings.TrimSpace(location) == "" {
		return nil, errors.New("上游配额接口返回了无效重定向")
	}
	reference, err := url.Parse(location)
	if err != nil {
		return nil, errors.New("上游配额接口返回了无效重定向")
	}
	target := source.ResolveReference(reference)
	if target.RawQuery == "" && source.RawQuery != "" {
		target.RawQuery = source.RawQuery
	}
	sameScheme := strings.EqualFold(target.Scheme, source.Scheme)
	safeSchemeUpgrade := strings.EqualFold(source.Scheme, "http") && strings.EqualFold(target.Scheme, "https")
	if (!sameScheme && !safeSchemeUpgrade) || !strings.EqualFold(target.Host, source.Host) {
		return nil, errors.New("上游配额接口重定向到了不受信任的地址")
	}
	return target, nil
}

func unwrapQuotaData(payload map[string]any) map[string]any {
	if data, ok := mapValue(payload["data"]); ok {
		return data
	}
	return payload
}

func appendSub2APISubscriptionMetric(result *AccountQuotaResult, data map[string]any, key, label, period string) {
	limit, hasLimit := firstNumber(data, key+"_limit_usd")
	used, hasUsed := firstNumber(data, key+"_usage_usd")
	if !hasLimit && !hasUsed {
		return
	}
	metric := AccountQuotaMetric{Key: key, Label: label, Period: period, Unit: "USD"}
	if hasLimit {
		metric.Limit = floatPtr(limit)
	}
	if hasUsed {
		metric.Used = floatPtr(used)
	}
	if hasLimit {
		remaining := limit - used
		if remaining < 0 {
			remaining = 0
		}
		metric.Remaining = &remaining
		if limit > 0 {
			metric.Utilization = floatPtr(used / limit * 100)
		}
	}
	metric.ResetAt = firstTime(data, key+"_reset_at", key+"_resets_at")
	if metric.ResetAt == nil {
		if windowStart := firstTime(data, key+"_window_start"); windowStart != nil {
			windowDuration := map[string]time.Duration{
				"daily":   24 * time.Hour,
				"weekly":  7 * 24 * time.Hour,
				"monthly": 30 * 24 * time.Hour,
			}[key]
			if windowDuration > 0 {
				resetAt := windowStart.Add(windowDuration)
				metric.ResetAt = &resetAt
			}
		}
	}
	result.Metrics = append(result.Metrics, metric)
}

func applyLegacySub2APIResetFallbacks(subscription map[string]any, now time.Time) {
	if subscription == nil {
		return
	}
	if firstTime(subscription, "daily_reset_at", "daily_resets_at", "daily_window_start") == nil {
		if weeklyStart := firstTime(subscription, "weekly_window_start"); weeklyStart != nil {
			resetAt := nextPeriodicReset(*weeklyStart, 24*time.Hour, now)
			subscription["daily_reset_at"] = resetAt
		}
	}
	if firstTime(subscription, "monthly_reset_at", "monthly_resets_at", "monthly_window_start") == nil {
		if expiresAt := firstTime(subscription, "expires_at", "expired_at"); expiresAt != nil && expiresAt.After(now) {
			subscription["monthly_reset_at"] = *expiresAt
		}
	}
}

func nextPeriodicReset(start time.Time, period time.Duration, now time.Time) time.Time {
	resetAt := start.Add(period)
	if period <= 0 || resetAt.After(now) {
		return resetAt
	}
	periodsElapsed := now.Sub(start)/period + 1
	return start.Add(periodsElapsed * period)
}

func appendQuotaMetric(result *AccountQuotaResult, key, label, period string, data map[string]any) {
	limit, hasLimit := firstNumber(data, "limit", "quota", "total", "total_granted")
	used, hasUsed := firstNumber(data, "used", "usage", "total_used")
	remaining, hasRemaining := firstNumber(data, "remaining", "balance", "available", "total_available")
	if !hasUsed && hasLimit && hasRemaining {
		used, hasUsed = limit-remaining, true
	}
	if !hasRemaining && hasLimit && hasUsed {
		remaining, hasRemaining = limit-used, true
	}
	metric := AccountQuotaMetric{Key: key, Label: label, Period: period, Unit: firstString(data, "unit", "currency")}
	if metric.Unit == "" {
		metric.Unit = "USD"
	}
	if hasLimit {
		metric.Limit = floatPtr(limit)
	}
	if hasUsed {
		metric.Used = floatPtr(used)
	}
	if hasRemaining {
		metric.Remaining = floatPtr(remaining)
	}
	if hasLimit && limit > 0 && hasUsed {
		metric.Utilization = floatPtr(used / limit * 100)
	}
	metric.ResetAt = firstTime(data, "reset_at", "resets_at")
	if metric.Limit != nil || metric.Used != nil || metric.Remaining != nil {
		result.Metrics = append(result.Metrics, metric)
	}
}

func mapValue(value any) (map[string]any, bool) {
	result, ok := value.(map[string]any)
	return result, ok
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

func firstString(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringValue(data[key]); value != "" {
			return value
		}
	}
	return ""
}

func identifierValue(value any) string {
	if text := stringValue(value); text != "" {
		return text
	}
	if number, ok := firstNumber(map[string]any{"value": value}, "value"); ok && number > 0 && number == float64(int64(number)) {
		return strconv.FormatInt(int64(number), 10)
	}
	return ""
}

func firstIdentifier(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := identifierValue(data[key]); value != "" {
			return value
		}
	}
	return ""
}

func mergeSuggestedQuotaConfig(current, suggested map[string]any) (map[string]any, bool) {
	if len(suggested) == 0 {
		return current, false
	}
	merged := make(map[string]any, len(current)+len(suggested))
	for key, value := range current {
		merged[key] = value
	}
	changed := false
	for key, value := range suggested {
		if !isEmptyQuotaConfigValue(merged[key]) || value == nil {
			continue
		}
		merged[key] = value
		changed = true
	}
	return merged, changed
}

func isEmptyQuotaConfigValue(value any) bool {
	if value == nil {
		return true
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) == ""
	}
	return false
}

func firstNumber(data map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		switch value := data[key].(type) {
		case float64:
			return value, true
		case json.Number:
			number, err := value.Float64()
			return number, err == nil
		case string:
			number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			if err == nil {
				return number, true
			}
		case int:
			return float64(value), true
		case int64:
			return float64(value), true
		}
	}
	return 0, false
}

func timeValue(value any) *time.Time {
	if value == nil {
		return nil
	}
	switch typed := value.(type) {
	case time.Time:
		parsed := typed
		return &parsed
	case *time.Time:
		return typed
	}
	if text := stringValue(value); text != "" {
		if parsed, err := time.Parse(time.RFC3339, text); err == nil {
			return &parsed
		}
		if unix, err := strconv.ParseInt(text, 10, 64); err == nil && unix > 0 {
			parsed := time.Unix(unix, 0).UTC()
			return &parsed
		}
	}
	if unix, ok := firstNumber(map[string]any{"value": value}, "value"); ok && unix > 0 {
		if unix > 1e12 {
			unix /= 1000
		}
		parsed := time.Unix(int64(unix), 0).UTC()
		return &parsed
	}
	return nil
}

func firstTime(data map[string]any, keys ...string) *time.Time {
	for _, key := range keys {
		if value := timeValue(data[key]); value != nil {
			return value
		}
	}
	return nil
}
