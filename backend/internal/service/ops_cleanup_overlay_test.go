//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// makeOverlayService 构造一个没有 cron / db 的 cleanup service，仅用来测试 effective overlay。
func makeOverlayService(repo SettingRepository, base config.OpsCleanupConfig) *OpsCleanupService {
	cfg := &config.Config{}
	cfg.Ops.Cleanup = base
	return &OpsCleanupService{
		cfg:         cfg,
		settingRepo: repo,
	}
}

func writeAdvancedSettings(t *testing.T, repo *runtimeSettingRepoStub, dr OpsDataRetentionSettings) {
	t.Helper()
	adv := OpsAdvancedSettings{DataRetention: dr}
	raw, err := json.Marshal(adv)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := repo.Set(context.Background(), SettingKeyOpsAdvancedSettings, string(raw)); err != nil {
		t.Fatalf("set: %v", err)
	}
}

func TestComputeEffective_FallbackToCfgWhenSettingsAbsent(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	base := config.OpsCleanupConfig{
		Enabled:                    false,
		Schedule:                   "0 2 * * *",
		ErrorLogRetentionDays:      30,
		MinuteMetricsRetentionDays: 30,
		HourlyMetricsRetentionDays: 30,
	}
	svc := makeOverlayService(repo, base)

	svc.computeEffectiveLocked(context.Background())

	want := opsCleanupEffectiveConfig{OpsCleanupConfig: base, SystemLogRetentionDays: 30}
	if svc.effective != want {
		t.Fatalf("expected effective == cfg base, got %#v", svc.effective)
	}
}

func TestComputeEffective_SettingsOverridesAll(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	writeAdvancedSettings(t, repo, OpsDataRetentionSettings{
		CleanupEnabled:             true,
		CleanupSchedule:            "0 * * * *",
		ErrorLogRetentionDays:      0,
		MinuteMetricsRetentionDays: 7,
		HourlyMetricsRetentionDays: 14,
	})
	base := config.OpsCleanupConfig{
		Enabled:                    false,
		Schedule:                   "0 2 * * *",
		ErrorLogRetentionDays:      30,
		MinuteMetricsRetentionDays: 30,
		HourlyMetricsRetentionDays: 30,
	}
	svc := makeOverlayService(repo, base)

	svc.computeEffectiveLocked(context.Background())

	want := opsCleanupEffectiveConfig{
		OpsCleanupConfig: config.OpsCleanupConfig{
			Enabled:                    true,
			Schedule:                   "0 * * * *",
			ErrorLogRetentionDays:      0,
			MinuteMetricsRetentionDays: 7,
			HourlyMetricsRetentionDays: 14,
		},
		SystemLogRetentionDays: 30,
	}
	if svc.effective != want {
		t.Fatalf("effective mismatch:\nwant %#v\n got %#v", want, svc.effective)
	}
}

func TestComputeEffective_EmptyScheduleFallbackToCfg(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	writeAdvancedSettings(t, repo, OpsDataRetentionSettings{
		CleanupEnabled:             true,
		CleanupSchedule:            "   ", // 空白被 trim 后视为空
		ErrorLogRetentionDays:      5,
		MinuteMetricsRetentionDays: 5,
		HourlyMetricsRetentionDays: 5,
	})
	base := config.OpsCleanupConfig{
		Enabled:                    false,
		Schedule:                   "0 2 * * *",
		ErrorLogRetentionDays:      30,
		MinuteMetricsRetentionDays: 30,
		HourlyMetricsRetentionDays: 30,
	}
	svc := makeOverlayService(repo, base)

	svc.computeEffectiveLocked(context.Background())

	if svc.effective.Schedule != "0 2 * * *" {
		t.Fatalf("expected schedule fallback to cfg, got %q", svc.effective.Schedule)
	}
	if !svc.effective.Enabled {
		t.Fatalf("expected enabled=true from settings")
	}
	if svc.effective.ErrorLogRetentionDays != 5 {
		t.Fatalf("expected retention=5 from settings, got %d", svc.effective.ErrorLogRetentionDays)
	}
}

