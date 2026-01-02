package service

import (
	"context"
	"time"
)

const (
	OpsAlertStatusFiring   = "firing"
	OpsAlertStatusResolved = "resolved"
)

const (
	OpsMetricSuccessRate        = "success_rate"
	OpsMetricErrorRate          = "error_rate"
	OpsMetricP95LatencyMs       = "p95_latency_ms"
	OpsMetricP99LatencyMs       = "p99_latency_ms"
	OpsMetricHTTP2Errors        = "http2_errors"
	OpsMetricCPUUsagePercent    = "cpu_usage_percent"
	OpsMetricMemoryUsagePercent = "memory_usage_percent"
	OpsMetricQueueDepth         = "concurrency_queue_depth"
)

type OpsAlertRule struct {
	ID               int64          `json:"id"`
	Name             string         `json:"name"`
	Description      string         `json:"description"`
	Enabled          bool           `json:"enabled"`
	MetricType       string         `json:"metric_type"`
	Operator         string         `json:"operator"`
	Threshold        float64        `json:"threshold"`
	WindowMinutes    int            `json:"window_minutes"`
	SustainedMinutes int            `json:"sustained_minutes"`
	Severity         string         `json:"severity"`
	NotifyEmail      bool           `json:"notify_email"`
	NotifyWebhook    bool           `json:"notify_webhook"`
	WebhookURL       string         `json:"webhook_url"`
	CooldownMinutes  int            `json:"cooldown_minutes"`
	DimensionFilters map[string]any `json:"dimension_filters,omitempty"`
	NotifyChannels   []string       `json:"notify_channels,omitempty"`
	NotifyConfig     map[string]any `json:"notify_config,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
REDACTED

type OpsAlertEvent struct {
	ID             int64      `json:"id"`
	RuleID         int64      `json:"rule_id"`
	Severity       string     `json:"severity"`
	Status         string     `json:"status"`
	Title          string     `json:"title"`
	Description    string     `json:"description"`
	MetricValue    float64    `json:"metric_value"`
	ThresholdValue float64    `json:"threshold_value"`
	FiredAt        time.Time  `json:"fired_at"`
	ResolvedAt     *time.Time `json:"resolved_at"`
	EmailSent      bool       `json:"email_sent"`
	WebhookSent    bool       `json:"webhook_sent"`
	CreatedAt      time.Time  `json:"created_at"`
REDACTED

func (s *OpsService) ListAlertRules(ctx context.Context) ([]OpsAlertRule, error) {
	return s.repo.ListAlertRules(ctx)
REDACTED

func (s *OpsService) GetActiveAlertEvent(ctx context.Context, ruleID int64) (*OpsAlertEvent, error) {
	return s.repo.GetActiveAlertEvent(ctx, ruleID)
REDACTED

func (s *OpsService) GetLatestAlertEvent(ctx context.Context, ruleID int64) (*OpsAlertEvent, error) {
	return s.repo.GetLatestAlertEvent(ctx, ruleID)
REDACTED

func (s *OpsService) CreateAlertEvent(ctx context.Context, event *OpsAlertEvent) error {
	return s.repo.CreateAlertEvent(ctx, event)
REDACTED

func (s *OpsService) UpdateAlertEventStatus(ctx context.Context, eventID int64, status string, resolvedAt *time.Time) error {
	return s.repo.UpdateAlertEventStatus(ctx, eventID, status, resolvedAt)
REDACTED

func (s *OpsService) UpdateAlertEventNotifications(ctx context.Context, eventID int64, emailSent, webhookSent bool) error {
	return s.repo.UpdateAlertEventNotifications(ctx, eventID, emailSent, webhookSent)
REDACTED

func (s *OpsService) ListRecentSystemMetrics(ctx context.Context, windowMinutes, limit int) ([]OpsMetrics, error) {
	return s.repo.ListRecentSystemMetrics(ctx, windowMinutes, limit)
REDACTED

func (s *OpsService) CountActiveAlerts(ctx context.Context) (int, error) {
	return s.repo.CountActiveAlerts(ctx)
REDACTED
