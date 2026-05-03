# OpenCode 图片恢复工具输出化设计

## 背景

当前 OpenCode 图片生成兼容层会把 `image_generation_call` 改写成普通 assistant 文本，文本里包含 `sub2api-image://img_...` marker 和短期下载 URL。下一轮 OpenCode 请求进入 sub2api 后，`rehydrateOpenCodeGeneratedImageMarkers` 会扫描整个 `input`，找到 marker 后在 input 末尾追加 synthetic `role:"user"` message，内容可能包含 `input_image`，也可能是“图片不可用”的文本说明。

这个实现有两个实际问题：

1. 它会扫描历史压缩摘要、复述文本和普通 URL-like 字符串，导致旧 marker 被反复命中。
2. 它总是追加到 input 末尾，会抢占模型注意力，并且可能把 `-Sys` 请求末尾的 dummy `function_call_output` 挤开。

本设计修正图片恢复语义：图片恢复结果应作为工具输出出现，并放在 marker 所在位置附近，而不是作为最新用户消息追加到末尾。

## 目标

1. 不再把 rehydrated 图片或不可用说明追加成尾部 synthetic user message。
2. 图片恢复结果用 synthetic `function_call` / `function_call_output` 表达。
3. 图片恢复必须在调度 target group 计算之前完成；最终调度语义必须与最终上游请求体一致。
4. synthetic tool pair 插在 marker 所在 input item 后面；遇到 `-Sys` 请求时，`sub2api_sys_bootstrap` dummy pair 必须在图片恢复之后追加并保持尾部。
5. 新 marker 具备更强特异性，避免 Agent 复读普通 URL-like schema 后重复触发。
6. 旧 `sub2api-image://...` 和下载 URL 兼容保留，但触发条件收紧，历史压缩摘要不触发。
7. 图片不存在或过期时可以提醒，但提醒必须是原位置附近的 tool output，不能抢尾，也不能跨轮反复刷屏。
8. 仍然只对 OpenCode Responses client 启用，不影响其他客户端的标准 Responses 语义。

## 非目标

1. 不修改 OpenCode 源码。
2. 不改变图片短期保存策略；过期图片仍不可恢复。
3. 不引入数据库表记录图片恢复状态。
4. 不依赖 OpenAI `store:true`、`previous_response_id` 或上游 `item_reference`。
5. 不把不可用图片伪装成成功恢复的图片输入。

## 已确认事实

1. OpenCode 工具执行结果支持 `attachments`，图片读取类工具可把图片作为 data URL 附件返回。
2. OpenCode `MessageV2.toModelMessages` 会把工具附件转换成 tool result 的 media content；对 `@ai-sdk/openai` 这类 provider，它认为支持 media in tool results。
3. OpenCode 当前 Copilot Responses converter 仍把普通 `function_call_output.output` 写成 string。
4. OpenAI Responses API 文档显示 `function_call_output.output` 可以是 string，也可以是 input text / input image / input file 组成的数组。因此平台 schema 层面允许工具输出携带图片内容。
5. sub2api 当前 OAuth/Codex transform 对 input item 基本按 map 透传，不主动把 `function_call_output.output` 限制为 string。

## 请求形态设计

### 一、成功恢复图片

命中可恢复图片时，构造 synthetic tool pair：

```json
{
  "type": "function_call",
  "call_id": "call_sub2api_image_<id>",
  "name": "sub2api_generated_image",
  "arguments": "{}"
}
```

```json
{
  "type": "function_call_output",
  "call_id": "call_sub2api_image_<id>",
  "output": [
    {
      "type": "input_text",
      "text": "Generated image <id> restored by sub2api from the nearby image marker."
    },
    {
      "type": "input_image",
      "image_url": "data:image/png;base64,..."
    }
  ]
}
```

`call_id` 基础值必须稳定且由图片 id 派生。只有当该基础值与现有真实 `call_id` 冲突时，才允许追加由当前 input 冲突集合确定的稳定消歧后缀。`name` 使用新的专用 synthetic tool name，避免与真实客户端工具混淆。该 tool pair 只发给上游，不返回给 OpenCode 客户端执行。

### 二、图片不可用

