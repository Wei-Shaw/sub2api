# sub2api OpenAI 路由观测设计

## 背景

当前 `sub2api` 已经具备 OpenAI `active` / `exhausted` 目标组路由能力，也已经在成功请求的 `usage_logs` 中保存了 `requested_model`、`upstream_model`、`account_id`、`inbound_endpoint`、`upstream_endpoint` 等基础观测字段，在运维侧也已有 `request details`、`error logs`、`account availability`、OpenAI token 统计等入口。

但和 OpenAI 路由策略直接相关的几个关键问题仍然无法稳定回答：

1. 单条请求最终是被判到 `active` 还是 `exhausted` 目标组。
2. 单条请求最终命中了哪一层调度路径，例如 `previous_response`、sticky session、load-balance 或 fresh resolve。
3. 请求模型、实际生效模型、`-Sys` / continuation 等 OpenAI 路由语义在使用记录里不可直接见。
4. failover 发生过几次、最终为什么停在当前账号或失败，在 usage/ops 入口里不可直接追踪。
5. 在运维监控中无法按请求次数、总 tokens、输入/输出 tokens 等口径稳定统计 active/exhausted 分布。

用户希望在两类入口都能清楚看到这些信息：

- 使用记录：看单请求是怎么路由的。
- 运维监控：看整体流量在 `active` / `exhausted` 间如何分布，并按多种 token 口径衡量。

## 目标

本设计要实现以下能力：

1. 对 OpenAI 路由相关请求，持久化一份稳定的最终路由快照。
2. 成功请求与错误请求使用尽量同构的路由观测字段，便于 usage/ops 两边统一理解。
3. 在使用记录中直接展示目标组、命中层、最终账号、请求模型与实际模型、failover 摘要。
4. 在运维监控中直接展示 `active` / `exhausted` 的流量分布，第一版支持：
   - 按请求次数
   - 按总 tokens
   - 按 input / output tokens 分拆
5. 所有聚合都支持沿用现有 ops 的时间范围、平台、group 过滤。

## 非目标

本设计明确不做以下内容：

1. 不新建完整的 `routing_attempts` / `routing_events` 逐跳审计表。
2. 不追求记录每一次候选筛选、每一次排除原因的全量轨迹。
3. 不把 Gemini、Anthropic、Sora、Antigravity 等非 OpenAI 路径卷入本次路由观测模型。
4. 不在第一版里加入按费用金额的分布统计。

## 设计概览

### 1. 路由观测模型

引入一份 request-scoped 的 OpenAI routing snapshot，表示“这条请求最终是如何被路由出去的”。

这份 snapshot 只记录最终足以解释行为的摘要，不记录完整 attempt-by-attempt 审计轨迹。

推荐字段如下：

- `routing_target_group`
  - 值域：`active` / `exhausted`
  - 含义：这条请求最终进入哪个目标组语义。
- `routing_schedule_layer`
  - 值域来自当前调度层语义，例如 `previous_response`、`sticky_session`、`load_balance`、`fresh_resolve`。
  - 含义：最终命中账号是通过哪一层被选中的。
- `routing_selected_account_id`
  - 含义：最终命中的账号 ID。
- `routing_selected_account_name`
  - 含义：最终命中的账号名称快照，用于避免仅凭 ID 追查。
- `routing_requested_model`
  - 含义：请求入口层最终认定的“用户请求模型”。
  - 与现有 `requested_model` 含义接近，但本字段明确属于 routing snapshot，以便后续与 routing 维度一起显示。
- `routing_effective_model`
  - 含义：真正送往上游调度/转发的模型，例如 `-Sys` strip 后的模型，或未来更广义的 routing 生效模型。
- `routing_failover_count`
  - 含义：本次请求在最终落定前经历了多少次 failover / reselect。
- `routing_failover_final_reason`
  - 含义：最终一次 failover 结束时的原因摘要，例如“成功切到 exhausted 组账号”“无可用 active 账号”“上游失败后切换成功”等。

这组字段同时写入：

- 成功请求：`usage_logs`
- 错误请求：`ops_error_logs`

字段集保持尽量同构，避免 usage/ops 两套观测语义分裂。

### 2. 数据落地点

本设计采用“扩展现有持久表”而不是“新建独立事件表”。

原因：

1. 使用记录本来就以 `usage_logs` 为中心，新增一组 routing 字段即可直接进入现有分页、导出、过滤和模型维度分析。
2. 错误请求本来就以 `ops_error_logs` 为中心，新增同构字段即可直接进入错误钻取与问题归因。
3. 用户要求同时强化 usage 和 ops，如果使用两套完全不同的数据源，会在 UI 和聚合上引入更多解释成本。
4. 第一版需求是“稳定看清最终路由行为”，不是“完整回放每一步决策树”，没必要立刻引入新的事件模型。

### 3. 数据生成时机

OpenAI handler / service 在一次请求生命周期内维护 routing snapshot：

1. 进入 `Responses()` / `Messages()` / `Chat Completions` 等 OpenAI 路由入口后，先确定 `target_group` 与请求模型语义。
2. 调度完成后，记录最终 `schedule_layer`、命中账号、effective model。
3. 若请求中途发生 failover，则累计 `routing_failover_count`，并更新 `routing_failover_final_reason`。
4. 请求成功时，在写 `usage_logs` 时带入 snapshot。
5. 请求失败时，在写 `ops_error_logs` 时带入 snapshot。

这样 usage 与 ops 看到的是同一份最终路由快照，而不是由前端或聚合层临时推断。

## 查询与展示设计

### 1. 使用记录

#### 1.1 Admin UsageView

在现有使用记录表格中新增或补充以下展示维度：

