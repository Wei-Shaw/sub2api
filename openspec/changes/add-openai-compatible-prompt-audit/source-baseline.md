# AICodex Prompt Audit 源基线

## 1. 基线状态

本文件记录用于本 change 功能对照的源仓库状态。参考工作区仍可继续变化，但本 change 已通过第 6 节登记的只读 patch bundle 固定实施基线；后续实现只以该冻结包和本 change specs 为依据。

| 字段 | 值 |
| --- | --- |
| 源仓库 | `/Users/mt/code/mt-ai/aicodex/aicodex-api` |
| 采集时间 | `2026-07-16 20:21:19 CST (+0800)` |
| 分支 | `yjb` |
| HEAD | `7a50378851a80650cb0c086260b23abeb3469e6b` |
| 工作区 | dirty |
| 已跟踪差异 | 38 files changed, 1306 insertions(+), 227 deletions(-) |
| 未跟踪范围 | Prompt Guard 实现/测试 6 个文件，加 1 个 OpenSpec change 目录 |
| 冻结状态 | **已用只读 patch bundle 冻结并在 detached worktree 恢复验证** |

当前 HEAD 只代表已提交历史，不能单独代表要迁移的完整功能。同步 fail-closed Guard、出站安全校验、WebSocket/路由顺序测试以及相应 OpenSpec 当前存在于未提交或未跟踪状态。因此，本 change 的临时功能参考是“上述 HEAD + 采集时磁盘工作区”，最终行为权威仍是本 change 的 specs。

## 2. 与迁移直接相关的已跟踪修改

### 后端入口与启动接线

- `ai-gateway/cmd/aicodex/main.go`
- `ai-gateway/internal/controller/prompt_audit.go`
- `ai-gateway/internal/router/relay-router.go`
- `ai-gateway/internal/router/video-router.go`
- `ai-gateway/internal/relay/ws_responses.go`
- `ai-gateway/internal/gatewayadapter/transport/anthropic.go`
- `ai-gateway/internal/gatewayadapter/transport/gemini.go`
- `ai-gateway/internal/gatewayadapter/transport/jimeng.go`
- `ai-gateway/internal/gatewayadapter/transport/kling.go`
- `ai-gateway/internal/gatewayadapter/transport/midjourney.go`
- `ai-gateway/internal/gatewayadapter/transport/openai.go`
- `ai-gateway/internal/gatewayadapter/transport/suno.go`
- `ai-gateway/internal/gatewayadapter/transport/task.go`

### Prompt Audit 核心

- `ai-gateway/internal/service/promptaudit/client.go`
- `ai-gateway/internal/service/promptaudit/config.go`
- `ai-gateway/internal/service/promptaudit/enqueue.go`
- `ai-gateway/internal/service/promptaudit/openai_client.go`
- `ai-gateway/internal/service/promptaudit/probe.go`
- `ai-gateway/internal/service/promptaudit/qwen3guard.go`
- `ai-gateway/internal/service/promptaudit/runtime.go`
- `ai-gateway/internal/service/promptaudit/runtime_coverage_test.go`
- `ai-gateway/internal/service/promptaudit/types.go`
- `ai-gateway/internal/service/promptaudit/worker.go`
- 同目录的 config、diagnostics、probe 测试

### 协议、错误和回归测试

- `ai-gateway/internal/types/error.go`
- `ai-gateway/internal/gatewayadapter/transport/user_concurrency_order_test.go`

### 控制台和类型

- `webui/src/api/promptAudit.test.ts`
- `webui/src/features/prompt-audit/PromptAuditPage.tsx`
- `webui/src/features/prompt-audit/PromptAuditPage.test.tsx`
- `webui/src/features/prompt-audit/promptAuditViewModel.ts`
- `webui/src/features/prompt-audit/promptAuditViewModel.test.ts`
- `webui/src/types/promptAudit.ts`

### 运行说明

- `deploy/.env.example`
- `docs/constraints/41-ai-readable-logging.md`
- `docs/workflows/02-local-dev.md`

## 3. 必须纳入冻结基线的未跟踪文件

