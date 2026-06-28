## 1. 数据层

- [x] 1.1 Ent schema：`groups` 表新增 `image_pricing_matrix`(JSON, optional) 与 `image_prefer_fal`(bool, default false) 字段，生成迁移
- [x] 1.2 `internal/domain/group.go`：领域模型补字段；`internal/repository/group_repository.go`：序列化/反序列化矩阵
- [x] 1.3 `internal/service/group_service.go`：DTO/校验——矩阵每格价格 ≥ 0 且 ≤ 上限阈值；`image_prefer_fal=true` 仅在 `platform=openai` 时允许
- [x] 1.4 `internal/handler/group_handler.go`：API 入参/出参补字段；admin API 层验证
- [x] 1.5 单测：repository 序列化、service 校验（含负数、非 openai 平台开 prefer_fal、超阈值）
- [x] 1.6 Ent schema：`groups` 表新增 `image_decode_size_on_rsp`(bool, default false)；新增迁移 `159_groups_image_decode_size_on_rsp.sql`（仅 ALTER TABLE ADD COLUMN IF NOT EXISTS）
- [x] 1.7 `internal/domain/group.go` 补 `ImageDecodeSizeOnRsp bool`；repository/DTO/handler/admin API 补字段；DTO 校验：仅 `platform=openai` 允许写 `true`
- [x] 1.8 单测：repository 序列化往返、service 校验（非 openai 平台拒绝、默认 false、与 `image_prefer_fal` 共存）

## 2. 计费引擎

- [x] 2.1 新增 `ClassifyImagePricingTier6(width, height int) string`：6 档 + 像素总数升序 + 向上取档 + 4K 封顶；返回档位 key
- [x] 2.2 新增 `NormalizeImageQuality(quality string) string`：`auto`/`""` → `high`；`low/medium/high` 透传；其他值视为 `high` 并日志告警
- [x] 2.3 修改 `BillingService.CalculateImageCost`：查找路径 = matrix 命中 → 按 size_tier 归并到旧 1K/2K/4K → LiteLLM 默认价
- [x] 2.4 单测：6 档边界、超 4K 封顶、quality 归一、矩阵命中、单维回退、LiteLLM 兜底
- [x] 2.5 单测：D3 不变量——同一分组同一 (size, quality) 不论 openai/fal 账号承载金额一致

## 3. 调度反转

- [x] 3.1 `SelectImageAccountMixed` 增加 `preferPlatform string` 参数；`"fal"` 时反转候选池排序
- [x] 3.2 `OpenAIGatewayHandler` 图片路径：从分组读取 `image_prefer_fal`，`true` 时传 `"fal"` 否则传 `""`
- [x] 3.3 单测：prefer_fal=true 且 fal 可用时选中 fal；fal 不可用时退 openai；prefer_fal=false 行为不变

## 4. 前端

- [x] 4.1 `frontend/src/views/admin/GroupsView.vue`（或对应表单组件）：图片定价区域改为 6×3 矩阵编辑器（行=size，列=quality）
- [x] 4.2 「填入官方默认表」按钮：写死前端常量，一键填充 18 格 OpenAI 公示价
- [x] 4.3 旧 `image_price_1k/2k/4k` 字段折叠到「兼容回退价」二级面板，保留可读可写
- [x] 4.4 `image_prefer_fal` toggle：仅在 `platform=openai` 分组下显示，含 tooltip 说明「fal 优先 + openai 兜底」
- [x] 4.5 i18n（zh/en）补齐：矩阵列标签、quality 名称、回退说明、prefer_fal 文案
- [x] 4.6 前端校验：每格数值 ≥ 0；保存前阻断非法输入
- [x] 4.7 `image_decode_size_on_rsp` toggle：仅 `platform=openai` 分组显示，与 `image_prefer_fal` 同区域；tooltip 说明「上游不返 size 或返 auto 时按 b64 内容自动识别真实分辨率」
- [x] 4.8 i18n（zh/en）补齐 `image_decode_size_on_rsp` 文案

## 5. 回包图片分辨率自检（base64）

