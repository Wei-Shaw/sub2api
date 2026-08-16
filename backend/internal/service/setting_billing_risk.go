package service

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	billingRiskSettingsCacheTTL   = 30 * time.Second
	billingRiskSettingsErrorTTL   = 5 * time.Second
	billingRiskSettingsDBTimeout  = 3 * time.Second
	billingRiskSettingsRefreshKey = "billing_risk_settings"
)

type cachedBillingRiskSettings struct {
	settings  BillingRiskSettings
	expiresAt int64
}

// BillingRiskSettings 是网关热路径读取的余额风险租约配置快照。
type BillingRiskSettings struct {
	Enabled                  bool
	LowBalanceThreshold      float64
	SafetyFactor             float64
	MinimumRequestRisk       float64
	OverdraftAllowance       float64
	HighCostTrigger          float64
	LeaseTTLSeconds          int
	RefreshIntervalSeconds   int
	UncertainCooldownSeconds int
	VideoLeaseTTLSeconds     int
	IdleBalanceTTLSeconds    int
}

func DefaultBillingRiskSettings() BillingRiskSettings {
	return BillingRiskSettings{
		Enabled:                  false,
		LowBalanceThreshold:      10,
		SafetyFactor:             1.25,
		MinimumRequestRisk:       0.001,
		OverdraftAllowance:       0.20,
		HighCostTrigger:          1,
		LeaseTTLSeconds:          60,
		RefreshIntervalSeconds:   15,
		UncertainCooldownSeconds: 300,
		VideoLeaseTTLSeconds:     86400,
		IdleBalanceTTLSeconds:    120,
	}
}

func NormalizeBillingRiskSettings(settings BillingRiskSettings) (BillingRiskSettings, error) {
	amounts := []struct {
		name  string
		value float64
		min   float64
	}{
		{name: "low_balance_threshold", value: settings.LowBalanceThreshold, min: 0},
		{name: "safety_factor", value: settings.SafetyFactor, min: 1},
		{name: "minimum_request_risk", value: settings.MinimumRequestRisk, min: 0},
		{name: "overdraft_allowance", value: settings.OverdraftAllowance, min: 0},
		{name: "high_cost_trigger", value: settings.HighCostTrigger, min: 0},
	}
	for _, amount := range amounts {
		if math.IsNaN(amount.value) || math.IsInf(amount.value, 0) || amount.value < amount.min {
			return BillingRiskSettings{}, fmt.Errorf("%s must be finite and >= %g", amount.name, amount.min)
		}
	}
	if settings.RefreshIntervalSeconds <= 0 {
		return BillingRiskSettings{}, fmt.Errorf("refresh_interval_seconds must be positive")
	}
	if settings.LeaseTTLSeconds <= 2*settings.RefreshIntervalSeconds {
		return BillingRiskSettings{}, fmt.Errorf("lease_ttl_seconds must be greater than two refresh intervals")
	}
	if settings.UncertainCooldownSeconds <= 0 {
		return BillingRiskSettings{}, fmt.Errorf("uncertain_cooldown_seconds must be positive")
	}
	if settings.VideoLeaseTTLSeconds < settings.LeaseTTLSeconds {
		return BillingRiskSettings{}, fmt.Errorf("video_lease_ttl_seconds must not be shorter than lease_ttl_seconds")
	}
	if settings.IdleBalanceTTLSeconds <= 0 {
		return BillingRiskSettings{}, fmt.Errorf("idle_balance_ttl_seconds must be positive")
	}
	return settings, nil
}

