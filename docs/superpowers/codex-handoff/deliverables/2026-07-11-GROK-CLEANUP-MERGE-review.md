# 2026-07-11 Grok Cleanup Merge Review

> 工程结论：**条件通过**
> 产品口径：**内部 mock 可演示 / 非生产 READY**
> 生成时间：2026-07-11

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
| （本审查包提交） | docs(status): publish post-delivery cleanup evidence | 真相源与审查包 |

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
- Windows Testcontainers 仍 panic：`rootless Docker is not supported on Windows` → **INTEGRATION_BLOCKED**（不得记为代码全绿）。

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
| `go test -tags=integration ./internal/repository -run 'Test(BillingReservation...\|...VideoTaskFinalization)' -count=1 -v` | 1 | **INTEGRATION_BLOCKED**（Windows rootless panic） |
| Frontend typecheck / vitest 定向 4 files 14 tests / build | 0 | PASS |
| `go vet ./...` | 0 | PASS |
| `go test ./... -count=1` | 0 | PASS |
| `gofmt -d` on `36de34b8..HEAD` Go files | 0 | CLEAN（收口时） |
| Frontend lint:check / typecheck / vitest 145 files 928 tests / build | 0 | PASS |
| `git diff --check` / cached check / forbidden path gate | 0 | PASS |

## 6. 外部门禁 / 未验证边界

- **INTEGRATION_BLOCKED**：Windows Testcontainers rootless Docker panic；未在 Linux/CI 真跑目标 integration。
- **DOCKER_IMAGE_BLOCKED**：本轮未重试镜像代理；无新镜像 `/health` 证据。
- **BROWSER_NOT_RUN**：未启动合成 mock 环境，未截图 admin 视频列表/详情/币种/mock 资产。
- 未验证真实 Seedance、真实支付、生产数据、公网暴露、跨机器 QCanvas。
- `succeeded + result_url` 不等于持久交付；本轮未做真实用户路径端到端截图。

## 7. 敏感交付包证明

- `sub2api-delivery/**`、`.delivery-tools/**`、`.worktrees/**`、`.impeccable/**`、`_review/**` 已写入 `.gitignore`。
- 未读取、未打印、未哈希、未暂存、未删除上述目录内容。
- `git ls-files -- sub2api-delivery .worktrees .delivery-tools` 预期为空。
- `kling-real` linked worktree 仍注册，未被 prune/remove。

## 8. 风险与回滚

- 回滚：对每个主题 commit 使用 `git revert <exact-commit>`；禁止 reset/rebase。
- 可靠性行为回滚优先关 `RELIABILITY_CORE_VIDEO_ENABLED`。
- G1 噪音清理可从 `_review/grok-cleanup-merge-20260711/noise-backup/` 恢复。

## 9. 当前状态与下一步

- 当前状态：本地代码与 Git 收口完成；工程结论 **条件通过**；产品口径 **内部 mock 可演示 / 非生产 READY**。
- 下一步（人工，本轮不执行）：
  1. 在支持 Testcontainers 的 Linux/CI 跑目标 integration。
  2. 镜像代理恢复后做 mock-only `/health` 与 create→poll 冒烟。
  3. 人工审查后决定是否 push / 开 PR / merge main。

## 10. 可复制的后续人工提示词

```text
请审查 D:\sub2api-trunk 分支 wujie/video-capture-moat-20260702 上
36de34b8..HEAD 的本地 cleanup commits（含 Grok cleanup review）。
确认未包含 sub2api-delivery 密钥/镜像，确认 INTEGRATION_BLOCKED 与
DOCKER_IMAGE_BLOCKED 已如实记录，再决定是否 push 或开 PR。
禁止在未授权时调用真实 Provider 或部署生产。
```
