package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tidwall/gjson"
)

// 上游模型目录中的协议能力。部分聚合平台（Command Code）在 {Chat Completions 基址}/models
// 的每个模型上给出 supported_endpoints，例如：
//
//	{"id":"deepseek/deepseek-v4-flash","supported_endpoints":["/chat/completions","/responses"]}
//	{"id":"claude-sonnet-4-6","supported_endpoints":["/messages"]}
//
// 按模型分流时据此判断模型是否支持入站协议：支持就同协议直通，省去一次协议转换
// （见 modelProtocolSet）。
//
// 缓存范围：所有目录均按账号和地址隔离，包括官方默认地址。目录可能随 Key、租户
// 或请求头不同，鉴权失败的刷新退避也不能影响其他账号（见 modelProtocolCatalogKey）。
//
// 刷新在后台进行：过期时继续使用旧目录。目录从未加载成功时，首批请求最多等待
// modelProtocolCatalogFirstLoadWait，各请求共用本轮刷新开始时的截止时间，并响应请求
// 取消。超时后同一轮刷新中的后续请求立即回落内置规则，失败退避期内不再等待。

const (
	modelProtocolCatalogTTL           = 30 * time.Minute
	modelProtocolCatalogRetryBackoff  = 5 * time.Minute
	modelProtocolCatalogTimeout       = 15 * time.Second
	modelProtocolCatalogFirstLoadWait = 2 * time.Second
	modelProtocolCatalogMaxBodyBytes  = 2 << 20
)

type modelProtocolCatalogEntry struct {
	// models 以小写模型 ID 为键，值为上游声明的协议（保持上游顺序）。
	models     map[string][]string
	fetchedAt  time.Time
	retryAt    time.Time
	refreshing bool
	// done 在进行中的刷新结束时关闭。
	done chan struct{}
	// firstLoadDeadline 是本轮冷启动刷新共享的等待截止时间。
	firstLoadDeadline time.Time
}

type modelProtocolCatalog struct {
	mu      sync.Mutex
	entries map[string]*modelProtocolCatalogEntry
}

// upstreamModelProtocols 是全进程共享的模型协议目录。
var upstreamModelProtocols modelProtocolCatalog

func (c *modelProtocolCatalog) entryLocked(key string) *modelProtocolCatalogEntry {
	if c.entries == nil {
		c.entries = make(map[string]*modelProtocolCatalogEntry)
	}
	entry := c.entries[key]
	if entry == nil {
		entry = &modelProtocolCatalogEntry{}
		c.entries[key] = entry
	}
	return entry
}

// lookup 返回 key 目录中 model 支持的协议；目录尚未就绪或模型不在目录中时返回 nil。
// 目录缺失或过期、且不在刷新中也不在失败退避期时，异步调用 refresh（由其调用 store）；
// 目录从未加载成功且刷新进行中时，只等待本轮冷启动窗口的剩余时间。
func (c *modelProtocolCatalog) lookup(ctx context.Context, key, model string, now time.Time, refresh func()) []string {
	if ctx.Err() != nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.entryLocked(key)
	stale := entry.models == nil || now.Sub(entry.fetchedAt) >= modelProtocolCatalogTTL
	if stale && !entry.refreshing && !now.Before(entry.retryAt) && refresh != nil {
		entry.refreshing = true
		entry.done = make(chan struct{})
		if entry.models == nil {
			entry.firstLoadDeadline = now.Add(modelProtocolCatalogFirstLoadWait)
		}
		go refresh()
	}
	if entry.models == nil && entry.refreshing && now.Before(entry.firstLoadDeadline) {
		done := entry.done
		deadline := entry.firstLoadDeadline
		c.mu.Unlock()
		timer := time.NewTimer(time.Until(deadline))
		select {
		case <-done:
		case <-timer.C:
		case <-ctx.Done():
		}
		timer.Stop()
		c.mu.Lock()
	}
	if entry.models == nil || ctx.Err() != nil {
		return nil
	}
	return entry.models[strings.ToLower(strings.TrimSpace(model))]
}

// store 写入一次刷新的结果；失败时保留旧目录并进入退避。
func (c *modelProtocolCatalog) store(key string, models map[string][]string, err error, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.entryLocked(key)
	entry.refreshing = false
	if entry.done != nil {
		close(entry.done)
		entry.done = nil
	}
	if err != nil {
		entry.retryAt = now.Add(modelProtocolCatalogRetryBackoff)
		return
	}
	entry.models = models
	entry.fetchedAt = now
	entry.retryAt = time.Time{}
}

// modelEndpointProtocol 把 supported_endpoints 中的端点路径映射为上游协议。
func modelEndpointProtocol(endpoint string) string {
	endpoint = strings.ToLower(strings.TrimRight(strings.TrimSpace(endpoint), "/"))
	switch {
	case strings.HasSuffix(endpoint, "/chat/completions"):
		return APIProtocolChatCompletions
	case strings.HasSuffix(endpoint, "/responses"):
		return APIProtocolResponses
	case strings.HasSuffix(endpoint, "/messages"):
		return APIProtocolAnthropic
	default:
		return ""
	}
}

