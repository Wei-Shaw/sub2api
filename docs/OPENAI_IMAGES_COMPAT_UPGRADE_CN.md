# OpenAI Images 图生图兼容与升级手册

本文记录本项目当前对 OpenAI Images 图生图请求的兼容实现，以及后续合并官方新版时必须保留和验证的点。目标是保证旧业务请求格式继续可用：

```json
{
  "model": "gpt-image-2",
  "prompt": "...",
  "response_format": "url",
  "size": "1088x1440",
  "image": ["https://example.com/reference.jpg"]
}
```

入口仍然是：

```text
POST /v1/images/generations
Content-Type: application/json
Authorization: Bearer <local-api-key>
```

注意：不要把真实 API key、上游 key、账号 token 写入文档或提交到 GitHub。测试时通过本地环境变量或后台已有配置读取。

## 当前实现

核心代码在 `backend/internal/service/openai_images.go`。

`ParseOpenAIImagesRequest` 负责解析 `/v1/images/generations` 和 `/v1/images/edits`。JSON 请求会识别这些图片输入字段：

- 单图或数组：`image`
- 兼容字段：`image_url`、`url`、`b64_json`、`base64`、`image_base64`、`input_image`
- 数组字段：`images`、`input_images`

这意味着旧业务可以继续用 `/v1/images/generations` 加顶层 `image: ["https://..."]` 来表达图生图，不需要强制改成 multipart，也不需要用户侧改到 `/v1/images/edits`。

当请求带图片输入，并且账号是自定义 OpenAI-compatible `base_url` 时，`ForwardImages` 会进入兼容聚合转发。当前实际顺序是：

1. `multipart_edits`：优先把请求转成 multipart `/v1/images/edits`，适配大多数兼容站的标准图生图接口。
2. `responses_bridge`：第一条路径失败后，尝试 Responses 图像生成桥接路径。
3. `json_edits`：最后尝试 JSON 形式的 `/v1/images/edits`。

带图片的 `/v1/images/generations` 请求**不能**回退成不带图片的 `/v1/images/generations` 文生图请求，否则上游可能成功返回一张与参考图无关的新图。无图片的 `/v1/images/generations` 请求继续使用原有文生图直通路径。

这个聚合链路由 `forwardOpenAIImagesAPIKeyCompatibleAggregate` 维护。它的作用是兼容不同上游对图生图的不同实现，不要在升级时删除其中任一路径，也不要恢复“先 generations、最后才 multipart edits”的旧顺序。

为降低多次兼容探测带来的延迟，聚合转发还有以下约束：

- 远程单图、多图、data URL、base64 和 multipart 上传都视为图片输入。
- 同一请求的远程图片只准备一次；后续兼容路径复用已经准备好的上传内容。
- 多张远程图片下载并发上限为 4，不能改成串行下载，也不能无限并发。
- 某个账号最近 5 分钟成功过某条兼容路径时，下一次优先尝试该路径；该路径再次失败后立即清除偏好并恢复完整回退链。
- 带图片请求的兼容回退只允许走上述三条图生图路径，不能调用 `/v1/images/generations` 作为忽略图片后的成功回退。

## 本次关键修复

`finalizeOpenAIImagesCompatibleAggregateError` 必须保留 `UpstreamFailoverError.RetryableOnSameAccount`。

以前这里会把 `RetryableOnSameAccount` 强行改成 `false`，导致池模式账号在聚合链路失败后不能继续同账号重试。对 `ttt` 这类 pool mode 上游来说，前几条兼容路径可能返回 `502/503`，但后续同账号重试可能成功出图。强行清掉这个标记，会让请求提前失败，看起来就像图生图坏了。

当前正确行为：

- 聚合链路全部失败时，错误仍然带着 `RetryableOnSameAccount=true`。
- handler 层看到该标记后，会触发 `openai.images.pool_mode_same_account_retry`。
- 同账号重试次数来自账号配置 `pool_mode_retry_count`。

对应测试在 `backend/internal/service/openai_images_test.go`：

