package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"
)

// ConnectionRiskSettings is the admin-tunable policy for abnormal connection
// detection (方案 B). Hot path must only read this via
// GetConnectionRiskSettingsCached — never settingRepo.GetValue.
type ConnectionRiskSettings struct {
	// Enabled turns the feature on at the settings layer. Still gated by
	// YAML connection_risk.enabled (master kill).
	Enabled bool `json:"enabled"`
	// EmitEnabled controls hot-path Redis signal writes.
	EmitEnabled bool `json:"emit_enabled"`
	// WorkerEnabled controls the async scorer. Default true when feature is on.
	WorkerEnabled bool `json:"worker_enabled"`
	// IncludeReadOnlyEndpoints includes /models, billing introspection, etc.
	IncludeReadOnlyEndpoints bool `json:"include_read_only_endpoints"`
	// EmitSampleRateEvidence is the probability of writing evidence-only
	// ipset/uaset members (Tier B). Always-on Tier A is never sampled.
	EmitSampleRateEvidence float64 `json:"emit_sample_rate_evidence"`
	// R7IncludeAdminActors counts admin JWT session-binding mismatches.
	R7IncludeAdminActors bool `json:"r7_include_admin_actors"`
	// MaxActiveMembers hard-caps cr:active:* ZCARD (default 50000).
	MaxActiveMembers int `json:"max_active_members"`
	// ActivePruneEveryNEmits runs ZREMRANGEBYSCORE on active sets every N emits.
	ActivePruneEveryNEmits int `json:"active_prune_every_n_emits"`
	// WorkerIntervalSeconds is the scorer tick interval (60–300, default 120).
	WorkerIntervalSeconds int `json:"worker_interval_seconds"`
	// Phase is an operator label: observe | soft_throttle | auto_disable.
	Phase string `json:"phase"`
	// NotifyEmail enables optional email on high+ events (MVP optional).
	NotifyEmail bool `json:"notify_email"`
	// MinNotifySeverity is the minimum severity that triggers notify (default high).
	MinNotifySeverity string `json:"min_notify_severity"`
	// Rules holds per-rule overrides (enabled + thresholds).
	Rules ConnectionRiskRuleSettings `json:"rules"`
	// Actions holds Phase B/C action flags (default all off).
	Actions ConnectionRiskActionSettings `json:"actions"`
	// RetentionDays for connection_risk_events (default 120).
	RetentionDays int `json:"retention_days"`
	// ExemptUserIDs skips scoring for listed users.
	ExemptUserIDs []int64 `json:"exempt_user_ids"`
	// ExemptAPIKeyIDs skips scoring for listed API keys.
	ExemptAPIKeyIDs []int64 `json:"exempt_api_key_ids"`
}

// ConnectionRiskRuleSettings per-rule knobs. Zero/empty fields fall back to defaults at score time.
type ConnectionRiskRuleSettings struct {
	R1    ConnectionRiskRuleR1    `json:"R1"`
	R2    ConnectionRiskRuleR2    `json:"R2"`
	R3Abs ConnectionRiskRuleR3Abs `json:"R3_abs"`
	R3    ConnectionRiskRuleToggle `json:"R3"` // Phase B only; Phase A always disabled
	R4    ConnectionRiskRuleR4    `json:"R4"`
	R5    ConnectionRiskRuleR5    `json:"R5"`
	R6    ConnectionRiskRuleR6    `json:"R6"`
	R7    ConnectionRiskRuleR7    `json:"R7"`
}

// ConnectionRiskRuleToggle is a minimal enabled flag.
type ConnectionRiskRuleToggle struct {
	Enabled *bool `json:"enabled,omitempty"`
}

type ConnectionRiskRuleR1 struct {
	Enabled      *bool `json:"enabled,omitempty"`
	DistinctIP5m int   `json:"distinct_ip_5m,omitempty"` // default 8
	ReqCount5m   int   `json:"req_count_5m,omitempty"`   // default 20
}

type ConnectionRiskRuleR2 struct {
	Enabled    *bool `json:"enabled,omitempty"`
	UACount1h  int   `json:"ua_count_1h,omitempty"` // default 6
}

type ConnectionRiskRuleR3Abs struct {
	Enabled *bool `json:"enabled,omitempty"`
	HLL24h  int   `json:"hll_24h,omitempty"` // default 40
	HLL1h   int   `json:"hll_1h,omitempty"`  // default 15
}

type ConnectionRiskRuleR4 struct {
	Enabled    *bool `json:"enabled,omitempty"`
	Keys1h     int   `json:"keys_1h,omitempty"`      // default 3
	UserIP1h   int   `json:"user_ip_1h,omitempty"`   // default 15
}

