package service

import (
	"encoding/json"
	"net/http"
)

func newUpstreamStreamEndedFailoverError(message string) *UpstreamFailoverError {
	if message == "" {
		message = "Upstream stream ended without a response"
	}
	body, err := json.Marshal(map[string]any{
		"error": map[string]any{
			"type":    "upstream_error",
			"message": message,
		},
	})
	if err != nil {
		body = []byte(`{"error":{"type":"upstream_error","message":"Upstream stream ended without a response"}}`)
	}
	return &UpstreamFailoverError{
		StatusCode:             http.StatusBadGateway,
		ResponseBody:           body,
		RetryableOnSameAccount: true,
	}
}
