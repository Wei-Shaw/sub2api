# Grok 4.5 独立复核任务书：Sub2API P0 可靠性内核

## 直接交给 Grok 的开头语

```text
你现在是 Sub2API P0 可靠性内核的独立终审员。请进入 D:\sub2api-trunk，完整读取 docs/superpowers/codex-handoff/CODEX_TASK_GROK45_P0_RELIABILITY_AUDIT_20260710.md，并严格按其中的读取顺序、审查矩阵、验证命令、反证规则和输出契约执行。

不要相信 Codex 的完成声明，不要只阅读审查包，不要抽样 diff，也不要为了给出“通过”而降低标准。你必须以 607facce6913239418e84516b5affa061b422496 为 AUDIT_BASE，以 74c4b1ff71c8b2dd4f33f613298add92175064cc 为 CODE_EVIDENCE_HEAD，审查完整变更、当前源码、真实 Wiring、迁移、测试和命令退出码。禁止调用真实 Provider、读取或打印 .env/密钥/token/cookie、部署、push、reset、clean、rebase、删除文件或修改业务代码。

默认只读审查；唯一允许写入的文件是 docs/superpowers/codex-handoff/deliverables/2026-07-10-GROK45-RELIABILITY-AUDIT.md。若发现 P0/P1，只记录可复现证据和最小修复建议，不要直接修复。没有证据的项目必须标记“待复核”，不得写成通过。最终结论只能是：通过、条件通过、需修复、已阻塞 四选一。
```

---

## 1. 审计目标

独立验证本轮 P0 可靠性内核是否在既定范围内符合最佳实践，重点判断：

1. 任务、账务、reservation、ledger、Outbox 是否具备正确且可恢复的事务边界。
2. Provider dispatch 的并发、重试与模糊结果是否不会导致重复提交、重复扣费或未知状态误重发。
3. 终态、结算和副作用是否幂等，且副作用失败不会污染任务终态。
4. 管理 API、前端展示、reconciliation、metrics 和真实 Composition Root 是否对齐同一事实。
5. migration、配置开关、旧接口和 flag-off 路径是否保持向后兼容。
6. Codex 的审查包、测试记录和状态声明是否与仓库证据一致。

“通过”只表示本任务书定义范围内的证据闭环，不代表绝对无 Bug、生产 READY、真实 Provider 已验证或公网可用。

## 2. 固定范围与硬边界

| 项目 | 固定值 |
|---|---|
| Repo | `D:\sub2api-trunk` |
| Branch | `wujie/video-capture-moat-20260702` |
| AUDIT_BASE | `607facce6913239418e84516b5affa061b422496` |
| CODE_EVIDENCE_HEAD | `74c4b1ff71c8b2dd4f33f613298add92175064cc` |
| 完整代码审查范围 | `AUDIT_BASE..CODE_EVIDENCE_HEAD` |
| Grok 唯一允许写入 | `docs/superpowers/codex-handoff/deliverables/2026-07-10-GROK45-RELIABILITY-AUDIT.md` |

当前 HEAD 可能比 `CODE_EVIDENCE_HEAD` 多一个“本审计任务书”文档提交。先执行：

```powershell
git diff --name-status 74c4b1ff71c8b2dd4f33f613298add92175064cc..HEAD
```

允许差异只能是本文件。如果包含任何业务代码、配置、migration、测试或其他审查文档，停止并把范围漂移标为“已阻塞”。

硬边界：

- 不调用真实或付费 Provider，不触碰真实支付或生产数据。
- 不读取、打印、复制或提交 `.env`、Key、token、cookie、JWT、账号密码。
- 不 push、deploy、reset、clean、rebase、删除或覆盖文件。
- 不修改业务代码；发现问题只写审计结果。
- 保留用户已有 `?? .worktrees/`，不得进入、扫描或清理该目录。
- 不把本地测试通过写成产品 READY。

## 3. 真相源读取顺序

必须按顺序完整阅读，后文不能覆盖前文的硬边界：

