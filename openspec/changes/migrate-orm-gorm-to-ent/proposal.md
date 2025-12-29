# Proposal: 将 GORM 迁移至 Ent（保留软删除语义）

## Change ID
`migrate-orm-gorm-to-ent`

## 背景
当前后端（`backend/`）使用 GORM 作为 ORM，仓储层（`backend/internal/repository/*.go`）大量依赖字符串 SQL、`Preload`、`gorm.Expr`、`clause` 等机制。

为支持后续从 GORM 迁移到 Ent，本变更首先把“schema 管理”从 GORM AutoMigrate 切换为 **版本化 SQL migrations**（`backend/migrations/*.sql`）+ `schema_migrations` 记录表，避免 ORM 层隐式改表导致的不可审计/不可回滚问题，并确保空库可通过 migrations 重建得到“当前代码可运行”的 schema。

项目已明确：
- **生产环境依赖软删除语义**（`deleted_at` 过滤必须默认生效）。
- 更看重 **类型安全 / 可维护性**（希望减少字符串拼接与运行期错误）。

因此，本变更将数据库访问从 GORM 迁移到 Ent（`entgo.io/ent`），并用 Ent 的 **Interceptor + Hook + Mixin** 实现与现有行为一致的软删除默认过滤能力（参考 Ent 官方拦截器文档中的 soft delete 模式）。

说明：
- Ent 的拦截器/软删除方案需要在代码生成阶段启用相关 feature（例如 `intercept`），并按 Ent 的要求在入口处引入 `ent/runtime` 以注册 schema hooks/interceptors（避免循环依赖）。
- 本仓库的 Go module 位于 `backend/go.mod`，因此 Ent 生成代码建议放在 `backend/ent/`（例如 `backend/ent/schema/`），而不是仓库根目录。

落地提示：
- 入口处的实际 import 路径应以模块路径为准。以当前仓库为例，若 ent 生成目录为 `backend/ent/`，则 runtime import 形如：`github.com/Wei-Shaw/sub2api/ent/runtime`。

## 目标
1. 用 Ent 替代 GORM，提升查询/更新的类型安全与可维护性。
2. **保持现有软删除语义**：默认查询不返回软删除记录；支持显式 bypass（例如后台审计/修复任务）。
3. 将“启动时 AutoMigrate”替换为“可审计、可控的迁移流程”（第一阶段采用 `backend/migrations/*.sql` 在部署阶段执行）。
4. 保持 `internal/service` 与 handler 等上层不感知 ORM（继续以 repository interface 为边界）。

## 非目标
- 不重写业务逻辑与对外 API 行为（除必要的错误类型映射外）。
- 不强行把现有复杂统计 SQL（如 `usage_log_repo.go` 的趋势/CTE/聚合）全部改成 Ent Builder；这类保持 Raw/SQL Builder 更可控。

## 关键决策（本提案给出推荐方案）

### 1) `users.allowed_groups`：从 Postgres array 改为关系表（推荐）
现状：`users.allowed_groups BIGINT[]`，并使用 `ANY()` / `array_remove()`（见 `user_repo.go` / `group_repo.go`）。

决策：新增中间表 `user_allowed_groups(user_id, group_id, created_at)`，并建立唯一约束 `(user_id, group_id)`。

理由：
- Ent 对 array 需要自定义类型 + 仍大量依赖 raw SQL；可维护性一般。
- 关系表建模更“Ent-friendly”，查询/权限/过滤更清晰，后续扩展（例如允许来源、备注、有效期）更容易。

约束与说明：
- **不建议对该 join 表做软删除**：解绑/移除应为硬删除（否则“重新绑定”与唯一约束会引入额外复杂度）。如需审计，建议写审计日志/事件表。
- 外键建议 `ON DELETE CASCADE`（删除 user/group 时自动清理绑定关系，语义更接近当前级联清理逻辑）。

兼容策略：
- Phase 1：新增表并 **从旧 array 回填**；仓储读取改从新表，写入可短期双写（可选）。
- Phase 2：灰度确认后移除 `allowed_groups` 列与相关 SQL。

