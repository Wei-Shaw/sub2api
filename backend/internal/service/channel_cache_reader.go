// V5/W6 ChannelCacheReader 是设计上 service 层直接读 Redis 的渠道缓存读取器:
// plugin 端写入的 cache key 需要在核心 Gateway 热路径上低延迟读取(200ms 超时
// 降级),引入 repository 抽象会增加调用栈与序列化开销且无业务收益。
// 该文件是 .golangci.yml 中 service-no-repository 规则的合理例外。
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9" //nolint:depguard // 见文件顶部说明
)

// 本文件实现 ChannelCacheReader：核心 Gateway 从 Redis 直接读取渠道（pricing /
// mapping / restriction / meta）数据的只读客户端。
//
// 设计依据见 plugins/channel-management/GATEWAY_CACHE_SPEC.md。本文件不依赖
// 任何插件代码、不持有任何渠道业务逻辑；仅按约定的 Redis Key 反序列化为
// 网关需要的中间结构。
//
// 失败语义：所有方法在 Redis 不可用 / 缓存缺失 / JSON 反序列化失败时,
// 都返回与"无渠道关联"等价的安全降级值,并记录 warn 日志。绝不返回 error,
// 以保护热路径不被缓存层故障阻塞。

const (
	channelCacheMetaKeyFmt       = "plugin:channel:meta:%d:%s"
	channelCachePricingKeyFmt    = "plugin:channel:pricing:%d:%s:%s"
	channelCacheWildcardPriceFmt = "plugin:channel:wildcard:pricing:%d:%s"
	channelCacheMappingKeyFmt    = "plugin:channel:mapping:%d:%s:%s"
	channelCacheWildcardMapFmt   = "plugin:channel:wildcard:mapping:%d:%s"
	channelCacheReadTimeoutShort = 200 * time.Millisecond
)

// channelMetaPayload 对应 GATEWAY_CACHE_SPEC.md §3.1（K1）。
type channelMetaPayload struct {
	SchemaVersion      string `json:"schema_version"`
	ChannelID          int64  `json:"channel_id"`
	BillingModelSource string `json:"billing_model_source"`
	RestrictModels     bool   `json:"restrict_models"`
	Features           string `json:"features"`
	Platform           string `json:"platform"`
	UpdatedAt          int64  `json:"updated_at"`
}

// channelPricingPayload 对应 GATEWAY_CACHE_SPEC.md §3.2（K2 元素）。
type channelPricingPayload struct {
	SchemaVersion    string                   `json:"schema_version"`
	ID               int64                    `json:"id"`
	ChannelID        int64                    `json:"channel_id"`
	Platform         string                   `json:"platform"`
	Models           []string                 `json:"models"`
	BillingMode      string                   `json:"billing_mode"`
	InputPrice       *float64                 `json:"input_price"`
	OutputPrice      *float64                 `json:"output_price"`
	CacheWritePrice  *float64                 `json:"cache_write_price"`
	CacheReadPrice   *float64                 `json:"cache_read_price"`
	ImageOutputPrice *float64                 `json:"image_output_price"`
	PerRequestPrice  *float64                 `json:"per_request_price"`
	Intervals        []pricingIntervalPayload `json:"intervals"`
	UpdatedAt        int64                    `json:"updated_at"`
}

type pricingIntervalPayload struct {
	ID              int64    `json:"id"`
	MinTokens       int      `json:"min_tokens"`
	MaxTokens       *int     `json:"max_tokens"`
	TierLabel       string   `json:"tier_label"`
	InputPrice      *float64 `json:"input_price"`
	OutputPrice     *float64 `json:"output_price"`
	CacheWritePrice *float64 `json:"cache_write_price"`
	CacheReadPrice  *float64 `json:"cache_read_price"`
	PerRequestPrice *float64 `json:"per_request_price"`
	SortOrder       int      `json:"sort_order"`
}

// wildcardPricingPayload 对应 K3 envelope 中的元素。
type wildcardPricingPayload struct {
	Prefix  string                `json:"prefix"`
	Pricing channelPricingPayload `json:"pricing"`
}

// wildcardMappingPayload 对应 K5 envelope 中的元素。
type wildcardMappingPayload struct {
	Prefix string `json:"prefix"`
	Target string `json:"target"`
}

