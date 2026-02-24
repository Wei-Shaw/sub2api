---
name: bug-fix-expert
description: 以"先确认、再修复"的多智能体协作方式处理缺陷，保证速度和安全。
license: MIT
compatibility: Claude Code（支持 Task 工具时启用并行协作，否则自动降级为单智能体顺序执行）。
metadata:
  author: project-team
  version: "4.3"
---

# Bug 修复专家（bug-fix-expert）

## 术语表

| 术语 | 定义 |
|------|------|
| **主控** | 主会话，负责协调流程、管理 worktree 生命周期、与用户沟通 |
| **子智能体** | 通过 Task 工具启动的独立 agent，执行具体任务后返回结果 |
| **角色** | 抽象职责分类（验证/分析/修复/安全/审查），映射到具体的子智能体 |
| **Beacon** | 完成信标（Completion Beacon），子智能体的结构化完成报告 |
| **Worktree** | 通过 `git worktree` 创建的隔离工作目录 |
| **三重门禁** | 交付前必须同时满足的三个条件：测试通过 + 审查通过 + 安全通过 |

## 触发条件

当以下任一条件满足时激活本技能：

- 用户明确报告 bug、异常、CI 失败、线上问题。
- 用户描述"实际行为 ≠ 预期行为"的现象。
- 代码审查报告中标记了 BUG-NNN / SEC-NNN 类问题需要修复。
- 用户显式要求"按 bug-fix-expert 流程处理"。

## 目标

以"先确认、再修复"的方式处理缺陷：

1. **先证明 bug 真实存在**（必须从多个角度确认）。
2. **若确认真实存在**：实施最佳修复方案，补齐测试，避免引入回归；修复后由独立角色审查改动，直至无明显问题。
3. **若确认不存在/无法证实**：只说明结论与证据，不修改任何代码。

## 适用范围

- **适用**：用户报告的异常、CI 失败、线上问题回溯、逻辑不符合预期、性能/并发/边界 bug 等。
- **不适用**：需求变更（应先确认产品预期）或纯重构（除非重构是修复的最小代价手段）。

## 强制原则（不可跳过）

1. **没有可重复的证据，不改代码**：至少满足"稳定复现"或"静态分析可严格证明存在"。
2. **多角度确认**：至少使用 3 种不同方式交叉验证（P0 可降至 2 种，但必须注明理由）。
3. **先写失败用例**：优先用最小化单元测试/集成测试把 bug "钉住"。
4. **修复必须带测试**：新增/完善测试覆盖 bug 场景与关键边界，确保回归保护。**改动代码的单元测试覆盖率必须 ≥ 85%**（以变更行为统计口径，非全仓覆盖率）。
5. **不引入新问题**：尽量小改动、低耦合；遵守项目既有分层与编码规范。
6. **修复与审查角色隔离**：修复者不得自审，必须由独立角色执行代码审查。
7. **安全前后双检**：修复前预扫描 + 修复后 diff 复核，两次都通过才算合格。
8. **Git 写操作必须确认**：任何会改变 Git 状态的操作必须先获得用户确认；只读诊断无需确认。**例外**：bugfix 流程中的临时 worktree 创建/删除和 `bugfix/*` 命名空间下的临时分支操作，在用户确认启动 bug 修复流程时即视为一次性授权，后续无需逐个确认。
9. **沟通与文档默认中文**：除非用户明确要求其他语言。
10. **Bug-ID 合法性校验**：Bug-ID 只允许包含字母、数字、连字符（`-`）和下划线（`_`），正则校验 `^[A-Za-z0-9_-]{1,64REDACTED$`。不符合规则的输入必须拒绝并提示用户修改。主控在构造路径和分支名前必须执行此校验。

## 严重度分级与响应策略

| 等级 | 定义 | 响应策略 |
|------|------|----------|
| **P0 — 线上崩溃/数据损坏** | 服务不可用、数据丢失/损坏、安全漏洞已被利用 | **快车道**：验证可降至 2 种交叉方式；跳过方案对比，直接最小修复；采用乐观并行（见"P0 乐观并行策略"） |
| **P1 — 核心功能阻断** | 主流程不可用但服务在线、影响大量用户 | **加速道**：方案设计精简为 1-2 句权衡；验证与分析并行 |
| **P2 — 功能异常/边界问题** | 非主流程异常、边界条件触发、体验降级 | **标准道**：完整执行全部步骤 |
| **P3 — 优化/改善** | 性能可改善、代码异味、非紧急潜在风险 | **标准道**：完整执行，可排入后续迭代 |

> 默认按 P2 处理；用户明确指出严重度或从上下文可判断时自动调级。

**P0 乐观并行策略**：P0 级别可同时启动验证和修复子智能体（修复基于初步分析的"最可能根因"先行工作）。若验证子智能体返回 `FAILED`（无法证实 bug），主控必须立即通过 `TaskStop` 终止修复子智能体、清理其 worktree，并跳转到"无法证实"结论。P0 乐观并行的回滚代价是浪费修复 agent 的工作量，但换取更快的修复速度。

## 标准工作流

### 0) 信息收集

收集并复述以下信息（缺失则主动追问）：

- **现象**：实际行为、报错信息/堆栈、日志片段。
- **预期**：应该发生什么？
- **环境**：版本号/分支、运行方式（本地/容器/CI）、关键配置。
- **复现步骤**：最小复现步骤与输入数据。
- **严重度**：根据影响面初步定级（P0-P3），决定后续流程节奏。

> 目标：确保"讨论的是同一个问题"，避免修错。

### 1) 真实性确认（多角度交叉验证）

