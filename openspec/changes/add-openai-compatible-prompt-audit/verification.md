# 验证与灰度手册

## 1. 验证原则

本文件既是实现期验收矩阵，也是上线前证据索引模板。所有“待实现”项必须替换为可重复执行的测试名、命令输出、SQL 结果、日志查询或页面截图路径；仅写“人工验证通过”不算证据。

验证顺序：

1. OpenSpec 结构和需求完整性。
2. 纯函数、配置和核心单元测试。
3. PostgreSQL/Redis 集成与多 Worker/多实例测试。
4. Handler 协议矩阵和无副作用断言。
5. 前端功能、凭据状态、可访问和原页面回归。
6. 全量构建、canary 泄露检查、async 灰度和 blocking 准入。

关键不变量：

- off 等于升级前行为。
- async 的任何失败都不改变主请求结果。
- blocking 的 Block/Unavailable/Invalid 必须发生在账号、计费和上游之前。
- 现有 Content Moderation Block 响应永远优先且原副作用保持不变。
- PostgreSQL、日志、管理 API、前端和错误响应中没有完整 Prompt 或 Guard token。

## 2. Requirement → Evidence 追踪矩阵

状态词：`待实现`、`通过`、`失败`、`豁免（必须有批准链接）`。证据路径建议统一放到实现 PR 的 CI artifact 或 `docs/evidence/prompt-audit/<date>/`，不要把包含真实 Prompt/token 的原始数据提交到仓库。

### 2.1 prompt-input-audit

| ID | Requirement | 必备自动化证据 | 补充证据 | 状态 |
| --- | --- | --- | --- | --- |
| A01 | 独立且默认关闭 | Coordinator off 单测；默认 config 单测；现有 Moderation 回归 | 升级后 config/runtime 截图 | 通过（自动化） |
| A02 | OpenAI 兼容节点 | request builder golden；mock server 断言 `/v1/chat/completions`、model/messages/temperature/max_tokens/seed | probe 脱敏结果 | 通过（自动化） |
| A03 | 凭据和出站地址安全 | 加密往返；Public DTO canary；SSRF/DNS rebinding/redirect/256 KiB 测试 | 配置 JSON 与日志扫描 | 通过（自动化） |
| A04 | 按协议提取输入快照 | Chat/Responses/Claude/Gemini/images/media/WS 表驱动测试；用户名/邮箱/API Key 名称分列 | 路由覆盖清单 | 通过（自动化） |
| A05 | 数据库快照脱敏不可恢复 | canary Prompt 入库后全列扫描；预览/hash 单测 | schema 禁止列 SQL | 通过（自动化+SQL） |
| A06 | 持久任务 + Redis TTL | staging→SET EX→queued；多实例队列 admission lock；Redis/发布失败补偿测试 | TTL 1800 秒窗口证据 | 通过（集成） |
| A07 | Worker 可靠消费 | SKIP LOCKED、claim_version fencing、retry、lease refresh/reclaim、panic、shutdown 测试 | 多 Worker 运行指标 | 通过（集成+race） |
| A08 | Qwen3Guard 严格归一 | Safe/Controversial/Unsafe、九类、未知类、重复/额外/缺失字段测试 | golden response 语料 | 通过（自动化） |
| A09 | Unicode 完整分片 | 中文/emoji/组合字符/超长文本覆盖与顺序测试；部分失败不 Allow；逐片日志无正文 | chunk_total 事件样本 | 通过（自动化） |
| A10 | 独立可关联事件 | event transaction、store_pass_events、身份快照、FK/筛选、IssueSummary 派生测试 | 管理事件详情截图 | 通过（集成） |
| A11 | 真实运行态 | healthy/degraded/error、Redis/DB/Worker/节点/config version 测试 | runtime JSON 样本 | 通过（自动化） |
| A12 | 安全查询和删除 | 复合筛选、分页、单条/批量、snapshot max ID、认证 token/actor/expiry/hash、分批删除测试 | 管理审计日志 | 通过（集成） |

### 2.2 prompt-input-guard

| ID | Requirement | 必备自动化证据 | 补充证据 | 状态 |
| --- | --- | --- | --- | --- |
| G01 | 显式启用三态 | 配置真值表和非法组合测试 | 页面联动截图 | 通过（自动化） |
| G02 | 两引擎独立语义 | fake engines 全组合；Legacy Block 优先；两类事件独立 | 现有邮件/封号/Hash 回归 | 通过（自动化） |
| G03 | 门禁在副作用之前 | Block/Unavailable/Invalid 的 account/billing/upstream counter 均为 0 | Ops 请求链日志 | 通过（矩阵） |
| G04 | 覆盖所有协议入口 | routes 自动枚举/结构测试；HTTP/SSE/WS E2E 矩阵 | 已签字路由清单 | 通过（矩阵） |
| G05 | 同步分片共享预算且完整 | fake clock 总 deadline；Block 早停；Allow 全片；最后片失败测试 | p95/p99 指标 | 通过（自动化） |
| G06 | 有序 fail-closed 故障切换 | 连接/429/5xx/timeout failover；401/403/invalid 终止；bulkhead 测试 | 节点运行态 | 通过（自动化） |
| G07 | HTTP 协议兼容错误 | OpenAI/Claude 可选 code、Gemini 数值 code/status + ErrorInfo reason golden；403/503 | curl 样本（脱敏） | 通过（golden） |
| G08 | WS 每个 response.create 门禁 | 首轮/后续轮次 Allow/Block/Unavailable/Invalid 测试；4403/1013 | WS trace（无正文） | 通过（结构+golden） |
| G09 | 同步结果复用且不重复扫描 | Guard fake 调用次数=chunk 数；record failure 不改 decision；无二次调用 | event/job 关联 SQL | 通过（自动化） |
| G10 | 版本化热路径快照 | PostgreSQL CAS 并发保存、双实例 invalidation、last-known-good、cold-start fail-closed、无热路径 DB 测试 | expected/active version 指标 | 通过（集成） |
| G11 | 可观测且不泄密 | 稳定日志/指标词典测试；canary 全介质扫描 | Dashboard/runtime 截图 | 通过（自动化+扫描） |
| G12 | 禁用/回滚即时生效 | blocking→async→off 多实例测试；进行中请求边界测试 | 回滚演练记录 | 通过（自动化；生产演练待签字） |

### 2.3 security-audit-console

