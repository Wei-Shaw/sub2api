package service

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// 智力检测（鹈鹕测试）全局配置的边界与默认值。
// 间隔区间与账号级覆盖共用同一套边界（见 intelligence_check_account_extra.go）。
const (
	// IntelligenceCheckDefaultIntervalMinutes 默认 12 小时：鹈鹕题单次开销很大
	// （长 prompt + 大 max_tokens + 长思考），跑太密既烧钱也会推高上游风控风险。
	IntelligenceCheckDefaultIntervalMinutes = 12 * 60

	IntelligenceCheckDefaultMaxConcurrency = 2
	IntelligenceCheckMinMaxConcurrency     = 1
	IntelligenceCheckMaxMaxConcurrency     = 32

	// 跑测重试：针对单次跑测的网络抖动、上游 5xx 等瞬时故障。
	IntelligenceCheckDefaultRunRetryCount       = 2
	IntelligenceCheckMaxRunRetryCount           = 10
	IntelligenceCheckDefaultRunRetryIntervalSec = 30
	IntelligenceCheckMinRunRetryIntervalSec     = 1
	IntelligenceCheckMaxRunRetryIntervalSec     = 3600

	// 账号自动恢复：账号级连续失败后进入冷却，按此间隔重试，用尽后暂停该账号。
	IntelligenceCheckDefaultAccountRetryCount       = 3
	IntelligenceCheckMaxAccountRetryCount           = 20
	IntelligenceCheckDefaultAccountRetryIntervalMin = 10
	IntelligenceCheckMinAccountRetryIntervalMin     = 1
	IntelligenceCheckMaxAccountRetryIntervalMin     = 24 * 60

	// 单次跑测的超时与输出上限。max_tokens 默认给足，否则作品会被上游截断成半张。
	IntelligenceCheckDefaultTimeoutSeconds = 180
	IntelligenceCheckMinTimeoutSeconds     = 30
	IntelligenceCheckMaxTimeoutSeconds     = 3600
	IntelligenceCheckMinMaxTokens          = 4096
	IntelligenceCheckMaxMaxTokens          = 128000

	// 每个账号保留的跑测记录条数，超出部分由 PruneRuns 裁剪。
	IntelligenceCheckDefaultMaxRunsPerAccount = 20
	IntelligenceCheckMinMaxRunsPerAccount     = 1
	IntelligenceCheckMaxMaxRunsPerAccount     = 500
)

// IntelligenceCheckGlobalSettings 是智力检测的全局配置快照。
// 读取时已完成兜底与钳制，调用方可直接使用。
type IntelligenceCheckGlobalSettings struct {
	Enabled                     bool
	IntervalMinutes             int
	ModelID                     string
	ReasoningEffort             string
	MaxConcurrency              int
	RunRetryCount               int
	RunRetryIntervalSeconds     int
	AccountRetryCount           int
	AccountRetryIntervalMinutes int
	TimeoutSeconds              int
	MaxTokens                   int
	MaxRunsPerAccount           int
	// StatusSyncEnabled 决定人工评审结论是否联动账号状态：
	// 评审为 fail 时把账号置为 error，评审为 pass 时恢复 active。
	// 默认关闭——联动会改动账号可用性，必须由管理员显式开启。
	StatusSyncEnabled bool
	// StreamEnabled 决定跑测以流式还是非流式发起。
	// 默认开启流式：上游网关（nginx 的 proxy_read_timeout 之类）按「整体响应」计时，
	// 非流式必须等作品整份生成完才返回，长思考时极易撞上网关超时；
	// 流式下每个增量到达都会重置读计时，因此默认走流式。
	StreamEnabled bool
}

// Interval 返回全局跑测间隔。
func (s IntelligenceCheckGlobalSettings) Interval() time.Duration {
	return time.Duration(s.IntervalMinutes) * time.Minute
}

// RunRetryInterval 返回单次跑测的重试间隔。
func (s IntelligenceCheckGlobalSettings) RunRetryInterval() time.Duration {
	return time.Duration(s.RunRetryIntervalSeconds) * time.Second
}

// AccountRetryInterval 返回账号自动恢复的重试间隔。
func (s IntelligenceCheckGlobalSettings) AccountRetryInterval() time.Duration {
	return time.Duration(s.AccountRetryIntervalMinutes) * time.Minute
}

// Timeout 返回单次跑测超时。
func (s IntelligenceCheckGlobalSettings) Timeout() time.Duration {
	return time.Duration(s.TimeoutSeconds) * time.Second
}

// Runnable 表示当前配置是否允许发起跑测：总开关打开且已配置模型。
// 模型留空时一律不跑——宁可留下「未配置」的清晰状态，也不要向全部上游账号发请求。
func (s IntelligenceCheckGlobalSettings) Runnable() bool {
	return s.Enabled && strings.TrimSpace(s.ModelID) != ""
}

// DefaultIntelligenceCheckGlobalSettings 返回全默认配置（无库可用时也要能跑）。
func DefaultIntelligenceCheckGlobalSettings() IntelligenceCheckGlobalSettings {
	return IntelligenceCheckGlobalSettings{
		Enabled:                     false,
		IntervalMinutes:             IntelligenceCheckDefaultIntervalMinutes,
		ModelID:                     "",
		ReasoningEffort:             "",
		MaxConcurrency:              IntelligenceCheckDefaultMaxConcurrency,
		RunRetryCount:               IntelligenceCheckDefaultRunRetryCount,
		RunRetryIntervalSeconds:     IntelligenceCheckDefaultRunRetryIntervalSec,
		AccountRetryCount:           IntelligenceCheckDefaultAccountRetryCount,
		AccountRetryIntervalMinutes: IntelligenceCheckDefaultAccountRetryIntervalMin,
		TimeoutSeconds:              IntelligenceCheckDefaultTimeoutSeconds,
		MaxTokens:                   IntelligenceCheckDefaultMaxTokens,
		MaxRunsPerAccount:           IntelligenceCheckDefaultMaxRunsPerAccount,
		StatusSyncEnabled:           false,
		StreamEnabled:               true,
	}
}

