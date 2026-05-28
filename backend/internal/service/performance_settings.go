package service

import (
	"context"
	"encoding/json"
	"errors"
)

// PerformanceSettings stores connection/latency performance tuning configuration (DB-backed).
type PerformanceSettings struct {
	ConnectionPrewarm ConnectionPrewarmSettings `json:"connection_prewarm"`
	DNSCache          DNSCacheSettings          `json:"dns_cache"`
	Dialer            DialerSettings            `json:"dialer"`
	SSEFlush          SSEFlushSettings          `json:"sse_flush"`
}

type ConnectionPrewarmSettings struct {
	Enabled             bool     `json:"enabled"`
	IntervalSeconds     int      `json:"interval_seconds"`
	TargetURLs          []string `json:"target_urls"`
	MaxConcurrentProbes int      `json:"max_concurrent_probes"`
}

type DNSCacheSettings struct {
	Enabled    bool `json:"enabled"`
	TTLSeconds int  `json:"ttl_seconds"`
}

type DialerSettings struct {
	TimeoutSeconds   int `json:"timeout_seconds"`
	KeepAliveSeconds int `json:"keepalive_seconds"`
}

type SSEFlushSettings struct {
	EagerFirstFlush bool `json:"eager_first_flush"`
}

func defaultPerformanceSettings() *PerformanceSettings {
	return &PerformanceSettings{
		ConnectionPrewarm: ConnectionPrewarmSettings{
			Enabled:             true,
			IntervalSeconds:     60,
			TargetURLs:          []string{"https://api.anthropic.com"},
			MaxConcurrentProbes: 3,
		},
		DNSCache: DNSCacheSettings{
			Enabled:    true,
			TTLSeconds: 60,
		},
		Dialer: DialerSettings{
			TimeoutSeconds:   10,
			KeepAliveSeconds: 30,
		},
		SSEFlush: SSEFlushSettings{
			EagerFirstFlush: true,
		},
	}
}

func normalizePerformanceSettings(cfg *PerformanceSettings) {
	if cfg.ConnectionPrewarm.IntervalSeconds < 10 {
		cfg.ConnectionPrewarm.IntervalSeconds = 60
	}
	if cfg.ConnectionPrewarm.IntervalSeconds > 300 {
		cfg.ConnectionPrewarm.IntervalSeconds = 300
	}
	if cfg.ConnectionPrewarm.MaxConcurrentProbes < 1 {
		cfg.ConnectionPrewarm.MaxConcurrentProbes = 3
	}
	if cfg.ConnectionPrewarm.MaxConcurrentProbes > 20 {
		cfg.ConnectionPrewarm.MaxConcurrentProbes = 20
	}
	if len(cfg.ConnectionPrewarm.TargetURLs) == 0 {
		cfg.ConnectionPrewarm.TargetURLs = []string{"https://api.anthropic.com"}
	}
	if cfg.DNSCache.TTLSeconds < 10 {
		cfg.DNSCache.TTLSeconds = 60
	}
	if cfg.DNSCache.TTLSeconds > 600 {
		cfg.DNSCache.TTLSeconds = 600
	}
	if cfg.Dialer.TimeoutSeconds < 1 {
		cfg.Dialer.TimeoutSeconds = 10
	}
	if cfg.Dialer.KeepAliveSeconds < 5 {
		cfg.Dialer.KeepAliveSeconds = 30
	}
}

// GetPerformanceSettings returns current performance tuning settings (DB-backed).
func (s *OpsService) GetPerformanceSettings(ctx context.Context) (*PerformanceSettings, error) {
	defaultCfg := defaultPerformanceSettings()
	if s == nil || s.settingRepo == nil {
		return defaultCfg, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	raw, err := s.settingRepo.GetValue(ctx, SettingKeyPerformanceSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			if b, mErr := json.Marshal(defaultCfg); mErr == nil {
				_ = s.settingRepo.Set(ctx, SettingKeyPerformanceSettings, string(b))
			}
			return defaultCfg, nil
		}
		return nil, err
	}

	cfg := defaultPerformanceSettings()
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return defaultCfg, nil
	}

	normalizePerformanceSettings(cfg)
	return cfg, nil
}

// UpdatePerformanceSettings updates performance tuning settings (DB-backed).
func (s *OpsService) UpdatePerformanceSettings(ctx context.Context, cfg *PerformanceSettings) (*PerformanceSettings, error) {
	if s == nil || s.settingRepo == nil {
		return nil, errors.New("setting repository not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg == nil {
		return nil, errors.New("invalid config")
	}

	normalizePerformanceSettings(cfg)
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	if err := s.settingRepo.Set(ctx, SettingKeyPerformanceSettings, string(raw)); err != nil {
		return nil, err
	}
	return cfg, nil
}
