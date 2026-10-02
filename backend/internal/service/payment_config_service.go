package service

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	SettingPaymentEnabled      = "payment_enabled"
	SettingMinRechargeAmount   = "MIN_RECHARGE_AMOUNT"
	SettingMaxRechargeAmount   = "MAX_RECHARGE_AMOUNT"
	SettingDailyRechargeLimit  = "DAILY_RECHARGE_LIMIT"
	SettingOrderTimeoutMinutes = "ORDER_TIMEOUT_MINUTES"
	SettingMaxPendingOrders    = "MAX_PENDING_ORDERS"
	SettingEnabledPaymentTypes = "ENABLED_PAYMENT_TYPES"
	SettingLoadBalanceStrategy = "LOAD_BALANCE_STRATEGY"
	SettingBalancePayDisabled  = "BALANCE_PAYMENT_DISABLED"
	SettingBalanceRechargeMult = "BALANCE_RECHARGE_MULTIPLIER"
	// SettingSubscriptionUSDToCNYRate 是订阅 CNY 换算汇率（1 USD = X CNY）。
	// 0/未配置 = 关闭换算（订阅按 price 数值直付），显式配置后 CNY 通道订阅按 price × rate 收款。
	SettingSubscriptionUSDToCNYRate = "SUBSCRIPTION_USD_TO_CNY_RATE"
	SettingRechargeFeeRate          = "RECHARGE_FEE_RATE"
	SettingProductNamePrefix        = "PRODUCT_NAME_PREFIX"
	SettingProductNameSuffix        = "PRODUCT_NAME_SUFFIX"
	SettingHelpImageURL             = "PAYMENT_HELP_IMAGE_URL"
	SettingHelpText                 = "PAYMENT_HELP_TEXT"
	SettingCancelRateLimitOn        = "CANCEL_RATE_LIMIT_ENABLED"
	SettingCancelRateLimitMax       = "CANCEL_RATE_LIMIT_MAX"
	SettingCancelWindowSize         = "CANCEL_RATE_LIMIT_WINDOW"
	SettingCancelWindowUnit         = "CANCEL_RATE_LIMIT_UNIT"
	SettingCancelWindowMode         = "CANCEL_RATE_LIMIT_WINDOW_MODE"
)

// Default values for payment configuration settings.
const (
	defaultOrderTimeoutMin  = 30
	defaultMaxPendingOrders = 3
)

// PaymentConfig holds the payment system configuration.
type PaymentConfig struct {
	Enabled                   bool     `json:"enabled"`
	MinAmount                 float64  `json:"min_amount"`
	MaxAmount                 float64  `json:"max_amount"`
	DailyLimit                float64  `json:"daily_limit"`
	OrderTimeoutMin           int      `json:"order_timeout_minutes"`
	MaxPendingOrders          int      `json:"max_pending_orders"`
	EnabledTypes              []string `json:"enabled_payment_types"`
	BalanceDisabled           bool     `json:"balance_disabled"`
	BalanceRechargeMultiplier float64  `json:"balance_recharge_multiplier"`
	// SubscriptionUSDToCNYRate 为 0 时订阅换算关闭（兼容存量行为）。
	SubscriptionUSDToCNYRate float64 `json:"subscription_usd_to_cny_rate"`
	RechargeFeeRate          float64 `json:"recharge_fee_rate"`
	// ExchangeRateMarkupPercent 叠加在牌价上：0 表示按牌价原价换算。
	ExchangeRateMarkupPercent float64 `json:"exchange_rate_markup_percent"`
	// ExchangeRateMaxAgeHours 是缓存汇率的最长可用时长，超过就拒绝建单。
	ExchangeRateMaxAgeHours int `json:"exchange_rate_max_age_hours"`
	// RechargeBonusTiers 余额充值优惠阶梯（按 MinAmount 升序）；空表示无优惠。
	RechargeBonusTiers []RechargeBonusTier `json:"recharge_bonus_tiers"`
	// RechargeBonusMode 阶梯模式：bonus（赠金）/ discount（折扣），已归一化。
	RechargeBonusMode string `json:"recharge_bonus_mode"`
	// RechargeBonusNotice 充值页展示的 Markdown 活动文案；空表示不展示。
	RechargeBonusNotice string `json:"recharge_bonus_notice"`
	LoadBalanceStrategy string `json:"load_balance_strategy"`
	ProductNamePrefix   string `json:"product_name_prefix"`
	ProductNameSuffix   string `json:"product_name_suffix"`
	HelpImageURL        string `json:"help_image_url"`
	HelpText            string `json:"help_text"`

	// Cancel rate limit settings
	CancelRateLimitEnabled bool   `json:"cancel_rate_limit_enabled"`
	CancelRateLimitMax     int    `json:"cancel_rate_limit_max"`
	CancelRateLimitWindow  int    `json:"cancel_rate_limit_window"`
	CancelRateLimitUnit    string `json:"cancel_rate_limit_unit"`
	CancelRateLimitMode    string `json:"cancel_rate_limit_window_mode"`

	// Force Alipay mobile users to use QR code instead of mobile redirect
	AlipayForceQRCode bool `json:"alipay_force_qrcode"`
	// Use Alipay face-to-face precreate and an app deep link on mobile clients.
	AlipayMobilePrecreateDeepLink bool `json:"alipay_mobile_precreate_deep_link"`
}

