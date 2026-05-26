package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"github.com/shopspring/decimal"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	pluginent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
	"github.com/Wei-Shaw/sub2api/plugins/payment/ent/paymentproviderinstance"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
)

// Settings keys (snake_case JSON keys defined in
// internal/settings/settings_schema.json).
const (
	SettingPaymentEnabled                    = "enabled"
	SettingMinRechargeAmount                 = "min_recharge_amount"
	SettingMaxRechargeAmount                 = "max_recharge_amount"
	SettingDailyRechargeLimit                = "daily_recharge_limit"
	SettingOrderTimeoutMinutes               = "order_timeout_minutes"
	SettingMaxPendingOrders                  = "max_pending_orders"
	SettingEnabledPaymentTypes               = "enabled_payment_types"
	SettingLoadBalanceStrategy               = "load_balance_strategy"
	SettingBalancePayDisabled                = "balance_payment_disabled"
	SettingBalanceRechargeMult               = "balance_recharge_multiplier"
	SettingRechargeFeeRate                   = "recharge_fee_rate"
	SettingProductNamePrefix                 = "product_name_prefix"
	SettingProductNameSuffix                 = "product_name_suffix"
	SettingHelpImageURL                      = "help_image_url"
	SettingHelpText                          = "help_text"
	SettingCancelRateLimitOn                 = "cancel_rate_limit_enabled"
	SettingCancelRateLimitMax                = "cancel_rate_limit_max"
	SettingCancelWindowSize                  = "cancel_rate_limit_window"
	SettingCancelWindowUnit                  = "cancel_rate_limit_unit"
	SettingCancelWindowMode                  = "cancel_rate_limit_window_mode"
	SettingPaymentVisibleMethodAlipayEnabled = "visible_method_alipay_enabled"
	SettingPaymentVisibleMethodAlipaySource  = "visible_method_alipay_source"
	SettingPaymentVisibleMethodWxpayEnabled  = "visible_method_wxpay_enabled"
	SettingPaymentVisibleMethodWxpaySource   = "visible_method_wxpay_source"
)

const (
	defaultOrderTimeoutMin  = 30
	defaultMaxPendingOrders = 3
)

// PaymentConfig is the full plugin payment config materialised for handlers.
//
// Money-typed fields are shopspring/decimal so all subsequent arithmetic
// stays exact. The settings backend persists these as numeric strings,
// which preserves precision across the read/write boundary.
type PaymentConfig struct {
	Enabled                   bool            `json:"enabled"`
	MinAmount                 decimal.Decimal `json:"min_amount"`
	MaxAmount                 decimal.Decimal `json:"max_amount"`
	DailyLimit                decimal.Decimal `json:"daily_limit"`
	OrderTimeoutMin           int             `json:"order_timeout_minutes"`
	MaxPendingOrders          int             `json:"max_pending_orders"`
	EnabledTypes              []string        `json:"enabled_payment_types"`
	BalanceDisabled           bool            `json:"balance_disabled"`
	BalanceRechargeMultiplier decimal.Decimal `json:"balance_recharge_multiplier"`
	RechargeFeeRate           decimal.Decimal `json:"recharge_fee_rate"`
	LoadBalanceStrategy       string          `json:"load_balance_strategy"`
	ProductNamePrefix         string          `json:"product_name_prefix"`
	ProductNameSuffix         string          `json:"product_name_suffix"`
	HelpImageURL              string          `json:"help_image_url"`
	HelpText                  string          `json:"help_text"`
	StripePublishableKey      string          `json:"stripe_publishable_key,omitempty"`

	CancelRateLimitEnabled bool   `json:"cancel_rate_limit_enabled"`
	CancelRateLimitMax     int    `json:"cancel_rate_limit_max"`
	CancelRateLimitWindow  int    `json:"cancel_rate_limit_window"`
	CancelRateLimitUnit    string `json:"cancel_rate_limit_unit"`
	CancelRateLimitMode    string `json:"cancel_rate_limit_window_mode"`

	VisibleMethodAlipaySource  string `json:"visible_method_alipay_source,omitempty"`
	VisibleMethodWxpaySource   string `json:"visible_method_wxpay_source,omitempty"`
	VisibleMethodAlipayEnabled bool   `json:"visible_method_alipay_enabled,omitempty"`
	VisibleMethodWxpayEnabled  bool   `json:"visible_method_wxpay_enabled,omitempty"`
}

