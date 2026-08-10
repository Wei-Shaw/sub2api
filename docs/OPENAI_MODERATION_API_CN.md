# OpenAI Moderation API 集成与运维指南

本文说明 sub2api 当前如何调用 OpenAI Moderation API，以及管理员应如何配置、测试、监控和排障。内容同时区分 OpenAI 官方 API 契约与 sub2api 的本地策略，避免把供应商返回的 `flagged`、图片能力或默认阈值误认为 sub2api 会原样采用。

本文以仓库当前实现为准。OpenAI 模型能力、价格、限流和响应字段可能变化，应同时核对 OpenAI 官方文档和实际项目控制台。

## 结论先行

- sub2api 调用 `POST <base_url>/v1/moderations`，默认 `base_url` 是 `https://api.openai.com`，默认模型是 `omni-moderation-latest`。
- `base_url` 不应包含 `/v1`。例如应填写 `https://api.openai.com`，而不是 `https://api.openai.com/v1`。
- 实际启用需要同时满足三个条件：全局 `risk_control_enabled=true`、内容审计 `enabled=true`、`mode` 不是 `off`。
- `pre_block` 会在转发业务请求前同步审核并可能拦截；`observe` 通常先放行业务请求，再异步审核。
- `observe` 不是绝对不拦截。如果开启 `pre_hash_check_enabled` 且命中历史风险哈希，当前请求仍会同步被拦截。
- 审核顺序不是只调用 OpenAI。主要顺序是关键词、历史风险哈希、确定性抽样、Moderation API。
- sub2api 不采用 OpenAI 返回的 `flagged` 作为最终结论，而是比较 `category_scores` 与本地 13 个类别阈值。
- Moderation API 不可用时，旧版内容审计链路采取 fail-open：超时、网络错误、非 2xx、无可用 Key、无效响应等情况通常放行原业务请求。
- API 命中后的日志、风险哈希、邮件和自动封号属于异步副作用。在 `pre_block` 中，即使客户端已经被拦截，队列满仍可能使这些副作用全部丢失；任务已出队后则先写哈希、执行封号和邮件，最后写日志，因此数据库写日志失败也可能出现“副作用已发生但没有审计行”。
- OpenAI 官方当前说明 Moderation endpoint 免费，但仍需有效的 OpenAI Platform API Key，并受组织或项目限流约束。

## 架构与调用链

网关请求进入安全审计协调器后，由旧版内容审计适配器调用 `ContentModerationService`：

```text
网关请求
  -> Gateway checkSecurityAudit / runSecurityAudit
  -> securityaudit.Coordinator.Check
  -> securityaudit.LegacyModerationAdapter.Check
  -> ContentModerationService.Check
  -> 本地关键词检查
  -> Redis 历史风险哈希检查
  -> 确定性抽样
  -> ContentModerationService.callModeration
  -> ContentModerationService.callModerationOnceWithInput
  -> POST <base_url>/v1/moderations
  -> 本地阈值判定
  -> 放行或按网关协议返回拦截错误
  -> 异步写日志、风险哈希、邮件和自动封号
```

`securityaudit.Coordinator` 还可同时编排独立的 Prompt Audit。本文所说的 fail-open 仅指旧版 OpenAI 内容审计。Prompt Audit 在 blocking 模式下不可用或响应无效时可能返回 HTTP `503`，不要把两套机制的故障策略混为一谈。

## 快速启用

### 管理后台

推荐按以下顺序启用：

1. 在管理员系统设置中开启全局风控，即保存 `risk_control_enabled=true`。
2. 进入管理员风控中心，开启内容审计 `enabled`。
3. 先选择 `observe`，配置 OpenAI Base URL、模型和至少一个 API Key。
4. 使用 API Key 测试功能验证连通性，并用代表性文本或图片检查本地阈值结果。
5. 观察一段时间的日志、误报、延迟、队列丢弃和 Key 状态。
6. 调整阈值、模型范围和分组范围后，再切换为 `pre_block`。

关闭全局开关或内容审计开关不会删除已有配置、日志或 Redis 风险哈希。

### 管理 API

管理 API 前缀是 `/api/v1/admin/risk-control`。以下示例使用管理员 JWT：

```bash
export SUB2API_URL="https://sub2api.example.com"
export ADMIN_JWT="<admin-jwt>"
export OPENAI_API_KEY="<openai-platform-api-key>"
```

先开启全局风控：

```bash
curl "$SUB2API_URL/api/v1/admin/settings" \
  -X PUT \
  -H "Authorization: Bearer $ADMIN_JWT" \
  -H "Content-Type: application/json" \
  -d '{"risk_control_enabled":true}'
```

再保存内容审计配置。`PUT /config` 支持部分更新，未提交的普通字段保留原值：

```bash
curl "$SUB2API_URL/api/v1/admin/risk-control/config" \
  -X PUT \
  -H "Authorization: Bearer $ADMIN_JWT" \
  -H "Content-Type: application/json" \
  -d "{
    \"enabled\": true,
    \"mode\": \"observe\",
    \"base_url\": \"https://api.openai.com\",
    \"model\": \"omni-moderation-latest\",
    \"api_keys\": [\"$OPENAI_API_KEY\"],
    \"api_keys_mode\": \"append\",
    \"timeout_ms\": 3000,
    \"retry_count\": 2,
    \"sample_rate\": 100,
    \"all_groups\": true,
    \"record_non_hits\": true
  }"
```

测试已保存的 Key：

```bash
curl "$SUB2API_URL/api/v1/admin/risk-control/api-keys/test" \
  -X POST \
  -H "Authorization: Bearer $ADMIN_JWT" \
  -H "Content-Type: application/json" \
  -d '{}'
```

测试一段实际文本。临时 Key 不会写入内容审计配置，测试也不会写 `content_moderation_logs`；但该管理 POST 受管理员操作审计中间件记录，`api_keys` 会脱敏，捕获上限内的 `prompt` 和较小的 `images` data URL 可能保存在管理员操作审计日志中：

```bash
curl "$SUB2API_URL/api/v1/admin/risk-control/api-keys/test" \
  -X POST \
  -H "Authorization: Bearer $ADMIN_JWT" \
  -H "Content-Type: application/json" \
  -d '{"prompt":"需要审核的测试文本"}'
```

确认状态：

```bash
curl "$SUB2API_URL/api/v1/admin/risk-control/status" \
  -H "Authorization: Bearer $ADMIN_JWT"
```

## OpenAI 上游请求契约

### URL 与 Header

生产代码发出的请求为：

```http
POST <base_url>/v1/moderations
Authorization: Bearer <api-key>
Content-Type: application/json
```

默认配置：

```text
base_url = https://api.openai.com
model    = omni-moderation-latest
```

代码使用 URL path join 将 `/v1/moderations` 追加到 `base_url`。因此：

| 配置 | 实际目标 | 结论 |
| --- | --- | --- |
| `https://api.openai.com` | `https://api.openai.com/v1/moderations` | 正确 |
| `https://proxy.example.com` | `https://proxy.example.com/v1/moderations` | 代理需暴露该路径 |
| `https://api.openai.com/v1` | `https://api.openai.com/v1/v1/moderations` | 错误，不要包含 `/v1` |

