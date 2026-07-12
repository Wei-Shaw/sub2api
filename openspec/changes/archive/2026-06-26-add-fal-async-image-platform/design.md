## Context

网关现有四个平台（anthropic/openai/gemini/antigravity），其中 `antigravity` 是最近新增的平台，完整实现了「平台常量 + `ForcePlatform` 中间件 + 专属 `/antigravity/v1` 路由 + `pkg/antigravity/` 双向 transformer + 模型映射」一整套范式，是本次 fal 接入的最佳参照模板。

现状关键事实（探索阶段已确认）：

- **平台与账号**：API Key 挂在 Group 上，`Group.Platform` 决定走哪套对外逻辑；`Account.Platform + Account.Type` 决定调度哪个上游账号。fal 账号本质是 `apikey` 类型，凭证为 `FAL_KEY`，上游认证 header 为 `Authorization: Key {FAL_KEY}`。
- **渠道(Channel)** 是一等实体（`channel_service.go`），挂在 Group 上，定义模型映射与计费；`BillingMode` 已支持 `token / per_request / image`，image 模式已支持按尺寸分档（`Intervals[] + TierLabel`，如 1K/2K/4K）。
- **usage_logs 是只追加表**（schema 明确注释"不支持更新和删除"），无法承载有状态的任务流转与退费。
- **对象存储**：已有 `service.BackupObjectStore` 接口与 `repository.S3BackupStore` 实现（基于 `aws-sdk-go-v2/s3`，支持自定义 Endpoint + ForcePathStyle），腾讯云 COS 兼容 S3 协议可直接复用。

fal API 事实（来自 fal.ai 官方文档）：

- 模型：文生图 `openai/gpt-image-2`，图生图/编辑 `openai/gpt-image-2/edit`。
- REST（queue 协议）：
  - 提交 `POST https://queue.fal.run/{model}` → `{request_id, status, status_url, response_url, cancel_url}`
  - 状态 `GET https://queue.fal.run/{model}/requests/{request_id}/status` → `{status: IN_QUEUE|IN_PROGRESS|COMPLETED|...}`
  - 结果 `GET https://queue.fal.run/{model}/requests/{request_id}` → `{images:[{url,width,height,content_type,file_name,file_size}]}`
  - 取消 `PUT .../requests/{request_id}/cancel`；同步 `POST https://fal.run/{model}`
- 请求字段：`prompt`(必填)、`image_urls[]`(edit 必填)、`mask_url`(可选)、`image_size`(枚举 `square_hd/square/portrait_4_3/portrait_16_9/landscape_4_3/landscape_16_9/auto` 或 `{width,height}`)、`quality`(`auto/low/medium/high`)、`num_images`、`output_format`(`jpeg/png/webp`)、`sync_mode`。
- 响应仅 `images[]`（无 seed/timings）。

## Goals / Non-Goals

**Goals:**

- 将 fal 作为一等平台接入，账号以 apikey 类型纳入调度。
- 实现 OpenAI⇄fal 双向 transformer，支撑四种 upstream 组合。
- 对外提供两个门面：OpenAI 伪同步图片门面、fal 原生异步门面（submit/status/result/queue/streaming）。
- 异步执行内核：可变任务表落 `request_id` 与生命周期；先扣费后退款；后台 reconciler 兜底。
- 按 (image_size × quality) 二维计费。
- 出图转存腾讯云 COS（全局可配、重试 3 次、失败回退 fal url）。

**Non-Goals:**

- 不实现 fal 视频/音频等非图片模型（仅 `gpt-image-2` 系列图片）。
- 不改造 `usage_logs` 的只追加语义（通过新表 + 终态追加写来兼容）。
- 不做渠道/账号级别的 COS 多租户隔离（仅全局一份配置）。
- 不实现 fal BYOK（自带 OpenAI key）模式。
- 不做精细到 token 级的 fal 计费（按 size×quality 档位计费）。

## Decisions

### D1. fal 作为独立平台 `PlatformFal`，复刻 antigravity 范式

