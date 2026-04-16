# OpenAI reserve 组调度设计

## 背景

当前 OpenAI 调度主链只有 `active / exhausted / any` 三种目标组语义。

在 `-Sys` 请求占比较高的场景下，流量会长期压向 exhausted 组；如果 exhausted 组中的账号数量只减不增，而 active 组长期收不到请求，则 exhausted 池会逐渐失去自我补充能力，最终导致：

- exhausted 组容量不足
- exhausted 组并发排满后只能失败
- active 组中的一部分免费层级账号明明还有配额，但没有承担“耗尽组高压缓冲”的角色。

## 目标

在 OpenAI 调度主链中新增一个显式 `reserve` **overflow 子组**，作为 exhausted 的备用容量池。

这个子组的目标不是替代 exhausted，而是：

1. 在 exhausted 自身配置容量不足时，用 free active 账号补足到目标线；
2. 在 exhausted 运行时压力过高时，承接溢出流量；
3. 让 exhausted 池中的账号数量可以继续增长，而不是只减不增。

## 非目标

- 不改其他平台（Anthropic、Gemini、Antigravity）的调度语义。
- 不改 `active/exhausted/-Sys` guardrail 的产品定位。
- 不新增复杂的实时池管理界面。
- 不把 reserve 做成永久静态账号组或新的账号固有状态。

## 核心设计

### 1. 语义分层：请求目标组 vs 实际命中子组

当前 OpenAI 调度的**请求语义目标组**仍然只有：

- `active`
- `exhausted`
- `any`

本次新增的是 exhausted-class 内部的一个**实际命中子组**：

- `reserve`

其中：

- `reserve` 不是账号固有的第三状态，也不是对外暴露的新请求语义组。
- 请求入口、错误码、计费、sticky/previous_response guardrail 仍以 `active/exhausted/any` 这组既有语义为准。
- `reserve` 只表示：**一个原本属于 active-class 的 free 账号，在 exhausted 高压阶段被 exhausted-class overflow 逻辑选中承接请求。**
- 因此本次需要区分两层写库/观测：
  - `routing_target_group`：请求语义目标组（仍是 `active/exhausted/any`）
  - `routing_selected_group`：最终实际命中的子组（`active/exhausted/reserve`）

### 2. reserve 候选池形成规则

reserve 只从满足以下条件的账号中选择：

- OpenAI 账号
- 当前可调度
- 属于 free 层级
- 当前不在 exhausted 组

第一阶段明确只从 **OpenAI OAuth free 账号** 中选择 reserve 候选。
其数据来源使用当前已有且稳定的账号信息：

- `account.platform == openai`
- `account.type == oauth`
- `account.credentials.plan_type == free`

reserve 的规模按**并发容量**而不是账号数计算。

定义：

- `exhausted_capacity`
- `active_free_capacity`
- `reserve_capacity`

规则：

- 当 exhausted 自身配置容量不足 60% 目标线时，从 free active 候选中切出 reserve 容量
- 直到：
  - `exhausted_capacity + reserve_capacity >= 60% 目标线`
  - 或 free active 候选耗尽。

这里的目标线是“池子容量有多大”，不等于请求立即进入 reserve。

### 3. reserve 启用规则（请求何时流向 reserve）

唯一运行时门槛：

- `exhausted` 当前并发容量使用率 `> 60%`

一旦超过 60%，reserve 进入与 exhausted 联动的调控阶段：

- exhausted 利用率尽量维持在 60%
- 超出部分优先流向 reserve
- 直到 reserve 也达到 60%
- 之后 exhausted 和 reserve 再一起上升。

因此：

- exhausted 使用率 `<= 60%` 时，请求仍只走 exhausted
- exhausted 使用率 `> 60%` 时，新增溢出请求开始走 reserve。

### 4. reserve 组内与 exhausted 组内的选号规则

本次不重做组内负载均衡算法。

规则为：

- exhausted 组内部继续复用当前同组负载均衡逻辑
- reserve 组内部也复用同组负载均衡逻辑

也就是说，本次新增的是：

- 什么时候把请求导向 reserve
- reserve 池里有哪些账号
- reserve 如何在日志/面板中呈现

而不是再造一套新的组内选号策略。

### 5. reserve 账号退出规则

reserve 账号退出有两种情形：

#### 被动退出

当 reserve 账号自身配额耗尽时，它直接转入 exhausted-class。

这是 reserve 账号最主要的退出方式。

#### 主动退出

主动退出只发生在一种情况：

- active 组中有账号新耗尽，导致 exhausted 组扩容
- 扩容后，reserve 相对 60% 目标线变成“超额”

这时才从 reserve 中主动回收一部分账号回 active。

主动退出规则：

- 仅在“周期性重算 / 配置刷新 / 状态变化”时触发
- 只回收 reserve 中高于所需容量的那部分账号
- 回收顺序优先选择：**剩余配额最多** 的 reserve 账号先退出，回到 active

