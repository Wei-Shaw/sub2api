# Design — balance-rpc

## 背景与约束

抽取动机、三层余额写入现状、与网关 `Apply` 路径的边界见 `proposal.md`。本设计聚焦四个硬点：**进程/端口拓扑、数据模型、扣/退状态机（不透支 + 部分退 + 双幂等）、缓存一致性与鉴权**。

已确认的决定：

| 维度 | 决定 |
|------|------|
| 接口粒度 | 薄账本：`Deduct` / `Refund` / `GetBalance`，只动 `users.balance` |
| 传输 | tRPC-Go（新增依赖），同进程、第二个监听端口 |
| 鉴权 | 独立、不复用 `admin_api_key`；无状态 token（AES-GCM 解密）+ 本地缓存查停用 |
| 扣费 | 不透支（余额不足拒绝） |
| 退款 | 部分退（带金额）+ `refund_request_id` 幂等 |
| 流水 | 永久 `balance_ledger`，含 `app_id` / `description`(必填) / `extra`(jsonb) |
| 缓存 | 提交后同步刷 Redis |
| app 限额 | 本期不做，仅 `enabled` 启停；退款行同样带 `description` + `extra` |

## 拓扑：同进程、第二端口

```
cmd/server/main.go (runMainServer)
        │
        ├── go app.Server.ListenAndServe()        gin HTTP   :server.port
        │
        └── go balanceRPCServer.Serve()           tRPC-Go    :balance_rpc.port
                    │
                    ▼
          BalanceLedgerService（同一进程，复用同一套 repo + BillingCacheService）
                    │
        ┌───────────┴───────────┐
        ▼                       ▼
   Postgres                 BillingCacheService → Redis
   users.balance            (InvalidateUserBalance / SetUserBalance)
   balance_ledger
   billing_apps
```

- 同进程的关键收益：**缓存一致性逻辑只有一份**。若拆独立 binary，两个进程都写 Redis 余额缓存虽可行，但要额外保证 `BillingCacheService` 的失效/写入语义一致，复杂度无收益。
- 优雅关闭：`main.go` 在收到 signal 时同时 `Shutdown` HTTP server 与 tRPC server。
- 端口配置：`config.BalanceRPC{ Enabled bool; Port int }`（tRPC-Go 的 service 端口可由 `trpc_go.yaml` 承载，也可由本配置注入；二选一，倾向用本项目既有 `config` 体系注入以统一来源）。未启用时不起第二 goroutine，行为等价现状。

## 数据模型

### 表 `billing_apps`（接入方注册）

| 列 | 类型 | 说明 |
|----|------|------|
| id | bigint PK | |
| app_id | varchar(64) UNIQUE NOT NULL | 对外业务主键，如 `bapp_<base32>` |
| app_name | varchar(100) NOT NULL | 接入方名称 |
| enabled | bool default true | 启停 |
| created_at / updated_at | timestamptz | TimeMixin |

- **不存任何 token 密文 / hash**。鉴权用无状态 token（见下「鉴权」），本表只做注册 + 启停 + 审计。
- 创建：admin 触发 → 生成 `app_id` → 用本地密钥铸 token，**仅返回一次**，库里不存。
- 启停：`enabled=false` 时鉴权拒绝（经本地缓存生效）。

### 表 `balance_ledger`（永久流水，钱的真值 + 审计）

| 列 | 类型 | 说明 |
|----|------|------|
| id | bigint PK | |
| request_id | varchar(128) NOT NULL | 调用方提供的本笔幂等键 |
| app_id | varchar(64) NOT NULL | 归属接入方 |
| user_id | bigint NOT NULL | |
| kind | smallint NOT NULL | 1=deduct, 2=refund |
| amount | numeric NOT NULL | 正数金额（方向由 kind 决定，避免符号歧义） |
| refunded_amount | numeric NOT NULL default 0 | **仅 deduct 行使用**：累计已退金额 |
| refund_of | varchar(128) NULL | **仅 refund 行使用**：被冲销的原 deduct 的 request_id |
| description | text NOT NULL | 本笔原因（扣费/退费都必填） |
| extra | jsonb NOT NULL default '{}' | 接入方自存数据 |
| balance_after | numeric NULL | 本笔后用户余额（审计快照，RETURNING 落库） |
| created_at | timestamptz | |

索引：

- `UNIQUE (app_id, request_id)` —— Deduct 幂等。**按 app 维度隔离**，避免不同 app 的 request_id 撞车。
- `UNIQUE (app_id, request_id)` 同时覆盖 refund 行（refund 行的 `request_id` = `refund_request_id`），所以 Refund 幂等也落在同一唯一键上。
- `INDEX (user_id, created_at)` —— 对账查询。
- `INDEX (refund_of)` —— 按原扣聚合退款。

