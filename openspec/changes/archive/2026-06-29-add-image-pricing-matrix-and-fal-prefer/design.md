## Context

现状关键事实（探索阶段已确认）：

- `media-prepay-billing/spec.md` 已声明「按分辨率×质量二维计费」与「兼容存量单维定价」，但 spec 未规定档位数量、边界、quality 归一规则、回退顺序，实现端 `NormalizeImageBillingTierOrDefault` 仅支持 1K/2K/4K 三档。
- `groups` 表当前以 `image_price_1k/2k/4k` 三个浮点字段承载 image 计费配置；该字段也是渠道(Channel) 与 BillingService 实际查找的存储位。
- `SelectImageAccountMixed` 已实现 openai+fal 混合候选池，硬编码"openai 优先、fal 兜底"。
- `gpt-image-2` 在 OpenAI 与 fal 公示的报价表为 6 档 size × 3 档 quality 共 18 格：
  - 1024×768 / 1024×1024 / 1024×1536 / 1920×1080 / 2560×1440 / 3840×2160
  - low / medium / high
- 不存在独立的 `groups` capability spec；分组配置语义历来分散在使用它的 capability（pricing-plaza、media-prepay-billing 等）中描述。

## Goals / Non-Goals

**Goals:**
- 让分组可以完整配置 OpenAI/fal 公示的 6×3 二维定价。
- 让计费 size 归档支持任意 (w,h) 输入，向上取档、最大 4K 封顶。
- 让 quality 归一规则在 spec 中固化（auto/缺省 → high）。
- 给 `platform=openai` 分组提供"反转优先级"开关，无须改写调度路径。
- 与现网零迁移：既有分组在不动配置的前提下行为完全不变。

**Non-Goals:**
- 不新增 `groups` capability spec（保持现状，配置语义随用例分布）。
- 不引入"硬只用 fal、不兜底 openai"的开关（兜底是必要的健壮性，反转开关只调整顺序）。
- 不改造 fal 分组——fal 分组本就只调度 fal 账号，无优先级反转语义。
- 不引入按渠道/账号粒度的反转开关（只在分组层面）。
- 不重新设计 `Intervals` / `TierLabel` 的底层计费抽象（在外层包一个查找函数即可命中既有引擎）。

## Decisions

### D1. Size 归档：6 档 + 像素总数升序 + 向上取档 + 4K 封顶

**决策**：以「像素总数」(width × height) 为排序键，升序排列 6 个档位边界。请求实际像素 ≤ 哪个边界，即归入哪个档位；超出最大档位（3840×2160 = 8,294,400 px）时一律封顶到 4K 档。

档位边界（像素总数）：档位 key 直接用「代表分辨率字符串」，与 D4 的 `image_pricing_matrix` 顶层键一一对应：

```
档位 key            代表分辨率      像素上限
1024x768            1024×768        786,432
1024x1024           1024×1024       1,048,576
1024x1536           1024×1536       1,572,864
1920x1080           1920×1080       2,073,600
2560x1440           2560×1440       3,686,400
3840x2160           3840×2160       8,294,400  ← 封顶
```

**为什么用像素总数而不是 max(w,h) / 宽高比**：像素总数能容纳任意 (w,h) 组合，且与 fal/OpenAI 计费实际跟"图像规模"挂钩的语义最贴近，不依赖 1:1 / 16:9 假设。横竖图同像素同价，符合直觉。

**Alternatives 否决**：
- 按 `max(w,h)` 归档：会把 `2560×1440` 与 `3840×2160` 混档（max=3840 与 2560），太粗。
- 按 (w, h) 双边都满足：边界几何不连续，超出某档位某一边时容易跳到 +2 档，对用户反直觉。

### D2. Quality 归一：`auto` / 缺省 → `high`

**决策**：客户端可能传 OpenAI 形式的 `quality: "auto"` 或不传该字段。计费时统一归一到 `high`。`low/medium/high` 保留原值。

**为什么 high 而不是 medium**：保守计费，避免默认值套利刷低价；运营实际数据显示 auto 大多数时间产出 high 质量。用户主动选 low/medium 才便宜。

