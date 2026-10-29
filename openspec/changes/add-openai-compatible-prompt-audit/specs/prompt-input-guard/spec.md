## ADDED Requirements

### Requirement: 同步提示词门禁必须由显式配置启用
系统 SHALL 使用 `enabled` 与 `blocking_enabled` 表达关闭、异步只审计、同步审计并阻止三态。旧配置或缺失字段 MUST 归一为 `blocking_enabled=false`；系统 MUST 拒绝 `enabled=false && blocking_enabled=true` 的配置。

#### Scenario: 关闭提示词审计
- **WHEN** enabled=false
- **THEN** 有效模式 MUST 为 off
- **THEN** blocking_enabled MUST 被视为 false

#### Scenario: 启用异步审计
- **WHEN** enabled=true 且 blocking_enabled=false
- **THEN** 有效模式 MUST 为 async_audit
- **THEN** Guard 故障 MUST NOT 改变主请求结果

#### Scenario: 启用同步阻止
- **WHEN** enabled=true 且 blocking_enabled=true
- **THEN** 有效模式 MUST 为 blocking
- **THEN** 适用请求 MUST 等待 Guard 判定后才能进入账号选择、计费和上游阶段

#### Scenario: 保存非法开关组合
- **WHEN** 管理员保存 enabled=false 且 blocking_enabled=true
- **THEN** 后端 MUST 返回 400 和 `prompt_guard_requires_audit_enabled`

### Requirement: 安全审计协调器必须保持两个引擎的独立语义
系统 SHALL 通过一个薄协调器把可信请求上下文交给现有内容审核和新增提示词审计。协调器 MUST 不转换两套风险分类、不共用事件表、不让提示词审计触发内容审核副作用，并 MUST 使用确定性的阻断优先级。

#### Scenario: 现有内容审核阻断
- **WHEN** 现有内容审核返回 Block
- **THEN** 客户端 MUST 继续收到升级前的状态码、错误码和文案
- **THEN** 提示词审计异步模式 MAY 继续完成自己的独立记录

#### Scenario: 仅提示词 Guard 阻断
- **WHEN** 现有内容审核允许但提示词 Guard 返回 Block
- **THEN** 客户端 MUST 收到 `prompt_guard_blocked`

#### Scenario: 两个引擎同时阻断
- **WHEN** 两个引擎都返回 Block
- **THEN** 现有内容审核错误语义 MUST 具有客户端响应优先级
- **THEN** 两个引擎 MUST 各自记录其结果和结构化日志

### Requirement: 同步门禁必须位于外部副作用之前
系统 MUST 在鉴权和请求格式校验完成后、账号选择、账户并发、计费资格检查、任何预扣、上游连接和上游写入之前完成同步判定。被 Block 或 fail-closed 拒绝的请求 MUST 不产生这些下游副作用。

#### Scenario: HTTP 请求被 Guard 阻断
- **WHEN** 任一支持的 HTTP 模型请求得到 Block
- **THEN** 账号选择次数、计费检查/预扣次数和上游请求次数 MUST 均为 0
- **THEN** 流式请求 MUST 在拒绝前未写出 SSE 响应头或首字节

#### Scenario: Guard 不可用
- **WHEN** 同步模式下所有可用节点均失败
- **THEN** 请求 MUST 在任何账号、计费或上游副作用之前返回 503

### Requirement: 同步门禁必须覆盖所有目标协议入口
系统 SHALL 覆盖现有内容审核已接入的所有用户文本入口，并通过结构测试防止后续路由绕过。至少包括 OpenAI Chat Completions、OpenAI Responses、Claude Messages、Gemini、OpenAI Images/Grok 媒体文本 prompt，以及 Responses WebSocket 首轮和后续轮次。

#### Scenario: OpenAI 兼容 HTTP 入口
- **WHEN** 客户端调用 Chat Completions 或 Responses 兼容入口
- **THEN** 系统 MUST 使用对应协议提取器并执行同一 Guard evaluator
- **THEN** 现有 OpenAI 请求和响应 envelope MUST 保持兼容

#### Scenario: Claude 或 Gemini 入口
- **WHEN** 客户端调用 Claude Messages 或 Gemini 入口
- **THEN** 系统 MUST 执行相同策略判定
- **THEN** 拒绝响应 MUST 使用该协议现有错误 envelope 和共享稳定 error_code

#### Scenario: 新增用户文本入口
- **WHEN** 后续代码新增一个可触发模型执行且包含用户文本的路由
- **THEN** 路由覆盖门禁 MUST 在缺少安全审计接线时失败

