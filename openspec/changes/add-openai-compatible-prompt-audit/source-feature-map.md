# AICodex 源功能迁移映射

## 1. 目的

本表用于证明“完整功能都必须要有”不是一句笼统目标。每个 AICodex 当前用户可见或运行时能力都必须映射到目标 Requirement、预期代码位置和验证证据；实施中发现新源能力时，先更新本表和相关 spec/tasks，再编码。

源参考状态见 `source-baseline.md`。只读冻结包已在 detached worktree 中恢复，以下测试在恢复副本执行：

```text
cd /Users/mt/code/mt-ai/aicodex/aicodex-api/ai-gateway
go test ./internal/service/promptaudit -count=1
ok github.com/mt21625457/aicodex/internal/service/promptaudit 2.081s

go test ./internal/router ./internal/relay ./internal/gatewayadapter/transport \
  -run 'PromptGuard|PromptAudit|ConcurrencyOrder' -count=1
ok github.com/mt21625457/aicodex/internal/router 1.184s
ok github.com/mt21625457/aicodex/internal/relay 2.201s
ok github.com/mt21625457/aicodex/internal/gatewayadapter/transport 3.233s
```

这证明冻结包可恢复且源参考测试通过，但不证明目标实现已完成；目标代码和证据仍须逐行补齐。

## 2. 功能映射