// UpdatePaymentConfigRequest carries the partial-update payload from the
// admin payment-config form. Pointer fields are optional ("not provided"
// = nil); UpdatePaymentConfig writes only the non-nil ones via the SDK
// SettingsClient.Set, so the form can submit deltas without resetting
// untouched keys.
type UpdatePaymentConfigRequest struct {
	Enabled                   *bool            `json:"enabled"`
	MinAmount                 *decimal.Decimal `json:"min_amount"`
	MaxAmount                 *decimal.Decimal `json:"max_amount"`
	DailyLimit                *decimal.Decimal `json:"daily_limit"`
	OrderTimeoutMin           *int             `json:"order_timeout_minutes"`
	MaxPendingOrders          *int             `json:"max_pending_orders"`
	EnabledTypes              []string         `json:"enabled_payment_types"`
	BalanceDisabled           *bool            `json:"balance_disabled"`
	BalanceRechargeMultiplier *decimal.Decimal `json:"balance_recharge_multiplier"`
	RechargeFeeRate           *decimal.Decimal `json:"recharge_fee_rate"`
	LoadBalanceStrategy       *string          `json:"load_balance_strategy"`
	ProductNamePrefix         *string          `json:"product_name_prefix"`
	ProductNameSuffix         *string          `json:"product_name_suffix"`
	HelpImageURL              *string          `json:"help_image_url"`
	HelpText                  *string          `json:"help_text"`

	CancelRateLimitEnabled *bool   `json:"cancel_rate_limit_enabled"`
	CancelRateLimitMax     *int    `json:"cancel_rate_limit_max"`
	CancelRateLimitWindow  *int    `json:"cancel_rate_limit_window"`
	CancelRateLimitUnit    *string `json:"cancel_rate_limit_unit"`
	CancelRateLimitMode    *string `json:"cancel_rate_limit_window_mode"`

	VisibleMethodAlipaySource  *string `json:"visible_method_alipay_source"`
	VisibleMethodWxpaySource   *string `json:"visible_method_wxpay_source"`
	VisibleMethodAlipayEnabled *bool   `json:"visible_method_alipay_enabled"`
	VisibleMethodWxpayEnabled  *bool   `json:"visible_method_wxpay_enabled"`
}

// MethodLimits holds per-payment-type limits. All currency-typed
// fields are decimal so the limit comparison never round-trips through
// float64.
type MethodLimits struct {
	PaymentType string          `json:"payment_type"`
	Currency    string          `json:"currency"`
	FeeRate     decimal.Decimal `json:"fee_rate"`
	DailyLimit  decimal.Decimal `json:"daily_limit"`
	SingleMin   decimal.Decimal `json:"single_min"`
	SingleMax   decimal.Decimal `json:"single_max"`
}

// MethodLimitsResponse is the full /limits API response.
type MethodLimitsResponse struct {
	Methods   map[string]MethodLimits `json:"methods"`
	GlobalMin decimal.Decimal         `json:"global_min"`
	GlobalMax decimal.Decimal         `json:"global_max"`
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
	GroupID       int64            `json:"group_id"`
	Name          string           `json:"name"`
	Description   string           `json:"description"`
	Price         decimal.Decimal  `json:"price"`
	OriginalPrice *decimal.Decimal `json:"original_price"`
	ValidityDays  int              `json:"validity_days"`
	ValidityUnit  string           `json:"validity_unit"`
	Features      string           `json:"features"`
	ProductName   string           `json:"product_name"`
	ForSale       bool             `json:"for_sale"`
	SortOrder     int              `json:"sort_order"`
}

