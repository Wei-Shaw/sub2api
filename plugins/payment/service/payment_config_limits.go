package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/shopspring/decimal"

	pluginent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
	"github.com/Wei-Shaw/sub2api/plugins/payment/ent/paymentproviderinstance"
	infraerrors "github.com/Wei-Shaw/sub2api/plugins/payment/internal/errors"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
)

// GetAvailableMethodLimits collects all payment types from enabled provider
// instances and returns limits for each, plus the global widest range.
// Stripe sub-types (card, link) are aggregated under "stripe".
func (s *PaymentConfigService) GetAvailableMethodLimits(ctx context.Context) (*MethodLimitsResponse, error) {
	if s == nil || s.entClient == nil {
		return &MethodLimitsResponse{Methods: map[string]MethodLimits{}}, nil
	}
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
		currency, ok := s.pcAggregateMethodCurrency(ctx, insts)
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

// pcApplyEnabledVisibleMethodInstances filters typeInstances for the
// visible-method buckets (alipay/wxpay) according to the operator's
// configured "source" (official/easypay). When the source resolves to
// a single provider key, only instances belonging to that provider are
// kept; when there is no preference and no candidates, the bucket is
// removed so the user does not see an empty method.
func (s *PaymentConfigService) pcApplyEnabledVisibleMethodInstances(ctx context.Context, typeInstances map[string][]*pluginent.PaymentProviderInstance, instances []*pluginent.PaymentProviderInstance) map[string][]*pluginent.PaymentProviderInstance {
	if len(typeInstances) == 0 {
		return typeInstances
	}

	filtered := make(map[string][]*pluginent.PaymentProviderInstance, len(typeInstances))
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
	if s == nil || s.entClient == nil {
		return nil, nil
	}
	instances, err := s.entClient.PaymentProviderInstance.Query().
		Where(paymentproviderinstance.EnabledEQ(true)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query provider instances: %w", err)
	}
	result := make([]MethodLimits, 0, len(types))
	for _, pt := range types {
		var matching []*pluginent.PaymentProviderInstance
		for _, inst := range instances {
			if payment.InstanceSupportsType(inst.SupportedTypes, pt) {
				matching = append(matching, inst)
			}
		}
		currency, ok := s.pcAggregateMethodCurrency(ctx, matching)
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

	currency, ok := s.pcAggregateMethodCurrency(ctx, matching)
	if !ok {
		return "", infraerrors.ServiceUnavailable(
			"PAYMENT_METHOD_CURRENCY_CONFLICT",
			"payment method has enabled provider instances with mixed currencies",
		).WithMetadata(map[string]string{"payment_type": method})
	}
	return currency, nil
}

func (s *PaymentConfigService) pcAggregateMethodCurrency(ctx context.Context, instances []*pluginent.PaymentProviderInstance) (string, bool) {
	currency := ""
	for _, inst := range instances {
		next := s.pcInstancePaymentCurrency(ctx, inst)
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

func (s *PaymentConfigService) pcInstancePaymentCurrency(ctx context.Context, inst *pluginent.PaymentProviderInstance) string {
	if inst == nil {
		return payment.DefaultPaymentCurrency
	}
	cfg := map[string]string{}
	if s != nil {
		decrypted, err := s.decryptConfig(ctx, inst.Config)
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
func pcGroupByPaymentType(instances []*pluginent.PaymentProviderInstance) map[string][]*pluginent.PaymentProviderInstance {
	typeInstances := make(map[string][]*pluginent.PaymentProviderInstance)
	seen := make(map[string]map[int]bool)
	add := func(key string, inst *pluginent.PaymentProviderInstance) {
		if seen[key] == nil {
			seen[key] = make(map[int]bool)
		}
		if !seen[key][inst.ID] {
			seen[key][inst.ID] = true
			typeInstances[key] = append(typeInstances[key], inst)
		}
	}
	for _, inst := range instances {
		// Stripe provider: all sub-types -> single "stripe" group
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
func pcInstanceTypeLimits(inst *pluginent.PaymentProviderInstance, pt string) (payment.ChannelLimits, bool) {
	if inst == nil || inst.Limits == "" {
		return payment.ChannelLimits{}, false
	}
	var limits payment.InstanceLimits
	if err := json.Unmarshal([]byte(inst.Limits), &limits); err != nil {
		return payment.ChannelLimits{}, false
	}
	cl, ok := limits[pt]
	return cl, ok
}

// unionDecimal merges a single limit value into the aggregate using UNION semantics.
//   - For "min" fields (wantMin=true): keeps the lowest non-zero value
//   - For "max"/"cap" fields (wantMin=false): keeps the highest non-zero value
//   - If any value is 0 (unlimited), the result is unlimited.
//
// Returns (aggregated value, still limited).
func unionDecimal(agg decimal.Decimal, limited bool, val decimal.Decimal, wantMin bool) (decimal.Decimal, bool) {
	if val.IsZero() {
		return agg, false
	}
	if !limited {
		return agg, false
	}
	if agg.IsZero() {
		return val, true
	}
	if wantMin && val.LessThan(agg) {
		return val, true
	}
	if !wantMin && val.GreaterThan(agg) {
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
func pcAggregateMethodLimits(pt string, instances []*pluginent.PaymentProviderInstance) MethodLimits {
	ml := MethodLimits{PaymentType: pt}
	minLimited, maxLimited, dailyLimited := true, true, true

	for _, inst := range instances {
		cl, hasLimits := pcInstanceTypeLimits(inst, pt)
		if !hasLimits {
			return MethodLimits{PaymentType: pt} // any unlimited instance -> all zeros
		}
		ml.SingleMin, minLimited = unionDecimal(ml.SingleMin, minLimited, cl.SingleMin, true)
		ml.SingleMax, maxLimited = unionDecimal(ml.SingleMax, maxLimited, cl.SingleMax, false)
		ml.DailyLimit, dailyLimited = unionDecimal(ml.DailyLimit, dailyLimited, cl.DailyLimit, false)
	}

	if !minLimited {
		ml.SingleMin = decimal.Zero
	}
	if !maxLimited {
		ml.SingleMax = decimal.Zero
	}
	if !dailyLimited {
		ml.DailyLimit = decimal.Zero
	}
	return ml
}

// pcComputeGlobalRange computes the widest [min, max] across all methods.
// Uses the same union logic: lowest min, highest max, 0 if any is unlimited.
func pcComputeGlobalRange(methods map[string]MethodLimits) (globalMin, globalMax decimal.Decimal) {
	minLimited, maxLimited := true, true
	for _, ml := range methods {
		globalMin, minLimited = unionDecimal(globalMin, minLimited, ml.SingleMin, true)
		globalMax, maxLimited = unionDecimal(globalMax, maxLimited, ml.SingleMax, false)
	}
	if !minLimited {
		globalMin = decimal.Zero
	}
	if !maxLimited {
		globalMax = decimal.Zero
	}
	return globalMin, globalMax
}
