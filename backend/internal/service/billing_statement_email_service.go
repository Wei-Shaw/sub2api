package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
)

// ─────────────────────────────────────────────────────────────────────────────
// Configuration model (stored as JSON in setting key billing_statement_email_config)
// ─────────────────────────────────────────────────────────────────────────────

// BillingStatementEmailConfig is the JSON structure stored in the setting key.
type BillingStatementEmailConfig struct {
	Enabled         bool   `json:"enabled"`
	DailyEnabled    bool   `json:"daily_enabled"`
	WeeklyEnabled   bool   `json:"weekly_enabled"`
	MonthlyEnabled  bool   `json:"monthly_enabled"`
	DailySchedule   string `json:"daily_schedule"`   // cron spec (5-field)
	WeeklySchedule  string `json:"weekly_schedule"`  // cron spec (5-field)
	MonthlySchedule string `json:"monthly_schedule"` // cron spec (5-field)
}

// DefaultBillingStatementEmailConfig returns the default configuration.
func DefaultBillingStatementEmailConfig() BillingStatementEmailConfig {
	return BillingStatementEmailConfig{
		Enabled:         false,
		DailyEnabled:    false,
		WeeklyEnabled:   false,
		MonthlyEnabled:  false,
		DailySchedule:   "0 8 * * *", // every day at 08:00
		WeeklySchedule:  "0 8 * * 1", // every Monday at 08:00
		MonthlySchedule: "0 8 1 * *", // 1st of month at 08:00
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// User preference model (stored per-user in setting key billing_statement_user_pref:{userID})
// ─────────────────────────────────────────────────────────────────────────────

// BillingStatementUserPreference represents a user's opt-in/out for each billing period.
type BillingStatementUserPreference struct {
	DailyEnabled   bool `json:"daily_enabled"`
	WeeklyEnabled  bool `json:"weekly_enabled"`
	MonthlyEnabled bool `json:"monthly_enabled"`
}

// DefaultBillingStatementUserPreference returns the default preference (all disabled).
// Existing users without an explicit preference JSON should not receive billing
// statements until they opt in from their profile.
func DefaultBillingStatementUserPreference() BillingStatementUserPreference {
	return BillingStatementUserPreference{
		DailyEnabled:   false,
		WeeklyEnabled:  false,
		MonthlyEnabled: false,
	}
}

// ParseBillingStatementUserPreference parses JSON into preference, falling back to defaults.
func ParseBillingStatementUserPreference(raw string) BillingStatementUserPreference {
	pref := DefaultBillingStatementUserPreference()
	if strings.TrimSpace(raw) == "" {
		return pref
	}
	if err := json.Unmarshal([]byte(raw), &pref); err != nil {
		return DefaultBillingStatementUserPreference()
	}
	return pref
}

// billingStatementUserPreferenceSettingKey returns the setting key for a user's billing preference.
func billingStatementUserPreferenceSettingKey(userID int64) string {
	return SettingKeyBillingStatementUserPreferencePrefix + strconv.FormatInt(userID, 10)
}

// ParseBillingStatementEmailConfig parses JSON into config, falling back to defaults.
func ParseBillingStatementEmailConfig(raw string) BillingStatementEmailConfig {
	cfg := DefaultBillingStatementEmailConfig()
	if strings.TrimSpace(raw) == "" {
		return cfg
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return DefaultBillingStatementEmailConfig()
	}
	return cfg
}

// ─────────────────────────────────────────────────────────────────────────────
// Aggregation DTO (computed from usage_logs)
// ─────────────────────────────────────────────────────────────────────────────

// BillingStatementLine represents one aggregated line in the billing statement.
type BillingStatementLine struct {
	Model        string  `json:"model"`
	BillingMode  string  `json:"billing_mode"` // "token" / "image" / ""
	GroupID      *int64  `json:"group_id"`
	GroupName    string  `json:"group_name"`
	Subscription *int64  `json:"subscription_id"`
	Requests     int64   `json:"requests"`
	TotalTokens  int64   `json:"total_tokens"`
	TotalCost    float64 `json:"total_cost"`  // standard price
	ActualCost   float64 `json:"actual_cost"` // user price after multiplier
	Discount     float64 `json:"discount"`    // total_cost - actual_cost
}

// BillingStatement is the full statement for one user in a time range.
type BillingStatement struct {
	UserID     int64
	UserEmail  string
	PeriodName string // "日账单" / "周账单" / "月账单"
	Start      time.Time
	End        time.Time
	Timezone   string
	Lines      []BillingStatementLine
	TotalCost  float64
	ActualCost float64
	Discount   float64
	Balance    float64
}

// ─────────────────────────────────────────────────────────────────────────────
// Service
// ─────────────────────────────────────────────────────────────────────────────

const (
	billingStatementLeaderLockKey = "billing_statement:leader"
	billingStatementLeaderLockTTL = 5 * time.Minute

	billingStatementLastRunKeyPrefix = "billing_statement:last_run:"
	billingStatementTickInterval     = 1 * time.Minute

	billingStatementUserPageSize = 100
)

var billingStatementCronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

type billingStatementRedisClient interface {
	SetNX(ctx context.Context, key string, value any, expiration time.Duration) (bool, error)
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value any, expiration time.Duration) error
	ReleaseIfValue(ctx context.Context, key string, value string) error
}

// BillingStatementEmailService sends periodic billing statement emails to users.
type BillingStatementEmailService struct {
	settingRepo  SettingRepository
	userRepo     UserRepository
	groupRepo    GroupRepository
	usageRepo    UsageLogRepository
	emailService *EmailService
	redisClient  billingStatementRedisClient
	cfg          *config.Config

	instanceID string
	loc        *time.Location

	distributedLockOn bool
	warnNoRedisOnce   sync.Once

	startOnce sync.Once
	stopOnce  sync.Once
	stopCtx   context.Context
	stop      context.CancelFunc
	wg        sync.WaitGroup
}

// NewBillingStatementEmailService creates the service.
func NewBillingStatementEmailService(
	settingRepo SettingRepository,
	userRepo UserRepository,
	groupRepo GroupRepository,
	usageRepo UsageLogRepository,
	emailService *EmailService,
	redisClient billingStatementRedisClient,
	cfg *config.Config,
) *BillingStatementEmailService {
	lockOn := cfg == nil || strings.TrimSpace(cfg.RunMode) != config.RunModeSimple

	loc := time.Local
	if cfg != nil && strings.TrimSpace(cfg.Timezone) != "" {
		if parsed, err := time.LoadLocation(strings.TrimSpace(cfg.Timezone)); err == nil && parsed != nil {
			loc = parsed
		}
	}
	return &BillingStatementEmailService{
		settingRepo:       settingRepo,
		userRepo:          userRepo,
		groupRepo:         groupRepo,
		usageRepo:         usageRepo,
		emailService:      emailService,
		redisClient:       redisClient,
		cfg:               cfg,
		instanceID:        uuid.NewString(),
		loc:               loc,
		distributedLockOn: lockOn,
	}
}

// Start begins the background ticker.
func (s *BillingStatementEmailService) Start() {
	s.StartWithContext(context.Background())
}

// StartWithContext begins the background ticker with a parent context.
func (s *BillingStatementEmailService) StartWithContext(ctx context.Context) {
	if s == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if s.emailService == nil || s.settingRepo == nil {
		return
	}

	s.startOnce.Do(func() {
		s.stopCtx, s.stop = context.WithCancel(ctx)
		s.wg.Add(1)
		go s.run()
	})
}

// Stop gracefully stops the service.
func (s *BillingStatementEmailService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.stop != nil {
			s.stop()
		}
	})
	s.wg.Wait()
}

