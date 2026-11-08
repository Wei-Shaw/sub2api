package service

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// 账号级智力检测配置直接放在 accounts.extra 上，跟着账号编辑表单一起保存，
// 与 upstream_billing_probe / ollama_cloud_usage 的做法保持一致。
const (
	// IntelligenceCheckEnabledExtraKey 标记该账号是否参与智力检测；缺省视为不参与。
	IntelligenceCheckEnabledExtraKey = "intelligence_check_enabled"
	// IntelligenceCheckIntervalMinutesExtraKey 是账号级跑测间隔覆盖（分钟）；缺省跟随全局设置。
	IntelligenceCheckIntervalMinutesExtraKey = "intelligence_check_interval_minutes"
	// IntelligenceCheckModelIDExtraKey 是账号级跑测模型覆盖；缺省跟随全局设置。
	IntelligenceCheckModelIDExtraKey = "intelligence_check_model_id"
	// IntelligenceCheckReasoningEffortExtraKey 是账号级思考强度覆盖；缺省跟随全局设置。
	// 留空即「跟随全局」，与该账号的模型覆盖同一套语义。
	IntelligenceCheckReasoningEffortExtraKey = "intelligence_check_reasoning_effort"
)

// 账号级覆盖间隔的合法区间（分钟），与全局设置共用同一套边界。
const (
	IntelligenceCheckMinIntervalMinutes = 5
	IntelligenceCheckMaxIntervalMinutes = 7 * 24 * 60
)

// ValidateIntelligenceCheckExtra 校验账号级智力检测配置。
// 非法值直接拒绝，不静默落库——否则调度器只能反复读坏数据。
func ValidateIntelligenceCheckExtra(extra map[string]any) error {
	if extra == nil {
		return nil
	}
	if raw, exists := extra[IntelligenceCheckEnabledExtraKey]; exists {
		if _, ok := raw.(bool); !ok {
			return infraerrors.BadRequest(
				"INTELLIGENCE_CHECK_ENABLED_INVALID",
				"intelligence_check_enabled must be a boolean",
			)
		}
	}
	if raw, exists := extra[IntelligenceCheckIntervalMinutesExtraKey]; exists {
		minutes, ok := coerceIntelligenceCheckMinutes(raw)
		if !ok {
			return infraerrors.BadRequest(
				"INTELLIGENCE_CHECK_INTERVAL_INVALID",
				"intelligence_check_interval_minutes must be an integer",
			)
		}
		if minutes != 0 && (minutes < IntelligenceCheckMinIntervalMinutes || minutes > IntelligenceCheckMaxIntervalMinutes) {
			return infraerrors.BadRequest(
				"INTELLIGENCE_CHECK_INTERVAL_OUT_OF_RANGE",
				fmt.Sprintf("intelligence_check_interval_minutes must be between %d and %d",
					IntelligenceCheckMinIntervalMinutes, IntelligenceCheckMaxIntervalMinutes),
			)
		}
	}
	if raw, exists := extra[IntelligenceCheckModelIDExtraKey]; exists {
		if _, ok := raw.(string); !ok {
			return infraerrors.BadRequest(
				"INTELLIGENCE_CHECK_MODEL_INVALID",
				"intelligence_check_model_id must be a string",
			)
		}
	}
	if raw, exists := extra[IntelligenceCheckReasoningEffortExtraKey]; exists {
		if _, ok := raw.(string); !ok {
			return infraerrors.BadRequest(
				"INTELLIGENCE_CHECK_REASONING_EFFORT_INVALID",
				"intelligence_check_reasoning_effort must be a string",
			)
		}
	}
	return nil
}

