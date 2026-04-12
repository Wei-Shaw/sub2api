package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

const (
	emailSendTimeout = 30 * time.Second

	// Quota dimension labels
	quotaDimDaily  = "daily"
	quotaDimWeekly = "weekly"
	quotaDimTotal  = "total"
)

// quotaDimLabels maps dimension names to display labels.
var quotaDimLabels = map[string]string{
	quotaDimDaily:  "日限额 / Daily",
	quotaDimWeekly: "周限额 / Weekly",
	quotaDimTotal:  "总限额 / Total",
}

// BalanceNotifyService handles balance and quota threshold notifications.
type BalanceNotifyService struct {
	emailService *EmailService
	settingRepo  SettingRepository
}

// NewBalanceNotifyService creates a new BalanceNotifyService.
func NewBalanceNotifyService(emailService *EmailService, settingRepo SettingRepository) *BalanceNotifyService {
	return &BalanceNotifyService{
		emailService: emailService,
		settingRepo:  settingRepo,
	}
}

// CheckBalanceAfterDeduction checks if balance crossed below threshold after deduction.
// oldBalance is the balance before deduction, cost is the amount deducted.
// Notification is sent only on first crossing: oldBalance >= threshold && newBalance < threshold.
func (s *BalanceNotifyService) CheckBalanceAfterDeduction(ctx context.Context, user *User, oldBalance, cost float64) {
	if user == nil || s.emailService == nil || s.settingRepo == nil {
		return
	}

	// Check user-level switch
	if !user.BalanceNotifyEnabled {
		return
	}

	// Check global switch
	globalEnabled, threshold := s.getBalanceNotifyConfig(ctx)
	if !globalEnabled {
		return
	}

	// User custom threshold overrides system default
	if user.BalanceNotifyThreshold != nil {
		threshold = *user.BalanceNotifyThreshold
	}

	if threshold <= 0 {
		return
	}

	newBalance := oldBalance - cost

	// Only notify on first crossing
	if oldBalance >= threshold && newBalance < threshold {
		siteName := s.getSiteName(ctx)
		recipients := s.collectBalanceNotifyRecipients(user)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic in balance notification", "recover", r)
				}
			}()
			s.sendBalanceLowEmails(recipients, user.Username, user.Email, newBalance, threshold, siteName)
		}()
	}
}

// quotaDim describes one quota dimension for notification checking.
type quotaDim struct {
	name      string
	enabled   bool
	threshold float64
	oldUsed   float64
	limit     float64
}

// buildQuotaDims returns the three quota dimensions for notification checking.
func buildQuotaDims(account *Account) []quotaDim {
	return []quotaDim{
		{quotaDimDaily, account.GetQuotaNotifyDailyEnabled(), account.GetQuotaNotifyDailyThreshold(), account.GetQuotaDailyUsed(), account.GetQuotaDailyLimit()},
		{quotaDimWeekly, account.GetQuotaNotifyWeeklyEnabled(), account.GetQuotaNotifyWeeklyThreshold(), account.GetQuotaWeeklyUsed(), account.GetQuotaWeeklyLimit()},
		{quotaDimTotal, account.GetQuotaNotifyTotalEnabled(), account.GetQuotaNotifyTotalThreshold(), account.GetQuotaUsed(), account.GetQuotaLimit()},
	}
}

// CheckAccountQuotaAfterIncrement checks if any quota dimension crossed above its notify threshold.
// The account's Extra fields contain pre-increment usage values.
func (s *BalanceNotifyService) CheckAccountQuotaAfterIncrement(ctx context.Context, account *Account, cost float64) {
	if account == nil || s.emailService == nil || s.settingRepo == nil || cost <= 0 {
		return
	}
	adminEmails := s.getAccountQuotaNotifyEmails(ctx)
	if len(adminEmails) == 0 {
		return
	}

	siteName := s.getSiteName(ctx)
	for _, dim := range buildQuotaDims(account) {
		if !dim.enabled || dim.threshold <= 0 {
			continue
		}
		newUsed := dim.oldUsed + cost
		if dim.oldUsed < dim.threshold && newUsed >= dim.threshold {
			s.asyncSendQuotaAlert(adminEmails, account.Name, dim, newUsed, siteName)
		}
	}
}

