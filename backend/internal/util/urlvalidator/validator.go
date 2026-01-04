package urlvalidator

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

type ValidationOptions struct {
	AllowedHosts     []string
	RequireAllowlist bool
	AllowPrivate     bool
REDACTED

func ValidateHTTPSURL(raw string, opts ValidationOptions) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("url is required")
REDACTED

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid url: %s", trimmed)
REDACTED
	if !strings.EqualFold(parsed.Scheme, "https") {
		return "", fmt.Errorf("invalid url scheme: %s", parsed.Scheme)
REDACTED

	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return "", errors.New("invalid host")
REDACTED
	if !opts.AllowPrivate && isBlockedHost(host) {
		return "", fmt.Errorf("host is not allowed: %s", host)
REDACTED

	allowlist := normalizeAllowlist(opts.AllowedHosts)
	if opts.RequireAllowlist && len(allowlist) == 0 {
		return "", errors.New("allowlist is not configured")
REDACTED
	if len(allowlist) > 0 && !isAllowedHost(host, allowlist) {
		return "", fmt.Errorf("host is not allowed: %s", host)
REDACTED

	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/"), nil
REDACTED

// ValidateResolvedIP 验证 DNS 解析后的 IP 地址是否安全
// 用于防止 DNS Rebinding 攻击：在实际 HTTP 请求时调用此函数验证解析后的 IP
func ValidateResolvedIP(host string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return fmt.Errorf("dns resolution failed: %w", err)
REDACTED

	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
			ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return fmt.Errorf("resolved ip %s is not allowed", ip.String())
	REDACTED
REDACTED
	return nil
REDACTED

func normalizeAllowlist(values []string) []string {
	if len(values) == 0 {
		return nil
REDACTED
	normalized := make([]string, 0, len(values))
	for _, v := range values {
		entry := strings.ToLower(strings.TrimSpace(v))
		if entry == "" {
			continue
	REDACTED
		if host, _, err := net.SplitHostPort(entry); err == nil {
			entry = host
	REDACTED
		normalized = append(normalized, entry)
REDACTED
	return normalized
REDACTED

func isAllowedHost(host string, allowlist []string) bool {
	for _, entry := range allowlist {
		if entry == "" {
			continue
	REDACTED
		if strings.HasPrefix(entry, "*.") {
			suffix := strings.TrimPrefix(entry, "*.")
			if host == suffix || strings.HasSuffix(host, "."+suffix) {
				return true
		REDACTED
			continue
	REDACTED
		if host == entry {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func isBlockedHost(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
REDACTED
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return true
	REDACTED
REDACTED
	return false
REDACTED
