package httputil

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

var (
	cfRayPattern  = regexp.MustCompile(`(?i)cf-ray[:\s=]+([a-z0-9-]+)`)
	cRayPattern   = regexp.MustCompile(`(?i)cRay:\s*'([a-z0-9-]+)'`)
	htmlChallenge = []string{
		"window._cf_chl_opt",
		"just a moment",
		"enable javascript and cookies to continue",
		"__cf_chl_",
		"challenge-platform",
	}
)

// IsCloudflareChallengeResponse reports whether the upstream response matches Cloudflare challenge behavior.
func IsCloudflareChallengeResponse(statusCode int, headers http.Header, body []byte) bool {
	if statusCode != http.StatusForbidden && statusCode != http.StatusTooManyRequests {
		return false
	}

	if headers != nil && strings.EqualFold(strings.TrimSpace(headers.Get("cf-mitigated")), "challenge") {
		return true
	}

	preview := strings.ToLower(TruncateBody(body, 4096))
	for _, marker := range htmlChallenge {
		if strings.Contains(preview, marker) {
			return true
		}
	}

	contentType := ""
	if headers != nil {
		contentType = strings.ToLower(strings.TrimSpace(headers.Get("content-type")))
	}
	if strings.Contains(contentType, "text/html") &&
		(strings.Contains(preview, "<html") || strings.Contains(preview, "<!doctype html")) &&
		(strings.Contains(preview, "cloudflare") || strings.Contains(preview, "challenge")) {
		return true
	}

	return false
}

// ExtractCloudflareRayID extracts cf-ray from headers or response body.
func ExtractCloudflareRayID(headers http.Header, body []byte) string {
	if headers != nil {
		rayID := strings.TrimSpace(headers.Get("cf-ray"))
		if rayID != "" {
			return rayID
		}
		rayID = strings.TrimSpace(headers.Get("Cf-Ray"))
		if rayID != "" {
			return rayID
		}
	}

	preview := TruncateBody(body, 8192)
	if matches := cfRayPattern.FindStringSubmatch(preview); len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	if matches := cRayPattern.FindStringSubmatch(preview); len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// FormatCloudflareChallengeMessage appends cf-ray info when available.
func FormatCloudflareChallengeMessage(base string, headers http.Header, body []byte) string {
	rayID := ExtractCloudflareRayID(headers, body)
	if rayID == "" {
		return base
	}
	return fmt.Sprintf("%s (cf-ray: %s)", base, rayID)
}

// ExtractUpstreamErrorCodeAndMessage extracts structured error code/message from common JSON layouts.
func ExtractUpstreamErrorCodeAndMessage(body []byte) (string, string) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "", ""
	}
	if !json.Valid([]byte(trimmed)) {
		return "", truncateMessage(trimmed, 256)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return "", truncateMessage(trimmed, 256)
	}

	code := firstNonEmpty(
		extractNestedString(payload, "error", "code"),
		extractRootString(payload, "code"),
	)
	message := firstNonEmpty(
		extractNestedString(payload, "error", "message"),
		extractRootString(payload, "message"),
		extractNestedString(payload, "error", "detail"),
		extractRootString(payload, "detail"),
	)
	return strings.TrimSpace(code), truncateMessage(strings.TrimSpace(message), 512)
}

// TruncateBody truncates body text for logging/inspection.
func TruncateBody(body []byte, max int) string {
	if max <= 0 {
		max = 512
	}
	raw := strings.TrimSpace(string(body))
	if len(raw) <= max {
		return raw
	}
	return raw[:max] + "...(truncated)"
}

func truncateMessage(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func extractRootString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

func extractNestedString(m map[string]any, parent, key string) string {
	if m == nil {
		return ""
	}
	node, ok := m[parent]
	if !ok {
		return ""
	}
	child, ok := node.(map[string]any)
	if !ok {
		return ""
	}
	s, _ := child[key].(string)
	return s
}
