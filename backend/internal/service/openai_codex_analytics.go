package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

const (
	chatGPTCodexProfileURL                                             = "https://chatgpt.com/backend-api/wham/profiles/me"
	openAICodexAnalyticsTTL                                            = 4 * time.Minute
	openAICodexAnalyticsFetchTimeout                                   = 2*openaiQuotaUpstreamTimeout + 10*time.Second
	openAICodexAnalyticsMax                                            = 30
	openAICodexAnalyticsSevenDaySeconds                                = int64(7 * 24 * time.Hour / time.Second)
	OpenAICodexAnalyticsCurrent7Days    OpenAICodexAnalyticsPeriodMode = "current_7d"
	OpenAICodexAnalyticsRecent          OpenAICodexAnalyticsPeriodMode = "recent"
)

type codexAnalyticsUsageRepository interface {
	GetAccountStatsAggregated(ctx context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.UsageStats, error)
	GetUsageTrendWithFilters(ctx context.Context, startTime, endTime time.Time, granularity string, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8) ([]usagestats.TrendDataPoint, error)
	GetModelStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8) ([]usagestats.ModelStat, error)
}
type OpenAICodexAnalyticsPeriodMode string

type OpenAICodexAnalyticsQuery struct {
	Period OpenAICodexAnalyticsPeriodMode
	Days   int
}

type OpenAICodexAnalytics struct {
	AccountID   int64                          `json:"account_id"`
	PeriodDays  int                            `json:"period_days"`
	PeriodMode  OpenAICodexAnalyticsPeriodMode `json:"period_mode"`
	PeriodStart int64                          `json:"period_start"`
	PeriodEnd   int64                          `json:"period_end"`
	FetchedAt   int64                          `json:"fetched_at"`
	DataScope   OpenAICodexAnalyticsDataScope  `json:"data_scope"`
	Cache       OpenAICodexAnalyticsCache      `json:"cache"`
	RateLimits  OpenAICodexAnalyticsRateLimits `json:"rate_limits"`
	Profile     OpenAICodexAnalyticsProfile    `json:"profile"`
	Summary     OpenAICodexAnalyticsSummary    `json:"summary"`
	TimeSeries  []OpenAICodexAnalyticsDay      `json:"time_series"`
	Models      []OpenAICodexAnalyticsModel    `json:"models"`
	Warnings    []OpenAICodexAnalyticsWarning  `json:"warnings,omitempty"`
}

type OpenAICodexAnalyticsDataScope struct {
	OfficialAccountActivity string `json:"official_account_activity"`
	ManagedTraffic          string `json:"managed_traffic"`
	RateLimits              string `json:"rate_limits"`
}

type OpenAICodexAnalyticsCache struct {
	Hit        bool  `json:"hit"`
	TTLSeconds int64 `json:"ttl_seconds"`
	ExpiresAt  int64 `json:"expires_at"`
}

type OpenAICodexAnalyticsRateLimits struct {
	FiveHour *OpenAICodexAnalyticsRateLimitWindow `json:"five_hour,omitempty"`
	SevenDay *OpenAICodexAnalyticsRateLimitWindow `json:"seven_day,omitempty"`
}

type OpenAICodexAnalyticsRateLimitWindow struct {
	UsedPercent        float64 `json:"used_percent"`
	LimitWindowSeconds int64   `json:"limit_window_seconds"`
	ResetAfterSeconds  int64   `json:"reset_after_seconds"`
	ResetAt            int64   `json:"reset_at"`
	Allowed            bool    `json:"allowed"`
	LimitReached       bool    `json:"limit_reached"`
}

type OpenAICodexDailyUsageBucket struct {
	StartDate string `json:"start_date"`
	Tokens    int64  `json:"tokens"`
}

