package xai

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// CLI client identity required by cli-chat-proxy billing endpoints.
	CLITokenAuthHeader     = "x-xai-token-auth"
	CLITokenAuthValue      = "xai-grok-cli"
	CLIClientVersionHeader = "x-grok-client-version"
	// CLIClientVersion is the one place the pinned Grok CLI version lives. The
	// repository and service layers build their own client identity from it, so
	// one bump here covers OAuth traffic and billing probes together.
	// Keep in sync with https://x.ai/cli/stable.
	CLIClientVersion = "0.2.114"
	// billingCLIUserAgent is the legacy pager/shell UA used by billing probes.
	// Distinct from CLIUserAgent() in cli_identity.go (workspace-style UA).
	billingCLIUserAgent = "grok-pager/" + CLIClientVersion + " grok-shell/" + CLIClientVersion + " (macos; aarch64)"

	BillingWeeklyPath  = "/billing?format=credits"
	BillingMonthlyPath = "/billing"

	SuperGrokLimitCents      = 15_000  // $150.00
	SuperGrokHeavyLimitCents = 150_000 // $1,500.00
)

// BillingPeriod describes the current weekly/monthly window.
type BillingPeriod struct {
	Type  string `json:"type,omitempty"`
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
}

// BillingProductUsage is per-product usage inside the weekly credits window.
type BillingProductUsage struct {
	Product      string   `json:"product,omitempty"`
	UsagePercent *float64 `json:"usagePercent,omitempty"`
}

// BillingConfig is the nested config object from /v1/billing responses.
// Weekly (`?format=credits`) and monthly (`/billing`) share this shape; absolute
// money fields typically appear on the credits (prepaid/on-demand) or monthly
// (limit/used) responses.
type BillingConfig struct {
	CurrentPeriod        *BillingPeriod        `json:"currentPeriod,omitempty"`
	CreditUsagePercent   *float64              `json:"creditUsagePercent,omitempty"`
	ProductUsage         []BillingProductUsage `json:"productUsage,omitempty"`
	MonthlyLimit         json.RawMessage       `json:"monthlyLimit,omitempty"`
	Used                 json.RawMessage       `json:"used,omitempty"`
	OnDemandCap          json.RawMessage       `json:"onDemandCap,omitempty"`
	OnDemandUsed         json.RawMessage       `json:"onDemandUsed,omitempty"`
	PrepaidBalance       json.RawMessage       `json:"prepaidBalance,omitempty"`
	IsUnifiedBillingUser bool                  `json:"isUnifiedBillingUser,omitempty"`
	TopUpMethod          string                `json:"topUpMethod,omitempty"`
	BillingPeriodStart   string                `json:"billingPeriodStart,omitempty"`
	BillingPeriodEnd     string                `json:"billingPeriodEnd,omitempty"`
}

// BillingPayload is the top-level body from /v1/billing.
type BillingPayload struct {
	Config *BillingConfig `json:"config,omitempty"`
}

// BillingProductSummary is a normalized product usage row for UI.
type BillingProductSummary struct {
	Product      string   `json:"product"`
	UsagePercent *float64 `json:"usage_percent,omitempty"`
}