func (s *BillingStatementEmailService) run() {
	defer s.wg.Done()

	ticker := time.NewTicker(billingStatementTickInterval)
	defer ticker.Stop()

	s.runOnce()
	for {
		select {
		case <-ticker.C:
			s.runOnce()
		case <-s.stopCtx.Done():
			return
		}
	}
}

func (s *BillingStatementEmailService) runOnce() {
	if s == nil || s.emailService == nil || s.settingRepo == nil {
		return
	}

	ctx, cancel := context.WithTimeout(s.stopCtx, 120*time.Second)
	defer cancel()

	// Read config from settings
	billingCfg := s.loadConfig(ctx)
	if !billingCfg.Enabled {
		return
	}

	// Acquire leader lock
	release, ok := s.tryAcquireLeaderLock(ctx)
	if !ok {
		return
	}
	if release != nil {
		defer release()
	}

	now := time.Now()
	if s.loc != nil {
		now = now.In(s.loc)
	}

	type statementDef struct {
		enabled  bool
		kind     string
		name     string
		schedule string
	}

	defs := []statementDef{
		{enabled: billingCfg.DailyEnabled, kind: "daily", name: "日账单 / Daily Billing Statement", schedule: billingCfg.DailySchedule},
		{enabled: billingCfg.WeeklyEnabled, kind: "weekly", name: "周账单 / Weekly Billing Statement", schedule: billingCfg.WeeklySchedule},
		{enabled: billingCfg.MonthlyEnabled, kind: "monthly", name: "月账单 / Monthly Billing Statement", schedule: billingCfg.MonthlySchedule},
	}

	for _, d := range defs {
		if !d.enabled {
			continue
		}
		spec := strings.TrimSpace(d.schedule)
		if spec == "" {
			continue
		}
		sched, err := billingStatementCronParser.Parse(spec)
		if err != nil {
			log.Printf("[BillingStatement] invalid cron spec=%q for kind=%s: %v", spec, d.kind, err)
			continue
		}

		lastRun := s.getLastRunAt(ctx, d.kind)
		base := lastRun
		if base.IsZero() {
			base = now.Add(-1 * time.Minute)
		}
		next := sched.Next(base)
		if next.IsZero() || next.After(now) {
			continue
		}
		if !s.isEmailDeliveryConfigured(ctx) {
			continue
		}

		// Time to run this statement. Only record last_run after the
		// send cycle completes without the context being canceled/timing out,
		// so interrupted runs remain eligible for retry on the next pass.
		s.sendStatements(ctx, d.kind, d.name, now)
		if ctx.Err() != nil {
			log.Printf("[BillingStatement] send interrupted for kind=%s; last_run not updated: %v", d.kind, ctx.Err())
			continue
		}
		s.setLastRunAt(ctx, d.kind, now)
	}
}

