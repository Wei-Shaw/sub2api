package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

const defaultImageMaxDownloadBytes int64 = 32 << 20 // 32 MiB

// ImageStorage 把图片字节写入对象存储并返回可访问 URL。
//
// 这是对象存储的可插拔抽象：适配一个新的对象存储厂商，只需实现本接口
// （例如包一个厂商 SDK），无需改动任务/网关逻辑。仓库内自带一个 S3 兼容实现
// （repository.S3ImageStorage），适用于 AWS S3 / Cloudflare R2 / 阿里云 OSS / MinIO 等。
type ImageStorage interface {
	// Save 把 data 以 key 存入对象存储，返回可下载的 URL（公开直链或 presigned 临时链接）。
	// contentType 为图片 MIME 类型，如 "image/png"。
	Save(ctx context.Context, key, contentType string, data []byte) (url string, err error)
}

// ImageResultUploader 是 ImageStorage 的上层编排器（与具体厂商无关）：
// 把上游生图响应里的每张图片（b64_json 解码 / url 下载）转存到对象存储，
// 并把响应结果改写为只含短链接的紧凑 JSON，从而避免大 base64 落 Redis。
type ImageResultUploader struct {
	storage          ImageStorage
	httpClient       *http.Client
	prefix           string
	maxDownloadBytes int64
}

// NewImageResultUploader 构造一个 uploader；storage 为 nil 时 Rewrite 直接透传。
func NewImageResultUploader(storage ImageStorage, prefix string, maxDownloadBytes int64, httpClient *http.Client) *ImageResultUploader {
	if httpClient == nil {
		httpClient = defaultImageDownloadHTTPClient()
	}
	if maxDownloadBytes <= 0 {
		maxDownloadBytes = defaultImageMaxDownloadBytes
	}
	return &ImageResultUploader{
		storage:          storage,
		httpClient:       httpClient,
		prefix:           prefix,
		maxDownloadBytes: maxDownloadBytes,
	}
}

// defaultImageDownloadHTTPClient 是进程内共享的图片下载客户端，复用连接池。
var defaultImageDownloadHTTPClient = sync.OnceValue(func() *http.Client {
	return newImageDownloadHTTPClient(http.ProxyFromEnvironment)
})

// newImageDownloadHTTPClient 返回只允许下载公网目的地的客户端。
// 图片 url 来自上游响应，是不可信输入：目的地为 localhost、回环、私网、链路本地、
// 云元数据等地址时一律拒绝，重定向的每一跳都经同一 RoundTrip 校验。
// 不经代理时在 socket 层校验真实连接 IP（防 DNS rebinding）；经环境代理时由代理解析目标，
// 只能在发出前校验本地解析结果。
func newImageDownloadHTTPClient(proxy func(*http.Request) (*url.URL, error)) *http.Client {
	direct := newImageDownloadBaseTransport(imageDownloadDialControl)
	proxied := newImageDownloadBaseTransport(nil)
	proxied.Proxy = proxy
	return &http.Client{
		Timeout:   60 * time.Second,
		Transport: &imageDownloadTransport{direct: direct, proxied: proxied, proxy: proxy, resolvesPrivate: isPrivateOrLoopbackHost},
	}
}

// newImageDownloadBaseTransport 返回与 http.DefaultTransport 参数一致、但不带代理的 Transport。
// control 非空时在每个 socket 连接前校验目标地址。
func newImageDownloadBaseTransport(control func(network, address string, c syscall.RawConn) error) *http.Transport {
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second, Control: control}
	return &http.Transport{
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

// imageDownloadDialControl 拒绝连接到非公网 IP。address 是 DNS 解析后的真实连接地址，
// 多个解析结果时 Dialer 会跳过被拒地址继续尝试下一个。
func imageDownloadDialControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(stripIPv6Zone(host)); ip == nil || isPrivateIP(ip) {
		return &net.AddrError{Err: "address not allowed for image download", Addr: address}
	}
	return nil
}

// imageDownloadTransport 按请求是否走环境代理分派到对应 Transport，并在发出前拒绝非公网目的地。
type imageDownloadTransport struct {
	direct  http.RoundTripper
	proxied http.RoundTripper
	proxy   func(*http.Request) (*url.URL, error)
	// resolvesPrivate 报告 host 的本地解析结果是否含非公网地址，仅用于经代理的请求。
	resolvesPrivate func(ctx context.Context, host string) (bool, error)
}

