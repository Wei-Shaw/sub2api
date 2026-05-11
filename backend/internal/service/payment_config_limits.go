package service

import (
	"context"
	"encoding/json"
	"fmt"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentproviderinstance"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// GetAvailableMethodLimits collects all payment types from enabled provider
// instances and returns limits for each, plus the global widest range.
// Stripe sub-types (card, link) are aggregated under "stripe".
func (s *PaymentConfigService) GetAvailableMethodLimits(ctx context.Context) (*MethodLimitsResponse, error) {
	instances, err := s.entClient.PaymentProviderInstance.Query().
		Where(paymentproviderinstance.EnabledEQ(true)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query provider instances: %w", err)
	}
	typeInstances := pcGroupByPaymentType(instances)
	typeInstances = s.pcApplyEnabledVisibleMethodInstances(ctx, typeInstances, instances)
	resp := &MethodLimitsResponse{
		Methods: make(map[string]MethodLimits, len(typeInstances)),
	}
	for pt, insts := range typeInstances {
		currency, ok := s.pcAggregateMethodCurrency(insts)
		if !ok {
			continue
		}
		ml := pcAggregateMethodLimits(pt, insts)
		ml.Currency = currency
		resp.Methods[ml.PaymentType] = ml
	}
	resp.GlobalMin, resp.GlobalMax = pcComputeGlobalRange(resp.Methods)
	return resp, nil
}

func (s *PaymentConfigService) pcApplyEnabledVisibleMethodInstances(ctx context.Context, typeInstances map[string][]*dbent.PaymentProviderInstance, instances []*dbent.PaymentProviderInstance) map[string][]*dbent.PaymentProviderInstance {
	if len(typeInstances) == 0 {
		return typeInstances
	}

	filtered := make(map[string][]*dbent.PaymentProviderInstance, len(typeInstances))
	for paymentType, groupedInstances := range typeInstances {
		filtered[paymentType] = groupedInstances
	}

	for _, method := range []string{payment.TypeAlipay, payment.TypeWxpay} {
		matching := filterEnabledVisibleMethodInstances(instances, method)
		providerKey, err := s.resolveVisibleMethodProviderKey(ctx, method, matching)
		if err != nil {
			delete(filtered, method)
			continue
		}
		if providerKey == "" {
			if len(matching) == 0 {
				delete(filtered, method)
				continue
			}
			filtered[method] = matching
			continue
		}
		selectedInstances := filterVisibleMethodInstancesByProviderKey(instances, method, providerKey)
		if len(selectedInstances) == 0 {
			delete(filtered, method)
			continue
		}
		filtered[method] = selectedInstances
	}
	return filtered
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
		var matching []*dbent.PaymentProviderInstance
		for _, inst := range instances {
			if payment.InstanceSupportsType(inst.SupportedTypes, pt) {
				matching = append(matching, inst)
			}
		}
		currency, ok := s.pcAggregateMethodCurrency(matching)
		if !ok {
			continue
		}
		ml := pcAggregateMethodLimits(pt, matching)
		ml.Currency = currency
		result = append(result, ml)
	}
	return result, nil
}

func (s *PaymentConfigService) ValidateMethodCurrencyConsistency(ctx context.Context, paymentType string) (string, error) {
	method := NormalizeVisibleMethod(paymentType)
	if method == "" || s == nil || s.entClient == nil {
		return payment.DefaultPaymentCurrency, nil
	}

	instances, err := s.entClient.PaymentProviderInstance.Query().
		Where(paymentproviderinstance.EnabledEQ(true)).All(ctx)
	if err != nil {
		return "", fmt.Errorf("query provider instances: %w", err)
	}

	typeInstances := pcGroupByPaymentType(instances)
	typeInstances = s.pcApplyEnabledVisibleMethodInstances(ctx, typeInstances, instances)
	matching := typeInstances[method]
	if len(matching) == 0 {
		return payment.DefaultPaymentCurrency, nil
	}

	currency, ok := s.pcAggregateMethodCurrency(matching)
	if !ok {
		return "", infraerrors.ServiceUnavailable(
			"PAYMENT_METHOD_CURRENCY_CONFLICT",
			"payment method has enabled provider instances with mixed currencies",
		).WithMetadata(map[string]string{"payment_type": method})
	}
	return currency, nil
}

func (s *PaymentConfigService) pcAggregateMethodCurrency(instances []*dbent.PaymentProviderInstance) (string, bool) {
	currency := ""
	for _, inst := range instances {
		next := s.pcInstancePaymentCurrency(inst)
		if next == "" {
			continue
		}
		if currency == "" {
			currency = next
			continue
		}
		if currency != next {
			return "", false
		}
	}
	if currency == "" {
		return payment.DefaultPaymentCurrency, true
	}
	return currency, true
}

