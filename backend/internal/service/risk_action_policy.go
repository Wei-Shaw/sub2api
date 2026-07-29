package service

import (
	"context"
	"log/slog"
	"time"
)

// RiskActionPolicy applies notify / soft-throttle / auto-disable for risk events.
type RiskActionPolicy struct {
	signals ConnectionSignalCache
	apiKeys *APIKeyService
	users   UserRepository
	authInv interface {
		InvalidateAuthCacheByUserID(ctx context.Context, userID int64)
	}
}

// NewRiskActionPolicy constructs the policy engine (Phase A notify-only by default).
func NewRiskActionPolicy() *RiskActionPolicy {
	return &RiskActionPolicy{}
}

// ProvideRiskActionPolicy wires optional dependencies for Phase B/C actions.
func ProvideRiskActionPolicy(
	signals ConnectionSignalCache,
	apiKeys *APIKeyService,
	users UserRepository,
) *RiskActionPolicy {
	p := &RiskActionPolicy{
		signals: signals,
		apiKeys: apiKeys,
		users:   users,
	}
	if apiKeys != nil {
		p.authInv = apiKeys
	}
	return p
}

// HandleNewEvent applies configured actions for a newly opened/updated event.
func (p *RiskActionPolicy) HandleNewEvent(ctx context.Context, event *ConnectionRiskEvent, settings ConnectionRiskSettings) {
	if p == nil || event == nil {
		return
	}
	// Phase A: no automatic mutation when action flags are off (defaults).
	if settings.Actions.SoftThrottleEnabled && p.signals != nil && event.APIKeyID != nil {
		capRPM := settings.Actions.ThrottleAbsRPM
		if capRPM <= 0 {
			capRPM = 30
		}
		until := time.Now().Add(30 * time.Minute).Unix()
		if err := p.signals.SetThrottle(ctx, *event.APIKeyID, capRPM, until); err != nil {
			slog.Warn("connection risk soft throttle failed", "error", err, "key_id", *event.APIKeyID)
		} else {
			event.ActionTaken = "throttled"
		}
	}

	if settings.Actions.AutoDisableEnabled && severityAtLeast(event.Severity, connectionRiskSeverityCritical) {
		p.autoDisable(ctx, event)
	}
}

func (p *RiskActionPolicy) autoDisable(ctx context.Context, event *ConnectionRiskEvent) {
	if p.apiKeys != nil && event.APIKeyID != nil {
		status := StatusAPIKeyDisabled
		if _, err := p.apiKeys.AdminUpdate(ctx, *event.APIKeyID, AdminUpdateAPIKeyRequest{Status: &status}); err != nil {
			slog.Warn("connection risk auto-disable key failed", "error", err, "key_id", *event.APIKeyID)
		} else {
			event.ActionTaken = "disabled_key"
		}
	}
	// Never auto-disable admin users; only disable non-admin when user-scoped critical.
	if p.users != nil && event.UserID != nil && event.SubjectType == ConnectionRiskSubjectUser {
		u, err := p.users.GetByID(ctx, *event.UserID)
		if err != nil || u == nil || u.IsAdmin() {
			return
		}
		// Soft path: mark disabled via repo if available through a minimal update interface.
		// UserRepository does not expose UpdateStatus directly in all impls; use type assert.
		type statusUpdater interface {
			UpdateStatus(ctx context.Context, id int64, status string) error
		}
		if su, ok := any(p.users).(statusUpdater); ok {
			if err := su.UpdateStatus(ctx, *event.UserID, StatusDisabled); err != nil {
				slog.Warn("connection risk auto-disable user failed", "error", err, "user_id", *event.UserID)
				return
			}
			if p.authInv != nil {
				p.authInv.InvalidateAuthCacheByUserID(ctx, *event.UserID)
			}
			event.ActionTaken = "disabled_user"
		}
	}
}

func severityAtLeast(have, need string) bool {
	rank := map[string]int{
		connectionRiskSeverityLow:      1,
		connectionRiskSeverityMedium:   2,
		connectionRiskSeverityHigh:     3,
		connectionRiskSeverityCritical: 4,
	}
	return rank[have] >= rank[need]
}

// ApplyWhitelistIPs merges sample IPs into the key whitelist via AdminUpdate.
func (p *RiskActionPolicy) ApplyWhitelistIPs(ctx context.Context, keyID int64, ips []string) (*APIKey, error) {
	if p == nil || p.apiKeys == nil {
		return nil, ErrInsufficientPerms
	}
	key, err := p.apiKeys.GetByID(ctx, keyID)
	if err != nil {
		return nil, err
	}
	merged := append([]string{}, key.IPWhitelist...)
	seen := map[string]struct{}{}
	for _, ip := range merged {
		seen[ip] = struct{}{}
	}
	for _, ip := range ips {
		if ip == "" {
			continue
		}
		if _, ok := seen[ip]; ok {
			continue
		}
		merged = append(merged, ip)
		seen[ip] = struct{}{}
	}
	return p.apiKeys.AdminUpdate(ctx, keyID, AdminUpdateAPIKeyRequest{IPWhitelist: &merged})
}
