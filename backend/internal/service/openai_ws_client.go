package service

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	openaiwsv2 "github.com/Wei-Shaw/sub2api/internal/service/openai_ws_v2"
	coderws "github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

var tlsEgressWSLogInitOnce sync.Once

// tlsEgressWSLogEnabled 由环境变量 TLS_EGRESS_LOG=1|true 开启（与 repository.DoWithTLS 同名开关一致）。
// Codex Responses 走 WebSocket 出站、用自己的 DialTLSContext 拨号器、不经过 DoWithTLS，
// 故这里为 WS 建连单独补一条握手指纹 + 出站 UA 日志。每次请求读 env（避免 init 期读取疑难），
// 首次调用打印一条 tls_egress_ws_init 诊断。开启时按「每次新建 WS 连接」打印（连接复用不重复）。
func tlsEgressWSLogEnabled() bool {
	v := strings.TrimSpace(os.Getenv("TLS_EGRESS_LOG"))
	on := v == "1" || strings.EqualFold(v, "true")
	tlsEgressWSLogInitOnce.Do(func() {
		logger.LegacyPrintf("service.tls_egress", "tls_egress_ws_init enabled=%v raw_env=%q", on, v)
	})
	return on
}

const openAIWSMessageReadLimitBytes int64 = 16 * 1024 * 1024
const (
	openAIWSProxyTransportMaxIdleConns        = 128
	openAIWSProxyTransportMaxIdleConnsPerHost = 64
	openAIWSProxyTransportIdleConnTimeout     = 90 * time.Second
	openAIWSProxyClientCacheMaxEntries        = 256
	openAIWSProxyClientCacheIdleTTL           = 15 * time.Minute
)

type OpenAIWSTransportMetricsSnapshot struct {
	ProxyClientCacheHits   int64   `json:"proxy_client_cache_hits"`
	ProxyClientCacheMisses int64   `json:"proxy_client_cache_misses"`
	TransportReuseRatio    float64 `json:"transport_reuse_ratio"`
}

// openAIWSClientConn 抽象 WS 客户端连接，便于替换底层实现。
type openAIWSClientConn interface {
	WriteJSON(ctx context.Context, value any) error
	ReadMessage(ctx context.Context) ([]byte, error)
	Ping(ctx context.Context) error
	Close() error
}

// openAIWSClientDialer 抽象 WS 建连器。
type openAIWSClientDialer interface {
	Dial(ctx context.Context, wsURL string, headers http.Header, proxyURL string, profile *tlsfingerprint.Profile) (openAIWSClientConn, int, http.Header, error)
}

type openAIWSTransportMetricsDialer interface {
	SnapshotTransportMetrics() OpenAIWSTransportMetricsSnapshot
}

func newDefaultOpenAIWSClientDialer() openAIWSClientDialer {
	return &coderOpenAIWSClientDialer{
		proxyClients: make(map[string]*openAIWSProxyClientEntry),
	}
}

type coderOpenAIWSClientDialer struct {
	proxyMu      sync.Mutex
	proxyClients map[string]*openAIWSProxyClientEntry
	proxyHits    atomic.Int64
	proxyMisses  atomic.Int64
}

type openAIWSProxyClientEntry struct {
	client           *http.Client
	lastUsedUnixNano int64
}

func (d *coderOpenAIWSClientDialer) Dial(
	ctx context.Context,
	wsURL string,
	headers http.Header,
	proxyURL string,
	profile *tlsfingerprint.Profile,
) (openAIWSClientConn, int, http.Header, error) {
	targetURL := strings.TrimSpace(wsURL)
	if targetURL == "" {
		return nil, 0, nil, errors.New("ws url is empty")
	}

	if tlsEgressWSLogEnabled() {
		ua := ""
		if headers != nil {
			ua = headers.Get("User-Agent")
		}
		profileName := "<none: go-default-ja3>"
		alpn := ""
		grease := false
		if profile != nil {
			profileName = profile.Name
			alpn = strings.Join(profile.ALPNProtocols, ",")
			grease = profile.EnableGREASE
		}
		// 注:WS 出站会被 HTTP1OnlyProfile 强制成 http/1.1,alpn 字段展示的是 profile 原始值。
		logger.LegacyPrintf("service.tls_egress",
			"tls_egress_ws url=%s ua=%q tls_profile=%s alpn=%s grease=%v",
			targetURL, ua, profileName, alpn, grease)
	}

	opts := &coderws.DialOptions{
		HTTPHeader:      cloneHeader(headers),
		CompressionMode: coderws.CompressionContextTakeover,
	}
	// 有 TLS 指纹或有代理时,都需要自定义 http.Client(指纹 → DialTLSContext;代理 → Proxy)。
	if profile != nil || strings.TrimSpace(proxyURL) != "" {
		proxyClient, err := d.proxyHTTPClient(proxyURL, profile)
		if err != nil {
			return nil, 0, nil, err
		}
		opts.HTTPClient = proxyClient
	}

	conn, resp, err := coderws.Dial(ctx, targetURL, opts)
	if err != nil {
		status := 0
		respHeaders := http.Header(nil)
		if resp != nil {
			status = resp.StatusCode
			respHeaders = cloneHeader(resp.Header)
		}
		return nil, status, respHeaders, err
	}
	// coder/websocket 默认单消息读取上限为 32KB，Codex WS 事件（如 rate_limits/大 delta）
	// 可能超过该阈值，需显式提高上限，避免本地 read_fail(message too big)。
	conn.SetReadLimit(openAIWSMessageReadLimitBytes)
	respHeaders := http.Header(nil)
	if resp != nil {
		respHeaders = cloneHeader(resp.Header)
	}
	return &coderOpenAIWSClientConn{conn: conn}, 0, respHeaders, nil
}

