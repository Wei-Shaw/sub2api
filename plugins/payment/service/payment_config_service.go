package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

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
	RechargeFeeRate           float64  `json:"recharge_fee_rate"`
	LoadBalanceStrategy       string   `json:"load_balance_strategy"`
	ProductNamePrefix         string   `json:"product_name_prefix"`
	ProductNameSuffix         string   `json:"product_name_suffix"`
	HelpImageURL              string   `json:"help_image_url"`
	HelpText                  string   `json:"help_text"`
	StripePublishableKey      string   `json:"stripe_publishable_key,omitempty"`

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

// UpdatePaymentConfigRequest is kept for handler signature compatibility.
// The plugin no longer accepts writes — settings are read-only and owned
// by the host admin form.
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
	RechargeFeeRate           *float64 `json:"recharge_fee_rate"`
	LoadBalanceStrategy       *string  `json:"load_balance_strategy"`
	ProductNamePrefix         *string  `json:"product_name_prefix"`
	ProductNameSuffix         *string  `json:"product_name_suffix"`
	HelpImageURL              *string  `json:"help_image_url"`
	HelpText                  *string  `json:"help_text"`

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

// MethodLimits holds per-payment-type limits.
type MethodLimits struct {
	PaymentType string  `json:"payment_type"`
	FeeRate     float64 `json:"fee_rate"`
	DailyLimit  float64 `json:"daily_limit"`
	SingleMin   float64 `json:"single_min"`
	SingleMax   float64 `json:"single_max"`
}

// MethodLimitsResponse is the full /limits API response.
type MethodLimitsResponse struct {
	Methods   map[string]MethodLimits `json:"methods"`
	GlobalMin float64                 `json:"global_min"`
	GlobalMax float64                 `json:"global_max"`
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
	ValidityDays  *int     `json:"validity_days"`
	ValidityUnit  *string  `json:"validity_unit"`
	Features      *string  `json:"features"`
	ProductName   *string  `json:"product_name"`
	ForSale       *bool    `json:"for_sale"`
	SortOrder     *int     `json:"sort_order"`
}

