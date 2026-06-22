// Package clienterror normalizes user-facing error messages.
package clienterror

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Message returns an English-only message suitable for API clients.
// It preserves existing English messages but replaces CJK/localized messages
// with a known English translation or a status-appropriate local generic
// message.
func Message(status int, message string) string {
	return MessageWithFallback(status, message, DefaultMessage(status))
}

// UpstreamMessage is like Message, but unknown localized upstream text falls
// back to an upstream-oriented message rather than a local application one.
func UpstreamMessage(status int, message string) string {
	return MessageWithFallback(status, message, DefaultUpstreamMessage(status))
}

// MessageWithFallback normalizes message to English and uses fallback when the
// original text is empty or localized but not in the known translation table.
func MessageWithFallback(status int, message string, fallback string) string {
	msg := strings.TrimSpace(message)
	if msg == "" {
		return fallbackOrDefault(status, fallback)
	}
	if translated := translateKnownMessage(msg); translated != "" {
		return translated
	}
	if ContainsCJK(msg) {
		return fallbackOrDefault(status, fallback)
	}
	return msg
}

// MessageWithCode is like MessageWithFallback, but prefers a stable machine
// code when one is available.
func MessageWithCode(status int, code string, message string, fallback string) string {
	if translated := translateKnownCode(code, false); translated != "" {
		return translated
	}
	msg := strings.TrimSpace(message)
	if msg == "" {
		return fallbackOrDefault(status, fallback)
	}
	if translated := translateKnownMessage(msg); translated != "" {
		return translated
	}
	if ContainsCJK(msg) {
		return fallbackOrDefault(status, fallback)
	}
	return msg
}

// UpstreamMessageWithCode is like MessageWithCode, but prefers upstream-oriented
// fallback text when the code is unknown.
func UpstreamMessageWithCode(status int, code string, message string) string {
	if translated := translateKnownCode(code, true); translated != "" {
		return translated
	}
	msg := strings.TrimSpace(message)
	if msg == "" {
		return DefaultUpstreamMessage(status)
	}
	if translated := translateKnownMessage(msg); translated != "" {
		return translated
	}
	if ContainsCJK(msg) {
		return DefaultUpstreamMessage(status)
	}
	return msg
}

// DefaultMessage returns a generic local-application English message for the
// HTTP status.
func DefaultMessage(status int) string {
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return "Invalid request"
	case http.StatusUnauthorized:
		return "Authentication failed"
	case http.StatusForbidden:
		return "Access forbidden"
	case http.StatusNotFound:
		return "Resource not found"
	case http.StatusRequestEntityTooLarge:
		return "Request body is too large"
	case http.StatusTooManyRequests:
		return "Rate limit exceeded, please retry later"
	case http.StatusGatewayTimeout, http.StatusRequestTimeout:
		return "Request timed out"
	case http.StatusServiceUnavailable, 529:
		return "Service temporarily unavailable"
	case http.StatusBadGateway:
		return "Upstream request failed"
	default:
		if status >= 500 {
			return "Internal server error"
		}
		return "Request failed"
	}
}

// DefaultUpstreamMessage returns a generic English message for errors whose
// source is an upstream provider or relay.
func DefaultUpstreamMessage(status int) string {
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return "Invalid upstream request"
	case http.StatusUnauthorized:
		return "Upstream authentication failed, please contact administrator"
	case http.StatusForbidden:
		return "Upstream access forbidden, please contact administrator"
	case http.StatusNotFound:
		return "Upstream resource not found"
	case http.StatusRequestEntityTooLarge:
		return "Upstream request body is too large"
	case http.StatusTooManyRequests:
		return "Upstream rate limit exceeded, please retry later"
	case http.StatusGatewayTimeout, http.StatusRequestTimeout:
		return "Upstream request timed out"
	case http.StatusServiceUnavailable, 529:
		return "Upstream service temporarily unavailable"
	case http.StatusBadGateway:
		return "Upstream request failed"
	default:
		if status >= 500 {
			return "Upstream service temporarily unavailable"
		}
		return "Upstream request failed"
	}
}

// TypeForHTTPStatus returns an OpenAI/Anthropic-style error type for a status.
func TypeForHTTPStatus(status int, fallback string) string {
	fallback = strings.TrimSpace(fallback)
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity, http.StatusRequestEntityTooLarge:
		return "invalid_request_error"
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusNotFound:
		return "not_found_error"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	case http.StatusGatewayTimeout, http.StatusRequestTimeout:
		return "timeout_error"
	case http.StatusServiceUnavailable, 529:
		return "overloaded_error"
	}
	if fallback != "" {
		return fallback
	}
	return "upstream_error"
}

