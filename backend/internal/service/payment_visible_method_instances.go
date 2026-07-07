package service

import (
	"context"
	"errors"
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
		if method != "" {
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
			delete(methodSet, method)
	REDACTED
REDACTED
	for _, supportedType := range splitTypes(supportedTypes) {
		method := NormalizeVisibleMethod(supportedType)
		if _, ok := methodSet[method]; ok {
			methods = append(methods, method)
			delete(methodSet, method)
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

func filterVisibleMethodInstancesByProviderKey(instances []*dbent.PaymentProviderInstance, method string, providerKey string) []*dbent.PaymentProviderInstance {
	filtered := make([]*dbent.PaymentProviderInstance, 0, len(instances))
	for _, inst := range instances {
		if !providerSupportsVisibleMethod(inst, method) {
			continue
	REDACTED
		if !strings.EqualFold(strings.TrimSpace(inst.ProviderKey), strings.TrimSpace(providerKey)) {
			continue
	REDACTED
		filtered = append(filtered, inst)
REDACTED
	return filtered
REDACTED

func distinctVisibleMethodProviderKeys(instances []*dbent.PaymentProviderInstance) []string {
	seen := make(map[string]struct{REDACTED, len(instances))
	keys := make([]string, 0, len(instances))
	for _, inst := range instances {
		if inst == nil {
			continue
	REDACTED
		key := strings.TrimSpace(inst.ProviderKey)
		if key == "" {
			continue
	REDACTED
		normalized := strings.ToLower(key)
		if _, ok := seen[normalized]; ok {
			continue
	REDACTED
		seen[normalized] = struct{REDACTED{REDACTED
		keys = append(keys, key)
REDACTED
	return keys
REDACTED

func selectVisibleMethodInstanceByProviderKey(instances []*dbent.PaymentProviderInstance, providerKey string) *dbent.PaymentProviderInstance {
	providerKey = strings.TrimSpace(providerKey)
	if providerKey == "" {
		return nil
REDACTED
	for _, inst := range instances {
		if strings.EqualFold(strings.TrimSpace(inst.ProviderKey), providerKey) {
			return inst
	REDACTED
REDACTED
	return nil
REDACTED

func (s *PaymentConfigService) validateVisibleMethodEnablementConflicts(
	ctx context.Context,
	excludeID int64,
	providerKey string,
	supportedTypes string,
	enabled bool,
) error {
	// Visible methods are selected by configured source (official/easypay),
	// so multiple enabled providers can intentionally claim the same user-facing
	// method. Order creation and limits will route through the configured source.
	_, _, _, _, _ = ctx, excludeID, providerKey, supportedTypes, enabled
	return nil
REDACTED

func (s *PaymentConfigService) resolveVisibleMethodSourceProviderKey(ctx context.Context, method string) (string, error) {
	method = NormalizeVisibleMethod(method)
	sourceKey := visibleMethodSourceSettingKey(method)
	rawSource := ""
	if s != nil && s.settingRepo != nil && sourceKey != "" {
		value, err := s.settingRepo.GetValue(ctx, sourceKey)
		if err != nil {
			if !errors.Is(err, ErrSettingNotFound) {
				return "", fmt.Errorf("get %s: %w", sourceKey, err)
		REDACTED
	REDACTED else {
			rawSource = value
	REDACTED
REDACTED

	normalizedSource, err := normalizeVisibleMethodSettingSource(method, rawSource, true)
	if err != nil {
		return "", err
REDACTED
	if normalizedSource == "" {
		return "", nil
REDACTED
	providerKey, ok := VisibleMethodProviderKeyForSource(method, normalizedSource)
	if !ok {
		return "", infraerrors.BadRequest(
			"INVALID_PAYMENT_VISIBLE_METHOD_SOURCE",
			fmt.Sprintf("%s source must be one of the supported payment providers", method),
		)
REDACTED
	return providerKey, nil
REDACTED

func (s *PaymentConfigService) resolveVisibleMethodProviderKey(
	ctx context.Context,
	method string,
	matching []*dbent.PaymentProviderInstance,
) (string, error) {
	switch providerKeys := distinctVisibleMethodProviderKeys(matching); len(providerKeys) {
	case 0:
		return "", nil
	case 1:
		return strings.TrimSpace(providerKeys[0]), nil
	default:
		providerKey, err := s.resolveVisibleMethodSourceProviderKey(ctx, method)
		if err != nil {
			return "", err
	REDACTED
		if providerKey == "" {
			return "", nil
	REDACTED
		selected := selectVisibleMethodInstanceByProviderKey(matching, providerKey)
		if selected == nil {
			return "", infraerrors.BadRequest(
				"INVALID_PAYMENT_VISIBLE_METHOD_SOURCE",
				fmt.Sprintf("%s source has no enabled provider instance", method),
			)
	REDACTED
		return strings.TrimSpace(selected.ProviderKey), nil
REDACTED
REDACTED

func (s *PaymentConfigService) resolveEnabledVisibleMethodInstance(
	ctx context.Context,
	method string,
) (*dbent.PaymentProviderInstance, error) {
	if s == nil || s.entClient == nil {
		return nil, nil
REDACTED

	method = NormalizeVisibleMethod(method)
	if method == "" {
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
	providerKey, err := s.resolveVisibleMethodProviderKey(ctx, method, matching)
	if err != nil {
		return nil, err
REDACTED
	if providerKey == "" {
		if len(matching) == 0 {
			return nil, nil
	REDACTED
		return &dbent.PaymentProviderInstance{ProviderKey: ""REDACTED, nil
REDACTED
	return selectVisibleMethodInstanceByProviderKey(matching, providerKey), nil
REDACTED
