package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	OpenAIAccountSelectionNoAvailableStatus  = 503
	OpenAIAccountSelectionNoAvailableMessage = "no available accounts"
)

// AccountSelectionErrorTemplateData contains best-effort, non-secret account
// details for local account-selection failures across supported platforms.
type AccountSelectionErrorTemplateData struct {
	NextResetAt      string
	NextResetAccount string
	AccountSummary   string
}

// OpenAIAccountSelectionErrorTemplateData is kept as a compatibility alias for
// existing templates/tests. The data itself is no longer OpenAI-only.
type OpenAIAccountSelectionErrorTemplateData = AccountSelectionErrorTemplateData

// AccountSelectionErrorTemplateData returns best-effort template values for
// local account-selection failures. It intentionally reads only non-secret
// account metadata, scheduler state, quota config, and public usage snapshots.
func (s *OpenAIGatewayService) AccountSelectionErrorTemplateData(ctx context.Context, platform string, groupID *int64) AccountSelectionErrorTemplateData {
	if s == nil {
		return AccountSelectionErrorTemplateData{}
	}
	return accountSelectionErrorTemplateData(ctx, s.accountRepo, platform, groupID)
}

// OpenAIAccountSelectionErrorTemplateData returns compatibility data for older
// OpenAI-specific call sites.
func (s *OpenAIGatewayService) OpenAIAccountSelectionErrorTemplateData(ctx context.Context, groupID *int64) OpenAIAccountSelectionErrorTemplateData {
	return s.AccountSelectionErrorTemplateData(ctx, PlatformOpenAI, groupID)
}

// AccountSelectionErrorTemplateData returns best-effort template values for
// local account-selection failures in the generic gateway.
func (s *GatewayService) AccountSelectionErrorTemplateData(ctx context.Context, platform string, groupID *int64) AccountSelectionErrorTemplateData {
	if s == nil {
		return AccountSelectionErrorTemplateData{}
	}
	return accountSelectionErrorTemplateData(ctx, s.accountRepo, platform, groupID)
}

// OpenAIAccountSelectionErrorTemplateData returns compatibility data for older
// OpenAI-specific call sites.
func (s *GatewayService) OpenAIAccountSelectionErrorTemplateData(ctx context.Context, groupID *int64) OpenAIAccountSelectionErrorTemplateData {
	return s.AccountSelectionErrorTemplateData(ctx, PlatformOpenAI, groupID)
}

func accountSelectionErrorTemplateData(ctx context.Context, accountRepo AccountRepository, platform string, groupID *int64) AccountSelectionErrorTemplateData {
	if accountRepo == nil || groupID == nil || *groupID <= 0 {
		return AccountSelectionErrorTemplateData{}
	}

	accounts, err := accountRepo.ListByGroup(ctx, *groupID)
	if err != nil {
		return AccountSelectionErrorTemplateData{}
	}

	platform = strings.TrimSpace(strings.ToLower(platform))
	selected := filterAccountsForSelectionSummary(accounts, platform)
	if len(selected) == 0 {
		if platform == "" {
			return AccountSelectionErrorTemplateData{AccountSummary: "no accounts in group"}
		}
		return AccountSelectionErrorTemplateData{AccountSummary: "no " + platform + " accounts in group"}
	}

	now := time.Now()
	items := make([]accountSelectionQuotaSummary, 0, len(selected))
	for i := range selected {
		items = append(items, buildAccountSelectionQuotaSummary(&selected[i], now))
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].NextResetAt == nil && items[j].NextResetAt != nil {
			return false
		}
		if items[i].NextResetAt != nil && items[j].NextResetAt == nil {
			return true
		}
		if items[i].NextResetAt != nil && items[j].NextResetAt != nil && !items[i].NextResetAt.Equal(*items[j].NextResetAt) {
			return items[i].NextResetAt.Before(*items[j].NextResetAt)
		}
		return items[i].ID < items[j].ID
	})

	data := AccountSelectionErrorTemplateData{
		AccountSummary: joinAccountSelectionQuotaSummaries(items),
	}
	for _, item := range items {
		if item.NextResetAt != nil {
			data.NextResetAt = item.NextResetAt.Format(time.RFC3339)
			data.NextResetAccount = item.Name
			break
		}
	}
	return data
}

