## ADDED Requirements

### Requirement: 提示词审计必须是独立且默认关闭的安全审计引擎
系统 SHALL 在现有内容审核之外提供独立的提示词审计引擎。新引擎 MUST 拥有独立配置、运行态、任务、事件和开关，并 MUST 默认关闭；现有 OpenAI Moderations 内容审核的配置、判定、关键词、Hash、邮件、自动封号、日志表和清理行为 MUST NOT 因本能力而改变。

#### Scenario: 升级后未启用新引擎
- **WHEN** 系统完成包含本能力的升级且管理员尚未保存提示词审计配置
- **THEN** 所有模型请求 MUST 继续按升级前的内容审核和转发链路执行
- **THEN** 系统 MUST NOT 创建提示词审计任务、写入提示词审计事件或调用外部 Guard

#### Scenario: 两个审计引擎同时启用
- **WHEN** 现有内容审核和新增提示词审计都已启用
- **THEN** 两个引擎 MUST 使用各自的配置与风险语义独立执行
- **THEN** 提示词审计命中 MUST NOT 自动触发现有内容审核的邮件、封号或 Hash 黑名单副作用

### Requirement: 提示词审计节点必须使用 OpenAI 兼容协议
系统 SHALL 仅支持通过 OpenAI 兼容 Chat Completions 接口调用提示词审计节点。节点配置 MUST 支持名称、Base URL、API Key、Model、超时、单片输入上限、启用状态和有序优先级；默认模型 MUST 为 `sileader/qwen3guard:0.6b`。

#### Scenario: Worker 调用已配置节点
- **WHEN** Worker 领取到可处理任务并选择一个启用节点
- **THEN** 系统 MUST 向 `{base_url}/v1/chat/completions` 发送请求
- **THEN** 请求 MUST 使用 `role=user`、`temperature=0`、确定性的输出限制和管理员配置的模型
- **THEN** 系统 MUST NOT 调用旧的 `/v1/scan/prompt` 或 `llm_guard` 专用协议

#### Scenario: 管理员保存未填写模型的节点
- **WHEN** 管理员保存一个 Base URL 有效但 Model 为空的节点
- **THEN** 系统 MUST 将节点模型归一为 `sileader/qwen3guard:0.6b`

#### Scenario: 管理员探测节点
- **WHEN** 管理员请求探测一个节点
- **THEN** 后端 MUST 使用服务端网络环境执行真实的认证与模型连通性探测
- **THEN** 响应 MUST 包含成功状态、稳定错误码、HTTP 状态、耗时、是否可重试和检查时间
- **THEN** 响应 MUST NOT 回显 API Key

### Requirement: 审计节点凭据必须受到安全保护且出站目标由管理员负责
系统 MUST 使用现有 SecretEncryptor 加密持久化节点 API Key，并 MUST 对响应体实施大小限制。节点地址及其网络目标由管理员自行配置和负责；系统 MUST NOT 按公网、私网、回环、link-local、元数据、保留地址或 DNS 解析结果阻止保存、探测和实际调用，也 MUST NOT 禁止 HTTP 或正常 HTTP 重定向。完整凭据只允许短暂存在于管理员写入请求、前端未持久化输入内存、服务端解密内存和发往 Guard 的 Authorization Header；它们以及 URL query、提示词正文 MUST NOT 出现在日志、错误响应、管理读取响应或前端持久化/调试状态中。

#### Scenario: 保存带 API Key 的节点
- **WHEN** 管理员保存一个包含 API Key 的节点
- **THEN** settings 中 MUST 只保存加密密文和是否已配置标记
- **THEN** 后续读取配置 MUST 只返回 `has_token=true` 或等价状态

#### Scenario: 保存管理员配置的内网或特殊地址
- **WHEN** Base URL 使用 HTTP(S) 且指向私网、回环、link-local、元数据、保留地址或解析到这些地址的域名
- **THEN** 系统 MUST 接受该节点配置并从服务端网络环境执行探测和实际调用
- **THEN** 系统 MUST NOT 对 DNS 结果进行地址类别拦截

#### Scenario: 节点返回重定向或超大响应
- **WHEN** Guard 返回正常 HTTP 重定向
- **THEN** 系统 MUST 使用标准 HTTP 客户端行为跟随重定向
- **WHEN** Guard 返回超过配置上限的响应体
- **THEN** 系统 MUST 将响应判定为无效或不可用

### Requirement: 系统必须按协议提取用户输入提示词快照
系统 SHALL 从目标项目所有已支持、包含用户文本的模型入口提取提示词快照。快照 MUST 包含 request ID、user ID、用户名、用户邮箱、API key ID/名称、group ID/名称、provider、endpoint、protocol、model、提示词 Hash、脱敏预览、Unicode 字符数和消息数量；文本审计 MUST 优先扫描最新用户输入，同时完整覆盖需要审计的历史用户文本。

