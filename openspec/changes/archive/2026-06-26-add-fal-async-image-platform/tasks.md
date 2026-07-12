## 1. 平台常量与模型映射

- [x] 1.1 在 `internal/domain/constants.go` 新增 `PlatformFal = "fal"` 并纳入平台枚举/校验
- [x] 1.2 新增内置默认模型映射（OpenAI 模型名 → fal slug：`gpt-image-2`、`gpt-image-2/edit`）
- [x] 1.3 支持在账号/渠道上配置模型映射覆盖，并实现「配置优先、内置兜底」的解析逻辑

## 2. fal 账号接入

- [x] 2.1 允许 `platform=fal`、`type=apikey` 账号的创建与校验（存 `FAL_KEY`）
- [x] 2.2 将 fal 账号纳入既有调度，使 fal 平台请求可选中可用 fal 账号
- [x] 2.3 实现 fal 上游请求构造（baseURL `queue.fal.run`/`fal.run` + `Authorization: Key {FAL_KEY}`）

## 3. OpenAI⇄fal 双向 transformer（pkg/fal）

- [x] 3.1 新建 `pkg/fal/` 包，定义 fal 请求/响应数据结构
- [x] 3.2 实现 OpenAI→fal 请求转换（size→image_size、quality→quality、n→num_images、edits 图片→image_urls[]/mask_url）
- [x] 3.3 实现 fal→OpenAI 响应转换（images[] → data[] url/b64）
- [x] 3.4 实现 fal→OpenAI 请求转换与 OpenAI→fal 响应转换（反向）
- [x] 3.5 实现 size 与 quality 的取值映射表（含 OpenAI standard/hd ↔ fal quality）

## 4. 数据库 Schema 与迁移

- [x] 4.1 新增 ent schema `async_media_tasks`（字段见 design：状态、held/final cost、image_urls、cos_url、upstream_request_id、fail_deadline_at 等）
- [x] 4.2 为 `usage_logs` schema 增列 `task_id`、`image_urls`、`cos_url`、`billing_status`(charged|refunded)（可空，兼容存量）
- [x] 4.3 扩展定价表 `Intervals` 增加 `quality` 维度并提供默认值
- [x] 4.4 生成 ent 代码并编写/校验数据库迁移脚本
- [x] 4.5 新增 `async_media_tasks` 的 repository（创建、按 request_id 查询、状态更新、扫描未终结任务）

## 5. 异步执行内核

- [x] 5.1 实现 fal 队列客户端：submit / status / result / cancel（以提交响应的 status_url/response_url 为准）
- [x] 5.2 实现任务提交流程：提交 fal → 落库 pending + 预扣费
- [x] 5.3 实现轮询/取结果流程：状态流转 running→succeeded、取 images 结果
- [x] 5.4 实现失败判定（status 明确失败 或 到达 fail_deadline_at）与退费流程（同事务）
- [x] 5.5 终态追加写 usage_log（charged/refunded，含 task_id、image_urls、cos_url）

## 6. 计费

- [x] 6.1 实现按 (image_size 档位 × quality) 二维计费的预估与结算
- [x] 6.2 实现预扣费（held_cost）与退费的余额账本/事务逻辑，保证幂等
- [x] 6.3 兼容存量单维（仅 size）定价：缺省 quality 走默认档位

## 7. COS 转存

- [x] 7.1 新增全局 COS 配置（settings-service 范式，DB 存储，后台可配：开关/Endpoint/Region/SecretId/SecretKey/Bucket/前缀）
- [x] 7.2 复用/封装对象存储抽象（基于 `aws-sdk-go-v2/s3`）实现 COS 上传与 url 生成
- [x] 7.3 实现出图后转存流程（下载 fal 临时图 → 上传 COS → 写 cos_url），重试最多 3 次
- [x] 7.4 实现回退逻辑：重试耗尽则 cos_url 留空、回退 fal url、任务仍成功不退费
- [x] 7.5 generations/edits、伪同步与原生门面全部接入转存；返回地址按 cos_url 优先

## 8. 对外门面与路由

- [x] 8.1 在 `routes/gateway.go` 放行 fal 平台并接入 OpenAI 伪同步 `/v1/images/generations` 与 `/v1/images/edits`
- [x] 8.2 实现伪同步主流程（提交→阻塞轮询→转存→返回），含阻塞超时返回错误且不退费/不终结
- [x] 8.3 新增 `/fal/...` 原生路由组 + `ForcePlatform(fal)` 中间件
- [x] 8.4 实现原生门面 submit/status/result/cancel（submit 立即返回 request_id）
- [x] 8.5 实现原生门面 streaming 透传

## 9. 后台对账 reconciler

- [x] 9.1 实现周期扫描 pending/running 任务的后台 worker
- [x] 9.2 实现补完成（取结果+转存+写终态成功+usage_log）与补退费（明确失败/超期 expired）
- [x] 9.3 实现幂等保护（按状态去重，避免重复退费/重复 usage_log）
- [x] 9.4 新增可配置项：扫描间隔、任务失败时间，并接入配置体系

## 10. 测试与收尾

- [x] 10.1 transformer 双向转换单元测试（四种 upstream 组合）
- [x] 10.2 异步流程与退费/超时/对账的集成测试（含幂等）
- [x] 10.3 计费二维档位与存量兼容测试
- [x] 10.4 COS 转存成功/重试/回退测试
- [x] 10.5 路由放行与两门面端到端测试