func filterAccountsForSelectionSummary(accounts []Account, platform string) []Account {
	if len(accounts) == 0 {
		return nil
	}
	if platform == "" {
		return accounts
	}
	matched := make([]Account, 0, len(accounts))
	for _, account := range accounts {
		if strings.EqualFold(account.Platform, platform) {
			matched = append(matched, account)
		}
	}
	if len(matched) > 0 {
		return matched
	}
	// If a group uses mixed scheduling or a platform alias, exact platform matches
	// may be absent. Fall back to all group accounts so templates still show useful
	// non-secret diagnostics instead of blank account_summary.
	return accounts
}

// RenderAccountSelectionErrorMessage replaces template variables in custom
// local account-selection error messages.
func RenderAccountSelectionErrorMessage(message string, data AccountSelectionErrorTemplateData) string {
	replacer := strings.NewReplacer(
		"{{next_reset_at}}", data.NextResetAt,
		"{{next_reset_account}}", data.NextResetAccount,
		"{{account_summary}}", data.AccountSummary,
	)
	return replacer.Replace(message)
}

// RenderOpenAIAccountSelectionErrorMessage is kept for compatibility.
func RenderOpenAIAccountSelectionErrorMessage(message string, data OpenAIAccountSelectionErrorTemplateData) string {
	return RenderAccountSelectionErrorMessage(message, data)
}

type accountSelectionQuotaSummary struct {
	ID          int64
	Name        string
	Platform    string
	Segments    []string
	NextResetAt *time.Time
}

func buildAccountSelectionQuotaSummary(account *Account, now time.Time) accountSelectionQuotaSummary {
	item := accountSelectionQuotaSummary{
		ID:       account.ID,
		Name:     strings.TrimSpace(account.Name),
		Platform: strings.TrimSpace(account.Platform),
	}
	if item.Name == "" {
		item.Name = fmt.Sprintf("account-%d", account.ID)
	}
	if item.Platform == "" {
		item.Platform = "unknown"
	}
	item.Segments = append(item.Segments, "platform "+item.Platform)
	if account.Status != "" {
		item.Segments = append(item.Segments, "status "+account.Status)
	}
	if !account.Schedulable {
		item.Segments = append(item.Segments, "unschedulable")
	}
	if account.ErrorMessage != "" {
		item.Segments = append(item.Segments, "error "+compactSummaryText(account.ErrorMessage))
	}
	if account.RateLimitResetAt != nil && now.Before(*account.RateLimitResetAt) {
		item.Segments = append(item.Segments, "rate_limit_reset "+account.RateLimitResetAt.Format(time.RFC3339))
		item.NextResetAt = earliestFutureTime(now, item.NextResetAt, account.RateLimitResetAt)
	}
	if account.OverloadUntil != nil && now.Before(*account.OverloadUntil) {
		item.Segments = append(item.Segments, "overload_until "+account.OverloadUntil.Format(time.RFC3339))
		item.NextResetAt = earliestFutureTime(now, item.NextResetAt, account.OverloadUntil)
	}
	if account.TempUnschedulableUntil != nil && now.Before(*account.TempUnschedulableUntil) {
		seg := "temp_unschedulable_until " + account.TempUnschedulableUntil.Format(time.RFC3339)
		if account.TempUnschedulableReason != "" {
			seg += " reason " + compactSummaryText(account.TempUnschedulableReason)
		}
		item.Segments = append(item.Segments, seg)
		item.NextResetAt = earliestFutureTime(now, item.NextResetAt, account.TempUnschedulableUntil)
	}
	if account.ExpiresAt != nil {
		item.Segments = append(item.Segments, "expires_at "+account.ExpiresAt.Format(time.RFC3339))
	}
	appendQuotaSummary(account, now, &item)
	appendCodexUsageSummary(account, now, &item)
	return item
}

