# API 参考

Base URL：`{{SITE_ORIGIN}}`。以下所有路径都需要 API Key，见
[认证](/docs/authentication)。

请求体与响应体沿用上游厂商自己的格式。本网关不发明新的 schema：Anthropic 的请求体
就是 Anthropic 的请求体。因此把下表读作 *"该发到哪里"*，把厂商官方文档读作
*"里面写什么"*。唯一属于本网关的端点是 `GET /v1/sub2api/billing`，见
[计费与用量](/docs/billing-and-usage)。

可用性取决于 Key 所属分组的平台。分组平台不提供的路径返回 `403`，不是 `404`。

## 文本

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/v1/messages` | Anthropic Messages。`"stream": true` 走流式。 |
| `POST` | `/v1/messages/count_tokens` | 计算 token。校验额度但不记录用量。也挂在 `/messages/count_tokens`。 |
| `POST` | `/v1/chat/completions` | OpenAI Chat Completions。 |
| `POST` | `/v1/responses` | OpenAI Responses。也挂在 `/responses`。 |
| `POST` | `/v1/responses/{subpath}` | Responses 子资源，例如取消。 |
| `GET` | `/v1/responses` | 获取某个 response。 |
| `POST` | `/v1/embeddings` | OpenAI 向量嵌入。 |
| `GET` | `/v1/models` | 该 Key 可调用的模型。也挂在 `/models`。 |

## Gemini

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/v1beta/models` | 列出模型。 |
| `GET` | `/v1beta/models/{model}` | 获取单个模型。 |
| `POST` | `/v1beta/models/{model}:generateContent` | 生成。 |
| `POST` | `/v1beta/models/{model}:streamGenerateContent` | 流式生成。 |

仅 Google 平台分组可用。此处错误使用 Google 的错误结构。

## 图像

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/v1/images/generations` | 同步生成。 |
| `POST` | `/v1/images/edits` | 同步编辑。 |
| `POST` | `/v1/images/generations/async` | 提交并返回任务 id。 |
| `POST` | `/v1/images/edits/async` | 提交并返回任务 id。 |
| `GET` | `/v1/images/tasks/{task_id}` | 轮询单个异步任务。 |

耗时较长的生成正是异步的用途：提交、保存 `task_id`、轮询。轮询是读操作，不会重复
计费。

### 批量图像

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/v1/images/batches` | 提交批次。 |
| `GET` | `/v1/images/batches` | 列出你的批次。 |
| `GET` | `/v1/images/batches/models` | 可用于批量的模型。 |
| `GET` | `/v1/images/batches/{id}` | 批次状态。 |
| `GET` | `/v1/images/batches/{id}/items` | 逐条状态。 |
| `GET` | `/v1/images/batches/{id}/items/{custom_id}/content` | 单条产出。 |
| `GET` | `/v1/images/batches/{id}/download` | 整批打包下载。 |
| `POST` | `/v1/images/batches/{id}/cancel` | 取消待处理条目。 |

带完整请求体的实操教程在控制台的 **批量图像指南** 中——需要登录后阅读。

## 视频

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/v1/videos`、`/v1/videos/generations` | 发起生成。 |
| `POST` | `/v1/videos/edits` | 发起编辑。 |
| `POST` | `/v1/videos/extensions` | 续写已有视频。 |
| `GET` | `/v1/videos/{request_id}` | 状态。也可用 `/v1/videos/generations/{request_id}` 及 `edits` / `extensions` 变体。 |
| `GET` | `/v1/videos/{request_id}/content` | 下载结果。 |

视频始终是异步的：发起、轮询状态、再取内容。

## 音频

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/v1/tts` | 文本转语音。 |
| `POST` | `/v1/stt` | 语音转文本。 |
| `POST` | `/v1/custom-voices` | 创建自定义音色。 |
| `GET` | `/v1/custom-voices` | 列出自定义音色。 |
| `GET` | `/v1/custom-voices/{voice_id}` | 获取单个音色。 |
| `GET` | `/v1/custom-voices/{voice_id}/audio` | 获取其示例音频。 |

## 实时与 Live

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/v1/realtime` | 实时会话。 |
| `POST` | `/v1/live` | 发起 live 通话。 |
| `GET` | `/v1/live/{call_id}` | live 旁路通道。 |
| `POST` | `/backend-api/codex/realtime/calls` | Codex 直连 live 通话。 |

## 搜索

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/v1/web_search` | 网页搜索。Grok 分组。 |
| `POST` | `/v1/x_search` | X 搜索。Grok 分组。 |
| `POST` | `/v1/alpha/search` | 也挂在 `/alpha/search`。 |

## 账户

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/v1/sub2api/billing` | 该 Key 当前生效的计费倍率。 |
| `GET` | `/v1/usage` | 你的消耗。可选 `days`，1–90。 |

见 [计费与用量](/docs/billing-and-usage)。
