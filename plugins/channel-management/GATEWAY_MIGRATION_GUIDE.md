# Gateway Migration Guide：从 ChannelService 切换到 ChannelCacheReader

本文记录 `backend/internal/service/gateway_service.go`、`openai_gateway_service.go` 与 `model_pricing_resolver.go` 中所有 `s.channelService.*` 调用点的替换方案。

> **执行前提**：core-cleaner 已经把 `channel_service.go` / `channel.go` 等渠道业务代码从 `backend/internal/service/` 中迁出。本指南仅描述 gateway 侧的"调用点重写"，不涉及插件 / repo 改造。

---

## 0. 前置准备

### 0.1 在 `GatewayService` / `OpenAIGatewayService` 上把 `channelService` 字段替换为 `channelCacheReader`

```go
// 旧
type GatewayService struct {
    // ...
    channelService *ChannelService
    // ...
}

// 新
type GatewayService struct {
    // ...
    channelCacheReader *ChannelCacheReader
    // ...
}
```

`OpenAIGatewayService` 同理。`ChannelCacheReader` 已经存在于 `backend/internal/service/channel_cache_reader.go`，直接注入。

### 0.2 在 `model_pricing_resolver.go` 替换依赖

```go
// 旧
type ModelPricingResolver struct {
    channelService *ChannelService
    billingService *BillingService
}

func NewModelPricingResolver(channelService *ChannelService, billingService *BillingService) *ModelPricingResolver

// 新
type ModelPricingResolver struct {
    channelCacheReader *ChannelCacheReader
    billingService     *BillingService
}

func NewModelPricingResolver(reader *ChannelCacheReader, billingService *BillingService) *ModelPricingResolver
```

### 0.3 平台参数从哪里取

`ChannelCacheReader` 的所有方法新增了 `platform string` 参数。Gateway 内调用约定：

| 调用上下文 | 取值来源 |
|-----------|----------|
| `apiKey` 在手 | `apiKey.Group.Platform`（`apiKey.Group != nil` 时） |
| 仅有 `groupID *int64` | 通过 `s.groupService.GetGroupByID(ctx, *groupID)` 拿 `Platform`；建议在调用 `checkChannelPricingRestriction` 之前提前解析 |
| 没有 group | 跳过渠道相关检查（保持当前行为） |

> 当前 `gateway_service.go:1184` 处的 `checkChannelPricingRestriction` 调用之前已经把 `group` 解析出来（`s.resolveGatewayGroup`），可以直接把 `group.Platform` 沿调用链传下去，无需第二次查询。

---

## 1. gateway_service.go 调用点替换清单

### 1.1 `ResolveChannelMapping`（line 7991-7997）

```go
// 旧
func (s *GatewayService) ResolveChannelMapping(ctx context.Context, groupID int64, model string) ChannelMappingResult {
    if s.channelService == nil {
        return ChannelMappingResult{MappedModel: model}
    }
    return s.channelService.ResolveChannelMapping(ctx, groupID, model)
}

// 新（增加 platform 参数）
func (s *GatewayService) ResolveChannelMapping(ctx context.Context, groupID int64, platform, model string) ChannelMappingResult {
    if s.channelCacheReader == nil {
        return ChannelMappingResult{MappedModel: model}
    }
    return s.channelCacheReader.ResolveChannelMapping(ctx, groupID, platform, model)
}
```

**调用方影响**：所有外部调用点（handler 层 / 其它 service）需补传 `platform`，建议从 `apiKey.Group.Platform` 取。

### 1.2 `IsModelRestricted`（line 8004-8010）

```go
// 旧
func (s *GatewayService) IsModelRestricted(ctx context.Context, groupID int64, model string) bool {
    if s.channelService == nil {
        return false
    }
    return s.channelService.IsModelRestricted(ctx, groupID, model)
}

// 新
func (s *GatewayService) IsModelRestricted(ctx context.Context, groupID int64, platform, model string) bool {
    if s.channelCacheReader == nil {
        return false
    }
    return s.channelCacheReader.IsModelRestricted(ctx, groupID, platform, model)
}
```