// asyncSendQuotaAlert sends quota alert email in a goroutine with panic recovery.
func (s *BalanceNotifyService) asyncSendQuotaAlert(adminEmails []string, accountName string, dim quotaDim, newUsed float64, siteName string) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in quota notification", "recover", r)
			}
		}()
		s.sendQuotaAlertEmails(adminEmails, accountName, dim.name, newUsed, dim.limit, dim.threshold, siteName)
	}()
}

// getBalanceNotifyConfig reads global balance notification settings.
func (s *BalanceNotifyService) getBalanceNotifyConfig(ctx context.Context) (enabled bool, threshold float64) {
	keys := []string{SettingKeyBalanceLowNotifyEnabled, SettingKeyBalanceLowNotifyThreshold}
	settings, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return false, 0
	}
	enabled = settings[SettingKeyBalanceLowNotifyEnabled] == "true"
	if v := settings[SettingKeyBalanceLowNotifyThreshold]; v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			threshold = f
		}
	}
	return
}

// getAccountQuotaNotifyEmails reads admin notification emails from settings.
func (s *BalanceNotifyService) getAccountQuotaNotifyEmails(ctx context.Context) []string {
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyAccountQuotaNotifyEmails)
	if err != nil || strings.TrimSpace(raw) == "" || raw == "[]" {
		return nil
	}
	return parseJSONStringArray(raw)
}

// getSiteName reads site name from settings with fallback.
func (s *BalanceNotifyService) getSiteName(ctx context.Context) string {
	name, err := s.settingRepo.GetValue(ctx, SettingKeySiteName)
	if err != nil || name == "" {
		return "Sub2API"
	}
	return name
}

// collectBalanceNotifyRecipients collects all email recipients for balance notifications.
func (s *BalanceNotifyService) collectBalanceNotifyRecipients(user *User) []string {
	recipients := []string{user.Email}
	for _, extra := range user.BalanceNotifyExtraEmails {
		email := strings.TrimSpace(extra)
		if email != "" && !strings.EqualFold(email, user.Email) {
			recipients = append(recipients, email)
		}
	}
	return recipients
}

// sendEmails sends an email to all recipients with shared timeout and error logging.
func (s *BalanceNotifyService) sendEmails(recipients []string, subject, body string, logAttrs ...any) {
	for _, to := range recipients {
		ctx, cancel := context.WithTimeout(context.Background(), emailSendTimeout)
		if err := s.emailService.SendEmail(ctx, to, subject, body); err != nil {
			attrs := append([]any{"to", to, "error", err}, logAttrs...)
			slog.Error("failed to send notification", attrs...)
		}
		cancel()
	}
}

// sendBalanceLowEmails sends balance low notification to all recipients.
func (s *BalanceNotifyService) sendBalanceLowEmails(recipients []string, userName, userEmail string, balance, threshold float64, siteName string) {
	displayName := userName
	if displayName == "" {
		displayName = userEmail
	}
	subject := fmt.Sprintf("[%s] 余额不足提醒 / Balance Low Alert", siteName)
	body := s.buildBalanceLowEmailBody(displayName, balance, threshold, siteName)
	s.sendEmails(recipients, subject, body, "user_email", userEmail, "balance", balance)
}

// sendQuotaAlertEmails sends quota alert notification to admin emails.
func (s *BalanceNotifyService) sendQuotaAlertEmails(adminEmails []string, accountName, dimension string, used, limit, threshold float64, siteName string) {
	dimLabel := quotaDimLabels[dimension]
	if dimLabel == "" {
		dimLabel = dimension
	}

	subject := fmt.Sprintf("[%s] 账号限额告警 / Account Quota Alert - %s", siteName, accountName)
	body := s.buildQuotaAlertEmailBody(accountName, dimLabel, used, limit, threshold, siteName)
	s.sendEmails(adminEmails, subject, body, "account", accountName, "dimension", dimension)
}

