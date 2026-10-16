# Spec Delta

## Purpose

识别并持久化每个 ChatGPT Login (Codex) OAuth 账号在上游拥有的能力：Cyber 访问项目（Daybreak 等级）与模型服务层级（Fast / Ultrafast），供路由、manifest 投影与管理端展示使用。

## ADDED Requirements

### Requirement: 能力记录由上游模型目录推导
系统 SHALL 从账号自身凭据请求的上游 Codex 模型目录推导该账号的能力记录。探测 MUST 直接请求上游而不读取 manifest 缓存，并在成功后更新该缓存。记录 MUST 按模型 slug 保存该模型的 `service_tiers` 标识列表与 `available_access_programs.cyber` 列表，并派生账号级能力集合：账号支持某 Daybreak 等级当且仅当任一模型的 cyber 列表包含该等级；账号支持某 service tier 当且仅当任一模型的 `service_tiers` 包含该 tier。记录 MUST 携带同步时间。

#### Scenario: Blue 账号与普通账号分别推导
- **WHEN** 账号 A 的目录中 `gpt-6-sol` 带 `cyber:["standard","daybreak_blue"]`、`gpt-6.1-sol` 带 `service_tiers:["priority","ultrafast"]`，账号 B 的目录中所有模型都只带 `cyber:["standard"]` 与 `service_tiers:["priority"]`
- **THEN** A 的记录派生出 Daybreak 等级集合 `{daybreak_blue}` 与 tier 集合 `{priority, ultrafast}`，B 的记录派生出空 Daybreak 等级集合与 tier 集合 `{priority}`

#### Scenario: 目录返回空模型列表
- **WHEN** 上游目录响应成功但 `models` 为空
- **THEN** 系统不覆盖该账号已有的能力记录，并将本次探测视为失败记录日志

#### Scenario: 目录响应失败
- **WHEN** 上游目录请求返回非 2xx 或网络错误
- **THEN** 系统保留已有能力记录不变

#### Scenario: 权益刚变化时探测
- **WHEN** 该账号的 manifest 缓存在 60 秒内刚被填充，随后上游撤销了其 Blue 权益并触发探测
- **THEN** 探测请求实际到达上游，记录移除 `daybreak_blue`

### Requirement: 能力记录随账号持久化且仅在变化时写入
系统 SHALL 将能力记录持久化到账号的扩展元数据中，使其在进程重启后仍可用，并对调度候选过滤可见。当新推导出的记录与已持久化记录在模型级与账号级能力上完全一致时，系统 MUST NOT 执行写入。管理端编辑账号其他字段时 MUST NOT 抹除该记录。

#### Scenario: 能力未变化
- **WHEN** 探测得到的记录与已持久化记录一致
- **THEN** 不产生数据库写入，也不触发调度快照重建

#### Scenario: 能力发生变化
- **WHEN** 探测得到的记录新增或移除了任一能力
- **THEN** 系统写入新记录，并使调度候选集合在下一次选号时反映新能力

#### Scenario: 管理员编辑账号
- **WHEN** 管理员在管理端修改账号名称或并发数并保存
- **THEN** 账号的能力记录保持不变

### Requirement: 能力记录在规定触发点刷新
系统 SHALL 在以下时刻对 OpenAI OAuth 类账号执行能力探测：账号创建完成后立即（异步）、管理端重新授权或更新凭据后（异步）、后台 token 刷新成功后、管理端账号测试、管理端「同步上游模型」操作、以及任一路径以不低于规范版本的客户端版本获取该账号完整上游目录时顺手回写。探测 MUST 使用服务端规范的 Codex 客户端版本号请求目录。

#### Scenario: 创建账号后立即探测
- **WHEN** 管理端通过 OAuth 流程或导入新建一个 OpenAI OAuth 账号
- **THEN** 系统在账号持久化后异步发起一次能力探测，不阻塞创建响应

#### Scenario: Token 刷新后探测
- **WHEN** 后台刷新任务成功刷新某账号的 access token
- **THEN** 系统在刷新后动作中对该账号执行一次能力探测

#### Scenario: 重新授权后探测
- **WHEN** 管理员对某 OAuth 账号执行重新授权并成功写入新凭据
- **THEN** 系统异步发起一次能力探测

#### Scenario: Manifest 拉取顺手回写
- **WHEN** 系统以不低于服务端规范版本的客户端版本为某账号拉取到完整上游目录
- **THEN** 系统用该目录推导记录并按「仅变化时写入」规则回写

#### Scenario: 旧版本客户端触发的拉取不回写
- **WHEN** 系统以客户端自带的、低于规范版本的 `client_version` 拉取目录
- **THEN** 不用该目录更新能力记录

#### Scenario: 部署后批量补齐
- **WHEN** 服务启动时存在没有能力记录的 OpenAI OAuth 账号
- **THEN** 系统在后台以限速方式逐个探测这些账号，直到全部拥有记录或探测失败被记录

### Requirement: 无记录账号视为不具备任何能力
当 OpenAI OAuth 类账号没有能力记录时，系统 SHALL 视其不支持任何 Daybreak 等级与任何 service tier。该规则不适用于无法探测的账号类型（API Key 及其他平台）。

#### Scenario: 尚未探测的账号
- **WHEN** 某账号从未成功探测
- **THEN** 带 Daybreak 或 tier 需求的请求不会被调度到该账号

### Requirement: 上游拒绝访问项目时重新探测
当上游以流内错误事件拒绝请求，且错误参数指向 `access_programs.cyber` 时，系统 SHALL 对所选账号异步触发一次能力重探测。同一账号在冷却窗口内 MUST 最多重探测一次。重探测 MUST NOT 改变当前请求的响应。

#### Scenario: 首帧访问项目拒绝
- **WHEN** 上游在 HTTP 200 的首个流事件返回 `error`，其 `param` 为 `access_programs.cyber`
- **THEN** 该错误照常透传给客户端，系统对该账号异步触发重探测

#### Scenario: 冷却窗口内重复拒绝
- **WHEN** 同一账号在冷却窗口内再次收到访问项目拒绝
- **THEN** 不再发起新的重探测

### Requirement: 管理端可见账号能力
管理端账号详情 SHALL 展示该账号已检测的 Daybreak 等级、service tier 与同步时间；无记录时 MUST 明确展示为未检测。

#### Scenario: 查看已探测账号
- **WHEN** 管理员打开记录含 `daybreak_blue` 与 `ultrafast` 的账号详情
- **THEN** 页面展示 Daybreak Blue 与 Ultrafast 徽标及同步时间
