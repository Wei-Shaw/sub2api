package service

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adobe"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

// 输入图（图生图源图）的抓取限制。
const (
	adobeInputImageMaxBytes = 20 << 20 // 20 MiB
	adobeInputImageTimeout  = 30 * time.Second
)

// errAdobeInputImageBlocked 表示目标地址被安全策略拒绝。
var errAdobeInputImageBlocked = errors.New("input image url is not allowed")

// errAdobeInputImageFetchFailed 表示网络请求本身失败（连接、状态码、读体）。
var errAdobeInputImageFetchFailed = errors.New("fetch input image failed")

// 面向调用方的固定文案：不回显连接错误、解析出的 IP 等内部细节。
const (
	adobeInputImageBlockedUserMessage = "input image url must be a public https domain on port 443 without redirects"
	adobeInputImageFetchUserMessage   = "failed to fetch input image"
)

// adobeInputImage 是一张待上传到 Adobe 的源图。
type adobeInputImage struct {
	Data        []byte
	ContentType string
}

// adobeInputImagePort 是输入图 URL 唯一允许的端口。
const adobeInputImagePort = "443"

// newAdobeInputImageClient 构造抓取输入图用的 HTTP 客户端。
//
// 安全要点：客户端把任意 URL 交给我们去请求，这是一条标准的 SSRF 入口。防护分三层：
//  1. 请求前由 validateAdobeInputImageURL 限定为 https + 域名 + 443；
//  2. 在 Dialer.Control 里校验**实际要连接的 IP 与端口**——只做请求前的 DNS 解析检查挡不住
//     DNS rebinding（第一次解析返回公网 IP、真正连接时解析到 127.0.0.1）；
//  3. 完全不跟随重定向：否则一个合规 URL 可以 302 到 http / IP / 内网地址，绕过上面两层。
func newAdobeInputImageClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control:   adobeInputImageDialControl,
	}
	transport := &http.Transport{
		// 显式不走代理：经代理时 Dialer.Control 看到的是代理地址，校验会失效。
		Proxy:               nil,
		DialContext:         dialer.DialContext,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout: 10 * time.Second,
		DisableKeepAlives:   true,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   adobeInputImageTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// adobeInputImageDialControl 在建立连接前校验目标 IP 与端口。
func adobeInputImageDialControl(_, address string, _ syscall.RawConn) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return errAdobeInputImageBlocked
	}
	if port != adobeInputImagePort {
		return fmt.Errorf("%w: port %s", errAdobeInputImageBlocked, port)
	}
	ip := net.ParseIP(host)
	if ip == nil || !isPublicUnicastIP(ip) {
		return fmt.Errorf("%w: %s", errAdobeInputImageBlocked, host)
	}
	return nil
}

// validateAdobeInputImageURL 校验客户端给的输入图 URL：只接受 https、域名主机（禁止 IP 字面量）、443 端口。
func validateAdobeInputImageURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: malformed url", errAdobeInputImageBlocked)
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return nil, fmt.Errorf("%w: scheme must be https, got %q", errAdobeInputImageBlocked, parsed.Scheme)
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("%w: userinfo is not allowed", errAdobeInputImageBlocked)
	}
	if port := parsed.Port(); port != "" && port != adobeInputImagePort {
		return nil, fmt.Errorf("%w: port %s", errAdobeInputImageBlocked, port)
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host == "" {
		return nil, fmt.Errorf("%w: missing host", errAdobeInputImageBlocked)
	}
	if strings.Contains(host, "%") || net.ParseIP(host) != nil {
		return nil, fmt.Errorf("%w: ip address hosts are not allowed", errAdobeInputImageBlocked)
	}
	if urlvalidator.IsBlockedHost(host) || !isAdobeInputImageDomain(host) {
		return nil, fmt.Errorf("%w: host %q is not a public domain", errAdobeInputImageBlocked, host)
	}
	parsed.Scheme = "https"
	return parsed, nil
}

// isAdobeInputImageDomain 判定 host 是否为合法的多级 ASCII 域名。
//
// 顶级域必须以字母开头：挡住 127.1、0x7f.1、0177.0.0.1 这类会被 getaddrinfo
// 当作 IP 解析的数字写法；单 label（intranet、2130706433）也一并拒绝。
// 非 ASCII 域名需由调用方先转成 punycode（xn--）。
func isAdobeInputImageDomain(host string) bool {
	if len(host) > 253 {
		return false
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
				return false
			}
		}
	}
	tld := labels[len(labels)-1]
	return tld[0] >= 'a' && tld[0] <= 'z'
}

