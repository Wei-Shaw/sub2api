package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

const (
	// settingKeyAsyncMediaRuntimeConfig 是异步媒体 reconciler 运行时配置的 setting key。
	settingKeyAsyncMediaRuntimeConfig = "async_media_runtime_config"

	// reconciler 扫描间隔的允许区间（秒）。
	minReconcileIntervalSeconds = 1
	maxReconcileIntervalSeconds = 3600
	// 任务强制判失（退费兜底）时间的允许区间（秒）。
	minFailTimeoutSeconds = 60
	maxFailTimeoutSeconds = 86400
)

var (
	// ErrInvalidReconcileInterval 表示扫描间隔超出允许范围。
	ErrInvalidReconcileInterval = fmt.Errorf("reconcile_interval_seconds must be between %d and %d", minReconcileIntervalSeconds, maxReconcileIntervalSeconds)
	// ErrInvalidFailTimeout 表示失败兜底时间超出允许范围。
	ErrInvalidFailTimeout = fmt.Errorf("fail_timeout_seconds must be between %d and %d", minFailTimeoutSeconds, maxFailTimeoutSeconds)
)

// AsyncMediaRuntimeConfig 是可在后台运行时调整的异步媒体对账参数。
type AsyncMediaRuntimeConfig struct {
	// ReconcileIntervalSeconds reconciler 扫描未终结任务的间隔（秒）。
	ReconcileIntervalSeconds int `json:"reconcile_interval_seconds"`
	// FailTimeoutSeconds 任务从创建到强制判失（退费兜底）的最长时间（秒）。
	// 注意：调整后仅对之后新提交的任务生效（fail_deadline_at 在提交时固化）。
	FailTimeoutSeconds int `json:"fail_timeout_seconds"`
}

// AsyncMediaConfigService 负责异步媒体 reconciler 运行时配置的持久化（DB 设置）、
// 启动加载与运行时热更新（动态重置 reconciler ticker 与 service 失败兜底时间）。
type AsyncMediaConfigService struct {
	settingRepo SettingRepository
	svc         *AsyncMediaService
	reconciler  *AsyncMediaReconciler
}

// NewAsyncMediaConfigService 创建运行时配置服务。
func NewAsyncMediaConfigService(
	settingRepo SettingRepository,
	svc *AsyncMediaService,
	reconciler *AsyncMediaReconciler,
) *AsyncMediaConfigService {
	return &AsyncMediaConfigService{
		settingRepo: settingRepo,
		svc:         svc,
		reconciler:  reconciler,
	}
}

// LoadAndApply 在启动时读取 DB 中的运行时配置并覆盖静态配置；未配置则保持现状。
func (s *AsyncMediaConfigService) LoadAndApply(ctx context.Context) {
	cfg, ok, err := s.loadFromDB(ctx)
	if err != nil {
		logger.L().Warn("async_media.runtime_config_load_failed", zap.Error(err))
		return
	}
	if !ok {
		return
	}
	s.apply(cfg)
	logger.L().Info("async_media.runtime_config_applied",
		zap.Int("reconcile_interval_seconds", cfg.ReconcileIntervalSeconds),
		zap.Int("fail_timeout_seconds", cfg.FailTimeoutSeconds))
}

// GetConfig 返回当前生效的运行时配置（来自运行中的 reconciler / service）。
func (s *AsyncMediaConfigService) GetConfig(_ context.Context) (*AsyncMediaRuntimeConfig, error) {
	cfg := &AsyncMediaRuntimeConfig{}
	if s.reconciler != nil {
		cfg.ReconcileIntervalSeconds = int(s.reconciler.Interval() / time.Second)
	}
	if s.svc != nil {
		cfg.FailTimeoutSeconds = int(s.svc.FailTimeout() / time.Second)
	}
	return cfg, nil
}

// UpdateConfig 校验并持久化运行时配置，同时热更新运行中的 reconciler / service。
func (s *AsyncMediaConfigService) UpdateConfig(ctx context.Context, cfg AsyncMediaRuntimeConfig) (*AsyncMediaRuntimeConfig, error) {
	if cfg.ReconcileIntervalSeconds < minReconcileIntervalSeconds || cfg.ReconcileIntervalSeconds > maxReconcileIntervalSeconds {
		return nil, ErrInvalidReconcileInterval
	}
	if cfg.FailTimeoutSeconds < minFailTimeoutSeconds || cfg.FailTimeoutSeconds > maxFailTimeoutSeconds {
		return nil, ErrInvalidFailTimeout
	}

	data, err := json.Marshal(&cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal async media config: %w", err)
	}
	if err := s.settingRepo.Set(ctx, settingKeyAsyncMediaRuntimeConfig, string(data)); err != nil {
		return nil, fmt.Errorf("persist async media config: %w", err)
	}

	s.apply(cfg)
	return s.GetConfig(ctx)
}

// apply 将配置热更新到运行中的 reconciler 与 service。
func (s *AsyncMediaConfigService) apply(cfg AsyncMediaRuntimeConfig) {
	if s.reconciler != nil && cfg.ReconcileIntervalSeconds > 0 {
		s.reconciler.SetInterval(time.Duration(cfg.ReconcileIntervalSeconds) * time.Second)
	}
	if s.svc != nil && cfg.FailTimeoutSeconds > 0 {
		s.svc.SetFailTimeout(time.Duration(cfg.FailTimeoutSeconds) * time.Second)
	}
}

// loadFromDB 读取 DB 中的运行时配置。返回 (cfg, 是否已配置, error)。
func (s *AsyncMediaConfigService) loadFromDB(ctx context.Context) (AsyncMediaRuntimeConfig, bool, error) {
	var cfg AsyncMediaRuntimeConfig
	raw, err := s.settingRepo.GetValue(ctx, settingKeyAsyncMediaRuntimeConfig)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return cfg, false, nil
		}
		return cfg, false, err
	}
	if raw == "" {
		return cfg, false, nil
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return cfg, false, fmt.Errorf("unmarshal async media config: %w", err)
	}
	return cfg, true, nil
}
