package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type capturedWebhook struct {
	body    []byte
	headers http.Header
}

// newWebhookRecorder returns a server plus a channel that receives one entry per
// delivered request.
func newWebhookRecorder(t *testing.T, status int) (*httptest.Server, chan capturedWebhook) {
	t.Helper()
	received := make(chan capturedWebhook, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- capturedWebhook{body: body, headers: r.Header.Clone()}
		w.WriteHeader(status)
	}))
	t.Cleanup(server.Close)
	return server, received
}

func awaitWebhook(t *testing.T, received chan capturedWebhook) capturedWebhook {
	t.Helper()
	select {
	case got := <-received:
		return got
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for webhook delivery")
		return capturedWebhook{}
	}
}

func requireNoWebhook(t *testing.T, received chan capturedWebhook) {
	t.Helper()
	select {
	case <-received:
		t.Fatal("unexpected webhook delivery")
	case <-time.After(300 * time.Millisecond):
	}
}

func enableWebhookForEvent(t *testing.T, svc *NotificationEmailService, url, event string, email bool) {
	t.Helper()
	on, off := true, false
	emailFlag := &off
	if email {
		emailFlag = &on
	}
	cfg := NotificationChannelConfig{
		Webhook: NotificationWebhookGlobalConfig{
			Enabled:  true,
			Endpoint: NotificationWebhookEndpoint{URL: url},
		},
		Events: map[string]NotificationEventChannelConfig{
			event: {Email: emailFlag, Webhook: &on},
		},
	}
	require.NoError(t, svc.SaveChannelConfig(context.Background(), cfg))
}

func TestResolveChannelsDefaultsToEmailOnly(t *testing.T) {
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)
	resolved := svc.ResolveChannels(context.Background(), NotificationEmailEventBalanceLow)
	require.True(t, resolved.Email)
	require.False(t, resolved.Webhook)
}

func TestResolveChannelsGlobalSwitchGatesEveryEvent(t *testing.T) {
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)
	on := true
	cfg := NotificationChannelConfig{
		Webhook: NotificationWebhookGlobalConfig{
			Enabled:  false,
			Endpoint: NotificationWebhookEndpoint{URL: "https://hooks.example.com/hook"},
		},
		Events: map[string]NotificationEventChannelConfig{
			NotificationEmailEventOpsAlert: {Webhook: &on},
		},
	}
	require.NoError(t, svc.SaveChannelConfig(context.Background(), cfg))

	resolved := svc.ResolveChannels(context.Background(), NotificationEmailEventOpsAlert)
	require.False(t, resolved.Webhook, "global switch must override the per-event setting")
}

func TestResolveChannelsPerEventEndpointOverridesGlobal(t *testing.T) {
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)
	on := true
	cfg := NotificationChannelConfig{
		Webhook: NotificationWebhookGlobalConfig{
			Enabled:  true,
			Endpoint: NotificationWebhookEndpoint{URL: "https://hooks.example.com/default"},
		},
		Events: map[string]NotificationEventChannelConfig{
			NotificationEmailEventOpsAlert: {
				Webhook:  &on,
				Endpoint: &NotificationWebhookEndpoint{URL: "https://hooks.example.com/alerts"},
			},
		},
	}
	require.NoError(t, svc.SaveChannelConfig(context.Background(), cfg))

	resolved := svc.ResolveChannels(context.Background(), NotificationEmailEventOpsAlert)
	require.True(t, resolved.Webhook)
	require.Equal(t, "https://hooks.example.com/alerts", resolved.Endpoint.URL)
}

