## ADDED Requirements

### Requirement: 账号代理取值必须收敛到单一领域方法

系统 SHALL 提供 `Account.ProxyURL()` 作为账号出站代理 URL 的唯一取值入口。所有上游请求构造点 MUST 通过该方法取代理，MUST NOT 内联判断 `account.Proxy` / `account.ProxyID` 后自行调用 `Proxy.URL()`。

#### Scenario: 账号未绑定任何代理

- **WHEN** 账号既无 `proxy_id` 也无 `proxy_group_id`
- **THEN** `ProxyURL()` MUST 返回空串
- **THEN** 上游请求 MUST 直连

#### Scenario: 账号绑定单个代理

- **WHEN** 账号 `proxy_id` 非空且代理已 hydrate
- **THEN** `ProxyURL()` MUST 返回该代理的 URL
- **THEN** 结果 MUST 与本变更前的行为逐字节一致

#### Scenario: 空账号指针

- **WHEN** 接收者为 nil
- **THEN** `ProxyURL()` MUST 返回空串且 MUST NOT panic

### Requirement: 代理组绑定不得改变既有单代理语义

系统 SHALL 支持账号通过 `proxy_group_id` 绑定一个代理组。`proxy_id` 与 `proxy_group_id` MUST 可共存，且 `proxy_id` MUST 具有更高优先级。存量账号 MUST 不需要数据迁移。

#### Scenario: 仅绑定 proxy_id

- **WHEN** 账号 `proxy_id` 非空
- **THEN** 系统 MUST 使用该代理
- **THEN** 系统 MUST NOT 执行任何组选择逻辑

#### Scenario: 同时绑定 proxy_id 与 proxy_group_id

- **WHEN** 两者均非空
- **THEN** 系统 MUST 使用 `proxy_id` 指向的代理
- **THEN** 组 MUST 被忽略

#### Scenario: 仅绑定 proxy_group_id

- **WHEN** `proxy_id` 为空且 `proxy_group_id` 非空
- **THEN** 系统 MUST 从组内按策略选出一个代理并填入 `account.Proxy`

### Requirement: 代理选择必须发生在账号 hydration 阶段

系统 SHALL 在账号 hydration 的统一入口完成组内代理选择，并把结果体现在 `account.Proxy` 上。上游消费点 MUST NOT 感知代理组的存在。

#### Scenario: 单次请求内代理稳定

- **WHEN** 一次请求内多次读取账号代理
- **THEN** 每次 MUST 得到同一个代理

#### Scenario: WebSocket 长连接

- **WHEN** 账号绑定代理组并建立 WebSocket 上游连接
- **THEN** 该连接生命周期内 MUST 始终使用同一个出口代理
- **THEN** 系统 MUST NOT 逐帧或逐轮重新选择

#### Scenario: 跨请求轮换

- **WHEN** 组策略为 `round_robin` 且组内有多个健康代理
- **THEN** 连续多次请求 MUST 分布到不同代理

### Requirement: 代理选择 MUST NOT 写回 account.ProxyID

系统 MUST 将 `account.ProxyID` 在代理组链路中视为只读。选择结果 MUST NOT 以任何形式（持久化或仅内存）写入 `ProxyID`。

#### Scenario: grok OAuth token 刷新期间发生代理选择

- **WHEN** grok 账号绑定代理组并触发 OAuth token 刷新
- **THEN** `grok_token_provider` 的代理一致性 CAS MUST NOT 报 `errOAuthRefreshAccountStateChanged`
- **THEN** 刷新 MUST 正常完成

#### Scenario: 同一账号两次 hydration 选出不同代理

- **WHEN** `round_robin` 策略导致两次 hydration 结果不同
- **THEN** 两次得到的 `account.ProxyID` MUST 相等（均为绑定值或均为 nil）

### Requirement: 选择器必须在候选阶段剔除不可用代理

系统 SHALL 在选择前过滤候选集，剔除 `status != active` 与已过期的代理。系统 MUST NOT 依赖 `SweepExpiredProxies` 为代理组账号提供可用性兜底。

#### Scenario: 组内存在已过期代理

