# 2026-07-11 Grok Cleanup Merge Review

> 工程结论：**通过**
> 产品口径：**内部 mock 可演示 / 非生产 READY**
> 生成时间：2026-07-11
> 门禁补证：WSL Ubuntu integration PASS + 缓存 `sub2api:local` mock 冒烟 PASS（未 push）

## 1. 目标与范围

- 目标：把 `D:\sub2api-trunk` 脏工作树整理为可审查的分批本地 commits，并收口到 Git clean。
- 执行目录：`D:\sub2api-trunk`
- 分支：`wujie/video-capture-moat-20260702`
- 起始 HEAD：`36de34b81c3a0981fd02fc1dc945d7dc60b587be`
- 本轮“合并”仅指本地分批提交收口；未 merge main、未 rebase、未 squash、未 push、未创建 PR、未部署。

## 2. G0 基线

| 项 | 值 |
|---|---|
| Branch | `wujie/video-capture-moat-20260702` |
| HEAD | `36de34b81c3a0981fd02fc1dc945d7dc60b587be` |
| `origin/main...HEAD` | `0 1`（起始）→ 收口后 `0 8` |
| Worktree | linked worktree；`.worktrees/kling-real` 仍注册 |
| 证据目录 | `_review/grok-cleanup-merge-20260711/`（已 ignore，未入 Git） |
| 噪音备份 | `noise-backup/frontend-pnpm-lock.yaml`、`frontend-pnpm-workspace.yaml` |
| 暂存区 | G0 后退回 3 个 staged 文件；暂存区为空后开始分批 |

## 3. 本地 commits

| Hash | Message | 文件摘要 |
|---|---|---|
| `69b67069` | chore(repo): ignore local delivery and worktree artifacts | `.gitignore` +7 |
| `08d23645` | test(video): repair fixtures and constructor drift | 5 files +55/-3 |
| `6dff26e7` | fix(config): restore video reliability delivery contracts | 10 files +439/-1 |
| `839d83bc` | fix(video): preserve task provenance and worker visibility | 5 files +231/-10 |
| `d2bbdd2e` | fix(reliability): isolate repository gates and reservation reaping | 7 files +194/-52 |
| `ef48e520` | fix(frontend): restore video console support modules | 5 files +269 |
| `714cbfd3` | docs(video): restore complete UTF-8 delivery contract | 5 files +358/-180 |
| `d1774066` | docs(status): publish post-delivery cleanup evidence | 真相源与审查包 |
| `e8b21ca8` | test(video): lock cancel HTTP provider_boundary provenance | HTTP cancel boundary |
| `71654009` | test(reliability): scope billable-fake adapter counts to owned tasks | owned-task isolation |
| （本补证提交） | docs(status): record WSL integration and mock smoke pass | 工程结论升级为通过 |

## 4. 三个阻断修复证据

### 4.1 Cancel provenance

- `CancelAPIKeyTrialTask` 现返回 `(*VideoTask, []*VideoTaskEvent, error)`，取消前读取 provenance；查询失败直接返回 error。
- Handler 删除取消后二次 lookup 与 `events=nil` fallback。
- 测试：`TestCancelAPIKeyTrialTaskPreservesProductionProvenance`、`...TinyTrial...`、`...AlreadyCancelled...`、`...ProvenanceLookupFailureDoesNotDefaultProduction`、`TestAPIKeyVideoTaskResponseDistinguishesSeedanceBoundaries` 全部 PASS。

### 4.2 Integration isolation

- 删除 `resetReliabilityReconciliationData` 全表 DELETE。
- 改为 `trackReliabilityOwnedIDs` + 按 task ID cleanup；断言只筛选本用例 ID。
- 删除 billable fake 中“批量取消其他 queued/submitted/running 任务”与全表 `DELETE FROM domain_outbox`。
- Docker 缺失默认 fail-closed；仅显式 `SUB2API_ALLOW_INTEGRATION_SKIP=1` 可本地 skip；CI 不可 skip。
- Windows 宿主仍不可跑 Testcontainers（rootless panic）；**WSL Ubuntu-24.04 + Linux Docker 目标用例已真实 PASS**（见 §5）。
- billable-fake 断言改为按 owned task ID 计数 / outbox fail-once；共享 `ProcessOnce` 不再因 leftover 任务假红。

### 4.3 视频契约语义保全

- 以 dirty UTF-8 为编码基线，恢复 HEAD 丢失的 admin JWT 端点、完整响应字段、usage/actual_*、计费/reservation/settlement/idempotency、证据层分离与三种 provider boundary。
- 品牌统一为 `QCanvas`。
- `TestVideoGatewayContractDocumentsCurrentQCanvasBoundary` PASS。

