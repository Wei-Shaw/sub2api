package service

import (
	"context"
	"encoding/json"
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
	// Legacy/admin bot settings. Existing operational notifications remain on this lane.
	SettingTelegramBotToken = "telegram_bot_token"
	SettingTelegramChatID   = "telegram_chat_id"
	// Split lanes: provider/influence alerts use the admin bot; user wallet alerts
	// use the customer bot and the recipient's own Telegram chat ID.
	SettingTelegramAdminBotToken         = "telegram_admin_bot_token"
	SettingTelegramAdminChatID           = "telegram_admin_chat_id"
	SettingTelegramCustomerBotToken      = "telegram_customer_bot_token"
	SettingTelegramNotifyCustomerBalance = "telegram_notify_customer_balance"
	SettingTelegramNotifyProviderBalance = "telegram_notify_provider_balance"
	SettingTelegramNotifyNewUser         = "telegram_notify_new_user"
	SettingTelegramNotifyAccountError    = "telegram_notify_account_error"
	SettingTelegramNotifyAccountExpired  = "telegram_notify_account_expired"
	SettingTelegramNotifyPaymentSuccess  = "telegram_notify_payment_success"
	SettingTelegramNotifyPaymentFailed   = "telegram_notify_payment_failed"
	SettingTelegramNotifyRefund          = "telegram_notify_refund"
	SettingTelegramNotifySubExpired      = "telegram_notify_sub_expired"
	SettingTelegramNotifyBalanceLow      = "telegram_notify_balance_low"
	SettingTelegramNotifyOpsAlert        = "telegram_notify_ops_alert"
	SettingTelegramNotifyProxyExpired    = "telegram_notify_proxy_expired"
)

// TelegramNotifyService sends notifications via Telegram Bot API.
type TelegramNotifyService struct {
	settingRepo SettingRepository
	httpClient  *http.Client
	apiBaseURL  string
	mu          sync.RWMutex
	cached      map[string]telegramConfigCache
}

// NewTelegramNotifyService creates a new TelegramNotifyService.
func NewTelegramNotifyService(settingRepo SettingRepository) *TelegramNotifyService {
	return &TelegramNotifyService{
		settingRepo: settingRepo,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		apiBaseURL: telegramAPIBase,
		cached:     make(map[string]telegramConfigCache),
	}
}

type telegramConfig struct {
	Token  string
	ChatID string
}

type telegramConfigCache struct {
	config  telegramConfig
	expires time.Time
}

const (
	telegramAdminLane    = "admin"
	telegramCustomerLane = "customer"
)

// getConfig returns the admin configuration. The legacy keys remain a
// deliberate compatibility fallback for existing operational notifications;
// customer balance notifications never use this method.
func (s *TelegramNotifyService) getConfig(ctx context.Context) (*telegramConfig, error) {
	return s.getLaneConfig(ctx, telegramAdminLane)
}

func (s *TelegramNotifyService) getCustomerConfig(ctx context.Context) (*telegramConfig, error) {
	return s.getLaneConfig(ctx, telegramCustomerLane)
}

func (s *TelegramNotifyService) getLaneConfig(ctx context.Context, lane string) (*telegramConfig, error) {
	if s == nil || s.settingRepo == nil {
		return nil, nil
	}
	s.mu.RLock()
	if cached, ok := s.cached[lane]; ok && time.Now().Before(cached.expires) && cached.config.Token != "" {
		cfg := cached.config
		s.mu.RUnlock()
		return &cfg, nil
	}
	s.mu.RUnlock()

	keys := []string{SettingTelegramAdminBotToken, SettingTelegramAdminChatID, SettingTelegramBotToken, SettingTelegramChatID}
	if lane == telegramCustomerLane {
		keys = []string{SettingTelegramCustomerBotToken}
	}
	settings, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("failed to get telegram settings: %w", err)
	}

	var token, chatID string
	if lane == telegramCustomerLane {
		// There is intentionally no fallback to the admin/legacy token.
		token = strings.TrimSpace(settings[SettingTelegramCustomerBotToken])
	} else {
		token = strings.TrimSpace(settings[SettingTelegramAdminBotToken])
		chatID = strings.TrimSpace(settings[SettingTelegramAdminChatID])
		if token == "" {
			token = strings.TrimSpace(settings[SettingTelegramBotToken])
		}
		if chatID == "" {
			chatID = strings.TrimSpace(settings[SettingTelegramChatID])
		}
	}
	if token == "" || (lane == telegramAdminLane && chatID == "") {
		return nil, nil
	}

	cfg := telegramConfig{Token: token, ChatID: chatID}
	s.mu.Lock()
	if s.cached == nil {
		s.cached = make(map[string]telegramConfigCache)
	}
	s.cached[lane] = telegramConfigCache{config: cfg, expires: time.Now().Add(60 * time.Second)}
	s.mu.Unlock()
	return &cfg, nil
}

func (s *TelegramNotifyService) isEnabled(ctx context.Context, settingKey string) bool {
	if s == nil || s.settingRepo == nil {
		return false
	}
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
	return s.sendWithConfig(ctx, cfg, cfg.ChatID, text)
}

func (s *TelegramNotifyService) sendAdminMessage(ctx context.Context, text string) error {
	return s.sendMessage(ctx, text)
}

