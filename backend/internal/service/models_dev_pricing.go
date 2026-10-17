package service

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	// modelsDevPricingTTL 与 upstream_models.go 的 metadata 同步同周期：
	// models.dev 数据低频变化，6 小时足够新鲜。
	modelsDevPricingTTL = 6 * time.Hour
	// modelsDevPricingRetryDelay 拉取失败退避窗口：窗口内跳过 models.dev 段，
	// 避免计费热路径每次请求都撞一个注定失败的远程拉取。
	modelsDevPricingRetryDelay = 60 * time.Second
	// modelsDevPricingTimeout 单次拉取超时；registry 较大（MB 级），给足时间。
	modelsDevPricingTimeout = 15 * time.Second
	// modelsDevPricingBodyLimit 与 upstream_models.go 的 registry 读取上限一致。
	modelsDevPricingBodyLimit = 8 << 20
)

// modelsDevCost 镜像 models.dev registry 的 cost 结构，单位 $/M token。
type modelsDevCost struct {
	Input      float64             `json:"input"`
	Output     float64             `json:"output"`
	CacheRead  float64             `json:"cache_read"`
	CacheWrite float64             `json:"cache_write"`
	Tiers      []modelsDevCostTier `json:"tiers"`
}

// modelsDevCostTier models.dev 的上下文分档价（如 >272k 输入档）。
type modelsDevCostTier struct {
	Input  float64       `json:"input"`
	Output float64       `json:"output"`
	Tier   modelsDevTier `json:"tier"`
}

// modelsDevTier 分档条件（type == "context" 时 Size 为 token 阈值）。
type modelsDevTier struct {
	Type string `json:"type"`
	Size int64  `json:"size"`
}

// modelsDevPricingSource 从 models.dev registry 读取模型价格。
// 与 upstream_models.go 的 metadata 同步相互独立：计费链在所有计费路径的
// 汇合点（getModelPricingAt）消费它，不依赖账号或代理形态。
type modelsDevPricingSource struct {
	mu        sync.Mutex
	registry  map[string]modelsDevProvider
	fetchedAt time.Time
	lastFail  time.Time
	client    *http.Client
	// sf 去重并发的 registry 拉取：TTL 过期或退避窗口刚过时涌入的请求
	// 只有一个真正发起网络拉取，其余共享其结果（先例：codex manifest 同步）。
	sf singleflight.Group
}

// newModelsDevPricingSource 创建 pricing 源。client 为 nil 时使用默认 client。
// 构造时不发起网络请求；首次查询按需拉取并缓存。
func newModelsDevPricingSource(client *http.Client) *modelsDevPricingSource {
	return &modelsDevPricingSource{client: client}
}

// modelsDevPricingURL 返回 registry 地址；测试通过环境变量覆盖。
func modelsDevPricingURL() string {
	if v := os.Getenv("SUB2API_MODELSDEV_PRICING_URL"); v != "" {
		return v
	}
	return "https://models.dev/api.json"
}

// ensureRegistry 返回 registry，必要时同步拉取。
// 新鲜（TTL 内）直接返回；过期则拉取，失败时回退旧缓存（stale-while-error），
// 失败后进入退避窗口（modelsDevPricingRetryDelay）内不再重试。
func (m *modelsDevPricingSource) ensureRegistry() (map[string]modelsDevProvider, bool) {
	m.mu.Lock()
	if len(m.registry) > 0 && time.Since(m.fetchedAt) < modelsDevPricingTTL {
		r := m.registry
		m.mu.Unlock()
		return r, true
	}
	if !m.lastFail.IsZero() && time.Since(m.lastFail) < modelsDevPricingRetryDelay {
		r, ok := m.registry, len(m.registry) > 0
		m.mu.Unlock()
		return r, ok
	}
	m.mu.Unlock()

	v, err, _ := m.sf.Do("registry", func() (any, error) {
		return m.fetch()
	})
	fetched, _ := v.(map[string]modelsDevProvider)
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		m.lastFail = time.Now()
		if len(m.registry) > 0 {
			slog.Warn("models.dev pricing refresh failed, serving stale", "error", err)
			return m.registry, true
		}
		slog.Warn("models.dev pricing unavailable, skipping models.dev source", "error", err)
		return nil, false
	}
	m.registry = fetched
	m.fetchedAt = time.Now()
	return m.registry, true
}