### 2) `account_groups`：保持复合主键，使用 Ent Edge Schema（推荐）
现状：`account_groups` 以 `(account_id, group_id)` 复合主键，并附带 `priority/created_at` 等额外字段（见 `account_repo.go`）。

决策：**不修改数据库表结构**，在 Ent 中将其建模为 Edge Schema（带额外字段的 M2M join entity），并将其标识符配置为复合主键（`account_id + group_id`）。

理由：
- 该表是典型“多对多 + 额外字段”场景，Ent 原生支持 Edge Schema，允许对 join 表做 CRUD、加 hooks/策略，并保持类型安全。
- 避免线上 DDL（更换主键）带来的锁表风险与回滚复杂度。
- 当前表已具备唯一性（复合主键），与 Edge Schema 的复合标识符完全匹配。

## 设计概览

### A. Ent 客户端与 DI
- 将 `ProvideDB/InitDB` 从返回 `*gorm.DB` 改为返回 `*ent.Client`（必要时同时暴露 `*sql.DB` 供 raw 统计使用）。
- `cmd/server/wire.go` 的 cleanup 从 `db.DB().Close()` 改为 `client.Close()`。

### A.1 迁移边界与命名映射（必须明确）
为保证线上数据与查询语义不变，Ent schema 需要显式对齐现有表/字段：
- **表名**：使用 `users`、`api_keys`、`groups`、`accounts`、`account_groups`、`proxies`、`redeem_codes`、`settings`、`user_subscriptions`、`usage_logs` 等现有名称（不要让 Ent 默认命名生成新表）。
- **ID 类型**：现有主键是 `BIGSERIAL`，建议 Ent 中统一用 `int64`（避免 Go 的 `int` 在 32-bit 环境或跨系统时产生隐性问题）。
- **时间字段**：`created_at/updated_at/deleted_at` 均为 `TIMESTAMPTZ`，schema 中应显式声明 DB 类型，避免生成 `timestamp without time zone` 导致行为变化。

### A.2 代码生成与 feature flags（必须写死）
建议在 `backend/ent/generate.go` 固化生成命令（示例）：
```go
//go:build ignore
package ent

//go:generate go run -mod=mod entgo.io/ent/cmd/ent generate --feature intercept --feature sql/upsert ./schema
```

说明：
- `intercept`：用于软删除的通用拦截器工具（以及未来可复用的全局 query policy）。
- `sql/upsert`：用于替代 GORM 的 `ON CONFLICT`（例如 `settings` 的 upsert）；如果短期不迁移 upsert，可暂不启用。

> 生成命令与 feature flags 必须进入 CI 校验（避免“本地生成了、CI/生产没生成”的隐性差异）。

### B. 软删除实现（必须）
对所有需要软删除的实体：
- 在 Ent schema 中通过 `Mixin` 添加 `deleted_at`（或 `delete_time`）字段。
- 通过 **Query Interceptor** 在查询阶段默认追加 `deleted_at IS NULL` 过滤（含 traversals）。
- 通过 **Mutation Hook** 处理两类行为：
  - 拦截 delete 操作，将 delete 变为 update：设置 `deleted_at = now()`。
  - 拦截 update 操作，默认追加 `deleted_at IS NULL` 过滤，避免软删除记录被意外更新（与当前 GORM 行为对齐）。
- 提供 `SkipSoftDelete(ctx)`：在需要包含软删数据的查询或需要 hard delete 的管理任务中显式使用。

**SkipSoftDelete 推荐实现**：
```go
type softDeleteKey struct{REDACTED

func SkipSoftDelete(ctx context.Context) context.Context {
    return context.WithValue(ctx, softDeleteKey{REDACTED, true)
REDACTED

func shouldSkipSoftDelete(ctx context.Context) bool {
    v, _ := ctx.Value(softDeleteKey{REDACTED).(bool)
    return v
REDACTED
```