| # | AICodex 当前能力与源证据 | 目标 OpenSpec 契约 | 目标主要代码 | 验证证据 |
| ---: | --- | --- | --- | --- |
| 1 | 独立 Prompt Audit 开关、默认关闭；`config.go` | prompt-input-audit：独立且默认关闭；prompt-input-guard：显式三态 | `prompt_config.go`、`coordinator.go` | A01、G01 |
| 2 | enabled + blocking_enabled 表达 off/async/blocking；`config.go`、`synchronous_guard.go` | prompt-input-guard：显式启用、即时回滚 | `prompt_config.go`、`prompt_guard.go` | G01、G12 |
| 3 | 配置持久化、版本、updated_by/change_summary；`config.go` | prompt-input-guard：版本化快照/CAS；console：可验证保存 | `prompt_config.go` | G10、C06、C10 |
| 4 | token 加密、空值保留、替换、clear；`config.go`、`config_test.go` | prompt-input-audit：凭据安全；console：池管理/保存 | `prompt_config.go`、`prompt_handler.go` | A03、C03、C06 |
| 5 | OpenAI-compatible endpoint、Qwen3Guard 默认模型；`openai_client.go` | prompt-input-audit：OpenAI 兼容节点 | `prompt_qwen3guard.go` | A02 |
| 6 | Base URL 规范化，固定 `/v1/chat/completions`；`openai_client.go` | prompt-input-audit：OpenAI 兼容节点/出站安全 | `prompt_qwen3guard.go`、`prompt_outbound_security.go` | A02、A03 |
| 7 | `/v1/models` readiness + scan fallback probe；`openai_client.go`、`probe.go` | prompt-input-audit：管理员探测；console：真实探测 | `prompt_qwen3guard.go`、`prompt_handler.go` | A02、C03 |
| 8 | probe 对话框阶段、结果、状态/耗时/错误；`PromptAuditPage.tsx` | console：完整审计池和真实探测 | `features/prompt-audit/components` | C03 |
| 9 | Guard SSRF、DNS/Dial 复检、重定向/响应上限；`outbound_security.go` | prompt-input-audit：凭据和出站地址安全 | `prompt_outbound_security.go` | A03 |
| 10 | Qwen3Guard `Safety/Categories` 解析；`qwen3guard.go` | prompt-input-audit：严格归一 | `prompt_qwen3guard.go` | A08 |
| 11 | 九类官方输入风险；`qwen3guard.go`、页面 scanner catalog | prompt-input-audit：九类；console：九类配置 | `prompt_qwen3guard.go`、前端 types/viewModel | A08、C04 |
| 12 | Safe/Controversial/Unsafe → Allow/Warn/Block；`openai_client.go`、`normalize.go` | prompt-input-audit：严格归一；guard：fail-closed | `prompt_qwen3guard.go`、`prompt_scanner.go` | A08、G06 |
| 13 | 高风险 Controversial 提升、未知 Unsafe 保持 Block；`openai_client.go` | prompt-input-audit：严格归一 | `prompt_qwen3guard.go` | A08 |
| 14 | Chat/Responses/Claude 多协议快照；`snapshot.go`、`multiprotocol.go` | prompt-input-audit：按协议提取 | `prompt_snapshot.go` | A04 |
| 15 | Gemini、图片/媒体等 transport 传递提示词上下文；gatewayadapter changes | prompt-input-audit：所有文本入口；guard：路由覆盖 | `prompt_snapshot.go`、各 Handler 薄接线 | A04、G04 |
| 16 | Responses WS 首轮和后续帧；`ws_responses.go`、顺序测试 | prompt-input-guard：每个 response.create 门禁 | `openai_gateway_handler.go` 薄接线 | G08 |
| 17 | 最新用户输入优先；`snapshot.go` | prompt-input-audit：提取/Unicode 分片 | `prompt_snapshot.go`、`prompt_scanner.go` | A04、A09 |
| 18 | rune input_limit 完整分片；`openai_client.go` | prompt-input-audit：Unicode 完整分片 | `prompt_scanner.go` | A09 |
| 19 | 多片最严重聚合、证据 metadata/去重、Block 早停；`openai_client.go` | prompt-input-audit：分片；guard：共享预算 | `prompt_scanner.go`、`prompt_guard.go` | A09、G05 |
| 20 | 每片前刷新 processing lease；`openai_client.go`、`worker.go` | prompt-input-audit：Worker/Unicode 分片 | `prompt_worker.go` | A07、A09 |
| 21 | scan_chunk_started/completed/failed/aggregated 日志；`openai_client.go` | prompt-input-audit：Unicode 分片；guard：可观测 | `prompt_logging.go`、`prompt_scanner.go` | A09、G11 |
| 22 | Prompt hash、脱敏 preview、敏感模式处理；`snapshot.go` | prompt-input-audit：不可恢复快照 | `prompt_snapshot.go` | A05 |
| 23 | 完整 scan text 使用 Redis 30 分钟 TTL；`payload_store.go` | prompt-input-audit：持久任务 + Redis TTL | `prompt_payload_store.go` | A06 |
| 24 | 异步 enqueue、范围/容量检查；`enqueue.go` | prompt-input-audit：异步持久投递 | `prompt_enqueue.go` | A06 |
| 25 | PromptAuditJob/Event 持久事实；Ent schema/store | prompt-input-audit：jobs/events | SQL migration、`prompt_repository.go` | A05、A07、A10 |
| 26 | 进程内 Worker、可配置数量、Start/Stop；`worker.go` | prompt-input-audit：可靠 Worker | `prompt_worker.go`、`prompt_module.go` | A07 |
| 27 | retry/backoff/max attempts；`worker.go` | prompt-input-audit：可靠 Worker | `prompt_worker.go` | A07 |
| 28 | processing stale reclaim；`worker.go` | prompt-input-audit：可靠 Worker | `prompt_worker.go`、Repository | A07 |
| 29 | runtime queue/Worker/DB/payload/connectivity/heartbeat；`runtime.go` | prompt-input-audit：真实运行态 | `prompt_runtime.go` | A11、C07 |
| 30 | config active/expected version 和失效通知；`config.go`、`runtime.go` | prompt-input-guard：版本化热路径快照 | `prompt_config.go`、`prompt_runtime.go` | G10、C07 |
| 31 | 同步 evaluator 不依赖 Worker；`synchronous_guard.go` | prompt-input-guard：同步门禁/结果复用 | `prompt_guard.go` | G03、G09 |
| 32 | 总 deadline、ordered failover、bulkhead；`synchronous_guard.go` | prompt-input-guard：共享预算/故障切换 | `prompt_guard.go` | G05、G06 |
| 33 | HTTP fail-closed 403/503；`prompt_guard.go`、router 接线 | prompt-input-guard：HTTP 稳定错误 | Handler helper + OpenAI/Claude code、Gemini ErrorInfo adapter | G03、G07 |
| 34 | WS 4403/1013；`ws_responses.go` | prompt-input-guard：每轮 WS 门禁 | Responses WS Handler | G08 |
| 35 | 同步结果轻量记录、不重复 Guard；`synchronous_guard.go` | prompt-input-guard：结果复用 | `prompt_guard.go`、Repository | G09 |
| 36 | Guard metrics Allow/Flag/Block/Unavailable/timeout/failover/bulkhead；`synchronous_guard.go`、`runtime.go` | prompt-input-guard：可观测；console：运行态 | `prompt_runtime.go`、metrics adapter | G11、C07 |
| 37 | 事件列表/详情、复合筛选；`store.go`、controller | prompt-input-audit：查询事件；console：列表详情 | `prompt_repository.go`、`prompt_handler.go`、前端 | A12、C08 |
| 38 | 用户名/邮箱分别展示和复制；probe-dialog change + controller/UI tests | prompt-input-audit：分列身份快照；console：复核身份 | Request/snapshot、event DTO、前端详情 | A04、A10、C08 |
| 39 | scanner evidence、Guard policy、结构化 issue summaries；`issue_summary.go` | prompt-input-audit：事件/风险摘要；console：具体风险 | `prompt_issue_summary.go`、event DTO | A10、C08 |
| 40 | 单条/批量硬删除；controller/store | prompt-input-audit：安全删除；console：防误操作 | Repository/Admin Handler/前端 | A12、C09 |
| 41 | delete preview + canonical filter hash + confirm；filter helper | prompt-input-audit：安全删除；console：防误操作 | Repository/Admin Handler/前端，增加 max_id/认证 token | A12、C09 |
| 42 | 配置、probe、删除的管理审计；controller/router tests | console：管理员操作审计 | `prompt_handler.go` + 现有 audit | C10 |
| 43 | 独立控制台、运行概览、池/策略/事件/保存栏；`PromptAuditPage.tsx` | console：独立工作区 | `frontend/src/features/prompt-audit/` | C01、C02 |
| 44 | dirty snapshot、统一保存、重置；页面/viewModel | console：工作区/可验证保存 | 前端 viewModel/page | C02、C06 |
| 45 | all/selected group、搜索、stale group；页面/config | prompt-input-audit：范围；console：范围配置 | config + 前端 selector | C04 |
| 46 | endpoint 新增/编辑/启停/删除、参数对话框；页面 | console：审计池管理 | 前端 components | C03 |
| 47 | blocking 二次确认和保存栏开关联动；页面 | console：开启风险确认 | 前端 viewModel/page | C05 |
| 48 | 事件技术/具体风险/结构化返回 tabs 和 JSON 查看；页面 | console：可复核详情 | 前端 detail components | C08 |
| 49 | 响应式、可访问状态、页面测试；redesign change | console：响应式/可访问/i18n | 前端 + i18n | C11 |
| 50 | AI 可读稳定日志和敏感字段约束；logging.go/constraints | prompt-input-guard：可观测且不泄密 | `prompt_logging.go` | G11 |

