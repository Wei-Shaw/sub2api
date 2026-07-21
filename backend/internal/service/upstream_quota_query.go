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
	"reflect"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

const upstreamQuotaStatusTimeout = 2 * time.Second

var (
	ErrUpstreamQuotaAccountInvalid = infraerrors.BadRequest(
		"UPSTREAM_QUOTA_ACCOUNT_INVALID", "account is not a configured OpenAI API key account",
	)
	ErrUpstreamQuotaUnsupported = infraerrors.New(
		http.StatusUnprocessableEntity, "UPSTREAM_QUOTA_UNSUPPORTED", "upstream quota protocol is unsupported",
	)
	ErrUpstreamQuotaAuthFailed = infraerrors.New(
		http.StatusBadGateway, "UPSTREAM_QUOTA_AUTH_FAILED", "upstream rejected the account API key",
	)
	ErrUpstreamQuotaRateLimited = infraerrors.ServiceUnavailable(
		"UPSTREAM_QUOTA_RATE_LIMITED", "upstream quota query was rate limited",
	)
	ErrUpstreamQuotaTimeout = infraerrors.GatewayTimeout(
		"UPSTREAM_QUOTA_TIMEOUT", "upstream quota query timed out",
	)
	ErrUpstreamQuotaInvalidResponse = infraerrors.New(
		http.StatusBadGateway, "UPSTREAM_QUOTA_INVALID_RESPONSE", "upstream returned an invalid quota response",
	)
	ErrUpstreamQuotaRequestFailed = infraerrors.New(
		http.StatusBadGateway, "UPSTREAM_QUOTA_REQUEST_FAILED", "upstream quota request failed",
	)
	ErrUpstreamQuotaIdentityChanged = infraerrors.Conflict(
		"UPSTREAM_QUOTA_IDENTITY_CHANGED", "account identity changed during upstream quota query; retry the query",
	)
)

type UpstreamQuotaQueryResult struct {
	AccountID  int64              `json:"account_id"`
	ObservedAt time.Time          `json:"observed_at"`
	Quota      *UpstreamQuotaInfo `json:"quota"`
}

type UpstreamQuotaInfo struct {
	Provider     string                    `json:"provider"`
	Mode         string                    `json:"mode"`
	Unit         string                    `json:"unit,omitempty"`
	Remaining    *float64                  `json:"remaining,omitempty"`
	Used         *float64                  `json:"used,omitempty"`
	Total        *float64                  `json:"total,omitempty"`
	ExpiresAt    *time.Time                `json:"expires_at,omitempty"`
	Windows      []UpstreamQuotaWindow     `json:"windows,omitempty"`
	Subscription *UpstreamSubscriptionInfo `json:"subscription,omitempty"`
}

type UpstreamSubscriptionInfo struct {
	PlanName  string                `json:"plan_name"`
	Remaining *float64              `json:"remaining,omitempty"`
	Unlimited bool                  `json:"unlimited,omitempty"`
	ExpiresAt time.Time             `json:"expires_at"`
	Windows   []UpstreamQuotaWindow `json:"windows,omitempty"`
}

type UpstreamQuotaWindow struct {
	Name      string     `json:"name"`
	Used      *float64   `json:"used,omitempty"`
	Limit     *float64   `json:"limit,omitempty"`
	Remaining *float64   `json:"remaining,omitempty"`
	ResetAt   *time.Time `json:"reset_at,omitempty"`
}

type upstreamQuotaQueryClient struct {
	upstream   HTTPUpstream
	account    *Account
	baseURL    string
	apiKey     string
	proxyURL   string
	tlsProfile *tlsfingerprint.Profile
}

type upstreamQuotaHTTPResponse struct {
	status int
	body   []byte
}