func (s *BillingStatementEmailService) isEmailDeliveryConfigured(ctx context.Context) bool {
	if s == nil || s.emailService == nil {
		return false
	}
	if _, err := s.emailService.GetSMTPConfig(ctx); err != nil {
		log.Printf("[BillingStatement] email delivery is not configured; skipping: %v", err)
		return false
	}
	return true
}

func (s *BillingStatementEmailService) loadConfig(ctx context.Context) BillingStatementEmailConfig {
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyBillingStatementEmailConfig)
	if err != nil || strings.TrimSpace(raw) == "" {
		return DefaultBillingStatementEmailConfig()
	}
	return ParseBillingStatementEmailConfig(raw)
}

func (s *BillingStatementEmailService) sendStatements(ctx context.Context, kind string, periodName string, now time.Time) {
	page := 1
	for {
		users, pageResult, err := s.userRepo.List(ctx, pagination.PaginationParams{
			Page:     page,
			PageSize: billingStatementUserPageSize,
		})
		if err != nil {
			log.Printf("[BillingStatement] list users page=%d error: %v", page, err)
			return
		}

		for i := range users {
			user := &users[i]
			if !isValidEmailForBilling(user.Email) {
				continue
			}
			// Check user preference for this period kind
			if !s.isUserPeriodEnabled(ctx, user.ID, kind) {
				continue
			}
			loc := s.userLocation(ctx, user.ID)
			start, end := billingStatementPeriodRange(kind, now, loc)
			s.sendStatementToUser(ctx, user, periodName, start, end, loc)
		}

		if pageResult == nil || page >= pageResult.Pages {
			break
		}
		page++
	}
}

func (s *BillingStatementEmailService) userLocation(ctx context.Context, userID int64) *time.Location {
	if s == nil {
		return time.Local
	}
	if s.settingRepo != nil {
		if raw, err := s.settingRepo.GetValue(ctx, userTimezoneSettingKey(userID)); err == nil {
			if loc, err := time.LoadLocation(strings.TrimSpace(raw)); err == nil && loc != nil {
				return loc
			}
		}
	}
	if s.loc != nil {
		return s.loc
	}
	return time.Local
}

func billingStatementPeriodRange(kind string, now time.Time, loc *time.Location) (time.Time, time.Time) {
	if loc == nil {
		loc = time.Local
	}
	localNow := now.In(loc)
	switch kind {
	case "weekly":
		end := startOfBillingStatementWeek(localNow, loc)
		return end.AddDate(0, 0, -7), end
	case "monthly":
		end := time.Date(localNow.Year(), localNow.Month(), 1, 0, 0, 0, 0, loc)
		return end.AddDate(0, -1, 0), end
	default:
		end := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)
		return end.AddDate(0, 0, -1), end
	}
}

