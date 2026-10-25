# 仓库层 SQL 方言化规范（PostgreSQL / MySQL 双方言）

项目现支持双数据库（PG 默认，MySQL 8.0+）。本文件规定 Go 仓库层手写 SQL 的方言分支写法。
**铁律：PostgreSQL 分支的行为与字节必须保持原样（默认 `IsMySQL()==false` 时走原路径）。**

## 分支结构

- 判定：`IsMySQL()`（`dialect.go`，进程级，InitEnt 设置）或 ent 场景 `client.Driver().Dialect() == dialect.MySQL`。
- 写法：在函数体前部 `if IsMySQL() { return xxxMySQL(...) }` 早退；PG 原代码不动。
- MySQL 实现函数命名 `xxxMySQL`，放同文件末尾，注释说明与 PG 版的语义对应关系。

## 占位符与参数

- MySQL 分支一律用 `?`；**ent 客户端的 QueryContext/ExecContext 不做 $n→? 改写**，原生 SQL 必须自己用对占位符。
- `= ANY($n)` + `pq.Array(ids)` → `IN (?,?,...)` 展开：用 `Placeholders(n)`（dialect.go）拼占位符，参数直接展开 append。
  参考已有实现：`api_key_repo.go` 的 `latestUsageLogIPsQuery` 非 PG 分支。
- `pq.Array` 仅出现在 PG 分支里；MySQL 分支不得引用（数组列已改 JSON，见下）。

## RETURNING 改写（按类别）

1. `INSERT ... RETURNING id`（自增主键）→ `ExecContext` + `res.LastInsertId()`。
2. `INSERT ... RETURNING id, created_at[, ...]` → 应用侧已知这些值（prepared 结构里有），插入后直接回填；
   或 LastInsertId + 同连接回查。
3. `UPDATE ... RETURNING <旧值, 新值>` → `sqlAtomicScope`（dialect.go）：
   `beginSQLAtomicScope(ctx, client, sqlq)` → `SELECT ... FOR UPDATE` 读旧值 → `UPDATE` → `scope.commit()`。
   **必须**走 scope（ctx 带 ent 事务时复用之；否则临时开 *sql.DB 事务），绝不能用两条独立语句。
4. `DELETE/UPDATE ... RETURNING`（多行）→ 同事务内：先 SELECT 目标行（FOR UPDATE）→ 执行 DML → 返回先前读到的行。
5. boolean 表达式 RETURNING（如 `RETURNING quota > 0 AND ...`）→ 同事务先 SELECT 该表达式 → 再 UPDATE。
6. ODKU 判定插入是否发生：`ON DUPLICATE KEY UPDATE id = id`（no-op 惯用法）后 `RowsAffected()`：1=新插入，0=冲突。
7. `INSERT ... SELECT ... ON CONFLICT DO NOTHING`（带 JOIN 的语句）注意 `ON DUPLICATE KEY UPDATE id = id`
   中裸 `id` 会歧义（错误 1052）→ 用 `ON DUPLICATE KEY UPDATE id = <目标表名>.id`。

## 可写 CTE（WITH x AS (UPDATE/DELETE/INSERT ... RETURNING) ...）

MySQL 无可写 CTE → 用 `sqlAtomicScope` 拆成多条语句：
先执行写语句（如 UPDATE accounts ... WHERE <条件>），把受影响行读出（FOR UPDATE 或写后 SELECT），
再执行后续语句（如 INSERT INTO scheduler_outbox SELECT ...）。语义对齐：同事务、同连接、同样的行集合。

## 行锁与咨询锁

- `FOR UPDATE` / `FOR UPDATE SKIP LOCKED`：MySQL 8 支持，保留；但 `LIMIT` 必须放在 `FOR UPDATE` 之前。
- `FOR SHARE` → `FOR SHARE`（8.0 支持）。
- `pg_advisory_lock/try_advisory_lock/unlock(hashtext('name'))` → `GET_LOCK('name', <timeout秒>)` / `RELEASE_LOCK('name')`，
  仍用 `db.Conn(ctx)` 独立连接持有（GET_LOCK 是连接级锁）。`SELECT GET_LOCK(?, 0)` 返回 1/0。
  参考实现：`migrations_runner.go` 的 `mysqlGetLock`。进程内哈希锁（`user_profile_identity_repo.go` advisoryLockHash 模式）可直接复用。