func TestSendDeliversDefaultWebhookPayload(t *testing.T) {
	ctx := context.Background()
	server, received := newWebhookRecorder(t, http.StatusOK)
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)
	enableWebhookForEvent(t, svc, server.URL, NotificationEmailEventContentModerationViolation, false)

	require.NoError(t, svc.Send(ctx, NotificationEmailSendInput{
		Event:          NotificationEmailEventContentModerationViolation,
		RecipientEmail: "user@example.com",
		RecipientName:  "alice",
		UserID:         1234,
		SourceType:     "content_moderation",
		SourceID:       "99",
		Variables: map[string]string{
			"moderation_category": "violence",
			"violation_count":     "3",
		},
	}))

	got := awaitWebhook(t, received)
	var payload notificationWebhookPayload
	require.NoError(t, json.Unmarshal(got.body, &payload))
	require.Equal(t, NotificationEmailEventContentModerationViolation, payload.Event)
	require.Equal(t, NotificationAudienceUser, payload.Audience)
	require.Equal(t, notificationWebhookSchemaVersion, payload.SchemaVersion)
	require.Equal(t, int64(1234), payload.Recipient.UserID)
	require.Equal(t, "user@example.com", payload.Recipient.Email)
	require.NotEmpty(t, payload.OccurredAt)
	data, ok := payload.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "violence", data["moderation_category"])
	require.Equal(t, "3", data["violation_count"])
	require.NotContains(t, string(got.body), `"rendered"`)
	require.NotContains(t, string(got.body), "<")
	require.Equal(t, NotificationEmailEventContentModerationViolation, got.headers.Get(notificationWebhookEventHeader))
}

func TestSendWithResultWebhookOnlyDoesNotReportEmailSent(t *testing.T) {
	ctx := context.Background()
	server, received := newWebhookRecorder(t, http.StatusOK)
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)
	enableWebhookForEvent(t, svc, server.URL, NotificationEmailEventBalanceLow, false)

	result, err := svc.SendWithResult(ctx, NotificationEmailSendInput{
		Event:          NotificationEmailEventBalanceLow,
		RecipientEmail: "user@example.com",
		SourceType:     "balance",
		SourceID:       "1",
		Variables:      map[string]string{"balance": "1.00"},
	})
	require.NoError(t, err)
	require.False(t, result.EmailSent)
	require.True(t, result.WebhookQueued)
	_ = awaitWebhook(t, received)
}

func TestCyberPolicyWebhookUsesStableNotificationFields(t *testing.T) {
	ctx := context.Background()
	server, received := newWebhookRecorder(t, http.StatusOK)
	settingRepo := newNotificationEmailMemorySettingRepo()
	notificationSvc := NewNotificationEmailService(settingRepo, nil)
	enableWebhookForEvent(t, notificationSvc, server.URL, NotificationEmailEventCyberPolicyNotice, false)

	emailSvc := NewEmailService(settingRepo, nil)
	emailSvc.SetNotificationEmailService(notificationSvc)
	moderationSvc := &ContentModerationService{
		settingRepo:  settingRepo,
		emailService: emailSvc,
	}

	require.NoError(t, moderationSvc.sendCyberPolicyEmail(ctx, &ContentModerationLog{
		ID:           99,
		UserEmail:    "user@example.com",
		Model:        "gpt-5.5",
		InputExcerpt: "actual user prompt",
		Error:        "upstream cyber response",
		CreatedAt:    time.Now().UTC(),
	}))

	got := awaitWebhook(t, received)
	var payload notificationWebhookPayload
	require.NoError(t, json.Unmarshal(got.body, &payload))
	data, ok := payload.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "gpt-5.5", data["model"])
	require.Equal(t, "-", data["group_name"])
	require.NotEmpty(t, data["triggered_at"])
	require.Equal(t, "upstream cyber response", data["upstream_message"])
	require.NotContains(t, data, "input_excerpt")
}

func TestSendUsesAdminAudienceForOpsEvents(t *testing.T) {
	ctx := context.Background()
	server, received := newWebhookRecorder(t, http.StatusOK)
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)
	enableWebhookForEvent(t, svc, server.URL, NotificationEmailEventOpsAlert, false)

	require.NoError(t, svc.Send(ctx, NotificationEmailSendInput{
		Event:          NotificationEmailEventOpsAlert,
		RecipientEmail: "ops@example.com",
		SourceType:     "ops_alert",
		SourceID:       "1",
	}))

	got := awaitWebhook(t, received)
	var payload notificationWebhookPayload
	require.NoError(t, json.Unmarshal(got.body, &payload))
	require.Equal(t, NotificationAudienceAdmin, payload.Audience)
}

