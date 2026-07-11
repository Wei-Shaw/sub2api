# Sub2API Dirty-Tree Cleanup and Local Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use a task-by-task execution workflow with a fresh verification checkpoint after every commit. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 `D:\sub2api-trunk` 当前混合的交付后修复、遗漏源文件、部署资料和本地交付资产，安全收敛为可审查的分批本地提交，并把工作树收口到 Git clean；不 push、不部署、不再次 merge/rebase。

**Architecture:** 当前 HEAD 已在本地 `origin/main` 之上完成一次整合，脏树是后续硬化修复，不是新的上游合并。执行采用“冻结证据 → 精确忽略本地资产 → 修复已确认阻断 → 白名单分批提交 → 全门禁 → 真相源收口”的路径；本地敏感交付包只忽略、不读取、不删除。

**Tech Stack:** Go 1.26.5、Gin、PostgreSQL、Redis、Testcontainers、Vue 3、TypeScript、pnpm 9、Vitest、Docker Compose、Windows PowerShell、Git linked worktree。

## Global Constraints

- 只操作仓库 `D:\sub2api-trunk`；不修改 QCanvas 或其他仓库。
- 只在分支 `wujie/video-capture-moat-20260702` 上工作；基线 HEAD 必须是 `36de34b81c3a0981fd02fc1dc945d7dc60b587be`。
- 本任务中的“合并”只指把当前脏树整理为分批本地提交并收口；不得执行 `git merge origin/main`、rebase、squash、push、PR 或远端合并。
- 不读取、打印、复制、哈希或提交 `sub2api-delivery/**` 的内容；只能按路径和文件名阻止其进入 Git。
- 不读取任何 `.env`、API Key、token、cookie、密钥备份、生产数据；验证只使用明确写在命令中的合成值。
- 不调用真实/付费 Provider，不触发真实支付，不部署，不公网暴露。
- 不删除、移动或清理 `.worktrees/kling-real`；它是已注册且独立干净的 linked worktree。
- 禁止 `git add .`、`git add -A`、`git commit -a`、`git clean`、任何 `git reset`、广域 `git restore`、`git checkout --`、stash、rebase、worktree prune/remove。
- 允许的索引拆分命令只有对本文列出的 3 个文件执行精确 `git restore --staged -- <path>`；不得影响工作区内容。
- 每批只暂存本文列出的精确文件；禁止 glob 暂存、目录级暂存和顺手修复无关文件。
- LF→CRLF 提示不是失败；禁止借机全仓换行、格式化或编码归一化。
- Docker/Testcontainers、镜像代理或 linked-worktree gitdir 阻塞时必须如实记录，不能用 skip/假绿冒充通过。
- 最终产品口径最多是“内部 mock 可演示 / 本地代码与 Git 收口完成 / 非生产 READY”。

---

## 直接交给 Grok 的开头语

```text
你现在负责 Sub2API 的脏工作树清理和本地提交收口。

进入 D:\sub2api-trunk，完整读取：
docs/superpowers/codex-handoff/CODEX_TASK_GROK_CLEANUP_MERGE_20260711.md

严格从 G0 开始按顺序执行，不能跳阶段。当前工作区里有必须保留的源文件，也有包含密钥与约 1.084 GiB 镜像/安装包的本地交付目录；不能把 untracked 一律当垃圾。

本轮“合并”仅表示在当前分支上分批本地 commit 并最终 Git clean。禁止 merge main、rebase、push、部署、读取密钥或调用真实 Provider。

遇到本文停止条件时立即停止，输出证据和状态，不要绕过。
```

## 1. 已确认的当前事实

| 项目 | 当前事实 |
|---|---|
| Repo | `D:\sub2api-trunk` |
| Branch | `wujie/video-capture-moat-20260702` |
| HEAD | `36de34b8`，`merge(origin/main): port video/reliability moat onto upstream` |
| 本地 `origin/main` | `e316ebf5`；按本地 refs，`origin/main...HEAD = 0/1` |
| Git 形态 | linked worktree；gitdir 在工作区外 |
| 默认状态项 | 48：3 staged、27 unstaged、18 顶层 untracked |
| 展开 untracked 后 | 65 状态行，其中 35 个 untracked 文件/路径 |
| committed branch diff | HEAD 相比本地 `origin/main` 为 161 files / 37,860 insertions / 59 deletions |
| 当前代码门禁 | 后端单元全量通过；前端 build、typecheck、视频/lifecycle 定向测试通过 |
| integration | Windows Testcontainers 出现 `rootless Docker is not supported on Windows`；不得写成通过 |
| Docker 新镜像 | 基础镜像代理 HTTP 429，未形成新镜像 `/health` 证据 |
| 产品状态 | 内部 mock 可演示；非生产 READY |