type OpenAICodexAnalyticsProfile struct {
	LifetimeTokens            *int64                        `json:"lifetime_tokens,omitempty"`
	PeakDailyTokens           *int64                        `json:"peak_daily_tokens,omitempty"`
	LongestRunningTurnSeconds *int64                        `json:"longest_running_turn_seconds,omitempty"`
	CurrentStreakDays         *int64                        `json:"current_streak_days,omitempty"`
	LongestStreakDays         *int64                        `json:"longest_streak_days,omitempty"`
	DailyUsageBuckets         []OpenAICodexDailyUsageBucket `json:"daily_usage_buckets"`
}

type OpenAICodexAnalyticsSummary struct {
	OfficialTotalTokens     *int64  `json:"official_total_tokens,omitempty"`
	ManagedTotalTokens      int64   `json:"managed_total_tokens"`
	InputTokens             int64   `json:"input_tokens"`
	OutputTokens            int64   `json:"output_tokens"`
	CacheTokens             int64   `json:"cache_tokens"`
	CacheReadTokens         int64   `json:"cache_read_tokens"`
	CacheHitRate            float64 `json:"cache_hit_rate"`
	EstimatedCost           float64 `json:"estimated_cost"`
	Requests                int64   `json:"requests"`
	CurrentLimitUsedPercent float64 `json:"current_limit_used_percent"`
}

