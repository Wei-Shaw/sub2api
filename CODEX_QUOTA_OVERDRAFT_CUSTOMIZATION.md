# sub2api-overdraft 5h / 7d 额度透支定制记录

本文件是维护者实现与升级记录。面向部署者的完整安装、验证、升级和故障排查说明见
[CODEX_OVERDRAFT_DEPLOYMENT_CN.md](CODEX_OVERDRAFT_DEPLOYMENT_CN.md)。

## 基本信息

- 当前官方合并基线：Sub2API `v0.1.179`，`upstream/main` 提交 `2bc139ab5`
- 参考实现：<https://github.com/Mxucc/cpa-account-config-manager>
- 功能开关：`gateway.codex_quota_overdraft_enabled`
- 代码默认值：关闭；`deploy/config.example.yaml` 部署示例默认开启
- Fork 版本文件：`FORK_VERSION`
- 更新源：`DeanZFC/sub2api-overdraft` 的 `codex-overdraft` 分支

## Fork 更新检查

源码 Docker 构建会读取根目录的 `FORK_VERSION`，并以 `BuildType=source` 写入二进制。后台更新服务通过 GitHub Contents API 读取 Fork 分支上的同名文件，使用语义化版本比较判断是否有新版本。Redis 缓存同时记录仓库和构建类型，因此旧的官方更新缓存不会继续生效。

源码构建仅显示 `git pull` 更新提示，`PerformUpdate`、指定版本回退和在线回退列表均被禁用，防止官方二进制覆盖透支功能。维护者每次发布 Fork 更新都必须递增 `FORK_VERSION`；当前版本为 `0.1.179-overdraft.1`。

Sub2API `v0.1.179` 继续保留原生 remote compaction v2 在 `/responses` 路径。本定制同时检查旧 Compact 路径和原生 v2 请求信号，二者都不会开启额度透支调度或注入透支请求形态。

## 后台测试连接与残留限流修复

管理页面的 OpenAI OAuth 常规文本“测试账号连接”与真实业务请求使用相同策略：用量达到 95% 后注入；100% 时成功直接确认 `passed`，明确额度 429 直接确认 `failed`。API Key、Shadow、图片和 Compact 测试不启用该行为。

探测得到 `passed`，或额度周期进入 `recovered` 后，会清除仍残留的账号级 `rate_limit_reset_at` 以及本功能或额度阈值产生的暂停。这里不再要求当前重置时间与探测开始时记录的 `observed_rate_limit_reset_at` 完全相同，因为并发 429 可能在探测期间更新该字段；`failed` 状态不会清理限流。发生清理时记录 `codex_quota_overdraft_stale_rate_limit_cleared`。

## PostgreSQL 兼容性修复

探测计划使用原子 claim 写入 `accounts.extra`。`jsonb_build_object` 的键参数必须显式
转换为 `text`，否则 PostgreSQL 会报 `could not determine data type of parameter $1`，导致
所有探测在启动前失败。对应 SQL 使用：

```sql
jsonb_build_object($1::text, $2::jsonb)
```

## 启用方式

在实际使用的 `config.yaml` 中加入：

```yaml
gateway:
  codex_quota_overdraft_enabled: true
```

修改配置后重启 sub2api。出现上游兼容问题时可改为 `false` 并重启，立即恢复官方调度和请求行为。

## 完整行为

开启后，对 OpenAI OAuth 普通账号和 Agent Identity 账号生效，Shadow 账号除外。支持路径：

- `/v1/responses`
- `/v1/chat/completions` 转换到 Responses 的请求
- `/v1/messages` 转换到 Responses 的请求
- Responses WebSocket v2 首轮和后续轮
- 管理页面 OpenAI OAuth 常规文本“测试账号连接”

用量达到 95% 后，每次最终发往上游前，如果 `input` 最后一项是用户消息，则追加一对与参考项目相同的无操作 `custom_tool_call` 和 `custom_tool_call_output`。请求体超过 32 MiB、JSON 无效、形状不匹配或已经注入时保持原样。

5h 或 7d 额度达到 100%，或者收到包含明确额度证据的真实 429 时，执行以下混合判定：

