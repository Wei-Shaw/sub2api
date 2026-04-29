# OpenCode `image_generation` 持久资源与上下文恢复设计

## 背景

OpenAI Responses 的 `image_generation` 返回格式不是普通 Chat Completions 工具结果，而是 `response.output[]` 中的 `image_generation_call` item。核心字段是：

```json
{
  "id": "ig_123",
  "type": "image_generation_call",
  "status": "completed",
  "result": "<base64-encoded image>",
  "revised_prompt": "..."
}
```

流式链路中，最终图片本体可能只出现在 `response.output_item.done.item.result`，而 `response.completed.response.output` 可以是空数组。因此网关不能只依赖 completed output。

OpenCode 当前的问题是：它把 `image_generation_call` 映射成 provider-executed `tool-call` / `tool-result`。这在本链路上不稳定，因为该 item 不是 OpenCode 本地真实工具结果；后续 `store:false` 时不会把结果可靠送回上游，`store:true` 时又会走 `item_reference`，而我们当前 OAuth 链路不能依赖上游持久化。结果是：Agent 可能看不到图片结果，并且历史消息可能被无效 provider-executed tool 状态污染。

## 目标

本设计只做 sub2api 侧 workaround，不修改 OpenCode。

目标如下：

1. OpenCode 客户端不再收到会被解析成 provider-executed tool 的 `image_generation_call`。
2. OpenCode 历史消息中保留稳定、普通文本形式的图片 marker 和下载链接。
3. Agent 可以在生成后 1 小时内从下载链接保存图片到本地文件。
4. 下一轮请求中，sub2api 能把仍可用的 marker 恢复成 `input_image`，让上游模型看到图片像素。
5. marker 过期、文件缺失或图片过大时，不破坏请求格式，不发 `item_reference`，只降级为普通文本说明。
6. 非 OpenCode 客户端继续保留标准 Responses `image_generation_call` 语义。

## 非目标

本次明确不做以下事情：

1. 不修改 OpenCode 源码。
2. 不把 `image_generation_call` 伪装成 OpenCode 本地真实工具。
3. 不长期保存生成图片；图片资源最长保留 1 小时。
4. 不引入数据库表存储图片。
5. 不依赖 OpenAI `store:true`、`previous_response_id` 或 `item_reference`。
6. 不保证过期 marker 能恢复图片；过期后只保证安全降级。

## 实现切分

为避免继续扩大 `openai_gateway_service.go`，实现应拆成清晰单元：

1. `openai_generated_image_store.go`
   - 负责图片 base64 解码、格式/MIME sniff、大小限制、原子写入、metadata 写入、过期判断、按 id 读取和 opportunistic cleanup。
2. `openai_opencode_image_rewrite.go`
   - 负责 OpenCode 响应输出替换、普通 message 构造、marker 文本构造、下载链接构造。
3. `openai_opencode_image_sse.go`
   - 负责 OpenCode SSE 事件过滤、`image_generation_call` 捕获、synthetic message SSE 序列生成、terminal completed output patch。
4. `openai_opencode_image_rehydrate.go`
   - 负责扫描 OpenCode 请求 input 中的 marker/下载链接，并在转发上游前注入 `input_image` 或过期说明。
5. 下载 handler / route 文件
   - 负责 `GET /sub2api/generated-images/:filename`，不挂到 `/v1` gateway 鉴权链。
6. 测试 helper
   - 负责临时数据目录、固定时钟、固定随机 id、图片 fixture 和 OpenCode 请求/响应构造。

## 核心设计

### 一、对 OpenCode 输出普通文本 marker

仅当 `isOpenCodeResponsesClient(c)` 为真时启用此兼容层。非 OpenCode 客户端不得执行输出替换，也不得执行输入 rehydrate。

当响应中出现 `image_generation_call.result` 时：

1. 将图片 base64 解码并写入磁盘临时资源。
2. 为资源生成高熵 opaque id，例如 `img_<random>`。
3. 生成稳定 marker：`sub2api-image://img_<random>`。
4. 在可确定公开访问根地址时生成绝对下载链接；不在 assistant 文本中暴露服务端相对路径。
5. 返回给 OpenCode 的内容改为普通 assistant 文本 message，不再包含 `image_generation_call`。

普通文本格式建议如下：

```text
Generated image: sub2api-image://img_abc123
Download URL: https://example.com/sub2api/generated-images/img_abc123.png
```

