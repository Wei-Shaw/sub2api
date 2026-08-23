package repository

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyutil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"

	"github.com/imroc/req/v3"
)

// reqClientOptions 定义 req 客户端的构建参数
type reqClientOptions struct {
	ProxyURL    string                  // 代理 URL（支持 http/https/socks5）
	Timeout     time.Duration           // 请求超时时间
	Impersonate bool                    // 是否模拟 Chrome 浏览器指纹
	ForceHTTP2  bool                    // 是否强制使用 HTTP/2
	TLSProfile  *tlsfingerprint.Profile // 可选 uTLS ClientHello profile
}

// sharedReqClients 存储按配置参数缓存的 req 客户端实例
//
// 性能优化说明：
// 原实现在每次 OAuth 刷新时都创建新的 req.Client：
// 1. claude_oauth_service.go: 每次刷新创建新客户端
// 2. openai_oauth_service.go: 每次刷新创建新客户端
// 3. gemini_oauth_client.go: 每次刷新创建新客户端
//
// 新实现使用 sync.Map 缓存客户端：
// 1. 相同配置（代理+超时+模拟设置）复用同一客户端
// 2. 复用底层连接池，减少 TLS 握手开销
// 3. LoadOrStore 保证并发安全，避免重复创建
var sharedReqClients sync.Map

// getSharedReqClient 获取共享的 req 客户端实例
// 性能优化：相同配置复用同一客户端，避免重复创建
func getSharedReqClient(opts reqClientOptions) (*req.Client, error) {
	// req 客户端路径通过 Transport.SetDialTLS 安装 uTLS dialer，没有 ALPN
	// 嗅探，只讲 HTTP/1.1：profile 若广播 "h2"，TLS 层会协商出 h2 而
	// transport 仍按 HTTP/1.1 写请求，所有请求在运行时失败。因此在派生
	// 缓存键和 dialer 之前，先把 ALPN 收敛到 HTTP/1.1（在 clone 上修改，
	// 不动调用方共享的 profile 对象），保证缓存键与实际使用的 profile 一致。
	opts.TLSProfile = http1OnlyTLSProfile(opts.TLSProfile)

	key := buildReqClientKey(opts)
	if cached, ok := sharedReqClients.Load(key); ok {
		if c, ok := cached.(*req.Client); ok {
			return c, nil
		}
	}

	client := req.C().SetTimeout(opts.Timeout)
	if opts.ForceHTTP2 {
		client = client.EnableForceHTTP2()
	}
	if opts.Impersonate {
		client = client.ImpersonateChrome()
	}
	trimmed, parsed, err := proxyurl.Parse(opts.ProxyURL)
	if err != nil {
		return nil, err
	}
	if opts.TLSProfile == nil && trimmed != "" {
		client.SetProxyURL(trimmed)
	} else if opts.TLSProfile != nil {
		switch {
		case parsed == nil:
			client.GetTransport().SetDialTLS(tlsfingerprint.NewDialer(opts.TLSProfile, nil).DialTLSContext)
		case strings.EqualFold(parsed.Scheme, "http"):
			client.SetProxy(proxyutil.PlainHTTPProxy(parsed))
			client.GetTransport().SetDialTLS(tlsfingerprint.NewHTTPProxyDialer(opts.TLSProfile, parsed).DialTLSContext)
		case strings.EqualFold(parsed.Scheme, "socks5"), strings.EqualFold(parsed.Scheme, "socks5h"):
			client.SetProxy(proxyutil.PlainHTTPProxy(parsed))
			client.GetTransport().SetDialTLS(tlsfingerprint.NewSOCKS5ProxyDialer(opts.TLSProfile, parsed).DialTLSContext)
		default:
			// Preserve routing for HTTPS/unknown proxies; the CONNECT-oriented
			// uTLS dialers cannot establish TLS to an HTTPS proxy.
			client.SetProxyURL(trimmed)
		}
	}
	client = instrumentReqClient(client)

	actual, _ := sharedReqClients.LoadOrStore(key, client)
	if c, ok := actual.(*req.Client); ok {
		return c, nil
	}
	return client, nil
}

func instrumentReqClient(client *req.Client) *req.Client {
	if client == nil {
		return nil
	}
	client.GetTransport().WrapRoundTripFunc(func(rt http.RoundTripper) req.HttpRoundTripFunc {
		timed := servertiming.WrapRoundTripper(rt)
		return timed.RoundTrip
	})
	return client
}

// http1OnlyTLSProfile 返回 ALPN 收敛为 HTTP/1.1 的 profile 副本。
// req 客户端路径（SetDialTLS）只讲 HTTP/1.1，见 getSharedReqClient 的说明。
// 返回 clone，不修改传入的共享 profile；nil 输入返回 nil。
func http1OnlyTLSProfile(profile *tlsfingerprint.Profile) *tlsfingerprint.Profile {
	return tlsfingerprint.HTTP1OnlyProfile(profile)
}

func buildReqClientKey(opts reqClientOptions) string {
	key := fmt.Sprintf("%s|%s|%t|%t",
		strings.TrimSpace(opts.ProxyURL),
		opts.Timeout.String(),
		opts.Impersonate,
		opts.ForceHTTP2,
	)
	if opts.TLSProfile != nil {
		key += "|tls:" + opts.TLSProfile.StableID()
	}
	return key
}

// CreatePrivacyReqClient creates an HTTP client for OpenAI privacy settings API
// This is exported for use by OpenAIPrivacyService
// Uses Chrome TLS fingerprint impersonation to bypass Cloudflare checks
func CreatePrivacyReqClient(proxyURL string) (*req.Client, error) {
	return getSharedReqClient(reqClientOptions{
		ProxyURL:    proxyURL,
		Timeout:     30 * time.Second,
		Impersonate: true, // Enable Chrome TLS fingerprint impersonation
	})
}

// CreateOpenAITLSProfileReqClient creates a shared req client whose TLS
// transport is keyed by and configured from the resolved OpenAI account profile.
func CreateOpenAITLSProfileReqClient(proxyURL string, profile *tlsfingerprint.Profile) (*req.Client, error) {
	return getSharedReqClient(reqClientOptions{
		ProxyURL:    proxyURL,
		Timeout:     30 * time.Second,
		Impersonate: true,
		TLSProfile:  profile,
	})
}
