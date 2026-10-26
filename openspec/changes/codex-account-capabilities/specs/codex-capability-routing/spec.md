# Spec Delta

## Purpose

在账号选择之前从请求解析出能力需求（Daybreak 等级、service tier），并只在具备对应能力的账号中进行标准调度，使客户端不会被路由到无法履行该能力的账号。

## ADDED Requirements

### Requirement: 从请求解析能力需求
系统 SHALL 在账号选择之前从请求体解析能力需求。`access_programs.cyber` 为 `daybreak_blue` 或 `daybreak_red` 时构成对应 Daybreak 等级需求；`standard` 不构成需求。`service_tier` 为 `priority` 或 `fast` 时构成 Fast 需求，为 `ultrafast` 时构成 Ultrafast 需求；`flex`、`auto`、`default`、`scale` 或缺省不构成需求。分组开启强制 Fast 时，选号需求的 tier MUST 折叠为 Fast（与转发层把 tier 改写为 `priority` 的行为一致），而分组白名单准入 MUST 以客户端原始值判定。解析 MUST 在 Responses HTTP 入口、Responses WebSocket 首帧、Chat Completions 入口与 Messages 入口执行，且凡会向上游转发 `access_programs` 的入口都 MUST 解析该字段。

#### Scenario: Codex 关闭 Daybreak 的常规请求
- **WHEN** 请求携带 `access_programs:{"cyber":"standard"}` 且无 `service_tier`
- **THEN** 不产生任何能力需求，按标准调度

#### Scenario: Daybreak 与 Ultrafast 同时请求
- **WHEN** 请求携带 `access_programs:{"cyber":"daybreak_blue"}` 与 `service_tier:"ultrafast"`
- **THEN** 需求为 Daybreak Blue 且 Ultrafast，两者必须同时满足

#### Scenario: fast 别名
- **WHEN** 请求携带 `service_tier:"fast"`
- **THEN** 需求为 Fast，与 `priority` 等价

#### Scenario: 强制 Fast 分组收到 ultrafast
- **WHEN** 分组开启强制 Fast，请求携带 `service_tier:"ultrafast"`，白名单允许 Ultrafast
- **THEN** 准入通过，选号需求为 Fast 而非 Ultrafast

#### Scenario: Chat Completions 入口携带 Daybreak
- **WHEN** `/v1/chat/completions` 收到 Responses 形态请求体且携带 `access_programs:{"cyber":"daybreak_blue"}`
- **THEN** 需求为 Daybreak Blue，与 Responses 入口同样准入与过滤

### Requirement: 按账号能力过滤候选
在现用调度路径的所有出口（`previous_response_id` 命中、guardian 父账号命中、负载感知候选循环、粘性会话命中、最佳账号回退及其新鲜度复检）中，系统 SHALL 排除能力记录不满足需求的账号。能力过滤只适用于使用 ChatGPT/Codex 协议的 OpenAI OAuth 类账号；API Key 账号与其他平台账号 MUST 不受能力需求影响，由上游自行处理。Daybreak 等级 MUST 精确匹配：Red 不满足 Blue 需求，Blue 不满足 Red 需求。过滤后 MUST 继续执行既有调度策略（优先级、负载、粘性等）。排除 MUST 以命名原因计入选号过滤统计。

#### Scenario: 混合账号池中的 Blue 请求
- **WHEN** 分组内有一个支持 Blue 的账号与两个无 Daybreak 的账号，请求需求为 Blue
- **THEN** 只有支持 Blue 的账号进入候选，再按标准策略选中

#### Scenario: Red 需求遇到 Blue 账号
- **WHEN** 请求需求为 Red，账号记录只含 `daybreak_blue`
- **THEN** 该账号被排除，原因为 Daybreak 等级不支持

#### Scenario: Ultrafast 需求遇到无 Ultrafast 账号
- **WHEN** 请求需求为 Ultrafast，账号记录的 tier 集合为 `{priority}`
- **THEN** 该账号被排除，原因为 service tier 不支持

#### Scenario: 无需求请求不受影响
- **WHEN** 请求不含任何能力需求
- **THEN** 候选集合与引入本能力前一致