**注意**：Ent 的“默认不更新软删记录”通常应通过 mutation hook 实现（而不是 query interceptor），否则容易出现“UpdateOneByID 仍可更新已软删记录”的行为差异。

**行为兼容性约定（建议写入测试）**：
- `Delete(id)` 对“已软删”的记录应尽量保持 **幂等**（返回成功或 rows=0，但不应抛 `NotFound` 破坏现有行为）。
- 默认查询（列表/详情/关联加载）均不应返回软删记录。
- 仅在明确管理/审计场景允许 hard delete（并且必须显式传递 `SkipSoftDelete(ctx)` 或使用专用方法）。

### B.1 Raw SQL 与事务一致性（必须遵守）
本项目存在不少事务型写操作（如 `group_repo.DeleteCascade`），并且部分逻辑使用 raw SQL（或未来保留 raw）。

规则：
- **事务内的 raw 写操作必须绑定到同一个事务**：优先使用 Ent 的 `tx.ExecContext(ctx, ...)` 执行 raw DML，确保与 Ent mutation 同一事务提交/回滚。
- 避免在事务中直接使用独立注入的 `*sql.DB` 执行写操作（会绕开事务，破坏原子性）。

### C. 仓储层迁移策略
优先改动“CRUD/关联加载明显”的仓储，复杂统计保持 raw：
1. `user_repo.go` / `api_key_repo.go` / `group_repo.go` / `proxy_repo.go` / `redeem_code_repo.go` / `setting_repo.go`
2. `account_repo.go`（JSONB merge、复杂筛选与 join 排序，部分保留 raw）
3. `user_subscription_repo.go`（原子增量、批量更新）
4. `usage_log_repo.go`（建议保留 Raw SQL，底层连接迁移到 `database/sql` 或 Ent driver）

### D. 错误映射
将 `repository/translatePersistenceError` 从 GORM error 改为：
- `ent.IsNotFound(err)` → 映射为 `service.ErrXxxNotFound`
- `ent.IsConstraintError(err)` / 驱动层 unique violation → 映射为 `service.ErrXxxExists`

同时清理所有 GORM 错误泄漏点：
- `backend/internal/server/middleware/api_key_auth_google.go` - 已修复：改为判断 `service.ErrApiKeyNotFound`（并已有单元测试覆盖）
- `backend/internal/repository/account_repo.go:50` - 需迁移：直接判断 `gorm.ErrRecordNotFound`
- `backend/internal/repository/redeem_code_repo.go:125` - 需迁移：使用 `gorm.ErrRecordNotFound`
- `backend/internal/repository/error_translate.go:16` - 核心翻译函数，需改为 Ent 错误

### E. JSONB 字段处理策略
`accounts` 表的 `credentials` 和 `extra` 字段使用 JSONB 类型，当前使用 PostgreSQL `||` 操作符进行合并更新。

Ent 处理方案：
- 定义自定义 `JSONMap` 类型用于 schema
- 对于简单的 JSONB 读写，使用 Ent 的 `field.JSON()` 类型
- 对于 JSONB 合并操作（`COALESCE(credentials,'{REDACTED') || ?`），使用 raw SQL：
  - **事务外**：使用 `client.ExecContext(ctx, ...)`（确保复用同一连接池与可观测性能力）。
  - **事务内**：使用 `tx.ExecContext(ctx, ...)`（确保原子性，不得绕开事务）。
- 或者在应用层先读取、合并、再写入（需要事务保证原子性）

### F. DECIMAL/NUMERIC 字段（必须显式确认）
当前 schema 中存在多处 `DECIMAL/NUMERIC`（例如 `users.balance`、`groups.rate_multiplier`、订阅/统计中的 cost 字段等）。GORM 当前用 `float64` 读写这些列。

