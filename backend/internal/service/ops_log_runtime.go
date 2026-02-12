package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

func defaultOpsRuntimeLogConfig(cfg *config.Config) *OpsRuntimeLogConfig {
	out := &OpsRuntimeLogConfig{
		Level:           "info",
		EnableSampling:  false,
		SamplingInitial: 100,
		SamplingNext:    100,
		Caller:          true,
		StacktraceLevel: "error",
		RetentionDays:   30,
REDACTED
	if cfg == nil {
		return out
REDACTED
	out.Level = strings.ToLower(strings.TrimSpace(cfg.Log.Level))
	out.EnableSampling = cfg.Log.Sampling.Enabled
	out.SamplingInitial = cfg.Log.Sampling.Initial
	out.SamplingNext = cfg.Log.Sampling.Thereafter
	out.Caller = cfg.Log.Caller
	out.StacktraceLevel = strings.ToLower(strings.TrimSpace(cfg.Log.StacktraceLevel))
	if cfg.Ops.Cleanup.ErrorLogRetentionDays > 0 {
		out.RetentionDays = cfg.Ops.Cleanup.ErrorLogRetentionDays
REDACTED
	return out
REDACTED

func normalizeOpsRuntimeLogConfig(cfg *OpsRuntimeLogConfig, defaults *OpsRuntimeLogConfig) {
	if cfg == nil || defaults == nil {
		return
REDACTED
	cfg.Level = strings.ToLower(strings.TrimSpace(cfg.Level))
	if cfg.Level == "" {
		cfg.Level = defaults.Level
REDACTED
	cfg.StacktraceLevel = strings.ToLower(strings.TrimSpace(cfg.StacktraceLevel))
	if cfg.StacktraceLevel == "" {
		cfg.StacktraceLevel = defaults.StacktraceLevel
REDACTED
	if cfg.SamplingInitial <= 0 {
		cfg.SamplingInitial = defaults.SamplingInitial
REDACTED
	if cfg.SamplingNext <= 0 {
		cfg.SamplingNext = defaults.SamplingNext
REDACTED
	if cfg.RetentionDays <= 0 {
		cfg.RetentionDays = defaults.RetentionDays
REDACTED
REDACTED

func validateOpsRuntimeLogConfig(cfg *OpsRuntimeLogConfig) error {
	if cfg == nil {
		return errors.New("invalid config")
REDACTED
	switch strings.ToLower(strings.TrimSpace(cfg.Level)) {
	case "debug", "info", "warn", "error":
	default:
		return errors.New("level must be one of: debug/info/warn/error")
REDACTED
	switch strings.ToLower(strings.TrimSpace(cfg.StacktraceLevel)) {
	case "none", "error", "fatal":
	default:
		return errors.New("stacktrace_level must be one of: none/error/fatal")
REDACTED
	if cfg.SamplingInitial <= 0 {
		return errors.New("sampling_initial must be positive")
REDACTED
	if cfg.SamplingNext <= 0 {
		return errors.New("sampling_thereafter must be positive")
REDACTED
	if cfg.RetentionDays < 1 || cfg.RetentionDays > 3650 {
		return errors.New("retention_days must be between 1 and 3650")
REDACTED
	return nil
REDACTED

func (s *OpsService) GetRuntimeLogConfig(ctx context.Context) (*OpsRuntimeLogConfig, error) {
	if s == nil || s.settingRepo == nil {
		var cfg *config.Config
		if s != nil {
			cfg = s.cfg
	REDACTED
		defaultCfg := defaultOpsRuntimeLogConfig(cfg)
		return defaultCfg, nil
REDACTED
	defaultCfg := defaultOpsRuntimeLogConfig(s.cfg)
	if ctx == nil {
		ctx = context.Background()
REDACTED

	raw, err := s.settingRepo.GetValue(ctx, SettingKeyOpsRuntimeLogConfig)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			b, _ := json.Marshal(defaultCfg)
			_ = s.settingRepo.Set(ctx, SettingKeyOpsRuntimeLogConfig, string(b))
			return defaultCfg, nil
	REDACTED
		return nil, err
REDACTED

	cfg := &OpsRuntimeLogConfig{REDACTED
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return defaultCfg, nil
REDACTED
	normalizeOpsRuntimeLogConfig(cfg, defaultCfg)
	return cfg, nil
REDACTED

func (s *OpsService) UpdateRuntimeLogConfig(ctx context.Context, req *OpsRuntimeLogConfig, operatorID int64) (*OpsRuntimeLogConfig, error) {
	if s == nil || s.settingRepo == nil {
		return nil, errors.New("setting repository not initialized")
REDACTED
	if req == nil {
		return nil, errors.New("invalid config")
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED
	if operatorID <= 0 {
		return nil, errors.New("invalid operator id")
REDACTED

	oldCfg, err := s.GetRuntimeLogConfig(ctx)
	if err != nil {
		return nil, err
REDACTED
	next := *req
	normalizeOpsRuntimeLogConfig(&next, defaultOpsRuntimeLogConfig(s.cfg))
	if err := validateOpsRuntimeLogConfig(&next); err != nil {
		s.auditRuntimeLogConfigFailure(operatorID, oldCfg, &next, "validation_failed: "+err.Error())
		return nil, err
REDACTED

	if err := applyOpsRuntimeLogConfig(&next); err != nil {
		s.auditRuntimeLogConfigFailure(operatorID, oldCfg, &next, "apply_failed: "+err.Error())
		return nil, err
REDACTED

	next.Source = "runtime_setting"
	next.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	next.UpdatedByUserID = operatorID

	encoded, err := json.Marshal(&next)
	if err != nil {
		return nil, err
REDACTED
	if err := s.settingRepo.Set(ctx, SettingKeyOpsRuntimeLogConfig, string(encoded)); err != nil {
		// 存储失败时回滚到旧配置，避免内存状态与持久化状态不一致。
		_ = applyOpsRuntimeLogConfig(oldCfg)
		s.auditRuntimeLogConfigFailure(operatorID, oldCfg, &next, "persist_failed: "+err.Error())
		return nil, err
REDACTED

	s.auditRuntimeLogConfigChange(operatorID, oldCfg, &next, "updated")

	return &next, nil
REDACTED

func (s *OpsService) ResetRuntimeLogConfig(ctx context.Context, operatorID int64) (*OpsRuntimeLogConfig, error) {
	if s == nil || s.settingRepo == nil {
		return nil, errors.New("setting repository not initialized")
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED
	if operatorID <= 0 {
		return nil, errors.New("invalid operator id")
REDACTED

	oldCfg, err := s.GetRuntimeLogConfig(ctx)
	if err != nil {
		return nil, err
REDACTED

	resetCfg := defaultOpsRuntimeLogConfig(s.cfg)
	normalizeOpsRuntimeLogConfig(resetCfg, defaultOpsRuntimeLogConfig(s.cfg))
	if err := validateOpsRuntimeLogConfig(resetCfg); err != nil {
		s.auditRuntimeLogConfigFailure(operatorID, oldCfg, resetCfg, "reset_validation_failed: "+err.Error())
		return nil, err
REDACTED
	if err := applyOpsRuntimeLogConfig(resetCfg); err != nil {
		s.auditRuntimeLogConfigFailure(operatorID, oldCfg, resetCfg, "reset_apply_failed: "+err.Error())
		return nil, err
REDACTED

	// 清理 runtime 覆盖配置，回退到 env/yaml baseline。
	if err := s.settingRepo.Delete(ctx, SettingKeyOpsRuntimeLogConfig); err != nil && !errors.Is(err, ErrSettingNotFound) {
		_ = applyOpsRuntimeLogConfig(oldCfg)
		s.auditRuntimeLogConfigFailure(operatorID, oldCfg, resetCfg, "reset_persist_failed: "+err.Error())
		return nil, err
REDACTED

	now := time.Now().UTC().Format(time.RFC3339Nano)
	resetCfg.Source = "baseline"
	resetCfg.UpdatedAt = now
	resetCfg.UpdatedByUserID = operatorID

	s.auditRuntimeLogConfigChange(operatorID, oldCfg, resetCfg, "reset")
	return resetCfg, nil
REDACTED

func applyOpsRuntimeLogConfig(cfg *OpsRuntimeLogConfig) error {
	if cfg == nil {
		return fmt.Errorf("nil runtime log config")
REDACTED
	if err := logger.Reconfigure(func(opts *logger.InitOptions) error {
		opts.Level = strings.ToLower(strings.TrimSpace(cfg.Level))
		opts.Caller = cfg.Caller
		opts.StacktraceLevel = strings.ToLower(strings.TrimSpace(cfg.StacktraceLevel))
		opts.Sampling.Enabled = cfg.EnableSampling
		opts.Sampling.Initial = cfg.SamplingInitial
		opts.Sampling.Thereafter = cfg.SamplingNext
		return nil
REDACTED); err != nil {
		return err
REDACTED
	return nil
REDACTED

func (s *OpsService) applyRuntimeLogConfigOnStartup(ctx context.Context) {
	if s == nil {
		return
REDACTED
	cfg, err := s.GetRuntimeLogConfig(ctx)
	if err != nil {
		return
REDACTED
	_ = applyOpsRuntimeLogConfig(cfg)
REDACTED

func (s *OpsService) auditRuntimeLogConfigChange(operatorID int64, oldCfg *OpsRuntimeLogConfig, newCfg *OpsRuntimeLogConfig, action string) {
	oldRaw, _ := json.Marshal(oldCfg)
	newRaw, _ := json.Marshal(newCfg)
	logger.With(
		zap.String("component", "audit.log_config_change"),
		zap.String("action", strings.TrimSpace(action)),
		zap.Int64("operator_id", operatorID),
		zap.String("old", string(oldRaw)),
		zap.String("new", string(newRaw)),
	).Info("runtime log config changed")
REDACTED

func (s *OpsService) auditRuntimeLogConfigFailure(operatorID int64, oldCfg *OpsRuntimeLogConfig, newCfg *OpsRuntimeLogConfig, reason string) {
	oldRaw, _ := json.Marshal(oldCfg)
	newRaw, _ := json.Marshal(newCfg)
	logger.With(
		zap.String("component", "audit.log_config_change"),
		zap.String("action", "failed"),
		zap.Int64("operator_id", operatorID),
		zap.String("reason", strings.TrimSpace(reason)),
		zap.String("old", string(oldRaw)),
		zap.String("new", string(newRaw)),
	).Warn("runtime log config change failed")
REDACTED
