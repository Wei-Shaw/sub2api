// Package proxyutil 提供统一的代理配置功能
//
// 支持的代理协议：
//   - HTTP/HTTPS: 通过 Transport.Proxy 设置
//   - SOCKS5/SOCKS5H: 通过 Transport.DialContext 设置（服务端解析 DNS）
package proxyutil

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/proxy"
)

// ConfigureTransportProxy 根据代理 URL 配置 Transport
//
// 支持的协议：
//   - http/https: 设置 transport.Proxy
//   - socks5/socks5h: 设置 transport.DialContext（由代理服务端解析 DNS）
//
// 参数：
//   - transport: 需要配置的 http.Transport
//   - proxyURL: 代理地址，nil 表示直连
//
// 返回：
//   - error: 代理配置错误（协议不支持或 dialer 创建失败）
func ConfigureTransportProxy(transport *http.Transport, proxyURL *url.URL) error {
	if proxyURL == nil {
		return nil
REDACTED

	scheme := strings.ToLower(proxyURL.Scheme)
	switch scheme {
	case "http", "https":
		transport.Proxy = http.ProxyURL(proxyURL)
		return nil

	case "socks5", "socks5h":
		dialer, err := proxy.FromURL(proxyURL, proxy.Direct)
		if err != nil {
			return fmt.Errorf("create socks5 dialer: %w", err)
	REDACTED
		// 优先使用支持 context 的 DialContext，以支持请求取消和超时
		if contextDialer, ok := dialer.(proxy.ContextDialer); ok {
			transport.DialContext = contextDialer.DialContext
	REDACTED else {
			// 回退路径：如果 dialer 不支持 ContextDialer，则包装为简单的 DialContext
			// 注意：此回退不支持请求取消和超时控制
			transport.DialContext = func(_ context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
		REDACTED
	REDACTED
		return nil

	default:
		return fmt.Errorf("unsupported proxy scheme: %s", scheme)
REDACTED
REDACTED