第一阶段结论（兼容优先）：
- 继续使用 `float64`，并在 Ent schema 中把字段的数据库类型显式设为 Postgres `numeric(… , …)`（避免生成 `double precision`），同时接受现有的精度风险（与当前行为一致）。
- **精度优先（后续可选）**：改用 `decimal.Decimal`（或其他 decimal 类型）作为 Go 类型，以避免金额/费率累积误差；但会波及 `internal/service` 的字段类型与 JSON 序列化，属于更大范围重构。

## 数据库迁移（建议）
本仓库已存在 `backend/migrations/*.sql`，且当前数据库演进也更契合“版本化 SQL 迁移”模式，而不是在应用启动时自动改动 schema。

**决策（第一阶段）**：继续使用 `backend/migrations/*.sql` 作为唯一的版本化迁移来源；Ent 仅负责运行期访问，不在启动阶段自动改动 schema。

**可选（后续阶段）**：若团队希望更强的 schema diff/漂移检测能力，可再引入 Atlas，并与现有 SQL 迁移策略对齐后逐步迁移（但不作为第一阶段前置）。

重要现状说明（必须先处理）：
- 历史上存在“启动期 AutoMigrate + 迁移脚本覆盖不全”的混用风险：新环境仅跑 SQL migrations 可能出现缺表/缺列。
- 另一个高风险点是 SQL migrations 中的默认管理员/默认分组种子（如果存在固定密码/固定账号，属于明显的生产安全隐患），应当从 migrations 中移除，改为在安装流程中显式创建。

当前处理策略（本变更已落地的基线）：
- 通过 `backend/internal/infrastructure/migrations_runner.go` 引入内置 migrations runner（`schema_migrations` + `pg_advisory_lock`），用于按文件名顺序执行 `backend/migrations/*.sql` 并记录校验和。
- 补齐 migrations 覆盖面（新增 schema parity / legacy 数据修复迁移），确保空库执行 migrations 后即可跑通当前集成测试。
- 移除 migrations 内的默认管理员/默认分组种子，避免固定凭据风险；管理员账号由 `internal/setup` 显式创建。

第一阶段至少包含：
- 新增 `user_allowed_groups` 表，并从 `users.allowed_groups` 回填数据。
- （如需要）为所有软删表统一索引：`(deleted_at)` 或 `(deleted_at, id)`，确保默认过滤不拖慢查询。

### 迁移 SQL 草案（PostgreSQL）
> 以下 SQL 旨在让执行方案更“可落地”，实际落地时请按 `backend/migrations/*.sql` 拆分为可回滚步骤，并评估锁表窗口。

**(1) 新增 join 表：`user_allowed_groups`**
```sql
CREATE TABLE IF NOT EXISTS user_allowed_groups (
  user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  group_id   BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (user_id, group_id)
);

CREATE INDEX IF NOT EXISTS idx_user_allowed_groups_group_id
  ON user_allowed_groups(group_id);
```

**(2) 从 `users.allowed_groups` 回填**
```sql
INSERT INTO user_allowed_groups (user_id, group_id)
SELECT u.id, x.group_id
FROM users u
CROSS JOIN LATERAL unnest(u.allowed_groups) AS x(group_id)
WHERE u.allowed_groups IS NOT NULL
ON CONFLICT DO NOTHING;
```

**(3) 回填校验（建议在灰度/发布前跑一次）**
```sql
-- 旧列展开后的行数（去重后） vs 新表行数
WITH old_pairs AS (
  SELECT DISTINCT u.id AS user_id, x.group_id
  FROM users u
  CROSS JOIN LATERAL unnest(u.allowed_groups) AS x(group_id)
  WHERE u.allowed_groups IS NOT NULL
)
SELECT
  (SELECT COUNT(*) FROM old_pairs)               AS old_pair_count,
  (SELECT COUNT(*) FROM user_allowed_groups)     AS new_pair_count;
```

> Phase 2 删除 `users.allowed_groups` 列应在“代码已完全切换到新表 + 已灰度验证”之后执行，并作为单独迁移文件。

### Phase 2 清理计划（仅在灰度完成后执行）

