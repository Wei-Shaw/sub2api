package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Webhook delivery headers. Every delivery is signed, so a receiver can always
// verify the sender.
const (
	notificationWebhookSignatureHeader = "X-Sub2Api-Signature"
	notificationWebhookTimestampHeader = "X-Sub2Api-Timestamp"
	notificationWebhookEventHeader     = "X-Sub2Api-Event"
	notificationWebhookDeliveryHeader  = "X-Sub2Api-Delivery"
)

var (
	notificationWebhookSchemaVersion = 1
)

// notificationWebhookPayloadInput carries the source event and delivery context.
// Webhooks deliberately do not include email-template output.
type notificationWebhookPayloadInput struct {
	Event         string
	Info          NotificationEmailEventInfo
	Locale        string
	SiteName      string
	Recipient     string
	RecipientName string
	UserID        int64
	SourceType    string
	SourceID      string
	DeliveryID    string
	OccurredAt    string
	Timestamp     string
	Variables     map[string]string
	Data          any
}

type notificationWebhookRecipient struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

type notificationWebhookSource struct {
	Type string `json:"type,omitempty"`
	ID   string `json:"id,omitempty"`
}

// notificationWebhookPayload is the default raw event envelope. `event`
// discriminates the event-specific shape carried in Data.
type notificationWebhookPayload struct {
	SchemaVersion int                          `json:"schema_version"`
	Event         string                       `json:"event"`
	EventLabel    string                       `json:"event_label"`
	Category      string                       `json:"category"`
	Audience      string                       `json:"audience"`
	Locale        string                       `json:"locale"`
	SiteName      string                       `json:"site_name"`
	DeliveryID    string                       `json:"delivery_id"`
	OccurredAt    string                       `json:"occurred_at"`
	Timestamp     string                       `json:"timestamp"`
	Recipient     notificationWebhookRecipient `json:"recipient"`
	Source        notificationWebhookSource    `json:"source,omitempty"`
	Data          any                          `json:"data"`
}

// deliverWebhook builds the body and posts it, retrying transient failures with
// bounded exponential backoff. It is best-effort: the error is returned for
// logging, never to fail the originating operation.
func (s *NotificationEmailService) deliverWebhook(
	ctx context.Context,
	channels ResolvedNotificationChannels,
	input notificationWebhookPayloadInput,
) error {
	body, err := buildNotificationWebhookBody(channels.Endpoint, input)
	if err != nil {
		return err
	}

	timeout := time.Duration(channels.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = notificationWebhookDefaultTimeoutSeconds * time.Second
	}
	client := notificationWebhookClient(timeout)

	attempts := channels.MaxRetries + 1
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}
		retryable, err := sendNotificationWebhookRequest(ctx, client, channels.Endpoint.URL, channels.Secret, input, body)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retryable {
			return err
		}
	}
	return lastErr
}

func sendNotificationWebhookRequest(
	ctx context.Context,
	client *http.Client,
	url string,
	secret string,
	input notificationWebhookPayloadInput,
	body []byte,
) (retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSpace(url), bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set(notificationWebhookEventHeader, input.Event)
	req.Header.Set(notificationWebhookDeliveryHeader, input.DeliveryID)
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	req.Header.Set(notificationWebhookTimestampHeader, timestamp)
	req.Header.Set(notificationWebhookSignatureHeader, signNotificationWebhookBody(secret, timestamp, body))

	resp, err := client.Do(req)
	if err != nil {
		return true, err
	}
	defer func() {
		// The response body is deliberately discarded and must never be
		// surfaced to a caller, a log, or the admin UI. The target URL is
		// operator-supplied, so returning what it replied with would turn this
		// into an SSRF exfiltration primitive. That invariant — not an address
		// policy — is what keeps this outbound call safe.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return false, nil
	}
	// 4xx means the receiver rejected the payload; retrying sends the same body.
	if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
		return false, fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return true, fmt.Errorf("webhook returned status %d", resp.StatusCode)
}

