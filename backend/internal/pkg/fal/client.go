package fal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyutil"
)

const (
	// proxyDialTimeout 代理 TCP 连接超时（含代理握手）。
	proxyDialTimeout = 10 * time.Second
	// proxyTLSHandshakeTimeout 代理 TLS 握手超时。
	proxyTLSHandshakeTimeout = 10 * time.Second
	// defaultClientTimeout 单次 HTTP 请求整体超时。
	// 注意：sync 协议（阻塞出图）可能耗时较久，调用方可用 ctx 控制实际等待。
	defaultClientTimeout = 120 * time.Second
	// bodyLimit 限制响应体大小，避免无界内存占用。
	bodyLimit int64 = 32 << 20
	// falPlatformModelsURL 是 fal 平台模型列表 API（固定域名，独立于 queue/sync）。
	// 文档：https://fal.ai/docs/platform-apis/v1/models
	falPlatformModelsURL = "https://api.fal.ai/v1/models"
	// falModelsPageLimit 是模型列表分页每页条数。
	falModelsPageLimit = 1000
	// falModelsMaxPages 是分页遍历的安全上限（防止上游异常导致无界循环）。
	falModelsMaxPages = 50
	// falModelsSearchLimit 是关键词搜索默认返回条数（单页，不翻页）。
	falModelsSearchLimit = 50
)

// APIError 表示 fal 上游返回的非 2xx 响应。
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("fal upstream error (HTTP %d): %s", e.StatusCode, e.Body)
}

// Client 是 fal.ai 的 HTTP 客户端，封装 queue（异步）与 sync（同步）两套协议。
type Client struct {
	httpClient   *http.Client
	apiKey       string
	queueBaseURL string
	syncBaseURL  string
}

// Config 构造 fal 客户端所需的配置。
type Config struct {
	APIKey       string // FAL_KEY（必填）
	QueueBaseURL string // 默认 https://queue.fal.run
	SyncBaseURL  string // 默认 https://fal.run
	ProxyURL     string // 可选代理
	Timeout      time.Duration
}

// NewClient 创建 fal 客户端。
func NewClient(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("fal: api key is required")
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultClientTimeout
	}
	client := &http.Client{Timeout: timeout}

	_, parsed, err := proxyurl.Parse(cfg.ProxyURL)
	if err != nil {
		return nil, err
	}
	if parsed != nil {
		transport := &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: proxyDialTimeout,
			}).DialContext,
			TLSHandshakeTimeout: proxyTLSHandshakeTimeout,
		}
		if err := proxyutil.ConfigureTransportProxy(transport, parsed); err != nil {
			return nil, fmt.Errorf("fal: configure proxy: %w", err)
		}
		client.Transport = transport
	}

	queueBase := strings.TrimRight(strings.TrimSpace(cfg.QueueBaseURL), "/")
	if queueBase == "" {
		queueBase = "https://queue.fal.run"
	}
	syncBase := strings.TrimRight(strings.TrimSpace(cfg.SyncBaseURL), "/")
	if syncBase == "" {
		syncBase = "https://fal.run"
	}

	return &Client{
		httpClient:   client,
		apiKey:       strings.TrimSpace(cfg.APIKey),
		queueBaseURL: queueBase,
		syncBaseURL:  syncBase,
	}, nil
}

