package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/paymentproviderinstance"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/payment/provider"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// validateProviderConfig runs the provider's constructor to surface config-level
// errors at save time (e.g. wxpay missing certSerial), instead of only failing
// when an order is created. Returns the structured ApplicationError from the
// constructor so the frontend i18n layer can localize it.
//
// Only validates enabled instances — a disabled instance may be a half-filled
// draft the admin will complete later.
func (s *PaymentConfigService) validateProviderConfig(providerKey string, config map[string]string) error {
	_, err := provider.CreateProvider(providerKey, "_validate_", config)
	return err
}

// --- Provider Instance CRUD ---

func (s *PaymentConfigService) ListProviderInstances(ctx context.Context) ([]*dbent.PaymentProviderInstance, error) {
	return s.entClient.PaymentProviderInstance.Query().Order(paymentproviderinstance.BySortOrder()).All(ctx)
}

// ProviderInstanceResponse is the API response for a provider instance.
type ProviderInstanceResponse struct {
	ID              int64             `json:"id"`
	ProviderKey     string            `json:"provider_key"`
	Name            string            `json:"name"`
	Config          map[string]string `json:"config"`
	SupportedTypes  []string          `json:"supported_types"`
	Limits          string            `json:"limits"`
	Enabled         bool              `json:"enabled"`
	RefundEnabled   bool              `json:"refund_enabled"`
	AllowUserRefund bool              `json:"allow_user_refund"`
	SortOrder       int               `json:"sort_order"`
	PaymentMode     string            `json:"payment_mode"`
}

// ListProviderInstancesWithConfig returns provider instances with decrypted config.
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
			SupportedTypes: splitTypes(inst.SupportedTypes), Limits: inst.Limits,
			Enabled: inst.Enabled, RefundEnabled: inst.RefundEnabled, AllowUserRefund: inst.AllowUserRefund,
			SortOrder: inst.SortOrder, PaymentMode: inst.PaymentMode,
		}
		resp.Config, err = s.decryptAndMaskConfig(inst.ProviderKey, inst.Config)
		if err != nil {
			return nil, fmt.Errorf("decrypt config for instance %d: %w", inst.ID, err)
		}
		result = append(result, resp)
	}
	return result, nil
}

// decryptAndMaskConfig returns the stored config with sensitive fields omitted.
// Admin UIs display masked placeholders for these; the raw values never leave
// the server. Callers that need the full config (e.g. payment runtime) must
// use decryptConfig directly.
func (s *PaymentConfigService) decryptAndMaskConfig(providerKey, encrypted string) (map[string]string, error) {
	cfg, err := s.decryptConfig(encrypted)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, nil
	}
	masked := make(map[string]string, len(cfg))
	for k, v := range cfg {
		if isSensitiveProviderConfigField(providerKey, k) {
			continue
		}
		masked[k] = v
	}
	return masked, nil
}

// pendingOrderStatuses are order statuses considered "in progress".
var pendingOrderStatuses = []string{
	payment.OrderStatusPending,
	payment.OrderStatusPaid,
	payment.OrderStatusRecharging,
}

// providerSensitiveConfigFields is the authoritative list of config keys that
// are treated as secrets per provider. Must stay in sync with the frontend
// definition at frontend/src/components/payment/providerConfig.ts
// (PROVIDER_CONFIG_FIELDS, fields with sensitive: true).
//
// Key matching is case-insensitive. Non-listed keys (e.g. appId, notifyUrl,
// stripe publishableKey) are returned in plaintext by the admin GET API.
var providerSensitiveConfigFields = map[string]map[string]struct{}{
	payment.TypeEasyPay: {"pkey": {}},
	payment.TypeAlipay:  {"privatekey": {}, "publickey": {}, "alipaypublickey": {}},
	payment.TypeWxpay:   {"privatekey": {}, "apiv3key": {}, "publickey": {}},
	payment.TypeStripe:  {"secretkey": {}, "webhooksecret": {}},
}

