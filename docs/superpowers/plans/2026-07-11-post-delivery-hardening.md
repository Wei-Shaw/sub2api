# Sub2API Post-Delivery Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:test-driven-development` for behavior changes. Execute tasks in order because later Docker and contract gates depend on earlier configuration and test fixes.

**Goal:** 把 2026-07-11 mock 交付彩排暴露的配置、构建、契约、integration 和部署体验问题固化为可重复、可审计的主线方案，同时保持 Qcanvas mock create/poll 与 Windows Docker 试跑不回退。

**Architecture:** 保持现有 Go 模块化单体与 Vue 前端，不新增服务或外部基础设施。硬化层由 typed-config 契约测试、官方 Docker frozen-lock 构建、Qcanvas 文档/代码契约测试、fail-closed integration harness 和部署 runbook 组成；`Dockerfile.delivery` 继续是离线交付适配，不取代官方根 Dockerfile。

**Tech Stack:** Go 1.26.5、PostgreSQL/Testcontainers、Docker/WSL、Vue 3、TypeScript、pnpm 9、Vitest。

## Global Constraints

- 不调用真实/付费 Provider，不触碰生产数据与真实支付。
- 不读取、打印或提交 `.env`、交付密钥备份、API Key、token、cookie。
- 不 push、deploy、reset、clean、rebase；保留 `.worktrees/` 与 `sub2api-delivery/`。
- mock 证据最多支持“内部可用 / 可演示”，不得写成生产 READY。
- 不修改 Qcanvas 仓库；Sub2API 侧兼容 `id/status/result_url/error_message/provider`。
- 每个产品风险必须对应自动测试或可重复命令；integration 只有实际执行才算通过。

---

### Task 1: Restore minimal truth-source navigation

**Files:**

- Create: `00_START_HERE.md`
- Create: `01_PROJECT_BASELINE.md`
- Create: `02_CURRENT_REALITY_STATUS.md`
- Create: `PRODUCT_INVARIANTS.md`
- Create: `ARCHITECTURE_GUARDRAILS.md`
- Create: `CODE_QUALITY_GATE.md`
- Create: `docs/goals/03_CURRENT_GOAL.md`
- Create: `docs/reviews/LATEST_REVIEW_PACKAGE.html`

**Interfaces:**

- Consumes: reliability design, delivery rehearsal progress, D1-D14 task book.
- Produces: `00_START_HERE.md` as the single navigation entry and explicit mock/non-production status.

- [x] Create short current-goal, invariant, reality, guardrail and quality-gate files.
- [x] Link them from `00_START_HERE.md` and publish a WIP HTML review package marked “待复核”.
- [x] Run `git diff --check` and verify every link target exists.

### Task 2: Lock startup configuration and worker diagnostics

**Files:**

- Modify: `backend/internal/config/config_test.go`
- Create: `backend/internal/config/delivery_config_contract_test.go`
- Create: `backend/internal/repository/video_key_encryptor_test.go`
- Modify: `backend/internal/service/video_gateway_worker.go`
- Modify: `backend/internal/service/video_gateway_worker_test.go`
- Modify: `deploy/docker-compose.yml`
- Modify: `deploy/docker-compose.local.yml`
- Modify: `deploy/docker-compose.dev.yml`
- Modify: `deploy/config.example.yaml` only if a tested field is absent
- Modify: `deploy/.env.example` only for non-secret variable names/default placeholders required by the contract

**Interfaces:**

- Consumes: `Config.VideoGateway`, `Config.ReliabilityCore`, Viper environment mapping.
- Produces: stable defaults/validation/env/compose contract and a one-minute diagnostic log when provider polling is disabled.