第一阶段“剩余配额最多”的判定数据源明确限定为当前已有稳定字段：

- 对 OpenAI OAuth free 账号，优先使用 `codex_7d_used_percent` / `codex_primary_used_percent`
- 以“已用百分比更低”近似“剩余配额更多”
- 不在本次范围内为 API Key 账号强行建立统一 remaining-quota 模型，因此 reserve 候选池只从 OAuth free 账号中挑选

这样可以避免 reserve 在 60% 附近频繁抖动。

### 6. reserve 退出调控状态

当 exhausted 实时利用率回落到 `<= 60%` 时，reserve 退出“承接溢出请求”的调控状态。

但 reserve 中的账号不一定立即全部退出 reserve 池；是否回收，仍由上面的“主动退出规则”决定。

### 7. sticky / previous_response_id / `-Sys` guardrail 关系

reserve 必须被视为 **exhausted-class overflow 子组**，而不是新的 guardrail 语义组。

因此以下既有语义保持不变：

- `-Sys` 请求的请求语义目标组仍然是 `exhausted`
- exhausted-class 的 429/503 错误语义不被 reserve 打散
- sticky / `previous_response_id` 的亲和域按 exhausted-class 判断

也就是说：

- 如果某次 exhausted 请求实际命中的是 reserve 账号，后续同一会话的 exhausted-class continuation 仍应视该绑定为可命中
- reserve 不得被当成会触发 `miss_binding_restricted` 的新独立组
- `TargetGroupAny` 的既有语义保持不变；reserve 不作为普通 any/active 流量的常规候选来源，而只在 exhausted overflow 决策内部被引入

为了让这一点在现有代码结构中可表达，本次必须新增一层**exhausted-class 亲和状态载体**，而不是只复用现有 `sessionHash -> account_id` / `previous_response_id -> account_id` 绑定：

- 当前绑定至少需要扩到：
  - `bound_account_id`
  - `affinity_domain`（第一阶段只需要 `active` 或 `exhausted`）
  - 可选 `selected_group`（`active/exhausted/reserve`）用于观测
- exhausted 请求若实际命中 reserve，后续 continuation/sticky 判定仍按 `affinity_domain=exhausted` 命中，而不是按账号当前固有 active-class 属性去做 mismatch。

第一阶段在实现路径上明确选择：

- reserve 所需的 `affinity_domain / selected_group` 绑定，必须是**可跨请求、跨进程、跨实例恢复**的 OpenAI 专用 carrier。
- 不允许只用进程内 map 作为最终方案。
- 推荐实现方式：
  - 在现有 Redis/cache 体系上新增 **OpenAI 专用 namespaced binding**
  - 旧的 shared `sessionHash -> accountID` / `response_id -> accountID` 语义继续保留
  - 另外新增 companion binding 承载：
    - `affinity_domain`
    - `selected_group`
- 也就是说，本次是 **OpenAI 专用 exhausted-class 亲和扩展**，但它必须落在可持久化的 carrier 上，而不是单进程内存态。

## 日志与运维观测

### 请求级写库

`usage_logs.routing_target_group` 继续记录**请求语义目标组**：

- `active`
- `exhausted`

新增一个新的请求级字段记录**最终实际命中的子组**：

- `routing_selected_group`：`active / exhausted / reserve`

这样才能同时回答：

- 这次请求在 guardrail 语义上属于哪一组
- 实际是落在 exhausted 本体，还是 reserve overflow 子组

同样的写库要求也适用于错误链（如 `ops_error_logs`），否则 reserve 高压期的失败请求会在 request details 中被系统性漏掉。

request details / retry 明细在第一阶段不仅要展示 `routing_selected_group`，还必须支持按 `routing_selected_group` 过滤；否则 routing/retry 卡片能点出 reserve，但 drilldown 会直接查空或查错。

### 运维面板

现有 OpenAI 路由分布卡片需要在**继续保留 exhausted-class 请求过滤**的前提下，以 `routing_selected_group` 为统计维度，从两组扩展为三组：

- active
- exhausted
- reserve

第一阶段统计口径明确为：

- 仍只统计 `routing_target_group in ('active','exhausted')` 的请求
- 但在这个范围内，按 `routing_selected_group` 聚合展示 `active/exhausted/reserve`

这样可以避免普通 `TargetGroupAny` 流量稀释 reserve 的承压占比。

第一阶段本次必须补齐的 reserve 相关指标只包含**当前已有稳定数据源可支撑的部分**：

- reserve 承接的请求量
- reserve 承接的 token 量

以下两类指标目前缺少可靠历史数据源，**不纳入第一阶段承诺**，留作后续专项：

- reserve 当前容量的历史可视化
- reserve -> exhausted / reserve -> active 的精确生命周期累计计数