后端只进行 URI 语法校验，没有强制绝对 URL、HTTPS、主机白名单或 SSRF allowlist。类似 `/proxy` 的相对 URI 也可能保存成功，但发起 HTTP 请求时会因缺少 scheme/host 失败并触发 fail-open。管理员必须配置包含 scheme 和 host 的可信 HTTPS 绝对 URL。

### 纯文本请求

```json
{
  "model": "omni-moderation-latest",
  "input": "提取并规范化后的文本"
}
```

### 图片或图文请求

```json
{
  "model": "omni-moderation-latest",
  "input": [
    {
      "type": "text",
      "text": "提取并规范化后的文本"
    },
    {
      "type": "image_url",
      "image_url": {
        "url": "https://example.com/image.png"
      }
    }
  ]
}
```

只有图片而没有文本时，`input` 数组中只包含 `image_url` part。图片 URL 可以是 HTTP(S) URL，也可以是 `data:image/...;base64,...` data URL。

### sub2api 实际解析的响应

OpenAI 官方响应包括顶层 `id`、`model`，以及每个结果中的 `flagged`、`categories`、`category_scores` 和 `category_applied_input_types`。sub2api 当前只解码：

```json
{
  "results": [
    {
      "flagged": true,
      "category_scores": {
        "harassment": 0.1,
        "violence": 0.99
      }
    }
  ]
}
```

当前实现行为：

- 只使用 `results[0]`，后续结果被忽略。
- `results` 为空视为调用错误。
- `flagged` 虽然被解码，但不参与最终拦截判定。
- `categories` 不解码，也不参与判定。
- `category_applied_input_types` 不解码。
- 顶层 `id` 和 `model` 不解码。
- 最终命中完全由 `category_scores` 与本地阈值决定。

这意味着代理或兼容服务即使返回 `flagged=true`，只要没有返回本地识别的类别分数，sub2api 也可能放行。

## 请求进入审核的条件

一个普通网关请求要进入 OpenAI Moderation API，依次需要满足：

1. 内容审计服务及设置、日志仓储可用。
2. 全局 `risk_control_enabled=true`。
3. 内容审计配置 `enabled=true`。
4. `mode` 是 `observe` 或 `pre_block`，不是 `off`。
5. 请求分组位于审核范围内。
6. 客户端请求模型位于模型过滤范围内。
7. 能从请求体提取出非空文本或至少一张图片。
8. 未被前置关键词或历史哈希直接处理。
9. 通过 `sample_rate` 确定性抽样。
10. 至少存在一个配置的 API Key。
11. 至少存在一个当前未冻结的可用 API Key。

范围之外、输入为空、未被抽样或无 Key 时，旧版内容审计直接放行，不调用 Moderation API。只有尚无运行时快照时的首次配置加载失败会直接放行；已有快照过期后会先继续使用旧策略并后台刷新，刷新失败时旧策略仍可继续审核甚至拦截。

## 完整判定顺序

### `pre_block`

```text
全局开关
  -> 内容审计开关和模式
  -> 分组范围
  -> 模型范围
  -> 提取并规范化输入
  -> 本地关键词
  -> keyword_only 未命中则直接放行
  -> Redis 历史风险哈希
  -> 确定性抽样
  -> API Key 可用性
  -> 同步调用 Moderation API，含重试
  -> category_scores 与本地阈值比较
  -> 命中则阻断，否则放行
  -> 异步执行日志、哈希、邮件、封号副作用
```

### `observe`

```text
全局开关
  -> 内容审计开关和模式
  -> 分组范围
  -> 模型范围
  -> 提取并规范化输入
  -> Redis 历史风险哈希，命中时仍立即阻断
  -> 确定性抽样
  -> 至少有一个已配置 API Key
  -> 将审核任务放入异步队列
  -> 当前业务请求立即放行
  -> worker 选择未冻结的可用 API Key
  -> worker 调用 Moderation API
  -> 命中时记录 action=allow、flagged=true
  -> 记录风险哈希、发送邮件并可能自动封号
```

`observe` 中不执行关键词阻断。即使配置 `keyword_only`，observe 路径仍会进入异步 API 审核。

## 模式语义

| 模式 | API 调用 | 当前请求 | 命中后副作用 |
| --- | --- | --- | --- |
| `off` | 不调用 | 放行 | 无 |
| `observe` | 异步 | 通常放行 | 可写日志、写风险哈希、发邮件、自动封号 |
| `pre_block` | 同步 | 本地阈值命中时拦截 | 异步写日志、写风险哈希、发邮件、自动封号 |

重要例外：

- `observe + pre_hash_check_enabled` 命中历史哈希时同步拦截。
- observe 的历史哈希拦截日志仍通过同一异步记录队列写入；队列满时客户端已经被拦截，但可能没有对应 `hash_block` 日志。
- `pre_block + keyword_only` 未命中关键词时会在哈希检查之前直接放行，因此不会执行历史哈希检查和 OpenAI API 调用。
- `pre_block` 的客户端拦截结果不依赖日志队列是否成功入队。队列满时仍可拦截客户端，但日志和后续副作用可能丢失。
- 已进入 observe 队列的任务会在 worker 执行时重新检查当前内容配置、Key、分组和模型范围，但不会重新检查全局 `risk_control_enabled`。关闭全局开关后，已经入队的任务仍可能继续处理。
- 已进入队列的 pre-block 记录任务使用命中时克隆的配置，且在 worker 中不重新检查全局开关、内容开关、模式、邮件或自动封号设置。命中后再关闭相关开关不能撤销已经排队的日志、哈希、邮件或封号副作用。

## 输入提取规则

sub2api 不会把完整会话历史原样发送给 OpenAI。消息类协议主要只取数组最后一个元素，并要求它是可审核的用户输入。

| 协议常量 | 主要入口 | 提取行为 |
| --- | --- | --- |
| `anthropic_messages` | Anthropic Messages | 只检查 `messages` 最后一项；必须是 `role=user`；提取支持的文本和图片 part |
| `openai_chat_completions` | OpenAI Chat Completions | 只检查 `messages` 最后一项；必须是 `role=user`；提取字符串、content part 文本和图片 |
| `openai_responses` | OpenAI Responses | `input` 为字符串时直接提取；为数组时只检查最后一项；最后一项需是用户或无 role 的可审核输入 |
| `gemini` | Gemini | 只检查 `contents` 最后一项；role 为空或 `user`；提取 `parts[].text`、未验证 MIME 的 inline data，以及仅限 HTTP(S)/data 前缀的 file URI |
| `openai_images` | OpenAI/Grok 图片入口 | 提取顶层 `prompt` 和规范化后的 `images` |

边界行为：