## jsonb → JSON 函数映射

| PostgreSQL | MySQL 8 |
|---|---|
| `col->>'key'` | `JSON_UNQUOTE(JSON_EXTRACT(col, '$.key'))` |
| `col->'key'` | `JSON_EXTRACT(col, '$.key')` |
| `'{}'::jsonb` | `CAST('{}' AS JSON)`（或 JSON_OBJECT()） |
| `x::numeric` | `CAST(x AS DECIMAL(20,10))`（对齐目标列精度；比较用可 DECIMAL(65,30)） |
| `jsonb_set(extra, '{a,b}', v, true)` | `JSON_SET(COALESCE(extra, JSON_OBJECT()), '$.a.b', v)` |
| `extra \|\| jsonb_build_object('k', v)` | `JSON_MERGE_PATCH(COALESCE(extra, JSON_OBJECT()), JSON_OBJECT('k', v))`（键值均为标量时与 PG `||` 等价；嵌套对象语义不同需注意） |
| `extra - 'key'` | `JSON_REMOVE(extra, '$.key')` |
| `extra ? 'key'` | `JSON_CONTAINS_PATH(extra, 'one', '$.key')` |
| `jsonb_typeof(x) = 'object'/'array'/'number'/'null'` | `JSON_TYPE(x) = 'OBJECT'/'ARRAY'/'INTEGER'...`（注意大写、number 细分为 INTEGER/DOUBLE/DECIMAL/UNSIGNED，判数字用 `JSON_TYPE(x) IN ('INTEGER','DOUBLE','DECIMAL','UNSIGNED')`） |
| `a @> b`（包含） | `JSON_CONTAINS(a, CAST('<json>' AS JSON))` |
| `jsonb_array_elements(x)` 展开行 | `JSON_TABLE(x, '$[*]' COLUMNS (v <type> PATH '$'))` |
| `jsonb_agg(... ORDER BY ...)` | `JSON_ARRAYAGG`（无序！需排序时先在派生表里 ORDER BY 再聚合） |
| `jsonb_build_object(...)` | `JSON_OBJECT(...)` |
| `'[]'::json` / `'{}'::json` | `CAST('[]' AS JSON)` / `CAST('{}' AS JSON)` |

## 日期/时间

- `NOW()` → `NOW(3)`（DATETIME(3) 毫秒精度对齐）。
- `NOW() - INTERVAL '5 minutes'` → `NOW(3) - INTERVAL 5 MINUTE`（数字无引号，单位单数）。
- `date_trunc('day', ts)` → `DATE(ts)`（或 `DATE_FORMAT(ts, '%Y-%m-%d 00:00:00')` 需要时间戳时）。
- `date_trunc('hour', ts)` → `DATE_FORMAT(ts, '%Y-%m-%d %H:00:00')`。
- `to_char(ts, 'YYYY-MM-DD')` → `DATE_FORMAT(ts, '%Y-%m-%d')`（注意 %i=分钟）。
- `ts AT TIME ZONE 'Asia/Shanghai'` → `CONVERT_TZ(ts, @@session.time_zone, '+08:00')`（PG 语义：timestamptz→该时区墙钟）；
  数据库存的是会话时区墙钟时可直接算。方向务必核对。
- `current_setting('TimeZone')` → `@@session.time_zone`。
- `EXTRACT(DOW FROM x)` → `DAYOFWEEK(x)`（PG DOW: 周日=0；MySQL DAYOFWEEK: 周日=1，差 1！）；`EXTRACT(HOUR...)` → `HOUR(x)`。
- `now() AT TIME ZONE 'UTC'` → `UTC_TIMESTAMP()`。

## 其他 PG 语法

