# Admin API keys and incremental usage / 管理员密钥与增量用量接口

## Authentication / 认证

All routes below use the existing admin middleware: an admin `x-api-key` or
an administrator JWT. They do not require the target user's JWT. Responses use
the standard `{ "code": 0, "message": "success", "data": ... }` envelope.

以下接口沿用管理员认证，无需目标用户的 JWT，响应沿用标准响应结构。

## User-scoped keys / 指定用户的密钥

| Method | Route | Result |
| --- | --- | --- |
| GET | `/api/v1/admin/users/:id/api-keys` | Existing paginated list / 现有分页列表 |
| POST | `/api/v1/admin/users/:id/api-keys` | Create and return APIKey / 创建并返回密钥 |
| GET | `/api/v1/admin/users/:id/api-keys/:key_id` | Return APIKey / 查询密钥 |
| PUT | `/api/v1/admin/users/:id/api-keys/:key_id` | Update and return APIKey / 更新密钥 |
| DELETE | `/api/v1/admin/users/:id/api-keys/:key_id` | Revoke through audited deletion / 审计撤销 |

IDs must be positive integers. A key belonging to another user returns 404 on
the user-scoped item routes. The existing `/admin/api-keys/:id` group-management
route and its automatic group-access grant behavior remain unchanged.

ID 必须为正整数。查询、更新和删除其他用户的密钥返回 404。旧管理员分组接口行为不变。

### Create / 创建

`Idempotency-Key` is required. Example body:

```json
{
  "name": "membership-client",
  "group_id": 1,
  "quota": 10,
  "expires_in_days": 30,
  "rate_limit_5h": 2,
  "rate_limit_1d": 5,
  "rate_limit_7d": 10,
  "ip_whitelist": [],
  "ip_blacklist": []
}
```

`name` is required. `custom_key` is optional and follows the user API's existing
format, uniqueness and creation-rate checks. Omit `group_id` for an unbound key;
a supplied group must already be bindable by this user. Quota/rate limits must
be finite and non-negative; zero means unlimited. Omit `expires_in_days` for no
expiry; a supplied value must be positive. IP lists follow existing validation.

Successful retries within the configured write-idempotency TTL return the saved
APIKey and `X-Idempotency-Replayed: true`. The fingerprint includes the target
user, payload and administrator identity scope. Reusing a key for a different
user or payload returns 409 (`IDEMPOTENCY_KEY_CONFLICT`). Missing/invalid keys
return 400 (`IDEMPOTENCY_KEY_REQUIRED` / `IDEMPOTENCY_KEY_INVALID`); unavailable
idempotency storage returns 503 (`IDEMPOTENCY_STORE_UNAVAILABLE`) before creation.
Concurrent retries use the existing processing/retry-after behavior.

This uses the existing idempotency coordinator, not a new atomic transaction
between key creation and replay persistence. After an ambiguous process/storage
failure, reconcile the user's key list before retrying with a new idempotency
key. A stable `custom_key` additionally uses the existing unique-key constraint.
Treat both key responses and stored replay responses as credentials.

创建要求幂等键，同一管理员范围、同一用户及同一请求在有效期内重放已有结果。
跨用户或请求内容冲突返回 409，幂等存储不可用返回 503。该机制沿用现有幂等协调器；
若进程在创建后、保存重放结果前异常，应先查询密钥列表核实结果。

### Update and revoke / 更新与撤销

PUT accepts the existing user key fields:
`name`, `group_id`, `status` (`active` or `inactive`), `ip_whitelist`,
`ip_blacklist`, `quota`, `expires_at`, `reset_quota`, `rate_limit_5h`,
`rate_limit_1d`, `rate_limit_7d`, `reset_rate_limit_usage`.

