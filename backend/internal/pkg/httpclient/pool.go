// Package httpclient 提供共享 HTTP 客户端池
//
// 性能优化说明：
// 原实现在多个服务中重复创建 http.Client：
// 1. proxy_probe_service.go: 每次探测创建新客户端
// 2. pricing_service.go: 每次请求创建新客户端
// 3. turnstile_service.go: 每次验证创建新客户端
// 4. github_release_service.go: 每次请求创建新客户端
// 5. claude_usage_service.go: 每次请求创建新客户端
//
// 新实现使用统一的客户端池：
// 1. 相同配置复用同一 http.Client 实例
// 2. 复用 Transport 连接池，减少 TCP/TLS 握手开销
// 3. 支持 HTTP/HTTPS/SOCKS5/SOCKS5H 代理
// 4. 代理配置失败时直接返回错误，不会回退到直连（避免 IP 关联风险）
package httpclient

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyutil"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

// Transport 连接池默认配置
const (
	defaultMaxIdleConns        = 100              // 最大空闲连接数
	defaultMaxIdleConnsPerHost = 10               // 每个主机最大空闲连接数
	defaultIdleConnTimeout     = 90 * time.Second // 空闲连接超时时间（建议小于上游 LB 超时）
	defaultDialTimeout         = 5 * time.Second  // TCP 连接超时（含代理握手），代理不通时快速失败
	defaultTLSHandshakeTimeout = 5 * time.Second  // TLS 握手超时
	validatedHostTTL           = 30 * time.Second // DNS Rebinding 校验缓存 TTL
)

// Options 定义共享 HTTP 客户端的构建参数
type Options struct {
	ProxyURL              string        // 代理 URL（支持 http/https/socks5/socks5h）
	Timeout               time.Duration // 请求总超时时间
	ResponseHeaderTimeout time.Duration // 等待响应头超时时间
	InsecureSkipVerify    bool          // 是否跳过 TLS 证书验证（已禁用，不允许设置为 true）
	ValidateResolvedIP    bool          // 是否校验解析后的 IP（防止 DNS Rebinding）
	AllowPrivateHosts     bool          // 允许私有地址解析（与 ValidateResolvedIP 一起使用）

	// 可选的连接池参数（不设置则使用默认值）
	MaxIdleConns        int // 最大空闲连接总数（默认 100）
	MaxIdleConnsPerHost int // 每主机最大空闲连接（默认 10）
	MaxConnsPerHost     int // 每主机最大连接数（默认 0 无限制）
}

// sharedClients 存储按配置参数缓存的 http.Client 实例
var sharedClients sync.Map

// 允许测试替换校验函数，生产默认指向真实实现。
var validateResolvedIP = urlvalidator.ValidateResolvedIP

// GetClient 返回共享的 HTTP 客户端实例
// 性能优化：相同配置复用同一客户端，避免重复创建 Transport
// 安全说明：代理配置失败时直接返回错误，不会回退到直连，避免 IP 关联风险
func GetClient(opts Options) (*http.Client, error) {
	key := buildClientKey(opts)
	if cached, ok := sharedClients.Load(key); ok {
		if client, ok := cached.(*http.Client); ok {
			return client, nil
		}
	}

	client, err := buildClient(opts)
	if err != nil {
		return nil, err
	}

	actual, _ := sharedClients.LoadOrStore(key, client)
	if c, ok := actual.(*http.Client); ok {
		return c, nil
	}
	return client, nil
}

func buildClient(opts Options) (*http.Client, error) {
	transport, err := buildTransport(opts)
	if err != nil {
		return nil, err
	}

	var rt http.RoundTripper = transport
	if opts.ValidateResolvedIP && !opts.AllowPrivateHosts {
		rt = newValidatedTransport(transport)
	}
	return &http.Client{
		Transport: rt,
		Timeout:   opts.Timeout,
	}, nil
}

func buildTransport(opts Options) (*http.Transport, error) {
	// 使用自定义值或默认值
	maxIdleConns := opts.MaxIdleConns
	if maxIdleConns <= 0 {
		maxIdleConns = defaultMaxIdleConns
	}
	maxIdleConnsPerHost := opts.MaxIdleConnsPerHost
	if maxIdleConnsPerHost <= 0 {
		maxIdleConnsPerHost = defaultMaxIdleConnsPerHost
	}

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: defaultDialTimeout,
		}).DialContext,
		TLSHandshakeTimeout:   defaultTLSHandshakeTimeout,
		MaxIdleConns:          maxIdleConns,
		MaxIdleConnsPerHost:   maxIdleConnsPerHost,
		MaxConnsPerHost:       opts.MaxConnsPerHost, // 0 表示无限制
		IdleConnTimeout:       defaultIdleConnTimeout,
		ResponseHeaderTimeout: opts.ResponseHeaderTimeout,
	}

	if opts.InsecureSkipVerify {
		// 安全要求：禁止跳过证书验证，避免中间人攻击。
		return nil, fmt.Errorf("insecure_skip_verify is not allowed; install a trusted certificate instead")
	}

	_, parsed, err := proxyurl.Parse(opts.ProxyURL)
	if err != nil {
		return nil, err
	}
	if parsed == nil {
		return transport, nil
	}

	if err := proxyutil.ConfigureTransportProxy(transport, parsed); err != nil {
		return nil, err
	}

	return transport, nil
}

func buildClientKey(opts Options) string {
	return fmt.Sprintf("%s|%s|%s|%t|%t|%t|%d|%d|%d",
		strings.TrimSpace(opts.ProxyURL),
		opts.Timeout.String(),
		opts.ResponseHeaderTimeout.String(),
		opts.InsecureSkipVerify,
		opts.ValidateResolvedIP,
		opts.AllowPrivateHosts,
		opts.MaxIdleConns,
		opts.MaxIdleConnsPerHost,
		opts.MaxConnsPerHost,
	)
}

type validatedTransport struct {
	base           http.RoundTripper
	validatedHosts sync.Map // map[string]time.Time, value 为过期时间
	now            func() time.Time
}

func newValidatedTransport(base http.RoundTripper) *validatedTransport {
	return &validatedTransport{
		base: base,
		now:  time.Now,
	}
}

func (t *validatedTransport) isValidatedHost(host string, now time.Time) bool {
	if t == nil {
		return false
	}
	raw, ok := t.validatedHosts.Load(host)
	if !ok {
		return false
	}
	expireAt, ok := raw.(time.Time)
	if !ok {
		t.validatedHosts.Delete(host)
		return false
	}
	if now.Before(expireAt) {
		return true
	}
	t.validatedHosts.Delete(host)
	return false
}

func (t *validatedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req != nil && req.URL != nil {
		host := strings.ToLower(strings.TrimSpace(req.URL.Hostname()))
		if host != "" {
			now := time.Now()
			if t != nil && t.now != nil {
				now = t.now()
			}
			if !t.isValidatedHost(host, now) {
				if err := validateResolvedIP(host); err != nil {
					return nil, err
				}
				t.validatedHosts.Store(host, now.Add(validatedHostTTL))
			}
		}
	}
	if t == nil || t.base == nil {
		return nil, fmt.Errorf("validated transport base is nil")
	}
	return t.base.RoundTrip(req)
}