func (t *imageDownloadTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	next, err := t.route(req)
	if err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, err
	}
	return next.RoundTrip(req)
}

func (t *imageDownloadTransport) route(req *http.Request) (http.RoundTripper, error) {
	if req.URL == nil {
		return nil, errors.New("image url is empty")
	}
	host := req.URL.Hostname()
	if isBlockedImageDownloadHost(host) {
		return nil, fmt.Errorf("image url host is not allowed: %s", host)
	}
	var proxyURL *url.URL
	if t.proxy != nil {
		var err error
		if proxyURL, err = t.proxy(req); err != nil {
			return nil, err
		}
	}
	if proxyURL == nil {
		return t.direct, nil
	}
	if t.resolvesPrivate == nil {
		return nil, errors.New("image download proxy host check is not configured")
	}
	blocked, err := t.resolvesPrivate(req.Context(), host)
	if err != nil {
		return nil, fmt.Errorf("resolve image url host: %w", err)
	}
	if blocked {
		return nil, fmt.Errorf("image url host is not allowed: %s", host)
	}
	return t.proxied, nil
}

// isBlockedImageDownloadHost 报告 host 是否为空、被拒绝的主机名，或非公网 IP 字面量。
// 域名的解析结果由 socket 层或代理分支再校验。
func isBlockedImageDownloadHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if urlvalidator.IsBlockedHost(host) || isBlockedHostname(host) {
		return true
	}
	if ip := net.ParseIP(stripIPv6Zone(host)); ip != nil {
		return isPrivateIP(ip)
	}
	return false
}

// stripIPv6Zone 去掉 IPv6 字面量的 zone（如 fe80::1%eth0），使其可被 net.ParseIP 解析。
func stripIPv6Zone(host string) string {
	if i := strings.LastIndexByte(host, '%'); i >= 0 {
		return host[:i]
	}
	return host
}

