package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type opsAlertNotifyStubRepo struct {
	OpsRepository

	mu     sync.Mutex
	marked []int64
}

func (r *opsAlertNotifyStubRepo) UpdateAlertEventEmailSent(_ context.Context, eventID int64, _ bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.marked = append(r.marked, eventID)
	return nil
}

func (r *opsAlertNotifyStubRepo) markedCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.marked)
}

// newOpsAlertNotifyHarness wires an evaluator with legacy email settings while
// the channel matrix routes ops.alert to webhook only.
func newOpsAlertNotifyHarness(t *testing.T, webhookURL string, rateLimitPerHour int) (*OpsAlertEvaluatorService, *opsAlertNotifyStubRepo) {
	t.Helper()
	ctx := context.Background()
	settingRepo := newNotificationEmailMemorySettingRepo()

	emailCfg := OpsEmailNotificationConfig{
		Alert: OpsEmailAlertConfig{
			Enabled:          true,
			Recipients:       []string{"ops@example.com"},
			RateLimitPerHour: rateLimitPerHour,
		},
	}
	encoded, err := json.Marshal(emailCfg)
	require.NoError(t, err)
	require.NoError(t, settingRepo.Set(ctx, SettingKeyOpsEmailNotificationConfig, string(encoded)))

	emailSvc := NewEmailService(settingRepo, nil)
	notificationSvc := NewNotificationEmailService(settingRepo, emailSvc)

	on, off := true, false
	require.NoError(t, notificationSvc.SaveChannelConfig(ctx, NotificationChannelConfig{
		Webhook: NotificationWebhookGlobalConfig{
			Enabled:  true,
			Endpoint: NotificationWebhookEndpoint{URL: webhookURL},
		},
		Events: map[string]NotificationEventChannelConfig{
			NotificationEmailEventOpsAlert: {Email: &off, Webhook: &on},
		},
	}))

	repo := &opsAlertNotifyStubRepo{}
	svc := &OpsAlertEvaluatorService{
		opsService:   &OpsService{settingRepo: settingRepo},
		opsRepo:      repo,
		emailService: emailSvc,
		emailLimiter: newSlidingWindowLimiter(0, time.Hour),
	}
	return svc, repo
}

// Webhook-only delivery must not mutate the historical email_sent marker.
func TestOpsAlertWebhookUsesRawSourceDataWithoutMarkingEmailSent(t *testing.T) {
	server, received := newWebhookRecorder(t, http.StatusOK)
	svc, repo := newOpsAlertNotifyHarness(t, server.URL, 1)

	metricValue := 6.91
	threshold := 5.0
	firedAt := time.Date(2026, time.July, 27, 4, 35, 48, 0, time.UTC)
	rule := &OpsAlertRule{
		ID:            1,
		Name:          "high error rate",
		Description:   "error rate too high",
		Severity:      "P1",
		MetricType:    "error_rate",
		Operator:      ">",
		Threshold:     threshold,
		WindowMinutes: 5,
		NotifyEmail:   false,
	}
	event := &OpsAlertEvent{
		ID:             101,
		RuleID:         rule.ID,
		Severity:       "P1",
		Status:         OpsAlertStatusFiring,
		MetricValue:    &metricValue,
		ThresholdValue: &threshold,
		Dimensions:     map[string]any{"platform": "openai", "group_id": float64(12)},
		FiredAt:        firedAt,
	}
	require.True(t, svc.maybeNotifyAlert(context.Background(), nil, rule, event))

	got := awaitWebhook(t, received)
	var payload struct {
		SchemaVersion int    `json:"schema_version"`
		Event         string `json:"event"`
		OccurredAt    string `json:"occurred_at"`
		Source        struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		} `json:"source"`
		Data opsAlertWebhookData `json:"data"`
	}
	require.NoError(t, json.Unmarshal(got.body, &payload))
	require.Equal(t, notificationWebhookSchemaVersion, payload.SchemaVersion)
	require.Equal(t, NotificationEmailEventOpsAlert, payload.Event)
	require.Equal(t, firedAt.Format(time.RFC3339), payload.OccurredAt)
	require.Equal(t, "ops_alert", payload.Source.Type)
	require.Equal(t, "101", payload.Source.ID)
	require.Equal(t, *rule, payload.Data.Rule)
	require.Equal(t, *event, payload.Data.Alert)
	require.NotContains(t, string(got.body), `"rendered"`)
	require.NotContains(t, string(got.body), "<")
	require.Equal(t, 0, repo.markedCount(), "webhook delivery must not mark email_sent")
}

