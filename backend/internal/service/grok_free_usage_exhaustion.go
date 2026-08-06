package service

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/tidwall/gjson"
)

const (
	grokFreeUsageExhaustedCode      = "subscription:free-usage-exhausted"
	grokFreeUsageExhaustionCooldown = 24 * time.Hour
)

var grokFreeUsageTokenPairPattern = regexp.MustCompile(`(?i)tokens?\s*(?:\(actual\s*/\s*limit\))?\s*[:=]?\s*(\d+)\s*/\s*(\d+)`)

type grokFreeUsageExhaustion struct {
	actual       int64
	limit        int64
	hasTokenPair bool
}

func parseGrokFreeUsageExhaustion(statusCode int, responseBody []byte) (grokFreeUsageExhaustion, bool) {
	if statusCode != http.StatusTooManyRequests || len(responseBody) == 0 {
		return grokFreeUsageExhaustion{}, false
	}

	code := strings.ToLower(strings.TrimSpace(gjson.GetBytes(responseBody, "code").String()))
	message := strings.TrimSpace(gjson.GetBytes(responseBody, "error").String())
	if message == "" {
		message = strings.TrimSpace(gjson.GetBytes(responseBody, "error.message").String())
	}
	combined := strings.ToLower(strings.Join([]string{code, message, string(responseBody)}, " "))
	canonicalCode := code == grokFreeUsageExhaustedCode || strings.Contains(combined, "free-usage-exhausted")
	canonicalMessage := strings.Contains(combined, "included free usage") && strings.Contains(combined, "rolling 24-hour")
	if !canonicalCode && !canonicalMessage {
		return grokFreeUsageExhaustion{}, false
	}

	result := grokFreeUsageExhaustion{limit: xai.GrokFreeRolling24hTokenLimit}
	match := grokFreeUsageTokenPairPattern.FindStringSubmatch(combined)
	if len(match) != 3 {
		return result, true
	}
	actual, actualErr := strconv.ParseInt(match[1], 10, 64)
	limit, limitErr := strconv.ParseInt(match[2], 10, 64)
	if actualErr != nil || limitErr != nil || actual < 0 || limit <= 0 {
		return result, true
	}
	result.actual = actual
	result.limit = limit
	result.hasTokenPair = true
	return result, true
}

func isGrokFreeUsageExhausted(statusCode int, responseBody []byte) bool {
	_, exhausted := parseGrokFreeUsageExhaustion(statusCode, responseBody)
	return exhausted
}

func parseGrokQuotaSnapshotWithBody(headers http.Header, statusCode int, responseBody []byte, now time.Time) *xai.QuotaSnapshot {
	snapshot := parseGrokQuotaSnapshot(headers, statusCode, now)
	exhaustion, exhausted := parseGrokFreeUsageExhaustion(statusCode, responseBody)
	if !exhausted {
		return snapshot
	}
	if snapshot == nil {
		snapshot = &xai.QuotaSnapshot{}
	}

	resetAt := now.Add(grokFreeUsageExhaustionCooldown)
	if observedResetAt, limited := grokRateLimitResetAt(snapshot, now); limited && observedResetAt.After(resetAt) {
		resetAt = observedResetAt
	}
	limit := exhaustion.limit
	if !exhaustion.hasTokenPair && snapshot.Tokens != nil && snapshot.Tokens.Limit != nil && *snapshot.Tokens.Limit > 0 {
		limit = *snapshot.Tokens.Limit
	}
	remaining := int64(0)
	resetUnix := resetAt.Unix()
	snapshot.Tokens = &xai.QuotaWindow{
		Limit:     &limit,
		Remaining: &remaining,
		ResetUnix: &resetUnix,
		ResetAt:   resetAt.UTC().Format(time.RFC3339),
	}
	if strings.TrimSpace(snapshot.SubscriptionTier) == "" {
		snapshot.SubscriptionTier = "free"
	}
	snapshot.StatusCode = statusCode
	snapshot.ObservationSource = "upstream_error_body"
	snapshot.UpdatedAt = now.UTC().Format(time.RFC3339)
	return snapshot
}
