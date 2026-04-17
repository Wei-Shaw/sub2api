# OpenAI OAuth 非流式 `/responses` Web Search 语义补齐设计

## 背景

当前问题不是“非流式 `/responses` 无法被 AI SDK 解析”了，而是“能解析，但语义不完整”。

已确认的现状有三层：

1. 非流式 JSON 已补齐最小 schema 兼容字段，`generateText` 不再因缺少 `output[0].id`、`annotations` 等字段报 `Invalid JSON response`。
2. 对 `OpenAI OAuth + 非 compact + 非流式 /responses` 链路，本地实际上会向上游请求 SSE，再在网关侧折叠成最终 JSON：
   - `buildUpstreamRequest()` 在 OAuth 且非 `/responses/compact` 时固定设置 `Accept: text/event-stream`
   - 之后由 `handleNonStreamingResponse -> handleSSEToJSON` 输出最终 JSON
3. 当前折叠逻辑只在 terminal `output` 为空时才从 SSE 重建；如果 terminal response 里只剩最终 `message`，但前序 SSE 已经出现过 `web_search_call`，这些结构化 item 不会被补回。

这会直接导致：

- `result.text` 正常
- 但 `result.toolResults` / `result.sources` 为空
- 同时还可能出现 `tool_usage.web_search.num_requests = 0` 与最终语义不一致的问题

## 目标

在不改动 OpenCode 本地临时兼容语义的前提下，补齐 `OpenAI OAuth + 非 compact + 非流式 /responses` 的 web search 非流式语义，使最终 JSON 更接近官方 provider-defined web search 可消费形态。

本次目标包括：

1. `generateText` 继续成功
2. `result.text` 保持正常
3. 当前序 SSE 已出现 `web_search_call` 时，最终非流式 JSON 不再退化成 message-only `output`
4. 当上游返回的 `web_search_call.action.sources` 非空时，sources 在最终 JSON 中保真
5. `tool_usage.web_search` 与最终响应语义一致，不再自相矛盾

## 非目标

本次明确不做以下事情：

1. 不修改 `OpenCode` 专用的 `web_search_call` 过滤逻辑
2. 不把修复外推到 `APIKey / 平台直连` 普通 JSON 路径
3. 不处理 `/responses/compact` 链路
4. 不做纯文本 URL/引用抽取，不从最终文本里猜测 sources
5. 不重写整套 Responses SSE 协议实现，只做当前问题所需的最小补齐

## 设计边界

### 一、只修 OAuth 非 compact 的主 SSE 折叠链

本次入口明确限定在：

- `internal/service/openai_gateway_service.go`
  - `buildUpstreamRequest()`
  - `handleNonStreamingResponse()`
  - `handleSSEToJSON()`

原因是这条主链已经确认会在 `AccountTypeOAuth` 且非 `/responses/compact` 时固定下发 `Accept: text/event-stream`，随后由 `handleNonStreamingResponse()` 折叠为最终 JSON；当前语义丢失就发生在这里。

实现 gating 也要写死：

- 即使最终 helper 落在共享的 `handleSSEToJSON()` 内部，也只能在 `account.Type == AccountTypeOAuth` 且 `!isOpenAIResponsesCompactPath(c)` 的非流式 `/responses` 主链上执行“缺项补齐 + tool_usage 修正”
- `APIKey` 的 SSE fallback 继续保持现状，不被本次逻辑带入

明确排除：

- `buildUpstreamRequestOpenAIPassthrough()`
- `handlePassthroughSSEToJSON()`

passthrough 是独立分支，Accept 语义与主链不同；本次 spec 不把它混入修复范围。

### 二、不改变现有 OpenCode 本地兼容

`sanitizeOpenCodeResponsesOutput()` 及相关流式过滤逻辑保持不动。本次补齐只针对通用最终 JSON 语义，不触碰你明确要求保留的本地临时兼容。

这条边界再收紧一层：

- OpenCode 客户端不仅继续过滤最终 `output` 里的 `web_search_call`
- 也跳过本次 `tool_usage.web_search.num_requests` 修正

