package xai

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildBillingURL(t *testing.T) {
	t.Parallel()
	require.Equal(t, "https://cli-chat-proxy.grok.com/v1/billing?format=credits", BuildBillingURL(true))
	require.Equal(t, "https://cli-chat-proxy.grok.com/v1/billing", BuildBillingURL(false))
}

func TestApplyCLIBillingHeaders(t *testing.T) {
	t.Parallel()
	req, err := http.NewRequest(http.MethodGet, BuildBillingURL(true), nil)
	require.NoError(t, err)

	ApplyCLIBillingHeaders(req, " token ")

	require.Equal(t, "Bearer token", req.Header.Get("Authorization"))
	require.Equal(t, CLITokenAuthValue, req.Header.Get(CLITokenAuthHeader))
	require.Equal(t, CLIClientVersion, req.Header.Get(CLIClientVersionHeader))
	require.Equal(t, "grok-pager/"+CLIClientVersion+" grok-shell/"+CLIClientVersion+" (macos; aarch64)", req.UserAgent())
}

func TestBuildBillingSummaryWeeklyAndMonthly(t *testing.T) {
	t.Parallel()

	weeklyBody := []byte(`{
		"config": {
			"currentPeriod": {"type":"WEEKLY","start":"2026-07-09T03:25:00Z","end":"2026-07-16T03:25:00Z"},
			"creditUsagePercent": 2.0,
			"productUsage": [{"product":"Api","usagePercent":2.0}]
		}
	}`)
	monthlyBody := []byte(`{
		"config": {
			"monthlyLimit": {"val": 15000},
			"used": {"val": 78},
			"billingPeriodStart": "2026-07-01T00:00:00Z",
			"billingPeriodEnd": "2026-08-01T00:00:00Z"
		}
	}`)

	weeklyPayload, err := ParseBillingPayload(weeklyBody)
	require.NoError(t, err)
	monthlyPayload, err := ParseBillingPayload(monthlyBody)
	require.NoError(t, err)

	weekly := BuildBillingSummary(weeklyPayload.Config)
	monthly := BuildBillingSummary(monthlyPayload.Config)
	require.NotNil(t, weekly)
	require.NotNil(t, monthly)
	require.Equal(t, "weekly", weekly.PeriodType)
	require.InDelta(t, 2.0, *weekly.UsagePercent, 1e-9)
	require.Equal(t, "Api", weekly.ProductUsage[0].Product)
	require.Equal(t, "SuperGrok", monthly.Plan)
	require.InDelta(t, 15000, *monthly.MonthlyLimitCents, 1e-9)
	require.InDelta(t, 78, *monthly.UsedCents, 1e-9)
	require.InDelta(t, 0.52, *monthly.UsedPercent, 1e-2)

	merged := MergeBillingProbeResult(nil, weekly, monthly, true, true)
	require.Equal(t, "weekly", merged.PeriodType)
	require.InDelta(t, 2.0, *merged.UsagePercent, 1e-9)
	require.Equal(t, "SuperGrok", merged.Plan)
	require.InDelta(t, 15000, *merged.MonthlyLimitCents, 1e-9)
	require.Equal(t, "2026-08-01T00:00:00Z", merged.BillingPeriodEnd)
}

func TestParseCentValueBareNumber(t *testing.T) {
	t.Parallel()
	raw, _ := json.Marshal(15000)
	v := parseCentValue(raw)
	require.NotNil(t, v)
	require.InDelta(t, 15000, *v, 1e-9)
}

func TestBuildBillingSummaryMonthlyOnlyKeepsWeeklyUsageEmpty(t *testing.T) {
	t.Parallel()
	payload, err := ParseBillingPayload([]byte(`{"config":{"monthlyLimit":{"val":15000},"used":{"val":7500},"billingPeriodStart":"2026-07-01T00:00:00Z","billingPeriodEnd":"2026-08-01T00:00:00Z"}}`))
	require.NoError(t, err)

	summary := BuildBillingSummary(payload.Config)
	require.NotNil(t, summary)
	require.Equal(t, "monthly", summary.PeriodType)
	require.Nil(t, summary.UsagePercent)
	require.InDelta(t, 50, *summary.UsedPercent, 1e-9)
}

func TestMergeBillingProbeResultRetainsFailedWindow(t *testing.T) {
	t.Parallel()
	previous := &BillingSummary{
		PeriodType:        "weekly",
		UsagePercent:      floatPointer(100),
		PeriodEnd:         "2026-07-16T00:00:00Z",
		MonthlyLimitCents: floatPointer(15000),
		UsedPercent:       floatPointer(20),
		BillingPeriodEnd:  "2026-08-01T00:00:00Z",
		WeeklyUpdatedAt:   "2026-07-10T00:00:00Z",
		MonthlyUpdatedAt:  "2026-07-10T00:00:00Z",
		FailedWindows:     []string{"monthly"},
	}
	monthly := &BillingSummary{
		PeriodType:        "monthly",
		MonthlyLimitCents: floatPointer(15000),
		UsedPercent:       floatPointer(30),
		BillingPeriodEnd:  "2026-08-01T00:00:00Z",
	}

	merged := MergeBillingProbeResult(previous, nil, monthly, false, true)
	require.Equal(t, "weekly", merged.PeriodType)
	require.InDelta(t, 100, *merged.UsagePercent, 1e-9)
	require.Equal(t, previous.WeeklyUpdatedAt, merged.WeeklyUpdatedAt)
	require.InDelta(t, 30, *merged.UsedPercent, 1e-9)
	require.NotEqual(t, previous.MonthlyUpdatedAt, merged.MonthlyUpdatedAt)
	require.True(t, merged.Partial)
	require.Equal(t, []string{"weekly"}, merged.FailedWindows)
	require.Equal(t, []string{"monthly"}, previous.FailedWindows)
}

func floatPointer(value float64) *float64 {
	return &value
}
