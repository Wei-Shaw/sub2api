package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type memSettingRepo struct {
	values map[string]string
}

func (m *memSettingRepo) Get(context.Context, string) (*Setting, error) { return nil, nil }
func (m *memSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	if m == nil || m.values == nil {
		return "", nil
	}
	return m.values[key], nil
}
func (m *memSettingRepo) Set(_ context.Context, key, value string) error {
	if m.values == nil {
		m.values = map[string]string{}
	}
	m.values[key] = value
	return nil
}
func (m *memSettingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return nil, nil
}
func (m *memSettingRepo) SetMultiple(context.Context, map[string]string) error { return nil }
func (m *memSettingRepo) GetAll(context.Context) (map[string]string, error)    { return nil, nil }
func (m *memSettingRepo) Delete(context.Context, string) error                 { return nil }

func TestNormalizeProxyHealthSettings(t *testing.T) {
	s := &ProxyHealthSettings{
		IntervalSec: 5,
		Concurrency: 100,
		ProbeScope:  "bad",
	}
	if err := normalizeProxyHealthSettings(s); err == nil {
		t.Fatal("expected probe_scope error")
	}
	s.ProbeScope = "group_members"
	s.ProbeMode = "connectivity"
	if err := normalizeProxyHealthSettings(s); err != nil {
		t.Fatal(err)
	}
	if s.IntervalSec != 10 {
		t.Fatalf("interval floor want 10 got %d", s.IntervalSec)
	}
	if s.Concurrency != 64 {
		t.Fatalf("concurrency cap want 64 got %d", s.Concurrency)
	}
	if len(s.SkipNamePrefix) == 0 || s.SkipNamePrefix[0] != "warp-" {
		t.Fatalf("default skip prefix: %#v", s.SkipNamePrefix)
	}
}

func TestProxyHealthUpdateConfigPersistsAndApplies(t *testing.T) {
	cfg := &config.Config{}
	cfg.ProxyHealth = config.ProxyHealthConfig{
		Enabled:     false,
		IntervalSec: 60,
		TimeoutMS:   8000,
		Concurrency: 4,
		ProbeScope:  "group_members",
		ProbeMode:   "connectivity",
		BatchSize:   50,
	}
	repo := &memSettingRepo{values: map[string]string{}}
	svc := NewProxyHealthService(cfg, nil, nil, nil, nil, nil, nil, ProvideProxyHealthMetrics(), repo)

	in := &ProxyHealthSettings{
		Enabled:          true,
		IntervalSec:      30,
		TimeoutMS:        5000,
		Concurrency:      2,
		FailThreshold:    4,
		SuccessThreshold: 3,
		ProbeScope:       "all_active",
		AutoRecover:      true,
		SkipNamePrefix:   []string{"warp-", "tmp-"},
		LeaderLockTTLSec: 40,
		BatchSize:        20,
		ProbeMode:        "connectivity",
	}
	out, err := svc.UpdateConfig(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Enabled || out.IntervalSec != 30 || out.ProbeScope != "all_active" {
		t.Fatalf("unexpected out: %+v", out)
	}
	if !cfg.ProxyHealth.Enabled || cfg.ProxyHealth.IntervalSec != 30 {
		t.Fatalf("cfg not applied: %+v", cfg.ProxyHealth)
	}
	raw, _ := repo.GetValue(context.Background(), SettingKeyProxyHealthSettings)
	if raw == "" {
		t.Fatal("expected persisted settings")
	}
	var stored ProxyHealthSettings
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.BatchSize != 20 {
		t.Fatalf("stored batch %d", stored.BatchSize)
	}

	rt := svc.RuntimeSnapshot(context.Background())
	if !rt.Config.Enabled || rt.Config.IntervalSec != 30 {
		t.Fatalf("runtime config: %+v", rt.Config)
	}
	if rt.Metrics.Ticks != 0 {
		t.Fatalf("expected zero ticks before worker start, got %d", rt.Metrics.Ticks)
	}
}

func TestProxyHealthWorkerApplyStartStop(t *testing.T) {
	cfg := &config.Config{}
	cfg.ProxyHealth.Enabled = false
	cfg.ProxyHealth.IntervalSec = 60
	svc := NewProxyHealthService(cfg, nil, nil, nil, nil, nil, nil, nil, nil)
	w := ProvideProxyHealthWorker(cfg, svc, nil)
	if w.Running() {
		t.Fatal("should not run when disabled")
	}
	cfg.ProxyHealth.Enabled = true
	w.Apply()
	if !w.Running() {
		t.Fatal("should run after enable")
	}
	cfg.ProxyHealth.Enabled = false
	w.Apply()
	if w.Running() {
		t.Fatal("should stop after disable")
	}
}
