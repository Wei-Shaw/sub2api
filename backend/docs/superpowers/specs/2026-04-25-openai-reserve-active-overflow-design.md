# OpenAI Reserve 作为 Active 基础身份的语义修正设计

## 背景

当前线上已经验证出一个新的产品语义问题：

- `GPT-5.4-Sys` 与 `GPT-5.5-Sys` 在当前版本下都已经恢复可用。
- 但普通 `gpt-5.5` 的 `active` 路径仍可能返回：
  - `No available accounts in target group (active)`

进一步线上排查证明，这不是因为模型 ID 再次被错误改写，也不是因为当前缓存未刷新，而是当前实现沿用了旧语义：

- `reserve` 被当成仅供 `exhausted` overflow 消费的子池。
- 当某模型子集里只剩 `exhausted + reserve`，没有普通 `active` 候选时，`active/any` 会把 `reserve` 排掉。

用户已经明确修正产品语义：

- `reserve` **首先是 active 身份的一部分**。
- `exhausted` overflow 只是 reserve 的额外使用方式，不是它的唯一消费方式。

## 目标

1. 把 `reserve` 重新定义为“带 reserve 身份的 active 账号”。
2. 让 `active/any` 请求也能正常消费 reserve 账号。
3. 保留 `exhausted` 路径现有 overflow 规则，包括：
   - `exhausted=0 => 占用率100%`
   - exhausted-class 仍优先消费 reserve
4. 统一 `routing_selected_group` / sticky / `previous_response` 的 reserve 观测与绑定语义。
5. 不放开 projection miss / unknown-model 的 fail-closed 边界。

## 非目标

1. 不重做 reserve 的投影生成算法。
2. 不重做 60% overflow 阈值规则。
3. 不把 reserve 升格成新的 `target_group`。
4. 不修改 `routing_target_group` 的外部语义。
5. 不借这次设计放开 unknown-model 或 projection miss 的 live fallback。

## 核心语义

### 1. reserve 的新定义

`reserve` 是账号的**显式身份标签**，并且：

1. 它首先属于 active-class 候选的一部分。
2. exhausted-class 在 overflow 时可以优先消费它。
3. 因此 reserve 不再表示“只有 exhausted 才能命中”的专属子池。

换句话说：

- 一个账号如果当前身份是 reserve，`active/any` 可以正常命中它。
- exhausted-class 也仍然可以在 overflow 条件下命中它。

### 2. target_group 与 selected_group

请求语义与账号身份继续分层：

- `routing_target_group`：始终记录请求本身的目标语义
  - `active`
  - `exhausted`
  - `any`
- `routing_selected_group`：记录最终命中的账号身份
  - `active`
  - `exhausted`
  - `reserve`

因此：

1. `active/any` 命中 reserve 时，`selected_group` 应写 `reserve`。
2. exhausted 命中 reserve 时，`selected_group` 也继续写 `reserve`。
3. `target_group` 不会因为命中 reserve 而被改写。

### 3. exhausted 现有规则保留

这次设计不改变 exhausted-class 的消费优先级：

1. exhausted-class 仍按当前 overflow 规则先看 exhausted 基础池。
2. `exhausted=0 => 占用率100%` 规则继续成立。
3. 在 overflow 成立时，reserve 仍是 exhausted-class 的第一优先承接对象。

区别仅在于：

- reserve 现在不再被 `active/any` 额外排斥。

## 请求路径设计

### 1. active / any 路径

请求消费逻辑调整为：

1. 若账号当前身份是 `reserve`，则 `active/any` 可正常选中它。
2. 不再把 overlay reserve 当作 `active/any` 的异常 guardrail 去拒绝。
3. 命中后观测统一写：
   - `routing_selected_group=reserve`

### 2. exhausted 路径

保持现有逻辑：

1. exhausted-class 仍使用 exhausted base + reserve overflow 的消费关系。
2. `exhausted=0 => 100%` 仍触发 reserve overflow。
3. 这条路径的变化仅限于与 active/any 保持一致的 reserve 身份表达，不改变 overflow 触发条件。

