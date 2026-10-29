## 1. 固定源基线与实施边界

- [x] 1.1 记录 aicodex-api 参考仓库的绝对路径、分支、HEAD commit、`git status --short` 和 `git diff --stat`，写入本 change 的 `source-baseline.md`
- [x] 1.2 为参考仓库未提交的 Prompt Audit/Prompt Guard 文件生成只读 patch 或固定到专用 commit/tag，并在 `source-baseline.md` 中记录校验和
- [x] 1.3 复核并维护 `source-feature-map.md` 的“源功能 → OpenSpec Requirement → 目标代码 → 目标测试”追踪表，覆盖异步、同步、HTTP、SSE、WS、配置、风险摘要、身份展示、事件、运行态和页面
- [x] 1.4 确认设计中的五个 Open Questions，并把已确认结论回写 `design.md`，未确认项不得在实现中自行漂移
- [x] 1.5 运行并保存现有基线信号：`cd backend && go test ./internal/service -run ContentModeration -count=1`
- [x] 1.6 运行并保存前端现有风控页面和路由相关 Vitest，证明修改前基线通过
- [x] 1.7 将实现拆成数据基础、异步审计、控制台、同步门禁和灰度五个可独立评审的提交/PR 阶段

## 2. 建立独立模块和公共契约

- [x] 2.1 创建 `backend/internal/securityaudit/` 并按 design 的文件职责建立最小包骨架，不在现有 `content_moderation.go` 中加入 Prompt Audit 实现
- [x] 2.2 定义可信 `Request`、分列身份快照、脱敏 `PromptSnapshot`、`Decision`、`NormalizedResult`、`IssueSummary`、`RuntimeSnapshot` 和稳定枚举/错误码
- [x] 2.3 定义 ConfigStore、JobRepository、PayloadStore、PromptScanner、Clock 和 Metrics 等可注入接口，避免核心逻辑依赖包级全局变量
- [x] 2.4 实现 Coordinator 的 off/async/blocking 分支和固定阻断优先级，并用 fake engine 单测覆盖两个引擎所有组合
- [x] 2.5 证明 Coordinator 不转换现有 Moderations 分类、不触发其额外副作用、不写入两个引擎的业务表
- [x] 2.6 为 PromptService 实现显式 `Start(ctx)`、`Shutdown(ctx)` 生命周期，禁止构造函数启动不可控 goroutine
- [x] 2.7 增加 Wire provider 和应用启动/停止接线，并保证 Prompt Worker 启动失败不会阻止主 API 提供非审计能力

## 3. 创建数据库迁移和 Repository

- [x] 3.1 基于实施时最大迁移序号新增不可变 SQL migration，创建 `prompt_audit_jobs` 和 `prompt_audit_events`
- [x] 3.2 为 jobs 添加 staging/queued/processing/retry/done/failed 状态字段、递增 claim_version fencing token、租约、尝试次数、配置版本、执行模式、用户名/邮箱/API Key 名称和请求快照列
- [x] 3.3 为 events 添加分列身份快照、脱敏提示词快照、decision/risk/action、JSONB scanner 数据、节点/策略/版本、分片数和耗时列
- [x] 3.4 添加 jobs 的调度、request、user、API key、group、Hash、时间索引，并检查索引名不与现有 schema 冲突
- [x] 3.5 添加 events 的 job、request、decision/time、risk/time、user/API key/group/time、Hash 和时间索引
- [x] 3.6 为 events.job_id 配置 `ON DELETE CASCADE`，为 user/api_key/group 配置 `ON DELETE SET NULL`，保留快照字符串
- [x] 3.7 添加数据库约束，拒绝负 attempts/max_attempts/claim_version/prompt_length/message_count/chunk_total/latency_ms 和不支持的关键状态
- [x] 3.8 实现 JobRepository 的 staging 创建、queued 发布、原子 `FOR UPDATE SKIP LOCKED` 领取并递增 claim_version，以及所有携带 claim_version 条件的租约刷新、事件提交和 done/retry/failed 更新
- [x] 3.9 实现 staging 和 processing 滞留任务的有界批量回收
- [x] 3.10 实现 EventRepository 的创建、分页、详情、复合筛选、计数和稳定排序
- [x] 3.11 实现单条、批量 ID、同一快照下的 snapshot_max_id/filter_hash 预览、短期管理员绑定 confirmation_token 和分批筛选删除，并只删除高水位内事件及无事件引用且非 processing 的孤立 job
- [x] 3.12 添加 migration 重复执行、索引存在、外键行为、原子领取并发、旧 Worker fencing 和 Repository 集成测试
- [x] 3.13 添加 schema 泄露门禁，断言两张表不存在 raw_prompt、payload、token、authorization 或等价原文/凭据列