func (s *PaymentConfigService) pcInstancePaymentCurrency(inst *dbent.PaymentProviderInstance) string {
	if inst == nil {
		return payment.DefaultPaymentCurrency
	}
	cfg := map[string]string{}
	if s != nil {
		decrypted, err := s.decryptConfig(inst.Config)
		if err == nil && decrypted != nil {
			cfg = decrypted
		}
	}
	return paymentProviderConfigCurrency(inst.ProviderKey, cfg)
}

// pcGroupByPaymentType groups instances by user-facing payment type.
// For Stripe providers, ALL sub-types (card, link, alipay, wxpay) map to "stripe"
// because the user sees a single "Stripe" button, not individual sub-methods.
// Uses a seen set to avoid counting one instance twice.
func pcGroupByPaymentType(instances []*dbent.PaymentProviderInstance) map[string][]*dbent.PaymentProviderInstance {
	typeInstances := make(map[string][]*dbent.PaymentProviderInstance)
	seen := make(map[string]map[int64]bool)
	add := func(key string, inst *dbent.PaymentProviderInstance) {
		if seen[key] == nil {
			seen[key] = make(map[int64]bool)
		}
		if !seen[key][int64(inst.ID)] {
			seen[key][int64(inst.ID)] = true
			typeInstances[key] = append(typeInstances[key], inst)
		}
	}
	for _, inst := range instances {
		// Stripe provider: all sub-types → single "stripe" group
		if inst.ProviderKey == payment.TypeStripe {
			add(payment.TypeStripe, inst)
			continue
		}
		for _, t := range splitTypes(inst.SupportedTypes) {
			add(t, inst)
		}
	}
	return typeInstances
}

// pcInstanceTypeLimits extracts per-type limits from a provider instance.
// Returns (limits, true) if configured; (zero, false) if unlimited.
// For Stripe instances, limits are stored under "stripe" key regardless of sub-types.
func pcInstanceTypeLimits(inst *dbent.PaymentProviderInstance, pt string) (payment.ChannelLimits, bool) {
	if inst.Limits == "" {
		return payment.ChannelLimits{}, false
	}
	var limits payment.InstanceLimits
	if err := json.Unmarshal([]byte(inst.Limits), &limits); err != nil {
		return payment.ChannelLimits{}, false
	}
	cl, ok := limits[pt]
	return cl, ok
}

// unionFloat merges a single limit value into the aggregate using UNION semantics.
//   - For "min" fields (wantMin=true): keeps the lowest non-zero value
//   - For "max"/"cap" fields (wantMin=false): keeps the highest non-zero value
//   - If any value is 0 (unlimited), the result is unlimited.
//
// Returns (aggregated value, still limited).
func unionFloat(agg float64, limited bool, val float64, wantMin bool) (float64, bool) {
	if val == 0 {
		return agg, false
	}
	if !limited {
		return agg, false
	}
	if agg == 0 {
		return val, true
	}
	if wantMin && val < agg {
		return val, true
	}
	if !wantMin && val > agg {
		return val, true
	}
	return agg, true
}

// pcAggregateMethodLimits computes the UNION (least restrictive) of limits
// across all provider instances for a given payment type.
//
// Since the load balancer can route an order to any available instance,
// the user should see the widest possible range:
//   - SingleMin: lowest floor across instances; 0 if any is unlimited
//   - SingleMax: highest ceiling across instances; 0 if any is unlimited
//   - DailyLimit: highest cap across instances; 0 if any is unlimited
func pcAggregateMethodLimits(pt string, instances []*dbent.PaymentProviderInstance) MethodLimits {
	ml := MethodLimits{PaymentType: pt}
	minLimited, maxLimited, dailyLimited := true, true, true

	for _, inst := range instances {
		cl, hasLimits := pcInstanceTypeLimits(inst, pt)
		if !hasLimits {
			return MethodLimits{PaymentType: pt} // any unlimited instance → all zeros
		}
		ml.SingleMin, minLimited = unionFloat(ml.SingleMin, minLimited, cl.SingleMin, true)
		ml.SingleMax, maxLimited = unionFloat(ml.SingleMax, maxLimited, cl.SingleMax, false)
		ml.DailyLimit, dailyLimited = unionFloat(ml.DailyLimit, dailyLimited, cl.DailyLimit, false)
	}

	if !minLimited {
		ml.SingleMin = 0
	}
	if !maxLimited {
		ml.SingleMax = 0
	}
	if !dailyLimited {
		ml.DailyLimit = 0
	}
	return ml
}

// pcComputeGlobalRange computes the widest [min, max] across all methods.
// Uses the same union logic: lowest min, highest max, 0 if any is unlimited.
func pcComputeGlobalRange(methods map[string]MethodLimits) (globalMin, globalMax float64) {
	minLimited, maxLimited := true, true
	for _, ml := range methods {
		globalMin, minLimited = unionFloat(globalMin, minLimited, ml.SingleMin, true)
		globalMax, maxLimited = unionFloat(globalMax, maxLimited, ml.SingleMax, false)
	}
	if !minLimited {
		globalMin = 0
	}
	if !maxLimited {
		globalMax = 0
	}
	return globalMin, globalMax
}