func TestComputeEffective_NegativeRetentionFallsBackToCfg(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	writeAdvancedSettings(t, repo, OpsDataRetentionSettings{
		CleanupEnabled:             true,
		CleanupSchedule:            "0 * * * *",
		ErrorLogRetentionDays:      -1,
		MinuteMetricsRetentionDays: -1,
		HourlyMetricsRetentionDays: -1,
	})
	base := config.OpsCleanupConfig{
		Enabled:                    false,
		Schedule:                   "0 2 * * *",
		ErrorLogRetentionDays:      30,
		MinuteMetricsRetentionDays: 60,
		HourlyMetricsRetentionDays: 90,
	}
	svc := makeOverlayService(repo, base)

	svc.computeEffectiveLocked(context.Background())

	if svc.effective.ErrorLogRetentionDays != 30 ||
		svc.effective.MinuteMetricsRetentionDays != 60 ||
		svc.effective.HourlyMetricsRetentionDays != 90 {
		t.Fatalf("expected retention fallback to cfg, got %#v", svc.effective)
	}
}

func TestComputeEffective_BadJSONFallsBackToCfg(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	if err := repo.Set(context.Background(), SettingKeyOpsAdvancedSettings, "{not json"); err != nil {
		t.Fatalf("set: %v", err)
	}
	base := config.OpsCleanupConfig{
		Enabled:                    true,
		Schedule:                   "0 3 * * *",
		ErrorLogRetentionDays:      30,
		MinuteMetricsRetentionDays: 30,
		HourlyMetricsRetentionDays: 30,
	}
	svc := makeOverlayService(repo, base)

	svc.computeEffectiveLocked(context.Background())

	want := opsCleanupEffectiveConfig{OpsCleanupConfig: base, SystemLogRetentionDays: 30}
	if svc.effective != want {
		t.Fatalf("expected fallback to cfg on bad JSON, got %#v", svc.effective)
	}
}

func TestComputeEffective_RuntimeConfigOwnsSystemLogRetention(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	writeAdvancedSettings(t, repo, OpsDataRetentionSettings{
		CleanupEnabled:        true,
		CleanupSchedule:       "0 2 * * *",
		ErrorLogRetentionDays: 90,
	})
	repo.values[SettingKeyOpsRuntimeLogConfig] = `{"retention_days":7}`
	svc := makeOverlayService(repo, config.OpsCleanupConfig{ErrorLogRetentionDays: 30})

	svc.computeEffectiveLocked(context.Background())

	if svc.effective.ErrorLogRetentionDays != 90 {
		t.Fatalf("error log retention = %d, want 90", svc.effective.ErrorLogRetentionDays)
	}
	if svc.effective.SystemLogRetentionDays != 7 {
		t.Fatalf("system log retention = %d, want 7", svc.effective.SystemLogRetentionDays)
	}
}

func TestComputeEffective_InvalidRuntimeConfigDoesNotDiscardAdvancedSettings(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	writeAdvancedSettings(t, repo, OpsDataRetentionSettings{
		CleanupEnabled:        true,
		CleanupSchedule:       "0 * * * *",
		ErrorLogRetentionDays: 14,
	})
	repo.values[SettingKeyOpsRuntimeLogConfig] = `{invalid-json}`
	svc := makeOverlayService(repo, config.OpsCleanupConfig{ErrorLogRetentionDays: 30})

	svc.computeEffectiveLocked(context.Background())

	if !svc.effective.Enabled || svc.effective.ErrorLogRetentionDays != 14 {
		t.Fatalf("advanced settings were discarded: %#v", svc.effective)
	}
	if svc.effective.SystemLogRetentionDays != 30 {
		t.Fatalf("system log retention = %d, want default 30", svc.effective.SystemLogRetentionDays)
	}
}