func appendQuotaSummary(account *Account, now time.Time, item *accountSelectionQuotaSummary) {
	if limit := account.GetQuotaLimit(); limit > 0 {
		item.Segments = append(item.Segments, fmt.Sprintf("quota %.4g/%.4g", account.GetQuotaUsed(), limit))
	}
	if limit := account.GetQuotaDailyLimit(); limit > 0 {
		item.Segments = append(item.Segments, fmt.Sprintf("daily_quota %.4g/%.4g", account.GetQuotaDailyUsed(), limit))
		if resetAt := quotaResetAt(account.Extra, "quota_daily_reset_at", now); resetAt != nil {
			item.Segments = append(item.Segments, "daily_reset "+resetAt.Format(time.RFC3339))
			item.NextResetAt = earliestFutureTime(now, item.NextResetAt, resetAt)
		}
	}
	if limit := account.GetQuotaWeeklyLimit(); limit > 0 {
		item.Segments = append(item.Segments, fmt.Sprintf("weekly_quota %.4g/%.4g", account.GetQuotaWeeklyUsed(), limit))
		if resetAt := quotaResetAt(account.Extra, "quota_weekly_reset_at", now); resetAt != nil {
			item.Segments = append(item.Segments, "weekly_reset "+resetAt.Format(time.RFC3339))
			item.NextResetAt = earliestFutureTime(now, item.NextResetAt, resetAt)
		}
	}
}

func appendCodexUsageSummary(account *Account, now time.Time, item *accountSelectionQuotaSummary) {
	if value, ok := resolveAccountExtraNumber(account.Extra, "codex_5h_used_percent"); ok {
		item.Segments = append(item.Segments, fmt.Sprintf("5h %.0f%%", value))
	}
	if resetAt := parseOpenAIAccountQuotaResetAt(account.Extra, "5h", now); resetAt != nil {
		item.Segments = append(item.Segments, "5h reset "+resetAt.Format(time.RFC3339))
		item.NextResetAt = earliestFutureTime(now, item.NextResetAt, resetAt)
	}
	if value, ok := resolveAccountExtraNumber(account.Extra, "codex_7d_used_percent"); ok {
		item.Segments = append(item.Segments, fmt.Sprintf("7d %.0f%%", value))
	}
	if resetAt := parseOpenAIAccountQuotaResetAt(account.Extra, "7d", now); resetAt != nil {
		item.Segments = append(item.Segments, "7d reset "+resetAt.Format(time.RFC3339))
		item.NextResetAt = earliestFutureTime(now, item.NextResetAt, resetAt)
	}
}

func quotaResetAt(extra map[string]any, key string, now time.Time) *time.Time {
	if len(extra) == 0 {
		return nil
	}
	if raw, ok := extra[key]; ok {
		if t, err := parseTime(fmt.Sprint(raw)); err == nil && now.Before(t) {
			tt := t
			return &tt
		}
	}
	return nil
}

func parseOpenAIAccountQuotaResetAt(extra map[string]any, window string, now time.Time) *time.Time {
	if len(extra) == 0 {
		return nil
	}
	if raw, ok := extra["codex_"+window+"_reset_at"]; ok {
		if t, err := parseTime(fmt.Sprint(raw)); err == nil && now.Before(t) {
			tt := t
			return &tt
		}
	}
	resetAfter := parseExtraInt(extra["codex_"+window+"_reset_after_seconds"])
	if resetAfter <= 0 {
		return nil
	}
	base := now
	if raw, ok := extra["codex_usage_updated_at"]; ok {
		if updatedAt, err := parseTime(fmt.Sprint(raw)); err == nil {
			base = updatedAt
		}
	}
	resetAt := base.Add(time.Duration(resetAfter) * time.Second)
	if !now.Before(resetAt) {
		return nil
	}
	return &resetAt
}

func earliestFutureTime(now time.Time, values ...*time.Time) *time.Time {
	var earliest *time.Time
	for _, value := range values {
		if value == nil || !now.Before(*value) {
			continue
		}
		if earliest == nil || value.Before(*earliest) {
			v := *value
			earliest = &v
		}
	}
	return earliest
}

func joinAccountSelectionQuotaSummaries(items []accountSelectionQuotaSummary) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		segments := []string{item.Name}
		segments = append(segments, item.Segments...)
		parts = append(parts, strings.Join(segments, " "))
	}
	return strings.Join(parts, "; ")
}

func compactSummaryText(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > 120 {
		return value[:117] + "..."
	}
	return value
}
