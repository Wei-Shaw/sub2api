package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

// Notification delivery channels.
//
// The notification pipeline (see NotificationEmailService.Send) resolves a
// per-event channel matrix before delivering. Email is the historical default
// and stays enabled unless an admin turns it off; webhook is opt-in and lets
// operators forward notifications into their own tooling.
const (
	NotificationChannelEmail   = "email"
	NotificationChannelWebhook = "webhook"
)

// Audience tells a webhook consumer whether the notification is addressed to an
// end user or to an operator, so a consumer can route it to the right target.
const (
	NotificationAudienceUser  = "user"
	NotificationAudienceAdmin = "admin"
)

const notificationChannelConfigKey = "notification_channel_config"

// Bounds for webhook delivery. Kept deliberately tight: delivery is best-effort
// and must never hold up the request path that produced the notification.
const (
	notificationWebhookDefaultTimeoutSeconds = 5
	notificationWebhookMaxTimeoutSeconds     = 30
	notificationWebhookDefaultMaxRetries     = 2
	notificationWebhookMaxRetries            = 5
	notificationWebhookMaxBodyTemplateLength = 20000
)

// NotificationWebhookEndpoint is where a notification is POSTed.
// An empty BodyTemplate means the default structured JSON payload is sent.
type NotificationWebhookEndpoint struct {
	URL          string `json:"url"`
	BodyTemplate string `json:"body_template,omitempty"`
}

// NotificationWebhookGlobalConfig holds the default endpoint plus transport
// knobs shared by every event.
type NotificationWebhookGlobalConfig struct {
	Enabled  bool                        `json:"enabled"`
	Endpoint NotificationWebhookEndpoint `json:"endpoint"`

	// Secret signs every delivery and is shared with the receiver, so unlike an
	// SMTP password it has to be readable by the operator. It is generated
	// automatically when webhook delivery is enabled without one.
	Secret string `json:"secret,omitempty"`

	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
	// MaxRetries is a pointer so an explicit 0 ("never retry") is
	// distinguishable from "not configured"; a plain int would silently
	// restore the default and keep retrying.
	MaxRetries *int `json:"max_retries,omitempty"`
}

// NotificationEventChannelConfig is the per-event override. Nil pointers mean
// "inherit the default", which keeps stored config small and makes added events
// behave sensibly without a migration.
type NotificationEventChannelConfig struct {
	Email    *bool                        `json:"email,omitempty"`
	Webhook  *bool                        `json:"webhook,omitempty"`
	Endpoint *NotificationWebhookEndpoint `json:"endpoint,omitempty"`
}

// NotificationChannelConfig is the whole stored blob.
type NotificationChannelConfig struct {
	Webhook NotificationWebhookGlobalConfig           `json:"webhook"`
	Events  map[string]NotificationEventChannelConfig `json:"events,omitempty"`
}

// ResolvedNotificationChannels is the effective decision for one event.
type ResolvedNotificationChannels struct {
	Email   bool
	Webhook bool

	Endpoint       NotificationWebhookEndpoint
	Secret         string
	TimeoutSeconds int
	MaxRetries     int
}

// notificationAudienceForCategory maps the existing event categories onto the
// user/admin split. Categories come from notificationEmailEventDefinitions.
func notificationAudienceForCategory(category string) string {
	switch strings.TrimSpace(category) {
	case "admin", "ops":
		return NotificationAudienceAdmin
	default:
		return NotificationAudienceUser
	}
}

// GetChannelConfig loads the stored channel configuration, falling back to
// defaults (email on, webhook off) when unset or unparseable.
func (s *NotificationEmailService) GetChannelConfig(ctx context.Context) (NotificationChannelConfig, error) {
	cfg := NotificationChannelConfig{}
	if s == nil || s.settingRepo == nil {
		return cfg, nil
	}
	raw, err := s.settingRepo.GetValue(ctx, notificationChannelConfigKey)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return cfg, nil
		}
		return cfg, err
	}
	if strings.TrimSpace(raw) == "" {
		return cfg, nil
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		// A malformed blob must not silently disable email notifications.
		return NotificationChannelConfig{}, nil
	}
	return cfg, nil
}

// SaveChannelConfig validates and persists the channel configuration.
func (s *NotificationEmailService) SaveChannelConfig(ctx context.Context, cfg NotificationChannelConfig) error {
	if s == nil || s.settingRepo == nil {
		return errors.New("setting repository is not configured")
	}
	// Normalize before anything else: signing must use exactly the bytes the
	// operator copies out of the API, so surrounding whitespace cannot survive
	// on one side only.
	cfg.Webhook.Secret = strings.TrimSpace(cfg.Webhook.Secret)
	if cfg.Webhook.Enabled && cfg.Webhook.Secret == "" {
		secret, err := generateNotificationWebhookSecret()
		if err != nil {
			return err
		}
		cfg.Webhook.Secret = secret
	}
	if err := s.validateChannelConfig(cfg); err != nil {
		return err
	}
	encoded, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return s.settingRepo.Set(ctx, notificationChannelConfigKey, string(encoded))
}