func TestSendCustomBodyTemplateEscapesJSON(t *testing.T) {
	ctx := context.Background()
	server, received := newWebhookRecorder(t, http.StatusOK)
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)

	on, off := true, false
	cfg := NotificationChannelConfig{
		Webhook: NotificationWebhookGlobalConfig{
			Enabled:  true,
			Endpoint: NotificationWebhookEndpoint{URL: server.URL},
		},
		Events: map[string]NotificationEventChannelConfig{
			NotificationEmailEventOpsAlert: {
				Email:   &off,
				Webhook: &on,
				Endpoint: &NotificationWebhookEndpoint{
					BodyTemplate: `{"msgtype":"text","text":{"content":"{{rule_name}}"}}`,
				},
			},
		},
	}
	require.NoError(t, svc.SaveChannelConfig(ctx, cfg))

	require.NoError(t, svc.Send(ctx, NotificationEmailSendInput{
		Event:          NotificationEmailEventOpsAlert,
		RecipientEmail: "ops@example.com",
		SourceType:     "ops_alert",
		SourceID:       "7",
		Variables:      map[string]string{"rule_name": `he said "boom"` + "\n" + `and \ broke`},
	}))

	got := awaitWebhook(t, received)
	require.True(t, json.Valid(got.body), "custom template must render to valid JSON: %s", string(got.body))

	var decoded struct {
		MsgType string `json:"msgtype"`
		Text    struct {
			Content string `json:"content"`
		} `json:"text"`
	}
	require.NoError(t, json.Unmarshal(got.body, &decoded))
	require.Equal(t, "text", decoded.MsgType)
	require.Equal(t, `he said "boom"`+"\n"+`and \ broke`, decoded.Text.Content)
}

func TestLegacyRenderedPlaceholdersStayValidButEmpty(t *testing.T) {
	ctx := context.Background()
	server, received := newWebhookRecorder(t, http.StatusOK)
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)

	on, off := true, false
	require.NoError(t, svc.SaveChannelConfig(ctx, NotificationChannelConfig{
		Webhook: NotificationWebhookGlobalConfig{
			Enabled:  true,
			Endpoint: NotificationWebhookEndpoint{URL: server.URL},
		},
		Events: map[string]NotificationEventChannelConfig{
			NotificationEmailEventOpsAlert: {
				Email:   &off,
				Webhook: &on,
				Endpoint: &NotificationWebhookEndpoint{
					BodyTemplate: `{"title":"{{rendered_title}}","body":"{{rendered_text}}","event":"{{event}}"}`,
				},
			},
		},
	}))
	require.NotContains(t, NotificationWebhookExtraPlaceholders(), "rendered_title")
	require.NotContains(t, NotificationWebhookExtraPlaceholders(), "rendered_text")

	require.NoError(t, svc.Send(ctx, NotificationEmailSendInput{
		Event:      NotificationEmailEventOpsAlert,
		SourceType: "ops_alert",
		SourceID:   "legacy-template",
	}))

	var got map[string]string
	require.NoError(t, json.Unmarshal(awaitWebhook(t, received).body, &got))
	require.Equal(t, "", got["title"])
	require.Equal(t, "", got["body"])
	require.Equal(t, NotificationEmailEventOpsAlert, got["event"])
}

func TestWebhookOnlyDoesNotLoadEmailTemplate(t *testing.T) {
	ctx := context.Background()
	repo := newNotificationEmailMemorySettingRepo()
	server, received := newWebhookRecorder(t, http.StatusOK)
	svc := NewNotificationEmailService(repo, nil)
	enableWebhookForEvent(t, svc, server.URL, NotificationEmailEventOpsAlert, false)

	// Bypass UpdateTemplate intentionally: this is a broken stored override that
	// would make GetTemplate fail if a webhook-only delivery still read email.
	require.NoError(t, repo.Set(ctx, notificationEmailTemplateKey(NotificationEmailEventOpsAlert, notificationEmailDefaultLocale), "{"))
	require.NoError(t, svc.Send(ctx, NotificationEmailSendInput{
		Event:      NotificationEmailEventOpsAlert,
		SourceType: "ops_alert",
		SourceID:   "template-independent",
	}))
	awaitWebhook(t, received)

	enableWebhookForEvent(t, svc, server.URL, NotificationEmailEventOpsAlert, true)
	err := svc.Send(ctx, NotificationEmailSendInput{
		Event:          NotificationEmailEventOpsAlert,
		RecipientEmail: "ops@example.com",
		SourceType:     "ops_alert",
		SourceID:       "template-required-for-email",
	})
	require.Error(t, err)
}

