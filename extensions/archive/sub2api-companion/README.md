> # ⚠️ 已归档：本目录不再维护
>
> **本扩展的经营对账能力已经收编进 Sub2API 主程序**，不再需要独立容器。
> 相关能力现在由后端 `backend/internal/service/reconciliation_*.go`、
> `backend/internal/handler/admin/companion.go` 与迁移
> `backend/migrations/242_reconciliation_tables.sql` 提供，管理入口仍在
> 管理后台的「经营对账」页面（接口路径与响应结构未变，前端无需改动）。
>
> - **本目录仅作历史参考**：其中的 SQLite 台账、Caddy 日志解析、Subarx 归集等做法
>   都已被取代，**不要**再基于它做二次开发，也不要按它的部署文档起容器。
> - 公开价格接口 `GET /api/provider/pricing` 也已迁入主程序（见
>   `backend/internal/handler/provider_pricing_handler.go`）。
> - 历史数据**不做迁移**：新账本从切换上线那一刻开始记账。
> - 旧生产环境的 SQLite 台账文件请单独备份留存，不要导入主库。
>
> 需要二开或排查对账问题，请改读 `docs/经营对账 Companion 维护与二开交接.md`。

# Sub2API Companion

独立于 Sub2API 镜像的经营对账与公开价格服务。它不修改 Sub2API 源码、数据库结构或请求链路。

## 能力

- `GET /api/provider/pricing`：Hvoy 价格 API。
- `GET /ops/profit`：Basic Auth 保护的利润看板。
- 看板账单明细按 25/50/100 条分页，默认每 30 秒自动刷新；页面隐藏时暂停刷新，重新显示后立即更新。
- 只读同步 Sub2API `usage_logs.actual_cost`、输入/输出/缓存 Token、调用分组，并通过 `users` 记录用户邮箱；不采集或保存上游预估成本。
- 使用 A6 系统访问令牌只读分页同步 `0.06`、`0.12` 等指定令牌的逐笔真实扣费。
- 只读解析 Caddy JSON 日志中的 `X-Client-Request-Id` 与 `X-Request-Id`。
- `POST /ops/api/upstream/import`：导入 A6 JSON 或 CSV 逐笔账单。
- 自有 SQLite 数据库，金额使用十进制定点运算。
- Sub2API 原生金额按平台计价单位保存；当前业务以 `1 平台单位 = 1 CNY` 生成不可变快照。
- A6 的 `quota / quota_per_unit` 是 USD 实扣，再按 `A6_USD_TO_CNY_RATE` 生成 CNY 汇率快照。
- 使用每个 Subarx 账号的独立 API Key 只读同步 `/v1/usage` 与 `/v1/sub2api/billing`。真实 `actual_cost` 按“账号 + 业务日 + 模型”校准，再按本站调用的标准成本权重分摊；不能可靠归属的费用保留为“Subarx 账单差额”。
- `SUBARX_ACCOUNT_MULTIPLIERS` 仅作为无真实账单 API 或接口未覆盖日期的规则回退。配置 API Key 的账号在覆盖期内会等待真实账单，不会临时显示“规则实扣”。
- 账号规则区读取主站当前 `groups/account_groups/accounts` 关系，按“当前分组 -> 渠道账号”组织配置；历史调用快照不再产生残留分组名。

## A6 导入格式

```json
[
  {
    "provider": "a6",
    "upstream_request_id": "req_example",
    "cost": "0.000026",
    "currency": "USD",
    "fx_rate_to_cny": "7.20",
    "occurred_at": "2026-07-29T12:00:00Z",
    "model": "gpt-5.2",
    "input_tokens": 10,
    "output_tokens": 4,
    "source": "a6_api"
  }
]
```

CSV 至少需要 `upstream_request_id,cost` 两列，其他支持列与 JSON 字段相同。非 CNY 账单必须携带调用发生时的 `fx_rate_to_cny`，系统同时保存原币金额、汇率和 CNY 成本快照。

## 安全边界

