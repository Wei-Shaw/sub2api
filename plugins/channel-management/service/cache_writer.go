package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

// 本文件实现 CacheWriter：channel-management 插件向 Redis 写入网关读取的
// 渠道元信息 / 定价 / 映射 / 限制数据。键格式与字段语义严格遵循
// plugins/channel-management/GATEWAY_CACHE_SPEC.md（v1）。
//
// 设计要点：
//   - SDK 暴露的 RedisClient 没有 Pipeline / MGET，写侧只能逐条 SetEx。
//     对单 (group, platform) 的键数 ≈ 1 (meta) + N (exact pricing) +
//     1 (wildcard pricing) + N (exact mapping) + 1 (wildcard mapping)。
//     渠道 CRUD 是低频路径，串行写入完全可接受。
//   - 写入失败只记 warn 不返回错误，避免缓存层故障阻塞 CRUD 主路径。
//     TTL（15 min）+ 下次 CRUD 重建会自然兜底。
//   - InvalidateCache 必须基于"旧 GroupIDs / 旧 ModelPricing 的 platform 集合"
//     去删 key，否则更新后旧分组 / 旧平台残留缓存得等 TTL。

const (
	// channelCacheSchemaVersion 与 channel_cache_reader.go 保持一致。
	channelCacheSchemaVersion = "v1"

	// channelCacheTTLDefault 见 GATEWAY_CACHE_SPEC.md §5。
	channelCacheTTLDefault = 15 * time.Minute

	// channelCacheWriteTimeout 单次 Redis 写入 / 删除的上下文超时。
	channelCacheWriteTimeout = 2 * time.Second

	// 失效广播频道 / scope（GATEWAY_CACHE_SPEC.md §6.2）。
	channelCacheInvalidateChannel = "plugin:channel:invalidate"
	channelCacheInvalidateScopeAll   = "all"
	channelCacheInvalidateScopeGroup = "group"

	// Reason 字符串供调试，无业务语义。
	channelCacheReasonBootstrap     = "bootstrap"
	channelCacheReasonChannelUpdate = "channel_updated"
	channelCacheReasonChannelDelete = "channel_deleted"
)

// GroupPlatformLookup 由 ChannelService 注入，用于把 groupID 解析为该分组的
// platform。返回的 map 不必包含全部 groupID（缺失视为"未知，跳过该 group"）。
type GroupPlatformLookup func(ctx context.Context, groupIDs []int64) (map[int64]string, error)

// CacheWriter 把 *Channel 拆解写入 Redis，保持与 ChannelCacheReader 的契约一致。
//
// CacheWriter 与具体的 *ChannelService 解耦：依赖 RedisClient（写键）+
// GroupPlatformLookup（解析 group→platform）即可独立工作，便于单元测试 mock。
type CacheWriter struct {
	redis  pluginsdk.RedisClient
	lookup GroupPlatformLookup
	ttl    time.Duration
}

// NewCacheWriter 构造一个 CacheWriter。
//
// 当 redis 为 nil 时所有方法都会变成 no-op（仅记 debug 日志），允许插件在
// 未注入 Redis 的环境下继续运行。lookup 为 nil 时 group→platform 解析会
// 退化为"未知"，所有写入都会跳过；调用方应在 plugin Init 时正确注入。
func NewCacheWriter(redis pluginsdk.RedisClient, lookup GroupPlatformLookup) *CacheWriter {
	return &CacheWriter{
		redis:  redis,
		lookup: lookup,
		ttl:    channelCacheTTLDefault,
	}
}

// RebuildCache 重建单个渠道关联的所有 Redis 缓存条目。
// 调用方：Create / Update 成功后。
//
// channel 为 nil 或 status != active 时仅删除既有缓存（按当前 channel.GroupIDs
// 推断需要删的 key）。
func (w *CacheWriter) RebuildCache(ctx context.Context, channel *Channel) error {
	if w == nil || w.redis == nil || channel == nil {
		return nil
	}

	groupPlatforms := w.fetchGroupPlatforms(channel.GroupIDs)
	for _, gid := range channel.GroupIDs {
		platform := groupPlatforms[gid]
		if platform == "" {
			// 没有 group → platform 映射意味着该分组已经被删除或者
			// repo 暂未提供查询途径；保险起见只清理可能残留的键。
			w.deleteGroupKeys(ctx, gid, w.platformsFromPricing(channel))
			continue
		}
		if !channel.IsActive() {
			w.deleteGroupKeys(ctx, gid, []string{platform})
			continue
		}
		w.writeChannelForGroup(ctx, channel, gid, platform)
	}
	w.publishInvalidate(ctx, channelCacheInvalidateScopeGroup, channel.GroupIDs, channelCacheReasonChannelUpdate)
	return nil
}