远端 refs 本轮未 fetch，因此不能声称 `origin/main` 是服务器实时最新状态。

## 2. 强制阅读顺序

- [ ] `00_START_HERE.md`
- [ ] `01_PROJECT_BASELINE.md`
- [ ] `02_CURRENT_REALITY_STATUS.md`
- [ ] `docs/goals/03_CURRENT_GOAL.md`
- [ ] `PRODUCT_INVARIANTS.md`
- [ ] `ARCHITECTURE_GUARDRAILS.md`
- [ ] `CODE_QUALITY_GATE.md`
- [ ] `docs/superpowers/specs/2026-07-10-reliability-core-design.md`
- [ ] `docs/superpowers/plans/2026-07-10-reliability-core-implementation.md`
- [ ] `docs/superpowers/codex-handoff/deliverables/2026-07-10-GROK45-RELIABILITY-AUDIT.md`
- [ ] `docs/superpowers/codex-handoff/deliverables/2026-07-11-DELIVERY-REHEARSAL-PROGRESS.md`
- [ ] `docs/superpowers/codex-handoff/CODEX_TASK_GPT56_POST_DELIVERY_HARDENING_20260711.md`
- [ ] `docs/superpowers/plans/2026-07-11-post-delivery-hardening.md`
- [ ] `docs/superpowers/codex-handoff/deliverables/2026-07-11-POST-DELIVERY-HARDENING-review.md`
- [ ] `docs/api/video-gateway-contract.md`
- [ ] `docs/reviews/LATEST_REVIEW_PACKAGE.html`
- [ ] 本任务书。

不得读取 `sub2api-delivery/**` 或真实 `.env` 来补充上下文。

## 3. 文件分类与最终归属

### 3.1 必须保留并进入 Git

#### 仓库卫生

- `.gitignore`：仅新增本文 G1 的精确本地目录规则。

#### 后端基线/fixture 修复

- `backend/cmd/server/wire_gen_test.go`
- `backend/internal/handler/admin/generation_content_handler_test.go`
- `backend/internal/service/gateway_response_capture_test.go`
- `backend/internal/service/video_gateway_billing_test.go`
- `backend/internal/service/testdata/ark_poll_succeeded.json`

#### 后端配置、专用密钥与部署契约

- `backend/internal/config/config.go`
- `backend/internal/config/config_test.go`
- `backend/internal/config/delivery_config_contract_test.go`
- `backend/internal/repository/video_key_encryptor.go`
- `backend/internal/repository/video_key_encryptor_test.go`
- `deploy/.env.example`
- `deploy/config.example.yaml`
- `deploy/docker-compose.yml`
- `deploy/docker-compose.local.yml`
- `deploy/docker-compose.dev.yml`

#### Provider boundary 与 worker 可观测性

- `backend/internal/handler/video_handler.go`
- `backend/internal/handler/video_handler_c1_contract_test.go`
- `backend/internal/service/video_gateway_worker.go`
- `backend/internal/service/video_gateway_worker_test.go`

#### 账务与 integration 可信度

- `backend/internal/repository/billing_reservation_repo.go`
- `backend/internal/repository/billing_reservation_repo_integration_test.go`
- `backend/internal/repository/integration_harness_test.go`
- `backend/internal/repository/integration_harness_policy_test.go`
- `backend/internal/repository/reliability_reconciliation_repo_integration_test.go`
- `backend/internal/repository/video_task_creation_repo_integration_test.go`
- `backend/internal/repository/video_task_finalization_repo_integration_test.go`

#### 前端 HEAD 缺失依赖闭合

- `frontend/src/utils/productMode.ts`
- `frontend/src/composables/useDisplayCurrency.ts`
- `frontend/src/composables/useAdminDisplayCurrencyRate.ts`
- `frontend/src/composables/__tests__/useVideoTaskLifecycle.spec.ts`
- `frontend/src/types/index.ts`

这些文件已被 HEAD 中视频页面引用，不是垃圾，必须原子提交。

#### 部署适配与文档

