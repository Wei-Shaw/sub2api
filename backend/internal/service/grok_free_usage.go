package service

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/tidwall/gjson"
)

const (
	grokFreeUsageExhaustedErrorCode = "subscription:free-usage-exhausted"
	grokFreeUsageExhaustedCooldown  = time.Hour
)

var grokFreeUsageTokensPattern = regexp.MustCompile(`(?i)tokens\s*\(actual/limit\)\s*:\s*([0-9][0-9,]*)\s*/\s*([0-9][0-9,]*)`)

type grokFreeUsageExhaustedDetails struct {
	Actual int64
	Limit  int64
}

func isGrokFreeUsageExhaustedError(body []byte) bool {
	return strings.TrimSpace(gjson.GetBytes(body, "code").String()) == grokFreeUsageExhaustedErrorCode
}

func parseGrokFreeUsageExhausted(body []byte) (grokFreeUsageExhaustedDetails, bool) {
	if !isGrokFreeUsageExhaustedError(body) {
		return grokFreeUsageExhaustedDetails{}, false
	}

	message := strings.TrimSpace(gjson.GetBytes(body, "error").String())
	if message == "" {
		message = strings.TrimSpace(gjson.GetBytes(body, "message").String())
	}
	matches := grokFreeUsageTokensPattern.FindStringSubmatch(message)
	if len(matches) != 3 {
		return grokFreeUsageExhaustedDetails{}, false
	}

	actual, actualErr := strconv.ParseInt(strings.ReplaceAll(matches[1], ",", ""), 10, 64)
	limit, limitErr := strconv.ParseInt(strings.ReplaceAll(matches[2], ",", ""), 10, 64)
	if actualErr != nil || limitErr != nil || actual < 0 || limit <= 0 {
		return grokFreeUsageExhaustedDetails{}, false
	}
	return grokFreeUsageExhaustedDetails{Actual: actual, Limit: limit}, true
}

func enrichGrokQuotaSnapshotFromError(snapshot *xai.QuotaSnapshot, body []byte, statusCode int, now time.Time) *xai.QuotaSnapshot {
	if !isGrokFreeUsageExhaustedError(body) {
		return snapshot
	}
	if snapshot == nil {
		snapshot = &xai.QuotaSnapshot{StatusCode: statusCode}
	}
	if details, ok := parseGrokFreeUsageExhausted(body); ok {
		if snapshot.Tokens == nil {
			snapshot.Tokens = &xai.QuotaWindow{}
		}
		limit := details.Limit
		remaining := int64(0)
		snapshot.Tokens.Limit = &limit
		snapshot.Tokens.Remaining = &remaining
	}
	snapshot.SubscriptionTier = "free"
	snapshot.ProviderErrorCode = grokFreeUsageExhaustedErrorCode
	if strings.TrimSpace(snapshot.ObservationSource) == "" {
		snapshot.ObservationSource = "error_body"
	}
	if strings.TrimSpace(snapshot.UpdatedAt) == "" {
		snapshot.UpdatedAt = now.UTC().Format(time.RFC3339)
	}
	return snapshot
}
