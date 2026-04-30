package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

type paymentCurrencyContextKey struct{}
type currencyCapabilitiesContextKey struct{}

// CurrencyCapabilityConfig is loaded from payment settings, not compiled code.
// It lets admins define which currencies each payment method/provider/instance
// can collect without code changes when new gateways or currencies are added.
type CurrencyCapabilityConfig struct {
	Methods   map[string][]string `json:"methods,omitempty"`
	Providers map[string][]string `json:"providers,omitempty"`
	Instances map[string][]string `json:"instances,omitempty"`
}

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

// WithCurrencyCapabilities attaches settings-driven currency capabilities to the
// selection context so the load balancer can filter concrete provider instances.
func WithCurrencyCapabilities(ctx context.Context, capabilities CurrencyCapabilityConfig) context.Context {
	return context.WithValue(ctx, currencyCapabilitiesContextKey{}, NormalizeCurrencyCapabilityConfig(capabilities))
}

func currencyCapabilitiesFromContext(ctx context.Context) CurrencyCapabilityConfig {
	if ctx == nil {
		return CurrencyCapabilityConfig{}
	}
	capabilities, _ := ctx.Value(currencyCapabilitiesContextKey{}).(CurrencyCapabilityConfig)
	return NormalizeCurrencyCapabilityConfig(capabilities)
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

func ParseCurrencyCapabilityConfig(raw string) (CurrencyCapabilityConfig, error) {
	if strings.TrimSpace(raw) == "" {
		return CurrencyCapabilityConfig{}, nil
	}
	var cfg CurrencyCapabilityConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return CurrencyCapabilityConfig{}, err
	}
	return NormalizeCurrencyCapabilityConfig(cfg), nil
}

func NormalizeCurrencyCapabilityConfig(cfg CurrencyCapabilityConfig) CurrencyCapabilityConfig {
	return CurrencyCapabilityConfig{
		Methods:   normalizeCurrencyMap(cfg.Methods),
		Providers: normalizeCurrencyMap(cfg.Providers),
		Instances: normalizeCurrencyMap(cfg.Instances),
	}
}

func MarshalCurrencyCapabilityConfig(cfg CurrencyCapabilityConfig) (string, error) {
	cfg = NormalizeCurrencyCapabilityConfig(cfg)
	if len(cfg.Methods) == 0 && len(cfg.Providers) == 0 && len(cfg.Instances) == 0 {
		return "{}", nil
	}
	blob, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(blob), nil
}

// InstancePaymentCurrencies resolves currencies accepted by a concrete provider
// instance for a user-facing payment method. Resolution order:
//  1. method-specific provider instance limits JSON override;
//  2. provider instance config override;
//  3. global payment settings instance override;
//  4. global payment settings provider/method override;
//  5. global payment settings method override;
//  6. compiled safety default for gateways with a single real-world currency.
//
// Admin/provider configuration is still the source of truth. The compiled SePay
// fallback prevents legacy configs with an empty capability map from defaulting
// to the ledger currency (USD) even though SePay can only collect VND.
func InstancePaymentCurrencies(inst *dbent.PaymentProviderInstance, paymentType string, config map[string]string, capabilities CurrencyCapabilityConfig) []string {
	paymentType = strings.TrimSpace(paymentType)
	providerKey := ""
	if inst != nil {
		providerKey = strings.TrimSpace(inst.ProviderKey)
		if currencies := instanceLimitPaymentCurrencies(inst.Limits, inst.ProviderKey, paymentType); len(currencies) > 0 {
			return currencies
		}
		if currencies := configPaymentCurrencies(config, paymentType); len(currencies) > 0 {
			return currencies
		}
		if currencies := capabilityInstancePaymentCurrencies(capabilities, inst.ID); len(currencies) > 0 {
			return currencies
		}
		if currencies := capabilityProviderPaymentCurrencies(capabilities, inst.ProviderKey, paymentType); len(currencies) > 0 {
			return currencies
		}
	}
	if currencies := capabilityMethodPaymentCurrencies(capabilities, paymentType); len(currencies) > 0 {
		return currencies
	}
	return defaultGatewayPaymentCurrencies(providerKey, paymentType)
}

func defaultGatewayPaymentCurrencies(providerKey, paymentType string) []string {
	providerKey = strings.TrimSpace(providerKey)
	paymentType = strings.TrimSpace(paymentType)
	if providerKey == TypeSepay || paymentType == TypeSepay || GetBasePaymentType(paymentType) == TypeSepay {
		return []string{"VND"}
	}
	return nil
}

func InstanceSupportsPaymentCurrency(inst *dbent.PaymentProviderInstance, paymentType string, currency string, config map[string]string, capabilities CurrencyCapabilityConfig) bool {
	currency = NormalizeCurrencyCode(currency)
	if currency == "" {
		return true
	}
	currencies := InstancePaymentCurrencies(inst, paymentType, config, capabilities)
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

func capabilityInstancePaymentCurrencies(cfg CurrencyCapabilityConfig, instanceID int64) []string {
	if instanceID <= 0 || len(cfg.Instances) == 0 {
		return nil
	}
	return cfg.Instances[strconv.FormatInt(instanceID, 10)]
}

func capabilityProviderPaymentCurrencies(cfg CurrencyCapabilityConfig, providerKey, paymentType string) []string {
	if len(cfg.Providers) == 0 {
		return nil
	}
	providerKey = strings.TrimSpace(providerKey)
	paymentType = strings.TrimSpace(paymentType)
	for _, key := range []string{
		providerMethodKey(providerKey, paymentType),
		providerMethodKey(providerKey, GetBasePaymentType(paymentType)),
		providerKey,
	} {
		if currencies := cfg.Providers[key]; len(currencies) > 0 {
			return currencies
		}
	}
	return nil
}

func capabilityMethodPaymentCurrencies(cfg CurrencyCapabilityConfig, paymentType string) []string {
	if len(cfg.Methods) == 0 {
		return nil
	}
	paymentType = strings.TrimSpace(paymentType)
	for _, key := range []string{paymentType, GetBasePaymentType(paymentType)} {
		if currencies := cfg.Methods[key]; len(currencies) > 0 {
			return currencies
		}
	}
	return nil
}

func providerMethodKey(providerKey, paymentType string) string {
	providerKey = strings.TrimSpace(providerKey)
	paymentType = strings.TrimSpace(paymentType)
	if providerKey == "" || paymentType == "" {
		return ""
	}
	return providerKey + ":" + paymentType
}

func normalizeCurrencyMap(input map[string][]string) map[string][]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string][]string, len(input))
	for key, values := range input {
		key = strings.TrimSpace(key)
		currencies := NormalizeCurrencyList(values)
		if key == "" || len(currencies) == 0 {
			continue
		}
		out[key] = currencies
	}
	if len(out) == 0 {
		return nil
	}
	return out
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

func CurrencyCapabilityConfigError(raw string, err error) error {
	return fmt.Errorf("invalid currency capability config %q: %w", raw, err)
}