func TestSaveChannelConfigRejectsTemplateRenderingToInvalidJSON(t *testing.T) {
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)
	on := true
	cfg := NotificationChannelConfig{
		Webhook: NotificationWebhookGlobalConfig{
			Enabled:  true,
			Endpoint: NotificationWebhookEndpoint{URL: "https://hooks.example.com/hook"},
		},
		Events: map[string]NotificationEventChannelConfig{
			NotificationEmailEventOpsAlert: {
				Webhook:  &on,
				Endpoint: &NotificationWebhookEndpoint{BodyTemplate: `{"content": {{rule_name}}}`},
			},
		},
	}
	require.Error(t, svc.SaveChannelConfig(context.Background(), cfg))
}

func TestSaveChannelConfigRejectsUnknownPlaceholder(t *testing.T) {
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)
	on := true
	cfg := NotificationChannelConfig{
		Webhook: NotificationWebhookGlobalConfig{
			Enabled:  true,
			Endpoint: NotificationWebhookEndpoint{URL: "https://hooks.example.com/hook"},
		},
		Events: map[string]NotificationEventChannelConfig{
			NotificationEmailEventOpsAlert: {
				Webhook:  &on,
				Endpoint: &NotificationWebhookEndpoint{BodyTemplate: `{"content":"{{totally_unknown}}"}`},
			},
		},
	}
	err := svc.SaveChannelConfig(context.Background(), cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "totally_unknown")
}

// With the email channel off, Send must report success even though no email
// service is wired up. Returning a template/config error here would make the
// call sites fall back to their hardcoded email bodies and defeat the operator.
func TestSendWithEmailDisabledNeverTriggersCallerEmailFallback(t *testing.T) {
	ctx := context.Background()
	server, received := newWebhookRecorder(t, http.StatusOK)
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)
	enableWebhookForEvent(t, svc, server.URL, NotificationEmailEventOpsAlert, false)

	err := svc.Send(ctx, NotificationEmailSendInput{
		Event:          NotificationEmailEventOpsAlert,
		RecipientEmail: "ops@example.com",
		SourceType:     "ops_alert",
		SourceID:       "42",
	})
	require.NoError(t, err)
	require.False(t, shouldFallbackNotificationEmail(err))
	awaitWebhook(t, received)
}

// Admin alerts fan out over every configured mailbox, but they are one event:
// the receiver must not get a duplicate per recipient.
func TestSendAdminFanOutDeliversSingleWebhook(t *testing.T) {
	ctx := context.Background()
	server, received := newWebhookRecorder(t, http.StatusOK)
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)
	enableWebhookForEvent(t, svc, server.URL, NotificationEmailEventOpsAlert, false)

	for _, addr := range []string{"a@example.com", "b@example.com", "c@example.com"} {
		require.NoError(t, svc.Send(ctx, NotificationEmailSendInput{
			Event:          NotificationEmailEventOpsAlert,
			RecipientEmail: addr,
			SourceType:     "ops_alert",
			SourceID:       "555",
		}))
	}

	awaitWebhook(t, received)
	requireNoWebhook(t, received)
}

// A user-facing event is addressed per mailbox, so each recipient is a separate
// notification and must not be collapsed.
func TestSendUserAudienceKeepsPerRecipientWebhooks(t *testing.T) {
	ctx := context.Background()
	server, received := newWebhookRecorder(t, http.StatusOK)
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)
	enableWebhookForEvent(t, svc, server.URL, NotificationEmailEventContentModerationViolation, false)

	for _, addr := range []string{"one@example.com", "two@example.com"} {
		require.NoError(t, svc.Send(ctx, NotificationEmailSendInput{
			Event:          NotificationEmailEventContentModerationViolation,
			RecipientEmail: addr,
			SourceType:     "content_moderation",
			SourceID:       "31",
		}))
	}

	awaitWebhook(t, received)
	awaitWebhook(t, received)
}

func TestSendUnsubscribeSilencesWebhookToo(t *testing.T) {
	ctx := context.Background()
	repo := newNotificationEmailMemorySettingRepo()
	server, received := newWebhookRecorder(t, http.StatusOK)
	svc := NewNotificationEmailService(repo, nil)
	enableWebhookForEvent(t, svc, server.URL, NotificationEmailEventBalanceLow, false)

	require.NoError(t, repo.Set(ctx, notificationEmailPreferenceKey(NotificationEmailEventBalanceLow, "user@example.com"), "unsubscribed"))

	require.NoError(t, svc.Send(ctx, NotificationEmailSendInput{
		Event:          NotificationEmailEventBalanceLow,
		RecipientEmail: "user@example.com",
		SourceType:     "balance",
		SourceID:       "1",
	}))
	requireNoWebhook(t, received)
}

