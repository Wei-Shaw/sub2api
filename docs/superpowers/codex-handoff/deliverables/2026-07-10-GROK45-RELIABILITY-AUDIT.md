# Grok 4.5 独立审计：Sub2API P0 可靠性内核

## 结论

- Verdict: **条件通过**
- Score: **86/100**
- P0/P1/P2/P3: **0/0/4/2**
- AUDIT_BASE: `607facce6913239418e84516b5affa061b422496`
- CODE_EVIDENCE_HEAD: `74c4b1ff71c8b2dd4f33f613298add92175064cc`
- observed HEAD: `8f5764718a02c8579678e4be67c0e9153f279a84`（仅多本审计任务书提交）

在本任务书定义范围内：未发现可复现 P0/P1；核心单元/静态门禁、Wire 可复现性与 secret scan 以当前源码新鲜通过；A–H 不变量有源码与（静态）测试反证支撑。降为“条件通过”的硬原因是：本机 Docker 不可用导致 `go test -tags=integration` **exit 0 但跳过 Testcontainers**，DB 级集成证据无法在本机复现；`-race` 因缺 CGO 未跑；依赖 advisory 未跑；浏览器/真实 Provider/部署仍为外部未验证边界。不得表述为产品 READY 或 100% 无 Bug。

## 范围与边界确认

| 检查项 | 结果 |
|---|---|
| Repo | `D:/sub2api-trunk` |
| Branch | `wujie/video-capture-moat-20260702` |
| `74c4b1ff..HEAD` | 仅 `A docs/superpowers/codex-handoff/CODEX_TASK_GROK45_P0_RELIABILITY_AUDIT_20260710.md` → **无业务范围漂移** |
| 工作树 | 仅 `?? .worktrees/`（未进入）+ 本报告 |
| 禁止项 | 未 push/deploy/reset/clean/rebase；未读 `.env`/密钥；未调真实 Provider；未改业务代码 |

变更规模核对：`607facce..74c4b1ff` = 74 files, +13554 / -576，与任务书一致。

## 真相源完整性

| # | 路径 | 状态 |
|---|---|---|
| 1 | `00_START_HERE.md` | 存在；内容仍指向 2026-07-09 Unified Sweep，未把 P0 可靠性内核标为当前执行入口 |
| 2 | `01_PROJECT_BASELINE.md` | **根目录缺失**（仅 `_archive/` / `_review/` 历史副本，不当作当前真相） |
| 3 | `02_CURRENT_REALITY_STATUS.md` | **根目录缺失**（同上） |
| 4 | `PRODUCT_INVARIANTS.md` | **仓库内不存在** |
| 5 | `ARCHITECTURE_GUARDRAILS.md` | **仓库内不存在** |
| 6 | `CODE_QUALITY_GATE.md` | **仓库内不存在** |
| 7 | `docs/goals/03_CURRENT_GOAL.md` | 存在；状态“内部可用 / 待复核”，与审查包一致 |
| 8 | `docs/superpowers/specs/2026-07-10-reliability-core-design.md` | 存在；本轮不变量主真相源 |
| 9 | `docs/superpowers/plans/2026-07-10-reliability-core-implementation.md` | 存在 |
| 10 | `docs/reviews/LATEST_REVIEW_PACKAGE.html` | 存在 |
| 11 | `docs/superpowers/codex-handoff/deliverables/2026-07-10-RELIABILITY-CORE-review.md` | 存在 |
| 12 | 本任务书 | 存在 |

缺失文件记为事实缺口（见 G45-P3-001），不编造替代规则；运行时不变量以 design + 当前源码为准。

## 命令证据台账

