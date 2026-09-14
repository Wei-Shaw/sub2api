package service

import (
	"context"
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
)

// 输入图（图生图源图）的抓取限制。
const (
	adobeInputImageMaxBytes = 20 << 20 // 20 MiB
	adobeInputImageTimeout  = 30 * time.Second
)

// errAdobeInputImageBlocked 表示目标地址被安全策略拒绝。
var errAdobeInputImageBlocked = errors.New("input image url is not allowed")

// adobeInputImage 是一张待上传到 Adobe 的源图。
type adobeInputImage struct {
	Data        []byte
	ContentType string
}

// newAdobeInputImageClient 构造抓取输入图用的 HTTP 客户端。
//
// 安全要点：客户端把任意 URL 交给我们去请求，这是一条标准的 SSRF 入口。防护分两层：
//  1. 请求前校验 scheme（只允许 http/https）；
//  2. 在 Dialer.Control 里校验**实际要连接的 IP**——只做请求前的 DNS 解析检查挡不住
//     DNS rebinding（第一次解析返回公网 IP、真正连接时解析到 127.0.0.1）。
//
// 同时禁用重定向跟随：否则一个公网 URL 可以 302 到内网地址，绕过上面两层。
func newAdobeInputImageClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return errAdobeInputImageBlocked
			}
			ip := net.ParseIP(host)
			if ip == nil || !isPublicUnicastIP(ip) {
				return fmt.Errorf("%w: %s", errAdobeInputImageBlocked, host)
			}
			return nil
		},
	}
	transport := &http.Transport{
		DialContext:         dialer.DialContext,
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

// isPublicUnicastIP 判定 IP 是否为可对外路由的单播地址。
// 环回、私网、链路本地、组播、未指定地址一律拒绝。
func isPublicUnicastIP(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() ||
		ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() {
		return false
	}
	// 100.64.0.0/10（运营商级 NAT）与 IPv4 广播地址不在标准库的判定里。
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return false
		}
		if v4.Equal(net.IPv4bcast) {
			return false
		}
	}
	return true
}

// fetchAdobeInputImage 把客户端给的图片引用取成字节。
//
// data: URL 走本地解码，不产生任何网络请求；http/https 才走上面那个受限客户端。
func fetchAdobeInputImage(ctx context.Context, client *http.Client, rawURL string) (*adobeInputImage, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return nil, errors.New("input image url is empty")
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "data:") {
		return decodeAdobeDataURL(trimmed)
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("parse input image url: %w", err)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return nil, fmt.Errorf("%w: unsupported scheme %q", errAdobeInputImageBlocked, parsed.Scheme)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, trimmed, nil)
	if err != nil {
		return nil, fmt.Errorf("build input image request: %w", err)
	}
	req.Header.Set("Accept", "image/*")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch input image: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch input image: unexpected status %d", resp.StatusCode)
	}

	// 多读一个字节，以此区分「恰好等于上限」与「超出上限」。
	data, err := io.ReadAll(io.LimitReader(resp.Body, adobeInputImageMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read input image: %w", err)
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
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(payload))
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
