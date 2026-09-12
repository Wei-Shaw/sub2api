# 提示词记录：功能、可靠性与性能审计

审计日期：2026年9月12日。审计对象为当前工作区文件，包括审计开始前已经存在的未提交修改。Git 基线为 `705c10458bbf197b8de342d228d2de7397cce341`，不能仅凭该提交复原本次被审计代码。

本轮按用户编辑后保留的 F02 至 F11 十项内容完成优化，其中三项为 P1，七项为 P2。修改保留在工作区，尚未提交或部署。下文的原始问题描述和首次审计数据作为修复前证据保留，当前实现以本节的整改结果为准。

## 优化结果

| 编号 | 当前结果 |
| --- | --- |
| F02 | 记录准备使用独立的元数据提取路径，不生成安全审计预览或扫描文本，截取文本时避免全文符文数组。关闭正文保存时，入队前仅计算原始请求字节的 SHA-256，不复制或解析正文；独立安全审计的检测路径保持原有语义。 |
| F03 | 复制前申请容量，单条记录内容预算为 16 MiB，请求、响应、等待关联任务共用 64 MiB 预算。关闭的内容不进入记录副本，响应字符串在取得预算后复制，避免短字符串引用整个大响应。统计区分请求与响应丢弃、保存失败，并提供当前占用字节。 |
| F04 | 响应先登记在有容量限制的等待集合中，请求入库成功后才提交响应工作线程，失败则释放对应等待内容。等待关联不会占用响应工作线程，也没有原来的五秒关联截止。 |
| F05 | 记录服务增加停止接收、请求排空、响应排空和后台清理退出流程，接入总服务关闭；调用方的关闭期限到期时取消数据库操作并返回超时。 |
| F06 | 新增 0 至 3650 天保留配置，默认 0 表示永久保留，仅影响新记录。到期记录不再出现在列表和详情中，后台每分钟至多清理十批、每批最多一千条。新增到期索引与清理统计。新记录正文只保存 request_body，详情按需提取 prompt_text，并兼容历史记录已有文本。 |
| F07 | 支持带 Windows 或 Unix 路径、空行和 CRLF 换行的 AGENTS.md 标题，保留 Codex 身份、完整区块、引用文字和错误标题的检查。 |
| F08 | 捕获被截断的 JSON 时尝试恢复已有文本前缀；解析失败或内容丢失时保存明确的不完整状态。SSE 扫描器容量随有界捕获体调整，检查扫描错误，损坏事件也会标记不完整。客户端实际收到的响应保持完整。 |
| F09 | 前端改用 created_at、id 游标分页，每页最多查询所需条数加一，不执行精确总数查询。原分页接口继续兼容旧客户端。模型保留子串筛选，新增 pg_trgm GIN 索引。 |
| F10 | 采用文档允许的“明确范围”方案，配置说明与 WebSocket 详情都明确当前只保存各轮请求。HTTP JSON/SSE 响应保存受到支持；本轮没有新增 WebSocket 响应帧采集。 |
| F11 | 取消信号传递到 HTTP 客户端，成功、失败分支检查当前请求身份；过时失败不会污染新列表，筛选和刷新会重置游标。 |

### 本轮性能对照

与首次审计使用相同的 Docker、Go 1.27.0、合成输入和每组三次的准备基准。以下是后台准备时间和累计分配量，不代表用户请求延迟或整个应用的内存峰值。

| 合成正文与开关 | 修复前耗时 | 修复后耗时 | 修复前分配 | 修复后分配 |
| --- | --- | --- | --- | --- |
| 1 MiB，保存开启 | 168.5 毫秒 | 6.35 毫秒 | 45.3 MiB | 9.3 MiB |
| 8 MiB，保存开启 | 1362.2 毫秒 | 52.60 毫秒 | 360.3 MiB | 72.3 MiB |
| 1 MiB，准备函数关闭保存 | 86.6 毫秒 | 3.10 毫秒 | 24.2 MiB | 6.0 MiB |
| 8 MiB，准备函数关闭保存 | 703.6 毫秒 | 25.12 毫秒 | 192.2 MiB | 48.0 MiB |

