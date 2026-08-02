package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// newScheduledReportHarness wires a report service whose ops.scheduled_report
// event is routed to webhook only, with retries disabled so the attempt count
// is a direct assertion on the delivery path.
func newScheduledReportHarness(t *testing.T, webhookURL string, webhookOn bool) *OpsScheduledReportService {
	t.Helper()
	ctx := context.Background()
	settingRepo := newNotificationEmailMemorySettingRepo()
	emailSvc := NewEmailService(settingRepo, nil)
	notificationSvc := NewNotificationEmailService(settingRepo, emailSvc)

	on, off := true, false
	zero := 0
	enabled := webhookOn
	require.NoError(t, notificationSvc.SaveChannelConfig(ctx, NotificationChannelConfig{
		Webhook: NotificationWebhookGlobalConfig{
			Enabled:    enabled,
			Endpoint:   NotificationWebhookEndpoint{URL: webhookURL},
			MaxRetries: &zero,
		},
		Events: map[string]NotificationEventChannelConfig{
			NotificationEmailEventOpsScheduledReport: {Email: &off, Webhook: &on},
		},
	}))

	return &OpsScheduledReportService{
		opsService:   &OpsService{settingRepo: settingRepo},
		emailService: emailSvc,
	}
}

func newScheduledReport(recipients []string) *opsScheduledReport {
	return &opsScheduledReport{
		Name:       "日报",
		ReportType: "daily_summary",
		Schedule:   "0 9 * * *",
		Enabled:    true,
		TimeRange:  24 * time.Hour,
		Recipients: recipients,
		NextRunAt:  time.Now(),
	}
}

// B3 (third fan-out): a failed report webhook must not be restarted once per
// mail recipient. With max_retries=0 one report run means one HTTP request.
func TestScheduledReportFailedWebhookNotRetriedByEmailFanOut(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	svc := newScheduledReportHarness(t, server.URL, true)
	report := newScheduledReport([]string{"a@example.com", "b@example.com", "c@example.com"})
	content := opsScheduledReportContent{html: "<p>report</p>"}

	require.True(t, svc.dispatchScheduledReportWebhook(context.Background(), report, time.Now().UTC(), content))

	// Let the dispatch fail and fully unwind first. This is the exact state the
	// bug needed: the in-flight claim released, no success marker persisted. A
	// fan-out that still reached the webhook channel would start a fresh
	// delivery here, so ordering it explicitly makes the assertion deterministic
	// instead of racing the goroutine.
	require.Eventually(t, func() bool { return atomic.LoadInt32(&attempts) >= 1 }, 5*time.Second, 20*time.Millisecond)
	time.Sleep(300 * time.Millisecond)

	// Drive the production fan-out, not a hand-rolled copy of it: this is what
	// proves the call site marks its sends email-only.
	svc.sendScheduledReportEmails(context.Background(), report, time.Now().UTC(), content, report.Recipients)

	time.Sleep(700 * time.Millisecond)
	require.Equal(t, int32(1), atomic.LoadInt32(&attempts),
		"one report run must not exceed max_retries+1 requests regardless of recipient count")
}

// A report with no mail recipients is a valid webhook-only configuration.
func TestScheduledReportWebhookOnlyWithoutRecipients(t *testing.T) {
	server, received := newWebhookRecorder(t, http.StatusOK)
	svc := newScheduledReportHarness(t, server.URL, true)

	report := newScheduledReport(nil)
	content := opsScheduledReportContent{html: "<p>report</p>"}

	require.True(t, svc.dispatchScheduledReportWebhook(context.Background(), report, time.Now().UTC(), content),
		"webhook-only reports must still dispatch without any mailbox configured")
	awaitWebhook(t, received)
}