- 如果最后一项是 assistant、tool、function 等非用户内容，不会向前回溯寻找更早的用户消息，而是把本次输入视为空并跳过审核。
- tool 输出结构本身也不审核，即使外层使用用户角色。例如 Anthropic `role=user` 的 `tool_result`、Gemini `role=user` 的 `functionResponse`、Responses 的 `function_call_output` 位于末尾时，旧版审核不会提取其输出，也不会回溯之前的用户消息。
- 所有协议共享的文本添加函数会对大小写敏感的字面量 `<system-reminder>` 做 `Contains` 检查。任一文本 part 包含该字面量时会丢弃整个 part，而不是只删除标签；若没有其他文本或图片，请求会因输入为空而放行。Anthropic 路径还会在特定文本分支检查该前缀。
- Anthropic system/developer 指令和大部分历史消息不会发送给 Moderation API。
- Responses API 的 `input` 字符串例外，它本身直接作为审核文本。
- 未识别协议会尝试从 `input`、`messages` 和 `contents` 提取。若请求结构不匹配，例如某些 embeddings 请求，通常得到空输入并跳过审核。
- 当前 `POST /v1/images/batches` 是已知覆盖缺口。该 handler 把 prompt 包装在 `request.items[].prompt`，但 `openai_images` 提取器只读取顶层 `prompt` 和 `images`，也没有传递批量条目的参考图片。因此旧版 Moderation API 会得到空输入并 fail-open，即使配置为 `pre_block` 也不会审核这些批量 prompt 或参考图片。
- HTTP 请求内成功完成一次安全审计后，可复用完成标记，避免同一个 Gin 请求被多个阶段重复扫描。WebSocket 每个需要审核的 frame 不使用该 HTTP 完成缓存。

## 文本规范化、图片与哈希

### 文本

提取后的文本会：

- 去除首尾空白。
- 用单个空格折叠连续空白，包括换行、Tab 等。
- 截断到前 12,000 个 Unicode rune。
- 多个 content part 先用换行拼接，再在规范化时折叠为空格。

截断发生在调用 OpenAI 和计算输入哈希之前。因此超过 12,000 rune 的后半部分既不会送审，也不会影响风险哈希。

### 图片

运行时图片处理规则：

- 接受以 `data:`、`http://` 或 `https://` 开头的提取结果。
- 去除首尾空白，按完整字符串精确去重，保留第一次出现的顺序。
- 一次 Moderation API 请求最多发送一张图片。
- 如果提取到多张图片，使用加密随机数选择一张；随机数失败时取第一张。
- 所有提取并去重后的图片都参与输入哈希，而不只是本次随机发送的一张。
- 普通网关路径不在本地下载图片，也不执行管理测试接口的 8 MiB 校验。远程图片是否可访问、格式和大小是否被接受由 OpenAI 或代理决定。
- Gemini inline data 不校验 MIME 是否真是图片，音频或视频 MIME 也可能被包装成 `image_url` 后送往 OpenAI，随后得到 HTTP `400` 并 fail-open；`gs://` 等非 HTTP(S)/data file URI 会被静默丢弃。

这会产生一个需要关注的边界：多图请求每次只随机审核一张，但历史风险哈希代表整组图片。改变图片顺序、添加或删除任一图片都会改变哈希。

### 输入哈希

风险哈希为 SHA-256 十六进制字符串，概念上按以下内容计算：

```text
SHA256(
  "text:" + normalized_text
  + 对每张图片依次追加 "\nimage:" + SHA256(image_string)
)
```

API 阈值命中会把哈希写入 Redis Set：

```text
content_moderation:flagged_hashes
```

该 Set 当前没有 TTL，也不随数据库日志保留策略自动清理。关键词命中不会写风险哈希，哈希命中也不会重复写入。

## 确定性抽样

`sample_rate` 的范围为 0 到 100。抽样不是每次请求重新随机，而是根据规范化输入哈希前两个字节稳定计算：

```text
uint16(hash[0:2]) % 100 < sample_rate
```

因此同一个规范化输入通常会持续被选中或持续被跳过：

- `100`：全部进入 API 审核。
- `0`：全部跳过 API 审核。
- `10`：约 10% 的不同输入进入 API 审核，但同一输入的结果稳定。

关键词和历史哈希检查发生在抽样之前。降低 `sample_rate` 只减少后续 API 调用，不会绕过已经执行的关键词检查或历史哈希拦截。

## 本地阈值判定

### 默认阈值

| 类别 | 默认阈值 | OpenAI 官方支持的输入类型 |
| --- | ---: | --- |
| `harassment` | 0.98 | 文本 |
| `harassment/threatening` | 0.90 | 文本 |
| `hate` | 0.65 | 文本 |
| `hate/threatening` | 0.65 | 文本 |
| `illicit` | 0.95 | 文本 |
| `illicit/violent` | 0.95 | 文本 |
| `self-harm` | 0.65 | 文本、图片 |
| `self-harm/intent` | 0.85 | 文本、图片 |
| `self-harm/instructions` | 0.65 | 文本、图片 |
| `sexual` | 0.65 | 文本、图片 |
| `sexual/minors` | 0.65 | 文本 |
| `violence` | 0.95 | 文本、图片 |
| `violence/graphic` | 0.95 | 文本、图片 |

图片支持列来自 OpenAI 官方说明，不代表其他兼容服务具有相同能力。

### 判定算法

对上述 13 个固定类别逐一执行：

```text
如果 category_scores[category] >= thresholds[category]
则本地 flagged = true
```

任意一个固定类别达到阈值即命中。具体细节：

- 使用 `>=`，分数恰好等于阈值也命中。
- 缺失的类别分数按 Go map 的零值 `0` 处理。
- 未知类别不会触发阈值命中。
- 未知类别如果分数最高，仍可能显示为 `highest_category`。
- `highest_score` 是所有返回类别中的最高分。
- 管理测试返回的 `composite_score` 当前仅等于 `highest_score`，不是加权复合分。
- OpenAI 返回的 `flagged` 不参与此算法。

不要把任何阈值设置为 `0`。由于缺失类别按 `0` 处理且判定使用 `>=`，阈值为 `0` 会使所有成功返回的审核结果都命中。

### 阈值更新语义

`PUT /config` 提交 `thresholds` 时，会把提交值覆盖到代码默认阈值，而不是覆盖到上一次已保存的自定义阈值。因此：

- 省略整个 `thresholds` 字段：保留当前阈值。
- 提交部分 `thresholds` 对象：提交类别使用新值，未提交类别恢复代码默认值。
- 提交空对象 `{}`：所有类别恢复默认值。
- 已知类别值会被限制到 `0..1`。
- 未知类别会被丢弃。

## 关键词策略

关键词仅在 `pre_block` 模式执行。

| `keyword_blocking_mode` | 行为 |
| --- | --- |
| `keyword_and_api` | 关键词命中立即拦截；未命中继续哈希、抽样和 API 审核 |
| `keyword_only` | 关键词命中立即拦截；未命中立即放行，不检查历史哈希，也不调用 API |
| `api_only` | 忽略关键词列表，继续哈希、抽样和 API 审核 |

关键词匹配是大小写不敏感的子串匹配，不是分词、正则或语义匹配。配置规范化规则：

- 最多保留 10,000 个关键词。
- 每个关键词最多 200 rune。
- 去除空白项。
- 大小写不敏感去重，保留第一次出现的写法和顺序。
- 非法 `keyword_blocking_mode` 不报错，而是规范化为 `keyword_and_api`。

关键词命中的 `highest_category` 为 `keyword`，分数为 `1.0`，日志 action 为 `keyword_block`。关键词命中会参与违规计数、邮件和自动封号，但不会写入风险哈希。

## 配置字段参考

### 核心与范围