- `Dockerfile.delivery`：仅作为受控离线适配器，不替代根 `Dockerfile` 或 CI。
- `deploy/README.md`
- `deploy/DOCKER.md`
- `docs/api/video-gateway-contract.md`：必须先按 G6 重构，禁止提交当前删减版。
- `backend/internal/handler/video_gateway_contract_doc_test.go`

#### 真相源与审查证据

- `00_START_HERE.md`
- `01_PROJECT_BASELINE.md`
- `02_CURRENT_REALITY_STATUS.md`
- `PRODUCT_INVARIANTS.md`
- `ARCHITECTURE_GUARDRAILS.md`
- `CODE_QUALITY_GATE.md`
- `docs/goals/03_CURRENT_GOAL.md`
- `docs/reviews/LATEST_REVIEW_PACKAGE.html`
- `docs/superpowers/codex-handoff/CODEX_TASK_GPT56_POST_DELIVERY_HARDENING_20260711.md`
- `docs/superpowers/plans/2026-07-11-post-delivery-hardening.md`
- `docs/superpowers/codex-handoff/deliverables/2026-07-11-DELIVERY-REHEARSAL-PROGRESS.md`
- `docs/superpowers/codex-handoff/deliverables/2026-07-11-POST-DELIVERY-HARDENING-review.md`
- 本任务书。
- Grok 最终审查包：`docs/superpowers/codex-handoff/deliverables/2026-07-11-GROK-CLEANUP-MERGE-review.md`。

`docs/*` 被 `.gitignore` 忽略；上述新增文档必须逐文件 `git add -f -- <exact-path>`，禁止对整个 `docs/` 强制添加。

### 3.2 永久留在本机、禁止进入 Git

- `sub2api-delivery/**`：15 个文件、约 1.084 GiB，文件名显示含 `.env`、QCanvas API Key、密钥备份、镜像 tar 和安装器。
- `.worktrees/**`：其中 `kling-real` 是已注册 worktree。
- `.impeccable/**`
- `.delivery-tools/**`：脚本硬编码本机 `/mnt/d/sub2api-trunk`，包含网络 pull、镜像保存和覆盖复制；日志是历史证据。
- `_review/**`
- 任意真实 `.env`、`*API-KEY*`、`*SECRETS-BACKUP*`、token、cookie、数据库备份。
- `*.tar`、Docker Desktop 安装器、node_modules、coverage、缓存、dist 和临时文件。

处理方式只能是精确 ignore 和暂存区拦截；不得读取、删除或移动这些目录。

### 3.3 已确认噪音，允许可逆清理

- `frontend/pnpm-lock.yaml`：当前 diff 仅为 deprecated 文本元数据，没有版本、integrity 或 overrides 语义变化。
- `frontend/pnpm-workspace.yaml`：内容是 `allowBuilds: ... set this to true or false` 占位文本，且与项目固定 pnpm 9 主线不匹配。

这两个文件必须先按 G0 备份；只有内容仍与上述事实完全一致时，才允许按 G1 的精确命令恢复/移出工作树。若出现任何额外语义变化，立即停止，不得清理。

## 4. 停止条件

出现任一条件立即停止，并将状态标为“已阻塞”或“需修复”：

- repo、branch、HEAD 与第 1 节不一致。
- `git worktree list --porcelain` 不再包含 `.worktrees/kling-real`。
- 状态中出现本文未分类的新业务文件。
- 暂存区出现 `sub2api-delivery/`、`.worktrees/`、`.delivery-tools/`、`.env`、密钥备份、API Key、`*.tar` 或安装器。
- 需要读取真实密钥、真实 Provider、生产数据或部署才能继续。
- Cancel tiny trial provenance 仍可能被错误标为 production。
- integration 用例仍通过全表 DELETE 或批量取消其他用例任务来制造绿色。
- 视频契约文档仍丢失 HEAD 中有效的端点、模式矩阵、响应字段或计费说明。
- `git add`/commit 出现外部 gitdir `index.lock Permission denied` 且同一精确命令在 Git 进程结束后仍失败。
- 测试失败且不能证明为单纯外部 Docker/镜像代理阻塞。

---

## G0：冻结基线证据，禁止先动索引

- [ ] 创建仅用于本机证据的目录：`_review/grok-cleanup-merge-20260711/`。该目录不得进入 Git。
- [ ] 记录以下命令的完整输出：

```powershell
cd D:\sub2api-trunk
git status --porcelain=v2 --branch --untracked-files=all
git branch --show-current
git rev-parse --show-toplevel
git rev-parse --git-dir
git rev-parse --git-common-dir
git worktree list --porcelain
git log -1 --format=fuller --stat
git rev-list --left-right --count origin/main...HEAD
git diff --binary HEAD
git diff --cached --binary
```