func TestScheduledReportWebhookUsesRawAggregateData(t *testing.T) {
	server, received := newWebhookRecorder(t, http.StatusOK)
	svc := newScheduledReportHarness(t, server.URL, true)
	now := time.Date(2026, time.July, 27, 5, 0, 0, 0, time.UTC)
	report := newScheduledReport(nil)
	content := opsScheduledReportContent{
		html: "<p>mail-only report</p>",
		overview: &OpsDashboardOverview{
			StartTime:         now.Add(-24 * time.Hour),
			EndTime:           now,
			RequestCountTotal: 1234,
			SuccessCount:      1200,
			ErrorCountSLA:     34,
			SLA:               0.9724,
		},
	}

	require.True(t, svc.dispatchScheduledReportWebhook(context.Background(), report, now, content))
	var payload struct {
		Data opsScheduledReportWebhookData `json:"data"`
	}
	got := awaitWebhook(t, received)
	require.NoError(t, json.Unmarshal(got.body, &payload))
	require.Equal(t, report.Name, payload.Data.Report.Name)
	require.Equal(t, report.ReportType, payload.Data.Report.Type)
	require.Equal(t, now.Add(-report.TimeRange), payload.Data.Report.StartTime)
	require.Equal(t, now, payload.Data.Report.EndTime)
	require.Equal(t, content.overview, payload.Data.Overview)
	require.Nil(t, payload.Data.ErrorDigest)
	require.Nil(t, payload.Data.AccountAvailability)
	require.NotContains(t, string(got.body), "report_html")
	require.NotContains(t, string(got.body), "report_detail_display")
	require.NotContains(t, string(got.body), "1,234")
	require.NotContains(t, string(got.body), "<p>")
}

func TestScheduledReportWebhookDataPreservesAggregateDTOs(t *testing.T) {
	now := time.Date(2026, time.July, 27, 5, 0, 0, 0, time.UTC)
	report := newScheduledReport(nil)
	errorDigest := &opsErrorDigestAggregate{Total: 2}
	availability := &OpsAccountAvailability{Accounts: map[int64]*AccountAvailability{}}
	data := newOpsScheduledReportWebhookData(report, now, opsScheduledReportContent{
		errorDigest:         errorDigest,
		accountAvailability: availability,
	})

	require.Equal(t, errorDigest, data.ErrorDigest)
	require.Equal(t, availability, data.AccountAvailability)
	require.Nil(t, data.Overview)
}

func TestScheduledReportWebhookSendsOnlyErrorDigestAggregate(t *testing.T) {
	server, received := newWebhookRecorder(t, http.StatusOK)
	svc := newScheduledReportHarness(t, server.URL, true)
	report := newScheduledReport(nil)
	report.ReportType = "error_digest"
	content := opsScheduledReportContent{
		html:        "<p>mail-only report</p>",
		errorDigest: &opsErrorDigestAggregate{Total: 42},
	}

	require.True(t, svc.dispatchScheduledReportWebhook(context.Background(), report, time.Now().UTC(), content))
	got := awaitWebhook(t, received)
	var payload struct {
		Data opsScheduledReportWebhookData `json:"data"`
	}
	require.NoError(t, json.Unmarshal(got.body, &payload))
	var rawPayload struct {
		Data struct {
			ErrorDigest map[string]json.RawMessage `json:"error_digest"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(got.body, &rawPayload))
	require.NotNil(t, payload.Data.ErrorDigest)
	require.Equal(t, 42, payload.Data.ErrorDigest.Total)
	require.Len(t, rawPayload.Data.ErrorDigest, 1)
	require.Contains(t, rawPayload.Data.ErrorDigest, "total")
	require.NotContains(t, string(got.body), "user_email")
	require.NotContains(t, string(got.body), "client_ip")
	require.NotContains(t, string(got.body), "<p>")
}

func TestOpsAccountAvailabilityMarshalsSnakeCase(t *testing.T) {
	collectedAt := time.Date(2026, time.July, 27, 5, 0, 0, 0, time.UTC)
	encoded, err := json.Marshal(&OpsAccountAvailability{
		Accounts:    map[int64]*AccountAvailability{},
		CollectedAt: &collectedAt,
	})
	require.NoError(t, err)
	var decoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Len(t, decoded, 3)
	require.Contains(t, decoded, "group")
	require.Contains(t, decoded, "accounts")
	require.Contains(t, decoded, "collected_at")
}

// With the webhook channel off the report path must behave exactly as before.
func TestScheduledReportWebhookDisabledDispatchesNothing(t *testing.T) {
	server, received := newWebhookRecorder(t, http.StatusOK)
	svc := newScheduledReportHarness(t, server.URL, false)

	report := newScheduledReport([]string{"a@example.com"})
	content := opsScheduledReportContent{html: "<p>report</p>"}

	require.False(t, svc.dispatchScheduledReportWebhook(context.Background(), report, time.Now().UTC(), content))
	requireNoWebhook(t, received)
}
