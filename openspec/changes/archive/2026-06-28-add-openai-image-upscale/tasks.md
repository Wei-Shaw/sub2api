# Tasks — add-openai-image-upscale

## 0. SeedVR 契约实测（先做）

- [ ] 0.1 （契约取自用户提供的 SeedVR 输入/输出样例，实测待真实 token）用系统配置的 endpoint/token 对 `fal-ai/seedvr/upscale/image` 打一发真实请求，确认：`image_url` 是否接受 base64 data URI 及体积上限；`upscale_mode=factor` + `upscale_factor` 取值；输出是否恒为托管 URL（有无 sync_mode 可内联）；`output_format` 支持值
- [ ] 0.2 （待实测）据实测结果锁定 req/resp 契约；若 data URI 体积超限，改为「先上传图取 URL 再放大」的备选路径并记录到 design

## 1. 分组开关

- [x] 1.1 `ent/schema/group.go` 新增 `image_upscale_on_rsp`(bool, default false)；迁移 `163_groups_image_upscale_on_rsp.sql`（ALTER ADD COLUMN IF NOT EXISTS）；ent codegen
- [x] 1.2 `internal/domain/group.go` + repository 序列化补字段
- [x] 1.3 `internal/service/admin_service.go` DTO/校验：置 true 时要求 `image_decode_size_on_rsp=true` 且 `platform=openai`，否则拒绝
- [x] 1.4 `internal/service/api_key_auth_cache*.go`：快照 + 反序列化补 `ImageUpscaleOnRsp`
- [x] 1.5 `internal/handler/admin/*` group handler 出入参补字段
- [x] 1.6 单测：依赖校验（缺 decode 拒绝、非 openai 拒绝）、默认 false、快照往返

## 2. 系统配置

- [x] 2.1 `internal/service/domain_constants.go`：`SettingKeyFalUpscaleEndpoint` / `SettingKeyFalUpscaleToken` / `SettingKeyFalUpscaleTimeoutSeconds`
- [x] 2.2 `internal/service/setting_service.go`：读写 + 默认值（endpoint=fal-ai/seedvr/upscale/image，timeout=300）；token 状态仅掩码
- [x] 2.3 `internal/handler/admin/setting_handler.go` + 路由：获取（掩码）/ 设置
- [x] 2.4 单测：默认值、token 不回显

## 3. SeedVR upscale 客户端

- [x] 3.1 `internal/pkg/fal/`：新增 upscale req/resp 类型（`image_url`/`upscale_mode`/`upscale_factor`/`output_format` → `image{url,content_type}`）
- [x] 3.2 放大编排 `internal/service/openai_image_upscale.go`：用系统配置构造独立 fal client；`UpscaleImage(b64, factor, timeout) ([]byte, error)` = Submit→Status 轮询→Result→下载 url 字节
- [x] 3.3 倍数与档位：`factorFor(realTier, targetTier)`（1K→2K=2 / 1K→4K=4 / 2K→4K=2）；复用 `image_billing_size.go` 档位归一/分类
- [x] 3.4 单测：factor 计算、未配置降级、超时取消、结果下载失败 → 返回错误（由调用方兜底）

## 4. 响应路径同步 upscale（4 处）

- [x] 4.1 `internal/service/openai_images.go` 非流式：`c.Data` 前对 b64 图按需放大 + 改写 body；失败兜底原图
- [x] 4.2 `internal/service/openai_images.go` 流式：缓冲整段 → 放大 → 一次性吐出
- [x] 4.3 `internal/service/openai_images_responses.go` 非流式 + 流式：同上两处
- [ ] 4.4 多图：当前为**串行**放大（每图独立，任一失败仅该图兜底）；并发优化待办
- [x] 4.5 放大后的字节回填 `ImageOutputBase64`/sizes（供 COS 与计费解码复用）；`imageUpscaled` 审计标记未加（可选）
- [x] 4.6 `internal/service/openai_image_cos_upload.go`：确认上传的是放大后字节（upscale 在前、COS 在后）
- [x] 4.7 计费自洽验证：成功→目标档位、失败→原图档位（由现有解码路径得出，无特判）

## 5. 前端

- [x] 5.1 `GroupsView`：`image_upscale_on_rsp` 开关（platform=openai 显示），前端校验依赖 decode 开关；tooltip 说明放大语义与流式退化
- [x] 5.2 `SettingsView`：fal upscale endpoint / token / timeout 配置项（token 掩码输入）
- [x] 5.3 i18n（zh/en）补齐

## 6. 文档与验证

- [x] 6.1 行为/延迟/流式退化/平台吸收成本已记入 design.md（未单独建用户文档）
- [ ] 6.2 （待真实环境运行，需配置 fal token）端到端验证：开关开 + 配置就绪 → 请求 4K 但上游回 1K → 客户端拿到 4K 图、COS 一致、按 4K 计费；upscale 故意失败 → 原图 + 按原图计费
- [x] 6.3 `openspec validate add-openai-image-upscale --strict` 通过