| Command | Start/End (local) | Exit | Summary | Status |
|---|---|---:|---|---|
| `git status/branch/rev-parse` + `git diff --check 607facce..HEAD` | 22:34:34 / 22:34:34 | 0 | 分支正确；diff --check 干净；脏项仅 `.worktrees/` | PASS |
| `backend: go test ./... -count=1` | 22:34:35 / 22:36:15 | 0 | 全包 ok；`internal/service` ~53s | PASS |
| `backend: go test -tags=integration ./... -count=1` | 22:36:27 / 22:38:11 | 0 | **exit 0，但本机无 Docker：TestMain 跳过集成套件**（见下方复核） | **SKIPPED_RUNTIME** |
| `backend: go test -tags=integration ./internal/repository -run 'Test(VideoTaskCreation\|…)' -v` | 22:40:28 / 22:40:39 | 0 | 日志：`docker is not available; skipping integration tests` | **SKIPPED_RUNTIME** |
| `backend: golangci-lint run ./...` | 22:36:24 / 22:38:04 | 0 | `0 issues.` | PASS |
| `backend: go vet ./...` | 22:37:03 / ~22:37:43 | 0 | 无输出 | PASS |
| `go test -race ./internal/repository ./internal/service ./internal/server/routes -count=1` | 22:42:36 / 22:42:36 | 2 | `go: -race requires cgo; enable cgo by setting CGO_ENABLED=1` | **未运行（工具链）** |
| `frontend: npx.cmd eslint . --ext .ts,.vue --max-warnings=0` | 22:38:28 / 22:38:51 | 0 | 通过（仅 npm warn Unknown env config devdir） | PASS |
| `frontend: npx.cmd vue-tsc --noEmit` | 22:39:45 / ~22:40:24 | 0 | 通过 | PASS |
| `frontend: npx.cmd vitest run --reporter=basic` | 22:40:59 / 22:41:31 | 0 | 113 files / 641 tests passed；既有 stderr/Browserslist 提示不改结论 | PASS |
| `python tools/secret_scan.py --include-untracked` | 22:42:29 / 22:42:33 | 0 | 使用 `...\Python312\python.exe`；`no high-confidence tracked-plus-untracked findings`（默认 `python` 为 WindowsApps stub exit 9009） | PASS |
| `go generate ./cmd/server` ×2 + SHA256 + `git diff -- wire_gen.go` | 22:41:43 / 22:42:09 | 0 | 两次哈希均为 `53EA182966735752152BD9F4C6D84CFA95AE31E74E5D5F85E7E1FF8D9C3B121A`；无 diff | PASS |
| `govulncheck ./...` | — | — | 本机未安装 | **待复核** |
| `pnpm audit --prod` | 22:42:42 / 22:42:43 | 1 | Windows 缺 `sh`，pnpm 无法完成 audit；lockfile 未改动 | **待复核** |

**集成测试反证（关键）：** `integration_harness_test.go` 在非 CI 且 Docker 不可用时 `os.Exit(0)`，因此 Codex/审查包中的 “Testcontainers exit 0” **不能被本机复现为已执行**。本审计将 DB 级集成结果标为**待复核**，不以本机 exit 0 冒充已跑通。

## 核心不变量逐项判定