- [ ] 保存 tracked diff 与 staged diff 到 `_review/grok-cleanup-merge-20260711/`；补丁中不得包含 `sub2api-delivery/**`。
- [ ] 只对本文 3.1 白名单中的 untracked 源文件记录路径、大小、修改时间和 SHA-256；禁止对 3.2 黑名单内容读取或哈希。
- [ ] 单独复制 `frontend/pnpm-lock.yaml` 与 `frontend/pnpm-workspace.yaml` 到 `_review/grok-cleanup-merge-20260711/noise-backup/`，保留可逆证据。
- [ ] 把当前 staged 的 3 个文件精确退回工作区，便于重新分批；不得使用 reset：

```powershell
git restore --staged -- `
  backend/internal/service/testdata/ark_poll_succeeded.json `
  frontend/src/composables/useAdminDisplayCurrencyRate.ts `
  frontend/src/composables/useDisplayCurrency.ts
```

预期：文件内容仍在，暂存区为空。

## G1：只做仓库卫生与两项可逆噪音清理

- [ ] 在 `.gitignore` 末尾只新增：

```gitignore
# Local linked worktrees, delivery bundles, and agent-local artifacts
/.worktrees/
/sub2api-delivery/
/.impeccable/
/.delivery-tools/
/_review/
```

- [ ] 验证 `git worktree list --porcelain` 仍包含 `D:\sub2api-trunk\.worktrees\kling-real`；ignore 不等于删除。
- [ ] 用 `git diff -- frontend/pnpm-lock.yaml` 复核它仍只有 deprecated 文本元数据。若完全一致，执行：

```powershell
git restore --worktree -- frontend/pnpm-lock.yaml
```

- [ ] 验证 `frontend/pnpm-workspace.yaml` 仍只是三行占位配置；若完全一致，将它移动到已创建的本机备份目录，禁止提交：

```powershell
Move-Item -LiteralPath 'D:\sub2api-trunk\frontend\pnpm-workspace.yaml' `
  -Destination 'D:\sub2api-trunk\_review\grok-cleanup-merge-20260711\noise-backup\frontend-pnpm-workspace.yaml'
```

- [ ] 只暂存 `.gitignore`，运行黑名单门禁和 diff check，然后提交：

```powershell
git add -- .gitignore
git diff --cached --name-status
git diff --cached --check
git commit -m "chore(repo): ignore local delivery and worktree artifacts"
```

## G2：修复并提交后端基线测试/fixture 漂移

**Files:**

- `backend/cmd/server/wire_gen_test.go`
- `backend/internal/handler/admin/generation_content_handler_test.go`
- `backend/internal/service/gateway_response_capture_test.go`
- `backend/internal/service/video_gateway_billing_test.go`
- `backend/internal/service/testdata/ark_poll_succeeded.json`

- [ ] 先运行定向测试，确认 Ark fixture、constructor 参数、类型和 SettingRepository stub 均闭合：

```powershell
cd D:\sub2api-trunk\backend
go test ./cmd/server ./internal/handler/admin ./internal/service -count=1
```

- [ ] 只暂存上述 5 个文件，确认 staged 清单完全匹配，然后提交：

```powershell
cd D:\sub2api-trunk
git add -- backend/cmd/server/wire_gen_test.go backend/internal/handler/admin/generation_content_handler_test.go backend/internal/service/gateway_response_capture_test.go backend/internal/service/video_gateway_billing_test.go backend/internal/service/testdata/ark_poll_succeeded.json
git diff --cached --name-status
git diff --cached --check
git commit -m "test(video): repair fixtures and constructor drift"
```

## G3：原子提交 typed config、专用密钥与 Compose 透传

**Files:**

- `backend/internal/config/config.go`
- `backend/internal/config/config_test.go`
- `backend/internal/config/delivery_config_contract_test.go`
- `backend/internal/repository/video_key_encryptor.go`
- `backend/internal/repository/video_key_encryptor_test.go`
- `deploy/.env.example`
- `deploy/config.example.yaml`
- `deploy/docker-compose.yml`
- `deploy/docker-compose.local.yml`
- `deploy/docker-compose.dev.yml`

- [ ] 确认默认值、env bind、Validate、专用 32-byte hex key、Compose/example 五面一致；不得恢复 TOTP key fallback。
- [ ] 运行后端定向测试：

