# Spec Delta

## Purpose

让运营者按分组限制客户端可以使用的 Codex 账号能力（Daybreak 等级与 service tier），在请求准入阶段拒绝被禁用的能力，并同步约束下发给客户端的 manifest。

## ADDED Requirements

### Requirement: 分组能力白名单配置
分组 SHALL 拥有能力白名单配置，包含：开关、允许的 Daybreak 等级集合（取值 `daybreak_blue`、`daybreak_red`）、允许的 service tier 集合（取值 `priority`、`ultrafast`）。开关关闭时 MUST 视为允许全部能力。开关开启时 MUST 只允许集合内的值，空集合表示该类能力全部禁用。`standard` 作为基线 MUST 始终允许，不出现在配置中。配置只对 OpenAI 与 Composite 平台分组生效，其他平台保存时 MUST 归零。

#### Scenario: 关闭白名单
- **WHEN** 分组白名单开关关闭
- **THEN** 任何 Daybreak 等级与 service tier 请求都不受分组限制

#### Scenario: 只允许 Fast
- **WHEN** 白名单开启，tier 集合为 `{priority}`，Daybreak 集合为空
- **THEN** 携带 `ultrafast` 或任一 Daybreak 等级的请求被拒绝，携带 `priority` 或 `standard` 的请求放行

### Requirement: 管理端校验
管理端创建与更新分组时，系统 SHALL 拒绝集合中出现未知取值的配置（HTTP 400）。白名单开启且 tier 集合不含 `priority` 而分组同时开启强制 Fast 时，系统 MUST 拒绝保存（HTTP 400）并说明冲突。

#### Scenario: 未知取值
- **WHEN** 更新请求的 Daybreak 集合包含 `daybreak_green`
- **THEN** 返回 400，原因为无效的能力白名单

#### Scenario: 与强制 Fast 冲突
- **WHEN** 分组开启强制 Fast，且白名单开启但 tier 集合不含 `priority`
- **THEN** 返回 400，说明强制 Fast 与白名单冲突

### Requirement: 请求准入拒绝
分组白名单开启时，系统 SHALL 在账号选择之前校验请求的能力需求。携带被禁用能力的请求 MUST 被拒绝，HTTP 入口返回与「没有可用账号」同类的 404 错误并指明被禁用的能力，WebSocket 首帧与后续 `response.create` 帧则以策略违规关闭连接并给出同样的消息。拒绝 MUST 标记为本地业务限制以供运维统计。

#### Scenario: HTTP 请求携带被禁用的 Ultrafast
- **WHEN** 白名单禁用 `ultrafast`，Responses 请求携带 `service_tier:"ultrafast"`
- **THEN** 返回 404，消息指出该分组不允许 Ultrafast，不发起上游请求

#### Scenario: WebSocket 后续帧携带被禁用的 Daybreak
- **WHEN** 白名单禁用全部 Daybreak 等级，已建立的连接收到携带 `daybreak_blue` 的 `response.create`
- **THEN** 连接以策略违规关闭，消息指出该分组不允许 Daybreak Blue

#### Scenario: 被允许的能力正常通过
- **WHEN** 白名单允许 `daybreak_blue`，请求携带 `daybreak_blue`
- **THEN** 请求进入能力路由与标准调度

### Requirement: 配置在所有分组加载路径可见并随复制保留
能力白名单 SHALL 出现在管理端分组响应、API Key 认证快照携带的分组对象与分组复制结果中。

#### Scenario: 复制分组
- **WHEN** 管理员复制一个开启了能力白名单的分组
- **THEN** 新分组拥有相同的白名单配置

#### Scenario: 认证快照
- **WHEN** API Key 通过认证缓存命中
- **THEN** 请求处理时读取到的分组对象包含能力白名单

### Requirement: 管理端 UI
分组创建与编辑对话框在 OpenAI 与 Composite 平台下 SHALL 展示能力白名单区块：一个总开关，开启后展示 Daybreak 等级多选（Blue、Red）与 Service Tier 多选（Fast、Ultrafast）。切换到其他平台时 MUST 隐藏该区块并清空配置。

#### Scenario: 开启后选择能力
- **WHEN** 管理员开启白名单并勾选 Fast 与 Daybreak Blue
- **THEN** 保存的配置为 tier 集合 `{priority}`、Daybreak 集合 `{daybreak_blue}`
