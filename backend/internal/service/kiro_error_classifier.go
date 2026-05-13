package service

import (
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
)

const (
	kiroErrorAuthError              = "auth_error"
	kiroErrorMonthlyRequest         = "monthly_request_count"
	kiroErrorProfileError           = "profile_error"
	kiroErrorQuotaExhausted         = "quota_exhausted"
	kiroErrorOverageExhausted       = "overage_exhausted"
	kiroErrorRateLimited            = "rate_limited"
	kiroErrorSuspended              = "suspended"
	kiroErrorUsageForbidden         = "usage_forbidden"
	kiroErrorUpstreamTransient      = "upstream_transient"
	kiroErrorBadRequestSchema       = "bad_request_schema"
	kiroErrorBadRequestToolPairing  = "bad_request_tool_pairing"
	kiroErrorBadRequestInvalidModel = "bad_request_invalid_model"
	kiroErrorBadRequestAuth         = "bad_request_auth"
	kiroErrorBadRequestQuota        = "bad_request_quota"
	kiroErrorBadRequestUnknown      = "bad_request_unknown"
	kiroErrorRefreshTokenInvalid    = "refresh_token_invalid"

	kiroQuotaStateNormal           = "normal"
	kiroQuotaStateOverageActive    = "overage_active"
	kiroQuotaStateCreditsExhausted = "credits_exhausted"
	kiroQuotaStateOverageExhausted = "overage_exhausted"
)

type kiroErrorClassification struct {
	Category   string
	StatusCode int
	Message    string
}

func classifyKiroHTTPError(statusCode int, body string) kiroErrorClassification {
	trimmed := strings.TrimSpace(body)
	lower := strings.ToLower(trimmed)

	switch {
	case statusCode == http.StatusUnauthorized:
		return kiroErrorClassification{Category: kiroErrorAuthError, StatusCode: statusCode, Message: trimmed}
	case statusCode == http.StatusPaymentRequired && looksLikeKiroMonthlyRequestCountError(trimmed):
		return kiroErrorClassification{Category: kiroErrorMonthlyRequest, StatusCode: statusCode, Message: trimmed}
	case statusCode == http.StatusForbidden && isKiroSuspendedBody([]byte(trimmed)):
		return kiroErrorClassification{Category: kiroErrorSuspended, StatusCode: statusCode, Message: trimmed}
	case looksLikeKiroProfileError(lower):
		return kiroErrorClassification{Category: kiroErrorProfileError, StatusCode: statusCode, Message: trimmed}
	case statusCode == http.StatusBadRequest:
		return classifyKiroBadRequest(trimmed, lower)
	case statusCode == http.StatusForbidden && isKiroTokenErrorBody([]byte(trimmed)):
		return kiroErrorClassification{Category: kiroErrorAuthError, StatusCode: statusCode, Message: trimmed}
	case looksLikeKiroOverageExhaustedError(lower):
		return kiroErrorClassification{Category: kiroErrorOverageExhausted, StatusCode: statusCode, Message: trimmed}
	case looksLikeKiroQuotaExhaustedError(lower):
		return kiroErrorClassification{Category: kiroErrorQuotaExhausted, StatusCode: statusCode, Message: trimmed}
	case statusCode == http.StatusTooManyRequests:
		return kiroErrorClassification{Category: kiroErrorRateLimited, StatusCode: statusCode, Message: trimmed}
	case statusCode == http.StatusForbidden:
		return kiroErrorClassification{Category: kiroErrorUsageForbidden, StatusCode: statusCode, Message: trimmed}
	case statusCode >= 500:
		return kiroErrorClassification{Category: kiroErrorUpstreamTransient, StatusCode: statusCode, Message: trimmed}
	default:
		return kiroErrorClassification{Category: kiroErrorUpstreamTransient, StatusCode: statusCode, Message: trimmed}
	}
}

func classifyKiroError(err error) kiroErrorClassification {
	if err == nil {
		return kiroErrorClassification{}
	}

	var httpErr *kiroUsageHTTPError
	if errors.As(err, &httpErr) && httpErr != nil {
		return classifyKiroHTTPError(httpErr.StatusCode, httpErr.Body)
	}

	errStr := strings.TrimSpace(err.Error())
	lower := strings.ToLower(errStr)
	switch {
	case looksLikeKiroInvalidGrantError(lower):
		return kiroErrorClassification{Category: kiroErrorRefreshTokenInvalid, Message: errStr}
	case looksLikeKiroMonthlyRequestCountError(errStr):
		return kiroErrorClassification{Category: kiroErrorMonthlyRequest, Message: errStr}
	case looksLikeKiroProfileError(lower):
		return kiroErrorClassification{Category: kiroErrorProfileError, Message: errStr}
	case looksLikeKiroOverageExhaustedError(lower):
		return kiroErrorClassification{Category: kiroErrorOverageExhausted, Message: errStr}
	case looksLikeKiroQuotaExhaustedError(lower):
		return kiroErrorClassification{Category: kiroErrorQuotaExhausted, Message: errStr}
	case strings.Contains(lower, "context deadline exceeded"),
		strings.Contains(lower, "timeout"),
		isNetErr(err):
		return kiroErrorClassification{Category: kiroErrorUpstreamTransient, Message: errStr}
	default:
		return kiroErrorClassification{Category: kiroErrorUpstreamTransient, Message: errStr}
	}
}

