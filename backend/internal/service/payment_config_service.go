package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/paymentproviderinstance"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionplan"
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
	SettingProductNamePrefix   = "PRODUCT_NAME_PREFIX"
	SettingProductNameSuffix   = "PRODUCT_NAME_SUFFIX"
	SettingHelpImageURL       = "PAYMENT_HELP_IMAGE_URL"
	SettingHelpText           = "PAYMENT_HELP_TEXT"
	SettingCancelRateLimitOn   = "CANCEL_RATE_LIMIT_ENABLED"
	SettingCancelRateLimitMax  = "CANCEL_RATE_LIMIT_MAX"
	SettingCancelWindowSize    = "CANCEL_RATE_LIMIT_WINDOW"
	SettingCancelWindowUnit    = "CANCEL_RATE_LIMIT_UNIT"
	SettingCancelWindowMode    = "CANCEL_RATE_LIMIT_WINDOW_MODE"
)

// Default values for payment configuration settings.
const (
	defaultMinRechargeAmount = 1
	defaultMaxRechargeAmount = 99999999.99
	defaultOrderTimeoutMin   = 30
	defaultMaxPendingOrders  = 3
)

// PaymentConfig holds the payment system configuration.
type PaymentConfig struct {
	Enabled             bool     `json:"enabled"`
	MinAmount           float64  `json:"min_amount"`
	MaxAmount           float64  `json:"max_amount"`
	DailyLimit          float64  `json:"daily_limit"`
	OrderTimeoutMin     int      `json:"order_timeout_minutes"`
	MaxPendingOrders    int      `json:"max_pending_orders"`
	EnabledTypes        []string `json:"enabled_payment_types"`
	BalanceDisabled     bool     `json:"balance_disabled"`
	LoadBalanceStrategy string   `json:"load_balance_strategy"`
	ProductNamePrefix   string   `json:"product_name_prefix"`
	ProductNameSuffix   string   `json:"product_name_suffix"`
	HelpImageURL        string   `json:"help_image_url"`
	HelpText            string   `json:"help_text"`
}

// UpdatePaymentConfigRequest contains fields to update payment configuration.
type UpdatePaymentConfigRequest struct {
	Enabled             *bool    `json:"enabled"`
	MinAmount           *float64 `json:"min_amount"`
	MaxAmount           *float64 `json:"max_amount"`
	DailyLimit          *float64 `json:"daily_limit"`
	OrderTimeoutMin     *int     `json:"order_timeout_minutes"`
	MaxPendingOrders    *int     `json:"max_pending_orders"`
	EnabledTypes        []string `json:"enabled_payment_types"`
	BalanceDisabled     *bool    `json:"balance_disabled"`
	LoadBalanceStrategy *string  `json:"load_balance_strategy"`
	ProductNamePrefix   *string  `json:"product_name_prefix"`
	ProductNameSuffix   *string  `json:"product_name_suffix"`
	HelpImageURL        *string  `json:"help_image_url"`
	HelpText            *string  `json:"help_text"`
}

// MethodLimits holds per-payment-type limits.
type MethodLimits struct {
	PaymentType string  `json:"paymentType"`
	FeeRate     float64 `json:"feeRate"`
	DailyLimit  float64 `json:"daily_limit"`
	SingleMin   float64 `json:"singleMin"`
	SingleMax   float64 `json:"singleMax"`
}

type CreateProviderInstanceRequest struct {
	ProviderKey    string            `json:"providerKey"`
	Name           string            `json:"name"`
	Config         map[string]string `json:"config"`
	SupportedTypes string            `json:"supportedTypes"`
	Enabled        bool              `json:"enabled"`
	SortOrder      int               `json:"sortOrder"`
	Limits         string            `json:"limits"`
	RefundEnabled  bool              `json:"refundEnabled"`
}

