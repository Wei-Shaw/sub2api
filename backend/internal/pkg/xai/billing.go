package xai

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Grok CLI billing endpoints used by CPAMC (Cli-Proxy-API-Management-Center).
const (
	BillingWeeklyURL  = "https://cli-chat-proxy.grok.com/v1/billing?format=credits"
	BillingMonthlyURL = "https://cli-chat-proxy.grok.com/v1/billing"
	GrokCLIVersion    = "0.2.91"
	GrokCLIUserAgent  = "grok-pager/0.2.91 grok-shell/0.2.91 (macos; aarch64)"
	GrokCLITokenAuth  = "xai-grok-cli"
)

// SuperGrok monthly included credits in cents.
// xAI has used more than one SuperGrok monthly-credit allotment over time ($150 / $200);
// Heavy remains $1500.
const (
	SuperGrokLimitCents      = 15_000  // historical SuperGrok included credits
	SuperGrokLimitCentsAlt   = 20_000  // common SuperGrok included credits observed in console
	SuperGrokHeavyLimitCents = 150_000 // SuperGrok Heavy
)

// ProductUsageSummary is one product row from billing.
type ProductUsageSummary struct {
	Product      string   `json:"product"`
	UsagePercent *float64 `json:"usage_percent,omitempty"`
}

// BillingSummary is the merged weekly + monthly billing view shown in CPAMC.
type BillingSummary struct {
	PeriodType          string                `json:"period_type"` // weekly | monthly | unknown
	UsagePercent        *float64              `json:"usage_percent,omitempty"`
	PeriodStart         string                `json:"period_start,omitempty"`
	PeriodEnd           string                `json:"period_end,omitempty"`
	ProductUsage        []ProductUsageSummary `json:"product_usage,omitempty"`
	MonthlyLimitCents   *int64                `json:"monthly_limit_cents,omitempty"`
	UsedCents           *int64                `json:"used_cents,omitempty"`
	IncludedUsedCents   *int64                `json:"included_used_cents,omitempty"`
	OnDemandCapCents    *int64                `json:"on_demand_cap_cents,omitempty"`
	OnDemandUsedCents   *int64                `json:"on_demand_used_cents,omitempty"`
	OnDemandUsedPercent *float64              `json:"on_demand_used_percent,omitempty"`
	BillingPeriodStart  string                `json:"billing_period_start,omitempty"`
	BillingPeriodEnd    string                `json:"billing_period_end,omitempty"`
	UsedPercent         *float64              `json:"used_percent,omitempty"`
	PlanLabel           string                `json:"plan_label,omitempty"` // supergrok | supergrok_heavy | ""
	UpdatedAt           string                `json:"updated_at,omitempty"`
	Source              string                `json:"source,omitempty"` // probe
}

// HasData reports whether any meaningful billing field is present.
func (s *BillingSummary) HasData() bool {
	if s == nil {
		return false
	}
	if s.UsagePercent != nil || len(s.ProductUsage) > 0 {
		return true
	}
	if s.MonthlyLimitCents != nil || s.UsedCents != nil || s.OnDemandCapCents != nil {
		return true
	}
	if s.PeriodEnd != "" || s.BillingPeriodEnd != "" {
		return true
	}
	return s.PeriodType == "weekly" || s.PeriodType == "monthly"
}

// ResolvePlanLabel maps monthly included limit to SuperGrok plan labels.
// 0 (or missing usable limit after probe) → free/basic Grok; $150/$200 → SuperGrok; $1500 → Heavy.
func ResolvePlanLabel(monthlyLimitCents *int64) string {
	if monthlyLimitCents == nil {
		return ""
	}
	switch *monthlyLimitCents {
	case 0:
		return "grok_free"
	case SuperGrokLimitCents, SuperGrokLimitCentsAlt:
		return "supergrok"
	case SuperGrokHeavyLimitCents:
		return "supergrok_heavy"
	default:
		// Fallback: large included monthly package → treat as SuperGrok family for UI.
		if *monthlyLimitCents > 0 && *monthlyLimitCents < SuperGrokLimitCents {
			// Small free/promo allotments still free-tier style.
			return "grok_free"
		}
		if *monthlyLimitCents >= SuperGrokLimitCents && *monthlyLimitCents < SuperGrokHeavyLimitCents {
			return "supergrok"
		}
		if *monthlyLimitCents >= SuperGrokHeavyLimitCents {
			return "supergrok_heavy"
		}
		return ""
	}
}