## 4. 实现配置、凭据和多实例快照

- [x] 4.1 新增 `SettingKeyPromptAuditConfig`，实现 DefaultConfig、存储 DTO、公共 DTO 和保存请求 DTO
- [x] 4.2 实现 enabled/blocking_enabled 三态归一和 `prompt_guard_requires_audit_enabled` 校验
- [x] 4.3 实现唯一 `strategy=priority`、worker_count、queue_capacity、timeout、input_limit、group_ids 和 scanners 边界校验
- [x] 4.4 复用 SecretEncryptor 保存 endpoint token_ciphertext，并实现“保留原密文、替换、显式清除”三种写入语义
- [x] 4.5 确保公共配置只返回 has_token/token_status，任何 JSON marshal 路径都不会输出密文或明文
- [x] 4.6 实现携带 expected_config_version 的 PostgreSQL advisory-lock CAS 保存、单调 config_version、409 conflict、updated_at、updated_by 和脱敏 change_summary
- [x] 4.7 实现原子内存配置快照、最后有效版本、加载错误和有界 TTL 刷新
- [x] 4.8 实现 Redis `sub2api:prompt_guard:config:invalidate` publish/subscribe 和 publish 失败降级日志
- [x] 4.9 添加配置加密往返、旧字段缺失、非法组合、边界值、两管理员/两实例并发 CAS、多实例失效和 Redis 不可用测试
- [x] 4.10 添加 canary secret 测试，断言 settings 公共读取、日志和错误均不出现节点 API Key

## 5. 实现安全出站 Client 和节点探测

- [x] 5.1 实现统一 Base URL 规范化并固定调用 `{base}/v1/chat/completions`
- [x] 5.2 实现 scheme、userinfo、query、fragment、metadata host、link-local、multicast、unspecified 和保留地址校验
- [x] 5.3 实现公网必须 HTTPS、本机/显式私网 HTTP 例外和 DNS 解析后 DialContext IP 二次校验
- [x] 5.4 创建独立 HTTP Transport，配置 Dial/TLS/ResponseHeader timeout、连接池和 256 KiB 响应上限
- [x] 5.5 禁止 HTTP 重定向，并确保每个错误只暴露 endpoint ID 和稳定错误码
- [x] 5.6 实现节点 `/models` 就绪检查以及必要时的真实 Qwen3Guard fallback probe
- [x] 5.7 实现探测结果 DTO：ok/status/error_code/message/latency_ms/http_status/retryable/checked_at/token_applied
- [x] 5.8 添加 SSRF、DNS rebinding、重定向、超大响应、认证失败、429、5xx、连接失败和超时测试

## 6. 实现协议快照、脱敏和分片

- [x] 6.1 实现 OpenAI Chat Completions 用户消息提取，支持字符串和文本内容块并把最新用户输入置于首段
- [x] 6.2 实现 OpenAI Responses input 字符串、消息数组和内容块提取
- [x] 6.3 实现 Claude Messages 用户文本块提取
- [x] 6.4 实现 Gemini contents/parts 用户文本提取
- [x] 6.5 实现 OpenAI Images、Grok 媒体和目标项目其他生成请求的纯文本 prompt 提取，明确排除图片/base64 数据
- [x] 6.6 实现 Responses WebSocket `response.create` 帧提取并支持 first_turn/subsequent_turn stage
- [x] 6.7 实现 SHA-256、消息数、Unicode 字符数和确定性 metadata 计算
- [x] 6.8 实现凭据、Bearer、邮箱、电话及常见敏感模式的预览脱敏和 rune 安全裁剪
- [x] 6.9 实现最新输入优先的 scan text 组合和按 rune 的 input_limit 分片
- [x] 6.10 添加中文、emoji、组合字符、超长文本、空输入、混合 content block、媒体 payload 和最新输入优先测试
- [x] 6.11 添加 canary prompt 测试，断言预览不可恢复完整输入且 Hash 与实际 scan text 一致

## 7. 实现 Qwen3Guard 严格解析和结果聚合

