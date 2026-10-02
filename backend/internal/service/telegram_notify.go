package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// TelegramEventLogin 不是邮件事件：登录提醒只走 Telegram。
const TelegramEventLogin = "auth.login"

const telegramSendTimeout = 10 * time.Second

// telegramMessageTemplates 是各事件的 Telegram 文案（纯文本，{{var}} 替换）。
// 站点面向越南用户，而通知邮件的语言偏好只区分 en/zh（vi 会被归一成 en），
// 所以这里只出越南语一份。
var telegramMessageTemplates = map[string]string{
	NotificationEmailEventBalanceRechargeSuccess:      "✅ Nạp tiền thành công\nSố tiền: ${{recharge_amount}}\nSố dư hiện tại: ${{current_balance}}\nMã đơn: #{{order_id}}",
	NotificationEmailEventSubscriptionPurchaseSuccess: "✅ Mua gói thành công\nGói: {{subscription_group}}\nThời hạn: {{subscription_days}} ngày\nHết hạn: {{expiry_time}}\nMã đơn: #{{order_id}}",
	NotificationEmailEventBalanceLow:                  "⚠️ Số dư sắp hết\nSố dư hiện tại: ${{current_balance}} (ngưỡng ${{threshold}})\nNạp thêm: {{recharge_url}}",
	NotificationEmailEventSubscriptionExpiryReminder:  "⏰ Gói {{subscription_group}} còn {{days_remaining}} ngày\nHết hạn: {{expiry_time}}",
	TelegramEventLogin:                                "🔐 Tài khoản của bạn vừa đăng nhập\nThời gian: {{time}}\nIP: {{ip}}\nThiết bị: {{user_agent}}\nNếu không phải bạn, hãy đổi mật khẩu ngay.",
}

// telegramNotifier 通过 Telegram 登录所用的同一个 bot 给已绑定 Telegram 的用户发消息。
// 登录授权带了 request_access=write，用户无需先对 bot 发 /start；
// 用户拒绝授权或屏蔽 bot 时 Telegram 返回 403，这里只记日志，不影响邮件通道。
type telegramNotifier struct {
	settingRepo SettingRepository
	userRepo    UserRepository
	client      *http.Client
	apiBase     string
}

// EnableTelegram 打开 Telegram 通道；未调用时 NotifyTelegram 为空操作。
func (s *NotificationEmailService) EnableTelegram(userRepo UserRepository) {
	if s == nil || userRepo == nil {
		return
	}
	s.telegram = &telegramNotifier{
		settingRepo: s.settingRepo,
		userRepo:    userRepo,
		client:      &http.Client{Timeout: telegramSendTimeout},
		apiBase:     "https://api.telegram.org",
	}
}

// NotifyTelegram 在后台给 input.UserID 绑定的 Telegram 发送 input.Event 对应的提醒。
// 去重沿用邮件的 delivery key（收件人记为 telegram:<userID>），同一事件只发一次，
// 与有几个邮件收件人、任务重跑几次无关。没有 SourceID 的事件（如登录）不去重。
func (s *NotificationEmailService) NotifyTelegram(input NotificationEmailSendInput) {
	if s == nil || s.telegram == nil || input.UserID <= 0 {
		return
	}
	tmpl, ok := telegramMessageTemplates[input.Event]
	if !ok {
		return
	}
	go func() {
		defer recoverBackgroundWorker("telegram notification", nil)
		ctx, cancel := context.WithTimeout(context.Background(), telegramSendTimeout)
		defer cancel()
		if err := s.sendTelegram(ctx, input, tmpl); err != nil {
			slog.Warn("telegram notification failed", "event", input.Event, "user_id", input.UserID, "error", err)
		}
	}()
}

func (s *NotificationEmailService) sendTelegram(ctx context.Context, input NotificationEmailSendInput, tmpl string) error {
	n := s.telegram
	deliveryKey := notificationEmailDeliveryKey(input.Event, input.SourceType, input.SourceID, "telegram:"+strconv.FormatInt(input.UserID, 10), input.ReminderKey)
	if deliveryKey != "" {
		if sent, err := s.deliveryExists(ctx, deliveryKey); err != nil || sent {
			return err
		}
	}
	token, err := n.botToken(ctx)
	if err != nil || token == "" {
		return err
	}
	chatID, err := n.chatID(ctx, input.UserID)
	if err != nil || chatID == "" {
		return err
	}
	if err := n.sendMessage(ctx, token, chatID, renderTelegramMessage(tmpl, input.Variables)); err != nil {
		return err
	}
	if deliveryKey != "" {
		return s.settingRepo.Set(ctx, deliveryKey, time.Now().UTC().Format(time.RFC3339Nano))
	}
	return nil
}

// botToken 返回 Telegram 登录的 bot token；登录未开启时返回空串（不发送）。
func (n *telegramNotifier) botToken(ctx context.Context) (string, error) {
	settings, err := n.settingRepo.GetMultiple(ctx, []string{SettingKeyTelegramOAuthEnabled, SettingKeyTelegramOAuthBotToken})
	if err != nil {
		return "", err
	}
	if settings[SettingKeyTelegramOAuthEnabled] != "true" {
		return "", nil
	}
	return strings.TrimSpace(settings[SettingKeyTelegramOAuthBotToken]), nil
}

// chatID 返回用户绑定的 Telegram 用户 ID；私聊里它就是 chat_id。未绑定时返回空串。
func (n *telegramNotifier) chatID(ctx context.Context, userID int64) (string, error) {
	records, err := n.userRepo.ListUserAuthIdentities(ctx, userID)
	if err != nil {
		return "", err
	}
	for _, record := range filterUserAuthIdentities(records, "telegram") {
		if subject := strings.TrimSpace(record.ProviderSubject); subject != "" {
			return subject, nil
		}
	}
	return "", nil
}

func (n *telegramNotifier) sendMessage(ctx context.Context, token, chatID, text string) error {
	body, err := json.Marshal(map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"disable_web_page_preview": true,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.apiBase+"/bot"+token+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.client.Do(req)
	if err != nil {
		// 错误里带着含 token 的 URL，不能原样写进日志。
		return fmt.Errorf("telegram sendMessage request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("telegram sendMessage: status %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
	}
	return nil
}

// notifyLoginTelegram 每次新建会话时提醒用户本人（IP / UA 取自会话指纹中间件）。
func (s *AuthService) notifyLoginTelegram(ctx context.Context, user *User) {
	if s == nil || s.emailService == nil || user == nil {
		return
	}
	ip, userAgent := "?", "?"
	if binding := SessionBindingFromContext(ctx); binding != nil {
		ip = firstNonEmpty(strings.TrimSpace(binding.IP), ip)
		userAgent = firstNonEmpty(strings.TrimSpace(binding.UserAgent), userAgent)
	}
	s.emailService.notificationEmailService.NotifyTelegram(NotificationEmailSendInput{
		Event:  TelegramEventLogin,
		UserID: user.ID,
		Variables: map[string]string{
			"time":       timezone.Now().Format("2006-01-02 15:04:05 MST"),
			"ip":         ip,
			"user_agent": userAgent,
		},
	})
}

func renderTelegramMessage(tmpl string, variables map[string]string) string {
	out := tmpl
	for key, value := range variables {
		out = strings.ReplaceAll(out, "{{"+key+"}}", value)
	}
	return out
}