**OpenAI ↔ fal 视为同义**：两边都用 `low/medium/high` 三档 + auto 默认，无需另行映射。

### D3. 计费表按"分组持有"，与承载账号平台无关

**决策**：同一分组的同一 (size_tier, quality) 请求金额完全由分组的 `image_pricing_matrix` 决定，不论调度选中的是 openai 账号还是 fal 账号。

**为什么**：
- 计费一致性：用户在同一 API Key 下发同一请求，金额可预测，不会因后端路由抖动而忽高忽低。
- 运营可控性：分组定价 = 对客单价；openai 与 fal 实际成本差由运营在分组创建时自行平衡，不与调度耦合。
- 模型简洁：避免在 BillingService 里区分"实际承载平台"再选不同价目表，调度链路与计费链路解耦。

**Alternatives 否决**：按"承载账号平台"决定计费表 → 同请求两价格、可观测困难、UI 需双表。

### D4. 数据模型：分组新增两个独立字段，旧字段保留作回退

**决策**：

```
groups
  image_price_1k        float, nullable   ← 保留，作回退价（不删除）
  image_price_2k        float, nullable   ← 保留
  image_price_4k        float, nullable   ← 保留
  image_pricing_matrix  JSON,  nullable   ← 新增
  image_prefer_fal      bool,  default false ← 新增
```

`image_pricing_matrix` 内部结构：

```json
{
  "1024x768":  { "low": 0.005, "medium": 0.037, "high": 0.145 },
  "1024x1024": { "low": 0.006, "medium": 0.053, "high": 0.211 },
  "1024x1536": { "low": 0.005, "medium": 0.042, "high": 0.165 },
  "1920x1080": { "low": 0.005, "medium": 0.040, "high": 0.158 },
  "2560x1440": { "low": 0.007, "medium": 0.056, "high": 0.222 },
  "3840x2160": { "low": 0.012, "medium": 0.101, "high": 0.401 }
}
```

key 用「`{w}x{h}` 字符串」，便于前后端同构与 JSON 持久化；缺失某 (size, quality) 即视为未配置，触发回退。

### D5. 计费查找回退顺序

```
1. 归档 (size_actual, quality_actual) → (size_tier, quality_norm)
       size: D1 的 6 档向上取档 + 4K 封顶
       quality: D2 的 auto/"" → high

2. 命中 image_pricing_matrix[size_tier][quality_norm]？
       是 → 用此价格，结束
       否 → 步骤 3

3. 按 size_tier 归并到旧字段：
       1024x768 / 1024x1024 / 1024x1536  → image_price_1k
       1920x1080 / 2560x1440             → image_price_2k
       3840x2160                         → image_price_4k
   命中且非空？
       是 → 用此价格（与 quality 无关），结束
       否 → 步骤 4

4. 回退到 LiteLLM 默认价（现状逻辑）。
```

**为什么旧字段映射时丢弃 quality**：旧字段本来就是粗粒度单维定价，用户从未为其指定 quality；为旧字段虚构 quality 维度反而引入歧义。命中旧字段时按 size_tier 一口价是最干净的语义。

### D6. 调度反转：`SelectImageAccountMixed` 增 `preferPlatform` 参数

**决策**：

```go
SelectImageAccountMixed(ctx, openaiCandidates, falCandidates, preferPlatform string) (*Account, error)
```

`preferPlatform=""` 或 `"openai"`：维持现状（openai 优先）。
`preferPlatform="fal"`：fal 优先入池排序，openai 兜底。

调用方：`OpenAIGatewayHandler` 的图片路径，在选定分组后从 `group.ImagePreferFal` 读出，`true` 则传 `"fal"`，否则传 `""`。

**为什么显式参数而非全局开关**：调度参数源于 Group 配置，是请求级语义，不应放进进程级状态。显式参数也方便单测覆盖反转分支。

**Alternatives 否决**：在 `listSchedulableImageAccounts` 里直接按 group.ImagePreferFal 反转入池顺序——耦合面小但隐式，单测不易覆盖；显式参数版本风险更低。

### D7. `image_prefer_fal` 仅在 `platform=openai` 分组生效