#### Scenario: 提取 OpenAI Chat Completions 输入
- **WHEN** `/v1/chat/completions` 或等价兼容入口包含一个或多个 `role=user` 消息
- **THEN** 系统 MUST 提取用户文本内容并把最新用户输入置于扫描顺序最前
- **THEN** 系统 MUST 不把 assistant 或 tool 输出当作用户提示词主体

#### Scenario: 提取 OpenAI Responses 输入
- **WHEN** `/v1/responses` 请求使用字符串、消息数组或内容块表达用户输入
- **THEN** 系统 MUST 提取其中的用户文本并保留 Responses 协议标识

#### Scenario: 提取 Claude 和 Gemini 输入
- **WHEN** Claude Messages 或 Gemini 兼容入口包含用户角色文本
- **THEN** 系统 MUST 提取可审计文本并保留真实 protocol、endpoint 和 model

#### Scenario: 提取图像或媒体生成提示词
- **WHEN** OpenAI Images、Grok 媒体或目标项目其他生成入口包含文本 prompt
- **THEN** 新引擎 MUST 审计文本 prompt
- **THEN** 新引擎 MUST NOT 把图片二进制、base64 图片或远程图片内容发送给 Qwen3Guard
- **THEN** 图片内容审核 MUST 继续由现有内容审核引擎负责

#### Scenario: 请求没有用户文本
- **WHEN** 请求体有效但没有可审计的用户文本
- **THEN** 系统 MUST 跳过提示词任务并记录稳定的 skipped reason

### Requirement: 提示词数据库快照必须脱敏且不可恢复原文
系统 SHALL 在写入数据库前计算 SHA-256 Hash 和脱敏裁剪预览。PostgreSQL、结构化日志、管理 API 和前端 MUST NOT 保存或返回完整原始提示词；用于实际扫描的正文只允许保存在请求内存或 Redis 短 TTL 载荷中。

#### Scenario: 创建异步任务
- **WHEN** 系统为用户输入创建异步审计任务
- **THEN** `prompt_audit_jobs` MUST 保存 Hash、脱敏预览、字符数、消息数、分列的用户/API Key 展示快照和可关联请求上下文
- **THEN** 表中 MUST 不存在 raw_prompt、payload 或等价原文字段

#### Scenario: 管理员查看事件详情
- **WHEN** 管理员打开提示词审计事件详情
- **THEN** 页面和 API MUST 只展示脱敏预览、Hash、分类、结构化风险摘要、证据摘要和技术元数据
- **THEN** 任何证据片段 MUST 经过脱敏、长度限制并包含不可逆 Hash，而不是完整命中正文

### Requirement: 异步审计必须使用持久任务和短期 Redis 载荷
系统 SHALL 使用 PostgreSQL `prompt_audit_jobs` 作为任务事实源，并使用 Redis 保存默认 30 分钟 TTL 的完整扫描正文。异步任务投递 MUST 不阻塞或改变主模型请求结果。

#### Scenario: 成功投递异步任务
- **WHEN** 提示词审计处于 async_audit、请求在审计范围内且队列未满
- **THEN** 系统 MUST 先创建不可被 Worker 领取的 staging 任务
- **THEN** 系统 MUST 成功写入 Redis 载荷后再把任务发布为 queued
- **THEN** 主请求 MUST 继续进入现有网关链路

#### Scenario: Redis 载荷写入失败
- **WHEN** 数据库任务已创建但 Redis 载荷写入失败
- **THEN** 系统 MUST 将任务标记为 failed 或保持可清理的 staging 状态
- **THEN** 系统 MUST 输出 `prompt_audit.enqueue_dropped` 和稳定错误码
- **THEN** 主模型请求 MUST 不受影响

#### Scenario: 队列达到容量上限
- **WHEN** queued、retry、processing 和 staging 活跃任务达到配置容量
- **THEN** 系统 MUST 拒绝创建新的异步任务并记录 `reason=queue_full`
- **THEN** 主模型请求 MUST 继续转发

#### Scenario: 多实例同时争抢最后队列容量
- **WHEN** 多个实例并发入队且剩余容量不足以容纳全部请求
- **THEN** active count 检查与 staging INSERT MUST 在同一数据库准入锁事务中串行化
- **THEN** 已接受的 active jobs MUST NOT 超过该配置快照的 queue_capacity
- **THEN** 未获准任务 MUST 按 queue_full 或 queue_admission_busy 丢弃且不影响主请求