前置条件（必须同时满足）：
- 应用侧 **读路径** 已完全从 `user_allowed_groups` 获取 allowed-groups（不再读取 `users.allowed_groups`）。
- 应用侧 **写路径** 已稳定双写/已切到只写 `user_allowed_groups`（并确认线上没有写回旧列的旧版本）。
- 运行期指标确认：allowed-groups 相关功能无报错、无权限回归（建议至少一个发布周期）。

执行步骤（建议）：
1. 先发布“只读新表 + 仍保留旧列”的版本（兼容期），并监控一段时间。
2. 发布“停止写旧列（只写 join 表）”的版本，并监控一段时间。
3. 执行独立迁移（DDL）：
   - `ALTER TABLE users DROP COLUMN allowed_groups;`
   - （可选）删除任何旧列相关的索引/约束（如果存在）。
4. 发布“移除旧列代码路径”的版本（清理遗留 SQL，例如 `ANY(allowed_groups)`/`array_remove`）。

回滚策略：
- 如果在步骤 1/2 发现功能回归，可直接回滚应用版本（DB 仍向后兼容）。
- 一旦执行步骤 3（DROP COLUMN），回滚将需要手动加回列并从 join 表回填（不推荐在线上紧急回滚时做）。

部署策略：
- 先跑 DB migration（兼容旧代码），再灰度切换 Ent 仓储。
- 保留回滚路径：feature flag 或快速回切到旧版本镜像（DB 迁移需保持向后兼容）。

## 影响范围
- 文件（预计修改）：`backend/internal/infrastructure/*`, `backend/cmd/server/*`, `backend/internal/repository/*`, `backend/internal/setup/*`, `backend/internal/server/middleware/*`
- 依赖：新增 `entgo.io/ent`、（可选）`ariga.io/atlas`/`ent/migrate`

## 风险与缓解
| 风险 | 说明 | 缓解 |
| --- | --- | --- |
| 软删除语义不一致 | Ent 默认不会自动过滤软删 | 强制使用 mixin+interceptor+hook，并加集成测试覆盖"软删不可见/可 bypass" |
| Schema 迁移风险 | `allowed_groups` 需要数据变更（array→join 表） | 迁移分两阶段；migration 保持向后兼容；灰度发布 |
| 迁移脚本缺失/漂移 | 过去依赖 AutoMigrate 演进 schema，SQL migrations 可能不完整 | 在切换前补齐 migrations；新增“迁移脚本可重建全量 schema”的 CI/集成测试校验 |
| 统计 SQL 行为变化 | 迁移连接方式后可能出现 SQL 细节差异 | `usage_log_repo` 保持原 SQL，优先做黑盒回归 |
| 性能退化 | 默认过滤 soft delete 增加条件 | 为 `deleted_at` 加索引；对热点查询做 explain/压测 |
| 集成测试中断 | 测试 harness 依赖 `*gorm.DB` 事务回滚 | 优先迁移测试基础设施，改用 `*ent.Tx` 或 `*sql.Tx` |
| JSONB 合并操作 | Ent 不直接支持 PostgreSQL `\|\|` 操作符 | 使用 `client.ExecContext/tx.ExecContext` 执行 raw SQL（事务内必须用 tx），或应用层合并 |
| 行级锁 | `clause.Locking{Strength: "UPDATE"REDACTED` 需替换 | 使用 Ent 的 `ForUpdate()` 方法 |
| Upsert 语义 | `clause.OnConflict` 的等价实现 | 使用 `OnConflict().UpdateNewValues()` 或 `DoNothing()` |

## 成功标准（验收）
1. 现有单元/集成测试通过；repository integration tests（带 Docker）通过。
2. 软删除默认过滤行为与线上一致：任意 `Delete` 后常规查询不可见；显式 `SkipSoftDelete` 可见。
3. `allowed_groups` 相关功能回归通过：查询/绑定/解绑/分组删除联动保持一致。
4. 关键读写路径（API key 鉴权、账户调度、订阅扣费/限额）无行为变化，错误类型与 HTTP 状态码保持兼容。