```powershell
cd D:\sub2api-trunk\backend
go test ./internal/config ./internal/repository -run 'Test(LoadDefaultVideoGatewayAndReliabilityCoreConfig|LoadVideoGatewayAndReliabilityCoreFromEnv|DeliveryConfigContract|NewVideoKeyEncryptor)' -count=1
```

- [ ] 使用空临时 env 和合成 key 只解析 Compose，不启动、不拉镜像、不读取 `deploy/.env`：

```powershell
cd D:\sub2api-trunk
$tmp = New-TemporaryFile
$env:VIDEO_GATEWAY_ENCRYPTION_KEY = ('1' * 64)
try {
  docker compose --env-file $tmp.FullName -f deploy/docker-compose.yml config --quiet
  docker compose --env-file $tmp.FullName -f deploy/docker-compose.local.yml config --quiet
  docker compose --env-file $tmp.FullName -f deploy/docker-compose.dev.yml config --quiet
} finally {
  Remove-Item Env:VIDEO_GATEWAY_ENCRYPTION_KEY -ErrorAction SilentlyContinue
  Remove-Item -LiteralPath $tmp.FullName -Force
}
```

- [ ] 只暂存本节 10 个文件并提交：

```powershell
git add -- backend/internal/config/config.go backend/internal/config/config_test.go backend/internal/config/delivery_config_contract_test.go backend/internal/repository/video_key_encryptor.go backend/internal/repository/video_key_encryptor_test.go deploy/.env.example deploy/config.example.yaml deploy/docker-compose.yml deploy/docker-compose.local.yml deploy/docker-compose.dev.yml
git diff --cached --name-status
git diff --cached --check
git commit -m "fix(config): restore video reliability delivery contracts"
```

## G4：先修 Cancel provenance，再提交 provider boundary 与 worker 告警

**Files:**

- `backend/internal/service/video_gateway_service.go`（当前不脏，但为修正已确认 bug 允许最小改动）
- `backend/internal/handler/video_handler.go`
- `backend/internal/handler/video_handler_c1_contract_test.go`
- `backend/internal/service/video_gateway_worker.go`
- `backend/internal/service/video_gateway_worker_test.go`

- [ ] 先新增失败测试，覆盖 production Seedance、tiny trial、已取消 tiny trial 和 provenance 查询失败；禁止 lookup 失败后默认输出 production boundary。
- [ ] 将 `CancelAPIKeyTrialTask` 的返回契约改为同时返回 `task` 与原始 `events`：

```go
func (s *VideoGatewayService) CancelAPIKeyTrialTask(
    ctx context.Context,
    id, userID int64,
    isAdmin bool,
) (*VideoTask, []*VideoTaskEvent, error)
```

- [ ] service 在取消前通过 `GetAPIKeyTrialTask` 获取 provenance events；已取消任务返回原 task+events；新取消任务返回 `CancelTask` 的 task+原 events。任何查询错误必须直接返回 error，不得吞掉。
- [ ] handler 直接使用 service 返回的 task+events 构造响应，删除取消后的第二次 lookup 和 `events=nil` fallback。
- [ ] 保持三种 boundary：mock=`api-key-video-mock-only`、正式 Seedance=`api-key-video-seedance-production`、tiny trial=`api-key-video-seedance-tiny-trial`。
- [ ] 运行：

```powershell
cd D:\sub2api-trunk\backend
go test ./internal/handler ./internal/service -run 'Test(APIKeyVideoTaskResponse|CancelAPIKey|VideoGatewayWorkerDisabled)' -count=1 -v
```

- [ ] 只暂存本节实际变更文件并提交：

```powershell
cd D:\sub2api-trunk
git add -- backend/internal/service/video_gateway_service.go backend/internal/handler/video_handler.go backend/internal/handler/video_handler_c1_contract_test.go backend/internal/service/video_gateway_worker.go backend/internal/service/video_gateway_worker_test.go
git diff --cached --name-status
git diff --cached --check
git commit -m "fix(video): preserve task provenance and worker visibility"
```

## G5：修复账务 SQL 与 integration 隔离，禁止“清库换绿”

**Files:**

- `backend/internal/repository/billing_reservation_repo.go`
- `backend/internal/repository/billing_reservation_repo_integration_test.go`
- `backend/internal/repository/integration_harness_test.go`
- `backend/internal/repository/integration_harness_policy_test.go`
- `backend/internal/repository/reliability_reconciliation_repo_integration_test.go`
- `backend/internal/repository/video_task_creation_repo_integration_test.go`
- `backend/internal/repository/video_task_finalization_repo_integration_test.go`