- `routing_target_group`
- `routing_schedule_layer`
- `routing_selected_account_name`（或 `account` 列增强为明确的最终路由账号）
- `routing_requested_model`
- `routing_effective_model`
- `routing_failover_count`
- `routing_failover_final_reason`

同时扩展筛选条件：

- 按 `routing_target_group` 过滤
- 按 `routing_schedule_layer` 过滤

导出 Excel 时也应包含这些字段，保证离线分析与页面看到的信息一致。

#### 1.2 Ops Request Details

在现有 `OpsRequestDetailsModal` 中补充与 usage 同构的路由字段，用于请求级钻取：

- 这条请求被判到 `active` 还是 `exhausted`
- 最终通过哪一层命中账号
- 最终命中的账号是谁
- 请求模型与 effective model 是什么
- 是否发生 failover、发生了几次、最后以什么原因收敛

这样 usage 和 ops 两个请求级入口都能回答“这条请求最后到底是怎么走的”。

### 2. 运维监控

在现有 Ops Dashboard 中新增一个 OpenAI Routing 区块，不单开页面。

第一版至少提供 3 组聚合图/卡片：

1. `active` vs `exhausted` 按请求次数分布
2. `active` vs `exhausted` 按总 tokens 分布
3. `active` vs `exhausted` 按 input / output tokens 分拆分布

这些聚合统一支持当前 ops 已有的过滤语义：

- 时间范围
- 平台（第一版重点是 OpenAI）
- group 过滤

展示上应满足两个目标：

1. 一眼看出 exhausted 组是否正在承接大量请求。
2. 不只看请求次数，也能判断这些请求的 token 体量是否集中在 exhausted 组。

### 3. 聚合数据来源

聚合不从应用日志回放，也不从 request details 临时拼装。

统一直接基于持久化的 routing snapshot 字段做 SQL 聚合，以保证：

1. 查询稳定
2. 历史数据可追
3. 统计口径统一
4. 前端只负责展示，不负责推断路由语义

## 字段语义约束

为避免后续统计歧义，本设计要求以下语义固定：

1. `routing_target_group`
   - 只反映最终请求语义落在哪个组，不表示过程中尝试过哪些组。
2. `routing_schedule_layer`
   - 只记录最终命中层，不记录所有经过的层。
3. `routing_selected_account_id/name`
   - 只记录最终成功命中或最终失败前最后一次选中的账号。
4. `routing_failover_count`
   - 记录从首选账号到最终收敛期间发生的 failover 次数。
5. `routing_failover_final_reason`
   - 记录最终结论，而不是每一步原因明细。
6. 非 OpenAI 请求
   - 上述字段全部保持 `NULL` / 空值，不做伪填充。

## 兼容性与迁移

### 1. 数据库迁移

因为用户接受持久字段扩展，本设计允许为 `usage_logs` 与 `ops_error_logs` 增加 routing 相关列。

迁移原则：

1. 新字段都允许为空，以兼容历史数据。
2. 历史行不做回填推断，避免把旧数据“猜成” active / exhausted。
3. 前端对空值要清晰显示为“无 routing snapshot”或空白，而不是误解释成 active。

### 2. 历史数据展示

历史 `usage_logs` / `ops_error_logs` 不具备 routing snapshot 的行，前端应显示为空值，不参与 active/exhausted 聚合，或在聚合中明确归为“无路由快照”并可选隐藏。

第一版更推荐：

- 对请求级列表：显示为空
- 对聚合图表：默认仅统计有 routing snapshot 的 OpenAI 行，避免历史空洞污染比例

## 风险与控制

### 风险 1：路由字段写入点分散，成功与失败语义不一致

控制方式：

- 在 OpenAI handler/service 中抽一份统一的 routing snapshot 结构
- 成功写 usage、失败写 ops error 时都复用它

### 风险 2：前端自己推断 active/exhausted，口径漂移

控制方式：

- 前端只展示持久字段
- 所有 active/exhausted 判定都以后端写入字段为准

### 风险 3：聚合查询性能下降

控制方式：

- 第一版仅增加必要字段
- 聚合按现有 ops 的时间范围与过滤条件做约束
- 如果后续数据量增长，再考虑索引或预聚合，而不是第一版先上独立事件体系

### 风险 4：把非 OpenAI 请求卷入统计

控制方式：

- routing snapshot 只在 OpenAI 路径生成
- 非 OpenAI 行统一保持 `NULL`
- 前端与聚合接口默认按 OpenAI routing 语义过滤

## 验证策略

### 1. 后端单测

覆盖以下行为：

1. handler/service 正确生成 routing snapshot
2. `target_group`、`schedule_layer`、selected account、effective model、failover 摘要写入成功请求
3. 同一套 snapshot 能写入错误请求

### 2. repository / 聚合测试

覆盖以下行为：

1. `usage_logs` 按 `routing_target_group` 聚合请求次数
2. `usage_logs` 按 `routing_target_group` 聚合总 tokens
3. `usage_logs` 按 `routing_target_group` 聚合 input / output tokens
4. 历史空值行不会被误计入 active/exhausted

### 3. 前端测试

覆盖以下行为：

1. UsageView 新列与过滤项可见且渲染正确
2. Ops Dashboard 新的 OpenAI Routing 区块可见且数值来自接口
3. Request Details / Error Details 中的 routing 字段展示正确

## 结论

本设计选择“扩展现有持久表 + 统一 routing snapshot”的路线，以最小的概念增量同时满足两类需求：

1. 在使用记录中看清单请求是如何路由的。
2. 在运维监控中稳定统计请求在 `active` / `exhausted` 中的分布比例，并支持请求次数、总 tokens、输入/输出 tokens 三种口径。

这条路线比“前端现拼”更稳，比“独立完整事件表”更轻，也与当前 `usage_logs`、`ops_error_logs`、`request details`、Ops Dashboard 的结构最一致。