type ConnectionRiskRuleR5 struct {
	Enabled           *bool   `json:"enabled,omitempty"`
	ConcurrencyRatio  float64 `json:"concurrency_ratio,omitempty"` // default 0.9
	DistinctIP5m      int     `json:"distinct_ip_5m,omitempty"`    // default 5
}

type ConnectionRiskRuleR6 struct {
	Enabled    *bool `json:"enabled,omitempty"`
	RPMAbs     int   `json:"rpm_abs,omitempty"`      // default 120
	DistinctIP int   `json:"distinct_ip,omitempty"`  // default 3 (current minute)
}

type ConnectionRiskRuleR7 struct {
	Enabled        *bool `json:"enabled,omitempty"`
	Mismatch15m    int   `json:"mismatch_15m,omitempty"` // default 1
	DistinctIP5m   int   `json:"distinct_ip_5m,omitempty"` // default 3
}

// ConnectionRiskActionSettings Phase B/C action flags.
type ConnectionRiskActionSettings struct {
	SoftThrottleEnabled       bool    `json:"soft_throttle_enabled"`
	ThrottleAbsRPM            int     `json:"throttle_abs_rpm"`
	ThrottleConcurrencyFactor float64 `json:"throttle_concurrency_factor"`
	AutoDisableEnabled        bool    `json:"auto_disable_enabled"`
}

const (
	connectionRiskCacheTTL  = 60 * time.Second
	connectionRiskErrorTTL  = 5 * time.Second
	connectionRiskDBTimeout = 5 * time.Second

	connectionRiskPhaseObserve      = "observe"
	connectionRiskPhaseSoftThrottle = "soft_throttle"
	connectionRiskPhaseAutoDisable  = "auto_disable"

	connectionRiskSeverityLow      = "low"
	connectionRiskSeverityMedium   = "medium"
	connectionRiskSeverityHigh     = "high"
	connectionRiskSeverityCritical = "critical"
)

// cachedConnectionRiskSettings is the in-process cache entry.
type cachedConnectionRiskSettings struct {
	settings  ConnectionRiskSettings
	expiresAt int64
}

// DefaultConnectionRiskSettings returns safe defaults: feature off, emit off,
// worker ready, evidence sample 0.1, R7 includes admin actors.
func DefaultConnectionRiskSettings() *ConnectionRiskSettings {
	return &ConnectionRiskSettings{
		Enabled:                  false,
		EmitEnabled:              false,
		WorkerEnabled:            true,
		IncludeReadOnlyEndpoints: true,
		EmitSampleRateEvidence:   0.1,
		R7IncludeAdminActors:     true,
		MaxActiveMembers:         50000,
		ActivePruneEveryNEmits:   32,
		WorkerIntervalSeconds:    120,
		Phase:                    connectionRiskPhaseObserve,
		NotifyEmail:              false,
		MinNotifySeverity:        connectionRiskSeverityHigh,
		Rules: ConnectionRiskRuleSettings{
			R1:    ConnectionRiskRuleR1{DistinctIP5m: 8, ReqCount5m: 20},
			R2:    ConnectionRiskRuleR2{UACount1h: 6},
			R3Abs: ConnectionRiskRuleR3Abs{HLL24h: 40, HLL1h: 15},
			R3:    ConnectionRiskRuleToggle{}, // Phase A disabled via scorer
			R4:    ConnectionRiskRuleR4{Keys1h: 3, UserIP1h: 15},
			R5:    ConnectionRiskRuleR5{ConcurrencyRatio: 0.9, DistinctIP5m: 5},
			R6:    ConnectionRiskRuleR6{RPMAbs: 120, DistinctIP: 3},
			R7:    ConnectionRiskRuleR7{Mismatch15m: 1, DistinctIP5m: 3},
		},
		Actions: ConnectionRiskActionSettings{
			SoftThrottleEnabled:       false,
			ThrottleAbsRPM:            30,
			ThrottleConcurrencyFactor: 0.5,
			AutoDisableEnabled:        false,
		},
		RetentionDays:   120,
		ExemptUserIDs:   []int64{},
		ExemptAPIKeyIDs: []int64{},
	}
}

// EffectiveEmitEnabled applies flag precedence:
// YAML connection_risk.enabled → settings.Enabled → settings.EmitEnabled.
func (s ConnectionRiskSettings) EffectiveEmitEnabled(yamlMasterEnabled bool) bool {
	return yamlMasterEnabled && s.Enabled && s.EmitEnabled
}

