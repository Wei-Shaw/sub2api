package service

import (
	"strconv"
	"strings"
	"time"
)

const (
	AccountStatusFilterActiveExcludingQuotaStopped = "active_excluding_quota_stopped"
	AccountStatusFilterOpenAI5HUsedZero            = "openai_5h_used_zero"
	AccountStatusFilterOpenAI7DUsedZero            = "openai_7d_used_zero"
	AccountStatusFilterOpenAIQuotaUsedRangePrefix  = "openai_quota_used_range:"
	AccountStatusFilterOpenAIQuotaFullPrefix       = "openai_quota_full:"
)

type openAIQuotaUsedRangeStatusFilter struct {
	window string
	min    float64
	max    float64
}

func MatchesAccountListStatusFilter(account *Account, requestedStatus string, now time.Time) bool {
	if account == nil {
		return false
	}

	switch strings.TrimSpace(requestedStatus) {
	case "":
		return true
	case StatusActive:
		return matchesActiveAccountListStatusFilter(account, now, false)
	case AccountStatusFilterActiveExcludingQuotaStopped:
		return matchesActiveAccountListStatusFilter(account, now, true)
	case "rate_limited":
		return account.Status == StatusActive &&
			isAccountRateLimitedAt(account, now) &&
			!isAccountTempUnschedulableAt(account, now)
	case "temp_unschedulable":
		return account.Status == StatusActive && isAccountTempUnschedulableAt(account, now)
	case "unschedulable":
		return account.Status == StatusActive &&
			!account.Schedulable &&
			!isAccountRateLimitedAt(account, now) &&
			!isAccountTempUnschedulableAt(account, now)
	case AccountStatusFilterOpenAI5HUsedZero:
		return isOpenAIUsagePercentExactlyZero(account, "5h", now)
	case AccountStatusFilterOpenAI7DUsedZero:
		return isOpenAIUsagePercentExactlyZero(account, "7d", now)
	default:
		if spec, ok := parseOpenAIQuotaUsedRangeStatusFilter(requestedStatus); ok {
			return matchesOpenAIQuotaUsedRangeFilter(account, spec, now)
		}
		if window, ok := parseOpenAIQuotaFullStatusFilter(requestedStatus); ok {
			return matchesOpenAIQuotaFullFilter(account, window, now)
		}
		return account.Status == requestedStatus
	}
}

func IsAccountListStatusFilterRequiringInMemory(status string) bool {
	normalized := strings.TrimSpace(status)
	switch normalized {
	case AccountStatusFilterActiveExcludingQuotaStopped, AccountStatusFilterOpenAI5HUsedZero, AccountStatusFilterOpenAI7DUsedZero:
		return true
	default:
		if _, ok := parseOpenAIQuotaUsedRangeStatusFilter(normalized); ok {
			return true
		}
		if _, ok := parseOpenAIQuotaFullStatusFilter(normalized); ok {
			return true
		}
		return false
	}
}

func IsOpenAIQuotaStatusFilter(status string) bool {
	normalized := strings.TrimSpace(status)
	switch normalized {
	case AccountStatusFilterOpenAI5HUsedZero, AccountStatusFilterOpenAI7DUsedZero:
		return true
	default:
		if _, ok := parseOpenAIQuotaUsedRangeStatusFilter(normalized); ok {
			return true
		}
		if _, ok := parseOpenAIQuotaFullStatusFilter(normalized); ok {
			return true
		}
		return false
	}
}

func parseOpenAIQuotaUsedRangeStatusFilter(status string) (openAIQuotaUsedRangeStatusFilter, bool) {
	normalized := strings.TrimSpace(status)
	if !strings.HasPrefix(normalized, AccountStatusFilterOpenAIQuotaUsedRangePrefix) {
		return openAIQuotaUsedRangeStatusFilter{}, false
	}
	parts := strings.Split(strings.TrimPrefix(normalized, AccountStatusFilterOpenAIQuotaUsedRangePrefix), ":")
	if len(parts) != 3 {
		return openAIQuotaUsedRangeStatusFilter{}, false
	}
	window := normalizeOpenAIQuotaStatusWindow(parts[0])
	if window == "" {
		return openAIQuotaUsedRangeStatusFilter{}, false
	}
	min, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return openAIQuotaUsedRangeStatusFilter{}, false
	}
	max, err := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
	if err != nil {
		return openAIQuotaUsedRangeStatusFilter{}, false
	}
	if min < 0 || max < 0 || min > 100 || max > 100 || min > max {
		return openAIQuotaUsedRangeStatusFilter{}, false
	}
	return openAIQuotaUsedRangeStatusFilter{window: window, min: min, max: max}, true
}