- [ ] **RED:** Add `TestLoadDefaultVideoGatewayAndReliabilityCoreConfig` expecting worker=true, poll=5, timeout=15, batch=20, attempts=72, reliability flag=false, TTL=6h, reaper=60s, outbox `1/50/120/8/[5,10,20,40,80,160,300]`.
- [ ] **RED:** Add env override test including comma-separated retry backoff; run `go test ./internal/config -run 'TestLoad(DefaultVideoGatewayAndReliabilityCoreConfig|VideoGatewayAndReliabilityCoreFromEnv)' -count=1` and keep the exact failure if decode is missing.
- [ ] **RED:** Add a repository test asserting empty encryption key fails with an operator-readable dedicated-key message, malformed values fail, and a valid synthetic key round-trips.
- [ ] **RED:** Add a worker test capturing `slog` and requiring `video_gateway_worker_disabled` plus the consequence “queued tasks will not progress” when polling and reaper are disabled.
- [ ] **RED:** Add a static contract test that checks all three mainline compose files expose every video/reliability environment name and that the Windows-recommended compose uses named volumes. Expected initial failure: missing keys in `docker-compose.yml`/`docker-compose.dev.yml` and missing retry-backoff mapping.
- [ ] **GREEN:** Add only missing compose mappings, preserve `reliability_core.video_enabled=false`, and emit deterministic startup/disabled logs without changing worker scheduling semantics.
- [ ] Run `go test ./internal/config ./internal/repository ./internal/service -run 'Test(Load.*VideoGateway|DeliveryConfig|NewVideoKeyEncryptor|VideoGatewayWorker.*Disabled)' -count=1`.

### Task 3: Make official and delivery Docker builds repeatable

**Files:**

- Modify: `frontend/pnpm-lock.yaml`
- Modify: `deploy/README.md`
- Modify: `deploy/DOCKER.md`
- Modify: `CODE_QUALITY_GATE.md`
- Keep: `Dockerfile` and `Dockerfile.delivery` unless verification proves a real runtime parity gap

**Interfaces:**

- Consumes: `frontend/package.json` pnpm overrides and the two Docker build paths.
- Produces: pnpm 9 frozen-lock parity, documented Dockerfile selection, Windows named-volume runbook, admin compliance checklist, and image-entrypoint smoke commands.

- [ ] **RED:** Run `corepack pnpm@9 install --frozen-lockfile --ignore-scripts` in `frontend`; expected failure is manifest/lock override mismatch.
- [ ] **GREEN:** Restore lockfile top-level overrides exactly matching `frontend/package.json`; retain unrelated user lock annotations and preserve untracked `frontend/pnpm-workspace.yaml`.
- [ ] Re-run the frozen-lock command and `pnpm run build`.
- [ ] Document: root Dockerfile builds frontend from source and is the mainline/CI path; delivery Dockerfile requires prebuilt `backend/internal/web/dist/index.html` and is offline handoff only.
- [ ] Document Windows Docker Desktop named volumes, first-admin compliance acceptance, API Key group binding, mock-only status and rollback.
- [ ] Build the root image in WSL and smoke `su-exec`, entrypoint executable, non-root launch path and `/health`; build delivery image when prebuilt dist is present. No Provider request is allowed.

### Task 4: Freeze Qcanvas contract and restore lifecycle unit coverage

**Files:**

- Modify: `docs/api/video-gateway-contract.md`
- Create: `backend/internal/handler/video_gateway_contract_doc_test.go`
- Create: `frontend/src/composables/__tests__/useVideoTaskLifecycle.spec.ts`
- Modify: `frontend/src/composables/useVideoTaskLifecycle.ts` only if RED tests expose a defect

**Interfaces:**

- Consumes: API-key create/get/cancel handlers and existing response mapper.
- Produces: readable UTF-8 contract aligned to implementation plus unit-level polling/abort/backoff regression coverage.

- [ ] **RED:** Add a doc contract test that requires readable UTF-8 status text, stable Qcanvas fields, mock boundary fields and current reasons (`VIDEO_PRODUCTION_NOT_AUTHORIZED`, `VIDEO_PROVIDER_DISABLED`, `VIDEO_TRIAL_BLOCKED`, `VIDEO_TRIAL_LIMIT_EXCEEDED`).
- [ ] **GREEN:** Replace mojibake video contract with concise readable Chinese; keep mock create→poll→`succeeded + result_url`, explain optional lifecycle fields, and document current real-provider gates without claiming readiness.
- [ ] **RED:** Add Vitest fake-timer tests for single in-flight request, exponential retry cap, hidden pause/visible refresh, scope-dispose abort, terminal+archiving slow poll, deliverable/failed/cancelled stop, and local-asset preference.
- [ ] Change composable only for reproducible failures; keep views free of independent polling timers.
- [ ] Run backend handler/routes contract tests and targeted Vitest files.