| Invariant | Source-to-sink evidence | Test evidence | Verdict |
|---|---|---|---|
| A 创建+reservation 同事务；Money；防超订；mock 不收费 | `video_task_creation_repo.go` BeginTx→FOR UPDATE→reserve→insert task；`billing_reservation_repo.go` available=balance−Σactive；`billing_money.go`；flag-on 非 mock 才走新路径 `video_gateway_service.go` | 创建/reservation 集成测试源码强（并发 SUM、不足余额零行）；本机未执行 | **源码通过 / 集成待复核** |
| B Dispatch CAS；unknown 不自动重发；自定义 transport | `MarkDispatchingCAS`→Create→`MarkDispatchAcceptedCAS` / `markVideoDispatchUnknown`；`ListRunnableTasks` 排除 `dispatch_state=unknown`；adapter 保留 custom RoundTripper | worker 单测：create 一次、unknown 不重创、persist 失败→unknown；dispatch_error 测 ambiguous | **通过** |
| C 终态单事务；幂等；overrun 按设计扣 actual | `video_task_finalization_repo.go` CAS+event+usage+settle+outbox 同 TX；replay 同终态 Idempotent；`actual>reserved` 仍扣 actual（design §8.3） | finalization 集成测试源码覆盖 fault matrix/并发/overrun；本机未执行 | **源码通过 / 集成待复核** |
| D Reaper；非 pending 不被领取；`billing.reservation_expired` 已注册 | reaper：queued+pending→release，否则 review_required；`ListRunnableTasks` 排除 `settlement_status<>pending`；`video_outbox_handlers.go` Registry+Handle→`notifyAudit` | reservation 集成测试含 ListRunnable 排除；handler 单测存在；本机集成未执行 | **源码通过 / 集成待复核** |
| E Outbox 与终态同事务；handler 失败不回滚终态；lease/dead | finalization enqueue 在同一 TX；`DomainOutboxWorker` claim/retry/dead；capture/archive 经 outbox | outbox repo 集成 + worker/outbox handler 单测；本机集成未执行 | **源码通过 / 集成待复核** |
| F Reconciliation dry-run 只读；历史 ledger 不误比；admin 授权；metrics 真接线 | 仅 SELECT；有更新交易则 projected NULL；`admin.Use(adminAuth)` + `GET .../reliability/reconciliation`；`ProvideReliabilityMetrics` 在 `wire_gen.go` | reconciliation 集成测试源码覆盖误比修复与 recent-first；本机未执行 | **源码通过 / 集成待复核**（见 P2 URL/窗口） |
| G API/前端交付事实；轮询；flag-off | `local_asset_available` 决定“本地资产可下载” vs “结果可下载”；`useVideoTaskLifecycle` 终态停、dispose abort；`video_enabled=false` 走 legacy create/terminal | 前端 Vitest 641 通过；flag-off worker/billing 单测 | **通过** |
| H Migration 增量；flag 默认关；Wire 真注入 | `156_reliability_core.sql` 无 DROP/TRUNCATE；`video_enabled` default false；outbox/reaper/reconciler/metrics 在 `wire_gen.go`；哈希与任务书一致 | schema 集成测试源码断言 156；本机未执行；Wire 本机双 generate 一致 | **通过**（schema 集成待复核） |

### Codex 五项最终修复声明核对

| 声明 | 独立证据 | 结论 |
|---|---|---|
| `billing.reservation_expired` 已注册 | `video_outbox_handlers.go` 常量+Registry+Handle→`notifyAudit` | **确认** |
| reconciliation 不再误比历史余额 | SQL EXISTS 更新交易则 NULL + 服务层双指针比较 | **确认**（源码） |
| reaper 标记任务不再被领取 | `ListRunnableTasks` 排除非 pending settlement + 集成测试源码 | **确认**（源码；集成待复核） |
| 远程仅“结果可下载” | Detail/List 文案 + Vitest | **确认** |
| reconciliation/metrics/outbox 真 Wire | `wire_gen.go` 注入 + cleanup Stop | **确认** |

## Findings

### G45-P2-001 Reconciliation 将非空 result_url 视为可交付，忽略过期

- Category: reconciliation / delivery consistency
- File:line: `backend/internal/repository/reliability_reconciliation_repo.go:55-56` vs `backend/internal/handler/video_handler.go`（expiry-aware remote availability）
- Source-to-sink: DryRun `RemoteAssetAvailable = (result_url <> '')` → 成功且 URL 已过期、无本地资产时，API 可报 `delivery_failed`，但 reconciliation 不报 `video_success_without_deliverable_asset`
- Reproduction/proof: 静态对比两处判定；无本机 DB 复现（Docker 不可用）
- Impact: 运维 dry-run 对过期远程-only 成功任务假阴性
- Minimal fix: 与 handler 共用 `ParseResultURLExpiry` / 同一 remote-available helper
- Confidence: High

### G45-P2-002 Bounded reconciliation 按任务 recency 而非 anomaly，可能饿死陈旧异常

- Category: observability
- File:line: `backend/internal/repository/reliability_reconciliation_repo.go:57-77`
- Source-to-sink: `FROM video_tasks … ORDER BY updated_at DESC LIMIT n`，无 anomaly 谓词；健康流量持续更新会把陈旧异常挤出窗口
- Reproduction/proof: 静态阅读查询；集成测试只证明“新行优先”，不证明陈旧异常不被饿死
- Impact: 长期未更新的异常在默认 limit 下不可见
- Minimal fix: 增加 anomaly 过滤/优先级，或独立异常游标
- Confidence: High