图片过期、缺失、metadata 无效或超过恢复上限时，也使用同一类 synthetic tool pair，但 `output` 只包含文本：

```json
{
  "type": "function_call_output",
  "call_id": "call_sub2api_image_<id>",
  "output": [
    {
      "type": "input_text",
      "text": "Generated image <id> is no longer available. Use the nearby marker only as historical context."
    }
  ]
}
```

不可用说明允许出现，但只能出现在 marker 所在位置附近，不能作为最新用户消息追加到末尾。

### 三、上游兼容降级

工具输出模式支持三种内部策略：

1. `array`：优先模式，`function_call_output.output` 使用 content array，包含 `input_text` 和可选 `input_image`。
2. `string`：兼容降级模式，保留同样的 synthetic tool pair，但 `output` 是 string。
3. `auto`：默认策略，先按 `array` 构造；如果上游在未向客户端写出正文前返回明确的 output 类型不兼容 4xx，则同账号重写为 `string` 并只重试一次。

`string` 降级模式只能包含文本说明、marker 和可选短期下载 URL。它不得包含 `data:image`、原始 base64 或任何可被当作图片字节的长字符串。该模式下模型没有收到图片像素，只收到历史引用说明。

streaming 场景下，只有在还没有向下游发送正文级事件之前才允许 `auto` 重试；一旦已向下游开流，不能为了降级而污染输出语义。若无法安全重试，应返回原始上游错误并记录诊断字段。

## 调度时机与插入位置

图片 rehydrate 的执行时机必须前移到 `/v1/responses` 调度准备阶段：解析请求体后、`-Sys` dummy continuation 追加前、`GetRequestTargetGroup(reqBody)` 计算前。这样最终上游请求体和调度 target group 使用同一份已恢复的 `reqBody`。

执行顺序：

1. 解析请求体。
2. 如果是 `-Sys` 模型，先完成 input string 正规化和模型名 strip，但暂不追加 dummy pair。
3. 在 rehydrate 之前计算并缓存 `needsSysDummy := NeedsSysToolContinuation(reqBody)`；不能在 rehydrate 之后才调用现有 `NeedsSysToolContinuation`，因为图片 synthetic pair 可能已经让最后一项变成 `function_call_output`。
4. 对 OpenCode normal `/v1/responses` 请求执行图片 rehydrate。
5. 如果 `needsSysDummy` 为 true，再追加 `sub2api_sys_bootstrap` dummy pair。
6. 基于最终 `reqBody` 计算 target group、session hash 和上游请求体。

如果工程切分暂时无法把 rehydrate 前移，则不得进入实现；不能保留“调度前 active、转发前变成 tool output”的分裂形态。

扫描 input 时记录每个有效 marker 的源 item index。生成 tool pair 后插入到源 item 后面。

为了兼容已经存在 dummy pair 的异常输入，可以使用共享 helper `findSysDummyTail(input)`，但只在以下条件全部满足时视为 sys dummy：

```text
input[-2].type == "function_call"
input[-2].name == "sub2api_sys_bootstrap"
input[-2].call_id == "sys_dummy"
input[-1].type == "function_call_output"
input[-1].call_id == "sys_dummy"
```

真实用户 tool result 即使位于尾部，也不能被当作 sys dummy。扫描必须跳过 sys dummy pair、已有 synthetic image pair、reasoning item。如果 marker 的 source index 落在 dummy pair 内或 dummy pair 之后，视为无效 marker 并跳过，不允许把它倒插到 dummy 前造成时间线反转。

插入规则：

1. 基于原始 input 快照收集所有候选 marker，先过滤无效 source，再去重和限量。
2. 同一图片 id 多次命中时，只使用最后一个有效 marker 位置。
3. 每次请求最多处理最近 3 个不同图片 id。
4. 选出最近 3 个不同图片 id 后，按它们在原始 input 中的相对顺序升序插入，保证时间线稳定。
5. 每个 synthetic `function_call` 与对应 `function_call_output` 必须成对、相邻、`call_id` 完全一致。
6. `call_id` 基础值必须与现有 input 中真实 call_id 不冲突；如冲突，使用确定性消歧后缀。
7. 不把 synthetic tool pair 插入到 reasoning item 内，也不修改已有 assistant/user content。
8. 插入后必须能通过 `ValidateFunctionCallOutputContext` 等价校验：所有 synthetic output 都有对应 synthetic call。