1. 实际注入的业务请求成功时直接判定 `passed`，账号继续调度并开始透支统计，不额外消耗探测请求。
2. 实际注入的业务请求返回明确额度 429 时直接判定 `failed`，账号暂停到对应额度恢复时间并切号；5h 和 7d 同时耗尽时取最晚恢复时间。
3. 如果没有可用的业务成功证据，同一额度周期仅启动 1 次独立 Responses 探测，最长 20 秒并优先使用当前业务模型。多实例使用 PostgreSQL 原子 claim 去重。
4. 独立探测成功即判定 `passed`，明确 `quota_limited` 即判定 `failed`；网络错误、超时、5xx、普通瞬时 429 和无效响应判定 `inconclusive`，同周期不自动重试。
5. `failed` 状态、临时暂停和调度 outbox 在同一数据库事务中提交。同周期 `failed` 为终态，晚到的业务成功或探测结果不能覆盖；业务明确额度 429 可以把并发产生的 `passed` 或 `inconclusive` 收敛为 `failed`。
6. 401/403、账号停用等认证问题交给原有认证异常逻辑处理；400/404 判定为 `inconclusive`，不会暂停整个账号。
7. 账号禁用、过载、代理/传输故障、模型级冷却及其他临时不可调度原因不被绕过。
8. 额度恢复后状态改为 `recovered`，清理本功能或额度阈值产生的暂停；5h 与 7d 分别维护透支起点，不会因另一窗口后来耗尽而重置已有统计。

状态值包括 `pending`、`passed`、`failed`、`inconclusive`、`recovered`。账号用量页面显示探测状态、尝试次数、额度周期、透支期成功请求数、Token、账号金额及预计恢复时间。

不需要新增数据库表或执行数据库结构迁移（schema migration）；周期状态保存在现有 `accounts.extra` JSONB 字段，透支统计读取现有 `usage_logs`。

`/responses/compact`、生图请求、Embedding、Count Tokens、Live 等端点不启用额度透支。

## 低冲突集成结构

从 `0.1.177-overdraft.6` 起，透支行为保持不变，但实现按“独立模块 + 固定钩子”组织，以减少同步官方更新时的冲突：

- 四个 PostgreSQL 原子状态方法及临时暂停查询扩展位于 `backend/internal/repository/account_repo_codex_overdraft.go`，`account_repo.go` 不再承载大段透支 SQL。
- Gateway 通过 `sync.Once` 懒加载并持有全进程唯一协调器；账号用量服务和账号测试服务从同一个 Gateway 获取实例，不再向 Wire 图添加 Fork 专用 Provider。
- `backend/cmd/server/wire_gen.go` 由官方 Wire 图正常生成，不包含透支专用变量或参数。
- 调度、429、业务成功、用量快照和账号测试接线统一放在 `openai_codex_quota_overdraft_hooks.go`、`openai_codex_quota_overdraft_integration.go` 和 `account_test_codex_overdraft.go`；官方热点文件仅保留短调用。
- 前端状态徽标和透支统计分别位于 `CodexOverdraftStatus.vue`、`CodexOverdraftStats.vue`，官方账号用量组件仅负责传入原有数据。

这次调整没有修改 95% 注入门槛、Payload 内容、额度信号解析、429 分类、探测次数、状态转换、事务边界、暂停/恢复规则、统计口径或页面文案。升级时应优先保留独立文件，再逐一核对官方文件中的短钩子。

## 修改文件

后端配置、路由和调度：