| 字段 | 默认值 | 规范化与语义 |
| --- | --- | --- |
| `enabled` | `false` | 内容审计子开关，不替代全局开关 |
| `mode` | `pre_block` | `off`、`observe`、`pre_block`；显式空字符串恢复 `pre_block`，其他未知值拒绝保存 |
| `base_url` | `https://api.openai.com` | 去除空白和尾部 `/`；只校验 URI 语法 |
| `model` | `omni-moderation-latest` | 空值恢复默认；后端不验证模型是否存在 |
| `timeout_ms` | `3000` | `<=0` 恢复 3000；最大 30000；后端没有 500ms 最小限制 |
| `retry_count` | `2` | 限制为 `0..5`；总尝试槽位为 `retry_count + 1` |
| `sample_rate` | `100` | 限制为 `0..100`；确定性抽样 |
| `all_groups` | `true` | true 时忽略 `group_ids` 范围限制 |
| `group_ids` | `[]` | 去除非正数、去重、升序；`all_groups=false` 时验证已有 ID |
| `model_filter` | `{"type":"all","models":[]}` | `all`、`include`、`exclude`；模型名精确、大小写不敏感匹配 |
| `record_non_hits` | `false` | 记录 API 审核通过和 API 错误；不记录所有范围跳过或抽样跳过 |

`all_groups=false` 且 `group_ids=[]` 是合法配置，效果是所有分组都不审核。

模型过滤最多 1,000 个模型名，每个最多 200 rune。过滤匹配客户端请求的公开模型名，不使用映射后的上游模型名，不支持通配符、前缀或正则。非法 filter type 会静默规范化为 `all`；`include` 或 `exclude` 最终没有模型时拒绝保存。

### 队列、拦截与账号处置

| 字段 | 默认值 | 规范化与语义 |
| --- | --- | --- |
| `worker_count` | `4` | `<=0` 恢复 4；最大 32 |
| `queue_size` | `32768` | `<=0` 恢复默认；最大 100000；后端允许小于前端建议的值 |
| `block_status` | `403` | 必须为 400 到 599；`<=0` 恢复 403 |
| `block_message` | `内容审计命中风险规则，请调整输入后重试` | 去除空白；空字符串恢复默认，不能保存真正的空消息 |
| `email_on_hit` | `true` | 普通 API 或关键词命中时发送违规提醒；不控制 `cyber_policy` 专用邮件 |
| `auto_ban_enabled` | `true` | 达到阈值时禁用普通用户 |
| `ban_threshold` | `10` | `<=0` 恢复 10；要禁用功能应关闭 `auto_ban_enabled` |
| `violation_window_hours` | `720` | `<=0` 恢复 720，即 30 天 |
| `cyber_policy_exclude_from_ban_count` | `false` | 是否从自动封号计数中排除独立的 `cyber_policy` 事件 |
| `pre_hash_check_enabled` | `false` | 是否在 API 调用前拦截历史风险哈希 |
| `hit_retention_days` | `180` | `<=0` 恢复 180；最大 3650 |
| `non_hit_retention_days` | `3` | `<=0` 恢复 3；最大也为 3 |

`cyber_policy` 是 OpenAI 上游返回的另一类事后硬阻断记录，不是 Moderation API 的类别。该记录只受全局 `risk_control_enabled` 约束，不受本页 `enabled`、模式、范围、抽样或 `email_on_hit` 约束；只要邮件服务和用户邮箱可用，就会尝试发送专用通知。

## 多 API Key 管理

### 存储与返回

- 配置支持多个 API Key。
- Key 去除首尾空白、删除空值并按完整字符串精确去重。
- 配置读取不返回明文，只返回掩码、SHA-256 hash 和进程内健康状态。
- 掩码通常是 `********` 加最后 4 个字节；长度不超过 4 的值显示 `****`。
- Key hash 是修剪后 Key 的 SHA-256，用于状态关联和按 hash 删除。
- 内容审计配置以 JSON 保存在 settings 中，应用层没有对其中的 API Key 做专用加密。数据库、备份、日志访问和磁盘加密必须按敏感凭据标准管理。

### 追加、替换、删除和清空

`PUT /config` 的 Key 操作优先级：

1. `clear_api_key=true`：清空所有 Key，并忽略同一请求中的添加、替换和删除。
2. 规范化 `api_keys_mode`：只有 `replace` 表示替换，其他值和省略值都按 append。
3. 非 replace 模式先应用 `delete_api_key_hashes`。
4. 提交了 `api_keys` 时，replace 模式替换列表，append 模式追加列表。
5. 最后只要旧式 `api_key` 非空，就会追加到上述结果，即使当前是 replace 模式。

注意：

- replace 模式忽略 `delete_api_key_hashes`。
- replace 模式显式提交 `api_keys:[]` 可清空所有 Key。
- 只提交 `api_keys_mode:"replace"` 而不提交 `api_keys` 不会替换。
- 非法删除 hash 在配置更新中被静默忽略。
- 未知 `api_keys_mode` 按 append 处理。
- 旧字段 `api_key` 只用于兼容添加单个 Key；空值不会清空已有 Key。
- replace 请求不要同时提交旧式 `api_key`，否则它会在替换后的 `api_keys` 列表末尾继续追加。

按 hash 删除一个已保存 Key：

```bash
curl "$SUB2API_URL/api/v1/admin/risk-control/config" \
  -X PUT \
  -H "Authorization: Bearer $ADMIN_JWT" \
  -H "Content-Type: application/json" \
  -d '{
    "api_keys_mode":"append",
    "delete_api_key_hashes":["<64-char-key-sha256>"]
  }'
```

替换全部 Key：

```bash
curl "$SUB2API_URL/api/v1/admin/risk-control/config" \
  -X PUT \
  -H "Authorization: Bearer $ADMIN_JWT" \
  -H "Content-Type: application/json" \
  -d '{
    "api_keys_mode":"replace",
    "api_keys":["<key-1>","<key-2>"]
  }'
```

## Key 选择、重试和冻结

### 选择算法

- 使用进程内原子游标进行 round-robin。
- 跳过 `frozen_until` 仍在未来的 Key。
- 不按当前并发、历史延迟、成功率或 failure count 做负载均衡。
- `active` 等并发指标只用于观测，不限制每个 Key 的并发。
- Key 健康状态是单进程内存状态，不写数据库或 Redis，进程重启后重置。
- 多实例部署中，每个实例有独立游标、冻结状态和统计。

### 重试次数与退避

总尝试槽位：

```text
attempts = retry_count + 1
最小 1，最大 6
```

失败后等待时间为：

```text
第 1 次失败后等待 100ms
第 2 次失败后等待 200ms
第 3 次失败后等待 300ms
依此类推，无 jitter
```

每次网络尝试都有独立的 `timeout_ms`。默认最多三次尝试，理论上限约为 `3 x 3s + 300ms = 9.3s`。最大配置在父 context 不更早取消时，理论上可达 `6 x 30s + 1.5s = 181.5s`。observe worker 的父 context 约为 40 秒，因此异步路径会更早终止。

实际网络请求次数可能少于尝试槽位。例如只有一个 Key 时，第一个 HTTP `500` 会冻结它 10 秒；下一尝试找不到未冻结 Key，循环立即结束。

### HTTP 状态与冻结

| 结果 | 是否继续重试 | 新冻结时间 | failure count |
| --- | --- | --- | --- |
| 成功的有效 2xx 响应 | 停止并成功 | 清除冻结 | 重置为 0 |
| HTTP `400` | 立即停止 | 不新增冻结 | 不增加 |
| 无 HTTP 响应，状态记为 `0` | 有剩余槽位则重试 | 不冻结 | 不增加 |
| HTTP `401`、`403` | 有剩余槽位则换可用 Key | 10 分钟 | 增加 |
| HTTP `429`、`529` | 有剩余槽位则换可用 Key | 1 分钟 | 增加 |
| 其他 HTTP 状态 | 有剩余槽位则换可用 Key | 10 秒 | 增加 |

