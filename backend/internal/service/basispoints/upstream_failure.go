package basispoints

import (
	"encoding/json"
	"net/http"
)

// UpstreamFailure is a safe interpretation of an in-band error. Status is a
// semantic status, not evidence of the HTTP status returned by the upstream.
type UpstreamFailure struct {
	Status  int
	Code    string
	Type    string
	Message string
}

func (f *UpstreamFailure) Error() string { return f.Message }

func (f *UpstreamFailure) Details() map[string]any {
	return object{"status": f.Status, "code": f.Code, "type": f.Type, "message": f.Message}
}

// ParseUpstreamFailure recognizes failure terminals only. Unknown identifiers
// and free-form upstream messages never enter client responses or diagnostics.
func ParseUpstreamFailure(raw []byte) *UpstreamFailure {
	var payload object
	if decode(raw, &payload) != nil {
		return nil
	}
	return classifyUpstreamFailure(text(payload["type"]), payload)
}

func classifyUpstreamFailure(kind string, payload object) *UpstreamFailure {
	switch kind {
	case "error", "response.failed", "response.cancelled":
	default:
		return nil
	}
	response, _ := payload["response"].(object)
	detail, _ := response["error"].(object)
	if detail == nil {
		detail, _ = payload["error"].(object)
	}
	if detail == nil && kind == "error" {
		detail = payload
	}
	code := text(detail["code"])
	status := failureIdentifierStatus(code)
	if status == 0 {
		code = "basispoints_upstream_error"
		if kind == "response.cancelled" {
			code = "basispoints_upstream_cancelled"
		}
		status = failureIdentifierStatus(text(detail["type"]))
	}
	// An explicit error status takes precedence over an inconsistent code/type.
	for _, value := range []any{payload["status"], payload["status_code"], detail["status"], detail["status_code"], response["status_code"]} {
		if explicit := failureStatus(value); explicit != 0 {
			status = explicit
			break
		}
	}
	if status == 0 {
		status = http.StatusBadGateway
	}
	errorType := "server_error"
	switch status {
	case http.StatusUnauthorized:
		errorType = "authentication_error"
	case http.StatusForbidden:
		errorType = "permission_error"
	case http.StatusTooManyRequests:
		errorType = "rate_limit_error"
	default:
		if status < 500 {
			errorType = "invalid_request_error"
		}
	}
	return &UpstreamFailure{Status: status, Code: code, Type: errorType,
		Message: "Excel BPS upstream failure: " + code + "; request was not replayed"}
}

func failureStatus(value any) int {
	number, ok := value.(json.Number)
	if !ok {
		return 0
	}
	status, err := number.Int64()
	if err != nil || status < 400 || status > 599 {
		return 0
	}
	return int(status)
}

func failureIdentifierStatus(identifier string) int {
	switch identifier {
	case "invalid_request", "invalid_request_error", "bad_request_error", "invalid_argument", "invalid_value",
		"unsupported_value", "invalid_prompt", "context_length_exceeded", "string_above_max_length",
		"previous_response_not_found", "thinking_signature_invalid", "invalid_encrypted_content",
		"cyber_policy", "content_policy_violation":
		return http.StatusBadRequest
	case "authentication_error", "invalid_api_key", "token_expired":
		return http.StatusUnauthorized
	case "permission_error", "permission_denied", "basispoints_model_access_changed":
		return http.StatusForbidden
	case "model_not_found", "model_not_found_error":
		return http.StatusNotFound
	case "message_too_big":
		return http.StatusRequestEntityTooLarge
	case "rate_limit_error", "rate_limit_exceeded", "insufficient_quota", "usage_limit_reached":
		return http.StatusTooManyRequests
	case "server_error", "internal_server_error":
		return http.StatusInternalServerError
	case "overloaded_error", "service_unavailable":
		return http.StatusServiceUnavailable
	case "api_error", "basispoints_upstream_error", "basispoints_upstream_cancelled", "basispoints_protocol_error":
		return http.StatusBadGateway
	default:
		return 0
	}
}

// normalizeFailureEvent preserves response identity/output/usage after the
// bridge has withheld tools, but replaces error details with safe identifiers.
func normalizeFailureEvent(kind string, payload object) object {
	failure := classifyUpstreamFailure(kind, payload)
	if failure == nil {
		return payload
	}
	out := object{"status": failure.Status}
	if usage, ok := payload["usage"].(object); ok {
		out["usage"] = usage
	}
	if kind == "error" {
		out["error"] = failure.Details()
	} else {
		response, _ := payload["response"].(object)
		if response == nil {
			response = object{"status": "failed", "output": []any{}}
			if kind == "response.cancelled" {
				response["status"] = "cancelled"
			}
		}
		response["error"] = failure.Details()
		out["response"] = response
	}
	return out
}