// wildcardEnvelope 是 K3 / K5 共用的顶层结构。
type wildcardEnvelope[T any] struct {
	SchemaVersion string `json:"schema_version"`
	Entries       []T    `json:"entries"`
	UpdatedAt     int64  `json:"updated_at"`
}

// ChannelMeta 是 K1 解码后供网关使用的精简元信息。
//
// 字段语义与 service.Channel 中的同名字段一致；不携带 GroupIDs / ModelPricing /
// ModelMapping（它们存放在其它 key 中）。
type ChannelMeta struct {
	ChannelID          int64
	BillingModelSource string
	RestrictModels     bool
	Features           string
	Platform           string
	UpdatedAt          time.Time
}

// ChannelCacheReader 通过 Redis Key 读取由 channel-management 插件维护的渠道
// 元信息 / 定价 / 映射 / 限制数据。该结构对插件零感知,只认 Redis 键格式。
//
// 所有公共方法都是协程安全的（go-redis 客户端本身协程安全）。
//
// P3 双源降级 (PLUGIN-PRICING):
//   - pricingCache 优先 (in-memory, host 端经 PricingExtensionClient 同步而来)
//   - cache miss 时落回 Redis 路径, 兼容 P4 之前 channel-management 仍在写
//     plugin:channel:pricing:* 等约定 key 的过渡期。
//
// 当 PricingOverrideCache 完全替代 Redis 写入路径时(P4), 仅需删除 Redis
// 分支即可。
type ChannelCacheReader struct {
	rdb *redis.Client
	// pricingCache 是 host wire 注入的 in-memory 覆盖层。GetChannelModelPricing
	// 优先查它, 命中即返回; 缺失才走 Redis fallback。nil 等价于"未启用 plugin
	// pricing", 行为与 P3 之前完全一致。
	pricingCache *PricingOverrideCache
}

// NewChannelCacheReader 构造一个只读缓存客户端。
//
// 当 rdb 为 nil 时,所有方法都会走"无渠道"降级路径,适用于 Redis 未配置或
// 单元测试场景。pricingCache 可由 host wire 通过 SetPricingOverrideCache
// 注入; 不传 / 传 nil 时本 reader 退化为 P3 之前的纯 Redis 行为。
func NewChannelCacheReader(rdb *redis.Client, pricingCache *PricingOverrideCache) *ChannelCacheReader {
	return &ChannelCacheReader{rdb: rdb, pricingCache: pricingCache}
}

// SetPricingOverrideCache (re)attaches the in-memory override cache. nil
// detaches the cache and reverts to the Redis-only path. Useful in tests
// that want to swap the cache without rebuilding the reader.
func (r *ChannelCacheReader) SetPricingOverrideCache(cache *PricingOverrideCache) {
	if r == nil {
		return
	}
	r.pricingCache = cache
}

// GetChannelMeta 读取 K1。返回 (nil, false) 表示该 (group, platform) 没有
// 渠道关联或缓存缺失,网关应走"无渠道"降级路径。
func (r *ChannelCacheReader) GetChannelMeta(ctx context.Context, groupID int64, platform string) (*ChannelMeta, bool) {
	if r == nil || r.rdb == nil {
		return nil, false
	}
	platform = normalizePlatform(platform)
	if platform == "" {
		return nil, false
	}

	key := fmt.Sprintf(channelCacheMetaKeyFmt, groupID, platform)
	raw, err := r.getString(ctx, key)
	if err != nil || raw == "" {
		return nil, false
	}

	var payload channelMetaPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		slog.Warn("channel cache: meta decode failed",
			"key", key, "error", err)
		return nil, false
	}
	if payload.ChannelID == 0 {
		return nil, false
	}
	return &ChannelMeta{
		ChannelID:          payload.ChannelID,
		BillingModelSource: payload.BillingModelSource,
		RestrictModels:     payload.RestrictModels,
		Features:           payload.Features,
		Platform:           payload.Platform,
		UpdatedAt:          time.Unix(payload.UpdatedAt, 0),
	}, true
}