func TestWebhookSignsBodyWhenSecretConfigured(t *testing.T) {
	ctx := context.Background()
	server, received := newWebhookRecorder(t, http.StatusOK)
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)

	on, off := true, false
	cfg := NotificationChannelConfig{
		Webhook: NotificationWebhookGlobalConfig{
			Enabled:  true,
			Endpoint: NotificationWebhookEndpoint{URL: server.URL},
			Secret:   "s3cr3t",
		},
		Events: map[string]NotificationEventChannelConfig{
			NotificationEmailEventOpsAlert: {Email: &off, Webhook: &on},
		},
	}
	require.NoError(t, svc.SaveChannelConfig(ctx, cfg))

	require.NoError(t, svc.Send(ctx, NotificationEmailSendInput{
		Event:          NotificationEmailEventOpsAlert,
		RecipientEmail: "ops@example.com",
		SourceType:     "ops_alert",
		SourceID:       "8",
	}))

	got := awaitWebhook(t, received)
	timestamp := got.headers.Get(notificationWebhookTimestampHeader)
	require.NotEmpty(t, timestamp)
	require.Equal(t, signNotificationWebhookBody("s3cr3t", timestamp, got.body), got.headers.Get(notificationWebhookSignatureHeader))
}

func TestValidateNotificationWebhookURLRejectsNonHTTP(t *testing.T) {
	require.Error(t, validateNotificationWebhookURL("file:///etc/passwd"))
	require.Error(t, validateNotificationWebhookURL("ftp://example.com/hook"))
	require.NoError(t, validateNotificationWebhookURL("https://example.com/hook"))
}

func TestEscapeNotificationJSONStringHandlesControlCharacters(t *testing.T) {
	escaped := escapeNotificationJSONString("a\"b\\c\nd")
	require.False(t, strings.Contains(escaped, "\n"))
	var out string
	require.NoError(t, json.Unmarshal([]byte(`"`+escaped+`"`), &out))
	require.Equal(t, "a\"b\\c\nd", out)
}

// B3 regression: an explicitly configured 0 must mean "never retry".
func TestWebhookExplicitZeroRetriesSendsExactlyOneRequest(t *testing.T) {
	ctx := context.Background()
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)
	on, off := true, false
	zero := 0
	cfg := NotificationChannelConfig{
		Webhook: NotificationWebhookGlobalConfig{
			Enabled:    true,
			Endpoint:   NotificationWebhookEndpoint{URL: server.URL},
			MaxRetries: &zero,
		},
		Events: map[string]NotificationEventChannelConfig{
			NotificationEmailEventOpsAlert: {Email: &off, Webhook: &on},
		},
	}
	require.NoError(t, svc.SaveChannelConfig(ctx, cfg))
	require.Equal(t, 0, svc.ResolveChannels(ctx, NotificationEmailEventOpsAlert).MaxRetries)

	require.NoError(t, svc.Send(ctx, NotificationEmailSendInput{
		Event:          NotificationEmailEventOpsAlert,
		RecipientEmail: "ops@example.com",
		SourceType:     "ops_alert",
		SourceID:       "zero-retry",
	}))

	require.Eventually(t, func() bool { return atomic.LoadInt32(&attempts) >= 1 }, 5*time.Second, 20*time.Millisecond)
	time.Sleep(500 * time.Millisecond)
	require.Equal(t, int32(1), atomic.LoadInt32(&attempts), "explicit 0 must disable retries")
}

func TestWebhookUnsetRetriesKeepsDefault(t *testing.T) {
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)
	on := true
	require.NoError(t, svc.SaveChannelConfig(context.Background(), NotificationChannelConfig{
		Webhook: NotificationWebhookGlobalConfig{
			Enabled:  true,
			Endpoint: NotificationWebhookEndpoint{URL: "https://hooks.example.com/hook"},
		},
		Events: map[string]NotificationEventChannelConfig{
			NotificationEmailEventOpsAlert: {Webhook: &on},
		},
	}))
	resolved := svc.ResolveChannels(context.Background(), NotificationEmailEventOpsAlert)
	require.Equal(t, notificationWebhookDefaultMaxRetries, resolved.MaxRetries)
}

