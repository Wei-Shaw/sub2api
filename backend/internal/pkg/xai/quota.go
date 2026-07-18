package xai

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

type QuotaWindow struct {
	Limit     *int64 `json:"limit,omitempty"`
	Remaining *int64 `json:"remaining,omitempty"`
	ResetUnix *int64 `json:"reset_unix,omitempty"`
	ResetAt   string `json:"reset_at,omitempty"`
}

type CreditBalance struct {
	CreditType string   `json:"credit_type,omitempty"`
	Label      string   `json:"label,omitempty"`
	Amount     *float64 `json:"amount,omitempty"`
	Limit      *float64 `json:"limit,omitempty"`
	Used       *float64 `json:"used,omitempty"`
	Remaining  *float64 `json:"remaining,omitempty"`
	Currency   string   `json:"currency,omitempty"`
	ResetAt    string   `json:"reset_at,omitempty"`
	Source     string   `json:"source,omitempty"`
}

type QuotaSnapshot struct {
	Requests          *QuotaWindow      `json:"requests,omitempty"`
	Tokens            *QuotaWindow      `json:"tokens,omitempty"`
	Credits           []CreditBalance   `json:"credits,omitempty"`
	RetryAfterSeconds *int              `json:"retry_after_seconds,omitempty"`
	SubscriptionTier  string            `json:"subscription_tier,omitempty"`
	EntitlementStatus string            `json:"entitlement_status,omitempty"`
	StatusCode        int               `json:"status_code,omitempty"`
	Headers           map[string]string `json:"headers,omitempty"`
	HeadersObserved   bool              `json:"headers_observed"`
	ObservationSource string            `json:"observation_source,omitempty"`
	LastProbeAt       string            `json:"last_probe_at,omitempty"`
	LastHeadersSeenAt string            `json:"last_headers_seen_at,omitempty"`
	UpdatedAt         string            `json:"updated_at"`
}

func (s *QuotaSnapshot) HasObservedHeaders() bool {
	if s == nil {
		return false
	}
	return s.HeadersObserved ||
		s.Requests != nil ||
		s.Tokens != nil ||
		len(s.Credits) > 0 ||
		s.RetryAfterSeconds != nil ||
		s.SubscriptionTier != "" ||
		s.EntitlementStatus != "" ||
		len(s.Headers) > 0
}

var quotaHeaderAllowlist = []string{
	"x-ratelimit-limit-requests",
	"x-ratelimit-remaining-requests",
	"x-ratelimit-reset-requests",
	"x-ratelimit-limit-tokens",
	"x-ratelimit-remaining-tokens",
	"x-ratelimit-reset-tokens",
	"retry-after",
	"x-subscription-tier",
	"xai-subscription-tier",
	"x-entitlement-status",
	"xai-entitlement-status",
	"xai-monthly-credits",
	"xai-monthly-credit-balance",
	"xai-monthly-credits-remaining",
	"xai-monthly-credits-limit",
	"xai-monthly-credits-used",
	"xai-payg-credits",
	"xai-payg-credit-balance",
	"xai-payg-credits-remaining",
	"xai-payg-credits-limit",
	"xai-payg-credits-used",
	"xai-pay-as-you-go-credits",
	"xai-pay-as-you-go-credits-remaining",
	"xai-pay-as-you-go-credits-limit",
	"xai-pay-as-you-go-credits-used",
	"xai-prepaid-credits",
	"xai-prepaid-credit-balance",
	"xai-extra-usage-credits",
	"xai-extra-usage-credits-remaining",
}

func ParseQuotaHeaders(headers http.Header, statusCode int) *QuotaSnapshot {
	return parseQuotaHeaders(headers, statusCode, "", false)
}

func ObserveQuotaHeaders(headers http.Header, statusCode int, source string) *QuotaSnapshot {
	return parseQuotaHeaders(headers, statusCode, source, true)
}