// NormalizeIntelligenceCheckExtra 归一化账号级智力检测配置：
// 间隔与模型为空时删除对应键（语义即「跟随全局设置」），enabled 的显式 true/false 原样保留。
func NormalizeIntelligenceCheckExtra(extra map[string]any) (map[string]any, error) {
	if extra == nil {
		return nil, nil
	}
	if err := ValidateIntelligenceCheckExtra(extra); err != nil {
		return nil, err
	}

	normalized := maps.Clone(extra)
	if normalized == nil {
		return nil, nil
	}

	if raw, exists := normalized[IntelligenceCheckIntervalMinutesExtraKey]; exists {
		minutes, _ := coerceIntelligenceCheckMinutes(raw)
		if minutes <= 0 {
			delete(normalized, IntelligenceCheckIntervalMinutesExtraKey)
		} else {
			normalized[IntelligenceCheckIntervalMinutesExtraKey] = minutes
		}
	}
	if raw, exists := normalized[IntelligenceCheckModelIDExtraKey]; exists {
		modelID := ""
		if value, ok := raw.(string); ok {
			modelID = strings.TrimSpace(value)
		}
		if modelID == "" {
			delete(normalized, IntelligenceCheckModelIDExtraKey)
		} else {
			normalized[IntelligenceCheckModelIDExtraKey] = modelID
		}
	}
	if raw, exists := normalized[IntelligenceCheckReasoningEffortExtraKey]; exists {
		effort := ""
		if value, ok := raw.(string); ok {
			effort = strings.TrimSpace(value)
		}
		if effort == "" {
			delete(normalized, IntelligenceCheckReasoningEffortExtraKey)
		} else {
			normalized[IntelligenceCheckReasoningEffortExtraKey] = effort
		}
	}
	return normalized, nil
}

// IntelligenceCheckAccountConfig 是账号级智力检测配置的读取结果。
type IntelligenceCheckAccountConfig struct {
	// Enabled 表示该账号是否参与智力检测。
	Enabled bool
	// IntervalMinutes 为 0 表示跟随全局设置。
	IntervalMinutes int
	// ModelID 为空表示跟随全局设置。
	ModelID string
	// ReasoningEffort 为空表示跟随全局设置。
	ReasoningEffort string
}

// ReadIntelligenceCheckAccountConfig 从账号 extra 读取配置。
// 缺省、空值、类型不符一律回退成「未覆盖」，绝不因为脏数据阻断调度。
func ReadIntelligenceCheckAccountConfig(extra map[string]any) IntelligenceCheckAccountConfig {
	config := IntelligenceCheckAccountConfig{}
	if extra == nil {
		return config
	}
	if enabled, ok := extra[IntelligenceCheckEnabledExtraKey].(bool); ok {
		config.Enabled = enabled
	}
	if minutes, ok := coerceIntelligenceCheckMinutes(extra[IntelligenceCheckIntervalMinutesExtraKey]); ok {
		if minutes >= IntelligenceCheckMinIntervalMinutes && minutes <= IntelligenceCheckMaxIntervalMinutes {
			config.IntervalMinutes = minutes
		}
	}
	if modelID, ok := extra[IntelligenceCheckModelIDExtraKey].(string); ok {
		config.ModelID = strings.TrimSpace(modelID)
	}
	if effort, ok := extra[IntelligenceCheckReasoningEffortExtraKey].(string); ok {
		config.ReasoningEffort = strings.TrimSpace(effort)
	}
	return config
}

// coerceIntelligenceCheckMinutes 兼容 JSON 数字、Go 整数与数字字符串三种形态。
// 空值视为「未设置」并返回 ok=true，便于与「非法值」区分。
func coerceIntelligenceCheckMinutes(raw any) (int, bool) {
	switch value := raw.(type) {
	case nil:
		return 0, true
	case bool:
		return 0, false
	case int:
		return value, true
	case int32:
		return int(value), true
	case int64:
		return int(value), true
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) || value != math.Trunc(value) {
			return 0, false
		}
		return int(value), true
	case float32:
		converted := float64(value)
		if math.IsNaN(converted) || math.IsInf(converted, 0) || converted != math.Trunc(converted) {
			return 0, false
		}
		return int(converted), true
	case json.Number:
		parsed, err := value.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return 0, true
		}
		parsed, err := strconv.Atoi(trimmed)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}