### Task 5: Make repository integration truthful and isolated

**Files:**

- Modify: `backend/internal/repository/billing_reservation_repo.go`
- Modify: `backend/internal/repository/reliability_reconciliation_repo_integration_test.go`
- Modify: `backend/internal/repository/video_task_creation_repo_integration_test.go`
- Modify: `backend/internal/repository/integration_harness_test.go`
- Create: `backend/internal/repository/integration_harness_policy_test.go`

**Interfaces:**

- Consumes: PostgreSQL repository tests under `//go:build integration`.
- Produces: deterministic reaper SQL, one statement per prepared execution, self-contained synthetic provider fixtures, and fail-closed Docker policy.

- [x] **RED evidence:** WSL targeted suite failed with `$3` inconsistent types, multi-command prepared statements, and `VIDEO_PROVIDER_DISABLED` for the billable fake fixture.
- [ ] **RED:** Add pure policy tests: Docker unavailable defaults to non-zero; CI cannot skip; only explicit local `SUB2API_ALLOW_INTEGRATION_SKIP=1` returns 0 with machine-readable `INTEGRATION_SKIPPED_DOCKER_UNAVAILABLE`.
- [ ] **GREEN:** Cast the reservation status parameter consistently as text in both assignment and CASE.
- [ ] **GREEN:** Split every reconciliation fixture’s multi-statement `ExecContext` into one statement per call.
- [ ] **GREEN:** Give the synthetic billable provider a test-only encrypted placeholder compatible with `billableFakeEncryptor`; never depend on migration demo rows.
- [ ] **GREEN:** Make TestMain log the Docker execution error and fail closed unless the explicit local skip variable is set.
- [ ] Re-run the exact WSL targeted integration suite and record test count/failures.

### Task 6: Full verification and self-contained review

**Files:**

- Modify: `docs/reviews/LATEST_REVIEW_PACKAGE.html`
- Modify: `docs/goals/03_CURRENT_GOAL.md`
- Create: `docs/superpowers/codex-handoff/deliverables/2026-07-11-POST-DELIVERY-HARDENING-review.md`

**Interfaces:**

- Consumes: fresh command outputs from Tasks 2-5.
- Produces: D1-D14 disposition, best-practice answers, exact commands/exit codes, rollback and non-claims.

- [ ] Run backend vet and targeted/full unit tests from the task book.
- [ ] Run actual WSL targeted integration; an explicit skip is evidence only of `SKIPPED`, never PASS.
- [ ] Run frontend build and targeted lifecycle/video Vitest.
- [ ] Validate compose without loading a local env file, run official/delivery Docker builds and safe entrypoint/health smoke when available.
- [ ] Run `git diff --check`; inspect final status and ensure no delivery secret or large artifact is staged.
- [ ] Perform a fresh whole-diff review for correctness, security, compatibility and evidence truthfulness; repair any P0/P1 before finalizing.
- [ ] Publish the Markdown and HTML review packages with one of `通过/条件通过/需修复/已阻塞`; never claim production READY.

## Self-review

- Spec coverage: D1-D7 → Tasks 2-3; D8/D14 → Task 4; D9-D12 → Task 5; D13 → Task 1; final D1-D14 ledger → Task 6.
- Placeholder scan: no deferred implementation placeholders; external gates are expressed as commands with truthful outcomes.
- Type consistency: configuration names match `VideoGatewayConfig`/`ReliabilityCoreConfig`; Qcanvas fields match `apiKeyVideoTaskResponse`; integration policy uses one explicit local skip variable.