// GetIntelligenceCheckGlobalSettings 读取全局配置。
// 设置表为空（老库升级）时 defaults 表不会被回填，因此这里必须自己给出合理值，
// 否则间隔会读成 0 并导致跑测连续触发。
func (s *SettingService) GetIntelligenceCheckGlobalSettings(ctx context.Context) (IntelligenceCheckGlobalSettings, error) {
	if s == nil || s.settingRepo == nil {
		return DefaultIntelligenceCheckGlobalSettings(), nil
	}
	settings, err := s.GetAllSettings(ctx)
	if err != nil {
		return DefaultIntelligenceCheckGlobalSettings(), err
	}
	return IntelligenceCheckGlobalSettingsFrom(settings), nil
}

// IntelligenceCheckGlobalSettingsFrom 从系统设置快照提取智力检测配置，并对每个数值兜底。
// parseSettings 已经钳过一次，这里再兜一次，因为快照也可能来自测试桩或缓存。
func IntelligenceCheckGlobalSettingsFrom(settings *SystemSettings) IntelligenceCheckGlobalSettings {
	resolved := DefaultIntelligenceCheckGlobalSettings()
	if settings == nil {
		return resolved
	}
	resolved.Enabled = settings.IntelligenceCheckEnabled
	resolved.IntervalMinutes = clampIntelligenceCheckSetting(
		settings.IntelligenceCheckIntervalMinutes, resolved.IntervalMinutes,
		IntelligenceCheckMinIntervalMinutes, IntelligenceCheckMaxIntervalMinutes,
	)
	resolved.ModelID = strings.TrimSpace(settings.IntelligenceCheckModelID)
	resolved.ReasoningEffort = strings.TrimSpace(settings.IntelligenceCheckReasoningEffort)
	resolved.MaxConcurrency = clampIntelligenceCheckSetting(
		settings.IntelligenceCheckMaxConcurrency, resolved.MaxConcurrency,
		IntelligenceCheckMinMaxConcurrency, IntelligenceCheckMaxMaxConcurrency,
	)
	resolved.RunRetryCount = clampIntelligenceCheckSetting(
		settings.IntelligenceCheckRunRetryCount, resolved.RunRetryCount,
		0, IntelligenceCheckMaxRunRetryCount,
	)
	resolved.RunRetryIntervalSeconds = clampIntelligenceCheckSetting(
		settings.IntelligenceCheckRunRetryIntervalSeconds, resolved.RunRetryIntervalSeconds,
		IntelligenceCheckMinRunRetryIntervalSec, IntelligenceCheckMaxRunRetryIntervalSec,
	)
	resolved.AccountRetryCount = clampIntelligenceCheckSetting(
		settings.IntelligenceCheckAccountRetryCount, resolved.AccountRetryCount,
		0, IntelligenceCheckMaxAccountRetryCount,
	)
	resolved.AccountRetryIntervalMinutes = clampIntelligenceCheckSetting(
		settings.IntelligenceCheckAccountRetryIntervalMinutes, resolved.AccountRetryIntervalMinutes,
		IntelligenceCheckMinAccountRetryIntervalMin, IntelligenceCheckMaxAccountRetryIntervalMin,
	)
	resolved.TimeoutSeconds = clampIntelligenceCheckSetting(
		settings.IntelligenceCheckTimeoutSeconds, resolved.TimeoutSeconds,
		IntelligenceCheckMinTimeoutSeconds, IntelligenceCheckMaxTimeoutSeconds,
	)
	resolved.MaxTokens = clampIntelligenceCheckSetting(
		settings.IntelligenceCheckMaxTokens, resolved.MaxTokens,
		IntelligenceCheckMinMaxTokens, IntelligenceCheckMaxMaxTokens,
	)
	resolved.MaxRunsPerAccount = clampIntelligenceCheckSetting(
		settings.IntelligenceCheckMaxRunsPerAccount, resolved.MaxRunsPerAccount,
		IntelligenceCheckMinMaxRunsPerAccount, IntelligenceCheckMaxMaxRunsPerAccount,
	)
	resolved.StatusSyncEnabled = settings.IntelligenceCheckStatusSyncEnabled
	resolved.StreamEnabled = settings.IntelligenceCheckStreamEnabled
	return resolved
}

// clampIntelligenceCheckSetting 越界即回退默认值：宁可回到已知安全值，也不钳到边界后
// 留下一个「看起来配过了」的假配置。
func clampIntelligenceCheckSetting(value, fallback, minValue, maxValue int) int {
	if value < minValue || value > maxValue {
		return fallback
	}
	return value
}

// parseIntelligenceCheckSetting 供 parseSettings 解析字符串设置项。
func parseIntelligenceCheckSetting(raw string, fallback, minValue, maxValue int) int {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return fallback
	}
	return clampIntelligenceCheckSetting(parsed, fallback, minValue, maxValue)
}
