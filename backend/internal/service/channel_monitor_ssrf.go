package service

import (
	"context"
	"net"
	"strings"
)

// SSRF 防护 helper：
//   - validateEndpoint 在 admin 提交时阻止公网 http/私网/云元数据 URL
//   - safeDialContext 在 socket 层再次校验真实 IP，防止 DNS rebinding
//   - localDialContext 仅允许显式 localhost/loopback，供本机调试监控使用
//
// 已知 cloud metadata hostname 拒绝列表（小写比较）。
var monitorBlockedHostnames = map[string]struct{}{
	"localhost":                  {},
	"localhost.localdomain":      {},
	"metadata":                   {},
	"metadata.google.internal":   {},
	"metadata.goog":              {},
	"instance-data":              {},
	"instance-data.ec2.internal": {},
}

// CIDR 列表：包含所有需要拒绝的 IPv4/IPv6 段。
// 解析时只 panic 一次（启动时确认），生产路径只做 Contains。
var monitorBlockedCIDRs = mustParseCIDRs([]string{
	"127.0.0.0/8",    // IPv4 loopback
	"10.0.0.0/8",     // RFC1918
	"172.16.0.0/12",  // RFC1918
	"192.168.0.0/16", // RFC1918
	"169.254.0.0/16", // link-local（含云元数据 169.254.169.254）
	"100.64.0.0/10",  // CGNAT
	"0.0.0.0/8",      // "this network"
	"::1/128",        // IPv6 loopback
	"fc00::/7",       // IPv6 ULA
	"fe80::/10",      // IPv6 link-local
	"::/128",         // IPv6 unspecified
})

// monitorDialer 共享 Dialer，与 net/http 默认值对齐。
var monitorDialer = &net.Dialer{
	Timeout:   monitorDialTimeout,
	KeepAlive: monitorDialKeepAlive,
}

// mustParseCIDRs 在包初始化时解析 CIDR 字符串，失败 panic。
func mustParseCIDRs(cidrs []string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic("channel_monitor_ssrf: invalid CIDR " + c + ": " + err.Error())
		}
		out = append(out, n)
	}
	return out
}

// isBlockedHostname 判断 hostname 是否命中黑名单。
func isBlockedHostname(hostname string) bool {
	if hostname == "" {
		return true
	}
	_, blocked := monitorBlockedHostnames[strings.ToLower(hostname)]
	return blocked
}

// isLocalMonitorHostname 只放行显式本机名称；不通过 DNS 把任意 hostname 判成 local，
// 避免外部域名临时解析到 127.0.0.1 后绕过公网 SSRF 策略。
func isLocalMonitorHostname(hostname string) bool {
	hostname = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(hostname)), ".")
	if hostname == "" {
		return false
	}
	return hostname == "localhost" ||
		hostname == "localhost.localdomain" ||
		strings.HasSuffix(hostname, ".localhost")
}

// isPrivateIP 判断 IP 是否落在禁止段（loopback/RFC1918/link-local/ULA 等）。
func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() {
		return true
	}
	for _, n := range monitorBlockedCIDRs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func isLoopbackIP(ip net.IP) bool {
	return ip != nil && ip.IsLoopback()
}

func isExplicitLocalMonitorHost(hostname string) bool {
	if ip := net.ParseIP(hostname); ip != nil {
		return isLoopbackIP(ip)
	}
	return isLocalMonitorHostname(hostname)
}

// isPrivateOrLoopbackHost 解析 hostname 的所有 A/AAAA 记录，
// 任一 IP 落在私网/loopback 段即认为不安全。
//
// hostname 是 IP 字面量时也走同一路径。
func isPrivateOrLoopbackHost(ctx context.Context, hostname string) (bool, error) {
	if isBlockedHostname(hostname) {
		return true, nil
	}
	// IP 字面量直接判断。
	if ip := net.ParseIP(hostname); ip != nil {
		return isPrivateIP(ip), nil
	}
	resolver := net.DefaultResolver
	addrs, err := resolver.LookupIPAddr(ctx, hostname)
	if err != nil {
		return false, err
	}
	if len(addrs) == 0 {
		return true, nil
	}
	for _, a := range addrs {
		if isPrivateIP(a.IP) {
			return true, nil
		}
	}
	return false, nil
}

// safeDialContext 在真实 dial 前再次校验目标 IP，防止 DNS rebinding。
// 解析 hostname 后逐个 IP 尝试连接，命中私网即拒绝（即便 validateEndpoint 时返回的是公网 IP）。
func safeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	// 字面量 IP 走快速路径。
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return nil, &net.AddrError{Err: "blocked by SSRF policy", Addr: address}
		}
		return monitorDialer.DialContext(ctx, network, address)
	}
	if isBlockedHostname(host) {
		return nil, &net.AddrError{Err: "blocked by SSRF policy", Addr: address}
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, &net.AddrError{Err: "no addresses for host", Addr: host}
	}
	var lastErr error
	for _, a := range addrs {
		if isPrivateIP(a.IP) {
			lastErr = &net.AddrError{Err: "blocked by SSRF policy", Addr: a.IP.String()}
			continue
		}
		conn, err := monitorDialer.DialContext(ctx, network, net.JoinHostPort(a.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = &net.AddrError{Err: "no usable addresses", Addr: host}
	}
	return nil, lastErr
}

// localDialContext 只连接显式本机 endpoint。它与 safeDialContext 分离，
// 让本地调试可用，同时避免 localhost redirect 到公网/内网其它地址。
func localDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if ip := net.ParseIP(host); ip != nil {
		if !isLoopbackIP(ip) {
			return nil, &net.AddrError{Err: "blocked by local monitor policy", Addr: address}
		}
		return monitorDialer.DialContext(ctx, network, address)
	}
	if !isLocalMonitorHostname(host) {
		return nil, &net.AddrError{Err: "blocked by local monitor policy", Addr: address}
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, a := range addrs {
		if !isLoopbackIP(a.IP) {
			lastErr = &net.AddrError{Err: "blocked by local monitor policy", Addr: a.IP.String()}
			continue
		}
		conn, err := monitorDialer.DialContext(ctx, network, net.JoinHostPort(a.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = &net.AddrError{Err: "no usable local addresses", Addr: host}
	}
	return nil, lastErr
}