marker 本身必须与域名无关，避免反向代理或域名变化导致上下文恢复失败。绝对下载链接的来源优先级是：公开设置 `api_base_url` 去掉末尾 `/v1` 后的站点根地址、`cfg.Server.FrontendURL`、trusted forwarded host、trusted request Host。只有在可信代理或可信 Host 场景下才从当前请求 Host 或 forwarded headers 推导；如果 Host 不可信，只输出 marker，不输出由 Host 推导的绝对 URL，也不把 `/sub2api/generated-images/...` 这种服务端相对路径写进 assistant 文本，避免误导 Agent 把它当成本地文件或不可见字节来源。

OpenCode 的 agent loop 只有在 `finish == tool-calls` 或存在非 provider-executed tool part 时才会继续。纯 assistant text 会让 loop 结束，导致用户要求“生成后继续保存到本地文件”这类后续动作无法执行。因此，每个成功保存的图片 message 后还要追加一个合成的本地 `bash` `function_call`，命令仅 `echo` 一段 continuation 提示，不包含图片 base64，不直接写文件。该 tool call 的目的只是让 OpenCode 保持下一轮模型循环；真正是否下载、保存到哪里仍由下一轮模型根据用户原始指令和前面的图片引用决定。

### 二、非流式普通 message schema

OpenCode 非流式路径中，替换后的 item 必须是完整合法的 Responses message item：

```json
{
  "id": "msg_sub2api_img_abc123",
  "type": "message",
  "status": "completed",
  "role": "assistant",
  "content": [
    {
      "type": "output_text",
      "text": "Generated image: sub2api-image://img_abc123\nDownload URL: https://example.com/sub2api/generated-images/img_abc123.png",
      "annotations": []
    }
  ]
}
```

同一图片后追加的 continuation tool call 示例：

```json
{
  "id": "fc_sub2api_img_abc123",
  "type": "function_call",
  "status": "completed",
  "call_id": "call_sub2api_img_abc123",
  "name": "bash",
  "arguments": "{\"command\":\"echo \\\"sub2api generated image is ready; use the preceding generated image reference if needed, and continue the user's original request\\\"\",\"description\":\"Reports generated image availability\"}"
}
```

`id` 使用 synthetic message id，不复用 OpenAI `ig_...` 原始 id。`text` 和 `arguments` 必须通过 JSON marshal 输出，不能手写拼接 JSON 字符串。

### 三、图片资源保存与下载

图片资源写入 sub2api 数据目录下的专用子目录，例如：

```text
<DATA_DIR>/openai-generated-images/img_abc123.png
```

数据目录优先使用已有 setup 约定：`DATA_DIR` 环境变量，其次 Docker `/app/data`，最后当前目录。实现应复用现有 `setup.GetDataDir()` 语义，避免新增全局配置依赖。

下载端点：

```text
GET /sub2api/generated-images/:filename
```

端点必须被嵌入前端中间件放行。当前实现中，`SetupRouter` 先挂前端中间件再注册路由，因此需要把 `/sub2api/` 加入前端 bypass 规则，并新增测试证明该路径不会返回 SPA HTML。

下载路由行为：

1. 不列目录。
2. 不要求 API key；该 URL 是 1 小时 bearer capability URL。
3. 文件名必须匹配 anchored regex，例如 `^img_[A-Za-z0-9_-]{32,}\.(png|jpe?g|webp)$`。
4. 拒绝任何路径分隔符、`..`、NUL、URL encoded separator。
5. 只使用校验后的 filename 与固定 base dir `filepath.Join`，`filepath.Clean` 后必须仍位于 `<DATA_DIR>/openai-generated-images` 内。
6. 只允许图片扩展名和 MIME：`png`、`jpeg`、`jpg`、`webp`。
7. MIME 和格式以 magic bytes / 实际解码结果为准，不能只信任 `output_format`。
8. 返回正确 `Content-Type`、`X-Content-Type-Options: nosniff`。
9. 返回 `Content-Disposition: attachment; filename="..."`，文件名只能来自已校验 filename。
10. 文件存在但已过期时返回 `410 Gone`，文件不存在或 metadata 缺失/损坏时返回 `404 Not Found` 或 `410 Gone`，不能泄露内部路径。

资源 id 必须由 CSPRNG 生成，至少 128-bit 熵，建议 192/256-bit。碰撞时必须重试且禁止覆盖。不要把原始 OpenAI item id、账号信息、用户信息或 API key 放进 URL。

### 四、最长保留 1 小时

生成图片资源最长保留 1 小时。

实现策略：