### 1.3 `ResolveChannelMappingAndRestrict`（line 8012-8019）

```go
// 旧
func (s *GatewayService) ResolveChannelMappingAndRestrict(ctx context.Context, groupID *int64, model string) (ChannelMappingResult, bool) {
    if s.channelService == nil {
        return ChannelMappingResult{MappedModel: model}, false
    }
    return s.channelService.ResolveChannelMappingAndRestrict(ctx, groupID, model)
}

// 新
func (s *GatewayService) ResolveChannelMappingAndRestrict(ctx context.Context, groupID *int64, platform, model string) (ChannelMappingResult, bool) {
    if s.channelCacheReader == nil {
        return ChannelMappingResult{MappedModel: model}, false
    }
    return s.channelCacheReader.ResolveChannelMappingAndRestrict(ctx, groupID, platform, model)
}
```

> `restricted` 第二返回值在新旧两侧都恒为 `false`（限制检查已下沉到调度阶段），只是签名上保留兼容。后续可考虑拆掉这个返回值。

### 1.4 `checkChannelPricingRestriction`（line 8021-8034）

```go
// 旧
func (s *GatewayService) checkChannelPricingRestriction(ctx context.Context, groupID *int64, requestedModel string) bool {
    if groupID == nil || s.channelService == nil || requestedModel == "" {
        return false
    }
    mapping := s.channelService.ResolveChannelMapping(ctx, *groupID, requestedModel)
    billingModel := billingModelForRestriction(mapping.BillingModelSource, requestedModel, mapping.MappedModel)
    if billingModel == "" {
        return false
    }
    return s.channelService.IsModelRestricted(ctx, *groupID, billingModel)
}

// 新（新增 platform）
func (s *GatewayService) checkChannelPricingRestriction(ctx context.Context, groupID *int64, platform, requestedModel string) bool {
    if groupID == nil || s.channelCacheReader == nil || requestedModel == "" || platform == "" {
        return false
    }
    mapping := s.channelCacheReader.ResolveChannelMapping(ctx, *groupID, platform, requestedModel)
    billingModel := billingModelForRestriction(mapping.BillingModelSource, requestedModel, mapping.MappedModel)
    if billingModel == "" {
        return false
    }
    return s.channelCacheReader.IsModelRestricted(ctx, *groupID, platform, billingModel)
}
```

**对应调用点（须同步改）**：
- `gateway_service.go:1184` — 此时上下文中已有 `platform`（line 1176/1179 处赋值），直接传入。
- `gateway_service.go:1237` — 需要在 `checkClaudeCodeRestriction` 之后从 `group.Platform` 取一次。

### 1.5 `isUpstreamModelRestrictedByChannel`（line 8051-8062）

```go
// 旧
return s.channelService.IsModelRestricted(ctx, groupID, upstreamModel)

// 新（platform 从 account 取，如：account.Platform）
return s.channelCacheReader.IsModelRestricted(ctx, groupID, account.Platform, upstreamModel)
```

`account.Platform` 已经在 `Account` 结构中存在，无需额外查询。

### 1.6 `needsUpstreamChannelRestrictionCheck`（line 8073-8086）

```go
// 旧
ch, err := s.channelService.GetChannelForGroup(ctx, *groupID)
if err != nil {
    slog.Warn("failed to check channel upstream restriction", "group_id", *groupID, "error", err)
    return false
}
if ch == nil || !ch.RestrictModels {
    return false
}
return ch.BillingModelSource == BillingModelSourceUpstream

// 新（用 GetChannelMeta 替代 GetChannelForGroup；调用方需提前传入 platform）
func (s *GatewayService) needsUpstreamChannelRestrictionCheck(ctx context.Context, groupID *int64, platform string) bool {
    if groupID == nil || s.channelCacheReader == nil || platform == "" {
        return false
    }
    meta, ok := s.channelCacheReader.GetChannelMeta(ctx, *groupID, platform)
    if !ok || !meta.RestrictModels {
        return false
    }
    return meta.BillingModelSource == BillingModelSourceUpstream
}
```