// ResolveChannelMapping 与 ChannelService.ResolveChannelMapping 行为对齐:
// 返回映射后的模型名 / 渠道 ID / 计费模型来源。
//
// 缓存缺失时返回 ChannelMappingResult{MappedModel: model},网关随后按"无渠道"
// 路径继续。
func (r *ChannelCacheReader) ResolveChannelMapping(
	ctx context.Context, groupID int64, platform, model string,
) ChannelMappingResult {
	platform = normalizePlatform(platform)
	if r == nil || r.rdb == nil || platform == "" {
		return ChannelMappingResult{MappedModel: model}
	}

	meta, ok := r.GetChannelMeta(ctx, groupID, platform)
	if !ok {
		return ChannelMappingResult{MappedModel: model}
	}
	source := meta.BillingModelSource
	if source == "" {
		source = BillingModelSourceChannelMapped
	}
	result := ChannelMappingResult{
		MappedModel:        model,
		ChannelID:          meta.ChannelID,
		BillingModelSource: source,
	}
	if mapped := r.lookupMapping(ctx, groupID, platform, model); mapped != "" {
		result.MappedModel = mapped
		result.Mapped = true
	}
	return result
}

// ResolveChannelMappingAndRestrict 与 ChannelService 同名方法对齐。
// 当前实现 restricted 始终返回 false（与 service 层保持一致,限制检查已下
// 沉到调度阶段）。
func (r *ChannelCacheReader) ResolveChannelMappingAndRestrict(
	ctx context.Context, groupID *int64, platform, model string,
) (ChannelMappingResult, bool) {
	if groupID == nil {
		return ChannelMappingResult{MappedModel: model}, false
	}
	return r.ResolveChannelMapping(ctx, *groupID, platform, model), false
}

// IsModelRestricted 检查模型是否被渠道限制(不在允许列表中)。
//
// 派生公式: K1.restrict_models == true AND lookup_pricing(...) == nil。
// 见 GATEWAY_CACHE_SPEC.md §4。
func (r *ChannelCacheReader) IsModelRestricted(
	ctx context.Context, groupID int64, platform, model string,
) bool {
	platform = normalizePlatform(platform)
	if r == nil || r.rdb == nil || platform == "" {
		return false
	}
	meta, ok := r.GetChannelMeta(ctx, groupID, platform)
	if !ok || !meta.RestrictModels {
		return false
	}
	return r.lookupPricing(ctx, groupID, platform, model) == nil
}

// GetChannelModelPricing 返回精确或通配符匹配的渠道定价。未命中返回 nil。
//
// 返回的 *ChannelModelPricing 是新分配的拷贝,调用方持有所有权。
//
// 双源降级 (P3): 优先查 in-memory PricingOverrideCache (由 plugin
// PricingExtension 写入), miss 才走 Redis 路径。Redis 仍由 channel-management
// CacheWriter 维护, P4 移除 Redis 写入时删除 fallback 分支即可。
func (r *ChannelCacheReader) GetChannelModelPricing(
	ctx context.Context, groupID int64, platform, model string,
) *ChannelModelPricing {
	platform = normalizePlatform(platform)
	if r == nil || platform == "" {
		return nil
	}
	// In-memory cache first. Cache 命中时不依赖 Redis K1 meta — pricing override
	// 已经包含了完整的字段 (含 BillingMode), 无需再读 meta。
	if pricing := r.lookupPricingFromCache(groupID, platform, model); pricing != nil {
		return pricing
	}
	if r.rdb == nil {
		return nil
	}
	if _, ok := r.GetChannelMeta(ctx, groupID, platform); !ok {
		return nil
	}
	return r.lookupPricing(ctx, groupID, platform, model)
}

// lookupPricingFromCache resolves a (group, platform, model) tuple via the
// in-memory PricingOverrideCache. Returns nil when the cache is detached
// or the entry is missing — caller falls through to the Redis path. The
// cache stores values normalised to lowercase so callers can pass the
// original case freely.
func (r *ChannelCacheReader) lookupPricingFromCache(groupID int64, platform, model string) *ChannelModelPricing {
	if r == nil || r.pricingCache == nil {
		return nil
	}
	override, ok := r.pricingCache.Get(groupID, platform, model)
	if !ok || override == nil {
		return nil
	}
	return overrideToChannelPricing(override)
}