请求构造错误、JSON marshal 错误、DNS/TLS/网络错误和超时通常没有 HTTP 响应，因此状态为 `0`，不会冻结 Key。

以下情况虽然收到 2xx，也会被视为错误：

- 响应 JSON 无法解码。
- `results` 为空。

由于 HTTP 状态已记录，这类无效 2xx 响应会进入“其他 HTTP 状态”的默认分支，使 Key 冻结 10 秒。

HTTP `400` 的特殊语义：

- 立即停止当前调用的后续重试，避免用其他 Key 重放明显无效的请求。
- Key 状态仍会显示 `error`，记录 `last_error` 和 `last_http_status=400`。
- 不新增冻结，Key 仍可被正常流量再次选择。
- 如果 Key 在管理员显式测试前已经处于冻结状态，后续 `400` 不会清除之前的冻结；成功请求才会清除。

### 健康状态

| 状态 | 条件 |
| --- | --- |
| `unknown` | 尚无健康记录 |
| `ok` | 有成功记录或已测试，且当前没有错误 |
| `error` | `last_error` 非空且当前未冻结 |
| `frozen` | `frozen_until` 在未来，优先于 error |

冻结到期不会自动清除 `last_error`，所以状态通常从 `frozen` 变为 `error`，直到下一次成功调用清除错误。

## 管理 API Key 测试

`POST /api/v1/admin/risk-control/api-keys/test` 请求字段：

```json
{
  "api_keys": ["<temporary-key>"],
  "base_url": "https://api.openai.com",
  "model": "omni-moderation-latest",
  "timeout_ms": 3000,
  "prompt": "测试文本",
  "images": ["data:image/png;base64,..."]
}
```

行为：

- 显式提交规范化后非空的 `api_keys` 时，每个临时 Key 按顺序测试一次，不保存配置；省略、空数组或全空白数组都会退回已保存 Key。
- 未提交 Key 且没有实际 prompt/image 时，使用 `"hello"` 依次测试所有已保存 Key。
- 未提交 Key 但有实际 prompt/image 时，round-robin 选择一个未冻结的已保存 Key。
- 只有“使用已保存 Key 且有实际 prompt/image”的路径会在所有 Key 冻结时不发请求。没有实际输入的 `"hello"` 连通性测试会直接逐个测试已保存 Key，包括当前已冻结的 Key；显式临时 Key 也不经过冻结筛选。
- 测试接口不使用 `retry_count`，每个 Key 只调用一次。
- 多 Key 测试顺序执行，没有 Key 数量上限。大量 Key 可能使请求持续很久。
- 第一把成功 Key 的评分生成 `audit_result`；后续 Key 仍继续测试。
- 单个 Key 失败通常只反映在 item 状态中，整个管理 API 仍返回 HTTP 200。
- 测试会更新进程内 Key 健康状态，但不会写内容审核日志、风险哈希、邮件或自动封号。
- 测试不会执行关键词、分组、模型范围、历史哈希或抽样逻辑。

测试 prompt 规范化并截断到 12,000 rune。空 prompt 且无图片时发送 `"hello"`。

测试图片限制：

- 最多一张非空图片。
- 必须是小写前缀 `data:image/` 的 base64 data URL。
- data URL header 必须包含 `;base64`。
- 解码后最大 8 MiB。
- 整个 data URL 字符串最大 12 MiB。
- 只校验 data URL 和 base64，不深度验证 MIME 与真实图片内容是否一致。

OpenAI 官方当前说明图片文件最大 20 MB，这是上游限制；sub2api 管理测试接口的本地限制是 8 MiB，两者不要混淆。

## 异步队列与 worker

同一队列承载两类任务：

- `observe` 模式的异步 Moderation API 审核。
- 同步 API/关键词拦截和任意模式历史哈希拦截后的日志、哈希、邮件和封号记录任务。
- `pre_block + record_non_hits=true` 的每次成功 API 放行日志。该配置会为每个审核通过请求额外入队一个记录任务。

服务内部创建最多 32 个 worker goroutine，但只有 ID 小于当前 `worker_count` 的 worker 会消费任务。物理 channel 容量为 100,000，配置的 `queue_size` 是入队前通过 `len(channel)` 检查的软阈值，不是原子保证的严格容量；并发入队或运行时调小配置时，长度和 `queue_usage_percent` 可能超过配置值与 100%。

队列满时：

- observe 审核任务被丢弃，当前业务请求已经放行。
- 同步拦截后的记录任务被丢弃，客户端拦截结果不改变，但日志和副作用可能丢失；这也包括 observe 中的历史哈希同步拦截日志。
- pre-block 非命中记录任务被丢弃时，业务请求仍正常放行，但对应 `allow` 日志丢失。
- `dropped` 增加。

observe 任务在出队后如果因当前配置、Key、分组或模型范围不再匹配而被跳过，不增加 `processed` 或 `dropped`。因此 `enqueued`、`processed`、`dropped` 不能用于严格对账。

普通 API/关键词命中记录任务出队后的执行顺序是风险哈希、自动封号、邮件、数据库日志。数据库 `CreateLog` 失败不会回滚先前副作用，可能出现用户已被禁用或收到邮件、Redis 也有哈希，但管理页面没有对应日志行。API 调用失败且 `record_non_hits=true` 时，`action=error` 日志不走记录队列，而是在执行 `checkSync` 的 context 中直接写库：pre-block 会增加原客户端请求等待时间但不计入 `pre_block_avg_latency_ms`；observe 则占用 worker 时间，不延迟已经放行的原请求。

建议同时监控 `queue_length`、`queue_usage_percent`、`dropped`、`active_workers` 和 `errors`，并结合数据库、Redis、邮件服务及服务端日志。单看业务请求是否被拦截不能证明审计记录和账号副作用成功执行。

## 日志、风险哈希、邮件与自动封号

### 日志 action

| action | 含义 |
| --- | --- |
| `allow` | API 审核通过，或 observe 模式下 API 已命中但当前请求仍放行 |
| `block` | pre-block 中 OpenAI 类别分数达到本地阈值 |
| `keyword_block` | 本地关键词命中 |
| `hash_block` | 历史风险哈希命中 |
| `error` | Moderation API 调用失败且开启 `record_non_hits` |
| `cyber_policy` | 独立的 OpenAI 上游事后策略事件，不是 Moderation API 结果 |

observe 模式命中时通常是 `flagged=true`、`action=allow`。因此不要只按 action 查询命中事件。

日志中的 `input_excerpt`：

- 只保存文本摘要，不保存图片 base64。
- 最多 240 rune。
- 写入前使用正则脱敏 URL、常见 token、API Key、JWT、长十六进制串、长 base64 等内容。
- 正则脱敏不能保证识别所有敏感数据，仍应限制风控日志访问权限和保留时间。

上述脱敏只覆盖 `input_excerpt`。上游非 2xx 响应体最多读取 512 字节并拼入错误字符串，随后可原样进入内容日志的 `error` 和进程内 Key 状态的 `last_error`；这些字段不经过 `redactContentModerationSecrets`。不可信或兼容代理若回显输入、URL token 或凭据，管理接口与日志可能暴露这些内容。

