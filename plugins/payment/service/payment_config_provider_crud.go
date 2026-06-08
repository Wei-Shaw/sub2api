package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	pluginent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
	"github.com/Wei-Shaw/sub2api/plugins/payment/ent/paymentproviderinstance"
	infraerrors "github.com/Wei-Shaw/sub2api/plugins/payment/internal/errors"
)

// CreateProviderInstance inserts a new instance after validating the
// supplied config when the instance starts enabled.
func (s *PaymentConfigService) CreateProviderInstance(ctx context.Context, req CreateProviderInstanceRequest) (*pluginent.PaymentProviderInstance, error) {
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
	enc, err := s.encryptConfig(ctx, req.Config)
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

func validateProviderRequest(providerKey, name, _ string) error {
	if strings.TrimSpace(name) == "" {
		return infraerrors.BadRequest("VALIDATION_ERROR", "provider name is required")
	}
	if !validProviderKeys[providerKey] {
		return infraerrors.BadRequest("VALIDATION_ERROR", fmt.Sprintf("invalid provider key: %s", providerKey))
	}
	return nil
}

// UpdateProviderInstance updates a provider instance by ID (patch semantics).
func (s *PaymentConfigService) UpdateProviderInstance(ctx context.Context, id int64, req UpdateProviderInstanceRequest) (*pluginent.PaymentProviderInstance, error) {
	current, err := s.entClient.PaymentProviderInstance.Get(ctx, int(id))
	if err != nil {
		return nil, fmt.Errorf("load provider instance: %w", err)
	}
	counter := &pendingOrderCounter{svc: s, instanceID: id}
	if err := s.validateProviderUpdateBasics(ctx, current, req); err != nil {
		return nil, err
	}
	mergedConfig, err := s.prepareProviderUpdateConfig(ctx, current, req, counter)
	if err != nil {
		return nil, err
	}
	if err := s.guardProviderUpdateDisable(ctx, req, counter); err != nil {
		return nil, err
	}
	if err := s.validateMergedProviderConfig(ctx, current, req, mergedConfig); err != nil {
		return nil, err
	}
	if err := s.guardSupportedTypesShrink(ctx, current, req, counter); err != nil {
		return nil, err
	}
	return s.applyProviderUpdate(ctx, current, req, mergedConfig)
}

// DeleteProviderInstance removes an instance after refusing while it has
// in-flight orders.
func (s *PaymentConfigService) DeleteProviderInstance(ctx context.Context, id int64) error {
	count, err := s.countPendingOrders(ctx, id)
	if err != nil {
		return fmt.Errorf("check pending orders: %w", err)
	}
	if count > 0 {
		return infraerrors.Conflict("PENDING_ORDERS",
			fmt.Sprintf("this instance has %d in-progress orders and cannot be deleted — wait for orders to complete or disable the instance first", count))
	}
	return s.entClient.PaymentProviderInstance.DeleteOneID(int(id)).Exec(ctx)
}

// GetUserRefundEligibleInstanceIDs returns the IDs of provider instances
// where allow_user_refund is true and refund is enabled.
func (s *PaymentConfigService) GetUserRefundEligibleInstanceIDs(ctx context.Context) ([]string, error) {
	if s == nil || s.entClient == nil {
		return nil, nil
	}
	instances, err := s.entClient.PaymentProviderInstance.Query().
		Where(
			paymentproviderinstance.RefundEnabledEQ(true),
			paymentproviderinstance.AllowUserRefundEQ(true),
			paymentproviderinstance.EnabledEQ(true),
		).All(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(instances))
	for _, inst := range instances {
		ids = append(ids, strconv.FormatInt(int64(inst.ID), 10))
	}
	return ids, nil
}