- `SOURCE_DATABASE_URL` 必须使用只授予 `SELECT usage_logs, users, groups, accounts, account_groups` 的 PostgreSQL 角色。
- `SUB2API_UNIT_TO_CNY_RATE` 默认为 `1`，用于表达平台计价单位兑换人民币的业务口径，不是外汇美元汇率。
- 启用 A6 自动同步时，必须配置 `A6_USER_ID`、`A6_ACCESS_TOKEN`、`A6_TOKEN_ACCOUNT_MAP` 和正数 `A6_USD_TO_CNY_RATE`。
- `A6_TOKEN_ACCOUNT_MAP` 格式为 `0.06:17,0.12:18`，左侧是 A6 `token_name`，右侧是对应的 Sub2API `account_id`。未映射账单不进入利润成本。
- Caddy 日志以只读卷挂载。
- A6 密钥只允许通过运行环境注入，不写入配置或镜像。
- Subarx API Key 只允许通过运行环境注入，不写入 SQLite、配置文件、镜像、日志或 Git。
- A6 系统访问令牌请求使用 `Authorization: Bearer` 与 `New-API-User`，不保存网页登录密码。
- 成本采用 A6 逐笔账单、Subarx 真实聚合账单分摊，或接口尚未覆盖时已明确且可复算的供应商规则实扣。不能可靠分配的调用标记为“账单待分配”，不计入已对账毛利和毛利率。
- A6 逐笔账单与 Subarx 聚合账单完成关联后统一显示“已对账”；Subarx 接口覆盖期内尚未同步的调用显示“等待账单”，仅接口未覆盖日期的回退成本显示“规则实扣”。
- 看板同时显示 A6 原始 USD 实扣、汇率快照和折算后的 CNY 成本，避免平台计价单位与美元混淆。

## 本地同构演练

`rehearsal/compose.yml` 使用与当前生产相同的主要镜像层级：

- `weishaw/sub2api:0.1.164`
- `caddy:2-alpine`
- `postgres:18-alpine`
- `redis:8-alpine`
- 独立 `sub2api-companion:local`

启动：

```powershell
docker compose -f rehearsal/compose.yml up -d --build --wait
```

本地入口：

- Sub2API + Caddy：`http://127.0.0.1:18080`
- Sub2API 回环调试端口：`http://127.0.0.1:18081`
- 利润看板：`http://127.0.0.1:18080/ops/profit`
- 价格接口：`http://127.0.0.1:18080/api/provider/pricing`

演练看板账号默认为 `rehearsal / rehearsal-only-companion`，只允许用于本地合成数据环境。生产必须通过环境变量设置独立强密码。

## A6 自动同步配置

```text
A6_BASE_URL=https://a6api.com
A6_USER_ID=1233
A6_ACCESS_TOKEN=<system-access-token>
A6_TOKEN_ACCOUNT_MAP=0.06:17,0.12:18
A6_USD_TO_CNY_RATE=7.20
A6_BOOTSTRAP_LOOKBACK=24h
A6_HISTORICAL_TOKEN_LOOKBACK=48h
A6_SYNC_INTERVAL=5m
A6_HTTP_TIMEOUT=90s
A6_PAGE_SIZE=100
A6_MAX_PAGES=50
A6_BACKFILL_CHUNK=24h
A6_BACKFILL_PAGE_DELAY=200ms
```

同步器只拉取映射中的 `token_name`，以 A6 `request_id` 去重，并用账号、模型、总输入 Token、缓存 Token、输出 Token和时间窗口匹配 Sub2API 调用。新导入账单在完成首次匹配前处于内部暂存状态，不会与下游调用在看板短暂分裂成两行；唯一匹配后进入经营成本，确认无法匹配后再显示为上游待匹配。后台默认每 5 分钟访问一次 A6；看板“立即同步”会绕过间隔执行一次人工触发同步。

当前规则改名后留下的历史 token 会按 `A6_HISTORICAL_TOKEN_LOOKBACK` 独立补偿，默认只回溯 48 小时，不会从 A6 全量历史第一页重新扫描。单个 token 同步失败会记录具体 token 错误并继续处理其他 token；`a6_sync_ok=false` 仅表示本轮存在部分失败，不代表其他 token 的账单没有继续采集。

历史回填由 `POST /ops/api/a6/backfill` 启动，可在 JSON 请求体中传入 RFC3339 格式的 `from`、`to`；省略 `from` 时从已映射 A6 账号的最早调用开始，省略 `to` 时截止当前时间。`GET /ops/api/a6/backfill` 返回持久化的 `status`、`cursor`、`processed` 和 `error`。回填按 `A6_BACKFILL_CHUNK` 分块，每完成整块才推进游标和处理数，因此块内数值暂时不变不表示任务卡死；容器重启后会从已保存游标继续。A6 单页之间通过 `A6_BACKFILL_PAGE_DELAY` 限速，过大时间段会自动拆分，重复记录按上游 Request ID 幂等跳过。

## Subarx 真实账单同步配置