func billingRiskSettingsFromSystem(settings *SystemSettings) BillingRiskSettings {
	if settings == nil {
		return DefaultBillingRiskSettings()
	}
	if !settings.BillingRiskEnabled &&
		settings.BillingRiskLowBalanceThreshold == 0 && settings.BillingRiskSafetyFactor == 0 &&
		settings.BillingRiskMinimumRequestRisk == 0 && settings.BillingRiskOverdraftAllowance == 0 &&
		settings.BillingRiskHighCostTrigger == 0 && settings.BillingRiskLeaseTTLSeconds == 0 &&
		settings.BillingRiskRefreshIntervalSeconds == 0 && settings.BillingRiskUncertainCooldownSeconds == 0 &&
		settings.BillingRiskVideoLeaseTTLSeconds == 0 && settings.BillingRiskIdleBalanceTTLSeconds == 0 {
		return DefaultBillingRiskSettings()
	}
	return BillingRiskSettings{
		Enabled:                  settings.BillingRiskEnabled,
		LowBalanceThreshold:      settings.BillingRiskLowBalanceThreshold,
		SafetyFactor:             settings.BillingRiskSafetyFactor,
		MinimumRequestRisk:       settings.BillingRiskMinimumRequestRisk,
		OverdraftAllowance:       settings.BillingRiskOverdraftAllowance,
		HighCostTrigger:          settings.BillingRiskHighCostTrigger,
		LeaseTTLSeconds:          settings.BillingRiskLeaseTTLSeconds,
		RefreshIntervalSeconds:   settings.BillingRiskRefreshIntervalSeconds,
		UncertainCooldownSeconds: settings.BillingRiskUncertainCooldownSeconds,
		VideoLeaseTTLSeconds:     settings.BillingRiskVideoLeaseTTLSeconds,
		IdleBalanceTTLSeconds:    settings.BillingRiskIdleBalanceTTLSeconds,
	}
}

func applyBillingRiskSettingsToSystem(target *SystemSettings, settings BillingRiskSettings) {
	if target == nil {
		return
	}
	target.BillingRiskEnabled = settings.Enabled
	target.BillingRiskLowBalanceThreshold = settings.LowBalanceThreshold
	target.BillingRiskSafetyFactor = settings.SafetyFactor
	target.BillingRiskMinimumRequestRisk = settings.MinimumRequestRisk
	target.BillingRiskOverdraftAllowance = settings.OverdraftAllowance
	target.BillingRiskHighCostTrigger = settings.HighCostTrigger
	target.BillingRiskLeaseTTLSeconds = settings.LeaseTTLSeconds
	target.BillingRiskRefreshIntervalSeconds = settings.RefreshIntervalSeconds
	target.BillingRiskUncertainCooldownSeconds = settings.UncertainCooldownSeconds
	target.BillingRiskVideoLeaseTTLSeconds = settings.VideoLeaseTTLSeconds
	target.BillingRiskIdleBalanceTTLSeconds = settings.IdleBalanceTTLSeconds
}

func parseBillingRiskSettings(values map[string]string) BillingRiskSettings {
	settings := DefaultBillingRiskSettings()
	if raw, ok := values[SettingKeyBillingRiskEnabled]; ok && strings.TrimSpace(raw) != "" {
		settings.Enabled = strings.EqualFold(strings.TrimSpace(raw), "true")
	}
	parseFloat := func(key string, target *float64) {
		if value, err := strconv.ParseFloat(strings.TrimSpace(values[key]), 64); err == nil {
			*target = value
		}
	}
	parseInt := func(key string, target *int) {
		if value, err := strconv.Atoi(strings.TrimSpace(values[key])); err == nil {
			*target = value
		}
	}
	parseFloat(SettingKeyBillingRiskLowBalanceThreshold, &settings.LowBalanceThreshold)
	parseFloat(SettingKeyBillingRiskSafetyFactor, &settings.SafetyFactor)
	parseFloat(SettingKeyBillingRiskMinimumRequestRisk, &settings.MinimumRequestRisk)
	parseFloat(SettingKeyBillingRiskOverdraftAllowance, &settings.OverdraftAllowance)
	parseFloat(SettingKeyBillingRiskHighCostTrigger, &settings.HighCostTrigger)
	parseInt(SettingKeyBillingRiskLeaseTTLSeconds, &settings.LeaseTTLSeconds)
	parseInt(SettingKeyBillingRiskRefreshIntervalSeconds, &settings.RefreshIntervalSeconds)
	parseInt(SettingKeyBillingRiskUncertainCooldownSeconds, &settings.UncertainCooldownSeconds)
	parseInt(SettingKeyBillingRiskVideoLeaseTTLSeconds, &settings.VideoLeaseTTLSeconds)
	parseInt(SettingKeyBillingRiskIdleBalanceTTLSeconds, &settings.IdleBalanceTTLSeconds)

	normalized, err := NormalizeBillingRiskSettings(settings)
	if err != nil {
		return DefaultBillingRiskSettings()
	}
	return normalized
}