type UpdatePlanRequest struct {
	GroupID       *int64           `json:"group_id"`
	Name          *string          `json:"name"`
	Description   *string          `json:"description"`
	Price         *decimal.Decimal `json:"price"`
	OriginalPrice *decimal.Decimal `json:"original_price"`
	ValidityDays  *int             `json:"validity_days"`
	ValidityUnit  *string          `json:"validity_unit"`
	Features      *string          `json:"features"`
	ProductName   *string          `json:"product_name"`
	ForSale       *bool            `json:"for_sale"`
	SortOrder     *int             `json:"sort_order"`
}

// SubscriptionPlan is a plugin-local mirror of the core's subscription_plans
// row.
type SubscriptionPlan struct {
	ID            int
	GroupID       int64
	Name          string
	Description   string
	Price         decimal.Decimal
	OriginalPrice *decimal.Decimal
	ValidityDays  int
	ValidityUnit  string
	Features      string
	ProductName   string
	ForSale       bool
	SortOrder     int
}

// PlanGroupInfo describes the user-facing group context for a plan card.
type PlanGroupInfo struct {
	GroupID  int64  `json:"group_id"`
	Platform string `json:"platform"`
	Name     string `json:"name"`
}

// ProviderInstanceResponse is the admin-facing row description.
type ProviderInstanceResponse struct {
	ID              int64             `json:"id"`
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

// PaymentConfigService manages payment configuration and provider/plan CRUD.
type PaymentConfigService struct {
	settings  pluginsdk.SettingsClient
	secrets   pluginsdk.SecretEncryptor
	entClient *pluginent.Client
	// db is the host-DB handle used for raw reads against tables the
	// plugin does not own (subscription_plans, groups). The plugin holds
	// CapabilityDBCoreRead so SELECTs on the whitelist are permitted.
	db     *sql.DB
	logger *slog.Logger
}

// NewPaymentConfigService constructs the service.
func NewPaymentConfigService(settings pluginsdk.SettingsClient, secrets pluginsdk.SecretEncryptor, entClient *pluginent.Client, db *sql.DB, logger *slog.Logger) *PaymentConfigService {
	if logger == nil {
		logger = slog.Default()
	}
	return &PaymentConfigService{settings: settings, secrets: secrets, entClient: entClient, db: db, logger: logger}
}

// errPlanWriteNotSupported is returned by every plan-write method.
var errPlanWriteNotSupported = errors.New("payment: plan CRUD is not supported from the plugin (use host admin)")

// IsPaymentEnabled returns whether the payment system is enabled.
func (s *PaymentConfigService) IsPaymentEnabled(ctx context.Context) bool {
	if s == nil || s.settings == nil {
		return false
	}
	var enabled bool
	if err := s.settings.GetTyped(ctx, SettingPaymentEnabled, &enabled); err != nil {
		return false
	}
	return enabled
}

// GetPaymentConfig returns the full payment configuration assembled from
// the SDK settings client and the plugin's provider table.
func (s *PaymentConfigService) GetPaymentConfig(ctx context.Context) (*PaymentConfig, error) {
	if s == nil || s.settings == nil {
		return &PaymentConfig{}, nil
	}
	cfg := &PaymentConfig{
		OrderTimeoutMin:           defaultOrderTimeoutMin,
		MaxPendingOrders:          defaultMaxPendingOrders,
		BalanceRechargeMultiplier: defaultBalanceRechargeMultiplier,
		LoadBalanceStrategy:       payment.DefaultLoadBalanceStrategy,
	}
	s.readBoolSetting(ctx, SettingPaymentEnabled, &cfg.Enabled)
	s.readDecimalSetting(ctx, SettingMinRechargeAmount, &cfg.MinAmount)
	s.readDecimalSetting(ctx, SettingMaxRechargeAmount, &cfg.MaxAmount)
	s.readDecimalSetting(ctx, SettingDailyRechargeLimit, &cfg.DailyLimit)
	s.readIntSetting(ctx, SettingOrderTimeoutMinutes, &cfg.OrderTimeoutMin)
	s.readIntSetting(ctx, SettingMaxPendingOrders, &cfg.MaxPendingOrders)
	s.readBoolSetting(ctx, SettingBalancePayDisabled, &cfg.BalanceDisabled)
	s.readDecimalSetting(ctx, SettingBalanceRechargeMult, &cfg.BalanceRechargeMultiplier)
	cfg.BalanceRechargeMultiplier = normalizeBalanceRechargeMultiplier(cfg.BalanceRechargeMultiplier)
	s.readDecimalSetting(ctx, SettingRechargeFeeRate, &cfg.RechargeFeeRate)
	s.readStringSetting(ctx, SettingLoadBalanceStrategy, &cfg.LoadBalanceStrategy)
	s.readStringSetting(ctx, SettingProductNamePrefix, &cfg.ProductNamePrefix)
	s.readStringSetting(ctx, SettingProductNameSuffix, &cfg.ProductNameSuffix)
	s.readStringSetting(ctx, SettingHelpImageURL, &cfg.HelpImageURL)
	s.readStringSetting(ctx, SettingHelpText, &cfg.HelpText)
	s.readBoolSetting(ctx, SettingCancelRateLimitOn, &cfg.CancelRateLimitEnabled)
	s.readIntSetting(ctx, SettingCancelRateLimitMax, &cfg.CancelRateLimitMax)
	s.readIntSetting(ctx, SettingCancelWindowSize, &cfg.CancelRateLimitWindow)
	s.readStringSetting(ctx, SettingCancelWindowUnit, &cfg.CancelRateLimitUnit)
	s.readStringSetting(ctx, SettingCancelWindowMode, &cfg.CancelRateLimitMode)
	s.readBoolSetting(ctx, SettingPaymentVisibleMethodAlipayEnabled, &cfg.VisibleMethodAlipayEnabled)
	s.readBoolSetting(ctx, SettingPaymentVisibleMethodWxpayEnabled, &cfg.VisibleMethodWxpayEnabled)
	s.readStringSetting(ctx, SettingPaymentVisibleMethodAlipaySource, &cfg.VisibleMethodAlipaySource)
	s.readStringSetting(ctx, SettingPaymentVisibleMethodWxpaySource, &cfg.VisibleMethodWxpaySource)
	cfg.EnabledTypes = s.readEnabledTypes(ctx)
	cfg.StripePublishableKey = s.getStripePublishableKey(ctx)
	return cfg, nil
}

// readEnabledTypes loads the JSON array of enabled payment types.
func (s *PaymentConfigService) readEnabledTypes(ctx context.Context) []string {
	var raw []string
	if err := s.settings.GetTyped(ctx, SettingEnabledPaymentTypes, &raw); err == nil {
		return NormalizeVisibleMethods(raw)
	}
	return nil
}

func (s *PaymentConfigService) readBoolSetting(ctx context.Context, key string, dst *bool) {
	var v bool
	if err := s.settings.GetTyped(ctx, key, &v); err == nil {
		*dst = v
	}
}

func (s *PaymentConfigService) readIntSetting(ctx context.Context, key string, dst *int) {
	var v int
	if err := s.settings.GetTyped(ctx, key, &v); err == nil && v != 0 {
		*dst = v
	}
}

// readDecimalSetting loads a decimal-valued setting with permissive
// parsing: stored numbers (float64) are accepted via NewFromFloat,
// strings via NewFromString. Missing / unparseable values leave dst
// untouched so callers keep their default.
func (s *PaymentConfigService) readDecimalSetting(ctx context.Context, key string, dst *decimal.Decimal) {
	// Try string form first — that is how the form persists values now
	// so precision is preserved through the round-trip.
	var asString string
	if err := s.settings.GetTyped(ctx, key, &asString); err == nil && strings.TrimSpace(asString) != "" {
		if d, err := decimal.NewFromString(strings.TrimSpace(asString)); err == nil {
			*dst = d
			return
		}
	}
	// Legacy: settings written before the decimal migration were stored
	// as JSON numbers; honour them via NewFromFloat so we don't break
	// existing deployments.
	var asFloat float64
	if err := s.settings.GetTyped(ctx, key, &asFloat); err == nil {
		*dst = decimal.NewFromFloat(asFloat)
	}
}

func (s *PaymentConfigService) readStringSetting(ctx context.Context, key string, dst *string) {
	var v string
	if err := s.settings.GetTyped(ctx, key, &v); err == nil && v != "" {
		*dst = v
	}
}

// getStripePublishableKey looks up the first enabled Stripe instance and
// extracts the publishable key from its decrypted config.
func (s *PaymentConfigService) getStripePublishableKey(ctx context.Context) string {
	if s == nil || s.entClient == nil {
		return ""
	}
	instances, err := s.entClient.PaymentProviderInstance.Query().
		Where(
			paymentproviderinstance.EnabledEQ(true),
			paymentproviderinstance.ProviderKeyEQ(payment.TypeStripe),
		).Limit(1).All(ctx)
	if err != nil || len(instances) == 0 {
		return ""
	}
	cfg, err := s.decryptConfig(ctx, instances[0].Config)
	if err != nil || cfg == nil {
		return ""
	}
	return cfg[payment.ConfigKeyPublishableKey]
}

// decryptConfig decrypts a stored provider-instance config blob.
//
// Layered handling: plaintext JSON wins (new records use that, identical
// to host policy after the AES-GCM rollback). When that fails, fall back
// to the SDK SecretEncryptor — it owns the legacy AES path centrally
// and is the encrypt-side counterpart used by encryptConfig.
//
// Unreadable blobs degrade to nil so the admin can re-enter the config
// without the row being permanently quarantined.
func (s *PaymentConfigService) decryptConfig(ctx context.Context, stored string) (map[string]string, error) {
	if stored == "" || s == nil {
		return nil, nil
	}
	if cfg, ok := tryDecodeConfigJSON(stored); ok {
		return cfg, nil
	}
	if s.secrets == nil {
		return nil, nil
	}
	plain, err := s.secrets.Decrypt(ctx, []byte(stored))
	if err != nil {
		// Treat unreadable blobs as empty so admins can re-enter the
		// config; matches host behaviour.
		if s.logger != nil {
			s.logger.Warn("payment provider config unreadable, treating as empty for re-entry",
				"stored_len", len(stored), "error", err)
		}
		return nil, nil
	}
	out := map[string]string{}
	if err := json.Unmarshal(plain, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// tryDecodeConfigJSON returns (cfg, true) when stored is a plaintext JSON
// object representing a provider config map.
func tryDecodeConfigJSON(stored string) (map[string]string, bool) {
	var cfg map[string]string
	if err := json.Unmarshal([]byte(stored), &cfg); err != nil {
		return nil, false
	}
	return cfg, true
}

// encryptConfig serialises a provider config for storage.
//
// New records are written as plaintext JSON, matching the host's
// post-rollback behaviour (decryptConfig accepts both plaintext and
// the legacy SDK-encrypted form during migration). Keeping ciphertext
// confined to the legacy path means newly written rows survive a
// SecretEncryptor key rotation without re-encrypting the whole table.
func (s *PaymentConfigService) encryptConfig(_ context.Context, cfg map[string]string) (string, error) {
	if s == nil {
		return "", errors.New("payment: config service unavailable")
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
