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
	methodSet := make(map[string]struct{}, 2)
	addMethod := func(method string) {
		method = NormalizeVisibleMethod(method)
		switch method {
		case payment.TypeAlipay, payment.TypeWxpay:
			methodSet[method] = struct{}{}
		}
	}

	switch strings.TrimSpace(providerKey) {
	case payment.TypeAlipay:
		if strings.TrimSpace(supportedTypes) == "" {
			addMethod(payment.TypeAlipay)
			break
		}
		for _, supportedType := range splitTypes(supportedTypes) {
			if NormalizeVisibleMethod(supportedType) == payment.TypeAlipay {
				addMethod(payment.TypeAlipay)
				break
			}
		}
	case payment.TypeWxpay:
		if strings.TrimSpace(supportedTypes) == "" {
			addMethod(payment.TypeWxpay)
			break
		}
		for _, supportedType := range splitTypes(supportedTypes) {
			if NormalizeVisibleMethod(supportedType) == payment.TypeWxpay {
				addMethod(payment.TypeWxpay)
				break
			}
		}
	case payment.TypeEasyPay:
		for _, supportedType := range splitTypes(supportedTypes) {
			addMethod(supportedType)
		}
	}

	methods := make([]string, 0, len(methodSet))
	for _, method := range []string{payment.TypeAlipay, payment.TypeWxpay} {
		if _, ok := methodSet[method]; ok {
			methods = append(methods, method)
		}
	}
	return methods
}

func providerSupportsVisibleMethod(inst *dbent.PaymentProviderInstance, method string) bool {
	if inst == nil || !inst.Enabled {
		return false
	}
	method = NormalizeVisibleMethod(method)
	for _, candidate := range enabledVisibleMethodsForProvider(inst.ProviderKey, inst.SupportedTypes) {
		if candidate == method {
			return true
		}
	}
	return false
}

func filterEnabledVisibleMethodInstances(instances []*dbent.PaymentProviderInstance, method string) []*dbent.PaymentProviderInstance {
	filtered := make([]*dbent.PaymentProviderInstance, 0, len(instances))
	for _, inst := range instances {
		if providerSupportsVisibleMethod(inst, method) {
			filtered = append(filtered, inst)
		}
	}
	return filtered
}

func filterVisibleMethodInstancesByProviderKey(instances []*dbent.PaymentProviderInstance, method string, providerKey string) []*dbent.PaymentProviderInstance {
	filtered := make([]*dbent.PaymentProviderInstance, 0, len(instances))
	for _, inst := range instances {
		if !providerSupportsVisibleMethod(inst, method) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(inst.ProviderKey), strings.TrimSpace(providerKey)) {
			continue
		}
		filtered = append(filtered, inst)
	}
	return filtered
}

func distinctVisibleMethodProviderKeys(instances []*dbent.PaymentProviderInstance) []string {
	seen := make(map[string]struct{}, len(instances))
	keys := make([]string, 0, len(instances))
	for _, inst := range instances {
		if inst == nil {
			continue
		}
		key := strings.TrimSpace(inst.ProviderKey)
		if key == "" {
			continue
		}
		normalized := strings.ToLower(key)
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		keys = append(keys, key)
	}
	return keys
}

func selectVisibleMethodInstanceByProviderKey(instances []*dbent.PaymentProviderInstance, providerKey string) *dbent.PaymentProviderInstance {
	providerKey = strings.TrimSpace(providerKey)
	if providerKey == "" {
		return nil
	}
	for _, inst := range instances {
		if strings.EqualFold(strings.TrimSpace(inst.ProviderKey), providerKey) {
			return inst
		}
	}
	return nil
}

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
}

func (s *PaymentConfigService) resolveVisibleMethodSourceProviderKey(ctx context.Context, method string) (string, error) {
	method = NormalizeVisibleMethod(method)
	sourceKey := visibleMethodSourceSettingKey(method)
	rawSource := ""
	if s != nil && s.settingRepo != nil && sourceKey != "" {
		value, err := s.settingRepo.GetValue(ctx, sourceKey)
		if err != nil {
			if !errors.Is(err, ErrSettingNotFound) {
				return "", fmt.Errorf("get %s: %w", sourceKey, err)
			}
		} else {
			rawSource = value
		}
	}

	normalizedSource, err := normalizeVisibleMethodSettingSource(method, rawSource, true)
	if err != nil {
		return "", err
	}
	if normalizedSource == "" {
		return "", nil
	}
	providerKey, ok := VisibleMethodProviderKeyForSource(method, normalizedSource)
	if !ok {
		return "", infraerrors.BadRequest(
			"INVALID_PAYMENT_VISIBLE_METHOD_SOURCE",
			fmt.Sprintf("%s source must be one of the supported payment providers", method),
		)
	}
	return providerKey, nil
}

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
		}
		if providerKey == "" {
			return "", nil
		}
		selected := selectVisibleMethodInstanceByProviderKey(matching, providerKey)
		if selected == nil {
			return "", infraerrors.BadRequest(
				"INVALID_PAYMENT_VISIBLE_METHOD_SOURCE",
				fmt.Sprintf("%s source has no enabled provider instance", method),
			)
		}
		return strings.TrimSpace(selected.ProviderKey), nil
	}
}

func (s *PaymentConfigService) resolveEnabledVisibleMethodInstance(
	ctx context.Context,
	method string,
) (*dbent.PaymentProviderInstance, error) {
	if s == nil || s.entClient == nil {
		return nil, nil
	}

	method = NormalizeVisibleMethod(method)
	if method != payment.TypeAlipay && method != payment.TypeWxpay {
		return nil, nil
	}

	instances, err := s.entClient.PaymentProviderInstance.Query().
		Where(paymentproviderinstance.EnabledEQ(true)).
		Order(paymentproviderinstance.BySortOrder()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query enabled payment providers: %w", err)
	}

	matching := filterEnabledVisibleMethodInstances(instances, method)
	providerKey, err := s.resolveVisibleMethodProviderKey(ctx, method, matching)
	if err != nil {
		return nil, err
	}
	if providerKey == "" {
		if len(matching) == 0 {
			return nil, nil
		}
		return &dbent.PaymentProviderInstance{ProviderKey: ""}, nil
	}
	return selectVisibleMethodInstanceByProviderKey(matching, providerKey), nil
}
