package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

type runtimeSettingRepoStub struct {
	values           map[string]string
	deleted          map[string]bool
	setCalls         int
	getValueCalls    int
	getMultipleCalls int
	getValueFn       func(key string) (string, error)
	setFn            func(key, value string) error
	deleteFn         func(key string) error
REDACTED

func newRuntimeSettingRepoStub() *runtimeSettingRepoStub {
	return &runtimeSettingRepoStub{
		values:  map[string]string{REDACTED,
		deleted: map[string]bool{REDACTED,
REDACTED
REDACTED

func (s *runtimeSettingRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	value, err := s.GetValue(ctx, key)
	if err != nil {
		return nil, err
REDACTED
	return &Setting{Key: key, Value: valueREDACTED, nil
REDACTED

func (s *runtimeSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	s.getValueCalls++
	if s.getValueFn != nil {
		return s.getValueFn(key)
REDACTED
	value, ok := s.values[key]
	if !ok {
		return "", ErrSettingNotFound
REDACTED
	return value, nil
REDACTED

func (s *runtimeSettingRepoStub) Set(_ context.Context, key, value string) error {
	if s.setFn != nil {
		if err := s.setFn(key, value); err != nil {
			return err
	REDACTED
REDACTED
	s.values[key] = value
	s.setCalls++
	return nil
REDACTED

func (s *runtimeSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	s.getMultipleCalls++
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
	REDACTED
REDACTED
	return out, nil
REDACTED

func (s *runtimeSettingRepoStub) SetMultiple(_ context.Context, settings map[string]string) error {
	for key, value := range settings {
		s.values[key] = value
REDACTED
	return nil
REDACTED

func (s *runtimeSettingRepoStub) GetAll(_ context.Context) (map[string]string, error) {
	out := make(map[string]string, len(s.values))
	for key, value := range s.values {
		out[key] = value
REDACTED
	return out, nil
REDACTED

func (s *runtimeSettingRepoStub) Delete(_ context.Context, key string) error {
	if s.deleteFn != nil {
		if err := s.deleteFn(key); err != nil {
			return err
	REDACTED
REDACTED
	if _, ok := s.values[key]; !ok {
		return ErrSettingNotFound
REDACTED
	delete(s.values, key)
	s.deleted[key] = true
	return nil
REDACTED

func TestUpdateRuntimeLogConfig_InvalidConfigShouldNotApply(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	svc := &OpsService{
		settingRepo: repo,
		cfg: &config.Config{
			Log: config.LogConfig{
				Level:           "info",
				Caller:          true,
				StacktraceLevel: "error",
				Sampling: config.LogSamplingConfig{
					Enabled:    false,
					Initial:    100,
					Thereafter: 100,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	if err := logger.Init(logger.InitOptions{
		Level:       "info",
		Format:      "json",
		ServiceName: "sub2api",
		Environment: "test",
		Output: logger.OutputOptions{
			ToStdout: true,
			ToFile:   false,
	REDACTED,
REDACTED); err != nil {
		t.Fatalf("init logger: %v", err)
REDACTED

	_, err := svc.UpdateRuntimeLogConfig(context.Background(), &OpsRuntimeLogConfig{
		Level:           "trace",
		EnableSampling:  true,
		SamplingInitial: 100,
		SamplingNext:    100,
		Caller:          true,
		StacktraceLevel: "error",
		RetentionDays:   30,
REDACTED, 1)
	if err == nil {
		t.Fatalf("expected validation error")
REDACTED
	if logger.CurrentLevel() != "info" {
		t.Fatalf("logger level changed unexpectedly: %s", logger.CurrentLevel())
REDACTED
	if repo.setCalls != 1 {
		// GetRuntimeLogConfig() 会在 key 缺失时写入默认值，此处应只有这一次持久化。
		t.Fatalf("unexpected set calls: %d", repo.setCalls)
REDACTED
REDACTED

func TestResetRuntimeLogConfig_ShouldFallbackToBaseline(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	existing := &OpsRuntimeLogConfig{
		Level:           "debug",
		EnableSampling:  true,
		SamplingInitial: 50,
		SamplingNext:    50,
		Caller:          true,
		StacktraceLevel: "error",
		RetentionDays:   60,
		Source:          "runtime_setting",
REDACTED
	raw, _ := json.Marshal(existing)
	repo.values[SettingKeyOpsRuntimeLogConfig] = string(raw)

	svc := &OpsService{
		settingRepo: repo,
		cfg: &config.Config{
			Log: config.LogConfig{
				Level:           "warn",
				Caller:          false,
				StacktraceLevel: "fatal",
				Sampling: config.LogSamplingConfig{
					Enabled:    false,
					Initial:    100,
					Thereafter: 100,
			REDACTED,
		REDACTED,
			Ops: config.OpsConfig{
				Cleanup: config.OpsCleanupConfig{
					ErrorLogRetentionDays: 45,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	if err := logger.Init(logger.InitOptions{
		Level:       "debug",
		Format:      "json",
		ServiceName: "sub2api",
		Environment: "test",
		Output: logger.OutputOptions{
			ToStdout: true,
			ToFile:   false,
	REDACTED,
REDACTED); err != nil {
		t.Fatalf("init logger: %v", err)
REDACTED

	resetCfg, err := svc.ResetRuntimeLogConfig(context.Background(), 9)
	if err != nil {
		t.Fatalf("ResetRuntimeLogConfig() error: %v", err)
REDACTED
	if resetCfg.Source != "baseline" {
		t.Fatalf("source = %q, want baseline", resetCfg.Source)
REDACTED
	if resetCfg.Level != "warn" {
		t.Fatalf("level = %q, want warn", resetCfg.Level)
REDACTED
	if resetCfg.RetentionDays != 45 {
		t.Fatalf("retention_days = %d, want 45", resetCfg.RetentionDays)
REDACTED
	if logger.CurrentLevel() != "warn" {
		t.Fatalf("logger level = %q, want warn", logger.CurrentLevel())
REDACTED
	if !repo.deleted[SettingKeyOpsRuntimeLogConfig] {
		t.Fatalf("runtime setting key should be deleted")
REDACTED
REDACTED

func TestResetRuntimeLogConfig_InvalidOperator(t *testing.T) {
	svc := &OpsService{settingRepo: newRuntimeSettingRepoStub()REDACTED
	_, err := svc.ResetRuntimeLogConfig(context.Background(), 0)
	if err == nil {
		t.Fatalf("expected invalid operator error")
REDACTED
	if err.Error() != "invalid operator id" {
		t.Fatalf("unexpected error: %v", err)
REDACTED
REDACTED

func TestGetRuntimeLogConfig_InvalidJSONFallback(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	repo.values[SettingKeyOpsRuntimeLogConfig] = `{invalid-jsonREDACTED`

	svc := &OpsService{
		settingRepo: repo,
		cfg: &config.Config{
			Log: config.LogConfig{
				Level:           "warn",
				Caller:          true,
				StacktraceLevel: "error",
				Sampling: config.LogSamplingConfig{
					Enabled:    false,
					Initial:    100,
					Thereafter: 100,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	got, err := svc.GetRuntimeLogConfig(context.Background())
	if err != nil {
		t.Fatalf("GetRuntimeLogConfig() error: %v", err)
REDACTED
	if got.Level != "warn" {
		t.Fatalf("level = %q, want warn", got.Level)
REDACTED
REDACTED

func TestUpdateRuntimeLogConfig_PersistFailureRollback(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	oldCfg := &OpsRuntimeLogConfig{
		Level:           "info",
		EnableSampling:  false,
		SamplingInitial: 100,
		SamplingNext:    100,
		Caller:          true,
		StacktraceLevel: "error",
		RetentionDays:   30,
REDACTED
	raw, _ := json.Marshal(oldCfg)
	repo.values[SettingKeyOpsRuntimeLogConfig] = string(raw)
	repo.setFn = func(key, value string) error {
		if key == SettingKeyOpsRuntimeLogConfig {
			return errors.New("db down")
	REDACTED
		return nil
REDACTED

	svc := &OpsService{
		settingRepo: repo,
		cfg: &config.Config{
			Log: config.LogConfig{
				Level:           "info",
				Caller:          true,
				StacktraceLevel: "error",
				Sampling: config.LogSamplingConfig{
					Enabled:    false,
					Initial:    100,
					Thereafter: 100,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	if err := logger.Init(logger.InitOptions{
		Level:       "info",
		Format:      "json",
		ServiceName: "sub2api",
		Environment: "test",
		Output: logger.OutputOptions{
			ToStdout: true,
			ToFile:   false,
	REDACTED,
REDACTED); err != nil {
		t.Fatalf("init logger: %v", err)
REDACTED

	_, err := svc.UpdateRuntimeLogConfig(context.Background(), &OpsRuntimeLogConfig{
		Level:           "debug",
		EnableSampling:  false,
		SamplingInitial: 100,
		SamplingNext:    100,
		Caller:          true,
		StacktraceLevel: "error",
		RetentionDays:   30,
REDACTED, 5)
	if err == nil {
		t.Fatalf("expected persist error")
REDACTED
	// Persist failure should rollback runtime level back to old effective level.
	if logger.CurrentLevel() != "info" {
		t.Fatalf("logger level should rollback to info, got %s", logger.CurrentLevel())
REDACTED
REDACTED

func TestApplyRuntimeLogConfigOnStartup(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	cfgRaw := `{"level":"debug","enable_sampling":false,"sampling_initial":100,"sampling_thereafter":100,"caller":true,"stacktrace_level":"error","retention_days":30REDACTED`
	repo.values[SettingKeyOpsRuntimeLogConfig] = cfgRaw

	svc := &OpsService{
		settingRepo: repo,
		cfg: &config.Config{
			Log: config.LogConfig{
				Level:           "info",
				Caller:          true,
				StacktraceLevel: "error",
				Sampling: config.LogSamplingConfig{
					Enabled:    false,
					Initial:    100,
					Thereafter: 100,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	if err := logger.Init(logger.InitOptions{
		Level:       "info",
		Format:      "json",
		ServiceName: "sub2api",
		Environment: "test",
		Output: logger.OutputOptions{
			ToStdout: true,
			ToFile:   false,
	REDACTED,
REDACTED); err != nil {
		t.Fatalf("init logger: %v", err)
REDACTED

	svc.applyRuntimeLogConfigOnStartup(context.Background())
	if logger.CurrentLevel() != "debug" {
		t.Fatalf("expected startup apply debug, got %s", logger.CurrentLevel())
REDACTED
REDACTED

func TestDefaultNormalizeAndValidateRuntimeLogConfig(t *testing.T) {
	defaults := defaultOpsRuntimeLogConfig(&config.Config{
		Log: config.LogConfig{
			Level:           "DEBUG",
			Caller:          false,
			StacktraceLevel: "FATAL",
			Sampling: config.LogSamplingConfig{
				Enabled:    true,
				Initial:    50,
				Thereafter: 20,
		REDACTED,
	REDACTED,
		Ops: config.OpsConfig{
			Cleanup: config.OpsCleanupConfig{
				ErrorLogRetentionDays: 7,
		REDACTED,
	REDACTED,
REDACTED)
	if defaults.Level != "debug" || defaults.StacktraceLevel != "fatal" || defaults.RetentionDays != 7 {
		t.Fatalf("unexpected defaults: %+v", defaults)
REDACTED

	cfg := &OpsRuntimeLogConfig{
		Level:           " ",
		EnableSampling:  true,
		SamplingInitial: 0,
		SamplingNext:    -1,
		Caller:          true,
		StacktraceLevel: "",
		RetentionDays:   0,
REDACTED
	normalizeOpsRuntimeLogConfig(cfg, defaults)
	if cfg.Level != "debug" || cfg.StacktraceLevel != "fatal" {
		t.Fatalf("normalize level/stacktrace failed: %+v", cfg)
REDACTED
	if cfg.SamplingInitial != 50 || cfg.SamplingNext != 20 || cfg.RetentionDays != 7 {
		t.Fatalf("normalize numeric defaults failed: %+v", cfg)
REDACTED
	if err := validateOpsRuntimeLogConfig(cfg); err != nil {
		t.Fatalf("validate normalized config should pass: %v", err)
REDACTED
REDACTED

func TestValidateRuntimeLogConfigErrors(t *testing.T) {
	cases := []struct {
		name string
		cfg  *OpsRuntimeLogConfig
REDACTED{
		{name: "nil", cfg: nilREDACTED,
		{name: "bad level", cfg: &OpsRuntimeLogConfig{Level: "trace", StacktraceLevel: "error", SamplingInitial: 1, SamplingNext: 1, RetentionDays: 1REDACTEDREDACTED,
		{name: "bad stack", cfg: &OpsRuntimeLogConfig{Level: "info", StacktraceLevel: "warn", SamplingInitial: 1, SamplingNext: 1, RetentionDays: 1REDACTEDREDACTED,
		{name: "bad initial", cfg: &OpsRuntimeLogConfig{Level: "info", StacktraceLevel: "error", SamplingInitial: 0, SamplingNext: 1, RetentionDays: 1REDACTEDREDACTED,
		{name: "bad next", cfg: &OpsRuntimeLogConfig{Level: "info", StacktraceLevel: "error", SamplingInitial: 1, SamplingNext: 0, RetentionDays: 1REDACTEDREDACTED,
		{name: "bad retention", cfg: &OpsRuntimeLogConfig{Level: "info", StacktraceLevel: "error", SamplingInitial: 1, SamplingNext: 1, RetentionDays: 0REDACTEDREDACTED,
REDACTED
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateOpsRuntimeLogConfig(tc.cfg); err == nil {
				t.Fatalf("expected validation error")
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestGetRuntimeLogConfigFallbackAndErrors(t *testing.T) {
	var nilSvc *OpsService
	cfg, err := nilSvc.GetRuntimeLogConfig(context.Background())
	if err != nil {
		t.Fatalf("nil svc should fallback default: %v", err)
REDACTED
	if cfg.Level != "info" {
		t.Fatalf("unexpected nil svc default level: %s", cfg.Level)
REDACTED

	repo := newRuntimeSettingRepoStub()
	repo.getValueFn = func(key string) (string, error) {
		return "", errors.New("boom")
REDACTED
	svc := &OpsService{
		settingRepo: repo,
		cfg: &config.Config{
			Log: config.LogConfig{
				Level:           "warn",
				Caller:          true,
				StacktraceLevel: "error",
				Sampling: config.LogSamplingConfig{
					Enabled:    false,
					Initial:    100,
					Thereafter: 100,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	if _, err := svc.GetRuntimeLogConfig(context.Background()); err == nil {
		t.Fatalf("expected get value error")
REDACTED
REDACTED

func TestUpdateRuntimeLogConfig_PreconditionErrors(t *testing.T) {
	svc := &OpsService{REDACTED
	if _, err := svc.UpdateRuntimeLogConfig(context.Background(), &OpsRuntimeLogConfig{REDACTED, 1); err == nil {
		t.Fatalf("expected setting repo not initialized")
REDACTED

	svc = &OpsService{settingRepo: newRuntimeSettingRepoStub()REDACTED
	if _, err := svc.UpdateRuntimeLogConfig(context.Background(), nil, 1); err == nil {
		t.Fatalf("expected invalid config")
REDACTED
	if _, err := svc.UpdateRuntimeLogConfig(context.Background(), &OpsRuntimeLogConfig{
		Level:           "info",
		StacktraceLevel: "error",
		SamplingInitial: 1,
		SamplingNext:    1,
		RetentionDays:   1,
REDACTED, 0); err == nil {
		t.Fatalf("expected invalid operator")
REDACTED
REDACTED

func TestUpdateRuntimeLogConfig_Success(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	svc := &OpsService{
		settingRepo: repo,
		cfg: &config.Config{
			Log: config.LogConfig{
				Level:           "info",
				Caller:          true,
				StacktraceLevel: "error",
				Sampling: config.LogSamplingConfig{
					Enabled:    false,
					Initial:    100,
					Thereafter: 100,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	if err := logger.Init(logger.InitOptions{
		Level:       "info",
		Format:      "json",
		ServiceName: "sub2api",
		Environment: "test",
		Output: logger.OutputOptions{
			ToStdout: true,
			ToFile:   false,
	REDACTED,
REDACTED); err != nil {
		t.Fatalf("init logger: %v", err)
REDACTED

	next, err := svc.UpdateRuntimeLogConfig(context.Background(), &OpsRuntimeLogConfig{
		Level:           "debug",
		EnableSampling:  false,
		SamplingInitial: 100,
		SamplingNext:    100,
		Caller:          true,
		StacktraceLevel: "error",
		RetentionDays:   30,
REDACTED, 2)
	if err != nil {
		t.Fatalf("UpdateRuntimeLogConfig() error: %v", err)
REDACTED
	if next.Source != "runtime_setting" || next.UpdatedByUserID != 2 || next.UpdatedAt == "" {
		t.Fatalf("unexpected metadata: %+v", next)
REDACTED
	if logger.CurrentLevel() != "debug" {
		t.Fatalf("expected applied level debug, got %s", logger.CurrentLevel())
REDACTED
REDACTED

func TestResetRuntimeLogConfig_IgnoreNotFoundDelete(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	repo.deleteFn = func(key string) error { return ErrSettingNotFound REDACTED
	svc := &OpsService{
		settingRepo: repo,
		cfg: &config.Config{
			Log: config.LogConfig{
				Level:           "info",
				Caller:          true,
				StacktraceLevel: "error",
				Sampling: config.LogSamplingConfig{
					Enabled:    false,
					Initial:    100,
					Thereafter: 100,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	if _, err := svc.ResetRuntimeLogConfig(context.Background(), 1); err != nil {
		t.Fatalf("reset should ignore ErrSettingNotFound: %v", err)
REDACTED
REDACTED

func TestApplyRuntimeLogConfigHelpers(t *testing.T) {
	if err := applyOpsRuntimeLogConfig(nil); err == nil {
		t.Fatalf("expected nil config error")
REDACTED

	normalizeOpsRuntimeLogConfig(nil, &OpsRuntimeLogConfig{Level: "info"REDACTED)
	normalizeOpsRuntimeLogConfig(&OpsRuntimeLogConfig{Level: "debug"REDACTED, nil)

	var nilSvc *OpsService
	nilSvc.applyRuntimeLogConfigOnStartup(context.Background())
REDACTED
