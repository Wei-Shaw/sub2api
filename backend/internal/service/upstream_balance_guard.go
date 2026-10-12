package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// IsUpstreamInsufficientBalance deliberately excludes generic permission,
// overload and quota errors. A relay may report payment failures as HTTP 403.
func IsUpstreamInsufficientBalance(status int, body []byte) bool {
	if status != 402 && status != 403 && status != 429 {
		return false
	}
	for _, path := range []string{"code", "error.code", "error.type"} {
		switch strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, path).String())) {
		case "insufficient_balance", "balance_not_enough", "insufficient_account_balance":
			return true
		}
	}
	for _, path := range []string{"message", "error.message"} {
		msg := strings.ToLower(gjson.GetBytes(body, path).String())
		if strings.Contains(msg, "insufficient account balance") || strings.Contains(msg, "insufficient balance") || strings.Contains(msg, "余额不足") {
			return true
		}
	}
	return false
}

func (s *RateLimitService) guardUpstreamBalance(ctx context.Context, account *Account, status int, body []byte) bool {
	if account == nil || account.IsCNProvider() || !IsUpstreamInsufficientBalance(status, body) {
		return false
	}
	until := time.Now().Add(15 * time.Minute)
	s.notifyAccountSchedulingBlocked(account, until, "upstream_insufficient_balance")
	if s.accountRepo != nil {
		if err := s.accountRepo.SetTempUnschedulable(ctx, account.ID, until, "Upstream balance insufficient; retry eligibility after 15 minutes or a successful recovery check"); err != nil {
			slog.Warn("upstream_balance_cooldown_save_failed", "account_id", account.ID, "error", err)
		}
	}
	slog.Info("upstream_balance_cooldown", "account_id", account.ID, "until", until)
	return true
}