// Submit 向 queue 协议提交一个异步任务，返回 request_id 及各 url。
//
// POST {queueBaseURL}/{model}
func (c *Client) Submit(ctx context.Context, model string, body *Request) (*SubmitResponse, error) {
	endpoint := fmt.Sprintf("%s/%s", c.queueBaseURL, strings.TrimLeft(model, "/"))
	var out SubmitResponse
	if err := c.doJSON(ctx, http.MethodPost, endpoint, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Status 查询任务状态。优先使用 submit 返回的 status_url，否则按约定拼接。
//
// GET {queueBaseURL}/{model}/requests/{id}/status
func (c *Client) Status(ctx context.Context, statusURL string) (*StatusResponse, error) {
	if strings.TrimSpace(statusURL) == "" {
		return nil, errors.New("fal: status url is empty")
	}
	var out StatusResponse
	if err := c.doJSON(ctx, http.MethodGet, statusURL, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Result 获取任务最终结果（出图）。优先使用 submit/status 返回的 response_url。
//
// GET {queueBaseURL}/{model}/requests/{id}
func (c *Client) Result(ctx context.Context, responseURL string) (*Response, error) {
	if strings.TrimSpace(responseURL) == "" {
		return nil, errors.New("fal: response url is empty")
	}
	var out Response
	if err := c.doJSON(ctx, http.MethodGet, responseURL, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Cancel 取消一个排队中的任务。
//
// PUT {queueBaseURL}/{model}/requests/{id}/cancel
func (c *Client) Cancel(ctx context.Context, cancelURL string) error {
	if strings.TrimSpace(cancelURL) == "" {
		return errors.New("fal: cancel url is empty")
	}
	return c.doJSON(ctx, http.MethodPut, cancelURL, nil, nil)
}

// Sync 以同步（阻塞）协议直接出图。
//
// POST {syncBaseURL}/{model}
func (c *Client) Sync(ctx context.Context, model string, body *Request) (*Response, error) {
	endpoint := fmt.Sprintf("%s/%s", c.syncBaseURL, strings.TrimLeft(model, "/"))
	var out Response
	if err := c.doJSON(ctx, http.MethodPost, endpoint, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// FetchModels 拉取 fal 平台支持的模型（endpoint_id）列表。
//
// GET https://api.fal.ai/v1/models?status=active&limit=N[&cursor=...]
//
// 接入域名固定为 api.fal.ai，与 queue/sync base url 无关；按 next_cursor/has_more
// 翻页直至取完，并对返回的 endpoint_id 去重。
func (c *Client) FetchModels(ctx context.Context) ([]string, error) {
	seen := make(map[string]struct{})
	models := make([]string, 0, 256)
	cursor := ""

	overallStart := time.Now()
	pageCount := 0

	for range falModelsMaxPages {
		endpoint, err := buildFalModelsURL(falPlatformModelsURL, cursor, falModelsPageLimit)
		if err != nil {
			return nil, err
		}

		pageCount++
		slog.Debug("fal_fetch_models_page_start",
			"page", pageCount,
			"cursor", cursor,
			"endpoint", endpoint,
		)
		pageStart := time.Now()

		var out ModelsResponse
		if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &out); err != nil {
			slog.Warn("fal_fetch_models_page_failed",
				"page", pageCount,
				"elapsed_ms", time.Since(pageStart).Milliseconds(),
				"error", err.Error(),
			)
			return nil, err
		}

		newOnPage := 0
		for _, m := range out.Models {
			id := strings.TrimSpace(m.EndpointID)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			models = append(models, id)
			newOnPage++
		}

		slog.Debug("fal_fetch_models_page_done",
			"page", pageCount,
			"elapsed_ms", time.Since(pageStart).Milliseconds(),
			"page_models", len(out.Models),
			"new_models", newOnPage,
			"has_more", out.HasMore,
		)

		next := strings.TrimSpace(out.NextCursor)
		if !out.HasMore || next == "" {
			break
		}
		cursor = next
	}

	slog.Debug("fal_fetch_models_done",
		"pages", pageCount,
		"total_models", len(models),
		"elapsed_ms", time.Since(overallStart).Milliseconds(),
	)

	return models, nil
}

// SearchModels 按关键词搜索 fal 平台支持的模型（endpoint_id）列表。
//
// GET https://api.fal.ai/v1/models?status=active&q={query}&limit=N
//
// 与 FetchModels 不同，这里只取第一页（带 q 过滤后结果已足够小），用于
// 模型白名单输入框的即时搜索，避免全量翻页带来的高延迟。
func (c *Client) SearchModels(ctx context.Context, query string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = falModelsSearchLimit
	}

	endpoint, err := buildFalModelsSearchURL(falPlatformModelsURL, strings.TrimSpace(query), limit)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	var out ModelsResponse
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &out); err != nil {
		slog.Warn("fal_search_models_failed",
			"query", query,
			"elapsed_ms", time.Since(start).Milliseconds(),
			"error", err.Error(),
		)
		return nil, err
	}

	seen := make(map[string]struct{}, len(out.Models))
	models := make([]string, 0, len(out.Models))
	for _, m := range out.Models {
		id := strings.TrimSpace(m.EndpointID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		models = append(models, id)
	}

	slog.Debug("fal_search_models_done",
		"query", query,
		"models", len(models),
		"elapsed_ms", time.Since(start).Milliseconds(),
	)

	return models, nil
}

// buildFalModelsURL 拼接带分页/过滤参数的模型列表 URL。
func buildFalModelsURL(base, cursor string, limit int) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("fal: parse models url: %w", err)
	}
	q := u.Query()
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	q.Set("status", "active")
	if strings.TrimSpace(cursor) != "" {
		q.Set("cursor", cursor)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// buildFalModelsSearchURL 拼接带关键词搜索参数（q）的模型列表 URL。
func buildFalModelsSearchURL(base, query string, limit int) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("fal: parse models url: %w", err)
	}
	q := u.Query()
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	q.Set("status", "active")
	if strings.TrimSpace(query) != "" {
		q.Set("q", query)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// BuildStatusURL 按 queue 协议约定拼接 status url（当 submit 未返回时回退使用）。
func (c *Client) BuildStatusURL(model, requestID string) string {
	return fmt.Sprintf("%s/%s/requests/%s/status", c.queueBaseURL, strings.TrimLeft(model, "/"), requestID)
}

// BuildResponseURL 按 queue 协议约定拼接 result url。
func (c *Client) BuildResponseURL(model, requestID string) string {
	return fmt.Sprintf("%s/%s/requests/%s", c.queueBaseURL, strings.TrimLeft(model, "/"), requestID)
}

// BuildCancelURL 按 queue 协议约定拼接 cancel url。
func (c *Client) BuildCancelURL(model, requestID string) string {
	return fmt.Sprintf("%s/%s/requests/%s/cancel", c.queueBaseURL, strings.TrimLeft(model, "/"), requestID)
}

// doJSON 执行一次 HTTP 请求，序列化 reqBody（可为 nil）并将响应解码到 out（可为 nil）。
func (c *Client) doJSON(ctx context.Context, method, endpoint string, reqBody, out any) error {
	var bodyReader io.Reader
	if reqBody != nil {
		raw, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("fal: marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, bodyReader)
	if err != nil {
		return fmt.Errorf("fal: build request: %w", err)
	}
	req.Header.Set("Authorization", "Key "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// debug：打印向 fal 上游发起的请求 header 与包体，便于排查鉴权/参数问题。
	// Authorization 含 FAL_KEY，做掩码避免日志泄露密钥。
	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		var reqBodyStr string
		if bodyReader != nil {
			if raw, err := json.Marshal(reqBody); err == nil {
				reqBodyStr = string(raw)
			}
		}
		slog.Debug("fal_http_request_dump",
			"method", method,
			"endpoint", endpoint,
			"headers", maskedHeaderString(req.Header),
			"body", reqBodyStr,
		)
	}

	doStart := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		slog.Warn("fal_http_do_failed",
			"method", method,
			"endpoint", endpoint,
			"elapsed_ms", time.Since(doStart).Milliseconds(),
			"error", err.Error(),
		)
		return fmt.Errorf("fal: do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	// do_ms 仅覆盖到收到响应头（含 DNS/建连/TLS/首字节），不含响应体读取。
	doMS := time.Since(doStart).Milliseconds()

	readStart := time.Now()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, bodyLimit))
	if err != nil {
		return fmt.Errorf("fal: read response: %w", err)
	}
	slog.Debug("fal_http_request",
		"method", method,
		"endpoint", endpoint,
		"status", resp.StatusCode,
		"do_ms", doMS,
		"read_body_ms", time.Since(readStart).Milliseconds(),
		"body_bytes", len(raw),
	)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Body: string(raw)}
	}

	if out == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("fal: decode response: %w", err)
	}
	return nil
}

// maskedHeaderString 将请求 header 拼成可读字符串，并对敏感头（Authorization）做掩码，
// 仅保留密钥前后少量字符用于辨识，避免 debug 日志泄露完整 FAL_KEY。
func maskedHeaderString(h http.Header) string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		for _, v := range h[k] {
			if b.Len() > 0 {
				b.WriteString("; ")
			}
			b.WriteString(k)
			b.WriteString(": ")
			if strings.EqualFold(k, "Authorization") {
				b.WriteString(maskSecret(v))
			} else {
				b.WriteString(v)
			}
		}
	}
	return b.String()
}

// maskSecret 对密钥串做掩码，保留前缀与尾部少量字符。
func maskSecret(v string) string {
	v = strings.TrimSpace(v)
	// 保留 scheme 前缀（如 "Key "），仅掩码其后的密钥本体。
	prefix := ""
	secret := v
	if idx := strings.IndexByte(v, ' '); idx >= 0 {
		prefix = v[:idx+1]
		secret = v[idx+1:]
	}
	if len(secret) <= 8 {
		return prefix + "****"
	}
	return prefix + secret[:4] + "****" + secret[len(secret)-4:]
}

// IsConnectionError 判断是否为网络连接错误（超时、DNS、连接拒绝）。
func IsConnectionError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	var urlErr *url.Error
	return errors.As(err, &urlErr)
}
