---
name: code-review-expert
description: >
  通用代码审核专家 — 基于 git worktree 隔离的多 Agent 并行代码审核系统，集成 Context7 MCP 三重验证对抗代码幻觉。
  语言无关，适用于任意技术栈（Go, Python, JS/TS, Rust, Java, C# 等）。
  Use when: (1) 用户要求代码审核、code review、安全审计、性能审查,
  (2) 用户说"审核代码"、"review"、"检查代码质量"、"安全检查",
  (3) 用户要求对 PR、分支、目录或文件做全面质量检查,
  (4) 用户提到"代码审核专家"或"/code-review-expert"。
  五大审核维度：安全合规、架构设计、性能资源、可靠性数据完整性、代码质量可观测性。
  自动创建 5 个 git worktree 隔离环境，派发 5 个专项子 Agent 并行审核，
  通过 Context7 MCP 拉取最新官方文档验证 API 用法，消除 LLM 幻觉，
  汇总后生成结构化 Markdown 审核报告，最终自动清理所有 worktree。
---

# Universal Code Review Expert

基于 git worktree 隔离 + 5 子 Agent 并行 + Context7 反幻觉验证的通用代码审核系统。

## Guardrails

- **只读审核**，绝不修改源代码，写入仅限报告文件
- **语言无关**，通过代码模式识别而非编译发现问题
- 每个子 Agent 在独立 **git worktree** 中工作
- 审核结束后**无条件清理**所有 worktree（即使中途出错）
- 问题必须给出**具体 `file:line`**，不接受泛泛而谈
- 涉及第三方库 API 的发现必须通过 **Context7 MCP** 验证，严禁凭记忆断言 API 状态
- 文件 > 500 个时自动启用**采样策略**
- **上下文保护**：严格遵循下方 Context Budget Control 规则，防止 200K 上下文耗尽

## Context Budget Control (上下文预算管理)

> **核心问题**：5 个子 Agent 并行审核时，每个 Agent 读取大量文件会快速耗尽 200K 上下文，导致审核卡住或失败。

### 预算分配策略

主 Agent 在 Phase 0 必须计算上下文预算，并分配给子 Agent：

```
总可用上下文 ≈ 180K tokens（预留 20K 给主 Agent 汇总）
每个子 Agent 预算 = 180K / 5 = 36K tokens
每个子 Agent 可读取的文件数 ≈ 36K / 平均文件大小
```

### 七项强制规则

1. **文件分片不重叠**：每个文件只分配给**一个主要维度**（按文件类型/路径自动判断），不要多维度重复审核同一文件。高风险文件（auth、crypto、payment）例外，可分配给最多 2 个维度。

2. **单文件读取上限**：子 Agent 读取单个文件时，使用 `Read` 工具的 `limit` 参数，每次最多读取 **300 行**。超过 300 行的文件分段读取，仅审核关键段落。

3. **子 Agent prompt 精简**：传递给子 Agent 的 prompt 只包含：
   - 该维度的**精简检查清单**（不要传全部 170 项，只传该维度的 ~30 项）
   - 文件列表（路径即可，不包含内容）
   - C7 缓存中**该维度相关的**部分（不传全量缓存）
   - 输出格式模板（一次，不重复）

4. **结果输出精简**：子 Agent 找到问题后只输出 JSON Lines，**不要**输出解释性文字、思考过程或总结。完成后只输出 status 行。

5. **子 Agent max_turns 限制**：每个子 Agent 使用 `max_turns` 参数限制最大轮次：
   - 文件数 ≤ 10: `max_turns=15`
   - 文件数 11-30: `max_turns=25`
   - 文件数 31-60: `max_turns=40`
   - 文件数 > 60: `max_turns=50`

6. **大仓库自动降级**：
   - 文件数 > 200：减为 **3 个子 Agent**（安全+可靠性、架构+性能、质量+可观测性）
   - 文件数 > 500：减为 **2 个子 Agent**（安全重点、质量重点）+ 采样 30%
   - 文件数 > 1000：单 Agent 串行 + 采样 15% + 仅审核变更文件

7. **子 Agent 使用 `run_in_background`**：所有子 Agent Task 调用设置 `run_in_background=true`，主 Agent 通过 Read 工具轮询 output_file 获取结果，避免子 Agent 的完整输出回填到主 Agent 上下文。

### 文件分配算法

按文件路径/后缀自动分配到主要维度：

