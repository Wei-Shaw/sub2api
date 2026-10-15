package service

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	OpenAIAutoResetModeExpiring                       = "expiring"
	OpenAIAutoResetCreditModeExtraKey                 = "auto_reset_credit_mode"
	OpenAIAutoResetCreditExpiryHorizonExtraKey        = "auto_reset_credit_expiry_horizon_seconds"
	OpenAIAutoResetCreditExpiryMinUtilizationExtraKey = "auto_reset_credit_expiry_min_utilization"
	OpenAIAutoResetModeThreshold                      = "threshold"
	OpenAIAutoResetModeExhausted                      = "exhausted"
	OpenAIAutoResetModeExpiringOrExhausted            = "expiring_or_exhausted"
	OpenAIAutoResetCreditEnabledExtraKey              = "auto_reset_credit_enabled"
	OpenAIAutoResetCredit5hThresholdExtraKey          = "auto_reset_credit_5h_threshold"
	OpenAIAutoResetCredit7dThresholdExtraKey          = "auto_reset_credit_7d_threshold"
	OpenAIAutoResetCreditStateExtraKey                = "codex_auto_reset_credit_state"

	openAIAutoResetCreditDefaultThreshold = 1.0
	openAIAutoResetCreditMinimumThreshold = 0.001
)

// OpenAIAutoResetCreditConfig 是账号级自动用卡配置。阈值采用 0-1 比例，
// 避免后端调度与前端百分比展示混用同一数值语义。
type OpenAIAutoResetCreditConfig struct {
	Mode                 string
	ExpiryHorizonSeconds int
	ExpiryMinUtilization float64
	Enabled              bool
	Threshold5h          float64
	Threshold7d          float64
}

// ResolveOpenAIAutoResetCreditConfig 只接受 OpenAI OAuth 母账号；历史账号未配置时
// 始终保持关闭，防止升级后产生意外消费。
func ResolveOpenAIAutoResetCreditConfig(account *Account) OpenAIAutoResetCreditConfig {
	config := OpenAIAutoResetCreditConfig{
		Mode:                 OpenAIAutoResetModeThreshold,
		ExpiryHorizonSeconds: 3600,
		ExpiryMinUtilization: .25,
		Threshold5h:          openAIAutoResetCreditDefaultThreshold,
		Threshold7d:          openAIAutoResetCreditDefaultThreshold,
	}
	if !isOpenAIAutoResetCreditAccount(account) || account.Extra == nil {
		return config
	}
	if raw, exists := account.Extra[OpenAIAutoResetCreditModeExtraKey]; exists {
		mode, ok := raw.(string)
		if !ok || !validOpenAIAutoResetMode(mode) {
			return config
		}
		config.Mode = mode
	}
	if v, ok := resolveAccountExtraNumber(account.Extra, OpenAIAutoResetCreditExpiryHorizonExtraKey); ok && v >= 60 && v <= 604800 && math.Trunc(v) == v {
		config.ExpiryHorizonSeconds = int(v)
	}
	if v, ok := resolveAccountExtraNumber(account.Extra, OpenAIAutoResetCreditExpiryMinUtilizationExtraKey); ok && isValidOpenAIAutoResetThreshold(v) {
		config.ExpiryMinUtilization = v
	}
	config.Enabled = resolveAccountExtraBool(account.Extra, OpenAIAutoResetCreditEnabledExtraKey)
	if value, ok := resolveAccountExtraNumber(account.Extra, OpenAIAutoResetCredit5hThresholdExtraKey); ok && isValidOpenAIAutoResetThreshold(value) {
		config.Threshold5h = value
	}
	if value, ok := resolveAccountExtraNumber(account.Extra, OpenAIAutoResetCredit7dThresholdExtraKey); ok && isValidOpenAIAutoResetThreshold(value) {
		config.Threshold7d = value
	}
	return config
}

func isOpenAIAutoResetCreditAccount(account *Account) bool {
	return account != nil && account.Platform == PlatformOpenAI && account.Type == AccountTypeOAuth && !account.IsShadow()
}

