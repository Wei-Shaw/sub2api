## Why

当前仓库不存在「代理池 / 代理分组」实体。代理模型是账号 1:1 绑定单个代理（`accounts.proxy_id` 单值外键，`backend/ent/schema/account.go:91`），加上一条代理过期后的持久化回退链（`fallback_mode` + `backup_proxy_id`，`backend/ent/schema/proxy.go:55-66`）。

这带来两个问题：

1. **无法多出口轮换**。单账号只有一个固定出口 IP，无法通过多代理分摊风控压力，也无法在单代理临时故障时自动切到同组其他出口。现有 `SweepExpiredProxies`（`backend/internal/repository/proxy_repo.go:614-668`）只在代理**过期**时改写 `accounts.proxy_id`，是写路径的批量后台作业，不是请求级的可用性兜底。
2. **该缺失是全平台的，不是 grok 特有的**。grok 的代理链路本身已完整（中继、媒体、配额、OAuth 全部读取账号绑定代理），且比多数平台更严格（`grok_token_provider.go` 有代理一致性 CAS 校验）。因此不应为 grok 单开开关，而应补齐全局能力。

本变更以「不改变既有单代理语义、不改变 `accounts.proxy_id` 的读写契约」为前提，新增并列的代理组绑定方式。

## What Changes

- 新增 `proxy_groups` 实体与管理 API，支持组的增删改查、成员管理和组级连通性测试。
- 新增代理选择策略：`round_robin`、`random`、`sticky`（按账号哈希取模，同账号恒定出口）。
- 账号新增 `proxy_group_id` 绑定字段，与既有 `proxy_id` **共存且 `proxy_id` 优先**；存量账号零迁移、零行为变化。
- 在账号 hydration 的唯一入口（`backend/internal/repository/account_repo.go:264` 的 `WithProxy()` 分支）完成组内选择，把结果填入 `account.Proxy`，使 58 处上游消费点无需感知代理池的存在。
- 选择器在候选过滤阶段剔除 `status != active` 与已过期代理，作为池账号的运行时可用性兜底。
- 新增 `Account.ProxyURL()` 领域方法，统一 58 处分散的代理取值写法（Phase 0，纯重构）。
- grok 专项：OAuth 刷新路径改为复用已 hydrate 的 `account.Proxy`，保证刷新与中继同出口 IP。
- 前端新增代理组管理页与账号编辑中的组选择器。
- 不删除、不迁移、不重命名 `proxies` 表及其过期回退机制；`proxy_id` 绑定的账号行为完全不变。

## Capabilities

### New Capabilities

- `proxy-group-pool`: 定义代理组实体、成员关系、选择策略、hydration 注入点、可用性过滤、连接池隔离约束、grok 出口一致性不变量和管理 API。

### Modified Capabilities

无。仓库当前没有已发布的 OpenSpec capability；现有单代理绑定与过期回退行为在本变更中作为兼容基线，语义不变。

## Impact

- **后端领域层**：`service/account.go` 新增 `ProxyURL()` 方法；新增 `service/proxy_group.go`、`service/proxy_selector.go`。
- **后端消费点**：58 处（43 个文件）机械替换为 `account.ProxyURL()`。已证明为零行为变化，见 design.md「不变量证明」。
- **数据库**：新增 `proxy_groups` 表；`proxies` 加 `group_id`；`accounts` 加 `proxy_group_id`。不修改既有列。
- **DI**：新增 repository / service / handler 三处 wire provider，需重跑 `make generate`。
- **管理 API**：新增顶层路由组 `/api/v1/admin/proxy-groups`（不可挂在 `/proxies/` 下，见 design.md 路由冲突约束）。
- **前端**：新增 `api/admin/proxyGroups.ts`、`views/admin/ProxyGroupsView.vue`；修改 `ProxySelector.vue`、`EditAccountModal.vue`、路由与 i18n。
- **兼容性**：无外部 API breaking change。新能力默认不启用（账号不绑定组即维持原行为）。
- **性能**：组成员列表位于每请求热路径，必须缓存并由变更事件失效；连接池客户端数量随池大小放大，`maxUpstreamClients` 上限需同步上调。
- **风险**：错误的实现方式（把选中代理写回 `account.ProxyID`）会导致 grok OAuth token 刷新全面失败，见 design.md 约束 C1，属于必须在实现和评审中显式守住的红线。

## Execution References

- `design.md`：四条硬约束的实测证据、架构决策与被否决方案。
- `specs/proxy-group-pool/spec.md`：Requirement 与 Scenario。
- `tasks.md`：按 Phase 拆分的可勾选实施清单。
- `verification.md`：证据矩阵与回滚手册。