**核心验证（必须完成至少 3 种，P0 可降至 2 种并注明理由）：**

**A. 运行复现**：按复现步骤在本地/容器复现；必要时降低变量（固定数据、关闭并发、固定随机种子）。

**B. 测试复现**：新增一个"修复前稳定失败"的最小测试（优先单测，其次集成测试）。
- 用例命名清晰，直接表达 bug。
- 失败原因明确，不依赖偶然时序。

**C. 静态交叉验证**：通过代码路径与边界条件推导 bug（空指针、越界、错误分支、并发竞态、上下文取消、事务边界、权限校验等），并与运行/测试现象一致。

**必做分析（不计入验证种类数，但每次必须执行）：**

**D. 影响面评估**：分析 bug 所在代码的调用链，列出可能受影响的上下游模块。

**E. 可选补充验证（强烈建议做至少 1 项）：**

- 变更输入/边界：最小值/最大值/空值/非法值/并发压力/时序变化。
- 对比历史/回归定位：优先只读方式（查看变更历史与责任行）。
- 临时诊断（不落库）：局部日志、断点、计数器、trace。

#### 判定标准

| 判定 | 条件 |
|------|------|
| **真实存在** | 可稳定复现（运行或测试）且现象可解释 |
| **可严格证明存在** | 难以复现，但静态分析可严格证明必现（明显的 nil deref/越界/必走错误分支） |
| **无法证实** | 无法稳定复现，且静态分析无法给出严格证明 → **停止，不修改任何代码** |

#### 结论汇总规则

- 若验证与分析结论一致 → 进入下一步。
- 若矛盾 → 启动额外验证（上述 E 项），**最多追加 2 轮**。仍矛盾则上报用户决策。

### 2) 方案设计

至少列出 2 个可行方案（P0 可跳过对比，直选最小修复并注明理由），明确权衡：

- 影响面（改动范围、是否影响 API/DB/数据兼容性）
- 风险（并发/安全/性能/回滚复杂度）
- 可测试性（是否容易写稳定测试）

选择"最小改动且可证明正确"的方案。

### 3) 实施修复

1. 先落地最小修复（尽量不重构、不改风格）。
2. 完善测试：
   - 覆盖 bug 场景（必须）
   - 覆盖关键边界与回归场景（必须）
   - 必要时增加集成/端到端验证（按影响面决定）
   - **改动代码覆盖率门禁**：对本次修改/新增的代码，单元测试行覆盖率必须 ≥ 85%。
     使用项目对应的覆盖率工具（Go: `go test -coverprofile` + 分析变更行覆盖；
     JS/TS: `--collectCoverageFrom` 指定变更文件；Python: `coverage run` + `coverage report --include`）
     仅统计本次变更文件中变更行的覆盖情况，不要求全仓覆盖率达标。
     若因代码结构原因（如纯配置、接口声明等不可测代码）无法达到 85%，
     必须在 Beacon 中说明原因和实际覆盖率。
3. 运行质量门禁（与项目 CI 对齐）：
   - 最小集合：受影响模块的单元测试 + 静态检查（lint/格式化/类型检查）。
   - 必要时：集成测试、端到端测试、兼容性验证、性能回归检查。
   - 不确定时：跑全量测试。
   - **覆盖率检查**：修复完成后运行覆盖率工具，确认变更代码覆盖率 ≥ 85%，将结果写入 Beacon。
4. 若引入新失败：优先修复新失败；不要用"忽略测试/删除用例"掩盖问题。

**安全预扫描（与修复并行）**：扫描修复方案**将要触及的代码区域的修复前基线版本**，检查已有安全隐患，评估修复方案是否可能引入新风险。注意：预扫描的对象是修复前的基线代码，而非修复进行中的中间状态。

### 4) 二次审查（角色隔离，独立审查）

由独立角色（而非修复者自身）执行代码审查，至少覆盖：

- **正确性**：空指针/越界/错误处理/返回值语义/事务与上下文。
- **并发**：竞态、锁粒度、goroutine 泄漏、通道关闭时序。
- **兼容性**：API/配置/数据迁移影响，旧数据是否可读。
- **可维护性**：命名、结构、可读性、分层依赖是否违规。
- **测试质量**：是否会偶发失败？是否覆盖根因？是否能防回归？变更代码覆盖率是否 ≥ 85%？

**安全最终复核**：对修复 diff 审查鉴权/越权、注入（SQL/命令/模板）、敏感信息泄露；若修复涉及依赖变更，额外检查依赖安全。主控在启动安全复核子智能体时，必须将第 3 步安全预扫描的 Beacon 结论作为上下文传入 prompt，复核者对比两次扫描结果，确认未引入新安全问题。

**迭代规则**：发现问题 → 修复者修正 → 再次审查。**最多迭代 3 轮**，超过则上报用户重新评估方案或引入人工审查。

### 5) 交付输出

> 进入交付前必须通过**三重门禁**：测试通过 + 审查通过 + 安全通过，缺一不可（无论严重度等级）。

#### bug 确认存在并已修复

```markdown
## Bug 修复报告

**Bug ID**：[BUG-NNN]
**严重度**：[P0🔴 / P1🟠 / P2🟡 / P3🟢]
**根因**：[触发条件 + 代码/逻辑原因，引用 file:line]

**影响面**：
- 受影响模块：[模块A → 模块B → ...]
- 受影响 API/用户：[说明]

**修复方案**：
- 改动说明：[做了什么、为何是最小且正确的修复]
- 改动文件：[file1:line, file2:line, ...]

**测试**：
- 新增/更新的测试：[测试名称 + 覆盖场景]
- 运行结果：[命令 + PASS/FAIL]

**安全扫描**：
- 预扫描：[通过/发现 N 项，已处理]
- 最终复核：[通过/发现 N 项，已处理]

**残余风险**：[仍可能存在的边界/后续建议，无则写"无"]

**回滚预案**：[P0/P1 必填：如何快速回滚]
```