// Deterministic guard for the mechanism the admin fan-out loops rely on: an
// email-only send must never reach the webhook channel, whatever the matrix
// says. The integration-level fan-out tests assert the same invariant, but
// only the ordering-independent guarantee is pinned here.
func TestSendEmailOnlyNeverDispatchesWebhook(t *testing.T) {
	ctx := context.Background()
	server, received := newWebhookRecorder(t, http.StatusOK)
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)
	enableWebhookForEvent(t, svc, server.URL, NotificationEmailEventOpsAlert, false)

	require.True(t, svc.ResolveChannels(ctx, NotificationEmailEventOpsAlert).Webhook,
		"precondition: webhook is enabled for this event")

	require.NoError(t, svc.Send(ctx, NotificationEmailSendInput{
		Event:          NotificationEmailEventOpsAlert,
		RecipientEmail: "ops@example.com",
		SourceType:     "ops_alert",
		SourceID:       "email-only",
		emailOnly:      true,
	}))
	requireNoWebhook(t, received)
}

// An unsigned delivery would let anyone who can reach the endpoint forge a
// notification, so enabling webhook delivery without a secret must generate one
// rather than silently ship unauthenticated requests.
func TestSaveChannelConfigGeneratesSecretWhenWebhookEnabled(t *testing.T) {
	ctx := context.Background()
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)

	require.NoError(t, svc.SaveChannelConfig(ctx, NotificationChannelConfig{
		Webhook: NotificationWebhookGlobalConfig{
			Enabled:  true,
			Endpoint: NotificationWebhookEndpoint{URL: "https://hooks.example.com/hook"},
		},
	}))

	cfg, err := svc.GetChannelConfig(ctx)
	require.NoError(t, err)
	require.Len(t, cfg.Webhook.Secret, 64, "expected a 32-byte hex secret")

	// Saving again must not rotate it.
	require.NoError(t, svc.SaveChannelConfig(ctx, cfg))
	after, err := svc.GetChannelConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, cfg.Webhook.Secret, after.Webhook.Secret)
}

// Every delivery is signed now, so the signature header is never absent.
func TestWebhookAlwaysSigned(t *testing.T) {
	ctx := context.Background()
	server, received := newWebhookRecorder(t, http.StatusOK)
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)
	enableWebhookForEvent(t, svc, server.URL, NotificationEmailEventOpsAlert, false)

	cfg, err := svc.GetChannelConfig(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, cfg.Webhook.Secret)

	require.NoError(t, svc.Send(ctx, NotificationEmailSendInput{
		Event:          NotificationEmailEventOpsAlert,
		RecipientEmail: "ops@example.com",
		SourceType:     "ops_alert",
		SourceID:       "signed",
	}))

	got := awaitWebhook(t, received)
	timestamp := got.headers.Get(notificationWebhookTimestampHeader)
	require.NotEmpty(t, timestamp)
	require.Equal(t,
		signNotificationWebhookBody(cfg.Webhook.Secret, timestamp, got.body),
		got.headers.Get(notificationWebhookSignatureHeader))
}

// B2 regression: forcing the channel on for a test delivery must not bypass the
// signing secret, or a saved-but-disabled config could be delivered signed with
// an empty key.
func TestSendTestWebhookRejectsBlankSecret(t *testing.T) {
	ctx := context.Background()
	server, received := newWebhookRecorder(t, http.StatusOK)
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)

	// Saved while disabled: SaveChannelConfig does not generate a secret here.
	cfg := NotificationChannelConfig{
		Webhook: NotificationWebhookGlobalConfig{
			Enabled:  false,
			Endpoint: NotificationWebhookEndpoint{URL: server.URL},
		},
	}
	require.NoError(t, svc.SaveChannelConfig(ctx, cfg))

	stored, err := svc.GetChannelConfig(ctx)
	require.NoError(t, err)
	require.Empty(t, stored.Webhook.Secret)

	require.Error(t, svc.SendTestWebhook(ctx, stored, NotificationEmailEventOpsAlert))
	requireNoWebhook(t, received)
}

