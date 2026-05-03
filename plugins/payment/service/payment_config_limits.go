package service

import (
	"context"
	"errors"

	pluginent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
	"github.com/Wei-Shaw/sub2api/plugins/payment/ent/paymentproviderinstance"
)

// GetAvailableMethodLimits returns the union of per-method limits derived
// from enabled provider instances.
//
// TODO(payment-migration): the original aggregation walked every enabled
// instance, applied visible-method routing, and unioned (single_min, single_max,
// daily_limit, fee_rate). The current stub returns an empty MethodLimitsResponse
// (which the frontend treats as "no limits configured") so the endpoint
// stops 503-ing while the real aggregator is ported.
func (s *PaymentConfigService) GetAvailableMethodLimits(ctx context.Context) (*MethodLimitsResponse, error) {
	if s == nil || s.entClient == nil {
		return &MethodLimitsResponse{Methods: map[string]MethodLimits{}}, nil
	}
	// Trigger a side-effect-free DB read so the endpoint surfaces real
	// connectivity errors instead of silently returning empty.
	_, err := s.entClient.PaymentProviderInstance.Query().
		Where(paymentproviderinstance.EnabledEQ(true)).
		Count(ctx)
	if err != nil {
		return nil, err
	}
	return &MethodLimitsResponse{Methods: map[string]MethodLimits{}}, nil
}

// GetMethodLimits returns the per-method limits filtered by the supplied types.
//
// TODO(payment-migration): port the aggregation logic. Stubbed to empty.
func (s *PaymentConfigService) GetMethodLimits(_ context.Context, _ []string) ([]MethodLimits, error) {
	return nil, nil
}

// pcApplyEnabledVisibleMethodInstances filters typeInstances to those
// configured as enabled visible methods.
//
// TODO(payment-migration): port the visible-method enable/source filter.
// The stub returns the input unchanged so callers do not crash.
func (s *PaymentConfigService) pcApplyEnabledVisibleMethodInstances(_ context.Context, typeInstances map[string][]*pluginent.PaymentProviderInstance, _ []*pluginent.PaymentProviderInstance) map[string][]*pluginent.PaymentProviderInstance {
	return typeInstances
}

// errLimitsHelperUnused keeps the package-level error ergonomic for future helpers.
var _ = errors.New