- [x] 7.1 定义九类 Qwen3Guard 官方输入类别和目标项目展示标签
- [x] 7.2 构建 OpenAI Chat Completions 请求，固定 role=user、temperature=0、max_tokens=64、seed=42
- [x] 7.3 实现 choices/message/content 提取，兼容目标审计节点允许的最小合法响应形态
- [x] 7.4 实现严格单 Safety 行、单 Categories 行、无额外非空说明解析
- [x] 7.5 实现类别别名归一、未知类别保留和启用类别过滤
- [x] 7.6 实现 Safe/Controversial/Unsafe 到 pass/flag/critical 与 Allow/Warn/Block 的确定性映射
- [x] 7.7 实现 Jailbreak、PII、Suicide & Self-Harm 的高风险 Controversial 提升规则
- [x] 7.8 实现多分片最严重结果聚合、分类/证据去重、分片 metadata 和 Block 早停
- [x] 7.9 确保只有全部必要分片成功才能 Allow，部分成功不得产生 Safe
- [x] 7.10 添加模型合法输出、重复字段、额外说明、未知 Safety、未知类别、禁用类别和多分片聚合测试
- [x] 7.11 从分类、策略和脱敏 evidence 确定性生成 IssueSummary，覆盖标题/说明/严重度/动作/score/位置/Hash，且不新增重复数据库事实列

## 8. 实现异步投递和 Worker

- [x] 8.1 实现 Prompt Audit 有效模式、risk_control_enabled、分组范围、节点可用性，以及 advisory-lock 事务内 active count + staging INSERT 的多实例严格队列容量准入
- [x] 8.2 实现 staging job → Redis SET EX 1800 → queued 的发布协议
- [x] 8.3 实现所有投递失败 reason 和 `prompt_audit.enqueue_skipped/enqueue_dropped/job_enqueued` 结构化日志
- [x] 8.4 保证异步投递复制必要请求数据并使用有界后台 context，不引用 Gin request 生命周期后的可变内存
- [x] 8.5 实现 Redis PayloadStore 的 Set/Get/Delete 和命名空间 key
- [x] 8.6 实现 Worker 轮询、活动计数、processing 租约、节点有序故障切换和任务处理
- [x] 8.7 实现 5s/30s/2m 有界退避、可重试分类和 max_attempts 终止
- [x] 8.8 实现 store_pass_events=false 时仅完成 job、不写 Pass event
- [x] 8.9 实现风险事件和 `prompt_audit.finding_recorded/processed/process_failed` 日志
- [x] 8.10 实现 Worker panic 单任务恢复、优雅停止和 shutdown timeout 日志
- [x] 8.11 添加队列满、Redis SET 失败、发布失败、进程中断、重复领取、租约刷新、滞留回收、旧 Worker claim_version 失效和重试集成测试
- [x] 8.12 证明异步模式所有失败都不改变模型请求状态、错误体和上游转发次数
- [x] 8.13 为逐分片开始/完成/失败和聚合输出稳定日志，字段只含索引、字符数、限制、节点、动作、耗时和错误码

## 9. 实现同步 Guard evaluator

- [x] 9.1 实现全局 64、每节点 16 的非阻塞 bulkhead，并允许测试注入更小容量
- [x] 9.2 实现以首个启用节点 timeout 为总 deadline，分片和节点切换共享剩余预算
- [x] 9.3 实现连接/429/5xx/超时切换下一节点，401/403/invalid_response 终止
- [x] 9.4 实现 Allow/Flag/Block/Unavailable Decision 和 allow_next_stage
- [x] 9.5 实现 prompt_guard total/allowed/flagged/blocked/unavailable/invalid/timeouts/failovers/bulkhead_full 指标
- [x] 9.6 实现同步结果轻量记录 adapter，在单事务中创建 done job 和可选 event，禁止再次扫描
- [x] 9.7 实现记录失败 `prompt_guard.result_record_failed`，并证明不改变已确定的 Allow/Block
- [x] 9.8 添加完整分片、Block 早停、最后分片失败、所有节点失败、bulkhead 满和 context cancel 测试

## 10. 接入网关并保持兼容