| ID | Requirement | 必备自动化证据 | 补充证据 | 状态 |
| --- | --- | --- | --- | --- |
| C01 | 安全审计分组和独立页面 | router/Sidebar/feature guard 测试；旧路由回归 | 侧栏和双页面截图 | 通过（自动化） |
| C02 | 清晰独立工作区 | 页面分区、独立加载/错误、dirty/reload 测试 | 桌面页面截图 | 通过（Vitest） |
| C03 | 审计池和真实探测 | endpoint CRUD draft、probe 进度/结果、token preserve/replace/clear 测试 | probe 对话框截图 | 通过（Vitest+API） |
| C04 | 范围和九类风险 | all/selected、搜索、失效 group、九类对称展示测试 | 选择器截图 | 通过（Vitest） |
| C05 | blocking 风险确认 | 开启二次确认；关闭 enabled 联动；取消确认测试 | 确认文案截图 | 通过（Vitest） |
| C06 | 保存可验证且不泄凭据 | 成功快照刷新、409 冲突保留草稿、secret state 清理、无 storage/console 测试 | Public DTO 捕获 | 通过（Vitest+扫描） |
| C07 | 真实运行态和 Guard 指标 | expected/active mismatch、Worker stale、Redis degraded、指标渲染测试 | 概览截图 | 通过（Vitest） |
| C08 | 可复核列表和详情 | filter/page/table/detail tabs、用户名/邮箱/API Key 分列复制、IssueSummary、脱敏预览测试 | 详情截图 | 通过（Vitest+API） |
| C09 | 防误删除 | 单条/批量/preview/max ID/认证 token/筛选变化失效/时间范围测试 | 删除确认截图 | 通过（集成+Vitest） |
| C10 | 管理操作审计 | config/probe/delete 成功失败审计测试；detail allowlist | audit_logs SQL/API 样本 | 通过（自动化） |
| C11 | 响应式/可访问/i18n | zh/en key 对称；键盘/focus/accessible name；窄屏测试 | 桌面与窄屏截图 | 通过（Vitest+lint） |

## 3. 标准验证命令

### 3.1 OpenSpec

```bash
cd /Users/mt/code/mt-ai/sub2api/sub2api-mt
openspec status --change add-openai-compatible-prompt-audit
openspec validate add-openai-compatible-prompt-audit --type change --strict --no-interactive
openspec show add-openai-compatible-prompt-audit
```

### 3.2 后端快速门禁

```bash
cd /Users/mt/code/mt-ai/sub2api/sub2api-mt/backend

go test ./internal/securityaudit/... -count=1
go test ./internal/handler/... ./internal/server/... -count=1
go test ./internal/service -run ContentModeration -count=1
go test -race ./internal/securityaudit/... -count=1
```

如果新模块采用单一 package，第一条可以写成 `go test ./internal/securityaudit -count=1`；以最终目录结构为准，但不能省略 race。

### 3.3 PostgreSQL/Redis 集成

项目已有基于 Testcontainers 的 integration harness。实现时把 Prompt Audit migration/Repository/Redis 场景接入同一模式：

```bash
cd /Users/mt/code/mt-ai/sub2api/sub2api-mt/backend
go test -tags=integration ./internal/repository ./internal/securityaudit/... -run 'PromptAudit|PromptGuard' -count=1
go test -tags=integration -race ./internal/securityaudit/... -run 'MultiWorker|MultiInstance|Lease|ConfigInvalidation' -count=1
```

CI 中 Docker 不可用必须失败；本地跳过要在证据中明确写“未执行”，不能标为通过。

### 3.4 前端

```bash
cd /Users/mt/code/mt-ai/sub2api/sub2api-mt
pnpm --dir frontend run lint:check
pnpm --dir frontend run typecheck
pnpm --dir frontend exec vitest run \
  src/features/prompt-audit \
  src/views/admin/__tests__/RiskControlView.spec.ts \
  src/router/__tests__/feature-access.spec.ts
pnpm --dir frontend run build
```

### 3.5 全量

```bash
cd /Users/mt/code/mt-ai/sub2api/sub2api-mt
make test-backend
make test-frontend
make build
```

保存命令、commit SHA、开始/结束时间、退出码和 CI artifact URL。不要只保存终端截图。

## 4. 模式和协议验收矩阵

### 4.1 运行模式

| risk_control | prompt enabled | blocking | 期望 Prompt 行为 | 主请求 |
| --- | --- | --- | --- | --- |
| false | 任意 | 任意 | off | 完全保持升级前行为 |
| true | false | false | off | 完全保持升级前行为 |
| true | false | true | 配置保存失败 | 无运行态变化 |
| true | true | false | async enqueue | 无论审计依赖成败都按原流程 |
| true | true | true | blocking evaluate | Block/Unavailable/Invalid fail-closed |

### 4.2 HTTP/SSE/WS

每行都要分别验证 benign、flag、block、Guard unavailable、invalid response；Legacy moderation 还需追加“Legacy 单独 Block”和“两者同时 Block”。

| 入口 | 非流式 Allow | SSE/流式 Allow | Prompt Block | Unavailable | Invalid | 必查副作用 |
| --- | --- | --- | --- | --- | --- | --- |
| OpenAI Chat Completions | 原 envelope | Guard 前 0 bytes，之后原流 | 403 `prompt_guard_blocked` | 503 `prompt_guard_unavailable` | 503 `prompt_guard_invalid_response` | account/billing/upstream |
| OpenAI Responses + aliases | 原 envelope | 同上 | 403 OpenAI-compatible | 503 | 503 | account/billing/upstream |
| Claude Messages | 原 envelope | 同上 | 403 Anthropic envelope | 503 Anthropic envelope | 503 Anthropic envelope | account/billing/upstream |
| Gemini generateContent | 原 envelope | 原流式行为 | 403 Google envelope + ErrorInfo reason | 503 + ErrorInfo reason | 503 + ErrorInfo reason | account/billing/upstream |
| Images/Grok media 文本入口 | 原 envelope | 保持原 keepalive 时序 | 403 | 503 | 503 | image slot/billing/upstream/task |
| Responses WS first turn | 正常继续 | N/A | close 4403 blocked | close 1013 unavailable | close 1013 invalid | user/account slot、billing、dial |
| Responses WS subsequent | 本轮继续 | N/A | close 4403，stage=subsequent_turn | close 1013 | close 1013 | 本轮 slot、billing、upstream write |

SSE 测试不能只断言最终状态；必须在 Guard fake 阻塞时读取连接并证明还没有 header/首字节/keepalive。

WS 测试必须检查 close code、短 reason、stage 日志和上游帧计数；不能把所有 1013 错误都写成同一内部错误事实。