func TestOpsAlertWebhookRespectsMinSeverityWhenAlertEmailIsDisabled(t *testing.T) {
	ctx := context.Background()
	server, received := newWebhookRecorder(t, http.StatusOK)
	svc, _ := newOpsAlertNotifyHarness(t, server.URL, 0)

	encoded, err := json.Marshal(OpsEmailNotificationConfig{
		Alert: OpsEmailAlertConfig{Enabled: false, MinSeverity: "critical"},
	})
	require.NoError(t, err)
	require.NoError(t, svc.opsService.settingRepo.Set(ctx, SettingKeyOpsEmailNotificationConfig, string(encoded)))

	rule := &OpsAlertRule{ID: 4, Name: "warning", Severity: "P1", NotifyEmail: false}
	event := &OpsAlertEvent{ID: 401, RuleID: rule.ID, Severity: "P1", Status: OpsAlertStatusFiring, FiredAt: time.Now().UTC()}
	require.False(t, svc.maybeNotifyAlert(ctx, nil, rule, event))
	requireNoWebhook(t, received)
}

func TestOpsAlertSilenceSuppressesWebhook(t *testing.T) {
	ctx := context.Background()
	server, received := newWebhookRecorder(t, http.StatusOK)
	svc, _ := newOpsAlertNotifyHarness(t, server.URL, 0)

	rule := &OpsAlertRule{ID: 5, Name: "critical", Severity: "P0", NotifyEmail: false}
	event := &OpsAlertEvent{ID: 501, RuleID: rule.ID, Severity: "P0", Status: OpsAlertStatusFiring, FiredAt: time.Now().UTC()}
	runtimeCfg := &OpsAlertRuntimeSettings{Silencing: OpsAlertSilencingSettings{
		Enabled:            true,
		GlobalUntilRFC3339: time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
	}}
	require.False(t, svc.maybeNotifyAlert(ctx, runtimeCfg, rule, event))
	requireNoWebhook(t, received)
}

// Several admin recipients are still one alert: exactly one webhook per event.
func TestOpsAlertWebhookNotDuplicatedPerRecipient(t *testing.T) {
	ctx := context.Background()
	server, received := newWebhookRecorder(t, http.StatusOK)
	svc, _ := newOpsAlertNotifyHarness(t, server.URL, 100)

	// Re-enable the email channel and add more mailboxes: the fan-out loop must
	// not multiply webhook deliveries.
	settingRepo := svc.opsService.settingRepo
	encoded, err := json.Marshal(OpsEmailNotificationConfig{
		Alert: OpsEmailAlertConfig{
			Enabled:          true,
			Recipients:       []string{"a@example.com", "b@example.com", "c@example.com"},
			RateLimitPerHour: 100,
		},
	})
	require.NoError(t, err)
	require.NoError(t, settingRepo.Set(ctx, SettingKeyOpsEmailNotificationConfig, string(encoded)))

	rule := &OpsAlertRule{ID: 2, Name: "queue depth", Severity: "critical", NotifyEmail: true}
	svc.maybeNotifyAlert(ctx, nil, rule, &OpsAlertEvent{
		ID:       201,
		RuleID:   rule.ID,
		Severity: "critical",
		Status:   OpsAlertStatusFiring,
		FiredAt:  time.Now().UTC(),
	})

	awaitWebhook(t, received)
	requireNoWebhook(t, received)
}

// countingWebhookServer fails every request so the retry budget is the only
// thing bounding how many requests a single source event produces.
func newFailingWebhookServer(t *testing.T) (*httptest.Server, *int32) {
	t.Helper()
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	return server, &attempts
}