原因是当前 OpenCode 兼容层只过滤 `output`，若在 OpenCode 路径上单独修正 `num_requests`，会产生“最终 output 看不到 `web_search_call`，但 `tool_usage` 却显示发生过 web search”的新矛盾。

## 核心设计

### 一、从“空 output 才重建”改为“缺项就补齐”

当前逻辑：

- 只有 terminal `output == []` 时，才会调用 `reconstructResponseOutputFromSSE()` 从 delta 重建 output

本次改为：

- 在 `internal/service/openai_gateway_service.go` 新增一个 `/responses` 主链专用的 SSE output merge helper
- 该 helper 负责把 terminal response 与整段 SSE 观测结果合并成最终 output
- 它不再只处理“完全空 output”
- 当 terminal response 已有 `message`，但 SSE 中出现过 terminal 缺失的结构化 item 时，也要补齐最终 `output`

这里补一条硬约束：

- 不改 `BufferedResponseAccumulator.SupplementResponseOutput()` 现有的 empty-output-only 语义
- 如需复用 `apicompat` 的 accumulator，只扩它产出 indexed SSE 视图的能力，不把全局补齐语义直接改写到 `SupplementResponseOutput()` 上，避免误伤 chat completions / anthropic 兼容链

具体判定规则：

1. 若 terminal `output` 为空，行为与当前类似，但新的 helper 会产出更完整的 output
2. 若 terminal `output` 非空，但缺少 SSE 中已经完成的 `web_search_call`，则使用 SSE 观测结果补齐最终 `output`
3. `reasoning` / `function_call` 若也存在同类缺项，可一并沿同一机制补齐，避免再次留坑

### 二、以 SSE 结构化 item 为准，不做文本猜测

补齐来源必须是 SSE 中真实出现过的结构化事件：

- `response.output_item.added`
- `response.output_item.done`
- `response.output_text.delta`
- `response.reasoning_summary_text.delta`
- `response.function_call_arguments.delta`

其中：

1. `web_search_call` 以 `response.output_item.done` 为准，因为只有这里能拿到完整 `action`
2. 最终 assistant 文本仍优先来自终态 message 或已知 delta 聚合结果
3. 不允许从 `output_text` 里的 Markdown 链接、URL 文本反推 `sources`

同时补一条实现约束：

- terminal response 的 merge/correction 必须在 raw JSON 或 generic map 上定点 patch，不允许把整份 `response.completed.response` 先 typed-unmarshal 成 `ResponsesResponse` 再 marshal 回去
- 原因是当前 `ResponsesResponse` 不承载 `tool_usage` 等未知顶层字段；typed round-trip 会静默吃掉这些字段

### 三、扩充 SSE accumulator，保留 output 顺序和完整 item

当前 `BufferedResponseAccumulator` 只聚合：

- `message` 文本
- `reasoning` summary delta
- `function_call` 参数 delta

它无法保留：

- `web_search_call`
- `response.output_item.done` 中已有的完整 item
- `action.sources`

本次需要把这层扩成“按 `output_index` 记录完整 output 视图”的聚合器，至少满足：

1. 能记录 `response.output_item.added/done` 的 item
2. 能在 `done` 到来时用完整 item 覆盖占位项
3. 能在 message/reasoning/function_call 仅有 delta 时继续从 delta 合成
4. 构建最终 SSE canonical 视图时保留原始 `output_index` 顺序，而不是简单按“reasoning -> message -> function_call”硬编码重排

这样生成的 SSE 视图才能作为 terminal JSON 的可靠补齐来源。

进一步把 merge 规则写死如下：