## 5. 无账号、无计费、无上游证明

### 5.1 测试装置

在每类 Handler E2E 测试注入以下可计数 fake/stub：

```text
account_select_calls
user_slot_acquire_calls
account_slot_acquire_calls
subscription_or_balance_check_calls
billing_preconsume_calls
usage_write_calls
upstream_dial_calls
upstream_http_calls
upstream_ws_write_calls
async_media_task_create_calls
```

对 Prompt Block、Unavailable、Invalid 分别断言所有适用计数为 0。若基础鉴权必须读取 API key/user/group，这不算“账号选择”；证据需区分认证主体读取与上游 account scheduler。

### 5.2 数据库前后快照

除 fake counter 外，测试还应记录请求前后这些业务表/统计不变：

- usage/billing/余额/订阅消费记录。
- API key/account quota 与 rate limit 计数。
- 上游请求/任务记录。
- 图片/媒体占用或预扣记录。

允许新增的只有 Prompt Audit 自己的脱敏 blocking job/event、结构化日志和指标。现有 Content Moderation 同时命中时，它原本会产生的记录/封号/邮件仍按既有行为执行。

### 5.3 日志断言

同步拒绝事件必须包含：

```text
upstream_dispatched=false
billing_preconsumed=false
stage=http|first_turn|subsequent_turn
error_code=<stable code>
```

不得仅依赖日志证明无副作用；日志必须与 counter 和 DB snapshot 同时通过。

## 6. PostgreSQL 验证

### 6.1 Schema

在 migration 集成测试中验证：

- 两表、所有列、默认值、CHECK、索引和 FK 删除行为。
- username/email/API Key name 快照分别落在显式列，API 分列返回；`issue_summaries` 从事件事实派生而非重复落列。
- migration 从空库和当前生产前一版本都能应用。
- 不改 `content_moderation_logs` 定义和数据。
- 关键索引被典型筛选/claim 查询使用；对代表性数据运行 `EXPLAIN`。

禁止列检查示例：

```sql
SELECT table_name, column_name
FROM information_schema.columns
WHERE table_name IN ('prompt_audit_jobs', 'prompt_audit_events')
  AND lower(column_name) ~ '(raw|prompt_text|request_body|payload|token|authorization|secret)';
```

期望 0 行。`prompt_hash` 和 `redacted_preview` 是允许字段，但必须用 canary 行为测试证明内容安全。

### 6.2 原子性与并发

至少覆盖：

- 1000 个 queued/retry jobs，由 8 个 Worker、2 个 service 实例消费，每个 job 最多一个最终 event。
- 两实例同时争抢最后 N 个 queue slots，admission lock 后 active jobs 不超过 capacity；锁超时只丢弃审计任务。
- Worker 在 claim 后崩溃，租约到期由另一 Worker reclaim。
- 旧 Worker 恢复后，旧 claim_version 的 refresh/event/done/retry/failed 全部 affected rows=0，无法覆盖新领取者状态或创建重复事件。
- staging 在 Redis SET 前不可领取。
- 进程在 Redis SET 与 queued publish 间退出，staging 被回收且 payload TTL 到期。
- event + done 事务中 event insert 失败时不得留下 done 无 event 的风险任务。
- delete-by-filter 与并发新事件/查询同时运行时，只删除 id≤snapshot_max_id；伪造、过期或其他管理员的 confirmation_token 均失败。

## 7. Redis 验证

必须验证：

- key 格式仅包含 job ID，不包含 user email、Prompt hash、模型文本或 token。
- value 是唯一允许的完整 scan text 存放处，默认 TTL 为 1800 秒，测试容差考虑执行时间。
- worker 成功/终态后主动 DEL；DEL 失败最终依靠 TTL。
- Redis 不可用时 async 主请求继续、job 明确 failed/staging 回收、runtime degraded。
- blocking 不依赖 Redis payload 才能决定当前请求，但配置通知失败时按 last-known-good/TTL refresh 规则运行。
- invalidation channel 只发布 config version，不发布 JSON 配置或 token。

测试代码可以读取 canary value 做等值/TTL 断言，但不得把 value 输出到 `t.Log`、CI artifact 或失败消息。失败时只输出 job ID 和长度/hash。

## 8. 多实例配置测试

启动两个 PromptService 实例，共享 PostgreSQL/Redis、使用独立内存快照：

1. A 保存 config v2，A 立即 active=v2。
2. B 收到 invalidation，从 settings 加载并 active=v2。
3. 在 B 人为制造一次解密/加载失败，B 保持 v1 且 runtime expected=v2/active=v1/degraded。
4. 修复依赖后，B 通过下一通知或 5 秒有界刷新到 v2。
5. Redis Pub/Sub 中断时保存 v3，A active=v3，B 最迟通过 TTL refresh 收敛。
6. 冷启动 C 无法加载、期望 blocking 时，C 不得按 off 放行，runtime 必须 error/degraded。
7. A/B 两个管理员以同一 expected version 并发保存，只有一个成功，另一个得到 409；config_version 不重复且成功配置不被静默覆盖。

证据记录版本和错误码，不记录 endpoint token/base URL query。

## 9. Canary 敏感信息门禁

### 9.1 Canary 设计

每次测试生成唯一、不像普通文本的随机 canary：

```text
PROMPT_CANARY_<random>
GUARD_TOKEN_CANARY_<random>
AUTH_CANARY_<random>
URL_QUERY_CANARY_<random>
```

不要在 shell 命令行或 CI 参数中直接传真实 secret；由测试进程生成并只在测试内存保存。

### 9.2 检查介质

| 介质 | 允许 | 禁止 | 证据 |
| --- | --- | --- | --- |
| PostgreSQL | SHA-256、脱敏预览、长度、分类 | 完整 Prompt、token、Authorization、Guard raw body | 扫描所有 text/json 列 |
| Redis key/metadata | job ID、TTL | Prompt/hash/email/token 出现在 key/channel | SCAN/channel payload 断言 |
| Redis value | 完整 Prompt，TTL≤1800 | token/Authorization；终态长期残留 | 程序内检查，不打印 value |
| 应用日志 | ID、长度、状态、稳定错误码 | 四类 canary、完整 URL/query、raw response | 捕获 sink 后字节扫描 |
| 管理 API | 脱敏 preview、has_token/status、分列身份、派生风险摘要 | token/ciphertext/canary Prompt 原文 | 序列化响应扫描 |
| 客户错误 | 通用消息、code、request ID | 分类证据、Prompt、endpoint、内部错误 | HTTP/WS body/reason 扫描 |
| 前端状态 | 公共 DTO、空 secret state | 保存后的 token、session/local storage、console | Vitest spies/state snapshot |
| 页面截图 | 脱敏预览、状态 | token、完整 Prompt | OCR/文本与人工复核 |