## Marker 设计

### 一、新 marker

新生成的 assistant 文本不再只输出裸 `sub2api-image://img_...`。新增高特异哨兵：

```text
[[sub2api-generated-image:id=img_...]]
```

推荐输出文本：

```text
Generated image saved by sub2api.
Image reference: [[sub2api-generated-image:id=img_...]]
Temporary download URL: https://example.com/sub2api/generated-images/img_....png
```

这个 marker 不是 URL-like schema，也不是 HTML-like tag，Markdown renderer 不应把它当作隐藏标签处理。扫描器仍不能只凭字符串触发；它必须同时校验 source item 的 role、message id 或精确模板。

### 二、旧 marker 兼容

继续识别旧格式：

1. `sub2api-image://img_...`
2. `/sub2api/generated-images/img_....png`
3. `https://.../sub2api/generated-images/img_....png`

旧格式只能在更严格条件下触发：

1. 任何包含 `[Compressed conversation section]` 的 input item 整体跳过。
2. `/responses/compact` 路径不执行图片 rehydrate。
3. 包含 `What did we do so far?` 的 compaction prompt item 不执行图片 rehydrate。
4. legacy marker 默认只在 `role:"assistant"` 的 sub2api 生成图片 message 中触发。
5. sub2api 生成图片 message 的机械判定为：message `id` 以 `msg_sub2api_img_` 开头，或 `output_text` 精确匹配旧模板 `Generated image: sub2api-image://img_...` 加可选 `I'll download from URL:` 行。
6. `role:"user"` 中的 legacy marker 默认不触发，避免用户讨论 marker 字符串时误恢复。
7. `role:"user"` 若要显式恢复图片，必须使用新哨兵 `[[sub2api-generated-image:id=...]]`，且该 item 不属于 compaction/压缩摘要。

新哨兵也必须经过 source item 判定。Agent 后续 assistant 回复中若只是复读哨兵，不能触发，除非该 assistant message 符合 sub2api 生成图片 message 模板或 id 前缀。

这样能避免压缩摘要中记录的旧 marker 反复被当成最新上下文。

## 不可用图片去重

图片不可用说明不能跨轮反复刷屏。实现采用两层策略：

1. 对历史 assistant marker，图片缺失、过期或 metadata 无效时默认静默跳过，不生成不可用 tool output。
2. 只有当前用户显式使用新哨兵请求恢复图片时，才生成不可用 tool output。
3. 对显式不可用提醒维护进程内 TTL/LRU，key 为 image id 和 unavailable reason；TTL 内重复命中时静默跳过。
4. 如果未来要跨进程/重启持久化“已提醒”，必须另行设计；本轮不引入数据库表。

## 代码切分

主要修改集中在：

1. `openai_opencode_image_rewrite.go`
   - 新生成文本改用高特异 marker。
   - 服务端 continuation 可复用新的 synthetic tool output 构造逻辑。
2. `openai_opencode_image_rehydrate.go`
   - 扫描结果从 `[]string` 升级为带 id、source index、format、legacy/new 标记类型的结构。
   - `appendOpenCodeRehydratedMessages` 改为插入 synthetic tool pairs。
   - 增加 `-Sys` dummy tail 保护。
   - 增加不可用图片 TTL/LRU 去重。
3. `openai_opencode_image_ops_redaction.go`
   - 确保 tool output array 内的 `input_image.image_url` 也会被 redacted。
   - 确保新哨兵内的 image id、旧 marker、下载 URL 在 ops 展示中按既有策略脱敏。
4. `openai_gateway_service.go`
   - 调用点前移到调度准备阶段，确保 target group 与最终请求体一致。
   - rehydrate 后仍需保证 ops 捕获体使用 redacted 副本。

## 测试计划

新增或更新单元测试：