在 `internal/domain/constants.go` 新增 `PlatformFal = "fal"` 及默认模型映射（OpenAI 模型名 → fal slug）。复用 `antigravity` 的整套接入方式：平台常量、`ForcePlatform(fal)` 中间件、专属路由组、`pkg/fal/` transformer、constants 模型映射。

**Alternatives**：把 fal 当作 openai 平台下的一种 upstream（不新增平台）。否决理由：fal 请求/响应与异步语义和 OpenAI 差异大，混进 openai 链路会让调度、计费、模型映射高度耦合，难以维护。独立平台更干净，且双向 upstream 的诉求用「门面协议由路由决定 + 上游协议由账号 platform 决定」的抽象更自然地覆盖。

### D2. OpenAI⇄fal 双向 transformer + 适配矩阵

实现一对双向转换（`pkg/fal/`）：OpenAI Images 请求/响应 ⇄ fal 请求/响应。四种组合由「门面协议 × 上游账号 platform」拼装：

```
                上游 Account 平台
                openai            fal
门面  openai   直通(现状)         openai→fal→openai
门面  fal      fal→openai→fal     直通
```

门面协议由命中的路由决定，上游协议由调度选中的账号 `platform` 决定；两者不同则启用对应方向的转换。这样四组合无需各写一遍。

字段映射要点：OpenAI `size`(如 `1024x1024`) ⇄ fal `image_size`(枚举或 `{width,height}`)；OpenAI `quality`(`standard/hd` 或 gpt-image-1 的 `low/medium/high/auto`) ⇄ fal `quality`；OpenAI `n` ⇄ fal `num_images`；OpenAI edits 的图片输入(multipart) ⇄ fal `image_urls[]` + `mask_url`（需先把上传的二进制转为可访问 URL 或 data URI）。

### D3. 异步执行内核 + 伪同步门面

新建可变表 `async_media_tasks` 承载生命周期：

```
async_media_tasks
  id, internal_request_id, upstream_request_id (fal request_id),
  account_id, api_key_id, user_id, group_id, channel_id,
  requested_model, upstream_model,
  image_size, quality, num_images,
  status: pending|running|succeeded|failed|refunded|expired,
  held_cost, final_cost,
  image_urls (成功的 fal 图片 url, JSON), cos_url (转存后地址, JSON/文本),
  fail_deadline_at, created_at, updated_at, finished_at
```

伪同步门面（`/v1/images/*`）主流程：提交 fal queue → 落 task(pending) + 预扣费 → 轮询 status 直到 COMPLETED → 取 result → 转存 COS → 写终态 + 追加 usage_log → 返回客户端（对外表现同步）。

fal 原生门面（`/fal/...`）：直接暴露 fal 异步协议，submit 立即返回 `request_id`，客户端自行 status/result 轮询；同样落 task、预扣费、并由 reconciler 兜底；result/转存在任务完成时进行。

### D4. 先扣费后退款 + usage_logs 兼容只追加

- 预扣费在提交时进行（写入余额账本/transaction），`async_media_tasks.held_cost` 记录预扣金额。
- **成功**：写终态 `succeeded`，追加一条 `usage_logs`（`billing_status=charged`、`task_id`、`image_urls`、`cos_url`、`final_cost`）。
- **失败/真超时**：退费（余额回退），终态置 `failed→refunded`/`expired`，追加一条 `usage_logs`（`billing_status=refunded`、`task_id`、无图片）。

`usage_logs` 因此新增字段：`task_id`、`image_urls`、`cos_url`、`billing_status`(charged|refunded)。每个任务在终态写一条 usage_log，保持只追加语义。

**真超时判定**：仅当 ① fal status 接口明确返回失败，或 ② 到达 `fail_deadline_at`（可配置失败时间），才视为失败并退费。伪同步阻塞超时（客户端等待超时）只返回错误、**不退费、不终结任务**，交给 reconciler 继续兜底。

### D5. 后台对账 reconciler