#### Scenario: API Key 账号不受过滤
- **WHEN** 请求需求为 Fast，分组内有一个无能力记录的 OpenAI API Key 账号
- **THEN** 该账号不因能力需求被排除

#### Scenario: previous_response_id 命中的账号不满足需求
- **WHEN** 请求携带指向账号 A 的 `previous_response_id`，需求为 Blue，而 A 的记录不含 `daybreak_blue`
- **THEN** 系统不复用 A，释放已占用的并发槽位，继续在满足需求的账号中选择

### Requirement: 粘性会话按需求复核
当粘性会话绑定的账号不满足当前请求的能力需求时，系统 SHALL 不复用该账号，转而在满足需求的账号中选择。

#### Scenario: 会话中途开启 Daybreak（HTTP）
- **WHEN** 某会话此前绑定到无 Daybreak 账号，新请求需求为 Blue
- **THEN** 系统放弃该绑定，在支持 Blue 的账号中选择

### Requirement: 无满足需求的账号时报无可用账号
当分组内不存在任何满足需求的账号时（以忽略限流、过载等临时状态的持久账号池判定，判定语义与候选过滤完全一致，不受能力过滤的账号类型视为满足），系统 SHALL 返回与「没有可用账号」同类的错误，归类为配置类不可用，错误码 `capability_not_available`，错误消息 MUST 指明缺失的能力。当存在具备能力的账号但其全部暂时不可用（账号级限流、过载、配额暂停、failover 排除等）时，系统 MUST 沿用既有的临时不可用分类（429 / 503）。HTTP 入口（Responses、Chat Completions、Messages）返回 HTTP 404；WebSocket 首帧已完成连接升级，MUST 先写一条带该错误码与消息的 `error` 事件再以策略违规关闭连接。

#### Scenario: 分组内没有 Ultrafast 账号
- **WHEN** 请求需求为 Ultrafast，分组内所有账号记录都不含 `ultrafast`
- **THEN** 返回 404，消息指出该分组没有支持 Ultrafast 的账号

#### Scenario: 有 Ultrafast 账号但正处于账号级限流
- **WHEN** 请求需求为 Ultrafast，分组内唯一含 `ultrafast` 的账号正处于账号级限流而未进入候选集合，另一个无 Ultrafast 的账号在候选集合中
- **THEN** 返回既有的临时不可用分类错误，而不是 `capability_not_available`

#### Scenario: 唯一满足需求的 API Key 账号被限流
- **WHEN** 请求需求为 Fast，分组内支持该模型的 OpenAI API Key 账号正处于账号级限流，另一无 Fast 记录的 OAuth 账号在候选集合中
- **THEN** 返回既有的临时不可用分类错误，而不是 `capability_not_available`

#### Scenario: WebSocket 首帧无能力账号
- **WHEN** WebSocket 首帧需求为 Blue，分组内没有任何账号的记录含 `daybreak_blue`
- **THEN** 客户端收到一条 `error` 事件（`capability_not_available`）后连接被以策略违规关闭

### Requirement: 能力字段原样到达上游
系统 SHALL 在所有 OAuth 转发路径（原始透传、compact 重建、Chat Completions 与 Messages 转 Responses 的重建）中保留请求的 `access_programs` 字段并发送给上游；`service_tier` 沿用既有策略处理。

#### Scenario: compact 请求携带 Daybreak
- **WHEN** compact 路径的请求携带 `access_programs:{"cyber":"daybreak_blue"}` 并被路由到 Blue 账号
- **THEN** 发往上游的请求体仍包含该 `access_programs`

### Requirement: WebSocket 连接内不重选账号
在 Responses WebSocket 连接内，系统 SHALL 在首帧按需求选号；后续 `response.create` 帧的能力需求若超出已绑定账号的能力，系统 MUST 原样转发并让上游错误透传给客户端，不关闭连接也不重选账号。

#### Scenario: 连接中途开启 Daybreak
- **WHEN** 已绑定无 Daybreak 账号的 WebSocket 连接收到携带 `daybreak_blue` 的后续帧
- **THEN** 帧被转发到上游，上游的拒绝事件透传给客户端