上表关闭保存的对照仍直接调用准备函数，便于保持旧基准输入一致；实际 RecordPrompt 关闭正文时会提前计算字节哈希并跳过解析，该入口不等同于这两行数据。新的饱和队列回归基准通过占满预留名额制造饱和，丢弃 1 MiB 请求的分配约为 3.4 KiB，包含低样本次数下的日志成本；修复前约为 1.01 MiB，测试没有将“未复制正文”表述为“零分配”。

独立 PostgreSQL 中保留六条兼容性测试记录并新增二十万条同分布合成记录：模型子串查询确认使用新增 GIN 索引，计数约 28.94 毫秒；深游标查询扫描 21 行，约 0.076 毫秒。计数查询仅用于检查索引，新页面实际不执行该查询。缓存和少量样本差异限制了耗时的可比性，不据此承诺固定加速倍数。

证据：[本轮基准日志](D:/project/ai/git/sub2api-new-ui/deploy/tests/prompt-records-review-2026-09-12/optimization-benchmark.log)、[本轮查询计划](D:/project/ai/git/sub2api-new-ui/deploy/tests/prompt-records-review-2026-09-12/optimization-query-plans.log)。

### 本轮验证与运行边界

- Docker 中的安全审计、网关处理器、路由和迁移四个 Go 包通过；新增回归覆盖就绪响应绕过慢请求、超过五秒后的回填、正常排空、关闭超时、并发提交、字节预算、保留配置、路径标题和大响应。
- 记录相关测试的 Go 竞态检测通过；清理批次上限及大 SSE 事件的补充回归通过。
- 独立 PostgreSQL 实测通过 239 至 243 号迁移、相同时间戳分页、翻页期间插入、模型筛选、到期不可见、分批删除及历史正文兼容性。使用专用测试数据库，没有向业务数据库写入数据。
- 前端相关 45 项测试通过，包含取消信号传递、过时失败隔离、游标历史、保留期限保存和 WebSocket 范围说明；生产构建、类型检查及多语言完整性检查通过。
- 在 Docker 内运行 Chromium，以 320、375、768 和 1440 像素宽度验证合成数据页面、前后翻页、保留期限保存和 WebSocket 详情；未发现页面横向溢出，并已检查截图。测试预览使用现有简约主题，并关闭测试账号的首次使用引导。
- 16 MiB 和 64 MiB 是记录服务持有内容的预算，不是整个进程内存上限；解析过程临时分配、HTTP 捕获缓冲和其他业务内存不在此数值中。队列仍为内存队列，突然断电或进程崩溃不提供恢复保证。
- 新增 242、243 号迁移。242 需要数据库可以安装 pg_trgm 扩展；243 采用非事务并发索引迁移，避免普通索引构建阻塞记录写入。尚未在业务服务执行这些迁移。
- 旧记录的空到期时间不会被追溯改写；更改保留天数只影响之后接收的记录。单条 HTTP 捕获仍有 2 MiB 原始响应上限，保存文本上限为 256 KiB；超出捕获范围的文本不能复原，会明确标记不完整。

验证日志：[后端](D:/project/ai/git/sub2api-new-ui/deploy/tests/prompt-records-review-2026-09-12/optimization-backend.log)、[竞态检测](D:/project/ai/git/sub2api-new-ui/deploy/tests/prompt-records-review-2026-09-12/optimization-race.log)、[PostgreSQL](D:/project/ai/git/sub2api-new-ui/deploy/tests/prompt-records-review-2026-09-12/optimization-postgres.log)、[前端](D:/project/ai/git/sub2api-new-ui/deploy/tests/prompt-records-review-2026-09-12/optimization-frontend.log)、[生产构建](D:/project/ai/git/sub2api-new-ui/deploy/tests/prompt-records-review-2026-09-12/optimization-build.log)。

