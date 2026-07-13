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

当请求带图片输入，并且账号是自定义 OpenAI-compatible `base_url` 时，`ForwardImages` 会进入兼容聚合转发。默认探测顺序是：

1. `generations_json`：保留原始 JSON 图片字段调用 `/v1/images/generations`，但必须校验 usage 中的图片输入 token。
2. `responses_bridge`：将图片输入桥接到 `/v1/responses` 的图片编辑动作。
3. `json_edits`：尝试 JSON 形式的 `/v1/images/edits`。
4. `multipart_edits`：最后转换为 multipart `/v1/images/edits`。

如果账号能力标记已经确认支持 Responses，则必须跳过普通探测顺序，直接把 `responses_bridge` 放在第一位。这个确认能力的优先级高于进程内的旧路由缓存，不能让早先缓存的 `generations_json` 覆盖已确认的 Responses 能力。

`generations_json` 只有在返回 usage 的图片输入 token 大于 `0` 时，才能被视为图生图成功。如果 HTTP `200` 但 `image_tokens=0`，说明上游忽略了参考图，必须继续尝试 `responses_bridge` 等真正使用图片的路径，不能把这种结果直接返回给调用方。无图片的 `/v1/images/generations` 请求继续使用原有文生图直通路径。

这个聚合链路由 `forwardOpenAIImagesAPIKeyCompatibleAggregate` 维护。它的作用是兼容不同上游对图生图的不同实现。升级时不能删除其中任一路径，也不能取消图片 token 校验或 Responses 能力优先逻辑。

每条协议探测必须使用 `backend/internal/service/openai_images_response_buffer.go` 中的临时响应 Writer。探测失败时，`400/404/415` 等错误只能保留在该次探测缓冲区，不能提前写入真实客户端响应；选中成功路径或确定终止后才一次性提交。否则前一条路径的错误 JSON 会污染后一条路径的成功 JSON，表现为 Apifox 一直等待、无法解析或看不到图片。

为降低多次兼容探测带来的延迟，聚合转发还有以下约束：

- 远程单图、多图、data URL、base64 和 multipart 上传都视为图片输入。
- 同一请求的远程图片只准备一次；后续兼容路径复用已经准备好的上传内容。
- 多张远程图片下载并发上限为 4，不能改成串行下载，也不能无限并发。
- 某个账号最近 5 分钟成功过某条兼容路径时，下一次优先尝试该路径；该路径再次失败后立即清除偏好并恢复完整回退链。
- 已确认支持 Responses 的账号始终优先 `responses_bridge`，即使旧缓存记录了其他路径。
- 池模式上游返回可同账号重试的服务器错误时，不再在一次聚合调用中继续串行等待所有慢兼容路径，而是保留错误属性并交给 handler 执行同账号重试或账号切换。
- Cloudflare `522` 不做同账号完整重试，也不继续串行探测其他慢路径，直接交给外层切换账号。
- 带图片请求不能接受 `image_tokens=0` 的文生图结果，也不能因为某条路径 HTTP `200` 就跳过图片输入校验。
- 只有明确属于协议或端点不兼容的错误才继续下一条兼容路径：`404/405/415/501`，或错误内容明确包含 `unsupported endpoint`、`unknown parameter`、`request format`、`multipart` 等标记。
- 普通 `502/503/504`、连接失败和上游超时不继续串行探测，直接交给 handler 做同账号重试或账号切换。
- 认证、权限、额度、限流、审核、安全和内容策略错误必须立即终止，不能换协议规避。

## 2026-07-14 通用协议探测响应隔离

本次修复针对所有自定义 OpenAI-compatible 图片上游，不绑定 `cpa`、`ttt` 或任何账号名称。

核心改动：

- 新增 `backend/internal/service/openai_images_response_buffer.go`。
- `forwardOpenAIImagesAPIKeyCompatibleAggregate` 为每条兼容路径创建独立响应缓冲区。
- 路径成功时只提交该路径的完整响应；路径不兼容时丢弃其响应并继续。
- 流式响应一旦 `Flush` 或 `Hijack`，立即提交并进入直通模式，避免破坏流式输出。
- 普通上游故障不再触发四条协议的串行慢探测。
- 所有协议都失败时，只返回最终终止错误的一份完整 JSON，不拼接前面路径的错误体。

必须保留的回归测试：

