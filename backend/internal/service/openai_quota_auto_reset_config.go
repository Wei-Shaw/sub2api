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
	OpenAIAutoResetCreditEnabledExtraKey           = "auto_reset_credit_enabled"
	OpenAIAutoResetCredit5hThresholdExtraKey       = "auto_reset_credit_5h_threshold"
	OpenAIAutoResetCredit7dThresholdExtraKey       = "auto_reset_credit_7d_threshold"
	OpenAIAutoResetCreditExpiryEnabledExtraKey     = "auto_reset_credit_expiry_enabled"
	OpenAIAutoResetCreditExpiryLeadMinutesExtraKey = "auto_reset_credit_expiry_lead_minutes"
	OpenAIAutoResetCreditStateExtraKey             = "codex_auto_reset_credit_state"
	OpenAIAutoResetCreditExpiryAtExtraKey          = "codex_auto_reset_credit_expiry_at"

	openAIAutoResetCreditDefaultThreshold     = 1.0
	openAIAutoResetCreditDefault5hThreshold   = 0.0
	openAIAutoResetCreditMinimumThreshold     = 0.001
	openAIAutoResetCreditMaxExpiryLeadMinutes = 366 * 24 * 60
	openAIAutoResetCreditMinExpiryLeadMinutes = 10
	openAIAutoResetCreditDefaultLeadMinutes   = openAIAutoResetCreditMinExpiryLeadMinutes
)

// OpenAIAutoResetCreditConfig 是账号级自动用卡配置。阈值采用 0-1 比例，
// 避免后端调度与前端百分比展示混用同一数值语义。Enabled 与 ExpiryEnabled
// 是两条独立的触发路径，任一开启账号即进入自动用卡调度。
type OpenAIAutoResetCreditConfig struct {
	Enabled       bool
	ExpiryEnabled bool
	Threshold5h   float64
	Threshold7d   float64
	ExpiryLead    time.Duration
}

func (c OpenAIAutoResetCreditConfig) Active() bool {
	return c.Enabled || c.ExpiryEnabled
}