// UpdatePaymentConfigRequest contains fields to update payment configuration.
type UpdatePaymentConfigRequest struct {
	Enabled                   *bool    `json:"enabled"`
	MinAmount                 *float64 `json:"min_amount"`
	MaxAmount                 *float64 `json:"max_amount"`
	DailyLimit                *float64 `json:"daily_limit"`
	OrderTimeoutMin           *int     `json:"order_timeout_minutes"`
	MaxPendingOrders          *int     `json:"max_pending_orders"`
	EnabledTypes              []string `json:"enabled_payment_types"`
	BalanceDisabled           *bool    `json:"balance_disabled"`
	BalanceRechargeMultiplier *float64 `json:"balance_recharge_multiplier"`
	SubscriptionUSDToCNYRate  *float64 `json:"subscription_usd_to_cny_rate"`
	RechargeFeeRate           *float64 `json:"recharge_fee_rate"`
	ExchangeRateMarkupPercent *float64 `json:"exchange_rate_markup_percent"`
	ExchangeRateMaxAgeHours   *int     `json:"exchange_rate_max_age_hours"`
	// RechargeBonusTiers nil 表示不更新；空切片表示清空阶梯。
	RechargeBonusTiers  *[]RechargeBonusTier `json:"recharge_bonus_tiers"`
	RechargeBonusMode   *string              `json:"recharge_bonus_mode"`
	RechargeBonusNotice *string              `json:"recharge_bonus_notice"`
	LoadBalanceStrategy *string              `json:"load_balance_strategy"`
	ProductNamePrefix   *string              `json:"product_name_prefix"`
	ProductNameSuffix   *string              `json:"product_name_suffix"`
	HelpImageURL        *string              `json:"help_image_url"`
	HelpText            *string              `json:"help_text"`

	// Cancel rate limit settings
	CancelRateLimitEnabled *bool   `json:"cancel_rate_limit_enabled"`
	CancelRateLimitMax     *int    `json:"cancel_rate_limit_max"`
	CancelRateLimitWindow  *int    `json:"cancel_rate_limit_window"`
	CancelRateLimitUnit    *string `json:"cancel_rate_limit_unit"`
	CancelRateLimitMode    *string `json:"cancel_rate_limit_window_mode"`

	// Force Alipay mobile users to use QR code instead of mobile redirect
	AlipayForceQRCode *bool `json:"alipay_force_qrcode"`
	// Use Alipay face-to-face precreate and an app deep link on mobile clients.
	AlipayMobilePrecreateDeepLink *bool `json:"alipay_mobile_precreate_deep_link"`

	VisibleMethodAlipaySource  *string `json:"payment_visible_method_alipay_source"`
	VisibleMethodWxpaySource   *string `json:"payment_visible_method_wxpay_source"`
	VisibleMethodAlipayEnabled *bool   `json:"payment_visible_method_alipay_enabled"`
	VisibleMethodWxpayEnabled  *bool   `json:"payment_visible_method_wxpay_enabled"`
}

