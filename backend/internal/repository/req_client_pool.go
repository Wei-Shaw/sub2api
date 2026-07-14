package repository

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"
	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"

	"github.com/imroc/req/v3"
)

// reqClientOptions 定义 req 客户端的构建参数
type reqClientOptions struct {
	ProxyURL    string        // 代理 URL（支持 http/https/socks5）
	Timeout     time.Duration // 请求超时时间
	Impersonate bool          // 是否模拟 Chrome 浏览器指纹
	ForceHTTP2  bool          // 是否强制使用 HTTP/2
}

const (
	sharedReqClientMaxEntries = 256
	sharedReqClientIdleTTL    = 10 * time.Minute
)

type sharedReqClientEntry struct {
	client   *req.Client
	lastUsed time.Time
}

type sharedReqClientPool struct {
	mu      sync.Mutex
	entries map[string]sharedReqClientEntry
	now     func() time.Time
}

func newSharedReqClientPool() *sharedReqClientPool {
	return &sharedReqClientPool{
		entries: make(map[string]sharedReqClientEntry),
		now:     time.Now,
	}
}

// sharedReqClients 存储按配置参数缓存的 req 客户端实例
//
// 性能优化说明：
// 原实现在每次 OAuth 刷新时都创建新的 req.Client：
// 1. claude_oauth_service.go: 每次刷新创建新客户端
// 2. openai_oauth_service.go: 每次刷新创建新客户端
// 3. gemini_oauth_client.go: 每次刷新创建新客户端
//
// 新实现使用有界缓存复用客户端：
// 1. 相同配置（代理+超时+模拟设置）复用同一客户端
// 2. 复用底层连接池，减少 TLS 握手开销
// 3. 并发插入时关闭未采用的重复客户端
var sharedReqClients = newSharedReqClientPool()

// getSharedReqClient 获取共享的 req 客户端实例
// 性能优化：相同配置复用同一客户端，避免重复创建
func getSharedReqClient(opts reqClientOptions) (*req.Client, error) {
	key := buildReqClientKey(opts)
	if client := sharedReqClients.get(key); client != nil {
		return client, nil
	}

	client := req.C().SetTimeout(opts.Timeout)
	if opts.ForceHTTP2 {
		client = client.EnableForceHTTP2()
	}
	if opts.Impersonate {
		client = client.ImpersonateChrome()
	}
	trimmed, _, err := proxyurl.Parse(opts.ProxyURL)
	if err != nil {
		return nil, err
	}
	if trimmed != "" {
		client.SetProxyURL(trimmed)
	}
	client = instrumentReqClient(client)

	return sharedReqClients.store(key, client), nil
}

func (p *sharedReqClientPool) get(key string) *req.Client {
	if p == nil {
		return nil
	}
	now := p.currentTime()
	p.mu.Lock()
	toClose := p.evictExpiredLocked(now)
	entry, ok := p.entries[key]
	if ok && entry.client != nil {
		entry.lastUsed = now
		p.entries[key] = entry
	}
	p.mu.Unlock()
	closeReqClients(toClose)
	if !ok {
		return nil
	}
	return entry.client
}

func (p *sharedReqClientPool) store(key string, client *req.Client) *req.Client {
	if p == nil || client == nil {
		return client
	}
	now := p.currentTime()
	p.mu.Lock()
	toClose := p.evictExpiredLocked(now)
	if existing, ok := p.entries[key]; ok && existing.client != nil {
		existing.lastUsed = now
		p.entries[key] = existing
		p.mu.Unlock()
		closeReqClients(append(toClose, client))
		return existing.client
	}
	for len(p.entries) >= sharedReqClientMaxEntries {
		oldestKey := p.oldestKeyLocked()
		oldest := p.entries[oldestKey]
		delete(p.entries, oldestKey)
		if oldest.client != nil {
			toClose = append(toClose, oldest.client)
		}
	}
	p.entries[key] = sharedReqClientEntry{client: client, lastUsed: now}
	p.mu.Unlock()
	closeReqClients(toClose)
	return client
}

func (p *sharedReqClientPool) currentTime() time.Time {
	if p != nil && p.now != nil {
		return p.now()
	}
	return time.Now()
}

func (p *sharedReqClientPool) evictExpiredLocked(now time.Time) []*req.Client {
	var toClose []*req.Client
	for key, entry := range p.entries {
		if entry.client == nil || now.Sub(entry.lastUsed) >= sharedReqClientIdleTTL {
			delete(p.entries, key)
			if entry.client != nil {
				toClose = append(toClose, entry.client)
			}
		}
	}
	return toClose
}

func (p *sharedReqClientPool) oldestKeyLocked() string {
	var oldestKey string
	var oldestAt time.Time
	for key, entry := range p.entries {
		if oldestKey == "" || entry.lastUsed.Before(oldestAt) {
			oldestKey = key
			oldestAt = entry.lastUsed
		}
	}
	return oldestKey
}

func closeReqClients(clients []*req.Client) {
	for _, client := range clients {
		if client != nil && client.GetClient() != nil {
			client.GetClient().CloseIdleConnections()
		}
	}
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

func buildReqClientKey(opts reqClientOptions) string {
	return fmt.Sprintf("%s|%s|%t|%t",
		strings.TrimSpace(opts.ProxyURL),
		opts.Timeout.String(),
		opts.Impersonate,
		opts.ForceHTTP2,
	)
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
