# 上游中转站资源池开发方案

## 目标

在不改变现有手工账号行为的前提下，新增独立的上游中转站控制模块，管理 NewAPI、Sub2API 等上游站点，并按请求协议和模型选择最低有效倍率的可用线路。

有效倍率按以下公式计算：

```text
P = X / K
```

- `X`：上游分组倍率。
- `K`：充值折算倍率，即充值 1 单位实际金额获得的站内余额；缺失时默认为 1。
- `P`：用于线路排序的有效倍率。第一阶段不把不同站点的模型基础价格并入 `P`。

## 已确认决策

1. 调用协议是主分类，模型品牌仅作为筛选项。
2. 自动创建的上游 API Key 统一使用 `sub2api-auto-*` 前缀，仅管理模块自身创建的 Key。
3. 只有普通 API Key 时进入固定线路模式：模型自动发现，分组、`X` 和 `K` 手工维护。
4. 最低成本层满载时最多等待 2 秒，随后尝试下一成本层。
5. 新的最低倍率仅影响新会话；已有 sticky session 或 `previous_response_id` 保持原线路，除非线路失败。
6. 第一阶段创建独立的自动上游分组：OpenAI、Claude、Gemini、Grok。
7. 新逻辑优先放入新增文件；现有代码只增加必要的路由、依赖注入和调度接入点。
8. 连接器、倍率快照、会话管理和同步流程参考 Upstream Hub 与 UpstreamOps，但按本项目边界重新实现。

## 范围

### V1 包含

- 上游站点 CRUD、启停、连接测试、手工同步和批量同步。
- NewAPI 与 Sub2API 站点连接器。
- 密码、用户 Token/Cookie、固定 API Key 三种凭据模式。
- 分组、倍率、余额、充值倍率和模型同步。
- 专用上游 API Key 的幂等创建与复用。
- 上游路由到现有 `Account` 的受管账号物化。
- 严格的最低成本层选择、错误下沉和恢复回切。
- 管理页面、同步日志和数据库倍率快照。

### V1 不包含

- 自动充值、兑换码和订阅购买。
- Telegram、邮件等通知渠道。
- 自动验证码求解；遇到 Turnstile 时使用用户 Token/Cookie 或固定 API Key。
- 按真实输入、输出、缓存 Token 单价进行跨站点比较。

## 隔离边界

新增后端模块位于：

```text
backend/internal/upstreamstation/
```

现有账号通过 `extra.managed_by=upstream_station_pool` 标识。同步、暂停和清理只处理带有模块标识且存在路由映射的账号，不按名称判断归属。

现有文件预计只发生以下接入性改动：

- 管理路由注册。
- Wire 依赖注入。
- 服务启动和停止时注册同步调度器。
- 通用网关与 OpenAI 网关在构造候选池后调用成本层策略。
- 前端路由、侧边栏和 i18n 注册。

## 数据模型

### upstream_stations

- 站点名称、类型、基础 URL。
- 凭据模式和 AES-GCM 加密后的凭据。
- 手工/自动充值倍率 `K`。
- 最近余额、同步时间、连接状态和脱敏错误。
- 启用和自动同步开关。

### upstream_routes

- 站点 ID、远程分组 ID/名称。
- 调用协议平台。
- 分组倍率 `X`、有效倍率 `P`。
- 模型列表、线路状态和最近测试结果。
- 上游 API Key 的远程 ID及加密后的转发 Key。
- 对应受管账号 ID。

唯一键为 `(station_id, remote_group_key, platform)`，保证重复同步幂等。

### upstream_rate_snapshots

记录每次 `X`、`K`、`P` 变化及采样时间，为自动切换提供审计依据。

### upstream_sync_logs

记录测试、同步、物化和恢复操作的结果。日志不保存密码、Token、Cookie 或完整 API Key。

## 连接器

基础连接器负责站点识别、鉴权、余额和分组；API Key 管理、充值倍率和模型发现通过可选能力接口提供。站点缺少某个能力时降级为手工输入，不影响其他能力。

### NewAPI

- `/api/status`：识别站点和读取公开设置。
- `/api/user/login`、`/api/user/self`：密码模式会话。
- `/api/user/self/groups`：用户可用分组倍率。
- `/api/token/*`：模块专用 API Key 管理。
- `/v1/models`：使用分组专用 Key 获取实际模型集合。

### Sub2API

- `/api/v1/settings/public`：识别站点。
- `/api/v1/auth/login`、`/api/v1/auth/refresh`、`/api/v1/auth/me`：用户会话。
- `/api/v1/groups/available`、`/api/v1/groups/rates`：分组和用户倍率。
- `/api/v1/payment/checkout-info`：自动读取充值倍率。
- `/api/v1/keys/*`：模块专用 API Key 管理。
- `/v1/models` 或 `/v1beta/models`：协议对应的模型发现。

## 同步流程

