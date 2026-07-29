# TokenSupply Fork Standard Operating Procedure

本文档是 TokenSupply 维护 Sub2API fork 时，开发、测试、审计、上游同步、发布和生产变更的唯一流程规范。其他文档可以解释具体系统，但不得定义与本文冲突的发布流程。

## 1. 目标与不可变原则

目标是让任意一个版本都可以回答以下问题：

- 为什么改，谁审计，哪些风险被接受？
- 哪个 PR、commit、tag、镜像 digest 和生产部署属于同一次变更？
- 跑过哪些测试，哪些测试因环境限制未运行？
- 数据库是否向后兼容，失败时如何回滚？
- Release notes 来自哪里，是否与发布代码完全一致？

所有流程必须遵守：

1. `main` 始终可部署，禁止直接开发和直接 push。
2. 一个 PR 只承担一种主要行为：功能、修复、上游同步、流程维护或发布准备。
3. 功能 PR 不修改 fork 版本号，不创建 tag，不部署生产。
4. 上游同步不夹带业务功能；Release PR 不夹带业务代码。
5. 生产只运行由成功 Release workflow 生成的不可变版本镜像。
6. tag、镜像 digest、release completion ledger 一经发布不可覆盖。
7. 未验证的事项必须明确记录为限制，不能用“应该没问题”代替证据。
8. 任何密钥、生产日志、用户数据和内部地址都不得进入公开仓库。

## 2. 权威记录

| 对象 | 权威来源 | 说明 |
|---|---|---|
| 需求与范围 | PR 描述 | 必须说明包含与不包含的内容 |
| 代码 | PR merge commit 或 squash commit | 禁止以本地未提交目录作为交付物 |
| 测试 | GitHub Actions 对应 commit SHA | 本地结果是补充，不替代 CI |
| 审计 | PR review/审计结论 | 必须关联具体 commit SHA |
| 版本 | `backend/cmd/server/VERSION` | Release PR 修改 |
| 发布说明 | `docs/releases/v<version>.md` | tag message 不是发布说明来源 |
| 发布源码 | annotated tag 指向的 commit | 必须可从 `origin/main` 到达 |
| 发布制品 | completion ledger 和 OCI digest | 版本标签只是便捷引用 |
| 生产状态 | deployer state 与当前容器 digest | UI 显示版本不能单独作为证据 |

## 3. 工作类型与分支

开始前先分类，不能在开发途中把分支悄悄变成另一类工作。

| 类型 | 分支名 | 合并方式 | 允许内容 |
|---|---|---|---|
| 功能 | `codex/feat-<name>` | Squash merge | 一个业务能力及其测试/文档 |
| 修复 | `codex/fix-<name>` | Squash merge | 一个缺陷及回归测试 |
| 安全修复 | `codex/security-<name>` | Squash merge | 漏洞修复、测试、必要迁移 |
| 运维改进 | `codex/ops-<name>` | Squash merge | 监控、告警、部署工具 |
| 上游同步 | `codex/sync-upstream-<version>` | Merge commit | upstream merge、冲突解法、同步修复 |
| 流程维护 | `codex/workflow-<name>` | Squash merge | SOP、CI、Release 自动化 |
| 发布准备 | `codex/release-<version>` | Squash merge | VERSION、版本化 release notes |
| 紧急修复 | `codex/hotfix-<name>` | Squash merge | 最小生产修复及回归测试 |

禁止使用长期 `develop` 分支。功能分支短期存在，合并后删除。

## 4. 工作区准备

每次开始工作：

```bash
git status --short --branch
git fetch origin --prune
git fetch upstream --prune
git switch main
git pull --ff-only origin main
git switch -c codex/<type>-<name>
```

如果工作区有未提交内容：

- 与任务无关：保留，不暂存，不清理。
- 与任务相关且来源明确：纳入当前工作并先理解差异。
- 来源不明或与目标冲突：停止写入，先建立备份分支或 worktree 清单。

禁止 `git add .`。必须选择性暂存并检查：

```bash
git add <explicit-paths>
git diff --cached --check
git diff --cached --stat
```

名称带 ` 2` 的 Finder 副本、报告、附件和临时产物不得提交。

## 5. 开发 SOP

1. 在 PR 描述草稿中写清目标、非目标、风险和验收条件。
2. 先定位现有实现、测试、配置和迁移约束，再修改代码。
3. 每个行为变化都要有成功路径、失败路径和边界条件测试。
4. 数据库变化只能追加新迁移，禁止修改已经进入 release 基线的迁移。
5. API、配置或 schema 变化必须保持蓝绿期间新旧版本可共存。
6. 生成代码必须由项目工具生成，并确认没有无关漂移。
7. 提交使用 Conventional Commits：`feat`、`fix`、`test`、`docs`、`refactor`、`chore`。
8. 审计修复可以保留为独立 commit，但最终普通 PR 使用 squash merge。

