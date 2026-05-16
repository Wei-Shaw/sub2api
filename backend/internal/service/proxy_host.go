package service

import (
	"net"
	"strconv"
	"strings"
)

const (
	ProxyIPVersionIPv4 = "ipv4"
	ProxyIPVersionIPv6 = "ipv6"
)

// NormalizeProxyHost strips IPv6 brackets and surrounding whitespace so the
// persisted host value stays canonical across UI, import, and API paths.
func NormalizeProxyHost(host string) string {
	trimmed := strings.TrimSpace(host)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		inner := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
		if inner != "" && strings.Contains(inner, ":") {
			return inner
		}
	}
	return trimmed
}

// FormatProxyHostPort formats host:port for display and URL generation,
// automatically adding IPv6 brackets when needed.
func FormatProxyHostPort(host string, port int) string {
	return net.JoinHostPort(NormalizeProxyHost(host), strconv.Itoa(port))
}

func NormalizeProxyIPVersion(ipVersion string) string {
	switch strings.ToLower(strings.TrimSpace(ipVersion)) {
	case ProxyIPVersionIPv6:
		return ProxyIPVersionIPv6
	default:
		return ProxyIPVersionIPv4
	}
}