#### bug 无法证实或不存在

```markdown
## Bug 调查报告

**结论**：无法证实 / 确认不存在
**判定依据**：
- 复现尝试：[方法 + 结果]
- 测试验证：[方法 + 结果]
- 静态分析：[分析要点]

**下一步**：[需要用户补充哪些信息才能继续]
```

## 智能体协作执行

### 角色与 Task 工具映射

本技能通过 Claude Code 的 Task 工具实现多角色协作。主会话即主控，子智能体通过 Task 工具启动。**所有涉及文件写操作的子智能体必须在独立 git worktree 中工作。**

| 角色 | Task subagent_type | 并行阶段 | 需要 Worktree | 职责 |
|------|-------------------|----------|:------------:|------|
| **主控** | 主会话（不用 Task） | 全程 | 否 | 协调流程、管理 worktree 生命周期、与用户沟通、汇总结论 |
| **验证** | `general-purpose` | 第 1 步 | **是** | 在隔离 worktree 中运行复现、编写失败测试、执行测试、收集运行时证据 |
| **分析** | `Explore` | 第 1 步（与验证并行） | 否（只读） | 静态代码分析、调用链追踪、影响面评估 |
| **修复** | `general-purpose` | 第 3 步 | **是** | 在隔离 worktree 中实施修复、补齐测试、运行质量门禁 |
| **安全** | `general-purpose` | 第 3-4 步 | 否（只读扫描） | 安全预扫描（扫基线代码）+ diff 复核 |
| **审查** | `general-purpose` | 第 4 步 | **是** | 在隔离 worktree 中独立审查 diff、运行测试验证（与修复者隔离） |

### Git Worktree 强制隔离策略

#### 核心规则

1. **写操作子智能体必须使用 git worktree**：验证（写测试）、修复（改代码）、审查（验证运行）必须在独立 worktree 中操作。
2. **只读子智能体无需 worktree**：分析（Explore）和安全扫描可直接读取主工作区或指定 worktree 的路径。
3. **主控独占 worktree 生命周期**：子智能体不得自行创建、删除或合并 worktree。

#### Bug-ID 校验（主控在第 0 步强制执行）

主控在使用 Bug-ID 构造路径前，必须校验其仅包含字母、数字、连字符和下划线（正则 `^[A-Za-z0-9_-]{1,64REDACTED$`）。不符合规则时拒绝并提示用户修改。此校验防止路径穿越（`../`）、命令注入（`;`、空格）和分支名冲突。

#### 命名规范

```bash
# Worktree 路径（使用 $TMPDIR 确保跨平台一致性，macOS 上为用户私有目录）
# 注意：macOS 的 $TMPDIR 通常以 / 结尾（如 /var/folders/xx/xxxx/T/），
# 必须先去除尾部斜杠，避免路径中出现双斜杠（//）。
# 由于 Bash 不支持嵌套参数展开，需要分两步处理：
_tmpbase="${TMPDIR:-/tmpREDACTED" && _tmpbase="${_tmpbase%/REDACTED"
BUGFIX_BASE="${_tmpbaseREDACTED/bugfix-$(id -u)"  # 以 UID 隔离不同用户
# 完整路径：${BUGFIX_BASEREDACTED-{bug-idREDACTED-{roleREDACTED
# 示例（macOS）：/var/folders/xx/xxxx/T/bugfix-501-BUG-042-verifier
# 示例（Linux）：/tmp/bugfix-1000-BUG-042-verifier

# 分支名
bugfix/{bug-idREDACTED/{roleREDACTED
# 示例
bugfix/BUG-042/verifier
bugfix/BUG-042/fixer
```

> 使用 `$TMPDIR` 而非硬编码 `/tmp/`，原因：(1) macOS 的 `/tmp` 是 `/private/tmp` 的符号链接，会导致 `git worktree list` 输出路径与构造路径不一致；(2) macOS 的 `$TMPDIR`（形如 `/var/folders/xx/xxxx/T/`）是用户私有目录（权限 700），其他用户无法读取，避免源码泄露。

#### Worktree 生命周期（主控执行）