- `TestOpenAIGatewayServiceParseOpenAIImagesRequest_JSONGenerationTopLevelImageArrayCompat`
- `TestOpenAIGatewayServiceForwardImages_APIKeyCustomGenerationAggregateServerErrorTriesCompatibleRoutesBeforeRetry`

## 2026-07-13 图生图路由与延迟优化

本次在 `v0.1.147` 的基础上补充了请求路由和图片准备优化。核心代码仍集中在：

```text
backend/internal/service/openai_images.go
backend/internal/service/openai_images_responses.go
backend/internal/service/openai_gateway_service.go
```

必须保留的行为：

- `/v1/images/generations` 请求体只要出现 `image`、`images`、`image_url`、`input_images`、base64 或 multipart 文件，就按图生图处理。
- 单图和多图都保留，不能只取数组第一项。
- 图片准备阶段完成后，multipart、Responses 和 JSON edits 三条路径共享同一份图片内容。
- 成功路径会按账号和上游 `base_url` 记忆 5 分钟，减少每次请求重复探测。
- 所有路径失败时仍保留 `RetryableOnSameAccount`，不能为了结束回退链而清掉池模式账号的同账号重试标记。

本次本地真实验证使用 `http://127.0.0.1:8080/v1/images/generations` 的 1K 图生图请求，结果为：

- HTTP `200`，耗时约 75 秒。
- 日志确认只调用一次上游 `/v1/images/edits`。
- 返回 usage 的 `image_tokens=1032`，证明参考图确实传入并被上游使用。
- 输出图片尺寸为 `1086x1448`，文件约 `2.25 MB`。
- 测试输出保存为 `C:\Users\Administrator\AppData\Local\Temp\sub2api-i2i-latency-optimized.png`。

升级后不能只看 HTTP `200`。必须同时确认 `image_tokens > 0`，并确认带图片请求的日志没有通过 `/v1/images/generations` 作为忽略图片的回退路径。

## 2026-07-08 v0.1.146 升级记录

本次已合并官方 `v0.1.146`，并保留本项目的图生图兼容逻辑。对应本地提交：

```text
0c09b1d4 Merge upstream v0.1.146 with image compatibility
```

本地 Docker 已替换到默认端口 8080：

```text
image: sub2api-custom:0.1.146-local-20260708-002402-0c09b1d4
alias: deploy-sub2api:latest
container: sub2api-dev
port: 0.0.0.0:8080->8080/tcp
health: http://127.0.0.1:8080/health -> {"status":"ok"}
```

打包文件已保存到：

```text
F:\BC\调查\sub2api-repo\backups\sub2api-image-20260708-002402-v0.1.146-0c09b1d4-upstream-502-unverified.tar
```

这个包的名字带 `upstream-502-unverified` 是故意的：它表示镜像构建、容器健康检查和代码测试通过，但当时使用 `ttt` 渠道做真实图生图没有通过。后续用户使用 `cpa` 渠道实测图生图已通过，说明本地图生图兼容逻辑和 8080 镜像本身可用；`ttt` 的失败应按上游通路问题处理。

本次真实验证结论：

- `POST http://127.0.0.1:8080/v1/images/generations` 使用 `ttt` 所在分组返回 `502`。
- 同一请求去掉 `image` 后，普通文生图也返回 `502`。
- 绕过本地 sub2api，直接请求 `https://sub.kedaya.xyz/v1/images/generations` 也返回 `502 error code: 502`。
- 参考图 `https://cdn.xingyuexiezuo.com/case/1_20260621223156.jpg` 在容器内可以正常下载。
- 用户后续切换到 `cpa` 渠道后，按同类 `/v1/images/generations` 图生图请求已成功出图。

所以这次失败不是顶层 `image` 解析、8080 端口、模型映射或本地 Docker 镜像没有替换导致的；当前证据指向 `ttt` 的上游 `https://sub.kedaya.xyz/v1` 当时对图片接口返回 502。若 `cpa` 或其他渠道可成功图生图，不要回滚本地图生图兼容代码。

