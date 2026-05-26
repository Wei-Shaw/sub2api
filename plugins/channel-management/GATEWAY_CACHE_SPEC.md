# Gateway Cache Specification（渠道→网关 Redis 缓存契约）

> **目的**：定义 channel-management 插件向 Redis 写入、核心 Gateway 直接读取的缓存数据契约。该契约一旦发布即视为稳定接口；插件与核心两侧必须严格遵守键格式、字段名、版本号与失效策略。

---

## 1. 设计原则

| # | 原则 | 含义 |
|---|------|------|
| 1 | **核心业务无感** | 核心只认 Redis Key 和 JSON 字段，不知道这是"渠道"数据。插件可被替换，键名是契约。 |
| 2 | **亚毫秒读取** | 热路径只允许 GET / MGET / HGET。禁止 SCAN / KEYS / Lua 脚本。 |
| 3 | **写多读多但读 ≫ 写** | 渠道 CRUD 频率低（人工配置），网关请求频率高（每请求 1~3 次读）。优化读侧。 |
| 4 | **最终一致** | 渠道更新后允许秒级延迟。失效采用 publish + TTL 双保险，不要求强一致。 |
| 5 | **Cache-First** | 网关只查 Redis；缓存缺失走"安全降级"（默认无渠道、无映射、无限制），不回源调用插件 gRPC。 |
| 6 | **向前兼容** | JSON 增加字段不破坏现有读侧；删除/重命名字段必须升级 schema 版本号。 |
| 7 | **跨平台严格隔离** | antigravity / anthropic / openai / gemini 各自独立命名空间，不跨平台匹配。 |

---

## 2. Key 命名空间

所有键统一前缀：

```
plugin:channel:<resource>:<groupID>:<platform>[:<model>]
```

| 元素 | 取值 |
|------|------|
| `plugin:channel:` | 固定前缀，标识本插件命名空间 |
| `<resource>` | `pricing` / `mapping` / `wildcard:pricing` / `wildcard:mapping` / `meta` |
| `<groupID>` | 分组 ID（int64，十进制字符串） |
| `<platform>` | `antigravity` / `anthropic` / `openai` / `gemini` 等小写平台标识 |
| `<model>` | 模型名小写。包含 `*` 的通配符模型存放于 `wildcard:*` 资源下。 |

> **保留前缀**：`plugin:channel:` 是本插件独占前缀，其它插件禁止占用。

### 2.1 全量键清单

| # | Key 模式 | 类型 | 说明 |
|---|----------|------|------|
| K1 | `plugin:channel:meta:{groupID}:{platform}` | String (JSON) | 渠道元信息（ID、限制开关、计费来源、特性 JSON 等） |
| K2 | `plugin:channel:pricing:{groupID}:{platform}:{model}` | String (JSON) | 精确匹配模型的渠道定价条目 |
| K3 | `plugin:channel:wildcard:pricing:{groupID}:{platform}` | String (JSON) | 通配符定价数组（按前缀长度降序） |
| K4 | `plugin:channel:mapping:{groupID}:{platform}:{model}` | String | 精确匹配模型的映射目标（纯字符串） |
| K5 | `plugin:channel:wildcard:mapping:{groupID}:{platform}` | String (JSON) | 通配符映射数组（按前缀长度降序） |
| K6 | `plugin:channel:schema_version` | String | schema 版本号（如 `v1`） |
| P1 | `plugin:channel:invalidate` | Pub/Sub Channel | 渠道变更广播频道 |

> **不存在 = 无配置**。Gateway 读取 K1 返回空时，应视为该 (group, platform) 没有渠道关联，所有后续读取（K2~K5）跳过。

---

## 3. 数据格式（JSON Schema）

UTF-8，无 BOM。整数使用 number（int64 不超过 2^53；超出用字符串）。价格字段统一 `*float64`（USD per token / per request），`null` 表示未配置。

### 3.1 K1 — Channel Meta