func classifyKiroBadRequest(trimmed, lower string) kiroErrorClassification {
	switch {
	case looksLikeKiroBadRequestSchemaError(lower):
		return kiroErrorClassification{Category: kiroErrorBadRequestSchema, StatusCode: http.StatusBadRequest, Message: trimmed}
	case looksLikeKiroBadRequestToolPairingError(lower):
		return kiroErrorClassification{Category: kiroErrorBadRequestToolPairing, StatusCode: http.StatusBadRequest, Message: trimmed}
	case looksLikeKiroBadRequestInvalidModelError(lower):
		return kiroErrorClassification{Category: kiroErrorBadRequestInvalidModel, StatusCode: http.StatusBadRequest, Message: trimmed}
	case looksLikeKiroInvalidGrantError(lower) || looksLikeKiroBadRequestAuthError(lower):
		return kiroErrorClassification{Category: kiroErrorBadRequestAuth, StatusCode: http.StatusBadRequest, Message: trimmed}
	case looksLikeKiroQuotaExhaustedError(lower) || looksLikeKiroMonthlyRequestCountError(trimmed):
		return kiroErrorClassification{Category: kiroErrorBadRequestQuota, StatusCode: http.StatusBadRequest, Message: trimmed}
	default:
		return kiroErrorClassification{Category: kiroErrorBadRequestUnknown, StatusCode: http.StatusBadRequest, Message: trimmed}
	}
}

func looksLikeKiroBadRequestSchemaError(lower string) bool {
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "schema") ||
		strings.Contains(lower, "inputschema") ||
		strings.Contains(lower, "improperly formed request") ||
		strings.Contains(lower, "additionalproperties") ||
		(strings.Contains(lower, "properties") && strings.Contains(lower, "required"))
}

func looksLikeKiroBadRequestToolPairingError(lower string) bool {
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "tool_use") ||
		strings.Contains(lower, "tool_result") ||
		strings.Contains(lower, "tooluseid") ||
		strings.Contains(lower, "toolresults") ||
		strings.Contains(lower, "must be paired") ||
		strings.Contains(lower, "missing tool result")
}

func looksLikeKiroBadRequestInvalidModelError(lower string) bool {
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "invalid model") ||
		strings.Contains(lower, "invalid_model_id") ||
		strings.Contains(lower, "model not supported") ||
		strings.Contains(lower, "unsupportedmodel") ||
		strings.Contains(lower, "modelid")
}

func looksLikeKiroBadRequestAuthError(lower string) bool {
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "invalid token") ||
		strings.Contains(lower, "expired token") ||
		strings.Contains(lower, "access token") ||
		strings.Contains(lower, "refresh token")
}

func looksLikeKiroInvalidGrantError(lower string) bool {
	return strings.Contains(lower, "invalid_grant")
}

func looksLikeKiroMonthlyRequestCountError(body string) bool {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return false
	}
	if strings.Contains(trimmed, "MONTHLY_REQUEST_COUNT") {
		return true
	}
	if !gjson.Valid(trimmed) {
		return false
	}
	return gjson.Get(trimmed, "reason").String() == "MONTHLY_REQUEST_COUNT" ||
		gjson.Get(trimmed, "error.reason").String() == "MONTHLY_REQUEST_COUNT"
}

func looksLikeKiroProfileError(lower string) bool {
	if lower == "" {
		return false
	}
	return (strings.Contains(lower, "profilearn") && strings.Contains(lower, "required")) ||
		(strings.Contains(lower, "profile arn") && strings.Contains(lower, "required")) ||
		(strings.Contains(lower, "profile") && strings.Contains(lower, "not found")) ||
		(strings.Contains(lower, "invalid profile")) ||
		(strings.Contains(lower, "listavailableprofiles"))
}

func looksLikeKiroQuotaExhaustedError(lower string) bool {
	if lower == "" {
		return false
	}
	return (strings.Contains(lower, "credit") && (strings.Contains(lower, "exhaust") || strings.Contains(lower, "depleted"))) ||
		(strings.Contains(lower, "quota") && (strings.Contains(lower, "exhaust") || strings.Contains(lower, "exceeded") || strings.Contains(lower, "depleted"))) ||
		(strings.Contains(lower, "usage limit") && (strings.Contains(lower, "reached") || strings.Contains(lower, "exceeded"))) ||
		(strings.Contains(lower, "resource has been exhausted"))
}

func looksLikeKiroOverageExhaustedError(lower string) bool {
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "overage") &&
		(strings.Contains(lower, "exhaust") ||
			strings.Contains(lower, "disabled") ||
			strings.Contains(lower, "not enabled") ||
			strings.Contains(lower, "not allowed") ||
			strings.Contains(lower, "limit"))
}

func isNetErr(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr)
}
