package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ModelDowngradePair 一条显式的"降级对"：发往上游的模型 → 上游成功响应自报的模型。
// 只做 TrimSpace + 大小写不敏感的精确匹配，不做版本号自动比较，避免把别名
// （例如 codex-auto-review → gpt-5.6-luna）和升级（gpt-5.6-sol → gpt-6-sol）误判成降级。
type ModelDowngradePair struct {
	// SentModel 发往上游的模型
	SentModel string `json:"sent_model"`
	// ResponseModel 上游成功响应自报的模型
	ResponseModel string `json:"response_model"`
}

// ModelDowngradeGuardSettings 模型降级守卫配置
type ModelDowngradeGuardSettings struct {
	// Enabled 总开关，默认关闭
	Enabled bool `json:"enabled"`
	// Action 命中阈值后的动作: "model_block" | "temp_unsched" | "none"
	Action string `json:"action"`
	// Pairs 显式配置的降级对列表
	Pairs []ModelDowngradePair `json:"pairs"`
	// ThresholdCount 触发阈值次数（1-100）
	ThresholdCount int `json:"threshold_count"`
	// ThresholdWindowMinutes 计数窗口（1-1440 分钟）
	ThresholdWindowMinutes int `json:"threshold_window_minutes"`
	// BlockHours 屏蔽/下线时长（1-72 小时），同时也是观察记录的保留时长
	BlockHours int `json:"block_hours"`
	// MaxBlockedRatio 安全阀：因降级而受限的账号占 OpenAI 活跃账号的比例上限（0-1）
	MaxBlockedRatio float64 `json:"max_blocked_ratio"`
}

// ModelDowngradeGuardAction 模型降级守卫动作常量
const (
	ModelDowngradeGuardActionModelBlock  = "model_block"  // 仅屏蔽该账号上命中的那个模型（默认）
	ModelDowngradeGuardActionTempUnsched = "temp_unsched" // 整账号临时不可调度
	ModelDowngradeGuardActionNone        = "none"         // 仅记录（观察模式）
)

// 模型降级守卫的取值边界与默认值，Get 时钳制、Set 时校验都用同一组常量。
const (
	modelDowngradeGuardMinThresholdCount   = 1
	modelDowngradeGuardMaxThresholdCount   = 100
	modelDowngradeGuardMinWindowMinutes    = 1
	modelDowngradeGuardMaxWindowMinutes    = 1440
	modelDowngradeGuardMinBlockHours       = 1
	modelDowngradeGuardMaxBlockHours       = 72
	modelDowngradeGuardMaxPairs            = 50
	modelDowngradeGuardDefaultAction       = ModelDowngradeGuardActionModelBlock
	modelDowngradeGuardDefaultSentModel    = "gpt-6-astra"
	modelDowngradeGuardDefaultRespModel    = "gpt-5.6-luna"
	modelDowngradeGuardDefaultMaxRatio     = 0.3
	modelDowngradeGuardDefaultBlockHours   = 24
	modelDowngradeGuardDefaultThreshold    = 5
	modelDowngradeGuardDefaultWindowMinute = 30
)

// DefaultModelDowngradeGuardSettings 返回默认的模型降级守卫配置（默认关闭）。
//
// 默认动作是「仅屏蔽该模型」：上游的静默降级是按（账号 × 模型）发生的，
// 同一个账号的其他模型通常仍然正常，整号下线会误伤。
func DefaultModelDowngradeGuardSettings() *ModelDowngradeGuardSettings {
	return &ModelDowngradeGuardSettings{
		Enabled: false,
		Action:  modelDowngradeGuardDefaultAction,
		Pairs: []ModelDowngradePair{
			{SentModel: modelDowngradeGuardDefaultSentModel, ResponseModel: modelDowngradeGuardDefaultRespModel},
		},
		ThresholdCount:         modelDowngradeGuardDefaultThreshold,
		ThresholdWindowMinutes: modelDowngradeGuardDefaultWindowMinute,
		BlockHours:             modelDowngradeGuardDefaultBlockHours,
		MaxBlockedRatio:        modelDowngradeGuardDefaultMaxRatio,
	}
}

// MatchPair 返回本次请求是否命中某条显式降级对。
// 匹配前统一 TrimSpace，并按大小写不敏感比较。
func (s *ModelDowngradeGuardSettings) MatchPair(sentModel, responseModel string) bool {
	if s == nil {
		return false
	}
	sentModel = strings.TrimSpace(sentModel)
	responseModel = strings.TrimSpace(responseModel)
	if sentModel == "" || responseModel == "" {
		return false
	}
	for _, pair := range s.Pairs {
		if strings.EqualFold(strings.TrimSpace(pair.SentModel), sentModel) &&
			strings.EqualFold(strings.TrimSpace(pair.ResponseModel), responseModel) {
			return true
		}
	}
	return false
}

func isValidModelDowngradeGuardAction(action string) bool {
	switch action {
	case ModelDowngradeGuardActionModelBlock, ModelDowngradeGuardActionTempUnsched, ModelDowngradeGuardActionNone:
		return true
	default:
		return false
	}
}

