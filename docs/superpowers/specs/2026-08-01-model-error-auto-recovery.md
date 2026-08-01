---
summary: 让账号按模型自动复测系统记录的临时错误，并在上游恢复后立即精确恢复该模型调度
doc_kind: spec
status: active
review_status: approved
---

# 模型错误自动复测与精确恢复

执行者只以本 Spec 为任务来源；批准后先进入任务 worktree，创建并持续更新 `PROGRESS.md`，拿不准或前提不符时写 `BLOCKED.md`。中断后先读这两个文件接着做。取舍顺序是：不误恢复仍失败的模型 > 自动尽快恢复 > 设置简单。本文“只允许 / 不许”是验收合同，“建议”可在 `PROGRESS.md` 说明理由后调整。

## 这活为什么干

账号可能仍为正常状态，但 `accounts.extra.model_rate_limits` 同时记录 `gpt-5.6-luna`、`gpt-5.6-sol`、`gpt-5.6-terra` 等模型的临时错误。现在只能等倒计时结束或手工测试；目标是仅在系统自动记录错误后按模型复测，哪个模型恢复就立刻只恢复哪个模型。

## 领导已经拍板

- 错误恢复默认每 5 分钟重试，可输入分钟数；高级模式默认隐藏，可改用 5 字段 Cron。
- 分钟间隔按测试启动时间计算；同一计划不并发重入，若上一轮超过间隔仍未完成，则在完成后的下一次扫描补跑。
- 健康账号不测试；手工禁用、手工不可调度、过期等人工或永久状态绝不自动恢复。
- 多个模型独立重试和恢复；一个模型成功不得清除其他模型的错误。
- 有结构化来源的账号级自动错误也可恢复；错误恢复计划可多选探测模型，默认全选。模型级错误始终测试实际报错模型；账号级错误使用首个已选模型探测一次。裸 `status=error` 无法区分人工与系统来源，不自动恢复。

## 我替领导拍的板（待确认）

- 每账号只允许 1 个 `error_recovery` 计划，避免重复请求；普通定时计划仍可有多个。
- 分钟间隔允许 `1..1440`；Cron 使用现有 5 字段解析器。`AICredits` 是额度状态而非模型，排除自动复测和自动清除。
- 检测到新错误后在下一次每分钟扫描中首次测试；之后按间隔或 Cron 重试，不额外等待一个完整周期。

## 范围与非目标

- 只允许修改 scheduled-test 的 migration、repository/service/handler、API/types、`ScheduledTestsPanel.vue`、中英文文案及对应测试；其他路径只读。
- 复用现有每分钟 `ScheduledTestRunnerService`、10 worker 上限和账号错误真源；不新增 worker、消息队列、依赖或从错误文本解析模型。
- 实现与自动测试不写 live 数据；完整验证通过后才允许替换本地 Sub2API app 镜像，保留当前镜像作为回滚点，不重建 PostgreSQL/Redis。真实测试只使用 AIINPUT，CIII 只读且不得恢复。
- 上游交付使用 feature branch PR；只有发现能脱离 PR 独立追踪的问题时才创建 issue。

## 改完后是什么样

现有普通计划请求保持兼容：

```json
{"account_id":42,"model_id":"gpt-5.6-luna","cron_expression":"*/30 * * * *","enabled":true,"max_results":100,"auto_recover":true}
```

新增错误恢复计划请求示例：

```json
{"account_id":42,"model_id":"gpt-5.6-luna","model_ids":[],"trigger_mode":"error_recovery","retry_interval_minutes":5,"retry_cron_expression":null,"enabled":true,"max_results":100,"auto_recover":true}
```

`model_ids=[]` 表示全部失败模型，非空数组表示只复测选中的失败模型；`model_id` 保留为账号级错误的 fallback probe model，并由服务端规范化为首个选中模型。历史错误恢复计划没有 `model_ids` 时按全部失败模型处理。

高级模式把 `retry_interval_minutes` 设为 `null`，并写入如 `*/5 * * * *` 的 `retry_cron_expression`。测试结果新增实际执行的 `model_id`，例如：

```json
{"id":901,"plan_id":7,"model_id":"gpt-5.6-sol","status":"success","response_text":"ok","error_message":"","latency_ms":842}
```

后台测试开始时先写入同一模型的 `status=running` 结果，完成后原地更新为 `success` 或 `failed`；管理界面仅在展开结果且存在运行项时轮询刷新。超过 runner 5 分钟执行上限仍为 `running` 的历史项显示为“执行中断”，但不据此恢复模型。

错误恢复基础字段由 `192_add_scheduled_test_error_recovery.sql` 提供；模型选择 allowlist 由新增 migration `193_add_scheduled_test_error_model_ids.sql` 提供：

