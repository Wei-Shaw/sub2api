## Why

OpenAI 出图上游（含兼容代理）常出现「请求了 2K/4K，实际回包分辨率更低」的情况。现有 `image_decode_size_on_rsp` 开关已能在回包 base64 上解码出真实分辨率并据此**计费**，但**交付给客户端的仍是低分辨率原图**——用户付了高档位的钱（或本应得到高分辨率），拿到的却是缩水图。

业务诉求：当分组开启该能力、请求档位 ≥ 2K、且解码出的真实档位低于目标档位时，在把图返回客户端（及转存 COS）之前，调用 fal 的 `fal-ai/seedvr/upscale/image` 把图放大到目标档位，使**客户端与 COS 都拿到放大后的图**。放大模型的 endpoint/token/超时作为系统配置，开关挂在分组上。

## What Changes

- **新增分组开关** `image_upscale_on_rsp`（仅 `platform=openai` 生效）：开启后对满足条件的回包图执行 upscale。依赖 `image_decode_size_on_rsp`（必须先能解码真实分辨率才能比对），开启 upscale 时 SHALL 校验 decode 已开。
- **新增系统配置**（settings 表，admin 管理）：`fal_upscale_endpoint`（默认 `fal-ai/seedvr/upscale/image`）、`fal_upscale_token`、`fal_upscale_timeout_seconds`（默认 300）。token 不回显明文。
- **同步 upscale 插入响应写出之前**：在 4 条 OpenAI 出图响应路径（`images/generations` 流式/非流式 + Responses API 流式/非流式）中，写客户端响应前缓冲整张图、按需放大、再写。**流式同样放大**（缓冲整张图后再吐，等于退化为非流式）。
- **放大判定**：仅 `b64_json` 模式；目标档位 ∈ {2K, 4K}；真实档位 < 目标档位时放大。放大倍数由档位计算（1K→2K=2，1K→4K=4，2K→4K=2），走 SeedVR `upscale_mode=factor`，不追求精确像素，够到目标档位即可。
- **失败兜底**：upscale 调用失败 / 超时 / 结果下载失败时，回退使用原图，且**按原图真实档位计费**；成功时按目标档位计费。计费随「交付字节」由现有解码路径自然得出，无需额外特判。
- **SeedVR upscale 客户端**：单图入单图出，走 fal 队列（Submit→Status→Result）轮询，结果为 fal 托管 URL，需下载字节后回填。N 张图调用 N 次。
- **COS 与客户端一致**：COS 转存的是放大后的同一份字节（COS 上传在 upscale 之后），不再出现「客户端低分辨率、COS 高分辨率」的分叉。
- **成本**：每次放大是一次 fal 付费调用，由平台吸收（不计入用户费用）。
- **前端**：分组编辑页新增 `image_upscale_on_rsp` 开关（platform=openai 显示，依赖 decode 开关）；系统设置页新增 fal upscale endpoint/token/timeout 配置项。

## Capabilities

### New Capabilities

- `openai-image-upscale`: OpenAI 出图回包分辨率不足时的同步 upscale 交付（分组开关 + 系统配置 + SeedVR 放大 + 原图兜底 + 计费自洽 + COS 一致）。

## Impact

- **数据库**：`groups` 新增 `image_upscale_on_rsp`（bool, default false）；新增迁移 `163_groups_image_upscale_on_rsp.sql`（ALTER ADD COLUMN IF NOT EXISTS）。
- **后端代码**：
  - `internal/service/openai_images.go`、`internal/service/openai_images_responses.go`：4 条响应路径在写客户端响应前插入同步 upscale；流式路径改为先缓冲再写。
  - `internal/service/image_billing_size.go`：复用档位归一/分类（`ClassifyImagePricingTier6` / `NormalizeImageBillingTier` / `ResolveImageBillingSize`），新增「目标档位 vs 真实档位 → 放大倍数」判定。
  - `internal/service/openai_image_cos_upload.go`：上传时使用已放大字节（upscale 在前，COS 在后）。
  - `internal/service/openai_gateway_service.go`：`OpenAIForwardResult` 视需要加字段（如 `imageUpscaled` 审计标记）；注入 upscale 依赖。
  - `internal/pkg/fal/`：新增 SeedVR upscale 的 request/response 类型（`image_url` data URI、`upscale_mode=factor`、`upscale_factor`、`output_format`；输出 `image{url,content_type}`）+ 结果下载。
  - 新增 `internal/service/openai_image_upscale.go`（或同类）：放大编排（判定→调用→下载→替换→兜底）。
  - 分组字段贯通：`internal/domain/group.go`、`ent/schema/group.go`、repository、`admin_service.go`（校验依赖 decode）、handler、`api_key_auth_cache*.go`（快照字段）。
  - 系统配置：`internal/service/domain_constants.go`（SettingKey）、`setting_service.go`、`internal/handler/admin/setting_handler.go` + 路由。
- **前端**：`GroupsView`（开关）、`SettingsView`（endpoint/token/timeout）、i18n(zh/en)。
- **配置/系统设置**：3 个 setting key。
- **Breaking**：无。`image_upscale_on_rsp=false`（默认）时行为等价现状；未配置 fal upscale endpoint/token 时即使开关开也按原图兜底（不报错、不放大）。

## Non-goals

- URL 模式回包不放大（仅 `b64_json`）。
- 不做精确目标像素裁剪/缩放（够到目标档位即可）。
- 不把 upscale 成本计入用户费用（平台吸收）。
- 不处理「真实档位 ≥ 目标档位」的缩小场景（只放大）。
- 不引入 `upscale_mode=resolution` 路径（仅 factor）。
