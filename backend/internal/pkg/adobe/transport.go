package adobe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

// maxErrorBodyBytes 是错误信息里回显的上游响应体上限，避免把整页 HTML 塞进日志。
const maxErrorBodyBytes = 300

// defaultRequestTimeout 是未显式指定时的单次请求超时。
const defaultRequestTimeout = 60 * time.Second

// impersonateClientTimeout 是 tls-client 客户端级超时的上限。
//
// 客户端只构造一次，不能沿用首个请求的 Timeout（否则先发一个 20s 的 credits 查询，后续 60s
// 的提交都会被截断）；单次请求的时限由 Do 里的 context.WithTimeout 负责。也不能设 0：
// tls-client 走 HTTP 代理 CONNECT 时会拿 now+timeout 当 deadline，0 会让握手立即失败。
const impersonateClientTimeout = 10 * time.Minute

// 响应体读取上限。Request.MaxBodyBytes 为 0 时 API 调用取 DefaultMaxResponseBytes。
const (
	DefaultMaxResponseBytes int64 = 16 << 20 // API JSON 响应
	MaxImageDownloadBytes   int64 = 64 << 20 // 单张图片产物
	MaxVideoDownloadBytes   int64 = 1 << 30  // 单个视频产物
)

// Request 是一次上游 HTTP 请求。
type Request struct {
	Method  string
	URL     string
	Headers map[string]string
	// HeaderOrder 指定发送顺序；为空时按 Headers 的字典序发送。
	//
	// 顺序是浏览器指纹的一部分：Go 的 map 迭代顺序随机，若不显式固定，同一请求每次
	// 发出的 header 顺序都不同，本身就是可被识别的特征。
	HeaderOrder []string
	Body        []byte
	Timeout     time.Duration
	// MaxBodyBytes 是响应体读取上限；超出时返回 RequestError。<=0 取 DefaultMaxResponseBytes。
	MaxBodyBytes int64
}

// Response 是一次上游 HTTP 响应。响应体已完整读入内存——Firefly 的响应都是小 JSON
// 或单个媒体文件，不需要流式处理。
type Response struct {
	StatusCode int
	// Headers 的键统一小写，多值只保留第一个。
	Headers map[string]string
	Body    []byte
}

// Header 取响应头（大小写不敏感）。
func (r *Response) Header(name string) string {
	if r == nil || r.Headers == nil {
		return ""
	}
	return r.Headers[strings.ToLower(name)]
}

// BodyPreview 返回截断后的响应体，用于拼错误信息。
func (r *Response) BodyPreview() string {
	if r == nil {
		return ""
	}
	if len(r.Body) > maxErrorBodyBytes {
		return string(r.Body[:maxErrorBodyBytes])
	}
	return string(r.Body)
}

// Transport 是 Firefly 直连的 HTTP 传输抽象。
//
// 拆成接口有两个用途：区分「需要 TLS 伪装的 Adobe API 调用」与「不需要伪装的产物
// 下载」，以及让 client/auth 的单测能注入假实现而不打真实网络。
type Transport interface {
	Do(ctx context.Context, req *Request) (*Response, error)
}

// NewImpersonateTransport 构造带 Chrome TLS 指纹的传输，用于所有 Adobe API 调用。
//
// proxyURL 为空表示直连。identity 的零值字段会用 DefaultIdentity 补齐。
func NewImpersonateTransport(identity Identity, proxyURL string) Transport {
	return &impersonateTransport{
		identity: identity.withDefaults(),
		proxyURL: strings.TrimSpace(proxyURL),
	}
}

// NewPlainTransport 构造标准库传输，用于下载 presigned 产物 URL。
//
// 产物是 S3 直链，不经 Adobe 风控，不需要也不应该带浏览器指纹。
func NewPlainTransport(proxyURL string) Transport {
	return &plainTransport{proxyURL: strings.TrimSpace(proxyURL)}
}

// ---- TLS 伪装传输 ----

type impersonateTransport struct {
	identity Identity
	proxyURL string

	once   sync.Once
	client tlsclient.HttpClient
	initEr error
}

func (t *impersonateTransport) ensureClient() (tlsclient.HttpClient, error) {
	t.once.Do(func() {
		profile, ok := profiles.MappedTLSClients[t.identity.TLSProfile]
		if !ok {
			t.initEr = NewRequestError(fmt.Sprintf("unknown tls-client profile: %s", t.identity.TLSProfile))
			return
		}
		options := []tlsclient.HttpClientOption{
			tlsclient.WithTimeoutSeconds(int(impersonateClientTimeout.Seconds())),
			tlsclient.WithClientProfile(profile),
			tlsclient.WithCookieJar(tlsclient.NewCookieJar()),
			// Firefly 的提交/轮询都是普通 HTTPS；关掉 HTTP/3 以免协商出与 profile
			// 不符的指纹。
			tlsclient.WithDisableHttp3(),
			tlsclient.WithCatchPanics(),
		}
		if t.proxyURL != "" {
			options = append(options, tlsclient.WithProxyUrl(t.proxyURL))
		}
		client, err := tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), options...)
		if err != nil {
			t.initEr = NewUpstreamTemporaryError(
				redactProxyURL(fmt.Sprintf("create tls client: %v", err), t.proxyURL), 0, ErrorTypeConnection)
			return
		}
		t.client = client
	})
	return t.client, t.initEr
}