### 风险哈希命中

开启 `pre_hash_check_enabled` 后，命中 Redis Set 中已有哈希会：

- 在抽样和 API Key 检查前同步拦截。
- 使用配置的 `block_status`。
- 在非空 `block_message` 后附加 `（hash: <64hex>）`。
- 写 `hash_block` 日志。
- 不重新写哈希。
- 不发送邮件。
- 不计入违规次数，不触发自动封号。

管理 API 当前不能列出 Set 中的所有哈希，只能在状态中查看数量、按已知 hash 删除或全部清空。排查误报时通常从客户端 hash-block 错误消息取得完整 hash。

### 自动封号

API 阈值命中和关键词命中可参与自动封号：

- 统计 `violation_window_hours` 内 `flagged=true` 的记录。
- 始终排除 `hash_block`。
- 可通过 `cyber_policy_exclude_from_ban_count` 排除 `cyber_policy`。
- 只统计最近一次 `auto_banned=true` 日志之后的事件。
- 当前尚未持久化的命中按 `+1` 计入本次判断。
- 达到 `ban_threshold` 时把普通用户状态改为 `disabled`，并失效其认证缓存。
- 管理员用户不会被自动禁用，但违规计数仍可记录。
- observe 模式的异步命中也可能在请求已放行后禁用用户。

该计数和更新没有数据库事务、用户级锁或原子计数。多个 worker 并发处理同一用户时可能读取相同的历史数量而延迟封号；历史计数查询失败时，本次判断会退化为 `count=1`。因此自动封号是最终一致、尽力而为的处置，不是严格的并发阈值。

`email_on_hit=true` 时每次普通 API 或关键词命中都会发违规提醒。新发生的自动封号还会独立发送账户禁用邮件，即使 `email_on_hit=false`；`cyber_policy` 专用通知也不受该开关控制。

### 保留与清理

- 服务启动约 5 分钟后首次清理。
- 之后约每 24 小时清理一次。
- 命中日志使用 `hit_retention_days`，默认 180 天。
- 非命中和 error 日志按 `flagged=false` 使用 `non_hit_retention_days`，最大 3 天。
- 清理任务即使风控当前关闭也会运行。
- Redis 风险哈希没有自动保留期限，不随日志删除。

## 管理接口

所有接口位于 `/api/v1/admin/risk-control`：

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| `GET` | `/config` | 读取规范化配置和脱敏 Key 状态 |
| `PUT` | `/config` | 部分更新配置、添加或删除 Key |
| `POST` | `/api-keys/test` | 测试临时或已保存 Key，可附文本或一张图片 |
| `GET` | `/status` | 获取进程内指标、Key 状态和风险哈希数量 |
| `GET` | `/logs` | 分页查询审计日志 |
| `POST` | `/users/:user_id/unban` | 把指定用户状态设为 active 并失效认证缓存 |
| `DELETE` | `/hashes` | 按 64 位 SHA-256 删除一个风险哈希 |
| `DELETE` | `/hashes/all` | 清空全部风险哈希 |

### 认证与响应 envelope

管理接口支持：

- `Authorization: Bearer <admin-jwt>`。
- `x-api-key: <admin-api-key>`。这是管理员 API Key，不是网关用户 API Key。

正常注入 `SettingService` 的运行环境要求管理员先完成当前版本的合规确认，否则除合规接口外的风控管理路由会返回 HTTP `423` 和 `ADMIN_COMPLIANCE_ACK_REQUIRED`；该要求没有生产/开发环境分支。这些路由没有单独的 TOTP step-up middleware，但仍受管理员认证、面板限流和管理操作审计中间件约束。

普通成功响应：

```json
{
  "code": 0,
  "message": "success",
  "data": {}
}
```

下文描述的字段通常位于 `data` 内。

### 查询日志

```bash
curl "$SUB2API_URL/api/v1/admin/risk-control/logs?page=1&page_size=20&result=hit" \
  -H "Authorization: Bearer $ADMIN_JWT"
```

支持 query：

| 参数 | 语义 |
| --- | --- |
| `page` | 默认 1 |
| `page_size` 或 `limit` | 默认 20，最大 100 |
| `result` | `hit/flagged`、`blocked/block`、`pass/allow`；`error` 匹配任何 `error` 字段非空的记录 |
| `group_id` | 正整数分组 ID |
| `endpoint` | 精确匹配入口路径 |
| `search` | 模糊搜索 request ID、用户邮箱、API Key 名、模型、输入摘要 |
| `from` | RFC3339 或 `YYYY-MM-DD` |
| `to` | RFC3339 或 `YYYY-MM-DD`；仅日期时包含 UTC 当日末尾 |

`blocked` 只匹配 `block`、`keyword_block`、`hash_block`，不包含 `cyber_policy`。`hit` 按 `flagged=true` 查询，更适合完整查看命中。`result=error` 不是按 `action=error` 查询，带有上游错误正文的 `cyber_policy` 行也可能出现。

### 删除风险哈希

```bash
curl "$SUB2API_URL/api/v1/admin/risk-control/hashes" \
  -X DELETE \
  -H "Authorization: Bearer $ADMIN_JWT" \
  -H "Content-Type: application/json" \
  -d '{"input_hash":"<64-char-sha256>"}'
```

合法但不存在的 hash 返回 HTTP 200 和 `deleted=false`。大写十六进制会规范化为小写。

清空全部风险哈希：

```bash
curl "$SUB2API_URL/api/v1/admin/risk-control/hashes/all" \
  -X DELETE \
  -H "Authorization: Bearer $ADMIN_JWT"
```

返回的 `deleted` 是删除前 Set 的成员数。该操作不删除数据库中的历史日志。

### 解封用户

```bash
curl "$SUB2API_URL/api/v1/admin/risk-control/users/1001/unban" \
  -X POST \
  -H "Authorization: Bearer $ADMIN_JWT"
```

接口会把找到的用户设为 `active`，即使该用户不是由内容审计禁用。它不会删除违规日志、风险哈希或显式重置统计。

## 运行状态与监控

`GET /status` 的主要字段：

### 开关与 worker

| 字段 | 含义 |
| --- | --- |
| `enabled` | 内容审计子开关 |
| `risk_control_enabled` | 全局风控开关 |
| `mode` | 当前内容审计模式 |
| `worker_count` | 配置启用的异步 worker 数 |
| `max_workers` | 代码最大 worker 数 32 |
| `active_workers` | 正在处理异步审核或记录任务的 worker |
| `idle_workers` | 配置 worker 减去 active |

### 队列

| 字段 | 含义 |
| --- | --- |
| `queue_size` | 当前软准入阈值，不是严格 channel 容量 |
| `queue_length` | 当前队列长度 |
| `queue_usage_percent` | `queue_length / queue_size * 100` |
| `enqueued` | 进程启动后成功入队数 |
| `dropped` | 队列满等原因丢弃数 |
| `processed` | worker 完成异步审核或记录任务数；出队后因配置不匹配而跳过的不计入 |
| `errors` | observe 异步 API 审核错误数，不包含所有 DB、邮件或 panic 错误 |

### pre-block