func (d *coderOpenAIWSClientDialer) proxyHTTPClient(proxy string, profile *tlsfingerprint.Profile) (*http.Client, error) {
	if d == nil {
		return nil, errors.New("openai ws dialer is nil")
	}
	normalizedProxy := strings.TrimSpace(proxy)
	if normalizedProxy == "" && profile == nil {
		return nil, errors.New("proxy url is empty")
	}
	var parsedProxyURL *url.URL
	if normalizedProxy != "" {
		var err error
		parsedProxyURL, err = url.Parse(normalizedProxy)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy url: %w", err)
		}
	}
	// WebSocket 使用 HTTP/1.1 Upgrade,TLS ALPN 不能声明 h2;缓存键纳入指纹,避免不同指纹复用同一 client。
	profile = tlsfingerprint.HTTP1OnlyProfile(profile)
	cacheKey := normalizedProxy + "|tls:" + tlsfingerprint.CacheKey(profile)
	now := time.Now().UnixNano()

	d.proxyMu.Lock()
	defer d.proxyMu.Unlock()
	if entry, ok := d.proxyClients[cacheKey]; ok && entry != nil && entry.client != nil {
		entry.lastUsedUnixNano = now
		d.proxyHits.Add(1)
		return entry.client, nil
	}
	d.cleanupProxyClientsLocked(now)
	transport, err := buildOpenAIWSHTTPTransport(normalizedProxy, parsedProxyURL, profile)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Transport: transport}
	d.proxyClients[cacheKey] = &openAIWSProxyClientEntry{
		client:           client,
		lastUsedUnixNano: now,
	}
	d.ensureProxyClientCapacityLocked()
	d.proxyMisses.Add(1)
	return client, nil
}

// buildOpenAIWSHTTPTransport 构造 WS 出站的 http.Transport:强制 HTTP/1.1;有指纹时用 tlsfingerprint 拨号器设 DialTLSContext。
func buildOpenAIWSHTTPTransport(normalizedProxy string, parsedProxyURL *url.URL, profile *tlsfingerprint.Profile) (*http.Transport, error) {
	transport := &http.Transport{
		MaxIdleConns:        openAIWSProxyTransportMaxIdleConns,
		MaxIdleConnsPerHost: openAIWSProxyTransportMaxIdleConnsPerHost,
		IdleConnTimeout:     openAIWSProxyTransportIdleConnTimeout,
		TLSHandshakeTimeout: 10 * time.Second,
		ForceAttemptHTTP2:   false,
		// WebSocket 固定 HTTP/1.1 Upgrade,显式关闭自动 HTTP/2 协商。
		TLSNextProto: make(map[string]func(string, *tls.Conn) http.RoundTripper),
	}
	profile = tlsfingerprint.HTTP1OnlyProfile(profile)
	if profile == nil {
		if parsedProxyURL != nil {
			transport.Proxy = http.ProxyURL(parsedProxyURL)
		}
		return transport, nil
	}
	// 自定义 DialTLSContext,让 wss 握手复用 TLS 指纹伪装。
	switch {
	case parsedProxyURL == nil:
		dialer := tlsfingerprint.NewDialer(profile, nil)
		transport.DialTLSContext = dialer.DialTLSContext
	case parsedProxyURL.Scheme == "socks5" || parsedProxyURL.Scheme == "socks5h":
		dialer := tlsfingerprint.NewSOCKS5ProxyDialer(profile, parsedProxyURL)
		transport.DialTLSContext = dialer.DialTLSContext
	case parsedProxyURL.Scheme == "http" || parsedProxyURL.Scheme == "https":
		dialer := tlsfingerprint.NewHTTPProxyDialer(profile, parsedProxyURL)
		transport.DialTLSContext = dialer.DialTLSContext
	default:
		return nil, fmt.Errorf("unsupported proxy URL for OpenAI WS TLS fingerprint: %s", normalizedProxy)
	}
	return transport, nil
}

