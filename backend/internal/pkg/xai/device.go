package xai

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DeviceCodeResponse is the response from the xAI device authorization endpoint.
type DeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// DevicePollStatus is the result of a single device-token poll.
type DevicePollStatus string

const (
	DevicePollPending    DevicePollStatus = "pending"
	DevicePollSlowDown   DevicePollStatus = "slow_down"
	DevicePollAuthorized DevicePollStatus = "authorized"
	DevicePollDenied     DevicePollStatus = "denied"
	DevicePollExpired    DevicePollStatus = "expired"
	DevicePollError      DevicePollStatus = "error"
)

// DevicePollResult is one device-token poll outcome.
type DevicePollResult struct {
	Status   DevicePollStatus
	Token    *TokenResponse
	Error    string
	HTTPCode int
}

// NormalizeDeviceCodeResponse fills defaults and validates required fields.
func NormalizeDeviceCodeResponse(device *DeviceCodeResponse) error {
	if device == nil {
		return fmt.Errorf("device code response is nil")
	}
	device.DeviceCode = strings.TrimSpace(device.DeviceCode)
	device.UserCode = strings.TrimSpace(device.UserCode)
	device.VerificationURI = strings.TrimSpace(device.VerificationURI)
	device.VerificationURIComplete = strings.TrimSpace(device.VerificationURIComplete)
	if device.DeviceCode == "" || device.UserCode == "" {
		return fmt.Errorf("device code response is incomplete")
	}
	if device.VerificationURIComplete == "" && device.VerificationURI == "" {
		return fmt.Errorf("device code response missing verification URI")
	}
	if device.Interval <= 0 {
		device.Interval = 5
	}
	if device.ExpiresIn <= 0 {
		device.ExpiresIn = 1800
	}
	return nil
}

// DeviceAuthURL returns the preferred browser URL for a device session.
func DeviceAuthURL(device *DeviceCodeResponse) string {
	if device == nil {
		return ""
	}
	if url := strings.TrimSpace(device.VerificationURIComplete); url != "" {
		return url
	}
	return strings.TrimSpace(device.VerificationURI)
}

// ParseOAuthError extracts error / error_description from an OAuth error body.
func ParseOAuthError(body string) (code, description string) {
	var payload struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return "", strings.TrimSpace(body)
	}
	return strings.TrimSpace(payload.Error), strings.TrimSpace(payload.ErrorDescription)
}

// ClassifyDevicePollBody maps a non-success OAuth device poll body to a status.
func ClassifyDevicePollBody(statusCode int, body string) DevicePollResult {
	code, description := ParseOAuthError(body)
	switch code {
	case "authorization_pending":
		return DevicePollResult{Status: DevicePollPending, HTTPCode: statusCode}
	case "slow_down":
		return DevicePollResult{Status: DevicePollSlowDown, HTTPCode: statusCode}
	case "access_denied":
		return DevicePollResult{Status: DevicePollDenied, HTTPCode: statusCode, Error: firstNonEmpty(description, code)}
	case "expired_token":
		return DevicePollResult{Status: DevicePollExpired, HTTPCode: statusCode, Error: firstNonEmpty(description, code)}
	default:
		msg := firstNonEmpty(description, code, strings.TrimSpace(body), fmt.Sprintf("status %d", statusCode))
		return DevicePollResult{Status: DevicePollError, HTTPCode: statusCode, Error: msg}
	}
}