- **WHEN** 组内部分代理 `expires_at` 已过
- **THEN** 这些代理 MUST NOT 被选中
- **THEN** 请求 MUST 使用组内其余健康代理

#### Scenario: 组内存在停用代理

- **WHEN** 组内部分代理 `status` 非 active
- **THEN** 这些代理 MUST NOT 被选中

#### Scenario: 组内全部代理不可用

- **WHEN** 过滤后候选集为空
- **THEN** 选择器 MUST 返回未命中信号而非任意代理
- **THEN** 系统 MUST 按配置降级并记录结构化告警

### Requirement: 代理选择策略必须可配置且可预测

系统 SHALL 支持 `round_robin`、`random`、`sticky` 三种策略，并 SHALL 提供 `sticky_by_account` 开关。选择器 MUST 实现为无 I/O 纯函数以保证可测性。

#### Scenario: sticky 策略下同账号恒定

- **WHEN** 策略为 `sticky` 且候选集不变
- **THEN** 同一账号 ID 的多次选择 MUST 命中同一代理
- **THEN** 不同账号 MUST 尽可能分散到不同代理

#### Scenario: sticky 策略下候选集变化

- **WHEN** 候选集因成员增删或代理过期而变化
- **THEN** 系统 MAY 为该账号重新映射到另一代理

#### Scenario: 未知策略值

- **WHEN** 组的 `strategy` 为不支持的值
- **THEN** 系统 MUST 回退到 `round_robin` 并记录告警

### Requirement: grok 出站与 OAuth 刷新必须同出口

系统 SHALL 保证 grok 账号的 OAuth token 刷新与中继请求使用同一出口代理。

#### Scenario: 代理组账号刷新 grok token

- **WHEN** grok 账号绑定代理组且 `proxy_id` 为空
- **THEN** OAuth 刷新 MUST 使用 hydration 选出的同一代理
- **THEN** OAuth 刷新 MUST NOT 退化为直连

#### Scenario: 管理端显式指定代理的 OAuth 流程

- **WHEN** 管理端在 OAuth 导入或授权流程中显式传入 proxyID
- **THEN** 系统 MUST 使用该显式代理
- **THEN** 组选择 MUST NOT 介入

### Requirement: 代理组管理 API 必须独立且不与代理路由冲突

系统 SHALL 在独立顶层路由组下提供代理组管理能力。系统 MUST NOT 将组路由挂载为现有代理路由的子路径。

#### Scenario: 路由注册

- **WHEN** 服务启动
- **THEN** 组管理路由 MUST 位于 `/api/v1/admin/proxy-groups`
- **THEN** 现有 `/api/v1/admin/proxies/:id` MUST 继续正常匹配

#### Scenario: 删除仍被引用的组

- **WHEN** 组内仍有代理成员或仍有账号绑定该组
- **THEN** 系统 MUST 拒绝删除并返回 `PROXY_GROUP_IN_USE`

### Requirement: 代理组不得放大连接池开销

系统 SHALL 保证代理组不会导致上游 HTTP 客户端被反复销毁重建。

#### Scenario: 隔离模式为 account

- **WHEN** 上游客户端隔离模式为 `account` 且账号绑定代理组
- **THEN** 系统 MUST 拒绝该组合或明确告警
- **THEN** 系统 MUST NOT 静默地每请求重建 Transport

#### Scenario: 隔离模式为 proxy 或 account_proxy

- **WHEN** 隔离模式为 `proxy` 或 `account_proxy`
- **THEN** 不同代理 MUST 落入不同客户端缓存键并各自复用连接池

### Requirement: 组成员查询必须走缓存

系统 SHALL 缓存组成员列表，MUST NOT 在每次账号 hydration 时查询数据库，且 MUST 在成员变更时使缓存失效。

#### Scenario: 批量 hydration

- **WHEN** 一次请求批量 hydrate 多个绑定同组的账号
- **THEN** 系统 MUST NOT 产生 N+1 查询

#### Scenario: 组成员变更

- **WHEN** 管理员增删组内代理或修改代理状态
- **THEN** 相关缓存 MUST 失效
- **THEN** 后续请求 MUST 反映新的候选集