```json
{
  "schema_version": "v1",
  "channel_id": 42,
  "billing_model_source": "channel_mapped",
  "restrict_models": true,
  "features": "[\"premium\"]",
  "platform": "antigravity",
  "updated_at": 1729785600
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `schema_version` | string | 必填，当前 `"v1"` |
| `channel_id` | int64 | 渠道主键 |
| `billing_model_source` | string | `"requested"` / `"upstream"` / `"channel_mapped"`，空字符串等价于 `"channel_mapped"` |
| `restrict_models` | bool | 是否启用模型限制 |
| `features` | string | 特性 JSON 字符串（透传到支付页面），可为空 |
| `platform` | string | 冗余字段，便于日志/调试 |
| `updated_at` | int64 | Unix 秒时间戳 |

### 3.2 K2 — Channel Pricing（精确模型）

```json
{
  "schema_version": "v1",
  "id": 101,
  "channel_id": 42,
  "platform": "antigravity",
  "models": ["claude-opus-4-6"],
  "billing_mode": "token",
  "input_price": 0.000003,
  "output_price": 0.000015,
  "cache_write_price": 0.0000037,
  "cache_read_price": 0.0000003,
  "image_output_price": null,
  "per_request_price": null,
  "intervals": [
    {
      "id": 0,
      "min_tokens": 0,
      "max_tokens": 200000,
      "tier_label": "",
      "input_price": 0.000003,
      "output_price": 0.000015,
      "cache_write_price": 0.0000037,
      "cache_read_price": 0.0000003,
      "per_request_price": null,
      "sort_order": 0
    }
  ],
  "updated_at": 1729785600
}
```

字段对应 `service.ChannelModelPricing`：

| 字段 | 类型 | 说明 |
|------|------|------|
| `schema_version` | string | 当前 `"v1"` |
| `id` | int64 | DB 主键（审计用，读侧可忽略） |
| `channel_id` | int64 | 关联渠道 |
| `platform` | string | 与 key 中一致 |
| `models` | []string | 绑定的模型列表（小写） |
| `billing_mode` | string | `"token"` / `"per_request"` / `"image"`，空 → `"token"` |
| `input_price` 等 | *float64 | flat 价格；nil → JSON `null` |
| `intervals` | []PricingInterval | 区间定价。空数组与缺失等价 |

### 3.3 K3 / K5 — Wildcard 数组

通配符按 **前缀长度降序** 排序后整体写入一个 JSON 数组（一次 GET 拿到全部，避免多次往返）。

K3 元素：

```json
{
  "prefix": "claude-opus-",
  "pricing": { "...": "与 K2 中 ChannelModelPricing 同构" }
}
```

K5 元素：

```json
{
  "prefix": "gpt-4o-",
  "target": "gpt-4o-2024-08-06"
}
```

整个数组顶层使用 envelope：

```json
{
  "schema_version": "v1",
  "entries": [],
  "updated_at": 1729785600
}
```

### 3.4 PricingInterval 内嵌结构

字段对应 `service.PricingInterval`：

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | int64 | DB 主键 |
| `min_tokens` | int | 区间下界（不含） |
| `max_tokens` | *int | 区间上界（含），nil → JSON `null` |
| `tier_label` | string | 按次/图片层级标签 |
| `input_price` 等 | *float64 | 区间内价格；nil → `null` |
| `sort_order` | int | 写侧已按 `min_tokens` 升序排好 |

> **区间语义**：左开右闭 `(min, max]`。读侧匹配 `tokens > min && (max == nil || tokens <= max)`，与 `service.FindMatchingInterval` 一致。

### 3.5 K4 — Mapping（精确模型）

值为**纯字符串**目标模型名（小写），不是 JSON：

```
plugin:channel:mapping:1024:antigravity:claude-opus-4-6 -> "claude-opus-4-7"
```

> 选择纯字符串以最小化反序列化成本。

---

## 4. 限制（restrict_models）查询语义

模型限制是 K1 + K2 / K3 的派生信息，不单独建表：

```
IsModelRestricted(group, platform, model) =
    K1(group, platform).restrict_models == true
    AND lookup_pricing(group, platform, model) == nil