不要求本次做复杂实时池状态页，但必须让运维能从现有统计链看到 reserve 的存在与承压作用。

## 代码边界

本次只动 OpenAI 调度主链与相关观测链：

- `backend/internal/service/openai_gateway_service.go`
- `backend/internal/service/openai_account_scheduler.go`
- `backend/internal/handler/openai_gateway_handler.go`
- `backend/internal/handler/openai_chat_completions.go`
- `backend/internal/service/openai_ws_state_store.go`
- `backend/internal/service/openai_ws_forwarder.go`
- `backend/internal/service/openai_sticky_compat.go`
- `backend/internal/service/openai_routing_observability.go`
- `backend/internal/service/gateway_service.go`
- `backend/internal/repository/gateway_cache.go`
- `backend/internal/service/usage_log.go`
- `backend/internal/service/ops_port.go`
- `backend/internal/service/ops_request_details.go`
- `backend/ent/schema/usage_log.go`
- `backend/internal/service/gateway_service.go`（仅在需要桥接现有 OpenAI sticky 共享入口时做最小改动）
- `backend/internal/repository/gateway_cache.go`（仅当最终证明必须扩 shared carrier 时才进入范围；第一阶段默认不动）
- migration / ent 生成 / repo / DTO / API carrier files needed for `routing_selected_group`
- `backend/internal/repository/usage_log_repo.go`
- `backend/internal/repository/ops_repo.go`
- `backend/internal/repository/ops_repo_request_details.go`
- `backend/internal/handler/ops_error_logger.go`
- `backend/internal/service/ops_openai_routing_stats.go`
- `backend/internal/repository/ops_repo_openai_routing_stats.go`
- `backend/internal/handler/admin/ops_handler.go`
- `backend/internal/handler/dto/types.go`
- `frontend/src/api/admin/ops.ts`
- `frontend/src/types/index.ts`
- `frontend/src/views/admin/ops/components/OpsOpenAIRoutingCard.vue`
- `frontend/src/views/admin/ops/components/OpsOpenAIRetryCard.vue`
- `frontend/src/views/admin/ops/components/OpsRequestDetailsModal.vue`
- `frontend/src/views/admin/ops/components/OpsOpenAIRetryDetailsModal.vue`
- `frontend/src/views/admin/ops/OpsDashboard.vue`
- `frontend/src/components/admin/usage/UsageTable.vue`
- `frontend/src/views/admin/UsageView.vue`
- 相关 i18n 与测试文件
- 相关测试文件

如果现有 ops/usage 统计链路需要扩字段，也只在 OpenAI 路由/usage 这条线上做最小增量。

不碰：

- Anthropic / Gemini / Antigravity 调度语义
- 已确认本地 guardrail 本身的产品定位
- 与 OpenAI reserve 无关的 dashboard 区块。

## 验证策略

### 后端单测必须覆盖

1. exhausted 容量不足时 reserve 补足到 60%
2. exhausted 使用率 `<= 60%` 时，请求不进入 reserve
3. exhausted 使用率 `> 60%` 时，溢出流量流向 reserve
4. reserve 账号耗尽后转入 exhausted-class
5. active 新耗尽导致 exhausted 扩容时，reserve 按“剩余配额最多优先退出”回 active
6. reserve 命中的请求会正确写入：
   - `routing_target_group = exhausted`
   - `routing_selected_group = reserve`
7. sticky / `previous_response_id` 在 reserve 命中后仍按 exhausted-class 亲和域工作，不会误记为 group mismatch
8. routing / retry 统计继续只统计 `routing_target_group in ('active','exhausted')` 的请求，但按 `routing_selected_group` 展示 reserve
9. request details / usage table 能看到 `routing_selected_group = reserve`
10. retry / request details drilldown 能按 `routing_selected_group = reserve` 正确过滤，不会出现卡片有值、明细查空

### 前端验证

- `pnpm typecheck`
- 与 OpenAI routing / retry / sticky 卡片相关的 Vitest（如已有）
- 至少确认 reserve 在面板中有单独展示位，而不是被混入 active。
- 至少确认 request details / usage table 中能看见 `routing_selected_group = reserve`。

## 风险与约束

- 本次新增的是一层“动态备用池”调控，不是永久的第四类账号归属。实现时要避免把 reserve 做成静态分组配置或新的账号固有状态。
- reserve 账号主动退出的触发，必须和 exhausted 扩容绑定；不能简单地因为 exhausted 利用率短时回落就把 reserve 全量退回，否则会造成来回抖动。
- reserve 是 exhausted-class 的 overflow 子组，不应破坏现有 `-Sys -> exhausted` 的产品约束；如果某条链暂时无法同时支持“独立展示 reserve”和“保留 exhausted-class 语义”，优先保留 exhausted-class guardrail 语义。
