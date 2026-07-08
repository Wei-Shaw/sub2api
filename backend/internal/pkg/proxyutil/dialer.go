// Package proxyutil 提供统一的代理配置功能
//
// 支持的代理协议：
//   - HTTP/HTTPS: 通过 Transport.Proxy 设置
//   - SOCKS5: 通过 Transport.DialContext 设置（客户端本地解析 DNS）
//   - SOCKS5H: 通过 Transport.DialContext 设置（代理端远程解析 DNS，推荐）
//
// 注意：proxyurl.Parse() 会自动将 socks5:// 升级为 socks5h://，
// 确保 DNS 也由代理端解析，防止 DNS 泄漏。
package proxyutil

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

// 建连阶段超时。此前 SOCKS5 隧道建立（TCP 连接 + SOCKS 协商）无任何兜底超时
// （请求 ctx 通常没有 deadline），代理黑洞时连接会无限挂起。这些超时只作用于
// 隧道建立阶段，不影响隧道建立后的流式读写（x/net/proxy 的 socks dialer 会在
// 协商结束后清除 conn deadline）。
const (
	// socksDialTimeout 约束到 SOCKS5 代理本身的 TCP 连接。
	socksDialTimeout = 15 * time.Second
	// socksKeepAlive TCP 保活探测间隔。
	socksKeepAlive = 30 * time.Second
	// tunnelEstablishTimeout 兜底约束整个隧道建立（TCP + SOCKS 协商）。
	tunnelEstablishTimeout = 30 * time.Second
)

type dialContextFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// withEstablishTimeout 为隧道建立补充兜底 deadline；调用方已有更短 deadline 时保持不变。
// cancel 在返回后立即执行是安全的：建连完成后 ctx 取消对已建立的连接无效。
func withEstablishTimeout(dial dialContextFunc) dialContextFunc {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		ctx, cancel := context.WithTimeout(ctx, tunnelEstablishTimeout)
		defer cancel()
		return dial(ctx, network, addr)
	}
}

// ConfigureTransportProxy 根据代理 URL 配置 Transport
//
// 支持的协议：
//   - http/https: 设置 transport.Proxy
//   - socks5: 设置 transport.DialContext（客户端本地解析 DNS）
//   - socks5h: 设置 transport.DialContext（代理端远程解析 DNS，推荐）
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
	}

	scheme := strings.ToLower(proxyURL.Scheme)
	switch scheme {
	case "http", "https":
		transport.Proxy = http.ProxyURL(proxyURL)
		return nil

	case "socks5", "socks5h":
		dialer, err := proxy.FromURL(proxyURL, &net.Dialer{Timeout: socksDialTimeout, KeepAlive: socksKeepAlive})
		if err != nil {
			return fmt.Errorf("create socks5 dialer: %w", err)
		}
		// 优先使用支持 context 的 DialContext，以支持请求取消和超时
		if contextDialer, ok := dialer.(proxy.ContextDialer); ok {
			transport.DialContext = withEstablishTimeout(contextDialer.DialContext)
		} else {
			// 回退路径：如果 dialer 不支持 ContextDialer，则包装为简单的 DialContext
			// 注意：此回退不支持请求取消和超时控制
			transport.DialContext = withEstablishTimeout(func(_ context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			})
		}
		return nil

	default:
		return fmt.Errorf("unsupported proxy scheme: %s", scheme)
	}
}
