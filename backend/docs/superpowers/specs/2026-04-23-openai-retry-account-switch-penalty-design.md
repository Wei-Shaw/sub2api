# OpenAI 上游失败重试换号与账号惩罚设计

## 背景

当前 OpenAI 主链已经有上游失败重试能力，但仍存在两个明显问题：

1. 同一账号连续失败时，重试层会继续恋战，不能足够积极地切换到其他账号。
2. 某个账号如果在一段时间内连续命中可重试上游失败，会持续参与路由，放大失败概率。

这次要在**保持现有上游失败重试作用范围不变**的前提下，补上两层策略：

- 单请求内：同一账号连续两次可重试失败后，后续 retry 优先更换账号。
- 跨请求：同一账号连续五次可重试失败后，临时退出路由十分钟。

## 目标

1. 在当前 OpenAI 上游失败重试链路内，引入“连续两次失败后积极换号”的行为。
2. 让连续失败账号能通过现有 `temp_unschedulable` 体系暂时退出路由。
3. 不改变 exhausted / reserve / active / `-Sys` 既有语义，也不改首轮选号逻辑。
4. 以最小改动获得稳定线上效果，不引入新的分布式计数系统。

## 非目标

1. 不扩展到 Gemini / Anthropic / Antigravity 等其他平台。
2. 不重写调度器主算法，不把惩罚逻辑下沉成新的全局调度层。
3. 不做跨实例实时共享失败计数。
4. 不改变现有不可重试错误、认证错误、参数错误的处理语义。

## 作用范围

本次只覆盖“和当前上游失败重试相同范围”的 OpenAI 主链：

- 仅在已有 OpenAI HTTP failover / retry 代码路径里生效，明确包括：
  - `/v1/responses` 主链
  - Chat Completions -> OpenAI compat 主链
  - Messages-on-OpenAI compat 主链
- 明确不包含：
  - OpenAI WebSocket ingress / reconnect 重连路径
  - 通用 `GatewayService` 的全平台重试语义改造
  - 认证错误、参数错误、payload 自愈类本地恢复路径
- 首轮调度不带惩罚偏置。
- 单请求内通过 retry 层显式排除账号来实现“积极换号”。

## 方案概述

### 1. 单请求内连续失败后积极换号

在 OpenAI 当前已有的上游失败重试入口维护一个**请求级账号失败计数**：

- key：`account_id`
- value：该请求内连续命中的可重试失败次数

行为规则：

1. 首次命中某账号的可重试失败：允许按现有逻辑重试。
2. 这里的“连续”按**账号维度**计，不要求在全局尝试序列里相邻；例如 `A失败 -> B失败 -> A失败` 会让 A 的请求内 streak 变成 2。
3. 当某账号请求内 streak 达到 2 时：
   - 从下一次 retry 开始，该账号必须进入当前请求的 `excludedIDs`
   - 不得因为更大的 `pool_mode_retry_count` 或其他同类预算继续恋战该账号
   - 如果现有总重试预算本来更小，则仍尊重更小预算
4. 后续 retry 重新选号时，不再优先复用这个账号，而是优先尝试其他可用账号。
5. 如果已经没有其他账号可选，则沿用现有兜底错误链返回，不做无限循环重试。

这样“连续两次失败后换号”由 retry 层控制，不污染首轮调度和正常粘性语义。

### 2. 跨请求连续失败惩罚

为每个账号维护一个**进程内连续失败计数器**：

- key：`account_id`
- value：跨请求连续可重试失败次数

行为规则：

1. 同一账号每次命中可重试失败，计数 `+1`。
2. 同一账号一旦成功响应，只清当前进程里的本地 streak。
3. 当计数达到 5 次时：
   - 调用现有仓储接口写入 `temp_unschedulable` 10 分钟。
   - reason 明确标记为“连续上游可重试失败触发惩罚”。
   - 写库成功后清空该账号的进程内失败计数。