### Requirement: 进程内 Worker 必须可靠消费持久任务
系统 SHALL 在主服务进程内启动可配置数量的 Worker。多实例 Worker MUST 通过 PostgreSQL 原子领取任务，并为每次领取生成单调递增的 claim version fencing token；租约刷新、事件提交和终态更新 MUST 校验该 token。系统还 MUST 支持重试退避、processing 租约刷新、滞留任务回收、最大尝试次数和优雅关闭。

#### Scenario: 多 Worker 并发领取任务
- **WHEN** 多个进程或 Worker 同时寻找可执行任务
- **THEN** 每个任务 MUST 只被一个 Worker 原子领取
- **THEN** 领取过程 MUST 使用数据库行锁/条件更新或等价的无重复执行机制

#### Scenario: 已回收的旧 Worker 恢复
- **WHEN** Worker A 的 processing 租约已被回收且任务随后由 Worker B 以更高 claim version 重新领取
- **THEN** Worker A 的租约刷新、事件写入和终态更新 MUST 因 claim version 不匹配而失败
- **THEN** Worker A MUST NOT 覆盖 Worker B 的任务状态或创建重复事件

#### Scenario: 可重试节点故障
- **WHEN** Guard 返回 429、5xx、连接失败或超时且任务仍有剩余尝试次数
- **THEN** Worker MUST 将任务置为 retry 并设置有界退避的 next_attempt_at

#### Scenario: 不可重试错误或达到最大尝试次数
- **WHEN** Guard 返回认证失败、严格解析失败或任务达到最大尝试次数
- **THEN** Worker MUST 将任务标记为 failed 并保存脱敏后的稳定错误码
- **THEN** Redis 载荷 MUST 被删除或等待短 TTL 自动清理

#### Scenario: 回收滞留 processing 任务
- **WHEN** processing 任务的租约超过允许时长
- **THEN** 系统 MUST 按剩余尝试次数把任务回收到 retry 或标记 failed
- **THEN** 系统 MUST 输出可关联 job ID 的回收日志

#### Scenario: Worker 启动失败
- **WHEN** 数据库、Redis、配置或加密依赖导致 Worker 无法启动
- **THEN** 主 API MUST 继续提供非提示词审计能力
- **THEN** 运行态 MUST 显示 error/degraded 和稳定错误码，而不是显示健康

### Requirement: Qwen3Guard 返回必须被严格归一化
系统 SHALL 严格解析单一 `Safety` 行和单一 `Categories` 行，并支持 Violent、Non-violent Illegal Acts、Sexual Content or Sexual Acts、PII、Suicide & Self-Harm、Unethical Acts、Politically Sensitive Topics、Copyright Violation、Jailbreak 九类输入风险。额外非空说明、重复字段、未知 Safety 或无法解析响应 MUST 视为 invalid_response。

#### Scenario: Safe 结果
- **WHEN** Guard 返回 `Safety: Safe`
- **THEN** 归一化结果 MUST 为 pass/low/Allow

#### Scenario: Controversial 结果
- **WHEN** Guard 返回 `Safety: Controversial`
- **THEN** 默认结果 MUST 为 flag/Warn
- **THEN** 命中已启用的 Jailbreak、PII 或 Suicide & Self-Harm 时 MUST 提升为 critical/Block

#### Scenario: Unsafe 结果
- **WHEN** Guard 返回 `Safety: Unsafe` 且命中至少一个已启用类别
- **THEN** 结果 MUST 为 critical/Block

#### Scenario: Unsafe 包含未知类别
- **WHEN** Guard 返回 Unsafe 但类别未知或不可映射
- **THEN** 系统 MUST 记录 `unknown_unsafe` 并保持 Block 语义

#### Scenario: 严格响应解析失败
- **WHEN** Guard 响应缺少字段、包含重复字段、出现额外非空说明或 Safety 不在允许枚举中
- **THEN** 系统 MUST 返回 `prompt_guard_invalid_response`
- **THEN** 系统 MUST NOT 把该结果伪装为 Safe

### Requirement: 长提示词必须完整进行 Unicode 分片审计
系统 SHALL 按 Unicode rune 而不是字节对提示词分片。最新用户输入 MUST 作为优先片段，其他输入按确定顺序完整覆盖；异步任务必须在每片开始前刷新 processing 租约，并为每片开始、完成、失败及最终聚合输出不含正文的结构化日志。

#### Scenario: 输入超过节点单片上限
- **WHEN** 提示词 Unicode 字符数超过节点 input_limit
- **THEN** 系统 MUST 生成覆盖全部非空文本的连续分片
- **THEN** 任一分片 Block MUST 使聚合结果为 Block
- **THEN** 只有全部必要分片成功后才能产生 Allow