// SubscriptionPlan is a plugin-local mirror of the core's subscription_plans
// row.
type SubscriptionPlan struct {
	ID            int
	GroupID       int64
	Name          string
	Description   string
	Price         float64
	OriginalPrice *float64
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

// errPaymentSettingsWriteNotSupported is returned by UpdatePaymentConfig.
var errPaymentSettingsWriteNotSupported = errors.New("payment: settings updates are owned by the host admin form")

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
	s.readFloatSetting(ctx, SettingMinRechargeAmount, &cfg.MinAmount)
	s.readFloatSetting(ctx, SettingMaxRechargeAmount, &cfg.MaxAmount)
	s.readFloatSetting(ctx, SettingDailyRechargeLimit, &cfg.DailyLimit)
	s.readIntSetting(ctx, SettingOrderTimeoutMinutes, &cfg.OrderTimeoutMin)
	s.readIntSetting(ctx, SettingMaxPendingOrders, &cfg.MaxPendingOrders)
	s.readBoolSetting(ctx, SettingBalancePayDisabled, &cfg.BalanceDisabled)
	s.readFloatSetting(ctx, SettingBalanceRechargeMult, &cfg.BalanceRechargeMultiplier)
	cfg.BalanceRechargeMultiplier = normalizeBalanceRechargeMultiplier(cfg.BalanceRechargeMultiplier)
	s.readFloatSetting(ctx, SettingRechargeFeeRate, &cfg.RechargeFeeRate)
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

func (s *PaymentConfigService) readFloatSetting(ctx context.Context, key string, dst *float64) {
	var v float64
	if err := s.settings.GetTyped(ctx, key, &v); err == nil {
		*dst = v
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
// Layered handling:
//
//  1. Try the SDK SecretEncryptor first — new records are AES-256-GCM
//     ciphertext sealed with the plugin's per-plugin key.
//  2. Fall back to plaintext JSON for legacy rows written before the
//     S1 fix. These will be re-encrypted on the next admin save; we log
//     a warning so operators can prioritise the rotation.
//  3. Unreadable blobs degrade to nil so the admin can re-enter the
//     config without the row being permanently quarantined.
func (s *PaymentConfigService) decryptConfig(ctx context.Context, stored string) (map[string]string, error) {
	if stored == "" || s == nil {
		return nil, nil
	}
	if s.secrets != nil {
		plain, err := s.secrets.Decrypt(ctx, []byte(stored))
		if err == nil {
			out := map[string]string{}
			if jsonErr := json.Unmarshal(plain, &out); jsonErr == nil {
				return out, nil
			}
			// Decrypt succeeded but plaintext is not valid JSON — fall
			// through to plaintext fallback (very old data could be
			// corrupted or non-JSON).
		}
	}
	// Legacy fallback: plaintext JSON written before the S1 encryption
	// fix. Log a warning so operators know which rows still need
	// re-saving via the admin UI to be encrypted at rest.
	if cfg, ok := tryDecodeConfigJSON(stored); ok {
		if s.logger != nil {
			s.logger.Warn("provider config is unencrypted; will re-encrypt on next save",
				"stored_len", len(stored))
		}
		return cfg, nil
	}
	if s.logger != nil {
		s.logger.Warn("payment provider config unreadable, treating as empty for re-entry",
			"stored_len", len(stored))
	}
	return nil, nil
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
// New records are encrypted via the SDK SecretEncryptor (AES-256-GCM
// sealed with a per-plugin HKDF-derived key, see plugin-sdk/secrets.go).
// We fail closed when no encryptor is wired — provider configs hold
// merchant API keys, secret signing material and webhook keys; storing
// them in plaintext is treated as a configuration bug, not a graceful
// degradation.
func (s *PaymentConfigService) encryptConfig(ctx context.Context, cfg map[string]string) (string, error) {
	if s == nil {
		return "", errors.New("payment: config service unavailable")
	}
	if s.secrets == nil {
		return "", errors.New("payment: SecretEncryptor not configured; refusing to store provider config in plaintext")
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	ciphertext, err := s.secrets.Encrypt(ctx, raw)
	if err != nil {
		return "", fmt.Errorf("encrypt provider config: %w", err)
	}
	return string(ciphertext), nil
}

// UpdatePaymentConfig is intentionally a no-op: settings live in the host's
// admin form.
func (s *PaymentConfigService) UpdatePaymentConfig(_ context.Context, _ UpdatePaymentConfigRequest) error {
	return errPaymentSettingsWriteNotSupported
}

// ReencryptResult reports the outcome of a bulk re-encrypt run.
//
// Errors is bounded to the first reencryptMaxErrors entries to keep the
// admin response payload predictable; the per-row failures are also
// emitted via slog.Warn so operators can drill into a specific instance
// without scrolling the JSON body.
type ReencryptResult struct {
	Total       int      `json:"total"`
	Reencrypted int      `json:"reencrypted"`
	AlreadyOK   int      `json:"already_encrypted"`
	Failed      int      `json:"failed"`
	Errors      []string `json:"errors,omitempty"`
}

// reencryptMaxErrors caps how many per-row error strings the result body
// carries. Operators get a representative sample without an unbounded
// payload when many rows are corrupted.
const reencryptMaxErrors = 10

// ReencryptAllProviderConfigs walks every payment_provider_instances row,
// detects rows that are still stored as plaintext JSON (the legacy format
// used before encryptConfig started writing AES-256-GCM ciphertext) and
// rewrites them as encrypted. Rows whose Config blob already decrypts via
// the SDK SecretEncryptor are skipped.
//
// The operation is idempotent — running twice in a row yields
// AlreadyOK == Total on the second run.
//
// Each row is read+rewritten as an independent ent UpdateOneID call. We do
// not wrap the whole sweep in one transaction: a long-running TX would
// hold a lock against admin saves of unrelated instance rows for the
// duration of the run. The trade-off is that an interrupted run can leave
// the table partially migrated; the next invocation simply finishes the
// remaining rows because the helper is idempotent.
//
// When SecretEncryptor is not wired (CapabilitySecretEncryption missing)
// the call returns an error rather than silently doing nothing — the
// endpoint exists specifically to fix the encryption gap, so failing
// closed surfaces the misconfiguration immediately.
func (s *PaymentConfigService) ReencryptAllProviderConfigs(ctx context.Context) (ReencryptResult, error) {
	var result ReencryptResult
	if s == nil || s.entClient == nil {
		return result, errors.New("payment: config service not initialised")
	}
	if s.secrets == nil {
		return result, errors.New("payment: SecretEncryptor not configured; cannot re-encrypt")
	}

	instances, err := s.entClient.PaymentProviderInstance.Query().All(ctx)
	if err != nil {
		return result, fmt.Errorf("list provider instances: %w", err)
	}
	result.Total = len(instances)

	for _, inst := range instances {
		s.reencryptOneInstance(ctx, inst, &result)
	}
	return result, nil
}

// reencryptOneInstance reclassifies a single row as already-encrypted,
// freshly-re-encrypted, or failed and updates result accordingly. Split
// out so ReencryptAllProviderConfigs stays under the 30-line budget.
func (s *PaymentConfigService) reencryptOneInstance(ctx context.Context, inst *pluginent.PaymentProviderInstance, result *ReencryptResult) {
	stored := inst.Config
	if stored == "" {
		// Empty config — no secrets to migrate; treat as already-fine.
		result.AlreadyOK++
		return
	}

	// Probe 1: does it already decrypt under the SDK SecretEncryptor?
	if plain, err := s.secrets.Decrypt(ctx, []byte(stored)); err == nil {
		// Decrypted blob must look like a JSON object to be a valid
		// provider config. If it does not, fall through to the plaintext
		// path so the row is rewritten in canonical form.
		var probe map[string]string
		if jsonErr := json.Unmarshal(plain, &probe); jsonErr == nil {
			result.AlreadyOK++
			return
		}
	}

	// Probe 2: does it parse as plaintext JSON? If so this is a legacy
	// row — re-encrypt and write it back.
	cfg, ok := tryDecodeConfigJSON(stored)
	if !ok {
		s.recordReencryptFailure(result, inst.ID, fmt.Errorf("config is neither valid ciphertext nor plaintext JSON"))
		return
	}
	enc, err := s.encryptConfig(ctx, cfg)
	if err != nil {
		s.recordReencryptFailure(result, inst.ID, fmt.Errorf("encrypt: %w", err))
		return
	}
	if _, err := s.entClient.PaymentProviderInstance.UpdateOneID(inst.ID).
		SetConfig(enc).Save(ctx); err != nil {
		s.recordReencryptFailure(result, inst.ID, fmt.Errorf("save: %w", err))
		return
	}
	result.Reencrypted++
	if s.logger != nil {
		s.logger.Info("payment: provider config re-encrypted",
			"instance_id", inst.ID, "provider_key", inst.ProviderKey)
	}
}

// recordReencryptFailure increments the Failed counter, appends a
// truncated error message (bounded to reencryptMaxErrors), and emits a
// warn log so operators see every failure even when the response body
// truncates.
func (s *PaymentConfigService) recordReencryptFailure(result *ReencryptResult, instanceID int, err error) {
	result.Failed++
	if s.logger != nil {
		s.logger.Warn("payment: provider config re-encrypt failed",
			"instance_id", instanceID, "error", err)
	}
	if len(result.Errors) >= reencryptMaxErrors {
		return
	}
	result.Errors = append(result.Errors, fmt.Sprintf("instance %d: %v", instanceID, err))
}

// splitTypes splits a CSV settings value, trimming whitespace.
func splitTypes(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