### Requirement: 同步分片必须共享总预算并完整覆盖
系统 SHALL 以有序节点列表中首个启用节点的 timeout 作为一次同步 evaluation 的总预算。所有分片和节点故障切换 MUST 共享该 deadline；任一必要分片失败、超时或无合法结果时 MUST fail-closed。

#### Scenario: 所有分片均为安全
- **WHEN** 每个非空分片都在总预算内返回 Safe 或允许的 Warn
- **THEN** 请求 MAY 进入下一阶段

#### Scenario: 中间分片阻断
- **WHEN** 任一分片返回 Block
- **THEN** evaluator MAY 立即早停
- **THEN** 请求 MUST 被阻断且不得部分转发

#### Scenario: 最后一个必要分片失败
- **WHEN** 前面分片安全但最后一个必要分片超时或响应无效
- **THEN** 系统 MUST 返回 unavailable/invalid_response
- **THEN** 系统 MUST NOT 根据部分结果放行

### Requirement: 同步节点故障切换必须有序且 fail-closed
系统 SHALL 按配置顺序尝试启用节点。连接失败、429、5xx 和超时 MAY 在总 deadline 尚有剩余时切换到下一节点；401/403、严格解析失败或耗尽节点 MUST 结束为不可用/非法响应。同步模式 MUST NOT 提供隐式 fail-open。

#### Scenario: 首节点暂时失败而次节点成功
- **WHEN** 首节点返回可重试错误且次节点在剩余预算内返回合法结果
- **THEN** 系统 MUST 使用次节点结果
- **THEN** failover 指标 MUST 增加

#### Scenario: 认证失败
- **WHEN** 节点返回 401 或 403
- **THEN** 系统 MUST 视为不可重试配置错误
- **THEN** 请求 MUST 返回 503 而不是按 Safe 放行

#### Scenario: 所有节点容量饱和
- **WHEN** 全局或每节点 bulkhead 均无法接受 evaluation
- **THEN** 系统 MUST 快速返回 `prompt_guard_unavailable`
- **THEN** 系统 MUST 不无限排队

### Requirement: HTTP 拒绝必须保持协议兼容和稳定错误码
同步 Guard MUST 使用现有 Handler 的协议错误构造器和最小扩展，且只向客户端暴露通用消息、稳定 Prompt Guard code/reason 和 request ID。OpenAI/Claude MUST 在 error 对象的可选 `code` 字段携带稳定代码并保留原合法 type；Gemini MUST 保留数值 `error.code` 与 canonical status，并在 `google.rpc.ErrorInfo.reason` 携带稳定代码。响应 MUST 不包含风险正文、类别细节、内部节点地址或凭据。

#### Scenario: HTTP Block
- **WHEN** 同步 Guard 判定为 Block
- **THEN** HTTP 状态 MUST 为 403
- **THEN** error_code MUST 为 `prompt_guard_blocked`

#### Scenario: Gemini HTTP Block
- **WHEN** Gemini 入口的同步 Guard 判定为 Block
- **THEN** Google error envelope 的 `error.code` MUST 保持数值 403 且 status MUST 为对应 canonical status
- **THEN** `error.details` 中 ErrorInfo reason MUST 为 `prompt_guard_blocked`

#### Scenario: HTTP Guard 不可用
- **WHEN** 节点超时、连接失败、熔断或容量不足
- **THEN** HTTP 状态 MUST 为 503
- **THEN** error_code MUST 为 `prompt_guard_unavailable`

#### Scenario: HTTP Guard 响应非法
- **WHEN** Guard 输出无法严格解析
- **THEN** HTTP 状态 MUST 为 503
- **THEN** error_code MUST 为 `prompt_guard_invalid_response`

### Requirement: Responses WebSocket 必须对每个 response.create 执行门禁
系统 SHALL 在 WebSocket 首次和后续每个 `response.create` 帧进入本轮用户/账号并发、计费和上游发送之前执行同步 Guard。一次安全结果 MUST NOT 被复用于不同的后续帧。

#### Scenario: 首轮 Block
- **WHEN** 首个 response.create 被判定为 Block
- **THEN** 服务端 MUST 不建立本轮上游请求或计费记录
- **THEN** 服务端 MUST 使用 close code 4403 和 reason `prompt_guard_blocked` 关闭连接