数据库扫描必须覆盖 `TEXT/VARCHAR/JSON/JSONB`，不能只查两张新表；至少还要查 settings、audit_logs、ops/error logs 和可能的 request log 表。日志捕获应覆盖成功、探测失败、超时、invalid response、DB/Redis 错误和删除操作。

发现任何 Guard token/Authorization 泄露是 blocking release 级别 P0：立即停止启用、删除不安全 artifact、轮换凭据并做影响范围调查。Prompt canary 出现在 PostgreSQL/日志/API/前端同样禁止发布。

## 10. 现有内容审核兼容回归

必须保存迁移前后同一套结果：

- off：OpenAI Moderations、关键词、Hash、API Key 健康、异步/同步模式行为相同。
- Legacy Block：状态码、`content_policy_violation`、客户端文案不变。
- 违规计数、邮件、auto-ban、unban、hash delete/clear 不变。
- `/admin/risk-control` config/status/logs/API-key test/unban/hash API 不变。
- `content_moderation_logs` 行内容和清理行为不变。
- `RiskControlView.vue` 加载、保存、测试、列表和功能总开关不变。
- 两引擎同时 Block：客户端仍得到 Legacy Block；Prompt event 独立存在，不触发现有副作用第二次执行。

建议在新增模块前把现有 `ContentModeration` 和 `RiskControlView` 测试输出保存为基线，最终对同 commit 运行一次差分对比。

## 11. Async 灰度观测

### 11.0 当前实施基线（非生产准入）

2026-07-16 在本地 async Worker 完整处理路径运行 `TestPromptAuditSyntheticAsyncBaseline`，使用 100 条无敏感信息的确定性测试分组语料：90 benign、5 flag、3 critical、1 invalid、1 timeout。结果为 P50=5ms、P95=5ms、P99=5ms、Guard 失败率=2%、已知 benign 误报率=0%、已知 critical 阻断率=100%、`store_pass_events=false` 时事件增长=8/100。该结果只证明指标链路、分母和事件策略可用，不代表真实 Guard/网络/业务流量性能，也不能替代下述 72h/10k 生产前 async 观测。

复现命令：

```bash
cd /Users/mt/code/mt-ai/sub2api/sub2api-mt/backend
go test ./internal/securityaudit -run TestPromptAuditSyntheticAsyncBaseline -count=1 -v
```

### 11.1 先决条件

- Prompt Audit enabled=true、blocking=false。
- 只选择内部测试 group，不全量。
- 至少两个通过真实 probe 的 Guard endpoint；token 已验证且未出现在任何日志/API。
- runtime active=expected，DB/Redis/Worker healthy。
- store_pass_events 默认 false，避免一开始放大事件量；指标仍统计 Pass。

### 11.2 最少观测窗口

推荐至少连续 72 小时且 ≥10,000 个合格请求；流量不足时延长到 7 天。记录：

- enqueue total/skipped/dropped 及原因。
- queued/processing/retry/failed/staging age 和队列容量占比。
- Worker active、处理吞吐、claim/reclaim、payload missing。
- Guard Allow/Flag/Block/Unavailable/Invalid/timeout/failover/bulkhead。
- 每 endpoint 与总体 P50/P95/P99。
- 分类分布、人工抽检误报率、已知恶意回归漏报率。
- 事件增长率、索引查询 P95、删除批次耗时。
- config version 收敛时间与 reload failure。

完整 Prompt 不得作为人工抽检材料从 Redis 导出。复核使用脱敏预览、类别证据和专门构造的无敏感测试语料；如业务确需原文复核，必须另起隐私/审批 change。

## 12. Blocking 准入和退出阈值

以下是建议初始门槛，最终值必须由安全、运营和业务责任人在上线记录中签字；未签字只能保持 async：

| 指标 | 建议准入阈值 | 建议紧急退出阈值 |
| --- | --- | --- |
| 健康 endpoint | ≥2，连续 72h | <1 个可用立即退出 |
| Guard Unavailable | 24h <0.1% | 5 分钟 ≥1% |
| Invalid response | 24h <0.01% | 5 分钟 ≥0.1% 或连续出现 |
| Guard 延迟 | P95 ≤500ms，P99 ≤1000ms | P99 >2000ms 持续 10 分钟 |
| bulkhead reject | 24h <0.05% | 5 分钟 ≥0.5% |
| async dropped/payload missing | <0.01%，payload missing=0 | 任一持续增长 |
| 人工确认误报率 | <0.5%，高价值流程为 0 | 任一严重合法流量阻断事件 |
| 已知恶意语料 | critical 用例 100% Block | 任一 critical 漏报 |
| config version 收敛 | 99.9% 实例 <10s | 任一实例 stale >60s |
| canary 泄露 | 0 | 任意命中立即停用并轮换 |

延迟阈值还必须低于目标接口现有首字节 SLO 允许的新增预算；若业务 SLO 更严格，以更严格值为准。

首次 blocking：

1. 仅一个内部 group，短窗口、有人值守。
2. 确认页面二次提示、指标和告警均工作。
3. 执行 benign/flag/block/unavailable/invalid 合成请求。
4. 证明拒绝时 account/billing/upstream 仍为 0。
5. 观察至少一个高峰窗口后再扩大 group。

### 12.1 值班检查步骤

开启 blocking 前、每次扩组前和收到告警后，值班人员按以下固定顺序执行；任一项不满足立即保持/恢复 async：

1. 打开 `/admin/prompt-audit`，确认 effective mode、expected/active config version、Worker heartbeat、PostgreSQL、Redis 和至少两个 endpoint 均健康。
2. 检查最近 5 分钟 Guard Unavailable、Invalid、timeout、bulkhead、P95/P99 和 async dropped 是否超过本节阈值。
3. 检查 queued/retry/staging 最老年龄和容量占比；队列持续增长或 staging 未回收时禁止扩组。
4. 对 benign、flag、block、unavailable、invalid 合成用例各执行一次，核对协议 envelope、错误码及 `upstream_dispatched=false`、`billing_preconsumed=false`。
5. 抽查最新风险事件的脱敏预览、身份分列、分类和 IssueSummary，禁止从 Redis 导出原文。
6. 记录值班人、时间、config version、测试 group、指标快照和结论；扩组必须由安全、运营、业务责任人共同确认。