// Rewrite 将 result（上游生图响应 JSON）里的每张图片转存到对象存储，
// 返回改写后的紧凑结果（data[i].url 指向对象存储，b64_json 被移除）。
// 任一图片转存失败即返回 error（调用方据此将任务标记为失败，绝不把大 blob 落 Redis）。
func (u *ImageResultUploader) Rewrite(ctx context.Context, taskID string, result json.RawMessage) (json.RawMessage, error) {
	if u == nil || u.storage == nil {
		return result, nil
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(result, &top); err != nil {
		return nil, fmt.Errorf("parse image response: %w", err)
	}
	rawData, ok := top["data"]
	if !ok {
		// 没有 data 数组（结构不符合预期），保持原样返回，交由上层决定。
		return result, nil
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(rawData, &items); err != nil {
		return nil, fmt.Errorf("parse image response data: %w", err)
	}
	if len(items) == 0 {
		return result, nil
	}
	for i, item := range items {
		data, contentType, err := u.fetchImageBytes(ctx, item)
		if err != nil {
			return nil, fmt.Errorf("image %d: %w", i, err)
		}
		key := u.buildKey(taskID, i, contentType)
		url, err := u.storage.Save(ctx, key, contentType, data)
		if err != nil {
			return nil, fmt.Errorf("image %d: upload to object storage: %w", i, err)
		}
		urlRaw, err := json.Marshal(url)
		if err != nil {
			return nil, fmt.Errorf("image %d: encode url: %w", i, err)
		}
		item["url"] = urlRaw
		delete(item, "b64_json")
		items[i] = item
	}
	newData, err := json.Marshal(items)
	if err != nil {
		return nil, fmt.Errorf("encode image response data: %w", err)
	}
	top["data"] = newData
	out, err := json.Marshal(top)
	if err != nil {
		return nil, fmt.Errorf("encode image response: %w", err)
	}
	return out, nil
}

func (u *ImageResultUploader) fetchImageBytes(ctx context.Context, item map[string]json.RawMessage) ([]byte, string, error) {
	if raw, ok := item["b64_json"]; ok {
		var b64 string
		if err := json.Unmarshal(raw, &b64); err == nil {
			if b64 = strings.TrimSpace(b64); b64 != "" {
				data, err := base64.StdEncoding.DecodeString(b64)
				if err != nil {
					return nil, "", fmt.Errorf("decode b64_json: %w", err)
				}
				return data, detectImageContentType(data), nil
			}
		}
	}
	if raw, ok := item["url"]; ok {
		var rawURL string
		if err := json.Unmarshal(raw, &rawURL); err == nil {
			if rawURL = strings.TrimSpace(rawURL); rawURL != "" {
				if len(rawURL) >= len("data:") && strings.EqualFold(rawURL[:len("data:")], "data:") {
					return u.decodeImageDataURL(rawURL)
				}
				return u.download(ctx, rawURL)
			}
		}
	}
	return nil, "", errors.New("image item has neither b64_json nor url")
}

func (u *ImageResultUploader) decodeImageDataURL(rawURL string) ([]byte, string, error) {
	header, payload, ok := strings.Cut(rawURL[len("data:"):], ",")
	if !ok {
		return nil, "", errors.New("decode image data URL: missing comma separator")
	}

	parts := strings.Split(header, ";")
	if strings.TrimSpace(parts[0]) == "" {
		return nil, "", errors.New("decode image data URL: missing media type")
	}
	base64Index := len(parts) - 1
	if base64Index < 1 || !strings.EqualFold(strings.TrimSpace(parts[base64Index]), "base64") {
		for i := 1; i < base64Index; i++ {
			if strings.EqualFold(strings.TrimSpace(parts[i]), "base64") {
				return nil, "", errors.New("decode image data URL: base64 marker must be the final header token")
			}
		}
		return nil, "", errors.New("decode image data URL: payload is not base64 encoded")
	}
	for i := 1; i < base64Index; i++ {
		if strings.EqualFold(strings.TrimSpace(parts[i]), "base64") {
			return nil, "", errors.New("decode image data URL: duplicate base64 marker")
		}
	}
	mediaTypeHeader := strings.Join(parts[:base64Index], ";")
	declaredType, _, err := mime.ParseMediaType(mediaTypeHeader)
	if err != nil {
		return nil, "", fmt.Errorf("decode image data URL: invalid media type: %w", err)
	}
	declaredType = strings.ToLower(declaredType)
	if !strings.HasPrefix(declaredType, "image/") {
		return nil, "", fmt.Errorf("decode image data URL: media type %q is not an image", declaredType)
	}

	limit := u.maxDownloadBytes
	if limit <= 0 {
		limit = defaultImageMaxDownloadBytes
	}
	decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(payload))
	data, err := io.ReadAll(io.LimitReader(decoder, limit+1))
	if err != nil {
		return nil, "", fmt.Errorf("decode image data URL base64 payload: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, "", fmt.Errorf("decoded image data URL exceeds %d bytes", limit)
	}

	contentType := detectedImageContentType(data)
	if contentType == "" {
		contentType = declaredType
	}
	return data, contentType, nil
}

func (u *ImageResultUploader) download(ctx context.Context, rawURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("build download request: %w", err)
	}
	resp, err := u.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("download image: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, "", fmt.Errorf("download image: unexpected status %d", resp.StatusCode)
	}
	limit := u.maxDownloadBytes
	if limit <= 0 {
		limit = defaultImageMaxDownloadBytes
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, "", fmt.Errorf("read image body: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, "", fmt.Errorf("downloaded image exceeds %d bytes", limit)
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	if !strings.HasPrefix(contentType, "image/") {
		// 声明与嗅探都不是图片时拒绝，避免把任意响应体（内部接口的 JSON/HTML 等）转存给租户。
		if contentType = detectedImageContentType(data); contentType == "" {
			return nil, "", errors.New("download image: content is not an image")
		}
	}
	return data, contentType, nil
}

func (u *ImageResultUploader) buildKey(taskID string, index int, contentType string) string {
	return u.prefix + taskID + "-" + strconv.Itoa(index) + extensionForContentType(contentType)
}

func detectImageContentType(data []byte) string {
	if ct := detectedImageContentType(data); ct != "" {
		return ct
	}
	return "image/png"
}

func detectedImageContentType(data []byte) string {
	ct := strings.TrimSpace(strings.Split(http.DetectContentType(data), ";")[0])
	if strings.HasPrefix(ct, "image/") {
		return ct
	}
	return ""
}

func extensionForContentType(ct string) string {
	switch {
	case strings.Contains(ct, "png"):
		return ".png"
	case strings.Contains(ct, "jpeg"), strings.Contains(ct, "jpg"):
		return ".jpg"
	case strings.Contains(ct, "webp"):
		return ".webp"
	case strings.Contains(ct, "gif"):
		return ".gif"
	default:
		return ".png"
	}
}