```text
阶段 ①  创建 worktree（主控在启动子智能体前执行）
  # 创建前校验 Bug-ID 合法性（强制原则 #10）
  # 重要：umask 和 git worktree add 必须在同一个 Bash 调用中执行，
  # 因为 Bash 工具的 shell 状态（含 umask）不跨调用持久化。
  umask 077 && git worktree add -b bugfix/{bug-idREDACTED/{roleREDACTED ${BUGFIX_BASEREDACTED-{bug-idREDACTED-{roleREDACTED HEAD

  # 创建后禁用 worktree 的远程 push 能力（纵深防御）
  git -C ${BUGFIX_BASEREDACTED-{bug-idREDACTED-{roleREDACTED remote set-url --push origin PUSH_DISABLED

  # 若创建失败，按以下条件分支处理：
  #   情况 A — 分支已存在但无对应 worktree（上次清理不完整）：
  #     git branch -D bugfix/{bug-idREDACTED/{roleREDACTED && 重试 git worktree add
  #   情况 B — worktree 路径已存在（残留目录）：
  #     git worktree remove --force ${BUGFIX_BASEREDACTED-{bug-idREDACTED-{roleREDACTED
  #     git branch -D bugfix/{bug-idREDACTED/{roleREDACTED  # 分支可能也残留
  #     重试 git worktree add
  #   情况 C — 磁盘空间不足：
  #     尝试回退到 ~/.cache/bugfix-worktrees/bugfix-$(id -u)-{bug-idREDACTED-{roleREDACTED 目录
  #     （需先 umask 077 && mkdir -p ~/.cache/bugfix-worktrees，确保权限 700）
  #     注意：回退路径保持 "bugfix-{uidREDACTED-{bug-idREDACTED-{roleREDACTED" 命名格式，
  #     确保与 grep -F -- "-{bug-idREDACTED-" 清理模式兼容
  #   所有情况：最多重试 1 次，仍然失败 → 降级为单智能体模式，通知用户

阶段 ②  传递路径给子智能体
  主控通过 git worktree list --porcelain 获取实际创建路径（--porcelain 输出
  机器可解析的格式，避免路径中含空格时被截断；同时规避符号链接导致的路径不一致），
  将实际路径写入 Task prompt 中。

阶段 ③  子智能体在 worktree 中工作
  - 子智能体完成后通过完成信标（Completion Beacon）主动通知主控
  - 子智能体允许在 worktree 内执行 git add 和 git commit（因为 worktree 分支
    是临时隔离分支，不影响主分支；最终合并由主控在用户确认后执行）
  - 子智能体禁止执行 git push / git merge / git checkout 到其他分支

阶段 ④  主控独立验证 + 决定采纳
  主控收到 Beacon 后，不可仅凭 Beacon 声明做决策，必须独立验证关键声明：
  - Beacon 声明"测试通过" → 主控在 worktree 中重新运行测试确认
  - Beacon 声明"变更文件" → 主控通过 git diff 独立确认实际变更范围
  - Beacon 中的文件引用只允许 worktree 内的相对路径，拒绝绝对路径和含 ../ 的路径
  采纳：在主工作区执行 git merge / cherry-pick / 手动应用 diff（需用户确认）
  拒绝：直接清理 worktree

阶段 ⑤  清理 worktree（流程结束时，无论成功/失败/中断）
  git worktree remove --force ${BUGFIX_BASEREDACTED-{bug-idREDACTED-{roleREDACTED
  git branch -D bugfix/{bug-idREDACTED/{roleREDACTED   # 大写 -D 强制删除（临时分支可能未合并）
  # 清理后校验（使用 --porcelain 确保路径解析可靠）：
  # 注意：使用 -F 固定字符串匹配 + "-{bug-idREDACTED-" 精确匹配（避免 BUG-1 误匹配 BUG-10）
  # 使用 if/then 避免 grep 无匹配时 exit code 1 被 Bash 工具误报为错误
  if git worktree list --porcelain | grep -F -- "-{bug-idREDACTED-"; then
    echo "WARNING: 残留 worktree 未清理"
  fi
  git branch --list "bugfix/{bug-idREDACTED/*" | xargs -r git branch -D

  # 若清理失败（目录被锁定等）：
  #   1. 等待后重试 git worktree remove --force
  #   2. 仍失败：手动 rm -rf 目录，然后 git worktree prune
  #   3. 记录警告并告知用户手动检查
```

#### Worktree 安全约束

- **原子互斥**：不依赖 `grep` 预检查（存在 TOCTOU 竞态），直接执行 `git worktree add`——若目标已存在，git 本身会原子性地报错拒绝。`grep` 仅用于友好提示，不作为安全保证。
- **分支保护**：子智能体禁止直接 push 到远程或合并到主分支，创建 worktree 后主控通过 `remote set-url --push` 禁用 push 能力。
- **强制清理**：流程结束（成功/失败/中断/异常）时，主控必须执行 `git worktree list --porcelain | grep -F -- "-{bug-idREDACTED-"` 检查并清理所有该 bug 的临时 worktree 和 `bugfix/{bug-idREDACTED/*` 分支。
- **磁盘保护**：worktree 创建在 `$TMPDIR`（用户私有临时目录）下；若空间不足，回退到 `~/.cache/bugfix-worktrees/`（用户私有，权限 700），不使用系统级共享临时目录（如 `/tmp`）。回退路径同样采用 `bugfix-{uidREDACTED-{bug-idREDACTED-{roleREDACTED` 命名格式，确保 `grep -F -- "-{bug-idREDACTED-"` 清理模式可匹配。
- **敏感数据保护**：子智能体禁止在测试数据中使用真实密钥/token/凭据，必须使用 mock 数据。

### 并行执行策略（含 Worktree 生命周期）