```text
SUBARX_BASE_URL=https://www.subarx.com
SUBARX_ACCOUNT_API_KEYS=<account_id:api_key,...>
SUBARX_SYNC_INTERVAL=5m
SUBARX_HTTP_TIMEOUT=30s
SUBARX_LOOKBACK_DAYS=90
SUBARX_TIMEZONE=Asia/Shanghai
SUBARX_ACCOUNT_MULTIPLIERS=<account_id:fallback_multiplier,...>
```

`SUBARX_ACCOUNT_API_KEYS` 的左侧是 Sub2API `account_id`，右侧是该上游账号的独立 API Key。同步器首次回填最近 `1-90` 天有费用的账单，之后固定刷新今天和昨天；更早且已成功落库的完整日保持不变。`/v1/sub2api/billing` 返回的 `effective_rate_multiplier` 会更新该账号的规则回退倍率，但不会保存 API Key。

Subarx 当前不提供逐笔 Request ID。Companion 会先验证单日汇总与模型汇总的请求数、`cost`、`actual_cost` 一致，再将模型真实费用按本站逐笔标准成本权重分摊。本站请求少于上游时，剩余真实费用写入“Subarx 账单差额”；本站请求多于上游、模型缺失或标准成本不一致时，该范围标记为“账单待分配”，不会用规则成本冒充真实对账。

经营看板默认显示滚动近 24 小时，并支持今天、近 3 天、近 7 天、本月、上个月、近三月、近一年及自定义起止时间。`GET /ops/api/summary` 与 `GET /ops/api/requests` 接受 RFC3339 格式的 `from`、`to`，统一采用 `[from, to)`（含开始、不含结束）区间；汇总卡片与分页明细始终使用同一时间口径。

`GET /ops/api/timeseries` 使用相同的 `from`、`to` 口径返回经营趋势。时间范围不超过 48 小时按 1 小时分桶，不超过 14 天按 6 小时分桶，不超过 120 天按天分桶，更长范围按周分桶。金额曲线展示下游收入、上游实扣和已对账毛利；调用量曲线展示已对账、待对账、上游待匹配和记录总数。

## 当前上线边界

本地已验证 JSON/CSV 逐笔账单导入、外币汇率快照、双 Request ID 对账、A6 系统令牌鉴权、A6 日志分页读取、组合匹配、Subarx 聚合账单校准及 Hvoy 价格接口。所有上游同步都保留超时、字段校验和旧快照保护；接口不可用时不会删除已同步账单。生产部署仍需在真实数据本地回归完成后单独审批。

## 与主仓库的集成（chenshuapi-v0.2.9 起）

从该分支起，Companion 源码随主仓库 `extensions/sub2api-companion/` 一起版本化，**部署形态保持不变**：仍是独立容器（监听 8090），Caddy 继续把 `/ops/*` 与 `/api/provider/pricing` 转发给它，SQLite 数据仍落在自身 `DATA_DIR`。

管理后台新增「经营对账」页面（`/admin/companion`，仅管理员可见），管理员不必再单独打开 `/ops/profit` 看板：

- 前端页面：`frontend/src/views/admin/CompanionView.vue`
- 代理接口：`/api/v1/admin/companion/*`，实现见 `backend/internal/handler/admin/companion.go`
- 路由注册：`backend/internal/server/routes/admin.go` 的 `registerCompanionRoutes`

主服务需要配置以下环境变量。**凭据只存在于主服务进程内，绝不下发到浏览器**：

| 变量 | 说明 |
| --- | --- |
| `COMPANION_BASE_URL` | Companion 地址；同一 Docker 网络下可填 `http://sub2api-companion:8090`。未配置时后台页面展示配置引导态 |
| `COMPANION_ADMIN_USER` | 与 Companion 的 `ADMIN_USER` 保持一致 |
| `COMPANION_ADMIN_PASSWORD` | 与 Companion 的 `ADMIN_PASSWORD` 保持一致 |
| `COMPANION_HTTP_TIMEOUT` | 可选，Go duration 格式，默认 `20s` |

代理的固定行为：

- 上游路径走硬编码白名单，不接受前端传入任意路径，避免退化成开放代理；
- 账号规则路径参数只接受数字账号 ID；
- 上游 401/403 统一转换成 502 `COMPANION_AUTH_FAILED`，不透传 401，避免面板把配置错误误判为管理员会话失效；
- 上游不可达返回 502 `COMPANION_UNREACHABLE`，返回非 JSON 内容返回 502 `COMPANION_INVALID_RESPONSE`。