- [x] 10.1 在 GatewayHandler 和 OpenAIGatewayHandler 中注入 SecurityAudit Coordinator，同时保留现有 ContentModerationService 供 cyber policy 记录使用
- [x] 10.2 将 Chat Completions 现有审核调用替换为统一 `checkSecurityAudit`
- [x] 10.3 将 HTTP Responses 现有审核调用替换为统一 `checkSecurityAudit`
- [x] 10.4 将 Claude Messages 现有审核调用替换为统一 `checkSecurityAudit`
- [x] 10.5 将 Gemini 现有审核调用替换为统一 `checkSecurityAudit`
- [x] 10.6 将 OpenAI Images 和 Grok 媒体文本 prompt 审核调用替换为统一 `checkSecurityAudit`
- [x] 10.7 接入 Responses WebSocket 首个 response.create 门禁，置于用户/账号 slot、计费和上游拨号前
- [x] 10.8 接入 Responses WebSocket 后续每个 response.create 门禁，置于本轮 slot、计费和上游发送前
- [x] 10.9 为现有错误 helper 增加最小 Prompt Guard adapter：OpenAI/Claude 可选 error.code，Gemini 保留数值 code/status 并使用 google.rpc.ErrorInfo.reason，映射 blocked/unavailable/invalid_response
- [x] 10.10 确保 SSE 在 Guard 完成前没有写 response header 或首字节
- [x] 10.11 增加静态/结构测试，枚举所有现有用户文本路由并在缺少 Coordinator 接线时失败
- [x] 10.12 用账号选择、计费和上游 fake counter 证明 Block/Unavailable/Invalid 时三者调用均为 0
- [x] 10.13 回归现有 ContentModeration Block 响应优先级、文案、封号、邮件和记录语义

## 11. 实现管理 API 和管理操作审计

- [x] 11.1 创建 PromptAdminHandler 并注册 `/admin/prompt-audit` 独立路由组，复用 AdminAuth 和现有安全中间件
- [x] 11.2 实现 GET/PUT config，PUT 强制 expected_config_version 并映射 409 conflict，返回公共 DTO 并记录脱敏配置更新审计
- [x] 11.3 实现 POST endpoints/probe，支持使用已保存密文或请求中的临时 token 且绝不回显
- [x] 11.4 实现 GET runtime，聚合配置版本、Worker、DB 队列、Redis、节点连通性和 Guard 指标
- [x] 11.5 实现 GET events 和 GET events/:id，支持完整筛选、分页、用户名/邮箱/API Key 名称分列快照和派生 issue_summaries
- [x] 11.6 实现 DELETE 单事件和 POST batch-delete，并限制单批 ID 数量
- [x] 11.7 实现 delete-preview 和 delete-by-filter，强制时间范围、snapshot_max_id、canonical filter_hash、SecretEncryptor 认证的管理员绑定/5 分钟 confirmation_token 和 confirm=true
- [x] 11.8 为配置、探测和删除的成功/失败写入现有管理员操作审计，detail 使用字段 allowlist
- [x] 11.9 添加未认证、非管理员、非法 ID/时间、Hash/token/操作者/过期不匹配、预览后新事件、并发删除和敏感字段响应测试

## 12. 实现独立控制台页面

- [x] 12.1 创建 `frontend/src/features/prompt-audit/` 的 api、types、viewModel、components、PromptAuditView 和测试目录
- [x] 12.2 新增 `/admin/prompt-audit` 路由并复用 requiresAuth/requiresAdmin/requiresRiskControl guard
- [x] 12.3 将侧栏现有风控入口改为 expandOnly“安全审计”分组，保留 `/admin/risk-control` 子入口并新增 Prompt Audit 子入口
- [x] 12.4 实现配置、运行态、分组和事件的有界并行加载及独立错误状态
- [x] 12.5 实现审计池新增/编辑/启停/删除、参数对话框和真实探测进度/结果
- [x] 12.6 实现 API Key 空值保留、显式替换/清除和保存成功后清除明文 state
- [x] 12.7 实现 all/selected group 范围、分组搜索、失效分组提示和九类 scanner 选择
- [x] 12.8 实现 enabled/blocking/store pass 固定保存栏、dirty snapshot、重置和同步阻止二次确认
- [x] 12.9 实现配置版本、Worker/队列、Redis、连通性、最近错误和 Guard 指标概览
- [x] 12.10 实现事件复合筛选、时间范围、分页、行选择、用户名/邮箱/API Key 名称分列复制、模型信息和风险展示
- [x] 12.11 实现事件详情的脱敏预览、审计返回、IssueSummary 具体风险和技术信息 tabs
- [x] 12.12 实现单条、批量和按筛选 snapshot/Hash/token/确认删除流程，筛选变化后使旧 Hash 与 confirmation_token 同时失效
- [x] 12.13 新增中英文对称 i18n，并为所有输入、开关、按钮、状态和对话框提供可访问名称
- [x] 12.14 添加桌面、窄屏、键盘操作、dirty 状态、探测、模式联动、事件删除和 secret state 清理 Vitest
- [x] 12.15 回归原 RiskControlView 的路由、功能开关和关键测试，确认业务逻辑未改变