func parseOpenAIQuotaFullStatusFilter(status string) (string, bool) {
	normalized := strings.TrimSpace(status)
	if !strings.HasPrefix(normalized, AccountStatusFilterOpenAIQuotaFullPrefix) {
		return "", false
	}
	window := normalizeOpenAIQuotaStatusWindow(strings.TrimPrefix(normalized, AccountStatusFilterOpenAIQuotaFullPrefix))
	if window == "" {
		return "", false
	}
	return window, true
}

func normalizeOpenAIQuotaStatusWindow(window string) string {
	switch strings.ToLower(strings.TrimSpace(window)) {
	case "5h":
		return "5h"
	case "7d":
		return "7d"
	default:
		return ""
	}
}

func matchesOpenAIQuotaUsedRangeFilter(account *Account, spec openAIQuotaUsedRangeStatusFilter, now time.Time) bool {
	if account == nil || !account.IsOpenAIOAuth() {
		return false
	}
	if !matchesActiveAccountListStatusFilter(account, now, true) {
		return false
	}
	usedPercent, ok := openAIQuotaUsedPercent(account, spec.window, now)
	if !ok {
		return false
	}
	return usedPercent >= spec.min && usedPercent <= spec.max
}

func matchesOpenAIQuotaFullFilter(account *Account, window string, now time.Time) bool {
	usedPercent, ok := openAIQuotaUsedPercent(account, window, now)
	return ok && usedPercent >= 100
}

func openAIQuotaUsedPercent(account *Account, window string, now time.Time) (float64, bool) {
	if account == nil || !account.IsOpenAIOAuth() || account.Extra == nil {
		return 0, false
	}
	progress := buildCodexUsageProgressFromExtra(account.Extra, window, now)
	if progress == nil {
		return 0, false
	}
	used := progress.Utilization
	if used < 0 {
		used = 0
	}
	return used, true
}

func matchesActiveAccountListStatusFilter(account *Account, now time.Time, excludeQuotaStopped bool) bool {
	if account == nil {
		return false
	}
	if account.Status != StatusActive || !account.Schedulable {
		return false
	}
	if isAccountRateLimitedAt(account, now) || isAccountTempUnschedulableAt(account, now) {
		return false
	}
	if excludeQuotaStopped && !isOpenAIQuotaStrategySchedulableAt(account, now) {
		return false
	}
	return true
}

func isOpenAIQuotaStrategySchedulableAt(account *Account, now time.Time) bool {
	strategy := getOpenAIQuotaStrategy(account)
	if strategy == "" {
		return true
	}
	remaining, ok := getOpenAIQuotaRemainingPercentByStrategyAt(account, strategy, now)
	if !ok {
		return true
	}
	return remaining >= getOpenAIQuotaStopThresholdPercent(account)
}

func getOpenAIQuotaStrategy(account *Account) string {
	if account == nil || !account.IsOpenAIOAuth() {
		return ""
	}
	switch strings.TrimSpace(account.GetExtraString("openai_quota_strategy")) {
	case "prefer_5h":
		return "prefer_5h"
	case "prefer_7d":
		return "prefer_7d"
	default:
		return ""
	}
}

func getOpenAIQuotaStopThresholdPercent(account *Account) float64 {
	if account == nil || !account.IsOpenAIOAuth() {
		return 0
	}
	threshold := account.getExtraFloat64("openai_quota_stop_threshold_percent")
	if threshold <= 0 {
		return 10
	}
	if threshold > 100 {
		return 100
	}
	return threshold
}

func getOpenAIQuotaRemainingPercentByStrategyAt(account *Account, strategy string, now time.Time) (float64, bool) {
	window := ""
	switch strategy {
	case "prefer_5h":
		window = "5h"
	case "prefer_7d":
		window = "7d"
	default:
		return 0, false
	}
	used, ok := openAIQuotaUsedPercent(account, window, now)
	if !ok {
		return 0, false
	}
	if used > 100 {
		used = 100
	}
	return 100 - used, true
}

func isAccountRateLimitedAt(account *Account, now time.Time) bool {
	return account != nil && account.RateLimitResetAt != nil && account.RateLimitResetAt.After(now)
}

func isAccountTempUnschedulableAt(account *Account, now time.Time) bool {
	return account != nil && account.TempUnschedulableUntil != nil && account.TempUnschedulableUntil.After(now)
}

func isOpenAIUsagePercentExactlyZero(account *Account, window string, now time.Time) bool {
	usedPercent, ok := openAIQuotaUsedPercent(account, window, now)
	return ok && usedPercent == 0
}