1. 文件旁写一个轻量 metadata 文件，记录 `created_at`、`expires_at`、`mime`、`format`、`source_item_id`、`sha256`、`decoded_bytes`。
2. 图片文件和 metadata 都使用同目录临时文件写入，校验成功并 close 后 rename；不得覆盖已有资源。
3. 每次下载或 rehydrate 前检查 `expires_at`。
4. 过期文件即视为不可用，可以同步删除。
5. 启动时执行一次 bounded cleanup。
6. 保存新图片或请求路径上节流执行 opportunistic cleanup。
7. cleanup 单次最多扫描/删除固定数量文件，避免请求延迟被大量历史文件放大。
8. 设置单图 decoded size 上限、单次 rehydrate data URL 上限和目录总容量上限。

保留 1 小时的产品语义：OpenCode Agent 应在生成后尽快把下载链接保存到本地文件。1 小时后，历史 marker 仍可被识别，但只会降级为“图片资源已过期”。

### 五、下一轮输入 rehydrate

仅当 `isOpenCodeResponsesClient(c)` 为真时执行 rehydrate。

执行时机必须写在 `/v1/responses` 转发上游前：解析 `reqBody` 后、`builtin_tools` / `metadata.builtin_tools` 本地 carrier 已消费并从上游请求中删除后、`applyCodexOAuthTransform` 与 `buildUpstreamRequest` 之前，并早于 `validateCodexSparkInput` 与 `setOpsUpstreamRequestBody`。

因为 rehydrate 会把 `input_image.image_url` 写成 data URL，`setOpsUpstreamRequestBody` 和任何 request detail/log capture 只能接收 redacted upstream body。真实 upstream body 只用于 HTTP request；进入 ops/request detail/log 的副本必须把 `input_image.image_url`、data URL、`image_generation_call.result` 替换为占位符。

scanner 覆盖以下文本节点：

1. input item 的 string content。
2. role message 的 string content。
3. `content[].type == "input_text"` 的 `text`。
4. `content[].type == "output_text"` 的 `text`。

识别两类引用：

1. `sub2api-image://img_<opaque>` marker。
2. `/sub2api/generated-images/img_<opaque>.<ext>` 或同路径绝对 URL。

命中且资源未过期、未超出 rehydrate 上限时，向上游 input 中插入合成 user message：

```json
{
  "role": "user",
  "content": [
    {
      "type": "input_text",
      "text": "Generated image context restored by sub2api from sub2api-image://img_abc123."
    },
    {
      "type": "input_image",
      "image_url": "data:image/png;base64,..."
    }
  ]
}
```

插入位置应靠近发现 marker 的历史消息之后，以尽量保持对话顺序。`input_image` 只能插入 synthetic `role:"user"` message，不能塞进 assistant content。

为避免上下文膨胀，每次请求最多 rehydrate 最近 3 张图片；同一图片多次出现只注入一次。过期/缺失 marker 也要去重和限量，避免数天历史反复注入大量过期说明。

marker 文本本身可以保留为普通历史文本，也可以在转发上游前压缩成简短说明，但不能把完整 base64 放进文本。

### 六、过期、缺失和超限降级

OpenCode 消息会长期保留 marker，因此过期是正常情况，不能视为错误。

当 marker 命中但资源不可用、已过期或超过 rehydrate 大小上限时：

1. 不返回 4xx 给 OpenCode。
2. 不向上游发送 `item_reference`。
3. 不发送 `image_generation_call`。
4. 不发送 `input_image`。
5. 在转发上游时把原 marker block 压缩或替换为普通文本说明：

```text
The generated image referenced by sub2api-image://img_abc123 is no longer available as image bytes. sub2api keeps generated image resources for at most 1 hour. Ask the user to provide the saved local file or regenerate the image.
```

如果资源仍可下载但超过 rehydrate 上限，则说明应改为：图片下载链接仍可用，但本次没有作为 `input_image` 附加给上游。这样模型不会误以为它已经看到了图片像素。

### 七、流式响应处理

OpenCode 流式路径必须避免透出所有 `response.image_generation_call.*` 事件，包括但不限于：

1. `response.image_generation_call.in_progress`
2. `response.image_generation_call.generating`
3. `response.image_generation_call.partial_image`
4. `response.image_generation_call.completed`

同时必须过滤：

1. `response.output_item.added` 中的 `item.type == "image_generation_call"`。
2. `response.output_item.done` 中的 `item.type == "image_generation_call"`。
3. `response.completed` / `response.done` 的 `response.output[]` 中残留的 `image_generation_call`。

当 `response.output_item.done.item.result` 到达时，保存图片资源，并向 OpenCode 补发完整 synthetic message SSE 序列。序列应使用新的 synthetic message id，并优先复用被过滤的原 `image_generation_call` 的 `output_index`；该 index 没有暴露给下游，且上游不会再用同一 output item index 发送其他 item。不要简单使用 `maxSeen+1`，避免流未结束时与后续 upstream output index 冲突。