// B3 end-to-end: a failed first dispatch must not be restarted by the mailbox
// fan-out. With max_retries=0 one alert event may produce exactly one request,
// no matter how many admin recipients are configured.
func TestOpsAlertFailedWebhookNotRetriedByEmailFanOut(t *testing.T) {
	ctx := context.Background()
	server, attempts := newFailingWebhookServer(t)
	svc, _ := newOpsAlertNotifyHarness(t, server.URL, 100)

	settingRepo := svc.opsService.settingRepo
	encoded, err := json.Marshal(OpsEmailNotificationConfig{
		Alert: OpsEmailAlertConfig{
			Enabled:          true,
			Recipients:       []string{"a@example.com", "b@example.com", "c@example.com"},
			RateLimitPerHour: 100,
		},
	})
	require.NoError(t, err)
	require.NoError(t, settingRepo.Set(ctx, SettingKeyOpsEmailNotificationConfig, string(encoded)))

	notificationSvc := svc.emailService.notificationEmailService
	on, off := true, false
	zero := 0
	require.NoError(t, notificationSvc.SaveChannelConfig(ctx, NotificationChannelConfig{
		Webhook: NotificationWebhookGlobalConfig{
			Enabled:    true,
			Endpoint:   NotificationWebhookEndpoint{URL: server.URL},
			MaxRetries: &zero,
		},
		Events: map[string]NotificationEventChannelConfig{
			NotificationEmailEventOpsAlert: {Email: &off, Webhook: &on},
		},
	}))

	rule := &OpsAlertRule{ID: 3, Name: "error rate", Severity: "critical", NotifyEmail: true}
	svc.maybeNotifyAlert(ctx, nil, rule, &OpsAlertEvent{
		ID:       301,
		RuleID:   rule.ID,
		Severity: "critical",
		Status:   OpsAlertStatusFiring,
		FiredAt:  time.Now().UTC(),
	})

	require.Eventually(t, func() bool { return atomic.LoadInt32(attempts) >= 1 }, 5*time.Second, 20*time.Millisecond)
	time.Sleep(700 * time.Millisecond)
	require.Equal(t, int32(1), atomic.LoadInt32(attempts),
		"a failed dispatch must not be restarted once per admin recipient")
}

// The same guarantee for the other admin fan-out call site.
func TestQuotaAlertFailedWebhookNotRetriedByEmailFanOut(t *testing.T) {
	ctx := context.Background()
	server, attempts := newFailingWebhookServer(t)
	settingRepo := newNotificationEmailMemorySettingRepo()
	emailSvc := NewEmailService(settingRepo, nil)
	notificationSvc := NewNotificationEmailService(settingRepo, emailSvc)

	on, off := true, false
	zero := 0
	require.NoError(t, notificationSvc.SaveChannelConfig(ctx, NotificationChannelConfig{
		Webhook: NotificationWebhookGlobalConfig{
			Enabled:    true,
			Endpoint:   NotificationWebhookEndpoint{URL: server.URL},
			MaxRetries: &zero,
		},
		Events: map[string]NotificationEventChannelConfig{
			NotificationEmailEventAccountQuotaAlert: {Email: &off, Webhook: &on},
		},
	}))

	svc := NewBalanceNotifyService(emailSvc, settingRepo, nil)
	svc.SetNotificationEmailService(notificationSvc)

	svc.sendQuotaAlertEmails(
		[]string{"a@example.com", "b@example.com", "c@example.com"},
		77, "acct", "anthropic",
		quotaDim{name: quotaDimDaily, threshold: 10, limit: 100},
		95, "TestSite",
	)

	require.Eventually(t, func() bool { return atomic.LoadInt32(attempts) >= 1 }, 5*time.Second, 20*time.Millisecond)
	time.Sleep(700 * time.Millisecond)
	require.Equal(t, int32(1), atomic.LoadInt32(attempts),
		"quota alert fan-out must not restart a failed webhook per recipient")
}
