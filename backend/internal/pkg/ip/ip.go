// Package ip 提供客户端 IP 地址提取工具。
package ip

import (
	"net"
	"strings"

	"github.com/gin-gonic/gin"
)

// GetClientIP 从 Gin Context 中提取客户端真实 IP 地址。
// 按以下优先级检查 Header：
// 1. CF-Connecting-IP (Cloudflare)
// 2. X-Real-IP (Nginx)
// 3. X-Forwarded-For (取第一个非私有 IP)
// 4. c.ClientIP() (Gin 内置方法)
func GetClientIP(c *gin.Context) string {
	// 1. Cloudflare
	if ip := c.GetHeader("CF-Connecting-IP"); ip != "" {
		return normalizeIP(ip)
REDACTED

	// 2. Nginx X-Real-IP
	if ip := c.GetHeader("X-Real-IP"); ip != "" {
		return normalizeIP(ip)
REDACTED

	// 3. X-Forwarded-For (多个 IP 时取第一个公网 IP)
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		for _, ip := range ips {
			ip = strings.TrimSpace(ip)
			if ip != "" && !isPrivateIP(ip) {
				return normalizeIP(ip)
		REDACTED
	REDACTED
		// 如果都是私有 IP，返回第一个
		if len(ips) > 0 {
			return normalizeIP(strings.TrimSpace(ips[0]))
	REDACTED
REDACTED

	// 4. Gin 内置方法
	return normalizeIP(c.ClientIP())
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
	// 规范化 IP
	clientIP = normalizeIP(clientIP)
	if clientIP == "" {
		return false, "access denied"
REDACTED

	// 1. 检查黑名单
	if len(blacklist) > 0 && MatchesAnyPattern(clientIP, blacklist) {
		return false, "access denied"
REDACTED

	// 2. 检查白名单（如果设置了白名单，IP 必须在其中）
	if len(whitelist) > 0 && !MatchesAnyPattern(clientIP, whitelist) {
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