**决策**：DTO/Handler 校验：当 `Group.Platform != openai` 时，写入 `image_prefer_fal=true` 应被拒绝（或忽略并返回告警）。后端读取时仅 openai 分组路径会消费该字段。

**为什么**：fal 分组本就只调度 fal 账号，无反转语义；其他平台（anthropic/gemini/antigravity）不参与图片混合候选池，开关无意义且容易误用。

### D8. 回包图片分辨率自检（base64-only）

**决策**：分组（仅 `platform=openai`）新增 `image_decode_size_on_rsp` 布尔开关，默认 `false`。开关 `true` 且回包某张图的 `size` 字段缺失或为 `auto` 时，系统在**异步记账阶段**对该张图的 `b64_json` 内容做最小代价的头部解码，回填该 slot 的 size，再交给 D1/D5 的归档逻辑。

**触发判定（逐张图）**：

```
对 result.ImageOutputSizes / result.ImageOutputBase64 的每个 slot:
  size 非空 且 ∉ {"", "auto"}  → 跳过, 用上游 size
  size ∈ {"", "auto"}          → 解码 b64
                                  ├─ 成功 → 回填 size = "{w}x{h}"
                                  └─ 失败 → 留空, 走 D1 默认档兜底
```

**实现位置**：
- `image_output_accounting.go::openAIImageOutputCounter`：扩展 `(key→base64)` 缓存（仅当 `result` 字段读到 b64 时缓存；URL 不缓存）。新增 `Base64Payloads() map[int]string`（按 `seenOrder` 索引）。
- 新增 `image_billing_size.go::DecodeOpenAIImageOutputSizes(result, group *domain.Group)`：仅在 `group != nil && group.Platform == openai && group.ImageDecodeSizeOnRsp == true` 时执行；遍历 `result.ImageOutputSizes`，对缺失/auto slot 用 base64 payload 喂给 `image.DecodeConfig`（导入 `_ "image/png" / _ "image/jpeg" / _ "image/webp"`），成功回填，失败留空。
- `ApplyOpenAIImageBillingResolution` 改签名：增 `group` 参数（nil 安全），在归档前调用 `DecodeOpenAIImageOutputSizes`。`ApplyForwardImageBillingResolution` 不动（fal 平台不消费此开关）。
- `OpenAIForwardResult` 新增 `ImageOutputBase64 []string`（与 `ImageOutputSizes` 同序、同长度，未知或 URL 模式置空字符串），由 forward 路径在 counter 收尾时统一写入。

**仅 base64**：URL 模式的远程拉取（含 Range 请求、并发控制、超时、scheme 白名单）本次不做，留待后续独立 change。`ImageOutputBase64[i] == ""` 即跳过解码并保持失败兜底语义。

**异步阶段执行的安全性**：`RecordUsage` 已经在异步 goroutine 上跑（gateway 路径已 deferred），解码耗时（PNG/JPEG/WebP 头部 read 通常 < 1ms）不阻塞客户端响应。

**为什么不放 `ResolveImageBillingSize` 内部**：`ResolveImageBillingSize` 是纯函数、被 forward 路径与异步路径共用，不应感知 group/解码副作用。把解码隔离在 `DecodeOpenAIImageOutputSizes` 这一层更易测试、易回滚。

**Alternatives 否决**：
- 在 forward 同步路径解码：阻塞客户端响应，违背"先扣费后退款"流水线时延敏感原则。
- 把开关挂 Account 级：违背"分组一刀切"用户决策；且 schedule 阶段才知道 Account，开关读取与回包处理不在同一上下文。
- 用 `Account.Extra` JSON 字段：用户明确选"加正式字段"。

### D9. 解码失败可观测性

**决策**：解码成功不打日志（避免高吞吐刷屏）；解码失败按 `warn` 级别打 `openai.images.size_decode_failed`，含 `slot_index`、`decoded_bytes`、`error`、`group_id`。`ImageSizeSource` 新增取值 `output_decoded`，与现有 `output/input/default/legacy` 区分以便审计。

**为什么不写 usage_log**：当前 `usage_logs` 不区分定价命中层级（design Open Questions 已留作独立 change），保持一致。

## Risks / Trade-offs

