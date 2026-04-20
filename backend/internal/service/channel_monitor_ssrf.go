package service

import (
	"context"
	"net"
	"strings"
)

// SSRF 防护 helper：
//   - validateEndpoint 在 admin 提交时阻止 http/loopback/私网/云元数据 URL
//   - safeDialContext 在 socket 层再次校验真实 IP，防止 DNS rebinding
//
// 已知 cloud metadata hostname 拒绝列表（小写比较）。
var monitorBlockedHostnames = map[string]struct{REDACTED{
	"localhost":                  {REDACTED,
	"localhost.localdomain":      {REDACTED,
	"metadata":                   {REDACTED,
	"metadata.google.internal":   {REDACTED,
	"metadata.goog":              {REDACTED,
	"instance-data":              {REDACTED,
	"instance-data.ec2.internal": {REDACTED,
REDACTED

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
REDACTED)

// monitorDialer 共享 Dialer，与 net/http 默认值对齐。
var monitorDialer = &net.Dialer{
	Timeout:   monitorDialTimeout,
	KeepAlive: monitorDialKeepAlive,
REDACTED

// mustParseCIDRs 在包初始化时解析 CIDR 字符串，失败 panic。
func mustParseCIDRs(cidrs []string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic("channel_monitor_ssrf: invalid CIDR " + c + ": " + err.Error())
	REDACTED
		out = append(out, n)
REDACTED
	return out
REDACTED

// isBlockedHostname 判断 hostname 是否命中黑名单。
func isBlockedHostname(hostname string) bool {
	if hostname == "" {
		return true
REDACTED
	_, blocked := monitorBlockedHostnames[strings.ToLower(hostname)]
	return blocked
REDACTED

// isPrivateIP 判断 IP 是否落在禁止段（loopback/RFC1918/link-local/ULA 等）。
func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return true
REDACTED
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() {
		return true
REDACTED
	for _, n := range monitorBlockedCIDRs {
		if n.Contains(ip) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

// isPrivateOrLoopbackHost 解析 hostname 的所有 A/AAAA 记录，
// 任一 IP 落在私网/loopback 段即认为不安全。
//
// hostname 是 IP 字面量时也走同一路径。
func isPrivateOrLoopbackHost(ctx context.Context, hostname string) (bool, error) {
	if isBlockedHostname(hostname) {
		return true, nil
REDACTED
	// IP 字面量直接判断。
	if ip := net.ParseIP(hostname); ip != nil {
		return isPrivateIP(ip), nil
REDACTED
	resolver := net.DefaultResolver
	addrs, err := resolver.LookupIPAddr(ctx, hostname)
	if err != nil {
		return false, err
REDACTED
	if len(addrs) == 0 {
		return true, nil
REDACTED
	for _, a := range addrs {
		if isPrivateIP(a.IP) {
			return true, nil
	REDACTED
REDACTED
	return false, nil
REDACTED

// safeDialContext 在真实 dial 前再次校验目标 IP，防止 DNS rebinding。
// 解析 hostname 后逐个 IP 尝试连接，命中私网即拒绝（即便 validateEndpoint 时返回的是公网 IP）。
func safeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
REDACTED
	// 字面量 IP 走快速路径。
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return nil, &net.AddrError{Err: "blocked by SSRF policy", Addr: addressREDACTED
	REDACTED
		return monitorDialer.DialContext(ctx, network, address)
REDACTED
	if isBlockedHostname(host) {
		return nil, &net.AddrError{Err: "blocked by SSRF policy", Addr: addressREDACTED
REDACTED
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
REDACTED
	if len(addrs) == 0 {
		return nil, &net.AddrError{Err: "no addresses for host", Addr: hostREDACTED
REDACTED
	var lastErr error
	for _, a := range addrs {
		if isPrivateIP(a.IP) {
			lastErr = &net.AddrError{Err: "blocked by SSRF policy", Addr: a.IP.String()REDACTED
			continue
	REDACTED
		conn, err := monitorDialer.DialContext(ctx, network, net.JoinHostPort(a.IP.String(), port))
		if err == nil {
			return conn, nil
	REDACTED
		lastErr = err
REDACTED
	if lastErr == nil {
		lastErr = &net.AddrError{Err: "no usable addresses", Addr: hostREDACTED
REDACTED
	return nil, lastErr
REDACTED
