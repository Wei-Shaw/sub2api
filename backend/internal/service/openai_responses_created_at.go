package service

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func openAIResponsesCreatedAtIsValid(createdAt gjson.Result) bool {
	if !createdAt.Exists() || createdAt.Type != gjson.Number {
		return false
	}
	value, err := strconv.ParseInt(createdAt.Raw, 10, 64)
	return err == nil && value > 0
}

// normalizeOpenAIResponsesCreatedAt repairs one Responses response timestamp
// without reserializing the surrounding JSON. Valid positive base-10 int64
// values remain byte-for-byte unchanged; strings, zero, negative values,
// fractions, exponent forms, and overflow are replaced with the current Unix
// timestamp.
func normalizeOpenAIResponsesCreatedAt(payload []byte, path string) ([]byte, bool) {
	path = strings.TrimSpace(path)
	if len(payload) == 0 || path == "" || !gjson.ValidBytes(payload) {
		return payload, false
	}
	if openAIResponsesCreatedAtIsValid(gjson.GetBytes(payload, path)) {
		return payload, true
	}
	updated, err := sjson.SetBytes(payload, path, time.Now().Unix())
	if err != nil {
		return payload, false
	}
	return updated, true
}

func openAIResponsesEventCarriesResponse(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "response.created",
		"response.in_progress",
		"response.completed",
		"response.done",
		"response.incomplete",
		"response.cancelled",
		"response.canceled",
		"response.failed":
		return true
	default:
		return false
	}
}

// normalizeOpenAIResponsesEventCreatedAt applies the strict timestamp contract
// only to event envelopes that carry a response object. Other Responses delta
// events, bare errors, and lifecycle envelopes without a JSON response object
// pass through unchanged; complete synthetic responses belong to their builders.
func normalizeOpenAIResponsesEventCreatedAt(payload []byte, eventType string) ([]byte, bool) {
	if !openAIResponsesEventCarriesResponse(eventType) {
		return payload, true
	}
	response := gjson.GetBytes(payload, "response")
	if !response.Exists() || !response.IsObject() {
		return payload, true
	}
	return normalizeOpenAIResponsesCreatedAt(payload, "response.created_at")
}

// buildOpenAIResponseFailedFallbackPayload is only reached if the normal
// primitive-only payload marshal unexpectedly fails. Quote the dynamic response
// ID explicitly so even an unusual upstream value cannot break the JSON frame.
func buildOpenAIResponseFailedFallbackPayload(responseID string) []byte {
	quotedResponseID, _ := json.Marshal(responseID)
	return []byte(`{"type":"response.failed","response":{"id":` + string(quotedResponseID) +
		`,"object":"response","created_at":` + strconv.FormatInt(time.Now().Unix(), 10) +
		`,"status":"failed","output":[],"error":{"code":"upstream_error","message":"Upstream response failed"}}}`)
}
