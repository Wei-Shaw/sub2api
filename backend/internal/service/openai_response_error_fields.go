package service

import (
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
)

type openAIResponsesErrorFields struct {
	Code            string
	Type            string
	Message         string
	TransportStatus int
	SemanticStatus  int
}

func firstOpenAIResponsesErrorString(payload []byte, paths ...string) string {
	for _, path := range paths {
		if value := strings.TrimSpace(gjson.GetBytes(payload, path).String()); value != "" {
			return value
		}
	}
	return ""
}

func extractOpenAIResponsesErrorFields(payload []byte, transportStatus int) openAIResponsesErrorFields {
	fields := openAIResponsesErrorFields{
		Code: firstOpenAIResponsesErrorString(payload,
			"error.code", "response.error.code", "code"),
		Type: firstOpenAIResponsesErrorString(payload,
			"error.type", "response.error.type"),
		Message: firstOpenAIResponsesErrorString(payload,
			"response.error.message", "error.message", "message"),
		TransportStatus: transportStatus,
		SemanticStatus:  http.StatusBadGateway,
	}

	for _, path := range []string{
		"response.error.status_code", "error.status_code", "status_code",
		"response.error.status", "error.status", "status",
	} {
		if status := int(gjson.GetBytes(payload, path).Int()); status >= 400 && status <= 599 {
			fields.SemanticStatus = status
			break
		}
	}

	code := strings.ToLower(strings.TrimSpace(fields.Code))
	errType := strings.ToLower(strings.TrimSpace(fields.Type))
	switch {
	case code == "cyber_policy":
		fields.SemanticStatus = http.StatusBadRequest
	case strings.Contains(code, "rate_limit"):
		fields.SemanticStatus = http.StatusTooManyRequests
	case strings.Contains(errType, "invalid_request"):
		fields.SemanticStatus = http.StatusBadRequest
	}

	return fields
}