1. `00_START_HERE.md`
2. `01_PROJECT_BASELINE.md`
3. `02_CURRENT_REALITY_STATUS.md`
4. `PRODUCT_INVARIANTS.md`
5. `ARCHITECTURE_GUARDRAILS.md`
6. `CODE_QUALITY_GATE.md`
7. `docs/goals/03_CURRENT_GOAL.md`
8. `docs/superpowers/specs/2026-07-10-reliability-core-design.md`
9. `docs/superpowers/plans/2026-07-10-reliability-core-implementation.md`
10. `docs/reviews/LATEST_REVIEW_PACKAGE.html`
11. `docs/superpowers/codex-handoff/deliverables/2026-07-10-RELIABILITY-CORE-review.md`
12. 本任务书

若某个文件不存在，记录精确路径和缺失事实；不要自行编造替代规则。

## 4. 本轮工作清单

### 4.1 规模

- 代码/文档证据范围：`607facce..74c4b1ff`
- 24 个本地提交。
- 74 个文件发生变化，约 13,554 行新增、576 行删除。
- 主要范围：Go 后端、PostgreSQL migration、Wire composition、Vue/TypeScript 管理前端、测试与审查文档。

### 4.2 提交序列与意图

| 阶段 | 提交 | 意图 |
|---|---|---|
| Task 1 | `9fee2e0b`, `ddd6eb59` | billing/outbox 基础、Money 边界与配置基础 |
| Task 2 | `0d26c859`, `43f4cc03` | reservation、immutable ledger、outbox repository、lease/replay 硬化 |
| Task 3 | `9b6d66ad`, `8143af1c`, `ba7b631f` | pricing adapter 契约、任务创建与余额预留、mock-free 计费 |
| Task 6 | `5b4bba7c`, `a3bfe408`, `694ae94e`, `06c7a9a4`, `a006af08`, `9ebbcd9e` | dispatch 契约、CAS、模糊提交证据、transport 保持、reservation reaper |
| Task 4 | `351a8e44`, `68493d2d`, `9ba146c0`, `b7f6fba2` | 终态与账务原子化、取消/异常、版本保护、poll 持久化重试 |
| Task 5 | `8a5d515e` | durable Domain Outbox 副作用 |
| Task 7 | `30673a55`, `2721e2e8` | 前端交付生命周期、轮询、资产事实与兼容性 |
| Task 8 | `5e9d4f23`, `198e87bc` | mock 结算/交付证明、reconciliation、metrics、真实 Wiring |
| Task 9 | `68edec77`, `74c4b1ff` | 全分支复核发现修复、真相源与最终审查包 |

### 4.3 Codex 声明的最终修复

这些只是待验证声明，不是证据：

- `billing.reservation_expired` 已注册为可确认的 Outbox 审计事件，不再落入 unknown dead-letter。
- reconciliation 不再把历史任务账务余额与用户当前余额错误比较，并优先返回最近异常。
- 被 reaper 标记为非 pending settlement 的 submitted/running 任务不再被 worker 领取。
- 远程 deliverable 只显示“结果可下载”，仅本地资产显示“本地资产可下载”。
- reconciliation、metrics、Outbox worker 已接入实际 Wire 运行图，不是测试专用代码。

## 5. 审计方法：先反证，再确认

### 5.1 证据优先规则

- Codex/Grok/其他 Agent 的自述不算证据。
- 文档中的“pass”必须被当前命令退出码或当前源码支持。
- 测试存在不等于测试有效；必须判断它是否能在实现回退时失败。
- mock-only 通过不等于真实 Provider 通过。
- `task succeeded`、`resultUrlPresent=true` 不等于资产已交付。
- 空 findings 只有在完整 diff、关键 source-to-sink 路径和所有可运行门禁都检查后才有效。

### 5.2 严重性

| 等级 | 判定 |
|---|---|
| P0 | 可导致密钥/生产数据泄露、任意越权、不可逆财务损失、广泛重复扣费或系统不可恢复 |
| P1 | 核心任务可能卡死/重复提交/错扣漏扣、账务不可审计、真实 Wiring 缺失、默认路径回归、审查包重大失真 |
| P2 | 非核心边缘错误、可维护性/性能/观测缺口，存在明确规避方法 |
| P3 | 文案、局部结构、低风险降噪建议 |