type sub2APIUsageResponse struct {
	Mode         string               `json:"mode"`
	IsValid      *bool                `json:"isValid"`
	Status       string               `json:"status"`
	PlanName     string               `json:"planName"`
	Unit         string               `json:"unit"`
	Remaining    *float64             `json:"remaining"`
	Balance      *float64             `json:"balance"`
	Quota        *sub2APIQuota        `json:"quota"`
	RateLimits   []sub2APIRateLimit   `json:"rate_limits"`
	Subscription *sub2APISubscription `json:"subscription"`
	ExpiresAt    *time.Time           `json:"expires_at"`
}

type sub2APIQuota struct {
	Limit     *float64 `json:"limit"`
	Used      *float64 `json:"used"`
	Remaining *float64 `json:"remaining"`
	Unit      string   `json:"unit"`
}

type sub2APIRateLimit struct {
	Window      string          `json:"window"`
	Limit       *float64        `json:"limit"`
	Used        *float64        `json:"used"`
	Remaining   *float64        `json:"remaining"`
	WindowStart json.RawMessage `json:"window_start"`
	ResetAt     *time.Time      `json:"reset_at"`
}

type sub2APISubscription struct {
	DailyUsageUSD      *float64   `json:"daily_usage_usd"`
	WeeklyUsageUSD     *float64   `json:"weekly_usage_usd"`
	MonthlyUsageUSD    *float64   `json:"monthly_usage_usd"`
	DailyLimitUSD      *float64   `json:"daily_limit_usd"`
	WeeklyLimitUSD     *float64   `json:"weekly_limit_usd"`
	MonthlyLimitUSD    *float64   `json:"monthly_limit_usd"`
	DailyWindowStart   *time.Time `json:"daily_window_start"`
	WeeklyWindowStart  *time.Time `json:"weekly_window_start"`
	MonthlyWindowStart *time.Time `json:"monthly_window_start"`
	DailyResetAt       *time.Time `json:"daily_reset_at"`
	WeeklyResetAt      *time.Time `json:"weekly_reset_at"`
	MonthlyResetAt     *time.Time `json:"monthly_reset_at"`
	Unlimited          *bool      `json:"unlimited"`
	ExpiresAt          *time.Time `json:"expires_at"`
}

type newAPISubscriptionResponse struct {
	Object             string          `json:"object"`
	HasPaymentMethod   *bool           `json:"has_payment_method"`
	SoftLimitUSD       *float64        `json:"soft_limit_usd"`
	HardLimitUSD       *float64        `json:"hard_limit_usd"`
	SystemHardLimitUSD *float64        `json:"system_hard_limit_usd"`
	AccessUntil        *int64          `json:"access_until"`
	Error              json.RawMessage `json:"error"`
}

type newAPIUsageResponse struct {
	Object     string          `json:"object"`
	TotalUsage *float64        `json:"total_usage"`
	Error      json.RawMessage `json:"error"`
}

type newAPIStatusMetadata struct {
	Version           string
	Logo              string
	QuotaDisplayType  *string
	DisplayInCurrency *bool
}

// QueryAccountQuota performs a fresh, transient query. Coalesced callers wait
// independently; canceling one waiter does not cancel the shared operation.
func (s *UpstreamBillingProbeService) QueryAccountQuota(ctx context.Context, accountID int64) (*UpstreamQuotaQueryResult, error) {
	if s == nil || s.accountRepo == nil || s.accountTestService == nil || s.accountTestService.httpUpstream == nil {
		return nil, ErrUpstreamBillingProbeUnavailable
	}
	if accountID <= 0 {
		return nil, ErrUpstreamQuotaAccountInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	resultCh := s.quotaGroup.DoChan(strconv.FormatInt(accountID, 10), func() (any, error) {
		opCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), upstreamBillingProbeRequestTimeout)
		defer cancel()
		return s.queryAccountQuota(opCtx, accountID)
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultCh:
		if result.Err != nil {
			return nil, result.Err
		}
		quota, ok := result.Val.(*UpstreamQuotaQueryResult)
		if !ok || quota == nil {
			return nil, ErrUpstreamQuotaInvalidResponse
		}
		return quota, nil
	}
}