页面验证：[四种宽度的交互检查结果](D:/project/ai/git/sub2api-new-ui/deploy/tests/prompt-records-review-2026-09-12/optimization-ui.json)、[桌面页面截图](D:/project/ai/git/sub2api-new-ui/deploy/tests/prompt-records-review-2026-09-12/optimization-ui-1440.png)、[移动端详情截图](D:/project/ai/git/sub2api-new-ui/deploy/tests/prompt-records-review-2026-09-12/optimization-detail-320.png)。预览 HTML、浏览器脚本和 Dockerfile 保存在同一目录，均使用合成数据。

## 首次审计时核验的实现链路

| 环节 | 当前实现与核验结果 |
| --- | --- |
| 请求接入 | HTTP 网关和 Responses WebSocket 请求通过 `runSecurityAudit` 进入协调器，记录功能与安全审计模式分别控制。 |
| 内容开关 | 总开关、请求头、提示词、响应及预设过滤设置已经接通配置存储。响应开关关闭时，中间件跳过响应缓冲。 |
| 内容准备 | JSON 使用 `UseNumber` 解码；原始提示词哈希用于响应关联，保存副本负责过滤预设和已知多模态结构。 |
| 异步写入 | 请求记录使用四个工作线程，响应回填使用一个工作线程，两者拥有独立主队列和溢出队列。 |
| 数据库 | `239` 至 `241` 号迁移创建记录表及请求、响应字段。实际 PostgreSQL 测试验证了插入、去重、过滤、响应关联、详情读取和批量删除。 |
| 管理界面 | 列表只读取摘要，打开详情才加载正文；详情通过 Vue 文本插值显示内容；删除有确认步骤。 |
| 权限边界 | 路由注册在管理员认证中间件之后；本次没有发现绕过该管理员认证的代码路径。 |
| 风险字段 | `risk_status` 等字段是预留接口，记录默认标记为 `pending`，未发现更新这些记录风险结果的工作流程。源码注释明确其用于后续风险服务，不能将其视为已经完成的风险检测功能。 |

主要入口：[security_audit_helper.go](D:/project/ai/git/sub2api-new-ui/backend/internal/handler/security_audit_helper.go:72)、[coordinator.go](D:/project/ai/git/sub2api-new-ui/backend/internal/securityaudit/coordinator.go:42)、[prompt_service.go](D:/project/ai/git/sub2api-new-ui/backend/internal/securityaudit/prompt_service.go:56)、[管理员路由](D:/project/ai/git/sub2api-new-ui/backend/internal/server/routes/admin.go:139)。

## 修复前的问题与依据

以下为用户保留的十项原始发现，描述首次审计时的实现，不表示修复后仍存在相同问题。

### F02 · P1：记录准备重复生成没有使用的安全审计预览

`preparePromptRecord` 对原始文档和保存文档分别调用快照提取。快照提取会生成 `RedactedPreview`、扫描文本和完整文本，但记录表并不使用脱敏预览。预览函数对完整文本执行五类正则，再通过 `[]rune` 转换截断。关闭提示词保存时，原始快照仍然先完成上述工作。

1 MiB 合成文本的准备耗时约 169 毫秒，累计分配约 45.3 MiB；8 MiB 文本约 1.36 秒，累计分配约 360.3 MiB。性能剖析中，正则匹配的累计 CPU 采样占比约为 84.6%，`TrimRunes` 占分配采样约 32.7%。这证明当前主要成本来自全文预览及文本处理，不能仅以“JSON 只解码一次”判断优化已经完成。

建议给记录功能建立只计算身份、必要元数据和保存正文的路径；原始身份计算不生成脱敏预览。关闭正文保存时提前跳过正文构造。截断使用有界 UTF-8 遍历，避免把完整文本转换为符文数组。独立安全审计仍保留其原有检测语义。

代码：[两次快照提取](D:/project/ai/git/sub2api-new-ui/backend/internal/securityaudit/prompt_record_prepare.go:78)、[预览构造](D:/project/ai/git/sub2api-new-ui/backend/internal/securityaudit/prompt_snapshot.go:90)、[全文正则](D:/project/ai/git/sub2api-new-ui/backend/internal/securityaudit/prompt_snapshot.go:586)、[字符转换](D:/project/ai/git/sub2api-new-ui/backend/internal/securityaudit/prompt_snapshot.go:660)。

