# OpenAI 按模型子集重算 exhausted/reserve 设计

## 背景

当前 OpenAI exhausted / reserve 语义仍然主要按“账号整体状态”分桶，然后在后续调度阶段再按请求模型过滤账号可达性。

这会带来一个结构性问题：

1. 不同账号可访问的模型集合并不相同。
2. 新模型刚发布时，往往只有极少数账号具备访问权限。
3. 这些账号可能既不在当前全局 exhausted 池里，也不在全局 reserve 池里。
4. 于是 exhausted-class 请求在模型过滤之前就被分桶逻辑提前卡死，无法命中真正有该模型权限的账号。

用户希望解决的不是简单的“请求阶段临时放宽规则”，而是把 exhausted / reserve 的生成逻辑改成**考虑模型可达子集后的派生结果**，并保持 reserve 的跨模型一致性。

## 目标

1. exhausted / reserve 的生成逻辑必须考虑“支持该模型的账号子集”。
2. reserve 仍然保持当前 overflow 语义，不新增第三 target group。
3. `exhausted = 0` 时仍按“exhausted 占用率 100%”处理，并继续走现有 reserve overflow 规则。
4. 一旦某账号因为任一模型子集被提升为 reserve，它在自己支持的所有模型子集里都保持 reserve 身份。
5. 账号状态变化后，要整体重算这套派生结果，并通过现有 scheduler snapshot / outbox 体系对外可见。

## 非目标

1. 不改变 exhausted-class 请求的外部语义。
2. 不把 reserve 变成独立 target group。
3. 不把请求阶段改成“临时现算 reserve 身份”的惰性逻辑。
4. 不推翻现有 overflow 判定阈值与 `exhausted=0 => 100%` 规则。
5. 不扩展到 Gemini / Anthropic / Antigravity 等其他平台。

## 核心语义

### 1. 模型子集先于账号静态分桶

对 OpenAI exhausted / reserve 的推导，不再从“全量账号静态分桶”出发，而是先建立模型可达视角：

1. 对每个可路由模型 `M`，取出“支持 `M` 的账号子集”。
2. exhausted / reserve 的容量规则只在这个子集内计算。
3. exhausted-class 请求在消费候选池时，看到的是“该模型子集下的 exhausted / reserve 结果”，而不是账号整体静态标签。

### 2. reserve 仍然只是 exhausted-class 的 overflow 子组

本次不会改变 reserve 的语义定位：

1. reserve 依然不是第三 target group。
2. exhausted-class 请求仍先看 exhausted 子集。
3. exhausted 子集占用率高时继续按现有规则让 reserve overflow 承接。
4. exhausted 子集为空时，继续按 `exhausted 占用率 = 100%` 处理，再进入 reserve overflow 判定。

### 3. reserve 身份提升为“全模型一致”

模型子集重算只是发现角色变化的来源，但最终输出不能停留在模型局部。

规则是：

1. 如果某账号在任一模型子集里被判定为 reserve overflow 候选。
2. 那么它在自己支持的所有模型子集里都保持 reserve 身份。
3. 因此，reserve 身份不再只是账号静态状态，而是“模型子集推导后，再提升成账号级一致角色”的结果。

这条规则优先级高于局部最优容量分配，目的是让线上 reserve 身份稳定、可解释、可观测。

## 架构方案

### 1. 新增 OpenAI 派生角色投影层

在现有 OpenAI 调度链前增加一个“派生角色投影”层，作为 exhausted / reserve 的统一来源。

该投影层输入：

1. 当前可调度的 OpenAI 账号集合。
2. 每个账号的 exhausted / active 基础状态。
3. 每个账号的模型可达信息：
   - `IsModelSupported`
   - model mapping
   - 新模型访问权限
4. 每个账号的 schedulable / temp-unsched / rate-limit 等状态。

该投影层输出：

1. exhausted 基础池。
2. reserve overflow 池。
3. 每个账号是否被提升为 reserve。
4. 每个账号支持的模型集合下，对应应消费的角色视图。

### 2. 生成流程

建议采用“两段式”派生：

1. 先对每个模型子集做局部计算。
2. 再把局部 reserve 结果归并成账号级全模型一致 reserve 身份。

具体顺序：

1. 遍历可路由模型集合。
2. 对每个模型，取支持该模型的账号子集。
3. 在这个子集内计算：
   - exhausted 基础池
   - reserve overflow 候选池
4. 把所有模型子集里被判为 reserve 的账号汇总成账号级 reserve 集合。
5. 再把这个账号级 reserve 集合回填到各模型子集，形成最终一致结果。

这样可以满足两个要求：

1. reserve 来源受模型可达性约束。
2. reserve 最终对账号保持全模型一致。