// providerPendingOrderProtectedConfigFields lists config keys that cannot be
// changed while the instance has in-progress orders. This includes secrets plus
// all provider identity fields that are snapshotted into orders or used by
// webhook/refund verification.
var providerPendingOrderProtectedConfigFields = map[string]map[string]struct{}{
	payment.TypeEasyPay: {"pkey": {}, "pid": {}},
	payment.TypeAlipay:  {"privatekey": {}, "publickey": {}, "alipaypublickey": {}, "appid": {}},
	payment.TypeWxpay:   {"privatekey": {}, "apiv3key": {}, "publickey": {}, "appid": {}, "mpappid": {}, "mchid": {}, "publickeyid": {}, "certserial": {}},
	payment.TypeStripe:  {"secretkey": {}, "webhooksecret": {}},
}

func isSensitiveProviderConfigField(providerKey, fieldName string) bool {
	fields, ok := providerSensitiveConfigFields[providerKey]
	if !ok {
		return false
	}
	_, found := fields[strings.ToLower(fieldName)]
	return found
}

func hasPendingOrderProtectedConfigChange(providerKey string, currentConfig, nextConfig map[string]string) bool {
	fields, ok := providerPendingOrderProtectedConfigFields[providerKey]
	if !ok {
		return false
	}
	for fieldName := range fields {
		if providerConfigFieldValue(currentConfig, fieldName) != providerConfigFieldValue(nextConfig, fieldName) {
			return true
		}
	}
	return false
}

