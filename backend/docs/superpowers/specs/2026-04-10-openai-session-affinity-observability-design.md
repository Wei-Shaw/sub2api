# OpenAI Session Affinity 与 Sticky 可观测性设计

## 背景

当前线上缓存命中率偏低，而现有分析已经证明两个事实：

1. OpenCode 官方源码会稳定发送 `x-session-affinity`，并在父子关系存在时发送 `x-parent-session-id`。
2. 当前网关的 OpenAI sticky/session hash 生成逻辑并不消费 `x-session-affinity`，只依赖：
   - `session_id`
   - `conversation_id`
   - `prompt_cache_key`
   - content-based fallback

这意味着 stock OpenCode 明明提供了稳定会话信号，但网关没有用它做 sticky 归桶，导致：

- 同一 OpenCode 会话请求可能无法稳定落到同一账号
- 上游 prompt cache 难以获得应有的命中率
- 当前观测链也无法回答“命中率低到底是信号缺失、sticky 未命中，还是命中了但仍无缓存收益”

## 目标

1. 将 `x-session-affinity` 正式纳入 OpenAI sticky/session hash 生成逻辑。
2. 在 **OpenAI HTTP 主链**（`/v1/responses`、`/v1/chat/completions`）补足 session/sticky 相关观测，至少能回答：
   - 本次请求最终采用了哪种 session 来源
   - 本次请求是否真正进入了 sticky 判定
   - 如果进入 sticky 判定，结果是命中还是 miss，以及 miss 原因
   - 是否相对原 sticky 绑定账号发生了账号切换
3. 在运维面板提供聚合指标，支持从总览快速判断 sticky/缓存问题主要出在哪一层。

## 非目标

- 不改 OpenCode 客户端行为
- 不引入 prefix-hash 相似度兜底
- 不调整 sticky TTL
- 不改 `active/exhausted/-Sys` guardrail
- 不改当前 failover / target-group 语义
- 不把其他 OpenAI 兼容入口（如更窄的 messages/WS 分支）混进第一阶段成功标准

## 设计一：会话信号优先级

OpenAI sticky/session hash 输入信号优先级调整为：

1. `session_id`
2. `conversation_id`
3. `x-session-affinity`
4. `prompt_cache_key`
5. content-based fallback

说明：

- `session_id` / `conversation_id` 仍保持最高优先级，兼容现有显式调用方。
- `x-session-affinity` 是 stock OpenCode 已稳定提供的真实会话信号，应高于 `prompt_cache_key`。
- `prompt_cache_key` 继续保留，但只作为更弱的会话信号。
- content-based fallback 继续保底，不承担主路径职责。

这一优先级不仅适用于 `GenerateSessionHash`，也适用于所有会把“会话信号”继续传给上游 cache/session 语义的路径：

- `/v1/chat/completions` 兼容路径中的 `ExtractSessionID` / compat `prompt_cache_key` 注入链
- `/v1/responses` 直连上游时，OAuth 请求里最终写给上游的 `session_id` / `conversation_id` / `prompt_cache_key` 选择逻辑

否则仍会出现“本地 sticky 已改，但 upstream cache key 继续走旧信号”的脱节，导致 cache hit 验证失真。

## 设计二：`x-parent-session-id` 处理策略

- `x-parent-session-id` 本轮**只进入观测，不参与 sticky hash**。

原因：

- 父会话不等于必须共用 sticky 的会话
- 直接将父会话纳入 hash 容易造成误粘性
- 先把它做成观测字段，更利于理解多子代理并发行为

仅有 `sticky_parent_session_present` 不足以支持后续分析，因此建议同时补一个不泄露原值的稳定关联字段：

- `sticky_parent_session_key`
  - 含义：对 `x-parent-session-id` 做稳定 hash/归一化后的观测值
  - 仅用于 request details / 聚合分析，不参与 sticky hash

## 设计三：第一阶段覆盖范围

第一阶段只覆盖 **OpenAI HTTP 主链**：

- `/v1/responses`
- `/v1/chat/completions`

理由：

- 这两条链已经有稳定的 `sessionHash` 生成和主路径选号逻辑；
- 当前 request details / usage / ops 聚合也主要围绕这两条 HTTP 链更容易闭环；
- 如果第一轮就要求所有 OpenAI 兼容入口一起覆盖，会让观测口径再次失真。

这意味着：

- 第一阶段运维面板与请求详情中的 sticky/session 观测，只承诺反映这两条 HTTP 主链的数据；
- 其他 OpenAI 兼容入口是否补齐同样观测，放到后续批次。

## 设计四：请求级观测字段

本轮不再用含糊的 `sticky_hit + sticky_miss_reason` 组合，而改成单一的 sticky 判定结果枚举，避免 `previous_response_id` 等绕过路径把统计口径弄乱。

这里的 `sticky_eval_result` 明确定义为：**该请求在首次进入选号主链时，对 sticky 绑定做出的第一次判定结果摘要**。后续同一请求内如果因为 upstream failover 再次选号，不覆盖这个字段；后续重选行为另由现有 failover 计数与账号切换观测解释。