下次如果再次遇到图生图失败，先按下面顺序排查，不要一上来改模型映射：

1. 先确认 `http://127.0.0.1:8080/health` 是新容器。
2. 再用同一个 key 测 `/v1/images/generations`，保留响应状态码和响应体。
3. 再用同一个上游 key 直连账号的 `base_url` 测 `/v1/images/generations`。
4. 如果直连上游也 502，先处理上游或代理，不要改本地图生图兼容代码。
5. 只有当直连上游能出图、但本地 8080 不能出图时，才继续检查 `openai_images.go` 的兼容链路。

注意：`ttt` 当前没有绑定账号代理；HTTP upstream 在账号 `proxyURL` 为空时按直连处理，不会自动使用容器里的 `HTTP_PROXY/HTTPS_PROXY` 环境变量。若 `sub.kedaya.xyz` 被 Cloudflare 或网络策略拦截，优先给该账号绑定可用代理，或确认本机代理端口真的在监听。

## 2026-07-09 v0.1.147 升级记录

本次合并官方 `v0.1.147`。官方更新包含批量图像生成、Grok 4.5、`response_format` 兼容映射、`/v1/messages` failover 强化、Go 1.26.5 和前端 i18n 拆分。

本次升级保留并验证了本项目已有图生图兼容逻辑：

- `/v1/images/generations` 顶层 `image: ["https://..."]` 仍会解析为 `InputImageURLs`。
- 自定义 OpenAI-compatible API key 账号仍保留 `multipart_edits -> responses_bridge -> json_edits` 聚合兼容链路；带图片请求不会回退为纯文生图。
- `finalizeOpenAIImagesCompatibleAggregateError` 仍保留 `RetryableOnSameAccount`。
- OpenAI usage 解析补回 `input_tokens_details.image_tokens` 和 `prompt_tokens_details.image_tokens`，避免图生图参考图消耗漏计。
- `previous_response_id` sticky 账号命中时仍检查 API key 分组隔离，跨组命中会删除绑定并回落常规调度。

本次升级还补回了官方拆分文件时容易漏掉的本地兼容点：

- `ChatCompletionsRequest.response_format` 统一保留为 `json.RawMessage`；图像桥接只在它是 JSON 字符串时当作图片 `response_format` 使用，避免和官方 `response_format` 对象映射冲突。
- 账号 `BatchID`、`Schedulable` 在 create/update/bulk update/list filters 中继续可用。
- `SettingService.GetAPIBaseURL` 继续可用于构建公开图片 URL。
- Antigravity SSE usage 继续支持 `cached_tokens` 作为 `cache_read_input_tokens` 兜底。

已执行验证：

```powershell
Set-Location F:\BC\调查\sub2api-repo\backend
go test ./internal/service -run "OpenAIImages|ImageGenerationIntent|ImageGeneration|ChatCompletions|ModelMapping|UpstreamModels|ParseSSEUsage|ExtractOpenAIUsage|ListAccounts|BulkUpdate"
go test ./internal/handler -run "Gateway|OpenAI|RequestBody|Endpoint"
go test ./...

Set-Location F:\BC\调查\sub2api-repo\frontend
& 'F:\Program Files\nodejs\npm.cmd' run build
```

结果：

- 后端完整 `go test ./...` 通过。
- 前端 `npm run build` 通过，仅有 Vite chunk/Browserslist 过期等既有警告。

## 升级官方版本时怎么做

每次合并官方新版后，先不要急着打包镜像，按下面顺序检查：

1. 对比 `backend/internal/service/openai_images.go`，确认这些函数还存在且语义没被覆盖：
   - `ParseOpenAIImagesRequest`
   - `appendOpenAIImagesJSONInputImages`
   - `shouldForwardOpenAIImagesAPIKeyAsCompatibleAggregate`
   - `forwardOpenAIImagesAPIKeyCompatibleAggregate`
   - `finalizeOpenAIImagesCompatibleAggregateError`
