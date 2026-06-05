// Package proxyurl 提供代理 URL 的统一验证（fail-fast，无效代理不回退直连）
//
// 所有需要解析代理 URL 的地方必须通过此包的 Parse 函数。
// 直接使用 url.Parse 处理代理 URL 是被禁止的。
// 这确保了 fail-fast 行为：无效代理配置在创建时立即失败，
// 而不是在运行时静默回退到直连（产生 IP 关联风险）。
package proxyurl

import (
	"fmt"
	"net/url"
	"strings"
)

// allowedSchemes 代理协议白名单
var allowedSchemes = map[string]bool{
	"http":    true,
	"https":   true,
	"socks5":  true,
	"socks5h": true,
}

// Parse 解析并验证代理 URL。
//
// 语义:
//   - 空字符串 → ("", nil, nil)，表示直连
//   - 非空且有效 → (trimmed, *url.URL, nil)
//   - 非空但无效 → ("", nil, error)，fail-fast 不回退
//
// 验证规则:
//   - TrimSpace 后为空视为直连
//   - url.Parse 失败返回 error（不含原始 URL，防凭据泄露）
//   - Host 为空返回 error（用 Redacted() 脱敏）
//   - Scheme 必须为 http/https/socks5/socks5h
//   - socks5:// 自动升级为 socks5h://（确保 DNS 由代理端解析，防止 DNS 泄漏）
func Parse(raw string) (trimmed string, parsed *url.URL, err error) {
	trimmed = strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil, nil
	}

	parsed, err = url.Parse(trimmed)
	if err != nil {
		// 不使用 %w 包装，避免 url.Parse 的底层错误消息泄漏原始 URL（可能含凭据）
		return "", nil, fmt.Errorf("invalid proxy URL: %v", err)
	}

	if parsed.Host == "" || parsed.Hostname() == "" {
		return "", nil, fmt.Errorf("proxy URL missing host: %s", parsed.Redacted())
	}

	scheme := strings.ToLower(parsed.Scheme)
	if !allowedSchemes[scheme] {
		return "", nil, fmt.Errorf("unsupported proxy scheme %q (allowed: http, https, socks5, socks5h)", scheme)
	}

	// 自动升级 socks5 → socks5h，确保 DNS 由代理端解析，防止 DNS 泄漏。
	// Go 的 golang.org/x/net/proxy 对 socks5:// 默认在客户端本地解析 DNS，
	// 仅 socks5h:// 才将域名发送给代理端做远程 DNS 解析。
	if scheme == "socks5" {
		parsed.Scheme = "socks5h"
		trimmed = parsed.String()
	}

	return trimmed, parsed, nil
}
