## 0. 基线确认与准备
- [x] 0.1 梳理生产依赖的软删除表清单（所有带 `deleted_at` 的实体）。
- [x] 0.2 盘点所有 GORM 用法：`Preload`、`Transaction`、`Locking`、`Expr`、`datatypes.JSONMap`、`Raw` 统计 SQL。
- [x] 0.3 确认数据库为 PostgreSQL，明确迁移执行位置（部署期 vs 启动期）。
- [x] 0.3.1 **确定迁移工具链（第一阶段）**：使用 `backend/migrations/*.sql` 作为唯一迁移来源；由内置 runner 记录 `schema_migrations`（含 checksum）。
- [x] 0.3.2 **补齐迁移脚本覆盖面**：新增 schema parity/legacy 数据修复迁移，确保空库可重建并覆盖当前代码所需表/列（含 `settings`、`redeem_codes` 扩展列、`accounts` 调度字段、`usage_logs.billing_type` 等）。
- [x] 0.4 **修复现有 GORM 错误处理 bug**：`api_key_auth_google.go` 已改为判断业务错误（`service.ErrApiKeyNotFound`），并补充单元测试覆盖。

## 1. 引入 Ent（代码生成与基础设施）
- [x] 1.1 新增 `backend/ent/` 目录（schema、生成代码、mixin），配置 `entc` 生成（go generate 或 make target）。
- [x] 1.1.1 固化 `go:generate` 命令与 feature flags（`intercept` + `sql/upsert`，并指定 `--idtype int64`）。
- [x] 1.2 实现 SoftDelete mixin（Query Interceptor + Mutation Hook + SkipSoftDelete(ctx)），确保默认过滤/软删 delete 语义可用。
- [x] 1.3 改造 `backend/internal/infrastructure`：提供 `*ent.Client`；同时提供 `*sql.DB`（当前阶段通过 `gorm.DB.DB()` 暴露，供 raw SQL 使用）。
- [x] 1.4 改造 `backend/cmd/server/wire.go` cleanup：关闭 ent client。
- [x] 1.5 **更新 Wire 依赖注入配置**：更新所有 Provider 函数签名，从 `*gorm.DB` 改为 `*ent.Client`。
- [x] 1.6 在服务入口引入 `backend/ent/runtime`（Ent 生成）以注册 schema hooks/interceptors（避免循环依赖导致未注册）。
  - 代码 import 示例：`github.com/Wei-Shaw/sub2api/ent/runtime`

## 2. 数据模型与迁移（向后兼容优先）
- [x] 2.1 新增 `user_allowed_groups` 表：定义字段、索引、唯一约束；从 `users.allowed_groups` 回填数据。
- [x] 2.1.1 为 `user_allowed_groups` 编写回填校验 SQL（old_pairs vs new_pairs），并把执行步骤写入部署文档/README。
- [x] 2.1.2 设计 Phase 2 清理：在灰度完成后删除 `users.allowed_groups` 列（独立迁移文件，确保可回滚窗口足够）。
- [x] 2.2 `account_groups` 保持现有复合主键，迁移为 Ent Edge Schema（无 DB 变更）；补充校验：确保 `(account_id, group_id)` 唯一性在 DB 层已被约束（PK 或 Unique）。
- [x] 2.3 为软删除字段建立必要索引（`deleted_at`）。
- [x] 2.4 移除启动时 `AutoMigrate`，改为执行 `backend/migrations/*.sql`（对齐单一迁移来源）。
- [x] 2.5 更新安装/初始化流程：`internal/setup` 不再调用 `repository.AutoMigrate`，改为执行 `backend/migrations/*.sql`（确保新安装环境与生产迁移链路一致）。

## 3. 仓储层迁移（按风险分批）

### 3.A 低风险仓储（优先迁移，用于验证 Ent 基础设施）
- [x] 3.1 迁移 `setting_repo`：简单 CRUD + upsert（Ent `OnConflictColumns(...).UpdateNewValues()`）。
- [x] 3.2 迁移 `proxy_repo`：CRUD + 软删除 + 账户数量统计（统计保持 raw SQL，proxy 表读写改为 Ent）。

### 3.B 中等风险仓储
- [x] 3.3 迁移 `api_key_repo`：关联 eager-load（`WithUser`、`WithGroup`），错误翻译为业务错误。
- [x] 3.4 迁移 `redeem_code_repo`：CRUD + 状态更新。
- [x] 3.5 迁移 `group_repo`：事务、级联删除逻辑（可保留 raw SQL，但必须在 ent Tx 内执行，例如 `tx.ExecContext`，避免绕开事务）。
  - 迁移 `users.allowed_groups` 相关逻辑：在删除分组时改为 `DELETE FROM user_allowed_groups WHERE group_id = ?`