### F03 · P1：队列没有字节预算，队列已满时仍先复制请求体

请求主队列容量为 256，溢出队列容量为 2048。`RecordPrompt` 在尝试入队前调用 `req.Clone()`，完整复制请求体；请求是否能够进入队列，要在复制之后才能确定。队列没有累计字节数限制，也没有记录专用的单条请求大小预算。

确定的容量关系是：2304 条各为 1 MiB 的请求，仅排队中的原始请求体副本就需要 2304 MiB，尚未计入响应、解析树及其他应用内存。饱和队列基准也确认，每丢弃一个 1 MiB 请求，仍会累计分配约 1.01 MiB。这里的容量计算不是已经发生的进程内存峰值，本次没有执行内存耗尽测试。

建议先用非阻塞方式取得条数和字节配额，再复制请求；设置单条预算和全队列字节预算，超限直接记录丢弃原因。请求头、正文关闭时也应避免复制不需要保留的数据。

代码：[队列容量](D:/project/ai/git/sub2api-new-ui/backend/internal/securityaudit/prompt_record.go:17)、[先复制再排队](D:/project/ai/git/sub2api-new-ui/backend/internal/securityaudit/prompt_record.go:403)、[完整复制](D:/project/ai/git/sub2api-new-ui/backend/internal/securityaudit/prompt_types.go:103)。复现：`BenchmarkAuditSaturatedQueueClone`。

### F04 · P1：响应等待入库超过五秒后永久丢弃，并阻塞其他响应

响应工作线程只有一个。该线程取得任务后，先等待相应请求记录完成入库，最长五秒。在此期间，已经具备关联标识的其他响应也不能处理。超过五秒后，该响应直接计为失败并返回；请求随后成功入库，不会重新触发已经丢弃的响应任务。

两个复现用例分别确认了已就绪响应被前一个任务阻塞，以及五秒到期后请求才完成时响应仍然没有回填。当前十次更新重试只覆盖“已经获得成功入库标识但更新没有匹配行”的情形，没有覆盖上述等待超时。

建议请求入库完成后再提交可执行的响应更新，或采用有时间预算的延迟重试队列；让尚未具备关联标识的任务释放工作线程。根据明确的保存时限记录最终失败状态，避免将暂时排队直接判成永久失败。

代码：[单响应工作线程](D:/project/ai/git/sub2api-new-ui/backend/internal/securityaudit/prompt_record.go:366)、[等待与返回](D:/project/ai/git/sub2api-new-ui/backend/internal/securityaudit/prompt_record.go:494)、[五秒等待](D:/project/ai/git/sub2api-new-ui/backend/internal/securityaudit/prompt_record_prepare.go:59)。复现：`TestAuditObservationResponseQueueHeadOfLineBlocking`、`TestAuditObservationLateRequestLosesResponse`。

### F05 · P2：服务关闭流程没有等待提示词记录完成

`PromptService.Shutdown` 等待安全审计工作线程及安全审计入队任务，但未调用记录服务的停止或排空逻辑。记录服务自身也没有关闭接口，其工作线程永久等待通道消息。复现中，一个记录写入仍被阻塞时，服务关闭已经返回成功。

建议增加停止接收、等待请求记录完成、等待响应回填完成的关闭流程，并设置整体超时。正常发布或重启至少应等待已接收记录；是否进一步支持崩溃恢复，需要明确持久性要求。

代码：[关闭流程](D:/project/ai/git/sub2api-new-ui/backend/internal/securityaudit/prompt_service.go:219)、[记录工作线程](D:/project/ai/git/sub2api-new-ui/backend/internal/securityaudit/prompt_record.go:372)。复现：`TestAuditObservationShutdownDoesNotDrainRecords`。

### F06 · P2：没有记录保留期限和自动清理流程

