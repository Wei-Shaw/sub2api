package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	telegramAPIBase    = "https://api.telegram.org/bot"
	telegramSendMethod = "/sendMessage"

	// Setting keys for Telegram notifications
	SettingTelegramBotToken             = "telegram_bot_token"
	SettingTelegramChatID               = "telegram_chat_id"
	SettingTelegramNotifyNewUser        = "telegram_notify_new_user"
	SettingTelegramNotifyAccountError   = "telegram_notify_account_error"
	SettingTelegramNotifyAccountExpired = "telegram_notify_account_expired"
	SettingTelegramNotifyPaymentSuccess = "telegram_notify_payment_success"
	SettingTelegramNotifyPaymentFailed  = "telegram_notify_payment_failed"
	SettingTelegramNotifyRefund         = "telegram_notify_refund"
	SettingTelegramNotifySubExpired     = "telegram_notify_sub_expired"
	SettingTelegramNotifyBalanceLow     = "telegram_notify_balance_low"
	SettingTelegramNotifyOpsAlert       = "telegram_notify_ops_alert"
	SettingTelegramNotifyProxyExpired   = "telegram_notify_proxy_expired"
)

// TelegramNotifyService sends notifications via Telegram Bot API.
type TelegramNotifyService struct {
	settingRepo  SettingRepository
	httpClient   *http.Client
	mu           sync.RWMutex
	cachedToken  string
	cachedChatID string
	cacheExpiry  time.Time
}

// NewTelegramNotifyService creates a new TelegramNotifyService.
func NewTelegramNotifyService(settingRepo SettingRepository) *TelegramNotifyService {
	return &TelegramNotifyService{
		settingRepo: settingRepo,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type telegramConfig struct {
	Token  string
	ChatID string
}

func (s *TelegramNotifyService) getConfig(ctx context.Context) (*telegramConfig, error) {
	s.mu.RLock()
	if time.Now().Before(s.cacheExpiry) && s.cachedToken != "" {
		cfg := &telegramConfig{Token: s.cachedToken, ChatID: s.cachedChatID}
		s.mu.RUnlock()
		return cfg, nil
	}
	s.mu.RUnlock()

	settings, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingTelegramBotToken,
		SettingTelegramChatID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get telegram settings: %w", err)
	}

	token := strings.TrimSpace(settings[SettingTelegramBotToken])
	chatID := strings.TrimSpace(settings[SettingTelegramChatID])

	if token == "" || chatID == "" {
		return nil, nil
	}

	s.mu.Lock()
	s.cachedToken = token
	s.cachedChatID = chatID
	s.cacheExpiry = time.Now().Add(60 * time.Second)
	s.mu.Unlock()

	return &telegramConfig{Token: token, ChatID: chatID}, nil
}

func (s *TelegramNotifyService) isEnabled(ctx context.Context, settingKey string) bool {
	val, err := s.settingRepo.GetValue(ctx, settingKey)
	if err != nil {
		return false
	}
	return val == "true" || val == "1"
}

func (s *TelegramNotifyService) sendMessage(ctx context.Context, text string) error {
	cfg, err := s.getConfig(ctx)
	if err != nil {
		return err
	}
	if cfg == nil {
		return nil
	}

	return s.sendMessageWithConfig(ctx, cfg, text)
}

func (s *TelegramNotifyService) sendMessageWithConfig(ctx context.Context, cfg *telegramConfig, text string) error {
	apiURL := telegramAPIBase + cfg.Token + telegramSendMethod

	form := url.Values{}
	form.Set("chat_id", cfg.ChatID)
	form.Set("text", text)
	form.Set("parse_mode", "HTML")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send telegram message: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned status %d", resp.StatusCode)
	}

	return nil
}

// SendTestMessage sends a test message to verify bot configuration.
func (s *TelegramNotifyService) SendTestMessage(ctx context.Context) error {
	text := fmt.Sprintf(
		"\xe2\x9c\x85 <b>Telegram Bot Connected</b>\n\n"+
			"Bot is configured and working correctly.\n"+
			"\xf0\x9f\x94\x94 Time: %s",
		time.Now().Format("2006-01-02 15:04:05"),
	)
	return s.sendMessage(ctx, text)
}

