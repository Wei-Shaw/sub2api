# PR：客服工单系统（add-support-ticket-system）

> OpenSpec change：[`openspec/changes/add-support-ticket-system`](../openspec/changes/add-support-ticket-system/)
>
> Validation：`openspec validate add-support-ticket-system --strict` ✅

## 一、背景与目标

平台此前无任何站内"反馈/问题报告"通道，遇到充值失败、API Key 异常、扣费争议等问题，用户只能在 footer 找 QQ 群 / Telegram，运营无法追踪一条问题的进度、状态与解决记录。

本 PR 落地"客服三件套"中**完全独立**的第一件——**站内工单系统**：

- 用户登录态下提交结构化问题，可查看自己的工单与回复时间线。
- admin 后台过滤、回复、改 status/priority/category、关闭工单。
- 全部生命周期沉淀到 PostgreSQL，可追溯、可统计。
- 不依赖 AI / embedding / 向量库；后续"浮窗"与"RAG 知识库"在本 PR 之上叠加。

## 二、改动总览

| 模块 | 文件数 | 关键点 |
|---|---|---|
| 数据库 | 1 SQL 迁移 + 2 ent schema | `support_tickets` / `support_ticket_replies` 双表 + 3 个索引 |
| 后端 service / repo / handler | 12 个新文件 + 6 个改动 | 状态机：open → in_progress → closed；feature_disabled 拦截；admin 不卡 enabled |
| 后端配置项 | settings.go / settings_view.go | 3 项新设置：enabled / categories / default_priority |
| 前端 API 客户端 | 1 个新文件 | `frontend/src/api/support.ts` 用户 + admin 共 10 个端点 |
| 前端用户端页面 | 3 view + 2 badge 组件 | 列表 / 新建 / 详情 |
| 前端 admin 端页面 | 1 view + drawer | 6 列过滤 + BaseDialog 抽屉 + dirty-only PATCH |
| 前端 SettingsView | 1 个新卡片 + 字段 | "客服与工单" 卡片（Features Tab 下） |
| 前端 i18n | zh + en | `support.*` / `admin.tickets.*` / `admin.settings.features.support.*` / `nav.support` / `nav.adminTickets` |
| 前端单测 | 4 spec 共 16 用例 | List / New / Detail / Admin |

## 三、核心设计

### 3.1 数据库

```sql
support_tickets
  id, user_id, title, content, category, status, priority,
  chat_context (TEXT, ≤50KB), closed_at, created_at, updated_at
  INDEX (user_id, status, created_at DESC)
  INDEX (status, priority, created_at DESC)

support_ticket_replies
  id, ticket_id, author_id ON DELETE SET NULL, is_admin,
  content, created_at
  INDEX (ticket_id)
```

`chat_context` 仅用于详情场景，**列表 DTO 编译期不含**该字段（双重保险防止大字段泄露到 list 路径）。

### 3.2 状态机与权限

```
open ──(admin reply)──▶ in_progress
open / in_progress ──(close)──▶ closed (terminal)
```

- 已关闭工单：用户与 admin 均不可 append reply（HTTP 409）。
- admin PATCH `status: closed → 非 closed` 一律 409 拒绝。
- admin 操作不受 `support_ticket_enabled` 影响（关闭后仍可处理存量）。
- 用户操作受 `support_ticket_enabled` 控制：关闭时 `POST/GET /support/tickets*` 返回 404。

### 3.3 前端 feature flag

- `frontend/src/utils/featureFlags.ts` 新增 `supportTicket = defineFlag({ key: 'support_ticket_enabled', mode: 'opt-in' })`。
- 用户 sidebar / 路由通过 `flagSupportTicket` getter 自动跟随 `cachedPublicSettings`。
- admin sidebar 同样挂同一 flag —— 关闭时菜单隐藏；admin 直接打开 URL 仍可处理（与后端语义一致）。

### 3.4 chat_context 浮窗对接协议

URL 协议（与未来"客服浮窗"对接）：

```
/support/tickets/new?from=chat&session=<localStorage-key>
```

新建页 `tryHydrateFromChatContext()` 从 `localStorage[<key>]` 读取浮窗序列化好的对话快照（`Array<{role, content}>` JSON）：
- 拼成 `## 对话上下文\n...\n## 我的问题\n` Markdown 草稿填入 content
- 同时塞进隐藏字段 `chat_context` 提交到后端
- 任何失败（key 不存在 / JSON.parse 失败 / 内容空）都 silent skip + warning toast，不阻塞用户手动新建

### 3.5 admin drawer：dirty-only PATCH

`AdminSupportTicketsView` drawer 的 PATCH 表单使用 `patchOriginal` 镜像 + `patchDirty` computed，只把变化字段塞进请求体。这样一来：

- 避免触发后端 `SUPPORT_TICKET_NO_FIELDS_TO_UPDATE`
- 减少审计噪音
- save 按钮 disabled 直观反映"有无修改"

## 四、测试