- `TestOpenAIGatewayServiceForwardImages_APIKeyCustomGenerationEndpointErrorDoesNotPolluteFallbackResponse`
- `TestOpenAIGatewayServiceForwardImages_APIKeyCustomGenerationAllProtocolErrorsReturnOnlyTerminalResponse`
- `TestOpenAIGatewayServiceForwardImages_APIKeyCustomGenerationOrdinaryServerErrorDoesNotProbeOtherProtocols`
- `TestShouldContinueOpenAIImagesCompatibleAggregateClassifiesProtocolAndTerminalErrors`

公网部署还必须检查 Nginx。图像生成可能超过默认的 60 或 90 秒，以下配置应追加到 `/v1/images/generations` 和 `/v1/images/edits` 对应的 `location`，不能用一份简化配置覆盖站点现有证书、域名或其他代理规则：

```nginx
proxy_read_timeout 1800s;
proxy_send_timeout 1800s;
send_timeout 1800s;
proxy_buffering off;
```

如果客户端约 90 秒后收到连接断开且没有 HTTP 状态码或 JSON，而容器仍在继续生成，先检查云服务器 Nginx/CDN 超时，不要修改模型映射或图生图请求格式。

如果 Nginx 已经配置 1800 秒超时，继续做同一请求的流式/非流式对照：

- `stream:true` 能持续收到 `:\n\n` keepalive 并成功出图，而原始非流式请求没有响应头、最终连接被关闭，说明问题是非流式 JSON 等待期间没有下行数据。
- 这种情况必须保留 `startOpenAIImagesResponseHeaderHeartbeat` 和 `readOpenAIImagesNonStreamingResponseBody`。前者覆盖等待上游响应头的阶段，后者覆盖读取上游响应体的阶段；两者都会按图片流 keepalive 间隔发送合法的 JSON 前导空白，并在完成后继续返回标准 JSON。
- 心跳必须通过 `openAIImagesBufferedResponseWriter.Unwrap` 穿透兼容探测缓冲层；不能提交或拼接失败探测的错误正文。
- `proxy_buffering off` 仍需保留，保证心跳及时发送给客户端。

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

## 2026-07-13 Responses 能力缓存与 Apifox 长等待修复

本次进一步修复了升级后偶发出现的图生图长等待和 Apifox 最终 `502`：

- 账号已经确认支持 `/v1/responses` 时，图生图请求直接优先 `responses_bridge`。
- 路由缓存过期后会重新读取账号的 Responses 能力标记。
- 旧的 `generations_json` 缓存不能覆盖已确认的 Responses 能力。
- 上游返回 Cloudflare `522` 时，不再对同一账号完整重试三次，也不再继续等待其他慢兼容路径。
- HTTP `200` 但 `usage.input_tokens_details.image_tokens=0` 时，不再把忽略参考图的结果当成图生图成功。

使用用户原始 Apifox 请求格式在本地 `8080` 做了两次真实验证：

```text
POST http://127.0.0.1:8080/v1/images/generations
model=gpt-image-2
size=1088x1440
image=["https://cdn.xingyuexiezuo.com/case/1_20260621223156.jpg"]
```

验证结果：

- 第一次：HTTP `200`，约 `214.8s`，账号 `46`，`responses_bridge`。
- 第二次：HTTP `200`，约 `113.6s`，账号 `52`，`responses_bridge`。
- 第二次响应包含明确的 `Content-Length: 1553`，证明 JSON 响应已经完整结束，不存在服务端漏写结束导致 Apifox 一直等待。
- `usage.input_tokens_details.image_tokens=1032`，证明参考图真实参与生成。
- 返回图片下载成功，PNG 文件约 `2.5 MB`。

Apifox 显示长时间转圈时，必须区分两种情况：

1. 最终收到 HTTP `502` 和 `Upstream request failed`：请求已经到达本地 8080，问题是该次上游生成失败或超时，不是 Apifox 无法解析 JSON。
2. 最终收到 HTTP `200` 且响应包含 `data[0].url`：接口已经成功；继续确认 `image_tokens > 0` 和图片 URL 可下载。

排查时优先记录响应头里的 `X-Request-ID` 或 `X-Client-Request-Id`，再用容器日志定位同一请求命中的账号和实际兼容路径。不要因为某一次上游 `502` 就修改模型映射、端口或请求体格式。

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
- 自定义 OpenAI-compatible API key 账号仍保留 `generations_json -> responses_bridge -> json_edits -> multipart_edits` 聚合兼容链路；已确认支持 Responses 的账号优先 `responses_bridge`。
- `generations_json` 返回 `image_tokens=0` 时必须继续回退，不能把忽略参考图的 HTTP `200` 当成图生图成功。
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
   - `openAIImagesPreferredCompatibleRoute`
   - `shouldContinueOpenAIImagesCompatibleAggregate`
   - `finalizeOpenAIImagesCompatibleAggregateError`