func (t *impersonateTransport) Do(ctx context.Context, req *Request) (*Response, error) {
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	client, err := t.ensureClient()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var body io.Reader
	if len(req.Body) > 0 {
		body = bytes.NewReader(req.Body)
	}
	httpReq, err := fhttp.NewRequestWithContext(ctx, req.Method, req.URL, body)
	if err != nil {
		return nil, NewRequestError(fmt.Sprintf("build request: %v", err))
	}
	applyHeaderOrder(httpReq, req.Headers, req.HeaderOrder)

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, classifyTransportError(err, t.proxyURL != "")
	}
	if resp == nil {
		// WithCatchPanics 恢复库内 panic 时返回 (nil, nil)。
		return nil, NewUpstreamTemporaryError("tls client returned no response", 0, ErrorTypeNetwork)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := readLimitedBody(resp.Body, req.MaxBodyBytes)
	if err != nil {
		return nil, classifyReadError(err, t.proxyURL != "")
	}
	return &Response{
		StatusCode: resp.StatusCode,
		Headers:    flattenHeaders(resp.Header),
		Body:       raw,
	}, nil
}

// applyHeaderOrder 写入请求头并固定发送顺序。
func applyHeaderOrder(req *fhttp.Request, headers map[string]string, order []string) {
	req.Header = fhttp.Header{}
	written := make(map[string]bool, len(headers))

	sendOrder := make([]string, 0, len(headers))

	// appendHeader 写入一个头并把它记进发送顺序；头不存在时什么也不做——
	// 顺序列表必须只描述真正发出去的头，混进不存在的名字会让指纹与声明不符。
	appendHeader := func(name string) {
		lower := strings.ToLower(name)
		if written[lower] {
			return
		}
		value, ok := headers[name]
		if !ok {
			// 允许调用方用任意大小写声明顺序。
			for key, v := range headers {
				if strings.EqualFold(key, name) {
					value, ok = v, true
					break
				}
			}
		}
		if !ok {
			return
		}
		req.Header[lower] = []string{value}
		written[lower] = true
		sendOrder = append(sendOrder, lower)
	}

	for _, name := range order {
		appendHeader(name)
	}
	// 未在 order 里出现的头补在后面，保证不会被静默丢弃。
	for name := range headers {
		appendHeader(name)
	}
	req.Header[fhttp.HeaderOrderKey] = sendOrder
}

func flattenHeaders(headers fhttp.Header) map[string]string {
	out := make(map[string]string, len(headers))
	for key, values := range headers {
		if strings.EqualFold(key, fhttp.HeaderOrderKey) || len(values) == 0 {
			continue
		}
		out[strings.ToLower(key)] = values[0]
	}
	return out
}

// ---- 标准库传输 ----

// plainTransportDialControl 是直连下载时的拨号校验；单测连 httptest 回环服务时置 nil。
var plainTransportDialControl = publicDialControl

// maxDownloadRedirects 是产物下载允许跟随的重定向次数上限。
const maxDownloadRedirects = 3

type plainTransport struct {
	proxyURL string

	once   sync.Once
	client *http.Client
	initEr error
	// resolveGuard 在经代理下载时为 true：拨号目标是代理本身，拨号层校验失效，
	// 改为发请求前在本地解析主机名做尽力而为的 IP 校验。
	resolveGuard bool
}

func (t *plainTransport) ensureClient() (*http.Client, error) {
	t.once.Do(func() {
		// 克隆默认传输以继承标准超时与 HTTP/2 设置；类型断言理论上不会失败，
		// 失败时退回一个最小可用的传输而不是让下载整条路不可用。
		transport := &http.Transport{Proxy: http.ProxyFromEnvironment}
		if base, ok := http.DefaultTransport.(*http.Transport); ok {
			transport = base.Clone()
		}
		if t.proxyURL != "" {
			parsed, err := url.Parse(t.proxyURL)
			if err != nil {
				// 不回显原串：代理 URL 里常带 user:pass。
				t.initEr = NewRequestError("invalid proxy url")
				return
			}
			transport.Proxy = http.ProxyURL(parsed)
			t.resolveGuard = true
		} else if environmentProxyConfigured() {
			t.resolveGuard = true
		} else if plainTransportDialControl != nil {
			// 直连时在拨号层校验实际 IP：产物直链来自上游响应，下载结果会原样交给调用方，
			// 不能被利用来探测内网。经代理（账号代理或环境代理）时拨号目标是代理本身，改走 resolveGuard。
			dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second, Control: plainTransportDialControl}
			transport.Proxy = nil
			transport.DialContext = dialer.DialContext
		}
		t.client = &http.Client{Transport: transport, CheckRedirect: t.checkRedirect}
	})
	return t.client, t.initEr
}