```

`lookup_pricing` 顺序：

1. 直接 GET K2(group, platform, lower(model))，命中即返回。
2. GET K3(group, platform)，得到通配符数组后从前缀长度最长开始匹配，命中即返回。
3. 都未命中 → nil。

> **不写独立的 restrict key**：避免与 pricing 双写不一致。

---

## 5. TTL 策略

| 类型 | 默认 TTL | 说明 |
|------|----------|------|
| K1 / K2 / K3 / K4 / K5 | **15 分钟** | 慢于核心原 `channelCacheTTL=10m`，配合 P1 主动失效后能在异常情况下兜底 |
| K6（schema_version） | 永久 | 由插件启动时写入，重启不清除 |

写侧（插件）**全量重建**：
1. 读 DB 全量渠道数据。
2. 用 pipeline 一次性 SETEX 所有 K1/K2/K3/K4/K5（带 TTL）。
3. 写入 K6 = "v1"。
4. 发布 P1 全量失效消息。

> **不主动删除 stale key**：依赖 TTL 自动清理。废弃的 (group, platform) 在 15 分钟内自然过期。如果业务关键路径要求立即生效，插件可在 CRUD 时显式 DEL 受影响 key。

---

## 6. 缓存失效（写侧职责）

### 6.1 触发时机（插件侧）

| 操作 | 影响范围 | 失效粒度 |
|------|----------|----------|
| 创建渠道 | 关联的所有 (group, platform) | 重建对应 K1/K2/K3/K4/K5 |
| 更新渠道基础字段 | 同上 | 重建 K1 |
| 更新渠道定价 | 同上 | 重建 K2/K3 + K1（restrict 派生依赖） |
| 更新渠道映射 | 同上 | 重建 K4/K5 |
| 删除渠道 | 同上 | DEL 全部相关 key |
| 修改渠道关联的分组列表 | 旧分组 + 新分组 | DEL 旧 + 重建新 |
| 修改分组的 platform | 该分组所有平台 key | DEL 全部 + 重建当前 platform |

### 6.2 Pub/Sub 通知（P1）

频道：`plugin:channel:invalidate`

Payload（JSON）：

```json
{
  "schema_version": "v1",
  "scope": "group",
  "group_ids": [1024, 1025],
  "platforms": ["antigravity"],
  "reason": "pricing_updated",
  "ts": 1729785600
}
```

| 字段 | 含义 |
|------|------|
| `scope` | `"group"` / `"all"`。`"all"` 表示全量刷新（如插件启动重建后） |
| `group_ids` | 受影响分组列表；`scope=all` 时为空 |
| `platforms` | 受影响平台列表；为空表示所有平台 |
| `reason` | 调试用途（如 `pricing_updated` / `channel_deleted` / `bootstrap`） |
| `ts` | Unix 秒时间戳 |

> **核心读侧无需订阅**：核心是 cache-first，TTL 自然过期 + 写侧立即重建即可保证最终一致。订阅是可选优化（用于二级 in-memory 缓存的提前失效），不在本次实现范围内。

---

## 7. 读侧（核心 Gateway）行为

### 7.1 标准查询流程

```
meta := GET K1(group, platform)
if meta == nil:
    return ChannelMappingResult{MappedModel: requested}