// BillingSummary is the merged weekly + monthly billing view.
// Cents fields remain the authoritative monthly numbers; dollar fields are the
// operator-facing absolute money view (prepaid / on-demand / monthly $).
type BillingSummary struct {
	PeriodType         string                  `json:"period_type,omitempty"` // weekly | monthly | unknown
	UsagePercent       *float64                `json:"usage_percent,omitempty"`
	PeriodStart        string                  `json:"period_start,omitempty"`
	PeriodEnd          string                  `json:"period_end,omitempty"`
	ProductUsage       []BillingProductSummary `json:"product_usage,omitempty"`
	MonthlyLimitCents  *float64                `json:"monthly_limit_cents,omitempty"`
	UsedCents          *float64                `json:"used_cents,omitempty"`
	IncludedUsedCents  *float64                `json:"included_used_cents,omitempty"`
	BillingPeriodStart string                  `json:"billing_period_start,omitempty"`
	BillingPeriodEnd   string                  `json:"billing_period_end,omitempty"`
	UsedPercent        *float64                `json:"used_percent,omitempty"`
	// Absolute money (USD). Prepaid/on-demand come from credits probe as dollars.
	// MonthlyLimit/MonthlyUsed are cents/100 for consistent $ display.
	PrepaidBalance       *float64 `json:"prepaid_balance,omitempty"`
	MonthlyLimit         *float64 `json:"monthly_limit,omitempty"`
	MonthlyUsed          *float64 `json:"monthly_used,omitempty"`
	OnDemandCap          *float64 `json:"on_demand_cap,omitempty"`
	OnDemandUsed         *float64 `json:"on_demand_used,omitempty"`
	TopUpMethod          string   `json:"top_up_method,omitempty"`
	IsUnifiedBillingUser bool     `json:"is_unified_billing_user,omitempty"`
	Plan                 string   `json:"plan,omitempty"` // SuperGrok | SuperGrok Heavy | ""
	StatusCode           int      `json:"status_code,omitempty"`
	WeeklyStatusCode     int      `json:"weekly_status_code,omitempty"`
	MonthlyStatusCode    int      `json:"monthly_status_code,omitempty"`
	Source               string   `json:"source,omitempty"`
	FetchedAt            string   `json:"fetched_at,omitempty"`
	UpdatedAt            string   `json:"updated_at,omitempty"`
	WeeklyUpdatedAt      string   `json:"weekly_updated_at,omitempty"`
	MonthlyUpdatedAt     string   `json:"monthly_updated_at,omitempty"`
	Partial              bool     `json:"partial,omitempty"`
	FailedWindows        []string `json:"failed_windows,omitempty"`
}

// BuildBillingURL builds weekly or monthly billing URL against the CLI chat proxy.
func BuildBillingURL(formatCredits bool) string {
	base := strings.TrimRight(DefaultCLIBaseURL, "/")
	if formatCredits {
		return base + BillingWeeklyPath
	}
	return base + BillingMonthlyPath
}

// BuildBillingURLWithValidator builds the weekly or monthly billing URL against
// the caller-resolved base URL, applying the caller's outbound URL trust policy
// first. Accounts forwarding through a custom upstream keep their billing
// probes on the same upstream.
func BuildBillingURLWithValidator(baseURL string, formatCredits bool, validator BaseURLValidator) (string, error) {
	validatedBaseURL, err := validatedBaseURLWithValidator(baseURL, validator)
	if err != nil {
		return "", fmt.Errorf("invalid base url: %w", err)
	}
	if formatCredits {
		return validatedBaseURL + BillingWeeklyPath, nil
	}
	return validatedBaseURL + BillingMonthlyPath, nil
}

// ApplyCLIBillingHeaders sets Authorization + CLI identity headers for billing GETs.
func ApplyCLIBillingHeaders(req *http.Request, accessToken string) {
	if req == nil {
		return
	}
	token := strings.TrimSpace(accessToken)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(CLITokenAuthHeader, CLITokenAuthValue)
	req.Header.Set(CLIClientVersionHeader, CLIClientVersion)
	req.Header.Set("User-Agent", billingCLIUserAgent)
}

// ParseBillingPayload unmarshals a billing API response body.
func ParseBillingPayload(body []byte) (*BillingPayload, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("empty billing body")
	}
	var payload BillingPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

