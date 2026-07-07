package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
REDACTED
	typeInstances := pcGroupByPaymentType(instances)
	typeInstances = s.pcApplyEnabledVisibleMethodInstances(ctx, typeInstances, instances)
	resp := &MethodLimitsResponse{
		Methods: make(map[string]MethodLimits, len(typeInstances)),
REDACTED
	for pt, insts := range typeInstances {
		currency, ok := s.pcAggregateMethodCurrency(insts)
		if !ok {
			continue
	REDACTED
		ml := pcAggregateMethodLimits(pt, insts)
		ml.DisplayName = s.pcAggregateMethodDisplayName(pt, insts)
		ml.Currency = currency
		resp.Methods[ml.PaymentType] = ml
REDACTED
	resp.GlobalMin, resp.GlobalMax = pcComputeGlobalRange(resp.Methods)
	return resp, nil
REDACTED

func (s *PaymentConfigService) pcApplyEnabledVisibleMethodInstances(ctx context.Context, typeInstances map[string][]*dbent.PaymentProviderInstance, instances []*dbent.PaymentProviderInstance) map[string][]*dbent.PaymentProviderInstance {
	if len(typeInstances) == 0 {
		return typeInstances
REDACTED

	filtered := make(map[string][]*dbent.PaymentProviderInstance, len(typeInstances))
	for paymentType, groupedInstances := range typeInstances {
		filtered[paymentType] = groupedInstances
REDACTED

	for _, method := range []string{payment.TypeAlipay, payment.TypeWxpayREDACTED {
		matching := filterEnabledVisibleMethodInstances(instances, method)
		providerKey, err := s.resolveVisibleMethodProviderKey(ctx, method, matching)
		if err != nil {
			delete(filtered, method)
			continue
	REDACTED
		if providerKey == "" {
			if len(matching) == 0 {
				delete(filtered, method)
				continue
		REDACTED
			filtered[method] = matching
			continue
	REDACTED
		selectedInstances := filterVisibleMethodInstancesByProviderKey(instances, method, providerKey)
		if len(selectedInstances) == 0 {
			delete(filtered, method)
			continue
	REDACTED
		filtered[method] = selectedInstances
REDACTED
	return filtered
REDACTED

// GetMethodLimits returns per-payment-type limits from enabled provider instances.
func (s *PaymentConfigService) GetMethodLimits(ctx context.Context, types []string) ([]MethodLimits, error) {
	instances, err := s.entClient.PaymentProviderInstance.Query().
		Where(paymentproviderinstance.EnabledEQ(true)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query provider instances: %w", err)
REDACTED
	result := make([]MethodLimits, 0, len(types))
	for _, pt := range types {
		var matching []*dbent.PaymentProviderInstance
		for _, inst := range instances {
			if payment.InstanceSupportsType(inst.SupportedTypes, pt) {
				matching = append(matching, inst)
		REDACTED
	REDACTED
		currency, ok := s.pcAggregateMethodCurrency(matching)
		if !ok {
			continue
	REDACTED
		ml := pcAggregateMethodLimits(pt, matching)
		ml.DisplayName = s.pcAggregateMethodDisplayName(pt, matching)
		ml.Currency = currency
		result = append(result, ml)
REDACTED
	return result, nil
REDACTED

func (s *PaymentConfigService) ValidateMethodCurrencyConsistency(ctx context.Context, paymentType string) (string, error) {
	method := NormalizeVisibleMethod(paymentType)
	if method == "" || s == nil || s.entClient == nil {
		return payment.DefaultPaymentCurrency, nil
REDACTED

	instances, err := s.entClient.PaymentProviderInstance.Query().
		Where(paymentproviderinstance.EnabledEQ(true)).All(ctx)
	if err != nil {
		return "", fmt.Errorf("query provider instances: %w", err)
REDACTED

	typeInstances := pcGroupByPaymentType(instances)
	typeInstances = s.pcApplyEnabledVisibleMethodInstances(ctx, typeInstances, instances)
	matching := typeInstances[method]
	if len(matching) == 0 {
		return payment.DefaultPaymentCurrency, nil
REDACTED

	currency, ok := s.pcAggregateMethodCurrency(matching)
	if !ok {
		return "", infraerrors.ServiceUnavailable(
			"PAYMENT_METHOD_CURRENCY_CONFLICT",
			"payment method has enabled provider instances with mixed currencies",
		).WithMetadata(map[string]string{"payment_type": methodREDACTED)
REDACTED
	return currency, nil
REDACTED

func (s *PaymentConfigService) pcAggregateMethodCurrency(instances []*dbent.PaymentProviderInstance) (string, bool) {
	currency := ""
	for _, inst := range instances {
		next := s.pcInstancePaymentCurrency(inst)
		if next == "" {
			continue
	REDACTED
		if currency == "" {
			currency = next
			continue
	REDACTED
		if currency != next {
			return "", false
	REDACTED
REDACTED
	if currency == "" {
		return payment.DefaultPaymentCurrency, true
REDACTED
	return currency, true
REDACTED

func (s *PaymentConfigService) pcInstancePaymentCurrency(inst *dbent.PaymentProviderInstance) string {
	if inst == nil {
		return payment.DefaultPaymentCurrency
REDACTED
	cfg := map[string]string{REDACTED
	if s != nil {
		decrypted, err := s.decryptConfig(inst.Config)
		if err == nil && decrypted != nil {
			cfg = decrypted
	REDACTED
REDACTED
	return paymentProviderConfigCurrency(inst.ProviderKey, cfg)
REDACTED

type easyPayCustomMethodDisplayConfig struct {
	Type        string `json:"type"`
	DisplayName string `json:"displayName"`
REDACTED

func (s *PaymentConfigService) pcAggregateMethodDisplayName(pt string, instances []*dbent.PaymentProviderInstance) string {
	pt = strings.TrimSpace(pt)
	if pt == "" {
		return ""
REDACTED
	for _, inst := range instances {
		displayName := s.pcInstanceEasyPayCustomMethodDisplayName(inst, pt)
		if displayName != "" {
			return displayName
	REDACTED
REDACTED
	return ""
REDACTED

func (s *PaymentConfigService) pcInstanceEasyPayCustomMethodDisplayName(inst *dbent.PaymentProviderInstance, pt string) string {
	if inst == nil || inst.ProviderKey != payment.TypeEasyPay {
		return ""
REDACTED
	cfg := map[string]string{REDACTED
	if s != nil {
		decrypted, err := s.decryptConfig(inst.Config)
		if err == nil && decrypted != nil {
			cfg = decrypted
	REDACTED
REDACTED
	raw := strings.TrimSpace(cfg["customMethods"])
	if raw == "" {
		return ""
REDACTED

	var methods []easyPayCustomMethodDisplayConfig
	if err := json.Unmarshal([]byte(raw), &methods); err != nil {
		return ""
REDACTED
	for _, method := range methods {
		if strings.TrimSpace(method.Type) == pt {
			return strings.TrimSpace(method.DisplayName)
	REDACTED
REDACTED
	return ""
REDACTED

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
	REDACTED
		if !seen[key][int64(inst.ID)] {
			seen[key][int64(inst.ID)] = true
			typeInstances[key] = append(typeInstances[key], inst)
	REDACTED
REDACTED
	for _, inst := range instances {
		// Stripe provider: all sub-types → single "stripe" group
		if inst.ProviderKey == payment.TypeStripe {
			add(payment.TypeStripe, inst)
			continue
	REDACTED
		for _, t := range splitTypes(inst.SupportedTypes) {
			add(t, inst)
	REDACTED
REDACTED
	return typeInstances
REDACTED

// pcInstanceTypeLimits extracts per-type limits from a provider instance.
// Returns (limits, true) if configured; (zero, false) if unlimited.
// For Stripe instances, limits are stored under "stripe" key regardless of sub-types.
func pcInstanceTypeLimits(inst *dbent.PaymentProviderInstance, pt string) (payment.ChannelLimits, bool) {
	if inst.Limits == "" {
		return payment.ChannelLimits{REDACTED, false
REDACTED
	var limits payment.InstanceLimits
	if err := json.Unmarshal([]byte(inst.Limits), &limits); err != nil {
		return payment.ChannelLimits{REDACTED, false
REDACTED
	cl, ok := limits[pt]
	return cl, ok
REDACTED

// unionFloat merges a single limit value into the aggregate using UNION semantics.
//   - For "min" fields (wantMin=true): keeps the lowest non-zero value
//   - For "max"/"cap" fields (wantMin=false): keeps the highest non-zero value
//   - If any value is 0 (unlimited), the result is unlimited.
//
// Returns (aggregated value, still limited).
func unionFloat(agg float64, limited bool, val float64, wantMin bool) (float64, bool) {
	if val == 0 {
		return agg, false
REDACTED
	if !limited {
		return agg, false
REDACTED
	if agg == 0 {
		return val, true
REDACTED
	if wantMin && val < agg {
		return val, true
REDACTED
	if !wantMin && val > agg {
		return val, true
REDACTED
	return agg, true
REDACTED

// pcAggregateMethodLimits computes the UNION (least restrictive) of limits
// across all provider instances for a given payment type.
//
// Since the load balancer can route an order to any available instance,
// the user should see the widest possible range:
//   - SingleMin: lowest floor across instances; 0 if any is unlimited
//   - SingleMax: highest ceiling across instances; 0 if any is unlimited
//   - DailyLimit: highest cap across instances; 0 if any is unlimited
func pcAggregateMethodLimits(pt string, instances []*dbent.PaymentProviderInstance) MethodLimits {
	ml := MethodLimits{PaymentType: ptREDACTED
	minLimited, maxLimited, dailyLimited := true, true, true

	for _, inst := range instances {
		cl, hasLimits := pcInstanceTypeLimits(inst, pt)
		if !hasLimits {
			return MethodLimits{PaymentType: ptREDACTED // any unlimited instance → all zeros
	REDACTED
		ml.SingleMin, minLimited = unionFloat(ml.SingleMin, minLimited, cl.SingleMin, true)
		ml.SingleMax, maxLimited = unionFloat(ml.SingleMax, maxLimited, cl.SingleMax, false)
		ml.DailyLimit, dailyLimited = unionFloat(ml.DailyLimit, dailyLimited, cl.DailyLimit, false)
REDACTED

	if !minLimited {
		ml.SingleMin = 0
REDACTED
	if !maxLimited {
		ml.SingleMax = 0
REDACTED
	if !dailyLimited {
		ml.DailyLimit = 0
REDACTED
	return ml
REDACTED

// pcComputeGlobalRange computes the widest [min, max] across all methods.
// Uses the same union logic: lowest min, highest max, 0 if any is unlimited.
func pcComputeGlobalRange(methods map[string]MethodLimits) (globalMin, globalMax float64) {
	minLimited, maxLimited := true, true
	for _, ml := range methods {
		globalMin, minLimited = unionFloat(globalMin, minLimited, ml.SingleMin, true)
		globalMax, maxLimited = unionFloat(globalMax, maxLimited, ml.SingleMax, false)
REDACTED
	if !minLimited {
		globalMin = 0
REDACTED
	if !maxLimited {
		globalMax = 0
REDACTED
	return globalMin, globalMax
REDACTED