// GetModelDowngradeGuardSettings 获取模型降级守卫配置
//
// 与 GetStreamTimeoutSettings 一样：读不到 / 解析失败都回退默认值，并对越界值
// 做钳制修正，保证热路径永远拿到一份可用配置。
func (s *SettingService) GetModelDowngradeGuardSettings(ctx context.Context) (*ModelDowngradeGuardSettings, error) {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyModelDowngradeGuardSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return DefaultModelDowngradeGuardSettings(), nil
		}
		return nil, fmt.Errorf("get model downgrade guard settings: %w", err)
	}
	if strings.TrimSpace(value) == "" {
		return DefaultModelDowngradeGuardSettings(), nil
	}

	var settings ModelDowngradeGuardSettings
	if err := json.Unmarshal([]byte(value), &settings); err != nil {
		return DefaultModelDowngradeGuardSettings(), nil
	}

	clampModelDowngradeGuardSettings(&settings)
	return &settings, nil
}

// clampModelDowngradeGuardSettings 把越界字段钳回合法区间，并规整降级对。
func clampModelDowngradeGuardSettings(settings *ModelDowngradeGuardSettings) {
	if settings == nil {
		return
	}
	if settings.ThresholdCount < modelDowngradeGuardMinThresholdCount {
		settings.ThresholdCount = modelDowngradeGuardMinThresholdCount
	}
	if settings.ThresholdCount > modelDowngradeGuardMaxThresholdCount {
		settings.ThresholdCount = modelDowngradeGuardMaxThresholdCount
	}
	if settings.ThresholdWindowMinutes < modelDowngradeGuardMinWindowMinutes {
		settings.ThresholdWindowMinutes = modelDowngradeGuardMinWindowMinutes
	}
	if settings.ThresholdWindowMinutes > modelDowngradeGuardMaxWindowMinutes {
		settings.ThresholdWindowMinutes = modelDowngradeGuardMaxWindowMinutes
	}
	if settings.BlockHours < modelDowngradeGuardMinBlockHours {
		settings.BlockHours = modelDowngradeGuardMinBlockHours
	}
	if settings.BlockHours > modelDowngradeGuardMaxBlockHours {
		settings.BlockHours = modelDowngradeGuardMaxBlockHours
	}
	if settings.MaxBlockedRatio < 0 {
		settings.MaxBlockedRatio = 0
	}
	if settings.MaxBlockedRatio > 1 {
		settings.MaxBlockedRatio = 1
	}
	if !isValidModelDowngradeGuardAction(settings.Action) {
		settings.Action = modelDowngradeGuardDefaultAction
	}
	settings.Pairs = sanitizeModelDowngradePairs(settings.Pairs)
}

// sanitizeModelDowngradePairs 去掉空白项与重复项，并保留配置顺序。
func sanitizeModelDowngradePairs(pairs []ModelDowngradePair) []ModelDowngradePair {
	cleaned := make([]ModelDowngradePair, 0, len(pairs))
	seen := make(map[string]struct{}, len(pairs))
	for _, pair := range pairs {
		sent := strings.TrimSpace(pair.SentModel)
		response := strings.TrimSpace(pair.ResponseModel)
		if sent == "" || response == "" {
			continue
		}
		key := strings.ToLower(sent) + "\x00" + strings.ToLower(response)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		cleaned = append(cleaned, ModelDowngradePair{SentModel: sent, ResponseModel: response})
		if len(cleaned) >= modelDowngradeGuardMaxPairs {
			break
		}
	}
	return cleaned
}

// SetModelDowngradeGuardSettings 保存模型降级守卫配置
func (s *SettingService) SetModelDowngradeGuardSettings(ctx context.Context, settings *ModelDowngradeGuardSettings) error {
	if settings == nil {
		return fmt.Errorf("settings cannot be nil")
	}

	if settings.ThresholdCount < modelDowngradeGuardMinThresholdCount || settings.ThresholdCount > modelDowngradeGuardMaxThresholdCount {
		return fmt.Errorf("threshold_count must be between %d-%d", modelDowngradeGuardMinThresholdCount, modelDowngradeGuardMaxThresholdCount)
	}
	if settings.ThresholdWindowMinutes < modelDowngradeGuardMinWindowMinutes || settings.ThresholdWindowMinutes > modelDowngradeGuardMaxWindowMinutes {
		return fmt.Errorf("threshold_window_minutes must be between %d-%d", modelDowngradeGuardMinWindowMinutes, modelDowngradeGuardMaxWindowMinutes)
	}
	if settings.BlockHours < modelDowngradeGuardMinBlockHours || settings.BlockHours > modelDowngradeGuardMaxBlockHours {
		return fmt.Errorf("block_hours must be between %d-%d", modelDowngradeGuardMinBlockHours, modelDowngradeGuardMaxBlockHours)
	}
	if settings.MaxBlockedRatio < 0 || settings.MaxBlockedRatio > 1 {
		return fmt.Errorf("max_blocked_ratio must be between 0-1")
	}
	if !isValidModelDowngradeGuardAction(settings.Action) {
		return fmt.Errorf("invalid action: %s", settings.Action)
	}
	if len(settings.Pairs) > modelDowngradeGuardMaxPairs {
		return fmt.Errorf("at most %d downgrade pairs are allowed", modelDowngradeGuardMaxPairs)
	}
	// 启用时必须至少留下一条有效降级对，否则守卫永远不会命中，属于配置事故。
	pairs := sanitizeModelDowngradePairs(settings.Pairs)
	if settings.Enabled && len(pairs) == 0 {
		return fmt.Errorf("at least one downgrade pair is required when the guard is enabled")
	}

	stored := *settings
	stored.Pairs = pairs
	data, err := json.Marshal(&stored)
	if err != nil {
		return fmt.Errorf("marshal model downgrade guard settings: %w", err)
	}

	return s.settingRepo.Set(ctx, SettingKeyModelDowngradeGuardSettings, string(data))
}
