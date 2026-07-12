## Why

网关目前的 OpenAI Images 链路是纯同步透传，只支持 `anthropic/openai/gemini/antigravity` 四个平台，无法接入 fal.ai 这类「异步队列出图」的供应商。fal 的 `gpt-image-2` 等模型走 queue 协议（提交拿 `request_id`、轮询状态、再取结果），长耗时、需要状态管理与失败兜底，现有只追加的 `usage_logs` 与同步转发层都撑不起来。

业务上需要：把 fal 作为一等平台接进来，对外既能伪装成同步的 OpenAI Images 渠道（客户端无感），又能暴露原生 fal 异步接口；并支持 fal/openai 账号互相挂载做 upstream；同时引入先扣费后退款、按分辨率×质量二维计费，以及把临时的 fal 图片转存到腾讯云 COS 以获得长期可用地址。

## What Changes

- 新增 `fal` 平台常量，fal 账号以 `apikey` 类型存 `FAL_KEY` 接入，纳入调度。
- 新增 **OpenAI⇄fal 双向协议适配层**，支撑四种 upstream 组合：openai 门面→fal 上游、fal 门面→openai 上游，以及各自直通。
- **对外双门面**：
  - OpenAI 伪同步门面：`POST /v1/images/generations`、`POST /v1/images/edits` 内部走 fal 异步队列并阻塞返回，对客户端表现为同步。
  - fal 原生异步门面：`/fal/...` 暴露 submit / status / result / queue / streaming 全套。
- **异步执行内核**：新建可变任务表 `async_media_tasks` 落 `request_id` 与生命周期状态（pending→running→succeeded/failed/refunded/expired）。
- **先扣费后退款**：提交时预扣费；成功写 `usage_logs`；失败/真超时退费。失败判定 = status 明确失败 **或** 到达可配置的失败截止时间。
- **后台对账 reconciler**：扫描未终结任务补完成或补退费，扫描间隔与任务失败时间可配置。
- **二维计费**：定价表在现有分辨率分档基础上新增 `quality` 维度，按 (image_size × quality) 定价。
- **COS 转存**：出图成功后下载 fal 临时图并转存腾讯云 COS（复用现有 S3 兼容对象存储抽象），全局一份后台可配置；转存最多重试 3 次，仍失败则回退用 fal 原始 url、任务仍算成功不退费。
- **前端管理端**：fal 作为一等平台接入前端 —— 平台注册中心（类型/色板/专属图标）、账号创建/编辑（apikey 存 `FAL_KEY`）、分组管理；定价配置 UI 升级为 (image_size × quality) 二维；后台设置新增 COS 全局配置面板与 reconciler 配置项。
- **数据库变更**：`async_media_tasks` 新表；`usage_logs` 新增 `task_id`、`image_urls`、`cos_url`、`billing_status`(charged|refunded) 字段；`async_media_tasks` 含 `cos_url`、`image_urls`。
- **前端接入**：将 fal 作为一等平台接入管理端 —— 平台注册中心（类型/色板/图标）、账号创建/编辑入口（apikey 存 `FAL_KEY`）、分组管理入口；定价配置 UI 支持 (image_size × quality) 二维配价；`SettingsView` 新增 COS 全局配置与 reconciler 配置项。

## Capabilities

### New Capabilities
- `fal-image-platform`: fal 平台与账号接入、OpenAI⇄fal 双向协议适配层、双向 upstream 调度。
- `fal-native-api`: 对外 fal 原生异步门面（submit/status/result/queue/streaming）与 OpenAI 伪同步图片门面。
- `async-media-task-lifecycle`: 异步媒体任务的提交、落库、状态流转、轮询与后台对账 reconciler。
- `media-prepay-billing`: 先扣费后退款流程与按分辨率×质量的二维计费。
- `media-cos-archival`: 生成图片转存腾讯云 COS（全局可配置、重试与回退策略）。

### Modified Capabilities
<!-- 现有 specs（captcha/console-navigation/oidc-provider/pricing-plaza/recharge-bonus）的需求不发生变更，留空。 -->

## Impact

- **新增代码**：`pkg/fal/`（双向 transformer）、fal 平台 handler/service、异步任务 service 与 reconciler、COS 转存 service。
- **路由**：`internal/server/routes/gateway.go` 放行 fal 平台并新增 `/fal/...` 原生路由组；`/v1/images/*` 接入异步内核。
- **数据库/Schema**：新增 ent schema `async_media_tasks`；扩展 `usage_logs` schema 与迁移；扩展定价表 `Intervals` 增加 quality 维度。
- **配置**：新增全局 COS 配置（后台可配，复用 settings-service 范式）、reconciler 扫描间隔与任务失败时间配置。
- **依赖**：复用既有 `aws-sdk-go-v2/s3`（COS 走 S3 协议），无需新增第三方依赖。
- **领域常量**：`internal/domain/constants.go` 新增 `PlatformFal` 及默认模型映射。