**调用方影响**：`isStickyAccountUpstreamRestricted`（line 8091）需要把 `account.Platform` 传下去。

### 1.7 `resolveChannelPricing` / `model_pricing_resolver.go:95`

```go
// 旧（model_pricing_resolver.go applyChannelOverrides）
chPricing := r.channelService.GetChannelModelPricing(ctx, groupID, model)

// 新（PricingInput 增加 Platform 字段）
chPricing := r.channelCacheReader.GetChannelModelPricing(ctx, groupID, input.Platform, model)
```

```go
// service/model_pricing_resolver.go
type PricingInput struct {
    Model    string
    GroupID  *int64
    Platform string // 新增
}
```

`gateway_service.go:7796` 的 `resolveChannelPricing` 需要把 `apiKey.Group.Platform` 塞进 `PricingInput`：

```go
// 新
gid := apiKey.Group.ID
resolved := s.resolver.Resolve(ctx, PricingInput{
    Model:    billingModel,
    GroupID:  &gid,
    Platform: apiKey.Group.Platform,
})
```

---

## 2. openai_gateway_service.go 调用点替换清单

替换模式与 `gateway_service.go` 完全对称：

| 行号 | 旧调用 | 新调用 |
|------|--------|--------|
| 404-408 | `s.channelService.ResolveChannelMapping(ctx, groupID, model)` | `s.channelCacheReader.ResolveChannelMapping(ctx, groupID, platform, model)` |
| 411-416 | `s.channelService.IsModelRestricted(ctx, groupID, model)` | `s.channelCacheReader.IsModelRestricted(ctx, groupID, platform, model)` |
| 419-425 | `s.channelService.ResolveChannelMappingAndRestrict(ctx, groupID, model)` | `s.channelCacheReader.ResolveChannelMappingAndRestrict(ctx, groupID, platform, model)` |
| 428-438 | `checkChannelPricingRestriction` 内两次 `s.channelService.*` | 改为 `s.channelCacheReader.*` 并补传 `platform` |
| 440-448 | `s.channelService.IsModelRestricted(ctx, groupID, upstreamModel)` | `s.channelCacheReader.IsModelRestricted(ctx, groupID, account.Platform, upstreamModel)` |
| 451-464 | `s.channelService.GetChannelForGroup(ctx, *groupID)` → 检查 `RestrictModels` / `BillingModelSource` | `s.channelCacheReader.GetChannelMeta(ctx, *groupID, platform)` 后读 `meta.RestrictModels` / `meta.BillingModelSource` |
| 1209 / 1389 | `s.checkChannelPricingRestriction(ctx, groupID, requestedModel)` | `s.checkChannelPricingRestriction(ctx, groupID, platform, requestedModel)` |

---

## 3. 行为差异 / 注意事项

### 3.1 缓存缺失的安全降级

`ChannelCacheReader` 在 Redis 缺键 / 不可用时返回"无渠道"等价值（见 `GATEWAY_CACHE_SPEC.md §7.2`）：

| 旧行为（ChannelService） | 新行为（ChannelCacheReader） |
|-------------------------|----------------------------|
| 在内存命中本地 cache，缺失时回 DB（`buildCache`） | 直接走 Redis；缺失即返回降级值 |
| DB 失败时短缓存 5s（`channelErrorTTL`） | Redis 不可用时每次请求都尝试 GET（200ms 超时） |
| 跨平台严格隔离 | 同 |
| 大小写归一 | 同 |

写侧（`CacheWriter`）保证 CRUD 后立即重建 K1~K5；读侧 + TTL（15min） + Pub/Sub 兜底。

### 3.2 `restricted` 第二返回值

