package service

import (
	"net/url"
	"strings"
)

func buildOpenAIEndpointURL(base string, endpoint string) string {
	normalized := strings.TrimRight(strings.TrimSpace(base), "/")
	endpoint = "/" + strings.TrimLeft(strings.TrimSpace(endpoint), "/")
	relative := strings.TrimPrefix(endpoint, "/v1")
	if strings.HasSuffix(normalized, endpoint) || strings.HasSuffix(normalized, relative) {
		return normalized
REDACTED
	if openAIBaseURLHasVersionSuffix(normalized) {
		return normalized + relative
REDACTED
	return normalized + endpoint
REDACTED

func buildOpenAIResponsesInputTokensURL(base string) string {
	return buildOpenAIEndpointURL(base, "/v1/responses/input_tokens")
REDACTED

func openAIBaseURLHasVersionSuffix(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
REDACTED

	pathValue := ""
	if parsed, err := url.Parse(trimmed); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		pathValue = parsed.Path
REDACTED else if slash := strings.Index(trimmed, "/"); slash >= 0 {
		pathValue = trimmed[slash:]
REDACTED

	pathValue = strings.TrimRight(pathValue, "/")
	if pathValue == "" {
		return false
REDACTED
	lastSlash := strings.LastIndex(pathValue, "/")
	segment := pathValue
	if lastSlash >= 0 {
		segment = pathValue[lastSlash+1:]
REDACTED
	return isOpenAIAPIVersionSegment(segment)
REDACTED

func isOpenAIAPIVersionSegment(segment string) bool {
	s := strings.ToLower(strings.TrimSpace(segment))
	if len(s) < 2 || s[0] != 'v' || !isASCIIDigit(s[1]) {
		return false
REDACTED

	i := 1
	for i < len(s) && isASCIIDigit(s[i]) {
		i++
REDACTED
	if i == len(s) {
		return true
REDACTED
	if s[i] == '.' {
		i++
		if i == len(s) || !isASCIIDigit(s[i]) {
			return false
	REDACTED
		for i < len(s) && isASCIIDigit(s[i]) {
			i++
	REDACTED
		return i == len(s)
REDACTED

	suffix := s[i:]
	return strings.HasPrefix(suffix, "alpha") ||
		strings.HasPrefix(suffix, "beta") ||
		strings.HasPrefix(suffix, "preview")
REDACTED

func isASCIIDigit(b byte) bool {
	return b >= '0' && b <= '9'
REDACTED