### G45-P2-003 Accept 持久化失败时丢弃内存中已知 UpstreamTaskID

- Category: dispatch / operability
- File:line: `backend/internal/service/video_gateway_worker.go:482-492,528-537`
- Source-to-sink: Provider 已返回 ID → `MarkDispatchAcceptedCAS` 失败 → `markVideoDispatchUnknown` 清空 `UpstreamTaskID` 且事件 payload 不含该 ID
- Reproduction/proof: 单测期望 persist 失败后 UpstreamTaskID 为空；符合 design §8.1“标 unknown、不自动重发”，但损失已知 ID 的人工恢复线索
- Impact: 防双提交正确；人工 reconciliation 更慢/更难
- Minimal fix: 在 `dispatch_unknown` 事件 payload 写入 `known_upstream_task_id`（仍不自动重发）
- Confidence: High

### G45-P2-004 非 CI 无 Docker 时 integration TestMain exit 0，掩盖“未执行”

- Category: test evidence authenticity
- File:line: `backend/internal/repository/integration_harness_test.go:53-60`
- Source-to-sink: `dockerIsAvailable==false && CI==""` → log skip → `os.Exit(0)` → `go test -tags=integration` 报告通过
- Reproduction/proof: 本机 22:40:39 日志原文：`docker is not available; skipping integration tests (start Docker to enable)`；package 仍 `ok`
- Impact: 审查包/本地门禁易把“跳过”写成“Testcontainers 通过”
- Minimal fix: 非显式 `ALLOW_SKIP_INTEGRATION=1` 时失败，或至少在汇总中强制非零 skip 计数/退出码策略
- Confidence: High

### G45-P3-001 任务书要求的根级真相源文件缺失 / START_HERE 过期

- Category: documentation / truth-source hygiene
- File:line: 根目录缺失 `01_PROJECT_BASELINE.md`、`02_CURRENT_REALITY_STATUS.md`、`PRODUCT_INVARIANTS.md`、`ARCHITECTURE_GUARDRAILS.md`、`CODE_QUALITY_GATE.md`；`00_START_HERE.md` 仍写 Unified Sweep
- Source-to-sink: 审计读取顺序无法闭环到根级不变量文档
- Reproduction/proof: glob/仓库搜索无当前根文件
- Impact: 新会话易读错入口；不阻断运行时正确性
- Minimal fix: 恢复或显式重定向到 design/goal，并更新 `00_START_HERE.md`
- Confidence: High

### G45-P3-002 审查包代码证据 SHA 与任务书 CODE_EVIDENCE_HEAD 表述不一致（可解释）

- Category: review-package clarity
- File:line: `docs/reviews/LATEST_REVIEW_PACKAGE.html:32` 写 `68edec77`；任务书固定 `74c4b1ff`（含随后文档收口）
- Source-to-sink: `68edec77` 为最后代码修复；`74c4b1ff` 为审查包发布；`8f576471` 为本审计任务书
- Reproduction/proof: `git log --oneline 607facce..HEAD`
- Impact: 读者可能误判审查包过期；实际无业务漂移
- Minimal fix: HTML 同时标注 “code HEAD / docs HEAD”
- Confidence: High

未发现 P0/P1。完整审查路径覆盖：migration 156、creation/reservation/ledger/outbox/finalization/reaper/dispatch/worker/outbox handlers/reconciliation/metrics/admin routes/wire_gen、前端 lifecycle/列表详情、flag-off、以及 §8 命令台账所列门禁。

## 测试质量与盲区