// BuildBillingSummary normalizes a billing config into a UI-friendly summary.
func BuildBillingSummary(config *BillingConfig) *BillingSummary {
	if config == nil {
		return nil
	}
	summary := &BillingSummary{}
	period := config.CurrentPeriod
	periodType := resolvePeriodType(period)
	creditUsage := cloneFloat(config.CreditUsagePercent)

	// Weekly period bounds must not fall back to monthly billing period ends —
	// that would park accounts on a multi-week horizon when weekly UsagePercent
	// is high (scheduler seven_day uses PeriodEnd).
	periodStart := ""
	periodEnd := ""
	if period != nil {
		periodStart = strings.TrimSpace(period.Start)
		periodEnd = strings.TrimSpace(period.End)
	}

	products := make([]BillingProductSummary, 0, len(config.ProductUsage))
	for _, item := range config.ProductUsage {
		product := strings.TrimSpace(item.Product)
		if product == "" {
			continue
		}
		products = append(products, BillingProductSummary{
			Product:      product,
			UsagePercent: cloneFloat(item.UsagePercent),
		})
	}

	monthlyLimit := parseCentValue(config.MonthlyLimit)
	used := parseCentValue(config.Used)
	// Absolute money on credits responses is dollar-denominated ({"val": 12}).
	// Monthly limit/used are cents (same as MonthlyLimitCents / UsedCents).
	prepaid := parseCentValue(config.PrepaidBalance)
	onDemandCap := parseCentValue(config.OnDemandCap)
	onDemandUsed := parseCentValue(config.OnDemandUsed)
	billingStart := strings.TrimSpace(config.BillingPeriodStart)
	billingEnd := strings.TrimSpace(config.BillingPeriodEnd)

	var includedUsed *float64
	if used != nil {
		if monthlyLimit != nil && *monthlyLimit > 0 {
			v := math.Min(*used, *monthlyLimit)
			includedUsed = &v
		} else {
			includedUsed = cloneFloat(used)
		}
	}

	var usedPercent *float64
	if monthlyLimit != nil && *monthlyLimit > 0 && includedUsed != nil {
		v := (*includedUsed / *monthlyLimit) * 100
		usedPercent = &v
	}

	hasWeekly := creditUsage != nil || periodType == "weekly" || len(products) > 0 || prepaid != nil || onDemandCap != nil || onDemandUsed != nil
	hasMonthly := monthlyLimit != nil || used != nil || (!hasWeekly && billingEnd != "")
	if !hasWeekly && !hasMonthly {
		return nil
	}

	if hasWeekly {
		if periodType == "unknown" {
			periodType = "weekly"
		}
		summary.PeriodType = periodType
		summary.UsagePercent = creditUsage
		summary.PeriodStart = periodStart
		summary.PeriodEnd = periodEnd
	} else {
		// Monthly-only: do not put monthly % into UsagePercent (weekly bar field).
		// Frontend weekly bar only renders when PeriodType == weekly.
		summary.PeriodType = "monthly"
		summary.PeriodStart = billingStart
		summary.PeriodEnd = billingEnd
	}
	summary.ProductUsage = products
	summary.MonthlyLimitCents = monthlyLimit
	summary.UsedCents = used
	summary.IncludedUsedCents = includedUsed
	if hasMonthly {
		summary.BillingPeriodStart = billingStart
		summary.BillingPeriodEnd = billingEnd
	}
	summary.UsedPercent = usedPercent
	summary.PrepaidBalance = prepaid
	if onDemandCap != nil {
		summary.OnDemandCap = onDemandCap
	}
	if onDemandUsed != nil {
		summary.OnDemandUsed = onDemandUsed
	}
	// Expose monthly cents as dollars for UI absolute rows.
	if monthlyLimit != nil {
		v := *monthlyLimit / 100
		summary.MonthlyLimit = &v
	}
	if used != nil {
		v := *used / 100
		summary.MonthlyUsed = &v
	}
	summary.TopUpMethod = strings.TrimSpace(config.TopUpMethod)
	summary.IsUnifiedBillingUser = config.IsUnifiedBillingUser
	summary.Plan = resolvePlan(monthlyLimit)
	return summary
}

