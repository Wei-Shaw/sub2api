// Package proxyutil 提供统一的代理配置功能
//
// 支持的代理协议：
//   - HTTP/HTTPS: 通过 Transport.Proxy 设置
//   - SOCKS5: 通过 Transport.DialContext 设置（客户端本地解析 DNS）
//   - SOCKS5H: 通过 Transport.DialContext 设置（代理端远程解析 DNS，推荐）
//
// 注意：proxyurl.Parse() 会自动将 socks5:// 升级为 socks5h://，
// 默认由代理端解析 DNS；显式 tcp6 策略例外：本地仅解析 AAAA 并提交 IPv6 字面量。
package proxyutil

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// socks5DialTimeout 限制代理建连、DNS 与隧道握手的总耗时。
	socks5DialTimeout = 10 * time.Second
	// socks5DialKeepAlive 与 Go 默认 keepalive 探测间隔保持一致。
	socks5DialKeepAlive = 30 * time.Second
)

// socks5ForwardDialer 是 SOCKS5 dialer 的底层拨号器。
//
// proxy.FromURL 的默认 forward dialer 是 proxy.Direct（零值 net.Dialer，无超时），
// 代理地址不可达时会一直卡到内核 TCP 重传耗尽（Linux 约 130 秒）。SOCKS5 分支会
// 覆盖 Transport.DialContext，因此调用方在 Transport 上设置的建连超时对这条路径
// 无效，必须在这里补上。
var socks5ForwardDialer = &net.Dialer{
	Timeout:   socks5DialTimeout,
	KeepAlive: socks5DialKeepAlive,
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
		transport.DialContext = (&TunnelDialer{ProxyURL: proxyURL}).DialContext
		return nil

	default:
		return fmt.Errorf("unsupported proxy scheme: %s", scheme)
	}
}