1. 解析并校验站点 URL。
2. 自动识别或使用手工指定的站点类型。
3. 建立或恢复用户会话。
4. 获取余额、`K`、可用分组和 `X`。
5. 为每个启用分组创建或复用 `sub2api-auto-*` API Key。
6. 使用分组 Key 获取模型并推断调用协议。
7. 计算 `P=X/K`，幂等更新线路，并在倍率变化时写入快照。
8. 创建或更新受管账号和独立本地分组绑定。
9. 使用线路 Key 读取模型列表；失败时保留旧快照、标记线路错误并暂停对应受管账号。
10. 通过现有账号管理服务更新调度快照所需的账号元数据。

同步返回空分组或模型列表时保留上一次成功快照并记录错误，不执行批量删除。

## 调度策略

手工账号不受成本层策略影响。受管账号在完成现有平台、模型、限流、配额和健康过滤后按 `P` 分层：

1. 已绑定 sticky session 时优先保持原账号。
2. 新会话从最低 `P` 层开始。
3. 同一成本层内继续使用现有负载、错误率、TTFT 和 LRU 策略。
4. 当前成本层没有可立即取得的槽位时最多等待 2 秒。
5. 超时、请求失败或账号被排除后，重新计算剩余候选的最低 `P`，自然进入下一层。
6. 倍率变化只更新新会话的选择；现有会话不主动迁移。

## 管理 API

```text
GET    /api/v1/admin/upstream-stations
POST   /api/v1/admin/upstream-stations
GET    /api/v1/admin/upstream-stations/:id
PUT    /api/v1/admin/upstream-stations/:id
DELETE /api/v1/admin/upstream-stations/:id
POST   /api/v1/admin/upstream-stations/:id/test
POST   /api/v1/admin/upstream-stations/:id/sync
POST   /api/v1/admin/upstream-stations/sync-all
GET    /api/v1/admin/upstream-stations/:id/routes
PUT    /api/v1/admin/upstream-routes/:id
POST   /api/v1/admin/upstream-routes/:id/test
POST   /api/v1/admin/upstream-routes/:id/schedulable
GET    /api/v1/admin/upstream-stations/:id/logs
```

## 管理页面

新增 `/admin/upstreams` 页面，使用与账号管理一致的紧凑表格布局：

- 站点类型、状态和关键字筛选；线路弹窗按协议展示并支持模型检索。
- 站点列表显示余额、`K`、最低 `P`、健康状态和最后同步时间。
- 展开行显示远程分组、模型数量、`X`、`P`、受管账号和测试状态。
- 编辑弹窗不回显敏感字段，只显示是否已配置。
- 日志弹窗展示同步步骤和脱敏错误；倍率变化保存在独立快照表中。

## 当前实现

截至 2026-07-15，V1 代码已按独立模块接入：

- 数据表迁移：`backend/migrations/174_upstream_station_pool.sql`。
- 连接器、同步、凭据加密、受管账号物化、管理 API 和 5 分钟自动同步：`backend/internal/upstreamstation/`。
- 成本层调度：`backend/internal/service/upstream_managed_cost.go`、`gateway_managed_cost.go`、`openai_managed_cost.go`。
- 管理页面：`frontend/src/views/admin/UpstreamStationsView.vue` 及 `frontend/src/components/admin/upstream/`。
- 受管账号通过 `managed_by=upstream_station_pool` 隔离；成本层策略仅在候选池全部属于该模块时启用。
- `Schedulable` 保存管理员开关意图，线路健康单独决定受管账号是否实际参与调度；同步恢复不会覆盖人工关闭状态。
- 固定线路的 `K` 为站点级配置，线路继承站点 `K`，避免同一余额体系出现相互矛盾的充值倍率。

数据库迁移、站点创建与远端 Key 创建属于多个系统，当前采用幂等补偿而非跨系统事务：重复同步复用 `sub2api-auto-*` Key 和既有路由；空分组响应不批量清理旧数据；单线路模型发现失败或分组消失时保留快照并退出调度。

## TDD 与验收

实现顺序为：领域计算与迁移、连接器解析、同步幂等、受管账号物化、成本层调度、管理 API、前端页面、端到端验证。

必须覆盖以下场景：

- `X=0.8`、`K=2` 时 `P=0.4`；无 `K` 时按 1 计算。
- 分组同步和专用 Key 创建可重复执行且不产生重复数据。
- 空响应不清空旧快照。
- 请求模型只由支持该模型的线路参与比较。
- 最低成本线路失败、限流或满载后进入下一层。
- 更低倍率恢复后仅新会话回切。
- sticky session 保持原账号，原账号失败时才下沉。
- 手工账号、手工 Key 和非模块本地分组不被修改。
- API 和页面响应不包含敏感凭据。

最终运行 Go 测试、lint、前端 Vitest、TypeScript 检查和构建，并使用本地模拟 NewAPI/Sub2API 验证完整流程。