4. 如果写库失败：
   - 不清空本地 streak
   - 进入本地 `penalty_pending` 等价态，防止同一批 in-flight 失败无限重复触发
   - 仅允许下一次有界重试再次尝试写库，而不是每次失败都立刻重打数据库
5. 惩罚到期后，账号在资格上恢复，但是否重新回到路由，要依赖后文定义的快照恢复机制。

### 3. 失败类型边界

只有**账号级、且当前 OpenAI failover 链已经认定为可重试**的失败，才会进入上述两层计数：

- 统计：可重试上游失败
- 不统计：
  - 参数错误、认证错误、明确不可重试错误、业务配置错误
  - `previous_response_not_found` 这类 continuation 锚点恢复
  - `invalid_encrypted_content` 这类 payload 清洗重试
  - 本地 validation 400、metadata/payload 改写、自愈型 compat 分支

这样可以避免把“本就不该重试”的失败误伤成惩罚事件。

## 架构落点

### 请求级换号

主落点放在 OpenAI 现有 failover / retry 代码路径：

1. retry 层记录“本请求内某账号连续失败次数”。
2. 当某账号在该请求内达到 2 次连续失败时，把它加入下一次选号的 `excludedIDs`。
3. 后续选号继续复用当前调度器接口，不改变调度器本身的首轮排序和 group 语义。

### 跨请求惩罚

主落点放在运行时服务层：

1. 引入进程内失败计数缓存。
2. 在可重试失败路径更新计数；在成功路径清零计数。
3. 达阈值后调用现有 `SetTempUnschedulable(accountID, until, reason)`。
4. 通过现有 scheduler outbox / snapshot 传播，让账号尽快从各实例路由视图中退出。

### 并发与状态机约束

为避免双重处罚、误清零和恢复语义漂移，账号级失败计数必须满足：

1. 以 `account_id` 为粒度做原子更新。
2. 阈值 crossing 只能触发一次惩罚；一旦账号已经进入 `temp_unschedulable`，后续 in-flight 失败不再继续累计或延长惩罚。
3. 成功清零和失败递增必须受同一账号粒度锁/原子状态保护。
4. 本地失败 streak 需要有失效时间（建议 15 分钟无新失败即自动过期），避免其他实例上已经恢复成功后，本实例长期保留陈旧计数。
5. 成功清零语义只保证当前进程立即生效；跨实例可能短时间保留旧 streak，这属于本次明确接受的边界。

## 数据与状态

### 新增运行时状态

新增一个轻量级运行时结构，用于维护账号连续失败计数：

- `account_id`
- `consecutive_retryable_failures`
- 最近一次失败时间（可选，仅用于日志/观测）
- `expires_at`（本地 streak 过期时间，用于抑制跨实例陈旧计数误踢）

该结构仅驻留进程内，不落库。

### 复用现有持久状态

惩罚触发后，不新增数据库字段，继续复用：

- `temp_unschedulable_until`
- `temp_unschedulable_reason`

其中 `temp_unschedulable_reason` 应采用机器可读结构或稳定前缀，至少包含：

- `penalty_kind=retryable_upstream_failure_streak`
- `failure_count`
- `trigger_kind`（例如 429/529/5xx）
- `instance_id`（若可取）

同时结构化日志顶层字段也必须直接输出：

- `penalty_kind`
- `trigger_kind`
- `failure_count`
- `instance_id`
- `write_result`

## 观测性

本次增加最少量观测，不新增复杂面板：

1. 单请求内因“连续两次失败”触发换号时，输出结构化日志。
2. 账号连续失败达到 5 次并触发惩罚时，输出结构化日志：
   - `account_id`
   - `failure_count`
   - `trigger_kind`
   - `until`
   - `reason`
   - `write_result`
   - `penalty_kind`
   - `instance_id`
3. 预留简单计数指标：
   - 请求内因连续失败触发的账号切换次数
   - 账号因连续失败被踢出路由的次数
   - 惩罚写库失败次数

## 恢复语义

仅依赖 `temp_unschedulable_until` 到时自然过期并不足以保证账号及时回到调度快照，因为现有 bucket snapshot 在无重建事件时可能继续返回旧桶。