| 模式 | 主维度 | 辅助维度（仅高风险文件） |
|------|--------|----------------------|
| `*auth*`, `*login*`, `*jwt*`, `*oauth*`, `*crypto*`, `*secret*` | Security | Reliability |
| `*route*`, `*controller*`, `*handler*`, `*middleware*`, `*service*` | Architecture | - |
| `*cache*`, `*pool*`, `*buffer*`, `*queue*`, `*worker*` | Performance | - |
| `*db*`, `*model*`, `*migration*`, `*transaction*` | Reliability | Performance |
| `*test*`, `*spec*`, `*log*`, `*metric*`, `*config*`, `*deploy*` | Quality | - |
| 其余文件 | 按目录轮询分配到 5 个维度 | - |

### 主 Agent 汇总时的上下文控制

Phase 3 汇总时，主 Agent **不要**重新读取子 Agent 审核过的文件。仅基于子 Agent 输出的 JSON Lines 进行：
- 去重合并
- 严重等级排序
- Context7 交叉验证（仅对 critical/high 且未验证的少数发现）
- 填充报告模板

---

## Workflow

### Phase 0 — Scope Determination

1. **确定审核范围**（按优先级）：
   - 用户指定的文件/目录
   - 未提交变更：`git diff --name-only` + `git diff --cached --name-only`
   - 未推送提交：`git log origin/{mainREDACTED..HEAD --name-only --pretty=format:""`
   - 全仓库（启用采样：变更文件 → 高风险目录 → 入口文件 → 其余 30% 采样）

2. **收集项目元信息**：语言构成、目录结构、文件数量

3. **生成会话 ID**：
   ```bash
   SESSION_ID="cr-$(date +%Y%m%d-%H%M%S)-$(openssl rand -hex 4)"
   WORKTREE_BASE="/tmp/${SESSION_IDREDACTED"
   ```

4. 将文件分配给 5 个审核维度（每个文件可被多维度审核）

### Phase 0.5 — Context7 Documentation Warm-up (反幻觉第一重)

> 详细流程见 [references/context7-integration.md](references/context7-integration.md)

1. 扫描依赖清单（go.mod, package.json, requirements.txt, Cargo.toml, pom.xml 等）
2. 提取核心直接依赖，按优先级筛选最多 **10 个关键库**：
   - P0 框架核心（web 框架、ORM）→ P1 安全相关 → P2 高频 import → P3 其余
3. 对每个库调用 `resolve-library-id` → `get-library-docs`（每库 ≤ 5000 tokens）
4. 构建 **C7 知识缓存 JSON**，传递给所有子 Agent
5. **降级**：Context7 不可用时跳过，报告标注 "未经官方文档验证"

### Phase 1 — Worktree Creation

```bash
CURRENT_COMMIT=$(git rev-parse HEAD)
for dim in security architecture performance reliability quality; do
  git worktree add "${WORKTREE_BASEREDACTED/${dimREDACTED" "${CURRENT_COMMITREDACTED" --detach
done
```

### Phase 2 — Parallel Sub-Agent Dispatch (反幻觉第二重)

**在一条消息中发出所有 Task 调用**（`subagent_type: general-purpose`），**必须设置**：
- `run_in_background: true` — 子 Agent 后台运行，结果写入 output_file，避免回填主 Agent 上下文
- `max_turns` — 按文件数量设置（见 Context Budget Control）
- `model: "sonnet"` — 子 Agent 使用 sonnet 模型降低延迟和 token 消耗

Agent 数量根据文件规模自动调整（见 Context Budget Control 大仓库降级规则）。

每个 Agent 收到：

| 参数 | 内容 |
|------|------|
| worktree 路径 | `${WORKTREE_BASEREDACTED/{dimensionREDACTED` |
| 文件列表 | 该维度**独占分配**的文件（不重叠） |
| 检查清单 | 该维度对应的精简清单（~30 项，非全量 170 项） |
| C7 缓存 | 仅该维度相关的库文档摘要 |
| 输出格式 | JSON Lines（见下方） |
| 文件读取限制 | 单文件最多 300 行，使用 Read 的 limit 参数 |

每个发现输出一行 JSON：
```json
{
  "dimension": "security",
  "severity": "critical|high|medium|low|info",
  "file": "path/to/file.go",
  "line": 42,
  "rule": "SEC-001",
  "title": "SQL Injection",
  "description": "详细描述",
  "suggestion": "修复建议（含代码片段）",
  "confidence": "high|medium|low",
  "c7_verified": true,
  "verification_method": "c7_cache|c7_realtime|model_knowledge",
  "references": ["CWE-89"]
REDACTED
```