func startOfBillingStatementWeek(t time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}
	t = t.In(loc)
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return time.Date(t.Year(), t.Month(), t.Day()-weekday+1, 0, 0, 0, 0, loc)
}

// isUserPeriodEnabled checks whether the user has opted in for the given period kind.
func (s *BillingStatementEmailService) isUserPeriodEnabled(ctx context.Context, userID int64, kind string) bool {
	raw, err := s.settingRepo.GetValue(ctx, billingStatementUserPreferenceSettingKey(userID))
	if err != nil || strings.TrimSpace(raw) == "" {
		// Default: no explicit user opt-in, do not send.
		return false
	}
	pref := ParseBillingStatementUserPreference(raw)
	switch kind {
	case "daily":
		return pref.DailyEnabled
	case "weekly":
		return pref.WeeklyEnabled
	case "monthly":
		return pref.MonthlyEnabled
	default:
		return true
	}
}

func (s *BillingStatementEmailService) sendStatementToUser(ctx context.Context, user *User, periodName string, start, end time.Time, loc *time.Location) {
	// Aggregate usage for this user in the time range
	lines := s.aggregateUserUsage(ctx, user.ID, start, end)
	if len(lines) == 0 {
		// No usage in this period, skip sending
		return
	}

	var totalCost, actualCost, discount float64
	for _, l := range lines {
		totalCost += l.TotalCost
		actualCost += l.ActualCost
		discount += l.Discount
	}

	stmt := &BillingStatement{
		UserID:     user.ID,
		UserEmail:  user.Email,
		PeriodName: periodName,
		Start:      start,
		End:        end,
		Timezone:   billingStatementLocationName(loc),
		Lines:      lines,
		TotalCost:  totalCost,
		ActualCost: actualCost,
		Discount:   discount,
		Balance:    user.Balance,
	}

	subject := fmt.Sprintf("[%s] %s (%s ~ %s)",
		"Sub2API",
		periodName,
		start.In(loc).Format("2006-01-02"),
		end.In(loc).Format("2006-01-02"),
	)

	// Try to get site name
	if siteName, err := s.settingRepo.GetValue(ctx, SettingKeySiteName); err == nil && strings.TrimSpace(siteName) != "" {
		subject = fmt.Sprintf("[%s] %s (%s ~ %s)",
			strings.TrimSpace(siteName),
			periodName,
			start.In(loc).Format("2006-01-02"),
			end.In(loc).Format("2006-01-02"),
		)
	}

	body := buildBillingStatementEmailHTML(stmt)
	if err := s.emailService.SendEmail(ctx, user.Email, subject, body); err != nil {
		log.Printf("[BillingStatement] send email to %s failed: %v", user.Email, err)
	}
}

func billingStatementLocationName(loc *time.Location) string {
	if loc == nil {
		return "UTC"
	}
	name := strings.TrimSpace(loc.String())
	if name == "" || name == "Local" {
		return "UTC"
	}
	return name
}

func (s *BillingStatementEmailService) aggregateUserUsage(ctx context.Context, userID int64, start, end time.Time) []BillingStatementLine {
	// Use ListByUserAndTimeRange to get raw logs, then aggregate in-memory.
	// This is the minimal viable approach using existing repository methods.
	logs, _, err := s.usageRepo.ListByUserAndTimeRange(ctx, userID, start, end)
	if err != nil {
		log.Printf("[BillingStatement] query usage for user=%d error: %v", userID, err)
		return nil
	}
	if len(logs) == 0 {
		return nil
	}

	type aggKey struct {
		Model        string
		BillingMode  string
		GroupID      int64 // 0 = nil
		Subscription int64 // 0 = nil
	}

	agg := make(map[aggKey]*BillingStatementLine)
	for i := range logs {
		l := &logs[i]
		bm := ""
		if l.BillingMode != nil {
			bm = *l.BillingMode
		}
		var gid int64
		if l.GroupID != nil {
			gid = *l.GroupID
		}
		var sid int64
		if l.SubscriptionID != nil {
			sid = *l.SubscriptionID
		}
		key := aggKey{
			Model:        l.Model,
			BillingMode:  bm,
			GroupID:      gid,
			Subscription: sid,
		}
		entry, ok := agg[key]
		if !ok {
			entry = &BillingStatementLine{
				Model:       l.Model,
				BillingMode: bm,
			}
			if gid != 0 {
				g := gid
				entry.GroupID = &g
			}
			if sid != 0 {
				ss := sid
				entry.Subscription = &ss
			}
			agg[key] = entry
		}
		entry.Requests++
		entry.TotalTokens += int64(l.TotalTokens())
		entry.TotalCost += l.TotalCost
		entry.ActualCost += l.ActualCost
		entry.Discount += l.TotalCost - l.ActualCost
	}

	result := make([]BillingStatementLine, 0, len(agg))
	for _, v := range agg {
		result = append(result, *v)
	}
	s.hydrateBillingStatementGroupNames(ctx, result)
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Model != result[j].Model {
			return result[i].Model < result[j].Model
		}
		if result[i].GroupName != result[j].GroupName {
			return result[i].GroupName < result[j].GroupName
		}
		return result[i].BillingMode < result[j].BillingMode
	})
	return result
}