提交信息描述结果，不记录聊天过程。例如：

```text
feat(email): add durable delivery policy
fix(ops): exclude client policy failures from SLA
test(deploy): cover rollback after failed handoff
```

禁止：

- `fix stuff`、`update`、`try again` 等无语义提交。
- 在功能 PR 中出现 `prepare release`、`bump version` 或撤销版本号。
- 反复 merge `main` 到普通功能分支。单维护者分支应在合并前 rebase。

## 6. 测试 SOP

测试分三级。

### L1：开发循环

每次小改动运行受影响包或组件：

```bash
cd backend
go test -tags=unit ./internal/<package> -run '<relevant-tests>' -count=1
```

```bash
cd frontend
pnpm vitest run <relevant-test-file>
```

### L2：PR 准入

PR 必须由 GitHub Actions 完成：

- 后端 unit tests。
- 后端 integration tests（真实 PostgreSQL/Redis Testcontainers）。
- frontend lint、typecheck 和关键 Vitest。
- `golangci-lint`。
- 部署脚本和 workflow contract 测试。

CI 失败后：

1. 定位首个真实失败，不用 rerun 掩盖稳定失败。
2. 若判断 flaky，必须单独重复运行并记录证据。
3. flaky 连续出现两次，应修测试或隔离根因，不能无限 rerun。
4. 修复后推新 commit，让所有 required checks 对新 SHA 重跑。

### L3：高风险/发布验证

满足任一条件时增加完整验证：

| 变化 | 额外验证 |
|---|---|
| 数据库迁移 | migration history、upgrade、rollback compatibility |
| deployer/Nginx/Compose | installer、handoff、drain、rollback、crash recovery |
| 邮件/支付 | 幂等、事务边界、重试、敏感数据脱敏 |
| 安全/IP/认证 | 可信代理、伪造头、IPv4/IPv6、fail-open/close |
| 前端关键流程 | 完整 Vitest、生产 build、必要时浏览器验收 |
| Release workflow | release safety fixtures、无制品 dry-run |
| 上游同步 | 完整 CI，重点回归冲突模块 |

本地环境不能运行的测试必须写明原因，并由 GitHub Actions 补齐。

## 7. 审计 SOP

以下变化必须独立审计：认证/授权、支付、迁移、生产部署、自动更新、密钥、邮件隐私、限流、告警和跨模块重构。

审计顺序：

1. 冻结待审 commit SHA。
2. 提供需求、PR diff、数据流、迁移和测试结果。
3. 审计者独立检查，不以实现者总结替代读代码。
4. 发现按 P0/P1/P2/P3 分类，并给出文件/行和复现条件。
5. P0/P1 必须修复；P2 必须修复或在 PR 中由维护者明确接受；P3 可排入后续。
6. 修复产生新 SHA 后，对修复和受影响区域复审。
7. 最终结论记录：审计 SHA、发现、处置、残余风险、审计者。

“没有发现问题”不代表证明安全，必须同时列出未覆盖的测试和假设。

## 8. PR SOP

PR 描述必须填写模板中的范围、风险、迁移、配置、测试、审计、发布和回滚项。

合并条件：

- diff 中无秘密、临时文件和无关格式化。
- PR 范围与标题一致。
- required checks 全绿且对应最新 SHA。
- 所需审计完成，没有未处理 P0/P1。
- 数据库和配置兼容性已说明。
- rollback 明确且实际可执行。
- PR branch 与目标 `main` 无未解决漂移。

普通 PR 使用 squash merge，PR 标题作为 `main` 的提交标题。上游同步必须保留 merge ancestry，使用 merge commit。

## 9. 上游同步 SOP

上游同步始终单独进行：

```bash
git switch main
git pull --ff-only origin main
git fetch upstream --prune
git switch -c codex/sync-upstream-<version>
git merge --no-ff upstream/main
```

处理规则：

1. 记录同步前 fork SHA、上游 SHA、双方 commit 数和冲突文件。
2. 冲突逐项描述“保留 fork 行为”和“采用 upstream 行为”。
3. 不通过 `ours`/`theirs` 批量覆盖安全敏感文件。
4. 保留 upstream merge parent，禁止 squash 上游历史。
5. 运行冲突模块测试和完整 CI。
6. PR notes 分开列出 upstream highlights 与 fork conflict resolutions。
7. 合并后再从新 `main` 开始功能或 Release PR。

如果 upstream 在同步期间继续前进，不自动追逐移动目标。当前 PR 固定到已记录 SHA；下一次变化另开同步 PR，除非存在阻断发布的安全修复。

## 10. Release Candidate SOP

只有计划发布时才建立 Release PR。

版本规则：

