package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

const (
	emailSendTimeout = 30 * time.Second

	// Threshold type values
	thresholdTypeFixed      = "fixed"
	thresholdTypePercentage = "percentage"

	// Quota dimension labels
	quotaDimDaily  = "daily"
	quotaDimWeekly = "weekly"
	quotaDimTotal  = "total"

	defaultSiteName = "Sub2API"
)

// quotaDimLabels maps dimension names to display labels.
var quotaDimLabels = map[string]string{
	quotaDimDaily:  "日限额 / Daily",
	quotaDimWeekly: "周限额 / Weekly",
	quotaDimTotal:  "总限额 / Total",
}

// AccountQuotaReader provides read access to account quota data.
type AccountQuotaReader interface {
	GetByID(ctx context.Context, id int64) (*Account, error)
}

// BalanceNotifyService handles balance and quota threshold notifications.
type BalanceNotifyService struct {
	emailService                *EmailService
	settingRepo                 SettingRepository
	accountRepo                 AccountQuotaReader
	notificationEmailService    *NotificationEmailService
	notificationEmailDispatcher *NotificationEmailDispatcher
}

// NewBalanceNotifyService creates a new BalanceNotifyService.
func NewBalanceNotifyService(emailService *EmailService, settingRepo SettingRepository, accountRepo AccountQuotaReader) *BalanceNotifyService {
	return &BalanceNotifyService{
		emailService: emailService,
		settingRepo:  settingRepo,
		accountRepo:  accountRepo,
	}
}

func (s *BalanceNotifyService) SetNotificationEmailService(notificationEmailService *NotificationEmailService) {
	s.notificationEmailService = notificationEmailService
}

func (s *BalanceNotifyService) SetNotificationEmailDispatcher(dispatcher *NotificationEmailDispatcher) {
	s.notificationEmailDispatcher = dispatcher
}

// resolveBalanceThreshold returns the effective balance threshold.
// For percentage type, it computes threshold = totalRecharged * percentage / 100.
func resolveBalanceThreshold(threshold float64, thresholdType string, totalRecharged float64) float64 {
	if thresholdType == thresholdTypePercentage && totalRecharged > 0 {
		return totalRecharged * threshold / 100
	}
	return threshold
}

// CheckBalanceAfterDeduction checks if balance crossed below threshold after deduction.
// Notification is sent only on first crossing: oldBalance >= threshold && newBalance < threshold.
func (s *BalanceNotifyService) CheckBalanceAfterDeduction(ctx context.Context, user *User, oldBalance, cost float64) {
	if !s.canNotifyBalance(user) {
		return
	}
	effectiveThreshold, rechargeURL, ok := s.resolveUserEffectiveThreshold(ctx, user)
	if !ok {
		return
	}
	newBalance := oldBalance - cost
	if !crossedDownward(oldBalance, newBalance, effectiveThreshold) {
		return
	}
	s.dispatchBalanceLowEmail(ctx, user, newBalance, effectiveThreshold, rechargeURL)
}

// canNotifyBalance checks nil guards and user-level toggle.
func (s *BalanceNotifyService) canNotifyBalance(user *User) bool {
	if user == nil || s.notificationEmailDispatcher == nil || s.settingRepo == nil {
		return false
	}
	return user.BalanceNotifyEnabled
}

// resolveUserEffectiveThreshold reads global + user config, returns the effective threshold.
// Returns ok=false when notifications should be skipped.
func (s *BalanceNotifyService) resolveUserEffectiveThreshold(ctx context.Context, user *User) (effectiveThreshold float64, rechargeURL string, ok bool) {
	globalEnabled, globalThreshold, rechargeURL := s.getBalanceNotifyConfig(ctx)
	if !globalEnabled {
		return 0, "", false
	}
	threshold := globalThreshold
	if user.BalanceNotifyThreshold != nil {
		threshold = *user.BalanceNotifyThreshold
	}
	if threshold <= 0 {
		return 0, "", false
	}
	effectiveThreshold = resolveBalanceThreshold(threshold, user.BalanceNotifyThresholdType, user.TotalRecharged)
	if effectiveThreshold <= 0 {
		return 0, "", false
	}
	return effectiveThreshold, rechargeURL, true
}

// crossedDownward returns true when oldV was at-or-above threshold but newV dropped below it.
func crossedDownward(oldV, newV, threshold float64) bool {
	return oldV >= threshold && newV < threshold
}

