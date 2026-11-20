package service

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	OpenAIAutoResetCreditEnabledExtraKey       = "auto_reset_credit_enabled"
	OpenAIAutoResetCredit5hThresholdExtraKey   = "auto_reset_credit_5h_threshold"
	OpenAIAutoResetCredit7dThresholdExtraKey   = "auto_reset_credit_7d_threshold"
	OpenAIAutoResetCreditExpiryEnabledExtraKey = "auto_reset_credit_expiry_enabled"
	OpenAIAutoResetCreditExpiryHoursExtraKey   = "auto_reset_credit_expiry_hours"
	OpenAIAutoResetCreditStateExtraKey         = "codex_auto_reset_credit_state"

	openAIAutoResetCreditDefaultThreshold = 1.0
	openAIAutoResetCreditMinimumThreshold = 0.001
)

// OpenAIAutoResetCreditConfig 是账号级自动用卡配置。阈值采用 0-1 比例，
// 避免后端调度与前端百分比展示混用同一数值语义。
type OpenAIAutoResetCreditConfig struct {
	Enabled       bool
	Threshold5h   float64
	Threshold7d   float64
	ExpiryEnabled bool
	ExpiryHours   float64
}

// ResolveOpenAIAutoResetCreditConfig 只接受 OpenAI OAuth 母账号；历史账号未配置时
// 始终保持关闭，防止升级后产生意外消费。
func ResolveOpenAIAutoResetCreditConfig(account *Account) OpenAIAutoResetCreditConfig {
	config := OpenAIAutoResetCreditConfig{
		Threshold5h: openAIAutoResetCreditDefaultThreshold,
		Threshold7d: openAIAutoResetCreditDefaultThreshold,
		ExpiryHours: 1,
	}
	if !isOpenAIAutoResetCreditAccount(account) || account.Extra == nil {
		return config
	}
	config.Enabled = resolveAccountExtraBool(account.Extra, OpenAIAutoResetCreditEnabledExtraKey)
	config.ExpiryEnabled = resolveAccountExtraBool(account.Extra, OpenAIAutoResetCreditExpiryEnabledExtraKey)
	if value, ok := resolveAccountExtraNumber(account.Extra, OpenAIAutoResetCreditExpiryHoursExtraKey); ok && isValidOpenAIAutoResetExpiryHours(value) {
		config.ExpiryHours = value
	}
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
	_, hasExpiryEnabled := normalized[OpenAIAutoResetCreditExpiryEnabledExtraKey]
	_, hasExpiryHours := normalized[OpenAIAutoResetCreditExpiryHoursExtraKey]
	if !hasEnabled && !has5h && !has7d && !hasExpiryEnabled && !hasExpiryHours {
		return normalized, nil
	}
	if hasExpiryEnabled {
		value, ok := normalized[OpenAIAutoResetCreditExpiryEnabledExtraKey].(bool)
		if !ok {
			return nil, infraerrors.BadRequest("OPENAI_AUTO_RESET_CREDIT_EXPIRY_ENABLED_INVALID", "auto_reset_credit_expiry_enabled must be a boolean")
		}
		if value && !hasExpiryHours {
			normalized[OpenAIAutoResetCreditExpiryHoursExtraKey] = float64(1)
		}
	}
	if hasExpiryHours {
		value, ok := parseOpenAIAutoResetThreshold(normalized[OpenAIAutoResetCreditExpiryHoursExtraKey])
		if !ok || !isValidOpenAIAutoResetExpiryHours(value) {
			return nil, infraerrors.BadRequest("OPENAI_AUTO_RESET_CREDIT_EXPIRY_HOURS_INVALID", "auto_reset_credit_expiry_hours must be positive finite hours within the supported duration range")
		}
		normalized[OpenAIAutoResetCreditExpiryHoursExtraKey] = value
	}
	if platform != PlatformOpenAI || accountType != AccountTypeOAuth || isShadow {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_AUTO_RESET_CREDIT_ACCOUNT_INVALID", "automatic reset credits are only supported for OpenAI OAuth parent accounts")
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
		delete(extra, OpenAIAutoResetCreditEnabledExtraKey)
		delete(extra, OpenAIAutoResetCredit5hThresholdExtraKey)
		delete(extra, OpenAIAutoResetCredit7dThresholdExtraKey)
		delete(extra, OpenAIAutoResetCreditExpiryEnabledExtraKey)
		delete(extra, OpenAIAutoResetCreditExpiryHoursExtraKey)
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

func isValidOpenAIAutoResetExpiryHours(value float64) bool {
	// 小数小时也允许；边界只限制 time.Duration 的纳秒精度与溢出，避免任意业务上限。
	nanoseconds := value * float64(time.Hour)
	return !math.IsNaN(value) && !math.IsInf(value, 0) && nanoseconds >= 1 && nanoseconds < float64(math.MaxInt64)
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