> 设计选择：deduct 与 refund 同表、靠 `kind` 区分，`request_id` 全表唯一（按 app）。Deduct 与 Refund 共用同一张唯一键即可分别去重，无需两套表。

为什么不复用 `idempotency_records`：那张表带 `expires_at` 会被清理，适合"瞬态请求去重"；而账本必须**永久**保留做审计。两者职责不同。`idempotency_records` 的 `scope+key` 唯一 + `locked_until` 锁模型在本设计中由 `balance_ledger` 的唯一键 + 行级 `FOR UPDATE` 直接覆盖，不再额外引入。

## 扣/退状态机

### Deduct（不透支 + 幂等）

```
Deduct(app_id, user_id, request_id, amount, description, extra):
  校验 amount > 0；description 非空（否则 INVALID_ARGUMENT）
  BEGIN
    -- 幂等：先尝试占位，命中冲突则返回既有结果
    INSERT INTO balance_ledger(app_id, request_id, user_id, kind=1, amount, description, extra)
      ON CONFLICT (app_id, request_id) DO NOTHING
    若未插入（已存在）:
      SELECT 既有行 → 校验 amount/user_id 一致（不一致 → ALREADY_EXISTS/冲突）
      返回 { applied=false, balance_after=既有快照 }   -- 幂等重放
    -- 不透支扣减
    UPDATE users SET balance = balance - amount, updated_at=NOW()
      WHERE id=user_id AND deleted_at IS NULL AND balance >= amount
      RETURNING balance
    若 0 行:
      区分原因:
        用户不存在 → NOT_FOUND
        余额不足   → FAILED_PRECONDITION (INSUFFICIENT_BALANCE)
      ROLLBACK（含刚插入的账本占位）
    UPDATE balance_ledger SET balance_after=新余额 WHERE 本行
  COMMIT
  同步刷 Redis（SetUserBalance 新余额，或 InvalidateUserBalance）
  返回 { applied=true, balance_after=新余额 }
```

要点：

- 占位 INSERT 与扣减 UPDATE 同一事务；扣减失败回滚会一并撤销占位，**不会留下"扣了占位没扣钱"的脏行**。
- 不透支由 `WHERE balance >= amount` 保证；0 行需再查一次用户区分"不存在 vs 余额不足"以返回精确错误码。
- 幂等重放（同 `request_id`）必须返回与首次一致的结果，不重复扣。

### Refund（部分退 + 凭原流水 + 幂等）

```
Refund(app_id, refund_request_id, original_request_id, amount, description, extra):
  校验 amount > 0；description 非空
  BEGIN
    -- 本笔退款幂等占位
    INSERT INTO balance_ledger(app_id, request_id=refund_request_id, user_id=<原扣的>, kind=2,
                               amount, refund_of=original_request_id, description, extra)
      ON CONFLICT (app_id, request_id) DO NOTHING
    若未插入（已存在）:
      返回既有结果（幂等重放）
    -- 凭原流水冲销
    SELECT * FROM balance_ledger
      WHERE app_id=app_id AND request_id=original_request_id AND kind=1
      FOR UPDATE
    若查不到 → NOT_FOUND（无可冲销流水；注意只能冲销本 app 自己的扣费）
    若 refunded_amount + amount > 原.amount → FAILED_PRECONDITION (OVER_REFUND)
    UPDATE 原 deduct 行 SET refunded_amount = refunded_amount + amount
    UPDATE users SET balance = balance + amount RETURNING balance
    UPDATE 本 refund 行 SET balance_after=新余额, user_id=原.user_id
  COMMIT
  同步刷 Redis
  返回 { applied=true, balance_after=新余额, refunded_total=原.refunded_amount+amount }
```

要点：

- **双幂等**：`refund_request_id` 防本笔退款重试重复退；`original_request_id` 用于定位原扣 + 累计超额校验。两者缺一不可——部分退是累加，光靠"原扣已退"无法防同一笔退款的网络重试。
- `FOR UPDATE` 锁原 deduct 行，串行化并发部分退，避免两笔并发退款都通过 `refunded_amount + amount <= amount` 检查后超额退。
- 边界：`refunded_amount + amount == amount` 合法（退到刚好全额）；`> amount` 拒绝。

## 缓存一致性