1. 新 marker 成功恢复为 synthetic `function_call` / `function_call_output`，且 output array 包含 `input_image`。
2. 当前用户显式新哨兵对应图片不存在时生成一次 tool output 文本，不追加 user message；历史 assistant marker 图片不存在时静默跳过。
3. marker 在 `[Compressed conversation section]` 内不触发。
4. `-Sys` 请求已有 dummy pair 时，图片 tool pair 插在 dummy pair 前，最后一项仍是 `function_call_output(sys_dummy)`。
5. 同一 id 出现多次只插入一次，并使用最后一个有效 marker 位置。
6. 旧 `sub2api-image://` 兼容路径不会因为普通复读文本无限重复注入。
7. ops redaction 覆盖 `function_call_output.output[].image_url`。
8. captured-style fixture：构造无敏大 payload，包含 `gpt-5.5-Sys`、`store:false`、`reasoning.encrypted_content`、压缩摘要旧 marker、新哨兵、多个历史 input，断言压缩摘要不触发、合法 marker 原位置插入、最终 tail 与 target group 一致。
9. transform 保真：含 `function_call_output.output` array 的请求经过 rehydrate、Codex OAuth transform 和最终 marshal 后，`output` 仍是 array，`call_id` 成对一致。
10. string fallback：降级后仍是 tool pair，插在原位置附近，且 string 不含 `data:image` 或原始 base64。
11. sys dummy 精确识别：真实用户 tool result 尾部不能被误判为 sys dummy；marker 落在 dummy pair 内或之后会被跳过；`gpt-5.5-Sys + marker at last user item` 会先缓存 `needsSysDummy`，最终尾部仍是 `function_call_output(sys_dummy)`。
12. 多 marker 排序：超过 3 个不同图片 id、同 id 重复、靠近尾部边界时，只处理最近 3 个不同 id，并保持原始相对顺序。
13. 新旧 marker 扫描矩阵：新哨兵触发；旧 marker 在压缩摘要、用户普通复述、Agent 普通复读中不触发；旧 marker 在 sub2api 生成 assistant image message 中兼容触发。
14. 不可用图片去重：历史 assistant marker 缺失时静默跳过；当前用户显式新哨兵缺失时生成一次 tool output；TTL 内第二次不重复生成。
15. non-OpenCode client 隔离：非 OpenCode 请求中出现 marker 不注入 synthetic pair，非 OpenCode image response 仍保留标准 `image_generation_call`。
16. ops 端到端 redaction：让上游 4xx 回显 `data:image/png;base64,...`，开启 `LogUpstreamErrorBody`，断言 upstream request body、ops error events、request detail、error body/detail 和运行时日志输出均不包含 data URL 或原始 base64。
17. `call_id` 冲突消歧：当现有 input 已占用 `call_sub2api_image_<id>` 时，synthetic pair 使用稳定后缀且 call/output 仍成对一致。

验证命令优先使用现有 Go 测试子集：

```powershell
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test ./internal/service -run "OpenCode|GeneratedImage|Rehydrate|RedactOpenCode|ToolContinuation|OpsServiceRecordErrorBatch|PrepareOpsRequestBody" -count=1
```

如改动影响 handler 下载路径，再补充：

```powershell
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test ./internal/handler -run "OpsErrorLogger|OpenAIGateway|GeneratedImage" -count=1
```

如改动触及 request detail / ops repository，再补充：

```powershell
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test ./internal/repository -run "Ops(ErrorLog|RequestDetails|RequestType)" -count=1
```

真实上游 smoke 只能在本地单元测试通过后执行一次，必须使用最小无敏 payload，手工单次触发，不得脚本循环或批量压测。失败后立即停止，先检查本地日志和 ops redaction。

## 风险与缓解

1. 风险：ChatGPT internal Codex endpoint 可能暂不接受 `function_call_output.output` array。
   缓解：实现 `array|string|auto` 模式、string 安全降级路径，并用一次低频 smoke 验证；不做批量线上请求。
2. 风险：旧 marker 过严导致用户显式引用旧图片时无法恢复。
   缓解：保留新哨兵优先，旧格式只在机械判定为 sub2api 生成图片 message 时恢复；用户显式恢复使用新哨兵。
3. 风险：tool output array 内图片 data URL 进入日志。
   缓解：扩展 redaction，测试锁定。
4. 风险：插入位置错误破坏 `-Sys` exhausted 路由语义。
   缓解：测试最后一项仍是 `function_call_output(sys_dummy)`，并保持调度前后语义一致。
