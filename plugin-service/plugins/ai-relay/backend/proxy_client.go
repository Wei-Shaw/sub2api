package backend

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

var ErrUnsupportedProxyProtocol = errors.New("unsupported proxy protocol")

const ProxyIDHeader = "X-Sub2api-Proxy-Id"

type RelayClientProvider interface {
	ClientFor(context.Context, string) (*http.Client, error)
}

type ProxyClientProvider struct {
	direct   *http.Client
	resolver ProxyResolver
	mu       sync.Mutex
	clients  map[int64]cachedProxyClient
}

type cachedProxyClient struct {
	config ProxyConfig
	client *http.Client
}

func NewProxyClientProvider(direct *http.Client, resolver ProxyResolver) *ProxyClientProvider {
	if direct == nil {
		direct = http.DefaultClient
	}
	return &ProxyClientProvider{
		direct:   direct,
		resolver: resolver,
		clients:  make(map[int64]cachedProxyClient),
	}
}

func (p *ProxyClientProvider) ClientFor(ctx context.Context, rawProxyID string) (*http.Client, error) {
	rawProxyID = strings.TrimSpace(rawProxyID)
	if rawProxyID == "" {
		return p.direct, nil
	}
	proxyID, err := strconv.ParseInt(rawProxyID, 10, 64)
	if err != nil || proxyID < 1 {
		return nil, ErrInvalidProxyID
	}
	if p == nil || p.resolver == nil {
		return nil, ErrProxyStorageUnavailable
	}
	config, err := p.resolver.Resolve(ctx, proxyID)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if cached, ok := p.clients[proxyID]; ok && sameProxyConfig(cached.config, config) {
		return cached.client, nil
	}
	client, err := p.buildClient(config)
	if err != nil {
		return nil, err
	}
	if cached, ok := p.clients[proxyID]; ok {
		cached.client.CloseIdleConnections()
	}
	p.clients[proxyID] = cachedProxyClient{config: config, client: client}
	return client, nil
}

func (p *ProxyClientProvider) buildClient(config ProxyConfig) (*http.Client, error) {
	protocol := strings.ToLower(strings.TrimSpace(config.Protocol))
	switch protocol {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, ErrUnsupportedProxyProtocol
	}
	if strings.TrimSpace(config.Host) == "" || config.Port < 1 || config.Port > 65535 {
		return nil, ErrProxyNotAvailable
	}
	proxyURL := &url.URL{
		Scheme: protocol,
		Host:   net.JoinHostPort(strings.TrimSpace(config.Host), strconv.Itoa(config.Port)),
	}
	if config.Username != "" || config.Password != "" {
		proxyURL.User = url.UserPassword(config.Username, config.Password)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxyURL)
	return &http.Client{
		Transport:     transport,
		CheckRedirect: p.direct.CheckRedirect,
		Jar:           p.direct.Jar,
		Timeout:       p.direct.Timeout,
	}, nil
}

func sameProxyConfig(left, right ProxyConfig) bool {
	return left.ID == right.ID &&
		left.Protocol == right.Protocol &&
		left.Host == right.Host &&
		left.Port == right.Port &&
		left.Username == right.Username &&
		left.Password == right.Password &&
		left.UpdatedAt.Equal(right.UpdatedAt)
}
