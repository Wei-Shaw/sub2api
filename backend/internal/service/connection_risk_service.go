package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// ConnectionRiskService is the admin-facing facade for risk events and config.
type ConnectionRiskService struct {
	settings *SettingService
	events   ConnectionRiskEventRepository
	signals  ConnectionSignalCache
	emitter  *ConnectionSignalEmitter
	policy   *RiskActionPolicy
	cfg      *config.Config
}

// NewConnectionRiskService constructs the admin service.
func NewConnectionRiskService(
	settings *SettingService,
	events ConnectionRiskEventRepository,
	signals ConnectionSignalCache,
	emitter *ConnectionSignalEmitter,
	policy *RiskActionPolicy,
	cfg *config.Config,
) *ConnectionRiskService {
	return &ConnectionRiskService{
		settings: settings,
		events:   events,
		signals:  signals,
		emitter:  emitter,
		policy:   policy,
		cfg:      cfg,
	}
}

func (s *ConnectionRiskService) yamlEnabled() bool {
	return s != nil && s.cfg != nil && s.cfg.ConnectionRisk.Enabled
}

// GetConfig returns settings JSON (no secrets).
func (s *ConnectionRiskService) GetConfig(ctx context.Context) (*ConnectionRiskSettings, error) {
	if s == nil || s.settings == nil {
		return DefaultConnectionRiskSettings(), nil
	}
	return s.settings.GetConnectionRiskSettings(ctx)
}

// UpdateConfig persists admin settings.
func (s *ConnectionRiskService) UpdateConfig(ctx context.Context, settings *ConnectionRiskSettings) error {
	if s == nil || s.settings == nil {
		return fmt.Errorf("settings service unavailable")
	}
	return s.settings.SetConnectionRiskSettings(ctx, settings)
}

// ListEvents returns a paginated event list.
func (s *ConnectionRiskService) ListEvents(ctx context.Context, filter *ConnectionRiskEventFilter) (*ConnectionRiskEventList, error) {
	if s == nil || s.events == nil {
		return &ConnectionRiskEventList{Items: []*ConnectionRiskEvent{}, Page: 1, PageSize: 20}, nil
	}
	return s.events.List(ctx, filter)
}

// GetEvent returns one event by id (nil if missing).
func (s *ConnectionRiskService) GetEvent(ctx context.Context, id int64) (*ConnectionRiskEvent, error) {
	if s == nil || s.events == nil {
		return nil, nil
	}
	return s.events.GetByID(ctx, id)
}

// AckEvent marks an event acknowledged.
func (s *ConnectionRiskService) AckEvent(ctx context.Context, id int64, resolverID *int64) error {
	if s == nil || s.events == nil {
		return fmt.Errorf("events repository unavailable")
	}
	return s.events.UpdateStatus(ctx, id, ConnectionRiskStatusAcknowledged, resolverID)
}

// ResolveEvent marks an event resolved and clears any active soft-throttle on the key.
func (s *ConnectionRiskService) ResolveEvent(ctx context.Context, id int64, resolverID *int64) error {
	if s == nil || s.events == nil {
		return fmt.Errorf("events repository unavailable")
	}
	ev, err := s.events.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.events.UpdateStatus(ctx, id, ConnectionRiskStatusResolved, resolverID); err != nil {
		return err
	}
	s.clearEventThrottle(ctx, ev)
	return nil
}

// SuppressEvent marks an event suppressed and clears soft-throttle for the subject key.
func (s *ConnectionRiskService) SuppressEvent(ctx context.Context, id int64, resolverID *int64) error {
	if s == nil || s.events == nil {
		return fmt.Errorf("events repository unavailable")
	}
	ev, err := s.events.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.events.UpdateStatus(ctx, id, ConnectionRiskStatusSuppressed, resolverID); err != nil {
		return err
	}
	s.clearEventThrottle(ctx, ev)
	return nil
}

func (s *ConnectionRiskService) clearEventThrottle(ctx context.Context, ev *ConnectionRiskEvent) {
	if s == nil || s.signals == nil || ev == nil || ev.APIKeyID == nil {
		return
	}
	_ = s.signals.ClearThrottle(ctx, *ev.APIKeyID)
}

// DeleteEvent permanently deletes one event row.
func (s *ConnectionRiskService) DeleteEvent(ctx context.Context, id int64) error {
	if s == nil || s.events == nil {
		return fmt.Errorf("events repository unavailable")
	}
	return s.events.Delete(ctx, id)
}

// ExemptSubject sets a Redis exemption (scope=k|u).
func (s *ConnectionRiskService) ExemptSubject(ctx context.Context, scope string, id int64, reason string, ttl time.Duration) error {
	if s == nil || s.signals == nil {
		return fmt.Errorf("signal cache unavailable")
	}
	if scope != "k" && scope != "u" {
		return fmt.Errorf("scope must be k or u")
	}
	if id <= 0 {
		return fmt.Errorf("id must be positive")
	}
	return s.signals.SetExempt(ctx, scope, id, reason, ttl)
}

// ClearExempt removes a Redis exemption.
func (s *ConnectionRiskService) ClearExempt(ctx context.Context, scope string, id int64) error {
	if s == nil || s.signals == nil {
		return fmt.Errorf("signal cache unavailable")
	}
	return s.signals.ClearExempt(ctx, scope, id)
}

// WhitelistIPs merges IPs onto the API key whitelist (Phase B) and clears throttle.
func (s *ConnectionRiskService) WhitelistIPs(ctx context.Context, keyID int64, ips []string) (*APIKey, error) {
	if s == nil {
		return nil, fmt.Errorf("service unavailable")
	}
	if s.policy == nil {
		return nil, fmt.Errorf("policy unavailable")
	}
	key, err := s.policy.ApplyWhitelistIPs(ctx, keyID, ips)
	if err != nil {
		return nil, err
	}
	if s.signals != nil {
		_ = s.signals.ClearThrottle(ctx, keyID)
		_ = s.signals.SetExempt(ctx, "k", keyID, "whitelist", 24*time.Hour)
	}
	return key, nil
}

// RunRetention deletes events older than settings.RetentionDays.
func (s *ConnectionRiskService) RunRetention(ctx context.Context) (int64, error) {
	if s == nil || s.events == nil {
		return 0, fmt.Errorf("events repository unavailable")
	}
	days := 120
	if s.settings != nil {
		st := s.settings.GetConnectionRiskSettingsCached(ctx)
		if st.RetentionDays > 0 {
			days = st.RetentionDays
		}
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	return s.events.DeleteOlderThan(ctx, cutoff)
}

// RuntimeSnapshot returns process metrics + active set sizes + master flags.
func (s *ConnectionRiskService) RuntimeSnapshot(ctx context.Context) map[string]any {
	yamlOn := s.yamlEnabled()
	out := map[string]any{
		"yaml_enabled": yamlOn,
		"settings":     DefaultConnectionRiskSettings(),
		"metrics":      ConnectionRiskMetricsSnapshot{},
	}
	if s != nil && s.settings != nil {
		st := s.settings.GetConnectionRiskSettingsCached(ctx)
		out["settings"] = st
		out["effective_emit"] = st.EffectiveEmitEnabled(yamlOn)
		out["effective_worker"] = st.EffectiveWorkerEnabled(yamlOn)
	}
	if s != nil && s.emitter != nil && s.emitter.Metrics() != nil {
		out["metrics"] = s.emitter.Metrics().Snapshot()
	}
	if s != nil && s.signals != nil {
		if k, u, err := s.signals.ActiveCards(ctx); err == nil {
			out["active_keys"] = k
			out["active_users"] = u
		}
	}
	return out
}