因此本次设计要额外定义一个轻量恢复机制：

1. 当惩罚触发时，除了写入 `temp_unschedulable`，还要确保到期后会有一次账号级或桶级快照刷新。
2. 推荐实现为：
   - 复用现有 worker / poller 模式，增加一个轻量的“惩罚到期恢复”扫描；
   - 扫描到 `temp_unschedulable_until <= now` 的账号时，触发 `account_changed` / 快照同步。
   - 恢复扫描必须幂等，并按 `account_id + until` 做去重，避免对同一个已到期账号持续重复触发 rebuild/outbox。
3. 设计验收要求明确恢复上界：
   - 账号在惩罚到期后，应在一个有界延迟内（建议 1 分钟级）重新进入调度视图。

## 测试计划

### 单元测试

至少覆盖：

1. 同一账号连续两次可重试失败后，第三次 retry 不再选同一账号。
2. `A失败 -> B失败 -> A失败` 会让 A 被视为达到 2 次账号维度 streak，并在下一次 retry 被排除。
3. 没有其他账号可选时，不进入无限切换循环，而是按现有错误链返回。
4. 同一账号跨请求累计 5 次可重试失败后，写入 `temp_unschedulable` 10 分钟。
5. 成功一次后，连续失败计数清零。
6. 不可重试错误不会增加惩罚计数。
7. `previous_response_not_found`、`invalid_encrypted_content`、本地 validation 400 等自愈/本地恢复分支不进入惩罚计数。
8. 惩罚触发后只写一次，不因并发失败重复延长。
9. 惩罚到期后，账号会在有界延迟内重新进入调度视图。
10. `SetTempUnschedulable` 写库失败时，本地 streak 不会被提前清空，并且后续只做有界重试，不会无限打库。

### 回归测试

必须确认以下行为不变：

1. 首轮选号逻辑不变。
2. exhausted / reserve / active / `-Sys` 语义不变。
3. sticky / `previous_response_id` 优先级不被破坏；被 `excludedIDs` 命中的 sticky/previous-response 账号应被绕过，但不能错误清理 binding。
4. `TargetGroupExhausted` 仍可正常走 reserve overlay；`TargetGroupActive/Any` 不会误吃 reserve 语义。
5. 现有 failover 上限与错误包装语义不变。

### 线上验证

上线后至少要验证两类证据：

1. 请求级换号证据
   - 同一请求第一次、第二次命中同账号可重试失败后，后续 retry 已切到其他账号。
   - 日志里能看出原账号被加入 `excludedIDs` 或等价排除集合。
2. 账号级惩罚证据
   - 某账号达到 5 次连续可重试失败后，产生结构化惩罚日志。
   - 顶层日志字段里能直接看到 `penalty_kind`、`trigger_kind`、`failure_count`、`instance_id`、`write_result`。
   - 该账号在惩罚窗口内不再出现在调度命中日志中。
   - 到期后在有界延迟内重新恢复到调度视图。

## 风险与缓解

### 风险 1：误把不可重试错误计入惩罚

缓解：只在当前 retry 链已经判定为可重试的分支里更新计数。

### 风险 2：进程重启后失败计数丢失

缓解：这是本次有意接受的边界。真正触发惩罚后会落库为 `temp_unschedulable`，跨实例一致性依赖该持久状态，而不是依赖实时共享计数。

### 风险 3：单请求内换号导致额外切号频率升高

缓解：只在同一账号连续两次失败后才触发，不是首次失败就强制切号。

## 验收标准

满足以下条件即视为本次设计完成：

1. OpenAI 当前重试作用范围内，连续两次命中同一账号的可重试失败后，会优先更换账号。
2. 同一账号连续五次可重试失败后，会被临时踢出路由十分钟。
3. 成功一次后当前进程本地 streak 清零。
4. 惩罚到期后，账号会在约定的恢复上界内重新回到调度视图。
5. exhausted / reserve / active / `-Sys` 语义无回归。
6. 没有引入新的数据库表或全局分布式计数系统。
