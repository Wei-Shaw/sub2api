## Context

Sub2API 是一个 LLM 网关，已经有完整的"用户/管理员"角色体系（`authStore.isAdmin`、`requiresAdmin: true` 路由守卫、`/api/admin/*` 路由组中间件）、ent + PostgreSQL 数据层、`internal/server/routes/*` 路由注册分组、`PublicSettings` 注入机制（参见 `public_settings_injection_schema_test.go` 的"防漂移"测试），以及 admin 后台 `SettingsView.vue` 多 tab 容器。这次工单系统是这套基建之上的一个独立 vertical slice，**不引入任何新的中间件或基建依赖**。

约束：
- 必须登录才能创建/查看工单（匿名工单等同垃圾邮件温床，已在 explore 阶段拍板）。
- 工单不做附件上传（M1 范围外，避免对象存储/虚拟扫描等附带依赖）。
- 工单不做 admin 之间的"分配"（小团队没这需求）。
- 工单系统必须能完全独立发版——不依赖后续的浮窗 / RAG change。
- Admin 工单管理菜单在 admin sidebar 一级菜单（高频运营操作），用户端入口在用户 sidebar 上（次级位置）。
- 工单系统总开关受 admin 配置 `support_ticket_enabled` 控制，关闭时所有 API 拒绝、所有入口隐藏（保持现有 `payment_enabled` 同款语义）。

## Goals / Non-Goals

**Goals：**

- 用户能在站内提交一条结构化的问题，包含分类（充值 / 账号 / API / Bug / 其他）、标题、Markdown 正文，可选地附带"AI 客服对话上下文"快照。
- 用户与 admin 能在同一条工单上多轮往返回复，每条回复带作者标记（用户 / admin）。
- Admin 能列表 / 过滤 / 分配优先级 / 关闭任意工单。
- 用户只能看自己的工单（严格 row-level 隔离）。
- 工单生命周期（`open → in_progress → closed`）由 admin 操作驱动；用户也能主动关闭自己的工单。
- 留好 `chat_context` 字段，让后续浮窗 change 落地时无需再改 schema。

**Non-Goals：**

- 不做附件上传（图片、日志文件等），M1 范围外。
- 不做 admin 间的工单分配 / 流转（assignee 字段不加）。
- 不做 SLA 计时器、自动升级、自动关闭等运营自动化。
- 不做邮件通知（项目目前没有邮件基建；如需，后续 change 单独加）。
- 不做匿名提单。未登录用户访问 `/support/tickets/*` 走标准 `requiresAuth` 重定向到 `/login`。
- 不和现有 footer `HomeContactSection`（QQ/QQ 群/Telegram）做任何耦合，那是社群联系入口，工单是站内反馈通道，两者并存、各司其职。
- 不做用户端"重开已关闭工单"——关闭即终态，需要的话用户重新提交一条新工单（admin 端可在详情页看到关联）。

## Decisions

### D1. Schema：两张表，外键软关联

```
support_tickets
  id            BIGSERIAL PRIMARY KEY
  user_id       BIGINT NOT NULL  REFERENCES users(id) ON DELETE CASCADE
  title         VARCHAR(200) NOT NULL
  content       TEXT NOT NULL                 -- 用户首次提交的正文，Markdown
  category      VARCHAR(50) NOT NULL          -- 必须在配置的 categories 集合内
  status        VARCHAR(20) NOT NULL DEFAULT 'open'
                                             -- 'open' | 'in_progress' | 'closed'
  priority      VARCHAR(20) NOT NULL DEFAULT 'normal'
                                             -- 'low' | 'normal' | 'high'
  chat_context  TEXT                          -- 可空；浮窗带过来的对话历史 JSON
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
  closed_at     TIMESTAMPTZ                   -- 仅在 status = 'closed' 时填

  INDEX (user_id, status, created_at DESC)
  INDEX (status, priority, created_at DESC)

support_ticket_replies
  id            BIGSERIAL PRIMARY KEY
  ticket_id     BIGINT NOT NULL REFERENCES support_tickets(id) ON DELETE CASCADE
  author_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE SET NULL
  is_admin      BOOLEAN NOT NULL              -- 写入时按"是否管理员路由"决定
  content       TEXT NOT NULL                 -- Markdown
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()

  INDEX (ticket_id, created_at)
```

**理由**：
- 单独的 `replies` 表而非 `messages` 数组列：标准做法，便于分页 / 排序 / 单条删除（虽然 M1 不做删除，留扩展性）。
- `is_admin` 在写入时**snapshot**用户当时的角色，不实时算 —— 避免 admin 降级用户后，老回复消失"权威标识"。
- `chat_context` 用 TEXT 而非 JSONB —— 这是个不可查询的快照字段，纯展示用，TEXT 足够且更便于跨数据库迁移。
- `closed_at` 单独存一列，不靠 `updated_at`，让"关闭时间"语义清晰；在 admin 列表的"已关闭多久"展示直接用。

