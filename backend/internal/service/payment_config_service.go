package service

import (
	"context"
	"fmt"
	"math"
	"os"
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
	// SettingSubscriptionUSDToVNDRate is the USD→VND rate used whenever a USD
	// amount is collected through a dong channel (SePay) — a plan price or a
	// balance top-up, both of which are denominated in USD. The key keeps its
	// historical SUBSCRIPTION_ name so existing installs need no migration.
	// 0/unset = no conversion (the USD figure is charged as-is).
	SettingSubscriptionUSDToVNDRate = "SUBSCRIPTION_USD_TO_VND_RATE"
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
	// SubscriptionUSDToVNDRate is 0 when conversion is off (price charged as-is).
	SubscriptionUSDToVNDRate float64 `json:"subscription_usd_to_vnd_rate"`
	RechargeFeeRate          float64 `json:"recharge_fee_rate"`
	LoadBalanceStrategy      string  `json:"load_balance_strategy"`
	ProductNamePrefix        string  `json:"product_name_prefix"`
	ProductNameSuffix        string  `json:"product_name_suffix"`
	HelpImageURL             string  `json:"help_image_url"`
	HelpText                 string  `json:"help_text"`

	// Cancel rate limit settings
	CancelRateLimitEnabled bool   `json:"cancel_rate_limit_enabled"`
	CancelRateLimitMax     int    `json:"cancel_rate_limit_max"`
	CancelRateLimitWindow  int    `json:"cancel_rate_limit_window"`
	CancelRateLimitUnit    string `json:"cancel_rate_limit_unit"`
	CancelRateLimitMode    string `json:"cancel_rate_limit_window_mode"`
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
	SubscriptionUSDToVNDRate  *float64 `json:"subscription_usd_to_vnd_rate"`
	RechargeFeeRate           *float64 `json:"recharge_fee_rate"`
	LoadBalanceStrategy       *string  `json:"load_balance_strategy"`
	ProductNamePrefix         *string  `json:"product_name_prefix"`
	ProductNameSuffix         *string  `json:"product_name_suffix"`
	HelpImageURL              *string  `json:"help_image_url"`
	HelpText                  *string  `json:"help_text"`

	// Cancel rate limit settings
	CancelRateLimitEnabled *bool   `json:"cancel_rate_limit_enabled"`
	CancelRateLimitMax     *int    `json:"cancel_rate_limit_max"`
	CancelRateLimitWindow  *int    `json:"cancel_rate_limit_window"`
	CancelRateLimitUnit    *string `json:"cancel_rate_limit_unit"`
	CancelRateLimitMode    *string `json:"cancel_rate_limit_window_mode"`
}

// MethodLimits holds per-payment-type limits.
type MethodLimits struct {
	PaymentType string  `json:"payment_type"`
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
	ProviderKey    string            `json:"provider_key"`
	Name           string            `json:"name"`
	Config         map[string]string `json:"config"`
	SupportedTypes []string          `json:"supported_types"`
	Enabled        bool              `json:"enabled"`
	PaymentMode    string            `json:"payment_mode"`
	SortOrder      int               `json:"sort_order"`
	Limits         string            `json:"limits"`
}

type UpdateProviderInstanceRequest struct {
	Name           *string           `json:"name"`
	Config         map[string]string `json:"config"`
	SupportedTypes []string          `json:"supported_types"`
	Enabled        *bool             `json:"enabled"`
	PaymentMode    *string           `json:"payment_mode"`
	SortOrder      *int              `json:"sort_order"`
	Limits         *string           `json:"limits"`
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
		SettingEnabledPaymentTypes, SettingBalancePayDisabled, SettingBalanceRechargeMult, SettingSubscriptionUSDToVNDRate, SettingRechargeFeeRate, SettingLoadBalanceStrategy,
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
		SubscriptionUSDToVNDRate:  normalizeUSDToVNDRate(pcParseFloat(vals[SettingSubscriptionUSDToVNDRate], 0)),
		RechargeFeeRate:           pcParseFloat(vals[SettingRechargeFeeRate], 0),
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
		cfg.EnabledTypes = types
	}
	return cfg
}

func pcEnvBoolOverride(key string, fallback bool) bool {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback
	}
	value, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return value
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
	if req.SubscriptionUSDToVNDRate != nil {
		v := *req.SubscriptionUSDToVNDRate
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return infraerrors.BadRequest("INVALID_SUBSCRIPTION_USD_TO_VND_RATE", "subscription USD to VND rate must be 0 (disabled) or a positive number")
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
	if req.SubscriptionUSDToVNDRate != nil {
		m[SettingSubscriptionUSDToVNDRate] = formatPositiveFloatExact(req.SubscriptionUSDToVNDRate)
	}
	if req.RechargeFeeRate != nil {
		m[SettingRechargeFeeRate] = formatNonNegativeFloat(req.RechargeFeeRate)
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