```sql
ALTER TABLE scheduled_test_plans
  ADD COLUMN trigger_mode VARCHAR(20) NOT NULL DEFAULT 'scheduled',
  ADD COLUMN retry_interval_minutes INT,
  ADD COLUMN retry_cron_expression VARCHAR(100);
ALTER TABLE scheduled_test_plans
  ADD CONSTRAINT chk_stp_trigger_mode CHECK (trigger_mode IN ('scheduled', 'error_recovery')),
  ADD CONSTRAINT chk_stp_recovery_schedule CHECK (
    trigger_mode = 'scheduled' OR
    ((retry_interval_minutes BETWEEN 1 AND 1440 AND retry_cron_expression IS NULL) OR
     (retry_interval_minutes IS NULL AND NULLIF(BTRIM(retry_cron_expression), '') IS NOT NULL))
  );
ALTER TABLE scheduled_test_results ADD COLUMN model_id VARCHAR(100);
UPDATE scheduled_test_results r SET model_id = p.model_id FROM scheduled_test_plans p WHERE p.id = r.plan_id;
ALTER TABLE scheduled_test_results ALTER COLUMN model_id SET NOT NULL;
CREATE UNIQUE INDEX uq_stp_account_error_recovery ON scheduled_test_plans(account_id) WHERE trigger_mode = 'error_recovery';
CREATE INDEX idx_str_plan_model_created ON scheduled_test_results(plan_id, model_id, created_at DESC);
```

```sql
ALTER TABLE scheduled_test_plans
  ADD COLUMN model_ids TEXT[] NOT NULL DEFAULT '{}';
```

新增字段由 plan API 写入、runner/repository 读取；`result.model_id` 由测试执行写入，所有仍失败的模型共享计划的下一次重试时间，但分别测试和清除。服务端必须校验错误恢复计划只能在“有效分钟间隔”和“有效 Cron”中二选一。历史 plan 通过默认值保持 `scheduled`，历史 result 从所属 plan 回填模型；错误恢复计划的 `max_results` 按 `plan_id + model_id` 分别保留，普通计划继续按 plan 保留。

## 现状与任务 0

- 2026-08-01 实测：runner 每分钟扫描、并发上限 10；手工测试成功调用无 `model_id` 的账号级恢复，`ClearModelRateLimits` 会删除整个 `model_rate_limits`。
- 基线：`go test ./internal/service ./internal/handler/admin -count=1` 通过；`pnpm --dir frontend exec vitest run src/components/account/__tests__/AccountStatusIndicator.spec.ts` 为 4/4；`pnpm --dir frontend run typecheck` 与 `pnpm --dir frontend run build` 通过。
- 开工先复跑上述命令并确认 migration 192 不存在；任一不符即停止受影响任务，把证据置顶写入 `BLOCKED.md`。核对后先写不超过 10 行的目标、顺序和最大风险到 `PROGRESS.md`。

## 任务 1：建立按模型恢复合同

- 目标：成功测试 `sol` 时只原子删除 `model_rate_limits.sol`，保留 `luna`、`terra`、`AICredits`；账号级自动状态可同时恢复但不得连带清其他模型。
- 顺序：先补 repository/service 的失败测试，再实现带 `model_id` 的 targeted recovery，并让手工测试与后台测试共用它。
- 死规矩：JSON key 删除、scheduler outbox/缓存刷新必须走账号 repository owner；否则 DB 与调度快照会分叉。
- 验收：`go test ./internal/service ./internal/repository ./internal/handler/admin -count=1`；全部通过且新增用例覆盖“只删成功模型、失败不删、兄弟模型保留、AICredits 保留”。
- 反向验证：临时把 targeted clear 改回全量 clear，确认兄弟模型保留用例失败；还原后全绿。

## 任务 2：只在自动错误存在时独立重试

- 目标：一个账号的 3 个活动模型错误产生 3 个独立测试目标；成功目标停止，失败目标按分钟或 Cron 继续，健康状态执行 0 次。
- 顺序：在任务 1 的精确恢复后扩展 plan schema、请求校验、结果 `model_id` 和 runner；复用计划的下一次运行时间，不建第二套调度器。
- 死规矩：只读取结构化 `model_rate_limits` 和现有账号级自动状态，不解析 reason/message；手工状态及 `AICredits` 必须过滤。
- 验收：`go test ./internal/service ./internal/repository ./internal/handler/admin -count=1`；新增 runner 用例机器判定 3 个目标、独立 next run、无重叠执行、健康/手工状态 0 次。
- 时间语义：分钟间隔从本轮启动时间计算；慢请求不得把固定间隔额外拖长一个完整周期，内部错误也必须把已创建的 `running` 结果更新为 `failed`。
- 反向验证：移除健康状态过滤，确认“健康账号 0 次”用例失败；还原后全绿。

## 任务 3：交付简单设置界面