```text
第 0 步  信息收集 → 主控
  ├─ 校验 Bug-ID 合法性（正则 ^[A-Za-z0-9_-]{1,64REDACTED$）
  ├─ 确定 BUGFIX_BASE 路径
  └─ 检查并清理可能残留的旧 worktree（git worktree list --porcelain | grep -F -- "-{bug-idREDACTED-"）

第 1 步  真实性确认 → 并行启动
  ├─ 主控: git worktree add ... verifier（创建验证 worktree）
  ├─ Task(general-purpose:验证, run_in_background=true, max_turns=30)
  │   ├─ prompt 包含 worktree 实际路径（从 git worktree list --porcelain 获取）
  │   ├─ 在 worktree 中编写失败测试、运行复现
  │   └─ 完成后输出 AGENT_COMPLETION_BEACON（主动通知）
  ├─ Task(Explore:分析, run_in_background=true, max_turns=20)
  │   ├─ 只读分析，无需 worktree
  │   └─ 完成后输出 AGENT_COMPLETION_BEACON（主动通知）
  ├─ [仅 P0] 主控: 同时创建 fixer worktree + 启动修复子智能体（乐观并行）
  │   ├─ 修复基于初步分析的"最可能根因"先行工作
  │   ├─ 若验证返回 FAILED → TaskStop 终止修复子智能体 + 清理其 worktree
  │   └─ 若验证成功 → 乐观修复已在进行中，直接跳到第 3 步等待其完成（跳过第 2 步方案设计）
  └─ 主控: 用 TaskOutput(block=false) 轮询，任一完成即处理
      若验证 agent 返回 FAILED → 可通过 TaskStop 终止分析 agent（或等待其完成后忽略结果）

第 2 步  方案设计 → 主控
  ├─ 汇总验证+分析的 Beacon 结论
  ├─ 若验证 agent 写了失败测试 → 从 worktree 获取 commit hash
  │   （git -C {verifier-worktreeREDACTED log -1 --format="%H"）
  │   然后在主分支执行 git cherry-pick {hashREDACTED（需用户确认）
  ├─ 清理验证 worktree
  └─ 创建修复 worktree 时以最新 HEAD（含已 cherry-pick 的测试）为基点

第 3 步  实施修复 → 分步启动
  ├─ 主控: git worktree add ... fixer（基于包含失败测试的最新 HEAD）
  ├─ Task(general-purpose:修复, run_in_background=true, max_turns=40)
  │   ├─ prompt 包含 worktree 路径 + 修复方案
  │   ├─ 在 fixer worktree 中实施修复、补齐测试、运行门禁
  │   └─ 完成后输出 AGENT_COMPLETION_BEACON（主动通知）
  ├─ Task(general-purpose:安全预扫描, run_in_background=true, max_turns=15)
  │   ├─ 扫描修复方案将触及的代码区域的修复前基线版本（读取主工作区）
  │   ├─ 注意：扫描对象是基线代码，不是 fixer worktree 中的中间状态
  │   └─ 完成后输出 AGENT_COMPLETION_BEACON（主动通知）
  ├─ 主控: 修复 Beacon 收到后，委托 Task(Bash, max_turns=3) 在 worktree 中重跑测试（仅返回 pass/fail）
  └─ 主控: 安全预扫描 + 修复验证都通过后，合并修复到主分支（需用户确认）

第 4 步  二次审查 → 并行启动
  ├─ 主控: git worktree add ... reviewer（基于合并修复后的最新 HEAD）
  ├─ Task(general-purpose:审查, run_in_background=true, max_turns=25)
  │   ├─ 在 reviewer worktree 中审查 diff、运行测试
  │   └─ 完成后输出 AGENT_COMPLETION_BEACON（主动通知）
  ├─ Task(general-purpose:安全复核, run_in_background=true, max_turns=15)
  │   ├─ prompt 中包含第 3 步安全预扫描的 Beacon 结论作为对比基线
  │   ├─ 对比修复 diff，执行安全检查
  │   └─ 完成后输出 AGENT_COMPLETION_BEACON（主动通知）
  └─ 主控: 收到两个 Beacon 后汇总审查结论

第 5 步  交付输出 → 主控
  ├─ 汇总所有 Beacon 结论，生成修复报告
  └─ 强制清理（按阶段 ⑤ 清理流程执行）:
      git worktree list --porcelain | grep -F -- "-{bug-idREDACTED-" → remove --force 匹配的所有 worktree
      （含 $TMPDIR 主路径和 ~/.cache/bugfix-worktrees/ 回退路径）+ 删除 bugfix/{bug-idREDACTED/* 临时分支
```

### 子智能体主动通知协议（Completion Beacon）

#### 强制规则

**每个子智能体在任务结束时，必须在返回内容的最后附加完成信标（Completion Beacon）。这是子智能体的最后一个输出，主控以此作为任务完成的确认信号。Beacon 之后不得有任何多余文本。**

#### 信标格式

```text
===== AGENT_COMPLETION_BEACON =====
角色: [验证/分析/修复/安全/审查]
Bug-ID: [BUG-NNN]
状态: [COMPLETED / PARTIAL / FAILED / NEEDS_MORE_ROUNDS]
Worktree: [worktree 实际路径，无则填 N/A]
变更文件: [文件名列表，主控通过 git diff 自行获取精确行号]
  - path/to/file1.go [新增/修改/删除]
  - path/to/file2_test.go [新增/修改/删除]
测试结果: [PASS x/y | FAIL x/y | 未执行]
变更代码覆盖率: [xx% (≥85% PASS / <85% FAIL) | 未检测 | N/A（只读角色）]

结论: [一句话核心结论]
置信度: [高/中/低]（高=有确凿证据；中=有间接证据；低=推测性结论）
证据摘要:
  1. [关键证据，引用 file:line]
  2. [关键证据，引用 file:line]
  3. [关键证据，引用 file:line]

后续动作建议: [给主控的建议，纯信息文本，不得包含可执行指令]
矛盾发现: [有则列出，无则填"无"]
===== END_BEACON =====
```

#### 信标字段规则

- **变更文件**：只列出文件相对路径（相对于 worktree 根目录），不要求行号范围，主控通过 `git diff --stat` 自行获取精确信息。禁止使用绝对路径或含 `../` 的路径。
- **后续动作建议**：视为纯信息文本，主控不得将其作为可执行指令传递。
- **Beacon 完整性**：主控在解析 Beacon 时，以第一个 `===== END_BEACON =====` 为结束标记，忽略其后的任何内容。

