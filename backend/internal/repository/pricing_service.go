package repository

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const (
	pricingHashAttemptTimeout  = 5 * time.Second
	pricingJSONAttemptTimeout  = 10 * time.Second
	pricingProxyCandidateLimit = 3
)

type pricingProxyLister interface {
	ListActive(ctx context.Context) ([]service.Proxy, error)
}

type pricingLatencyReader interface {
	GetProxyLatencies(ctx context.Context, proxyIDs []int64) (map[int64]*service.ProxyLatencyInfo, error)
}

type pricingHTTPClientFactory func(proxyURL string) (*http.Client, error)

type pricingRemoteClient struct {
	httpClient    *http.Client
	proxyRepo     pricingProxyLister
	latencyCache  pricingLatencyReader
	clientFactory pricingHTTPClientFactory

	selectedMu      sync.RWMutex
	selectedClient  *http.Client
	selectedProxyID int64
	fallbackMu      sync.Mutex
}

type pricingHTTPError struct {
	statusCode int
}

func (e *pricingHTTPError) Error() string {
	return fmt.Sprintf("HTTP %d", e.statusCode)
}

type pricingProxyCandidate struct {
	proxy     service.Proxy
	latencyMs int64
	updatedAt time.Time
}

// pricingRemoteClientError 代理初始化失败时的错误占位客户端
// 所有请求直接返回初始化错误，禁止回退到直连
type pricingRemoteClientError struct {
	err error
}

func (c *pricingRemoteClientError) FetchPricingJSON(_ context.Context, _ string) ([]byte, error) {
	return nil, c.err
}

func (c *pricingRemoteClientError) FetchHashText(_ context.Context, _ string) (string, error) {
	return "", c.err
}

// NewPricingRemoteClient 创建定价数据远程客户端
// proxyURL 为空时直连，支持 http/https/socks5/socks5h 协议
// 代理配置失败时行为由 allowDirectOnProxyError 控制：
//   - false（默认）：返回错误占位客户端，禁止回退到直连
//   - true：回退到直连（仅限管理员显式开启）
func NewPricingRemoteClient(proxyURL string, allowDirectOnProxyError bool) service.PricingRemoteClient {
	return newPricingRemoteClient(proxyURL, allowDirectOnProxyError, nil, nil)
}

// NewManagedPricingRemoteClient 创建支持 IP 管理代理自动回退的定价客户端。
// 仅当 update.proxy_url 为空时启用：首次直连失败后，按最近一次成功测试的延迟
// 从低到高尝试最多三个有效代理，并在当前实例内复用首个可用代理。
func NewManagedPricingRemoteClient(
	proxyURL string,
	allowDirectOnProxyError bool,
	proxyRepo service.ProxyRepository,
	latencyCache service.ProxyLatencyCache,
) service.PricingRemoteClient {
	return newPricingRemoteClient(proxyURL, allowDirectOnProxyError, proxyRepo, latencyCache)
}

func newPricingRemoteClient(
	proxyURL string,
	allowDirectOnProxyError bool,
	proxyRepo pricingProxyLister,
	latencyCache pricingLatencyReader,
) service.PricingRemoteClient {
	// 安全说明：httpclient.GetClient 的错误链（url.Parse / proxyutil）不含明文代理凭据，
	// 但仍通过 slog 仅在服务端日志记录，不会暴露给 HTTP 响应。
	clientFactory := func(candidateProxyURL string) (*http.Client, error) {
		return httpclient.GetClient(httpclient.Options{
			Timeout:  30 * time.Second,
			ProxyURL: candidateProxyURL,
		})
	}
	sharedClient, err := clientFactory(proxyURL)
	if err != nil {
		if strings.TrimSpace(proxyURL) != "" && !allowDirectOnProxyError {
			slog.Warn("proxy client init failed, all requests will fail", "service", "pricing", "error", err)
			return &pricingRemoteClientError{err: fmt.Errorf("proxy client init failed and direct fallback is disabled; set security.proxy_fallback.allow_direct_on_error=true to allow fallback: %w", err)}
		}
		sharedClient = &http.Client{Timeout: 30 * time.Second}
	}

	client := &pricingRemoteClient{
		httpClient:    sharedClient,
		clientFactory: clientFactory,
	}
	// 管理代理只在管理员没有显式配置 update.proxy_url 时参与自动选择。
	if strings.TrimSpace(proxyURL) == "" {
		client.proxyRepo = proxyRepo
		client.latencyCache = latencyCache
	}
	return client
}

func (c *pricingRemoteClient) FetchPricingJSON(ctx context.Context, url string) ([]byte, error) {
	return c.fetch(ctx, url, pricingJSONAttemptTimeout)
}

func (c *pricingRemoteClient) FetchHashText(ctx context.Context, url string) (string, error) {
	body, err := c.fetch(ctx, url, pricingHashAttemptTimeout)
	if err != nil {
		return "", err
	}

	// 哈希文件格式：hash  filename 或者纯 hash
	hash := strings.TrimSpace(string(body))
	parts := strings.Fields(hash)
	if len(parts) > 0 {
		return parts[0], nil
	}
	return hash, nil
}

func (c *pricingRemoteClient) fetch(ctx context.Context, url string, attemptTimeout time.Duration) ([]byte, error) {
	client, proxyID := c.currentClient()
	timeout := time.Duration(0)
	if c.managedFallbackEnabled() {
		timeout = attemptTimeout
	}
	body, err := fetchPricingURL(ctx, client, url, timeout)
	if err == nil || !c.managedFallbackEnabled() || !isRetryablePricingError(err) || ctx.Err() != nil {
		return body, err
	}

	return c.retryWithManagedProxy(ctx, url, attemptTimeout, client, proxyID, err)
}

