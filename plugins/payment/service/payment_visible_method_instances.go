package service

import (
	"context"
	"errors"

	pluginent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
)

// errVisibleMethodNotImplemented is returned by every visible-method
// enable/source resolver that has not been ported.
//
// TODO(payment-migration): port the visible-method resolution from
// backend/internal/service/payment_visible_method_instances.go.
var errVisibleMethodNotImplemented = errors.New("payment: visible method resolver not yet wired")

// validateVisibleMethodEnablementConflicts enforces that operators do
// not enable two competing sources for the same visible method.
func (s *PaymentConfigService) validateVisibleMethodEnablementConflicts(
	_ context.Context, _ int64, _ string, _ string, _ bool,
) error {
	return nil
}

// resolveVisibleMethodSourceProviderKey returns the provider-key the
// load balancer should route to for the supplied visible method, based
// on the configured source.
func (s *PaymentConfigService) resolveVisibleMethodSourceProviderKey(_ context.Context, _ string) (string, error) {
	return "", errVisibleMethodNotImplemented
}

// resolveVisibleMethodProviderKey is the variant that consumes a slice of
// candidate instances rather than reading from settings.
func (s *PaymentConfigService) resolveVisibleMethodProviderKey(
	_ context.Context, _ string, _ []*pluginent.PaymentProviderInstance,
) (string, error) {
	return "", errVisibleMethodNotImplemented
}

// resolveEnabledVisibleMethodInstance picks the active provider instance
// for a visible method.
func (s *PaymentConfigService) resolveEnabledVisibleMethodInstance(
	_ context.Context, _ string,
) (*pluginent.PaymentProviderInstance, error) {
	return nil, errVisibleMethodNotImplemented
}

// enabledVisibleMethodsForProvider returns the ordered list of canonical
// visible methods (alipay/wxpay/...) the supplied provider supports.
//
// TODO(payment-migration): port the original mapping. Stubbed to return
// the unprocessed supportedTypes.
func enabledVisibleMethodsForProvider(_ string, _ string) []string {
	return nil
}

func providerSupportsVisibleMethod(_ *pluginent.PaymentProviderInstance, _ string) bool {
	return false
}

func filterEnabledVisibleMethodInstances(instances []*pluginent.PaymentProviderInstance, _ string) []*pluginent.PaymentProviderInstance {
	return instances
}

func filterVisibleMethodInstancesByProviderKey(instances []*pluginent.PaymentProviderInstance, _ string, _ string) []*pluginent.PaymentProviderInstance {
	return instances
}

func distinctVisibleMethodProviderKeys(instances []*pluginent.PaymentProviderInstance) []string {
	seen := make(map[string]struct{}, len(instances))
	out := make([]string, 0, len(instances))
	for _, inst := range instances {
		if inst == nil {
			continue
		}
		if _, ok := seen[inst.ProviderKey]; ok {
			continue
		}
		seen[inst.ProviderKey] = struct{}{}
		out = append(out, inst.ProviderKey)
	}
	return out
}

func selectVisibleMethodInstanceByProviderKey(instances []*pluginent.PaymentProviderInstance, providerKey string) *pluginent.PaymentProviderInstance {
	for _, inst := range instances {
		if inst != nil && inst.ProviderKey == providerKey {
			return inst
		}
	}
	return nil
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