- **R1: 6×3 = 18 个 input 的前端编辑界面拥挤** → 用 6 行 × 3 列的紧凑矩阵网格 + 「填入官方默认表」批量按钮缓解；旧字段折叠到「兼容回退价」二级面板。
- **R2: 矩阵价格配错（负数 / 非数字）** → 后端 DTO 校验：每个数值必须 ≥ 0 且 ≤ 上限阈值（如 100 美元/张）；前端 input 实时校验并禁用保存。
- **R3: 反转开关开了但 fal 账号都 schedulable=false** → 自动退到 openai 账号，不报错（即"fal 优先 + openai 兜底"语义本意）；可观测性上保留日志记录"反转生效但实际选中 openai"。
- **R4: 同分组内 openai/fal 实际成本差与同价计费的张力** → 这是 D3 的明示 trade-off，留给运营在分组定价时自行平衡；不在系统层面拆开。
- **R5: 旧字段映射到 6 档时的有损归并** → 仅作回退路径，不可避免；前端在编辑矩阵时给出提示「保存矩阵后旧字段仅在矩阵未填某格时被读取」。
- **R6: 像素总数边界 786,432 与 1,048,576 之间的灰色区间** → 严格按"≤"比较，1024×768=786,432 命中 `1024x768`；1024×769=787,456 命中 `1024x1024`。spec scenario 给出明确边界用例。
- **R7: 解码 b64 内存峰值** → 仅头部 `image.DecodeConfig` 不解码像素数据；标准库自带 PNG/JPEG，需引入 `golang.org/x/image/webp` 满足 WebP；解码前限制 b64 长度上限（如 50 MB）防御异常 payload。
- **R8: 解码错误的 b64 引发 panic / OOM** → `recover` 兜底 + 限长，失败一律走默认 2K 档；不抛错给客户端（异步路径已经返回响应）。
- **R9: 上游真实返回 size 与解码值不一致** → 信任上游 size（解码仅在缺失/auto 时触发，不覆盖已知值），D8 触发条件已明示。

## Migration Plan

1. Schema 迁移：`groups` 表新增 `image_pricing_matrix`(JSON, nullable)、`image_prefer_fal`(bool, default false)、`image_decode_size_on_rsp`(bool, default false) 三列。无回填——存量分组三列均为 null/false，等价于现状行为。
2. 后端先实现 `NormalizeImageBillingTier`(6 档) 与 `NormalizeImageQuality`(auto→high) 并加单测覆盖边界；再接入 `BillingService.CalculateImageCost` 的查找路径；再接入 `SelectImageAccountMixed.preferPlatform`；最后接入 `DecodeOpenAIImageOutputSizes`（依赖前置 `ImageOutputBase64` 缓存接入到 forward 路径）。
3. 前端先把矩阵编辑器作为「高级定价」二级面板上线（默认折叠），旧字段编辑入口保持顶层可见；运营验证一轮后再调整默认折叠/展开策略。`image_decode_size_on_rsp` toggle 与 `image_prefer_fal` 同区域，platform=openai 时显示。
4. 灰度：先内部分组配置矩阵 + 反转开关 + decode 开关，比对单价与计费日志一致性；解码开关单独验证 size_decoded 在审计字段中的命中率；无误后开放给所有分组。
5. 回滚：清空分组的 `image_pricing_matrix` 即回退到旧 1K/2K/4K 单维计费；置 `image_prefer_fal=false` 即回退到现状调度顺序；置 `image_decode_size_on_rsp=false` 即关闭回包解码、回到默认 2K 兜底。三列保留可空可改、零数据迁移。

## Open Questions

- 「批量填入官方默认表」按钮的默认值表是写死在前端常量还是从后端 settings 拉取？倾向写死前端常量（静态报价、变更频率低、零网络往返）；如未来供应商频繁调价，可升级为后端 settings。
- 是否需要在 `usage_logs` 中记录"实际命中的定价层级"（matrix / fallback_legacy / litellm_default）作为运营审计字段？本次不做，可作为后续独立 change。
- URL 模式的回包分辨率自检（远程 Range 拉取 + 并发/超时控制）何时落地？本次明确不做；如果生产环境观察到大量 URL 模式上游不返 size 的样本，再开独立 change 引入。