## 3. 架构适配而非逐行复制

以下差异是目标架构适配，不是功能删减：

| AICodex 实现细节 | sub2api 目标实现 | 等价性理由/门禁 |
| --- | --- | --- |
| Ent PromptAuditJob/Event | PostgreSQL migration + `database/sql` | 目标项目以 SQL migration 为 schema 事实源；字段和行为由 A05/A07/A10/A12 验证 |
| 表/对象可能带 AICodex 命名 | `prompt_audit_jobs/events` | 不复制 `aicodex_` 前缀；管理能力不变 |
| `PromptAuditConfigJSON` option | settings `prompt_audit_config` | 复用目标 SettingRepository，Public/Storage DTO 行为不变 |
| AICodex secret helper | 现有 `SecretEncryptor` | A03 canary 和加密往返证明 |
| React/Ant Design 页面 | Vue 3 既有组件体系 | C01-C11 以行为和可访问性验收，不按框架验收 |
| `/api/prompt-audit` | `/admin/prompt-audit` | 复用目标 AdminAuth/管理审计；API 能力一一对应 |
| token/channel/group 字符串 | API key/group/provider 可信 ID + 快照 | 使用目标身份域，保留查询/复核能力 |
| 6068/9068 双端口一致性 | `/v1`、root alias、`/backend-api/codex` 等目标路由一致性 | G04 以目标实际 routes 自动枚举，不复制不存在的端口拓扑 |
| 源 queued 后再写 payload 的竞态 | staging → Redis SET EX → queued | 是可靠性增强；A06/A07 证明 Worker 不提前领取 |
| 源进程内唤醒队列 + DB 事实 | PostgreSQL 原子 claim + 递增 claim_version fencing + 进程内 Worker | 支持多实例并防旧 Worker 覆盖，无功能损失；A07 并发测试证明 |
| 源 MemoryRepository | 只作为目标测试 fake，不作为生产 fallback | 生产需要持久任务；依赖失败由 A11 显示 degraded，不伪装成功 |
| `scan_url`/旧 llm_guard 协议兼容 | 只接受 Base URL + OpenAI compatible | 目标是新增 setting、无旧 Prompt Audit 配置；A02 明确禁止旧协议 |
| 源旧 strategy 迁移 | 第一版仅 `priority`，其他值拒绝 | 目标无历史 Prompt config；G01/配置测试保证确定性 |
| endpoint `weight` 兼容展示字段 | 显式数组顺序作为 priority | 源当前只允许 priority，扫描代码未使用 weight 做选择；目标去除无效歧义，故障切换能力由 G06 证明 |
| endpoint `policy_id/tenant_id` 历史兼容输入 | Qwen 结果固定 policy_id/version，event 持久化 | 当前 Qwen 请求不发送这两个 endpoint 字段；目标保留实际策略结果而不暴露无效输入 |
| 源 env 默认配置 | settings 管理页面初始化默认值 | 目标配置事实源是 settings；默认 off 和完整可配置性由 A01/C03/C06 证明 |
| 源 event API 查询时解析用户 | 事件保存用户名/邮箱/API Key 名称分列快照 | 删除主体后仍可复核；访问与保留沿用现有管理员政策 |
| 源 `issue_summaries` 由 evidence 派生 | 目标同样派生，不新增数据库列 | 防止双份风险事实漂移；A10/C08 golden 测试证明 |