- `backend/internal/config/config.go`
- `backend/internal/handler/openai_chat_completions.go`
- `backend/internal/handler/openai_gateway_handler.go`
- `backend/internal/repository/account_repo.go`
- `backend/internal/repository/account_repo_codex_overdraft.go`
- `backend/internal/repository/account_repo_schedulable_projection_test.go`
- `backend/internal/service/account_test_codex_overdraft.go`
- `backend/internal/service/openai_account_runtime_block_fastpath.go`
- `backend/internal/service/openai_account_scheduler.go`
- `backend/internal/service/openai_codex_quota_overdraft.go`
- `backend/internal/service/openai_codex_quota_overdraft_hooks.go`
- `backend/internal/service/openai_codex_quota_overdraft_integration.go`
- `backend/internal/service/openai_codex_quota_overdraft_probe.go`
- `backend/internal/service/openai_codex_quota_overdraft_test.go`
- `backend/internal/service/openai_codex_quota_overdraft_probe_test.go`
- `backend/internal/service/openai_gateway_forward.go`
- `backend/internal/service/openai_gateway_passthrough.go`
- `backend/internal/service/openai_gateway_scheduling.go`
- `backend/internal/service/openai_gateway_service.go`
- `backend/internal/service/openai_gateway_usage.go`
- `backend/internal/service/openai_ws_forwarder_ingress.go`
- `backend/internal/service/openai_ws_forwarder_support.go`
- `backend/internal/service/openai_ws_forwarder_v2.go`
- `backend/internal/service/openai_ws_v2_passthrough_adapter.go`
- `backend/internal/service/ratelimit_service.go`
- `backend/internal/service/account_usage_service.go`
- `backend/internal/service/account_test_service.go`
- `backend/internal/service/account_test_service_openai_test.go`
- `backend/internal/service/account_test_service_openai_compact_test.go`
- `backend/internal/service/wire.go`

前端、配置示例和测试：

- `frontend/src/types/index.ts`
- `frontend/src/components/account/CodexOverdraftStats.vue`
- `frontend/src/components/account/CodexOverdraftStatus.vue`
- `frontend/src/components/account/UsageProgressBar.vue`
- `frontend/src/components/account/AccountUsageCell.vue`
- `frontend/src/components/account/__tests__/UsageProgressBar.spec.ts`
- `frontend/src/components/account/__tests__/AccountUsageCell.spec.ts`
- `frontend/src/i18n/locales/zh/dashboard.ts`
- `frontend/src/i18n/locales/en/dashboard.ts`
- `deploy/config.example.yaml`

## 官方更新后的重新实施

普通 `git pull` 不会静默丢弃未提交修改：能自动合并时会保留，冲突时会停止。安装脚本若强制重置、删除目录后重拉或重新解压，则可能覆盖本定制。

1. 更新前保存补丁：`git diff > codex-quota-overdraft.patch`
2. 拉取官方代码并处理冲突。
3. 尝试恢复补丁：`git apply --3way codex-quota-overdraft.patch`
4. 无法自动应用时，按本文件“完整行为”和“修改文件”逐项重新接入。
5. 在 `backend` 执行 `go generate ./cmd/server`，重新生成 Wire。
6. 确认实际 `config.yaml` 仍有 `gateway.codex_quota_overdraft_enabled: true`。
7. 运行验证：

```bash
cd backend
gofmt -w internal/service/openai_codex_quota_overdraft*.go
go test -tags=unit ./internal/repository ./internal/handler ./internal/service \
  -run 'CodexQuotaOverdraft|AccountTestService_OpenAI|AccountSchedulingThreshold|SchedulerSnapshot|SchedulableAccountQueryScopesCodex' -count=1
go test ./internal/repository ./internal/handler ./internal/service ./cmd/server -run '^$' -count=1

cd ../frontend
pnpm install --frozen-lockfile
pnpm run typecheck
pnpm exec vitest run \
  src/components/account/__tests__/UsageProgressBar.spec.ts \
  src/components/account/__tests__/AccountUsageCell.spec.ts

cd ..
git diff --check
```

人工核对所有 Responses 出站路径仍在最终发送前注入，并检查日志关键字：

```text
codex_quota_overdraft_probe_passed
codex_quota_overdraft_probe_failed
codex_quota_overdraft_probe_inconclusive
codex_quota_overdraft_probe_attempt
codex_quota_overdraft_business_passed
codex_quota_overdraft_business_exhausted
codex_quota_overdraft_pause_applied
codex_quota_overdraft_stale_rate_limit_cleared
```

数据库状态可这样检查：

```sql
SELECT id, name, extra->'codex_quota_overdraft_probe'
FROM accounts
WHERE platform = 'openai' AND type = 'oauth';
```