// InvalidateCache clears the cached config.
func (s *TelegramNotifyService) InvalidateCache() {
	s.mu.Lock()
	s.cacheExpiry = time.Time{}
	s.mu.Unlock()
}

// SendTestMessageWithOverrides sends a test message using saved config with optional unsaved token/chat overrides.
// This allows testing admin-entered settings before saving them. It never caches the token override.
func (s *TelegramNotifyService) SendTestMessageWithOverrides(ctx context.Context, botToken, chatID string) error {
	cfg, err := s.getConfig(ctx)
	if err != nil {
		return err
	}
	if cfg == nil {
		cfg = &telegramConfig{}
	} else {
		cfg = &telegramConfig{Token: cfg.Token, ChatID: cfg.ChatID}
	}

	if token := strings.TrimSpace(botToken); token != "" {
		cfg.Token = token
	}
	if id := strings.TrimSpace(chatID); id != "" {
		cfg.ChatID = id
	}

	if strings.TrimSpace(cfg.Token) == "" {
		return fmt.Errorf("telegram bot token is not configured")
	}
	if strings.TrimSpace(cfg.ChatID) == "" {
		return fmt.Errorf("telegram chat id is not configured")
	}

	text := fmt.Sprintf(
		"\xe2\x9c\x85 <b>Telegram Bot Connected</b>\n\n"+
			"Bot is configured and working correctly.\n"+
			"\xf0\x9f\x94\x94 Time: %s",
		time.Now().Format("2006-01-02 15:04:05"),
	)

	return s.sendMessageWithConfig(ctx, cfg, text)
}

// SendTestMessageWithChatID sends a test message using the saved bot token but overriding the chat ID.
// This allows testing delivery to a different chat without changing saved settings.
func (s *TelegramNotifyService) SendTestMessageWithChatID(ctx context.Context, chatID string) error {
	return s.SendTestMessageWithOverrides(ctx, "", chatID)
}

// --- Notification Methods ---

// NotifyNewUser sends a notification when a new user registers.
func (s *TelegramNotifyService) NotifyNewUser(ctx context.Context, email string, source string) {
	if !s.isEnabled(ctx, SettingTelegramNotifyNewUser) {
		return
	}
	sourceLabel := "Email"
	if source != "" {
		sourceLabel = source
	}
	text := fmt.Sprintf(
		" <b>New User Registered</b>\n\n"+
			" Email: <code>%s</code>\n"+
			" Source: %s\n"+
			" Time: %s",
		escapeHTML(email),
		escapeHTML(sourceLabel),
		time.Now().Format("2006-01-02 15:04:05"),
	)
	if err := s.sendMessage(ctx, text); err != nil {
		slog.Error("telegram notify new user failed", "error", err, "email", email)
	}
}

// NotifyAccountError sends a notification when an account encounters an error.
func (s *TelegramNotifyService) NotifyAccountError(ctx context.Context, accountID int64, accountName, platform, errorMsg string) {
	if !s.isEnabled(ctx, SettingTelegramNotifyAccountError) {
		return
	}
	text := fmt.Sprintf(
		" <b>Account Error</b>\n\n"+
			" ID: <code>%d</code>\n"+
			" Name: %s\n"+
			" Platform: %s\n"+
			" Error: <code>%s</code>\n"+
			" Time: %s",
		accountID,
		escapeHTML(accountName),
		escapeHTML(platform),
		escapeHTML(truncateTelegram(errorMsg, 200)),
		time.Now().Format("2006-01-02 15:04:05"),
	)
	if err := s.sendMessage(ctx, text); err != nil {
		slog.Error("telegram notify account error failed", "error", err, "account_id", accountID)
	}
}