func (s *BillingStatementEmailService) hydrateBillingStatementGroupNames(ctx context.Context, lines []BillingStatementLine) {
	if s == nil || s.groupRepo == nil || len(lines) == 0 {
		return
	}
	cache := make(map[int64]string)
	for i := range lines {
		if lines[i].GroupID == nil {
			continue
		}
		groupID := *lines[i].GroupID
		if name, ok := cache[groupID]; ok {
			lines[i].GroupName = name
			continue
		}
		group, err := s.groupRepo.GetByIDLite(ctx, groupID)
		if err != nil || group == nil || strings.TrimSpace(group.Name) == "" {
			cache[groupID] = fmt.Sprintf("#%d", groupID)
		} else {
			cache[groupID] = strings.TrimSpace(group.Name)
		}
		lines[i].GroupName = cache[groupID]
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Email HTML template
// ─────────────────────────────────────────────────────────────────────────────

func buildBillingStatementEmailHTML(stmt *BillingStatement) string {
	if stmt == nil {
		return "<p>无数据 / No data.</p>"
	}

	var rows strings.Builder
	for _, line := range stmt.Lines {
		billingMode := billingStatementBillingModeLabel(line.BillingMode)
		groupStr := "-"
		if strings.TrimSpace(line.GroupName) != "" {
			groupStr = strings.TrimSpace(line.GroupName)
		} else if line.GroupID != nil {
			groupStr = fmt.Sprintf("#%d", *line.GroupID)
		}
		subStr := "-"
		if line.Subscription != nil {
			subStr = fmt.Sprintf("#%d", *line.Subscription)
		}
		_, _ = rows.WriteString(fmt.Sprintf(
			"<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%d</td><td>%d</td><td>$%.4f</td><td>$%.4f</td><td>$%.4f</td></tr>",
			billingStatementHTMLEscape(line.Model),
			billingStatementHTMLEscape(billingMode),
			billingStatementHTMLEscape(groupStr),
			billingStatementHTMLEscape(subStr),
			line.Requests,
			line.TotalTokens,
			line.TotalCost,
			line.ActualCost,
			line.Discount,
		))
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<style>
body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background-color: #f5f5f5; margin: 0; padding: 20px; }
.container { max-width: 800px; margin: 0 auto; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.1); }
.header { background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%); color: white; padding: 30px; text-align: center; }
.header h1 { margin: 0; font-size: 22px; }
.header h1 .en { display: block; margin-top: 6px; font-size: 16px; font-weight: 500; opacity: 0.92; }
.content { padding: 30px; }
table { width: 100%%; border-collapse: collapse; font-size: 13px; margin-top: 16px; }
th, td { border: 1px solid #e0e0e0; padding: 8px 10px; text-align: left; }
th { background-color: #f8f9fa; font-weight: 600; }
.summary { margin-top: 20px; padding: 16px; background-color: #f8f9fa; border-radius: 6px; }
.summary p { margin: 6px 0; font-size: 14px; }
.footer { background-color: #f8f9fa; padding: 16px; text-align: center; color: #999; font-size: 12px; }
.footer p { margin: 6px 0; }
.footer .en { display: block; margin-top: 4px; }
</style>
</head>
<body>
<div class="container">
<div class="header"><h1>%s</h1></div>
<div class="content">
<p><b>致 %s：</b><br><b>To %s:</b></p>
<p><b>时间段 / Period</b>: %s ~ %s (%s)</p>
<table>
<thead><tr><th>模型 / Model</th><th>计费模式 / Billing Mode</th><th>分组 / Group</th><th>订阅 / Subscription</th><th>请求数 / Requests</th><th>Token 数 / Tokens</th><th>标准价格 / Standard Price</th><th>实际价格 / Actual Price</th><th>优惠差额 / Discount</th></tr></thead>
<tbody>%s</tbody>
</table>
<div class="summary">
<p><b>标准总价 / Total Cost</b>: $%.4f</p>
<p><b>实际总价 / Actual Cost</b>: $%.4f</p>
<p><b>优惠总额 / Total Discount</b>: $%.4f</p>
<p><b>账户余额 / Balance</b>: $%.4f</p>
</div>
</div>
<div class="footer">
<p>此邮件由系统自动发送，请勿回复。<span class="en">This is an automated message, please do not reply.</span></p>
<p>如要退订该账单，请在 个人资料 - 账单邮件偏好 进行退订。<span class="en">To unsubscribe from this statement, go to Profile - Billing Statement Email Preferences.</span></p>
</div>
</div>
</body>
</html>`,
		billingStatementBilingualTitleHTML(stmt.PeriodName),
		billingStatementHTMLEscape(stmt.UserEmail),
		billingStatementHTMLEscape(stmt.UserEmail),
		billingStatementHTMLEscape(stmt.Start.Format("2006-01-02 15:04")),
		billingStatementHTMLEscape(stmt.End.Format("2006-01-02 15:04")),
		billingStatementHTMLEscape(stmt.Timezone),
		rows.String(),
		stmt.TotalCost,
		stmt.ActualCost,
		stmt.Discount,
		stmt.Balance,
	)
}

func billingStatementBillingModeLabel(mode string) string {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "image":
		return "图片 / Image"
	case "token", "":
		return "Token"
	default:
		return mode
	}
}

func billingStatementBilingualTitleHTML(title string) string {
	parts := strings.SplitN(strings.TrimSpace(title), " / ", 2)
	if len(parts) != 2 {
		return billingStatementHTMLEscape(title)
	}
	return fmt.Sprintf("%s<span class=\"en\">%s</span>",
		billingStatementHTMLEscape(parts[0]),
		billingStatementHTMLEscape(parts[1]),
	)
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// isValidEmailForBilling returns true if the email is a real deliverable address.
func isValidEmailForBilling(email string) bool {
	addr := strings.TrimSpace(email)
	if addr == "" {
		return false
	}
	if !strings.Contains(addr, "@") {
		return false
	}
	// Skip synthetic/invalid emails from third-party auth
	if strings.HasSuffix(addr, ".invalid") {
		return false
	}
	return true
}

func billingStatementHTMLEscape(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
	)
	return replacer.Replace(s)
}

// ─────────────────────────────────────────────────────────────────────────────
// Distributed lock + last_run (same pattern as ops_scheduled_report_service)
// ─────────────────────────────────────────────────────────────────────────────

func (s *BillingStatementEmailService) tryAcquireLeaderLock(ctx context.Context) (func(), bool) {
	if s == nil || !s.distributedLockOn {
		return nil, true
	}
	if s.redisClient == nil {
		s.warnNoRedisOnce.Do(func() {
			log.Printf("[BillingStatement] redis not configured; running without distributed lock")
		})
		return nil, true
	}

	ok, err := s.redisClient.SetNX(ctx, billingStatementLeaderLockKey, s.instanceID, billingStatementLeaderLockTTL)
	if err != nil {
		log.Printf("[BillingStatement] leader lock SetNX failed; skipping: %v", err)
		return nil, false
	}
	if !ok {
		return nil, false
	}
	return func() {
		_ = s.redisClient.ReleaseIfValue(ctx, billingStatementLeaderLockKey, s.instanceID)
	}, true
}

func (s *BillingStatementEmailService) getLastRunAt(ctx context.Context, kind string) time.Time {
	if s == nil || s.redisClient == nil {
		return time.Time{}
	}
	key := billingStatementLastRunKeyPrefix + strings.TrimSpace(kind)
	raw, err := s.redisClient.Get(ctx, key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return time.Time{}
	}
	sec, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || sec <= 0 {
		return time.Time{}
	}
	last := time.Unix(sec, 0)
	if s.loc != nil {
		return last.In(s.loc)
	}
	return last.UTC()
}

func (s *BillingStatementEmailService) setLastRunAt(ctx context.Context, kind string, t time.Time) {
	if s == nil || s.redisClient == nil {
		return
	}
	key := billingStatementLastRunKeyPrefix + strings.TrimSpace(kind)
	if t.IsZero() {
		t = time.Now().UTC()
	}
	_ = s.redisClient.Set(ctx, key, strconv.FormatInt(t.UTC().Unix(), 10), 35*24*time.Hour)
}
