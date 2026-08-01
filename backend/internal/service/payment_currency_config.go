package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func normalizeCurrencyCode(value, fallback string) string {
	v := strings.ToUpper(strings.TrimSpace(value))
	if v == "" {
		return strings.ToUpper(strings.TrimSpace(fallback))
	}
	return v
}

func normalizeCurrencyList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		currency := normalizeCurrencyCode(value, "")
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

func parseCurrencyList(raw string, fallback string) []string {
	if strings.TrimSpace(raw) == "" {
		raw = fallback
	}
	parts := strings.Split(raw, ",")
	return normalizeCurrencyList(parts)
}

func parseManualFXRates(raw string) map[string]float64 {
	parsed, err := parseManualFXRatesJSON(raw)
	if err != nil {
		parsed, _ = parseManualFXRatesJSON(defaultManualFXRatesJSON)
	}
	return parsed
}

func parseManualFXRatesJSON(raw string) (map[string]float64, error) {
	result := make(map[string]float64)
	if strings.TrimSpace(raw) == "" {
		raw = defaultManualFXRatesJSON
	}
	var payload map[string]float64
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, err
	}
	for currency, rate := range payload {
		normalized := normalizeCurrencyCode(currency, "")
		if normalized == "" {
			continue
		}
		if rate <= 0 {
			return nil, fmt.Errorf("invalid fx rate for %s", normalized)
		}
		result[normalized] = rate
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("manual fx rates cannot be empty")
	}
	return result, nil
}

func normalizeManualFXRatesJSON(raw string) string {
	parsed, err := parseManualFXRatesJSON(raw)
	if err != nil {
		parsed, _ = parseManualFXRatesJSON(defaultManualFXRatesJSON)
	}
	keys := make([]string, 0, len(parsed))
	for currency := range parsed {
		keys = append(keys, currency)
	}
	sort.Strings(keys)
	ordered := make(map[string]float64, len(keys))
	for _, currency := range keys {
		ordered[currency] = parsed[currency]
	}
	blob, err := json.Marshal(ordered)
	if err != nil {
		return defaultManualFXRatesJSON
	}
	return string(blob)
}
