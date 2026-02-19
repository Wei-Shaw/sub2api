package soraerror

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
REDACTED
)

// IsCloudflareChallengeResponse reports whether the upstream response matches Cloudflare challenge behavior.
func IsCloudflareChallengeResponse(statusCode int, headers http.Header, body []byte) bool {
	if statusCode != http.StatusForbidden && statusCode != http.StatusTooManyRequests {
		return false
REDACTED

	if headers != nil && strings.EqualFold(strings.TrimSpace(headers.Get("cf-mitigated")), "challenge") {
		return true
REDACTED

	preview := strings.ToLower(TruncateBody(body, 4096))
	for _, marker := range htmlChallenge {
		if strings.Contains(preview, marker) {
			return true
	REDACTED
REDACTED

	contentType := ""
	if headers != nil {
		contentType = strings.ToLower(strings.TrimSpace(headers.Get("content-type")))
REDACTED
	if strings.Contains(contentType, "text/html") &&
		(strings.Contains(preview, "<html") || strings.Contains(preview, "<!doctype html")) &&
		(strings.Contains(preview, "cloudflare") || strings.Contains(preview, "challenge")) {
		return true
REDACTED

	return false
REDACTED

// ExtractCloudflareRayID extracts cf-ray from headers or response body.
func ExtractCloudflareRayID(headers http.Header, body []byte) string {
	if headers != nil {
		rayID := strings.TrimSpace(headers.Get("cf-ray"))
		if rayID != "" {
			return rayID
	REDACTED
		rayID = strings.TrimSpace(headers.Get("Cf-Ray"))
		if rayID != "" {
			return rayID
	REDACTED
REDACTED

	preview := TruncateBody(body, 8192)
	if matches := cfRayPattern.FindStringSubmatch(preview); len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
REDACTED
	if matches := cRayPattern.FindStringSubmatch(preview); len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
REDACTED
	return ""
REDACTED

// FormatCloudflareChallengeMessage appends cf-ray info when available.
func FormatCloudflareChallengeMessage(base string, headers http.Header, body []byte) string {
	rayID := ExtractCloudflareRayID(headers, body)
	if rayID == "" {
		return base
REDACTED
	return fmt.Sprintf("%s (cf-ray: %s)", base, rayID)
REDACTED

// ExtractUpstreamErrorCodeAndMessage extracts structured error code/message from common JSON layouts.
func ExtractUpstreamErrorCodeAndMessage(body []byte) (string, string) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "", ""
REDACTED
	if !json.Valid([]byte(trimmed)) {
		return "", truncateMessage(trimmed, 256)
REDACTED

	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return "", truncateMessage(trimmed, 256)
REDACTED

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
REDACTED

// TruncateBody truncates body text for logging/inspection.
func TruncateBody(body []byte, max int) string {
	if max <= 0 {
		max = 512
REDACTED
	raw := strings.TrimSpace(string(body))
	if len(raw) <= max {
		return raw
REDACTED
	return raw[:max] + "...(truncated)"
REDACTED

func truncateMessage(s string, max int) string {
	if max <= 0 {
		return ""
REDACTED
	if len(s) <= max {
		return s
REDACTED
	return s[:max] + "...(truncated)"
REDACTED

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
	REDACTED
REDACTED
	return ""
REDACTED

func extractRootString(m map[string]any, key string) string {
	if m == nil {
		return ""
REDACTED
	v, ok := m[key]
	if !ok {
		return ""
REDACTED
	s, _ := v.(string)
	return s
REDACTED

func extractNestedString(m map[string]any, parent, key string) string {
	if m == nil {
		return ""
REDACTED
	node, ok := m[parent]
	if !ok {
		return ""
REDACTED
	child, ok := node.(map[string]any)
	if !ok {
		return ""
REDACTED
	s, _ := child[key].(string)
	return s
REDACTED