- [ ] 保留 `$3::text` PostgreSQL 类型修复，语义仍只能把 active reservation 变为 released 或 review_required。
- [ ] 删除 `resetReliabilityReconciliationData` 的全表 DELETE 方案；每个用例创建唯一 fixture，断言时只筛选该用例创建的 task/reservation/outbox ID，并在 `t.Cleanup` 中只清理这些确定 ID。
- [ ] 删除“把所有其他 queued/submitted/running 任务批量改 cancelled”的方案；只允许更新该测试自己创建的任务 ID。
- [ ] 保持 Docker 缺失默认 fail-closed；只有显式本地诊断可输出 `INTEGRATION_SKIPPED_DOCKER_UNAVAILABLE`，CI 永远不得 skip。
- [ ] 先跑非 integration 单元：

```powershell
cd D:\sub2api-trunk\backend
go test ./internal/repository -run 'TestIntegrationDockerUnavailablePolicy' -count=1
go test ./internal/service ./internal/repository -count=1
```

- [ ] 在 Linux/CI 或 Testcontainers 支持的 Docker 环境真实执行目标 integration；Windows rootless panic 不能算代码通过：

```powershell
go test -tags=integration ./internal/repository -run 'Test(BillingReservationRepositoryReap|ExpiredInFlightReservation|ReliabilityReconciliation|VideoReliabilityBillableFake|VideoTaskFinalization)' -count=1 -v
```

预期：真实执行、PASS、无 skip、无 panic、无真实 Provider 请求。若环境仍不支持，允许完成本地代码提交，但最终状态必须保留 `INTEGRATION_BLOCKED`，不得写“全绿/可合并生产”。

- [ ] 只暂存本节 7 个文件并提交：

```powershell
cd D:\sub2api-trunk
git add -- backend/internal/repository/billing_reservation_repo.go backend/internal/repository/billing_reservation_repo_integration_test.go backend/internal/repository/integration_harness_test.go backend/internal/repository/integration_harness_policy_test.go backend/internal/repository/reliability_reconciliation_repo_integration_test.go backend/internal/repository/video_task_creation_repo_integration_test.go backend/internal/repository/video_task_finalization_repo_integration_test.go
git diff --cached --name-status
git diff --cached --check
git commit -m "fix(reliability): isolate repository gates and reservation reaping"
```

## G6：提交前端缺失依赖闭合

**Files:**

- `frontend/src/utils/productMode.ts`
- `frontend/src/composables/useDisplayCurrency.ts`
- `frontend/src/composables/useAdminDisplayCurrencyRate.ts`
- `frontend/src/composables/__tests__/useVideoTaskLifecycle.spec.ts`
- `frontend/src/types/index.ts`

- [ ] 确认 HEAD 中视频页面对上述模块的 import 均能解析；不得把 `frontend/pnpm-lock.yaml` 或 `frontend/pnpm-workspace.yaml` 混入本批。
- [ ] 从 `frontend` 执行：

```powershell
cd D:\sub2api-trunk\frontend
pnpm.cmd run typecheck
npx.cmd vitest run src/composables/__tests__/useVideoTaskLifecycle.spec.ts src/views/admin/video --reporter=basic
pnpm.cmd run build
```

预期：typecheck/build exit 0；4 个定向测试文件、14 个测试通过或更多，不得减少。

- [ ] 只暂存 5 个文件并提交：

```powershell
cd D:\sub2api-trunk
git add -- frontend/src/utils/productMode.ts frontend/src/composables/useDisplayCurrency.ts frontend/src/composables/useAdminDisplayCurrencyRate.ts frontend/src/composables/__tests__/useVideoTaskLifecycle.spec.ts frontend/src/types/index.ts
git diff --cached --name-status
git diff --cached --check
git commit -m "fix(frontend): restore video console support modules"
```

## G7：重构完整视频契约，再提交部署文档与离线适配器

**Files:**

- `docs/api/video-gateway-contract.md`
- `backend/internal/handler/video_gateway_contract_doc_test.go`
- `deploy/README.md`
- `deploy/DOCKER.md`
- `Dockerfile.delivery`