- `ILIKE` → `LIKE`（utf8mb4_0900_ai_ci 天然大小写不敏感）。
- `x::text` → `CAST(x AS CHAR)`；`x::bigint` → `CAST(x AS UNSIGNED)` 或 `SIGNED`（按语义）。
- `IS DISTINCT FROM` → `<=>` 取反：`NOT (a <=> b)`；`IS NOT DISTINCT FROM` → `a <=> b`。
- `split_part(s, ',', n)` → `SUBSTRING_INDEX(SUBSTRING_INDEX(s, ',', n), ',', -1)`。
- `btrim(x)` / `trim(x)` → `TRIM(x)`。
- `COALESCE/GREATEST/LEAST`：MySQL 的 GREATEST/LEAST 遇 NULL 返回 NULL（PG 忽略 NULL），必要时 `GREATEST(COALESCE(a,0), ...)`。
- `FILTER (WHERE cond)` 聚合 → `SUM(CASE WHEN cond THEN 1 ELSE 0 END)`（或对应聚合列 `SUM(CASE WHEN cond THEN col ELSE 0 END)`）。
- `GROUPING SETS` → `UNION ALL` 展开各组（或 `WITH ROLLUP`，语义需核对）。
- `DISTINCT ON (cols) ... ORDER BY ...` → 窗口函数：`SELECT ... FROM (SELECT *, ROW_NUMBER() OVER (PARTITION BY cols ORDER BY ...) AS rn FROM ...) t WHERE rn = 1`。
- `unnest($1::bigint[])` 行展开 / 配对数组 → `IN` 展开、VALUES 行构造器 `(ROW(?,?),(?,?))` 或 JOIN JSON_TABLE。
- `CROSS JOIN LATERAL ... LIMIT 1`（每行取最新）→ 相关子查询或 `JOIN (SELECT ..., ROW_NUMBER() OVER (PARTITION BY ...) rn ...) x ON x.rn=1 AND ...`。
- `DELETE FROM t USING a WHERE ...` → `DELETE t FROM t JOIN a ...`。
- `UPDATE t SET ... FROM (SELECT ...) prev WHERE ...` → `UPDATE t JOIN (SELECT ...) prev ON ... SET ...`。
- `tableoid` / `ctid`（PG 物理行标识）→ 用主键替代。
- `to_tsvector` / `similarity()` → LIKE（本项目仅索引处出现）。
- `setval/nextval` → 删除（AUTO_INCREMENT 自维护）。
- `RETURNING` 在 CTE 里的（已归上节）。
- 布尔字面量 `TRUE/FALSE` 两边通用。
- `SIMILAR TO` → REGEXP。

## 错误分类

- PG `*pq.Error` 判断（23505 等）→ 补 MySQL 分支：`mysql.MySQLError.Number`：1062=唯一冲突、1213=死锁、1205=锁超时。
- 统一 helper：`isMySQLUniqueViolation(err)`（dialect.go）。

## 特殊注意

- **`groups` 是 MySQL 8 保留字**：MySQL 分支的 SQL 里该表名/列引用必须反引号 `` `groups` ``（PG 分支不用）。
- `key`、`rank`、`rows`、`system`、`lateral` 同理（出现时反引号）。
- `usage_logs` 在 MySQL 是普通表（无分区）：分区相关函数（`isUsageLogsPartitioned` 等）MySQL 分支直接返回 false/no-op。
- 数组列已改 JSON（`channel_account_stats_pricing_rules.group_ids/account_ids`、`channel_monitor_v2_config.group_ids/ignored_error_categories`）：
  MySQL 分支读写用 `encoding/json` 序列化成 JSON 字符串参数 / Scan 到 `[]byte` 后反序列化。
- `RETURNING` 全部消除后，PG 分支里的 `scanBalanceChange` 等共享 helper 保持原样。
- DECIMAL 列扫描到 float64：`database/sql` 自动转换（[]byte→float64），无需特殊处理。
- 改完必须 `gofmt -w` 且 `go build ./...` 通过；不要改 *_test.go。
- 保留 PG 分支原字符串不变（包括注释），MySQL 分支自建常量/字符串。

## 已完成的参考实现

- `usage_log_repo_insert.go`：createSingleMySQL / batchInsertUsageLogsMySQL / best-effort（ODKU + LastInsertId + 回查）。
- `user_repo.go`：adjustBalanceMySQL / setBalanceMySQL / deductAvailableBalanceMySQL（sqlAtomicScope + FOR UPDATE）。
- `api_key_repo.go:562+`：IN 展开的既有先例。
- `payment_fulfillment.go:843+`：paymentAuditDialect 双方言先例（注意其非 PG 分支是未完成的 stub，可参照重写但需补全）。
