package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestUpdateOpsMonitoringSettingsWritesAtomicallyAndPreservesGlobalMailRouting(t *testing.T) {
	ctx := context.Background()
	repo := newRuntimeSettingRepoStub()
	legacyEmail := defaultOpsEmailNotificationConfig()
	legacyEmail.Alert.Enabled = true
	legacyEmail.Alert.Recipients = []string{"oncall@example.com"}
	legacyEmail.Report.Enabled = true
	legacyEmail.Report.Recipients = []string{"reports@example.com"}
	rawEmail, err := json.Marshal(legacyEmail)
	if err != nil {
		t.Fatal(err)
	}
	repo.values[SettingKeyOpsEmailNotificationConfig] = string(rawEmail)

	svc := &OpsService{settingRepo: repo}
	runtimeCfg := defaultOpsAlertRuntimeSettings()
	advancedCfg := defaultOpsAdvancedSettings()
	thresholds := defaultOpsMetricThresholds()
	req := &OpsMonitoringSettings{
		Runtime: *runtimeCfg,
		EmailBehavior: OpsEmailBehaviorSettings{
			Alert: OpsEmailAlertBehaviorSettings{
				MinSeverity:           "warning",
				RateLimitPerHour:      12,
				BatchingWindowSeconds: 30,
				IncludeResolvedAlerts: true,
			},
			Report: OpsEmailReportBehaviorSettings{
				DailySummaryEnabled:             true,
				DailySummarySchedule:            "0 8 * * *",
				WeeklySummaryEnabled:            true,
				WeeklySummarySchedule:           "0 8 * * 1",
				ErrorDigestEnabled:              true,
				ErrorDigestSchedule:             "0 */6 * * *",
				ErrorDigestMinCount:             5,
				AccountHealthEnabled:            true,
				AccountHealthSchedule:           "30 8 * * *",
				AccountHealthErrorRateThreshold: 7.5,
			},
		},
		Advanced:         *advancedCfg,
		MetricThresholds: *thresholds,
	}

	updated, err := svc.UpdateOpsMonitoringSettings(ctx, req)
	if err != nil {
		t.Fatalf("UpdateOpsMonitoringSettings() error = %v", err)
	}
	if repo.setMultipleCalls != 1 {
		t.Fatalf("SetMultiple calls = %d, want 1", repo.setMultipleCalls)
	}
	if repo.setCalls != 0 {
		t.Fatalf("individual Set calls = %d, want 0", repo.setCalls)
	}
	if updated.EmailBehavior.Alert.MinSeverity != "warning" {
		t.Fatalf("updated email behavior = %+v", updated.EmailBehavior)
	}

	var storedEmail OpsEmailNotificationConfig
	if err := json.Unmarshal([]byte(repo.values[SettingKeyOpsEmailNotificationConfig]), &storedEmail); err != nil {
		t.Fatal(err)
	}
	if !storedEmail.Alert.Enabled || len(storedEmail.Alert.Recipients) != 1 || storedEmail.Alert.Recipients[0] != "oncall@example.com" {
		t.Fatalf("alert global routing changed: %+v", storedEmail.Alert)
	}
	if !storedEmail.Report.Enabled || len(storedEmail.Report.Recipients) != 1 || storedEmail.Report.Recipients[0] != "reports@example.com" {
		t.Fatalf("report global routing changed: %+v", storedEmail.Report)
	}
}

func TestUpdateOpsMonitoringSettingsRejectsBeforeAtomicWrite(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	svc := &OpsService{settingRepo: repo}
	req := &OpsMonitoringSettings{
		Runtime:          *defaultOpsAlertRuntimeSettings(),
		EmailBehavior:    opsEmailBehaviorSettings(defaultOpsEmailNotificationConfig()),
		Advanced:         *defaultOpsAdvancedSettings(),
		MetricThresholds: *defaultOpsMetricThresholds(),
	}
	req.MetricThresholds.SLAPercentMin = float64Ptr(120)

	if _, err := svc.UpdateOpsMonitoringSettings(context.Background(), req); err == nil {
		t.Fatal("invalid settings were accepted")
	}
	if repo.setMultipleCalls != 0 {
		t.Fatalf("SetMultiple calls = %d, want 0", repo.setMultipleCalls)
	}
}

func TestMetricThresholdsRejectZeroAndNormalizePreviouslyStoredZero(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	svc := &OpsService{settingRepo: repo}
	req := &OpsMonitoringSettings{
		Runtime:          *defaultOpsAlertRuntimeSettings(),
		EmailBehavior:    opsEmailBehaviorSettings(defaultOpsEmailNotificationConfig()),
		Advanced:         *defaultOpsAdvancedSettings(),
		MetricThresholds: *defaultOpsMetricThresholds(),
	}
	req.MetricThresholds.RequestErrorRatePercentMax = float64Ptr(0)

	if _, err := svc.UpdateOpsMonitoringSettings(context.Background(), req); err == nil {
		t.Fatal("zero request error threshold was accepted")
	}
	if repo.setMultipleCalls != 0 {
		t.Fatalf("SetMultiple calls = %d, want 0", repo.setMultipleCalls)
	}

	repo.values[SettingKeyOpsMetricThresholds] = `{"request_error_rate_percent_max":0}`
	got, err := svc.GetMetricThresholds(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := defaultOpsMetricThresholds()
	if got.RequestErrorRatePercentMax == nil || want.RequestErrorRatePercentMax == nil || *got.RequestErrorRatePercentMax != *want.RequestErrorRatePercentMax {
		t.Fatalf("stored zero was not normalized to defaults: got=%+v want=%+v", got, want)
	}
}

func TestUpdateOpsMonitoringSettingsRepositoryFailureDoesNotPartiallyWrite(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	repo.setMultipleFn = func(map[string]string) error { return errors.New("write failed") }
	svc := &OpsService{settingRepo: repo}
	req := &OpsMonitoringSettings{
		Runtime:          *defaultOpsAlertRuntimeSettings(),
		EmailBehavior:    opsEmailBehaviorSettings(defaultOpsEmailNotificationConfig()),
		Advanced:         *defaultOpsAdvancedSettings(),
		MetricThresholds: *defaultOpsMetricThresholds(),
	}

	if _, err := svc.UpdateOpsMonitoringSettings(context.Background(), req); err == nil {
		t.Fatal("repository failure was ignored")
	}
	for _, key := range []string{
		SettingKeyOpsAlertRuntimeSettings,
		SettingKeyOpsAdvancedSettings,
		SettingKeyOpsMetricThresholds,
	} {
		if _, ok := repo.values[key]; ok {
			t.Fatalf("key %q was partially written", key)
		}
	}
}

func TestUpdateOpsMonitoringSettingsRejectsInvalidEnabledReportSchedule(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	svc := &OpsService{settingRepo: repo}
	req := &OpsMonitoringSettings{
		Runtime:          *defaultOpsAlertRuntimeSettings(),
		EmailBehavior:    opsEmailBehaviorSettings(defaultOpsEmailNotificationConfig()),
		Advanced:         *defaultOpsAdvancedSettings(),
		MetricThresholds: *defaultOpsMetricThresholds(),
	}
	req.EmailBehavior.Report.DailySummaryEnabled = true
	req.EmailBehavior.Report.DailySummarySchedule = "not a schedule"

	if _, err := svc.UpdateOpsMonitoringSettings(context.Background(), req); err == nil {
		t.Fatal("invalid enabled report schedule was accepted")
	}
	if repo.setMultipleCalls != 0 {
		t.Fatalf("SetMultiple calls = %d, want 0", repo.setMultipleCalls)
	}
}
