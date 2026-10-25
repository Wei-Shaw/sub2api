# MySQL 迁移翻译规则（PostgreSQL → MySQL 8.0.13+）

本目录（`backend/migrations/mysql/`）是上级目录 PostgreSQL 迁移的**同名镜像翻译**。
每个 `NNN_xxx.sql` 必须与 `../NNN_xxx.sql` 一一对应，文件名完全相同。

维护约定：
- 上游新增 PG 迁移时，必须同步翻译对应的 MySQL 版本。
- PG 版文件的内容 checksum 保护不适用于本目录（方言不同，checksum 必然不同）。
- 语义对等优先：翻译后的行为（列类型精度、默认值、约束）必须与 PG 版一致。

## 类型映射

| PostgreSQL | MySQL 8 |
|---|---|
| `BIGSERIAL` / `SERIAL` | `BIGINT NOT NULL AUTO_INCREMENT`（必须作为主键或唯一键；`BIGSERIAL PRIMARY KEY` → `BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY`） |
| `TIMESTAMPTZ` / `TIMESTAMP` | `DATETIME(3)`（毫秒精度；可空写成 `DATETIME(3) NULL`） |
| `DEFAULT NOW()` | `DEFAULT CURRENT_TIMESTAMP(3)` |
| `JSONB` | `JSON`（`DEFAULT '{}'` → `DEFAULT ('{}')`，8.0.13+ 支持表达式默认值） |
| `BOOLEAN` | `BOOLEAN`（MySQL 别名，实际 TINYINT(1)） |
| `BIGINT[]` / `TEXT[]` / 任何数组类型 | `JSON`（种子数据 `'{a,b}'` → `'["a","b"]'`） |
| `TEXT` / `VARCHAR(n)` / `DECIMAL(p,s)` / `INT` / `BIGINT` | 原样保留 |
| `bytea` / `CREATE TYPE` / ENUM | 本项目未使用 |

所有 `CREATE TABLE` 显式追加 `ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`。

## 标识符

- `groups` 是 MySQL 8 保留字（窗口函数 GROUPS），表名和列引用一律加反引号 `` `groups` ``。
- 其他常见保留字（`rank`、`system`、`lateral`、`rows`、`groups`）作为标识符出现时同样加反引号。
- FK 约束必须显式命名（`CONSTRAINT fk_xxx FOREIGN KEY ...`），PG 的内联 `REFERENCES` 在 MySQL 同样合法，但为便于后续 `DROP FOREIGN KEY`，统一命名。

## 索引

- MySQL 的 `CREATE INDEX` **不支持** `IF NOT EXISTS`/`CONCURRENTLY`：直接去掉这两个修饰词（迁移执行器保证每份迁移只执行一次）。
- `INCLUDE (col)`（PG 覆盖列）→ 删除该子句。
- `USING gin/gin_trgm_ops/BRIN`、`CREATE EXTENSION pg_trgm`、`to_tsvector` 表达式索引 → 一律改为普通 B-tree 索引（`CREATE INDEX name ON t(col)`）。
- 表达式索引：`(extra->>'k')` → `((JSON_UNQUOTE(JSON_EXTRACT(extra, '$.k'))))`（注意双括号）。
- **部分索引（`WHERE <predicate>`）** 是最重要的规则：
  - 谓词只含 `col IS NOT NULL` → 直接建普通唯一索引（MySQL 唯一索引天然允许多个 NULL）。
  - 其他谓词 → 追加函数键部分：`CREATE UNIQUE INDEX x ON t(<key_cols>, (IF(<predicate>, 0, NULL)))`。
    例：`... ON groups(name) WHERE deleted_at IS NULL` →
    `CREATE UNIQUE INDEX x ON \`groups\`(name, (IF(deleted_at IS NULL, 0, NULL)));`
    谓词为真时键为 `(..., 0)`（参与唯一性），为假时键含 NULL（MySQL 唯一索引不约束 NULL）。
  - 非唯一的部分索引 → 直接去掉 WHERE（普通索引，仅性能差异）。
- `DESC` 排序键：MySQL 8 支持，保留。

## DDL 语句

- `ADD COLUMN IF NOT EXISTS` / `DROP COLUMN IF EXISTS` / `DROP CONSTRAINT IF EXISTS` / `DROP INDEX IF EXISTS` → 去掉 `IF EXISTS`/`IF NOT EXISTS`。
- `ALTER COLUMN x TYPE <type> [USING expr]` → `MODIFY COLUMN x <完整列定义>`。
  **必须**回溯该列的原始定义（查 `001_init.sql` 或更早迁移）补全 NOT NULL/DEFAULT，不能只写类型。