// MethodLimits holds per-payment-type limits.
type MethodLimits struct {
	PaymentType string  `json:"payment_type"`
	DisplayName string  `json:"display_name,omitempty"`
	Currency    string  `json:"currency"`
	FeeRate     float64 `json:"fee_rate"`
	DailyLimit  float64 `json:"daily_limit"`
	SingleMin   float64 `json:"single_min"`
	SingleMax   float64 `json:"single_max"`
}

// MethodLimitsResponse is the full response for the user-facing /limits API.
// It includes per-method limits and the global widest range (union of all methods).
type MethodLimitsResponse struct {
	Methods   map[string]MethodLimits `json:"methods"`
	GlobalMin float64                 `json:"global_min"` // 0 = no minimum
	GlobalMax float64                 `json:"global_max"` // 0 = no maximum
}

type CreateProviderInstanceRequest struct {
	ProviderKey     string            `json:"provider_key"`
	Name            string            `json:"name"`
	Config          map[string]string `json:"config"`
	SupportedTypes  []string          `json:"supported_types"`
	Enabled         bool              `json:"enabled"`
	PaymentMode     string            `json:"payment_mode"`
	SortOrder       int               `json:"sort_order"`
	Limits          string            `json:"limits"`
	RefundEnabled   bool              `json:"refund_enabled"`
	AllowUserRefund bool              `json:"allow_user_refund"`
}