// sendCustomerMessage deliberately accepts the recipient separately from the
// saved customer-bot configuration. A missing customer token never falls back
// to the admin token.
func (s *TelegramNotifyService) sendCustomerMessage(ctx context.Context, chatID, text string) error {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return fmt.Errorf("customer telegram chat ID is empty")
	}
	cfg, err := s.getCustomerConfig(ctx)
	if err != nil {
		return err
	}
	if cfg == nil {
		return fmt.Errorf("customer telegram bot token is not configured")
	}
	return s.sendWithConfig(ctx, cfg, chatID, text)
}

func sanitizeTelegramTransportError(err error, token string) string {
	if err == nil {
		return "unknown transport error"
	}
	message := err.Error()
	if token != "" {
		message = strings.ReplaceAll(message, token, "[REDACTED]")
	}
	return message
}

func (s *TelegramNotifyService) sendWithConfig(ctx context.Context, cfg *telegramConfig, chatID, text string) error {
	apiURL := s.apiBaseURL + cfg.Token + telegramSendMethod

	form := url.Values{}
	form.Set("chat_id", chatID)
	form.Set("text", text)
	form.Set("parse_mode", "HTML")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create telegram request (%s)", sanitizeTelegramTransportError(err, cfg.Token))
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send telegram message (%s)", sanitizeTelegramTransportError(err, cfg.Token))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned status %d", resp.StatusCode)
	}
	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode telegram API response: %w", err)
	}
	if !result.OK {
		return fmt.Errorf("telegram API rejected message: %s", result.Description)
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
	s.cached = make(map[string]telegramConfigCache)
	s.mu.Unlock()
}

// SendTestMessageWithChatID sends a test message using the saved bot token but overriding the chat ID.
// This allows testing delivery to a different chat without changing saved settings.
func (s *TelegramNotifyService) SendTestMessageWithChatID(ctx context.Context, chatID string) error {
	cfg, err := s.getConfig(ctx)
	if err != nil {
		return err
	}
	if cfg == nil {
		return fmt.Errorf("telegram bot token is not configured")
	}

	text := fmt.Sprintf(
		"\xe2\x9c\x85 <b>Telegram Bot Connected</b>\n\n"+
			"Bot is configured and working correctly.\n"+
			"\xf0\x9f\x94\x94 Time: %s",
		time.Now().Format("2006-01-02 15:04:05"),
	)

	apiURL := s.apiBaseURL + cfg.Token + telegramSendMethod

	form := url.Values{}
	form.Set("chat_id", chatID)
	form.Set("text", text)
	form.Set("parse_mode", "HTML")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create telegram request (%s)", sanitizeTelegramTransportError(err, cfg.Token))
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send telegram message (%s)", sanitizeTelegramTransportError(err, cfg.Token))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned status %d", resp.StatusCode)
	}
	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode telegram API response: %w", err)
	}
	if !result.OK {
		return fmt.Errorf("telegram API rejected message: %s", result.Description)
	}

	return nil
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

// NotifyBalanceLow sends a user wallet notification through the customer bot.
// chatID is resolved from the user's Telegram attribute; it is never replaced
// with the admin chat ID. The delivery error is returned so callers can apply
// their own retry/deduplication policy without marking a failed send as done.
func (s *TelegramNotifyService) NotifyBalanceLow(ctx context.Context, chatID, userEmail string, balance, threshold float64) error {
	if !s.isEnabled(ctx, SettingTelegramNotifyCustomerBalance) {
		return fmt.Errorf("customer telegram balance notifications are disabled")
	}
	text := fmt.Sprintf(
		" <b>Balance Low</b>\n\n"+
			" Balance: <b>%.4f</b>\n"+
			" Threshold: %.4f\n"+
			" Time: %s",
		balance,
		threshold,
		time.Now().Format("2006-01-02 15:04:05"),
	)
	if err := s.sendCustomerMessage(ctx, chatID, text); err != nil {
		slog.Warn("telegram notify customer balance failed", "error", err, "user_id", userEmail)
		return err
	}
	return nil
}

// NotifyProviderBalance sends provider/influence balance alerts to the admin
// lane. The account name/id are local identifiers only; secrets are excluded.
func (s *TelegramNotifyService) NotifyProviderBalance(ctx context.Context, accountID int64, accountName, platform string, status providerBalanceAlertStatus, balance, threshold float64, reason string) error {
	if !s.isEnabled(ctx, SettingTelegramNotifyProviderBalance) {
		return fmt.Errorf("provider telegram balance notifications are disabled")
	}
	label := string(status)
	if status == providerBalanceAlertHealthy {
		label = "recovered"
	}
	text := fmt.Sprintf(
		"<b>Provider Balance %s</b>\n\n"+
			"Account: <code>%d</code> %s\n"+
			"Provider: <b>%s</b>\n"+
			"Balance: <b>%.4f</b>\n"+
			"Threshold: %.4f\n"+
			"Reason: %s\n"+
			"Time: %s",
		escapeHTML(label), accountID, escapeHTML(accountName), escapeHTML(platform),
		balance, threshold, escapeHTML(truncateTelegram(reason, 200)), time.Now().Format("2006-01-02 15:04:05"),
	)
	if err := s.sendAdminMessage(ctx, text); err != nil {
		slog.Warn("telegram notify provider balance failed", "error", err, "account_id", accountID, "platform", platform)
		return err
	}
	return nil
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