type UpdateProviderInstanceRequest struct {
	Name           *string           `json:"name"`
	Config         map[string]string `json:"config"`
	SupportedTypes *string           `json:"supportedTypes"`
	Enabled        *bool             `json:"enabled"`
	SortOrder      *int              `json:"sortOrder"`
	Limits         *string           `json:"limits"`
	RefundEnabled  *bool             `json:"refundEnabled"`
}
type CreatePlanRequest struct {
	GroupID       int64    `json:"groupId"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Price         float64  `json:"price"`
	OriginalPrice *float64 `json:"originalPrice"`
	ValidityDays  int      `json:"validityDays"`
	ValidityUnit  string   `json:"validityUnit"`
	Features      string   `json:"features"`
	ProductName   string   `json:"productName"`
	ForSale       bool     `json:"forSale"`
	SortOrder     int      `json:"sortOrder"`
}

type UpdatePlanRequest struct {
	GroupID       *int64   `json:"groupId"`
	Name          *string  `json:"name"`
	Description   *string  `json:"description"`
	Price         *float64 `json:"price"`
	OriginalPrice *float64 `json:"originalPrice"`
	ValidityDays  *int     `json:"validityDays"`
	ValidityUnit  *string  `json:"validityUnit"`
	Features      *string  `json:"features"`
	ProductName   *string  `json:"productName"`
	ForSale       *bool    `json:"forSale"`
	SortOrder     *int     `json:"sortOrder"`
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
		SettingEnabledPaymentTypes, SettingBalancePayDisabled, SettingLoadBalanceStrategy,
		SettingProductNamePrefix, SettingProductNameSuffix,
		SettingHelpImageURL, SettingHelpText,
	}
	vals, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("get payment config settings: %w", err)
	}
	return s.parsePaymentConfig(vals), nil
}

func (s *PaymentConfigService) parsePaymentConfig(vals map[string]string) *PaymentConfig {
	cfg := &PaymentConfig{
		Enabled:             vals[SettingPaymentEnabled] == "true",
		MinAmount:           pcParseFloat(vals[SettingMinRechargeAmount], defaultMinRechargeAmount),
		MaxAmount:           pcParseFloat(vals[SettingMaxRechargeAmount], defaultMaxRechargeAmount),
		DailyLimit:          pcParseFloat(vals[SettingDailyRechargeLimit], 0),
		OrderTimeoutMin:     pcParseInt(vals[SettingOrderTimeoutMinutes], defaultOrderTimeoutMin),
		MaxPendingOrders:    pcParseInt(vals[SettingMaxPendingOrders], defaultMaxPendingOrders),
		BalanceDisabled:     vals[SettingBalancePayDisabled] == "true",
		LoadBalanceStrategy: vals[SettingLoadBalanceStrategy],
		ProductNamePrefix:   vals[SettingProductNamePrefix],
		ProductNameSuffix:   vals[SettingProductNameSuffix],
		HelpImageURL:        vals[SettingHelpImageURL],
		HelpText:            vals[SettingHelpText],
	}
	if cfg.LoadBalanceStrategy == "" {
		cfg.LoadBalanceStrategy = "round-robin"
	}
	if raw := vals[SettingEnabledPaymentTypes]; raw != "" {
		for _, t := range strings.Split(raw, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				cfg.EnabledTypes = append(cfg.EnabledTypes, t)
			}
		}
	}
	return cfg
}

// UpdatePaymentConfig updates the payment configuration settings.
// NOTE: This function exceeds 30 lines because each field requires an independent
// nil-check before serialisation — this is inherent to patch-style update patterns
// and cannot be meaningfully decomposed without introducing unnecessary abstraction.
func (s *PaymentConfigService) UpdatePaymentConfig(ctx context.Context, req UpdatePaymentConfigRequest) error {
	m := make(map[string]string)
	if req.Enabled != nil {
		m[SettingPaymentEnabled] = strconv.FormatBool(*req.Enabled)
	}
	if req.MinAmount != nil {
		m[SettingMinRechargeAmount] = strconv.FormatFloat(*req.MinAmount, 'f', 2, 64)
	}
	if req.MaxAmount != nil {
		m[SettingMaxRechargeAmount] = strconv.FormatFloat(*req.MaxAmount, 'f', 2, 64)
	}
	if req.DailyLimit != nil {
		m[SettingDailyRechargeLimit] = strconv.FormatFloat(*req.DailyLimit, 'f', 2, 64)
	}
	if req.OrderTimeoutMin != nil {
		m[SettingOrderTimeoutMinutes] = strconv.Itoa(*req.OrderTimeoutMin)
	}
	if req.MaxPendingOrders != nil {
		m[SettingMaxPendingOrders] = strconv.Itoa(*req.MaxPendingOrders)
	}
	if req.EnabledTypes != nil {
		m[SettingEnabledPaymentTypes] = strings.Join(req.EnabledTypes, ",")
	}
	if req.BalanceDisabled != nil {
		m[SettingBalancePayDisabled] = strconv.FormatBool(*req.BalanceDisabled)
	}
	if req.LoadBalanceStrategy != nil {
		m[SettingLoadBalanceStrategy] = *req.LoadBalanceStrategy
	}
	if req.ProductNamePrefix != nil {
		m[SettingProductNamePrefix] = *req.ProductNamePrefix
	}
	if req.ProductNameSuffix != nil {
		m[SettingProductNameSuffix] = *req.ProductNameSuffix
	}
	if req.HelpImageURL != nil {
		m[SettingHelpImageURL] = *req.HelpImageURL
	}
	if req.HelpText != nil {
		m[SettingHelpText] = *req.HelpText
	}
	if len(m) == 0 {
		return nil
	}
	return s.settingRepo.SetMultiple(ctx, m)
}

// --- Provider Instance CRUD ---

func (s *PaymentConfigService) ListProviderInstances(ctx context.Context) ([]*dbent.PaymentProviderInstance, error) {
	return s.entClient.PaymentProviderInstance.Query().Order(paymentproviderinstance.BySortOrder()).All(ctx)
}

// ProviderInstanceResponse is the API response for a provider instance.
// Config contains non-sensitive fields in cleartext; sensitive fields are masked.
type ProviderInstanceResponse struct {
	ID             int64             `json:"id"`
	ProviderKey    string            `json:"provider_key"`
	Name           string            `json:"name"`
	Config         map[string]string `json:"config"`
	SupportedTypes string            `json:"supported_types"`
	Limits         string            `json:"limits"`
	Enabled        bool              `json:"enabled"`
	RefundEnabled  bool              `json:"refund_enabled"`
	SortOrder      int               `json:"sort_order"`
}

// ListProviderInstancesWithConfig returns provider instances with decrypted
// non-sensitive config fields and masked sensitive fields.
func (s *PaymentConfigService) ListProviderInstancesWithConfig(ctx context.Context) ([]ProviderInstanceResponse, error) {
	instances, err := s.entClient.PaymentProviderInstance.Query().
		Order(paymentproviderinstance.BySortOrder()).All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]ProviderInstanceResponse, 0, len(instances))
	for _, inst := range instances {
		resp := ProviderInstanceResponse{
			ID: int64(inst.ID), ProviderKey: inst.ProviderKey, Name: inst.Name,
			SupportedTypes: inst.SupportedTypes, Limits: inst.Limits,
			Enabled: inst.Enabled, RefundEnabled: inst.RefundEnabled, SortOrder: inst.SortOrder,
		}
		resp.Config = s.decryptAndMaskConfig(inst.Config)
		result = append(result, resp)
	}
	return result, nil
}

func (s *PaymentConfigService) decryptAndMaskConfig(encrypted string) map[string]string {
	raw := s.decryptConfig(encrypted)
	if raw == nil {
		return nil
	}
	masked := make(map[string]string, len(raw))
	for k, v := range raw {
		if isSensitiveConfigField(k) && v != "" {
			masked[k] = "••••••••"
		} else {
			masked[k] = v
		}
	}
	return masked
}

// pendingOrderStatuses are order statuses considered "in progress" —
// modifying provider credentials or deleting a provider/plan is blocked
// while orders in these states exist.
var pendingOrderStatuses = []string{
	payment.OrderStatusPending,
	payment.OrderStatusPaid,
	payment.OrderStatusRecharging,
}

// sensitiveConfigPatterns are substrings that identify credential fields.
// Changes to these fields are blocked when the provider has pending orders.
var sensitiveConfigPatterns = []string{"key", "pkey", "secret", "private", "password"}

func isSensitiveConfigField(fieldName string) bool {
	lower := strings.ToLower(fieldName)
	for _, p := range sensitiveConfigPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func (s *PaymentConfigService) countPendingOrders(ctx context.Context, providerInstanceID int64) (int, error) {
	return s.entClient.PaymentOrder.Query().
		Where(
			paymentorder.ProviderInstanceIDEQ(strconv.FormatInt(providerInstanceID, 10)),
			paymentorder.StatusIn(pendingOrderStatuses...),
		).Count(ctx)
}

func (s *PaymentConfigService) countPendingOrdersByPlan(ctx context.Context, planID int64) (int, error) {
	return s.entClient.PaymentOrder.Query().
		Where(
			paymentorder.PlanIDEQ(planID),
			paymentorder.StatusIn(pendingOrderStatuses...),
		).Count(ctx)
}

// validProviderKeys enumerates all recognized provider keys.
var validProviderKeys = map[string]bool{
	"easypay": true, "alipay": true, "wxpay": true, "stripe": true,
}

func (s *PaymentConfigService) CreateProviderInstance(ctx context.Context, req CreateProviderInstanceRequest) (*dbent.PaymentProviderInstance, error) {
	if err := validateProviderRequest(req.ProviderKey, req.Name, req.SupportedTypes); err != nil {
		return nil, err
	}
	enc, err := s.encryptConfig(req.Config)
	if err != nil {
		return nil, err
	}
	return s.entClient.PaymentProviderInstance.Create().
		SetProviderKey(req.ProviderKey).SetName(req.Name).SetConfig(enc).
		SetSupportedTypes(req.SupportedTypes).SetEnabled(req.Enabled).
		SetSortOrder(req.SortOrder).SetLimits(req.Limits).SetRefundEnabled(req.RefundEnabled).
		Save(ctx)
}

func validateProviderRequest(providerKey, name, supportedTypes string) error {
	if strings.TrimSpace(name) == "" {
		return infraerrors.BadRequest("VALIDATION_ERROR", "provider name is required")
	}
	if !validProviderKeys[providerKey] {
		return infraerrors.BadRequest("VALIDATION_ERROR", fmt.Sprintf("invalid provider key: %s", providerKey))
	}
	if strings.TrimSpace(supportedTypes) == "" {
		return infraerrors.BadRequest("VALIDATION_ERROR", "supported payment types are required")
	}
	return nil
}

func (s *PaymentConfigService) UpdateProviderInstance(ctx context.Context, id int64, req UpdateProviderInstanceRequest) (*dbent.PaymentProviderInstance, error) {
	// Check credential change safety when config is being modified
	if req.Config != nil {
		hasSensitive := false
		for k := range req.Config {
			if isSensitiveConfigField(k) && req.Config[k] != "" {
				hasSensitive = true
				break
			}
		}
		if hasSensitive {
			count, err := s.countPendingOrders(ctx, id)
			if err != nil {
				return nil, fmt.Errorf("check pending orders: %w", err)
			}
			if count > 0 {
				return nil, infraerrors.Conflict("PENDING_ORDERS",
					fmt.Sprintf("this instance has %d in-progress orders; changing credentials may break payment callbacks — wait for orders to complete or disable the instance first", count))
			}
		}
	}

	u := s.entClient.PaymentProviderInstance.UpdateOneID(id)
	if req.Name != nil {
		u.SetName(*req.Name)
	}
	if req.Config != nil {
		// Merge with existing config: preserve sensitive fields that were not re-submitted
		merged, err := s.mergeConfig(ctx, id, req.Config)
		if err != nil {
			return nil, err
		}
		enc, err := s.encryptConfig(merged)
		if err != nil {
			return nil, err
		}
		u.SetConfig(enc)
	}
	if req.SupportedTypes != nil {
		u.SetSupportedTypes(*req.SupportedTypes)
	}
	if req.Enabled != nil {
		u.SetEnabled(*req.Enabled)
	}
	if req.SortOrder != nil {
		u.SetSortOrder(*req.SortOrder)
	}
	if req.Limits != nil {
		u.SetLimits(*req.Limits)
	}
	if req.RefundEnabled != nil {
		u.SetRefundEnabled(*req.RefundEnabled)
	}
	return u.Save(ctx)
}

// mergeConfig merges new config with existing config, preserving sensitive
// fields that were not re-submitted (frontend skips masked ••••••••).
func (s *PaymentConfigService) mergeConfig(ctx context.Context, id int64, newConfig map[string]string) (map[string]string, error) {
	inst, err := s.entClient.PaymentProviderInstance.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load existing provider: %w", err)
	}
	existing := s.decryptConfig(inst.Config)
	if existing == nil {
		return newConfig, nil
	}
	// Start from existing, overwrite with new non-empty values
	for k, v := range newConfig {
		existing[k] = v
	}
	return existing, nil
}

func (s *PaymentConfigService) decryptConfig(encrypted string) map[string]string {
	if encrypted == "" {
		return nil
	}
	decrypted, err := payment.Decrypt(encrypted, s.encryptionKey)
	if err != nil {
		return nil
	}
	var raw map[string]string
	if err := json.Unmarshal([]byte(decrypted), &raw); err != nil {
		return nil
	}
	return raw
}

func (s *PaymentConfigService) DeleteProviderInstance(ctx context.Context, id int64) error {
	count, err := s.countPendingOrders(ctx, id)
	if err != nil {
		return fmt.Errorf("check pending orders: %w", err)
	}
	if count > 0 {
		return infraerrors.Conflict("PENDING_ORDERS",
			fmt.Sprintf("this instance has %d in-progress orders and cannot be deleted — wait for orders to complete or disable the instance first", count))
	}
	return s.entClient.PaymentProviderInstance.DeleteOneID(id).Exec(ctx)
}

func (s *PaymentConfigService) encryptConfig(cfg map[string]string) (string, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("marshal config: %w", err)
	}
	enc, err := payment.Encrypt(string(data), s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("encrypt config: %w", err)
	}
	return enc, nil
}

// --- Channel CRUD ---

// --- Plan CRUD ---

func (s *PaymentConfigService) ListPlans(ctx context.Context) ([]*dbent.SubscriptionPlan, error) {
	return s.entClient.SubscriptionPlan.Query().Order(subscriptionplan.BySortOrder()).All(ctx)
}

func (s *PaymentConfigService) ListPlansForSale(ctx context.Context) ([]*dbent.SubscriptionPlan, error) {
	return s.entClient.SubscriptionPlan.Query().Where(subscriptionplan.ForSaleEQ(true)).Order(subscriptionplan.BySortOrder()).All(ctx)
}

func (s *PaymentConfigService) CreatePlan(ctx context.Context, req CreatePlanRequest) (*dbent.SubscriptionPlan, error) {
	b := s.entClient.SubscriptionPlan.Create().
		SetGroupID(req.GroupID).SetName(req.Name).SetDescription(req.Description).
		SetPrice(req.Price).SetValidityDays(req.ValidityDays).SetValidityUnit(req.ValidityUnit).
		SetFeatures(req.Features).SetProductName(req.ProductName).
		SetForSale(req.ForSale).SetSortOrder(req.SortOrder)
	if req.OriginalPrice != nil {
		b.SetOriginalPrice(*req.OriginalPrice)
	}
	return b.Save(ctx)
}

// UpdatePlan updates a subscription plan by ID (patch semantics).
// NOTE: This function exceeds 30 lines due to per-field nil-check patch update
// boilerplate — same rationale as UpdatePaymentConfig.
func (s *PaymentConfigService) UpdatePlan(ctx context.Context, id int64, req UpdatePlanRequest) (*dbent.SubscriptionPlan, error) {
	u := s.entClient.SubscriptionPlan.UpdateOneID(id)
	if req.GroupID != nil {
		u.SetGroupID(*req.GroupID)
	}
	if req.Name != nil {
		u.SetName(*req.Name)
	}
	if req.Description != nil {
		u.SetDescription(*req.Description)
	}
	if req.Price != nil {
		u.SetPrice(*req.Price)
	}
	if req.OriginalPrice != nil {
		u.SetOriginalPrice(*req.OriginalPrice)
	}
	if req.ValidityDays != nil {
		u.SetValidityDays(*req.ValidityDays)
	}
	if req.ValidityUnit != nil {
		u.SetValidityUnit(*req.ValidityUnit)
	}
	if req.Features != nil {
		u.SetFeatures(*req.Features)
	}
	if req.ProductName != nil {
		u.SetProductName(*req.ProductName)
	}
	if req.ForSale != nil {
		u.SetForSale(*req.ForSale)
	}
	if req.SortOrder != nil {
		u.SetSortOrder(*req.SortOrder)
	}
	return u.Save(ctx)
}

func (s *PaymentConfigService) DeletePlan(ctx context.Context, id int64) error {
	count, err := s.countPendingOrdersByPlan(ctx, id)
	if err != nil {
		return fmt.Errorf("check pending orders: %w", err)
	}
	if count > 0 {
		return infraerrors.Conflict("PENDING_ORDERS",
			fmt.Sprintf("this plan has %d in-progress orders and cannot be deleted — wait for orders to complete first", count))
	}
	return s.entClient.SubscriptionPlan.DeleteOneID(id).Exec(ctx)
}

// GetPlan returns a subscription plan by ID.
func (s *PaymentConfigService) GetPlan(ctx context.Context, id int64) (*dbent.SubscriptionPlan, error) {
	plan, err := s.entClient.SubscriptionPlan.Get(ctx, id)
	if err != nil {
		return nil, infraerrors.NotFound("PLAN_NOT_FOUND", "subscription plan not found")
	}
	return plan, nil
}

// GetMethodLimits returns per-payment-type limits from enabled provider instances.
func (s *PaymentConfigService) GetMethodLimits(ctx context.Context, types []string) ([]MethodLimits, error) {
	instances, err := s.entClient.PaymentProviderInstance.Query().
		Where(paymentproviderinstance.EnabledEQ(true)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query provider instances: %w", err)
	}
	result := make([]MethodLimits, 0, len(types))
	for _, pt := range types {
		ml := MethodLimits{PaymentType: pt}
		for _, inst := range instances {
			if !payment.InstanceSupportsType(inst.SupportedTypes, pt) {
				continue
			}
			pcApplyInstanceLimits(inst, pt, &ml)
		}
		result = append(result, ml)
	}
	return result, nil
}

func pcApplyInstanceLimits(inst *dbent.PaymentProviderInstance, pt string, ml *MethodLimits) {
	if inst.Limits == "" {
		return
	}
	var limits payment.InstanceLimits
	if err := json.Unmarshal([]byte(inst.Limits), &limits); err != nil {
		return
	}
	cl, ok := limits[pt]
	if !ok {
		return
	}
	if cl.DailyLimit > 0 && (ml.DailyLimit == 0 || cl.DailyLimit < ml.DailyLimit) {
		ml.DailyLimit = cl.DailyLimit
	}
	if cl.SingleMin > 0 && (ml.SingleMin == 0 || cl.SingleMin > ml.SingleMin) {
		ml.SingleMin = cl.SingleMin
	}
	if cl.SingleMax > 0 && (ml.SingleMax == 0 || cl.SingleMax < ml.SingleMax) {
		ml.SingleMax = cl.SingleMax
	}
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