// isPublicUnicastIP 判定 IP 是否为可对外路由的单播地址；规则与产物下载共用 adobe.IsPublicUnicastIP。
func isPublicUnicastIP(ip net.IP) bool {
	return adobe.IsPublicUnicastIP(ip)
}

// fetchAdobeInputImage 把客户端给的图片引用取成字节。
//
// data: URL 走本地解码，不产生任何网络请求；其余必须通过 validateAdobeInputImageURL
// 后才交给上面那个受限客户端。
func fetchAdobeInputImage(ctx context.Context, client *http.Client, rawURL string) (*adobeInputImage, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return nil, errors.New("input image url is empty")
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "data:") {
		return decodeAdobeDataURL(trimmed)
	}

	parsed, err := validateAdobeInputImageURL(trimmed)
	if err != nil {
		return nil, err
	}
	return doFetchAdobeInputImage(ctx, client, parsed)
}

// doFetchAdobeInputImage 请求已通过校验的 URL 并读取图片字节。
func doFetchAdobeInputImage(ctx context.Context, client *http.Client, target *url.URL) (*adobeInputImage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build input image request: %w", err)
	}
	req.Header.Set("Accept", "image/*")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errAdobeInputImageFetchFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return nil, fmt.Errorf("%w: redirects are not allowed (status %d)", errAdobeInputImageBlocked, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: unexpected status %d", errAdobeInputImageFetchFailed, resp.StatusCode)
	}

	// 多读一个字节，以此区分「恰好等于上限」与「超出上限」。
	data, err := io.ReadAll(io.LimitReader(resp.Body, adobeInputImageMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %w", errAdobeInputImageFetchFailed, err)
	}
	if len(data) > adobeInputImageMaxBytes {
		return nil, fmt.Errorf("input image exceeds %d bytes", adobeInputImageMaxBytes)
	}
	if len(data) == 0 {
		return nil, errors.New("input image is empty")
	}

	contentType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		// 上游没给出可信的类型时按字节嗅探，嗅不出来才拒。
		// 这里必须用 detectedImageContentType 而不是 detectImageContentType——
		// 后者嗅不出时会兜底返回 "image/png"，会让这个校验永远通过。
		contentType = detectedImageContentType(data)
		if contentType == "" {
			return nil, errors.New("input image url did not return an image")
		}
	}
	return &adobeInputImage{Data: data, ContentType: contentType}, nil
}

// decodeAdobeDataURL 解 data: URL，不走网络。
func decodeAdobeDataURL(rawURL string) (*adobeInputImage, error) {
	_, payload, found := strings.Cut(rawURL, ",")
	if !found {
		return nil, errors.New("malformed data url")
	}
	meta := rawURL[len("data:"):strings.Index(rawURL, ",")]
	if !strings.Contains(strings.ToLower(meta), ";base64") {
		return nil, errors.New("data url must be base64 encoded")
	}
	payload = strings.TrimSpace(payload)
	// 解码前按长度估算上限，避免为超大 data URL 先分配整块内存。DecodedLen 对带 padding 的
	// 输入最多高估 2 字节，精确判断留给解码后的检查。
	if base64.StdEncoding.DecodedLen(len(payload)) > adobeInputImageMaxBytes+2 {
		return nil, fmt.Errorf("input image exceeds %d bytes", adobeInputImageMaxBytes)
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("decode data url: %w", err)
	}
	if len(data) == 0 {
		return nil, errors.New("input image is empty")
	}
	if len(data) > adobeInputImageMaxBytes {
		return nil, fmt.Errorf("input image exceeds %d bytes", adobeInputImageMaxBytes)
	}

	contentType := strings.TrimSpace(strings.Split(meta, ";")[0])
	if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		// 同上：用严格版嗅探，兜底版会把任意字节都说成 image/png。
		contentType = detectedImageContentType(data)
		if contentType == "" {
			return nil, errors.New("data url does not carry an image")
		}
	}
	return &adobeInputImage{Data: data, ContentType: contentType}, nil
}

// adobeInputImageRequestError 把取图错误包装成 Adobe 终态错误。
// 策略拒绝与网络失败对外只给固定文案；大小、格式等本地校验错误不含内部信息，原样透出。
func adobeInputImageRequestError(err error) *adobe.RequestError {
	reqErr := adobe.NewRequestError(err.Error())
	switch {
	case errors.Is(err, errAdobeInputImageBlocked):
		reqErr.UserMessage = adobeInputImageBlockedUserMessage
	case errors.Is(err, errAdobeInputImageFetchFailed):
		reqErr.UserMessage = adobeInputImageFetchUserMessage
	}
	return reqErr
}