func providerConfigFieldValue(config map[string]string, fieldName string) string {
	for key, value := range config {
		if strings.EqualFold(key, fieldName) {
			return value
		}
	}
	return ""
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

var validProviderKeys = map[string]bool{
	payment.TypeEasyPay: true, payment.TypeAlipay: true, payment.TypeWxpay: true, payment.TypeStripe: true,
}

func (s *PaymentConfigService) CreateProviderInstance(ctx context.Context, req CreateProviderInstanceRequest) (*dbent.PaymentProviderInstance, error) {
	typesStr := joinTypes(req.SupportedTypes)
	if err := validateProviderRequest(req.ProviderKey, req.Name, typesStr); err != nil {
		return nil, err
	}
	if err := s.validateVisibleMethodEnablementConflicts(ctx, 0, req.ProviderKey, typesStr, req.Enabled); err != nil {
		return nil, err
	}
	if req.Enabled {
		if err := s.validateProviderConfig(req.ProviderKey, req.Config); err != nil {
			return nil, err
		}
	}
	enc, err := s.encryptConfig(req.Config)
	if err != nil {
		return nil, err
	}
	allowUserRefund := req.AllowUserRefund && req.RefundEnabled
	return s.entClient.PaymentProviderInstance.Create().
		SetProviderKey(req.ProviderKey).SetName(req.Name).SetConfig(enc).
		SetSupportedTypes(typesStr).SetEnabled(req.Enabled).SetPaymentMode(req.PaymentMode).
		SetSortOrder(req.SortOrder).SetLimits(req.Limits).SetRefundEnabled(req.RefundEnabled).
		SetAllowUserRefund(allowUserRefund).
		Save(ctx)
}

func validateProviderRequest(providerKey, name, supportedTypes string) error {
	if strings.TrimSpace(name) == "" {
		return infraerrors.BadRequest("VALIDATION_ERROR", "provider name is required")
	}
	if !validProviderKeys[providerKey] {
		return infraerrors.BadRequest("VALIDATION_ERROR", fmt.Sprintf("invalid provider key: %s", providerKey))
	}
	// supported_types can be empty (provider accepts no payment types until configured)
	return nil
}

// UpdateProviderInstance updates a provider instance by ID (patch semantics).
// NOTE: This function exceeds 30 lines due to per-field nil-check patch update
// boilerplate and pending-order safety checks.
func (s *PaymentConfigService) UpdateProviderInstance(ctx context.Context, id int64, req UpdateProviderInstanceRequest) (*dbent.PaymentProviderInstance, error) {
	current, err := s.entClient.PaymentProviderInstance.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load provider instance: %w", err)
	}
	var pendingOrderCount *int
	getPendingOrderCount := func() (int, error) {
		if pendingOrderCount != nil {
			return *pendingOrderCount, nil
		}
		count, err := s.countPendingOrders(ctx, id)
		if err != nil {
			return 0, fmt.Errorf("check pending orders: %w", err)
		}
		pendingOrderCount = &count
		return count, nil
	}
	nextEnabled := current.Enabled
	if req.Enabled != nil {
		nextEnabled = *req.Enabled
	}
	nextSupportedTypes := current.SupportedTypes
	if req.SupportedTypes != nil {
		nextSupportedTypes = joinTypes(req.SupportedTypes)
	}
	if err := s.validateVisibleMethodEnablementConflicts(ctx, id, current.ProviderKey, nextSupportedTypes, nextEnabled); err != nil {
		return nil, err
	}
	var mergedConfig map[string]string
	if req.Config != nil {
		currentConfig, err := s.decryptConfig(current.Config)
		if err != nil {
			return nil, fmt.Errorf("decrypt existing config: %w", err)
		}
		mergedConfig, err = s.mergeConfig(ctx, id, req.Config)
		if err != nil {
			return nil, err
		}
		if hasPendingOrderProtectedConfigChange(current.ProviderKey, currentConfig, mergedConfig) {
			count, err := getPendingOrderCount()
			if err != nil {
				return nil, err
			}
			if count > 0 {
				return nil, infraerrors.Conflict("PENDING_ORDERS", "instance has pending orders").
					WithMetadata(map[string]string{"count": strconv.Itoa(count)})
			}
		}
	}
	if req.Enabled != nil && !*req.Enabled {
		count, err := getPendingOrderCount()
		if err != nil {
			return nil, err
		}
		if count > 0 {
			return nil, infraerrors.Conflict("PENDING_ORDERS", "instance has pending orders").
				WithMetadata(map[string]string{"count": strconv.Itoa(count)})
		}
	}
	// Validate merged config when the instance will end up enabled.
	// This surfaces provider-level errors (e.g. wxpay missing certSerial) at save time,
	// so admins see them in the dialog instead of only when an order is created.
	finalEnabled := current.Enabled
	if req.Enabled != nil {
		finalEnabled = *req.Enabled
	}
	if finalEnabled {
		configToValidate := mergedConfig
		if configToValidate == nil {
			configToValidate, err = s.decryptConfig(current.Config)
			if err != nil {
				return nil, fmt.Errorf("decrypt existing config: %w", err)
			}
		}
		if err := s.validateProviderConfig(current.ProviderKey, configToValidate); err != nil {
			return nil, err
		}
	}
	u := s.entClient.PaymentProviderInstance.UpdateOneID(id)
	if req.Name != nil {
		u.SetName(*req.Name)
	}
	if mergedConfig != nil {
		enc, err := s.encryptConfig(mergedConfig)
		if err != nil {
			return nil, err
		}
		u.SetConfig(enc)
	}
	if req.SupportedTypes != nil {
		// Check pending orders before removing payment types
		count, err := getPendingOrderCount()
		if err != nil {
			return nil, err
		}
		if count > 0 {
			// Load current instance to compare types
			oldTypes := strings.Split(current.SupportedTypes, ",")
			newTypes := req.SupportedTypes
			for _, ot := range oldTypes {
				ot = strings.TrimSpace(ot)
				if ot == "" {
					continue
				}
				found := false
				for _, nt := range newTypes {
					if strings.TrimSpace(nt) == ot {
						found = true
						break
					}
				}
				if !found {
					return nil, infraerrors.Conflict("PENDING_ORDERS", "cannot remove payment types while instance has pending orders").
						WithMetadata(map[string]string{"count": strconv.Itoa(count)})
				}
			}
		}
		u.SetSupportedTypes(joinTypes(req.SupportedTypes))
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
		// Cascade: turning off refund_enabled also disables allow_user_refund
		if !*req.RefundEnabled {
			u.SetAllowUserRefund(false)
		}
	}
	if req.AllowUserRefund != nil {
		// Only allow enabling when refund_enabled is (or will be) true
		if *req.AllowUserRefund {
			refundEnabled := false
			if req.RefundEnabled != nil {
				refundEnabled = *req.RefundEnabled
			} else {
				refundEnabled = current.RefundEnabled
			}
			if refundEnabled {
				u.SetAllowUserRefund(true)
			}
		} else {
			u.SetAllowUserRefund(false)
		}
	}
	if req.PaymentMode != nil {
		u.SetPaymentMode(*req.PaymentMode)
	}
	return u.Save(ctx)
}