**关键规则**：
- 涉及第三方库 API 的发现，未经 Context7 验证时 `confidence` 不得为 `high`
- `verification_method == "model_knowledge"` 的发现自动降一级置信度
- 每个子 Agent 最多消耗分配的 Context7 查询预算
- 完成后输出：`{"status":"complete","dimension":"...","files_reviewed":N,"issues_found":N,"c7_queries_used":NREDACTED`

### Phase 3 — Aggregation + Cross-Validation (反幻觉第三重)

1. 等待所有子 Agent 完成
2. 合并 findings，按 severity 排序
3. **Context7 交叉验证**：
   - 筛选 `c7_verified==false` 且 severity 为 critical/high 的 API 相关发现
   - 主 Agent 独立调用 Context7 验证
   - 验证通过 → 保留 | 验证失败 → 降级或删除（标记 `c7_invalidated`）
4. 去重（同一 file:line 合并）
5. 生成报告到 `code-review-report.md`（模板见 [references/report-template.md](references/report-template.md)）

### Phase 4 — Cleanup (必须执行)

```bash
for dim in security architecture performance reliability quality; do
  git worktree remove "${WORKTREE_BASEREDACTED/${dimREDACTED" --force 2>/dev/null
done
git worktree prune
rm -rf "${WORKTREE_BASEREDACTED"
```

> 即使前面步骤失败也**必须执行**此清理。

## Severity Classification

| 等级 | 标签 | 定义 |
|------|------|------|
| P0 | `critical` | 已存在的安全漏洞或必然导致数据丢失/崩溃 |
| P1 | `high` | 高概率触发的严重问题或重大性能缺陷 |
| P2 | `medium` | 可能触发的问题或明显设计缺陷 |
| P3 | `low` | 代码质量问题，不直接影响运行 |
| P4 | `info` | 优化建议或最佳实践提醒 |

置信度：`high` / `medium` / `low`，低置信度须说明原因。

## Five Review Dimensions

每个维度对应一个子 Agent，详细检查清单见 [references/checklists.md](references/checklists.md)：

1. **Security & Compliance** — 注入漏洞(10 类)、认证授权、密钥泄露、密码学、依赖安全、隐私保护
2. **Architecture & Design** — SOLID 原则、架构模式、API 设计、错误策略、模块边界
3. **Performance & Resource** — 算法复杂度、数据库性能、内存管理、并发性能、I/O、缓存、资源泄漏
4. **Reliability & Data Integrity** — 错误处理、空值安全、并发安全、事务一致性、超时重试、边界条件、优雅关闭
5. **Code Quality & Observability** — 复杂度、重复、命名、死代码、测试质量、日志、可观测性、构建部署

## Context7 Anti-Hallucination Overview

> 详细集成文档见 [references/context7-integration.md](references/context7-integration.md)

三重验证防御 5 类 LLM 幻觉：

| 幻觉类型 | 说明 | 防御层 |
|----------|------|--------|
| API 幻觉 | 错误断言函数签名 | 第一重 + 第二重 |
| 废弃幻觉 | 错误标记仍在用的 API 为 deprecated | 第二重 + 第三重 |
| 不存在幻觉 | 声称新增 API 不存在 | 第一重 + 第二重 |
| 参数幻觉 | 错误描述参数类型/默认值 | 第二重实时查 |
| 版本混淆 | 混淆不同版本 API 行为 | 第一重版本锚定 |

验证覆盖度评级：`FULL` (100% API 发现已验证) > `PARTIAL` (50%+) > `LIMITED` (<50%) > `NONE`

## Error Handling

- 某个子 Agent 失败：继续汇总其他结果，报告标注不完整维度
- git worktree 创建失败：`git worktree prune` 重试 → 仍失败则回退串行模式
- Context7 不可用：跳过验证阶段，报告标注 "未经官方文档验证"
- 所有情况下 **Phase 4 清理必须执行**

## Resources

- **[references/checklists.md](references/checklists.md)** — 5 个子 Agent 的完整检查清单 (~170 项)
- **[references/context7-integration.md](references/context7-integration.md)** — Context7 MCP 集成详细流程、缓存格式、查询规范
- **[references/report-template.md](references/report-template.md)** — 审核报告 Markdown 模板
