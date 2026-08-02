package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// ProxyHealthSettings is the admin-tunable proxy pool health poller policy.
// Stored as JSON under SettingKeyProxyHealthSettings. When absent, YAML
// config.ProxyHealth is used as the baseline.
type ProxyHealthSettings struct {
	Enabled          bool     `json:"enabled"`
	IntervalSec      int      `json:"interval_sec"`
	TimeoutMS        int      `json:"timeout_ms"`
	Concurrency      int      `json:"concurrency"`
	FailThreshold    int      `json:"fail_threshold"`
	SuccessThreshold int      `json:"success_threshold"`
	ProbeScope       string   `json:"probe_scope"` // group_members | all_active
	AutoRecover      bool     `json:"auto_recover"`
	SkipNamePrefix   []string `json:"skip_name_prefix"`
	LeaderLockTTLSec int      `json:"leader_lock_ttl_sec"`
	BatchSize        int      `json:"batch_size"`
	ProbeMode        string   `json:"probe_mode"` // connectivity | quality
}

// ProxyHealthRuntime is the admin monitoring payload.
type ProxyHealthRuntime struct {
	Config           ProxyHealthSettings        `json:"config"`
	YAMLEnabled      bool                       `json:"yaml_enabled"`
	WorkerRunning    bool                       `json:"worker_running"`
	WorkerInstanceID string                     `json:"worker_instance_id,omitempty"`
	Metrics          ProxyHealthMetricsSnapshot `json:"metrics"`
	LastTickAgeSec   *int64                     `json:"last_tick_age_sec,omitempty"`
	IsolatedCount    int64                      `json:"isolated_count"`
	RecentIsolated   []ProxyHealthIsolatedItem  `json:"recent_isolated"`
	NowUnix          int64                      `json:"now_unix"`
}

// ProxyHealthIsolatedItem is a compact row for the monitoring table.
type ProxyHealthIsolatedItem struct {
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	Host             string `json:"host"`
	Port             int    `json:"port"`
	Protocol         string `json:"protocol"`
	Status           string `json:"status"`
	HealthFailCount  int    `json:"health_fail_count"`
	HealthIsolatedBy string `json:"health_isolated_by"`
	LastHealthAt     *int64 `json:"last_health_at,omitempty"`
}

// DefaultProxyHealthSettingsFromYAML builds settings from process YAML config.
func DefaultProxyHealthSettingsFromYAML(cfg *config.Config) ProxyHealthSettings {
	base := config.ProxyHealthConfig{
		Enabled:          false,
		IntervalSec:      60,
		TimeoutMS:        10000,
		Concurrency:      8,
		FailThreshold:    3,
		SuccessThreshold: 2,
		ProbeScope:       "group_members",
		AutoRecover:      true,
		SkipNamePrefix:   []string{"warp-"},
		LeaderLockTTLSec: 50,
		BatchSize:        100,
		ProbeMode:        "connectivity",
	}
	if cfg != nil {
		base = cfg.ProxyHealth
	}
	return proxyHealthConfigToSettings(base)
}

func proxyHealthConfigToSettings(c config.ProxyHealthConfig) ProxyHealthSettings {
	return ProxyHealthSettings{
		Enabled:          c.Enabled,
		IntervalSec:      c.IntervalSec,
		TimeoutMS:        c.TimeoutMS,
		Concurrency:      c.Concurrency,
		FailThreshold:    c.FailThreshold,
		SuccessThreshold: c.SuccessThreshold,
		ProbeScope:       c.ProbeScope,
		AutoRecover:      c.AutoRecover,
		// Always include "warp-" so health never isolates warp proxies
		// (warp sync rewrites status → fight with health isolation).
		SkipNamePrefix:   config.EnsureWarpSkipNamePrefix(c.SkipNamePrefix),
		LeaderLockTTLSec: c.LeaderLockTTLSec,
		BatchSize:        c.BatchSize,
		ProbeMode:        c.ProbeMode,
	}
}