一键回滚动作固定为：在独立页面关闭 `blocking_enabled` 并保存，等待所有实例 `active_config_version == expected_config_version` 且 effective mode=`async_audit`。若管理页面不可用，使用同一管理员 API 的 GET config 取得版本，只修改 `blocking_enabled=false` 并携带 `expected_config_version` PUT 回去；不得直接修改 settings JSON。若 async 仍造成压力，再关闭 `enabled`。全局 `risk_control_enabled` 只作最后手段，因为它也会停用既有内容审核。

## 13. 回滚清单

### 13.1 一键功能回滚

1. 在 `/admin/prompt-audit` 关闭 `blocking_enabled` 并保存。
2. 确认所有实例 `active_version == expected_version`，有效模式变为 async_audit。
3. 用 benign 请求证明立即恢复原主流程；用 Guard unavailable 合成请求证明不再返回 503。
4. 继续观察 async，保留事件用于复盘。

若 async 本身引发 DB/Redis 压力或隐私问题：

1. 关闭 `enabled`，有效模式变为 off。
2. 停止 Worker 领取新任务；有界等待活动任务。
3. queued/retry 保留等待明确处置，不自动删除历史证据。
4. 若发生 secret 泄露，轮换 endpoint token。

只有在 Prompt 配置通道无法使用且影响仍持续时，才关闭全局 `risk_control_enabled`；这会同时停用现有内容审核，是最后手段。

### 13.2 数据和部署回滚

- 不回退已应用的 migration，不 drop 两张表，不删除历史事件。
- 可部署上一版本应用；新表和 setting key 保持向后兼容、无人读取。
- 停 Worker 不改变现有网关能力；恢复后按状态继续或由管理员明确清理。
- 回滚后记录触发时间、阈值、config version、影响 group、错误码分布和恢复时间。

### 13.3 回滚验收

- 所有实例模式正确，stale config=0。
- 新 403/503/4403/1013 已停止（除现有审核自己的响应）。
- 上游成功率、首字节延迟恢复基线。
- 队列不继续增长，Redis payload 最迟按 TTL 清除。
- 现有 `/admin/risk-control` 和 Content Moderation 仍正常。

## 14. 最终发布签字模板

| 项目 | 结果/链接 | 责任人 | 时间 |
| --- | --- | --- | --- |
| 源基线冻结 | TODO | TODO | TODO |
| OpenSpec strict validate | TODO | TODO | TODO |
| 后端 unit/race/integration | TODO | TODO | TODO |
| 前端 lint/typecheck/Vitest/build | TODO | TODO | TODO |
| 协议与无副作用矩阵 | TODO | TODO | TODO |
| canary 泄露门禁 | TODO | TODO | TODO |
| async 72h/10k 报告 | TODO | TODO | TODO |
| blocking 阈值批准 | TODO | TODO | TODO |
| 告警和值班人 | TODO | TODO | TODO |
| 回滚演练 | TODO | TODO | TODO |

任何必填项为 TODO、失败或无证据时，不得开启生产 blocking。

## 15. 2026-07-16 实施验证记录

验证基线：branch=`dev`，HEAD=`a2779cd5f30d6d3904a9d59088aed09507678dfe`，工作区包含本 change 的未提交实现；时间为 2026-07-16 CST。以下命令退出码均为 0，除首次发现并修复的 lint 问题外不隐藏失败。

| 门禁 | 实际证据 |
| --- | --- |
| OpenSpec | `openspec validate add-openai-compatible-prompt-audit --type change --strict --no-interactive` → valid |
| SecurityAudit 单元/集成 | PostgreSQL `127.0.0.1:32768`、Redis `127.0.0.1:32769` 下 `go test ./internal/securityaudit/... -count=1` → pass |
| Race | 同一真实依赖下 `go test -race ./internal/securityaudit/... -count=1` → pass |
| Migration/Repository/Config | `TestPromptAuditConfigCASSecretRoundTripInvalidationAndTTL`、migration/schema、admission/fencing/FK/high-water/concurrent delete、Redis TTL、Worker lifecycle 全部 pass |
| Handler/Routes | `go test ./internal/handler/... ./internal/server/... -count=1` → pass；路由矩阵由 `TestEveryGatewayPOSTRouteIsClassifiedForPromptAuditCoverage` 固定 |
| 全量后端 | 临时安装 CI 同版 golangci-lint v2.9 后 `make test-backend` → 全量 Go tests pass，`0 issues` |
| 前端 | ESLint pass；vue-tsc pass；Prompt Audit、RiskControl、Sidebar、router 共 8 个文件 34 tests pass |
| 生产构建 | `make build` → Go binary 与 Vite production build pass，独立 `PromptAuditView` chunk 生成 |
| 协议/副作用矩阵 | 13 个实际入口的 Guard-before-side-effect 结构测试；Block/Unavailable/Invalid counter=0；OpenAI/Responses/Claude/Gemini golden；WS 4403/1013；first/subsequent gate；媒体 task/billing gate 全部 pass |
| 泄露门禁 | 统一 canary 覆盖日志、DB row、管理 JSON、前端保存后 DOM；测试 PostgreSQL 39 个 text/json 列全库扫描 0 命中；Redis key/channel scan 0 命中；feature 源码无 local/session storage 或 console |
| Async 指标基线 | 100 条合成 async Worker 样本：P50/P95/P99=5/5/5ms，failure=2%，known-benign false-positive=0%，event growth=8/100；只用于验证观测链路 |
| Deploy 容器 | Docker Hub 超时后使用已缓存的正式运行层 + 当前 `linux/arm64` embed release binary 构建离线增量镜像 `sha256:c86353b0...`；Compose 重建后 app/PostgreSQL/Redis healthy，migration 181 已登记，两张表存在，`/health`=200 |
| Deploy 管理 API | 本地测试管理员登录成功；`GET config/runtime/events` 均为 200；默认 config=`enabled=false, blocking=false, mode=off, version=1, group_ids=[], endpoints=[]`；runtime active/expected=1/1 |
| Deploy 页面 | 首次容器检查发现并修复默认 `group_ids:null` 导致的运行时错误；重建后桌面/390px 窄屏 DOM 与截图均通过，截图不含 token/Prompt canary |
| Deploy 全介质扫描 | 完整生产测试库所有 public text/varchar/json/jsonb 列动态扫描：hit_columns=0/hit_rows=0；两表禁用列=0；Redis canary key=`0`、channel=`[]`、payload key=`0`；容器日志 canary=0 |

