package service

// models.dev 官方价格数据源（2026-08-08 fork 新增）
//
// 数据源：https://models.dev/api.json（免费公开，无需 key）
//   按 provider 收录全球各厂商「官方 API 价」+ 模型元数据（context/模态/推理等）。
//
// 设计：
//   - 价格 merge 为 additive 兜底：只补充 pricingData 中不存在的模型 key，
//     不覆盖 LiteLLM 主文件与 fallback JSON 已有价格（与 mergeFallbackPricingData 同理）。
//   - 每 30 分钟轮询（可配置 MODELSDEV_SYNC_INTERVAL_MINUTES），热更新，无需重启。
//   - 元数据（context_length / modalities）独立存储，供模型广场展示（Task B）。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// ModelsDevModel 单模型条目（models.dev api.json 结构）
type ModelsDevModel struct {
	ID         string `json:"id"`
	Reasoning  bool   `json:"reasoning"`
	ToolCall   bool   `json:"tool_call"`
	Modalities struct {
		Input  []string `json:"input"`
		Output []string `json:"output"`
	} `json:"modalities"`
	Limit struct {
		Context int64 `json:"context"`
		Output  int64 `json:"output"`
	} `json:"limit"`
	Cost struct {
		Input      float64 `json:"input"`       // $ / 1M tokens
		Output     float64 `json:"output"`      // $ / 1M tokens
		CacheRead  float64 `json:"cache_read"`  // $ / 1M tokens
		CacheWrite float64 `json:"cache_write"` // $ / 1M tokens
	} `json:"cost"`
}

// modelsDevProvider api.json 顶层：provider -> {models: {modelID: {...}}}
type modelsDevProvider struct {
	Models map[string]ModelsDevModel `json:"models"`
}

// ModelsDevClient models.dev 数据源客户端。
// 线程安全：所有读取通过 RLock，更新通过 Lock。
type ModelsDevClient struct {
	url        string
	httpClient *http.Client

	mu          sync.RWMutex
	data        map[string]ModelsDevModel // key = 模型 ID（小写归一）
	lastSyncAt  time.Time
	lastSyncErr string
}

// NewModelsDevClient 创建客户端。url 为空时禁用同步。
func NewModelsDevClient(url string) *ModelsDevClient {
	return &ModelsDevClient{
		url:        strings.TrimSpace(url),
		httpClient: &http.Client{Timeout: 30 * time.Second},
		data:       make(map[string]ModelsDevModel),
	}
}

// Enabled 数据源是否启用
func (c *ModelsDevClient) Enabled() bool {
	return c != nil && c.url != ""
}

