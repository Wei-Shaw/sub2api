package service

import "strings"

func optionalTrimmedStringPtr(raw string) *string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func optionalStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func forwardResultBillingModel(requestedModel, upstreamModel string) string {
	if trimmed := strings.TrimSpace(requestedModel); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(upstreamModel)
}

func optionalInt64Ptr(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}

// usedProxyID returns the proxy actually used for the upstream request.
// Matches gateway_forward: skip when custom BaseURL is enabled.
func usedProxyID(account *Account) *int64 {
	if account == nil || account.ProxyID == nil || *account.ProxyID == 0 || account.Proxy == nil {
		return nil
	}
	if account.IsCustomBaseURLEnabled() && account.GetCustomBaseURL() != "" {
		return nil
	}
	return account.ProxyID
}