func (s ProxyHealthSettings) toConfig() config.ProxyHealthConfig {
	return config.ProxyHealthConfig{
		Enabled:          s.Enabled,
		IntervalSec:      s.IntervalSec,
		TimeoutMS:        s.TimeoutMS,
		Concurrency:      s.Concurrency,
		FailThreshold:    s.FailThreshold,
		SuccessThreshold: s.SuccessThreshold,
		ProbeScope:       s.ProbeScope,
		AutoRecover:      s.AutoRecover,
		SkipNamePrefix:   append([]string(nil), s.SkipNamePrefix...),
		LeaderLockTTLSec: s.LeaderLockTTLSec,
		BatchSize:        s.BatchSize,
		ProbeMode:        s.ProbeMode,
	}
}

// normalizeProxyHealthSettings clamps/fills illegal values in-place.
func normalizeProxyHealthSettings(s *ProxyHealthSettings) error {
	if s == nil {
		return fmt.Errorf("proxy health settings is nil")
	}
	if s.IntervalSec < 0 {
		return fmt.Errorf("interval_sec must be non-negative")
	}
	if s.IntervalSec == 0 {
		s.IntervalSec = 60
	}
	if s.IntervalSec < 10 {
		s.IntervalSec = 10
	}
	if s.TimeoutMS < 0 {
		return fmt.Errorf("timeout_ms must be non-negative")
	}
	if s.TimeoutMS == 0 {
		s.TimeoutMS = 10000
	}
	if s.TimeoutMS > 120000 {
		s.TimeoutMS = 120000
	}
	if s.Concurrency <= 0 {
		s.Concurrency = 8
	}
	if s.Concurrency > 64 {
		s.Concurrency = 64
	}
	if s.FailThreshold <= 0 {
		s.FailThreshold = 3
	}
	if s.SuccessThreshold <= 0 {
		s.SuccessThreshold = 2
	}
	switch strings.TrimSpace(s.ProbeScope) {
	case "", "group_members":
		s.ProbeScope = "group_members"
	case "all_active":
		// ok
	default:
		return fmt.Errorf("probe_scope must be group_members or all_active")
	}
	switch strings.TrimSpace(strings.ToLower(s.ProbeMode)) {
	case "", "connectivity":
		s.ProbeMode = "connectivity"
	case "quality":
		s.ProbeMode = "quality"
	default:
		return fmt.Errorf("probe_mode must be connectivity or quality")
	}
	if s.LeaderLockTTLSec <= 0 {
		s.LeaderLockTTLSec = 50
	}
	if s.BatchSize <= 0 {
		s.BatchSize = 100
	}
	if s.BatchSize > 500 {
		s.BatchSize = 500
	}
	s.SkipNamePrefix = config.EnsureWarpSkipNamePrefix(s.SkipNamePrefix)
	return nil
}

// GetConfig returns effective admin settings (DB override or YAML baseline).
func (s *ProxyHealthService) GetConfig(ctx context.Context) (*ProxyHealthSettings, error) {
	if s == nil {
		return nil, fmt.Errorf("proxy health service not configured")
	}
	cfg := s.effectiveSettings(ctx)
	return &cfg, nil
}

// UpdateConfig validates, persists, applies runtime config, and restarts worker if needed.
func (s *ProxyHealthService) UpdateConfig(ctx context.Context, in *ProxyHealthSettings) (*ProxyHealthSettings, error) {
	if s == nil {
		return nil, fmt.Errorf("proxy health service not configured")
	}
	if in == nil {
		return nil, fmt.Errorf("settings is required")
	}
	next := *in
	if err := normalizeProxyHealthSettings(&next); err != nil {
		return nil, err
	}
	if s.settingRepo != nil {
		raw, err := json.Marshal(next)
		if err != nil {
			return nil, fmt.Errorf("marshal proxy health settings: %w", err)
		}
		if err := s.settingRepo.Set(ctx, SettingKeyProxyHealthSettings, string(raw)); err != nil {
			return nil, fmt.Errorf("save proxy health settings: %w", err)
		}
	}
	s.applySettings(next)
	if s.worker != nil {
		s.worker.Apply()
	}
	return &next, nil
}