2. 确认 `/v1/images/generations` 的顶层 `image` 数组仍会解析进 `InputImageURLs`。
3. 确认带图片输入的自定义 OpenAI-compatible 账号仍走兼容聚合链路。
4. 确认 `finalizeOpenAIImagesCompatibleAggregateError` 不会清掉 `RetryableOnSameAccount`。
5. 如果官方改了 `response_format`，确认 `ChatCompletionsRequest.response_format` 仍可兼容对象格式和图像字符串格式。
6. 如果官方拆分 service 文件，确认 `GetAPIBaseURL`、账号 `BatchID/Schedulable`、Antigravity `cached_tokens` fallback、OpenAI `previous_response_id` 分组隔离没有丢。
7. 跑单测，再构建本地 Docker 镜像。
8. 先替换本地 `sub2api-dev`，确认 `http://127.0.0.1:8080/health` 正常。
9. 用 1K 图生图请求真实验证，再打包镜像上传服务器。

推荐单测命令：

```powershell
Set-Location F:\BC\调查\sub2api-repo\backend
go test ./internal/service -run "TestOpenAIGatewayServiceParseOpenAIImagesRequest_JSONGenerationTopLevelImageArrayCompat|TestOpenAIGatewayServiceForwardImages_APIKeyCustomGenerationAggregateServerErrorTriesCompatibleRoutesBeforeRetry"
```

## 本地真实验证

真实验证要用本地 8080，不要换端口制造额外变量：

```powershell
$env:SUB2API_TEST_KEY = "<local-api-key>"

$body = @'
{
  "model": "gpt-image-2",
  "prompt": "[CRITICAL IMAGE REQUIREMENT] You MUST generate a PORTRAIT image with a 3:4 aspect ratio (width:height = 3:4). The image MUST be taller than it is wide. This is a book cover — vertical/portrait orientation is mandatory. Do NOT generate landscape or square images.\n\n参考信息：。按照这是封面提示词风格生成封面图片, 标题，副标题，作者名，分别为： 12311\n12312313\n123123131\n aspectRatio => 3:4",
  "response_format": "url",
  "size": "1088x1440",
  "image": [
    "https://cdn.xingyuexiezuo.com/case/1_20260621223156.jpg"
  ]
}
'@

$headers = @{
  Authorization = "Bearer $env:SUB2API_TEST_KEY"
  "Content-Type" = "application/json"
}

Invoke-WebRequest `
  -Uri "http://127.0.0.1:8080/v1/images/generations" `
  -Method Post `
  -Headers $headers `
  -Body $body `
  -TimeoutSec 400