// NotifyAccountExpired sends a notification when accounts are auto-paused due to expiry.
func (s *TelegramNotifyService) NotifyAccountExpired(ctx context.Context, count int64) {
	if !s.isEnabled(ctx, SettingTelegramNotifyAccountExpired) || count == 0 {
		return
	}
	text := fmt.Sprintf(
		" <b>Accounts Expired</b>\n\n"+
			" Count: <b>%d</b> account(s) auto-paused\n"+
			" Time: %s",
		count,
		time.Now().Format("2006-01-02 15:04:05"),
	)
	if err := s.sendMessage(ctx, text); err != nil {
		slog.Error("telegram notify account expired failed", "error", err, "count", count)
	}
}

// NotifyPaymentSuccess sends a notification when a payment is completed successfully.
func (s *TelegramNotifyService) NotifyPaymentSuccess(ctx context.Context, userEmail string, amount float64, orderType string, orderID string) {
	if !s.isEnabled(ctx, SettingTelegramNotifyPaymentSuccess) {
		return
	}
	text := fmt.Sprintf(
		" <b>Payment Success</b>\n\n"+
			" User: <code>%s</code>\n"+
			" Amount: <b>%.2f</b>\n"+
			" Type: %s\n"+
			" Order: <code>%s</code>\n"+
			" Time: %s",
		escapeHTML(userEmail),
		amount,
		escapeHTML(orderType),
		escapeHTML(orderID),
		time.Now().Format("2006-01-02 15:04:05"),
	)
	if err := s.sendMessage(ctx, text); err != nil {
		slog.Error("telegram notify payment success failed", "error", err, "order_id", orderID)
	}
}

// NotifyPaymentFailed sends a notification when a payment fulfillment fails.
func (s *TelegramNotifyService) NotifyPaymentFailed(ctx context.Context, orderID string, reason string) {
	if !s.isEnabled(ctx, SettingTelegramNotifyPaymentFailed) {
		return
	}
	text := fmt.Sprintf(
		" <b>Payment Failed</b>\n\n"+
			" Order: <code>%s</code>\n"+
			" Reason: <code>%s</code>\n"+
			" Time: %s",
		escapeHTML(orderID),
		escapeHTML(truncateTelegram(reason, 200)),
		time.Now().Format("2006-01-02 15:04:05"),
	)
	if err := s.sendMessage(ctx, text); err != nil {
		slog.Error("telegram notify payment failed", "error", err, "order_id", orderID)
	}
}

// NotifyRefund sends a notification when a refund is processed.
func (s *TelegramNotifyService) NotifyRefund(ctx context.Context, orderID string, amount float64, reason string, success bool) {
	if !s.isEnabled(ctx, SettingTelegramNotifyRefund) {
		return
	}
	status := " Success"
	if !success {
		status = " Failed"
	}
	text := fmt.Sprintf(
		" <b>Refund %s</b>\n\n"+
			" Order: <code>%s</code>\n"+
			" Amount: <b>%.2f</b>\n"+
			" Reason: %s\n"+
			" Time: %s",
		status,
		escapeHTML(orderID),
		amount,
		escapeHTML(truncateTelegram(reason, 150)),
		time.Now().Format("2006-01-02 15:04:05"),
	)
	if err := s.sendMessage(ctx, text); err != nil {
		slog.Error("telegram notify refund failed", "error", err, "order_id", orderID)
	}
}

// NotifySubscriptionExpired sends a notification when subscriptions expire.
func (s *TelegramNotifyService) NotifySubscriptionExpired(ctx context.Context, count int64) {
	if !s.isEnabled(ctx, SettingTelegramNotifySubExpired) || count == 0 {
		return
	}
	text := fmt.Sprintf(
		" <b>Subscriptions Expired</b>\n\n"+
			" Count: <b>%d</b> subscription(s) expired\n"+
			" Time: %s",
		count,
		time.Now().Format("2006-01-02 15:04:05"),
	)
	if err := s.sendMessage(ctx, text); err != nil {
		slog.Error("telegram notify subscription expired failed", "error", err, "count", count)
	}
}