### 3. 请求阶段只消费投影结果

请求阶段不再负责临时生成 reserve 身份。

请求阶段只做三件事：

1. 取请求模型 `M` 的投影结果。
2. 取其中的 exhausted 基础池与 reserve overflow 池。
3. 继续沿用现有 overflow 判定：
   - exhausted 子集占用率 > 阈值时走 reserve
   - exhausted 子集为空时按 100% 处理

这意味着：

1. exhausted-class 请求外部语义不变。
2. 但它不会再被“全局 exhausted/reserve 为空”这种静态结果提前卡死。
3. 新模型只被少量账号支持时，仍能通过子集重算产出合法 reserve overflow 候选。

## 重算触发机制

这套投影必须是**状态变化驱动的整体重算结果**，而不是请求时现算。

至少以下变化要触发整体重算：

1. 配额/耗尽状态变化。
2. model mapping 变化。
3. 可用模型/新模型访问权限变化。
4. temp-unsched / schedulable / rate-limit 状态变化。
5. 账号新增、删除、编辑。
6. scheduler snapshot 重建。

实现上应尽量复用现有 scheduler snapshot / outbox 事件链，让“账号状态变更 -> 派生角色重算 -> 路由视图刷新”保持同一套刷新机制。

## 数据与状态

### 1. 新增派生投影状态

需要新增一份 OpenAI 专属派生投影状态，至少包含：

1. 账号级 reserve 身份集合。
2. 模型到候选池的映射：
   - exhausted 基础池
   - reserve overflow 池
3. 生成版本或时间戳。

### 2. 存储边界

本次不建议引入新的数据库表。

优先级建议：

1. 作为 scheduler snapshot 的扩展派生结果一起缓存。
2. 随 snapshot/outbox 重建而更新。
3. 保证请求阶段只读，不在热路径重新做全量模型子集推导。

## 观测性

至少增加以下观测：

1. 派生重算日志：
   - 触发原因
   - 参与账号数
   - 模型子集数
   - 最终 reserve 账号数
2. 关键账号角色变化日志：
   - `account_id`
   - 旧角色
   - 新角色
   - 触发原因
3. 请求命中日志仍保留现有：
   - `routing_target_group`
   - `routing_selected_group`
   - 但应能解释这是基于模型子集投影后的结果

## 测试计划

### 单元测试

至少覆盖：

1. 新模型只被 2 个账号支持时：
   - 在该模型子集里会重算 reserve
   - 至少 1 个账号会进入 reserve overflow 池
2. 某账号一旦因任一模型子集进入 reserve：
   - 在自己支持的所有模型子集里都保持 reserve 身份
3. `exhausted=0 => 100%` 规则在模型子集上继续成立。
4. 账号状态变化后，投影会整体重算。
5. 新模型访问权限变化后，投影会整体重算。
6. temp-unsched / schedulable 变化后，投影会整体重算。

### 回归测试

必须确认以下行为不回归：

1. reserve 仍然只是 exhausted overflow 子组。
2. exhausted / reserve / active / `-Sys` 语义不回归。
3. sticky / previous_response / exhausted-reserve overlay 语义不被打穿。
4. exhausted-class 请求不会被偷偷降成 active / any。
5. 请求阶段不重新现算 reserve，而是只消费投影结果。

### 线上验证

上线后至少验证：

1. 新模型场景：
   - 当前只有少量账号有权限
   - exhausted-class 请求可以命中模型子集重算后的 reserve overflow 账号
2. 账号状态变化场景：
   - 配额或模型权限变化后，reserve 视图会刷新
   - 不会长期保留旧 reserve 结果

## 风险与缓解

### 风险 1：全模型一致 reserve 让局部最优容量分配变差

缓解：这是本次明确接受的设计取舍，优先稳定和可解释性，而不是局部最优。

### 风险 2：投影重算成本偏高

缓解：放在状态变化链上整体重算，不放在请求热路径现算；必要时后续再做增量优化。

### 风险 3：角色变化和 snapshot 刷新不同步

缓解：要求复用现有 scheduler snapshot / outbox 链，让派生投影和调度视图一起更新。

## 验收标准

满足以下条件即视为本次设计完成：

1. exhausted / reserve 的生成已考虑模型可达子集。
2. reserve 仍保持 overflow 语义，`exhausted=0 => 100%` 规则继续成立。
3. 某账号一旦因任一模型子集进入 reserve，它在自己支持的所有模型子集里都保持 reserve 身份。
4. exhausted-class 请求消费的是投影结果，而不是账号静态分桶结果。
5. 账号状态变化后，整套投影会整体重算并对请求阶段可见。