表中存在 `expires_at`，但当前记录构造没有设置该字段，也没有发现针对 `prompt_records` 的定时过期删除。读取操作不排除过期记录，删除入口仅支持按 ID 单条或批量删除。本地现有 924 条记录的过期时间全部为空，表及索引总占用约 22 MB。

这是长期运行能力缺口，不表示当前磁盘已经不足。建议增加可配置保留期限、对应时间索引、分批清理和积压监控；对于请求体与提取文本同时保存的情况，先确定详情的唯一正文来源，再减少重复存储。

代码：[记录构造](D:/project/ai/git/sub2api-new-ui/backend/internal/securityaudit/prompt_record.go:457)、[过期字段](D:/project/ai/git/sub2api-new-ui/backend/migrations/239_prompt_records.sql:25)。

### F07 · P2：带路径的 AGENTS.md 指令不会被过滤

Codex 规则要求精确匹配 `# AGENTS.md instructions\n`。实际的 `# AGENTS.md instructions for D:\sample\project` 不符合这个前缀。即使已识别到 `You are Codex`，且预设过滤与 AGENT 子开关全部开启，带路径的指令块仍会进入保存正文。

建议依据实际标题格式匹配带路径和不带路径的完整指令区块，同时保留身份检查、闭合边界和引用文本反例。不能通过直接删除任意 AGENTS.md 字样替代结构识别。

代码：[精确前缀](D:/project/ai/git/sub2api-new-ui/backend/internal/securityaudit/prompt_preset_filter.go:14)、[前缀判断](D:/project/ai/git/sub2api-new-ui/backend/internal/securityaudit/prompt_preset_filter.go:197)。复现：`TestAuditObservationAgentPresetWithPath`。

### F08 · P2：大响应会完全漏记或被错误标记为完整

确认了两个独立触发条件：第一，非流式 JSON 超过 2 MiB 捕获上限，截断后的 JSON 无法解析，提取结果为未识别，中间件直接跳过整条响应；第二，SSE 中单行超过扫描器 1 MiB 上限时，扫描提前终止，但代码没有检查 `scanner.Err()`，已经提取的前半段仍返回 `truncated=false`。

建议针对响应协议增量提取需要保留的文本，并将缓冲截断、解析失败和扫描错误转换为明确的保存状态。数据已经不完整时，不能继续显示为完整响应。

代码：[捕获上限](D:/project/ai/git/sub2api-new-ui/backend/internal/handler/security_audit_helper.go:162)、[未识别时跳过](D:/project/ai/git/sub2api-new-ui/backend/internal/handler/security_audit_helper.go:205)、[JSON 解析](D:/project/ai/git/sub2api-new-ui/backend/internal/securityaudit/prompt_response.go:32)、[SSE 扫描](D:/project/ai/git/sub2api-new-ui/backend/internal/securityaudit/prompt_response.go:45)。复现：`TestAuditObservationSSEScannerFailureUnmarked`、`TestAuditObservationTruncatedJSONResponseLost`。

### F09 · P2：精确计数、模型模糊搜索和深分页成本随数据增长

每次列表请求先执行精确 `COUNT(*)`，再执行 `LIMIT/OFFSET`；模型筛选使用 `ILIKE '%关键词%'`，现有迁移没有对应搜索索引。

在隔离 PostgreSQL 中生成 20 万条合成记录后，模型计数采用并行顺序扫描，耗时约 55 毫秒。偏移 10 万行的查询实际扫描 100020 行；缓存命中时执行约 19.4 毫秒。使用现有时间索引和相同位置的游标条件，只扫描 20 行，约 0.091 毫秒。首次偏移查询约 354 毫秒，包含读取缓存未命中数据的成本，不应与游标结果直接宣传为固定加速倍数。

建议优先增加基于 `created_at,id` 的游标分页，避免每次翻页计算总数；保留总数需求时考虑单独获取或缓存。模型筛选优先明确精确匹配、前缀匹配或子串匹配的产品要求，再选择匹配索引。仅增加工作线程不能解决查询扫描量。