// overrideToChannelPricing translates the cache value type into the shape
// downstream consumers expect (service.ChannelModelPricing). Pointer
// pricing fields are reconstructed only when the override carries a
// non-zero value so "unset" semantics survive the round-trip — see proto
// PricingOverride for the zero-means-unset contract.
func overrideToChannelPricing(o *PricingOverride) *ChannelModelPricing {
	if o == nil {
		return nil
	}
	out := &ChannelModelPricing{
		Platform:    o.Key.Platform,
		Models:      []string{o.Key.Model},
		BillingMode: BillingMode(o.BillingMode),
		UpdatedAt:   o.UpdatedAt,
	}
	if out.BillingMode == "" {
		out.BillingMode = BillingModeToken
	}
	out.InputPrice = nonZeroFloat(o.InputPrice)
	out.OutputPrice = nonZeroFloat(o.OutputPrice)
	out.CacheWritePrice = nonZeroFloat(o.CacheWritePrice)
	out.CacheReadPrice = nonZeroFloat(o.CacheReadPrice)
	out.ImageOutputPrice = nonZeroFloat(o.ImageOutputPrice)
	out.PerRequestPrice = nonZeroFloat(o.PerRequestPrice)
	if len(o.Intervals) > 0 {
		out.Intervals = make([]PricingInterval, 0, len(o.Intervals))
		for i := range o.Intervals {
			iv := &o.Intervals[i]
			var maxTokens *int
			if iv.MaxTokens > 0 {
				m := int(iv.MaxTokens)
				maxTokens = &m
			}
			out.Intervals = append(out.Intervals, PricingInterval{
				MinTokens:       int(iv.MinTokens),
				MaxTokens:       maxTokens,
				InputPrice:      nonZeroFloat(iv.InputPrice),
				OutputPrice:     nonZeroFloat(iv.OutputPrice),
				CacheWritePrice: nonZeroFloat(iv.CacheWritePrice),
				CacheReadPrice:  nonZeroFloat(iv.CacheReadPrice),
				PerRequestPrice: nonZeroFloat(iv.PerRequestPrice),
			})
		}
	}
	return out
}

// nonZeroFloat returns &v when v != 0, nil otherwise. The proto contract
// treats zero as "unset" so we must NOT install a *float64 pointer for
// zero values — downstream code distinguishes nil (fall back to base
// pricing) from a real zero price.
func nonZeroFloat(v float64) *float64 {
	if v == 0 {
		return nil
	}
	return &v
}

// ---- 内部读取与匹配 ----

// lookupPricing 先查精确 key (K2),未命中再扫通配符数组 (K3)。
func (r *ChannelCacheReader) lookupPricing(
	ctx context.Context, groupID int64, platform, model string,
) *ChannelModelPricing {
	modelLower := normalizeModel(model)
	if modelLower == "" {
		return nil
	}

	if exact := r.fetchExactPricing(ctx, groupID, platform, modelLower); exact != nil {
		return exact
	}
	return r.fetchWildcardPricing(ctx, groupID, platform, modelLower)
}

func (r *ChannelCacheReader) fetchExactPricing(
	ctx context.Context, groupID int64, platform, modelLower string,
) *ChannelModelPricing {
	key := fmt.Sprintf(channelCachePricingKeyFmt, groupID, platform, modelLower)
	raw, err := r.getString(ctx, key)
	if err != nil || raw == "" {
		return nil
	}
	var payload channelPricingPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		slog.Warn("channel cache: pricing decode failed",
			"key", key, "error", err)
		return nil
	}
	return decodePricing(&payload)
}

func (r *ChannelCacheReader) fetchWildcardPricing(
	ctx context.Context, groupID int64, platform, modelLower string,
) *ChannelModelPricing {
	key := fmt.Sprintf(channelCacheWildcardPriceFmt, groupID, platform)
	raw, err := r.getString(ctx, key)
	if err != nil || raw == "" {
		return nil
	}
	var envelope wildcardEnvelope[wildcardPricingPayload]
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		slog.Warn("channel cache: wildcard pricing decode failed",
			"key", key, "error", err)
		return nil
	}
	// envelope.Entries 由写侧保证按前缀长度降序,直接遍历即可。
	for i := range envelope.Entries {
		entry := &envelope.Entries[i]
		if strings.HasPrefix(modelLower, entry.Prefix) {
			return decodePricing(&entry.Pricing)
		}
	}
	return nil
}

