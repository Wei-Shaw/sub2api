## Why

当前项目的“风控中心”只提供基于 OpenAI Moderations 的内容审核，异步观察依赖进程内队列，且没有 aicodex-api 已具备的持久任务队列、短期敏感载荷存储、Qwen3Guard 分类、同步 fail-closed 门禁和独立提示词事件工作台。直接替换或扩写现有内容审核会混淆两种风险模型，并可能改变关键词、Hash、邮件和自动封号等既有行为，因此需要以并列、默认关闭的独立能力引入。

本变更以 `/Users/mt/code/mt-ai/aicodex/aicodex-api` 当前磁盘实现为功能参考基线，把其中与目标项目实际协议入口相适配的提示词输入审计能力迁入 sub2api，同时保持现有 OpenAI 兼容接口、内容审核页面、数据库记录和错误语义不变。

## What Changes

- 新增独立的 OpenAI 兼容提示词审计引擎，审计节点通过 `{base_url}/v1/chat/completions` 调用 Qwen3Guard，并严格解析 `Safety` 与 `Categories`。
- 新增三态运行模式：关闭、异步只审计、同步审计并阻止；所有新增开关默认关闭。
- 新增 PostgreSQL 持久任务队列、Redis 短 TTL 原文载荷、进程内 Worker、重试退避、processing 租约刷新和滞留任务回收。
- 新增脱敏提示词快照、Hash、Unicode 分片、最新用户输入优先和九类 Qwen3Guard 风险分类。
- 新增逐分片安全日志、结构化风险摘要，以及用户名、邮箱、API Key 名称分列的管理员复核信息；风险摘要只使用脱敏证据。
- 新增同步 fail-closed 门禁，在账号选择、计费检查和上游调用之前完成；覆盖目标项目现有 Chat Completions、Responses、Claude Messages、Gemini、图像/媒体文本入口及 Responses WebSocket 首轮与后续轮次。
- 新增独立管理 API、运行态、审计节点探测、事件查询/详情/删除能力和“提示词审计”页面。
- 将侧栏现有“风控中心”入口组织为“安全审计”分组；保留原 `/admin/risk-control` 页面和行为，新增 `/admin/prompt-audit` 页面。
- 新增安全审计协调器，只负责给两个独立引擎分发同一份可信请求上下文和归并最终阻断结果，不合并配置、风险分类、事件表或副作用。
- 复用现有 SettingRepository、Redis Client、SecretEncryptor、管理员鉴权、管理操作审计、请求身份上下文、分页、日志和前端基础组件。
- 新增结构化日志、运行指标、路由覆盖测试、无上游副作用断言和敏感信息泄露门禁。
- 不删除、不迁移、不重命名现有 `content_moderation_logs`，不改变现有 Moderations 阈值、关键词、Hash、邮件、封号或清理策略。

## Capabilities

### New Capabilities

- `prompt-input-audit`: 定义提示词快照、异步投递、持久任务队列、OpenAI 兼容 Qwen3Guard 扫描、脱敏事件、运行态、配置和事件管理 API。
- `prompt-input-guard`: 定义同步阻止模式、跨协议入口覆盖、fail-closed 错误语义、WebSocket 每轮门禁、配置快照和无计费/无上游副作用不变量。
- `security-audit-console`: 定义安全审计导航、独立提示词审计页面、节点探测、配置保存、运行态观测、事件筛选/详情/安全删除和响应式可访问体验。

### Modified Capabilities

无。仓库当前没有已发布的 OpenSpec capability；现有内容审核行为在本变更中作为兼容基线，不修改其正式需求语义。

## Impact

- **后端模块**：新增 `backend/internal/securityaudit/` 垂直模块；现有 Handler 仅增加协调器依赖和接入调用。
- **网关入口**：机械替换现有统一内容审核调用点为安全审计协调调用，保持其位于鉴权之后、账号选择/计费/上游之前；WebSocket 保持逐轮检查。
- **管理 API**：新增 `/admin/prompt-audit/*`，复用现有管理员鉴权和管理操作审计。
- **数据库**：新增 `prompt_audit_jobs`、`prompt_audit_events` 和相应索引；配置存入现有 `settings`，API Key 加密保存；不修改现有内容审核表。
- **Redis**：新增短 TTL 提示词载荷和配置失效通知 key/channel；Redis 不可用时异步 Worker 必须显式降级或报错，不得伪装健康。
- **前端**：新增 `frontend/src/features/prompt-audit/`，少量修改路由、侧栏和 i18n；原 `RiskControlView.vue` 业务逻辑保持不变。
- **兼容性**：没有外部 API breaking change；新能力默认关闭。只有管理员显式开启同步阻止后，适用请求才可能新增 403/503 或 WebSocket 4403/1013 响应。
- **安全与隐私**：完整提示词只允许存在于请求内存和 Redis 短 TTL 载荷，不得进入 PostgreSQL、日志、管理 API、前端状态或错误响应；审计节点凭据必须使用现有 SecretEncryptor 加密。
- **实施基线风险**：参考仓库当前 `yjb` 分支包含未提交的同步阻止相关改动。开始编码前必须固定源 commit/tag 或保存可审计 diff，避免“完整迁移”范围漂移。

## Execution References

- `source-baseline.md`：源仓库状态、dirty 文件和实施前冻结门禁。
- `source-feature-map.md`：AICodex 功能到目标 Requirement、代码位置和证据的逐项映射。
- `implementation-guide.md`：按文件实施顺序、时序、状态机、API/路由矩阵和常见错误。
- `verification.md`：35 条 Requirement 的证据矩阵、协议测试、泄露门禁、灰度阈值和回滚手册。
