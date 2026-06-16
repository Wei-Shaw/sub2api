package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	pluginent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
	"github.com/Wei-Shaw/sub2api/plugins/payment/ent/paymentproviderinstance"
	infraerrors "github.com/Wei-Shaw/sub2api/plugins/payment/internal/errors"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
)

// enabledVisibleMethodsForProvider returns the ordered list of canonical
// visible methods (alipay/wxpay) the supplied provider supports.
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

// providerSupportsVisibleMethod returns true when the supplied (enabled)
// instance can serve the requested visible method.
func providerSupportsVisibleMethod(inst *pluginent.PaymentProviderInstance, method string) bool {
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

// filterEnabledVisibleMethodInstances returns the subset of instances
// that can serve the supplied visible method.
func filterEnabledVisibleMethodInstances(instances []*pluginent.PaymentProviderInstance, method string) []*pluginent.PaymentProviderInstance {
	filtered := make([]*pluginent.PaymentProviderInstance, 0, len(instances))
	for _, inst := range instances {
		if providerSupportsVisibleMethod(inst, method) {
			filtered = append(filtered, inst)
		}
	}
	return filtered
}

// filterVisibleMethodInstancesByProviderKey narrows the candidate list to
// those instances whose provider key matches the configured source.
func filterVisibleMethodInstancesByProviderKey(instances []*pluginent.PaymentProviderInstance, method string, providerKey string) []*pluginent.PaymentProviderInstance {
	filtered := make([]*pluginent.PaymentProviderInstance, 0, len(instances))
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

// distinctVisibleMethodProviderKeys returns the unique (case-insensitive)
// provider keys present in the candidate list, preserving first-seen order.
func distinctVisibleMethodProviderKeys(instances []*pluginent.PaymentProviderInstance) []string {
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

// selectVisibleMethodInstanceByProviderKey returns the first instance
// whose ProviderKey matches the supplied key, or nil when no match.
func selectVisibleMethodInstanceByProviderKey(instances []*pluginent.PaymentProviderInstance, providerKey string) *pluginent.PaymentProviderInstance {
	providerKey = strings.TrimSpace(providerKey)
	if providerKey == "" {
		return nil
	}
	for _, inst := range instances {
		if inst == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(inst.ProviderKey), providerKey) {
			return inst
		}
	}
	return nil
}

// validateVisibleMethodEnablementConflicts is a no-op placeholder retained
// for parity with the host implementation. Visible methods are now
// resolved by the configured source, so multiple enabled providers can
// intentionally claim the same user-facing method.
func (s *PaymentConfigService) validateVisibleMethodEnablementConflicts(
	ctx context.Context,
	excludeID int64,
	providerKey string,
	supportedTypes string,
	enabled bool,
) error {
	_, _, _, _, _ = ctx, excludeID, providerKey, supportedTypes, enabled
	return nil
}

// resolveVisibleMethodSourceProviderKey resolves the operator-configured
// source (official_alipay / easypay_alipay / official_wechat /
// easypay_wechat) to the provider key the load-balancer should use. An
// empty / missing source returns ("", nil).
func (s *PaymentConfigService) resolveVisibleMethodSourceProviderKey(ctx context.Context, method string) (string, error) {
	method = NormalizeVisibleMethod(method)
	sourceKey := visibleMethodSourceSettingKey(method)
	rawSource := ""
	if s != nil && s.settings != nil && sourceKey != "" {
		var value string
		err := s.settings.GetTyped(ctx, sourceKey, &value)
		switch {
		case err == nil:
			rawSource = value
		case errors.Is(err, pluginsdk.ErrSettingNotFound):
			// missing setting -> treat as no preference
		default:
			return "", fmt.Errorf("get %s: %w", sourceKey, err)
		}
	}

	if strings.TrimSpace(rawSource) == "" {
		return "", nil
	}
	canonicalSource := NormalizeVisibleMethodSource(method, rawSource)
	if canonicalSource == "" {
		return "", infraerrors.BadRequest(
			"INVALID_PAYMENT_VISIBLE_METHOD_SOURCE",
			fmt.Sprintf("%s source must be one of the supported payment providers", method),
		)
	}
	providerKey, ok := VisibleMethodProviderKeyForSource(method, canonicalSource)
	if !ok {
		return "", infraerrors.BadRequest(
			"INVALID_PAYMENT_VISIBLE_METHOD_SOURCE",
			fmt.Sprintf("%s source must be one of the supported payment providers", method),
		)
	}
	return providerKey, nil
}

// resolveVisibleMethodProviderKey returns the provider key that should be
// used for the given visible method, given the candidate instance list.
func (s *PaymentConfigService) resolveVisibleMethodProviderKey(
	ctx context.Context,
	method string,
	matching []*pluginent.PaymentProviderInstance,
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

// resolveEnabledVisibleMethodInstance returns the active provider instance
// for a visible method (alipay/wxpay), respecting the operator-configured
// source. Returns (nil, nil) when no enabled instance is available.
func (s *PaymentConfigService) resolveEnabledVisibleMethodInstance(
	ctx context.Context,
	method string,
) (*pluginent.PaymentProviderInstance, error) {
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
		return &pluginent.PaymentProviderInstance{ProviderKey: ""}, nil
	}
	return selectVisibleMethodInstanceByProviderKey(matching, providerKey), nil
}

// visibleMethodEnabledSettingKey returns the settings key for the
// enabled-flag of the supplied method, or "" when the method has no
// matching key.
func visibleMethodEnabledSettingKey(method string) string {
	switch method {
	case "alipay":
		return SettingPaymentVisibleMethodAlipayEnabled
	case "wxpay":
		return SettingPaymentVisibleMethodWxpayEnabled
	}
	return ""
}

// visibleMethodSourceSettingKey is the parallel for source.
func visibleMethodSourceSettingKey(method string) string {
	switch method {
	case "alipay":
		return SettingPaymentVisibleMethodAlipaySource
	case "wxpay":
		return SettingPaymentVisibleMethodWxpaySource
	}
	return ""
}