type UpdateProviderInstanceRequest struct {
	Name            *string           `json:"name"`
	Config          map[string]string `json:"config"`
	SupportedTypes  []string          `json:"supported_types"`
	Enabled         *bool             `json:"enabled"`
	PaymentMode     *string           `json:"payment_mode"`
	SortOrder       *int              `json:"sort_order"`
	Limits          *string           `json:"limits"`
	RefundEnabled   *bool             `json:"refund_enabled"`
	AllowUserRefund *bool             `json:"allow_user_refund"`
}
type CreatePlanRequest struct {
	GroupID       int64    `json:"group_id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Price         float64  `json:"price"`
	OriginalPrice *float64 `json:"original_price"`
	Currency      string   `json:"currency"`
	ValidityDays  int      `json:"validity_days"`
	ValidityUnit  string   `json:"validity_unit"`
	Features      string   `json:"features"`
	ProductName   string   `json:"product_name"`
	ForSale       bool     `json:"for_sale"`
	SortOrder     int      `json:"sort_order"`
}

type UpdatePlanRequest struct {
	GroupID       *int64   `json:"group_id"`
	Name          *string  `json:"name"`
	Description   *string  `json:"description"`
	Price         *float64 `json:"price"`
	OriginalPrice *float64 `json:"original_price"`
	Currency      *string  `json:"currency"`
	ValidityDays  *int     `json:"validity_days"`
	ValidityUnit  *string  `json:"validity_unit"`
	Features      *string  `json:"features"`
	ProductName   *string  `json:"product_name"`
	ForSale       *bool    `json:"for_sale"`
	SortOrder     *int     `json:"sort_order"`
}

// PaymentConfigService manages payment configuration and CRUD for
// provider instances, channels, and subscription plans.
type PaymentConfigService struct {
	entClient     *dbent.Client
	settingRepo   SettingRepository
	encryptionKey []byte
}

// NewPaymentConfigService creates a new PaymentConfigService.
func NewPaymentConfigService(entClient *dbent.Client, settingRepo SettingRepository, encryptionKey []byte) *PaymentConfigService {
	return &PaymentConfigService{entClient: entClient, settingRepo: settingRepo, encryptionKey: encryptionKey}
}

// IsPaymentEnabled returns whether the payment system is enabled.
func (s *PaymentConfigService) IsPaymentEnabled(ctx context.Context) bool {
	val, err := s.settingRepo.GetValue(ctx, SettingPaymentEnabled)
	if err != nil {
		return false
	}
	return val == "true"
}

// GetPaymentConfig returns the full payment configuration.
func (s *PaymentConfigService) GetPaymentConfig(ctx context.Context) (*PaymentConfig, error) {
	keys := []string{
		SettingPaymentEnabled, SettingMinRechargeAmount, SettingMaxRechargeAmount,
		SettingDailyRechargeLimit, SettingOrderTimeoutMinutes, SettingMaxPendingOrders,
		SettingEnabledPaymentTypes, SettingBalancePayDisabled, SettingBalanceRechargeMult, SettingSubscriptionUSDToCNYRate, SettingRechargeFeeRate,
		SettingExchangeRateMarkupPercent, SettingExchangeRateMaxAgeHours, SettingLoadBalanceStrategy,
		SettingRechargeBonusTiers, SettingRechargeBonusMode, SettingRechargeBonusNotice,
		SettingProductNamePrefix, SettingProductNameSuffix,
		SettingHelpImageURL, SettingHelpText,
		SettingCancelRateLimitOn, SettingCancelRateLimitMax,
		SettingCancelWindowSize, SettingCancelWindowUnit, SettingCancelWindowMode,
	}
	vals, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("get payment config settings: %w", err)
	}
	return s.parsePaymentConfig(vals), nil
}

func (s *PaymentConfigService) parsePaymentConfig(vals map[string]string) *PaymentConfig {
	cfg := &PaymentConfig{
		Enabled:                   vals[SettingPaymentEnabled] == "true",
		MinAmount:                 pcParseFloat(vals[SettingMinRechargeAmount], 1),
		MaxAmount:                 pcParseFloat(vals[SettingMaxRechargeAmount], 0),
		DailyLimit:                pcParseFloat(vals[SettingDailyRechargeLimit], 0),
		OrderTimeoutMin:           pcParseInt(vals[SettingOrderTimeoutMinutes], defaultOrderTimeoutMin),
		MaxPendingOrders:          pcParseInt(vals[SettingMaxPendingOrders], defaultMaxPendingOrders),
		BalanceDisabled:           vals[SettingBalancePayDisabled] == "true",
		BalanceRechargeMultiplier: normalizeBalanceRechargeMultiplier(pcParseFloat(vals[SettingBalanceRechargeMult], defaultBalanceRechargeMultiplier)),
		SubscriptionUSDToCNYRate:  normalizeSubscriptionUSDToCNYRate(pcParseFloat(vals[SettingSubscriptionUSDToCNYRate], 0)),
		RechargeFeeRate:           pcParseFloat(vals[SettingRechargeFeeRate], 0),
		ExchangeRateMarkupPercent: pcParseFloat(vals[SettingExchangeRateMarkupPercent], 0),
		ExchangeRateMaxAgeHours:   pcParseInt(vals[SettingExchangeRateMaxAgeHours], defaultExchangeRateMaxAgeHours),
		RechargeBonusTiers:        parseRechargeBonusTiers(vals[SettingRechargeBonusTiers]),
		RechargeBonusNotice:       vals[SettingRechargeBonusNotice],
		LoadBalanceStrategy:       vals[SettingLoadBalanceStrategy],
		ProductNamePrefix:         vals[SettingProductNamePrefix],
		ProductNameSuffix:         vals[SettingProductNameSuffix],
		HelpImageURL:              vals[SettingHelpImageURL],
		HelpText:                  vals[SettingHelpText],

		CancelRateLimitEnabled: vals[SettingCancelRateLimitOn] == "true",
		CancelRateLimitMax:     pcParseInt(vals[SettingCancelRateLimitMax], 10),
		CancelRateLimitWindow:  pcParseInt(vals[SettingCancelWindowSize], 1),
		CancelRateLimitUnit:    vals[SettingCancelWindowUnit],
		CancelRateLimitMode:    vals[SettingCancelWindowMode],
	}
	cfg.RechargeBonusMode, _ = NormalizeRechargeBonusMode(vals[SettingRechargeBonusMode])
	if cfg.LoadBalanceStrategy == "" {
		cfg.LoadBalanceStrategy = payment.DefaultLoadBalanceStrategy
	}
	if raw := vals[SettingEnabledPaymentTypes]; raw != "" {
		types := make([]string, 0, len(strings.Split(raw, ",")))
		for _, t := range strings.Split(raw, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				types = append(types, t)
			}
		}
		cfg.EnabledTypes = NormalizeVisibleMethods(types)
	}
	return cfg
}

// UpdatePaymentConfig updates the payment configuration settings.
// NOTE: This function exceeds 30 lines because each field requires an independent
// nil-check before serialisation — this is inherent to patch-style update patterns
// and cannot be meaningfully decomposed without introducing unnecessary abstraction.
func (s *PaymentConfigService) UpdatePaymentConfig(ctx context.Context, req UpdatePaymentConfigRequest) error {
	if req.BalanceRechargeMultiplier != nil {
		if math.IsNaN(*req.BalanceRechargeMultiplier) || math.IsInf(*req.BalanceRechargeMultiplier, 0) || *req.BalanceRechargeMultiplier <= 0 {
			return infraerrors.BadRequest("INVALID_BALANCE_RECHARGE_MULTIPLIER", "balance recharge multiplier must be greater than 0")
		}
	}
	if req.SubscriptionUSDToCNYRate != nil {
		v := *req.SubscriptionUSDToCNYRate
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return infraerrors.BadRequest("INVALID_SUBSCRIPTION_USD_TO_CNY_RATE", "subscription USD to CNY rate must be 0 (disabled) or a positive number")
		}
	}
	if req.RechargeFeeRate != nil {
		v := *req.RechargeFeeRate
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 100 {
			return infraerrors.BadRequest("INVALID_RECHARGE_FEE_RATE", "recharge fee rate must be between 0 and 100")
		}
		// Enforce max 2 decimal places
		if math.Round(v*100) != v*100 {
			return infraerrors.BadRequest("INVALID_RECHARGE_FEE_RATE", "recharge fee rate allows at most 2 decimal places")
		}
	}
	if req.ExchangeRateMarkupPercent != nil {
		v := *req.ExchangeRateMarkupPercent
		// 100% 以上的加价几乎肯定是填错了，挡住比事后退款便宜。
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 100 {
			return infraerrors.BadRequest("INVALID_EXCHANGE_RATE_MARKUP", "exchange rate markup must be between 0 and 100 percent")
		}
	}
	if req.ExchangeRateMaxAgeHours != nil {
		// 0 会让每一次抓取失败都变成「随便用多旧的价格」，所以下限是 1 小时。
		if v := *req.ExchangeRateMaxAgeHours; v < 1 || v > 24*30 {
			return infraerrors.BadRequest("INVALID_EXCHANGE_RATE_MAX_AGE", "exchange rate max age must be between 1 and 720 hours")
		}
	}
	rechargeBonusTiersValue, rechargeBonusModeValue, err := s.resolveRechargeBonusUpdate(ctx, req)
	if err != nil {
		return err
	}
	if req.RechargeBonusNotice != nil {
		if err := validateRechargeBonusNotice(*req.RechargeBonusNotice); err != nil {
			return infraerrors.BadRequest("INVALID_RECHARGE_BONUS_NOTICE", err.Error())
		}
	}
	m := make(map[string]string)
	if req.Enabled != nil {
		m[SettingPaymentEnabled] = formatBoolOrEmpty(req.Enabled)
	}
	if req.MinAmount != nil {
		m[SettingMinRechargeAmount] = formatPositiveFloat(req.MinAmount)
	}
	if req.MaxAmount != nil {
		m[SettingMaxRechargeAmount] = formatPositiveFloat(req.MaxAmount)
	}
	if req.DailyLimit != nil {
		m[SettingDailyRechargeLimit] = formatPositiveFloat(req.DailyLimit)
	}
	if req.OrderTimeoutMin != nil {
		m[SettingOrderTimeoutMinutes] = formatPositiveInt(req.OrderTimeoutMin)
	}
	if req.MaxPendingOrders != nil {
		m[SettingMaxPendingOrders] = formatPositiveInt(req.MaxPendingOrders)
	}
	if req.EnabledTypes != nil {
		m[SettingEnabledPaymentTypes] = strings.Join(req.EnabledTypes, ",")
	}
	if req.BalanceDisabled != nil {
		m[SettingBalancePayDisabled] = formatBoolOrEmpty(req.BalanceDisabled)
	}
	if req.BalanceRechargeMultiplier != nil {
		m[SettingBalanceRechargeMult] = formatPositiveFloat(req.BalanceRechargeMultiplier)
	}
	if req.SubscriptionUSDToCNYRate != nil {
		m[SettingSubscriptionUSDToCNYRate] = formatPositiveFloatExact(req.SubscriptionUSDToCNYRate)
	}
	if req.RechargeFeeRate != nil {
		m[SettingRechargeFeeRate] = formatNonNegativeFloat(req.RechargeFeeRate)
	}
	if req.ExchangeRateMarkupPercent != nil {
		m[SettingExchangeRateMarkupPercent] = formatNonNegativeFloat(req.ExchangeRateMarkupPercent)
	}
	if req.ExchangeRateMaxAgeHours != nil {
		m[SettingExchangeRateMaxAgeHours] = strconv.Itoa(*req.ExchangeRateMaxAgeHours)
	}
	if req.RechargeBonusTiers != nil {
		m[SettingRechargeBonusTiers] = rechargeBonusTiersValue
	}
	if req.RechargeBonusMode != nil {
		m[SettingRechargeBonusMode] = rechargeBonusModeValue
	}
	if req.RechargeBonusNotice != nil {
		m[SettingRechargeBonusNotice] = strings.TrimSpace(*req.RechargeBonusNotice)
	}
	if req.LoadBalanceStrategy != nil {
		m[SettingLoadBalanceStrategy] = derefStr(req.LoadBalanceStrategy)
	}
	if req.ProductNamePrefix != nil {
		m[SettingProductNamePrefix] = derefStr(req.ProductNamePrefix)
	}
	if req.ProductNameSuffix != nil {
		m[SettingProductNameSuffix] = derefStr(req.ProductNameSuffix)
	}
	if req.HelpImageURL != nil {
		m[SettingHelpImageURL] = derefStr(req.HelpImageURL)
	}
	if req.HelpText != nil {
		m[SettingHelpText] = derefStr(req.HelpText)
	}
	if req.CancelRateLimitEnabled != nil {
		m[SettingCancelRateLimitOn] = formatBoolOrEmpty(req.CancelRateLimitEnabled)
	}
	if req.CancelRateLimitMax != nil {
		m[SettingCancelRateLimitMax] = formatPositiveInt(req.CancelRateLimitMax)
	}
	if req.CancelRateLimitWindow != nil {
		m[SettingCancelWindowSize] = formatPositiveInt(req.CancelRateLimitWindow)
	}
	if req.CancelRateLimitUnit != nil {
		m[SettingCancelWindowUnit] = derefStr(req.CancelRateLimitUnit)
	}
	if req.CancelRateLimitMode != nil {
		m[SettingCancelWindowMode] = derefStr(req.CancelRateLimitMode)
	}
	return s.settingRepo.SetMultiple(ctx, m)
}

func formatBoolOrEmpty(v *bool) string {
	if v == nil {
		return ""
	}
	return strconv.FormatBool(*v)
}

func formatPositiveFloat(v *float64) string {
	if v == nil || *v <= 0 {
		return "" // empty → parsePaymentConfig uses default
	}
	return strconv.FormatFloat(*v, 'f', 2, 64)
}

// formatPositiveFloatExact 保留完整精度，用于汇率等对小数位敏感的配置。
func formatPositiveFloatExact(v *float64) string {
	if v == nil || *v <= 0 {
		return "" // empty → parsePaymentConfig 视为未配置（换算关闭）
	}
	return strconv.FormatFloat(*v, 'f', -1, 64)
}

func formatNonNegativeFloat(v *float64) string {
	if v == nil || *v < 0 {
		return ""
	}
	return strconv.FormatFloat(*v, 'f', 2, 64)
}

func formatPositiveInt(v *int) string {
	if v == nil || *v <= 0 {
		return ""
	}
	return strconv.Itoa(*v)
}

func derefStr(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func splitTypes(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func joinTypes(types []string) string {
	return strings.Join(types, ",")
}

func pcParseFloat(s string, defaultVal float64) float64 {
	if s == "" {
		return defaultVal
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return defaultVal
	}
	return v
}

func pcParseInt(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}