1. 先按 `output_index` 构建 SSE canonical 视图
2. merge terminal output 时，优先按稳定标识去重；`web_search_call` / `message` / `reasoning` 优先用 item `id`，`function_call` 优先用 `call_id`
3. 只有缺稳定标识时，才回退到 canonical 槽位上的 `output_index + type`
4. terminal array index 不能直接当作 `output_index` 使用；对缺稳定标识的 terminal item，只有在能唯一匹配 canonical 槽位时才合并，否则走保守策略，避免错配和重复
5. `web_search_call` 的权威来源是 `response.output_item.done`
6. 如果 terminal item 已存在但字段更弱（例如缺 `action.sources`），用 SSE full item 覆盖或深合并，不能仅在“完全缺项”时 append
7. 命中同一 dedupe key 时，最终只能产出一个 item；`web_search_call` / `function_call` / `reasoning` 以 SSE full item 为 base，terminal 仅补 SSE 缺失字段
8. 数组字段如 `action.sources`、`content`、`summary` 一律整段替换，不做拼接
9. assistant `message` 若 terminal message 与 delta 聚合 message 不一致，以 terminal message 为准，但最终 output 里同一 `output_index` 只允许保留一条 message
10. 最终 output 顺序以 canonical `output_index` 为准

### 四、保留 `web_search_call.action.sources`

当前 `internal/pkg/apicompat/types.go` 里的：

- `WebSearchAction` 仅定义 `type/query`

这意味着只要走 typed unmarshal，`action.sources` 就会被静默吃掉。

本次必须扩充该类型，让它能够完整承载 `sources`。这里补一条更硬的约束：

- 必须保留 `WebSearchAction.Type` / `WebSearchAction.Query` 的现有 typed 访问方式
- 只新增一个可原样透传 `sources` 的成员
- 不把 `Action` 整体改成 `map[string]any`、`[]map[string]any` 或完全 opaque blob

实现优先级上，`sources` 应使用 raw-preserving 的字段表达（例如 `json.RawMessage` 级别），以便既保真又不污染现有调用点。

设计要求：

1. SSE `web_search_call` done item 里若带 `action.sources`，最终 JSON 必须原样保留该字段
2. 后续 `normalizeResponsesJSONForAISDK()` 只做补 schema，不得覆盖或裁剪这些 sources
3. `sources` 只能来自 SSE item 的 `action.sources`，不能来自 message annotations 或文本 URL 推断
4. 需要额外补一条 `apicompat` 级单测：对包含 `action.sources` 的 `response.output_item.done` 或 terminal response 做 typed unmarshal / round-trip，断言 `Action.Type`、`Action.Query` 仍可直接访问，`Action.Sources` 原样保留

### 五、同步校正 `tool_usage.web_search.num_requests`

当前仓库没有任何针对 `tool_usage.web_search` 的修正逻辑。

本次在补齐最终 `output` 后，同步修正：

- `tool_usage.web_search.num_requests`

最小一致性原则：

1. 如果最终 `output` 中已有 `web_search_call`，`num_requests` 不得继续为 `0`
2. 计数以下最终去重后的 merged output 中 `web_search_call` 数量为下限，不按原始 SSE event 条数或 naive append 后数组长度计算
3. 只修 `tool_usage.web_search.num_requests` 这一处矛盾字段，不重算或重写整个 `tool_usage` 结构
4. 不伪造额外结构；仅在现有 top-level `tool_usage` 上做 raw JSON 定点修正，使其与最终 output 不矛盾
5. 测试中要放入 `tool_usage` 的 sibling 字段与 `tool_usage.web_search` 内部未知字段，验收只允许 `num_requests` 变化，其余字段必须保持不变

## 文件落点

预计修改以下文件：

1. `internal/service/openai_gateway_service.go`
   - 新增 `/responses` 主链专用 SSE output merge helper
   - 在 `handleSSEToJSON()` 中接入“缺项补齐”逻辑
   - 补一层 `tool_usage.web_search.num_requests` 校正
2. `internal/pkg/apicompat/types.go`
   - 扩充 `WebSearchAction`，确保 `sources` 不在 typed unmarshal 时丢失
3. `internal/pkg/apicompat/responses_to_chatcompletions.go`
   - 如需要，扩充 accumulator 以保留 `output_index`、`output_item.done` 与 `web_search_call`，但不改变 `SupplementResponseOutput()` 的既有 empty-output-only 语义
4. `internal/service/openai_gateway_service_test.go`
   - 增加 OAuth 非 compact SSE→JSON 语义补齐测试
5. `internal/pkg/apicompat/chatcompletions_responses_test.go`
   - 若 accumulator 的 indexed 视图能力发生变化，补对应单测，锁定 `output_index` 顺序、done item 覆盖占位项，以及旧 empty-output 路径不回归