// dispatchBalanceLowEmail collects recipients and sends the alert in a goroutine.
func (s *BalanceNotifyService) dispatchBalanceLowEmail(ctx context.Context, user *User, newBalance, threshold float64, rechargeURL string) {
	siteName := s.getSiteName(ctx)
	recipients := s.collectBalanceNotifyRecipients(ctx, user)
	slog.Info("CheckBalanceAfterDeduction: sending notification",
		"user_id", user.ID, "recipients", recipients, "new_balance", newBalance, "threshold", threshold)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in balance notification", "recover", r)
			}
		}()
		s.sendBalanceLowEmails(recipients, user.ID, user.Username, user.Email, newBalance, threshold, siteName, rechargeURL)
	}()
}

// quotaDim describes one quota dimension for notification checking.
type quotaDim struct {
	name          string
	enabled       bool
	threshold     float64
	thresholdType string // "fixed" (default) or "percentage"
	currentUsed   float64
	limit         float64
}

// resolvedThreshold converts the user-facing "remaining" threshold into a usage-based trigger point.
// The threshold represents how much quota REMAINS when the alert fires:
//   - Fixed ($): threshold=400, limit=1000 → fires when usage reaches 600 (remaining drops to 400)
//   - Percentage (%): threshold=30, limit=1000 → fires when usage reaches 700 (remaining drops to 30%)
func (d quotaDim) resolvedThreshold() float64 {
	if d.limit <= 0 {
		return 0
	}
	if d.thresholdType == thresholdTypePercentage {
		return d.limit * (1 - d.threshold/100)
	}
	return d.limit - d.threshold
}

// buildQuotaDims returns the three quota dimensions for notification checking.
func buildQuotaDims(account *Account) []quotaDim {
	return []quotaDim{
		{quotaDimDaily, account.GetQuotaNotifyDailyEnabled(), account.GetQuotaNotifyDailyThreshold(), account.GetQuotaNotifyDailyThresholdType(), account.GetQuotaDailyUsed(), account.GetQuotaDailyLimit()},
		{quotaDimWeekly, account.GetQuotaNotifyWeeklyEnabled(), account.GetQuotaNotifyWeeklyThreshold(), account.GetQuotaNotifyWeeklyThresholdType(), account.GetQuotaWeeklyUsed(), account.GetQuotaWeeklyLimit()},
		{quotaDimTotal, account.GetQuotaNotifyTotalEnabled(), account.GetQuotaNotifyTotalThreshold(), account.GetQuotaNotifyTotalThresholdType(), account.GetQuotaUsed(), account.GetQuotaLimit()},
	}
}

// buildQuotaDimsFromState builds quota dimensions using DB transaction state instead of account snapshot.
// Notification settings (enabled, threshold, thresholdType) come from the account; usage values from quotaState.
func buildQuotaDimsFromState(account *Account, state *AccountQuotaState) []quotaDim {
	return []quotaDim{
		{quotaDimDaily, account.GetQuotaNotifyDailyEnabled(), account.GetQuotaNotifyDailyThreshold(), account.GetQuotaNotifyDailyThresholdType(), state.DailyUsed, state.DailyLimit},
		{quotaDimWeekly, account.GetQuotaNotifyWeeklyEnabled(), account.GetQuotaNotifyWeeklyThreshold(), account.GetQuotaNotifyWeeklyThresholdType(), state.WeeklyUsed, state.WeeklyLimit},
		{quotaDimTotal, account.GetQuotaNotifyTotalEnabled(), account.GetQuotaNotifyTotalThreshold(), account.GetQuotaNotifyTotalThresholdType(), state.TotalUsed, state.TotalLimit},
	}
}

// CheckAccountQuotaAfterIncrement checks if any quota dimension crossed above its notify threshold.
// When quotaState is non-nil (from DB transaction RETURNING), it is used directly for threshold
// checking, avoiding a separate DB read. Otherwise it falls back to fetching fresh account data.
func (s *BalanceNotifyService) CheckAccountQuotaAfterIncrement(ctx context.Context, account *Account, cost float64, quotaState *AccountQuotaState) {
	if account == nil || s.notificationEmailDispatcher == nil || s.settingRepo == nil || cost <= 0 {
		return
	}
	if !s.shouldSendAccountQuotaNotifications(ctx) {
		return
	}
	adminEmails := s.getAccountQuotaNotifyEmails(ctx)
	if len(adminEmails) == 0 {
		return
	}

	siteName := s.getSiteName(ctx)
	var dims []quotaDim
	if quotaState != nil {
		dims = buildQuotaDimsFromState(account, quotaState)
	} else {
		freshAccount := s.fetchFreshAccount(ctx, account)
		dims = buildQuotaDims(freshAccount)
		account = freshAccount // use fresh data for alert metadata
	}
	s.checkQuotaDimCrossings(account, dims, cost, adminEmails, siteName)
}