func (s *NotificationEmailService) validateChannelConfig(cfg NotificationChannelConfig) error {
	if cfg.Webhook.Enabled {
		if err := validateNotificationWebhookEndpoint(cfg.Webhook.Endpoint, true); err != nil {
			return err
		}
		// Unsigned deliveries would let anyone who can reach the endpoint forge a
		// notification, so a secret is not optional. SaveChannelConfig
		// generates one when the operator has not supplied it.
		if strings.TrimSpace(cfg.Webhook.Secret) == "" {
			return errors.New("webhook signing secret is required when webhook delivery is enabled")
		}
	}
	if cfg.Webhook.TimeoutSeconds < 0 || cfg.Webhook.TimeoutSeconds > notificationWebhookMaxTimeoutSeconds {
		return errors.New("webhook timeout_seconds out of range")
	}
	if cfg.Webhook.MaxRetries != nil && (*cfg.Webhook.MaxRetries < 0 || *cfg.Webhook.MaxRetries > notificationWebhookMaxRetries) {
		return errors.New("webhook max_retries out of range")
	}
	for event, eventCfg := range cfg.Events {
		if _, _, err := s.eventInfo(event); err != nil {
			return err
		}
		if eventCfg.Endpoint != nil {
			if err := validateNotificationWebhookEndpoint(*eventCfg.Endpoint, false); err != nil {
				return err
			}
		}
		if eventCfg.Endpoint != nil && strings.TrimSpace(eventCfg.Endpoint.BodyTemplate) != "" {
			if err := s.validateWebhookBodyTemplate(event, eventCfg.Endpoint.BodyTemplate); err != nil {
				return err
			}
		}
	}
	if strings.TrimSpace(cfg.Webhook.Endpoint.BodyTemplate) != "" {
		// The global template must render for every event that can use it.
		for _, event := range notificationEmailEventOrder {
			if err := s.validateWebhookBodyTemplate(event, cfg.Webhook.Endpoint.BodyTemplate); err != nil {
				return err
			}
		}
	}
	return nil
}

// defaultResolvedNotificationChannels is the behaviour before any admin
// configuration: email on, webhook off. Keeping it here means a fresh install
// and an unparseable config land in the same known-good state.
func defaultResolvedNotificationChannels() ResolvedNotificationChannels {
	return ResolvedNotificationChannels{
		Email:          true,
		Webhook:        false,
		TimeoutSeconds: notificationWebhookDefaultTimeoutSeconds,
		MaxRetries:     notificationWebhookDefaultMaxRetries,
	}
}

// ResolveChannels computes the effective channel decision for one event.
func (s *NotificationEmailService) ResolveChannels(ctx context.Context, event string) ResolvedNotificationChannels {
	resolved := defaultResolvedNotificationChannels()
	cfg, err := s.GetChannelConfig(ctx)
	if err != nil {
		return resolved
	}
	return applyNotificationChannelConfig(resolved, cfg, event)
}

func applyNotificationChannelConfig(resolved ResolvedNotificationChannels, cfg NotificationChannelConfig, event string) ResolvedNotificationChannels {
	resolved.Endpoint = cfg.Webhook.Endpoint
	resolved.Secret = cfg.Webhook.Secret
	if cfg.Webhook.TimeoutSeconds > 0 {
		resolved.TimeoutSeconds = cfg.Webhook.TimeoutSeconds
	}
	if cfg.Webhook.MaxRetries != nil {
		resolved.MaxRetries = *cfg.Webhook.MaxRetries
	}
	eventCfg, ok := cfg.Events[event]
	if ok {
		if eventCfg.Email != nil {
			resolved.Email = *eventCfg.Email
		}
		if eventCfg.Webhook != nil {
			resolved.Webhook = *eventCfg.Webhook
		}
		if eventCfg.Endpoint != nil {
			resolved.Endpoint = mergeNotificationWebhookEndpoint(resolved.Endpoint, *eventCfg.Endpoint)
		}
	}

	// The global switch is a hard gate: turning it off silences every webhook
	// without having to unset each event.
	if !cfg.Webhook.Enabled {
		resolved.Webhook = false
	}
	if strings.TrimSpace(resolved.Endpoint.URL) == "" {
		resolved.Webhook = false
	}
	// Every delivery is signed, so a configuration without a secret must not
	// deliver at all. This also fails safe for stored JSON written before the
	// secret moved off the endpoint: the old field is no longer read, and
	// silently shipping requests signed with an empty key would be worse than
	// not delivering.
	if strings.TrimSpace(resolved.Secret) == "" {
		resolved.Webhook = false
	}
	return resolved
}

