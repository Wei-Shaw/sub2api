package httpx

import (
	"net/http"
	"net/url"
	"strings"
)

func ResolveFrameAncestorOrigin(r *http.Request) string {
	if r == nil {
		return ""
	}

	proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if proto == "" || host == "" {
		forwardedProto, forwardedHost := parseForwardedHeader(r.Header.Get("Forwarded"))
		if proto == "" {
			proto = forwardedProto
		}
		if host == "" {
			host = forwardedHost
		}
	}
	if host != "" {
		if proto == "" {
			proto = "http"
		}
		return strings.TrimRight(proto+"://"+host, "/")
	}
	if refererOrigin := parseOriginHeader(r.Header.Get("Referer")); refererOrigin != "" {
		return refererOrigin
	}
	if origin := parseOriginHeader(r.Header.Get("Origin")); origin != "" {
		return origin
	}
	return ""
}

func ResolveRequestBaseURL(r *http.Request) string {
	if r == nil {
		return ""
	}

	if origin := ResolveFrameAncestorOrigin(r); origin != "" {
		return origin
	}
	proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if proto == "" {
		proto = "http"
	}
	host := strings.TrimSpace(r.Host)
	if host == "" {
		return ""
	}
	return strings.TrimRight(proto+"://"+host, "/")
}

func parseForwardedHeader(raw string) (string, string) {
	for _, entry := range strings.Split(raw, ",") {
		var proto string
		var host string
		for _, part := range strings.Split(entry, ";") {
			key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
			if !ok {
				continue
			}
			value = strings.Trim(strings.TrimSpace(value), `"`)
			switch strings.ToLower(key) {
			case "proto":
				proto = value
			case "host":
				host = value
			}
		}
		if proto != "" || host != "" {
			return proto, host
		}
	}
	return "", ""
}

func parseOriginHeader(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return strings.TrimRight(parsed.Scheme+"://"+parsed.Host, "/")
}