// ParseBillingBody parses a raw HTTP body into a BillingSummary (config section).
func ParseBillingBody(body []byte) (*BillingSummary, error) {
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, nil
	}
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("parse billing json: %w", err)
	}
	cfgRaw, ok := root["config"]
	if !ok || cfgRaw == nil {
		// Some responses may be the config itself.
		return BuildBillingSummary(root), nil
	}
	cfgMap, ok := cfgRaw.(map[string]any)
	if !ok {
		return nil, nil
	}
	return BuildBillingSummary(cfgMap), nil
}

// BuildBillingSummary normalizes a billing config object (CPAMC-compatible).
func BuildBillingSummary(config map[string]any) *BillingSummary {
	if config == nil {
		return nil
	}
	summary := &BillingSummary{PeriodType: "unknown", ProductUsage: nil}

	currentPeriod := asMap(firstAny(config, "currentPeriod", "current_period"))
	periodType := resolvePeriodType(asString(currentPeriod["type"]))
	creditUsagePercent := asFloatPtr(firstAny(config, "creditUsagePercent", "credit_usage_percent"))

	periodStart := firstNonEmpty(
		asString(currentPeriod["start"]),
		asString(firstAny(config, "billingPeriodStart", "billing_period_start")),
	)
	periodEnd := firstNonEmpty(
		asString(currentPeriod["end"]),
		asString(firstAny(config, "billingPeriodEnd", "billing_period_end")),
	)
	productUsage := normalizeProductUsage(firstAny(config, "productUsage", "product_usage"))

	monthlyLimitCents := normalizeCentValue(firstAny(config, "monthlyLimit", "monthly_limit"))
	usedCents := normalizeCentValue(config["used"])
	onDemandCapCents := normalizeCentValue(firstAny(config, "onDemandCap", "on_demand_cap"))
	explicitOnDemandUsed := normalizeCentValue(firstAny(config, "onDemandUsed", "on_demand_used"))
	billingPeriodStart := asString(firstAny(config, "billingPeriodStart", "billing_period_start"))
	billingPeriodEnd := asString(firstAny(config, "billingPeriodEnd", "billing_period_end"))

	var includedUsedCents *int64
	if usedCents != nil {
		if monthlyLimitCents != nil && *monthlyLimitCents > 0 {
			v := *usedCents
			if v > *monthlyLimitCents {
				v = *monthlyLimitCents
			}
			includedUsedCents = &v
		} else {
			includedUsedCents = usedCents
		}
	}

	var onDemandUsedCents *int64
	if explicitOnDemandUsed != nil {
		onDemandUsedCents = explicitOnDemandUsed
	} else if usedCents != nil && monthlyLimitCents != nil {
		v := *usedCents - *monthlyLimitCents
		if v < 0 {
			v = 0
		}
		onDemandUsedCents = &v
	}

	var usedPercent *float64
	if monthlyLimitCents != nil && *monthlyLimitCents > 0 && includedUsedCents != nil {
		p := float64(*includedUsedCents) / float64(*monthlyLimitCents) * 100
		usedPercent = &p
	}
	var onDemandUsedPercent *float64
	if onDemandCapCents != nil && *onDemandCapCents > 0 && onDemandUsedCents != nil {
		p := float64(*onDemandUsedCents) / float64(*onDemandCapCents) * 100
		onDemandUsedPercent = &p
	}

	hasWeekly := creditUsagePercent != nil || periodType == "weekly" || len(productUsage) > 0
	hasMonthly := monthlyLimitCents != nil || usedCents != nil ||
		(!hasWeekly && (onDemandCapCents != nil || billingPeriodEnd != ""))

	if !hasWeekly && !hasMonthly {
		return nil
	}

	if hasWeekly {
		if periodType == "unknown" {
			periodType = "weekly"
		}
		summary.PeriodType = periodType
		summary.UsagePercent = creditUsagePercent
		summary.PeriodStart = periodStart
		summary.PeriodEnd = periodEnd
	} else {
		summary.PeriodType = "monthly"
		summary.UsagePercent = usedPercent
		summary.PeriodStart = billingPeriodStart
		summary.PeriodEnd = billingPeriodEnd
	}
	summary.ProductUsage = productUsage
	summary.MonthlyLimitCents = monthlyLimitCents
	summary.UsedCents = usedCents
	summary.IncludedUsedCents = includedUsedCents
	summary.OnDemandCapCents = onDemandCapCents
	summary.OnDemandUsedCents = onDemandUsedCents
	summary.OnDemandUsedPercent = onDemandUsedPercent
	if hasMonthly {
		summary.BillingPeriodStart = billingPeriodStart
		summary.BillingPeriodEnd = billingPeriodEnd
	}
	summary.UsedPercent = usedPercent
	summary.PlanLabel = ResolvePlanLabel(monthlyLimitCents)
	summary.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return summary
}