### D2. 状态机：故意做得很简单

```
        ┌────────┐  admin 回复     ┌──────────────┐
        │  open  │ ───────────────▶│ in_progress  │
        └───┬────┘                 └──────┬───────┘
            │                             │
   user/admin 关闭                user/admin 关闭
            ▼                             ▼
                       ┌─────────┐
                       │ closed  │  (终态)
                       └─────────┘
```

**规则**：
- `open` → 初始态。
- `open → in_progress`：admin 第一次回复时**自动**切换。
- `in_progress → in_progress`：admin/用户继续回复，状态不变。
- `* → closed`：用户或 admin 显式调用关闭接口；同时写 `closed_at = now()`。
- `closed → *`：禁止（M1 不支持重开）。
- 已关闭工单：用户和 admin 都**不能**再追加回复（API 返回 409）。

**不引入 `pending_user_reply` / `pending_admin_reply` 这种细分状态**——M1 不需要 SLA，状态越少越省心。

### D3. 权限边界

| 路由前缀                     | 中间件                | 谁能访问                     |
| ---------------------------- | --------------------- | ---------------------------- |
| `/api/v1/support/tickets`    | `RequireAuth`         | 任何登录用户                 |
| 单个工单的 GET/PATCH         | `RequireAuth` + 行内 owner check | 工单的 `user_id` 必须等于当前用户 ID（admin 也通过此路径**只能**看自己提的） |
| `/api/admin/support/tickets` | `RequireAuth + RequireAdmin` | admin 看全部                 |

**关键决策**：admin 看自己提的工单走**用户端路由**，不走 admin 路由。这避免"admin 在用户端看不见自己的工单"这种心智割裂——工单的所有权语义不分 admin/user。

### D4. 用户端工单创建：接受 `chat_context` 但前端来源不绑定浮窗

```jsonc
POST /api/v1/support/tickets
{
  "title": "充值后余额没到账",
  "content": "...",                  // Markdown
  "category": "充值",                 // 必须在 settings.support_ticket_categories 内
  "chat_context": "<...optional...>"  // 任意文本；前端可以把浮窗对话拼成 Markdown 塞进去
}
```

**约束**：
- `title` 1..200 字符；`content` 1..20000 字符；`chat_context` 0..50000 字符（hard cap，防止有人灌一个超大 JSON）。
- 后端**不**校验 `chat_context` 的格式 —— 它就是个不透明字符串，浮窗 change 落地时再约定结构（多半是 Markdown 序列化的对话）。
- 创建成功后返回工单详情（含 id），前端拿到 id 立即跳 `/support/tickets/:id`。

### D5. 前端"从浮窗带对话过来"的工程边界

```
浮窗 (后续 change)                    工单新建页 (本 change)
─────────────                       ───────────────────────
  对话保存在                            读 query.from === 'chat'
  localStorage[支持窗会话key]    ──▶  query.session 取 key
                                        从 localStorage 取对话
                                        .map(渲染成 Markdown)
                                        填到 content 字段做草稿
                                        塞到 chat_context 隐藏字段
```

本 change 只**预留**这条接线，不依赖浮窗：
- 新建页接受 `from=chat&session=<localStorage-key>` 这两个 URL query；
- 没有这两个 query 时表现为空白草稿；
- 有这两个 query 但 localStorage 里读不到时，silent skip（仅 console.warn），仍然显示空白草稿。

这样**本 change 单独发版**时，URL 这条接线不会触发任何行为；浮窗 change 落地时无需再改新建页的代码。

### D6. Admin 列表过滤与分页

```
GET /api/admin/support/tickets?
    status=open|in_progress|closed
    & category=充值
    & priority=high
    & user_id=12
    & q=关键词       (在 title + content 模糊查 —— ILIKE，不上 FTS)
    & page=1
    & page_size=20  (max 100)
```

**理由**：
- 不上 FTS / pg_trgm —— M1 工单量预计小（< 千条），ILIKE 完全够；后续真的慢了再加 trigram 索引。
- 排序固定为 `priority DESC, created_at DESC`（高优先级 + 最新优先），不让前端传排序字段，简化。

### D7. PublicSettings 字段：只暴露开关

`support_ticket_enabled` 进 `PublicSettings`（让匿名访客也能"知道有这个功能"，虽然他们点了会被路由守卫拦回登录）；`support_ticket_categories` 与 `support_ticket_default_priority` **不**进公开设置——这两个是 admin 配置，前端通过 `/api/v1/support/categories`（一个轻量 GET）拉，已登录即可。

