# Tasks — add-balance-rpc-service

## 1. 数据层

- [x] 1.1 Ent schema `ent/schema/billing_app.go`：`app_id`(varchar64 unique)、`app_name`(varchar100)、`enabled`(bool default true)、TimeMixin；生成迁移
- [x] 1.2 Ent schema `ent/schema/balance_ledger.go`：`request_id`(varchar128)、`app_id`(varchar64)、`user_id`(bigint)、`kind`(smallint)、`amount`(numeric)、`refunded_amount`(numeric default 0)、`refund_of`(varchar128 nullable)、`description`(text)、`extra`(jsonb default '{}')、`balance_after`(numeric nullable)、created_at；生成迁移
- [x] 1.3 索引：`UNIQUE(app_id, request_id)`、`INDEX(user_id, created_at)`、`INDEX(refund_of)`；确认 `users.balance` 的 numeric 精度与账本对齐
- [x] 1.4 `internal/repository/billing_app_repo.go`：`GetByAppID`、`Create`、`SetEnabled`、`List`
- [x] 1.5 `internal/repository/balance_ledger_repo.go`：`Deduct`（占位 INSERT ON CONFLICT + `balance >= amount` 条件 UPDATE RETURNING + 区分 NOT_FOUND/INSUFFICIENT，单事务）、`Refund`（refund 占位 + `FOR UPDATE` 取原扣 + 累计超额校验 + 反向 UPDATE，单事务）、`GetByAppRequestID`
- [x] 1.6 仓储集成测试（走真实 DB）：不透支、Deduct 幂等重放、部分退累计、超额退被拒、并发部分退串行化（`FOR UPDATE`）、原扣不存在/非本 app

## 2. 接入方身份与鉴权服务

- [x] 2.1 `internal/service/billing_app_service.go`：`CreateApp`（生成 `bapp_<base32>` app_id + 铸 token，库不存 token，token 仅返回一次）、`SetEnabled`（更新+失效本地缓存）、`Authenticate(token)`（解密+本地缓存查 enabled，统一未认证错误）
- [x] 2.2 `internal/service/billing_app_token.go`：AES-256-GCM token codec（Mint/Parse，密钥来自 `balance_rpc.encryption_key`）；密钥绝不进日志
- [ ] 2.3 （可选，未做）`billing_apps` 短 TTL Redis 缓存，降低高频鉴权 DB 压力
- [x] 2.4 单测：Authenticate 各分支、CreateApp 明文仅一次、SetEnabled 后立即失效

## 3. 账本业务服务

- [x] 3.1 `internal/service/balance_ledger_service.go`：`Deduct`/`Refund`/`GetBalance`，参数校验（amount>0、description 非空）、调用 repo、提交后**同步**刷 Redis（`SetUserBalance` 新余额，失败降级 `InvalidateUserBalance`）
- [x] 3.2 `GetBalance` 复用 `BillingCacheService.GetUserBalance`（缓存→singleflight 回源）
- [x] 3.3 错误码映射到 RPC 层语义（INVALID_ARGUMENT / NOT_FOUND / FAILED_PRECONDITION / UNAUTHENTICATED / INTERNAL）
- [x] 3.4 单测：扣后/退后缓存同步、原因必填校验、幂等重放返回一致结果

## 4. tRPC-Go 服务

- [x] 4.1 引入依赖 `trpc.group/trpc-go/trpc-go` 及 protobuf 工具链；go.mod/go.sum
- [x] 4.2 protobuf 定义 `BalanceLedger`（Deduct/Refund/GetBalance），金额走十进制字符串；生成 stub
- [x] 4.3 `internal/rpc/balance_ledger_server.go`：实现三个方法，转调 `BalanceLedgerService`
- [x] 4.4 鉴权拦截器（tRPC filter）：从 metadata 取 `app-token` → `BillingAppService.Authenticate(token)` → 注入 `app_id` 到 ctx；失败返回 UNAUTHENTICATED
- [x] 4.5 金额字符串 ↔ numeric 解析/格式化工具，拒绝非法/负数/超精度

## 5. 进程与配置

- [x] 5.1 `internal/config/config.go`：新增 `BalanceRPC{ Enabled bool; Port int }`，校验 Port≠server.port
- [x] 5.2 `cmd/server/main.go`：`balance_rpc.enabled=true` 时多起一个 goroutine 启动 tRPC server；signal 时与 HTTP server 一同优雅关闭
- [x] 5.3 wire 注入：`billing_app_repo` / `balance_ledger_repo` / `billing_app_service` / `balance_ledger_service` / rpc server
- [x] 5.4 未启用路径冒烟：`enabled=false` 时不监听第二端口、现有行为不变

## 6. 接入方管理后台

- [x] 6.1 `internal/handler/admin/billing_app_handler.go`：创建（返回一次性 token）、列表、启停
- [x] 6.2 路由注册（admin 路由组）
- [x] 6.3 前端管理页 `BillingAppsView.vue` + `api/admin/billingApps.ts`：列表 + 创建（一次性展示 token）+ 启停；路由 + 侧边栏 + i18n(zh/en)
- [x] 6.4 单测/handler 测：创建明文仅一次、停用后鉴权失败

## 7. 文档与验证

- [x] 7.1 接入方对接文档：metadata 鉴权、三方法语义、幂等键约定（request_id / refund_request_id）、错误码、金额字符串格式
- [ ] 7.2 端到端验证（待真实环境运行）：起第二端口 → 创建 app → Deduct（不透支/幂等）→ 部分 Refund（累计/超额/幂等）→ GetBalance 一致 → Redis 缓存一致。集成测试已就绪（`-tags integration`，需 PG）。
- [x] 7.3 `openspec validate add-balance-rpc-service --strict` 通过

## 8. 接入方增强（刷新 token / 删除 / 费用统计）

- [x] 8.1 `billing_apps` 加 `token_version`（ent schema + 迁移 161 + codegen）；token payload 带版本，鉴权比对版本
- [x] 8.2 repo：`BumpTokenVersion` / `Delete`；service：`RefreshToken`（自增版本+失效缓存+铸新 token，旧 token 失效）/ `DeleteApp`
- [x] 8.3 `balance_ledger` 累计统计：repo `AppStats`（GROUP BY kind）+ service `AppStats`
- [x] 8.4 handler/路由：`POST /:app_id/refresh-token`、`DELETE /:app_id`、`GET /:app_id/stats`；wire（handler 注入 BalanceLedgerService）
- [x] 8.5 前端：操作列加「费用 / 刷新 Token / 删除」，费用弹窗、刷新/删除二次确认、刷新后揭示新 token；i18n(zh/en)
- [x] 8.6 单测：刷新后旧 token 失效 + 新 token 有效、删除后 token 失效