func (s *UpstreamBillingProbeService) queryAccountQuota(ctx context.Context, accountID int64) (*UpstreamQuotaQueryResult, error) {
	select {
	case s.probeSlots <- struct{}{}:
		defer func() { <-s.probeSlots }()
	case <-ctx.Done():
		return nil, upstreamQuotaContextError(ctx)
	}

	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, upstreamQuotaRepositoryError(ctx, err)
	}
	client, err := s.newUpstreamQuotaQueryClient(account)
	if err != nil {
		return nil, err
	}
	quota, err := client.query(ctx)
	if err != nil {
		return nil, err
	}

	current, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, upstreamQuotaRepositoryError(ctx, err)
	}
	if !sameUpstreamQuotaIdentity(account, current) {
		return nil, ErrUpstreamQuotaIdentityChanged
	}
	return &UpstreamQuotaQueryResult{
		AccountID:  accountID,
		ObservedAt: s.currentTime().UTC(),
		Quota:      quota,
	}, nil
}

func (s *UpstreamBillingProbeService) newUpstreamQuotaQueryClient(account *Account) (*upstreamQuotaQueryClient, error) {
	if !isUpstreamBillingProbeAccount(account) {
		return nil, ErrUpstreamQuotaAccountInvalid
	}
	apiKey := strings.TrimSpace(account.GetOpenAIApiKey())
	if apiKey == "" {
		return nil, ErrUpstreamQuotaAccountInvalid
	}
	baseURL, err := s.accountTestService.validateUpstreamBaseURL(account.GetOpenAIBaseURL())
	if err != nil {
		return nil, ErrUpstreamQuotaAccountInvalid
	}
	proxyURL := ""
	if account.ProxyID != nil {
		if account.Proxy == nil {
			return nil, ErrUpstreamQuotaRequestFailed
		}
		if account.Proxy.ID != *account.ProxyID {
			return nil, ErrUpstreamQuotaIdentityChanged
		}
		proxyURL = account.Proxy.URL()
	}
	var tlsProfile *tlsfingerprint.Profile
	if s.accountTestService.tlsFPProfileService != nil {
		tlsProfile = s.accountTestService.tlsFPProfileService.ResolveTLSProfile(account)
	}
	return &upstreamQuotaQueryClient{
		upstream:   s.accountTestService.httpUpstream,
		account:    account,
		baseURL:    baseURL,
		apiKey:     apiKey,
		proxyURL:   proxyURL,
		tlsProfile: tlsProfile,
	}, nil
}

func (c *upstreamQuotaQueryClient) query(ctx context.Context) (*UpstreamQuotaInfo, error) {
	response, err := c.get(ctx, buildOpenAIEndpointURL(c.baseURL, "/v1/usage"), true)
	if err != nil {
		return nil, err
	}
	if response.status == http.StatusNotFound || response.status == http.StatusMethodNotAllowed {
		return c.queryNewAPI(ctx)
	}
	if err := upstreamQuotaHTTPError(response.status, false); err != nil {
		return nil, err
	}
	quota, err := parseSub2APIUsage(response.body)
	if err != nil {
		return nil, ErrUpstreamQuotaInvalidResponse
	}
	return quota, nil
}

