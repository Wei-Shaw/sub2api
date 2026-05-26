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

// wildcardMappingPayload 对应 K5 envelope 中的元素。
type wildcardMappingPayload struct {
	Prefix string `json:"prefix"`
	Target string `json:"target"`
}

// wildcardEnvelope 是 K5 顶层结构。P4 之前 K3 也共用此结构, 现仅 K5 用到。
type wildcardEnvelope[T any] struct {
	SchemaVersion string `json:"schema_version"`
	Entries       []T    `json:"entries"`
	UpdatedAt     int64  `json:"updated_at"`
}

// ChannelMeta 是 K1 解码后供网关使用的精简元信息。
//
// 字段语义与 channel-management 插件 service.Channel 中的同名字段一致；
// 不携带 GroupIDs / ModelPricing / ModelMapping（它们存放在其它 key 中）。
type ChannelMeta struct {
	ChannelID          int64
	BillingModelSource string
	RestrictModels     bool
	Features           string
	Platform           string
	UpdatedAt          time.Time
}

// ChannelCacheReader 通过 Redis Key 读取由 channel-management 插件维护的渠道
// 元信息 (K1) 和 model 映射 (K4 / K5) 数据。该结构对插件零感知,只认 Redis
// 键格式。
//
// 所有公共方法都是协程安全的（go-redis 客户端本身协程安全）。
//
// P4 拆分后的数据来源:
//   - GetChannelModelPricing: 仅查 in-memory PricingOverrideCache。Redis
//     上的 K2 / K3 已经从 channel-management CacheWriter 下线, 不再被读取。
//     未注入 pricingCache 时所有定价查询返回 nil（无渠道）。
//   - GetChannelMeta / lookupMapping / IsModelRestricted: 继续走 Redis,
//     因为 PricingOverride proto 不携带 ChannelID / restrict_models /
//     model→model mapping 字段。这些 capability 仍由 channel-management
//     CacheWriter 维护对应的 K1 / K4 / K5 keys, RedisRaw 在 P4 之后仍保留。
type ChannelCacheReader struct {
	rdb *redis.Client
	// pricingCache 是 host wire 注入的 in-memory 覆盖层。P4 之后是 pricing
	// 唯一来源（不再走 Redis fallback）。nil 等价于 "host 没有装载任何 plugin
	// pricing", GetChannelModelPricing 一律返回 nil。
	pricingCache *PricingOverrideCache
}

// NewChannelCacheReader 构造一个只读缓存客户端。
//
// 当 rdb 为 nil 时,所有依赖 Redis 的方法 (GetChannelMeta / lookupMapping /
// IsModelRestricted) 都会走"无渠道"降级路径,适用于 Redis 未配置或单元测试
// 场景。pricingCache 由 host wire 通过 SetPricingOverrideCache 注入; 不传 /
// 传 nil 时 GetChannelModelPricing 永远返回 nil。
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

// ResolveChannelMapping 通过 K1 meta + K4/K5 mapping 返回映射后的模型名 /
// 渠道 ID / 计费模型来源。该方法是核心 Gateway 侧的入口, 与 channel-management
// 插件 service 层的同名方法行为对齐。
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

// ResolveChannelMappingAndRestrict 与 channel-management 插件 service 层的
// 同名方法对齐。当前实现 restricted 始终返回 false（限制检查已下沉到调度阶段）。
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
//
// P4 拆分后: restrict 标记仍由 K1 (Redis meta) 提供; 但 "model 是否在
// 允许列表" 的判断已经迁移到 PricingOverrideCache (in-memory) — Redis
// K2 / K3 已停止写入。因此 lookup 走 lookupPricingFromCache。
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
	return r.lookupPricingFromCache(groupID, platform, model) == nil
}

// GetChannelModelPricing 返回精确匹配的渠道定价, 数据来源于 in-memory
// PricingOverrideCache (由 host PricingExtensionClient 通过 plugin gRPC
// stream 同步)。未命中或 cache 未注入时返回 nil。
//
// P4 拆分: Redis K2 / K3 已经从 channel-management 写侧下线, 这里也不再
// 读 Redis — pricing 唯一通路是 PricingExtension。如果 plugin 离线 / cache
// 未就绪, host 自然降级到 LiteLLM / 本地 fallback (调用方 PricingService 的
// 行为, 与本 reader 无关)。
//
// 返回的 *ChannelModelPricing 是新分配的拷贝, 调用方持有所有权。
func (r *ChannelCacheReader) GetChannelModelPricing(
	ctx context.Context, groupID int64, platform, model string,
) *ChannelModelPricing {
	_ = ctx // ctx kept for symmetry / future use; in-memory lookup is synchronous
	platform = normalizePlatform(platform)
	if platform == "" {
		return nil
	}
	return r.lookupPricingFromCache(groupID, platform, model)
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

func normalizePlatform(platform string) string {
	return strings.ToLower(strings.TrimSpace(platform))
}

func normalizeModel(model string) string {
	return strings.ToLower(strings.TrimSpace(model))
}
