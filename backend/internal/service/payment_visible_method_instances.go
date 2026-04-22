package service

import (
	"context"
	"fmt"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentproviderinstance"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func enabledVisibleMethodsForProvider(providerKey, supportedTypes string) []string {
	methodSet := make(map[string]struct{REDACTED, 2)
	addMethod := func(method string) {
		method = NormalizeVisibleMethod(method)
		switch method {
		case payment.TypeAlipay, payment.TypeWxpay:
			methodSet[method] = struct{REDACTED{REDACTED
	REDACTED
REDACTED

	switch strings.TrimSpace(providerKey) {
	case payment.TypeAlipay:
		if strings.TrimSpace(supportedTypes) == "" {
			addMethod(payment.TypeAlipay)
			break
	REDACTED
		for _, supportedType := range splitTypes(supportedTypes) {
			if NormalizeVisibleMethod(supportedType) == payment.TypeAlipay {
				addMethod(payment.TypeAlipay)
				break
		REDACTED
	REDACTED
	case payment.TypeWxpay:
		if strings.TrimSpace(supportedTypes) == "" {
			addMethod(payment.TypeWxpay)
			break
	REDACTED
		for _, supportedType := range splitTypes(supportedTypes) {
			if NormalizeVisibleMethod(supportedType) == payment.TypeWxpay {
				addMethod(payment.TypeWxpay)
				break
		REDACTED
	REDACTED
	case payment.TypeEasyPay:
		for _, supportedType := range splitTypes(supportedTypes) {
			addMethod(supportedType)
	REDACTED
REDACTED

	methods := make([]string, 0, len(methodSet))
	for _, method := range []string{payment.TypeAlipay, payment.TypeWxpayREDACTED {
		if _, ok := methodSet[method]; ok {
			methods = append(methods, method)
	REDACTED
REDACTED
	return methods
REDACTED

func providerSupportsVisibleMethod(inst *dbent.PaymentProviderInstance, method string) bool {
	if inst == nil || !inst.Enabled {
		return false
REDACTED
	method = NormalizeVisibleMethod(method)
	for _, candidate := range enabledVisibleMethodsForProvider(inst.ProviderKey, inst.SupportedTypes) {
		if candidate == method {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func filterEnabledVisibleMethodInstances(instances []*dbent.PaymentProviderInstance, method string) []*dbent.PaymentProviderInstance {
	filtered := make([]*dbent.PaymentProviderInstance, 0, len(instances))
	for _, inst := range instances {
		if providerSupportsVisibleMethod(inst, method) {
			filtered = append(filtered, inst)
	REDACTED
REDACTED
	return filtered
REDACTED

func buildPaymentProviderConflictError(method string, conflicting *dbent.PaymentProviderInstance) error {
	metadata := map[string]string{
		"payment_method": NormalizeVisibleMethod(method),
REDACTED
	if conflicting != nil {
		metadata["conflicting_provider_id"] = fmt.Sprintf("%d", conflicting.ID)
		metadata["conflicting_provider_key"] = conflicting.ProviderKey
		metadata["conflicting_provider_name"] = conflicting.Name
REDACTED
	return infraerrors.Conflict(
		"PAYMENT_PROVIDER_CONFLICT",
		fmt.Sprintf("%s payment already has an enabled provider instance", NormalizeVisibleMethod(method)),
	).WithMetadata(metadata)
REDACTED

func (s *PaymentConfigService) validateVisibleMethodEnablementConflicts(
	ctx context.Context,
	excludeID int64,
	providerKey string,
	supportedTypes string,
	enabled bool,
) error {
	if s == nil || s.entClient == nil || !enabled {
		return nil
REDACTED

	claimedMethods := enabledVisibleMethodsForProvider(providerKey, supportedTypes)
	if len(claimedMethods) == 0 {
		return nil
REDACTED

	query := s.entClient.PaymentProviderInstance.Query().
		Where(paymentproviderinstance.EnabledEQ(true))
	if excludeID > 0 {
		query = query.Where(paymentproviderinstance.IDNEQ(excludeID))
REDACTED
	instances, err := query.All(ctx)
	if err != nil {
		return fmt.Errorf("query enabled payment providers: %w", err)
REDACTED

	for _, method := range claimedMethods {
		for _, inst := range instances {
			if providerSupportsVisibleMethod(inst, method) {
				return buildPaymentProviderConflictError(method, inst)
		REDACTED
	REDACTED
REDACTED
	return nil
REDACTED

func (s *PaymentConfigService) resolveEnabledVisibleMethodInstance(
	ctx context.Context,
	method string,
) (*dbent.PaymentProviderInstance, error) {
	if s == nil || s.entClient == nil {
		return nil, nil
REDACTED

	method = NormalizeVisibleMethod(method)
	if method != payment.TypeAlipay && method != payment.TypeWxpay {
		return nil, nil
REDACTED

	instances, err := s.entClient.PaymentProviderInstance.Query().
		Where(paymentproviderinstance.EnabledEQ(true)).
		Order(paymentproviderinstance.BySortOrder()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query enabled payment providers: %w", err)
REDACTED

	matching := filterEnabledVisibleMethodInstances(instances, method)
	switch len(matching) {
	case 0:
		return nil, nil
	case 1:
		return matching[0], nil
	default:
		return nil, buildPaymentProviderConflictError(method, matching[0])
REDACTED
REDACTED