func (s *SettingService) storeBillingRiskSettings(settings BillingRiskSettings) {
	if s == nil {
		return
	}
	s.billingRiskSettingsMu.Lock()
	s.billingRiskSettingsVersion++
	s.storeBillingRiskSettingsCache(settings, billingRiskSettingsCacheTTL)
	s.billingRiskSettingsMu.Unlock()
	s.billingRiskSettingsSF.Forget(billingRiskSettingsRefreshKey)
}

func (s *SettingService) storeBillingRiskSettingsCache(settings BillingRiskSettings, ttl time.Duration) {
	s.billingRiskSettingsCache.Store(&cachedBillingRiskSettings{
		settings:  settings,
		expiresAt: time.Now().Add(ttl).UnixNano(),
	})
}

// GetBillingRiskSettings 只在热路径读取原子快照；过期时后台刷新并继续返回旧值。
func (s *SettingService) GetBillingRiskSettings() BillingRiskSettings {
	if s == nil {
		return DefaultBillingRiskSettings()
	}
	cached, _ := s.billingRiskSettingsCache.Load().(*cachedBillingRiskSettings)
	if cached != nil && time.Now().UnixNano() < cached.expiresAt {
		return cached.settings
	}
	if s.settingRepo != nil {
		s.billingRiskSettingsSF.DoChan(billingRiskSettingsRefreshKey, func() (any, error) {
			if err := s.refreshBillingRiskSettings(context.Background()); err != nil {
				slog.Warn("刷新余额风险设置失败", "error", err)
			}
			return nil, nil
		})
	}
	if cached != nil {
		return cached.settings
	}
	return DefaultBillingRiskSettings()
}

func (s *SettingService) refreshBillingRiskSettings(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	} else {
		ctx = context.WithoutCancel(ctx)
	}
	dbCtx, cancel := context.WithTimeout(ctx, billingRiskSettingsDBTimeout)
	defer cancel()
	return s.LoadBillingRiskSettings(dbCtx)
}

// LoadBillingRiskSettings 在启动阶段从持久化设置加载一次运行时快照。
func (s *SettingService) LoadBillingRiskSettings(ctx context.Context) error {
	if s == nil || s.settingRepo == nil {
		return nil
	}
	s.billingRiskSettingsMu.Lock()
	version := s.billingRiskSettingsVersion
	s.billingRiskSettingsMu.Unlock()
	values, err := s.settingRepo.GetMultiple(ctx, billingRiskSettingKeys())
	if err != nil {
		s.billingRiskSettingsMu.Lock()
		if version == s.billingRiskSettingsVersion {
			fallback := DefaultBillingRiskSettings()
			if cached, _ := s.billingRiskSettingsCache.Load().(*cachedBillingRiskSettings); cached != nil {
				fallback = cached.settings
			}
			s.storeBillingRiskSettingsCache(fallback, billingRiskSettingsErrorTTL)
		}
		s.billingRiskSettingsMu.Unlock()
		return err
	}
	s.billingRiskSettingsMu.Lock()
	if version == s.billingRiskSettingsVersion {
		s.storeBillingRiskSettingsCache(parseBillingRiskSettings(values), billingRiskSettingsCacheTTL)
	}
	s.billingRiskSettingsMu.Unlock()
	return nil
}

func billingRiskSettingKeys() []string {
	return []string{
		SettingKeyBillingRiskEnabled,
		SettingKeyBillingRiskLowBalanceThreshold,
		SettingKeyBillingRiskSafetyFactor,
		SettingKeyBillingRiskMinimumRequestRisk,
		SettingKeyBillingRiskOverdraftAllowance,
		SettingKeyBillingRiskHighCostTrigger,
		SettingKeyBillingRiskLeaseTTLSeconds,
		SettingKeyBillingRiskRefreshIntervalSeconds,
		SettingKeyBillingRiskUncertainCooldownSeconds,
		SettingKeyBillingRiskVideoLeaseTTLSeconds,
		SettingKeyBillingRiskIdleBalanceTTLSeconds,
	}
}

func validateBillingRiskSystemSettings(settings *SystemSettings) (BillingRiskSettings, error) {
	normalized, err := NormalizeBillingRiskSettings(billingRiskSettingsFromSystem(settings))
	if err != nil {
		return BillingRiskSettings{}, infraerrors.BadRequest("INVALID_BILLING_RISK_SETTINGS", err.Error())
	}
	applyBillingRiskSettingsToSystem(settings, normalized)
	return normalized, nil
}