// RuntimeSnapshot builds the monitoring dashboard payload.
func (s *ProxyHealthService) RuntimeSnapshot(ctx context.Context) *ProxyHealthRuntime {
	now := time.Now().Unix()
	out := &ProxyHealthRuntime{
		NowUnix:        now,
		RecentIsolated: []ProxyHealthIsolatedItem{},
	}
	if s == nil {
		return out
	}
	out.Config = s.effectiveSettings(ctx)
	out.YAMLEnabled = s.yamlBaselineEnabled
	if s.metrics != nil {
		out.Metrics = s.metrics.Snapshot()
		if out.Metrics.LastTickUnix > 0 {
			age := now - out.Metrics.LastTickUnix
			if age < 0 {
				age = 0
			}
			out.LastTickAgeSec = &age
		}
	}
	if s.worker != nil {
		out.WorkerRunning = s.worker.Running()
		out.WorkerInstanceID = s.worker.InstanceID()
	}
	if s.proxyRepo != nil {
		if n, err := s.proxyRepo.CountHealthIsolated(ctx); err == nil {
			out.IsolatedCount = n
		}
		if items, err := s.proxyRepo.ListHealthIsolated(ctx, 30); err == nil {
			out.RecentIsolated = make([]ProxyHealthIsolatedItem, 0, len(items))
			for i := range items {
				p := items[i]
				row := ProxyHealthIsolatedItem{
					ID:               p.ID,
					Name:             p.Name,
					Host:             p.Host,
					Port:             p.Port,
					Protocol:         p.Protocol,
					Status:           p.Status,
					HealthFailCount:  p.HealthFailCount,
					HealthIsolatedBy: p.HealthIsolatedBy,
				}
				if p.LastHealthAt != nil {
					u := p.LastHealthAt.Unix()
					row.LastHealthAt = &u
				}
				out.RecentIsolated = append(out.RecentIsolated, row)
			}
		}
	}
	return out
}

func (s *ProxyHealthService) effectiveSettings(ctx context.Context) ProxyHealthSettings {
	// Prefer in-memory runtime (already applied), then DB, then YAML.
	s.runtimeMu.RLock()
	if s.runtime != nil {
		cur := *s.runtime
		s.runtimeMu.RUnlock()
		return cur
	}
	s.runtimeMu.RUnlock()

	if loaded, ok := s.loadSettingsFromDB(ctx); ok {
		return loaded
	}
	return DefaultProxyHealthSettingsFromYAML(s.cfg)
}

func (s *ProxyHealthService) loadSettingsFromDB(ctx context.Context) (ProxyHealthSettings, bool) {
	if s == nil || s.settingRepo == nil {
		return ProxyHealthSettings{}, false
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyProxyHealthSettings)
	if err != nil || strings.TrimSpace(raw) == "" {
		return ProxyHealthSettings{}, false
	}
	var parsed ProxyHealthSettings
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		s.log.Warn("invalid proxy_health_settings json, falling back to yaml", "error", err)
		return ProxyHealthSettings{}, false
	}
	if err := normalizeProxyHealthSettings(&parsed); err != nil {
		s.log.Warn("proxy_health_settings normalize failed", "error", err)
		return ProxyHealthSettings{}, false
	}
	return parsed, true
}

func (s *ProxyHealthService) applySettings(settings ProxyHealthSettings) {
	if s == nil {
		return
	}
	_ = normalizeProxyHealthSettings(&settings)
	s.runtimeMu.Lock()
	cp := settings
	s.runtime = &cp
	s.runtimeMu.Unlock()
	// Keep process config in sync so worker interval/lock reads stay correct.
	if s.cfg != nil {
		s.cfg.ProxyHealth = settings.toConfig()
	}
}

func (s *ProxyHealthService) bootstrapRuntimeSettings() {
	if s == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if loaded, ok := s.loadSettingsFromDB(ctx); ok {
		s.applySettings(loaded)
		return
	}
	// Seed runtime from YAML so conf() is consistent.
	s.applySettings(DefaultProxyHealthSettingsFromYAML(s.cfg))
}

// SetWorker attaches the background worker for hot enable/disable.
func (s *ProxyHealthService) SetWorker(w *ProxyHealthWorker) {
	if s == nil {
		return
	}
	s.workerMu.Lock()
	s.worker = w
	s.workerMu.Unlock()
}