- [ ] 以 `git show HEAD:docs/api/video-gateway-contract.md` 为旧语义清单，以当前 dirty 文档为 UTF-8 和本轮 boundary 修正来源；禁止任选一版整文件覆盖。
- [ ] 新文档必须同时保留并校验：

  - admin JWT 视频端点；
  - QCanvas API-Key create/get/cancel/poll；
  - content 数量与组合限制；
  - mock / production / tiny trial 模式矩阵；
  - 完整任务响应字段；
  - usage、实际分辨率、实际时长；
  - 计费、reservation、settlement 和 idempotency 说明；
  - `succeeded`、`result_url`、可预览/下载/持久交付是不同证据层；
  - mock-only、非生产 READY 和禁止真实 Provider 的边界；
  - 品牌统一写为 `QCanvas`。

- [ ] 文档测试至少断言关键端点、关键字段、reason 和三种 provider boundary；不能只检查一个乱码片段。
- [ ] 明确根 `Dockerfile` 是主线；`Dockerfile.delivery` 只消费已有 dist 的离线适配器，必须记录 dist 的源 commit/build/hash，不能替代 CI。
- [ ] 运行：

```powershell
cd D:\sub2api-trunk\backend
go test ./internal/handler -run 'Test(APIKeyVideoTaskResponse|VideoGatewayContract)' -count=1 -v
cd D:\sub2api-trunk
git diff --check -- docs/api/video-gateway-contract.md deploy/README.md deploy/DOCKER.md Dockerfile.delivery backend/internal/handler/video_gateway_contract_doc_test.go
```

- [ ] 只暂存 5 个文件并提交：

```powershell
git add -- docs/api/video-gateway-contract.md backend/internal/handler/video_gateway_contract_doc_test.go deploy/README.md deploy/DOCKER.md Dockerfile.delivery
git diff --cached --name-status
git diff --cached --check
git commit -m "docs(video): restore complete UTF-8 delivery contract"
```

## G8：全量门禁、真实用户路径与状态判定

- [ ] Backend：

```powershell
cd D:\sub2api-trunk\backend
go vet ./...
go test ./... -count=1
```

- [ ] 从仓库根目录只检查基线后实际变更的 Go 文件；`gofmt -d` 只能报告：

```powershell
cd D:\sub2api-trunk
git diff --name-only 36de34b8..HEAD -- 'backend/**/*.go' | ForEach-Object { gofmt -d $_ }
```

若有输出，只格式化本文白名单内且本轮实际修改的 Go 文件，再重跑对应测试。

- [ ] Frontend：

```powershell
cd D:\sub2api-trunk\frontend
pnpm.cmd run lint:check
pnpm.cmd run typecheck
npx.cmd vitest run --reporter=basic
pnpm.cmd run build
```

- [ ] Git 与禁止路径门禁：

```powershell
cd D:\sub2api-trunk
git diff --check
git diff --cached --check
$forbidden = git diff --cached --name-only | Where-Object {
  $_ -like 'sub2api-delivery/*' -or
  $_ -like '.delivery-tools/*' -or
  $_ -like '.worktrees/*' -or
  $_ -eq 'deploy/.env' -or
  $_ -match '(?i)(secret|api.?key|backup).*(txt|env|json|ya?ml)$' -or
  $_ -match '\.(tar|exe)$'
}
if ($forbidden) { throw '禁止的交付资产进入暂存区' }
```

- [ ] 如 Docker 镜像代理恢复，只允许使用合成配置、mock-only 做根 Dockerfile、delivery Dockerfile、entrypoint、`/health` 和 create→poll 冒烟；禁止真实 Provider。若仍为 HTTP 429，记录为外部门禁，不无限重试。
- [ ] UI/浏览器：若能用合成 mock 环境启动，截图验证 admin 视频列表、详情、币种显示和 mock 结果资产；若环境未启动或需要真实凭证，写明未截图原因，状态保持待复核。
- [ ] 不能把 `go test`、build 或 `result_url` 存在等同于完整用户可用；必须分别记录任务创建、任务 ID、后台状态流转、结果预览/下载、失败显示是否真实验证。

## G9：发布单一审查包与最终 Git clean

- [ ] 创建 `docs/superpowers/codex-handoff/deliverables/2026-07-11-GROK-CLEANUP-MERGE-review.md`，至少包含：

  - 目标、执行目录、branch、起始 HEAD；
  - G0 基线状态；
  - 每个 commit hash、文件清单和 diff 摘要；
  - Cancel provenance、integration isolation、契约文档语义保全的修复证据；
  - 每条验证命令、退出码、测试数量；
  - Docker/integration/browser 的已验证、待复核或已阻塞边界；
  - 敏感交付包未读取、未暂存、未删除证明；
  - 风险、回滚、当前状态、下一步；
  - 可复制的后续人工 merge/push 提示词，但本轮不执行。