// normalizeOpenAIAutoResetCreditExtra 校验管理请求中的配置并剥离服务运行态。
// enabled=true 时补齐两个 100% 默认阈值；关闭时保留已设置阈值，方便再次开启。
func normalizeOpenAIAutoResetCreditExtra(platform, accountType string, isShadow bool, extra map[string]any) (map[string]any, error) {
	if extra == nil {
		return nil, nil
	}
	normalized := cloneOpenAIAutoResetExtra(extra)
	delete(normalized, OpenAIAutoResetCreditStateExtraKey)

	_, hasEnabled := normalized[OpenAIAutoResetCreditEnabledExtraKey]
	_, has5h := normalized[OpenAIAutoResetCredit5hThresholdExtraKey]
	_, has7d := normalized[OpenAIAutoResetCredit7dThresholdExtraKey]
	_, hasMode := normalized[OpenAIAutoResetCreditModeExtraKey]
	_, hasHorizon := normalized[OpenAIAutoResetCreditExpiryHorizonExtraKey]
	_, hasMin := normalized[OpenAIAutoResetCreditExpiryMinUtilizationExtraKey]
	if !hasEnabled && !has5h && !has7d && !hasMode && !hasHorizon && !hasMin {
		return normalized, nil
	}
	if platform != PlatformOpenAI || accountType != AccountTypeOAuth || isShadow {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_AUTO_RESET_CREDIT_ACCOUNT_INVALID", "automatic reset credits are only supported for OpenAI OAuth parent accounts")
	}

	if hasMode {
		mode, ok := normalized[OpenAIAutoResetCreditModeExtraKey].(string)
		if !ok || !validOpenAIAutoResetMode(mode) {
			return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_AUTO_RESET_MODE_INVALID", "invalid auto reset mode")
		}
	}
	if hasHorizon {
		v, ok := parseOpenAIAutoResetThreshold(normalized[OpenAIAutoResetCreditExpiryHorizonExtraKey])
		if !ok || math.IsNaN(v) || v < 60 || v > 604800 || math.Trunc(v) != v {
			return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_AUTO_RESET_EXPIRY_HORIZON_INVALID", "expiry horizon must be integer seconds between 60 and 604800")
		}
		normalized[OpenAIAutoResetCreditExpiryHorizonExtraKey] = int(v)
	}
	if hasMin {
		v, ok := parseOpenAIAutoResetThreshold(normalized[OpenAIAutoResetCreditExpiryMinUtilizationExtraKey])
		if !ok || !isValidOpenAIAutoResetThreshold(v) {
			return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_AUTO_RESET_EXPIRY_MIN_INVALID", "expiry minimum utilization must be between 0.001 and 1")
		}
		normalized[OpenAIAutoResetCreditExpiryMinUtilizationExtraKey] = v
	}
	enabled := false
	if hasEnabled {
		value, ok := normalized[OpenAIAutoResetCreditEnabledExtraKey].(bool)
		if !ok {
			return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_AUTO_RESET_CREDIT_ENABLED_INVALID", "auto_reset_credit_enabled must be a boolean")
		}
		enabled = value
	}
	for key, present := range map[string]bool{
		OpenAIAutoResetCredit5hThresholdExtraKey: has5h,
		OpenAIAutoResetCredit7dThresholdExtraKey: has7d,
	} {
		if !present {
			if enabled {
				normalized[key] = openAIAutoResetCreditDefaultThreshold
			}
			continue
		}
		value, ok := parseOpenAIAutoResetThreshold(normalized[key])
		if !ok || !isValidOpenAIAutoResetThreshold(value) {
			return nil, infraerrors.Newf(http.StatusBadRequest, "OPENAI_AUTO_RESET_CREDIT_THRESHOLD_INVALID", "%s must be between 0.001 and 1.0", key)
		}
		normalized[key] = value
	}
	return normalized, nil
}

func stripOpenAIAutoResetCreditManagedExtra(extra map[string]any, stripConfig bool) map[string]any {
	if extra == nil {
		return nil
	}
	delete(extra, OpenAIAutoResetCreditStateExtraKey)
	if stripConfig {
		delete(extra, OpenAIAutoResetCreditModeExtraKey)
		delete(extra, OpenAIAutoResetCreditExpiryHorizonExtraKey)
		delete(extra, OpenAIAutoResetCreditExpiryMinUtilizationExtraKey)
		delete(extra, OpenAIAutoResetCreditEnabledExtraKey)
		delete(extra, OpenAIAutoResetCredit5hThresholdExtraKey)
		delete(extra, OpenAIAutoResetCredit7dThresholdExtraKey)
	}
	return extra
}

func parseOpenAIAutoResetThreshold(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func isValidOpenAIAutoResetThreshold(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= openAIAutoResetCreditMinimumThreshold && value <= 1
}

func cloneOpenAIAutoResetExtra(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func validOpenAIAutoResetMode(mode string) bool {
	return mode == OpenAIAutoResetModeExpiring || mode == OpenAIAutoResetModeThreshold || mode == OpenAIAutoResetModeExhausted || mode == OpenAIAutoResetModeExpiringOrExhausted
}