以下文件不在 HEAD 中，但属于“完整功能必须要有”的关键证据：

- `ai-gateway/internal/gatewaycore/prompt_guard.go`
- `ai-gateway/internal/service/promptaudit/outbound_security.go`
- `ai-gateway/internal/service/promptaudit/synchronous_guard.go`
- `ai-gateway/internal/service/promptaudit/synchronous_guard_test.go`
- `ai-gateway/internal/relay/ws_responses_prompt_guard_order_test.go`
- `ai-gateway/internal/router/prompt_guard_order_test.go`
- `openspec/changes/add-prompt-audit-synchronous-blocking/`

不得只执行 `git diff HEAD` 后就声称已冻结，因为普通 diff 不包含这些未跟踪文件。

## 4. 功能对照优先级

遇到源实现、源测试和本 change 描述不一致时，按以下顺序决策：

1. 本 change 的三个 delta specs：目标行为契约。
2. 本 change 的 `design.md` 和 `implementation-guide.md`：目标架构与落地约束。
3. 冻结后的源测试及源 OpenSpec：功能完整性参考。
4. 冻结后的源实现：算法、边界和交互参考。
5. 当前已提交 HEAD：历史参考。

目标项目不得复制源仓库的 Ent、Caddy/gatewaycore、React 或全局 option 依赖；只迁移可以被规格和测试证明的行为。

## 5. 实施前冻结步骤

在源仓库所有者确认工作区内容属于迁移基线后，选择一种方式：

### 方案 A：专用 commit/tag（推荐）

1. 在源仓库专用分支提交与 Prompt Audit/Guard 有关的已跟踪和未跟踪文件。
2. 运行源模块及路由/WS 顺序测试。
3. 创建不可移动 tag，或记录完整 commit SHA。
4. 把最终标识和测试结果回写本文件。

### 方案 B：只读 patch 包

1. 生成 tracked diff。
2. 使用能够包含未跟踪文件的归档或补丁流程补齐第 3 节文件。
3. 生成文件清单和 SHA-256；在干净临时目录中恢复并运行测试。
4. 把 patch 路径、清单路径和校验和回写本文件。

禁止把包含真实 API Key、Redis payload、`.env` 私密值或运行日志中的完整 Prompt 放入基线包。

## 6. 最终冻结登记

| 字段 | 待填写值 |
| --- | --- |
| 冻结方式 | 只读 tracked patch + untracked tar archive |
| 冻结 commit/tag | base commit `7a50378851a80650cb0c086260b23abeb3469e6b`（detached restore） |
| patch/archive 绝对路径 | `/Users/mt/code/mt-ai/sub2api/sub2api-mt/openspec/changes/add-openai-compatible-prompt-audit/source-freeze/` |
| manifest SHA-256 | `badab312bf6af4d2c77857a9400381f4da4fbf45722d9f4a6df23bc7005273b6` |
| tracked patch SHA-256 | `f751a13cce3f3a73cd60cae3aececcef6e1e76dcec8c551a7a4747f032234d2b` |
| untracked archive SHA-256 | `1536e2781703b7620e26f2d08b249431fa5846ad9e32b2e8b0d547c3fa3b3632` |
| 冻结人/复核人 | Codex；由恢复后的文件清单、`git diff --check` 和测试命令复核 |
| 冻结时间 | `2026-07-16 20:21:19 CST (+0800)` |
| 源测试结果 | 恢复副本中 Prompt Audit 核心、router、relay、gateway transport 目标测试全部通过，详见 `source-freeze/MANIFEST.md` |

## 7. 复核命令

```bash
cd /Users/mt/code/mt-ai/aicodex/aicodex-api
git branch --show-current
git rev-parse HEAD
git status --short
git diff --stat
git diff --name-only
git ls-files --others --exclude-standard
cd ai-gateway
go test ./internal/service/promptaudit
```

本提案编写时上述模块测试已在当前 dirty 磁盘状态通过；冻结后必须再次执行，并记录最终 commit/patch 校验和、执行目录和完整输出。