### 3. projection miss / unknown-model

这次语义修正不改变 fail-closed 边界：

1. projection miss 仍按现有受控 fallback/fail-closed 语义处理。
2. unknown-model 仍必须保守排除，直到目录/能力源显式更新。
3. 不允许因为“reserve 现在也属于 active”而重新引入 live reserve 推导。

## sticky / previous_response / continuation

### 1. 绑定语义

reserve 既然是 active 基础身份的一部分，那么 binding 语义也必须放宽：

1. `active/any` 请求命中的 reserve，可以继续按 `reserve` 身份绑定。
2. exhausted 请求命中的 reserve，也继续按 `reserve` 身份绑定。
3. 后续命中校验的核心从：
   - “reserve 只允许 exhausted 命中”
   转为：
   - “当前 binding 的 reserve 身份是否仍与账号当前身份一致”

### 2. 版本一致性

现有 `projection_version` / `projection_model_key` / `projection_built_at` 继续保留，用来保证：

1. binding 与当前 projection 版本一致。
2. stale binding 在身份变化后会失效或重绑。

### 3. continuation / previous_response

当 continuation / `previous_response_id` 命中 reserve 绑定时：

1. 不再因为请求是 `active/any` 就自动拒绝该 binding。
2. 只要 binding 对应账号当前仍是 reserve，且版本/模型键一致，就允许继续命中。

## 观测与解释层

需要保持以下可解释性：

1. 请求语义通过 `target_group` 体现。
2. 账号实际身份通过 `selected_group` 体现。
3. active/any 命中 reserve 时，日志/请求明细/ops 统计必须能明确看出：
   - 请求是 `active/any`
   - 实际命中的是 `reserve`

这能避免再次出现“是不是缓存没刷新”与“是不是语义本来就不允许”的混淆。

## 测试要求

### 必测行为

1. `gpt-5.5` 这类子集里若账号 `23` 是 reserve，则 `active/any` 能正常命中它，不再 `503`。
2. `GPT-5.5-Sys` 仍可命中 reserve，并保持 exhausted-class 语义不变。
3. active/any 命中 reserve 时：
   - `routing_selected_group=reserve`
   - affinity / sticky / `previous_response` 绑定也写 `reserve`
4. sticky / `previous_response` 不再因“reserve 只允许 exhausted”而误拒绝 active/any 命中。

### 必保留行为

1. `routing_target_group` 保持原始请求语义。
2. `exhausted=0 => 100%` 保留。
3. projection miss / unknown-model 仍 fail-closed。
4. `GPT-5.4-Sys` / `GPT-5.5-Sys` 当前已经修好的链路不回归。

### 回归矩阵

至少要覆盖：

1. `active + reserve` 命中。
2. `any + reserve` 命中。
3. `exhausted + reserve overflow` 命中。
4. `previous_response + active + reserve` 继续命中。
5. `sticky + any + reserve` 继续命中。
6. projection miss 下，active/any 对 reserve 的放宽**不能**连带放宽 unknown-model / cache-not-ready 的 fail-closed 边界。

## 风险与缓解

### 风险 1：把 reserve 误当成新 target group

缓解：只改消费语义，不新增 `target_group=reserve`。

### 风险 2：放宽 active/any 后打穿 projection 边界

缓解：明确区分“reserve 可被 active/any 消费”和“projection miss 仍 fail-closed”这两件事；测试必须同时锁住两者。

### 风险 3：sticky / previous_response 行为回归

缓解：把 reserve binding 的 active/any 场景列为显式回归测试，而不是只测 exhausted。

## 验收标准

满足以下条件即视为本次语义修正完成：

1. reserve 被明确视为 active 基础身份的一部分。
2. `active/any` 可以正常消费 reserve 账号。
3. exhausted 仍按现有 overflow 规则消费 reserve。
4. `routing_selected_group` 在所有命中 reserve 的路径里都统一写 `reserve`。
5. projection miss / unknown-model 仍然 fail-closed。
6. `GPT-5.4-Sys`、`GPT-5.5-Sys` 以及当前已修好的 GPT-5.5 路由链不回归。