// MergeBillingProbeResult updates successful billing domains while retaining
// the previous value for any domain that could not be refreshed.
func MergeBillingProbeResult(previous, weekly, monthly *BillingSummary, weeklyOK, monthlyOK bool) *BillingSummary {
	var out BillingSummary
	if previous != nil {
		out = *previous
		previousUpdatedAt := previous.UpdatedAt
		if previousUpdatedAt == "" {
			previousUpdatedAt = previous.FetchedAt
		}
		if out.WeeklyUpdatedAt == "" && (out.UsagePercent != nil || len(out.ProductUsage) > 0) {
			out.WeeklyUpdatedAt = previousUpdatedAt
		}
		if out.MonthlyUpdatedAt == "" && (out.MonthlyLimitCents != nil || out.UsedPercent != nil) {
			out.MonthlyUpdatedAt = previousUpdatedAt
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)

	if weeklyOK && weekly != nil {
		out.PeriodType = weekly.PeriodType
		out.UsagePercent = weekly.UsagePercent
		out.PeriodStart = weekly.PeriodStart
		out.PeriodEnd = weekly.PeriodEnd
		out.ProductUsage = weekly.ProductUsage
		// Absolute prepaid / on-demand usually ride the credits (weekly) response.
		if weekly.PrepaidBalance != nil {
			out.PrepaidBalance = weekly.PrepaidBalance
		}
		if weekly.OnDemandCap != nil {
			out.OnDemandCap = weekly.OnDemandCap
		}
		if weekly.OnDemandUsed != nil {
			out.OnDemandUsed = weekly.OnDemandUsed
		}
		if weekly.TopUpMethod != "" {
			out.TopUpMethod = weekly.TopUpMethod
		}
		if weekly.IsUnifiedBillingUser {
			out.IsUnifiedBillingUser = true
		}
		out.WeeklyUpdatedAt = now
	}
	if monthlyOK && monthly != nil {
		if out.PeriodType == "" {
			out.PeriodType = "monthly"
		}
		out.MonthlyLimitCents = monthly.MonthlyLimitCents
		out.UsedCents = monthly.UsedCents
		out.IncludedUsedCents = monthly.IncludedUsedCents
		out.BillingPeriodStart = monthly.BillingPeriodStart
		out.BillingPeriodEnd = monthly.BillingPeriodEnd
		out.UsedPercent = monthly.UsedPercent
		out.MonthlyLimit = monthly.MonthlyLimit
		out.MonthlyUsed = monthly.MonthlyUsed
		// Monthly probe may also carry on-demand cap when credits omitted it.
		if monthly.OnDemandCap != nil && out.OnDemandCap == nil {
			out.OnDemandCap = monthly.OnDemandCap
		}
		if monthly.OnDemandUsed != nil && out.OnDemandUsed == nil {
			out.OnDemandUsed = monthly.OnDemandUsed
		}
		out.Plan = monthly.Plan
		out.MonthlyUpdatedAt = now
	}

	out.Partial = !weeklyOK || !monthlyOK
	out.FailedWindows = nil
	if !weeklyOK {
		out.FailedWindows = append(out.FailedWindows, "weekly")
	}
	if !monthlyOK {
		out.FailedWindows = append(out.FailedWindows, "monthly")
	}
	if !weeklyOK && !monthlyOK && previous == nil {
		return nil
	}
	return &out
}

// StampBillingSummary sets fetch metadata.
func StampBillingSummary(summary *BillingSummary, statusCode int, source string) *BillingSummary {
	if summary == nil {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	summary.StatusCode = statusCode
	summary.Source = source
	summary.FetchedAt = now
	summary.UpdatedAt = now
	return summary
}

func resolvePeriodType(period *BillingPeriod) string {
	if period == nil {
		return "unknown"
	}
	raw := strings.ToLower(strings.TrimSpace(period.Type))
	if strings.Contains(raw, "weekly") {
		return "weekly"
	}
	if strings.Contains(raw, "monthly") {
		return "monthly"
	}
	return "unknown"
}

func resolvePlan(monthlyLimitCents *float64) string {
	if monthlyLimitCents == nil {
		return ""
	}
	// Allow small float noise.
	limit := math.Round(*monthlyLimitCents)
	switch limit {
	case SuperGrokLimitCents:
		return "SuperGrok"
	case SuperGrokHeavyLimitCents:
		return "SuperGrok Heavy"
	default:
		return ""
	}
}

func parseCentValue(raw json.RawMessage) *float64 {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	// Object form: {"val": 123}
	var obj struct {
		Val any `json:"val"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil && obj.Val != nil {
		return anyToFloat(obj.Val)
	}
	// Bare number / string
	var n any
	if err := json.Unmarshal(raw, &n); err != nil {
		return nil
	}
	return anyToFloat(n)
}

func anyToFloat(v any) *float64 {
	switch n := v.(type) {
	case float64:
		return &n
	case float32:
		f := float64(n)
		return &f
	case int:
		f := float64(n)
		return &f
	case int64:
		f := float64(n)
		return &f
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return nil
		}
		return &f
	case string:
		s := strings.TrimSpace(n)
		if s == "" {
			return nil
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil
		}
		return &f
	default:
		return nil
	}
}

func cloneFloat(v *float64) *float64 {
	if v == nil {
		return nil
	}
	f := *v
	return &f
}
