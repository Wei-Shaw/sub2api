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

func (t *impersonateTransport) ensureClient(timeout time.Duration) (tlsclient.HttpClient, error) {
	t.once.Do(func() {
		profile, ok := profiles.MappedTLSClients[t.identity.TLSProfile]
		if !ok {
			t.initEr = NewRequestError(fmt.Sprintf("unknown tls-client profile: %s", t.identity.TLSProfile))
			return
		}
		options := []tlsclient.HttpClientOption{
			tlsclient.WithTimeoutSeconds(int(timeout.Seconds())),
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
				fmt.Sprintf("create tls client: %v", err), 0, ErrorTypeConnection)
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
	client, err := t.ensureClient(timeout)
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
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, classifyTransportError(err, t.proxyURL != "")
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

type plainTransport struct {
	proxyURL string

	once   sync.Once
	client *http.Client
	initEr error
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
				t.initEr = NewRequestError(fmt.Sprintf("invalid proxy url: %v", err))
				return
			}
			transport.Proxy = http.ProxyURL(parsed)
		}
		t.client = &http.Client{Transport: transport}
	})
	return t.client, t.initEr
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

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, classifyTransportError(err, t.proxyURL != "")
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, classifyTransportError(err, t.proxyURL != "")
	}

	out := make(map[string]string, len(resp.Header))
	for key, values := range resp.Header {
		if len(values) > 0 {
			out[strings.ToLower(key)] = values[0]
		}
	}
	return &Response{StatusCode: resp.StatusCode, Headers: out, Body: raw}, nil
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