func (c *upstreamQuotaQueryClient) queryNewAPI(ctx context.Context) (*UpstreamQuotaInfo, error) {
	subscriptionResponse, err := c.get(ctx, buildOpenAIEndpointURL(c.baseURL, "/v1/dashboard/billing/subscription"), true)
	if err != nil {
		return nil, err
	}
	if err := upstreamQuotaHTTPError(subscriptionResponse.status, true); err != nil {
		return nil, err
	}
	subscription, err := parseNewAPISubscription(subscriptionResponse.body)
	if err != nil {
		return nil, ErrUpstreamQuotaInvalidResponse
	}

	usageResponse, err := c.get(ctx, buildOpenAIEndpointURL(c.baseURL, "/v1/dashboard/billing/usage"), true)
	if err != nil {
		return nil, err
	}
	if err := upstreamQuotaHTTPError(usageResponse.status, false); err != nil {
		return nil, err
	}
	usage, err := parseNewAPIUsage(usageResponse.body)
	if err != nil {
		return nil, ErrUpstreamQuotaInvalidResponse
	}

	used := *usage.TotalUsage / 100
	total := *subscription.HardLimitUSD
	remaining := total - used
	quota := &UpstreamQuotaInfo{
		Provider:  "new_api",
		Mode:      "quota",
		Remaining: float64Ptr(remaining),
		Used:      float64Ptr(used),
		Total:     float64Ptr(total),
	}
	if *subscription.AccessUntil > 0 {
		expiresAt := time.Unix(*subscription.AccessUntil, 0).UTC()
		quota.ExpiresAt = &expiresAt
	}

	statusCtx, cancel := context.WithTimeout(ctx, upstreamQuotaStatusTimeout)
	defer cancel()
	quota.Unit = c.queryNewAPIUnit(statusCtx)
	return quota, nil
}

func (c *upstreamQuotaQueryClient) queryNewAPIUnit(ctx context.Context) string {
	statusURL, err := upstreamQuotaStatusURL(c.baseURL)
	if err != nil {
		return ""
	}
	response, err := c.get(ctx, statusURL, false)
	if err != nil || response.status < http.StatusOK || response.status >= http.StatusMultipleChoices {
		return ""
	}
	status, err := parseNewAPIStatusMetadata(response.body)
	if err != nil || status.QuotaDisplayType == nil {
		return ""
	}
	unit, ok := upstreamQuotaUnit(*status.QuotaDisplayType)
	if !ok {
		return ""
	}
	return unit
}