| 字段 | 含义 |
| --- | --- |
| `pre_block_active` | 当前同步 API 审核数 |
| `pre_block_checked` | 同步路径已计数检查，包含本地关键词、哈希、抽样和无 Key 等决策 |
| `pre_block_allowed` | 同步路径放行数 |
| `pre_block_blocked` | API、关键词或哈希拦截数 |
| `pre_block_errors` | 同步 API 错误和无 Key 等错误数 |
| `pre_block_avg_latency_ms` | API 调用阶段累计延迟除以 checked；本地零延迟决策也进入分母，不包含同步 error 日志的数据库写入耗时 |

### API Key

| 字段 | 含义 |
| --- | --- |
| `pre_block_api_key_active` | 所有已保存 Key 的同步在途 API 调用数 |
| `pre_block_api_key_available_count` | 当前未冻结 Key 数；error 但未冻结的 Key 仍算可用 |
| `pre_block_api_key_total_calls` | 同步 API 尝试总数，重试每次单独计数 |
| `pre_block_api_key_loads` | 每 Key 的 active、total、success、errors、平均延迟和最近状态 |
| `api_key_statuses` | 每 Key 的 `unknown/ok/error/frozen`、最近错误和冻结时间 |
| `flagged_hash_count` | Redis 风险哈希 Set 当前成员数 |

这些计数和 Key 健康大多是进程内状态，重启后归零。多实例部署中，请求命中不同节点时看到的状态可能不同，不能把单节点 `/status` 当作集群汇总。

`/status` 直接读取当前 settings 和 Redis：全局开关读取失败时 `risk_control_enabled` 会显示 false，风险哈希计数失败时 `flagged_hash_count` 会显示 0，而响应中没有字段区分“真实关闭/为空”和“依赖读取失败”。另外，热路径可能仍在使用旧运行时快照，所以 `/status` 与实际执行策略可短暂甚至在持续刷新失败时长期不一致。监控必须结合服务端 warning 及独立数据库、Redis 健康检查。

建议告警：

- `risk_control_enabled=true` 但 `enabled=false` 或 `mode=off`。
- `pre_block_api_key_available_count=0`。
- `dropped` 持续增长。
- `queue_usage_percent` 长时间高位。
- `pre_block_errors` 或 `errors` 持续增长。
- Key 长时间处于 `frozen` 或 `error`。
- `last_http_status` 集中出现 `401/403`、`429/529` 或 `5xx`。
- `pre_block_avg_latency_ms` 接近业务网关可接受延迟上限。

## 拦截响应

本地旧版内容审计在内部和 OpenAI/Anthropic/WebSocket 响应中使用错误码：

```text
content_policy_violation
```

HTTP 状态由 `block_status` 决定，无效值在运行时回退为 `403`。响应 body 按入口协议包装：

- Anthropic Messages：`{"type":"error","error":{"type":"content_policy_violation","message":"..."}}` 风格。
- OpenAI Chat Completions：`{"error":{"type":"content_policy_violation","message":"..."}}` 风格。
- OpenAI Responses 有两条 handler 路径：主 `OpenAIGatewayHandler` 旧版拦截返回 `{"error":{"type":"content_policy_violation","message":"..."}}`；兼容 `GatewayHandler` 路径返回 `{"error":{"code":"content_policy_violation","message":"..."}}`。
- Gemini：Google error envelope，HTTP 状态保持配置值；旧版内容审计分支只返回数字 code、Google status 和 message，不包含 `content_policy_violation` 字符串。
- OpenAI WebSocket：先发送 `evt_content_moderation_blocked` error frame，再按策略违规关闭。

不同 handler 的具体字段略有差异。OpenAI、Anthropic 和 WebSocket 客户端可结合 HTTP 状态、`content_policy_violation` 和配置 message；Gemini 旧版拦截只能依赖 HTTP 状态和 message。

## 故障策略

旧版 OpenAI 内容审计对以下情况采取 fail-open：

- 服务依赖或配置仓储不可用。
- 首次运行时配置读取失败。
- 全局或内容审计开关关闭。
- 输入提取为空。
- 无已配置 API Key。
- 所有 API Key 当前被冻结。
- URL/request/payload 构造失败。
- DNS、TLS、网络、context timeout 等 transport 错误。
- 上游返回非 2xx。
- 上游响应 JSON 无法解码。
- 上游 `results` 为空。
- observe 队列满，审核任务被丢弃。

observe 在请求线程只检查是否至少配置一把 Key，不检查是否存在未冻结 Key。所有 Key 都冻结时，任务仍会入队并放行当前请求；worker 后续找不到可用 Key，不发送 HTTP 请求，将该任务计为已处理的异步错误，并在 `record_non_hits=true` 时直接尝试写 `action=error` 日志。

在 `pre_block` 中，API 调用失败会增加错误指标，并在 `record_non_hits=true` 时尽力记录 `action=error`，但原始业务请求仍被允许。

配置故障需区分冷、热状态：尚无运行时快照时读取失败会 fail-open；已有快照后，过期请求先继续使用旧快照，后台刷新失败会保留旧配置并延迟重试。因此设置仓储故障不会保证放行，旧的关键词、阈值、模式和全局开关可能继续生效并拦截请求。

这是一项可用性优先的设计，不是强制合规网关。若业务要求“审核服务异常时必须拒绝所有请求”，当前旧版内容审计实现不满足该要求，需要额外设计 fail-closed 策略及其降级、熔断和告警机制。

## 供应商兼容性

sub2api 当前没有 Moderation provider adapter。兼容服务必须同时满足：

1. 接受 `POST <base_url>/v1/moderations`。
2. 接受 Bearer API Key。
3. 接受 `{"model":...,"input":...}`。
4. 图像场景接受 OpenAI 的 `text`、`image_url` part 结构。
5. 返回非空 `results`。
6. 在 `results[0].category_scores` 返回与本地 13 个固定 OpenAI 类别同名的数值。

| 服务 | 当前直接兼容性 | 说明 |
| --- | --- | --- |
| OpenAI 官方 | 完全兼容 | 推荐 `https://api.openai.com` 和 `omni-moderation-latest` |
| LiteLLM Proxy | 条件兼容 | 代理后端必须暴露 OpenAI `/v1/moderations`，并原样返回 OpenAI 类别分数 |
| Mistral Moderation | 不建议直接用于生产拦截 | 路径和文本请求可能相似，但类别名与本地 13 类大多不同，未知类别不会触发阈值；图片契约也不等价 |
| DeepSeek | 不直接兼容 | 没有当前实现所需的 OpenAI Moderations 契约 |
| Groq/Cloudflare Llama Guard | 不直接兼容 | 通常返回生成式标签而非固定 `category_scores` |
| Qwen3Guard | 不直接兼容 | 请求和输出契约不匹配 |
| AWS Bedrock Guardrails | 不直接兼容 | 使用 AWS 专用 API 和认证 |
| Gemini Safety | 不直接兼容 | 安全设置与响应结构不同 |

仅仅“返回 `flagged=true`”不够，因为 sub2api 忽略该字段。接入其他服务前必须抓取真实响应，确认类别名、分数范围、图片支持和错误语义，并通过管理测试及集成测试验证。

## 安全与隐私