### 3.C 高风险仓储
- [x] 3.6 迁移 `user_repo`：CRUD、分页/过滤、余额/并发原子更新（`gorm.Expr`）；allowed groups 改为 join 表实现。
  - 替换 `ANY(allowed_groups)`/`array_remove` 语义：改为对 `user_allowed_groups` 的 join/filter/delete
  - 覆盖 `RemoveGroupFromAllowedGroups`：改为 `DELETE FROM user_allowed_groups WHERE group_id = ?` 并返回 rowsAffected
- [x] 3.7 迁移 `user_subscription_repo`：批量过期、用量增量更新（`gorm.Expr`）、关联预加载。
- [x] 3.8 迁移 `account_repo`：join 表排序、JSONB merge（写操作优先用 `client.ExecContext/tx.ExecContext` 执行 raw SQL）；校验 bulk update 的 rowsAffected 语义一致。

### 3.D 保留 Raw SQL
- [x] 3.9 `usage_log_repo` 保留原 SQL：底层改为注入/获取 `*sql.DB` 执行（例如 infrastructure 同时提供 `*sql.DB`）。
  - 识别可用 Ent Builder 的简单查询（如 `Create`、`GetByID`）
  - 保留 CTE/聚合等复杂 SQL（趋势统计、Top N 等）

## 4. 错误处理与边角清理
- [x] 4.1 替换 `repository/error_translate.go`：用 `ent.IsNotFound/IsConstraintError` 等映射。
- [x] 4.2 清理 GORM 泄漏点：
  - [x] `middleware/api_key_auth_google.go` - 已修复：从 `gorm.ErrRecordNotFound` 判断迁移为业务错误判断
  - [x] `repository/account_repo.go:50` - 直接判断 `gorm.ErrRecordNotFound`
  - [x] `repository/redeem_code_repo.go:125` - 使用 `gorm.ErrRecordNotFound`
- [x] 4.3 检查 `internal/setup/` 包是否有 GORM 依赖。
- [x] 4.4 检查 `*_cache.go` 文件是否有潜在 GORM 依赖。

## 5. 测试与回归
- [x] 5.1 **迁移测试基础设施**（优先级高）：
  - [x] **建表策略对齐生产（GORM 阶段）**：在 Postgres testcontainer 中执行 `backend/migrations/*.sql` 初始化 schema（不再依赖 AutoMigrate）。
  - [x] 增加“schema 对齐/可重建”校验：新增集成测试断言关键表/列存在，并验证 migrations runner 幂等性。
  - [x] 为已迁移仓储增加 Ent 事务测试工具：使用 `*sql.Tx` + Ent driver 绑定到同一事务，实现按测试用例回滚（见 `testEntSQLTx`）。
  - [x] 更新 `integration_harness_test.go`：从 `*gorm.DB` 改为 `*ent.Client`
  - [x] 更新 `IntegrationDBSuite`：从 `testTx()` 返回 `*gorm.DB` 改为 `*ent.Tx` 或 `*sql.Tx`
  - [x] 确保事务回滚机制在 Ent 下正常工作
- [x] 5.2 新增软删除回归用例：
  - delete 后默认不可见
  - `SkipSoftDelete(ctx)` 可见
  - 重复 delete 的幂等性（不应引入新的 `NotFound` 行为）
  - hard delete 可用（仅管理场景）
- [ ] 5.3 跑全量单测 + 集成测试；重点覆盖：
  - API key 鉴权
  - 订阅扣费/限额
  - 账号调度
  - 统计接口

## 6. 收尾（去除 GORM）
- [x] 6.1 移除 `gorm.io/*` 依赖与相关代码路径。
- [x] 6.2 更新 README/部署文档：迁移命令、回滚策略、开发者生成代码指引。
- [x] 6.3 清理 `go.mod` 中的 GORM 相关依赖：
  - `gorm.io/gorm`
  - `gorm.io/driver/postgres`
  - `gorm.io/datatypes`

## 附录：工作量参考

| 组件 | 代码行数 | GORM 调用点 | 复杂度 |
|------|---------|------------|--------|
| 仓储层总计 | ~13,000 行 | （待统计） | - |
| Raw SQL | - | （待统计） | 高 |
| gorm.Expr | - | （待统计） | 中 |
| 集成测试 | （待统计） | - | 高 |

**建议迁移顺序**：
1. 测试基础设施（5.1）→ 确保后续迁移可验证
2. 低风险仓储（3.1-3.2）→ 验证 Ent 基础设施
3. 中等风险仓储（3.3-3.5）→ 验证关联加载和事务
4. 高风险仓储（3.6-3.8）→ 处理复杂场景
5. 错误处理清理（4.x）→ 统一错误映射
6. 收尾（6.x）→ 移除 GORM