代码：[列表查询](D:/project/ai/git/sub2api-new-ui/backend/internal/securityaudit/prompt_record.go:199)、[现有索引](D:/project/ai/git/sub2api-new-ui/backend/migrations/239_prompt_records.sql:28)。证据：[查询计划](D:/project/ai/git/sub2api-new-ui/deploy/tests/prompt-records-review-2026-09-12/query-plans.log)、[缓存命中时的对照](D:/project/ai/git/sub2api-new-ui/deploy/tests/prompt-records-review-2026-09-12/query-plans-warm.log)。

### F10 · P2：WebSocket 只有请求记录，没有对应响应保存

WebSocket 的 `first_turn` 和 `subsequent_turn` 会进入请求记录，但 HTTP 响应捕获明确跳过这两类阶段。仓库中没有另外接入 WebSocket 响应帧的记录回调。因此，“响应记录”目前只覆盖受支持的 HTTP JSON/SSE 输出。

建议为每一轮 WebSocket 输出接入轻量文本累积和同一关联标识，或者在功能说明和详情状态中清楚呈现当前协议范围。此项是代码路径确认的覆盖缺口，本次没有发起真实 WebSocket 模型调用。

代码：[WebSocket 请求接入](D:/project/ai/git/sub2api-new-ui/backend/internal/handler/openai_gateway_handler.go:2404)、[响应明确跳过](D:/project/ai/git/sub2api-new-ui/backend/internal/handler/security_audit_helper.go:202)。

### F11 · P2：前端取消请求没有传递到底层，旧错误污染新列表

列表组件创建了 `AbortController`，但 `listPromptRecords` 没有接受或传递 `signal`。成功分支检查了逻辑取消状态，异常分支却没有检查旧请求是否已经作废。复现中，新一页已经加载成功，旧请求随后失败，界面仍显示加载失败提示。

建议将取消信号传递到请求客户端，并在成功、异常和结束分支统一检查请求序号。这样既能保护界面状态，也能让已经不需要的查询尽早终止。

代码：[列表状态处理](D:/project/ai/git/sub2api-new-ui/frontend/src/features/prompt-records/PromptRecordsView.vue:604)、[API 调用](D:/project/ai/git/sub2api-new-ui/frontend/src/features/prompt-records/api.ts:79)。复现：[前端观察用例](D:/project/ai/git/sub2api-new-ui/deploy/tests/prompt-records-review-2026-09-12/frontend-probes.spec.ts)。

## 实测性能与数据规模

### 记录准备基准

环境为本地 Docker、Linux AMD64、Go 1.27.0，处理器标识为 Intel Core Ultra 7 255H。输入是合成 ASCII 文本，主基准每种情况执行三次；额外对 1 MiB 开启保存的情况执行十次并采集性能剖析。以下分配量是每次操作累计分配，不是峰值驻留内存；准备耗时发生在记录后台线程，不等于用户请求延迟。

| 合成正文 | 保存提示词 | 单次准备耗时 | 累计分配量 |
| --- | --- | --- | --- |
| 1 KiB | 开启 | 0.154 毫秒 | 约 50.5 KiB |
| 1 KiB | 关闭 | 0.103 毫秒 | 约 55.0 KiB |
| 1 MiB | 开启 | 168.5 毫秒 | 约 45.3 MiB |
| 1 MiB | 关闭 | 86.6 毫秒 | 约 24.2 MiB |
| 8 MiB | 开启 | 1362.2 毫秒 | 约 360.3 MiB |
| 8 MiB | 关闭 | 703.6 毫秒 | 约 192.2 MiB |

十次剖析基准测得 1 MiB 请求平均约 166.7 毫秒，与初测一致。三次基准不用于判断微小差异，例如 1 KiB 开关两组的分配量差异。

证据：[基准与复现日志](D:/project/ai/git/sub2api-new-ui/deploy/tests/prompt-records-review-2026-09-12/backend-probes.log)、[CPU 剖析](D:/project/ai/git/sub2api-new-ui/deploy/tests/prompt-records-review-2026-09-12/cpu-top.log)、[分配剖析](D:/project/ai/git/sub2api-new-ui/deploy/tests/prompt-records-review-2026-09-12/memory-top.log)。