// MergeBillingSummaries prefers primary (weekly) fields, fills gaps from fallback (monthly).
func MergeBillingSummaries(primary, fallback *BillingSummary) *BillingSummary {
	if primary == nil {
		return fallback
	}
	if fallback == nil {
		return primary
	}
	out := *primary
	if out.PeriodType == "unknown" && fallback.PeriodType != "unknown" {
		out.PeriodType = fallback.PeriodType
	}
	if out.UsagePercent == nil {
		out.UsagePercent = fallback.UsagePercent
	}
	if out.PeriodStart == "" {
		out.PeriodStart = fallback.PeriodStart
	}
	if out.PeriodEnd == "" {
		out.PeriodEnd = fallback.PeriodEnd
	}
	if len(out.ProductUsage) == 0 {
		out.ProductUsage = fallback.ProductUsage
	}
	if out.MonthlyLimitCents == nil {
		out.MonthlyLimitCents = fallback.MonthlyLimitCents
	}
	if out.UsedCents == nil {
		out.UsedCents = fallback.UsedCents
	}
	if out.IncludedUsedCents == nil {
		out.IncludedUsedCents = fallback.IncludedUsedCents
	}
	if out.OnDemandCapCents == nil {
		out.OnDemandCapCents = fallback.OnDemandCapCents
	}
	if out.OnDemandUsedCents == nil {
		out.OnDemandUsedCents = fallback.OnDemandUsedCents
	}
	if out.OnDemandUsedPercent == nil {
		out.OnDemandUsedPercent = fallback.OnDemandUsedPercent
	}
	if out.BillingPeriodStart == "" {
		out.BillingPeriodStart = fallback.BillingPeriodStart
	}
	if out.BillingPeriodEnd == "" {
		out.BillingPeriodEnd = fallback.BillingPeriodEnd
	}
	if out.UsedPercent == nil {
		out.UsedPercent = fallback.UsedPercent
	}
	if out.PlanLabel == "" {
		out.PlanLabel = fallback.PlanLabel
	}
	if out.PlanLabel == "" {
		out.PlanLabel = ResolvePlanLabel(out.MonthlyLimitCents)
	}
	out.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return &out
}

// SubjectFromIDToken extracts sub (or email) for x-userid header.
func SubjectFromIDToken(idToken string) string {
	claims := claimsFromIDToken(idToken)
	if claims == nil {
		return ""
	}
	for _, key := range []string{"sub", "user_id", "userId", "id"} {
		if v, ok := claims[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func claimsFromIDToken(idToken string) map[string]any {
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return nil
		}
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil
	}
	return claims
}

// --- helpers ---

func asMap(v any) map[string]any {
	if v == nil {
		return map[string]any{}
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		if math.Trunc(t) == t {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return strings.TrimSpace(t.String())
	default:
		return ""
	}
}

func asFloatPtr(v any) *float64 {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case float64:
		return &t
	case float32:
		f := float64(t)
		return &f
	case int:
		f := float64(t)
		return &f
	case int64:
		f := float64(t)
		return &f
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return nil
		}
		return &f
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return nil
		}
		s = strings.TrimSuffix(s, "%")
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil
		}
		return &f
	default:
		return nil
	}
}

func normalizeCentValue(v any) *int64 {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return normalizeCentValue(m["val"])
	}
	switch t := v.(type) {
	case float64:
		i := int64(math.Round(t))
		return &i
	case float32:
		i := int64(math.Round(float64(t)))
		return &i
	case int:
		i := int64(t)
		return &i
	case int64:
		return &t
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return nil
		}
		i := int64(math.Round(f))
		return &i
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return nil
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil
		}
		i := int64(math.Round(f))
		return &i
	default:
		return nil
	}
}

func normalizeProductUsage(raw any) []ProductUsageSummary {
	arr, ok := raw.([]any)
	if !ok || len(arr) == 0 {
		return nil
	}
	out := make([]ProductUsageSummary, 0, len(arr))
	for i, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		product := asString(m["product"])
		if product == "" {
			product = fmt.Sprintf("Product %d", i+1)
		}
		pct := asFloatPtr(firstAny(m, "usagePercent", "usage_percent"))
		out = append(out, ProductUsageSummary{Product: product, UsagePercent: pct})
	}
	return out
}

func resolvePeriodType(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if strings.Contains(s, "weekly") {
		return "weekly"
	}
	if strings.Contains(s, "monthly") {
		return "monthly"
	}
	return "unknown"
}

func firstAny(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return v
		}
	}
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