- `ALTER COLUMN x SET/DROP DEFAULT` → 语法相同，可用。
- `DROP CONSTRAINT name`：按约束类型改写——FK → `DROP FOREIGN KEY name`；唯一键 → `DROP INDEX name`；主键 → `DROP PRIMARY KEY`。
- `ADD CONSTRAINT ... UNIQUE/PRIMARY KEY/FOREIGN KEY` → 同名保留。
- `RENAME COLUMN a TO b` → MySQL 8 支持，保留。
- `COMMENT ON COLUMN/TABLE` → **整句跳过**（MySQL 需要整列定义重建，风险大于收益），
  在原位置留一行 `-- (comment skipped)`。
- `setval`/`nextval`/序列操作 → **整句跳过**（InnoDB AUTO_INCREMENT 会自动越过显式插入的大 id，无需重置）。
- `CREATE EXTENSION` → 整句跳过。

## DML 与数据回填

- `ON CONFLICT (cols) DO NOTHING` → `ON DUPLICATE KEY UPDATE id = id`（表有 id 列时；否则用首列自赋值）。
- `ON CONFLICT (cols) DO UPDATE SET x = EXCLUDED.y` → `ON DUPLICATE KEY UPDATE x = VALUES(y)`。
  若冲突目标是部分唯一索引，MySQL 侧按本目录的函数唯一索引语义处理，必要时改写为
  `INSERT ... SELECT ... WHERE NOT EXISTS (...)` 模式。
- `UPDATE t SET ... FROM (SELECT ...) prev WHERE ...` → `UPDATE t JOIN (SELECT ...) prev ON ... SET ...`；
  CTE（`WITH s AS (...) UPDATE ...`）MySQL 8 支持，可保留。
- `DELETE FROM t USING a JOIN ...` → `DELETE t FROM t JOIN a ...`（多表删除）。
- `INSERT ... SELECT ... RETURNING ...` → 去掉 RETURNING 并按需改写（极少数文件，见单文件指引）。
- 字符串拼接 `||` → `CONCAT()`。
- `ILIKE` → `LIKE`。
- `x::type` → `CAST(x AS <type>)`（`::numeric` → `CAST(x AS DECIMAL(20,10))` 或与目标列一致的精度）。
- `now() - interval '5 minutes'` → `NOW(3) - INTERVAL 5 MINUTE`。
- `to_char(ts, 'YYYY-MM-DD')` → `DATE_FORMAT(ts, '%Y-%m-%d')`；`date_trunc('day', ts)` → `DATE(ts)` 等价改写。
- `AT TIME ZONE 'Asia/Shanghai'` → `CONVERT_TZ(ts, '+00:00', '+08:00')`（按语义确定方向）。
- `gen_random_uuid()` → `UUID()`；`to_jsonb(x)` → `CAST(x AS JSON)` / `JSON_OBJECT(...)` 按语义。
- `GREATEST/LEAST`：注意 MySQL 任一参数为 NULL 时返回 NULL（PG 忽略 NULL），必要时用 COALESCE 包裹。
- `$1` 占位符（迁移内罕见）→ `?`。

## DO $$ 匿名块（29 个文件）

- 优先**理解意图后改写为普通语句**：多数 DO 块是「条件存在性检查 + DDL/DML」，而迁移执行器
  保证单次执行，因此存在性分支通常可以去掉、直接执行语句体。
- `information_schema` 查询：`table_schema = 'public'` → `table_schema = DATABASE()`。
- 确实需要循环/过程逻辑（数据回填 WHILE 等）→ 用临时存储过程：
  `CREATE PROCEDURE __mig_tmp() BEGIN ... END; CALL __mig_tmp(); DROP PROCEDURE __mig_tmp;`
  注意 MySQL 服务器端解析器能正确处理过程体内的分号，**不要**写 `DELIMITER`（那是 mysql CLI 指令）。
- `RAISE EXCEPTION` / `RAISE NOTICE` → 过程内用 `SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = '...'`；日志型 NOTICE 可跳过。
- PL/pgSQL 变量 → 过程内 `DECLARE`。

## 触发器/函数（8 个文件：115, 116, 175, 184, 186, 193, 222, 223）

- `CREATE OR REPLACE FUNCTION` + `CREATE TRIGGER` → MySQL 触发器语法重写。
- MySQL 无语句级触发器/transition table → 行级触发器 + 早退条件（`IF <不相关> THEN LEAVE/RETURN` 语义，
  用 `IF ... THEN ... END IF;` 包裹全部逻辑实现 no-op 快速路径）。
- `IS DISTINCT FROM` → `<=>` 取反；`sha256(bytea)` → `SHA2(x, 256)`；`to_jsonb` → `CAST/JSON_OBJECT`。
- 同名函数/触发器重复定义（184→186→193 演进链）→ 每个文件先 `DROP TRIGGER IF EXISTS` 再 `CREATE TRIGGER`。
- 触发器内禁止直接对**本表**读写（MySQL 限制，错误 1442）——PG 版若在触发器里 UPDATE 本表，需要重新设计（见单文件指引）。