// signNotificationWebhookBody produces hex(HMAC-SHA256(secret, timestamp.body)).
// The timestamp is inside the signed material so a captured request cannot be
// replayed with a fresh timestamp.
func signNotificationWebhookBody(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func buildNotificationWebhookBody(endpoint NotificationWebhookEndpoint, input notificationWebhookPayloadInput) ([]byte, error) {
	if template := strings.TrimSpace(endpoint.BodyTemplate); template != "" {
		allowed := notificationWebhookAllowedPlaceholderSet(input.Event)
		variables := notificationWebhookTemplateVariables(input)
		rendered, err := renderNotificationTemplateString(allowed, template, variables, nil, notificationEscapeJSON, nil)
		if err != nil {
			return nil, err
		}
		if !json.Valid([]byte(rendered)) {
			return nil, errors.New("rendered webhook body is not valid JSON")
		}
		return []byte(rendered), nil
	}
	return json.Marshal(buildNotificationWebhookPayload(input))
}

func buildNotificationWebhookPayload(input notificationWebhookPayloadInput) notificationWebhookPayload {
	data := input.Data
	if data == nil {
		// Callers without an event-specific DTO use Variables as source data. Do
		// not pass email-template output or presentation values through this path.
		data = cloneNotificationWebhookVariables(input.Variables)
	}
	return notificationWebhookPayload{
		SchemaVersion: notificationWebhookSchemaVersion,
		Event:         input.Event,
		EventLabel:    input.Info.Label,
		Category:      input.Info.Category,
		Audience:      notificationAudienceForCategory(input.Info.Category),
		Locale:        input.Locale,
		SiteName:      input.SiteName,
		DeliveryID:    input.DeliveryID,
		OccurredAt:    input.OccurredAt,
		Timestamp:     input.Timestamp,
		Recipient: notificationWebhookRecipient{
			UserID:   input.UserID,
			Username: input.RecipientName,
			Email:    input.Recipient,
		},
		Source: notificationWebhookSource{
			Type: input.SourceType,
			ID:   input.SourceID,
		},
		Data: data,
	}
}

func cloneNotificationWebhookVariables(input map[string]string) map[string]string {
	data := make(map[string]string, len(input))
	for key, value := range input {
		data[key] = value
	}
	return data
}

// notificationWebhookTemplateVariables merges event variables with the
// webhook-only context values so a custom template can reference either.
func notificationWebhookTemplateVariables(input notificationWebhookPayloadInput) map[string]string {
	variables := make(map[string]string, len(input.Variables)+len(notificationWebhookInternalPlaceholders))
	for key, value := range input.Variables {
		variables[key] = value
	}
	variables["event"] = input.Event
	variables["event_label"] = input.Info.Label
	variables["event_category"] = input.Info.Category
	variables["audience"] = notificationAudienceForCategory(input.Info.Category)
	variables["locale"] = input.Locale
	variables["site_name"] = input.SiteName
	variables["recipient_name"] = input.RecipientName
	variables["recipient_email"] = input.Recipient
	variables["user_id"] = strconv.FormatInt(input.UserID, 10)
	variables["source_type"] = input.SourceType
	variables["source_id"] = input.SourceID
	variables["occurred_at"] = input.OccurredAt
	variables["timestamp"] = input.Timestamp
	// Compatibility only: old custom templates remain valid but no longer
	// receive sender-rendered email content.
	variables["rendered_title"] = ""
	variables["rendered_text"] = ""
	return variables
}

// notificationWebhookClient builds the delivery client.
//
// There is no dial-time address policy here, so an operator-configured URL can
// reach any address this process can route to — a known admin-level blind SSRF.
// That is a deliberate scoping decision, not an oversight: the closest analogue
// in this repo, content moderation, only format-validates its base URL, follows
// redirects, and reads back the response body, so a webhook-only restriction
// would not stop an attacker holding admin authority. Tightening the threat
// model belongs in one project-wide decision covering every admin-configured
// outbound endpoint.
//
// Note that security.url_allowlist does NOT apply here: this client never
// receives Config, and neither does content moderation. It is a related global
// setting, not protection this path already has.
//
// Two things are stricter than the moderation client because they cost nothing:
// redirects are refused so a delivery cannot be diverted to an address the
// operator never configured, and the response body is always discarded (see
// sendNotificationWebhookRequest).
func notificationWebhookClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   timeout,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   timeout,
			ResponseHeaderTimeout: timeout,
			MaxIdleConns:          8,
			IdleConnTimeout:       60 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func validateNotificationWebhookURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return errors.New("webhook url is invalid")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return errors.New("webhook url must use http or https")
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return errors.New("webhook url must include a host")
	}
	return nil
}

func logNotificationWebhookFailure(event, recipient string, err error) {
	slog.Warn("notification webhook delivery failed",
		"event", event,
		"recipient_hash", notificationEmailHash(recipient),
		"error", err.Error(),
	)
}

