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
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/platforms"
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/wildcard"
)

// 本文件实现 CacheWriter：channel-management 插件向 Redis 写入网关读取的
// 渠道元信息 (K1) 与模型映射 (K4 / K5) 数据。键格式与字段语义遵循
// plugins/channel-management/GATEWAY_CACHE_SPEC.md（v1）。
//
// 设计要点：
//   - P4 拆分: pricing (K2 / K3) 已经从 Redis 通道下线, 改由
//     PricingExtension gRPC stream 推送到 host in-memory PricingOverrideCache。
//     本写侧只剩 meta / mapping 三类 key, 因为 PricingOverride proto 不携带
//     restrict_models / channel_id / model→model mapping 字段。RedisRaw
//     capability 因此仍然保留。
//   - SDK 暴露的 RedisClient 没有 Pipeline / MGET，写侧只能逐条 SetEx。
//     对单 (group, platform) 的键数 ≈ 1 (meta) + N (exact mapping) +
//     1 (wildcard mapping)。渠道 CRUD 是低频路径，串行写入完全可接受。
//   - 写入失败只记 warn 不返回错误，避免缓存层故障阻塞 CRUD 主路径。
//     TTL（15 min）+ 下次 CRUD 重建会自然兜底。
//   - InvalidateCache 基于"旧 GroupIDs / 旧 ModelPricing 的 platform 集合"
//     推断要删的 key — pricing 走 in-memory cache 后, 旧 platform 集合仍是
//     最准确的 mapping platform 来源(因为 ModelMapping map 也按 platform 分桶)。

const (
	// channelCacheSchemaVersion 与 channel_cache_reader.go 保持一致。
	channelCacheSchemaVersion = "v1"

	// channelCacheTTLDefault 见 GATEWAY_CACHE_SPEC.md §5。
	channelCacheTTLDefault = 15 * time.Minute

	// channelCacheWriteTimeout 单次 Redis 写入 / 删除的上下文超时。
	channelCacheWriteTimeout = 2 * time.Second

	// 失效广播频道 / scope（GATEWAY_CACHE_SPEC.md §6.2）。
	channelCacheInvalidateChannel    = "plugin:channel:invalidate"
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
// 会因此跳过对应分组的 K1 / K4 / K5 写入，但仍会保留 InvalidateCache 删除路径）。
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

// writeChannelForGroup 把 channel 拆解为 K1/K4/K5 写入 Redis。
//
// P4 注：K2/K3（pricing / wildcard pricing）已经从 Redis 通道下线，
// 改由 PricingExtension gRPC + in-memory PricingOverrideCache 提供，
// 见 plugins/channel-management/internal/pricing。本写侧仅保留
// meta（K1）、mapping（K4 / K5）三类 key — 它们承载的字段（restrict
// flag / channel_id / model→model mapping）当前不在 PricingOverride
// proto 内，仍由 ChannelCacheReader 直接从 Redis 读。
func (w *CacheWriter) writeChannelForGroup(ctx context.Context, channel *Channel, groupID int64, platform string) {
	platform = normalizePlatformValue(platform)
	if platform == "" {
		return
	}

	w.writeMeta(ctx, channel, groupID, platform)
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
		if rawPrefix, isWild := wildcard.SplitSuffix(srcLower); isWild {
			wildcards = append(wildcards, wildcardMappingItem{
				Prefix: rawPrefix,
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

// deleteGroupKeys 删除给定 (group, platforms) 下的 K1 + K5 wildcard envelope。
//
// 读侧依赖 K1 缺失即视为"无渠道"，删除 K1 已经能让网关进入降级路径；
// 我们仍然主动删除 K5 wildcard envelope（体积大，残留浪费内存），
// 精确 K4 mapping 留给 TTL 自然过期。
//
// P4 注：K2 / K3（pricing / wildcard pricing）已经从写侧下线，删除路径
// 也不再涉及它们 — 读侧 ChannelCacheReader.GetChannelModelPricing 现在
// 只读 PricingOverrideCache，Redis 上即便有残留 key 也不会被命中。
func (w *CacheWriter) deleteGroupKeys(ctx context.Context, groupID int64, platforms []string) {
	for _, p := range platforms {
		platform := normalizePlatformValue(p)
		if platform == "" {
			continue
		}
		w.del(ctx,
			fmt.Sprintf("plugin:channel:meta:%d:%s", groupID, platform),
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

// normalizePlatformValue 单点委托到 platforms.Normalize；保留 wrapper 以
// 维持本文件原有调用面，调用方读起来仍是"normalize platform"语义。
func normalizePlatformValue(p string) string {
	return platforms.Normalize(p)
}

func normalizeModelValue(m string) string {
	return strings.ToLower(strings.TrimSpace(m))
}
