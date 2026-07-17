package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var (
	benchmarkOpsMonitoringEnabled bool
	benchmarkOpsAdvancedSettings  OpsAdvancedSettings
)

type opsRuntimeRefreshRepo struct {
	SettingRepository
	mu     sync.RWMutex
	values map[string]string
	fail   atomic.Bool
	calls  atomic.Int64
REDACTED

func (r *opsRuntimeRefreshRepo) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	r.calls.Add(1)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
REDACTED
	if r.fail.Load() {
		return nil, errors.New("settings unavailable")
REDACTED
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			out[key] = value
	REDACTED
REDACTED
	return out, nil
REDACTED

func (r *opsRuntimeRefreshRepo) set(key, value string) {
	r.mu.Lock()
	r.values[key] = value
	r.mu.Unlock()
REDACTED

func waitForOpsRefresh(t *testing.T, timeout time.Duration, condition func() bool) {
REDACTED
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
	REDACTED
		time.Sleep(time.Millisecond)
REDACTED
	t.Fatal("condition not satisfied before timeout")
REDACTED

func TestOpsRuntimeSettingsSnapshotLoadsOnceAndServesHotPath(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	repo.values[SettingKeyOpsMonitoringEnabled] = "false"
	repo.values[SettingKeyOpsAdvancedSettings] = `{"ignore_context_canceled":false,"auto_refresh_interval_seconds":45REDACTED`

	svc := &OpsService{settingRepo: repoREDACTED
	svc.initRuntimeSettings(context.Background())
	if repo.getMultipleCalls != 1 {
		t.Fatalf("startup GetMultiple calls = %d, want 1", repo.getMultipleCalls)
REDACTED

	for range 1000 {
		if svc.IsMonitoringEnabled(context.Background()) {
			t.Fatal("monitoring enabled, want false")
	REDACTED
		cfg, err := svc.GetOpsAdvancedSettings(context.Background())
		if err != nil {
			t.Fatalf("GetOpsAdvancedSettings() error = %v", err)
	REDACTED
		if cfg.AutoRefreshIntervalSec != 45 {
			t.Fatalf("AutoRefreshIntervalSec = %d, want 45", cfg.AutoRefreshIntervalSec)
	REDACTED
REDACTED
	if repo.getValueCalls != 0 || repo.getMultipleCalls != 1 {
		t.Fatalf("hot path touched repository: get=%d get_multiple=%d", repo.getValueCalls, repo.getMultipleCalls)
REDACTED
REDACTED

func TestOpsRuntimeSettingsAdministrativeUpdatesAreImmediatelyVisible(t *testing.T) {
	svc := &OpsService{REDACTED
	svc.initRuntimeSettings(context.Background())

	svc.SetMonitoringEnabled(false)
	if svc.IsMonitoringEnabled(context.Background()) {
		t.Fatal("monitoring update was not visible")
REDACTED

	cfg := defaultOpsAdvancedSettings()
	cfg.IgnoreNoAvailableAccounts = true
	svc.storeAdvancedSettingsSnapshot(cfg)
	got, err := svc.GetOpsAdvancedSettings(context.Background())
	if err != nil {
		t.Fatalf("GetOpsAdvancedSettings() error = %v", err)
REDACTED
	if !got.IgnoreNoAvailableAccounts {
		t.Fatal("advanced settings update was not visible")
REDACTED
	if svc.IsMonitoringEnabled(context.Background()) {
		t.Fatal("advanced update overwrote monitoring setting")
REDACTED
REDACTED

func TestOpsRuntimeSettingsBackgroundRefreshConverges(t *testing.T) {
	repo := &opsRuntimeRefreshRepo{values: map[string]string{SettingKeyOpsMonitoringEnabled: "false"REDACTEDREDACTED
	svc := &OpsService{settingRepo: repoREDACTED
	svc.initRuntimeSettings(context.Background())
	if svc.IsMonitoringEnabled(context.Background()) {
		t.Fatal("initial monitoring state = true, want false")
REDACTED

	repo.set(SettingKeyOpsMonitoringEnabled, "true")
	svc.startRuntimeSettingsRefresh(context.Background(), 5*time.Millisecond, 0, 50*time.Millisecond)
	t.Cleanup(svc.StopRuntimeSettingsRefresh)
	waitForOpsRefresh(t, time.Second, func() bool {
		return svc.IsMonitoringEnabled(context.Background()) && svc.RuntimeSettingsRefreshHealth().SuccessTotal > 0
REDACTED)
REDACTED

func TestOpsRuntimeSettingsRefreshFailuresKeepLastKnownGoodSnapshot(t *testing.T) {
	repo := &opsRuntimeRefreshRepo{values: map[string]string{
		SettingKeyOpsMonitoringEnabled: "false",
		SettingKeyOpsAdvancedSettings:  `{"ignore_no_available_accounts":trueREDACTED`,
REDACTEDREDACTED
	svc := &OpsService{settingRepo: repoREDACTED
	svc.initRuntimeSettings(context.Background())
	repo.fail.Store(true)
	svc.startRuntimeSettingsRefresh(context.Background(), 5*time.Millisecond, 0, 50*time.Millisecond)
	t.Cleanup(svc.StopRuntimeSettingsRefresh)
	waitForOpsRefresh(t, time.Second, func() bool {
		return svc.RuntimeSettingsRefreshHealth().FailureTotal >= 3
REDACTED)

	if svc.IsMonitoringEnabled(context.Background()) {
		t.Fatal("failed refresh overwrote last known monitoring state")
REDACTED
	if !svc.OpsAdvancedSettingsSnapshot().IgnoreNoAvailableAccounts {
		t.Fatal("failed refresh overwrote last known advanced settings")
REDACTED
REDACTED

func TestOpsRuntimeSettingsRefreshStopEndsLifecycle(t *testing.T) {
	repo := &opsRuntimeRefreshRepo{values: map[string]string{REDACTEDREDACTED
	svc := &OpsService{settingRepo: repoREDACTED
	svc.initRuntimeSettings(context.Background())
	svc.startRuntimeSettingsRefresh(context.Background(), 5*time.Millisecond, 0, 50*time.Millisecond)
	waitForOpsRefresh(t, time.Second, func() bool {
		return svc.RuntimeSettingsRefreshHealth().SuccessTotal > 0
REDACTED)

	svc.StopRuntimeSettingsRefresh()
	callsAfterStop := repo.calls.Load()
	time.Sleep(20 * time.Millisecond)
	if got := repo.calls.Load(); got != callsAfterStop {
		t.Fatalf("refresh continued after Stop: before=%d after=%d", callsAfterStop, got)
REDACTED
	if svc.RuntimeSettingsRefreshHealth().Running {
		t.Fatal("refresh health still reports running after Stop")
REDACTED
	// Idempotence is part of the cleanup contract.
	svc.StopRuntimeSettingsRefresh()
REDACTED

func BenchmarkOpsRuntimeSettingsSnapshotRead(b *testing.B) {
	svc := &OpsService{REDACTED
	svc.initRuntimeSettings(context.Background())
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		benchmarkOpsMonitoringEnabled = svc.IsMonitoringEnabled(ctx)
		benchmarkOpsAdvancedSettings = svc.OpsAdvancedSettingsSnapshot()
REDACTED
REDACTED