mapping := lookup_mapping(group, platform, requested)
pricing := lookup_pricing(group, platform, mapping_or_requested)
restricted := meta.restrict_models && pricing == nil
```

### 7.2 缓存缺失（cache miss）的安全降级

| 场景 | 行为 |
|------|------|
| K1 缺失 | `ChannelMappingResult{MappedModel: requested, Mapped: false, ChannelID: 0, BillingModelSource: ""}`，restricted=false |
| K2/K3 缺失 | pricing=nil → 走 LiteLLM/Fallback 路径 |
| K4/K5 缺失 | MappedModel=requested，Mapped=false |
| Redis 整体不可用 | 同 K1 缺失。**记日志 + 指标**，不阻塞请求 |

### 7.3 性能预算

| 调用 | 操作 | 预算 |
|------|------|------|
| `ResolveChannelMapping` | 1×GET (K1) + 1×GET (K4 或 K5) | < 0.5 ms |
| `IsModelRestricted` | 1×GET (K1) + 1×GET (K2 或 K3) | < 0.5 ms |
| `GetChannelModelPricing` | 1×GET (K2) ± 1×GET (K3) | < 0.5 ms |

> 推荐网关在请求维度合并：一次 MGET 拿 K1/K4/K2，避免重复往返。

---

## 8. 版本与演进

* 当前 schema 版本：**v1**
* 兼容性：v1 内只允许新增可选字段；新增字段读侧未识别时应忽略。
* 不兼容变更必须升级到 v2，写侧并行写入 v1 + v2，读侧灰度切换后再下线 v1。

读侧启动时检查 K6：缺失或值与读侧支持版本不一致 → 记 error 日志、上报告警，但仍按已有键继续工作（保护可用性）。

---

## 9. 与现有 Go 类型的映射关系

| 缓存对象 | Go 类型 | 备注 |
|----------|---------|------|
| K1 | `service.Channel`（精简） | 不缓存 GroupIDs / ModelPricing / ModelMapping |
| K2 element | `service.ChannelModelPricing` | 1:1 |
| K2.intervals[] | `service.PricingInterval` | 1:1 |
| K3 | `[]wildcardPricingEntry` | 等价于现有 `cache.wildcardByGroupPlatform` |
| K4 | `string` | 等价于现有 `cache.mappingByGroupModel` 的 value |
| K5 | `[]wildcardMappingEntry` | 等价于现有 `cache.wildcardMappingByGP` |
| `ChannelMappingResult` | 由读侧组装 | 不缓存 |

---

## 10. 安全 / 限制

* **不缓存敏感数据**：本契约只涉及定价/映射/限制元信息，不涉及账号凭证、用户余额等。
* **键大小约束**：单 key value 上限 1 MB（go-redis 默认）。极端情况下若某 (group, platform) 通配符条目数过多导致 K3 超限，写侧需按 platform 分片（v2 改进）。
* **模型名规范化**：所有 model 字段写入前 `strings.ToLower(strings.TrimSpace(...))`，读侧同样规范化后再查。

---

## 11. 受影响的 GatewayService 方法（核心读侧改造清单）

后续在 `backend/internal/service/gateway_service.go` 与 `openai_gateway_service.go` 中需要替换的调用点（仅枚举，本次不修改代码）：

| 当前调用 | 替换为 | 备注 |
|----------|--------|------|
| `s.channelService.ResolveChannelMapping(ctx, gid, model)` | `s.channelCacheReader.ResolveChannelMapping(ctx, gid, platform, model)` | 平台从 `apiKey.Group.Platform` 取 |
| `s.channelService.IsModelRestricted(ctx, gid, model)` | `s.channelCacheReader.IsModelRestricted(ctx, gid, platform, model)` | 同上 |
| `s.channelService.ResolveChannelMappingAndRestrict(ctx, &gid, model)` | `s.channelCacheReader.ResolveChannelMappingAndRestrict(ctx, gid, platform, model)` | restricted 已不再使用，可拆分调用 |
| `s.channelService.GetChannelForGroup(ctx, gid)` | `s.channelCacheReader.GetChannelMeta(ctx, gid, platform)` | 仅返回元信息，不再返回完整 Channel |
| `s.channelService.GetChannelModelPricing(ctx, gid, model)` | `s.channelCacheReader.GetChannelModelPricing(ctx, gid, platform, model)` | 通过 model_pricing_resolver 调用 |
| `s.resolveChannelPricing(...)` 中的 resolver.Resolve | resolver 内部把 channelService 替换为 channelCacheReader | 需要在 NewModelPricingResolver 注入新依赖 |

`needsUpstreamChannelRestrictionCheck` 只读取 `RestrictModels` + `BillingModelSource`，K1 已包含该信息。