// checkRedirect 对每一跳重定向重新校验目标：否则合规的直链可以 302 到内网主机名，
// 经代理时拨号层校验看不到真实目标。
func (t *plainTransport) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxDownloadRedirects {
		return fmt.Errorf("%w: more than %d redirects", ErrBlockedDestination, maxDownloadRedirects)
	}
	if err := validateDownloadURL(req.URL.String()); err != nil {
		return fmt.Errorf("%w: %v", ErrBlockedDestination, err)
	}
	if t.resolveGuard {
		return checkResolvedPublicHost(req.Context(), req.URL.Hostname())
	}
	return nil
}

func (t *plainTransport) Do(ctx context.Context, req *Request) (*Response, error) {
	client, err := t.ensureClient()
	if err != nil {
		return nil, err
	}

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var body io.Reader
	if len(req.Body) > 0 {
		body = bytes.NewReader(req.Body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, body)
	if err != nil {
		return nil, NewRequestError(fmt.Sprintf("build request: %v", err))
	}
	for name, value := range req.Headers {
		httpReq.Header.Set(name, value)
	}

	if t.resolveGuard {
		if err := checkResolvedPublicHost(ctx, httpReq.URL.Hostname()); err != nil {
			return nil, NewRequestError("media download destination is not allowed")
		}
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		if errors.Is(err, ErrBlockedDestination) {
			return nil, NewRequestError("media download destination is not allowed")
		}
		return nil, classifyTransportError(err, t.proxyURL != "")
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := readLimitedBody(resp.Body, req.MaxBodyBytes)
	if err != nil {
		return nil, classifyReadError(err, t.proxyURL != "")
	}

	out := make(map[string]string, len(resp.Header))
	for key, values := range resp.Header {
		if len(values) > 0 {
			out[strings.ToLower(key)] = values[0]
		}
	}
	return &Response{StatusCode: resp.StatusCode, Headers: out, Body: raw}, nil
}

// errBodyTooLarge 表示响应体超过 Request.MaxBodyBytes。
var errBodyTooLarge = errors.New("response body too large")

// readLimitedBody 读取响应体，超过 limit（<=0 取 DefaultMaxResponseBytes）时返回 errBodyTooLarge。
func readLimitedBody(body io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		limit = DefaultMaxResponseBytes
	}
	raw, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("%w: exceeds %d bytes", errBodyTooLarge, limit)
	}
	return raw, nil
}

// classifyReadError 区分「响应体超限」（终态）与读体时的网络错误（可重试）。
func classifyReadError(err error, viaProxy bool) error {
	if errors.Is(err, errBodyTooLarge) {
		return NewRequestError(err.Error())
	}
	return classifyTransportError(err, viaProxy)
}

// environmentProxyConfigured 报告进程环境是否为 https 请求配置了代理。
func environmentProxyConfigured() bool {
	proxy, err := http.ProxyFromEnvironment(&http.Request{URL: &url.URL{Scheme: "https", Host: "firefly.adobe.io"}})
	return err == nil && proxy != nil
}

// redactProxyURL 把错误信息里的代理 URL 替换成不含凭据的形式。
func redactProxyURL(message, proxyURL string) string {
	if proxyURL == "" {
		return message
	}
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return strings.ReplaceAll(message, proxyURL, "[proxy]")
	}
	safe := parsed.Scheme + "://" + parsed.Host
	message = strings.ReplaceAll(message, proxyURL, safe)
	if parsed.User != nil {
		message = strings.ReplaceAll(message, parsed.User.String(), "***")
		if password, ok := parsed.User.Password(); ok && password != "" {
			message = strings.ReplaceAll(message, password, "***")
		}
	}
	return message
}

// classifyTransportError 把网络层错误归到可重试的临时错误，并标出来源，
// 便于上层区分「换个账号重试」与「换个代理」。
func classifyTransportError(err error, viaProxy bool) error {
	if err == nil {
		return nil
	}
	// 调用方主动取消不是上游故障，原样上抛。
	if errors.Is(err, context.Canceled) {
		return err
	}

	errorType := ErrorTypeNetwork
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		errorType = ErrorTypeTimeout
	case isTimeoutError(err):
		errorType = ErrorTypeTimeout
	case viaProxy && isProxyError(err):
		errorType = ErrorTypeProxy
	case isConnectionError(err):
		errorType = ErrorTypeConnection
	}
	return NewUpstreamTemporaryError(err.Error(), 0, errorType)
}

func isTimeoutError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func isProxyError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "proxy") || strings.Contains(msg, "socks")
}

func isConnectionError(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "eof")
}
