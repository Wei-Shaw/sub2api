package tlsfingerprint

import (
	"net"
	"strings"

	utls "github.com/refraction-networking/utls"
)

// SupportsHTTP2 返回模板是否会通过 ALPN 声明 h2。
func SupportsHTTP2(profile *Profile) bool {
	if !profileHasALPNExtension(profile) {
		return false
	}
	for _, protocol := range effectiveALPNProtocols(profile) {
		if strings.EqualFold(strings.TrimSpace(protocol), "h2") {
			return true
		}
	}
	return false
}

// HTTP1OnlyProfile 返回移除 h2 ALPN 后的模板副本，用于显式 HTTP/1.1 回退。
func HTTP1OnlyProfile(profile *Profile) *Profile {
	if profile == nil || !SupportsHTTP2(profile) {
		return profile
	}
	cloned := *profile
	protocols := make([]string, 0, len(effectiveALPNProtocols(profile)))
	hasHTTP1 := false
	for _, protocol := range effectiveALPNProtocols(profile) {
		trimmed := strings.TrimSpace(protocol)
		if trimmed == "" || strings.EqualFold(trimmed, "h2") {
			continue
		}
		if strings.EqualFold(trimmed, "http/1.1") {
			hasHTTP1 = true
		}
		protocols = append(protocols, protocol)
	}
	if !hasHTTP1 {
		protocols = append(protocols, "http/1.1")
	}
	cloned.ALPNProtocols = protocols
	return &cloned
}

// NegotiatedProtocol 从 uTLS 连接中取出实际协商到的 ALPN 协议。
func NegotiatedProtocol(conn net.Conn) string {
	if conn == nil {
		return ""
	}
	type negotiatedProtocolConn interface {
		ConnectionState() utls.ConnectionState
	}
	if stateConn, ok := conn.(negotiatedProtocolConn); ok {
		return strings.TrimSpace(stateConn.ConnectionState().NegotiatedProtocol)
	}
	return ""
}

func effectiveALPNProtocols(profile *Profile) []string {
	if profile != nil && len(profile.ALPNProtocols) > 0 {
		return profile.ALPNProtocols
	}
	return []string{"http/1.1"}
}

func profileHasALPNExtension(profile *Profile) bool {
	if profile == nil || len(profile.Extensions) == 0 {
		return true
	}
	for _, extensionID := range profile.Extensions {
		if extensionID == 16 {
			return true
		}
	}
	return false
}