// RebuildAllCache 在插件启动时全量重建。channels 为当前 DB 中所有渠道的快照。
//
// 调用方：plugin Init() 加载完成后。
func (w *CacheWriter) RebuildAllCache(ctx context.Context, channels []Channel) error {
	if w == nil || w.redis == nil {
		return nil
	}

	if err := w.writeSchemaVersion(ctx); err != nil {
		slog.Warn("channel cache: failed to write schema_version", "error", err)
	}

	allGroupIDs := collectGroupIDs(channels)
	groupPlatforms := w.fetchGroupPlatforms(allGroupIDs)

	for i := range channels {
		ch := &channels[i]
		if !ch.IsActive() {
			continue
		}
		for _, gid := range ch.GroupIDs {
			platform := groupPlatforms[gid]
			if platform == "" {
				continue
			}
			w.writeChannelForGroup(ctx, ch, gid, platform)
		}
	}

	w.publishInvalidate(ctx, channelCacheInvalidateScopeAll, nil, channelCacheReasonBootstrap)
	return nil
}

// InvalidateCache 删除渠道关联的所有 Redis 键。
// 调用方：Delete 之前（先抓 GroupIDs/ModelPricing 再调用本函数）。
func (w *CacheWriter) InvalidateCache(ctx context.Context, channel *Channel) error {
	if w == nil || w.redis == nil || channel == nil {
		return nil
	}

	groupPlatforms := w.fetchGroupPlatforms(channel.GroupIDs)
	pricingPlatforms := w.platformsFromPricing(channel)

	for _, gid := range channel.GroupIDs {
		platforms := uniqueStrings(append([]string{groupPlatforms[gid]}, pricingPlatforms...))
		w.deleteGroupKeys(ctx, gid, platforms)
	}
	w.publishInvalidate(ctx, channelCacheInvalidateScopeGroup, channel.GroupIDs, channelCacheReasonChannelDelete)
	return nil
}

// fetchGroupPlatforms 通过注入的 lookup 把 groupIDs 解析为
// {groupID → platform}。lookup 为 nil 或返回错误时退化为空 map（写侧
// 会因此跳过对应分组的 K2~K5 写入，但仍会保留 InvalidateCache 删除路径）。
func (w *CacheWriter) fetchGroupPlatforms(groupIDs []int64) map[int64]string {
	if w == nil || w.lookup == nil || len(groupIDs) == 0 {
		return map[int64]string{}
	}
	cctx, cancel := context.WithTimeout(context.Background(), channelCacheWriteTimeout)
	defer cancel()
	platforms, err := w.lookup(cctx, groupIDs)
	if err != nil {
		slog.Warn("channel cache: group platform lookup failed",
			"group_ids", groupIDs, "error", err)
		return map[int64]string{}
	}
	return platforms
}

// writeChannelForGroup 把 channel 拆解为 K1/K2/K3/K4/K5 写入 Redis。
func (w *CacheWriter) writeChannelForGroup(ctx context.Context, channel *Channel, groupID int64, platform string) {
	platform = normalizePlatformValue(platform)
	if platform == "" {
		return
	}

	w.writeMeta(ctx, channel, groupID, platform)
	w.writePricing(ctx, channel, groupID, platform)
	w.writeMapping(ctx, channel, groupID, platform)
}

func (w *CacheWriter) writeMeta(ctx context.Context, channel *Channel, groupID int64, platform string) {
	billingSource := channel.BillingModelSource
	if billingSource == "" {
		billingSource = BillingModelSourceChannelMapped
	}
	payload := map[string]any{
		"schema_version":       channelCacheSchemaVersion,
		"channel_id":           channel.ID,
		"billing_model_source": billingSource,
		"restrict_models":      channel.RestrictModels,
		"features":             channel.Features,
		"platform":             platform,
		"updated_at":           channel.UpdatedAt.Unix(),
	}
	value, err := json.Marshal(payload)
	if err != nil {
		slog.Warn("channel cache: marshal meta failed", "channel_id", channel.ID, "error", err)
		return
	}
	key := fmt.Sprintf("plugin:channel:meta:%d:%s", groupID, platform)
	w.setEx(ctx, key, string(value))
}