// Sync 拉取并解析 models.dev api.json（全量替换 data，原子切换）。
// 跨 provider 同名模型：按 provider 名排序后先到先得（官方主 provider 优先）。
func (c *ModelsDevClient) Sync(ctx context.Context) error {
	if c == nil || !c.Enabled() {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return fmt.Errorf("modelsdev: build request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.mu.Lock()
		c.lastSyncErr = fmt.Sprintf("fetch: %v", err)
		c.mu.Unlock()
		return fmt.Errorf("modelsdev: fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.mu.Lock()
		c.lastSyncErr = fmt.Sprintf("status %d", resp.StatusCode)
		c.mu.Unlock()
		return fmt.Errorf("modelsdev: unexpected status %d", resp.StatusCode)
	}

	var providers map[string]modelsDevProvider
	if err := json.NewDecoder(resp.Body).Decode(&providers); err != nil {
		c.mu.Lock()
		c.lastSyncErr = fmt.Sprintf("decode: %v", err)
		c.mu.Unlock()
		return fmt.Errorf("modelsdev: decode: %w", err)
	}

	// provider 名排序保证确定性
	names := make([]string, 0, len(providers))
	for p := range providers {
		names = append(names, p)
	}
	sort.Strings(names)

	// 同一模型可能出现在多个 provider（官方 + 第三方聚合商，价格可能不同，
	// 如 302ai 对 MiniMax-M2.7-highspeed 收 $0.6/$4.8 而官方 $0.6/$2.4）。
	// 策略：收集全部条目，选「众数价格」组合（多数 provider 一致的价格 = 官方价），
	// 平局时取 provider 名排序靠前的条目。
	type priceKey struct{ in, out, cr, cw float64 }
	type entry struct {
		p   string
		m   ModelsDevModel
		pk  priceKey
	}
	byKey := make(map[string][]entry, len(providers)*8)
	for _, p := range names {
		for id, m := range providers[p].Models {
			key := strings.ToLower(strings.TrimSpace(id))
			if key == "" {
				continue
			}
			byKey[key] = append(byKey[key], entry{
				p: p,
				m: m,
				pk: priceKey{m.Cost.Input, m.Cost.Output, m.Cost.CacheRead, m.Cost.CacheWrite},
			})
		}
	}

	merged := make(map[string]ModelsDevModel, len(byKey))
	for key, entries := range byKey {
		// 统计众数：排除 0 价条目（coding-plan/token-plan 订阅套餐 provider
		// 的模型标 $0/$0，不是按量官方价，会污染众数）。
		zeroKey := priceKey{}
		counts := make(map[priceKey]int)
		firstPaid := -1
		for i, e := range entries {
			if e.pk == zeroKey {
				continue
			}
			if firstPaid < 0 {
				firstPaid = i
			}
			counts[e.pk]++
		}
		if firstPaid < 0 {
			// 全部 0 价（纯套餐/免费模型），取第一个条目
			merged[key] = entries[0].m
			continue
		}
		best := entries[firstPaid]
		bestCount := counts[best.pk]
		for i, e := range entries {
			if i == firstPaid {
				continue
			}
			if e.pk == zeroKey {
				continue
			}
			if counts[e.pk] > bestCount {
				best, bestCount = e, counts[e.pk]
			}
		}
		merged[key] = best.m
	}

	c.mu.Lock()
	c.data = merged
	c.lastSyncAt = time.Now()
	c.lastSyncErr = ""
	c.mu.Unlock()
	return nil
}

// Lookup 按模型名查元数据（大小写不敏感）
func (c *ModelsDevClient) Lookup(modelName string) (ModelsDevModel, bool) {
	if c == nil || modelName == "" {
		return ModelsDevModel{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	m, ok := c.data[strings.ToLower(strings.TrimSpace(modelName))]
	return m, ok
}

// Count 已加载模型数
func (c *ModelsDevClient) Count() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.data)
}

// LastSync 最近同步时间与错误
func (c *ModelsDevClient) LastSync() (time.Time, string) {
	if c == nil {
		return time.Time{}, "disabled"
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastSyncAt, c.lastSyncErr
}

// ModelsDevModelPricing 将 models.dev 价格转换为 LiteLLM 格式（additive 兜底）。
// models.dev 的 cost 单位为 $/1M tokens → 换算为 $/token。
func ModelsDevModelPricing(m ModelsDevModel) *LiteLLMModelPricing {
	p := &LiteLLMModelPricing{
		Mode: "chat",
	}
	if m.Cost.Input > 0 {
		p.InputCostPerToken = m.Cost.Input / 1e6
	}
	if m.Cost.Output > 0 {
		p.OutputCostPerToken = m.Cost.Output / 1e6
	}
	if m.Cost.CacheRead > 0 {
		p.CacheReadInputTokenCost = m.Cost.CacheRead / 1e6
	}
	if m.Cost.CacheWrite > 0 {
		p.CacheCreationInputTokenCost = m.Cost.CacheWrite / 1e6
	}
	p.LiteLLMProvider = "models.dev"
	if p.InputCostPerToken == 0 && p.OutputCostPerToken == 0 {
		p.TokenPricingAbsent = true
	}
	return p
}