func (d *coderOpenAIWSClientDialer) cleanupProxyClientsLocked(nowUnixNano int64) {
	if d == nil || len(d.proxyClients) == 0 {
		return
	}
	idleTTL := openAIWSProxyClientCacheIdleTTL
	if idleTTL <= 0 {
		return
	}
	now := time.Unix(0, nowUnixNano)
	for key, entry := range d.proxyClients {
		if entry == nil || entry.client == nil {
			delete(d.proxyClients, key)
			continue
		}
		lastUsed := time.Unix(0, entry.lastUsedUnixNano)
		if now.Sub(lastUsed) > idleTTL {
			closeOpenAIWSProxyClient(entry.client)
			delete(d.proxyClients, key)
		}
	}
}

func (d *coderOpenAIWSClientDialer) ensureProxyClientCapacityLocked() {
	if d == nil {
		return
	}
	maxEntries := openAIWSProxyClientCacheMaxEntries
	if maxEntries <= 0 {
		return
	}
	for len(d.proxyClients) > maxEntries {
		var oldestKey string
		var oldestLastUsed int64
		hasOldest := false
		for key, entry := range d.proxyClients {
			lastUsed := int64(0)
			if entry != nil {
				lastUsed = entry.lastUsedUnixNano
			}
			if !hasOldest || lastUsed < oldestLastUsed {
				hasOldest = true
				oldestKey = key
				oldestLastUsed = lastUsed
			}
		}
		if !hasOldest {
			return
		}
		if entry := d.proxyClients[oldestKey]; entry != nil {
			closeOpenAIWSProxyClient(entry.client)
		}
		delete(d.proxyClients, oldestKey)
	}
}

func closeOpenAIWSProxyClient(client *http.Client) {
	if client == nil || client.Transport == nil {
		return
	}
	if transport, ok := client.Transport.(*http.Transport); ok && transport != nil {
		transport.CloseIdleConnections()
	}
}

func (d *coderOpenAIWSClientDialer) SnapshotTransportMetrics() OpenAIWSTransportMetricsSnapshot {
	if d == nil {
		return OpenAIWSTransportMetricsSnapshot{}
	}
	hits := d.proxyHits.Load()
	misses := d.proxyMisses.Load()
	total := hits + misses
	reuseRatio := 0.0
	if total > 0 {
		reuseRatio = float64(hits) / float64(total)
	}
	return OpenAIWSTransportMetricsSnapshot{
		ProxyClientCacheHits:   hits,
		ProxyClientCacheMisses: misses,
		TransportReuseRatio:    reuseRatio,
	}
}

type coderOpenAIWSClientConn struct {
	conn *coderws.Conn
}

var _ openaiwsv2.FrameConn = (*coderOpenAIWSClientConn)(nil)

func (c *coderOpenAIWSClientConn) WriteJSON(ctx context.Context, value any) error {
	if c == nil || c.conn == nil {
		return errOpenAIWSConnClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return wsjson.Write(ctx, c.conn, value)
}

func (c *coderOpenAIWSClientConn) ReadMessage(ctx context.Context) ([]byte, error) {
	if c == nil || c.conn == nil {
		return nil, errOpenAIWSConnClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}

	msgType, payload, err := c.conn.Read(ctx)
	if err != nil {
		return nil, err
	}
	switch msgType {
	case coderws.MessageText, coderws.MessageBinary:
		return payload, nil
	default:
		return nil, errOpenAIWSConnClosed
	}
}

func (c *coderOpenAIWSClientConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	if c == nil || c.conn == nil {
		return coderws.MessageText, nil, errOpenAIWSConnClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	msgType, payload, err := c.conn.Read(ctx)
	if err != nil {
		return coderws.MessageText, nil, err
	}
	return msgType, payload, nil
}

func (c *coderOpenAIWSClientConn) WriteFrame(ctx context.Context, msgType coderws.MessageType, payload []byte) error {
	if c == nil || c.conn == nil {
		return errOpenAIWSConnClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return c.conn.Write(ctx, msgType, payload)
}

func (c *coderOpenAIWSClientConn) Ping(ctx context.Context) error {
	if c == nil || c.conn == nil {
		return errOpenAIWSConnClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return c.conn.Ping(ctx)
}

func (c *coderOpenAIWSClientConn) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	// Close 为幂等，忽略重复关闭错误。
	_ = c.conn.Close(coderws.StatusNormalClosure, "")
	_ = c.conn.CloseNow()
	return nil
}