2. 确认 `/v1/images/generations` 的顶层 `image` 数组仍会解析进 `InputImageURLs`。
3. 确认带图片输入的自定义 OpenAI-compatible 账号仍走兼容聚合链路。
4. 确认已确认支持 Responses 的账号优先 `responses_bridge`，旧缓存不能覆盖该能力。
5. 确认 HTTP `200` 但 `image_tokens=0` 时仍会继续兼容回退。
6. 确认每条协议探测都使用 `openAIImagesBufferedResponseWriter`，失败路径不能直接污染真实客户端 Writer。
7. 确认普通 `502/503/504`、连接失败和超时不会在聚合层串行等待所有慢路径。
8. 确认只有明确协议不兼容错误才继续，认证、权限、额度、限流和内容策略错误立即终止。
9. 确认 `522` 和可同账号重试的服务器错误会交给 handler。
10. 确认 `finalizeOpenAIImagesCompatibleAggregateError` 不会清掉 `RetryableOnSameAccount`。
11. 如果官方改了 `response_format`，确认 `ChatCompletionsRequest.response_format` 仍可兼容对象格式和图像字符串格式。
12. 如果官方拆分 service 文件，确认 `GetAPIBaseURL`、账号 `BatchID/Schedulable`、Antigravity `cached_tokens` fallback、OpenAI `previous_response_id` 分组隔离没有丢。
13. 跑单测，再构建本地 Docker 镜像。
14. 先替换本地 `sub2api-dev`，确认 `http://127.0.0.1:8080/health` 正常。
15. 用 1K 图生图请求真实验证，再打包镜像上传服务器。
16. 确认 `startOpenAIImagesResponseHeaderHeartbeat` 同时包住原生 Images 与 Responses bridge 的上游请求，等待响应头时也会发送心跳。
17. 确认 `readOpenAIImagesNonStreamingResponseBody` 同时用于原生 Images JSON 和 Responses bridge 的非流式响应。
18. 确认 `openAIImagesBufferedResponseWriter.Unwrap` 仍存在，且 JSON 心跳不会让失败探测污染最终响应。

推荐单测命令：