| 测试组 | 删 CAS/handler/本地资产判断会红？ | 断言 DB/余额/账本？ | 覆盖面 | TC+156 | 质量 |
|---|---|---|---|---|---|
| creation integration | Y（静态） | Y | 并发/回滚/幂等；缺 unknown/overrun | Y（源码） | strong |
| finalization integration | Y | Y | fault matrix/并发/overrun/outbox | Y | strong |
| reservation integration | Y | Y | 过期 reap、排除 runnable | Y | strong |
| outbox integration | Y | Y | claim/lease/dead | Y | strong |
| reconciliation integration | N（只读） | Y（漂移发现） | 检测器级 | Y | adequate |
| mock e2e routes | Y（mock 零财务/本地资产） | N（内存） | 本地交付；缺并发/unknown | NA | adequate |
| worker unit | Y | N | unknown/CAS/persist/poll retry | NA | strong |
| dispatch_error unit | Y | N | ambiguous 分类 | NA | strong（窄） |
| VideoReliabilityFlow | N | N | UI 轮询/文案 | NA | weak（可靠性反证） |
| useVideoTaskLifecycle | Partial | N | 轮询/资产偏好 | NA | weak |

**本机执行：** 上述 `//go:build integration` 套件因无 Docker **全部未实际跑库**；质量评价为静态反证。`migrations_schema_integration_test.go` 源码断言 156 字段/表/唯一键，同样未执行。

## 审查包真实性核对

| 项 | 核对 |
|---|---|
| 目标/边界/分支/基线 | 与仓库一致 |
| 代码证据 SHA | HTML=`68edec77`（最后代码）；任务书证据头=`74c4b1ff`（文档）→ 可解释，见 P3-002 |
| 变更规模 | HTML 写 71 文件 / +13441/-395（至代码提交）；完整至 `74c4b1ff` 为 74/+13554/-576 → 非重大失真 |
| 命令表 | 与本机：单元/lint/vet/前端/secret/wire **可复现通过**；**integration 本机未真正执行**，故不得采信本机复现“Testcontainers 全绿”，对 Codex 当时环境标**待复核** |
| 未跑浏览器/真实 Provider/部署 | HTML 已标 pending → 合格 |
| goal / HTML / Markdown deliverable | 状态均为内部可用/待复核；下一步浏览器闭环一致 |
| 过期判定 | HEAD 仅多审计任务书 → 审查包**未因业务变更过期** |

## 性能与可维护性

- Outbox claim / reservation 用户状态索引在 156 中存在；reconciliation 按 `updated_at DESC` 可能缺理想索引（P2 级，见窗口问题）。
- 前端轮询有 dispose abort 与终态停止；Vitest 覆盖。
- 分层：finalization/reservation/outbox 落 repository；handler 不直连 gorm/redis；Wire 生成可复现。
- 残留：定价仍经 `float→Money` 入口（可增加 overrun 概率，属 design 允许范围）；metrics 无独立 admin HTTP 面（进程内 recorder，非缺陷）。

## 未验证项

1. **Testcontainers / migration 156 运行时**（本机无 Docker）— 待复核
2. **`-race`**（需 CGO）— 待复核
3. **govulncheck / pnpm audit** — 待复核
4. 受控 mock-only **浏览器**列表/详情/reconciliation 截图闭环 — 待复核
5. **真实 Provider / 支付 / 生产数据 / 部署** — 明确未做，且不得写成通过
6. A3–A6 后续阶段 — 未开始

## 回滚与修复优先级

1. **P2-004**（可选流程）：避免 integration skip 被当成通过（CI 已 fail-closed；本地文档/脚本需明示）。
2. **P2-001**：reconciliation remote-available 与 handler 对齐过期判定。
3. **P2-003**：unknown 事件保留 known upstream id。
4. **P2-002**：异常优先/独立游标。
5. **P3**：补真相源入口与审查包双 SHA 标注。
6. 回滚业务：按主题 `git revert`（优先 `68edec77`、`198e87bc` 及前序可靠性提交）；已应用 156 须新写向后兼容迁移，禁止 DROP 回滚。

## 最终签字

- Auditor: Grok 4.5（独立终审，不采信 Codex 自述）
- Date: 2026-07-10 Asia/Shanghai
- Verdict: **条件通过**（P0=0, P1=0；核心单元/静态门禁通过；集成 DB 与外部 gate 待复核）
- Allowed write: 仅本文件
- Post-check: 见下方 git 状态（写入后执行）
