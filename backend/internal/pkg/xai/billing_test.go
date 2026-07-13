package xai

import (
	"encoding/base64"
	"testing"
)

func TestBuildBillingSummaryWeekly(t *testing.T) {
	raw := `{
		"config": {
			"currentPeriod": {"type": "weekly", "start": "2026-07-01T00:00:00Z", "end": "2026-07-08T00:00:00Z"},
			"creditUsagePercent": 42.5,
			"productUsage": [{"product": "Grok", "usagePercent": 40}]
		}
	}`
	summary, err := ParseBillingBody([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if summary == nil || !summary.HasData() {
		t.Fatal("expected summary")
	}
	if summary.PeriodType != "weekly" {
		t.Fatalf("period=%s", summary.PeriodType)
	}
	if summary.UsagePercent == nil || *summary.UsagePercent != 42.5 {
		t.Fatalf("usage=%v", summary.UsagePercent)
	}
	if summary.PeriodEnd != "2026-07-08T00:00:00Z" {
		t.Fatalf("end=%s", summary.PeriodEnd)
	}
	if len(summary.ProductUsage) != 1 || summary.ProductUsage[0].Product != "Grok" {
		t.Fatalf("products=%+v", summary.ProductUsage)
	}
}

func TestBuildBillingSummaryMonthly(t *testing.T) {
	raw := `{
		"config": {
			"monthlyLimit": {"val": 15000},
			"used": {"val": 3000},
			"onDemandCap": {"val": 5000},
			"billingPeriodEnd": "2026-07-31T00:00:00Z"
		}
	}`
	summary, err := ParseBillingBody([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if summary == nil {
		t.Fatal("nil")
	}
	if summary.PlanLabel != "supergrok" {
		t.Fatalf("plan=%s", summary.PlanLabel)
	}
	if summary.MonthlyLimitCents == nil || *summary.MonthlyLimitCents != 15000 {
		t.Fatalf("limit=%v", summary.MonthlyLimitCents)
	}
	if summary.UsedPercent == nil || *summary.UsedPercent != 20 {
		t.Fatalf("usedPercent=%v", summary.UsedPercent)
	}
	if summary.IncludedUsedCents == nil || *summary.IncludedUsedCents != 3000 {
		t.Fatalf("included=%v", summary.IncludedUsedCents)
	}
}

func TestResolvePlanLabelAltSuperGrok200(t *testing.T) {
	// Live accounts often report $200 included monthly credits for SuperGrok.
	cents := int64(20_000)
	if got := ResolvePlanLabel(&cents); got != "supergrok" {
		t.Fatalf("got %q, want supergrok", got)
	}
	raw := `{"config":{"monthlyLimit":{"val":20000},"used":{"val":885},"billingPeriodEnd":"2026-08-01T00:00:00Z"}}`
	summary, err := ParseBillingBody([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if summary == nil || summary.PlanLabel != "supergrok" {
		t.Fatalf("summary plan=%v", summary)
	}
}

func TestResolvePlanLabelFreeGrok(t *testing.T) {
	cents := int64(0)
	if got := ResolvePlanLabel(&cents); got != "grok_free" {
		t.Fatalf("got %q, want grok_free", got)
	}
	raw := `{"config":{"monthlyLimit":{"val":0},"used":{"val":0},"currentPeriod":{"type":"weekly","end":"2026-07-15T00:00:00Z"}}}`
	summary, err := ParseBillingBody([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if summary == nil || summary.PlanLabel != "grok_free" {
		t.Fatalf("summary plan=%v", summary)
	}
}

func TestMergeBillingSummaries(t *testing.T) {
	weekly, _ := ParseBillingBody([]byte(`{"config":{"currentPeriod":{"type":"weekly","end":"2026-07-08T00:00:00Z"},"creditUsagePercent":10}}`))
	monthly, _ := ParseBillingBody([]byte(`{"config":{"monthlyLimit":15000,"used":1500,"billingPeriodEnd":"2026-07-31T00:00:00Z"}}`))
	merged := MergeBillingSummaries(weekly, monthly)
	if merged.PeriodType != "weekly" {
		t.Fatalf("period=%s", merged.PeriodType)
	}
	if merged.UsagePercent == nil || *merged.UsagePercent != 10 {
		t.Fatalf("weekly usage missing")
	}
	if merged.MonthlyLimitCents == nil || *merged.MonthlyLimitCents != 15000 {
		t.Fatalf("monthly missing")
	}
	if merged.PlanLabel != "supergrok" {
		t.Fatalf("plan=%s", merged.PlanLabel)
	}
}

func TestSubjectFromIDToken(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"user-1","email":"a@b.com"}`))
	token := "eyJhbGciOiJub25lIn0." + payload + ".x"
	if got := SubjectFromIDToken(token); got != "user-1" {
		t.Fatalf("got %q", got)
	}
}