synthetic message SSE 序列至少包含：

```text
event: response.output_item.added
data: {"type":"response.output_item.added","output_index":<synthetic_index>,"item":{"id":"msg_sub2api_img_abc123","type":"message","status":"in_progress","role":"assistant","content":[]}}

event: response.content_part.added
data: {"type":"response.content_part.added","item_id":"msg_sub2api_img_abc123","output_index":<synthetic_index>,"content_index":0,"part":{"type":"output_text","text":"","annotations":[]}}

event: response.output_text.delta
data: {"type":"response.output_text.delta","item_id":"msg_sub2api_img_abc123","output_index":<synthetic_index>,"content_index":0,"delta":"Generated image: sub2api-image://img_abc123\nDownload URL: https://example.com/sub2api/generated-images/img_abc123.png"}

event: response.output_text.done
data: {"type":"response.output_text.done","item_id":"msg_sub2api_img_abc123","output_index":<synthetic_index>,"content_index":0,"text":"Generated image: sub2api-image://img_abc123\nDownload URL: https://example.com/sub2api/generated-images/img_abc123.png"}

event: response.content_part.done
data: {"type":"response.content_part.done","item_id":"msg_sub2api_img_abc123","output_index":<synthetic_index>,"content_index":0,"part":{"type":"output_text","text":"Generated image: sub2api-image://img_abc123\nDownload URL: https://example.com/sub2api/generated-images/img_abc123.png","annotations":[]}}

event: response.output_item.done
data: {"type":"response.output_item.done","output_index":<synthetic_index>,"item":{"id":"msg_sub2api_img_abc123","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"...","annotations":[]}]}}
```

OpenCode 当前 parser 对 delta-only 也能自动创建 text-start，但本设计要求完整 message SSE 序列，避免未来 parser 变更或其他 Responses 客户端出现 dangling delta。

如果没有 `output_item.done`，但 `response.completed.response.output[]` 中首次出现带 `result` 的 `image_generation_call`，必须在转发该 `response.completed` / `response.done` 前先保存图片并发送同样的 synthetic message SSE 序列。terminal output patch 只能作为终态清理，不能作为可见输出 fallback，因为 OpenCode streaming parser 不会从 terminal output 自动生成 assistant text。

如果 image `output_item.done` 没有 `result`，仍必须过滤原事件，不能透出 provider tool 状态。实现可以发送一条普通文本说明，表示图片生成 item 已完成但没有可保存的图片结果；terminal completed output 也必须清理残留 `image_generation_call`。

### 八、非流式响应处理

OpenCode 非流式路径中，`sanitizeOpenCodeResponsesOutput()` 需要升级：

1. `web_search_call` 继续按现有逻辑过滤。
2. `image_generation_call` 不再原样保留。
3. 如果有 `result`，保存图片并替换为第二节定义的普通 `message` item。
4. 如果没有 `result`，替换为普通文本说明，避免 OpenCode 进入 tool 状态。

非 OpenCode 路径不走该替换，继续返回标准 Responses output。

### 九、当前未提交改动和推荐配置关系

当前已有的 `ResponsesOutput.Result` 等字段扩展、`responsesOutputStableKey(image_generation_call)` 支持，对非 OpenCode 客户端和 OpenCode 前置 SSE capture 仍有价值，可以保留。

OpenCode 最终写回客户端前必须执行本设计的替换，不能因为 typed 结构支持了 `image_generation_call.result` 就把 raw `image_generation_call` 暴露给 OpenCode。

`UseKeyModal.vue` 中 `sub2api-openai` 的 OpenCode 推荐配置链应保留：`gpt-5.5*` 仍可通过 `metadata.builtin_tools.image_generation` 启用图片工具。不要改成 upstream `provider.openai`，也不要删除 GPT-5.5 image_generation 推荐。

第一版不保留无条件的 Codex/OpenCode `image_generation` 自动注入。OpenCode image generation 启用来源应是 `metadata.builtin_tools.image_generation` 或等价本地 carrier，主要由 `UseKeyModal.vue` 的推荐配置提供。旧的 `TestForwardResponsesRequest_CodexClientAutoInjectsImageGenerationTool` 应删除或改写为“carrier 触发 + OpenCode 响应会被 rewrite 保护”的测试。若未来恢复通用自动注入，也必须同时证明响应写回 OpenCode 前不会暴露 raw `image_generation_call`。

实现不得通过 `store:true`、`previous_response_id` 或 `item_reference` 解决该问题，也不得把本地 carrier `metadata` 透传上游。

### 十、日志与隐私

实现必须避免图片内容进入日志和持久化请求详情：

