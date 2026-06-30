## Why

现有"余额扣费/回退"逻辑分散在三层、且与网关请求强耦合，无法被外部服务（"扣费 app"）直接调用：

- 真值写入散落在 `usageBillingRepository.Apply`（生产网关原子路径，幂等靠 `usage_billing_dedup`，但同事务里耦合了 apikey 配额/限流/账号配额/订阅用量）、`userRepository.DeductBalance`（`AddBalance(-amt)`，**不幂等、允许透支**）、`userRepository.UpdateBalance`（`AddBalance(+amt)`，async media 退预扣在用）。
- Redis 余额缓存（`BillingCacheService`）是在 DB 提交**之后由调用方**补写的，没有一个自洽的"扣/退"入口能同时保证 DB + 缓存一致。
- 没有任何对外 RPC 入口，也没有"外部接入方"身份模型。

业务诉求：把"余额扣费 + 回退"抽成一个独立的薄账本服务，通过 **tRPC-Go** 暴露给其它服务调用；RPC 监听**独立端口**，与现有 HTTP API 分离；支持**多个扣费 app 接入**，各自独立身份与审计。

## What Changes

- **新增 tRPC-Go 服务 `BalanceLedger`**：同进程内、在与现有 gin HTTP 不同的端口上监听。提供三个方法：`Deduct` / `Refund` / `GetBalance`。
- **薄账本语义**：RPC 只动 `users.balance`，不触碰 apikey 配额/限流/账号配额/订阅用量（那些仍由网关 `Apply` 路径负责）。外部 app 自行决定扣费金额。
- **不允许透支**：`Deduct` 在余额不足时直接拒绝（`balance >= amount` 条件写入，0 行即为余额不足），不沿用现有 `DeductBalance` 的"允许负数"策略。
- **部分退**：`Refund` 带金额参数，支持对一笔原扣费分多次部分冲销；累计已退不得超过原扣金额。
- **幂等**：`Deduct` 以调用方提供的 `request_id` 去重（账本 UNIQUE）；`Refund` 以调用方提供的 `refund_request_id` 去重（部分退是累加，不能只靠"原扣已退"判定）。
- **凭原流水冲销**：`Refund` 引用原扣费的 `request_id`，服务端查出原始金额做反向冲销。外部 app 只能冲销**它自己通过本 RPC 扣的款**，无法冲销网关 `Apply` 的扣费（边界清晰）。
- **永久流水账本 `balance_ledger`**：每笔扣/退落一行，永久保留做审计。含 `app_id`（归属接入方）、`description`（**扣费/退费原因，必填**）、`extra`（jsonb，接入方自存数据）。与会过期的 `idempotency_records` 区分——账本是钱的真值，绝不清理。
- **缓存一致性**：每次扣/退在 DB 事务提交后**同步**刷 Redis 余额缓存（失效或写入新余额）。
- **独立鉴权 + 多 app 接入（无状态 token）**：**不复用 `admin_api_key`**（避免授予全管理员权限）。新增 `billing_apps` 表仅做接入方注册（`app_id` + `app_name` + `enabled`，admin 后台录入/启停），**不存任何 token 密文或 hash**。鉴权采用无状态 token：`token = AES-256-GCM(本地密钥 balance_rpc.encryption_key, payload{app_id})`，创建时签发并仅返回一次。tRPC 拦截器从 metadata 取 token → **本地密钥解密**（成功即证明由本方签发）→ 校验解密出的 `app_id` 对应 app 存在且未停用（app 启停状态走进程内本地缓存，短 TTL + 启停主动失效，热路径不查 DB）→ 注入 `app_id` 到 ctx 并归属流水。
- **配置**：新增 `config` 段配置 RPC 监听端口（独立于 `server.port`），main.go 多起一个 goroutine 启动并优雅关闭。

## Capabilities

### New Capabilities

- `balance-rpc`: 独立端口的 tRPC-Go 余额账本服务（扣费/部分退/查询余额、多 app 接入鉴权、永久流水审计、缓存同步、不透支）。

## Impact

- **新增依赖**：`trpc.group/trpc-go/trpc-go` 及其 protobuf 工具链（go.mod 当前**没有** tRPC-Go；gRPC/protobuf 已有但本次按用户决定走 tRPC）。
- **数据库**：
  - 新增表 `billing_apps`（接入方身份）。
  - 新增表 `balance_ledger`（永久流水）。
  - 两张表的 ent schema + 迁移。
- **后端代码**：
  - 新增 `internal/service/balance_ledger_service.go`：`Deduct`/`Refund`/`GetBalance` 业务逻辑，封装 DB 事务 + Redis 同步。
  - 新增 `internal/repository/balance_ledger_repo.go`：账本读写、原子扣/退 SQL（含 `balance >= amount` 不透支条件、`FOR UPDATE` 查原流水、部分退累计校验）。
  - 新增 `internal/repository/billing_app_repo.go`：接入方查询/校验。
  - 新增 `internal/service/billing_app_service.go`（创建 app + 铸 token、启停 + 本地缓存、解密鉴权）、`internal/service/billing_app_token.go`（AES-256-GCM token codec）。
  - 新增 `internal/rpc/`（或 `internal/server/rpc/`）：tRPC-Go 服务实现 + protobuf 定义 + 鉴权拦截器。
  - `cmd/server/main.go`：第二端口 goroutine 启动 + 优雅关闭；wire 注入新服务。
  - `internal/config/config.go`：新增 RPC 端口/启用开关配置。
  - admin 侧：handler + 路由，管理 `billing_apps`（套用 `oidc_client` 的管理套路）。
- **缓存**：复用 `BillingCacheService` 的 `InvalidateUserBalance`/`SetUserBalance`，扣/退提交后同步调用。
- **Breaking**：无。新增独立服务与表，不改动现有网关计费路径与现有 HTTP API。RPC 端口未配置/未启用时行为等价于现状。

## Non-goals

- 不改动网关请求的现有计费路径（`Apply` / `postUsageBilling` / async media 预扣）。
- 不做接入方的单笔限额/可操作用户范围/scope（本期仅 `enabled` 启停；留作后续扩展）。
- 不冲销网关 `Apply` 的扣费（外部 app 只能冲销自己经 RPC 扣的款）。
- 不做 URL 模式之外的其它传输（仅 tRPC-Go）。
