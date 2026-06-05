package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	pluginent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
	"github.com/Wei-Shaw/sub2api/plugins/payment/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/plugins/payment/ent/paymentproviderinstance"
	infraerrors "github.com/Wei-Shaw/sub2api/plugins/payment/internal/errors"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment/provider"
)

// pendingOrderStatuses are order statuses considered "in progress".
// Updates that change provider identity are blocked while any order is in
// one of these statuses to avoid stranding webhooks against a rotated
// merchant.
var pendingOrderStatuses = []string{
	payment.OrderStatusPending,
	payment.OrderStatusPaid,
	payment.OrderStatusRecharging,
}

// providerSensitiveConfigFields is the authoritative list of config keys that
// are treated as secrets per provider. Must stay in sync with the frontend
// definition at frontend/src/components/payment/providerConfig.ts.
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
// changed while the instance has in-progress orders. Includes secrets plus
// all provider identity fields snapshotted into orders or used by
// webhook/refund verification.
var providerPendingOrderProtectedConfigFields = map[string]map[string]struct{}{
	payment.TypeEasyPay: {"pkey": {}, "pid": {}},
	payment.TypeAlipay:  {"privatekey": {}, "publickey": {}, "alipaypublickey": {}, "appid": {}},
	payment.TypeWxpay:   {"privatekey": {}, "apiv3key": {}, "publickey": {}, "appid": {}, "mpappid": {}, "mchid": {}, "publickeyid": {}, "certserial": {}},
	payment.TypeStripe:  {"secretkey": {}, "webhooksecret": {}},
}