周期扫描处于 `pending/running` 的任务：用 `upstream_request_id` 查 fal status。
- COMPLETED → 取结果、转存 COS、写终态成功 + usage_log；
- 明确失败 → 退费 + 终态 `failed→refunded`；
- 超过 `fail_deadline_at` → 退费 + 终态 `expired`。

扫描间隔与任务失败时间均可配置（全局配置）。

### D6. 二维计费：定价表增加 quality 维度

在现有 image 计费的 `Intervals`(按 size tier) 基础上增加 `quality` 字段，定价键为 `(size_tier, quality)`。

**Alternatives**：把 quality 拼进 `TierLabel`(如 `"1K|high"`)。否决理由：label 组合爆炸、UI 拼接易错；显式 quality 字段查询/展示更清晰，长期更规范。

### D7. COS 转存：复用 S3 抽象

抽出通用对象存储用法（直接复用 `BackupObjectStore` 工厂或新增等价的 `MediaStorage`），新增**全局一份** COS 配置（走 settings-service 范式，DB 存储、后台可改）：`Endpoint`(cos.{region}.myqcloud.com)、`Region`、`SecretId/SecretKey`(映射到 S3 AccessKeyID/SecretAccessKey)、`Bucket`、可选 `PathPrefix`/自定义域名、`enabled` 开关。

转存时机：任务出图成功后下载 fal 临时图 → `PutObject` 到 COS → 得到 `cos_url`。**generations/edits、OpenAI 伪同步门面与 fal 原生门面全部转存**。失败重试最多 3 次，仍失败则 `cos_url` 留空、回退用 fal 原 url，任务仍算成功、不退费。对客户端优先返回 `cos_url`（开启且成功时），否则返回 fal url。

## Risks / Trade-offs

- **伪同步阻塞占用连接/超时** → 设置合理阻塞上限；超时返回错误但保留任务，由 reconciler 补完成，客户端可后续通过 fal 原生门面或任务查询拿结果。
- **fal `/edit` 的 status/result 路径约定** → 文档仅给 JS client，REST 中带子路径模型的 status/result 路径里 app id 可能回退到基础 `openai/gpt-image-2`。实现时以 submit 返回的 `status_url`/`response_url` 为准（fal 提交响应直接给出这两个绝对 URL），避免手拼路径。
- **预扣费与终态写 usage_log 之间的一致性** → 退费与终态更新放同一事务；reconciler 幂等（按 `upstream_request_id`/task 状态去重），避免重复退费或重复写 usage_log。
- **COS 转存增加出图链路延迟** → 转存在取到结果后进行；伪同步可接受额外耗时，原生门面转存可异步完成（先返回 fal url，转存完成后补 `cos_url`）。
- **fal 图片 url 时效性** → 正是引入 COS 转存的原因；未开启 COS 时回退 fal url 存在过期风险，需在文档中说明。
- **二维计费 schema 变更影响既有定价数据** → quality 字段提供默认值（如 `default`/`any`）以兼容存量按 size 单维定价的渠道。

## Migration Plan

1. Schema 迁移：新增 `async_media_tasks` 表；为 `usage_logs` 增列 `task_id/image_urls/cos_url/billing_status`（可空，兼容存量）。
2. 定价表 `Intervals` 增加 `quality` 字段并提供默认值，存量记录回填默认 quality。
3. 灰度：先上线 fal 平台 + 原生门面（真异步，风险低），再开启 OpenAI 伪同步门面。
4. COS 默认关闭，配置就绪后由后台开启。
5. 回滚：路由层可关闭 fal 平台放行与 `/fal/*` 路由；新增列可空不影响旧逻辑；COS 开关置关即回退 fal url。

## Open Questions

- fal `gpt-image-2` 在 fal 侧的实际计费单位与档位边界（用于设定默认内置定价档），需结合实际账单核定。
- OpenAI `quality` 取值与 fal `quality` 的精确映射（OpenAI 客户端可能传 `standard/hd`，需定一张映射表）。
