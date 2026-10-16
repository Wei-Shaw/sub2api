package service

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// MiniMax 2062 means a request-rate limit, not exhaustion of the 5h/weekly
// entitlement. The compatibility endpoint may carry the code only in message.
func isMiniMaxRequestRateLimit(account *Account, body []byte) bool {
	if account == nil || account.Platform != PlatformMiniMax {
		return false
	}
	for _, path := range []string{"base_resp.status_code", "error.code", "error.status_code", "code", "status_code"} {
		if value := gjson.GetBytes(body, path); value.Exists() && value.String() == "2062" {
			return true
		}
	}
	message := extractUpstreamErrorMessage(body)
	return strings.Contains(message, "(2062)") || strings.Contains(message, "（2062）")
}

func miniMaxRequestRateReset(now time.Time, headers http.Header) time.Time {
	value := strings.TrimSpace(headers.Get("Retry-After"))
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return now.Add(time.Duration(seconds) * time.Second)
	}
	if until, err := http.ParseTime(value); err == nil && until.After(now) {
		return until
	}
	// Use a full minute after the rejection, not the next calendar-minute edge.
	return now.Add(time.Minute)
}

func (s *RateLimitService) handleMiniMaxRequestRate429(ctx context.Context, account *Account, headers http.Header) {
	until := miniMaxRequestRateReset(time.Now(), headers)
	// Do not shorten a separately established quota cooldown.
	if account.RateLimitResetAt != nil && account.RateLimitResetAt.After(until) {
		until = *account.RateLimitResetAt
	}
	var err error
	if writer, ok := s.accountRepo.(interface {
		SetRateLimitedIfLater(context.Context, int64, time.Time) error
	}); ok {
		err = writer.SetRateLimitedIfLater(ctx, account.ID, until)
	} else {
		err = s.accountRepo.SetRateLimited(ctx, account.ID, until)
	}
	s.notifyAccountSchedulingBlocked(account, until, "minimax_request_rate_limit")
	if err != nil {
		slog.Warn("minimax_request_rate_limit_persist_failed", "account_id", account.ID, "error", err)
		return
	}
	slog.Info("minimax_request_rate_limited", "account_id", account.ID, "reset_at", until.UTC())
}