- 基于上游 `X.Y.Z` 的第一次 fork 发布：`X.Y.Z-ts.1`。
- 同一上游基线的后续 fork 修订依次递增。
- 追到新上游版本后 revision 从 `ts.1` 重新开始。
- 不复用已创建的 tag，不用 `latest` 作为生产部署版本。

Release PR 正常只修改：

```text
backend/cmd/server/VERSION
docs/releases/v<version>.md
```

发布说明必须从 `docs/releases/TEMPLATE.md` 创建，包含用户可理解的变化、上游基线、配置/迁移、部署/回滚、验证和已知限制。不得粘贴原始 commit 列表代替 release notes。

Release PR 合并前：

1. 所有目标功能和上游同步已经进入 `main`。
2. `main` CI 绿色。
3. release notes 已人工审阅。
4. `go mod tidy`、迁移历史和 release safety tests 无漂移。
5. 不存在尚未决定是否纳入本版本的代码。
6. Release PR 的精确 head SHA 必须通过 `Release Preflight` workflow；工作流必须显式检出该 SHA，其中 `make test-frontend-release` 包含 frontend lint、完整 Vitest 和 production build，同时验证迁移/tidy、发布契约和 deployer bundle 构建。

Release PR 合并后、创建 tag 前：

1. 等待合并后的精确 `origin/main` SHA 再次通过 `Backend CI` 和 `Release Preflight` 的 push run。
2. 确认该 SHA 的完整前端发布预检已通过，不能沿用 PR 合并前的 head SHA 或合成 merge SHA 结果。
3. 完成该 SHA 的最终审计并确认版本 tag、GitHub Release 和版本镜像均未被占用。
4. 任一门禁失败都回到修复 PR；在新的 `main` SHA 全绿前禁止创建正式 tag。

## 11. Tag 与 Release SOP

Release PR 合并且上述 tag 前门禁全部通过后，禁止从本地人工创建或推送发布 tag。仓库使用两层 tag ruleset：不可变层禁止 `v*-ts.*` 更新和删除且无任何 bypass；创建层只禁止 creation，唯一 bypass actor 类型是 Deploy Key。仓库只允许保留一把可写 Deploy Key，名称为 `Sub2API release tag promoter`；私钥只保存在 `RELEASE_TAG_DEPLOY_KEY` Actions secret，普通 `GITHUB_TOKEN` 不具备创建发布 tag 的权限。GitHub ruleset API 会隐藏 Deploy Key actor ID，因此最终审计必须同时核对两层 ruleset 和 repository deploy key 列表。

从 Actions 页面运行 `Promote Release`，输入已审计的版本号和完整 `origin/main` SHA。该工作流必须再次确认远端 `main` 没有移动、`VERSION` 一致、tag/Release 未占用，并核验同一 SHA 的 `Backend CI` 与 `Release Preflight` push run 均成功；之后才由工作流创建 annotated tag，并在同一执行链中调用 reusable `Release` workflow。`Release` 不提供独立的 tag push 或手动 dispatch 入口。

Promote 在创建 tag 前再次读取远端 `main`。若 Actions 在 tag 创建后、进入 Release job 前发生暂时故障，重跑同一 Promote run 可以复用“同版本、同审计 SHA、annotated”的已有 tag；任何对象不一致都会停止，不能覆盖 tag。

命令行等价入口仅用于触发同一个受控工作流，不直接创建 tag：

```bash
VERSION=$(cat backend/cmd/server/VERSION)
MAIN_SHA=$(git rev-parse origin/main)
gh workflow run promote-release.yml \
  --ref main \
  -f "version=${VERSION}" \
  -f "main_sha=${MAIN_SHA}"
```

Release workflow 必须：

1. 验证 tag 格式、annotated tag object、main ancestry 和版本顺序。
2. 验证 `VERSION` 与 tag 一致。
3. 从 tag 中读取 `docs/releases/v<version>.md`，禁止依赖 tag body。
4. 验证迁移可回滚、Go module 无漂移、unit tests 和 release assets tests。
5. 构建前端、deployer 和多架构镜像。
6. 校验 OCI manifest、digest、checksums 和 completion ledger。
7. 所有制品确认后才公开 GitHub Release 并移动 `latest`。

Release workflow 完成前不得部署生产。

## 12. Release 失败状态机

| 失败阶段 | 是否可重跑同一 tag | 处理 |
|---|---:|---|
| Release PR/preflight，尚未创建 tag | 是 | 修代码或 notes，保持计划版本 |
| tag 已创建，gate 失败，代码无需修改且没有冲突制品 | 是 | 修复外部配置或偶发基础设施后 rerun |
| tag 已创建，发现代码/迁移/notes 需要修改 | 否 | 新 commit、新 revision、新 tag |
| draft Release 已创建但 workflow 未完成 | 仅无代码变化时 | 保留证据，按 workflow reconciliation 处理 |
| 版本镜像已推送但验证失败 | 否 | 不覆盖镜像；恢复 `latest`，使用新 revision |
| 发布状态不明确/网络超时 | 否，先调查 | 查询 Release、tag、digest、completion ledger 后再决定 |
| Release 已完成，生产尚未部署 | 不修改旧版本 | 有问题则发布新 revision |
| 生产部署后发现问题 | 不修改旧版本 | 立即回滚上一完成版本，再做 hotfix release |