每条 finding 必须包含：`ID / severity / category / file:line / source-to-sink / reproduction or proof / impact / minimal fix / confidence`。

## 6. 必查架构不变量

### A. 创建任务与 reservation

- 用户/API Key 归属、pricing snapshot、任务和 reservation 是否在正确事务中创建。
- 金额是否全程使用精确定点表示；检查负数、溢出、舍入、币种和零金额边界。
- 并发创建是否可能超订余额；失败是否完整回滚。
- mock/free 任务是否绕过真实收费，且不能伪造 billable evidence。

### B. Provider dispatch 与模糊结果

- 从 worker 选取、claim、CAS、submit 到持久化 provider task ID 全链路追踪。
- 多 worker、lease 过期、进程崩溃、写库失败、上游超时但已接收时是否可能重复提交。
- `unknown`/ambiguous 状态是否只能 reconciliation，不能自动重发。
- 自定义 dispatch transport 是否被构造器/Wire 覆盖。

### C. Poll 与 terminal finalization

- 非终态 poll 更新是否有 version/CAS 保护，持久化失败是否正确重试。
- completed/failed/cancelled 的 task、reservation、ledger、余额和 Outbox 是否原子提交。
- finalizer 重放是否幂等；两个终态竞争时是否只能有一个胜者。
- actual cost 大于/小于 reservation 时，差额、余额不足和 review_required 是否正确。

### D. Reservation reaper

- 只处理真正过期且仍 active 的 reservation。
- queued/submitted/running 各状态行为是否一致且可解释。
- 已被 reaper 标记的任务是否绝不会进入需要 active reservation 的 finalizer。
- `billing.reservation_expired` 是否有真实注册 handler，且不会被永久 dead-letter。

### E. Domain Outbox

- 事件与业务终态同事务写入；handler 失败不回滚终态。
- claim/lease、重试、退避、幂等、dead-letter、unknown event 策略是否正确。
- content capture、archive、cache、notification 是否真正走 Outbox，而非同时存在重复直接调用。
- handler 日志不得泄露远程响应、密钥或用户敏感数据。

### F. Reconciliation 与 metrics

- dry-run 必须是纯读取；确认没有隐式修复、UPDATE、DELETE 或副作用。
- 历史 ledger 与当前余额比较不得误报；有更新交易时必须有明确处理。
- bounded query 不得长期饿死新异常；检查排序和索引成本。
- 管理端路由必须有 admin 授权，只返回最小必要字段。
- metrics 必须由生产路径调用且线程安全；测试 recorder 不能冒充运行时 wiring。

### G. API 与前端事实

- 新字段必须向后兼容；旧客户端/flag-off 路径不得被破坏。
- delivery status、next action、local asset、remote URL、过期 URL、失败原因必须基于后端事实。
- 轮询必须在终态停止，切页/卸载时取消，错误不能静默伪装成功。
- UI 不得把远程结果称为本地资产，也不得把 URL 存在等同于可交付。

### H. Migration、配置与 Composition Root

- `backend/migrations/156_reliability_core.sql` 只能向后兼容增量；不得 DROP/破坏旧数据。
- 检查约束、唯一键、外键、索引是否支撑并发正确性和主要查询。
- feature flags 默认必须关闭；flag-off 旧主链应可运行。
- 以 `backend/cmd/server/wire.go` 和生成的 `wire_gen.go` 为准，确认 repository/service/worker/handler 真正注入。
- 连续生成 Wire 后哈希一致且没有新增 diff。

## 7. 安全、性能与可维护性审查

### 安全

- SQL 参数化、命令/模板/XSS 注入、SSRF/远程资产下载边界。
- admin reconciliation 的认证、授权、IDOR 和错误信息。
- 日志、错误、DTO、审查包中是否泄露 secret、内部 provider payload、用户余额明细或本地敏感路径。
- 依赖漏洞：仅在工具已安装且不会修改 lockfile 时运行审计；不可用时记录“待复核”，不得自动安装或改依赖。

### 性能

- repository 是否存在 N+1、无界扫描、错误排序、缺失索引、长事务和锁顺序死锁风险。
- reconciliation 的 lateral/exists 查询、outbox claim、worker runnable query 是否有合理索引和 limit。
- 前端轮询是否造成重复请求、并发定时器或组件卸载后泄漏。