建议新增字段：

- `sticky_session_source`
  - 值域：`session_id` / `conversation_id` / `x_session_affinity` / `prompt_cache_key` / `content_fallback` / `none`
- `sticky_session_hash_present`
- `sticky_eval_result`
  - 值域建议固定为：
    - `hit`
    - `miss_no_binding`
    - `miss_binding_invalid`
    - `miss_binding_restricted`
    - `miss_binding_excluded`
    - `bypassed_previous_response_id`
    - `no_session_signal`
- `sticky_selected_account_changed`
  - 定义：存在 sticky 绑定账号，且最终选中的账号与该绑定账号不同
- `sticky_parent_session_present`
- `sticky_parent_session_key`
  - 含义：对 `x-parent-session-id` 做稳定 hash/归一化后的观测值
  - 仅用于 request details / 聚合分析，不参与 sticky hash

这些字段需要进入两条观测链：

1. **success 链**：成功请求写入 `usage_logs` / request details
2. **error 链**：失败请求写入 `ops_error_logs` / request details

原因：

- sticky miss、绑定失效、被 `previous_response_id` 绕过、以及“命中了但最终仍失败”的很多关键信息，都可能发生在错误路径；
- 只改 success write path，会让 request details 对最关键的 sticky 问题失明。

### 统计口径约束

- `sticky_hit_rate` 的分母只统计真正进入 sticky 判定的请求：
  - `hit`
  - `miss_*`
- `bypassed_previous_response_id` 与 `no_session_signal` 单独统计，**不计入 hit rate 分母**。

## 设计五：运维聚合指标

运维面板新增一组 sticky/cache 相关聚合指标，目标是让总览层直接回答：

1. 当前请求主要依赖哪种 session 信号
2. sticky 命中率有多高
3. sticky miss 的主要原因是什么
4. 在命中同一会话信号时，账号切换率是否仍然偏高

建议的第一批聚合指标：

- `session_source_count_by_type`
- `sticky_hit_rate`
- `sticky_eval_result_count`
- `sticky_account_switch_rate`

### 聚合口径

本轮 sticky 聚合明确以 **success + error 联合口径** 为主，也就是从 `usage_logs + ops_error_logs` 的统一 request details 视图统计，而不是沿用现有 `openai-routing` 那种只看成功请求的 `usage_logs` 聚合范式。

原因：

- `miss_binding_invalid`
- `miss_binding_restricted`
- `bypassed_previous_response_id`
- 以及“命中了 sticky 但最终仍失败”的关键信息

很多都发生在错误路径；若只看成功请求，`sticky_hit_rate` 和 `sticky_account_switch_rate` 会系统性偏乐观。

因此本轮设计约束是：

- `sticky_hit_rate`
- `sticky_eval_result_count`
- `sticky_account_switch_rate`

默认都采用 `all_evaluated`（success + error 联合）口径。

其中：

- `sticky_hit_rate` / `sticky_eval_result_count` 统计的是“首次 sticky 判定结果”
- `sticky_account_switch_rate` 用于补充解释“虽然首次 sticky 命中，但后续因 failover/重选导致最终账号变化”的情况

不直接在这一步引入“缓存命中归因”复杂推断，而是先把 sticky 层看清。

## 设计六：实施边界

本轮只做两类改动：

1. sticky 输入信号修正：支持 `x-session-affinity`
2. 观测补强：请求级 + 聚合级

明确不做：

- prefix-hash 相似度兜底
- sticky TTL 调整
- target-group / failover 规则改动

这样可以把回归面严格控制在：

- session hash 生成顺序
- success/error 双链的观测字段
- ops 聚合与展示

## 验证方式

### 代码验证

- 单测覆盖 `GenerateSessionHash` 新优先级
- 单测覆盖 `x-session-affinity` 命中 sticky 的路径
- 单测覆盖 success/error 双链的观测字段写入与聚合

### 线上验证

针对同一 API key 下的多 OpenCode 会话 / 多子代理并发场景，重点验证：

- `x-session-affinity` 是否被识别成 session 来源
- sticky 命中率是否提升
- 同一 session 来源下的账号切换率是否下降
- cache hit 是否随之改善

## 风险与后续

如果这一轮落地后仍发现：

- 已识别 `x-session-affinity`
- sticky 也命中
- 但缓存命中率依然偏低

那下一步才有必要进入更复杂的阶段：

- 分析 OpenCode 子代理调度是否天然破坏前缀稳定性
- 评估 prefix-hash 相似度兜底是否值得引入

在此之前，不建议直接上 prefix-hash。

## 结论

这轮最现实、最小、最贴近根因的方案，不是继续猜测客户端行为，也不是直接上复杂的前缀相似度算法，而是：

- 先消费 OpenCode 已经提供的 `x-session-affinity`
- 再把 sticky/session 观测做完整

只有这样，后续关于缓存命中率的判断才会建立在可验证证据上，而不是近似推断。