**理由**：
- 防止把"运营内部口径"的分类列表暴露给未登录访客；
- 给 admin 后台改完分类后立刻生效的可能（不依赖下次 SSR 注入）。

### D8. Admin 一级菜单 vs 系统设置二级 tab

```
admin sidebar:
  ├ 仪表盘
  ├ 用户
  ├ 渠道
  ├ ...
  ├ 工单                ← 新增（在"用量"之前插入）
  ├ 用量
  ├ 系统设置
  │   └ general tab
  │       └ "客服与工单"分组
  │           ├ ☐ 启用工单系统
  │           ├ 工单分类（数组编辑器）
  │           └ 默认优先级（select）
  │   └ ... 其它 tab
```

**理由**：
- 工单管理是高频运营操作 → 一级菜单。
- 工单**配置**是低频改动 → 藏在系统设置里。
- 这两个是不同心智模型，分两处合理；和"渠道（一级菜单） vs 渠道相关 settings（藏在系统设置里）"同款逻辑。

### D9. i18n 命名

- `support.tickets.*`：用户端页面（list / new / detail / status / category 等）。
- `admin.tickets.*`：admin 工单管理页面（filter / table column / status badge / reply box）。
- `admin.settings.support.*`：admin 系统设置里的"客服与工单"分组（label / hint）。

## Risks / Trade-offs

- **[admin 滥用工单查所有用户的私人内容]** → admin 本来就有 `/admin/users` 等更深入的工具，工单 admin 接口不构成新的隐私泄露面。Mitigation：admin 操作工单（回复、改状态、改优先级）写 audit log（项目里有 audit log 基建——如有就接，没有就先记 backend log，不阻塞本 change）。
- **[`q` 用 ILIKE 在 title+content 模糊查，工单量大后变慢]** → M1 工单量预估 < 1k，ILIKE 在带索引的 title 上够用；content 上的 ILIKE 会全表扫，但 EXPLAIN 看 PG 在小表上 ms 级。Mitigation：超过 5k 工单时再加 `pg_trgm` GIN 索引；不阻塞本 change。
- **[`chat_context` 可能很大，列表查询时把它一并返回拖慢响应]** → SELECT 语句**不**SELECT `chat_context` 列做列表，仅在详情页 SELECT。Mitigation：仓库层 List 方法明确 `Select(...)` 排除 `chat_context`，详情方法才 SELECT it。
- **[已关闭工单不能追加回复，但用户可能想"补充信息"]** → M1 不做重开。前端在已关闭工单详情页提示"该工单已关闭，如需追加请新建"。
- **[未启用时入口仍渲染]** → `support_ticket_enabled = false` 时，前端 sidebar 入口、admin 后台菜单项、`/support/tickets` 路由（守卫层）都隐藏；后端 API 直接 `404 Not Found`（不是 403，避免暴露功能存在）。
- **[admin 改了分类列表后，旧工单的 category 不再合法]** → category 字段不做强校验外键，只在**写入**时校验当前列表；旧工单照常显示原值。Mitigation：admin 端列表过滤的 category 下拉同时显示"配置内分类 + 历史已存在分类（用 distinct 取出来）"，避免旧值过滤不到。

## Migration Plan

1. **后端 schema 迁移**：跑 ent generate + migration，无破坏性，两张新表 + 三个新 setting key。
2. **后端 API 上线**：handler / service / repo 全跑测试通过后部署；前端没用到时 API 是空操作。
3. **前端上线**：
   - PublicSettings 注入新字段后，老前端 build 忽略新字段不影响；
   - 新前端 build 上线后 sidebar 入口、admin 菜单同时出现。
4. **首次启用**：
   - admin 在系统设置里勾选"启用工单系统"，并校验/修改默认分类；
   - 立即可用。
5. **回滚**：
   - 设置项关掉 → 前后端入口立即消失；
   - 数据保留在表里不丢；
   - 真要回滚代码的话 `git revert` 即可，schema 反向迁移**不必做**（保留两张表无害）。

## Open Questions

- **是否要给 admin 加站内通知？**——有新工单时 admin 怎么知道？M1 倾向：admin 后台 sidebar 工单菜单上加一个"未读 badge"（拉 `GET /api/admin/support/tickets/unread-count`），不做推送、不做声音、不做邮件。如果产品在 review 时认为 badge 也是 over-engineering，那就连 badge 也不做，让 admin 自己定期刷。
- **是否要支持工单内"AI 自动建议回复"？**——属于 `add-support-knowledge-rag` change 的范围（admin 在工单详情页点"AI 起草回复"），本 change 只留好 hook 不实现。
- **要不要在用户端工单详情页显示 admin 用户名/头像？**——倾向：仅显示"客服"统称（不暴露真实 admin 用户名），降低运营被骚扰风险；admin 端正常显示真实用户名。
