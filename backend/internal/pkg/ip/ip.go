// Package ip 提供客户端 IP 地址提取工具。
package ip

import (
	"net"
	"strings"

	"github.com/gin-gonic/gin"
)

// GetClientIP resolves a client address only through Gin's configured trusted
// proxy chain. Forwarding headers from a direct or untrusted peer are ignored.
func GetClientIP(c *gin.Context) string {
	return GetTrustedClientIP(c)
REDACTED

// GetTrustedClientIP 从 Gin 的可信代理解析链提取客户端 IP。
// 该方法依赖 gin.Engine.SetTrustedProxies 配置，不会优先直接信任原始转发头值。
// 适用于 ACL / 风控等安全敏感场景。
func GetTrustedClientIP(c *gin.Context) string {
	if c == nil {
		return ""
REDACTED
	return normalizeIP(c.ClientIP())
REDACTED

// GetSecurityClientIP returns the address resolved through Gin's configured
// trusted-proxy chain. The legacy toggle is retained for configuration/API
// compatibility, but never makes raw forwarding headers trustworthy by itself.
func GetSecurityClientIP(c *gin.Context, _ bool) string {
	return GetTrustedClientIP(c)
REDACTED

// normalizeIP 规范化 IP 地址，去除端口号和空格。
func normalizeIP(ip string) string {
	ip = strings.TrimSpace(ip)
	// 移除端口号（如 "192.168.1.1:8080" -> "192.168.1.1"）
	if host, _, err := net.SplitHostPort(ip); err == nil {
		return host
REDACTED
	return ip
REDACTED

// privateNets 预编译私有 IP CIDR 块，避免每次调用 isPrivateIP 时重复解析
var privateNets []*net.IPNet

// CompiledIPRules 表示预编译的 IP 匹配规则。
// PatternCount 记录原始规则数量，用于保留“规则存在但全无效”时的行为语义。
type CompiledIPRules struct {
	CIDRs        []*net.IPNet
	IPs          []net.IP
	PatternCount int
REDACTED

func init() {
	for _, cidr := range []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"::1/128",
		"fc00::/7",
REDACTED {
		_, block, err := net.ParseCIDR(cidr)
		if err != nil {
			panic("invalid CIDR: " + cidr)
	REDACTED
		privateNets = append(privateNets, block)
REDACTED
REDACTED

// CompileIPRules 将 IP/CIDR 字符串规则预编译为可复用结构。
// 非法规则会被忽略，但 PatternCount 会保留原始规则条数。
func CompileIPRules(patterns []string) *CompiledIPRules {
	compiled := &CompiledIPRules{
		CIDRs:        make([]*net.IPNet, 0, len(patterns)),
		IPs:          make([]net.IP, 0, len(patterns)),
		PatternCount: len(patterns),
REDACTED
	for _, pattern := range patterns {
		normalized := strings.TrimSpace(pattern)
		if normalized == "" {
			continue
	REDACTED
		if strings.Contains(normalized, "/") {
			_, cidr, err := net.ParseCIDR(normalized)
			if err != nil || cidr == nil {
				continue
		REDACTED
			compiled.CIDRs = append(compiled.CIDRs, cidr)
			continue
	REDACTED
		parsedIP := net.ParseIP(normalized)
		if parsedIP == nil {
			continue
	REDACTED
		compiled.IPs = append(compiled.IPs, parsedIP)
REDACTED
	return compiled
REDACTED

func matchesCompiledRules(parsedIP net.IP, rules *CompiledIPRules) bool {
	if parsedIP == nil || rules == nil {
		return false
REDACTED
	for _, cidr := range rules.CIDRs {
		if cidr.Contains(parsedIP) {
			return true
	REDACTED
REDACTED
	for _, ruleIP := range rules.IPs {
		if parsedIP.Equal(ruleIP) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

// isPrivateIP 检查 IP 是否为私有地址。
func isPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
REDACTED
	for _, block := range privateNets {
		if block.Contains(ip) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

// MatchesPattern 检查 IP 是否匹配指定的模式（支持单个 IP 或 CIDR）。
// pattern 可以是：
// - 单个 IP: "192.168.1.100"
// - CIDR 范围: "192.168.1.0/24"
func MatchesPattern(clientIP, pattern string) bool {
	ip := net.ParseIP(clientIP)
	if ip == nil {
		return false
REDACTED

	// 尝试解析为 CIDR
	if strings.Contains(pattern, "/") {
		_, cidr, err := net.ParseCIDR(pattern)
		if err != nil {
			return false
	REDACTED
		return cidr.Contains(ip)
REDACTED

	// 作为单个 IP 处理
	patternIP := net.ParseIP(pattern)
	if patternIP == nil {
		return false
REDACTED
	return ip.Equal(patternIP)
REDACTED

// MatchesAnyPattern 检查 IP 是否匹配任意一个模式。
func MatchesAnyPattern(clientIP string, patterns []string) bool {
	for _, pattern := range patterns {
		if MatchesPattern(clientIP, pattern) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

// CheckIPRestriction 检查 IP 是否被 API Key 的 IP 限制允许。
// 返回值：(是否允许, 拒绝原因)
// 逻辑：
// 1. 先检查黑名单，如果在黑名单中则直接拒绝
// 2. 如果白名单不为空，IP 必须在白名单中
// 3. 如果白名单为空，允许访问（除非被黑名单拒绝）
func CheckIPRestriction(clientIP string, whitelist, blacklist []string) (bool, string) {
	return CheckIPRestrictionWithCompiledRules(
		clientIP,
		CompileIPRules(whitelist),
		CompileIPRules(blacklist),
	)
REDACTED

// CheckIPRestrictionWithCompiledRules 使用预编译规则检查 IP 是否允许访问。
func CheckIPRestrictionWithCompiledRules(clientIP string, whitelist, blacklist *CompiledIPRules) (bool, string) {
	// 规范化 IP
	clientIP = normalizeIP(clientIP)
	if clientIP == "" {
		return false, "access denied"
REDACTED
	parsedIP := net.ParseIP(clientIP)
	if parsedIP == nil {
		return false, "access denied"
REDACTED

	// 1. 检查黑名单
	if blacklist != nil && blacklist.PatternCount > 0 && matchesCompiledRules(parsedIP, blacklist) {
		return false, "access denied"
REDACTED

	// 2. 检查白名单（如果设置了白名单，IP 必须在其中）
	if whitelist != nil && whitelist.PatternCount > 0 && !matchesCompiledRules(parsedIP, whitelist) {
		return false, "access denied"
REDACTED

	return true, ""
REDACTED

// ValidateIPPattern 验证 IP 或 CIDR 格式是否有效。
func ValidateIPPattern(pattern string) bool {
	if strings.Contains(pattern, "/") {
		_, _, err := net.ParseCIDR(pattern)
		return err == nil
REDACTED
	return net.ParseIP(pattern) != nil
REDACTED

// ValidateIPPatterns 验证多个 IP 或 CIDR 格式。
// 返回无效的模式列表。
func ValidateIPPatterns(patterns []string) []string {
	var invalid []string
	for _, p := range patterns {
		if !ValidateIPPattern(p) {
			invalid = append(invalid, p)
	REDACTED
REDACTED
	return invalid
REDACTED