func (w *CacheWriter) writePricing(ctx context.Context, channel *Channel, groupID int64, platform string) {
	exact := make([]ChannelModelPricing, 0)
	wildcards := make([]wildcardPricingItem, 0)

	for i := range channel.ModelPricing {
		pricing := &channel.ModelPricing[i]
		if normalizePlatformValue(pricing.Platform) != platform {
			continue
		}
		for _, model := range pricing.Models {
			modelLower := normalizeModelValue(model)
			if modelLower == "" {
				continue
			}
			if strings.HasSuffix(modelLower, "*") {
				wildcards = append(wildcards, wildcardPricingItem{
					Prefix:  strings.TrimSuffix(modelLower, "*"),
					Pricing: pricingPayload(pricing),
				})
				continue
			}
			key := fmt.Sprintf("plugin:channel:pricing:%d:%s:%s", groupID, platform, modelLower)
			value, err := json.Marshal(pricingPayload(pricing).withModels([]string{modelLower}))
			if err != nil {
				slog.Warn("channel cache: marshal pricing failed", "key", key, "error", err)
				continue
			}
			w.setEx(ctx, key, string(value))
			_ = exact // exact 仅用于潜在的全量列举，当前未用
		}
	}

	wildcardKey := fmt.Sprintf("plugin:channel:wildcard:pricing:%d:%s", groupID, platform)
	if len(wildcards) == 0 {
		// 无通配符 → 主动删除可能残留的旧 key，避免读侧命中过期数据
		w.del(ctx, wildcardKey)
		return
	}
	sort.SliceStable(wildcards, func(i, j int) bool {
		return len(wildcards[i].Prefix) > len(wildcards[j].Prefix)
	})
	envelope := map[string]any{
		"schema_version": channelCacheSchemaVersion,
		"entries":        wildcards,
		"updated_at":     channel.UpdatedAt.Unix(),
	}
	value, err := json.Marshal(envelope)
	if err != nil {
		slog.Warn("channel cache: marshal wildcard pricing failed", "key", wildcardKey, "error", err)
		return
	}
	w.setEx(ctx, wildcardKey, string(value))
}

func (w *CacheWriter) writeMapping(ctx context.Context, channel *Channel, groupID int64, platform string) {
	platformMapping, ok := channel.ModelMapping[platform]
	if !ok {
		w.del(ctx, fmt.Sprintf("plugin:channel:wildcard:mapping:%d:%s", groupID, platform))
		return
	}

	wildcards := make([]wildcardMappingItem, 0)
	for src, dst := range platformMapping {
		srcLower := normalizeModelValue(src)
		if srcLower == "" || dst == "" {
			continue
		}
		if strings.HasSuffix(srcLower, "*") {
			wildcards = append(wildcards, wildcardMappingItem{
				Prefix: strings.TrimSuffix(srcLower, "*"),
				Target: dst,
			})
			continue
		}
		key := fmt.Sprintf("plugin:channel:mapping:%d:%s:%s", groupID, platform, srcLower)
		// K4 是纯字符串值（GATEWAY_CACHE_SPEC.md §3.5）
		w.setEx(ctx, key, dst)
	}

	wildcardKey := fmt.Sprintf("plugin:channel:wildcard:mapping:%d:%s", groupID, platform)
	if len(wildcards) == 0 {
		w.del(ctx, wildcardKey)
		return
	}
	sort.SliceStable(wildcards, func(i, j int) bool {
		return len(wildcards[i].Prefix) > len(wildcards[j].Prefix)
	})
	envelope := map[string]any{
		"schema_version": channelCacheSchemaVersion,
		"entries":        wildcards,
		"updated_at":     channel.UpdatedAt.Unix(),
	}
	value, err := json.Marshal(envelope)
	if err != nil {
		slog.Warn("channel cache: marshal wildcard mapping failed", "key", wildcardKey, "error", err)
		return
	}
	w.setEx(ctx, wildcardKey, string(value))
}

// deleteGroupKeys 删除给定 (group, platforms) 下所有 K1~K5 键。
// 由于读侧依赖 K1 缺失即视为"无渠道",这里仅需删除 meta + 通配符 envelope
// 即可让网关进入降级路径。残留的精确 K2/K4 即便存活,因 K1 已缺失,
// ChannelCacheReader.GetChannelModelPricing / lookupMapping 都不会读到它们
// (lookupMapping 还会被读到一次,但 ResolveChannelMapping 读 K1 后会先返回)。
//
// 为了双保险,我们仍然主动删除 wildcard envelope (体积大)。精确 key 等 TTL
// 自然过期。
func (w *CacheWriter) deleteGroupKeys(ctx context.Context, groupID int64, platforms []string) {
	for _, p := range platforms {
		platform := normalizePlatformValue(p)
		if platform == "" {
			continue
		}
		w.del(ctx,
			fmt.Sprintf("plugin:channel:meta:%d:%s", groupID, platform),
			fmt.Sprintf("plugin:channel:wildcard:pricing:%d:%s", groupID, platform),
			fmt.Sprintf("plugin:channel:wildcard:mapping:%d:%s", groupID, platform),
		)
	}
}