func (s *BalanceNotifyService) shouldSendAccountQuotaNotifications(ctx context.Context) bool {
	if s.notificationEmailService != nil {
		channel, configured, err := s.notificationEmailService.GetChannelPolicyState(ctx, NotificationEmailChannelAccountQuota)
		if err == nil && configured {
			return channel.Enabled
		}
	}
	return s.isAccountQuotaNotifyEnabled(ctx)
}

// fetchFreshAccount loads the latest account from DB; falls back to the snapshot on error.
func (s *BalanceNotifyService) fetchFreshAccount(ctx context.Context, snapshot *Account) *Account {
	if s.accountRepo == nil {
		return snapshot
	}
	fresh, err := s.accountRepo.GetByID(ctx, snapshot.ID)
	if err != nil {
		slog.Warn("failed to fetch fresh account for quota notify, using snapshot",
			"account_id", snapshot.ID, "error", err)
		return snapshot
	}
	return fresh
}

// checkQuotaDimCrossings iterates pre-built quota dimensions and sends alerts for threshold crossings.
// Pre-increment value is reconstructed as currentUsed - cost to detect the crossing moment.
func (s *BalanceNotifyService) checkQuotaDimCrossings(account *Account, dims []quotaDim, cost float64, adminEmails []string, siteName string) {
	for _, dim := range dims {
		if !dim.enabled || dim.threshold <= 0 {
			continue
		}
		effectiveThreshold := dim.resolvedThreshold()
		if effectiveThreshold <= 0 {
			continue
		}
		newUsed := dim.currentUsed
		oldUsed := dim.currentUsed - cost
		if oldUsed < effectiveThreshold && newUsed >= effectiveThreshold {
			s.asyncSendQuotaAlert(adminEmails, account.ID, account.Name, account.Platform, dim, newUsed, effectiveThreshold, siteName)
		}
	}
}

// asyncSendQuotaAlert sends quota alert email in a goroutine with panic recovery.
func (s *BalanceNotifyService) asyncSendQuotaAlert(adminEmails []string, accountID int64, accountName, platform string, dim quotaDim, newUsed, effectiveThreshold float64, siteName string) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in quota notification", "recover", r)
			}
		}()
		s.sendQuotaAlertEmails(adminEmails, accountID, accountName, platform, dim, newUsed, siteName)
	}()
}

// getBalanceNotifyConfig reads global balance notification settings.
func (s *BalanceNotifyService) getBalanceNotifyConfig(ctx context.Context) (enabled bool, threshold float64, rechargeURL string) {
	keys := []string{SettingKeyBalanceLowNotifyEnabled, SettingKeyBalanceLowNotifyThreshold, SettingKeyBalanceLowNotifyRechargeURL}
	settings, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return false, 0, ""
	}
	enabled = settings[SettingKeyBalanceLowNotifyEnabled] == "true"
	if v := settings[SettingKeyBalanceLowNotifyThreshold]; v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			threshold = f
		}
	}
	rechargeURL = settings[SettingKeyBalanceLowNotifyRechargeURL]
	return
}

// isAccountQuotaNotifyEnabled checks the global account quota notification toggle.
func (s *BalanceNotifyService) isAccountQuotaNotifyEnabled(ctx context.Context) bool {
	val, err := s.settingRepo.GetValue(ctx, SettingKeyAccountQuotaNotifyEnabled)
	if err != nil {
		return false
	}
	return val == "true"
}

// getAccountQuotaNotifyEmails reads admin notification emails from settings,
// filtering out disabled and unverified entries.
func (s *BalanceNotifyService) getAccountQuotaNotifyEmails(ctx context.Context) []string {
	if s.notificationEmailService != nil {
		if recipients, err := s.notificationEmailService.ResolveGroupRecipients(ctx, NotificationEmailChannelAccountQuota); err == nil && recipients != nil {
			return recipients
		}
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyAccountQuotaNotifyEmails)
	if err != nil || strings.TrimSpace(raw) == "" || raw == "[]" {
		return nil
	}

	entries := ParseNotifyEmails(raw)
	if len(entries) == 0 {
		return nil
	}

	return filterVerifiedEmails(entries)
}

// getSiteName reads site name from settings with fallback.
func (s *BalanceNotifyService) getSiteName(ctx context.Context) string {
	name, err := s.settingRepo.GetValue(ctx, SettingKeySiteName)
	if err != nil || name == "" {
		return defaultSiteName
	}
	return name
}

// filterVerifiedEmails returns deduplicated, non-disabled, verified emails.
func filterVerifiedEmails(entries []NotifyEmailEntry) []string {
	var recipients []string
	seen := make(map[string]bool)
	for _, entry := range entries {
		if entry.Disabled || !entry.Verified {
			continue
		}
		email := strings.TrimSpace(entry.Email)
		if email == "" {
			continue
		}
		lower := strings.ToLower(email)
		if seen[lower] {
			continue
		}
		seen[lower] = true
		recipients = append(recipients, email)
	}
	return recipients
}