### 可维护性

- service/repository/handler 边界是否清楚，生成代码是否从 source provider 正确导出。
- 是否存在测试专用生产分支、重复状态机、variadic 兼容掩盖真实依赖、无上下文 warning 或吞错。
- comments、名称、错误码、DTO 与设计文档是否一致。

## 8. 必跑验证命令

每条记录开始时间、结束时间、exit code、通过/失败数量和关键输出；不要只写“pass”。命令必须逐条运行，不要用会掩盖前一条退出码的链式脚本。

### 8.1 Git 与范围

```powershell
cd D:\sub2api-trunk
git status --short
git branch --show-current
git rev-parse --show-toplevel
git rev-parse HEAD
git diff --name-status 607facce6913239418e84516b5affa061b422496..74c4b1ff71c8b2dd4f33f613298add92175064cc
git diff --check 607facce6913239418e84516b5affa061b422496..HEAD
git log --oneline 607facce6913239418e84516b5affa061b422496..HEAD
```

预期：分支正确；除本审计任务书提交外没有范围漂移；工作树只允许已有 `.worktrees/` 和 Grok 最终报告。

### 8.2 后端

```powershell
cd D:\sub2api-trunk\backend
go test ./... -count=1
go test -tags=integration ./... -count=1
golangci-lint run ./...
go vet ./...
```

如环境支持，额外运行关键并发路径 race gate；不支持时给出精确工具链原因：

```powershell
go test -race ./internal/repository ./internal/service ./internal/server/routes -count=1
```

### 8.3 前端

```powershell
cd D:\sub2api-trunk\frontend
npx.cmd eslint . --ext .ts,.vue --max-warnings=0
npx.cmd vue-tsc --noEmit
npx.cmd vitest run --reporter=basic
```

### 8.4 Secret gate 与 Wire 可复现性

```powershell
cd D:\sub2api-trunk
python tools/secret_scan.py --include-untracked
```

该脚本应跳过 `.env`/私钥类文件并且不打印 secret value。若机器 `python` 不可用，可以用已配置的 bundled Python，但必须记录绝对路径和退出码。

```powershell
cd D:\sub2api-trunk\backend
go generate ./cmd/server
(Get-FileHash cmd\server\wire_gen.go -Algorithm SHA256).Hash
go generate ./cmd/server
(Get-FileHash cmd\server\wire_gen.go -Algorithm SHA256).Hash
git diff -- cmd/server/wire_gen.go
```

预期哈希：`53EA182966735752152BD9F4C6D84CFA95AE31E74E5D5F85E7E1FF8D9C3B121A`。若当前源码合理变化导致新哈希，必须解释并确认两次一致，不能机械套用旧值。

### 8.5 可选依赖安全门禁

仅当命令已安装、网络策略允许且确认不会修改 lockfile 时运行；否则记录待复核：

```powershell
govulncheck ./...
pnpm audit --prod
```

禁止为完成审计而自动升级依赖或写 lockfile。

## 9. 测试质量反证

至少手工抽查并评价以下测试是否验证真实持久化/状态转换，而非仅验证 mock 自述：

- `backend/internal/repository/video_task_creation_repo_integration_test.go`
- `backend/internal/repository/video_task_finalization_repo_integration_test.go`
- `backend/internal/repository/billing_reservation_repo_integration_test.go`
- `backend/internal/repository/domain_outbox_repo_integration_test.go`
- `backend/internal/repository/reliability_reconciliation_repo_integration_test.go`
- `backend/internal/server/routes/video_reliability_mock_e2e_test.go`
- `backend/internal/service/video_gateway_worker_test.go`
- `backend/internal/service/video_gateway_dispatch_error_test.go`
- `frontend/src/views/admin/video/__tests__/VideoReliabilityFlow.spec.ts`
- `frontend/src/composables/__tests__/useVideoTaskLifecycle.spec.ts`

对每组关键测试回答：

