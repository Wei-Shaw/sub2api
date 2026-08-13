# 上游余额监控功能 - 需求文档

## 1. 功能概述

在管理员后台新增「上游余额监控」页面，作为左侧侧边栏的一个独立入口。管理员可以配置多个上游站点（sub2api 类型或 new-api 类型），系统定期拉取各站点的余额/配额信息并在页面中集中展示，方便管理员一目了然地掌握所有对接上游的资金状态。

---

## 2. 上游类型定义

### 2.1 Sub2API 类型

- **余额查询接口**: `GET {base_url}/v1/sub2api/billing`
- **认证方式**: `Authorization: Bearer {api_key}`
- **响应格式**:
```json
{
  "object": "sub2api.key_billing",
  "schema_version": 1,
  "billing_scope": "token",
  "group_rate_multiplier": 1.0,
  "user_rate_multiplier": 1.0,
  "resolved_rate_multiplier": 1.0,
  "peak_rate_enabled": false,
  "peak_start": "09:00",
  "peak_end": "18:00",
  "peak_rate_multiplier": 1.5,
  "applied_peak_multiplier": 1.0,
  "effective_rate_multiplier": 1.0,
  "timezone": "Asia/Shanghai",
  "observed_at": "2026-07-27T10:00:00Z"
}
```
- **关键字段说明**:
  - `effective_rate_multiplier`: 当前生效的费率倍率（含高峰时段）
  - `resolved_rate_multiplier`: 基础费率倍率
  - `peak_rate_enabled`: 是否启用高峰时段计费
  - `observed_at`: 数据观测时间

### 2.2 New-API 类型

- **余额查询接口**: `GET {base_url}/api/user/self`
- **认证方式**: `Authorization: Bearer {access_token}`
- **响应格式**:
```json
{
  "success": true,
  "message": "",
  "data": {
    "username": "user123",
    "display_name": "My Account",
    "quota": 500000,
    "used_quota": 123000,
    "request_count": 456,
    "group": "default"
  }
}
```
- **关键字段说明**:
  - `quota`: 剩余配额（原始单位，实际余额 = quota / 500000 美元）
  - `used_quota`: 已使用配额
  - `request_count`: 总请求次数
  - `group`: 所属分组

---

## 3. 数据模型

### 3.1 上游站点配置表 `upstream_balance_monitors`

| 字段 | 类型 | 说明 |
|------|------|------|
| id | int64 (PK) | 主键 |
| name | string | 站点显示名称（管理员自定义） |
| type | enum | 上游类型：`sub2api` / `newapi` |
| base_url | string | 上游站点地址（如 `https://api.example.com`） |
| api_key | string (encrypted) | 认证密钥/Token |
| enabled | bool | 是否启用监控 |
| display_order | int | 排列顺序 |
| probe_interval_minutes | int | 轮询间隔（分钟），默认 30，范围 5~1440 |
| last_probe_at | datetime | 上次探测时间 |
| last_probe_status | string | 上次探测状态：`ok` / `failed` / `pending` |
| last_probe_error | string | 上次探测错误信息 |
| snapshot_data | json | 最近一次成功获取的完整响应数据 |
| created_at | datetime | 创建时间 |
| updated_at | datetime | 更新时间 |

---

## 4. 后端 API 设计

所有接口均需管理员权限（`requiresAdmin`）。

### 4.1 CRUD 接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/admin/upstream-balance-monitors` | 获取所有监控站点列表 |
| POST | `/admin/upstream-balance-monitors` | 创建新监控站点 |
| PUT | `/admin/upstream-balance-monitors/:id` | 更新站点配置 |
| DELETE | `/admin/upstream-balance-monitors/:id` | 删除站点 |
| POST | `/admin/upstream-balance-monitors/:id/probe` | 手动触发一次探测 |
| POST | `/admin/upstream-balance-monitors/probe-all` | 手动触发所有已启用站点探测 |

### 4.2 创建/更新请求体

```json
{
  "name": "主力上游-Sub2API",
  "type": "sub2api",
  "base_url": "https://api.example.com",
  "api_key": "sk-xxxxxxxxxxxx",
  "enabled": true,
  "display_order": 1,
  "probe_interval_minutes": 30
}
```

### 4.3 列表响应体

```json
{
  "success": true,
  "data": [
    {
      "id": 1,
      "name": "主力上游-Sub2API",
      "type": "sub2api",
      "base_url": "https://api.example.com",
      "enabled": true,
      "display_order": 1,
      "probe_interval_minutes": 30,
      "last_probe_at": "2026-07-27T10:00:00Z",
      "last_probe_status": "ok",
      "last_probe_error": null,
      "balance_display": {
        "effective_rate_multiplier": 1.0,
        "peak_rate_enabled": false,
        "observed_at": "2026-07-27T10:00:00Z"
      },
      "created_at": "2026-07-01T00:00:00Z",
      "updated_at": "2026-07-27T10:00:00Z"
    },
    {
      "id": 2,
      "name": "备用上游-NewAPI",
      "type": "newapi",
      "base_url": "https://newapi.example.com",
      "enabled": true,
      "display_order": 2,
      "probe_interval_minutes": 60,
      "last_probe_at": "2026-07-27T09:30:00Z",
      "last_probe_status": "ok",
      "last_probe_error": null,
      "balance_display": {
        "quota_remaining_usd": 1.0,
        "used_quota_usd": 0.246,
        "request_count": 456,
        "group": "default"
      },
      "created_at": "2026-07-01T00:00:00Z",
      "updated_at": "2026-07-27T09:30:00Z"
    }
  ]
}
```