- 扣/退 DB 事务提交**之后**，同步调用 `BillingCacheService`：优先 `SetUserBalance(user_id, balance_after)` 直接写入新余额（已从 RETURNING 拿到）；失败则降级 `InvalidateUserBalance`。
- 选"同步"而非现网关的"异步队列"：RPC 是动钱的强一致入口，下一次 `GetBalance` / 网关 preflight 必须立刻看到新值，避免透支判断读到陈旧缓存。
- `GetBalance` 读路径复用 `BillingCacheService.GetUserBalance`（缓存 → singleflight 回源 DB）。

## 鉴权（无状态 token + 本地缓存查停用）

token = `AES-256-GCM(balance_rpc.encryption_key, payload{app_id})`（base64）。GCM 是 AEAD：
带认证标签，只有持本地密钥方能造出可干净解密的密文 → **解密成功即证明 token 由本方签发**。

```
创建 app:
  生成 app_id → token = Encrypt(本地密钥, {app_id})  ← 一次性返回；DB 不存 token/hash

tRPC 拦截器 (filter):
  从 metadata 取 token（app-token）
  plain, err := Decrypt(本地密钥, token)
    err（伪造/篡改/换密钥，GCM tag 失败） → UNAUTHENTICATED（统一错误）
    密钥未配置 → 系统错误（配置问题，非调用方）
  app := 查 app_id 的启停状态（本地缓存优先，未命中回源 DB 并缓存）
    未找到 / enabled=false → UNAUTHENTICATED
  注入 app_id 到 ctx → 业务方法用它归属流水
```

- **无状态解密**：热路径 = 解密 + 本地缓存命中（进程内 map + RWMutex，TTL 5s），**不必每请求查 DB**。
- **停用即时生效**：`SetEnabled` 在更新 DB 后**主动删本地缓存**，下次鉴权重新加载读到新状态。撤销全部 token = 轮换本地密钥。
- **不复用 `admin_api_key`**：那是全管理员单例密钥，违反动钱接口的最小权限。
- 密钥独立（`balance_rpc.encryption_key`，32 字节 hex），与 TOTP 等其它密钥隔离；绝不进日志。
- 本期不做单笔限额/用户范围 scope；`billing_apps` 预留可扩展。
- 传输层建议叠加网络隔离（RPC 端口只对内网/服务网格开放）+ 可选 mTLS。

## protobuf 契约（tRPC-Go IDL 草案）

```proto
service BalanceLedger {
  rpc Deduct(DeductRequest) returns (DeductResponse);
  rpc Refund(RefundRequest) returns (RefundResponse);
  rpc GetBalance(GetBalanceRequest) returns (GetBalanceResponse);
}

message DeductRequest {
  int64  user_id      = 1;
  string request_id   = 2;   // 幂等键
  string amount       = 3;   // 字符串十进制，避免 float 精度问题
  string description  = 4;   // 必填，扣费原因
  string extra        = 5;   // jsonb 文本，接入方自存
}
message DeductResponse {
  bool   applied        = 1; // false=幂等重放
  string balance_after  = 2;
}

message RefundRequest {
  string refund_request_id    = 1; // 本笔退款幂等键
  string original_request_id  = 2; // 被冲销的原扣
  string amount               = 3; // 部分退金额
  string description          = 4; // 必填，退费原因
  string extra                = 5;
}
message RefundResponse {
  bool   applied         = 1;
  string balance_after   = 2;
  string refunded_total  = 3; // 原扣累计已退
}

message GetBalanceRequest  { int64 user_id = 1; }
message GetBalanceResponse { string balance = 1; }
```

- 金额用字符串十进制传输，服务端转 `numeric` 入库；避免 protobuf `double` 的二进制浮点误差污染账本。
- token 走 tRPC **metadata**（键 `app-token`，鉴权层），不进业务 message。

## 错误码映射

| 场景 | 码 |
|------|----|
| description 空 / amount<=0 / 参数非法 | INVALID_ARGUMENT |
| 鉴权失败 / app 禁用 / 未找到 app | UNAUTHENTICATED |
| 用户不存在 / 原扣流水不存在 | NOT_FOUND |
| 余额不足（不透支） | FAILED_PRECONDITION (INSUFFICIENT_BALANCE) |
| 累计退款超原扣 | FAILED_PRECONDITION (OVER_REFUND) |
| 同 request_id 但参数不一致 | ALREADY_EXISTS / ABORTED |
| DB/内部错误 | INTERNAL |

## 待实现确认（实现期再定，不阻塞 spec）

- tRPC-Go 端口是否复用项目 config 注入还是 `trpc_go.yaml`：倾向 config 注入统一来源。
- `numeric` 精度位数与 `users.balance` 现有列精度对齐。
- app 启停状态本地缓存 TTL（当前 5s）；密钥绝不进日志；如需跨实例即时停用可改 Redis 失效广播。