// JSONBody rewrites common JSON error message fields to English. If the body is
// not JSON and contains CJK text, it returns a standard JSON error envelope.
func JSONBody(status int, body []byte, fallbackType, fallbackMessage string) []byte {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return buildJSON(status, fallbackType, fallbackMessage)
	}
	if !gjson.ValidBytes(trimmed) {
		if ContainsCJK(string(trimmed)) {
			return buildJSON(status, fallbackType, fallbackMessage)
		}
		return append([]byte(nil), body...)
	}

	out := append([]byte(nil), trimmed...)
	fallbackMessage = UpstreamMessage(status, fallbackMessage)
	for _, path := range []string{"error.message", "message", "detail", "error.detail"} {
		result := gjson.GetBytes(out, path)
		if !result.Exists() || result.Type != gjson.String {
			continue
		}
		normalized := MessageWithFallback(status, result.String(), fallbackMessage)
		if normalized != result.String() {
			var err error
			out, err = sjson.SetBytes(out, path, normalized)
			if err != nil {
				return buildJSON(status, fallbackType, fallbackMessage)
			}
		}
	}
	if errValue := gjson.GetBytes(out, "error"); errValue.Exists() && errValue.IsObject() && !gjson.GetBytes(out, "error.type").Exists() {
		var err error
		out, err = sjson.SetBytes(out, "error.type", TypeForHTTPStatus(status, fallbackType))
		if err != nil {
			return buildJSON(status, fallbackType, fallbackMessage)
		}
	}

	if errValue := gjson.GetBytes(out, "error"); errValue.Exists() && errValue.Type == gjson.String && ContainsCJK(errValue.String()) {
		return buildJSON(status, fallbackType, fallbackMessage)
	}
	if ContainsCJK(string(out)) {
		return buildJSON(status, fallbackType, fallbackMessage)
	}
	return out
}

// ContainsCJK reports whether s contains CJK characters likely to be non-English
// user-facing text. Vietnamese Latin diacritics are intentionally not matched.
func ContainsCJK(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) {
			return true
		}
	}
	return false
}

func buildJSON(status int, fallbackType, fallbackMessage string) []byte {
	errType := TypeForHTTPStatus(status, fallbackType)
	message := MessageWithFallback(status, fallbackMessage, DefaultUpstreamMessage(status))
	if message == "" {
		message = DefaultUpstreamMessage(status)
	}
	return []byte(fmt.Sprintf(`{"error":{"type":%q,"message":%q}}`, errType, message))
}

func fallbackOrDefault(status int, fallback string) string {
	if trimmed := strings.TrimSpace(fallback); trimmed != "" {
		if translated := translateKnownMessage(trimmed); translated != "" {
			return translated
		}
		if !ContainsCJK(trimmed) {
			return trimmed
		}
	}
	return DefaultMessage(status)
}

func translateKnownMessage(msg string) string {
	for _, item := range knownTranslations {
		if strings.Contains(msg, item.match) {
			return item.english
		}
	}
	return ""
}

func translateKnownCode(code string, upstream bool) string {
	normalized := normalizeErrorCode(code)
	if normalized == "" {
		return ""
	}
	if upstream {
		if msg := knownUpstreamCodeTranslations[normalized]; msg != "" {
			return msg
		}
	}
	return knownCodeTranslations[normalized]
}

func normalizeErrorCode(code string) string {
	normalized := strings.ToUpper(strings.TrimSpace(code))
	if normalized == "" {
		return ""
	}
	normalized = errorCodeReplacer.Replace(normalized)
	for strings.Contains(normalized, "__") {
		normalized = strings.ReplaceAll(normalized, "__", "_")
	}
	return strings.Trim(normalized, "_")
}

var errorCodeReplacer = strings.NewReplacer("-", "_", " ", "_", ".", "_")