## 4. 源专属能力的明确处理

以下内容不作为目标运行功能移植，但必须明确原因：

- AICodex 旧 `/v1/scan/prompt` 和 llm_guard 配置迁移：目标项目从未发布 Prompt Audit，无历史配置需要兼容；目标只实现当前 OpenAI-compatible Qwen3Guard 行为。
- AICodex Caddy/gatewaycore、6068/9068 端口和 channel dispatch：目标使用 Gin Handler、目标账号调度和目标路由 alias；以 G04/G03 证明等价接入顺序。
- AICodex React/旧 deprecated 页面：只迁移当前管理行为到 Vue 独立 feature，不同时维护两套前端。
- AICodex 产品特有 transport：目标只覆盖目标项目实际存在且可触发模型的文本入口；`implementation-guide.md` 的路由枚举是硬门禁。
- 输出审核、Redact、人工审批和申诉：当前迁移范围是用户输入 Prompt Audit/Guard，且本 change 明确列为 Non-Goals；不得把源旧 LLM Guard 的 `Redact` 兼容文案误当成当前 Qwen 输入审计功能。

如果实施评审发现上述任一项实际上在目标项目有已发布数据或用户依赖，必须把它从本节移回第 2 节，新增 Requirement/Scenario 后才能继续。

## 5. 完整性复核步骤

每次源基线或目标设计变化后执行：

1. 对源 `internal/service/promptaudit`、Prompt Audit controller/router、WS/transport 和当前前端目录重新列出文件/公开符号。
2. 对源主 spec 和所有未归档 Prompt Audit changes 提取 Requirement/Scenario。
3. 为新发现功能在第 2 节新增一行；若无目标 Requirement，先更新 specs。
4. 检查每行同时有目标代码位置和 `verification.md` ID。
5. 检查第 3/4 节每项确实是架构适配或源专属，而不是为了缩小实现范围。
6. 冻结时把最终源 commit/tag/patch SHA-256 写入 `source-baseline.md`。
7. 实现完成后把每行的计划证据替换为实际测试名/CI artifact 链接。

本表没有“以后再做”状态。除第 4 节经解释的源专属项外，第 2 节任一行没有通过证据都表示“完整迁移”未完成。