Omitted/null fields are unchanged. Empty IP arrays clear their lists. An empty
`expires_at` string clears expiry; otherwise it must be RFC3339. A supplied group
must be bindable by the user; this endpoint does not automatically grant access.
Updates use the existing selective-column writes and cache invalidation. DELETE
uses the existing audit/tombstone and cache invalidation; subsequent reads or
repeated deletes return 404. PUT/DELETE do not require an idempotency header;
resetting usage counters again is a new reset and must not be retried blindly.

更新沿用用户接口语义、分组权限、选择性列写入和缓存失效。空字符串清除过期时间，
空数组清除 IP 列表。删除执行现有审计撤销，重复删除返回 404。用量重置不可盲目重试。

## Incremental usage / 增量用量

`GET /api/v1/admin/usage/incremental?cursor=<next_cursor>&limit=100`

Only `cursor` and `limit` are accepted. `limit` defaults to 100 and must be
1–1000. Omit `cursor` to start with all retained history. Invalid parameters
return 400. Existing `/admin/usage` date filters and pagination are unchanged.

Response `data`:

```json
{
  "items": [],
  "next_cursor": "v1:0:0",
  "has_more": false
}
```

`items` contains the existing AdminUsageLog DTO, including source ID, user/key
IDs, request timestamp, model, tokens and costs. Records are ordered by
`(ingestion_txid, id)`, not request timestamp. Cursors are opaque to clients;
transaction IDs are decimal strings to preserve full 64-bit precision.

The database supplies `ingestion_txid = pg_current_xact_id()` on insertion.
Existing rows receive marker 0 using a constant default, without a table rewrite.
An index supports the tuple seek. Each read only includes transaction IDs below
`pg_snapshot_xmin(pg_current_snapshot())` in that statement's snapshot. All lower
transactions have finished, so neither out-of-order commits nor late requests
can insert another record behind a returned cursor. Multiple rows from one
transaction are ordered by source ID. Rollbacks and conflict-discarded inserts
do not produce feed entries. There is no global serialization lock on writers.

接口按数据库插入事务与行 ID 排序，仅导出快照中最老活动事务之前的记录，
避免异步迟到记录或乱序提交落在游标之后而被遗漏。回滚、冲突丢弃不产生记录。
既有行用常量 0 标记，新写入自动记录完整事务 ID，无需串行化写入。

### Consumer contract and limits / 消费约定及边界

- Atomically upsert `items` by source usage ID and save `next_cursor` in the
  consumer database. A retry with the same cursor can redeliver the same page;
  source IDs make processing idempotent. Sequential cursor advancement returns
  each retained inserted row once. This is not a network exactly-once guarantee.
- Continue while `has_more` is true, then poll again with the last cursor. False
  means no additional *eligible* row in that snapshot, not that every request
  has finished. An empty page keeps the supplied cursor unchanged.
- Database long-running/prepared transactions can hold back export, including
  unrelated transactions. Monitor transaction age and feed lag; no fixed grace
  period can safely replace this watermark.
- Read from the primary. Persist cursors per database; reset/resynchronize after
  a restore/replacement or rewind. Do not reuse a cursor with another database.
- This is an insertion feed, not CDC for later updates or deletions. Keep usage
  cleanup/retention beyond the consumer's maximum lag, or pause cleanup until
  processing catches up. Data already deleted or never persisted by the
  best-effort usage writer cannot be recovered by this endpoint.
- The feed describes usage, not a payment ledger. Existing `actual_cost` and
  billing-failure semantics are preserved. External billing must reconcile and
  apply its own retail pricing and idempotent balance operations.
- The migration adds a normal column and a concurrent index. Adding the column
  still needs a brief schema lock; schedule around long transactions. As with
  existing concurrent usage indexes, manually partitioned installations must
  provision corresponding partition indexes through their migration procedure.

消费者应将用量 ID 去重写入与游标保存放在同一事务。长事务会延迟导出，历史清理必须
覆盖消费延迟。接口不恢复已删除或从未成功落库的数据，不表示账务事务已成功。