6. `internal/pkg/apicompat/*` 对应测试
   - 增加 `WebSearchAction` typed unmarshal / round-trip 的 sources 保真测试

## 测试设计

### 一、先写失败用例

先补一个失败测试，精确覆盖当前回归：

1. 构造 OAuth 非 compact SSE body
2. 前序事件包含：
   - `response.output_item.added(web_search_call)`
   - `response.output_item.done(web_search_call)`，带 `action.query`，必要时带 `action.sources`
   - 最终 assistant `message` 对应 delta / done
3. terminal `response.completed.response.output` 只保留最终 `message`
4. terminal `response.completed.response.tool_usage.web_search.num_requests = 0`
5. 断言当前实现失败，修复后应通过
6. 同一组 fixture 里还要带 `tool_usage` 的 sibling 字段和 `tool_usage.web_search` 内部未知字段，后续验证 raw patch 没有误删它们

### 二、关键断言

修复后的断言包括：

1. 最终 JSON `output` 中包含 `web_search_call`
2. 最终 JSON `output` 中保留 `message`
3. 若 SSE done item 带 `action.sources`，最终 JSON 中仍可见这些 sources，并对 `url` / `title` / 代表字段做精确 JSON 断言
4. 在“恰好一个 `web_search_call`”的用例里，`tool_usage.web_search.num_requests` 必须从 `0` 被纠正为 `1`
5. 最终 `output` 顺序按 `output_index` 保持，例如 `web_search_call(0) -> reasoning(1) -> message(2)`
6. 仓库内验收映射明确为：
   - `output[*].type == "web_search_call"` 作为上层 `result.toolResults` 不再为空的代理证明
   - `output[*].action.sources` 保留作为上层 `result.sources` 不再为空的代理证明
7. 同一条修复路径下，最终 `message` 仍经过 `normalizeResponsesJSONForAISDK()` 的最小 schema 补齐，至少包含 `message.id` 与 `output_text.annotations`，作为 `generateText` 继续成功的仓库内代理证明
8. `tool_usage` 的 sibling 字段与 `tool_usage.web_search` 内部未知字段保持不变，只允许 `num_requests` 被修正
9. 额外补一个“terminal 已有同 `id` 的 `web_search_call` 但字段较弱”的用例，断言最终只保留一个 `web_search_call`，字段被补强而不是重复插入，且 `tool_usage.web_search.num_requests == 1`

### 三、回归保护

以下现有语义必须继续保住：

1. OpenCode 过滤相关测试继续通过
2. `/responses/compact` 的 JSON Accept 路径测试继续通过
3. 纯 JSON 非流式归一化测试继续通过
4. 空 output 重建既有测试继续通过
5. 新增一个 OpenCode UA 版本的“message-only terminal + earlier web_search_call”用例，明确断言：
   - 普通客户端最终 body 保留 `web_search_call`
   - OpenCode 客户端最终 body 仍不含 `web_search_call`
   - `message` / `reasoning` 仍保持可见
   - `tool_usage.web_search.num_requests` 保持原值，不做本次修正
6. 新增一个 `APIKey` SSE fallback 的负向回归用例，复用 OAuth 主失败用例同款 SSE fixture，但把账户改为 `APIKey`，明确断言最终仍保持当前非修复行为：不补回 `web_search_call`，也不改写 `tool_usage.web_search.num_requests`

## 验收标准

以用户给定验收口径为准：

1. 同一份官方最小复现脚本下，`generateText` 继续成功
2. `result.text` 正常
3. `result.toolResults` 不再为空
4. `result.sources` 不再为空（前提是上游返回的 `web_search_call.action.sources` 非空）
5. `tool_usage.web_search` 与最终响应语义一致

## 风险与约束

1. 这次不能把“OAuth 非 compact SSE 折叠问题”误当成所有非流式 `/responses` 的统一根因
2. `sources` 必须做结构保真，不能为了最小改动继续沿用会吃字段的旧类型
3. output 补齐应优先复用现有 accumulator / helper，不要在 handler 内散落第二套 ad-hoc 解析逻辑