版本号空洞是不可变发布留下的审计证据，不通过删除或覆盖来美化历史。通过 tag 前 preflight 减少空洞。

## 13. 生产部署 SOP

生产只允许通过 managed deployer/UI 更新，不在服务器手工编辑应用代码。

部署前：

1. Release 为公开且包含 completion ledger。
2. 目标镜像 digest 与 ledger 一致。
3. 数据库备份完成并可读取目录。
4. 当前版本、容器、upstream 和健康状态已记录。
5. 上一完成版本仍可拉取。

部署：

1. 提交带 `expected_current_version` 和唯一 `request_id` 的部署任务。
2. 等待 candidate health、Nginx probe、流量切换和 drain 完成。
3. 不同时运行第二个部署任务，不并行修改 Nginx/Compose。
4. 观察健康、5xx、延迟、日志、数据库连接和后台任务至少 5 分钟。

成功证据包括：active version、active container ID、active port、image digest、degraded=false 和公网健康检查。

## 14. 回滚与 degraded SOP

自动更新失败时优先让 deployer 自动回滚。发生以下任一状态必须停止继续更新：

- `degraded`
- `rollback_failed`
- active slot 与 Nginx upstream 不一致
- container identity 无法确认
- 数据库迁移不兼容旧版本

处理顺序：

1. 不删除容器、不手改 state.json、不重复点击更新。
2. 确认当前实际承载流量的容器和 Nginx upstream。
3. 保存 deployer state、job、systemd、Nginx 和容器日志。
4. 若旧 slot 健康，按受控 reconcile/rollback 恢复。
5. 数据库已发生不可逆变化时，不得仅切回旧镜像。
6. 恢复后创建事件记录和代码修复 PR。

## 15. Hotfix SOP

仅生产故障、安全漏洞或严重数据错误使用 hotfix：

1. 从当前 `origin/main` 建最小分支。
2. 只修一个根因，不夹带重构和顺手优化。
3. 必须有回归测试；无法先写测试时说明原因并在同 PR 补齐。
4. required CI 与风险相关审计不能跳过。
5. 合并后建立独立 Release PR，递增 fork revision。
6. 生产部署仍走 managed deployer，不直接替换二进制。
7. 事后把临时防护转成长期修复或删除。

## 16. 安全与生产事件 SOP

事件响应与代码发布分离：

- 先通过 Cloudflare、Nginx、网络提供商或配置开关止损。
- 保全时间线、指标和日志，禁止把敏感原始数据提交到 GitHub。
- 需要代码修复时建立 security/hotfix PR。
- 紧急不等于允许复用 tag、跳过审计或手改生产源码。
- 事件结束后记录根因、影响、检测缺口和防复发项。

## 17. 分支、worktree 与废弃工作

PR 合并后：

```bash
git switch main
git pull --ff-only origin main
git branch -d <merged-branch>
git push origin --delete <merged-branch>
git fetch --prune
```

删除 worktree 前必须检查：

```bash
git worktree list --porcelain
git -C <worktree> status --short --branch
```

- 干净且内容已进入 main：可删除。
- 有未提交内容：先比较 main，确认是否有独有成果。
- 内容来源不明：保留并建立清单，不使用 `--force`。
- 由当前任务创建的临时验证 worktree：验证结束后可删除。

废弃 PR 必须说明原因和替代 PR。不得静默删除仍含独有提交的分支。

## 18. 禁止操作

- 在生产服务器直接修改源码或数据库 schema。
- 对共享 `main` force push。
- 覆盖、移动或删除已发布 tag。
- 使用同一版本重新发布不同 digest。
- 在 CI 未完成时创建正式 tag。
- 用 rerun 掩盖确定性失败。
- 将审计结果只保存在聊天记录。
- 在功能 PR 中同步 upstream 或准备 release。
- 未检查状态就强制删除 dirty worktree。
- 以 `latest` 作为生产回滚依据。

## 19. 完成定义

一项工作只有同时满足以下条件才算完成：

- 范围内代码、测试和文档已提交。
- PR 与 commit 历史可理解。
- CI 和必要审计针对最终 SHA 通过。
- 迁移、配置、部署和回滚影响已记录。
- PR 已合并，临时分支按规则处理。
- 若为 Release：notes、tag、digest、ledger 和生产状态可互相追溯。