- 后端 service / repo / handler 全部单测：3 个新 spec 文件共 30+ 用例（spec 内部）。
- 后端 testcontainers PG 集成测试：1 个 `support_ticket_integration_test.go`，验证状态机 + 权限边界。
- 前端 vitest：4 个 spec 共 16 用例全绿。
  - `SupportTicketsListView.spec.ts` (5)：空态 / 行渲染 / 分页交互 / 路由跳转 / 错误 toast
  - `SupportTicketNewView.spec.ts` (4)：disabled 校验 / 提交跳详情 / chat-query 注入 / localStorage 缺失降级
  - `SupportTicketDetailView.spec.ts` (3)：admin / user 时间线区分 / closed 状态 UI 锁定 / append reply 重拉
  - `AdminSupportTicketsView.spec.ts` (4)：filter 触发列表 / categories 404 降级 free-text / dirty-only PATCH / closed 状态在 table row 反映
- `npm run typecheck` (vue-tsc --noEmit) ✅
- `openspec validate add-support-ticket-system --strict` ✅

## 五、本地联调清单（Reviewer 自查）

1. 用户提交工单（带 chat_context query 与不带两条路径）
2. admin 在 drawer 内回复 → 确认用户端 status 从 `open` 翻为 `in_progress`
3. 用户追加回复 → 时间线新增条目，author 显示"用户"
4. admin 关闭工单 → 用户详情看到只读卡片 + 锁定图标，无法再回复
5. 关闭 `support_ticket_enabled`：
   - 用户 sidebar / 路由入口隐藏 ✓
   - `POST /api/v1/support/tickets` 返回 404 ✓
   - admin 仍可访问 `/admin/support/tickets` 处理存量 ✓
6. admin 改 categories 后用户新建页下拉 **立即** 更新（依赖 `cachedPublicSettings` 立即刷新）

## 六、附件

- 工单 schema 截图：`assets/support-schema.png`（待补）
- 用户端列表 / 新建 / 详情：`assets/support-user-*.png`（待补）
- admin 端列表 / drawer：`assets/support-admin-*.png`（待补）

## 七、文件清单

### 后端

```
backend/ent/schema/supportticket.go (new)
backend/ent/schema/supportticketreply.go (new)
backend/ent/supportticket/... (generated)
backend/ent/supportticketreply/... (generated)
backend/migrations/150_add_support_tickets.sql (new)
backend/internal/service/support_ticket.go (new)
backend/internal/service/support_ticket_service.go (new)
backend/internal/service/support_ticket_settings.go (new)
backend/internal/service/support_ticket_*_test.go (new)
backend/internal/repository/support_ticket_repo.go (new)
backend/internal/repository/support_ticket_repo_integration_test.go (new)
backend/internal/handler/support_ticket_handler.go (new)
backend/internal/handler/admin/support_ticket_handler.go (new)
backend/internal/handler/dto/support_ticket.go (new)
backend/internal/server/routes/support.go (new)
backend/internal/server/routes/support_ticket_integration_test.go (new)
# 改动文件：dto/settings.go、setting_handler.go、admin/setting_handler.go、
#           service/setting_service.go、settings_view.go、domain_constants.go、
#           server/router.go、routes/admin.go、wire/*.go、ent 生成文件
```

### 前端

```
frontend/src/api/support.ts (new)
frontend/src/components/support/SupportStatusBadge.vue (new)
frontend/src/components/support/SupportPriorityBadge.vue (new)
frontend/src/views/support/SupportTicketsListView.vue (new)
frontend/src/views/support/SupportTicketNewView.vue (new)
frontend/src/views/support/SupportTicketDetailView.vue (new)
frontend/src/views/admin/AdminSupportTicketsView.vue (new)
frontend/src/views/support/__tests__/*.spec.ts (new)
frontend/src/views/admin/__tests__/AdminSupportTicketsView.spec.ts (new)
# 改动文件：
#   frontend/src/api/admin/settings.ts        (+ 3 字段)
#   frontend/src/components/layout/AppSidebar.vue (+ 用户/admin 入口)
#   frontend/src/router/index.ts              (+ 4 路由)
#   frontend/src/stores/app.ts                (+ public settings 默认)
#   frontend/src/types/index.ts               (+ PublicSettings.support_ticket_enabled)
#   frontend/src/utils/featureFlags.ts        (+ supportTicket flag)
#   frontend/src/i18n/locales/{zh,en}.ts      (+ nav / support.* / admin.tickets.* / admin.settings.features.support.*)
#   frontend/src/views/admin/SettingsView.vue (+ "客服与工单" 卡片)
```

## 八、后续

- 浮窗 widget（独立 PR）：在站点全局右下角嵌入 chat 浮窗，点击 "提交工单" 时按 §3.4 协议跳到 `/support/tickets/new`。
- RAG 知识库（独立 PR）：admin 回复时基于历史工单 + 文档检索给出建议。
- 可选：admin sidebar 未读 badge（§10.4 已搁置，建议后续轮询 `GET /api/admin/support/tickets?status=open&page_size=1` 实现）。