### 15.1 Requirement 自动化证据索引

- A01/G01/G02/G12：`coordinator_test.go`、`prompt_config_test.go`、`prompt_config_integration_test.go`、原 ContentModeration/RiskControl 回归。
- A02/A03/G06：`prompt_outbound_security_test.go`、`prompt_qwen3guard_test.go`、配置 secret 往返与 probe handler 测试。
- A04/A05/A09：`prompt_snapshot_test.go`、`TestPromptAuditDatabaseAndAdminJSONNeverPersistCanaryPromptOrRawErrors`、schema leakage gate。
- A06/A07：`prompt_worker_test.go`、Redis payload 集成、Repository admission/claim_version/reclaim 集成和 race。
- A08：Qwen3Guard strict/alias/unknown/aggregate/IssueSummary tests。
- A10/A12/G09：event transaction、FK/filter/high-water/confirmation/concurrent delete、record-once tests。
- A11/G10/G11：runtime aggregation、config invalidation/TTL、metrics/log dictionary/canary tests。
- G03/G04/G07/G08：`security_audit_order_test.go`、`security_audit_media_submit_test.go`、`security_audit_errors_test.go`、`prompt_audit_route_coverage_test.go`。
- G05：Guard complete chunks、last-chunk failure、Block early-stop、shared deadline/context tests。
- C01–C11：`frontend/src/features/prompt-audit/__tests__/`、Sidebar/router/RiskControl tests、handler/admin route tests、lint/typecheck/build。

### 15.2 尚未构成生产 blocking 批准的事项

本地实现验证通过不等于生产启用批准。最终签字表中的真实 async 72h/10k 流量报告、至少两个真实 endpoint 连续健康、业务流量误报抽检、告警接线、值班人和回滚演练仍为 TODO；在这些外部运营证据完成前，生产只能保持 off 或受控 async，不能开启 blocking。

### 15.3 页面证据

- 桌面：`/Users/mt/.codex/visualizations/2026/07/16/019f6a2c-ce90-7ae2-8eca-6a3a66b837f2/prompt-audit-desktop.png`
- 390px 窄屏：`/Users/mt/.codex/visualizations/2026/07/16/019f6a2c-ce90-7ae2-8eca-6a3a66b837f2/prompt-audit-narrow.png`

截图只包含默认关闭状态、空事件/空节点和测试管理员展示名；未配置节点 token、未产生 Prompt 事件，并已目视确认没有 canary、Authorization、完整 Prompt 或内部错误。截图完成后测试库已恢复 `risk_control_enabled=false`，Prompt Audit 保持默认 off。

## 16. 偏离审计与修正记录（2026-10-20）

### 16.1 偏离：提示词原文进入 PostgreSQL、管理 API 与前端

审计发现实现偏离了本 change 的设计约束：

- `182_prompt_audit_full_prompt.sql` 向 `prompt_audit_events` 增加了 `full_prompt TEXT` 列，
  把未脱敏的完整提示词写入 PostgreSQL。
- 该列经 `insertEvent`（`prompt_repository.go`）写入，经 `eventDetailColumns`
  （`prompt_event_repository.go`）读出并由 `PromptSnapshot.FullPrompt` 序列化进管理 API 响应，
  前端 `EventDetailDialog.vue` 优先展示 `full_prompt`。
- 中英文 i18n 键 `promptFull` / `promptFullHint` 明确描述"完整提示词已随事件入库"。

违反条款：`design.md:39`、`proposal.md:43`、`specs/prompt-input-audit/spec.md:81`、
`specs/security-audit-console/spec.md:113`。

同时 `182` 引入的 `full_prompt` 不匹配原泄露门禁的 forbidden 列名单
（`raw_prompt, raw_request, payload, token, authorization, credential, ciphertext`），
因此 `TestPromptAuditMigrationSchemaAndLeakageGate` 静默通过，`verification.md` 第 15 节
"39 个 text/json 列全库扫描 0 命中" 的结论在该列存在时不再成立。

### 16.2 修正

| 修正项 | 证据 |
| --- | --- |
| 删除 `prompt_audit_events.full_prompt` 列 | `251_prompt_audit_drop_full_prompt.sql`；不修改已应用的 182，保持迁移不可变；本地库已执行，`information_schema` 复查该列已不存在 |
| 移除 `PromptSnapshot.FullPrompt` 及写入/读取路径 | `prompt_types.go`、`prompt_repository.go`（`insertEvent` 不再写该列）、`prompt_event_repository.go`（`eventDetailColumns`/`scanEvent` 不再读该列） |
| Worker 不再从 Redis 载荷反推原文 | `prompt_worker.go` 移除 `FullPromptFromScanText` 调用，事件只保留脱敏快照 |
| 前端只展示脱敏预览 | `features/prompt-audit/types.ts` 移除 `full_prompt`；`EventDetailDialog.vue` 的 `displayPrompt` 只读 `redacted_preview`；中英文 `promptFull`/`promptFullHint` 改写 |
| 泄露门禁加固 | `prompt_repository_integration_test.go` 的 forbidden 名单加入 `full_prompt`，并断言 `information_schema` 中该列计数为 0 |
| 泄露测试语义纠正 | `TestPromptAuditDatabasePersistsFullPromptOnEventsOnly` 改写为 `TestPromptAuditDatabaseAndAdminJSONNeverPersistCanaryPromptOrRawErrors`：断言 canary 原文不出现在事件行 `row_to_json`、管理 API JSON 与事件详情中 |

### 16.3 偏离：两处入口的同步门禁晚于计费资格检查

`grok_audio.go`（`GrokVoice`）与 `gateway_web_search.go`（`WebSearch`）的
`checkSecurityAudit` 位于 `CheckBillingEligibility` 之后。`CheckBillingEligibility`
内部的 `checkRPM` 会执行 `IncrementUserGroupRPM` / `IncrementUserRPM`，
因此被 Guard 拦截的请求仍会消耗 RPM 配额，违反
`specs/prompt-input-guard/spec.md:43` 与 `:47`（"计费检查/预扣次数 MUST 均为 0"）。

原结构测试 `TestPromptAuditGatePrecedesAccountBillingAndUpstreamSideEffects`
未覆盖这两个文件，缺陷因此未被门禁发现。

修正：两处门禁上移到 `CheckBillingEligibility` 之前，并把两个文件纳入该结构测试；
测试现覆盖 15 个入口，全部通过。

### 16.4 修正后的验证