## 5. 验证命令与结果

| 命令 | 退出码 | 结果 |
|---|---|---|
| `go test ./cmd/server ./internal/handler/admin ./internal/service -count=1` | 0 | PASS |
| `go test ./internal/config ./internal/repository -run 'Test(LoadDefault...\|...NewVideoKeyEncryptor)' -count=1` | 0 | PASS |
| Compose `config --quiet` ×3（合成 env） | 0 | PASS（REDIS_PASSWORD 空值 warning） |
| `go test ./internal/handler ./internal/service -run 'Test(APIKeyVideoTaskResponse\|CancelAPIKey\|VideoGatewayWorkerDisabled)' -count=1 -v` | 0 | PASS |
| `go test ./internal/repository -run TestIntegrationDockerUnavailablePolicy -count=1` | 0 | PASS |
| `go test ./internal/service ./internal/repository -count=1` | 0 | PASS |
| `go test -tags=integration ./internal/repository -run 'Test(BillingReservation...\|...VideoTaskFinalization)' -count=1 -v`（Windows 宿主） | 1 | Windows rootless panic（宿主不可用；不以宿主为门禁） |
| **同上命令在 WSL Ubuntu-24.04**（`GOTOOLCHAIN=auto`，无 skip） | **0** | **PASS**（目标 BillingReservation / ExpiredInFlight / ReliabilityReconciliation / VideoReliabilityBillableFake / VideoTaskFinalization） |
| 合成 env + 缓存 `sub2api:local`：`compose config` → `/health` → mock create→poll | **0** | **PASS**：`succeeded` + `result_url=/api/v1/video/mock-assets/1.svg`；`provider_boundary=api-key-video-mock-only` |
| Frontend typecheck / vitest 定向 4 files 14 tests / build | 0 | PASS |
| `go vet ./...` | 0 | PASS |
| `go test ./... -count=1` | 0 | PASS |
| `gofmt -d` on `36de34b8..HEAD` Go files | 0 | CLEAN（收口时） |
| Frontend lint:check / typecheck / vitest 145 files 928 tests / build | 0 | PASS |
| `git diff --check` / cached check / forbidden path gate | 0 | PASS |

## 6. 外部门禁 / 未验证边界

- Windows 宿主 Testcontainers 仍不可用；**以 WSL Linux Docker 目标 integration 为工程门禁真相**。
- 冒烟使用**已缓存** `sub2api:local`（可能早于 tip）；**未** `docker build` 新镜像、未 push。
- **BROWSER_NOT_RUN**：管理台截图未补；不阻止工程「通过」（任务书核心是 integration + Docker mock 用户路径）。
- 未验证真实 Seedance、真实支付、生产数据、公网暴露、跨机器 QCanvas。
- `succeeded + result_url` 不等于持久交付或生产 READY。

## 7. 敏感交付包证明

- `sub2api-delivery/**`、`.delivery-tools/**`、`.worktrees/**`、`.impeccable/**`、`_review/**` 已写入 `.gitignore`。
- 未读取、未打印、未哈希、未暂存、未删除上述目录内容；冒烟仅用 `_review/` 合成占位密钥。
- `git ls-files -- sub2api-delivery .worktrees .delivery-tools` 预期为空。
- `kling-real` linked worktree 仍注册，未被 prune/remove。

## 8. 风险与回滚

- 回滚：对每个主题 commit 使用 `git revert <exact-commit>`；禁止 reset/rebase。
- 可靠性行为回滚优先关 `RELIABILITY_CORE_VIDEO_ENABLED`。
- G1 噪音清理可从 `_review/grok-cleanup-merge-20260711/noise-backup/` 恢复。

## 9. 当前状态与下一步

- 当前状态：本地代码与 Git 收口完成；工程结论 **通过**；产品口径仍 **内部 mock 可演示 / 非生产 READY**。
- 证据要点：WSL integration exit 0；缓存镜像 mock `/health` + create→poll `succeeded + result_url`；未 push。
- 下一步（人工，本轮不执行）：
  1. 人工审查后决定是否 push / 开 PR / merge main。
  2. 需要与 tip 对齐的镜像时，再在代理可用时重建 `sub2api:local`。
  3. 可选：管理台截图补证。

## 10. 可复制的后续人工提示词

```text
请审查 D:\sub2api-trunk 分支 wujie/video-capture-moat-20260702 上
36de34b8..HEAD 的本地 cleanup + pass-gates 补证。
确认未包含 sub2api-delivery 密钥/镜像，确认 WSL integration 与
缓存镜像 mock 冒烟已如实记为通过，再决定是否 push 或开 PR。
禁止在未授权时调用真实 Provider 或部署生产。
```
