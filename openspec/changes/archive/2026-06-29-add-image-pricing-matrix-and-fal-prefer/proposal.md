## Why

`media-prepay-billing` spec 已声明「按 (image_size × quality) 二维计费」，但落地时只兜住了 1K/2K/4K 三档 size 与单一 quality 维度的存量回退路径，定价 UI 也只暴露 `image_price_1k/2k/4k` 三个字段。OpenAI 与 fal 上游公开的 `gpt-image-2` 报价表是 6 档 size × 3 档 quality 共 18 格的二维矩阵，现状既无法配置完整定价、也未规定 size 不在档位边界时如何归档、`quality=auto` 或缺省时如何归一，spec 与实现存在分叉。

业务上同时出现一个调度诉求：`platform=openai` 的分组里运营希望把 fal 上游账号当作主路径、openai 账号兜底（即"反转"现有"openai 优先 + fal 兜底"的优先级），用于压测 / 成本切换 / fal 容量倾销等场景。该开关只影响候选池排序、不改变计费语义。

另一个观测到的痛点：部分 OpenAI 兼容的代理上游对 `images/generations` 回包不返回 `size` 字段，或一律返回 `size=auto`。当前归档逻辑会把这类响应一刀切到默认 2K 档，导致客户端实际拿到 4K 图却按 2K 计费、或拿到 1K 图却按 2K 多收，造成与公示价目表的偏差。需要一个分组级开关让运营按需启用回包 base64 内容解码以获得真实分辨率，自然命中正确档位。

## What Changes

- 计费语义精化：在 `media-prepay-billing` 中明确 6 档 size 边界、向上取档与 4K 封顶规则；明确 `quality` 归一（`auto` 或缺省视为 `high`）；明确"计费表按分组持有"——同一分组的同一请求计费金额与最终承载账号的 `platform` 无关。
- 配置面扩展：分组新增 `image_pricing_matrix`（JSON，6 档 size × 3 档 quality 共 18 个价格格）字段，覆盖现有 `image_price_1k/2k/4k`。
- 兼容回退：当 `image_pricing_matrix` 未配置某 (size_tier, quality) 时，按 size_tier 归并回退到旧 `image_price_1k/2k/4k`；旧字段也未配置时回退到 LiteLLM 默认价。旧字段保留可编辑、不删除。
- 调度反转开关：分组（仅 `platform=openai`）新增 `image_prefer_fal` 布尔开关；为 `true` 时图片调度候选池排序反转为「fal 优先 + openai 兜底」，为 `false` 时维持现状「openai 优先 + fal 兜底」。无可用 fal 账号时自动回退到 openai 账号，不报错。
- 回包图片分辨率自检（解决上游不返 `size` 或返 `auto` 时被默认 2K 兜底导致计费失真）：分组（仅 `platform=openai`）新增 `image_decode_size_on_rsp` 布尔开关；为 `true` 且回包某张图的 `size` 字段缺失或为 `auto` 时，系统 SHALL 在异步记账阶段对该张图的 `b64_json` 内容解码出真实 `width × height`，并以解码值作为该张图的 `size` 进入 6 档归档计费。仅 base64 模式启用解码；URL 模式不在本次变更范围内（解析失败时沿用现状默认档兜底，不报错）。
- 前端管理端：`GroupsView` 在图片定价区域改造为 6×3 矩阵编辑器（行=size、列=quality），支持「按 OpenAI 官方默认表一键填充」批量写入；旧 1K/2K/4K 字段保留为「兼容回退价」编辑入口；在 `platform=openai` 的分组下显示 `image_prefer_fal` 与 `image_decode_size_on_rsp` 两个切换开关。

## Capabilities

### Modified Capabilities

- `media-prepay-billing`: 精化「按分辨率×质量二维计费」需求（6 档 + 向上取档 + 4K 封顶 + quality 归一 + 分组持有 + 旧字段回退）；新增「回包图片分辨率自检」需求（分组级开关，base64 解码兜底归档）。
- `fal-image-platform`: 在「双向 upstream 调度」需求下新增分组级反转优先级开关（`image_prefer_fal`）。

### New Capabilities

<!-- 本次不新增 capability。`groups` 域配置变更并入 media-prepay-billing 与 fal-image-platform 现有能力。 -->

## Impact

- **后端代码**：
  - `internal/service/billing_service.go`：扩展 `NormalizeImageBillingTier` 为 6 档 + 向上取档 + 4K 封顶；新增 `NormalizeImageQuality`(`auto/""→high`)；`CalculateImageCost` 查找路径增加矩阵命中→旧字段回退→LiteLLM 兜底三级。
  - `internal/service/gateway_service.go`：`SelectImageAccountMixed` 新增 `preferPlatform` 参数，`true` 时反转 openai/fal 候选池排序。
  - `internal/service/image_output_accounting.go`：counter 顺手缓存 `(key→b64_json)`，供 `Sizes()` 之外新增 `Base64Payloads()` 取用。
  - `internal/service/image_billing_size.go` 或新增 `internal/service/openai_image_payload_size.go`：新增 `DecodeOpenAIImageOutputSizes(result, group)`，仅当 `Group.ImageDecodeSizeOnRsp=true` 时对缺失/auto size 的 slot 进行 base64 解码（PNG/JPEG/WebP via `image.DecodeConfig`），成功则回填 `result.ImageOutputSizes[i]="WxH"`，失败留空走默认档；`ApplyOpenAIImageBillingResolution` 在归档之前调用。
  - `internal/service/openai_gateway_service.go`：`RecordUsage` 在调用 `ApplyOpenAIImageBillingResolution` 前传入 group 上下文；并行 forward 路径同步缓存 b64 到 `result` 的私有 slot 字段。
  - `internal/handler/openai_gateway_handler.go`：图片路径读取分组 `ImagePreferFal` 字段并传入调度。
  - `internal/domain/group.go` + ent schema + repository + DTO + handler：新增 `ImagePricingMatrix`(JSON)、`ImagePreferFal`(bool)、`ImageDecodeSizeOnRsp`(bool) 三列，含 GORM/Ent 迁移、序列化与校验（价格非负、`ImagePreferFal`/`ImageDecodeSizeOnRsp` 仅 `platform=openai` 时允许 `true`）。
- **前端代码**：
  - `frontend/src/views/admin/GroupsView.vue`（或对应分组编辑表单组件）：图片定价区域升级为 6×3 矩阵编辑器、「填入官方默认表」按钮、`image_prefer_fal` 与 `image_decode_size_on_rsp` 两个 toggle（platform=openai 时显示）。
  - i18n（zh/en）补齐文案。
- **数据库**：新增三列 `groups.image_pricing_matrix`(JSON, nullable)、`groups.image_prefer_fal`(bool, default false)、`groups.image_decode_size_on_rsp`(bool, default false)；不删除既有 `image_price_1k/2k/4k`。新增迁移 `159_groups_image_decode_size_on_rsp.sql`。
- **Spec 文件**：`openspec/specs/media-prepay-billing/spec.md`、`openspec/specs/fal-image-platform/spec.md`。
- **Breaking**：无。`image_pricing_matrix=null` + `image_prefer_fal=false` + `image_decode_size_on_rsp=false` 等价于现状行为；旧字段保留可读可写。