---

## 5. 前端页面设计

### 5.1 侧边栏入口

- **位置**: 管理员侧边栏，建议放在「Accounts」之后
- **菜单名称**: 「上游余额」或「Upstream Balance」
- **图标**: 使用钱包/余额相关图标
- **路由**: `/admin/upstream-balance`
- **权限**: `requiresAdmin: true`

### 5.2 页面布局

#### 5.2.1 顶部操作栏
- 「添加上游」按钮 → 打开创建弹窗
- 「刷新全部」按钮 → 调用 probe-all 接口
- 显示/隐藏开关（可筛选只看已启用的）

#### 5.2.2 卡片列表展示

每个上游站点以卡片形式展示，包含：

**Sub2API 类型卡片:**
```
┌─────────────────────────────────────┐
│ 🟢 主力上游-Sub2API          [编辑] │
│ 类型: Sub2API                       │
│ 地址: https://api.example.com       │
│ ─────────────────────────────────── │
│ 当前费率倍率: 1.0x                   │
│ 高峰时段: 未启用                     │
│ 最后更新: 2026-07-27 10:00          │
│ 状态: ✅ 正常    [手动刷新]          │
└─────────────────────────────────────┘
```

**New-API 类型卡片:**
```
┌─────────────────────────────────────┐
│ 🟢 备用上游-NewAPI           [编辑] │
│ 类型: New-API                       │
│ 地址: https://newapi.example.com    │
│ ─────────────────────────────────── │
│ 剩余额度: $1.00                     │
│ 已用额度: $0.25                     │
│ 请求次数: 456                       │
│ 最后更新: 2026-07-27 09:30          │
│ 状态: ✅ 正常    [手动刷新]          │
└─────────────────────────────────────┘
```

#### 5.2.3 状态指示器
- 🟢 绿色: 正常（last_probe_status = ok）
- 🟡 黄色: 余额低（new-api 余额 < 阈值，可配置）
- 🔴 红色: 探测失败（last_probe_status = failed）
- ⚪ 灰色: 未启用

#### 5.2.4 创建/编辑弹窗

表单字段：
- 站点名称（必填）
- 上游类型（下拉选择：Sub2API / New-API）
- 站点地址（必填，URL 格式校验）
- API Key / Token（必填，密码类型输入框）
- 轮询间隔（数字输入，默认 30 分钟）
- 是否启用（开关）
- 排列顺序（数字输入）

创建后立即触发一次探测以验证配置是否正确。

---

## 6. 后端定时任务

### 6.1 周期探测逻辑

复用项目已有的定时任务模式（参考 `UpstreamBillingProbeService`）：
- 启动一个后台 goroutine，每分钟检查一次是否有站点需要探测
- 对每个已启用且到达 `next_probe_at` 时间的站点发起探测
- 并发度限制: 最多 4 个并发探测
- 单次请求超时: 10 秒
- 失败后指数退避，最大延迟 24 小时

### 6.2 探测流程

```
1. 从数据库查询 enabled=true 且 next_probe_at <= now 的记录
2. 根据 type 选择对应的探测方式:
   - sub2api: GET {base_url}/v1/sub2api/billing, Bearer {api_key}
   - newapi:  GET {base_url}/api/user/self, Bearer {api_key}
3. 解析响应并更新 snapshot_data、last_probe_at、last_probe_status
4. 计算下次探测时间 = now + probe_interval_minutes + jitter
5. 探测失败时保留上次成功的 snapshot_data，记录错误信息
```

---

## 7. 技术实现指引

### 7.1 后端（Go + Gin + Ent）

1. **Schema**: 在 `backend/ent/schema/` 新建 `upstreambalancemonitor.go`
2. **Repository**: 在 `backend/internal/repository/` 新建对应 repo
3. **Service**: 在 `backend/internal/service/` 新建 `upstream_balance_monitor.go`
4. **Handler**: 在 `backend/internal/handler/admin/` 新建 `upstream_balance_monitor_handler.go`
5. **Routes**: 在 `backend/internal/server/routes/admin.go` 注册路由
6. **Wire DI**: 更新 Wire 依赖注入配置

### 7.2 前端（Vue 3 + TypeScript + TailwindCSS）

1. **API 层**: `frontend/src/api/admin/upstreamBalance.ts`
2. **页面组件**: `frontend/src/views/admin/UpstreamBalanceView.vue`
3. **子组件**:
   - `frontend/src/components/admin/upstream-balance/MonitorCard.vue`（卡片展示）
   - `frontend/src/components/admin/upstream-balance/MonitorForm.vue`（创建/编辑表单）
4. **路由**: 在 `frontend/src/router/index.ts` 添加 `/admin/upstream-balance`
5. **侧边栏**: 在 `frontend/src/components/layout/AppSidebar.vue` 的 `adminNavItems` 中添加菜单项
6. **i18n**: 在 `frontend/src/i18n/` 添加中英文翻译

---

## 8. 安全考虑

- API Key 在数据库中加密存储，列表接口返回时脱敏显示（仅显示前4后4位）
- 所有接口需验证管理员权限
- 探测请求设置合理超时（10s），防止慢响应阻塞
- base_url 需校验为合法的 HTTP/HTTPS URL，禁止内网地址（防 SSRF）
- 响应体大小限制 64KB，防止内存溢出

---

## 9. 扩展性

- `type` 字段设计为枚举，未来可扩展更多上游类型
- `snapshot_data` 使用 JSON 存储，不同类型的响应结构可灵活适配
- 预留 webhook 通知能力：余额低于阈值时可触发告警（后续迭代）