type OpenAICodexAnalyticsDay struct {
	Date                string  `json:"date"`
	OfficialTotalTokens *int64  `json:"official_total_tokens,omitempty"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheTokens         int64   `json:"cache_tokens"`
	TotalTokens         int64   `json:"total_tokens"`
	Requests            int64   `json:"requests"`
	EstimatedCost       float64 `json:"estimated_cost"`
}

type OpenAICodexAnalyticsModel struct {
	Model         string  `json:"model"`
	InputTokens   int64   `json:"input_tokens"`
	OutputTokens  int64   `json:"output_tokens"`
	CacheTokens   int64   `json:"cache_tokens"`
	TotalTokens   int64   `json:"total_tokens"`
	Requests      int64   `json:"requests"`
	EstimatedCost float64 `json:"estimated_cost"`
}

type OpenAICodexAnalyticsWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type codexTokenUsageProfileResponse struct {
	Stats codexTokenUsageProfileStats `json:"stats"`
}

type codexTokenUsageProfileStats struct {
	LifetimeTokens        *int64                        `json:"lifetime_tokens"`
	PeakDailyTokens       *int64                        `json:"peak_daily_tokens"`
	LongestRunningTurnSec *int64                        `json:"longest_running_turn_sec"`
	CurrentStreakDays     *int64                        `json:"current_streak_days"`
	LongestStreakDays     *int64                        `json:"longest_streak_days"`
	DailyUsageBuckets     []OpenAICodexDailyUsageBucket `json:"daily_usage_buckets"`
}

func (s *OpenAIQuotaService) QueryCodexAnalytics(ctx context.Context, accountID int64, query OpenAICodexAnalyticsQuery) (*OpenAICodexAnalytics, error) {
	query, err := normalizeCodexAnalyticsQuery(query)
	if err != nil {
		return nil, err
	}
	if s == nil || s.analyticsUsageRepo == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_CODEX_ANALYTICS_NOT_CONFIGURED", "codex analytics service is not configured")
	}

	key := openAICodexAnalyticsCacheKey(accountID, query)
	cached, cacheWarning := s.readCodexAnalyticsCache(ctx, key, query)
	if cached != nil {
		return cached, nil
	}

	resultCh := s.analyticsFlight.DoChan(key, func() (any, error) {
		sharedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), openAICodexAnalyticsFetchTimeout)
		defer cancel()

		if cached, _ := s.readCodexAnalyticsCache(sharedCtx, key, query); cached != nil {
			return cached, nil
		}
		return s.fetchCodexAnalytics(sharedCtx, accountID, query, key, cacheWarning)
	})

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case shared := <-resultCh:
		if shared.Err != nil {
			return nil, shared.Err
		}
		result, ok := shared.Val.(*OpenAICodexAnalytics)
		if !ok || result == nil {
			return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_CODEX_ANALYTICS_EMPTY_RESULT", "codex analytics query returned an empty result")
		}
		return result, nil
	}
}

func normalizeCodexAnalyticsQuery(query OpenAICodexAnalyticsQuery) (OpenAICodexAnalyticsQuery, error) {
	if query.Period == "" {
		query.Period = OpenAICodexAnalyticsCurrent7Days
	}
	switch query.Period {
	case OpenAICodexAnalyticsCurrent7Days:
		query.Days = 7
	case OpenAICodexAnalyticsRecent:
		if query.Days < 1 || query.Days > openAICodexAnalyticsMax {
			return OpenAICodexAnalyticsQuery{}, infraerrors.New(http.StatusBadRequest, "OPENAI_CODEX_ANALYTICS_INVALID_DAYS", "days must be between 1 and 30")
		}
	default:
		return OpenAICodexAnalyticsQuery{}, infraerrors.New(http.StatusBadRequest, "OPENAI_CODEX_ANALYTICS_INVALID_PERIOD", "period must be current_7d or recent")
	}
	return query, nil
}

func (s *OpenAIQuotaService) fetchCodexAnalytics(ctx context.Context, accountID int64, query OpenAICodexAnalyticsQuery, cacheKey string, cacheWarning *OpenAICodexAnalyticsWarning) (*OpenAICodexAnalytics, error) {
	now := time.Now().UTC().Truncate(time.Second)
	warnings := make([]OpenAICodexAnalyticsWarning, 0, 5)
	if cacheWarning != nil {
		warnings = append(warnings, *cacheWarning)
	}

	quota, err := s.QueryUsage(ctx, accountID)
	if err != nil {
		status, _ := infraerrors.ToHTTP(err)
		if status == http.StatusUnauthorized || status == http.StatusTooManyRequests {
			return nil, err
		}
		warnings = append(warnings, OpenAICodexAnalyticsWarning{Code: "rate_limits_unavailable", Message: "Current ChatGPT rate limits are temporarily unavailable."})
		quota = nil
	}
	rateLimits := normalizeCodexAnalyticsRateLimits(quota)
	start, end, periodWarning := resolveCodexAnalyticsPeriod(query, rateLimits, now)
	if periodWarning != nil {
		warnings = append(warnings, *periodWarning)
	}

	profile, err := s.queryCodexTokenUsageProfile(ctx, accountID)
	profileAvailable := err == nil
	if err != nil {
		status, _ := infraerrors.ToHTTP(err)
		if status == http.StatusUnauthorized || status == http.StatusTooManyRequests {
			return nil, err
		}
		warnings = append(warnings, OpenAICodexAnalyticsWarning{Code: "official_profile_unavailable", Message: "Official account activity is temporarily unavailable."})
		profile = &OpenAICodexAnalyticsProfile{DailyUsageBuckets: []OpenAICodexDailyUsageBucket{}}
	}
	if profileAvailable && codexAnalyticsHasPartialDayBoundary(start, end) {
		warnings = append(warnings, OpenAICodexAnalyticsWarning{Code: "official_daily_buckets_approximate_period", Message: "Official account activity is available only in daily buckets and may approximate partial-day period boundaries."})
	}

	stats, err := s.analyticsUsageRepo.GetAccountStatsAggregated(ctx, accountID, start, end)
	if err != nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_CODEX_ANALYTICS_LOCAL_QUERY_FAILED", "failed to query managed traffic summary").WithCause(err)
	}
	trend, err := s.analyticsUsageRepo.GetUsageTrendWithFilters(ctx, start, end, "day", 0, 0, accountID, 0, "", nil, nil, nil)
	if err != nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_CODEX_ANALYTICS_LOCAL_QUERY_FAILED", "failed to query managed traffic timeline").WithCause(err)
	}
	modelStats, err := s.analyticsUsageRepo.GetModelStatsWithFilters(ctx, start, end, 0, 0, accountID, 0, nil, nil, nil)
	if err != nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_CODEX_ANALYTICS_LOCAL_QUERY_FAILED", "failed to query managed traffic models").WithCause(err)
	}

	filterCodexProfileBuckets(profile, start, end)
	officialPeriodTotal := sumCodexOfficialPeriodTokens(profile, profileAvailable)
	result := &OpenAICodexAnalytics{
		AccountID:   accountID,
		PeriodDays:  query.Days,
		PeriodMode:  query.Period,
		PeriodStart: start.Unix(),
		PeriodEnd:   end.Unix(),
		FetchedAt:   now.Unix(),
		DataScope: OpenAICodexAnalyticsDataScope{
			OfficialAccountActivity: "chatgpt_profile",
			ManagedTraffic:          "sub2api_usage_logs",
			RateLimits:              "chatgpt_wham_usage",
		},
		Cache:      OpenAICodexAnalyticsCache{},
		RateLimits: rateLimits,
		Profile:    *profile,
		Summary:    buildCodexAnalyticsSummary(stats, officialPeriodTotal, rateLimits),
		TimeSeries: buildCodexAnalyticsTimeSeries(start, end, trend, profile.DailyUsageBuckets),
		Models:     buildCodexAnalyticsModels(modelStats),
		Warnings:   warnings,
	}

	if warning := s.writeCodexAnalyticsCache(ctx, cacheKey, result); warning != nil {
		result.Warnings = append(result.Warnings, *warning)
	}
	return result, nil
}

func resolveCodexAnalyticsPeriod(query OpenAICodexAnalyticsQuery, limits OpenAICodexAnalyticsRateLimits, now time.Time) (time.Time, time.Time, *OpenAICodexAnalyticsWarning) {
	if query.Period == OpenAICodexAnalyticsRecent {
		return now.Add(-time.Duration(query.Days) * 24 * time.Hour), now, nil
	}
	if window := limits.SevenDay; window != nil && window.LimitWindowSeconds == openAICodexAnalyticsSevenDaySeconds && window.ResetAt > 0 {
		resetAt := time.Unix(window.ResetAt, 0).UTC()
		start := resetAt.Add(-time.Duration(openAICodexAnalyticsSevenDaySeconds) * time.Second)
		if resetAt.After(now) && start.Before(now) {
			return start, now, nil
		}
	}
	return now.Add(-time.Duration(openAICodexAnalyticsSevenDaySeconds) * time.Second), now, &OpenAICodexAnalyticsWarning{
		Code:    "current_7d_window_unavailable",
		Message: "The official seven-day window was unavailable or invalid; recent seven-day analytics were returned.",
	}
}

func codexAnalyticsHasPartialDayBoundary(start, end time.Time) bool {
	return !start.Equal(start.Truncate(24*time.Hour)) || !end.Equal(end.Truncate(24*time.Hour))
}

func (s *OpenAIQuotaService) queryCodexTokenUsageProfile(ctx context.Context, accountID int64) (*OpenAICodexAnalyticsProfile, error) {
	accessToken, chatGPTAccountID, proxyURL, fedRAMP, err := s.prepareUpstreamCall(ctx, accountID)
	if err != nil {
		return nil, err
	}
	client, err := s.privacyClientFactory(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_PROFILE_CLIENT_ERROR", "failed to build upstream client: %v", err)
	}
	callCtx, cancel := context.WithTimeout(ctx, openaiQuotaUpstreamTimeout)
	defer cancel()
	agentIdentity := s.isAgentIdentityAccount(ctx, accountID)

	var payload codexTokenUsageProfileResponse
	for recovered := false; ; {
		headers, expectedTaskID, headerErr := s.buildCodexQuotaHeaders(callCtx, accountID, accessToken, chatGPTAccountID, fedRAMP)
		if headerErr != nil {
			return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_PROFILE_AUTH_FAILED", "failed to build upstream authentication: %v", headerErr)
		}
		resp, requestErr := client.R().SetContext(callCtx).SetHeaders(headers).SetSuccessResult(&payload).Get(chatGPTCodexProfileURL)
		if requestErr != nil {
			return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_PROFILE_REQUEST_FAILED", "upstream request failed: %v", requestErr)
		}
		if resp.IsSuccessState() {
			break
		}
		if agentIdentity && !recovered && isAgentIdentityTaskInvalidHTTPResponse(resp.StatusCode, resp.Bytes()) {
			recovered = true
			if recoverErr := s.recoverAgentIdentityTask(ctx, accountID, expectedTaskID); recoverErr != nil {
				return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_PROFILE_AUTH_FAILED", "agent identity task recovery failed: %v", recoverErr)
			}
			continue
		}
		status := resp.StatusCode
		body := truncate(logredact.RedactText(s.redactQuotaErrorBody(callCtx, accountID, resp.String())), 240)
		slog.Warn("openai_codex_profile_query_failed", "account_id", accountID, "status", status, "body", body)
		switch status {
		case http.StatusUnauthorized:
			return nil, infraerrors.Newf(http.StatusUnauthorized, "OPENAI_CODEX_PROFILE_UNAUTHORIZED", "official profile authentication failed: %s", body)
		case http.StatusTooManyRequests:
			return nil, infraerrors.Newf(http.StatusTooManyRequests, "OPENAI_CODEX_PROFILE_RATE_LIMITED", "official profile rate limited: %s", body)
		default:
			return nil, infraerrors.Newf(mapUpstreamStatus(status), "OPENAI_CODEX_PROFILE_UPSTREAM_ERROR", "official profile returned %d: %s", status, body)
		}
	}

	return &OpenAICodexAnalyticsProfile{
		LifetimeTokens:            payload.Stats.LifetimeTokens,
		PeakDailyTokens:           payload.Stats.PeakDailyTokens,
		LongestRunningTurnSeconds: payload.Stats.LongestRunningTurnSec,
		CurrentStreakDays:         payload.Stats.CurrentStreakDays,
		LongestStreakDays:         payload.Stats.LongestStreakDays,
		DailyUsageBuckets:         append(make([]OpenAICodexDailyUsageBucket, 0, len(payload.Stats.DailyUsageBuckets)), payload.Stats.DailyUsageBuckets...),
	}, nil
}

func (s *OpenAIQuotaService) readCodexAnalyticsCache(ctx context.Context, key string, query OpenAICodexAnalyticsQuery) (*OpenAICodexAnalytics, *OpenAICodexAnalyticsWarning) {
	if s == nil || s.analyticsCache == nil {
		return nil, nil
	}
	raw, ttl, err := s.analyticsCache.Get(ctx, key)
	if errors.Is(err, ErrCodexAnalyticsCacheMiss) {
		return nil, nil
	}
	if err != nil {
		slog.Warn("openai_codex_analytics_cache_read_failed", "error", err)
		return nil, &OpenAICodexAnalyticsWarning{Code: "cache_read_failed", Message: "Analytics cache was unavailable; fresh data was fetched."}
	}
	var cached OpenAICodexAnalytics
	if err := json.Unmarshal(raw, &cached); err != nil {
		slog.Warn("openai_codex_analytics_cache_decode_failed", "error", err)
		_ = s.analyticsCache.Delete(ctx, key)
		return nil, &OpenAICodexAnalyticsWarning{Code: "cache_read_failed", Message: "Analytics cache was invalid; fresh data was fetched."}
	}
	if query.Period == OpenAICodexAnalyticsCurrent7Days && cached.RateLimits.SevenDay != nil && cached.RateLimits.SevenDay.ResetAt > 0 && cached.RateLimits.SevenDay.ResetAt <= time.Now().Unix() {
		_ = s.analyticsCache.Delete(ctx, key)
		return nil, nil
	}
	if ttl <= 0 {
		return nil, nil
	}
	cached.Cache = OpenAICodexAnalyticsCache{
		Hit:        true,
		TTLSeconds: int64(ttl / time.Second),
		ExpiresAt:  time.Now().Add(ttl).Unix(),
	}
	return &cached, nil
}

func (s *OpenAIQuotaService) writeCodexAnalyticsCache(ctx context.Context, key string, result *OpenAICodexAnalytics) *OpenAICodexAnalyticsWarning {
	if s == nil || s.analyticsCache == nil || result == nil {
		return nil
	}
	ttl := openAICodexAnalyticsTTL
	if result.PeriodMode == OpenAICodexAnalyticsCurrent7Days && result.RateLimits.SevenDay != nil && result.RateLimits.SevenDay.ResetAt > 0 {
		untilReset := time.Until(time.Unix(result.RateLimits.SevenDay.ResetAt, 0))
		if untilReset <= 0 {
			result.Cache = OpenAICodexAnalyticsCache{}
			return nil
		}
		if untilReset < ttl {
			ttl = untilReset
		}
	}
	result.Cache = OpenAICodexAnalyticsCache{TTLSeconds: int64(ttl / time.Second), ExpiresAt: time.Now().Add(ttl).Unix()}
	raw, err := json.Marshal(result)
	if err == nil {
		err = s.analyticsCache.Set(ctx, key, raw, ttl)
	}
	if err == nil {
		return nil
	}
	result.Cache = OpenAICodexAnalyticsCache{}
	slog.Warn("openai_codex_analytics_cache_write_failed", "error", err)
	return &OpenAICodexAnalyticsWarning{Code: "cache_write_failed", Message: "Fresh analytics were returned without caching."}
}

func openAICodexAnalyticsCacheKey(accountID int64, query OpenAICodexAnalyticsQuery) string {
	if query.Period == OpenAICodexAnalyticsRecent {
		return fmt.Sprintf("codex_analytics:v2:%d:recent:%d", accountID, query.Days)
	}
	return fmt.Sprintf("codex_analytics:v2:%d:current_7d", accountID)
}

func normalizeCodexAnalyticsRateLimits(usage *OpenAIQuotaUsage) OpenAICodexAnalyticsRateLimits {
	if usage == nil || usage.RateLimit == nil {
		return OpenAICodexAnalyticsRateLimits{}
	}
	rateLimit := usage.RateLimit
	windows := make([]*OpenAIRateLimitWindow, 0, 2)
	if rateLimit.PrimaryWindow != nil {
		windows = append(windows, rateLimit.PrimaryWindow)
	}
	if rateLimit.SecondaryWindow != nil {
		windows = append(windows, rateLimit.SecondaryWindow)
	}
	sort.SliceStable(windows, func(i, j int) bool { return windows[i].LimitWindowSeconds < windows[j].LimitWindowSeconds })
	toAnalyticsWindow := func(window *OpenAIRateLimitWindow) *OpenAICodexAnalyticsRateLimitWindow {
		return &OpenAICodexAnalyticsRateLimitWindow{
			UsedPercent:        window.UsedPercent,
			LimitWindowSeconds: window.LimitWindowSeconds,
			ResetAfterSeconds:  window.ResetAfterSeconds,
			ResetAt:            window.ResetAt,
			Allowed:            rateLimit.Allowed,
			LimitReached:       rateLimit.LimitReached,
		}
	}
	result := OpenAICodexAnalyticsRateLimits{}
	for _, window := range windows {
		switch {
		case window.LimitWindowSeconds == openAICodexAnalyticsSevenDaySeconds:
			result.SevenDay = toAnalyticsWindow(window)
		case result.FiveHour == nil && window.LimitWindowSeconds <= int64(6*time.Hour/time.Second):
			result.FiveHour = toAnalyticsWindow(window)
		}
	}
	return result
}

func buildCodexAnalyticsSummary(stats *usagestats.UsageStats, officialPeriodTotal *int64, limits OpenAICodexAnalyticsRateLimits) OpenAICodexAnalyticsSummary {
	result := OpenAICodexAnalyticsSummary{OfficialTotalTokens: officialPeriodTotal}
	if stats != nil {
		result.ManagedTotalTokens = stats.TotalTokens
		result.InputTokens = stats.TotalInputTokens
		result.OutputTokens = stats.TotalOutputTokens
		result.CacheTokens = stats.TotalCacheTokens
		result.CacheReadTokens = stats.TotalCacheReadTokens
		result.EstimatedCost = stats.TotalCost
		result.Requests = stats.TotalRequests
		denominator := stats.TotalInputTokens + stats.TotalCacheReadTokens
		if denominator > 0 {
			result.CacheHitRate = float64(stats.TotalCacheReadTokens) / float64(denominator) * 100
		}
	}
	for _, window := range []*OpenAICodexAnalyticsRateLimitWindow{limits.FiveHour, limits.SevenDay} {
		if window != nil && window.UsedPercent > result.CurrentLimitUsedPercent {
			result.CurrentLimitUsedPercent = window.UsedPercent
		}
	}
	return result
}

func filterCodexProfileBuckets(profile *OpenAICodexAnalyticsProfile, start, end time.Time) {
	if profile == nil {
		return
	}
	filtered := make([]OpenAICodexDailyUsageBucket, 0, len(profile.DailyUsageBuckets))
	for _, bucket := range profile.DailyUsageBuckets {
		date := codexBucketDate(bucket.StartDate)
		parsed, err := time.Parse("2006-01-02", date)
		if err == nil && parsed.AddDate(0, 0, 1).After(start) && parsed.Before(end) {
			bucket.StartDate = date
			filtered = append(filtered, bucket)
		}
	}
	profile.DailyUsageBuckets = filtered
}

func sumCodexOfficialPeriodTokens(profile *OpenAICodexAnalyticsProfile, available bool) *int64 {
	if !available || profile == nil {
		return nil
	}
	total := int64(0)
	for _, bucket := range profile.DailyUsageBuckets {
		total += bucket.Tokens
	}
	return &total
}

func buildCodexAnalyticsTimeSeries(start, end time.Time, trend []usagestats.TrendDataPoint, official []OpenAICodexDailyUsageBucket) []OpenAICodexAnalyticsDay {
	managedByDate := make(map[string]usagestats.TrendDataPoint, len(trend))
	for _, point := range trend {
		managedByDate[point.Date] = point
	}
	officialByDate := make(map[string]int64, len(official))
	for _, bucket := range official {
		officialByDate[codexBucketDate(bucket.StartDate)] = bucket.Tokens
	}
	firstDay := start.UTC().Truncate(24 * time.Hour)
	series := make([]OpenAICodexAnalyticsDay, 0, int(end.Sub(firstDay)/(24*time.Hour))+1)
	for day := firstDay; day.Before(end); day = day.AddDate(0, 0, 1) {
		date := day.Format("2006-01-02")
		point := managedByDate[date]
		row := OpenAICodexAnalyticsDay{
			Date:          date,
			InputTokens:   point.InputTokens,
			OutputTokens:  point.OutputTokens,
			CacheTokens:   point.CacheCreationTokens + point.CacheReadTokens,
			TotalTokens:   point.TotalTokens,
			Requests:      point.Requests,
			EstimatedCost: point.Cost,
		}
		if tokens, ok := officialByDate[date]; ok {
			row.OfficialTotalTokens = &tokens
		}
		series = append(series, row)
	}
	return series
}

func buildCodexAnalyticsModels(stats []usagestats.ModelStat) []OpenAICodexAnalyticsModel {
	models := make([]OpenAICodexAnalyticsModel, 0, len(stats))
	for _, model := range stats {
		models = append(models, OpenAICodexAnalyticsModel{
			Model:         model.Model,
			InputTokens:   model.InputTokens,
			OutputTokens:  model.OutputTokens,
			CacheTokens:   model.CacheCreationTokens + model.CacheReadTokens,
			TotalTokens:   model.TotalTokens,
			Requests:      model.Requests,
			EstimatedCost: model.Cost,
		})
	}
	return models
}

func codexBucketDate(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= len("2006-01-02") {
		return value[:len("2006-01-02")]
	}
	return value
}