- `go build ./...`：通过。
- `go vet ./internal/handler/... ./internal/securityaudit/...`：通过。
- `go test ./internal/handler/ -run TestPromptAuditGatePrecedesAccountBillingAndUpstreamSideEffects -count=1`：
  15 个子用例全部 PASS（含新增的 `grok_audio.go/GrokVoice`、`gateway_web_search.go/WebSearch`）。
- `openspec validate add-openai-compatible-prompt-audit --type change --strict --no-interactive`：
  valid。
- 本地 PostgreSQL 执行 `251_prompt_audit_drop_full_prompt.sql` 后，`prompt_audit_events`
  的 `full_prompt` 列已不存在。

### 16.5 对既有结论的影响

- 第 15 节"泄露门禁"与"Deploy 全介质扫描"的结论对应的是 `full_prompt` 引入之前的状态；
  本节 16.2 的修正恢复了该不变量，但**上线前必须重新执行一次全库 canary 扫描**，
  以覆盖本次修正后新写入的事件行。
- 最终发布签字表（第 14 节）仍全部为 TODO，结论不变：未完成运营证据前，
  生产只能保持 off 或受控 async，不能开启 blocking。

### 16.6 端到端验证记录（2026-10-20，本地容器）

在本地 dev 容器（重新构建并运行含本次修正的镜像）上，用一个 OpenAI 兼容的
Qwen3Guard 桩服务（可切换 Safe / Unsafe / 重复 Safety 行的非法响应）执行端到端验证，
覆盖 28 项断言，全部通过。

| 编号 | 场景 | 期望 | 实测 |
| --- | --- | --- | --- |
| T0 | 管理员登录、准备测试用户/API Key/分组 | 就绪 | 通过 |
| T1 | 保存 Guard 节点配置 + 节点探测 | 保存成功、探测 healthy | config_version 递增、healthy (3ms) |
| T2 | 异步模式发请求 | 任务入队并被 Worker 消费 | jobs=1 events=1 |
| T3 | 同步模式 + Unsafe 提示词 | 403 `prompt_guard_blocked`，事件落库 | 403 / `prompt_guard_blocked` / events 1→2 |
| T4 | Guard 节点不可达 | 503 `prompt_guard_unavailable`（fail-closed） | 503 / `prompt_guard_unavailable` |
| T5 | Guard 返回重复 `Safety` 行 | 503 `prompt_guard_invalid_response` | 503 / `prompt_guard_invalid_response` |
| T6 | 泄露检查 | PG 无 `full_prompt` 列；事件行/任务行/Admin API 均无 canary 原文；事件详情仍返回脱敏预览 | 全部通过；预览为 `PROMPT_AUDIT***…`，判定 `critical`/`Block`，分类 `violent,jailbreak` |
| T7 | 门禁顺序 | 3 次被拦截请求均为 403 且不推进 RPM；放行请求推进 RPM | 被拦截 RPM 1→1 不变；放行后 RPM→3 |

T7 的关键设计：分组 `rpm_limit` 临时设为 2 并失效 API Key 认证缓存，使 RPM 计数器真实生效。
先发 3 次必被拦截的请求（均为 403 而非 429，且计数不变），再把 Guard 切到 Safe 发 2 次放行请求
（计数推进到 3）。双向证明「门禁在计费之前」与「计费检查确实会执行」，避免因计数器从未生效
而产生的假阳性。

T6 的 Redis 短 TTL 载荷在 Worker 处理完成后为 0 个 key，符合"处理完即删除"的预期。

#### 16.6.1 已知限制

- 上游 Gemini 账号本身返回 404（`Resource not found`），因此 T2/T7 的"放行请求"最终状态码是 404
  而非 2xx。该状态码来自上游，与提示词审计无关；审计链路（入队、扫描、落库、拦截）均已独立验证。
- 本记录不构成生产 blocking 批准。第 14 节最终签字表仍全部为 TODO，真实 async 72h/10k 流量报告、
  至少两个真实 endpoint 连续健康、业务流量误报抽检、告警接线、值班人与回滚演练仍未完成；
  在这些运营证据补齐前，生产只能保持 off 或受控 async。

### 16.7 双平台真实上游验证（2026-10-20）

在本地容器上用**两个真实上游**验证提示词审计链路，不使用模拟上游响应：

| 平台 | 账号类型 | 分组 | 端点 | 模型 |
| --- | --- | --- | --- | --- |
| Gemini | AI Studio API Key | 3 | `/v1/chat/completions`、`/v1beta/models/{model}:generateContent` | `gemini-3-flash-preview` |
| OpenAI | Codex OAuth (plus) | 4 | `/v1/responses`、`/v1/chat/completions` | `gpt-5.6-luna` |

先确认上游本身可用：Gemini 直连 `generativelanguage.googleapis.com` 与 OpenAI 直连
`chatgpt.com/backend-api/codex/models` 均成功；两个模型在本地账号的 `model_mapping` 中存在
（`gemini-2.0-flash` 因该 Key 无新用户权限而不可用，`gpt-6-luna` 不在账号映射内，故改用
`gemini-3-flash-preview` / `gpt-5.6-luna`）。

#### 16.7.1 双平台矩阵（24 项断言，全部通过）

| 编号 | 场景 | Gemini | OpenAI |
| --- | --- | --- | --- |
| A | 放行模式透传到真实上游 | 200，内容 `OK` | 200，内容 `OK` |
| B | 异步事件落库且协议/模型正确 | `openai_chat_completions` / `gemini-3-flash-preview` | `openai_responses` / `gpt-5.6-luna` |
| C | 同步阻止 | 403 `prompt_guard_blocked` | 403 `prompt_guard_blocked` |
| D | Guard 不可达 → fail-closed | 503 `prompt_guard_unavailable` | 503 `prompt_guard_unavailable` |
| E | 泄露检查（9 项） | PG 无 `full_prompt` 列；事件/任务行、Admin API 列表与逐条详情均无 canary 原文与 `full_prompt` 字段 | 同左 |

#### 16.7.2 原生协议验证（14 项断言，全部通过）

| 编号 | 场景 | 结果 |
| --- | --- | --- |
| A | Gemini 原生 `/v1beta` 放行 | 200，内容 `OK` |
| B | OpenAI `/v1/chat/completions` 放行 | 200，内容 `OK` |
| C | 协议识别 | Gemini 原生事件 `protocol=gemini`；OpenAI cc 事件 `protocol=openai_chat_completions` |
| D | Gemini 原生阻断的 Google 错误信封 | HTTP 403；`error.code` 为**数值 403**；`error.status=PERMISSION_DENIED`；`google.rpc.ErrorInfo.reason=prompt_guard_blocked` |
| E | OpenAI cc 阻断 | 403 `prompt_guard_blocked`，`type=permission_error` |
| F | 原生协议事件泄露检查 | 事件行与 Admin API 均无 canary 原文、无 `full_prompt` 字段 |