// parseModelProtocolCatalog 解析 /models 响应；没有任何模型带可识别的 supported_endpoints
// 时返回错误（上游不提供该字段，或是中转站的普通 /models）。
func parseModelProtocolCatalog(body []byte) (map[string][]string, error) {
	entries, err := extractUpstreamModelRawEntries(body)
	if err != nil {
		return nil, err
	}
	models := make(map[string][]string)
	for _, raw := range entries {
		id := strings.ToLower(strings.TrimSpace(gjson.GetBytes(raw, "id").String()))
		if id == "" {
			continue
		}
		var protocols []string
		for _, endpoint := range gjson.GetBytes(raw, "supported_endpoints").Array() {
			protocol := modelEndpointProtocol(endpoint.String())
			if protocol != "" && !slices.Contains(protocols, protocol) {
				protocols = append(protocols, protocol)
			}
		}
		if len(protocols) > 0 {
			models[id] = protocols
		}
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("model list has no supported_endpoints")
	}
	return models, nil
}

// modelCatalogProtocols 返回上游模型目录中 model 支持的协议；平台不提供目录、账号固定了
// 上游协议或目录尚未就绪时返回 nil。
func (s *OpenAIGatewayService) modelCatalogProtocols(ctx context.Context, account *Account, model string) []string {
	if s == nil || s.httpUpstream == nil || strings.TrimSpace(model) == "" || !account.routesByModel() {
		return nil
	}
	if profile := account.providerProfile(); profile == nil || !profile.ModelCatalog {
		return nil
	}
	if account.GetAPIProtocol() != APIProtocolAdaptive {
		return nil
	}
	base := strings.TrimSpace(account.GetCNProtocolBaseURL(APIProtocolChatCompletions))
	if base == "" {
		return nil
	}
	url := buildOpenAIModelsURL(base)
	key := modelProtocolCatalogKey(account, url)
	headers := modelProtocolCatalogHeaders(account, url)
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	accountID, concurrency := account.ID, account.Concurrency
	return upstreamModelProtocols.lookup(ctx, key, model, time.Now(), func() {
		ctx, cancel := context.WithTimeout(context.Background(), modelProtocolCatalogTimeout)
		defer cancel()
		models, err := s.fetchModelProtocolCatalog(ctx, url, headers, proxyURL, accountID, concurrency)
		if err != nil {
			slog.Warn("model_protocol_catalog_refresh_failed", "account_id", accountID, "url", url, "error", err)
		} else {
			slog.Info("model_protocol_catalog_refreshed", "account_id", accountID, "url", url, "models", len(models))
		}
		upstreamModelProtocols.store(key, models, err, time.Now())
	})
}

// modelProtocolCatalogKey 按账号和地址隔离目录及刷新退避，避免一个账号的目录或
// 鉴权失败影响同地址的其他账号。
func modelProtocolCatalogKey(account *Account, url string) string {
	return fmt.Sprintf("%s#account=%d", url, account.ID)
}

// modelProtocolCatalogHeaders 构造目录请求头，UA 收敛与账号请求头覆写与转发一致
// （见 buildUpstreamRequest）。在请求时构造，后台刷新不再读取账号。
func modelProtocolCatalogHeaders(account *Account, url string) http.Header {
	headers := http.Header{}
	headers.Set("Accept", "application/json")
	if apiKey := strings.TrimSpace(account.GetOpenAIProtocolAPIKey()); apiKey != "" {
		headers.Set("Authorization", "Bearer "+apiKey)
	}
	applyOpenCodeUpstreamUserAgent(account, url, headers)
	account.ApplyHeaderOverrides(headers)
	return headers
}

func (s *OpenAIGatewayService) fetchModelProtocolCatalog(ctx context.Context, url string, headers http.Header, proxyURL string, accountID int64, concurrency int) (map[string][]string, error) {
	validatedURL, err := s.validateUpstreamBaseURL(url)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(WithHTTPUpstreamRedirectsDisabled(ctx), http.MethodGet, validatedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header = headers.Clone()
	resp, err := s.httpUpstream.Do(req, proxyURL, accountID, maxInt(concurrency, 1))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("model list returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, modelProtocolCatalogMaxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > modelProtocolCatalogMaxBodyBytes {
		return nil, fmt.Errorf("model list exceeds %d bytes", modelProtocolCatalogMaxBodyBytes)
	}
	return parseModelProtocolCatalog(body)
}

// resolveUpstreamProtocolFor 是网关入口使用的 resolveUpstreamProtocol：按模型分流且平台
// 提供模型目录时，带上目录中该模型支持的协议。
func (s *OpenAIGatewayService) resolveUpstreamProtocolFor(ctx context.Context, account *Account, inbound, model string) string {
	return resolveUpstreamProtocol(account, inbound, model, s.modelCatalogProtocols(ctx, account, model))
}
