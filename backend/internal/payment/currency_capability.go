package payment

import (
	"context"
	"encoding/json"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

type paymentCurrencyContextKey struct{}

// WithPaymentCurrency constrains instance selection to providers that can collect
// the requested payment currency. Empty currencies leave selection unchanged.
func WithPaymentCurrency(ctx context.Context, currency string) context.Context {
	currency = NormalizeCurrencyCode(currency)
	if currency == "" {
		return ctx
	}
	return context.WithValue(ctx, paymentCurrencyContextKey{}, currency)
}

func paymentCurrencyFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	currency, _ := ctx.Value(paymentCurrencyContextKey{}).(string)
	return NormalizeCurrencyCode(currency)
}

// NormalizeCurrencyCode normalizes ISO-like payment currency codes used across
// providers. Unknown codes are preserved as uppercase to keep the payment layer
// extensible for new gateway currencies.
func NormalizeCurrencyCode(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

// NormalizeCurrencyList normalizes, deduplicates, and preserves caller order.
func NormalizeCurrencyList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		currency := NormalizeCurrencyCode(value)
		if currency == "" {
			continue
		}
		if _, ok := seen[currency]; ok {
			continue
		}
		seen[currency] = struct{}{}
		out = append(out, currency)
	}
	return out
}

// ProviderDefaultPaymentCurrencies is the provider capability catalog. Business
// flows must ask this catalog/instance overrides instead of hardcoding a special
// case such as "SePay == VND" in quote/order logic.
func ProviderDefaultPaymentCurrencies(providerKey, paymentType string) []string {
	providerKey = strings.TrimSpace(providerKey)
	baseType := GetBasePaymentType(strings.TrimSpace(paymentType))

	switch providerKey {
	case TypeSepay:
		return []string{"VND"}
	case TypeAlipay, TypeWxpay, TypeEasyPay:
		return []string{"CNY"}
	case TypePaddle:
		return []string{"USD"}
	case TypeStripe:
		switch baseType {
		case TypeAlipay, TypeWxpay:
			return []string{"CNY"}
		case TypeCard, TypeLink, TypeStripe:
			return []string{"USD"}
		default:
			return []string{"USD"}
		}
	}

	switch baseType {
	case TypeSepay:
		return []string{"VND"}
	case TypeAlipay, TypeWxpay:
		return []string{"CNY"}
	case TypePaddle:
		return []string{"USD"}
	case TypeCard, TypeLink, TypeStripe:
		return []string{"USD"}
	default:
		return nil
	}
}

// InstancePaymentCurrencies resolves the currencies accepted by a concrete
// provider instance for a user-facing payment method. Resolution order:
//  1. method-specific limits JSON override;
//  2. provider config override;
//  3. provider capability catalog default.
//
// The first two allow future gateways/currencies (e.g. Paddle KRW) without code
// changes or schema migrations.
func InstancePaymentCurrencies(inst *dbent.PaymentProviderInstance, paymentType string, config map[string]string) []string {
	if inst == nil {
		return ProviderDefaultPaymentCurrencies("", paymentType)
	}
	if currencies := instanceLimitPaymentCurrencies(inst.Limits, inst.ProviderKey, paymentType); len(currencies) > 0 {
		return currencies
	}
	if currencies := configPaymentCurrencies(config, paymentType); len(currencies) > 0 {
		return currencies
	}
	return ProviderDefaultPaymentCurrencies(inst.ProviderKey, paymentType)
}

func InstanceSupportsPaymentCurrency(inst *dbent.PaymentProviderInstance, paymentType string, currency string, config map[string]string) bool {
	currency = NormalizeCurrencyCode(currency)
	if currency == "" {
		return true
	}
	currencies := InstancePaymentCurrencies(inst, paymentType, config)
	if len(currencies) == 0 {
		return true
	}
	for _, supported := range currencies {
		if currency == supported {
			return true
		}
	}
	return false
}

func instanceLimitPaymentCurrencies(rawLimits, providerKey, paymentType string) []string {
	if strings.TrimSpace(rawLimits) == "" {
		return nil
	}
	var limits map[string]json.RawMessage
	if err := json.Unmarshal([]byte(rawLimits), &limits); err != nil {
		return nil
	}
	lookupKeys := []string{paymentType}
	if providerKey == TypeStripe {
		lookupKeys = append([]string{TypeStripe}, lookupKeys...)
	}
	if aliasKey := legacyVisibleMethodAlias(paymentType); aliasKey != "" {
		lookupKeys = append(lookupKeys, aliasKey)
	}
	for _, key := range lookupKeys {
		raw, ok := limits[key]
		if !ok {
			continue
		}
		if currencies := channelLimitPaymentCurrencies(raw); len(currencies) > 0 {
			return currencies
		}
	}
	return nil
}

func channelLimitPaymentCurrencies(raw json.RawMessage) []string {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	for _, key := range []string{"allowedPaymentCurrencies", "allowed_payment_currencies", "paymentCurrencies", "payment_currencies", "currencies"} {
		if currencies := parseCurrencyListRaw(payload[key]); len(currencies) > 0 {
			return currencies
		}
	}
	return nil
}

func configPaymentCurrencies(config map[string]string, paymentType string) []string {
	if len(config) == 0 {
		return nil
	}
	methodKeys := []string{
		"allowedPaymentCurrencies." + paymentType,
		"allowed_payment_currencies." + paymentType,
		"paymentCurrencies." + paymentType,
		"payment_currencies." + paymentType,
	}
	for _, key := range methodKeys {
		if currencies := parseCurrencyListString(configValue(config, key)); len(currencies) > 0 {
			return currencies
		}
	}
	for _, key := range []string{"allowedPaymentCurrencies", "allowed_payment_currencies", "paymentCurrencies", "payment_currencies", "currencies"} {
		if currencies := parseCurrencyListString(configValue(config, key)); len(currencies) > 0 {
			return currencies
		}
	}
	if currency := NormalizeCurrencyCode(configValue(config, "currency")); currency != "" {
		return []string{currency}
	}
	return nil
}

func configValue(config map[string]string, fieldName string) string {
	for key, value := range config {
		if strings.EqualFold(key, fieldName) {
			return value
		}
	}
	return ""
}

func parseCurrencyListRaw(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return NormalizeCurrencyList(list)
	}
	var csv string
	if err := json.Unmarshal(raw, &csv); err == nil {
		return parseCurrencyListString(csv)
	}
	return nil
}

func parseCurrencyListString(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var list []string
	if strings.HasPrefix(raw, "[") && json.Unmarshal([]byte(raw), &list) == nil {
		return NormalizeCurrencyList(list)
	}
	return NormalizeCurrencyList(strings.Split(raw, ","))
}
