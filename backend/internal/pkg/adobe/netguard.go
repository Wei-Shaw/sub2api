package adobe

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"syscall"

	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

// ErrBlockedDestination 表示出站目标被安全策略拒绝（私网、保留地址等）。
var ErrBlockedDestination = fmt.Errorf("destination is not allowed")

// blockedPrefixes 是标准库判定之外、仍不可作为出站目标的网段。
var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),       // 本网络
	netip.MustParsePrefix("100.64.0.0/10"),   // 运营商级 NAT
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF 协议分配
	netip.MustParsePrefix("192.0.2.0/24"),    // 文档 TEST-NET-1
	netip.MustParsePrefix("198.18.0.0/15"),   // 基准测试
	netip.MustParsePrefix("198.51.100.0/24"), // 文档 TEST-NET-2
	netip.MustParsePrefix("203.0.113.0/24"),  // 文档 TEST-NET-3
	netip.MustParsePrefix("240.0.0.0/4"),     // 保留（含广播地址）
	netip.MustParsePrefix("64:ff9b::/96"),    // NAT64，可映射到内网 IPv4
	netip.MustParsePrefix("64:ff9b:1::/48"),  // 本地 NAT64
	netip.MustParsePrefix("100::/64"),        // 丢弃前缀
	netip.MustParsePrefix("2001::/32"),       // Teredo
	netip.MustParsePrefix("2001:db8::/32"),   // 文档
	netip.MustParsePrefix("2002::/16"),       // 6to4，可封装任意 IPv4
}

// IsPublicUnicastIP 判定 IP 是否为可对外路由的单播地址。
// 环回、私网、链路本地、组播、未指定地址及 blockedPrefixes 一律拒绝。
func IsPublicUnicastIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	if addr.IsUnspecified() || addr.IsLoopback() || addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() ||
		addr.IsInterfaceLocalMulticast() || addr.IsMulticast() {
		return false
	}
	for _, prefix := range blockedPrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

// publicDialControl 在建立连接前校验实际要连的 IP，挡住 DNS rebinding 与重定向到内网。
func publicDialControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return ErrBlockedDestination
	}
	ip := net.ParseIP(host)
	if ip == nil || !IsPublicUnicastIP(ip) {
		return fmt.Errorf("%w: %s", ErrBlockedDestination, host)
	}
	return nil
}

// downloadResolver 解析下载主机名；单测替换它来模拟解析到内网的结果。
var downloadResolver = func(ctx context.Context, host string) ([]net.IPAddr, error) {
	return net.DefaultResolver.LookupIPAddr(ctx, host)
}

// checkResolvedPublicHost 在经代理下载前本地解析主机名，任一地址不是公网单播即拒绝。
//
// 这是尽力而为的校验：真正的解析发生在代理侧，结果可能不同（split DNS、rebinding）。
// 本地解析失败时放行——那种主机名只在代理所在网络可达，与本机内网无关。
func checkResolvedPublicHost(ctx context.Context, host string) error {
	if ip := net.ParseIP(host); ip != nil {
		if !IsPublicUnicastIP(ip) {
			return fmt.Errorf("%w: %s", ErrBlockedDestination, host)
		}
		return nil
	}
	addrs, err := downloadResolver(ctx, host)
	if err != nil {
		return nil
	}
	for _, addr := range addrs {
		if !IsPublicUnicastIP(addr.IP) {
			return fmt.Errorf("%w: %s resolves to %s", ErrBlockedDestination, host, addr.IP)
		}
	}
	return nil
}

// validateAPIURL 校验要携带账号 token 的 Adobe API 地址（轮询链接等来自上游响应）：
// 只接受 https 且主机为 adobe.io 或其子域，避免把 Bearer token 发往第三方。
func validateAPIURL(rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return NewRequestError("adobe api url is malformed")
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return NewRequestError(fmt.Sprintf("adobe api url must be https, got %q", parsed.Scheme))
	}
	if !isAdobeIOHost(parsed.Hostname()) {
		return NewRequestError(fmt.Sprintf("adobe api url host %q is not an adobe.io host", parsed.Hostname()))
	}
	return nil
}

func isAdobeIOHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	return host == "adobe.io" || strings.HasSuffix(host, ".adobe.io")
}

// validateDownloadURL 校验产物直链：只接受 https 与域名主机，拒绝 IP 字面量与 localhost。
// 直链由上游给出，不带 token，但下载结果会原样交给调用方，必须挡住内网探测。
func validateDownloadURL(rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return NewRequestError("media url is malformed")
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return NewRequestError(fmt.Sprintf("media url must be https, got %q", parsed.Scheme))
	}
	host := strings.TrimSuffix(parsed.Hostname(), ".")
	if host == "" || net.ParseIP(host) != nil || strings.Contains(host, "%") || urlvalidator.IsBlockedHost(host) {
		return NewRequestError(fmt.Sprintf("media url host %q is not allowed", host))
	}
	return nil
}