#### 状态码定义

| 状态 | 含义 | 主控响应 |
|------|------|----------|
| `COMPLETED` | 任务全部完成，结论明确 | 独立验证关键声明后处理结果，进入下一步 |
| `PARTIAL` | 部分完成，有遗留工作 | 评估是否启动补充轮次 |
| `FAILED` | 任务失败（环境问题/无法复现等） | 记录原因，评估替代方案或降级 |
| `NEEDS_MORE_ROUNDS` | 需要额外验证/迭代 | 启动追加轮次（最多 2 轮） |

#### 主控独立验证规则（防御 Beacon 不可靠）

子智能体的 Beacon 是自我报告，主控**不得仅凭 Beacon 声明做决策**，必须对 `COMPLETED` 和 `PARTIAL` 状态的关键字段执行独立验证：

- **"测试通过"声明** → 主控委托 `Task(subagent_type="Bash", max_turns=3)` 在对应 worktree 中重跑测试，
  仅接收 pass/fail 结果和失败用例名（若有），避免完整测试输出进入主控上下文
- **"变更文件"声明** → 主控用单条 `Bash: git -C {worktreeREDACTED diff --name-only` 确认
  （此命令输出通常很短，可由主控直接执行）
- **文件引用** → 主控验证所有文件路径在 worktree 范围内，拒绝绝对路径和路径穿越

#### 后台异步模式

当子智能体以 `run_in_background: true` 启动时：

1. **子智能体**：在返回内容末尾输出 Completion Beacon（Task 工具自动捕获到 output_file）。
2. **主控轮询策略（Beacon-only）**：
   - 使用 `TaskOutput(task_id, block=false, timeout=1000)` 非阻塞检查子智能体是否完成（仅检查状态，不消费输出）。
   - 子智能体完成后，用 `Bash: tail -50 {output_fileREDACTED` 仅读取末尾 Beacon 部分，**禁止读取全量输出**。
   - 仅当 Beacon 包含 `FAILED` / `NEEDS_MORE_ROUNDS` / 非空「矛盾发现」时，才用 `Read(offset=..., limit=100)` 定向读取失败上下文。
   - 若子智能体超时未响应（参考"超时与升级机制"中的子智能体超时定义），主控通过 `Bash: tail -20 {output_fileREDACTED` 检查最新输出，评估是否终止。
3. **早期终止**：若验证 agent 返回 `FAILED`（无法复现），主控可通过 `TaskStop` 终止其他正在运行的子智能体，并跳转到"无法证实"结论。

#### 通信规则

- 子智能体间不直接通信，全部经主控中转。
- 发现与预期矛盾的证据时，必须在 Beacon 的"矛盾发现"字段标注。
- 主控收到包含矛盾发现的 Beacon 后，必须暂停流程：终止所有已启动但未完成的下游子智能体，清理其 worktree，然后启动额外验证。

### 子智能体 Prompt 模板

主控启动子智能体时，必须在 Task prompt 中包含以下标准化信息：

```text
你是 Bug 修复流程中的【{角色名REDACTED】智能体。

## 任务上下文
- Bug-ID: {bug-idREDACTED
- 严重度: {P0-P3REDACTED
- Bug 描述: {现象概述REDACTED
- 你的工作目录: {worktree 实际路径，从 git worktree list --porcelain 获取REDACTED
- 允许修改的文件范围: {主控根据影响面分析预先确定的文件/目录列表，如 "backend/internal/service/*.go, backend/internal/handler/chat.go"；若为"不限"则可修改任意文件REDACTED

## 项目约定（主控根据实际项目填写，以下为示例）
- 后端语言：Go | 前端框架：Vue 3 + TypeScript
- 构建命令：make build | 测试命令：make test-backend / make test-frontend
- 代码风格：Go 用 gofmt，前端用 ESLint
- 沟通与代码注释使用中文
> 注：以上为本项目默认值。主控在启动子智能体时应根据实际项目的技术栈、
> 构建系统和编码规范调整此部分内容。

## 工作指令
{角色特定的工作指令REDACTED

## 强制约束
- 使用 Read/Write/Edit 工具时，所有文件路径必须以 {worktree 路径REDACTED 为前缀
- 使用 Bash 工具时，命令中使用绝对路径，或在命令开头加 cd {worktree 路径REDACTED &&
- 禁止读写工作目录之外的文件（除非是只读分析角色读取主工作区）
- 禁止执行 git push / git merge / git checkout 到其他分支
- 允许在 worktree 内执行 git add 和 git commit（临时分支，不影响主分支）
- 修改文件必须在"允许修改的文件范围"内；若需修改范围外的文件，在 Beacon 的"后续动作建议"中说明原因并请求主控确认，不要直接修改
- 测试中禁止使用真实密钥/token/凭据，必须使用 mock 数据
- 测试中禁止使用固定端口号，使用 0 端口让 OS 分配随机端口
- 如果尝试 5 轮后仍无法完成任务，立即输出 FAILED 状态的 Beacon 并停止
- **变更代码覆盖率 ≥ 85%**：修复/验证角色完成后，必须运行覆盖率工具检测本次变更代码的行覆盖率；
  低于 85% 时须补充测试直到达标，或在 Beacon 中说明无法达标的原因（如纯接口声明/配置等不可测代码）
- 返回结果必须精简：Beacon 的「证据摘要」每条不超过 80 字符
- 禁止在 Beacon 中复制大段源码，只引用 file:line
- Beacon 之前的工作过程输出（调试日志、中间推理）不需要结构化，主控不会读取这些内容

## 完成后必须做
任务完成后，你必须在返回内容的最后输出完成信标（Completion Beacon），格式如下：
===== AGENT_COMPLETION_BEACON =====
角色: {角色名REDACTED
Bug-ID: {bug-idREDACTED
状态: [COMPLETED / PARTIAL / FAILED / NEEDS_MORE_ROUNDS]
Worktree: {worktree 路径REDACTED
变更文件:
  - path/to/file.go [新增/修改/删除]
测试结果: [PASS x/y | FAIL x/y | 未执行]
变更代码覆盖率: [xx% | 未检测 | N/A]
结论: [一句话核心结论]
置信度: [高/中/低]
证据摘要:
  1. [关键证据，引用 file:line]
后续动作建议: [给主控的建议]
矛盾发现: [有则列出，无则填"无"]
===== END_BEACON =====

Beacon 之后不得输出任何内容。
```