```

成功标准：

- HTTP 状态码是 `200`。
- 返回 JSON 里有 `data[0].url`。
- `usage.input_tokens_details.image_tokens` 大于 `0`，说明参考图真的被使用。
- 日志里的 `account_id` 是目标图生图账号。

如果上游慢，客户端超时不要设太短。当前 `ttt` 渠道曾出现过 180 秒客户端先超时、服务端 211 秒后成功返回的情况。测试图生图时建议 `TimeoutSec >= 400`。

## 常见故障判断

如果返回 `No available compatible accounts`：

- 先查本地 key 所在 group。
- 确认目标账号在该 group 中。
- 确认目标账号 `status=active` 且 `schedulable=true`。
- 确认目标账号支持 `gpt-image-2` 或有正确的模型映射。

如果返回 `502/503`：

- 看日志是否出现 `openai.images.pool_mode_same_account_retry`。
- 看日志是否出现 `openai.images.upstream_failover_switching`。
- 如果只剩一个账号可调度，最终失败通常来自上游账号、上游池或上游权限，而不是本地解析。

如果能出图但不像参考图：

- 先看返回 usage 的 `image_tokens`。如果为 `0`，说明图片没有被上游使用，要优先查兼容链路。
- 如果 `image_tokens > 0`，说明参考图已进入上游，问题通常是提示词约束、上游模型能力或随机性。

## 打包前检查

打包上传云服务器前至少确认：

```powershell
docker ps --format "table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}"
Invoke-WebRequest http://127.0.0.1:8080/health
```

镜像包继续放到：

```text
F:\BC\调查\sub2api-repo\backups
```

不要把 builder 镜像或中间层打进上传包。历史可接受包体通常是几十 MB 级别，不应变成几百 MB 或 GB。

## 时段计费升级注意事项

当前时段计费是在现有 `token`、`per_request`、`image` 计费模式上增加可选覆盖，不新增或改写原有 `billing_mode`。数据库迁移为：

```text
backend/migrations/173_add_channel_time_pricing.sql
```

该迁移为以下两张表增加 `time_pricing JSONB`：

- `channel_model_pricing`
- `channel_account_stats_model_pricing`

配置结构示例：

```json
{
  "enabled": true,
  "timezone": "Asia/Shanghai",
  "periods": [
    {
      "name": "夜间",
      "start_time": "22:00",
      "end_time": "02:00",
      "weekdays": [1, 2, 3, 4, 5],
      "input_price": 0.000001,
      "output_price": 0.000002,
      "per_request_price": 0.01
    }
  ]
}
```

升级时必须保留以下规则：

- 使用 IANA 时区；未填写时默认 `Asia/Shanghai`。
- 管理端时区控件必须使用下拉选择，第一项为 `Asia/Shanghai`，不能退回容易输错的自由文本输入。
- 时区选项显示名称必须通过 `admin.channels.form.timezoneOptions` 获取，跟随系统当前语言；不要在组件中硬编码中文或英文。
- 前端显示翻译名称，保存和提交给后端的值仍必须是标准 IANA 时区，例如 `Asia/Shanghai`，不能保存“北京时间”等翻译文本。
- 已保存但不在预置列表中的合法 IANA 时区必须继续动态显示，不能在编辑渠道时被丢弃或强制替换。
- 普通时段为 `[start_time, end_time)`，结束时刻不计入该时段。
- 跨午夜时段的凌晨部分仍归属于时段开始日期的星期。
- `weekdays` 为空表示每天，星期编号为 Go 标准：`0=周日` 到 `6=周六`。
- 时段只覆盖显式填写的价格字段；未填写字段继承命中的 token 区间价格或默认价格。
- token 区间先按 token 数选择，再应用时段覆盖；不能让区间选择把时段覆盖丢掉。
- 未启用或未配置时段时，旧计费结果必须保持不变。
- 客户计费和账号统计必须使用同一个匹配时刻；账号统计使用 `UsageLog.CreatedAt`。

合并官方新版后，除了图生图兼容检查外，还要验证：

1. token 模式：默认时段、跨午夜、区间定价和未填写字段继承。
2. per-request/image 模式：按次价格覆盖以及未命中时段的旧价格。
3. 账号统计自定义规则：同样的 token/per-request/image 时段价格。
4. 管理端保存、重新打开渠道后 `time_pricing` 仍完整回填。
5. 执行迁移后旧渠道的 `time_pricing` 为 `NULL` 时，计费行为不变。
6. 中文界面显示“北京时间”，英文界面显示 “China Standard Time”，两种语言提交值都仍为 `Asia/Shanghai`。
7. 新启用时段计费或旧配置时区为空时，界面和后端匹配逻辑都使用 `Asia/Shanghai`。

不要为了合并官方定价结构而删除图生图兼容链路；`time_pricing` 与 `/v1/images/generations` 的图片输入解析和兼容聚合转发是两套独立逻辑，升级时都要分别测试。

### 时区下拉升级检查

官方升级如果改动渠道定价表单或 i18n 拆分，重点检查以下文件：

```text
frontend/src/components/admin/channel/TimePricingEditor.vue
frontend/src/i18n/locales/zh/admin/channels.ts
frontend/src/i18n/locales/en/admin/channels.ts
```

必须确认：

1. `defaultConfig()` 的时区仍是 `Asia/Shanghai`。
2. 空时区的 `Select` 显示值仍回退到 `Asia/Shanghai`。
3. `timezoneOptions` 的标签仍通过当前语言的 i18n key 生成。
4. 未收录的旧 IANA 时区仍会追加到下拉列表并保留。
5. 中英文 locale 中的 `timezoneOptions` key 完全一致，避免切换语言后显示原始 key。

推荐验证命令：

```powershell
Set-Location F:\BC\调查\sub2api-repo\frontend
npm run test:run -- src/components/admin/channel src/i18n/__tests__/localesNoKeyCollision.spec.ts
npm run build

