package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpsSummaryReportEmailVariables(t *testing.T) {
	latencyP50 := 8231
	latencyP99 := 151260
	ttftP50 := 1674
	ttftP99 := 11222
	now := time.Date(2026, time.July, 19, 1, 0, 26, 0, time.UTC)
	report := &opsScheduledReport{
		Name:       "日报",
		ReportType: "daily_summary",
		TimeRange:  24 * time.Hour,
	}
	overview := &OpsDashboardOverview{
		RequestCountTotal:            2374,
		SuccessCount:                 1451,
		ErrorCountSLA:                2,
		BusinessLimitedCount:         921,
		SLA:                          0.9986,
		ErrorRate:                    0.0014,
		UpstreamErrorRate:            0.0028,
		UpstreamErrorCountExcl429529: 4,
		Upstream429Count:             0,
		Upstream529Count:             0,
		TokenConsumed:                121550190,
		Duration:                     OpsPercentiles{P50: &latencyP50, P99: &latencyP99},
		TTFT:                         OpsPercentiles{P50: &ttftP50, P99: &ttftP99},
		QPS:                          OpsRateSummary{Current: 0, Peak: 1.2, Avg: 0},
		TPS:                          OpsRateSummary{Current: 0, Peak: 133421.2, Avg: 1406.8},
	}

	variables := opsSummaryReportEmailVariables(report, now, overview, "en")
	require.Equal(t, "Daily summary", variables["report_name"])
	require.Equal(t, "daily_summary", variables["report_type"])
	require.Equal(t, "2026-07-18T01:00:26Z", variables["report_start_time"])
	require.Equal(t, "2026-07-19T01:00:26Z", variables["report_end_time"])
	require.Equal(t, "2,374", variables["report_total_requests"])
	require.Equal(t, "1,451", variables["report_success_count"])
	require.Equal(t, "2", variables["report_sla_error_count"])
	require.Equal(t, "921", variables["report_business_limited_count"])
	require.Equal(t, "99.86%", variables["report_sla"])
	require.Equal(t, "0.14%", variables["report_error_rate"])
	require.Equal(t, "0.28%", variables["report_upstream_error_rate"])
	require.Equal(t, "4", variables["report_upstream_error_count_excl_429_529"])
	require.Equal(t, "8,231 ms", variables["report_latency_p50"])
	require.Equal(t, "151,260 ms", variables["report_latency_p99"])
	require.Equal(t, "1,674 ms", variables["report_ttft_p50"])
	require.Equal(t, "11,222 ms", variables["report_ttft_p99"])
	require.Equal(t, "121,550,190", variables["report_tokens"])
	require.Equal(t, "1.2", variables["report_qps_peak"])
	require.Equal(t, "133421.2", variables["report_tps_peak"])
	require.Equal(t, "1406.8", variables["report_tps_avg"])
	require.Empty(t, variables["report_html"])
	require.Equal(t, "none", variables["report_detail_display"])

	zhVariables := opsSummaryReportEmailVariables(report, now, overview, "zh-CN")
	require.Equal(t, "日报", zhVariables["report_name"])
	emptyVariables := opsSummaryReportEmailVariables(report, now, nil, "en")
	require.Equal(t, "block", emptyVariables["report_summary_display"])
	require.Equal(t, "none", emptyVariables["report_detail_display"])

	legacyTemplate, err := renderNotificationEmail(
		NotificationEmailEventOpsScheduledReport,
		"Report",
		`<section>{{report_html}}</section>`,
		variables,
		map[string]string{"report_html": `<h2>generated summary</h2>`},
	)
	require.NoError(t, err)
	require.Contains(t, legacyTemplate.HTML, `<h2>generated summary</h2>`)
}

func TestOpsScheduledReportVariablesDoNotUsePreviewMetrics(t *testing.T) {
	now := time.Date(2026, time.July, 19, 1, 0, 26, 0, time.UTC)
	report := &opsScheduledReport{
		Name:       "错误摘要",
		ReportType: "error_digest",
		TimeRange:  24 * time.Hour,
	}

	variables := opsScheduledReportLocalizedEmailVariables(report, now, "en")
	require.Equal(t, "Error digest", variables["report_name"])
	require.Empty(t, variables["report_html"])
	require.Equal(t, "block", variables["report_detail_display"])
	for _, placeholder := range notificationEmailOpsSummaryPlaceholders {
		if placeholder == "report_summary_display" {
			require.Equal(t, "none", variables[placeholder])
			continue
		}
		require.Equal(t, "-", variables[placeholder])
	}
}

func TestOpsScheduledReportLegacyTemplateReceivesSummaryHTML(t *testing.T) {
	ctx := context.Background()
	repo := newNotificationEmailMemorySettingRepo()
	smtpServer := startNotificationEmailTestSMTPServer(t)
	require.NoError(t, repo.SetMultiple(ctx, smtpServer.settings()))

	emailService := NewEmailService(repo, nil)
	notificationService := NewNotificationEmailService(repo, emailService)
	_, err := notificationService.UpdateTemplate(
		ctx,
		NotificationEmailEventOpsScheduledReport,
		"en",
		"Legacy report {{report_name}}",
		`<section data-template="legacy">{{report_html}}</section>`,
	)
	require.NoError(t, err)

	svc := &OpsScheduledReportService{
		opsService:   &OpsService{opsRepo: &opsRepoMock{}},
		emailService: emailService,
	}
	report := &opsScheduledReport{
		Name:       "日报",
		ReportType: "daily_summary",
		TimeRange:  24 * time.Hour,
		Recipients: []string{"ops@example.com"},
	}

	attempts, err := svc.runReport(ctx, report, time.Date(2026, time.July, 19, 1, 0, 26, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, 1, attempts)
	require.Equal(t, int64(1), smtpServer.messageCount())
	message := smtpServer.lastMessage()
	require.Contains(t, message, `<section data-template="legacy">`)
	require.Contains(t, message, `<h2>日报</h2>`)
}

func TestFormatOpsReportIntegerGroupsDigits(t *testing.T) {
	require.Equal(t, "2,374", formatOpsReportInteger(2374))
	require.Equal(t, "-1,234", formatOpsReportInteger(-1234))
	require.Equal(t, "42", formatOpsReportInteger(42))
}