func parseNewAPIStatusMetadata(body []byte) (*newAPIStatusMetadata, error) {
	var status struct {
		Success *bool `json:"success"`
		Data    *struct {
			Version           string  `json:"version"`
			Logo              string  `json:"logo"`
			QuotaDisplayType  *string `json:"quota_display_type"`
			DisplayInCurrency *bool   `json:"display_in_currency"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &status); err != nil || status.Success == nil || !*status.Success || status.Data == nil {
		return nil, errors.New("invalid New API status response")
	}
	return &newAPIStatusMetadata{
		Version:           strings.TrimSpace(status.Data.Version),
		Logo:              strings.TrimSpace(status.Data.Logo),
		QuotaDisplayType:  status.Data.QuotaDisplayType,
		DisplayInCurrency: status.Data.DisplayInCurrency,
	}, nil
}

func (c *upstreamQuotaQueryClient) get(ctx context.Context, endpoint string, authenticated bool) (*upstreamQuotaHTTPResponse, error) {
	return c.getLimited(ctx, endpoint, authenticated, "application/json", upstreamBillingProbeMaxBodyBytes)
}

func (c *upstreamQuotaQueryClient) getLimited(ctx context.Context, endpoint string, authenticated bool, accept string, maxBodyBytes int64) (*upstreamQuotaHTTPResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, ErrUpstreamQuotaRequestFailed
	}
	reqCtx := WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI)
	req = req.WithContext(WithHTTPUpstreamRedirectsDisabled(reqCtx))
	req.Header.Set("Accept", accept)
	c.account.ApplyHeaderOverrides(req.Header)
	if authenticated {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	} else {
		req.Header.Del("Authorization")
	}

	resp, err := c.upstream.DoWithTLS(req, c.proxyURL, c.account.ID, c.account.Concurrency, c.tlsProfile)
	if err != nil {
		return nil, upstreamQuotaOperationError(ctx, err)
	}
	if resp == nil || resp.Body == nil {
		return nil, ErrUpstreamQuotaInvalidResponse
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	_ = resp.Body.Close()
	if readErr != nil {
		return nil, upstreamQuotaOperationError(ctx, readErr)
	}
	if int64(len(body)) > maxBodyBytes {
		return nil, ErrUpstreamQuotaInvalidResponse
	}
	return &upstreamQuotaHTTPResponse{status: resp.StatusCode, body: body}, nil
}

func parseSub2APIUsage(body []byte) (*UpstreamQuotaInfo, error) {
	var response sub2APIUsageResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	if response.IsValid == nil || !*response.IsValid {
		return nil, errors.New("invalid Sub2API key state")
	}
	switch response.Mode {
	case "quota_limited":
		return normalizeSub2APIQuotaLimited(&response)
	case "unrestricted":
		return normalizeSub2APIUnrestricted(&response)
	default:
		return nil, errors.New("unexpected Sub2API usage mode")
	}
}

func normalizeSub2APIQuotaLimited(response *sub2APIUsageResponse) (*UpstreamQuotaInfo, error) {
	if response.Status != StatusAPIKeyActive && response.Status != StatusAPIKeyQuotaExhausted && response.Status != StatusAPIKeyExpired {
		return nil, errors.New("invalid Sub2API key status")
	}
	windows, err := normalizeSub2APIRateLimits(response.RateLimits)
	if err != nil {
		return nil, err
	}
	subscription, err := normalizeSub2APISubscription(response.PlanName, response.Subscription, nil)
	if err != nil {
		return nil, err
	}
	if response.Quota == nil {
		if len(windows) == 0 || response.Remaining != nil || response.Unit != "" {
			return nil, errors.New("incomplete Sub2API rate limit response")
		}
		unit := ""
		if subscription != nil {
			unit = "USD"
		}
		return &UpstreamQuotaInfo{
			Provider: "sub2api", Mode: "rate_limits", Unit: unit, Windows: windows, Subscription: subscription,
		}, nil
	}

	quota := response.Quota
	if quota.Unit != "USD" || response.Unit != quota.Unit || quota.Limit == nil || quota.Used == nil || quota.Remaining == nil || response.Remaining == nil || *quota.Limit <= 0 ||
		!validNonNegativeQuotaNumber(*quota.Limit) || !validNonNegativeQuotaNumber(*quota.Used) ||
		!validNonNegativeQuotaNumber(*quota.Remaining) || !validNonNegativeQuotaNumber(*response.Remaining) ||
		!equalBillingMultiplier(*quota.Remaining, math.Max(0, *quota.Limit-*quota.Used)) ||
		!equalBillingMultiplier(*quota.Remaining, *response.Remaining) {
		return nil, errors.New("invalid Sub2API quota response")
	}
	expiresAt, err := normalizedQuotaTime(response.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return &UpstreamQuotaInfo{
		Provider:     "sub2api",
		Mode:         "quota",
		Unit:         "USD",
		Remaining:    quota.Remaining,
		Used:         quota.Used,
		Total:        quota.Limit,
		ExpiresAt:    expiresAt,
		Windows:      windows,
		Subscription: subscription,
	}, nil
}

func normalizeSub2APIUnrestricted(response *sub2APIUsageResponse) (*UpstreamQuotaInfo, error) {
	if response.Unit != "USD" || strings.TrimSpace(response.PlanName) == "" || response.Remaining == nil {
		return nil, errors.New("incomplete Sub2API unrestricted response")
	}
	if (response.Subscription == nil) == (response.Balance == nil) {
		return nil, errors.New("ambiguous Sub2API unrestricted response")
	}
	if response.Balance != nil {
		if !validQuotaNumber(*response.Balance) || !validQuotaNumber(*response.Remaining) ||
			!equalBillingMultiplier(*response.Balance, *response.Remaining) {
			return nil, errors.New("invalid Sub2API balance response")
		}
		return &UpstreamQuotaInfo{
			Provider: "sub2api", Mode: "balance", Unit: "USD", Remaining: response.Balance,
		}, nil
	}

	subscription, err := normalizeSub2APISubscription(response.PlanName, response.Subscription, response.Remaining)
	if err != nil || subscription == nil {
		return nil, errors.New("invalid Sub2API subscription")
	}
	if subscription.Unlimited && *response.Remaining != -1 {
		return nil, errors.New("inconsistent unlimited Sub2API subscription")
	}
	if !subscription.Unlimited && (!validNonNegativeQuotaNumber(*response.Remaining) ||
		subscription.Remaining == nil || !equalBillingMultiplier(*response.Remaining, *subscription.Remaining)) {
		return nil, errors.New("inconsistent Sub2API subscription remaining")
	}
	return &UpstreamQuotaInfo{
		Provider: "sub2api", Mode: "subscription", Unit: "USD", Subscription: subscription,
	}, nil
}

func normalizeSub2APIRateLimits(raw []sub2APIRateLimit) ([]UpstreamQuotaWindow, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(raw))
	windows := make([]UpstreamQuotaWindow, 0, len(raw))
	for _, item := range raw {
		if item.Window != "5h" && item.Window != "1d" && item.Window != "7d" {
			return nil, errors.New("unknown Sub2API rate limit window")
		}
		if _, exists := seen[item.Window]; exists {
			return nil, errors.New("duplicate Sub2API rate limit window")
		}
		seen[item.Window] = struct{}{}
		if item.Limit == nil || item.Used == nil || item.Remaining == nil || *item.Limit <= 0 ||
			!validNonNegativeQuotaNumber(*item.Limit) || !validNonNegativeQuotaNumber(*item.Used) ||
			!validNonNegativeQuotaNumber(*item.Remaining) ||
			!equalBillingMultiplier(*item.Remaining, math.Max(0, *item.Limit-*item.Used)) {
			return nil, errors.New("invalid Sub2API rate limit window")
		}
		if err := validateSub2APIWindowStart(item.WindowStart); err != nil {
			return nil, err
		}
		resetAt, err := normalizedQuotaTime(item.ResetAt)
		if err != nil {
			return nil, err
		}
		windows = append(windows, UpstreamQuotaWindow{
			Name: item.Window, Used: item.Used, Limit: item.Limit, Remaining: item.Remaining, ResetAt: resetAt,
		})
	}
	return windows, nil
}

func normalizeSub2APISubscription(planName string, subscription *sub2APISubscription, legacyRemaining *float64) (*UpstreamSubscriptionInfo, error) {
	if subscription == nil {
		return nil, nil
	}
	if strings.TrimSpace(planName) == "" || subscription.DailyUsageUSD == nil || subscription.WeeklyUsageUSD == nil || subscription.MonthlyUsageUSD == nil ||
		!validNonNegativeQuotaNumber(*subscription.DailyUsageUSD) ||
		!validNonNegativeQuotaNumber(*subscription.WeeklyUsageUSD) ||
		!validNonNegativeQuotaNumber(*subscription.MonthlyUsageUSD) {
		return nil, errors.New("invalid Sub2API subscription usage")
	}
	windows, err := normalizeSub2APISubscriptionWindows(subscription)
	if err != nil {
		return nil, err
	}
	expiresAt, err := normalizedQuotaTime(subscription.ExpiresAt)
	if err != nil || expiresAt == nil {
		return nil, errors.New("invalid Sub2API subscription expiry")
	}
	unlimited := subscription.Unlimited != nil && *subscription.Unlimited ||
		subscription.Unlimited == nil && legacyRemaining != nil && *legacyRemaining == -1
	if unlimited {
		if len(windows) != 0 {
			return nil, errors.New("inconsistent unlimited Sub2API subscription")
		}
		return &UpstreamSubscriptionInfo{
			PlanName: strings.TrimSpace(planName), Unlimited: true, ExpiresAt: *expiresAt,
		}, nil
	}
	if len(windows) == 0 {
		return nil, errors.New("missing Sub2API subscription limits")
	}
	minimumRemaining := *windows[0].Remaining
	for _, window := range windows[1:] {
		minimumRemaining = math.Min(minimumRemaining, *window.Remaining)
	}
	return &UpstreamSubscriptionInfo{
		PlanName: strings.TrimSpace(planName), Remaining: float64Ptr(minimumRemaining), ExpiresAt: *expiresAt, Windows: windows,
	}, nil
}

func normalizeSub2APISubscriptionWindows(subscription *sub2APISubscription) ([]UpstreamQuotaWindow, error) {
	type windowInput struct {
		name        string
		used        *float64
		limit       *float64
		resetAt     *time.Time
		legacyStart *time.Time
		duration    time.Duration
	}
	inputs := []windowInput{
		{name: "daily", used: subscription.DailyUsageUSD, limit: subscription.DailyLimitUSD, resetAt: subscription.DailyResetAt, legacyStart: subscription.DailyWindowStart, duration: 24 * time.Hour},
		{name: "weekly", used: subscription.WeeklyUsageUSD, limit: subscription.WeeklyLimitUSD, resetAt: subscription.WeeklyResetAt, legacyStart: subscription.WeeklyWindowStart, duration: 7 * 24 * time.Hour},
		{name: "monthly", used: subscription.MonthlyUsageUSD, limit: subscription.MonthlyLimitUSD, resetAt: subscription.MonthlyResetAt, legacyStart: subscription.MonthlyWindowStart, duration: 30 * 24 * time.Hour},
	}
	windows := make([]UpstreamQuotaWindow, 0, len(inputs))
	for _, input := range inputs {
		if input.limit == nil || *input.limit == 0 {
			continue
		}
		if *input.limit < 0 || !validNonNegativeQuotaNumber(*input.limit) {
			return nil, errors.New("invalid Sub2API subscription limit")
		}
		remaining := math.Max(0, *input.limit-*input.used)
		window := UpstreamQuotaWindow{Name: input.name, Used: input.used, Limit: input.limit, Remaining: float64Ptr(remaining)}
		resetAt, err := normalizedQuotaTime(input.resetAt)
		if err != nil {
			return nil, err
		}
		if resetAt == nil && input.legacyStart != nil {
			start, err := normalizedQuotaTime(input.legacyStart)
			if err != nil {
				return nil, err
			}
			legacyResetAt := start.Add(input.duration)
			resetAt = &legacyResetAt
		}
		window.ResetAt = resetAt
		windows = append(windows, window)
	}
	return windows, nil
}

func parseNewAPISubscription(body []byte) (*newAPISubscriptionResponse, error) {
	var response newAPISubscriptionResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	if hasUpstreamQuotaError(response.Error) || response.Object != "billing_subscription" ||
		response.HasPaymentMethod == nil || response.SoftLimitUSD == nil || response.HardLimitUSD == nil ||
		response.SystemHardLimitUSD == nil || response.AccessUntil == nil ||
		!validNonNegativeQuotaNumber(*response.SoftLimitUSD) || !validNonNegativeQuotaNumber(*response.HardLimitUSD) ||
		!validNonNegativeQuotaNumber(*response.SystemHardLimitUSD) || *response.AccessUntil < 0 || *response.AccessUntil > 253402300799 {
		return nil, errors.New("invalid New API subscription response")
	}
	return &response, nil
}

func parseNewAPIUsage(body []byte) (*newAPIUsageResponse, error) {
	var response newAPIUsageResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	if hasUpstreamQuotaError(response.Error) || response.Object != "list" || response.TotalUsage == nil ||
		!validNonNegativeQuotaNumber(*response.TotalUsage) {
		return nil, errors.New("invalid New API usage response")
	}
	return &response, nil
}

func hasUpstreamQuotaError(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null"
}

func upstreamQuotaHTTPError(status int, unsupported bool) error {
	if status >= http.StatusOK && status < http.StatusMultipleChoices {
		return nil
	}
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrUpstreamQuotaAuthFailed
	case http.StatusTooManyRequests:
		return ErrUpstreamQuotaRateLimited
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		if unsupported {
			return ErrUpstreamQuotaUnsupported
		}
	}
	return ErrUpstreamQuotaInvalidResponse
}

func upstreamQuotaOperationError(ctx context.Context, err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ErrUpstreamQuotaTimeout
	}
	return ErrUpstreamQuotaRequestFailed
}

func upstreamQuotaRepositoryError(ctx context.Context, err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ErrUpstreamQuotaTimeout
	}
	return err
}

func upstreamQuotaContextError(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ErrUpstreamQuotaTimeout
	}
	return ErrUpstreamQuotaRequestFailed
}

func upstreamQuotaStatusURL(base string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("invalid upstream base URL")
	}
	parsed.User = nil
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	path := strings.TrimRight(parsed.Path, "/")
	if openAIBaseURLHasVersionSuffix(path) {
		path = path[:strings.LastIndex(path, "/")]
	}
	parsed.Path = strings.TrimRight(path, "/") + "/api/status"
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.ForceQuery = false
	return parsed.String(), nil
}

func sameUpstreamQuotaIdentity(expected, current *Account) bool {
	if !sameUpstreamProtocolIdentity(expected, current) || expected.Concurrency != current.Concurrency {
		return false
	}
	return true
}

func sameUpstreamProtocolIdentity(expected, current *Account) bool {
	if expected == nil || current == nil || expected.Platform != current.Platform || expected.Type != current.Type ||
		!reflect.DeepEqual(expected.Credentials, current.Credentials) || !sameOptionalInt64(expected.ProxyID, current.ProxyID) ||
		!sameUpstreamQuotaProxy(expected.ProxyID, expected.Proxy, current.Proxy) {
		return false
	}
	for _, key := range []string{"enable_tls_fingerprint", "tls_fingerprint_profile_id"} {
		expectedValue, expectedOK := expected.Extra[key]
		currentValue, currentOK := current.Extra[key]
		if expectedOK != currentOK || !reflect.DeepEqual(expectedValue, currentValue) {
			return false
		}
	}
	return true
}

func sameOptionalInt64(left, right *int64) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func sameUpstreamQuotaProxy(proxyID *int64, expected, current *Proxy) bool {
	if proxyID == nil {
		return true
	}
	return expected != nil && current != nil && expected.ID == *proxyID && current.ID == *proxyID &&
		expected.Protocol == current.Protocol && expected.Host == current.Host && expected.Port == current.Port &&
		expected.Username == current.Username && expected.Password == current.Password && expected.Status == current.Status
}

func upstreamQuotaUnit(raw string) (string, bool) {
	unit := strings.TrimSpace(raw)
	switch unit {
	case "USD", "CNY", "TOKENS":
		return unit, true
	default:
		return "", false
	}
}

func validQuotaNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func validNonNegativeQuotaNumber(value float64) bool {
	return value >= 0 && validQuotaNumber(value)
}

func normalizedQuotaTime(value *time.Time) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	if value.IsZero() {
		return nil, fmt.Errorf("invalid quota timestamp")
	}
	normalized := value.UTC()
	return &normalized, nil
}

func validateSub2APIWindowStart(raw json.RawMessage) error {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return errors.New("missing Sub2API rate limit window_start")
	}
	if trimmed == "null" {
		return nil
	}
	var value time.Time
	if err := json.Unmarshal(raw, &value); err != nil || value.IsZero() {
		return errors.New("invalid Sub2API rate limit window_start")
	}
	return nil
}