- [ ] 更新根级真相源、`docs/goals/03_CURRENT_GOAL.md` 与 `docs/reviews/LATEST_REVIEW_PACKAGE.html`，只写实际结果，不沿用预写“通过”。
- [ ] 逐文件暂存根真相源；对被 `docs/*` 忽略的新文档逐文件 force-add：

```powershell
git add -- 00_START_HERE.md 01_PROJECT_BASELINE.md 02_CURRENT_REALITY_STATUS.md PRODUCT_INVARIANTS.md ARCHITECTURE_GUARDRAILS.md CODE_QUALITY_GATE.md
git add -f -- docs/goals/03_CURRENT_GOAL.md docs/reviews/LATEST_REVIEW_PACKAGE.html docs/superpowers/codex-handoff/CODEX_TASK_GPT56_POST_DELIVERY_HARDENING_20260711.md docs/superpowers/plans/2026-07-11-post-delivery-hardening.md docs/superpowers/codex-handoff/deliverables/2026-07-11-DELIVERY-REHEARSAL-PROGRESS.md docs/superpowers/codex-handoff/deliverables/2026-07-11-POST-DELIVERY-HARDENING-review.md docs/superpowers/codex-handoff/CODEX_TASK_GROK_CLEANUP_MERGE_20260711.md docs/superpowers/codex-handoff/deliverables/2026-07-11-GROK-CLEANUP-MERGE-review.md
git diff --cached --name-status
git diff --cached --check
git commit -m "docs(status): publish post-delivery cleanup evidence"
```

- [ ] 最终验收：

```powershell
git status --short --untracked-files=all
git diff --name-status
git diff --cached --name-status
git ls-files --others --exclude-standard
git ls-files -- sub2api-delivery .worktrees .delivery-tools
git worktree list --porcelain
git merge-base --is-ancestor origin/main HEAD
git rev-list --left-right --count origin/main...HEAD
git log --oneline --decorate -10
```

预期：

- Git status、工作区 diff、暂存区 diff、未忽略 untracked 均为空。
- `sub2api-delivery`、`.worktrees`、`.delivery-tools` 没有 tracked 文件。
- nested `kling-real` worktree 仍注册且未被改变。
- 本地 `origin/main` 仍是 HEAD 祖先。
- 当前分支领先若干本地 commits；必须报告 ahead 数，不得写成已同步远端。
- 不 push、不 merge main。

## 5. linked-worktree index.lock 处理

若精确 `git add` 或 commit 报 `index.lock Permission denied`：

1. 立即停止当前批次，不扩大 staging。
2. 记录 `git rev-parse --git-dir`、该 gitdir 下 `index.lock` 是否存在、是否有正在运行的 Git 进程。
3. 有 Git 进程时等其结束，只重试同一个精确命令一次。
4. 无 Git 进程但 lock 存在：不得删除 lock，报告用户。
5. lock 不存在但仍 Permission denied：按外部 linked-worktree metadata 权限阻塞处理。
6. 禁止替代 `GIT_INDEX_FILE`、复制 `.git`、重建 worktree、改 ACL/所有者或 broad restage 绕过。

## 6. 回滚策略

- 每个主题是独立本地 commit；回滚使用 `git revert <exact-commit>`，不得 reset/rebase。
- 数据库 migration 不在本轮新增；若未来需回滚可靠性行为，优先关闭 `RELIABILITY_CORE_VIDEO_ENABLED`，保留 mock 与诊断。
- G1 噪音清理已在 `_review/grok-cleanup-merge-20260711/noise-backup/` 保留副本。
- 本地交付包和 nested worktree全程只忽略、不删除，回滚不涉及其内容。

## 7. 最终状态词

最终报告只能选择一个工程结论：

- `通过`：本地代码、Git、必要 integration、Docker mock 用户路径均真实通过。
- `条件通过`：代码/Git 收口通过，但 Docker 镜像、integration 或浏览器存在明确外部门禁。
- `需修复`：发现可本地修复的正确性、安全或契约问题尚未修完。
- `已阻塞`：linked-worktree 权限、Docker/Testcontainers 或授权边界使任务无法继续。

无论结论为何，都必须附加产品口径：`内部 mock 可演示 / 非生产 READY`。