### 单智能体降级模式

当环境不支持并行 Task（或任务简单无需多角色）时，主会话依次扮演所有角色：

1. **验证 + 分析**：先运行复现，再做静态分析（顺序执行）。降级模式下仍建议使用新分支隔离（`git checkout -b bugfix/{bug-idREDACTED/solo`），但不强制使用 worktree。
2. **安全预扫描**：修复前切换到"安全视角"，扫描修复将触及的代码区域，记录预扫描结论。
3. **修复**：直接在主会话的隔离分支中实施。
4. **审查**：修复完成后，主会话切换到"审查视角"，用 `git diff` 逐项审查清单。此时必须假设自己不是修复者，严格按清单逐条检查。同步执行安全 diff 复核，与预扫描结论对比。
5. **安全**：在审查阶段同步检查安全项。

> 降级模式下审查质量不可降低：审查清单的每一项都必须逐条确认。
> P0/P1 级别问题不建议使用降级模式（自审偏见风险），建议至少启动一个独立审查子智能体。

降级模式下每个阶段结束仍需输出简化版阶段检查点：

```text
----- 阶段检查点 -----
阶段: [验证/分析/预扫描/修复/审查]
状态: [COMPLETED / PARTIAL / FAILED / NEEDS_MORE_ROUNDS]
结论: [一句话核心结论]
置信度: [高/中/低]
证据摘要: [关键证据 1-3 条]
----- 检查点结束 -----
```

## 安全规则

### Git 操作

| 类别 | 规则 |
|------|------|
| **只读诊断** | 默认允许：查看状态/差异、搜索、查看历史与责任行 |
| **有副作用** | 必须先获得用户确认：提交、暂存、拉取/推送、切换分支、合并、变基、打标签。执行前输出变更摘要 + 影响范围 + 测试结果。**例外**：`bugfix/*` 临时分支和 worktree 的创建/删除在用户确认启动修复流程时一次性授权 |
| **破坏性** | 默认禁止：强制回退/清理/推送。用户二次确认且说明风险后方可执行 |

### 多智能体并行安全

当多个 agent 同时修复不同 bug 时：

1. **工作区隔离（强制）**：每个写操作 agent **必须**使用 git worktree 隔离工作区，禁止多个 agent 在同一工作目录并行写操作。违反此规则的子智能体结果将被主控拒绝。
2. **变更范围预声明**：主控在启动修复子智能体时，在 prompt 中预先声明该 agent 允许修改的文件范围。子智能体若需修改范围外的文件，必须在 Beacon 中标注并请求主控确认。
3. **禁止破坏性全局变更**：禁止全仓格式化、大规模重命名、批量依赖升级（除非已获用户确认）。
4. **临时产物隔离**：复现脚本、测试数据等放入 worktree 内的 `.bugfix-tmp/` 目录。清理 worktree 时使用 `--force` 参数确保连同临时产物一起删除。子智能体禁止在 worktree 外创建临时文件。
5. **并发测试安全**：子智能体编写测试时必须使用 `0` 端口让 OS 分配随机端口，使用 `os.MkdirTemp` 创建独立临时目录，禁止使用固定端口或固定临时文件名。
6. **Worktree 清理强制**：流程结束（无论成功/失败/中断）必须使用 `git worktree remove --force` 清理所有临时 worktree，然后用 `git branch -D` 删除对应的临时分支。清理后执行校验确认无残留。
7. **合并冲突处理**：主控合并 worktree 变更时若遇冲突，必须暂停并上报用户决策，不得自动解决冲突。
8. **残留清理**：每次 bug-fix-expert 流程启动时（第 0 步），主控检查是否有超过 24 小时的残留 bugfix worktree 并清理。

### 安全护栏

1. **修复前影响面分析**：分析智能体生成调用链，防止改动波及意外模块。
2. **安全前后双检**：第 3 步预扫描（扫基线代码）+ 第 4 步 diff 复核（扫修复后 diff），形成闭环。
3. **角色隔离**：审查者与修复者必须是不同的智能体/角色。
4. **矛盾即暂停**：任意两个角色结论矛盾时，主控暂停流程——终止所有进行中的下游子智能体、清理其 worktree——然后启动额外验证。
5. **三重门禁不可跳过**：测试通过 + 审查通过 + 安全通过，缺一不可（无论严重度等级）。
6. **Beacon 独立验证**：主控不得仅凭子智能体 Beacon 的自我声明做决策，必须独立验证测试结果和变更范围（详见"主控独立验证规则"）。
7. **Prompt 约束为软约束**：子智能体的约束（不 push、不越界操作等）通过 Prompt 声明，属于软约束层。主控通过独立验证（检查 `git log`、`git remote -v`、`git diff`）提供纵深防御，确认子智能体未执行禁止操作。