- 发给 Moderation API 的是用户输入内容，可能包含个人信息、商业秘密、代码、图片 URL 或 base64 图片。部署前应评估供应商数据处理条款、地域、保留和合规要求。
- `base_url` 可由管理员配置且只做 URI 语法校验。不要指向不可信服务，避免把用户输入和 API Key 泄露给第三方。
- OpenAI API Key 保存在 settings JSON 中。限制数据库、备份、管理员 API 和审计日志访问，并使用磁盘或数据库层加密。
- 管理接口返回完整 Key hash。hash 不能直接还原 Key，但仍应视为敏感关联标识。
- `input_excerpt` 有正则脱敏和长度限制，但不是通用 DLP。不要依赖它保证完全脱敏。
- 上游错误正文进入 `error`/`last_error` 时不使用该脱敏函数；只接入可信代理，并限制配置、状态和日志接口权限。
- 管理 API Key 测试不会写内容审计日志，但测试 POST 会进入管理员操作审计；Key 会按字段脱敏，prompt 和捕获上限内的小型图片 data URL 可能被保存。
- 风险哈希没有 TTL。虽然只保存 SHA-256，但低熵、可猜测输入可能被字典验证，应限制 Redis 访问。
- 使用 HTTP 图片 URL 会让 OpenAI 或代理访问该 URL。应评估 URL 中 token、内网地址和访问控制风险。
- 普通运行时接受 `http://` 图片 URL；生产上建议由可信上游和网络策略强制 HTTPS。

## 上线建议

1. 先启用全局开关和 `observe`，保持 `sample_rate=100` 建立基线。
2. 开启 `record_non_hits` 只用于短期校准，注意非命中日志最多保留 3 天且会增加数据库写入。
3. 使用真实但已脱敏的代表性文本和图片测试 13 类阈值。
4. 检查 `flagged=true, action=allow` 的 observe 日志，评估误报和漏报。
5. 为至少两把不同项目或额度池的 Key 验证 round-robin 和冻结切换。
6. 为 `401/403`、`429/529`、`5xx` 和 timeout 建立监控。
7. 确认业务可接受 fail-open；不可接受时不要直接切换 `pre_block` 作为唯一控制。
8. 切换 `pre_block` 前先关闭或谨慎配置自动封号，避免阈值误报批量禁用用户。
9. 最后再评估开启 `pre_hash_check_enabled`。开启后 observe 也会同步拦截历史哈希。
10. 定期检查 Redis 风险哈希数量，并建立误报 hash 删除流程。

## 排障

### 配置已开启，但没有调用 OpenAI

按顺序检查：

1. `/status` 中 `risk_control_enabled` 是否为 true。
2. `/status` 中 `enabled` 是否为 true。
3. `mode` 是否为 `observe` 或 `pre_block`。
4. `all_groups/group_ids` 是否包含当前 API Key 分组。
5. `model_filter` 是否匹配客户端公开模型名。
6. 最后一条消息是否是可提取的用户内容，而不是 assistant/tool tail。
7. 输入是否只位于 system/developer/history 中。
8. `sample_rate` 是否为 0，或该稳定哈希是否未被抽样。
9. 是否配置至少一把 Key。
10. observe 的 `queue_length/dropped/processed` 是否正常。

### Base URL 返回 404

- 确认配置没有包含 `/v1`。
- 确认代理实际暴露 `/v1/moderations`。
- 使用管理测试查看 `last_http_status` 和 `last_error`。
- 注意 404 会使 Key 冻结 10 秒，并可能在只有一把 Key 时提前结束后续尝试。

### OpenAI 返回 400

- 检查模型名是否支持当前 input，尤其是图片请求是否使用 `omni-moderation-latest`。
- 检查代理是否接受 OpenAI multimodal input part。
- 检查远程图片 URL、data URL 和 payload 大小。
- 400 不冻结 Key，但会立即停止本次调用重试，因此不会自动换另一把 Key。

### 所有 Key 都 frozen

- `401/403`：检查 Key 有效性、项目权限和 Base URL，冻结 10 分钟。
- `429/529`：检查组织或项目限流，冻结 1 分钟。
- 其他状态：检查代理和上游健康，默认冻结 10 秒。
- 健康状态是单进程内存数据。重启会清除状态，但不应把重启作为修复无效凭据或限流的常规方式。

### 上游 `flagged=true`，sub2api 却放行

- 这是可能的，因为本地忽略上游 `flagged`。
- 检查 `category_scores` 是否存在。
- 检查类别名是否与 13 个固定名称完全一致。
- 检查分数是否达到本地阈值。
- 检查当前是否为 observe；observe API 命中仍放行当前请求。

### observe 仍拦截请求

- 检查是否开启 `pre_hash_check_enabled`。
- 查看响应 message 中是否附有 `hash: <64hex>`。
- 确认误报后使用 `DELETE /hashes` 删除该 hash。
- 若不希望 observe 发生任何同步拦截，关闭 pre-hash 检查。

### 请求已同步拦截，但日志中没有记录

- 检查 `dropped` 是否增长、队列是否满。
- 检查 worker、数据库和 Redis 日志。
- pre-block 的 API/关键词拦截，以及任意模式的历史哈希拦截，都会把记录任务异步执行；客户端拦截不保证日志、副作用已经持久化。

### 延迟明显增大

- `pre_block` 会把 Moderation API 延迟加入每个业务请求。
- 检查 `pre_block_avg_latency_ms` 和每 Key `avg_latency_ms`。
- 检查 timeout、retry count 和实际重试次数。
- 只有一把 Key 时冻结可能减少调用次数，但 transport timeout 状态为 0，不冻结，可能对同一 Key 连续重试。
- 可先切换 observe 或降低 retry count 排查，但应同步评估风险策略变化。

## 官方资料与源码入口

OpenAI 官方资料：

- API Reference：<https://platform.openai.com/docs/api-reference/moderations>
- Moderation Guide：<https://platform.openai.com/docs/guides/moderation>

主要源码：

- `backend/internal/service/content_moderation.go`：配置、默认值、模式、API 调用、阈值、Key 健康、日志和副作用。
- `backend/internal/service/content_moderation_input.go`：各协议文本和图片提取。
- `backend/internal/service/content_moderation_redact.go`：日志摘要脱敏。
- `backend/internal/securityaudit/coordinator.go`：旧版内容审计与 Prompt Audit 编排。
- `backend/internal/securityaudit/coordinator_legacy.go`：旧版内容审计适配器。
- `backend/internal/handler/security_audit_helper.go`：网关审计入口和请求元数据。
- `backend/internal/handler/security_audit_errors.go`：不同协议的拦截错误格式。
- `backend/internal/handler/admin/content_moderation_handler.go`：管理员风控接口 DTO 和 handler。
- `backend/internal/server/routes/admin.go`：管理员风控路由。
- `backend/internal/repository/content_moderation_repo.go`：日志查询、计数和清理。
- `backend/internal/repository/content_moderation_hash_cache.go`：Redis 风险哈希 Set。
- `frontend/src/api/admin/riskControl.ts`：前端管理 API 类型。
- `frontend/src/views/admin/RiskControlView.vue`：管理员风控页面。

关键测试：

- `backend/internal/service/content_moderation_test.go`
- `backend/internal/service/content_moderation_input_test.go`
- `backend/internal/service/content_moderation_runtime_cache_test.go`
- `backend/internal/service/content_moderation_cyber_test.go`
- `backend/internal/handler/security_audit_errors_test.go`