// mergeNotificationWebhookEndpoint layers a per-event override on top of the
// global endpoint so an event can redirect to its own receiver or emit its own
// body while inheriting everything else.
func mergeNotificationWebhookEndpoint(base, override NotificationWebhookEndpoint) NotificationWebhookEndpoint {
	merged := base
	if strings.TrimSpace(override.URL) != "" {
		merged.URL = override.URL
	}
	if strings.TrimSpace(override.BodyTemplate) != "" {
		merged.BodyTemplate = override.BodyTemplate
	}
	return merged
}

func validateNotificationWebhookEndpoint(endpoint NotificationWebhookEndpoint, requireURL bool) error {
	trimmedURL := strings.TrimSpace(endpoint.URL)
	if trimmedURL == "" {
		if requireURL {
			return errors.New("webhook url is required when webhook delivery is enabled")
		}
		return nil
	}
	if err := validateNotificationWebhookURL(trimmedURL); err != nil {
		return err
	}
	if len(endpoint.BodyTemplate) > notificationWebhookMaxBodyTemplateLength {
		return errors.New("webhook body template is too long")
	}
	return nil
}

// validateWebhookBodyTemplate ensures a custom template only references
// placeholders the event actually provides, and that it renders to valid JSON
// with representative values.
func (s *NotificationEmailService) validateWebhookBodyTemplate(event, template string) error {
	_, normalizedEvent, err := s.eventInfo(event)
	if err != nil {
		return err
	}
	allowed := notificationWebhookAllowedPlaceholderSet(normalizedEvent)
	for _, placeholder := range notificationEmailPlaceholdersIn(template) {
		if _, ok := allowed[placeholder]; !ok {
			return errors.New("unsupported placeholder {{" + placeholder + "}} for event " + normalizedEvent)
		}
	}
	sample := notificationEmailSampleVariables(notificationEmailDefaultLocale)
	addNotificationEmailOpsSummarySampleVariables(sample)
	for _, placeholder := range allowed2slice(allowed) {
		if _, ok := sample[placeholder]; !ok {
			sample[placeholder] = "sample"
		}
	}
	rendered, err := renderNotificationTemplateString(allowed, template, sample, nil, notificationEscapeJSON, nil)
	if err != nil {
		return err
	}
	if !json.Valid([]byte(rendered)) {
		return errors.New("webhook body template must render to valid JSON")
	}
	return nil
}

func allowed2slice(allowed map[string]struct{}) []string {
	out := make([]string, 0, len(allowed))
	for name := range allowed {
		out = append(out, name)
	}
	return out
}

// notificationWebhookAllowedPlaceholderSet keeps deprecated names renderable so
// saved templates do not turn into silent delivery failures after the raw event
// contract removed sender-rendered content.
func notificationWebhookAllowedPlaceholderSet(event string) map[string]struct{} {
	allowed := notificationEmailAllowedPlaceholderSet(event)
	for _, extra := range notificationWebhookInternalPlaceholders {
		allowed[extra] = struct{}{}
	}
	return allowed
}

var notificationWebhookAdvertisedPlaceholders = []string{
	"event",
	"event_label",
	"event_category",
	"audience",
	"locale",
	"user_id",
	"source_type",
	"source_id",
	"occurred_at",
	"timestamp",
}

var notificationWebhookInternalPlaceholders = append(
	append([]string{}, notificationWebhookAdvertisedPlaceholders...),
	"rendered_title", // Deprecated: always renders as an empty string.
	"rendered_text",  // Deprecated: always renders as an empty string.
)

// NotificationWebhookExtraPlaceholders exposes the webhook-only placeholders so
// the admin UI can show what a custom body template may reference.
func NotificationWebhookExtraPlaceholders() []string {
	return append([]string{}, notificationWebhookAdvertisedPlaceholders...)
}

// NotificationAudienceForEvent reports whether an event addresses an end user
// or an operator.
func NotificationAudienceForEvent(info NotificationEmailEventInfo) string {
	return notificationAudienceForCategory(info.Category)
}

// NotificationWebhookDefaultMaxRetries is the retry count applied when an
// operator has not configured one.
func NotificationWebhookDefaultMaxRetries() int {
	return notificationWebhookDefaultMaxRetries
}

// generateNotificationWebhookSecret produces a signing secret for a new webhook
// configuration. It is surfaced to the admin UI (unlike an SMTP password)
// because the receiver needs the same value to verify signatures.
func generateNotificationWebhookSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
