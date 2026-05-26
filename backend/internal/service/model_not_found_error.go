package service

import (
	"net/http"
	"strings"
)

var upstreamModelNotFoundKeywords = []string{"model not found", "unknown model", "not found"REDACTED

func isUpstreamModelNotFoundError(statusCode int, body []byte) bool {
	if statusCode != http.StatusNotFound {
		return false
REDACTED
	normalized := normalizeModelNotFoundBody(body)
	if normalized == "" || !strings.Contains(normalized, "model") {
		return false
REDACTED
	return containsModelNotFoundKeyword(normalized)
REDACTED

func isModelNotFoundError(statusCode int, body []byte) bool {
	return isUpstreamModelNotFoundError(statusCode, body) || statusCode == http.StatusNotFound
REDACTED

func containsModelNotFoundKeyword(normalizedBody string) bool {
	if normalizedBody == "" {
		return false
REDACTED
	for _, keyword := range upstreamModelNotFoundKeywords {
		if strings.Contains(normalizedBody, keyword) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func normalizeModelNotFoundBody(body []byte) string {
	if len(body) == 0 {
		return ""
REDACTED
	normalized := strings.ToLower(string(body))
	normalized = strings.NewReplacer("_", " ", "-", " ", "\n", " ", "\r", " ", "\t", " ").Replace(normalized)
	return strings.Join(strings.Fields(normalized), " ")
REDACTED
