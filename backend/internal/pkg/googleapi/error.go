// Package googleapi provides helpers for Google-style API responses.
package googleapi

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ErrorResponse represents a Google API error response
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains the error details from Google API
type ErrorDetail struct {
	Code    int               `json:"code"`
	Message string            `json:"message"`
	Status  string            `json:"status"`
	Details []json.RawMessage `json:"details,omitempty"`
}

// ErrorDetailInfo contains additional error information
type ErrorDetailInfo struct {
	Type     string            `json:"@type"`
	Reason   string            `json:"reason,omitempty"`
	Domain   string            `json:"domain,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ErrorHelp contains help links
type ErrorHelp struct {
	Type  string     `json:"@type"`
	Links []HelpLink `json:"links,omitempty"`
}

// HelpLink represents a help link
type HelpLink struct {
	Description string `json:"description"`
	URL         string `json:"url"`
}

// ParseError parses a Google API error response and extracts key information
func ParseError(body string) (*ErrorResponse, error) {
	var errResp ErrorResponse
	if err := json.Unmarshal([]byte(body), &errResp); err != nil {
		return nil, fmt.Errorf("failed to parse error response: %w", err)
	}
	return &errResp, nil
}

// ExtractActivationURL extracts the API activation URL from error details
func ExtractActivationURL(body string) string {
	var errResp ErrorResponse
	if err := json.Unmarshal([]byte(body), &errResp); err != nil {
		return ""
	}

	// Check error details for activation URL
	for _, detailRaw := range errResp.Error.Details {
		// Parse as ErrorDetailInfo
		var info ErrorDetailInfo
		if err := json.Unmarshal(detailRaw, &info); err == nil {
			if info.Metadata != nil {
				if activationURL, ok := info.Metadata["activationUrl"]; ok && activationURL != "" {
					return activationURL
				}
			}
		}

		// Parse as ErrorHelp
		var help ErrorHelp
		if err := json.Unmarshal(detailRaw, &help); err == nil {
			for _, link := range help.Links {
				if strings.Contains(link.Description, "activation") ||
					strings.Contains(link.Description, "API activation") ||
					strings.Contains(link.URL, "/apis/api/") {
					return link.URL
				}
			}
		}
	}

	return ""
}

// IsServiceDisabledError checks if the error is a SERVICE_DISABLED error
func IsServiceDisabledError(body string) bool {
	var errResp ErrorResponse
	if err := json.Unmarshal([]byte(body), &errResp); err != nil {
		return false
	}

	// Check if it's a 403 PERMISSION_DENIED with SERVICE_DISABLED reason
	if errResp.Error.Code != 403 || errResp.Error.Status != "PERMISSION_DENIED" {
		return false
	}

	for _, detailRaw := range errResp.Error.Details {
		var info ErrorDetailInfo
		if err := json.Unmarshal(detailRaw, &info); err == nil {
			if info.Reason == "SERVICE_DISABLED" {
				return true
			}
		}
	}

	return false
}