func TestComputeEffective_PartialAdvancedSettingsUseSafeDefaults(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	repo.values[SettingKeyOpsAdvancedSettings] = `{"data_retention":{"cleanup_enabled":true}}`
	svc := makeOverlayService(repo, config.OpsCleanupConfig{
		Schedule:                   "0 3 * * *",
		ErrorLogRetentionDays:      90,
		MinuteMetricsRetentionDays: 90,
		HourlyMetricsRetentionDays: 90,
	})

	svc.computeEffectiveLocked(context.Background())

	if svc.effective.Schedule != opsCleanupDefaultSchedule ||
		svc.effective.ErrorLogRetentionDays != 30 ||
		svc.effective.MinuteMetricsRetentionDays != 30 ||
		svc.effective.HourlyMetricsRetentionDays != 30 {
		t.Fatalf("partial settings did not use safe defaults: %#v", svc.effective)
	}
}

// 验证 OpsService.UpdateOpsAdvancedSettings 写入后会调用 cleanupReloader.Reload。
type fakeCleanupReloader struct {
	calls int
	last  context.Context
	err   error
}

func (f *fakeCleanupReloader) Reload(ctx context.Context) error {
	f.calls++
	f.last = ctx
	return f.err
}

func TestUpdateOpsAdvancedSettings_TriggersReload(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	reloader := &fakeCleanupReloader{}
	svc := &OpsService{settingRepo: repo}
	svc.SetCleanupReloader(reloader)

	cfg := defaultOpsAdvancedSettings()
	cfg.DataRetention.CleanupEnabled = true
	cfg.DataRetention.CleanupSchedule = "0 * * * *"
	cfg.DataRetention.ErrorLogRetentionDays = 3
	cfg.DataRetention.MinuteMetricsRetentionDays = 3
	cfg.DataRetention.HourlyMetricsRetentionDays = 3

	if _, err := svc.UpdateOpsAdvancedSettings(context.Background(), cfg); err != nil {
		t.Fatalf("update: %v", err)
	}
	if reloader.calls != 1 {
		t.Fatalf("expected reloader.Reload called once, got %d", reloader.calls)
	}
}

func TestReload_BeforeStart_IsNoop(t *testing.T) {
	svc := &OpsCleanupService{}
	if err := svc.Reload(context.Background()); err != nil {
		t.Fatalf("Reload before Start should return nil, got %v", err)
	}
}

func TestReload_AfterStop_IsNoop(t *testing.T) {
	svc := &OpsCleanupService{started: true, stopped: true}
	if err := svc.Reload(context.Background()); err != nil {
		t.Fatalf("Reload after Stop should return nil, got %v", err)
	}
}

func TestUpdateOpsAdvancedSettings_NilReloader_NoPanic(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	svc := &OpsService{settingRepo: repo}
	// cleanupReloader intentionally nil

	cfg := defaultOpsAdvancedSettings()
	cfg.DataRetention.ErrorLogRetentionDays = 7

	// should not panic
	if _, err := svc.UpdateOpsAdvancedSettings(context.Background(), cfg); err != nil {
		t.Fatalf("update with nil reloader: %v", err)
	}
}

func TestStart_IdempotentSecondCall(t *testing.T) {
	svc := &OpsCleanupService{started: true}
	svc.Start() // second call should be noop, not panic
}

func TestRefreshEffectiveBeforeRun_UpdatesSnapshot(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	base := config.OpsCleanupConfig{
		Enabled:               true,
		Schedule:              "0 2 * * *",
		ErrorLogRetentionDays: 30,
	}
	svc := makeOverlayService(repo, base)
	svc.computeEffectiveLocked(context.Background())

	if svc.effective.ErrorLogRetentionDays != 30 {
		t.Fatalf("initial retention should be 30, got %d", svc.effective.ErrorLogRetentionDays)
	}

	// simulate UI change
	writeAdvancedSettings(t, repo, OpsDataRetentionSettings{
		CleanupEnabled:        true,
		CleanupSchedule:       "0 * * * *",
		ErrorLogRetentionDays: 7,
	})

	svc.refreshEffectiveBeforeRun(context.Background())
	snap := svc.snapshotEffective()
	if snap.ErrorLogRetentionDays != 7 {
		t.Fatalf("after refresh, retention should be 7, got %d", snap.ErrorLogRetentionDays)
	}
}