## 13. 补齐日志、指标和敏感信息门禁

- [x] 13.1 实现 design 中列出的 prompt_audit/prompt_guard 稳定事件词典和字段 allowlist helper
- [x] 13.2 为关键路径补齐 request_id、user/api-key/group、protocol、endpoint、model、config/job/event/node/version、结果、耗时和错误字段
- [x] 13.3 在同步拒绝日志中明确 upstream_dispatched=false 和 billing_preconsumed=false
- [x] 13.4 实现运行计数和延迟指标，并确保 runtime API 的字段与日志错误码使用同一词典
- [x] 13.5 添加日志捕获测试，使用 canary prompt、API Key、Authorization 和带 query URL 证明敏感内容不出现
- [x] 13.6 添加数据库/API/前端快照泄露测试，统一扫描 canary secret
- [x] 13.7 对错误消息和 last_error_message 做长度限制与脱敏，禁止保存 Guard 原始响应正文

## 14. 完成验证、质量门禁和灰度准备

- [x] 14.1 运行 `openspec validate add-openai-compatible-prompt-audit --type change --strict --no-interactive`
- [x] 14.2 运行 `cd backend && go test ./internal/securityaudit/... -count=1`
- [x] 14.3 运行 `cd backend && go test ./internal/handler/... ./internal/server/... -count=1` 并保存路由矩阵结果
- [x] 14.4 运行 `cd backend && go test -race ./internal/securityaudit/... -count=1`
- [x] 14.5 在可用 PostgreSQL/Redis 环境运行 migration、Repository、多 Worker 和配置失效集成测试
- [x] 14.6 运行 `make test-backend`，记录全量 Go test 和 golangci-lint 结果
- [x] 14.7 运行 `pnpm --dir frontend run lint:check`、`pnpm --dir frontend run typecheck` 和 Prompt Audit/RiskControl Vitest
- [x] 14.8 运行 `make build`，验证后端和前端生产构建
- [x] 14.9 执行 HTTP/SSE/WS 端到端矩阵，保存 Block/Unavailable 无账号、无计费、无上游证据
- [x] 14.10 执行敏感信息审查，检查 PostgreSQL、Redis key metadata、日志、API JSON、浏览器存储和页面截图
- [x] 14.11 在 async 模式对测试 group 记录 Guard P50/P95/P99、失败率、误报率和事件增长率基线
- [x] 14.12 定义 blocking 灰度准入阈值、告警阈值、值班检查步骤和一键关闭 blocking 的回滚手册
- [x] 14.13 更新 `verification.md`，为每条验收 Requirement 关联测试、日志、指标、SQL 或截图证据
- [x] 14.14 在实现偏离设计时先回写 proposal/design/specs/tasks，再继续编码，禁止让 OpenSpec 落后于代码

## 15. 管理员自主管理审计节点网络目标

- [x] 15.1 更新规格与设计，明确节点目标安全由管理员负责，不再实施地址类别、DNS 结果、HTTP 或重定向拦截
- [x] 15.2 移除 Base URL 私网/特殊地址限制和 DialContext DNS/IP 二次拦截，恢复标准重定向行为
- [x] 15.3 更新出站客户端测试，覆盖 HTTP、私网/特殊地址配置和重定向，并回归响应上限与凭据保护
- [x] 15.4 运行 OpenSpec 严格校验和 securityaudit 测试，并用真实内网节点探测验证

## 16. 优化审计池节点列表并重新部署

- [x] 16.1 将审计池改为紧凑、响应式的节点列表，修复开关与节点名称拥挤并强化状态、限制和操作层级
- [x] 16.2 回归 Prompt Audit 前端组件测试、类型检查和生产构建
- [x] 16.3 在 deploy 目录按现有 Compose 配置重建镜像、重启容器并验证页面和节点探测