1. 若删除 CAS/事务/handler 注册/本地资产判断，测试是否会失败？
2. 是否断言数据库最终状态、余额、ledger、reservation、outbox 和用户可见结果，而非只断言函数返回 nil？
3. 是否覆盖 crash window、重复执行、并发竞争、持久化失败、unknown dispatch、overrun、expired reservation 和远程/本地交付差异？
4. Testcontainers 是否真的启动 PostgreSQL，并执行 migration 156？

不允许为了做 mutation proof 而编辑业务代码。若只能通过源码推理判断，明确写“静态反证”。

## 10. 审查包真实性核对

逐项核对 `docs/reviews/LATEST_REVIEW_PACKAGE.html` 与仓库事实：

- 目标、背景、repo、branch、证据 HEAD。
- 变更规模、提交序列、架构描述和关键文件。
- 每条验证命令、exit code、测试范围和警告解释。
- 未运行的浏览器/真实 Provider/部署 gate 是否明确为待复核。
- 风险、回滚、文件索引、状态和下一步 prompt 是否可执行。
- `docs/goals/03_CURRENT_GOAL.md`、HTML 和 Markdown deliverable 是否一致。

若只是文档 hash 之后新增本任务书，不算审查包过期；若有任何业务变更晚于 `CODE_EVIDENCE_HEAD`，审查包必须判过期。

## 11. 最终判定规则

评分只能辅助排序，不能覆盖硬门禁：

| 维度 | 权重 |
|---|---:|
| 正确性、事务、并发与幂等 | 30 |
| 安全、授权与数据保护 | 25 |
| 架构、Wiring、兼容与演进 | 20 |
| 测试质量与证据真实性 | 15 |
| 性能与可维护性 | 10 |

### 通过

必须同时满足：

- P0 = 0，P1 = 0。
- 所有必跑门禁均以当前源码新鲜通过。
- 核心不变量逐条有源码、测试或数据库证据。
- 审查包没有重大失真。
- 仅保留已明确的外部 gate：真实 Provider、部署和浏览器实机闭环，并且这些没有被写成通过。

### 条件通过

- P0 = 0，P1 = 0；只有不阻断内部 mock/integration 使用的 P2/P3。
- 或 race/依赖 advisory 等增强门禁因可证明的本机工具限制未运行，但核心必跑门禁均通过。

### 需修复

- 任一可复现 P0/P1。
- 核心门禁失败、关键测试无效、真实 Wiring 缺失、migration 破坏性、flag-off 回归或审查包重大失真。

### 已阻塞

- 范围漂移、仓库/分支不匹配、必需依赖连续不可用，或无法在不触发真实 Provider/密钥/生产数据的前提下继续。

不得输出“100% 无 Bug”。可以输出“在本任务书定义范围内达到通过标准”，并列出外部未验证边界。

## 12. Grok 输出契约

唯一写入：

`docs/superpowers/codex-handoff/deliverables/2026-07-10-GROK45-RELIABILITY-AUDIT.md`

必须包含：

```markdown
# Grok 4.5 独立审计：Sub2API P0 可靠性内核

## 结论
- Verdict: 通过 / 条件通过 / 需修复 / 已阻塞
- Score: x/100
- P0/P1/P2/P3: x/x/x/x
- AUDIT_BASE / CODE_EVIDENCE_HEAD / observed HEAD

## 范围与边界确认
## 真相源完整性
## 命令证据台账
| Command | Start/End | Exit | Summary | Status |

## 核心不变量逐项判定
| Invariant | Source-to-sink evidence | Test evidence | Verdict |

## Findings
### G45-P1-001 标题
- Category:
- File:line:
- Source-to-sink:
- Reproduction/proof:
- Impact:
- Minimal fix:
- Confidence:

## 测试质量与盲区
## 审查包真实性核对
## 性能与可维护性
## 未验证项
## 回滚与修复优先级
## 最终签字
```

即使没有 finding，也必须保留 `## Findings` 并写“未发现 P0/P1/P2/P3”，随后说明完整审查过的路径和命令，不能只写一句“通过”。

完成报告后执行：

```powershell
git diff --check
git status --short
```

不要 commit、push 或修改最新审查包。把报告路径、Verdict、finding 数量、失败/未运行门禁直接回复用户。