// fetch 拉取并解析 models.dev registry。
func (m *modelsDevPricingSource) fetch() (map[string]modelsDevProvider, error) {
	url := modelsDevPricingURL()
	// 始终以本源的 15s 超时为准（registry 为 MB 级响应，不能无超时），
	// 注入 client 仅保留其 Transport（测试注入 stub）。
	client := m.client
	if client == nil {
		client = http.DefaultClient
	}
	client = &http.Client{Timeout: modelsDevPricingTimeout, Transport: client.Transport}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch models.dev pricing: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("models.dev pricing returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, modelsDevPricingBodyLimit+1))
	if err != nil {
		return nil, fmt.Errorf("read models.dev pricing: %w", err)
	}
	if int64(len(body)) > modelsDevPricingBodyLimit {
		return nil, fmt.Errorf("models.dev pricing response exceeds %d bytes", modelsDevPricingBodyLimit)
	}
	var registry map[string]modelsDevProvider
	if err := json.Unmarshal(body, &registry); err != nil {
		return nil, fmt.Errorf("parse models.dev pricing: %w", err)
	}
	if len(registry) == 0 {
		return nil, fmt.Errorf("models.dev pricing is empty")
	}
	return registry, nil
}

// getModelPricing 查询模型价格：registry 内 "openai" provider，精确 → 大小写
// 不敏感 → 已知变体归一化（与渠道定价同口径，见 normalizeKnownOpenAICodexModel）。
// 无 input/output 价的条目（图片类）跳过。仅在 LiteLLM 快照未命中时被调用
// （新模型补缺口），不遮蔽 LiteLLM 对已知模型的定价语义。
func (m *modelsDevPricingSource) getModelPricing(model string) (*ModelPricing, bool) {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return nil, false
	}
	registry, ok := m.ensureRegistry()
	if !ok {
		return nil, false
	}
	provider, ok := matchModelsDevProviderByKnownHost(registry, "https://api.openai.com/v1")
	if !ok {
		return nil, false
	}
	// 精确 → 大小写不敏感
	modelData, found := provider.Models[model]
	if !found {
		for candidateID, candidate := range provider.Models {
			if strings.EqualFold(strings.TrimSpace(candidateID), model) ||
				strings.EqualFold(strings.TrimSpace(candidate.ID), model) {
				modelData, found = candidate, true
				break
			}
		}
	}
	if !found {
		// 已知变体归一化（gpt-5.6-luna-high → gpt-5.6-luna）
		if normalized := normalizeKnownOpenAICodexModel(model); normalized != "" && !strings.EqualFold(normalized, model) {
			if modelData, found = provider.Models[normalized]; !found {
				return nil, false
			}
		} else {
			return nil, false
		}
	}
	return modelsDevCostToModelPricing(modelData.Cost)
}

// modelsDevCostToModelPricing 把 models.dev 的 cost（$/M token）换算为内部
// ModelPricing（per-token USD）。首个 context 分档折算为长上下文阶梯：
// 阈值取 tier size，倍率 = 分档价 ÷ 基础价（与 LiteLLM above-tier 折算同思路）。
func modelsDevCostToModelPricing(cost modelsDevCost) (*ModelPricing, bool) {
	const perMillion = 1_000_000.0
	if cost.Input == 0 && cost.Output == 0 {
		return nil, false // 无 token 价（图片类），跳过
	}
	pricing := &ModelPricing{
		InputPricePerToken:         cost.Input / perMillion,
		OutputPricePerToken:        cost.Output / perMillion,
		CacheReadPricePerToken:     cost.CacheRead / perMillion,
		CacheCreationPricePerToken: cost.CacheWrite / perMillion,
	}
	if cost.CacheRead > 0 || cost.CacheWrite > 0 {
		pricing.SupportsCacheBreakdown = true
		pricing.CacheCreation5mPrice = cost.CacheWrite / perMillion
	}
	// ponytail: ModelPricing 仅支持单一长上下文阶梯，多档模型只取**最大** context
	// 档折算——介于两档之间的请求会按高档倍率多收，宁可多收不可白送长上下文 token。
	// 若后续需要多档，ModelPricing/计费链需先支持阶梯列表。
	if len(cost.Tiers) > 0 && cost.Input > 0 {
		for _, tier := range cost.Tiers {
			if tier.Tier.Type != "context" || tier.Tier.Size <= 0 {
				continue
			}
			if pricing.LongContextInputThreshold == 0 || int(tier.Tier.Size) > pricing.LongContextInputThreshold {
				pricing.LongContextInputThreshold = int(tier.Tier.Size)
				pricing.LongContextInputMultiplier = tier.Input / cost.Input
				if cost.Output > 0 {
					pricing.LongContextOutputMultiplier = tier.Output / cost.Output
				}
			}
		}
	}
	return pricing, true
}