var validProviderKeys = map[string]bool{
	payment.TypeEasyPay: true, payment.TypeAlipay: true, payment.TypeWxpay: true, payment.TypeStripe: true,
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

// validateProviderConfig surfaces config-level errors at save time by running
// the provider's constructor. Returns the structured ApplicationError so the
// frontend i18n layer can localize it.
func (s *PaymentConfigService) validateProviderConfig(providerKey string, config map[string]string) error {
	_, err := provider.CreateProvider(providerKey, "_validate_", config)
	return err
}

// ListProviderInstances returns every payment_provider_instances row.
func (s *PaymentConfigService) ListProviderInstances(ctx context.Context) ([]*pluginent.PaymentProviderInstance, error) {
	if s == nil || s.entClient == nil {
		return nil, nil
	}
	return s.entClient.PaymentProviderInstance.Query().
		Order(paymentproviderinstance.BySortOrder()).
		All(ctx)
}

// ListProviderInstancesWithConfig returns the admin-facing instance rows
// with their decrypted+masked config blobs. Sensitive secrets are stripped
// per provider type before returning to keep raw secrets server-side.
func (s *PaymentConfigService) ListProviderInstancesWithConfig(ctx context.Context) ([]ProviderInstanceResponse, error) {
	instances, err := s.ListProviderInstances(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ProviderInstanceResponse, 0, len(instances))
	for _, inst := range instances {
		resp, err := s.buildProviderInstanceResponse(ctx, inst)
		if err != nil {
			return nil, err
		}
		out = append(out, resp)
	}
	return out, nil
}

// buildProviderInstanceResponse constructs a single admin-facing response
// for a provider instance with its decrypted+masked config.
func (s *PaymentConfigService) buildProviderInstanceResponse(ctx context.Context, inst *pluginent.PaymentProviderInstance) (ProviderInstanceResponse, error) {
	cfg, err := s.decryptAndMaskConfig(ctx, inst.ProviderKey, inst.Config)
	if err != nil {
		return ProviderInstanceResponse{}, fmt.Errorf("decrypt config for instance %d: %w", inst.ID, err)
	}
	return ProviderInstanceResponse{
		ID:              int64(inst.ID),
		ProviderKey:     inst.ProviderKey,
		Name:            inst.Name,
		Config:          cfg,
		SupportedTypes:  splitTypes(inst.SupportedTypes),
		Enabled:         inst.Enabled,
		PaymentMode:     inst.PaymentMode,
		SortOrder:       inst.SortOrder,
		Limits:          inst.Limits,
		RefundEnabled:   inst.RefundEnabled,
		AllowUserRefund: inst.AllowUserRefund,
	}, nil
}

// decryptAndMaskConfig returns the stored config with sensitive fields omitted.
// Admin UIs display masked placeholders for these; the raw values never leave
// the server.
func (s *PaymentConfigService) decryptAndMaskConfig(ctx context.Context, providerKey, encrypted string) (map[string]string, error) {
	cfg, err := s.decryptConfig(ctx, encrypted)
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

func (s *PaymentConfigService) countPendingOrders(ctx context.Context, providerInstanceID int64) (int, error) {
	return s.entClient.PaymentOrder.Query().
		Where(
			paymentorder.ProviderInstanceIDEQ(strconv.FormatInt(providerInstanceID, 10)),
			paymentorder.StatusIn(pendingOrderStatuses...),
		).Count(ctx)
}

// pendingOrderCounter caches the in-progress order count for an instance
// so the patch flow can short-circuit repeated DB queries when several
// protected fields change in one update.
type pendingOrderCounter struct {
	svc        *PaymentConfigService
	instanceID int64
	cached     *int
}

func (p *pendingOrderCounter) get(ctx context.Context) (int, error) {
	if p.cached != nil {
		return *p.cached, nil
	}
	count, err := p.svc.countPendingOrders(ctx, p.instanceID)
	if err != nil {
		return 0, fmt.Errorf("check pending orders: %w", err)
	}
	p.cached = &count
	return count, nil
}

// validateProviderUpdateBasics enforces visible-method and provider-key
// invariants for the next state regardless of which patch fields are set.
func (s *PaymentConfigService) validateProviderUpdateBasics(
	ctx context.Context,
	current *pluginent.PaymentProviderInstance,
	req UpdateProviderInstanceRequest,
) error {
	nextEnabled := current.Enabled
	if req.Enabled != nil {
		nextEnabled = *req.Enabled
	}
	nextSupportedTypes := current.SupportedTypes
	if req.SupportedTypes != nil {
		nextSupportedTypes = joinTypes(req.SupportedTypes)
	}
	return s.validateVisibleMethodEnablementConflicts(ctx, int64(current.ID), current.ProviderKey, nextSupportedTypes, nextEnabled)
}

// prepareProviderUpdateConfig merges the inbound patch over the stored
// config (preserving secrets the admin omitted) and refuses changes that
// rotate provider identity while in-flight orders exist.
func (s *PaymentConfigService) prepareProviderUpdateConfig(
	ctx context.Context,
	current *pluginent.PaymentProviderInstance,
	req UpdateProviderInstanceRequest,
	counter *pendingOrderCounter,
) (map[string]string, error) {
	if req.Config == nil {
		return nil, nil
	}
	currentConfig, err := s.decryptConfig(ctx, current.Config)
	if err != nil {
		return nil, fmt.Errorf("decrypt existing config: %w", err)
	}
	merged, err := s.mergeConfig(ctx, current, req.Config)
	if err != nil {
		return nil, err
	}
	if hasPendingOrderProtectedConfigChange(current.ProviderKey, currentConfig, merged) {
		count, err := counter.get(ctx)
		if err != nil {
			return nil, err
		}
		if count > 0 {
			return nil, pendingOrdersConflictError(count)
		}
	}
	return merged, nil
}

// pendingOrdersConflictError builds a standard conflict error for pending orders.
func pendingOrdersConflictError(count int) error {
	return infraerrors.Conflict("PENDING_ORDERS", "instance has pending orders").
		WithMetadata(map[string]string{"count": strconv.Itoa(count)})
}

// guardProviderUpdateDisable refuses to disable an instance while it has
// in-flight orders — they would lose their reconciliation path.
func (s *PaymentConfigService) guardProviderUpdateDisable(
	ctx context.Context,
	req UpdateProviderInstanceRequest,
	counter *pendingOrderCounter,
) error {
	if req.Enabled == nil || *req.Enabled {
		return nil
	}
	count, err := counter.get(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return pendingOrdersConflictError(count)
	}
	return nil
}

// validateMergedProviderConfig runs the provider constructor on the
// next-state config when the resulting instance will be enabled. This
// surfaces provider-level errors (e.g. wxpay missing certSerial) at save
// time instead of only when an order is created.
func (s *PaymentConfigService) validateMergedProviderConfig(
	ctx context.Context,
	current *pluginent.PaymentProviderInstance,
	req UpdateProviderInstanceRequest,
	mergedConfig map[string]string,
) error {
	finalEnabled := current.Enabled
	if req.Enabled != nil {
		finalEnabled = *req.Enabled
	}
	if !finalEnabled {
		return nil
	}
	cfg := mergedConfig
	if cfg == nil {
		decoded, err := s.decryptConfig(ctx, current.Config)
		if err != nil {
			return fmt.Errorf("decrypt existing config: %w", err)
		}
		cfg = decoded
	}
	return s.validateProviderConfig(current.ProviderKey, cfg)
}

// guardSupportedTypesShrink rejects supported-type removals while orders
// referencing the dropped types are still in flight.
func (s *PaymentConfigService) guardSupportedTypesShrink(
	ctx context.Context,
	current *pluginent.PaymentProviderInstance,
	req UpdateProviderInstanceRequest,
	counter *pendingOrderCounter,
) error {
	if req.SupportedTypes == nil {
		return nil
	}
	count, err := counter.get(ctx)
	if err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	if hasSupportedTypeRemoval(current.SupportedTypes, req.SupportedTypes) {
		return infraerrors.Conflict("PENDING_ORDERS", "cannot remove payment types while instance has pending orders").
			WithMetadata(map[string]string{"count": strconv.Itoa(count)})
	}
	return nil
}

// hasSupportedTypeRemoval checks if any old type was removed from new types.
func hasSupportedTypeRemoval(oldTypesCSV string, newTypes []string) bool {
	oldTypes := strings.Split(oldTypesCSV, ",")
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
			return true
		}
	}
	return false
}

