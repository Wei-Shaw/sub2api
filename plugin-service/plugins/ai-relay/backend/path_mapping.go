package backend

import (
	"net/url"
	"strings"
)

func canonicalRelayPath(value string) string {
	path := strings.Trim(strings.TrimSpace(value), "/")
	if strings.HasPrefix(path, "v1/") {
		path = strings.TrimPrefix(path, "v1/")
	}
	return path
}

func normalizePathMappings(input map[string]string) (map[string]string, error) {
	normalized := make(map[string]string, len(input))
	seenCanonicalSources := make(map[string]struct{}, len(input))
	for rawSource, rawTarget := range input {
		source := strings.Trim(strings.TrimSpace(rawSource), "/")
		trimmedTarget := strings.TrimSpace(rawTarget)
		if strings.HasPrefix(trimmedTarget, "//") {
			return nil, ErrInvalidRouteConfig
		}
		target := strings.Trim(trimmedTarget, "/")
		canonicalSource := canonicalRelayPath(source)
		if canonicalSource == "" || !validMappedTarget(target) {
			return nil, ErrInvalidRouteConfig
		}
		if _, exists := seenCanonicalSources[canonicalSource]; exists {
			return nil, ErrInvalidRouteConfig
		}
		seenCanonicalSources[canonicalSource] = struct{}{}
		normalized[source] = target
	}
	return normalized, nil
}

func validMappedTarget(target string) bool {
	if target == "" {
		return false
	}
	parsed, err := url.Parse(target)
	return err == nil && !parsed.IsAbs() && parsed.Host == "" && parsed.RawQuery == "" && parsed.Fragment == ""
}

func copyPathMappings(input map[string]string) map[string]string {
	result := make(map[string]string, len(input))
	for source, target := range input {
		result[source] = target
	}
	return result
}

func ResolveRouteEndpointURL(config RouteConfig, endpoint string) (string, error) {
	baseURL, err := url.Parse(strings.TrimRight(strings.TrimSpace(config.BaseURL), "/"))
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return "", ErrInvalidRouteConfig
	}
	canonicalEndpoint := canonicalRelayPath(endpoint)
	target, ok := config.PathMappings[canonicalEndpoint]
	if !ok {
		for source, candidate := range config.PathMappings {
			if canonicalRelayPath(source) == canonicalEndpoint {
				target, ok = candidate, true
				break
			}
		}
	}
	if ok {
		baseURL.Path = strings.TrimRight(baseURL.Path, "/") + "/" + strings.Trim(target, "/")
		baseURL.RawPath = ""
		baseURL.RawQuery = ""
		baseURL.Fragment = ""
		return baseURL.String(), nil
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + "/" + canonicalEndpoint
	baseURL.RawPath = ""
	baseURL.Fragment = ""
	return baseURL.String(), nil
}