var knownCodeTranslations = map[string]string{
	"ACCESS_TOKEN_EXPIRED":                     "Access token has expired",
	"API_KEY_DISABLED":                         "API key is disabled",
	"API_KEY_EXPIRED":                          "API key has expired",
	"API_KEY_INACTIVE":                         "API key is inactive",
	"API_KEY_NOT_FOUND":                        "API key not found",
	"API_KEY_QUOTA_EXHAUSTED":                  "API key quota exhausted",
	"API_KEY_RATE_1D_EXCEEDED":                 "API key daily rate limit exceeded",
	"API_KEY_RATE_5H_EXCEEDED":                 "API key 5-hour rate limit exceeded",
	"API_KEY_RATE_7D_EXCEEDED":                 "API key 7-day rate limit exceeded",
	"API_KEY_RATE_LIMITED":                     "API key is rate limited",
	"API_KEY_REQUIRED":                         "API key is required",
	"AUTH_IDENTITY_CHANNEL_OWNERSHIP_CONFLICT": "Auth identity channel already belongs to another user",
	"AUTH_IDENTITY_EMAIL_MISMATCH":             "OAuth identity belongs to a different email",
	"AUTH_IDENTITY_OWNERSHIP_CONFLICT":         "Auth identity already belongs to another user",
	"BACKEND_MODE_ADMIN_ONLY":                  "Backend mode is active. Only admin login is allowed.",
	"BAD_REQUEST":                              "Invalid request",
	"BALANCE_NOT_ENOUGH":                       "Balance is not enough",
	"BALANCE_PAYMENT_DISABLED":                 "Balance recharge has been disabled",
	"BILLING_MODE_MISSING_PRICE":               "Per-request price or intervals are required for per-request/image billing mode",
	"BILLING_SERVICE_ERROR":                    "Billing service temporarily unavailable. Please retry later.",
	"CONFIG_NOT_READY":                         "Config is not loaded",
	"CONFLICT":                                 "Conflict",
	"CONTEXT_LENGTH_EXCEEDED":                  "Context length exceeded",
	"DAILY_LIMIT_EXCEEDED":                     "Daily usage limit exceeded",
	"FORBIDDEN":                                "Access forbidden",
	"GATEWAY_TIMEOUT":                          "Request timed out",
	"GROUP_DELETED":                            "API key group has been deleted",
	"GROUP_DISABLED":                           "API key group has been disabled",
	"GROUP_NOT_ALLOWED":                        "API key group is no longer available for this user",
	"INSUFFICIENT_BALANCE":                     "Insufficient balance or quota",
	"INTERNAL_SERVER_ERROR":                    "Internal server error",
	"INVALID_API_KEY":                          "Invalid API key",
	"INVALID_CONTENT_MODERATION_BASE_URL":      "Invalid content moderation base URL",
	"INVALID_CONTENT_MODERATION_BLOCK_STATUS":  "Blocked HTTP status must be between 400 and 599",
	"INVALID_CONTENT_MODERATION_CONFIG":        "Invalid content moderation config",
	"INVALID_CONTENT_MODERATION_HASH":          "Invalid content moderation hash",
	"INVALID_CONTENT_MODERATION_MODE":          "Invalid content moderation mode",
	"INVALID_CONTENT_MODERATION_MODEL_FILTER":  "At least one model is required when specifying include/exclude models",
	"INVALID_INPUT":                            "Invalid input",
	"INVALID_MODERATION_TEST_IMAGE":            "Invalid moderation test image",
	"INVALID_ORDER_TYPE":                       "Only balance orders can request refund",
	"INVALID_PAYMENT_QUOTE":                    "Payment quote amount is invalid",
	"INVALID_PROXY_ASSIGNMENT":                 "Default live proxy assignment requires an active proxy",
	"INVALID_RESET_TOKEN":                      "Invalid or expired password reset token",
	"INVALID_RETURN_URL":                       "Return_url must be a valid absolute URL",
	"INVALID_STATUS":                           "Invalid status",
	"INVALID_TOKEN":                            "Invalid token",
	"INVALID_USER":                             "Invalid user",
	"INVALID_USER_ID":                          "Invalid user ID",
	"INVALID_VERIFY_CODE":                      "Invalid or expired verification code",
	"INVITATION_CODE_REQUIRED":                 "Invitation code is required",
	"MODERATION_TEST_IMAGE_TOO_LARGE":          "Test image must not exceed 8MB",
	"MONTHLY_LIMIT_EXCEEDED":                   "Monthly usage limit exceeded",
	"NOT_FOUND":                                "Not found",
	"OAUTH_DISABLED":                           "OAuth login is disabled",
	"OAUTH_INVITATION_REQUIRED":                "Invitation code required to complete OAuth registration",
	"PASSWORD_REQUIRED":                        "Password is required",
	"PAYMENT_CURRENCY_REQUIRED":                "Payment currency is required",
	"PAYMENT_DISABLED":                         "Payment system is disabled",
	"PAYMENT_FRONTEND_URL_INVALID":             "Payment frontend_url must be an absolute https URL",
	"PENDING_AUTH_CODE_EXPIRED":                "Pending auth completion code has expired",
	"PENDING_AUTH_CODE_INVALID":                "Pending auth completion code is invalid",
	"PENDING_AUTH_SESSION_CONSUMED":            "Pending auth session has already been used",
	"PENDING_AUTH_SESSION_EXPIRED":             "Pending auth session has expired",
	"PENDING_AUTH_SESSION_INVALID":             "Pending auth registration context is invalid",
	"PENDING_AUTH_SESSION_NOT_FOUND":           "Pending auth session not found",
	"RATE_LIMIT_EXCEEDED":                      "Rate limit exceeded, please retry later",
	"REFRESH_TOKEN_EXPIRED":                    "Refresh token has expired",
	"REFRESH_TOKEN_INVALID":                    "Invalid refresh token",
	"REFRESH_TOKEN_REUSED":                     "Refresh token has been reused",
	"REQUEST_ENTITY_TOO_LARGE":                 "Request body is too large",
	"REQUEST_TIMEOUT":                          "Request timed out",
	"SERVICE_UNAVAILABLE":                      "Service temporarily unavailable",
	"SUBSCRIPTION_EXPIRED":                     "Subscription has expired",
	"SUBSCRIPTION_INVALID":                     "Subscription is invalid or expired",
	"SUBSCRIPTION_NOT_FOUND":                   "No active subscription found",
	"SYSTEM_OPERATION_ID_REQUIRED":             "Operation id is required",
	"TOKEN_EXPIRED":                            "Token has expired",
	"TOKEN_REVOKED":                            "Token has been revoked",
	"TOKEN_TOO_LARGE":                          "Token is too large",
	"TOO_MANY_REQUESTS":                        "Rate limit exceeded, please retry later",
	"TURNSTILE_INVALID_SECRET_KEY":             "Invalid Turnstile secret key",
	"TURNSTILE_VERIFICATION_FAILED":            "Turnstile verification failed",
	"UNAUTHORIZED":                             "Authentication required",
	"UNPROCESSABLE_ENTITY":                     "Invalid request",
	"UNSUPPORTED_FIELD":                        "TargetGroupId is not accepted; send plan_id or balance_package_code",
	"USAGE_LIMIT_EXCEEDED":                     "Usage limit exceeded",
	"USER_PLATFORM_DAILY_QUOTA_EXHAUSTED":      "Daily usage quota exhausted for this platform.",
	"USER_PLATFORM_MONTHLY_QUOTA_EXHAUSTED":    "Monthly usage quota exhausted for this platform.",
	"USER_PLATFORM_WEEKLY_QUOTA_EXHAUSTED":     "Weekly usage quota exhausted for this platform.",
	"USER_RPM_EXCEEDED":                        "User requests-per-minute limit exceeded",
	"VALIDATION_ERROR":                         "Validation error",
	"WEEKLY_LIMIT_EXCEEDED":                    "Weekly usage limit exceeded",
}

