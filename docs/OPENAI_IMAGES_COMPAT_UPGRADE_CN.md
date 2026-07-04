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

当请求带图片输入，并且账号是自定义 OpenAI-compatible `base_url` 时，`ForwardImages` 会进入兼容聚合转发：

1. `generations_json`：先按 `/v1/images/generations` 转发，并把远程参考图下载后内联为 data URL。
2. `responses_bridge`：如果第一步失败或上游忽略图片输入，则尝试 Responses 桥接路径。
3. `json_edits`：继续尝试 JSON 形式的 `/v1/images/edits`。
4. `multipart_edits`：最后尝试 multipart `/v1/images/edits`。

这个聚合链路由 `forwardOpenAIImagesAPIKeyCompatibleAggregate` 维护。它的作用是兼容不同上游对图生图的不同实现，不要在升级时删除其中任一路径。

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
5. 跑单测，再构建本地 Docker 镜像。
6. 先替换本地 `sub2api-dev`，确认 `http://127.0.0.1:8080/health` 正常。
7. 用 1K 图生图请求真实验证，再打包镜像上传服务器。

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
