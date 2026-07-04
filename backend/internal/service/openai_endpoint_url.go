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
	}
	if openAIBaseURLHasVersionSuffix(normalized) || openAIBaseURLHasVersionedOpenAISuffix(normalized) {
		return normalized + relative
	}
	return normalized + endpoint
}

func buildOpenAIResponsesInputTokensURL(base string) string {
	return buildOpenAIEndpointURL(base, "/v1/responses/input_tokens")
}

func openAIBaseURLHasVersionSuffix(raw string) bool {
	pathValue := openAIBaseURLPath(raw)
	if pathValue == "" {
		return false
	}
	segments := strings.Split(strings.Trim(pathValue, "/"), "/")
	if len(segments) == 0 {
		return false
	}
	return isOpenAIAPIVersionSegment(segments[len(segments)-1])
}

func openAIBaseURLHasVersionedOpenAISuffix(raw string) bool {
	pathValue := openAIBaseURLPath(raw)
	if pathValue == "" {
		return false
	}
	segments := strings.Split(strings.Trim(pathValue, "/"), "/")
	if len(segments) < 2 {
		return false
	}
	last := strings.ToLower(strings.TrimSpace(segments[len(segments)-1]))
	if last != "openai" {
		return false
	}
	return isOpenAIAPIVersionSegment(segments[len(segments)-2])
}

func openAIBaseURLPath(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	if parsed, err := url.Parse(trimmed); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return strings.TrimRight(parsed.EscapedPath(), "/")
	}
	if slash := strings.Index(trimmed, "/"); slash >= 0 {
		return strings.TrimRight(trimmed[slash:], "/")
	}
	return ""
}

func isOpenAIAPIVersionSegment(segment string) bool {
	s := strings.ToLower(strings.TrimSpace(segment))
	if len(s) < 2 || s[0] != 'v' || !isASCIIDigit(s[1]) {
		return false
	}

	i := 1
	for i < len(s) && isASCIIDigit(s[i]) {
		i++
	}
	if i == len(s) {
		return true
	}
	if s[i] == '.' {
		i++
		if i == len(s) || !isASCIIDigit(s[i]) {
			return false
		}
		for i < len(s) && isASCIIDigit(s[i]) {
			i++
		}
		return i == len(s)
	}

	suffix := s[i:]
	return strings.HasPrefix(suffix, "alpha") ||
		strings.HasPrefix(suffix, "beta") ||
		strings.HasPrefix(suffix, "preview")
}

func isASCIIDigit(b byte) bool {
	return b >= '0' && b <= '9'
}