```powershell
Set-Location F:\BC\调查\sub2api-repo\backend
go test ./internal/service -run "OpenAIImages|ImageGeneration"
go test ./internal/handler ./internal/server/routes
go test ./...
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
- 用响应头的 `X-Request-ID` 对照日志，确认该次请求实际命中的账号和兼容路径。
- 如果 Apifox 在数分钟后明确收到 `502 Upstream request failed`，说明它已经正常收到了服务端响应；此时排查上游，不要排查 Apifox JSON 解析。
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

## Docker 数据保护与应用替换

升级或回滚时只处理 `sub2api` 应用服务，不重建 PostgreSQL、Redis，不删除 `/app/data` 挂载目录。

本地替换命令：

```powershell
Set-Location F:\BC\调查\sub2api-repo\deploy
docker compose -f docker-compose.dev.yml up -d --no-build --no-deps sub2api
```

### 2026-07-14 非流式图片心跳本地镜像

修复提交：

```text
9c366687 fix: keep non-stream image requests alive
```

本地固定镜像标签：

```text
sub2api-custom:v0.1.147-non-stream-heartbeat-20260714-9c366687
```

镜像与运行状态：

```text
image_id=sha256:aa327b46042182c41ef4a1925e220b3bfa26ad59681987761331d628121bbeca
image_size_bytes=55236253
alias=deploy-sub2api:latest
container=sub2api-dev
port=0.0.0.0:8080->8080/tcp
health=http://127.0.0.1:8080/health -> 200 {"status":"ok"}
image_keepalive_interval=10
postgres=sub2api-postgres-dev -> healthy
redis=sub2api-redis-dev -> healthy
```

本次仅重建并替换 `sub2api-dev`，没有重建 PostgreSQL、Redis，也没有修改或清空数据目录。旧版本仍可通过以下固定标签回退：

```text
sub2api-custom:v0.1.147-image-probe-isolation-20260714-52b46a02
```

服务器替换命令：

```powershell
docker compose -f docker-compose.yml up -d --no-build --no-deps sub2api
```

必须遵守：

- 不执行 `docker compose down -v`。
- 不删除 `deploy/postgres_data`、`deploy/redis_data`、`deploy/data` 或服务器对应持久化目录。
- 不把 PostgreSQL、Redis、容器可写层或数据卷导出到应用镜像包。
- 不使用 `docker system prune --volumes` 清理项目环境。
- 固定标签加载完成后再更新 compose 使用的运行标签；回滚时重新加载上一份已验证镜像，只替换 `sub2api` 服务。
- 替换后同时检查 `sub2api`、PostgreSQL、Redis 状态，确认数据库和缓存容器没有被重建。

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

### 2026-07-13 Responses 优先图生图镜像记录

本次交付对应修复提交：

```text
19d08c36 fix: prefer confirmed responses for image edits
```

镜像直接从完成两次真实 1K 图生图验证的 `deploy-sub2api:latest` 增加固定标签后导出。没有重新构建镜像，没有替换或重建正在运行的 `sub2api-dev`，也没有打包容器、PostgreSQL、Redis 或数据卷。

固定镜像标签：

```text
sub2api-custom:v0.1.147-image-responses-20260713-2236-19d08c36
```

镜像包和校验文件：

```text
F:\BC\调查\sub2api-repo\backups\sub2api-image-20260713-2236-v0.1.147-image-responses-19d08c36.tar
F:\BC\调查\sub2api-repo\backups\sub2api-image-20260713-2236-v0.1.147-image-responses-19d08c36.tar.sha256.txt
F:\BC\调查\sub2api-repo\backups\sub2api-20260713-2236-v0.1.147-image-responses-19d08c36-metadata.txt
```

镜像信息：

```text
image_id=sha256:f4f8c945fca6391a8001bfc22d4fdac97e375ec2e1ab14ef318e1b0ba9ac6f40
image_size_bytes=55221310
archive_size_bytes=55245312
sha256=bc06d239a052235fd879f23c75b5f58662fd4a87d859cdfb2733b467237acace
container=sub2api-dev
port=0.0.0.0:8080->8080/tcp
health=http://127.0.0.1:8080/health -> 200 {"status":"ok"}
```

验证结果：

- `go test ./internal/service ./internal/handler -count=1` 通过。
- 用户原始 `/v1/images/generations` 顶层 `image` 请求两次返回 HTTP `200`。
- 两次均走 `responses_bridge`，`image_tokens=1032`，参考图真实参与生成。
- 已确认支持 Responses 的账号不会再被旧的 `generations_json` 路由缓存覆盖。
- HTTP `200` 但 `image_tokens=0` 的结果不会被误判为图生图成功。
- Cloudflare `522` 不再触发同账号完整重试和全部慢路径串行等待。

上传服务器时使用：

```powershell
docker load -i F:\BC\调查\sub2api-repo\backups\sub2api-image-20260713-2236-v0.1.147-image-responses-19d08c36.tar
docker tag sub2api-custom:v0.1.147-image-responses-20260713-2236-19d08c36 weishaw/sub2api:latest
Set-Location F:\BC\调查\sub2api-repo\deploy
docker compose -f docker-compose.yml up -d --no-build sub2api
```

本地恢复时，把目标标签改为 `deploy-sub2api:latest`，再使用 `docker-compose.dev.yml` 启动。不要直接把固定标签改名或覆盖为 `latest` 后重新导出，否则会失去包与提交之间的一一对应关系。

### 2026-07-14 通用响应隔离修复镜像记录

本次修复提交：

```text
52b46a02 fix: isolate image compatibility probe responses
```

修复范围是所有自定义 OpenAI-compatible 图片上游，不绑定 `cpa`、`ttt` 或某个账号：

- 每条兼容协议探测使用独立响应缓冲区。
- 失败探测的错误 JSON 不会污染后续成功响应。
- 只有明确的协议不兼容错误才继续下一条路径。
- 普通上游故障、认证、额度、限流和内容策略错误不会被错误地串行探测。

固定镜像标签：

```text
sub2api-custom:v0.1.147-image-probe-isolation-20260714-52b46a02
```

镜像与运行状态：

```text
image_id=sha256:c827ba9b2c626285968cefed71aeb009e8e0c7f20ec8e994d32a0ce86bb9d97e
image_size_bytes=55230488
container=sub2api-dev
port=0.0.0.0:8080->8080/tcp
health=http://127.0.0.1:8080/health -> 200 {"status":"ok"}
postgres=sub2api-postgres-dev -> healthy
redis=sub2api-redis-dev -> healthy
```

镜像包和校验：

```text
F:\BC\调查\sub2api-repo\backups\sub2api-image-20260714-0044-v0.1.147-image-probe-isolation-52b46a02.tar
F:\BC\调查\sub2api-repo\backups\sub2api-image-20260714-0044-v0.1.147-image-probe-isolation-52b46a02.tar.sha256.txt
F:\BC\调查\sub2api-repo\backups\sub2api-20260714-0044-v0.1.147-image-probe-isolation-52b46a02-metadata.txt
```

```text
archive_size_bytes=55254016
sha256=DE520110A756D0D162662BEDF64686E930A77A425BFE10B6E3610C590DE3190B
```

真实验证结果：

- 本地 `http://127.0.0.1:8080/v1/images/generations` 使用顶层 `image` 数组请求返回 HTTP `200`，耗时约 `169.8` 秒。
- 返回包含 `data[0].url`，`usage.input_tokens_details.image_tokens=1032`，返回 PNG 下载 HTTP `200`，大小约 `2.6 MB`。
- 本地日志确认请求使用 `responses_bridge`，并记录 `uploads=1`，说明参考图进入了图生图链路。
- 同一请求访问 `https://sub1.70api.top/v1/images/generations` 时曾在没有 HTTP 状态码或 JSON 的情况下提前断开。后续确认站点 Nginx 已配置 1800 秒超时，不能再把该现象简单归因于这组 Nginx 配置。