// GetUserRefundEligibleInstanceIDs returns provider instance IDs that allow user refund.
func (s *PaymentConfigService) GetUserRefundEligibleInstanceIDs(ctx context.Context) ([]string, error) {
	instances, err := s.entClient.PaymentProviderInstance.Query().
		Where(
			paymentproviderinstance.RefundEnabledEQ(true),
			paymentproviderinstance.AllowUserRefundEQ(true),
		).Select(paymentproviderinstance.FieldID).All(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(instances))
	for _, inst := range instances {
		ids = append(ids, strconv.FormatInt(int64(inst.ID), 10))
	}
	return ids, nil
}

func (s *PaymentConfigService) mergeConfig(ctx context.Context, id int64, newConfig map[string]string) (map[string]string, error) {
	inst, err := s.entClient.PaymentProviderInstance.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load existing provider: %w", err)
	}
	existing, err := s.decryptConfig(inst.Config)
	if err != nil {
		return nil, fmt.Errorf("decrypt existing config for instance %d: %w", id, err)
	}
	if existing == nil {
		existing = map[string]string{}
	}
	for k, v := range newConfig {
		// Preserve existing secrets when the client submits an empty value
		// (admin UI omits the value to indicate "leave unchanged").
		if v == "" && isSensitiveProviderConfigField(inst.ProviderKey, k) {
			continue
		}
		existing[k] = v
	}
	return existing, nil
}

// decryptConfig parses a stored provider config.
// New records are plaintext JSON; legacy records are AES-256-GCM ciphertext
// ("iv:authTag:ciphertext"). Values that cannot be parsed as either — including
// legacy ciphertext with no/invalid TOTP_ENCRYPTION_KEY — are treated as empty,
// letting the admin re-enter the config via the UI to complete the migration.
//
// TODO(deprecated-legacy-ciphertext): The AES fallback branch is a transitional
// shim for pre-plaintext records. Remove it (and the encryptionKey field) after
// a few releases once all live deployments have re-saved their provider configs.
func (s *PaymentConfigService) decryptConfig(stored string) (map[string]string, error) {
	if stored == "" {
		return nil, nil
	}
	var cfg map[string]string
	if err := json.Unmarshal([]byte(stored), &cfg); err == nil {
		return cfg, nil
	}
	// Deprecated: legacy AES-256-GCM ciphertext fallback — scheduled for removal.
	if len(s.encryptionKey) == payment.AES256KeySize {
		//nolint:staticcheck // SA1019: intentional legacy fallback, scheduled for removal
		if plaintext, err := payment.Decrypt(stored, s.encryptionKey); err == nil {
			if err := json.Unmarshal([]byte(plaintext), &cfg); err == nil {
				return cfg, nil
			}
		}
	}
	slog.Warn("payment provider config unreadable, treating as empty for re-entry",
		"stored_len", len(stored))
	return nil, nil
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

// encryptConfig serialises a provider config for storage.
// New records are written as plaintext JSON; the historical AES-GCM wrapping
// has been dropped but decryptConfig still accepts old ciphertext during migration.
func (s *PaymentConfigService) encryptConfig(cfg map[string]string) (string, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("marshal config: %w", err)
	}
	return string(data), nil
}
