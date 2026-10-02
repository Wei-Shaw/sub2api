package service

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
)

var (
	antigravityProjectRefRegex = regexp.MustCompile(`(?i)\bprojects/[a-z0-9][a-z0-9._:-]*`)
	antigravityEmailRegex      = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	antigravityConsumerRegex   = regexp.MustCompile(`(?i)\b(consumer|project(?:[ _-]?(?:id|number))?)(\s*[:=]?\s*['"]?)[0-9]{6,}`)
)

// sanitizeAntigravityErrorText 清除上游错误文本中的 GCP 项目号/项目 ID、服务账号邮箱及敏感查询参数。
func sanitizeAntigravityErrorText(msg string) string {
	if msg == "" {
		return msg
	}
	msg = sanitizeUpstreamErrorMessage(msg)
	msg = antigravityProjectRefRegex.ReplaceAllString(msg, "projects/***")
	msg = antigravityEmailRegex.ReplaceAllString(msg, "***")
	msg = antigravityConsumerRegex.ReplaceAllString(msg, "$1$2***")
	return msg
}

// buildAntigravityClientErrorBody 为客户端构造 Gemini 风格的错误体：
// 仅保留 code/status/message（message 已脱敏），丢弃 details 等可能含账号身份的字段。
func buildAntigravityClientErrorBody(statusCode int, body []byte) []byte {
	code := statusCode
	status := ""
	message := ""

	var parsed struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil {
		if parsed.Error.Code != 0 {
			code = parsed.Error.Code
		}
		status = parsed.Error.Status
		message = parsed.Error.Message
	}
	if strings.TrimSpace(message) == "" {
		message = strings.TrimSpace(extractUpstreamErrorMessage(body))
	}
	if strings.TrimSpace(message) == "" {
		message = http.StatusText(statusCode)
		if message == "" {
			message = "Upstream request failed"
		}
	}
	if status == "" {
		status = antigravityGeminiStatusFromHTTP(statusCode)
	}

	out, err := json.Marshal(map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": sanitizeAntigravityErrorText(message),
			"status":  status,
		},
	})
	if err != nil {
		return []byte(`{"error":{"code":500,"message":"Upstream request failed","status":"INTERNAL"}}`)
	}
	return out
}

func antigravityGeminiStatusFromHTTP(statusCode int) string {
	switch statusCode {
	case http.StatusBadRequest:
		return "INVALID_ARGUMENT"
	case http.StatusUnauthorized:
		return "UNAUTHENTICATED"
	case http.StatusForbidden:
		return "PERMISSION_DENIED"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusTooManyRequests:
		return "RESOURCE_EXHAUSTED"
	case http.StatusServiceUnavailable:
		return "UNAVAILABLE"
	case http.StatusGatewayTimeout:
		return "DEADLINE_EXCEEDED"
	default:
		if statusCode >= 500 {
			return "INTERNAL"
		}
		return "UNKNOWN"
	}
}