### 2026-07-14 云端非流式图片心跳修复

使用云端真实地址、同一个 API Key 和同一张参考图做对照：

- 纯文生图非流式返回 HTTP `200`，耗时约 `52.68` 秒。
- 图生图 `stream:true` 返回 HTTP `200`，约 `86.20` 秒完成，期间持续收到 SSE keepalive 和完整图片事件。
- 原始图生图非流式请求两次在约 `112.45` 秒和 `227.4` 秒被服务器侧关闭，`curl` 返回 `HTTP_STATUS=000` 和 `schannel: server closed abruptly`，没有任何 HTTP 响应头。

结论：图生图上游和账号本身可以成功，故障点是非流式请求在等待 Responses 图片 SSE 完成时一直没有向客户端发送数据，公网连接可能在最终 JSON 生成前被外层空闲连接策略关闭。

修复必须同时覆盖：

- `startOpenAIImagesResponseHeaderHeartbeat`：等待任意图片上游响应头时保持非流式客户端连接。
- `prepareOpenAIImagesNonStreamingResponse`：原生 Images JSON 非流式返回。
- `handleOpenAIImagesOAuthNonStreamingResponse`：Responses bridge 非流式返回。
- `openAIImagesBufferedResponseWriter.Unwrap`：允许心跳穿透协议探测缓冲层，但失败探测正文仍保持隔离。
- 心跳内容只能是 JSON 允许的前导空白，最终响应仍必须通过标准 JSON 解析。
- 首次心跳会提交下游 HTTP `200`。如果上游随后失败，错误正文仍是可解析 JSON，但传输层状态码无法再改成 `4xx/5xx`；这是保持标准非流式 JSON 且持续发送心跳的必要取舍。

必须保留的回归测试：

- `TestReadOpenAIImagesNonStreamingResponseBodySendsJSONHeartbeat`
- `TestReadOpenAIImagesNonStreamingResponseBodyHeartbeatBypassesProbeBuffer`
- `TestReadOpenAIImagesNonStreamingResponseBodyErrorAfterHeartbeatRemainsJSON`

验证命令：

```powershell
Set-Location F:\BC\调查\sub2api-repo\backend
go test ./internal/service -run "TestReadOpenAIImagesNonStreamingResponseBody" -count=1
go test ./...
go build ./cmd/server
```

服务器更新：

```powershell
docker load -i F:\BC\调查\sub2api-repo\backups\sub2api-image-20260714-0044-v0.1.147-image-probe-isolation-52b46a02.tar
docker tag sub2api-custom:v0.1.147-image-probe-isolation-20260714-52b46a02 weishaw/sub2api:latest
Set-Location <server-sub2api>/deploy
docker compose -f docker-compose.yml up -d --no-build --no-deps sub2api
```

本地恢复：

```powershell
docker load -i F:\BC\调查\sub2api-repo\backups\sub2api-image-20260714-0044-v0.1.147-image-probe-isolation-52b46a02.tar
docker tag sub2api-custom:v0.1.147-image-probe-isolation-20260714-52b46a02 deploy-sub2api:latest
Set-Location F:\BC\调查\sub2api-repo\deploy
docker compose -f docker-compose.dev.yml up -d --no-build --no-deps sub2api
```
