# Claude Code Batch Image QA Report

Date: 2026-07-06
Tester: Claude Code
Claude model selection:

- Preferred for deep QA: `opus`, but the first run exceeded the initial budget before producing output.
- Practical model used for this recorded report: `sonnet` with `--safe-mode --effort low`, because it produced a bounded independent QA report at lower cost.

## Original Claude Output

> Batch Image 功能 QA 报告（只读探查）
>
> ## 范围
> 后端计费冻结/结算/退款、状态机与异常兜底、前端批量生图说明文案。基于代码走查(Explore agent)+ 2 条本地 grep 命令验证，未修改任何文件，未执行且未查看任何密钥。
>
> ## 执行命令
>
> | # | 命令 | 目的 |
> |---|------|------|
> | 1 | `grep -n "FOR UPDATE\|Lock(" batch_image_settlement.go batch_image_repo.go` | 验证取消/结算并发是否有行锁保护 |
> | 2 | `grep -rn "SETTLEMENT_BILLING_FAILED\|enqueueBillingRetry\|MaxRetr" ...` | 验证结算失败重试是否有次数/退避上限 |
>
> ## 通过/失败表
>
> | 检查项 | 结果 | 依据 |
> |---|---|---|
> | 状态转换行锁保护(防止取消/结算竞态) | ✅ 通过 | `batch_image_repo.go:193,322,415` 均用 `SELECT ... FOR UPDATE` |
> | 结算超额扣费保护 | ✅ 通过 | `batch_image_settlement.go:126-130`，`actualCost > holdAmount` 超万分之一即失败中止 |
> | 冻结→结算→释放状态机完整性 | ✅ 通过 | 冻结(billing_hold.go) → 结算(settlement.go) → 释放(processor.go:225-239) 链路闭合 |
> | 僵尸/未提交任务资金释放 | ✅ 通过 | `billing_recovery.go:22-62`，10分钟未提交自动 failed + 释放冻结 |
> | 非法状态转换保护 | ✅ 通过 | `batch_image.go:356-401` 终态不可逆流转 |
> | 部分失败正确计费(仅成功项扣费) | ✅ 通过 | `actualCost = successCount * unitPrice` |
> | 结算失败重试有界（次数/超时上限） | ⚠️ 未证实 | grep 未发现 `MaxRetr`/退避上限，仅见标记 `SETTLEMENT_BILLING_FAILED` 后重新入队，逻辑分散在其他文件未定位到边界 |
> | 前端费用/取消文案与后端逻辑一致 | ⚠️ 基本一致，措辞有偏差 | 见问题清单 P2 |
>
> ## 问题清单（按严重级别）
>
> **P1（无，未发现资金泄漏或重复扣款的确认性缺陷）**
>
> **P2 – 中**
> 1. 结算失败重试缺乏可见的次数/超时上限（`SETTLEMENT_BILLING_FAILED` 后 `enqueueBillingRetry`），存在长期卡在 `settling` 状态、资金持续冻结但不释放也不完成结算的风险；需要进一步定位重试调度代码确认是否有兜底超时释放。
> 2. 前端取消提示文案（"已生成图片仍可能结算扣费"）与后端实际计费口径（以索引完成后统计的成功项为准）表述不完全对齐，可能造成用户对扣费范围的误解，建议澄清措辞而非改变逻辑。
>
> **P3 – 低**
> 3. 结算过程中途宕机（`Settle()` 执行到一半进程重启）依赖外部定时任务/人工介入恢复，未在本次探查中确认是否有自动扫描 `settling` 超时状态的兜底任务。
>
> ## 剩余风险
> - 未验证"结算失败重试"的调度器代码（未在本次两条命令范围内），无法排除无限重试或永久悬挂的可能性。
> - 未做真实并发压测，行锁存在但未验证高并发下取消+结算同时触发的实际表现（仅代码静态确认加锁点存在）。
> - 前端文案审查仅基于关键字定位的片段，未通读整个 Guide 组件的所有分支文案。
>
> ## 建议后续测试
> 1. 定位并审查 `enqueueBillingRetry` 实际调度器（重试次数、退避策略、是否有最终告警/人工介入路径），必要时补充单测覆盖"结算持续失败"场景。
> 2. 编写并发集成测试：同一 batch_id 同时发起"取消"与"结算完成回调"，验证最终状态与金额一致性（是否只释放或只结算一次）。
> 3. 对 `settling` 状态增加超时巡检的专项测试（类比现有 10 分钟未提交巡检），确认是否已有等价机制，如无需评估是否要补齐。
> 4. 前端文案走查+产品确认，将"取消后扣费口径"说明与后端"仅索引完成的成功项计费"对齐后再验收。

## Codex Follow-Up Note

Codex spot-checked the first P2 after Claude's report. `enqueueBillingRetry` exists in `batch_image_public.go`, but no obvious max retry or terminal handoff was found in the quick search. Keep this as an open risk for the next implementation/test pass rather than treating it as resolved.