#### Scenario: 最新输入包含风险
- **WHEN** 最新用户输入位于长会话尾部并包含 Block 风险
- **THEN** 该输入 MUST 在历史文本之前接受扫描
- **THEN** 同步模式 MAY 在确认 Block 后停止后续分片，但 MUST NOT 部分放行

#### Scenario: 多分片扫描完成
- **WHEN** 一个提示词被拆成多个分片并完成聚合
- **THEN** 日志 MUST 包含 chunk_index、chunk_total、chunk_chars、input_chars、input_limit、guard endpoint、action 和 latency
- **THEN** 日志 MUST NOT 包含分片正文、脱敏前证据或内部优先级分隔符

### Requirement: 审计事件必须独立、可关联且可安全管理
系统 SHALL 把归一化结果写入 `prompt_audit_events`，并支持是否保存 Pass 事件。事件 MUST 包含请求上下文、分列的用户名/邮箱/API Key 名称快照、脱敏提示词快照、decision、risk_level、action、分类、scanner、证据、策略、节点、配置版本、分片数和耗时；管理 DTO MUST 从这些事实确定性派生结构化 `issue_summaries`，不得复制保存第二套风险事实。

#### Scenario: 风险事件被记录
- **WHEN** Worker 或同步 Guard 得到 flag/critical 结果
- **THEN** 系统 MUST 创建独立提示词审计事件
- **THEN** 事件 MUST 可通过 request_id、user_id、api_key_id、group_id 和 prompt_hash 检索

#### Scenario: Pass 事件存储关闭
- **WHEN** 结果为 pass 且 store_pass_events=false
- **THEN** 系统 MUST 完成任务但 MAY 不创建事件

#### Scenario: 同步结果写入失败
- **WHEN** 同步 Guard 已完成判定但事件持久化失败
- **THEN** 系统 MUST 输出 `prompt_guard.result_record_failed`
- **THEN** 持久化失败 MUST NOT 把已确定的 Allow 改成 Block，也 MUST NOT 撤销已确定的 Block

### Requirement: 提示词审计运行态必须反映真实依赖和处理状态
系统 SHALL 提供运行态接口，返回有效模式、期望/生效配置版本、配置加载时间与错误、Worker 心跳、队列容量与各状态数量、处理/失败统计、最近错误、节点连通性、数据库/Redis 状态和同步 Guard 指标。

#### Scenario: 管理员查询健康运行态
- **WHEN** Worker 正常心跳、数据库与 Redis 可用且至少一个节点探测成功
- **THEN** 运行态 MUST 显示 running/ok 和真实统计值

#### Scenario: Redis 不可用
- **WHEN** 提示词审计已启用但 Redis 载荷存储不可用
- **THEN** 异步运行态 MUST 显示 error 或 degraded
- **THEN** 页面 MUST NOT 仅因 Base URL 已配置而显示健康

### Requirement: 管理员必须能够查询和安全删除提示词审计事件
系统 SHALL 提供分页列表、详情、单条删除、批量 ID 删除和按筛选删除。筛选 MUST 支持 decision、risk level、endpoint、group、user、API key、request ID、prompt Hash、关键字和时间范围。

#### Scenario: 按筛选查询事件
- **WHEN** 管理员提交一个或多个受支持筛选条件
- **THEN** 系统 MUST 返回稳定排序的分页事件和总数

#### Scenario: 预览按筛选删除
- **WHEN** 管理员提交包含明确时间范围的删除筛选
- **THEN** 系统 MUST 返回 matched_count、规范化筛选摘要、snapshot_max_id、filter_hash 和绑定当前管理员且短期有效的 confirmation_token
- **THEN** 系统 MUST 不立即删除数据

#### Scenario: 确认按筛选删除
- **WHEN** 管理员提交相同筛选、有效 filter_hash、未过期 confirmation_token 和显式 confirm=true
- **THEN** 系统 MUST 只分批删除匹配且 id 不高于预览 snapshot_max_id 的事件，以及已无事件引用的孤立任务
- **THEN** 系统 MUST 清理相关 Redis 载荷并写入管理操作审计

#### Scenario: 伪造或重放其他管理员的删除确认
- **WHEN** confirmation_token 无法认证、已过期、操作者不匹配、Hash 不匹配或缺失
- **THEN** 系统 MUST 拒绝删除并返回稳定错误码
- **THEN** 客户端自行计算 filter_hash MUST NOT 绕过 delete-preview

#### Scenario: 无时间范围的大范围删除
- **WHEN** 管理员尝试按筛选删除但未提供明确时间范围
- **THEN** 系统 MUST 拒绝操作并返回稳定错误码