// NotifyBalanceLow sends a notification when a user balance drops below threshold.
func (s *TelegramNotifyService) NotifyBalanceLow(ctx context.Context, userEmail string, balance, threshold float64) {
	if !s.isEnabled(ctx, SettingTelegramNotifyBalanceLow) {
		return
	}
	text := fmt.Sprintf(
		" <b>Balance Low</b>\n\n"+
			" User: <code>%s</code>\n"+
			" Balance: <b>%.4f</b>\n"+
			" Threshold: %.4f\n"+
			" Time: %s",
		escapeHTML(userEmail),
		balance,
		threshold,
		time.Now().Format("2006-01-02 15:04:05"),
	)
	if err := s.sendMessage(ctx, text); err != nil {
		slog.Error("telegram notify balance low failed", "error", err, "email", userEmail)
	}
}

// NotifyOpsAlert sends a notification when an ops alert fires.
func (s *TelegramNotifyService) NotifyOpsAlert(ctx context.Context, ruleName, severity, description string, metricValue float64) {
	if !s.isEnabled(ctx, SettingTelegramNotifyOpsAlert) {
		return
	}
	severityIcon := ""
	switch severity {
	case "critical":
		severityIcon = ""
	case "info":
		severityIcon = ""
	}
	text := fmt.Sprintf(
		"%s <b>Ops Alert: %s</b>\n\n"+
			" Severity: <b>%s</b>\n"+
			" Metric: <b>%.2f</b>\n"+
			" Description: %s\n"+
			" Time: %s",
		severityIcon,
		escapeHTML(ruleName),
		escapeHTML(severity),
		metricValue,
		escapeHTML(truncateTelegram(description, 200)),
		time.Now().Format("2006-01-02 15:04:05"),
	)
	if err := s.sendMessage(ctx, text); err != nil {
		slog.Error("telegram notify ops alert failed", "error", err, "rule", ruleName)
	}
}

// NotifyProxyExpired sends a notification when proxies are about to expire or have expired.
func (s *TelegramNotifyService) NotifyProxyExpired(ctx context.Context, proxyName string, expiresAt time.Time, isExpired bool) {
	if !s.isEnabled(ctx, SettingTelegramNotifyProxyExpired) {
		return
	}
	status := "expiring soon"
	icon := ""
	if isExpired {
		status = "EXPIRED"
		icon = ""
	}
	text := fmt.Sprintf(
		"%s <b>Proxy %s</b>\n\n"+
			" Name: <b>%s</b>\n"+
			" Expires: %s\n"+
			" Time: %s",
		icon,
		status,
		escapeHTML(proxyName),
		expiresAt.Format("2006-01-02 15:04:05"),
		time.Now().Format("2006-01-02 15:04:05"),
	)
	if err := s.sendMessage(ctx, text); err != nil {
		slog.Error("telegram notify proxy expired failed", "error", err, "proxy", proxyName)
	}
}

// NotifyProxyExpiring sends a batch notification for proxies nearing expiration.
func (s *TelegramNotifyService) NotifyProxyExpiring(ctx context.Context, proxies []Proxy, thresholdDays int) {
	if !s.isEnabled(ctx, SettingTelegramNotifyProxyExpired) || len(proxies) == 0 {
		return
	}

	var details string
	for i, p := range proxies {
		if i >= 10 {
			details += fmt.Sprintf("\n... and %d more", len(proxies)-10)
			break
		}
		remaining := ""
		if p.ExpiresAt != nil {
			remaining = fmt.Sprintf(" (expires %s)", p.ExpiresAt.Format("2006-01-02"))
		}
		details += fmt.Sprintf("\n <b>%s</b> [%s:%d]%s", escapeHTML(p.Name), escapeHTML(p.Host), p.Port, remaining)
	}

	text := fmt.Sprintf(
		"  <b>Proxies Expiring Soon</b>\n\n"+
			" Count: <b>%d</b> proxy(ies) within %d days\n"+
			"%s\n\n"+
			" Time: %s",
		len(proxies),
		thresholdDays,
		details,
		time.Now().Format("2006-01-02 15:04:05"),
	)
	if err := s.sendMessage(ctx, text); err != nil {
		slog.Error("telegram notify proxy expiring failed", "error", err, "count", len(proxies))
	}
}

// --- Helpers ---

// escapeHTML escapes HTML special characters for Telegram.
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// truncateTelegram limits a string to maxLen characters for telegram messages.
func truncateTelegram(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
