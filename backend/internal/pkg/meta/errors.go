package meta

import (
	"net/http"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

func ParseRetryAfter(statusCode int, body []byte, now time.Time) *time.Duration {
	if statusCode != http.StatusTooManyRequests && statusCode != http.StatusNotFound {
		return nil
	}
	if len(body) == 0 {
		return nil
	}
	resetsAt := gjson.GetBytes(body, "error.resets_at").Int()
	if resetsAt <= 0 {
		return nil
	}
	resetAt := time.Unix(resetsAt, 0)
	if !resetAt.After(now) {
		return nil
	}
	retryAfter := resetAt.Sub(now)
	return &retryAfter
}

func IsSubscriptionQuota(statusCode int, body []byte) bool {
	if statusCode != http.StatusTooManyRequests || len(body) == 0 {
		return false
	}
	message := strings.ToLower(gjson.GetBytes(body, "error.message").String())
	code := strings.ToLower(gjson.GetBytes(body, "error.code").String())
	if strings.Contains(message, "subscription quota") || strings.Contains(message, "quota exhausted") {
		return true
	}
	return (code == "rate_limit_exceeded" || strings.Contains(code, "quota")) && gjson.GetBytes(body, "error.resets_at").Exists()
}