// EffectiveWorkerEnabled applies flag precedence for the scorer.
func (s ConnectionRiskSettings) EffectiveWorkerEnabled(yamlMasterEnabled bool) bool {
	return yamlMasterEnabled && s.Enabled && s.WorkerEnabled
}

// normalizeConnectionRiskSettings clamps illegal values in-place.
func normalizeConnectionRiskSettings(s *ConnectionRiskSettings) {
	if s == nil {
		return
	}
	if s.EmitSampleRateEvidence < 0 {
		s.EmitSampleRateEvidence = 0
	}
	if s.EmitSampleRateEvidence > 1 {
		s.EmitSampleRateEvidence = 1
	}
	// Avoid NaN/Inf from bad JSON.
	if math.IsNaN(s.EmitSampleRateEvidence) || math.IsInf(s.EmitSampleRateEvidence, 0) {
		s.EmitSampleRateEvidence = 0.1
	}
	if s.MaxActiveMembers <= 0 {
		s.MaxActiveMembers = 50000
	}
	if s.MaxActiveMembers > 500000 {
		s.MaxActiveMembers = 500000
	}
	if s.ActivePruneEveryNEmits <= 0 {
		s.ActivePruneEveryNEmits = 32
	}
	if s.ActivePruneEveryNEmits > 10000 {
		s.ActivePruneEveryNEmits = 10000
	}
	if s.WorkerIntervalSeconds < 60 {
		s.WorkerIntervalSeconds = 60
	}
	if s.WorkerIntervalSeconds > 300 {
		s.WorkerIntervalSeconds = 300
	}
	switch strings.ToLower(strings.TrimSpace(s.Phase)) {
	case connectionRiskPhaseSoftThrottle, connectionRiskPhaseAutoDisable:
		s.Phase = strings.ToLower(strings.TrimSpace(s.Phase))
	default:
		s.Phase = connectionRiskPhaseObserve
	}
	switch strings.ToLower(strings.TrimSpace(s.MinNotifySeverity)) {
	case connectionRiskSeverityLow, connectionRiskSeverityMedium, connectionRiskSeverityHigh, connectionRiskSeverityCritical:
		s.MinNotifySeverity = strings.ToLower(strings.TrimSpace(s.MinNotifySeverity))
	default:
		s.MinNotifySeverity = connectionRiskSeverityHigh
	}
	if s.RetentionDays <= 0 {
		s.RetentionDays = 120
	}
	if s.RetentionDays > 3650 {
		s.RetentionDays = 3650
	}
	if s.Actions.ThrottleAbsRPM < 0 {
		s.Actions.ThrottleAbsRPM = 0
	}
	if s.Actions.ThrottleAbsRPM > 100000 {
		s.Actions.ThrottleAbsRPM = 100000
	}
	if s.Actions.ThrottleConcurrencyFactor < 0 {
		s.Actions.ThrottleConcurrencyFactor = 0
	}
	if s.Actions.ThrottleConcurrencyFactor > 1 {
		s.Actions.ThrottleConcurrencyFactor = 1
	}
	if s.ExemptUserIDs == nil {
		s.ExemptUserIDs = []int64{}
	}
	if s.ExemptAPIKeyIDs == nil {
		s.ExemptAPIKeyIDs = []int64{}
	}
	if s.Actions.ThrottleAbsRPM <= 0 {
		s.Actions.ThrottleAbsRPM = 30
	}
	if s.Actions.ThrottleConcurrencyFactor <= 0 {
		s.Actions.ThrottleConcurrencyFactor = 0.5
	}
	normalizeRuleThresholds(s)
}

func normalizeRuleThresholds(s *ConnectionRiskSettings) {
	if s.Rules.R1.DistinctIP5m <= 0 {
		s.Rules.R1.DistinctIP5m = 8
	}
	if s.Rules.R1.ReqCount5m <= 0 {
		s.Rules.R1.ReqCount5m = 20
	}
	if s.Rules.R2.UACount1h <= 0 {
		s.Rules.R2.UACount1h = 6
	}
	if s.Rules.R3Abs.HLL24h <= 0 {
		s.Rules.R3Abs.HLL24h = 40
	}
	if s.Rules.R3Abs.HLL1h <= 0 {
		s.Rules.R3Abs.HLL1h = 15
	}
	if s.Rules.R4.Keys1h <= 0 {
		s.Rules.R4.Keys1h = 3
	}
	if s.Rules.R4.UserIP1h <= 0 {
		s.Rules.R4.UserIP1h = 15
	}
	if s.Rules.R5.ConcurrencyRatio <= 0 || s.Rules.R5.ConcurrencyRatio > 1 {
		s.Rules.R5.ConcurrencyRatio = 0.9
	}
	if s.Rules.R5.DistinctIP5m <= 0 {
		s.Rules.R5.DistinctIP5m = 5
	}
	if s.Rules.R6.RPMAbs <= 0 {
		s.Rules.R6.RPMAbs = 120
	}
	if s.Rules.R6.DistinctIP <= 0 {
		s.Rules.R6.DistinctIP = 3
	}
	if s.Rules.R7.Mismatch15m <= 0 {
		s.Rules.R7.Mismatch15m = 1
	}
	if s.Rules.R7.DistinctIP5m <= 0 {
		s.Rules.R7.DistinctIP5m = 3
	}
}

