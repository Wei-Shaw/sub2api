package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// ProviderBalanceAlerts delivers bounded admin alerts for API-key providers'
// structured insufficient-balance responses.
type ProviderBalanceAlerts struct {
	telegram *TelegramNotifyService
	claims   *IdempotencyCoordinator
}

func NewProviderBalanceAlerts(telegram *TelegramNotifyService, claims *IdempotencyCoordinator) *ProviderBalanceAlerts {
	return &ProviderBalanceAlerts{telegram: telegram, claims: claims}
}

// providerInsufficientBalance matches only structured account-level
// balance signals, never generic 403/HTML/CDN failures or a customer's wallet.
func providerInsufficientBalance(account *Account, statusCode int, body []byte) bool {
	if account == nil || !domain.IsConcretePlatform(account.Platform) || account.Type != AccountTypeAPIKey ||
		(statusCode != http.StatusForbidden && statusCode != http.StatusPaymentRequired) || !json.Valid(body) {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(extractUpstreamErrorCode(body)), "INSUFFICIENT_BALANCE") {
		return true
	}
	message := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(body)))
	return message == "insufficient balance" || strings.HasPrefix(message, "insufficient account balance")
}

// Observe does not change the upstream error policy. One durable claim per
// account/hour limits repeated failed requests across instances; a failed send
// is retryable on the next matching response after coordinator backoff.
func (s *ProviderBalanceAlerts) Observe(ctx context.Context, account *Account, statusCode int, body []byte) {
	if !providerInsufficientBalance(account, statusCode, body) {
		return
	}
	s.Deliver(ctx, account.ID, account.Name, account.Platform)
}

// Deliver is called only after classifying the raw upstream provider response.
// Its arguments are copied scalars, so it is safe to invoke from the error
// handler asynchronously without retaining a response buffer or account object.
func (s *ProviderBalanceAlerts) Deliver(ctx context.Context, accountID int64, accountName, platform string) {
	if s == nil || s.telegram == nil || s.claims == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 12*time.Second)
	defer cancel()
	if !s.telegram.isEnabled(ctx, SettingTelegramNotifyProviderBalance) {
		return
	}
	key := fmt.Sprintf("provider-balance:%d:%s", accountID, time.Now().UTC().Format("2006010215"))
	_, err := s.claims.Execute(ctx, IdempotencyExecuteOptions{
		Scope: "telegram_provider_balance", ActorScope: fmt.Sprintf("account:%d", accountID),
		Method: "POST", Route: "telegram/admin/provider-balance",
		IdempotencyKey: key, Payload: map[string]any{"account_id": accountID, "platform": platform, "status": "exhausted"},
		RequireKey: true, TTL: 2 * time.Hour,
	}, func(sendCtx context.Context) (any, error) {
		cfg, configErr := s.telegram.getConfig(sendCtx)
		if configErr != nil {
			return nil, configErr
		}
		if cfg == nil {
			return nil, fmt.Errorf("admin telegram bot is not configured")
		}
		text := fmt.Sprintf("<b>Provider hết số dư — cần nạp tiền</b>\n\nAccount: <code>%d</code> %s\nProvider: <b>%s</b>\nLỗi upstream: Insufficient account balance\nThời gian: %s",
			accountID, escapeHTML(truncateTelegram(accountName, 128)), escapeHTML(truncateTelegram(platform, 64)), time.Now().Format("2006-01-02 15:04:05"))
		if sendErr := s.telegram.sendWithConfig(sendCtx, cfg, cfg.ChatID, text); sendErr != nil {
			// HTTP transport errors may contain the bot token in the URL. Never
			// persist them in the claim or emit them to the application log.
			return nil, fmt.Errorf("admin telegram delivery failed")
		}
		return nil, nil
	})
	if err != nil && err != ErrIdempotencyInProgress && err != ErrIdempotencyRetryBackoff {
		slog.Warn("provider_balance_alert_failed", "account_id", accountID, "platform", platform, "error", err)
	}
}