func TestSendTestWebhookMatchesOpsEventDataShapes(t *testing.T) {
	ctx := context.Background()
	server, received := newWebhookRecorder(t, http.StatusOK)
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)
	require.NoError(t, svc.SaveChannelConfig(ctx, NotificationChannelConfig{
		Webhook: NotificationWebhookGlobalConfig{
			Enabled:  true,
			Endpoint: NotificationWebhookEndpoint{URL: server.URL},
		},
	}))
	cfg, err := svc.GetChannelConfig(ctx)
	require.NoError(t, err)

	require.NoError(t, svc.SendTestWebhook(ctx, cfg, NotificationEmailEventOpsAlert))
	var alertPayload struct {
		Data opsAlertWebhookData `json:"data"`
	}
	require.NoError(t, json.Unmarshal(awaitWebhook(t, received).body, &alertPayload))
	require.NotZero(t, alertPayload.Data.Rule.ID)
	require.NotZero(t, alertPayload.Data.Alert.ID)

	require.NoError(t, svc.SendTestWebhook(ctx, cfg, NotificationEmailEventOpsScheduledReport))
	var reportPayload struct {
		Data opsScheduledReportWebhookData `json:"data"`
	}
	got := awaitWebhook(t, received)
	require.NoError(t, json.Unmarshal(got.body, &reportPayload))
	require.Equal(t, "daily_summary", reportPayload.Data.Report.Type)
	require.NotNil(t, reportPayload.Data.Overview)
	require.NotContains(t, string(got.body), "report_html")
	require.NotContains(t, string(got.body), "report_detail_display")
}

// Config that somehow carries no secret (e.g. JSON written before the secret
// moved off the endpoint) must not deliver at all.
func TestResolveChannelsDisablesWebhookWithoutSecret(t *testing.T) {
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)
	on := true
	resolved := applyNotificationChannelConfig(
		defaultResolvedNotificationChannels(),
		NotificationChannelConfig{
			Webhook: NotificationWebhookGlobalConfig{
				Enabled:  true,
				Endpoint: NotificationWebhookEndpoint{URL: "https://hooks.example.com/hook"},
			},
			Events: map[string]NotificationEventChannelConfig{
				NotificationEmailEventOpsAlert: {Webhook: &on},
			},
		},
		NotificationEmailEventOpsAlert,
	)
	require.False(t, resolved.Webhook, "a blank secret must fail safe to no delivery")
	_ = svc
}

// The stored secret and the signed secret must be the same bytes.
func TestSaveChannelConfigTrimsSecret(t *testing.T) {
	ctx := context.Background()
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)
	require.NoError(t, svc.SaveChannelConfig(ctx, NotificationChannelConfig{
		Webhook: NotificationWebhookGlobalConfig{
			Enabled:  true,
			Endpoint: NotificationWebhookEndpoint{URL: "https://hooks.example.com/hook"},
			Secret:   "  padded-secret  ",
		},
	}))
	cfg, err := svc.GetChannelConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, "padded-secret", cfg.Webhook.Secret)
}

// Compatibility contract: JSON written before the secret moved off the endpoint
// must fail safe. The old field is no longer read, so the resolver has to treat
// the configuration as "no secret" and refuse to deliver rather than sign with
// an empty key.
func TestResolveChannelsIgnoresLegacyEndpointSecret(t *testing.T) {
	ctx := context.Background()
	repo := newNotificationEmailMemorySettingRepo()
	svc := NewNotificationEmailService(repo, nil)

	legacy := `{
		"webhook": {
			"enabled": true,
			"endpoint": {
				"url": "https://hooks.example.com/hook",
				"secret": "legacy-secret-on-endpoint",
				"method": "PUT",
				"headers": {"Authorization": "Bearer legacy"}
			}
		},
		"events": {"ops.alert": {"webhook": true}}
	}`
	require.NoError(t, repo.Set(ctx, notificationChannelConfigKey, legacy))

	cfg, err := svc.GetChannelConfig(ctx)
	require.NoError(t, err)
	require.True(t, cfg.Webhook.Enabled, "the rest of the legacy blob still parses")
	require.Empty(t, cfg.Webhook.Secret, "the legacy endpoint-level secret is not read")

	resolved := svc.ResolveChannels(ctx, NotificationEmailEventOpsAlert)
	require.False(t, resolved.Webhook, "a legacy config must not deliver unsigned")
	require.True(t, resolved.Email, "the email channel is unaffected")
}