## 11. 前端接入

- [x] 11.1 平台注册中心新增 fal：`types/index.ts` 的 `GroupPlatform`/`AccountPlatform` 联合类型加 `'fal'`；`utils/platformColors.ts` 的 `Platform` 类型、全部色板 Record（BADGE/BADGE_LIGHT/BORDER/ACCENT_BAR/TEXT/ICON/BUTTON/DISCOUNT/GRADIENT/GRADIENT_TEXT/GRADIENT_SUBTEXT）、`isPlatform()`、`platformLabel()` 均补 fal
- [x] 11.2 `components/common/PlatformIcon.vue` 新增 fal 专属 SVG 图标分支
- [x] 11.3 账号创建/编辑（`CreateAccountModal.vue`/`EditAccountModal.vue`）顶部平台选择新增 fal 按钮，选中 fal 时强制 `type=apikey`，复用通用 apikey 输入存 `FAL_KEY`，补平台切换时的字段重置逻辑
- [x] 11.4 分组（`GroupsView.vue`）`platformOptions`/`platformFilterOptions` 与列表/卡片徽章配色分支新增 fal，并梳理 fal 参与/跳过哪些条件配置块（跳过 antigravity OAuth/MCP/模型系列等专属块）
- [x] 11.5 二维定价配置 UI：`components/admin/channel/{types.ts,IntervalRow.vue,PricingEntryCard.vue}` 支持按 (image_size 档位 × quality) 配价，兼容存量单维（仅 size）数据（含补齐 admin handler DTO 的 quality 字段往返）
- [x] 11.6 `SettingsView.vue` 新增「图片转存」标签页 + COS 全局配置面板（开关/Endpoint/Region/SecretId/SecretKey 掩码/Bucket/前缀/公开域名/Path-Style），接入 `api/admin/cosImage.ts`（`GET/PUT /admin/cos-image/config`）
- [x] 11.7 reconciler 运行时配置（扫描间隔、失败兜底时间）：后端补 DB 设置（`async_media_runtime_config`）+ `AsyncMediaConfigService`（启动加载 + 热更新）+ admin `GET/PUT /admin/async-media/config`，reconciler 支持运行时重置 ticker；前端 `api/admin/asyncMedia.ts` + `AsyncMediaConfigSection.vue` 并入「图片转存」标签页
- [x] 11.8 i18n `zh.ts`/`en.ts` 补 fal 平台文案（`admin.accounts.fal.{baseUrlHint,apiKeyHint}` + 两处 `platforms` 枚举块加 `fal`）；清扫硬编码平台分支补 fal：`PlatformTypeBadge.vue`（label/配色 pink 系，原 fal 误显示为 Gemini）、`ChannelsView.vue` `platformOrder`（修复 fal 通道二维定价无法加载/编辑）、`AccountTableFilters.vue` 平台筛选下拉。平台配额体系（`PlatformQuotaPlatform` 4-key 后端契约）与 antigravity OAuth/upstream/模型限制等专属块按设计跳过 fal
- [x] 11.9 前端 lint / type-check 通过（`pnpm typecheck` + `pnpm lint:check` 均 0 报错；既有的 `AccountUsageCell`/`UsageTable` 7 项失败经 git 基线验证为分支历史遗留，与本次改动无关）

## 12. fal 账号挂载 OpenAI 分组（跨平台图片调度）

> 目标：fal 账号既能挂到 fal 分组，也能挂到 openai 分组；挂到 openai 分组后，openai 伪同步门面 `/v1/images/generations`、`/v1/images/edits` 的请求可真正调度到 fal 账号并用 fal 协议出图。

- [x] 12.1 前端 `GroupSelector.vue`：fal 账号放开分组过滤，允许同时选择 `platform === 'fal'` 与 `platform === 'openai'` 的分组（在原 antigravity `mixedScheduling` 分支旁新增 fal 分支）；账号创建/编辑弹窗已传 `:platform`，无需额外开关
- [x] 12.2 后端选号：新增 `GatewayService.SelectFalAccountInGroup`，在任意分组内**强制按 `PlatformFal`** 选号（复用既有 `selectAccountForModelWithPlatform` 路径，与 fal 分组完全一致），避免侵入 openai 调度器的 OAuth/transport/compact/recheck 等专属门槛
- [x] 12.3 后端图片分流：`OpenAIGatewayHandler` 在 openai 分组**没有可用 openai 账号**时回退到 fal 伪同步门面（新增 `tryFalImageFallback` + setter 注入 `FalGatewayHandler`）；`FalGatewayHandler` 抽出 `ServeOpenAIImages`/`selectFalAccount`/`writeOpenAIImagesResponse`，`buildSubmitInput`/`runPseudoSync` 改为接收预选账号
- [x] 12.4 计费去重：fal 分支复用 `AsyncMediaService` 的预扣/结算（held/final cost + usage_log），openai handler 命中 fal 兜底后**直接 return**、不再走 `RecordUsage`，无重复计费
- [x] 12.5 测试与 lint：`go build ./...`、`go vet`（handler/service/server）通过，handler 包 Fal/Image 相关测试通过；前端 `pnpm typecheck` 通过、`GroupSelector.vue` 无 lint 报错