- [x] 5.1 `internal/service/image_output_accounting.go`：`openAIImageOutputCounter` 扩展 `seenBase64 map[string]string` 缓存（仅当 `b64_json` 字段读到时缓存，不缓存 url）；新增 `Base64Payloads() []string`（按 `seenOrder` 同序输出，未知/url 时占位空串）
- [x] 5.2 `internal/service/openai_gateway_service.go`：`OpenAIForwardResult` 新增 `ImageOutputBase64 []string`；非流式与 SSE forward 路径在 counter 收尾时统一回填到 `result.ImageOutputBase64`
- [x] 5.3 新增 `internal/service/openai_image_payload_size.go`：`DecodeOpenAIImageOutputSizes(result *OpenAIForwardResult, group *Group)`；仅当 `group != nil && group.Platform == openai && group.ImageDecodeSizeOnRsp == true` 触发；遍历 `result.ImageOutputSizes` 与 `result.ImageOutputBase64`，对 size ∈ {"", "auto"} 且 b64 非空的 slot 用 `image.DecodeConfig` 解码，成功回填 `"{w}x{h}"`，失败留空；recover 兜底 + b64 长度上限（50 MB）防御
- [x] 5.4 引入图片格式驱动：`_ "image/png"`、`_ "image/jpeg"`、`_ "golang.org/x/image/webp"`（go.mod 已存在 v0.39.0）
- [x] 5.5 `image_billing_size.go::ApplyOpenAIImageBillingResolution` 改签名增加 `group *Group`；归档前调用 `DecodeOpenAIImageOutputSizes`；同步更新所有调用点（`openai_gateway_service.go::RecordUsage`），nil group 安全
- [x] 5.6 `image_billing_size.go`：`ImageSizeSourceOutputDecoded = "output_decoded"` 常量；解码生效时 `Source` 取此值
- [x] 5.7 解码失败按 `warn` 级别记 `openai.images.size_decode_failed`（含 `slot_index`、`bytes`、`error`、`group_id`）；成功不打日志
- [x] 5.8 单测：(a) 关闭开关不解码；(b) 开启开关 + size="" + 合法 PNG → 命中真实尺寸；(c) 开启开关 + size="auto" + 合法 JPEG → 覆盖；(d) 已有 size="1024x1024" + 开关开启 → 不覆盖；(e) 损坏 b64 → 失败留空走默认；(f) URL slot（b64 为空）→ 不解码不报错；(g) 非 openai 平台开关无效；(h) 解码后 size 进入 6 档归档与矩阵命中端到端

## 6. Spec 同步

- [x] 6.1 `openspec/specs/media-prepay-billing/spec.md`：精化二维计费需求（6 档 + auto→high + 分组持有 + 旧字段回退）
- [x] 6.2 `openspec/specs/fal-image-platform/spec.md`：在「双向 upstream 调度」需求下补充 prefer_fal 反转语义
- [x] 6.3 `openspec validate add-image-pricing-matrix-and-fal-prefer --strict` 全过（首轮）
- [x] 6.4 spec delta 增补「回包图片分辨率自检（base64）」需求；`openspec validate add-image-pricing-matrix-and-fal-prefer --strict` 全过（含新需求）

## 7. 验收

- [x] 7.1 backend `go test ./...` 全过（首轮，未含 §1.6-1.8 / §5）
- [x] 7.2 frontend `pnpm typecheck && pnpm lint` 全过（首轮，未含 §4.7-4.8）
- [x] 7.3 backend `go test ./...` 全过（含 §1.6-1.8 / §5 解码模块）【本任务新增的所有单测全部通过；`TestGatewayService_SelectImageAccountMixed` 的 5 个子用例在上游 baseline 即已 fail（与本 change 无关、已验证）】
- [x] 7.4 frontend `pnpm typecheck && pnpm lint` 全过（含 §4.7-4.8）
- [x] 7.5 手工冒烟：建一个 openai 分组配满矩阵 → 1024×1024 high 请求金额命中矩阵；删除矩阵某格 → 同请求金额回退到 image_price_1k；置 prefer_fal=true → 调度日志显示 fal 优先
- [x] 7.6 手工冒烟：开 `image_decode_size_on_rsp`，构造一个不返回 size 的代理上游 → 计费命中真实分辨率档；关 toggle → 同请求回退默认 2K 档
- [x] 7.7 archive 流程：`openspec archive add-image-pricing-matrix-and-fal-prefer`