// buildBalanceLowEmailBody builds HTML email for balance low notification.
// Lines exceed 30 due to inline HTML template (not splittable).
func (s *BalanceNotifyService) buildBalanceLowEmailBody(userName string, balance, threshold float64, siteName string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background-color: #f5f5f5; margin: 0; padding: 20px; }
        .container { max-width: 600px; margin: 0 auto; background-color: #fff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.1); }
        .header { background: linear-gradient(135deg, #f59e0b 0%%, #d97706 100%%); color: white; padding: 30px; text-align: center; }
        .header h1 { margin: 0; font-size: 24px; }
        .content { padding: 40px 30px; text-align: center; }
        .balance { font-size: 36px; font-weight: bold; color: #dc2626; margin: 20px 0; }
        .info { color: #666; font-size: 14px; line-height: 1.6; margin-top: 20px; }
        .footer { background-color: #f8f9fa; padding: 20px; text-align: center; color: #999; font-size: 12px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header"><h1>%s</h1></div>
        <div class="content">
            <p style="font-size: 18px; color: #333;">%s，您的余额不足</p>
            <p style="color: #666;">Dear %s, your balance is running low</p>
            <div class="balance">$%.2f</div>
            <div class="info">
                <p>您的账户余额已低于提醒阈值 <strong>$%.2f</strong>。</p>
                <p>Your account balance has fallen below the alert threshold of <strong>$%.2f</strong>.</p>
                <p>请及时充值以免服务中断。</p>
                <p>Please top up to avoid service interruption.</p>
            </div>
        </div>
        <div class="footer"><p>此邮件由系统自动发送，请勿回复。</p></div>
    </div>
</body>
</html>`, siteName, userName, userName, balance, threshold, threshold)
}

// buildQuotaAlertEmailBody builds HTML email for account quota alert.
// Lines exceed 30 due to inline HTML template (not splittable).
func (s *BalanceNotifyService) buildQuotaAlertEmailBody(accountName, dimLabel string, used, limit, threshold float64, siteName string) string {
	limitStr := fmt.Sprintf("$%.2f", limit)
	if limit <= 0 {
		limitStr = "无限制 / Unlimited"
	}
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background-color: #f5f5f5; margin: 0; padding: 20px; }
        .container { max-width: 600px; margin: 0 auto; background-color: #fff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.1); }
        .header { background: linear-gradient(135deg, #ef4444 0%%, #dc2626 100%%); color: white; padding: 30px; text-align: center; }
        .header h1 { margin: 0; font-size: 24px; }
        .content { padding: 40px 30px; }
        .metric { display: flex; justify-content: space-between; padding: 12px 0; border-bottom: 1px solid #eee; }
        .metric-label { color: #666; }
        .metric-value { font-weight: bold; color: #333; }
        .info { color: #666; font-size: 14px; line-height: 1.6; margin-top: 20px; text-align: center; }
        .footer { background-color: #f8f9fa; padding: 20px; text-align: center; color: #999; font-size: 12px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header"><h1>%s</h1></div>
        <div class="content">
            <p style="font-size: 18px; color: #333; text-align: center;">账号限额告警 / Account Quota Alert</p>
            <div class="metric"><span class="metric-label">账号 / Account</span><span class="metric-value">%s</span></div>
            <div class="metric"><span class="metric-label">维度 / Dimension</span><span class="metric-value">%s</span></div>
            <div class="metric"><span class="metric-label">已使用 / Used</span><span class="metric-value">$%.2f</span></div>
            <div class="metric"><span class="metric-label">限额 / Limit</span><span class="metric-value">%s</span></div>
            <div class="metric"><span class="metric-label">告警阈值 / Threshold</span><span class="metric-value">$%.2f</span></div>
            <div class="info">
                <p>账号配额用量已达到告警阈值，请及时关注。</p>
                <p>Account quota usage has reached the alert threshold.</p>
            </div>
        </div>
        <div class="footer"><p>此邮件由系统自动发送，请勿回复。</p></div>
    </div>
</body>
</html>`, siteName, accountName, dimLabel, used, limitStr, threshold)
}

// parseJSONStringArray parses a JSON string array, returns nil on error.
func parseJSONStringArray(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return nil
	}
	var result []string
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil
	}
	return result
}