func (w *CacheWriter) writeSchemaVersion(ctx context.Context) error {
	cctx, cancel := context.WithTimeout(ctx, channelCacheWriteTimeout)
	defer cancel()
	return w.redis.Set(cctx, "plugin:channel:schema_version", channelCacheSchemaVersion)
}

func (w *CacheWriter) setEx(ctx context.Context, key, value string) {
	cctx, cancel := context.WithTimeout(ctx, channelCacheWriteTimeout)
	defer cancel()
	if err := w.redis.SetEx(cctx, key, value, w.ttl); err != nil {
		slog.Warn("channel cache: setex failed", "key", key, "error", err)
	}
}

func (w *CacheWriter) del(ctx context.Context, keys ...string) {
	if len(keys) == 0 {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, channelCacheWriteTimeout)
	defer cancel()
	if err := w.redis.Del(cctx, keys...); err != nil {
		slog.Warn("channel cache: del failed", "keys", keys, "error", err)
	}
}

func (w *CacheWriter) publishInvalidate(ctx context.Context, scope string, groupIDs []int64, reason string) {
	payload := map[string]any{
		"schema_version": channelCacheSchemaVersion,
		"scope":          scope,
		"group_ids":      groupIDs,
		"platforms":      []string{},
		"reason":         reason,
		"ts":             time.Now().Unix(),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		slog.Warn("channel cache: marshal invalidate payload failed", "error", err)
		return
	}
	cctx, cancel := context.WithTimeout(ctx, channelCacheWriteTimeout)
	defer cancel()
	if err := w.redis.Publish(cctx, channelCacheInvalidateChannel, body); err != nil {
		slog.Warn("channel cache: publish invalidate failed", "error", err)
	}
}

// platformsFromPricing 取出 channel.ModelPricing + channel.ModelMapping 中
// 出现过的 platform 集合，用于在缺少 group→platform 映射时尽量删干净残留。
func (w *CacheWriter) platformsFromPricing(channel *Channel) []string {
	seen := make(map[string]struct{})
	for _, p := range channel.ModelPricing {
		platform := normalizePlatformValue(p.Platform)
		if platform != "" {
			seen[platform] = struct{}{}
		}
	}
	for platform := range channel.ModelMapping {
		platform = normalizePlatformValue(platform)
		if platform != "" {
			seen[platform] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	return out
}

// --- 序列化辅助 ---

// pricingPayloadStruct 与 ChannelCacheReader.channelPricingPayload 字段对齐。
type pricingPayloadStruct struct {
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
	Intervals        []intervalPayloadStruct  `json:"intervals"`
	UpdatedAt        int64                    `json:"updated_at"`
}

type intervalPayloadStruct struct {
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

func (p pricingPayloadStruct) withModels(models []string) pricingPayloadStruct {
	p.Models = models
	return p
}

func pricingPayload(p *ChannelModelPricing) pricingPayloadStruct {
	models := make([]string, 0, len(p.Models))
	for _, m := range p.Models {
		models = append(models, normalizeModelValue(m))
	}
	intervals := make([]intervalPayloadStruct, 0, len(p.Intervals))
	for _, iv := range p.Intervals {
		intervals = append(intervals, intervalPayloadStruct{
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
		})
	}
	billingMode := string(p.BillingMode)
	if billingMode == "" {
		billingMode = string(BillingModeToken)
	}
	return pricingPayloadStruct{
		SchemaVersion:    channelCacheSchemaVersion,
		ID:               p.ID,
		ChannelID:        p.ChannelID,
		Platform:         normalizePlatformValue(p.Platform),
		Models:           models,
		BillingMode:      billingMode,
		InputPrice:       p.InputPrice,
		OutputPrice:      p.OutputPrice,
		CacheWritePrice:  p.CacheWritePrice,
		CacheReadPrice:   p.CacheReadPrice,
		ImageOutputPrice: p.ImageOutputPrice,
		PerRequestPrice:  p.PerRequestPrice,
		Intervals:        intervals,
		UpdatedAt:        p.UpdatedAt.Unix(),
	}
}

type wildcardPricingItem struct {
	Prefix  string               `json:"prefix"`
	Pricing pricingPayloadStruct `json:"pricing"`
}

type wildcardMappingItem struct {
	Prefix string `json:"prefix"`
	Target string `json:"target"`
}

// --- 工具函数 ---

func collectGroupIDs(channels []Channel) []int64 {
	out := make([]int64, 0)
	for i := range channels {
		out = append(out, channels[i].GroupIDs...)
	}
	return out
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func normalizePlatformValue(p string) string {
	return strings.ToLower(strings.TrimSpace(p))
}

func normalizeModelValue(m string) string {
	return strings.ToLower(strings.TrimSpace(m))
}