// collectBalanceNotifyRecipients returns verified, non-disabled email recipients.
// Only emails with verified=true and disabled=false are included.
func (s *BalanceNotifyService) collectBalanceNotifyRecipients(ctx context.Context, user *User) []string {
	if s.notificationEmailService != nil {
		if recipients, err := s.notificationEmailService.ResolveUserRecipients(ctx, NotificationEmailChannelBalance, user.Email, user.BalanceNotifyExtraEmails); err == nil {
			return recipients
		}
	}
	return filterVerifiedEmails(user.BalanceNotifyExtraEmails)
}

// sendBalanceLowEmails sends balance low notification to all recipients.
func (s *BalanceNotifyService) sendBalanceLowEmails(recipients []string, userID int64, userName, userEmail string, balance, threshold float64, siteName, rechargeURL string) {
	displayName := userName
	if displayName == "" {
		displayName = userEmail
	}
	if s.notificationEmailDispatcher == nil {
		slog.Warn("balance low notification dispatcher is not configured", "user_id", userID)
		return
	}
	for _, to := range recipients {
		ctx, cancel := context.WithTimeout(context.Background(), notificationEmailDeliveryDBTimeout)
		_, err := s.notificationEmailDispatcher.Enqueue(ctx, NotificationEmailSendInput{
			Event:          NotificationEmailEventBalanceLow,
			RecipientEmail: to,
			RecipientName:  displayName,
			UserID:         userID,
			SourceType:     "balance_low",
			SourceID:       firstNonEmpty(strconv.FormatInt(userID, 10), userEmail),
			ReminderKey:    time.Now().UTC().Format("2006-01-02"),
			Variables: map[string]string{
				"current_balance": fmt.Sprintf("%.2f", balance),
				"threshold":       fmt.Sprintf("%.2f", threshold),
				"recharge_url":    rechargeURL,
			},
		})
		cancel()
		if err != nil && !errors.Is(err, ErrNotificationEmailChannelDisabled) {
			slog.Warn("enqueue balance low notification failed", "recipient_hash", notificationEmailHash(to), "user_id", userID, "err", err.Error())
		}
	}
}

// sendQuotaAlertEmails sends quota alert notification to admin emails.
func (s *BalanceNotifyService) sendQuotaAlertEmails(adminEmails []string, accountID int64, accountName, platform string, dim quotaDim, used float64, siteName string) {
	dimLabel := quotaDimLabels[dim.name]
	if dimLabel == "" {
		dimLabel = dim.name
	}

	// Format the remaining-based threshold for display
	thresholdDisplay := fmt.Sprintf("$%.2f", dim.threshold)
	if dim.thresholdType == thresholdTypePercentage {
		thresholdDisplay = fmt.Sprintf("%.0f%%", dim.threshold)
	}
	remaining := dim.limit - used
	if remaining < 0 {
		remaining = 0
	}

	if s.notificationEmailDispatcher == nil {
		slog.Warn("account quota notification dispatcher is not configured", "account_id", accountID)
		return
	}
	for _, to := range adminEmails {
		ctx, cancel := context.WithTimeout(context.Background(), notificationEmailDeliveryDBTimeout)
		_, err := s.notificationEmailDispatcher.Enqueue(ctx, NotificationEmailSendInput{
			Event:          NotificationEmailEventAccountQuotaAlert,
			RecipientEmail: to,
			RecipientName:  emailRecipientName(to),
			SourceType:     "account_quota",
			SourceID:       fmt.Sprintf("%d-%s", accountID, dim.name),
			ReminderKey:    time.Now().UTC().Format("2006-01-02"),
			Variables: map[string]string{
				"account_id":      strconv.FormatInt(accountID, 10),
				"account_name":    accountName,
				"platform":        platform,
				"quota_dimension": dimLabel,
				"quota_used":      fmt.Sprintf("%.2f", used),
				"quota_limit":     fmt.Sprintf("%.2f", dim.limit),
				"quota_remaining": fmt.Sprintf("%.2f", remaining),
				"quota_threshold": thresholdDisplay,
			},
		})
		cancel()
		if err != nil && !errors.Is(err, ErrNotificationEmailChannelDisabled) {
			slog.Warn("enqueue account quota notification failed", "recipient_hash", notificationEmailHash(to), "account_id", accountID, "dimension", dim.name, "err", err.Error())
		}
	}
}

// sanitizeEmailHeader removes CR/LF characters to prevent SMTP header injection.
func sanitizeEmailHeader(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}