// applyProviderUpdate performs the actual ent UpdateOne after all validation
// passed. Each setter is gated by the patch nullability so callers can
// toggle individual fields without resending the whole record.
func (s *PaymentConfigService) applyProviderUpdate(
	ctx context.Context,
	current *pluginent.PaymentProviderInstance,
	req UpdateProviderInstanceRequest,
	mergedConfig map[string]string,
) (*pluginent.PaymentProviderInstance, error) {
	u := s.entClient.PaymentProviderInstance.UpdateOneID(current.ID)
	applyProviderScalarFields(u, req)
	if mergedConfig != nil {
		enc, err := s.encryptConfig(ctx, mergedConfig)
		if err != nil {
			return nil, err
		}
		u.SetConfig(enc)
	}
	if req.SupportedTypes != nil {
		u.SetSupportedTypes(joinTypes(req.SupportedTypes))
	}
	applyProviderRefundFields(u, current, req)
	return u.Save(ctx)
}

// applyProviderScalarFields sets the non-config, non-refund scalar fields.
func applyProviderScalarFields(u *pluginent.PaymentProviderInstanceUpdateOne, req UpdateProviderInstanceRequest) {
	if req.Name != nil {
		u.SetName(*req.Name)
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
	if req.PaymentMode != nil {
		u.SetPaymentMode(*req.PaymentMode)
	}
}

// applyProviderRefundFields encapsulates the cascading rules between
// refund_enabled / allow_user_refund. Disabling refund cascades to
// allow_user_refund=false; enabling allow_user_refund requires
// refund_enabled true.
func applyProviderRefundFields(
	u *pluginent.PaymentProviderInstanceUpdateOne,
	current *pluginent.PaymentProviderInstance,
	req UpdateProviderInstanceRequest,
) {
	if req.RefundEnabled != nil {
		u.SetRefundEnabled(*req.RefundEnabled)
		if !*req.RefundEnabled {
			u.SetAllowUserRefund(false)
		}
	}
	if req.AllowUserRefund == nil {
		return
	}
	if !*req.AllowUserRefund {
		u.SetAllowUserRefund(false)
		return
	}
	refundEnabled := current.RefundEnabled
	if req.RefundEnabled != nil {
		refundEnabled = *req.RefundEnabled
	}
	if refundEnabled {
		u.SetAllowUserRefund(true)
	}
}

func (s *PaymentConfigService) mergeConfig(ctx context.Context, current *pluginent.PaymentProviderInstance, newConfig map[string]string) (map[string]string, error) {
	existing, err := s.decryptConfig(ctx, current.Config)
	if err != nil {
		return nil, fmt.Errorf("decrypt existing config for instance %d: %w", current.ID, err)
	}
	if existing == nil {
		existing = map[string]string{}
	}
	for k, v := range newConfig {
		// Preserve existing secrets when the client submits an empty value
		// (admin UI omits the value to indicate "leave unchanged").
		if v == "" && isSensitiveProviderConfigField(current.ProviderKey, k) {
			continue
		}
		existing[k] = v
	}
	return existing, nil
}