// ResolveOpenAIAutoResetCreditConfig 只接受 OpenAI OAuth 母账号；历史账号未配置时
// 始终保持关闭，防止升级后产生意外消费。
func ResolveOpenAIAutoResetCreditConfig(account *Account) OpenAIAutoResetCreditConfig {
	config := OpenAIAutoResetCreditConfig{
		Threshold5h: openAIAutoResetCreditDefault5hThreshold,
		Threshold7d: openAIAutoResetCreditDefaultThreshold,
		ExpiryLead:  openAIAutoResetCreditDefaultLeadMinutes * time.Minute,
	}
	if !isOpenAIAutoResetCreditAccount(account) || account.Extra == nil {
		return config
	}
	config.Enabled = resolveAccountExtraBool(account.Extra, OpenAIAutoResetCreditEnabledExtraKey)
	config.ExpiryEnabled = resolveAccountExtraBool(account.Extra, OpenAIAutoResetCreditExpiryEnabledExtraKey)
	if value, ok := resolveAccountExtraNumber(account.Extra, OpenAIAutoResetCredit5hThresholdExtraKey); ok && isValidOpenAIAutoResetThreshold(value) {
		config.Threshold5h = value
	}
	if value, ok := resolveAccountExtraNumber(account.Extra, OpenAIAutoResetCredit7dThresholdExtraKey); ok && isValidOpenAIAutoResetThreshold(value) {
		config.Threshold7d = value
	}
	if value, ok := resolveAccountExtraNumber(account.Extra, OpenAIAutoResetCreditExpiryLeadMinutesExtraKey); ok && isValidOpenAIAutoResetExpiryLeadMinutes(value) {
		config.ExpiryLead = time.Duration(value) * time.Minute
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
	delete(normalized, OpenAIAutoResetCreditExpiryAtExtraKey)

	_, hasEnabled := normalized[OpenAIAutoResetCreditEnabledExtraKey]
	_, has5h := normalized[OpenAIAutoResetCredit5hThresholdExtraKey]
	_, has7d := normalized[OpenAIAutoResetCredit7dThresholdExtraKey]
	_, hasExpiryEnabled := normalized[OpenAIAutoResetCreditExpiryEnabledExtraKey]
	_, hasLead := normalized[OpenAIAutoResetCreditExpiryLeadMinutesExtraKey]
	if !hasEnabled && !has5h && !has7d && !hasExpiryEnabled && !hasLead {
		return normalized, nil
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
	expiryEnabled := false
	if hasExpiryEnabled {
		value, ok := normalized[OpenAIAutoResetCreditExpiryEnabledExtraKey].(bool)
		if !ok {
			return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_AUTO_RESET_CREDIT_EXPIRY_ENABLED_INVALID", "auto_reset_credit_expiry_enabled must be a boolean")
		}
		expiryEnabled = value
	}
	for key, defaultValue := range map[string]float64{
		OpenAIAutoResetCredit5hThresholdExtraKey: openAIAutoResetCreditDefault5hThreshold,
		OpenAIAutoResetCredit7dThresholdExtraKey: openAIAutoResetCreditDefaultThreshold,
	} {
		if _, present := normalized[key]; !present {
			if enabled {
				normalized[key] = defaultValue
			}
			continue
		}
		value, ok := parseOpenAIAutoResetThreshold(normalized[key])
		if !ok || !isValidOpenAIAutoResetThreshold(value) {
			return nil, infraerrors.Newf(http.StatusBadRequest, "OPENAI_AUTO_RESET_CREDIT_THRESHOLD_INVALID", "%s must be 0 (disabled) or between 0.001 and 1.0", key)
		}
		normalized[key] = value
	}
	if hasLead {
		value, ok := parseOpenAIAutoResetThreshold(normalized[OpenAIAutoResetCreditExpiryLeadMinutesExtraKey])
		if !ok || !isValidOpenAIAutoResetExpiryLeadMinutes(value) || (expiryEnabled && value == 0) {
			return nil, infraerrors.Newf(http.StatusBadRequest, "OPENAI_AUTO_RESET_CREDIT_EXPIRY_LEAD_INVALID", "%s must be a whole number of minutes between %d and %d (0 only when expiry auto-use is off)", OpenAIAutoResetCreditExpiryLeadMinutesExtraKey, openAIAutoResetCreditMinExpiryLeadMinutes, openAIAutoResetCreditMaxExpiryLeadMinutes)
		}
		normalized[OpenAIAutoResetCreditExpiryLeadMinutesExtraKey] = value
	} else if expiryEnabled {
		normalized[OpenAIAutoResetCreditExpiryLeadMinutesExtraKey] = float64(openAIAutoResetCreditDefaultLeadMinutes)
	}
	return normalized, nil
}

func stripOpenAIAutoResetCreditManagedExtra(extra map[string]any, stripConfig bool) map[string]any {
	if extra == nil {
		return nil
	}
	delete(extra, OpenAIAutoResetCreditStateExtraKey)
	delete(extra, OpenAIAutoResetCreditExpiryAtExtraKey)
	if stripConfig {
		delete(extra, OpenAIAutoResetCreditEnabledExtraKey)
		delete(extra, OpenAIAutoResetCredit5hThresholdExtraKey)
		delete(extra, OpenAIAutoResetCredit7dThresholdExtraKey)
		delete(extra, OpenAIAutoResetCreditExpiryEnabledExtraKey)
		delete(extra, OpenAIAutoResetCreditExpiryLeadMinutesExtraKey)
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

// 0 表示该窗口不参与阈值触发。
func isValidOpenAIAutoResetThreshold(value float64) bool {
	return value == 0 || (!math.IsNaN(value) && !math.IsInf(value, 0) && value >= openAIAutoResetCreditMinimumThreshold && value <= 1)
}

// 非零提前量不得短于取信息失败后的 10 分钟重试间隔，否则到点那次拿不到上游时间就可能错过卡。
func isValidOpenAIAutoResetExpiryLeadMinutes(value float64) bool {
	if math.IsNaN(value) || value != math.Trunc(value) || value > openAIAutoResetCreditMaxExpiryLeadMinutes {
		return false
	}
	return value == 0 || value >= openAIAutoResetCreditMinExpiryLeadMinutes
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