## 超时与升级机制

| 阶段 | 超时信号 | 处理方式 |
|------|----------|----------|
| 子智能体响应 | 子智能体启动后连续 3 次 `TaskOutput(block=false)` 检查（每次间隔处理其他工作后再查）仍无完成输出 | 主控通过 `Read` 检查其 output_file 最新内容；若输出停滞（最后一行内容与上次检查相同），通过 `TaskStop` 终止并降级为主控直接执行该角色任务 |
| 真实性确认 | 矛盾验证追加超过 2 轮仍无共识 | 上报用户：当前证据 + 请求补充信息或决定是否继续 |
| 方案设计 | 所有方案风险都较高，无明显最优解 | 呈现方案对比，由用户决策 |
| 实施修复 | 修复引入的新失败无法在合理迭代内解决 | 建议回退修复或切换方案 |
| 二次审查 | 审查-修复迭代超过 3 轮仍有问题 | 建议重新评估方案或引入人工审查 |

> 注：由于 Claude Code 的 Task 工具不提供基于挂钟时间的超时机制，子智能体超时通过"轮询无进展"来判定，而非固定时间阈值。主控在等待期间应处理其他可并行的工作（如处理另一个已完成的子智能体结果），然后再回来检查。

## 上下文管理

长时间 bug 调查可能消耗大量上下文窗口，遵循以下原则：

- **Beacon-only 消费（最重要）**：主控通过 `tail -50` 仅读取子 agent 输出末尾的 Beacon，
  禁止通过 `TaskOutput(block=true)` 或 `Read` 全量读取子 agent 输出。详见「上下文预算控制」。
- **独立验证委托**：测试重跑等验证操作委托给 Bash 子 agent，主控只接收 pass/fail 结论。
- **大文件用子智能体**：超过 500 行的代码分析任务，优先用 Task(Explore) 处理，避免主会话上下文膨胀。
- **阶段性摘要卡**：每完成一个步骤，输出不超过 15 行的摘要卡，后续步骤仅引用摘要卡。
- **只保留关键证据**：子智能体返回结果时只包含关键的 file:line 引用，不复制大段源码。
- **复杂度评估**：主控在第 0 步评估 bug 复杂度——对于 P2/P3 级别的简单 bug（影响单文件、根因明确），默认使用降级模式以节省上下文开销；仅当 bug 复杂（P0/P1 或跨多模块）时启用并行模式。
- **max_turns 强制**：所有子 agent 必须设置 max_turns（详见「上下文预算控制」表格）。

### 上下文预算控制（强制执行）

#### A. Beacon-only 消费模式

主控读取子 agent 结果时，**禁止读取全量输出**，必须采用 Beacon-only 模式：

1. 子 agent 以 `run_in_background=true` 启动，输出写入 output_file
2. 子 agent 完成后，主控用 Bash `tail -50 {output_fileREDACTED` 只读取末尾的 Beacon 部分
3. 仅当 Beacon 状态为 `FAILED` / `NEEDS_MORE_ROUNDS` 或包含"矛盾发现"时，
   才用 `Read(offset=...)` 定向读取相关段落（不超过 100 行）
4. **禁止使用 `TaskOutput(block=true)` 获取完整输出** — 这会将全量内容灌入上下文

#### B. 独立验证委托

主控的"独立验证"（重跑测试、检查 diff）不再由主控亲自执行，而是委托给轻量级验证子 agent：

| 验证项 | 委托方式 | 返回格式 |
|--------|---------|---------|
| 重跑测试 | `Task(subagent_type="Bash", max_turns=3)` | `PASS x/y` 或 `FAIL x/y + 失败用例名` |
| 检查变更范围 | `Task(subagent_type="Bash", max_turns=2)` | `git diff --name-only` 的文件列表 |
| 路径合规检查 | 主控直接用单条 Bash 命令 | 仅 pass/fail |

这样避免测试输出（可能数百行）和 diff 内容进入主控上下文。

#### C. 子 agent max_turns 约束

所有子 agent 启动时必须设置 `max_turns` 参数，防止单个 agent 输出爆炸：

| 角色 | max_turns 上限 | 说明 |
|------|---------------|------|
| 验证 | 30 | 需要写测试+运行，允许较多轮次 |
| 分析（Explore） | 20 | 只读探索，通常足够 |
| 修复 | 40 | 改代码+测试+门禁，需要较多轮次 |
| 安全扫描 | 15 | 只读扫描 |
| 审查 | 25 | 审查+可能的验证运行 |
| 独立验证（Bash） | 3 | 仅跑命令取结果 |

#### D. 阶段性上下文压缩

每完成一个工作流步骤，主控必须将该阶段结论压缩为「阶段摘要卡」（不超过 15 行），
后续步骤仅引用摘要卡，不回溯原始 Beacon：

```text
阶段摘要卡格式：

----- 阶段摘要 #{步骤号REDACTED {步骤名REDACTED -----
结论: {一句话REDACTED
关键证据: {最多 3 条，每条一行，含 file:lineREDACTED
影响文件: {文件列表REDACTED
前置条件满足: [是/否]
遗留问题: {有则列出，无则"无"REDACTED
-----
```

#### E. 子 agent Prompt 精简指令

在子 agent Prompt 模板的「强制约束」部分追加以下要求：

- 返回结果必须精简：Beacon 的「证据摘要」每条不超过 80 字符
- 禁止在 Beacon 中复制大段源码，只引用 file:line
- Beacon 之前的工作过程输出（调试日志、中间推理）不需要结构化，
  因为主控不会读取这些内容
