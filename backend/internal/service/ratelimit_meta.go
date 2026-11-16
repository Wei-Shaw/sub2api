package service

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/meta"
)

const metaNotFoundCooldown = 5 * time.Minute

func (s *RateLimitService) handleMetaThrottle(ctx context.Context, account *Account, statusCode int, responseBody []byte) bool {
	if s == nil || account == nil || !account.IsMeta() {
		return false
	}
	reason := "meta_rate_limited"
	if meta.IsSubscriptionQuota(statusCode, responseBody) {
		reason = "meta_subscription_quota"
	}
	resetAt := meta.ParseRetryAfter(statusCode, responseBody, time.Now())
	if resetAt != nil {
		until := time.Now().Add(*resetAt)
		s.notifyAccountSchedulingBlocked(account, until, reason)
		if err := s.accountRepo.SetRateLimited(ctx, account.ID, until); err != nil {
			slog.Warn("rate_limit_set_failed", "account_id", account.ID, "error", err)
			return false
		}
		slog.Info("meta_account_rate_limited",
			"account_id", account.ID,
			"platform", account.Platform,
			"reason", reason,
			"reset_at", until.UTC(),
		)
		return true
	}
	if statusCode != http.StatusNotFound {
		return false
	}
	until := time.Now().Add(metaNotFoundCooldown)
	s.setCNProviderTempUnschedulable(ctx, account, until, reason, reason+"_status_404", "meta_account_temp_unschedulable")
	return true
}