// lookupMapping 先查精确 key (K4),未命中再扫通配符数组 (K5)。
func (r *ChannelCacheReader) lookupMapping(
	ctx context.Context, groupID int64, platform, model string,
) string {
	modelLower := normalizeModel(model)
	if modelLower == "" {
		return ""
	}

	if exact := r.fetchExactMapping(ctx, groupID, platform, modelLower); exact != "" {
		return exact
	}
	return r.fetchWildcardMapping(ctx, groupID, platform, modelLower)
}

func (r *ChannelCacheReader) fetchExactMapping(
	ctx context.Context, groupID int64, platform, modelLower string,
) string {
	key := fmt.Sprintf(channelCacheMappingKeyFmt, groupID, platform, modelLower)
	raw, err := r.getString(ctx, key)
	if err != nil {
		return ""
	}
	return raw
}

func (r *ChannelCacheReader) fetchWildcardMapping(
	ctx context.Context, groupID int64, platform, modelLower string,
) string {
	key := fmt.Sprintf(channelCacheWildcardMapFmt, groupID, platform)
	raw, err := r.getString(ctx, key)
	if err != nil || raw == "" {
		return ""
	}
	var envelope wildcardEnvelope[wildcardMappingPayload]
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		slog.Warn("channel cache: wildcard mapping decode failed",
			"key", key, "error", err)
		return ""
	}
	for i := range envelope.Entries {
		entry := &envelope.Entries[i]
		if strings.HasPrefix(modelLower, entry.Prefix) {
			return entry.Target
		}
	}
	return ""
}

// getString 包装 redis GET,处理 redis.Nil 与超时。返回 (value, error)。
// redis.Nil 视为 "" + nil; 网络错误返回 ("" , err) 并由调用方记 warn。
func (r *ChannelCacheReader) getString(ctx context.Context, key string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, channelCacheReadTimeoutShort)
	defer cancel()

	val, err := r.rdb.Get(cctx, key).Result()
	if err == nil {
		return val, nil
	}
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	slog.Warn("channel cache: redis get failed",
		"key", key, "error", err)
	return "", err
}

// decodePricing 将 channelPricingPayload 转换为 service.ChannelModelPricing。
func decodePricing(p *channelPricingPayload) *ChannelModelPricing {
	if p == nil {
		return nil
	}
	out := ChannelModelPricing{
		ID:               p.ID,
		ChannelID:        p.ChannelID,
		Platform:         p.Platform,
		Models:           append([]string(nil), p.Models...),
		BillingMode:      BillingMode(p.BillingMode),
		InputPrice:       p.InputPrice,
		OutputPrice:      p.OutputPrice,
		CacheWritePrice:  p.CacheWritePrice,
		CacheReadPrice:   p.CacheReadPrice,
		ImageOutputPrice: p.ImageOutputPrice,
		PerRequestPrice:  p.PerRequestPrice,
		UpdatedAt:        time.Unix(p.UpdatedAt, 0),
	}
	if out.BillingMode == "" {
		out.BillingMode = BillingModeToken
	}
	if len(p.Intervals) > 0 {
		out.Intervals = make([]PricingInterval, len(p.Intervals))
		for i := range p.Intervals {
			iv := &p.Intervals[i]
			out.Intervals[i] = PricingInterval{
				ID:              iv.ID,
				MinTokens:       iv.MinTokens,
				MaxTokens:       iv.MaxTokens,
				TierLabel:       iv.TierLabel,
				InputPrice:      iv.InputPrice,
				OutputPrice:     iv.OutputPrice,
				CacheWritePrice: iv.CacheWritePrice,
				CacheReadPrice:  iv.CacheReadPrice,
				PerRequestPrice: iv.PerRequestPrice,
				SortOrder:       iv.SortOrder,
			}
		}
	}
	return &out
}

func normalizePlatform(platform string) string {
	return strings.ToLower(strings.TrimSpace(platform))
}

func normalizeModel(model string) string {
	return strings.ToLower(strings.TrimSpace(model))
}