func parseQuotaHeaders(headers http.Header, statusCode int, source string, keepEmpty bool) *QuotaSnapshot {
	if headers == nil && !keepEmpty {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	snapshot := &QuotaSnapshot{
		Requests:          parseQuotaWindow(headers, "requests"),
		Tokens:            parseQuotaWindow(headers, "tokens"),
		Credits:           parseCreditHeaders(headers),
		StatusCode:        statusCode,
		Headers:           make(map[string]string),
		ObservationSource: strings.TrimSpace(source),
		UpdatedAt:         now,
	}
	if snapshot.ObservationSource == "active_probe" {
		snapshot.LastProbeAt = now
	}
	if retryAfter := parseRetryAfter(headers.Get("retry-after")); retryAfter != nil {
		snapshot.RetryAfterSeconds = retryAfter
	}
	snapshot.SubscriptionTier = firstHeader(headers, "xai-subscription-tier", "x-subscription-tier")
	snapshot.EntitlementStatus = firstHeader(headers, "xai-entitlement-status", "x-entitlement-status")

	for _, name := range quotaHeaderAllowlist {
		if value := strings.TrimSpace(headers.Get(name)); value != "" {
			snapshot.Headers[name] = value
		}
	}

	if snapshot.Requests == nil &&
		snapshot.Tokens == nil &&
		len(snapshot.Credits) == 0 &&
		snapshot.RetryAfterSeconds == nil &&
		snapshot.SubscriptionTier == "" &&
		snapshot.EntitlementStatus == "" &&
		len(snapshot.Headers) == 0 {
		if keepEmpty {
			return snapshot
		}
		return nil
	}
	snapshot.HeadersObserved = true
	snapshot.LastHeadersSeenAt = now
	return snapshot
}

func parseQuotaWindow(headers http.Header, dimension string) *QuotaWindow {
	window := &QuotaWindow{
		Limit:     parseInt64Ptr(headers.Get("x-ratelimit-limit-" + dimension)),
		Remaining: parseInt64Ptr(headers.Get("x-ratelimit-remaining-" + dimension)),
	}
	if reset := parseResetHeader(headers.Get("x-ratelimit-reset-" + dimension)); reset != nil {
		window.ResetUnix = reset
		window.ResetAt = time.Unix(*reset, 0).UTC().Format(time.RFC3339)
	}
	if window.Limit == nil && window.Remaining == nil && window.ResetUnix == nil {
		return nil
	}
	return window
}

type creditHeaderAliases struct {
	creditType string
	label      string
	amount     []string
	remaining  []string
	limit      []string
	used       []string
	reset      []string
}

var creditHeaderAliasSets = []creditHeaderAliases{
	{
		creditType: "monthly_credits",
		label:      "Monthly credits",
		amount:     []string{"xai-monthly-credits", "xai-monthly-credit-balance"},
		remaining:  []string{"xai-monthly-credits-remaining", "xai-monthly-credit-remaining"},
		limit:      []string{"xai-monthly-credits-limit", "xai-monthly-credit-limit"},
		used:       []string{"xai-monthly-credits-used", "xai-monthly-credit-used"},
		reset:      []string{"xai-monthly-credits-reset", "xai-monthly-credit-reset"},
	},
	{
		creditType: "pay_as_you_go",
		label:      "Pay-as-you-go",
		amount:     []string{"xai-payg-credits", "xai-payg-credit-balance", "xai-pay-as-you-go-credits", "xai-pay-as-you-go-credit-balance"},
		remaining:  []string{"xai-payg-credits-remaining", "xai-payg-credit-remaining", "xai-pay-as-you-go-credits-remaining", "xai-pay-as-you-go-credit-remaining"},
		limit:      []string{"xai-payg-credits-limit", "xai-payg-credit-limit", "xai-pay-as-you-go-credits-limit", "xai-pay-as-you-go-credit-limit"},
		used:       []string{"xai-payg-credits-used", "xai-payg-credit-used", "xai-pay-as-you-go-credits-used", "xai-pay-as-you-go-credit-used"},
		reset:      []string{"xai-payg-credits-reset", "xai-payg-credit-reset", "xai-pay-as-you-go-credits-reset", "xai-pay-as-you-go-credit-reset"},
	},
	{
		creditType: "prepaid_credits",
		label:      "Prepaid credits",
		amount:     []string{"xai-prepaid-credits", "xai-prepaid-credit-balance"},
		remaining:  []string{"xai-prepaid-credits-remaining", "xai-prepaid-credit-remaining"},
		limit:      []string{"xai-prepaid-credits-limit", "xai-prepaid-credit-limit"},
		used:       []string{"xai-prepaid-credits-used", "xai-prepaid-credit-used"},
		reset:      []string{"xai-prepaid-credits-reset", "xai-prepaid-credit-reset"},
	},
	{
		creditType: "extra_usage_credits",
		label:      "Extra usage credits",
		amount:     []string{"xai-extra-usage-credits", "xai-extra-usage-credit-balance"},
		remaining:  []string{"xai-extra-usage-credits-remaining", "xai-extra-usage-credit-remaining"},
		limit:      []string{"xai-extra-usage-credits-limit", "xai-extra-usage-credit-limit"},
		used:       []string{"xai-extra-usage-credits-used", "xai-extra-usage-credit-used"},
		reset:      []string{"xai-extra-usage-credits-reset", "xai-extra-usage-credit-reset"},
	},
}

func parseCreditHeaders(headers http.Header) []CreditBalance {
	if headers == nil {
		return nil
	}
	credits := make([]CreditBalance, 0, 4)
	for _, aliases := range creditHeaderAliasSets {
		if credit := parseNamedCreditHeaders(headers, aliases); credit != nil {
			credits = append(credits, *credit)
		}
	}
	return credits
}

func parseNamedCreditHeaders(headers http.Header, aliases creditHeaderAliases) *CreditBalance {
	credit := CreditBalance{
		CreditType: aliases.creditType,
		Label:      aliases.label,
		Currency:   "USD",
		Source:     "headers",
	}
	if value := firstNumericHeader(headers, aliases.amount...); value != nil {
		credit.Amount = value
	}
	if value := firstNumericHeader(headers, aliases.remaining...); value != nil {
		credit.Remaining = value
	}
	if value := firstNumericHeader(headers, aliases.limit...); value != nil {
		credit.Limit = value
	}
	if value := firstNumericHeader(headers, aliases.used...); value != nil {
		credit.Used = value
	}
	for _, name := range aliases.reset {
		if value := strings.TrimSpace(headers.Get(name)); value != "" {
			if reset := parseResetHeader(value); reset != nil {
				credit.ResetAt = time.Unix(*reset, 0).UTC().Format(time.RFC3339)
				break
			}
		}
	}
	if credit.Amount == nil && credit.Limit == nil && credit.Used == nil && credit.Remaining == nil {
		return nil
	}
	return &credit
}

func firstNumericHeader(headers http.Header, names ...string) *float64 {
	for _, name := range names {
		if value := parseFloat64Ptr(headers.Get(name)); value != nil {
			return value
		}
	}
	return nil
}

func parseResetHeader(raw string) *int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if value, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if value > 1_000_000_000_000 {
			value = value / 1000
		}
		return &value
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		value := t.Unix()
		return &value
	}
	return nil
}

func parseRetryAfter(raw string) *int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if value, err := strconv.Atoi(raw); err == nil {
		return &value
	}
	if t, err := http.ParseTime(raw); err == nil {
		seconds := int(time.Until(t).Seconds())
		if seconds < 0 {
			seconds = 0
		}
		return &seconds
	}
	return nil
}

func parseInt64Ptr(raw string) *int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil
	}
	return &value
}

func parseFloat64Ptr(raw string) *float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	raw = strings.TrimPrefix(raw, "$")
	raw = strings.ReplaceAll(raw, ",", "")
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil
	}
	return &value
}

func firstHeader(headers http.Header, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(headers.Get(name)); value != "" {
			return value
		}
	}
	return ""
}