- 目标：计划表单提供“定时测试 / 错误恢复”；错误恢复默认显示 5 分钟输入、自动恢复和全部失败模型，探测模型可多选，只有打开高级模式才显示 Cron，并展示每条结果的实际模型。错误恢复旁提供问号说明边界。
- 顺序：后端合同稳定后更新 API types、表单、列表和中英文文案，避免前端先猜 payload。
- 死规矩：普通模式不得要求 Cron；高级 Cron 与分钟输入不得同时提交；现有定时计划编辑行为保持不变。
- 验收：`pnpm --dir frontend run typecheck && pnpm --dir frontend exec vitest run src/components/admin/account/__tests__/ScheduledTestsPanel.spec.ts && pnpm --dir frontend run build`；组件测试覆盖默认 5 分钟、自动恢复默认勾选、默认全选、子集 payload、问号说明、展开高级、互斥 payload、旧计划回显。
- 运行可见性：结果展开后显示模型级“执行中”，完成后自动刷新为最终状态；超过 5 分钟的遗留运行项显示“执行中断”，关闭面板或没有运行项时停止轮询。
- 反向验证：临时恢复“Cron 必填”，确认默认错误恢复提交用例失败；还原后全绿。使用本地测试数据打开账号定时测试面板，确认 console 无错误，并按 selector 截取受影响面板的桌面与移动端截图作为证据。

## 任务 4：安全部署并提交上游

- 目标：先备份当前 app 镜像引用和 Compose 配置摘要，再只替换 `sub2api` app 容器；health、登录页、AIINPUT 测试与错误恢复 UI 全部通过后提交 PR。
- 顺序：完整测试与独立 review 通过后构建本地镜像；迁移自动执行并验活，PostgreSQL/Redis 容器和数据卷保持不变。
- 死规矩：不得对 CIII 发测试或恢复请求；任一 health、migration、console 或 AIINPUT 检查失败立即回滚 app 镜像并停止 PR。
- 验收：`docker compose -f /Users/admin/sub2api/docker-compose.yml ps`、`curl -fsS http://127.0.0.1:8080/health` 与浏览器 AIINPUT 测试成功；截图及容器状态写入 `PROGRESS.md`。

## 规矩

- 不许 skip/todo、删测试、放宽断言、mock 被测恢复 owner、修改验收命令或加 `|| true`；测试数量不得低于开工基线。
- migration 只能新增，不能改历史文件；不得清洗 live `accounts.extra`。同一验收连续失败 3 次即停止该项；结果比基线差就撤销本任务新增改动并如实记录，不以扩大范围补洞。
- `BLOCKED.md` 随交付提交，空也写“无”。未经最新用户授权不得部署或创建 PR；当前授权仅涵盖本地 Sub2API 与该功能的上游 PR。

## 完成条件

- 固定样本 `luna=失败、sol=成功、terra=失败、AICredits=存在` 跑一轮后，只消失 `sol`；5 分钟后只重试 `luna/terra`，健康账号和手工禁用账号请求数为 0。
- 旧定时计划无需迁移操作且行为不变；后端 broad tests、前端 typecheck/build/组件测试全绿，UI 截图无重叠且 console 无错误。
- 本地部署后 health、AIINPUT 与 UI 通过，CIII 状态未被修改；PR 只包含本功能、测试与 Spec。每条验收必须提交实际命令、红→绿反向证据和证据路径；同一验收连败 3 次则停止并列出缺口。

## Goal

- 验收摘要: 用 `luna/sol/terra + AICredits` 固定样本跑通自动错误发现、按模型复测到精确恢复；看到只清成功模型、失败模型继续重试、健康及手工状态零请求才算通过，且不得写 live 数据。
- 执行依据: `docs/superpowers/specs/2026-08-01-model-error-auto-recovery.md` 的任务 0 → 4；任务级 RED/GREEN 与反向验证只按正文执行。
- 真实验收: 固定 4-key 账号样本｜两轮 runner 到精确恢复｜`go test ./internal/service ./internal/repository ./internal/handler/admin -count=1`｜成功=目标独立且只删 `sol`｜证据=`PROGRESS.md`。
- 真实验收: 错误恢复设置面板｜默认分钟与高级 Cron 互斥｜`pnpm --dir frontend run typecheck && pnpm --dir frontend exec vitest run src/components/admin/account/__tests__/ScheduledTestsPanel.spec.ts && pnpm --dir frontend run build`｜成功=全绿并产出桌面/移动截图｜证据=`PROGRESS.md` 与截图路径。
- 非回归: `go test ./internal/service ./internal/handler/admin -count=1 && pnpm --dir frontend exec vitest run src/components/account/__tests__/AccountStatusIndicator.spec.ts`。
- 边界: 只改白名单路径；完整验证后仅替换本地 app 容器，真实测试只用 AIINPUT，CIII 只读；不满足 schema/基线时写 `BLOCKED.md` 并停止受影响任务。