#### 16.7.3 测试过程中修正的自身问题

均为测试脚本缺陷，未暴露产品缺陷，但记录以避免误读历史结论：

1. `expected_config_version` 硬编码为 1 → 保存被乐观锁以 409 拒绝，改为先 GET 真实版本号。
2. `"blocking_enabled": 0`（JSON 数字）→ 字段为 `bool`，`ShouldBindJSON` 绑定失败返回
   `prompt_audit_invalid_config_request`；改为传 `true`/`false`。
3. RPM 计数器扫描模式只匹配 `rpm:u:` → 漏掉分组级 `rpm:ug:`，造成"计数器未推进"的假阴性。
4. 分组 `rpm_limit` 直接改 SQL 不会失效 API Key 认证缓存 → 快照仍为 `rpm_limit=0`，需一并删除
   `apikey:auth:*` 缓存键。
5. 对照组请求未把 Guard 切到 Safe → 对照请求同样被 403 拦截，无法证明计数生效。

#### 16.7.4 已知限制

- 本节只验证提示词审计链路本身。上游账号的额度、模型权限、OAuth 有效期等仍由各自平台决定，
  审计层不会改变上游语义。
- 本记录同样不构成生产 blocking 批准，理由同 16.6.1。

### 16.8 上游同步与冲突解决记录（2026-11-10）

#### 16.8.1 上游同步过程

本变更最初基于 `881f32026`（2026-09-15）开发，随后上游持续演进。同步过程与结论：

| 阶段 | 上游 HEAD | 与本分支差距 | 处理 |
| --- | --- | --- | --- |
| 初次定位基线 | `8f8358cc6` | 112 文件差异 | 判定为错误基线（该提交含本地不存在的新文件） |
| 精确定位基线 | `881f32026` | 29 文件差异 | 逐个候选提交取最小差异，确认真基线 |
| 首次重建分支 | `9a62841fd`（09-28） | 0 | 分支重建完成，23 文件单点 diff |
| 上游再次前进 | `ee58c0013`（11-09） | 617 提交 | rebase，见 16.8.2 |

真基线的确定方法：遍历 `origin/main` 候选提交，对每个候选计算「工作区相对该提交」的差异文件数，取最小值。
这比按日期猜测可靠 —— `881f32026` 与 `8f8358cc6` 仅差 8 天，但差异文件数相差 4 倍。

#### 16.8.2 迁移编号重编号

上游在 `881f32026` 之后已占用 `239`（`239_channel_reasoning_effort_multipliers.sql`），最大编号推进到 `250`。
本变更的删除迁移因此由 `239_prompt_audit_drop_full_prompt.sql` 重编号为 `251_prompt_audit_drop_full_prompt.sql`，
并同步更新集成测试与 `design.md` / `tasks.md` / `verification.md` 中的四处引用。
迁移编号唯一性已复查：`251` 为当前最大序号。

#### 16.8.3 冲突解决：门禁顺序不变量

上游这 617 个提交中，5 个文件与本变更重叠：

| 文件 | 上游改动 | 解决方式 |
| --- | --- | --- |
| `prompt_snapshot.go` | +75 行，新增 `typesafe_systemone` 协议提取器 | 自动合并（改动位于不同区段） |
| `prompt_snapshot_test.go` | +50 行 | 自动合并 |
| `security_audit_order_test.go` | +1 行（新增 `gemini_cached_contents_handler.go` 用例） | 自动合并，本变更新增的两条用例保留 |
| `grok_audio.go` | +45/−6 | 手工解冲突 |
| `gateway_web_search.go` | +12 | 手工解冲突 |

两个手工冲突的根因相同：上游在 `CheckBillingEligibility` 之外新增了 `reserveInflightBalance`
（余额在途预留），并在 billing 与门禁之间插入代码，与本变更「把门禁前移到 billing 之前」的修改重叠。

**解决依据不是选择某一边，而是先确定不变量**：

> 门禁必须早于**所有**计费副作用，而不只是 `CheckBillingEligibility`。

`reserveInflightBalance` 同样会占用用户余额，因此它与 RPM 计数属于同一类副作用。
约束链闭合后顺序唯一确定：

```
1. API Key 校验 / 依赖检查
2. 读取请求体                    ← 门禁需要提示词，必须先读
3. Prompt Guard 门禁              ← 拦截点
4. CheckBillingEligibility        ← RPM 计数递增
5. reserveInflightBalance         ← 余额预留
6. 选账号 / 转发上游
```

`grok_audio.go` 的 `subscription` 声明随 `CheckBillingEligibility` 一并下移，
确保 `reserveInflightBalance` 仍在 `subscription` 声明之后。

**附带的行为变化（已在 PR 描述中说明）**：

| 变化 | 上游行为 | 修正后 |
| --- | --- | --- |
| 被 Guard 拦截的请求 | 消耗 RPM + 占用余额预留 | 两者均不消耗 |
| 请求体非法的请求 | 消耗 RPM | 不消耗 |

第二项是 `body` 读取必须前置于门禁的连带结果：读不出请求体即不应计费。
这超出「审计门禁」的最小范围，属于有意为之并主动披露的行为变化。

#### 16.8.4 顺序验证仍为双向证明

结构测试 `TestPromptAuditGatePrecedesAccountBillingAndUpstreamSideEffects` 覆盖 16 个入口
（含上游新增的 `gemini_cached_contents_handler.go`）。静态断言只能证明顺序，
副作用未发生由端到端双向证明：对照组放行请求推进 RPM 计数，实验组被拦截请求计数不变。
详见 16.7.1 与 16.6 的 T7。

#### 16.8.5 上游仍未修复

截至 `ee58c0013`，上游 `main` 仍包含 `182_prompt_audit_full_prompt.sql` 与
`PromptSnapshot.FullPrompt`，且没有任何删除迁移。本变更在最新上游基线上重新验证通过：
`go build ./...`、`go test ./internal/securityaudit/... ./internal/handler/... ./internal/server/...`、
前端 vitest（30 tests）、`vue-tsc --noEmit` 与 `openspec validate --strict` 全部通过。