1. 不记录完整 base64、data URL、`image_generation_call.result`。
2. 下载 URL、filename、opaque id/token 在 access/request/ops/payload 日志中应脱敏或只记录路由模板。
3. usage/request detail 不保存图片本体。
4. 错误日志中只记录资源 id 的短 hash 或脱敏片段、状态码、错误类别。
5. metadata 中的 `source_item_id` 只用于本地调试，不写入 URL、用户文本或普通日志。

## 测试计划

按 TDD 实现，至少新增以下测试：

1. OpenCode 非流式响应中，`image_generation_call.result` 被替换为普通 message，且 message schema 包含 `id/type/status/role/content/output_text/annotations`。
2. 非 OpenCode 非流式响应仍保留 `image_generation_call`。
3. OpenCode 流式响应中过滤所有 `response.image_generation_call.*`，过滤 image `output_item.added/done`，最终补发完整普通 message SSE 序列。
4. `response.completed.response.output[]` 含 image 且缺少 `output_item.done` 时，OpenCode 终态 output 仍被替换为普通 message。
5. 图片资源写盘后，下载端点返回正确 MIME、`Content-Disposition`、`nosniff` 和内容。
6. 嵌入前端开启时，下载路径被 bypass，不返回 SPA HTML。
7. 过期资源下载返回 `410 Gone` 或等价过期响应。
8. 非法 filename、path traversal、URL encoded separator、NUL、unsupported extension 被拒绝。
9. malformed base64、unsupported format、oversized image 按设计降级。
10. 下一轮 OpenCode 请求中 marker 命中未过期资源时，上游请求被注入 synthetic user `input_image`。
11. 下一轮非 OpenCode 请求中出现 marker 文本时，不触发 rehydrate。
12. marker 过期或缺失时，只注入普通缺失说明，不发 `item_reference`、`image_generation_call` 或 `input_image`。
13. 同一 marker 多次出现时只 rehydrate 一次。
14. 多张 marker 时最多 rehydrate 最近 3 张。
15. 并发保存多个图片时 opaque id 不冲突，不覆盖已有文件。
16. 同一响应多张图片时，每张图都有独立 marker 和下载链接。
17. cleanup 不删除未过期文件，单次 cleanup 有扫描/删除上限。
18. 绝对 URL 生成优先使用 public base URL，Host fallback 不接受不可信 Host。
19. `UseKeyModal.vue` focused 测试继续确认 `gpt-5.5*` 推荐配置包含 `metadata.builtin_tools.image_generation`。
20. rehydrate 注入 `input_image` 后，`OpsUpstreamRequestBodyKey`、request detail 和日志脱敏副本不包含 `data:image` 或原始 base64。

后续实现计划至少应包含以下验证命令：

```text
go test -tags unit ./internal/service ./internal/pkg/apicompat -count=1
go test ./internal/server/... ./internal/handler/... -count=1
go build ./cmd/server
pnpm exec vitest run src/components/keys/__tests__/UseKeyModal.spec.ts --pool=forks --poolOptions.forks.singleFork --reporter=verbose
pnpm typecheck
pnpm build
rtk git diff --check
```

Go 命令在当前 Windows 环境通常应使用完整路径：`C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe`。

## 风险与缓解

1. 链接可被拥有消息内容的人访问。
   - 缓解：使用 CSPRNG 高熵 opaque id、无目录列举、1 小时最长保留、不包含敏感账号信息。
2. 生成图片较大，转发时 data URL 增加请求体大小。
   - 缓解：限制单图 decoded bytes 和 rehydrate data URL bytes；超限时仍提供下载链接，但不做 rehydrate，并给普通文本说明。
3. 旧 marker 过期后无法恢复图片。
   - 缓解：消息明确提示 1 小时内下载；过期后要求用户提供本地文件或重新生成。
4. 反向代理域名变化导致链接不可用。
   - 缓解：marker 与域名无关；rehydrate 只依赖 marker id 和本地文件，不依赖 URL host。
5. 单实例本地文件不跨节点。
   - 缓解：第一版明确是单实例或共享 `DATA_DIR` 语义；多实例无共享存储时，其他节点只能按缺失资源降级。

## 验收标准

1. OpenCode 历史消息中不再出现 `image_generation_call` provider tool 状态。
2. OpenCode 用户或 Agent 能从消息里的下载链接保存图片文件。
3. 1 小时内继续对话时，sub2api 能把 marker 恢复成上游 `input_image`。
4. 1 小时后继续对话时，请求仍成功，只降级为普通缺失说明。
5. 非 OpenCode 客户端的标准 Responses image generation 行为不变。