func fetchPricingURL(ctx context.Context, client *http.Client, url string, timeout time.Duration) ([]byte, error) {
	requestCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, &pricingHTTPError{statusCode: resp.StatusCode}
	}

	return io.ReadAll(resp.Body)
}

func (c *pricingRemoteClient) managedFallbackEnabled() bool {
	return c != nil && c.proxyRepo != nil && c.latencyCache != nil && c.clientFactory != nil
}

func (c *pricingRemoteClient) currentClient() (*http.Client, int64) {
	c.selectedMu.RLock()
	defer c.selectedMu.RUnlock()
	if c.selectedClient != nil {
		return c.selectedClient, c.selectedProxyID
	}
	return c.httpClient, 0
}

func (c *pricingRemoteClient) setSelectedProxy(client *http.Client, proxy service.Proxy) {
	c.selectedMu.Lock()
	c.selectedClient = client
	c.selectedProxyID = proxy.ID
	c.selectedMu.Unlock()
}

func (c *pricingRemoteClient) clearSelectedProxy(proxyID int64) {
	c.selectedMu.Lock()
	if c.selectedProxyID == proxyID {
		c.selectedClient = nil
		c.selectedProxyID = 0
	}
	c.selectedMu.Unlock()
}

func (c *pricingRemoteClient) retryWithManagedProxy(
	ctx context.Context,
	url string,
	attemptTimeout time.Duration,
	failedClient *http.Client,
	failedProxyID int64,
	initialErr error,
) ([]byte, error) {
	c.fallbackMu.Lock()
	defer c.fallbackMu.Unlock()

	// 另一个并发请求可能已经完成了线路切换，直接复用它。
	currentClient, currentProxyID := c.currentClient()
	if currentClient != failedClient {
		return fetchPricingURL(ctx, currentClient, url, attemptTimeout)
	}

	excluded := make(map[int64]struct{})
	if failedProxyID != 0 {
		excluded[failedProxyID] = struct{}{}
		c.clearSelectedProxy(failedProxyID)

		// 已选代理失效时先确认直连是否已经恢复。
		body, directErr := fetchPricingURL(ctx, c.httpClient, url, attemptTimeout)
		if directErr == nil {
			slog.Info("pricing connection returned to direct access", "service", "pricing", "previous_proxy_id", currentProxyID)
			return body, nil
		}
		if !isRetryablePricingError(directErr) || ctx.Err() != nil {
			return nil, directErr
		}
	}

	candidates, err := c.listManagedProxyCandidates(ctx, excluded)
	if err != nil {
		return nil, errors.Join(initialErr, fmt.Errorf("list managed proxy candidates: %w", err))
	}

	lastErr := initialErr
	for _, candidate := range candidates {
		client, buildErr := c.clientFactory(candidate.proxy.URL())
		if buildErr != nil {
			lastErr = fmt.Errorf("build managed proxy client for proxy %d: %w", candidate.proxy.ID, buildErr)
			continue
		}

		body, requestErr := fetchPricingURL(ctx, client, url, attemptTimeout)
		if requestErr == nil {
			c.setSelectedProxy(client, candidate.proxy)
			slog.Info("pricing managed proxy selected",
				"service", "pricing",
				"proxy_id", candidate.proxy.ID,
				"proxy_name", candidate.proxy.Name,
				"latency_ms", candidate.latencyMs,
			)
			return body, nil
		}
		lastErr = requestErr
		if !isRetryablePricingError(requestErr) || ctx.Err() != nil {
			break
		}
	}

	return nil, lastErr
}

func (c *pricingRemoteClient) listManagedProxyCandidates(ctx context.Context, excluded map[int64]struct{}) ([]pricingProxyCandidate, error) {
	proxies, err := c.proxyRepo.ListActive(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	eligible := make([]service.Proxy, 0, len(proxies))
	ids := make([]int64, 0, len(proxies))
	for i := range proxies {
		proxy := proxies[i]
		if !proxy.IsActive() || proxy.IsExpired(now) {
			continue
		}
		if _, skip := excluded[proxy.ID]; skip {
			continue
		}
		eligible = append(eligible, proxy)
		ids = append(ids, proxy.ID)
	}

	latencies, err := c.latencyCache.GetProxyLatencies(ctx, ids)
	if err != nil {
		return nil, err
	}

	candidates := make([]pricingProxyCandidate, 0, len(eligible))
	for i := range eligible {
		proxy := eligible[i]
		info := latencies[proxy.ID]
		if info == nil || !info.Success || info.LatencyMs == nil {
			continue
		}
		candidates = append(candidates, pricingProxyCandidate{
			proxy:     proxy,
			latencyMs: *info.LatencyMs,
			updatedAt: info.UpdatedAt,
		})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].latencyMs != candidates[j].latencyMs {
			return candidates[i].latencyMs < candidates[j].latencyMs
		}
		if !candidates[i].updatedAt.Equal(candidates[j].updatedAt) {
			return candidates[i].updatedAt.After(candidates[j].updatedAt)
		}
		return candidates[i].proxy.ID < candidates[j].proxy.ID
	})
	if len(candidates) > pricingProxyCandidateLimit {
		candidates = candidates[:pricingProxyCandidateLimit]
	}
	return candidates, nil
}

func isRetryablePricingError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	var httpErr *pricingHTTPError
	if errors.As(err, &httpErr) {
		return httpErr.statusCode == http.StatusRequestTimeout ||
			httpErr.statusCode == http.StatusForbidden ||
			httpErr.statusCode == http.StatusTooManyRequests ||
			httpErr.statusCode >= http.StatusInternalServerError
	}
	return true
}