### 本地业务库只读统计

| 指标 | 结果 |
| --- | --- |
| 记录总数 | 924 条 |
| 过期时间为空 | 924 条 |
| 已记录响应捕获时间 | 368 条 |
| 含 Authorization 字段 | 195 条 |
| 表及索引总占用 | 约 22 MB |
| 请求体平均逻辑长度 | 15138 字节 |
| 请求体最大逻辑长度 | 582614 字节 |
| 提示词文本平均逻辑长度 | 25192 字节 |
| 请求头平均逻辑长度 | 442 字节 |
| 响应文本平均逻辑长度 | 135 字节 |

这些记录包含历史版本与不同配置下的数据。不能将 368/924 用作当前版本的响应记录成功率，也不能用混合历史数据直接计算当前配置的存储压缩率。本次未连接远端服务器，未检查远端磁盘或运行配置。

## 首次审计的验证范围和局限

- 本地 Docker 中运行 `go test ./internal/securityaudit ./internal/handler ./internal/server/routes ./migrations -count=1 -timeout=240s`，四个包全部通过。原有依赖专用数据库或 Redis 环境变量的集成测试在未配置时仍会跳过，不能将本结果称为全量集成测试通过。
- 复用仓库根目录 Dockerfile 的 `frontend-builder` 阶段完成前端生产构建、类型检查及三项多语言完整性测试。
- 在上述前端镜像中执行记录页面七项测试及安全审计页面八项测试，十五项全部通过。
- 新增七项后端缺陷观察、两项前端缺陷观察，全部复现。观察用例通过表示缺陷仍然存在，不表示产品满足相应要求。
- 在本次新建的临时 PostgreSQL 容器中执行记录仓储的实际读写验证，并生成二十万条合成数据分析查询计划。未向业务数据库写入测试数据。
- 没有发起真实付费模型调用，没有执行浏览器手工交互或竞态检测，没有修改线上服务。未将当前工作区与正在运行的应用镜像视为同一代码版本。

## 实施与材料说明

本轮实施范围为 F02 至 F11。新增正确行为的回归断言位于后端 prompt_record_optimization_test.go、prompt_record_optimization_integration_test.go、网关响应捕获测试及前端提示词记录测试目录。

原 probes.go 和 frontend-probes.spec.ts 保留为首次审计观察材料，其中“观察用例通过”表示旧缺陷复现，不能用作本轮修复验收标准。本轮基准只复用其中的准备基准；新的饱和队列基准适配预留机制。页面现已展示请求与响应分项计数、占用字节、等待响应数量及清理情况，统计仅针对当前实例，重启后归零。

## 复核材料

- [后端观察与基准源码](D:/project/ai/git/sub2api-new-ui/deploy/tests/prompt-records-review-2026-09-12/probes.go)：通过 `overlay.json` 临时加入容器内安全审计测试包，不修改业务源码。
- [生命周期和仓储日志](D:/project/ai/git/sub2api-new-ui/deploy/tests/prompt-records-review-2026-09-12/backend-lifecycle.log)。
- [前端现有测试日志](D:/project/ai/git/sub2api-new-ui/deploy/tests/prompt-records-review-2026-09-12/frontend-tests.log)、[前端缺陷观察日志](D:/project/ai/git/sub2api-new-ui/deploy/tests/prompt-records-review-2026-09-12/frontend-probes.log)。
- [合成数据与查询计划脚本](D:/project/ai/git/sub2api-new-ui/deploy/tests/prompt-records-review-2026-09-12/query-plans.sql)。脚本仅用于空的临时审计数据库。
- `cpu.out`、`memory.out` 为合成样本的原始性能剖析文件，`profile-benchmark.log` 保留十次剖析基准结果。
- [被审计源码校验清单](D:/project/ai/git/sub2api-new-ui/deploy/tests/prompt-records-review-2026-09-12/source-hashes.json)。