// SendTestWebhook delivers a sample payload synchronously so an operator can
// verify an endpoint from the admin UI before enabling it. The per-event
// webhook switch is bypassed on purpose: the point is to test the transport.
func (s *NotificationEmailService) SendTestWebhook(ctx context.Context, cfg NotificationChannelConfig, event string) error {
	info, normalizedEvent, err := s.eventInfo(event)
	if err != nil {
		return err
	}
	channels := applyNotificationChannelConfig(defaultResolvedNotificationChannels(), cfg, normalizedEvent)
	channels.Webhook = true
	if strings.TrimSpace(channels.Endpoint.URL) == "" {
		return errors.New("no webhook url configured for this event")
	}
	// Forcing the channel on above bypasses the enabled gate, so the secret has
	// to be checked here too: otherwise a saved-but-disabled configuration could
	// be test-delivered signed with an empty key.
	if strings.TrimSpace(channels.Secret) == "" {
		return errors.New("save the webhook configuration first to generate a signing secret")
	}

	locale := notificationEmailDefaultLocale
	variables := notificationEmailSampleVariables(locale)
	addNotificationEmailOpsSummarySampleVariables(variables)
	for _, placeholder := range info.Placeholders {
		if _, ok := variables[placeholder]; !ok {
			variables[placeholder] = "sample"
		}
	}

	now := time.Now().UTC()
	payload := notificationWebhookPayloadInput{
		Event:         normalizedEvent,
		Info:          info,
		Locale:        locale,
		SiteName:      s.siteName(ctx),
		Recipient:     variables["recipient_email"],
		RecipientName: variables["recipient_name"],
		SourceType:    "webhook_test",
		SourceID:      strconv.FormatInt(time.Now().UTC().UnixNano(), 10),
		DeliveryID:    notificationWebhookDeliveryID(""),
		OccurredAt:    now.Format(time.RFC3339),
		Timestamp:     now.Format(time.RFC3339),
		Variables:     variables,
		Data:          notificationWebhookSampleData(normalizedEvent, now),
	}
	return s.deliverWebhook(ctx, channels, payload)
}

// notificationWebhookSampleData keeps the admin test button's event payload
// shape aligned with production without involving an email template.
func notificationWebhookSampleData(event string, now time.Time) any {
	switch event {
	case NotificationEmailEventOpsAlert:
		metricValue := 6.91
		threshold := 5.0
		return newOpsAlertWebhookData(&OpsAlertRule{
			ID:            1,
			Name:          "sample alert rule",
			Severity:      "P1",
			MetricType:    "error_rate",
			Operator:      ">",
			Threshold:     threshold,
			WindowMinutes: 5,
		}, &OpsAlertEvent{
			ID:             1,
			RuleID:         1,
			Severity:       "P1",
			Status:         OpsAlertStatusFiring,
			MetricValue:    &metricValue,
			ThresholdValue: &threshold,
			FiredAt:        now,
			CreatedAt:      now,
		})
	case NotificationEmailEventOpsScheduledReport:
		return newOpsScheduledReportWebhookData(&opsScheduledReport{
			Name:       "sample report",
			ReportType: "daily_summary",
			Schedule:   "0 9 * * *",
			TimeRange:  24 * time.Hour,
		}, now, opsScheduledReportContent{overview: &OpsDashboardOverview{
			StartTime:         now.Add(-24 * time.Hour),
			EndTime:           now,
			RequestCountTotal: 1234,
			SuccessCount:      1200,
			ErrorCountSLA:     34,
			SLA:               0.9724,
		}})
	default:
		return nil
	}
}

// notificationWebhookMaxConcurrentDeliveries bounds in-flight webhook requests.
const notificationWebhookMaxConcurrentDeliveries = 32

// acquireWebhookSlot reserves one of the bounded delivery slots.
//
// The reservation is non-blocking on purpose. It runs on the path that produced
// the notification, so waiting here would push webhook latency into a request;
// and because it is taken before the delivery goroutine is spawned, it bounds
// goroutines and in-flight HTTP requests alike. Delivery is best-effort, so a
// saturated pool drops rather than queues.
func (s *NotificationEmailService) acquireWebhookSlot() (release func(), ok bool) {
	if s == nil || s.webhookSlots == nil {
		return func() {}, true
	}
	select {
	case s.webhookSlots <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-s.webhookSlots }) }, true
	default:
		return nil, false
	}
}