`ResolveChannelMappingAndRestrict` 的第二返回值在新旧两侧都恒为 `false`。后续 PR 可考虑去掉这个值，但**本次切换不动签名**以减小 diff 面。

### 3.3 `GetChannelForGroup` → `GetChannelMeta`

旧 `GetChannelForGroup` 返回完整 `*Channel`（含 `GroupIDs / ModelPricing / ModelMapping`）。新的 `GetChannelMeta` 仅返回 `ChannelMeta`（`ChannelID / BillingModelSource / RestrictModels / Features / Platform / UpdatedAt`）。

`needsUpstreamChannelRestrictionCheck` 只读了 `RestrictModels` + `BillingModelSource`，因此切换无损。

如果有其它地方依赖 `GroupIDs / ModelPricing / ModelMapping`（例如 admin handler），那些代码不应在 gateway 包内，应已经被 core-cleaner 一并迁出。**切换 PR 前先全量 grep `channelService.GetChannelForGroup`** 确认没有遗漏。

### 3.4 平台参数推导失败

如果上层逻辑算不出 `platform`（例如 `apiKey.Group == nil`），新方法会一律返回降级值（`MappedModel == requestedModel`、`restricted == false`、`pricing == nil`），等同于"无渠道"路径。这与旧行为（`ResolveChannelMapping` 在没有 group 时返回原模型）一致。

### 3.5 sticky session 路径

`isStickyAccountUpstreamRestricted`（gateway_service.go:8091）需要拿到 `account.Platform`。当前签名是 `(ctx, groupID *int64, account *Account, requestedModel string)`，新增 platform 时直接从 `account.Platform` 取，**不要新增参数**避免改动调用方。

```go
// 新
func (s *GatewayService) isStickyAccountUpstreamRestricted(ctx context.Context, groupID *int64, account *Account, requestedModel string) bool {
    if groupID == nil || account == nil {
        return false
    }
    if !s.needsUpstreamChannelRestrictionCheck(ctx, groupID, account.Platform) {
        return false
    }
    return s.isUpstreamModelRestrictedByChannel(ctx, *groupID, account, requestedModel)
}
```

---

## 4. 测试更新

### 4.1 gateway_channel_restriction_test.go

测试目前直接构造 `*GatewayService{channelService: ...}`。切换后改为：

```go
// 旧
svc := &GatewayService{channelService: fakeChannelService}

// 新
svc := &GatewayService{channelCacheReader: NewChannelCacheReader(redisClient)}
// 或注入 mock：channelCacheReader 是 struct，建议引入接口或用 miniredis
```

推荐用 [`miniredis`](https://github.com/alicebob/miniredis)（项目已通过 `pricing_redis_cache_test.go` 等使用）：在测试 setup 时调用与 `CacheWriter` 等价的 SetEx 写入 K1~K5，然后让 `ChannelCacheReader` 直接读。

### 4.2 model_pricing_resolver 测试

`Resolve` 的输入新增 `Platform` 字段，所有测试用例需要补传，否则渠道覆盖路径会被跳过（platform="" → reader 直接返回 nil）。

---

## 5. 切换 PR 检查清单

- [ ] 所有 `s.channelService.*` 调用都被替换为 `s.channelCacheReader.*` 或 `s.resolver.*`（resolver 内部已迁移）
- [ ] `GatewayService` / `OpenAIGatewayService` 的构造函数签名同步更新（`channelCacheReader` 替代 `channelService`）
- [ ] `model_pricing_resolver.PricingInput` 新增 `Platform` 字段，所有调用方补全
- [ ] `gateway_service.go:1184` / `1237` / `8091` 等 helper 调用补传 `platform`
- [ ] `gateway_channel_restriction_test.go` 用 miniredis 替代 fake ChannelService
- [ ] `cd backend && go build ./...` 通过
- [ ] `cd backend && make test-unit` 通过
- [ ] 部署到 beta 验证：`/v1/messages` 命中渠道映射 + `restrict_models=true` 时拒绝未授权模型