#### Scenario: 后续轮次 Block
- **WHEN** 已建立连接的后续 response.create 被判定为 Block
- **THEN** 该帧 MUST 不发送给上游且不得创建本轮计费记录
- **THEN** 服务端 MUST 使用 4403 关闭连接并记录 stage=subsequent_turn

#### Scenario: WebSocket Guard 不可用
- **WHEN** 首轮或后续轮次 Guard 不可用或响应非法
- **THEN** 服务端 MUST 使用 close code 1013
- **THEN** reason MUST 为 `prompt_guard_unavailable` 或 `prompt_guard_invalid_response`

### Requirement: 同步结果必须复用到脱敏事件且不得重复扫描
系统 SHALL 在一次同步 evaluation 后把已得到的归一化结果交给独立记录路径。记录路径 MUST NOT 重新调用 Guard，也 MUST NOT 需要完整提示词正文；同步结果最多对应一个任务事实和一个按存储策略决定的事件。

#### Scenario: 同步 Block 被记录
- **WHEN** evaluator 已得到 Block
- **THEN** 系统 MUST 用脱敏快照和既有结果创建 done 任务及风险事件
- **THEN** Guard 调用次数 MUST 等于 evaluation 实际需要的节点/分片次数，而不是因记录而增加

#### Scenario: 同步 Allow 且不保存 Pass
- **WHEN** evaluator 得到 Allow 且 store_pass_events=false
- **THEN** 系统 MAY 只保存任务/指标而不创建 Pass 事件

### Requirement: 配置必须以版本化快照发布到请求热路径
系统 SHALL 为提示词审计配置维护单调递增 config_version、updated_at、updated_by 和 change_summary。保存后 MUST 原子替换本实例快照并通过 Redis 发布失效通知；请求热路径 MUST 读取内存快照而不是逐请求查询数据库。

#### Scenario: 多实例收到配置更新
- **WHEN** 管理员成功保存新配置
- **THEN** 保存实例 MUST 立即安装新版本并发布 Redis 失效通知
- **THEN** 其他实例 MUST 重新加载并原子替换快照

#### Scenario: 两个管理员并发保存配置
- **WHEN** 两个保存请求携带相同 expected_config_version 且第一个已提交新版本
- **THEN** 第二个请求 MUST 返回 409 `prompt_audit_config_conflict`
- **THEN** 第二个请求 MUST NOT 静默覆盖第一个请求或复用相同 config_version

#### Scenario: Redis 通知不可用
- **WHEN** 配置已保存但 Redis publish 失败
- **THEN** 系统 MUST 记录 `prompt_guard.config_reload_degraded`
- **THEN** 其他实例 MUST 通过有界 TTL 刷新最终获得新版本

#### Scenario: 冷启动无法加载严格配置
- **WHEN** 实例冷启动且无法获得有效配置快照
- **THEN** 对已知要求同步阻止的适用请求 MUST fail-closed
- **THEN** 运行态 MUST 暴露配置加载错误

### Requirement: Guard 关键路径必须可观测且不得泄密
系统 SHALL 输出稳定结构化事件并提供计数/耗时指标。日志至少 MUST 覆盖配置更新/加载/降级、evaluation 开始、Allow、Block、失败、结果记录失败、异步投递/丢弃、Worker 处理/重试/失败、逐分片开始/完成/失败、分片聚合和滞留回收。

#### Scenario: 同步请求被阻断
- **WHEN** Guard 阻断一个请求
- **THEN** 日志 MUST 包含 request_id、user_id、api_key_id、group_id、protocol、endpoint、model、config_version、guard_endpoint_id、decision、action、chunk_total、latency_ms、status 和 error_code
- **THEN** 日志 MUST 明确包含 `upstream_dispatched=false` 和 `billing_preconsumed=false` 或目标项目等价字段

#### Scenario: 检查日志敏感字段
- **WHEN** 测试捕获提示词审计日志
- **THEN** 日志中 MUST 不包含原始提示词、API Key、Authorization、完整 Guard URL query 或 Redis 载荷

### Requirement: 禁用或回滚同步阻止必须即时恢复异步行为
系统 SHALL 支持仅通过关闭 blocking_enabled 回到异步只审计，无需删除表、清空历史事件或停止现有内容审核。

#### Scenario: 管理员关闭同步阻止
- **WHEN** blocking_enabled 从 true 保存为 false 且新配置已生效
- **THEN** 后续适用请求 MUST 不再等待 Guard 同步结果
- **THEN** enabled=true 时后续请求 MUST 改为异步投递
- **THEN** 历史任务和事件 MUST 保留