var knownUpstreamCodeTranslations = map[string]string{
	"INSUFFICIENT_QUOTA":      "Upstream quota exhausted",
	"PERMISSION_DENIED":       "Upstream access forbidden, please contact administrator",
	"QUOTA_EXCEEDED":          "Upstream quota exhausted",
	"RATE_LIMIT_EXCEEDED":     "Upstream rate limit exceeded, please retry later",
	"RESOURCE_EXHAUSTED":      "Upstream rate limit exceeded, please retry later",
	"UNAUTHENTICATED":         "Upstream authentication failed, please contact administrator",
	"UNAVAILABLE":             "Upstream service temporarily unavailable",
	"UPSTREAM_AUTHENTICATION": "Upstream authentication failed, please contact administrator",
}

var knownTranslations = []struct {
	match   string
	english string
}{
	{"请求格式或参数不正确", "Invalid request format or parameters"},
	{"参数无效", "Invalid request parameters"},
	{"参数不正确", "Invalid request parameters"},
	{"请求格式", "Invalid request format"},
	{"参数错误", "Invalid request parameters"},
	{"请求无效", "Invalid request"},
	{"上游请求失败", "Upstream request failed"},
	{"上游失败", "Upstream request failed"},
	{"上游服务暂时不可用", "Upstream service temporarily unavailable"},
	{"上游服务不可用", "Upstream service temporarily unavailable"},
	{"上游认证失败", "Upstream authentication failed, please contact administrator"},
	{"上游访问被拒绝", "Upstream access forbidden, please contact administrator"},
	{"请求体为空", "Request body is empty"},
	{"API Key 所属专属分组不再允许当前用户使用", "API key group is no longer available for this user"},
	{"API Key 所属分组已删除", "API key group has been deleted"},
	{"API Key 所属分组已停用", "API key group has been disabled"},
	{"API key 额度已用完", "API key quota exhausted"},
	{"api key 额度已用完", "API key quota exhausted"},
	{"API key 已过期", "API key has expired"},
	{"api key 已过期", "API key has expired"},
	{"余额不足", "Insufficient balance or quota"},
	{"额度不足", "Insufficient balance or quota"},
	{"上下文超限", "Context length exceeded"},
	{"上下文长度", "Context length exceeded"},
	{"超出上下文", "Context length exceeded"},
	{"未登录", "Authentication required"},
	{"无权限", "Access forbidden"},
	{"资源不存在", "Resource not found"},
	{"服务器内部错误", "Internal server error"},
}