Set-Location F:\BC\调查\sub2api-repo\backend
go test ./internal/service ./internal/handler ./internal/repository
```

### 2026-07-13 时区国际化镜像记录

本次在 `v0.1.147` 时段计费版本上补充了时区下拉国际化，提交为：

```text
b7fbb1b5 feat: localize time pricing timezone options
```

验证结果：

- `sub2api-dev` 使用镜像 ID `sha256:daa902aff5357aa3e5f00f87b57a31b4f4976790013dc8a17a01d08a8b930eb0`。
- 容器状态为 `healthy`，继续使用 `0.0.0.0:8080->8080/tcp`。
- `GET http://127.0.0.1:8080/health` 返回 `200 {"status":"ok"}`。
- 生产前端资源中同时包含中文和英文时区标签。
- 前端相关 13 项测试通过；后端 service、handler、repository 测试通过。

可上传或回退的镜像包：

```text
F:\BC\调查\sub2api-repo\backups\sub2api-image-20260713-0259-v0.1.147-time-pricing-i18n-b7fbb1b5.tar
```

镜像标签与校验值：

```text
sub2api-custom:0.1.147-time-pricing-i18n-20260713-0259-b7fbb1b5
SHA256: e9f8f8044b835560756764983d49f7c67e9fbafa634ae75d77e821e1ef75924e
```

恢复命令：

```powershell
docker load -i F:\BC\调查\sub2api-repo\backups\sub2api-image-20260713-0259-v0.1.147-time-pricing-i18n-b7fbb1b5.tar
docker tag sub2api-custom:0.1.147-time-pricing-i18n-20260713-0259-b7fbb1b5 deploy-sub2api:latest
Set-Location F:\BC\调查\sub2api-repo\deploy
docker compose -f docker-compose.dev.yml up -d --no-build sub2api
```

### 2026-07-13 图生图兼容优化镜像记录

本次镜像基于已经完成 1K 图生图实测的本地 `deploy-sub2api:latest` 导出，没有重新构建容器，也没有打包 PostgreSQL、Redis 或数据卷。

镜像包：

```text
F:\BC\调查\sub2api-repo\backups\sub2api-image-20260713-1439-v0.1.147-image-routing-latency-4c5efc22.tar
```

校验文件：

```text
F:\BC\调查\sub2api-repo\backups\sub2api-image-20260713-1439-v0.1.147-image-routing-latency-4c5efc22.tar.sha256.txt
```

SHA256：

```text
7550b0590cd27dd3b89bc2fcd6c8418e81fbcb99c3ad6c8471d7508664627239
```

镜像和容器信息：

```text
image=deploy-sub2api:latest
image_id=sha256:46a205980abc7982014177f8df87882e566a42926513436557da16a1e96b1433
image_size_bytes=55217838
archive_size_bytes=55241728
container=sub2api-dev
port=0.0.0.0:8080->8080/tcp
health=http://127.0.0.1:8080/health -> 200 {"status":"ok"}
commit=4c5efc22
```

上传服务器时使用：

```powershell
docker load -i F:\BC\调查\sub2api-repo\backups\sub2api-image-20260713-1439-v0.1.147-image-routing-latency-4c5efc22.tar
docker tag deploy-sub2api:latest weishaw/sub2api:latest
Set-Location F:\BC\调查\sub2api-repo\deploy
docker compose -f docker-compose.yml up -d --no-build sub2api
```

回滚时重新加载上一份已验证的镜像包，并按上一份包的 metadata 恢复对应标签；不要执行 `docker system prune`，也不要删除数据库和 Redis 数据卷。