// GetConnectionRiskSettings loads settings from DB (admin path). Missing/invalid → defaults.
func (s *SettingService) GetConnectionRiskSettings(ctx context.Context) (*ConnectionRiskSettings, error) {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyConnectionRiskSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return DefaultConnectionRiskSettings(), nil
		}
		return nil, fmt.Errorf("get connection risk settings: %w", err)
	}
	if strings.TrimSpace(value) == "" {
		return DefaultConnectionRiskSettings(), nil
	}
	settings := DefaultConnectionRiskSettings()
	if err := json.Unmarshal([]byte(value), settings); err != nil {
		slog.Warn("failed to unmarshal connection risk settings, falling back to defaults",
			"error", err, "key", SettingKeyConnectionRiskSettings)
		return DefaultConnectionRiskSettings(), nil
	}
	normalizeConnectionRiskSettings(settings)
	return settings, nil
}

// SetConnectionRiskSettings persists settings and refreshes the process cache.
func (s *SettingService) SetConnectionRiskSettings(ctx context.Context, settings *ConnectionRiskSettings) error {
	if settings == nil {
		return fmt.Errorf("settings cannot be nil")
	}
	normalizeConnectionRiskSettings(settings)
	if settings.EmitSampleRateEvidence < 0 || settings.EmitSampleRateEvidence > 1 {
		return fmt.Errorf("emit_sample_rate_evidence must be between 0 and 1")
	}
	if settings.WorkerIntervalSeconds < 60 || settings.WorkerIntervalSeconds > 300 {
		return fmt.Errorf("worker_interval_seconds must be between 60 and 300")
	}
	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal connection risk settings: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyConnectionRiskSettings, string(data)); err != nil {
		return err
	}
	s.storeConnectionRiskCache(*settings, connectionRiskCacheTTL)
	return nil
}

// GetConnectionRiskSettingsCached returns settings with a 60s in-process cache.
// Hot path only: never blocks on DB after a warm cache hit. DB errors return
// last-known or defaults with a short error TTL.
func (s *SettingService) GetConnectionRiskSettingsCached(ctx context.Context) ConnectionRiskSettings {
	if s == nil || s.settingRepo == nil {
		return *DefaultConnectionRiskSettings()
	}
	if cached, ok := s.connectionRiskCache.Load().(*cachedConnectionRiskSettings); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.settings
		}
	}

	result, _, _ := s.connectionRiskSF.Do("connection_risk_settings", func() (any, error) {
		if cached, ok := s.connectionRiskCache.Load().(*cachedConnectionRiskSettings); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cached.settings, nil
			}
		}
		if ctx == nil {
			ctx = context.Background()
		}
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), connectionRiskDBTimeout)
		defer cancel()

		settings, err := s.GetConnectionRiskSettings(dbCtx)
		if err != nil {
			slog.Warn("failed to get connection risk settings", "error", err)
			fallback := *DefaultConnectionRiskSettings()
			if prior, ok := s.connectionRiskCache.Load().(*cachedConnectionRiskSettings); ok && prior != nil {
				fallback = prior.settings
			}
			s.storeConnectionRiskCache(fallback, connectionRiskErrorTTL)
			return fallback, nil
		}
		s.storeConnectionRiskCache(*settings, connectionRiskCacheTTL)
		return *settings, nil
	})
	if settings, ok := result.(ConnectionRiskSettings); ok {
		return settings
	}
	return *DefaultConnectionRiskSettings()
}

func (s *SettingService) storeConnectionRiskCache(settings ConnectionRiskSettings, ttl time.Duration) {
	s.connectionRiskCache.Store(&cachedConnectionRiskSettings{
		settings:  settings,
		expiresAt: time.Now().Add(ttl).UnixNano(),
	})
}
