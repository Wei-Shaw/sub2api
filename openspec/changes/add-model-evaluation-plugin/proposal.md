# 内置模型推理评测模块

## 任务理解与结论

在管理员账号和分组页面增加“推理评测”入口。按用户选定的内置模块形态实现，选好参数后点击“开始评测”直接运行，无需开启全局模块。使用现有 Go / Vue 技术栈，不依赖本地 Codex CLI，不新增第三方运行依赖或修改插件协议。

参考项目：https://github.com/haowang02/codex-candy-eval ，核对提交 `29127fa5a12fb7654e865f684dcaf55ade181349`。当前引擎及提示词独立编写，没有嵌入或执行上游 Python 脚本。

## 原因分析

原项目通过本机 `codex exec` 重复测试一道糖果题，用全文出现独立的 21 作为正确判据。原账号连通性测试也不能直接承载评测：OpenAI Responses 路径固定发送 hi，返回信息不足，测试成功可能恢复账号状态。

因此新增独立管理员评测服务，复用 OpenAI 网关转发处理认证、代理、模型映射和协议转换。答对、答错、无法判分、请求失败分别统计，不根据数学答案改变账号状态。

## 使用方法

1. 账号管理 → OpenAI 账号“更多” → “推理评测”；或分组管理 → OpenAI 分组行“推理评测”。
2. 从候选列表选择或输入文本模型，选择 low / medium / high / xhigh / max，设置 1～20 轮（默认 5 轮）。候选列表加载失败时可手动输入模型。
3. 点击“开始评测”直接运行，每轮串行执行并显示结果。查看实际账号、请求/上游/上游报告模型、请求/生效推理强度、文本、判分、用量和耗时。
4. 结果逐轮保存到服务端。取消、关闭或刷新后，重新打开同一账号/分组的评测弹窗，展开“历史记录”，点击“查看结果”载入报告；也可导出 JSON。

## 处理方案与边界

### 目标模式

- 固定账号：仅调用指定账号，不自动切换其他账号；支持普通 OpenAI OAuth / API Key 文本模型。当前不支持影子、首发账号、其他平台账号。
- 分组抽样：读取分组模型白名单、推理策略和渠道映射，然后通过既有调度器选号；逐轮记录实际账号。不自动重试到另一个账号，不保证覆盖分组所有账号。
- 分组抽样是管理员探测，未模拟客户余额、订阅、会话粘性和完整故障切换。遇到利润准入门时明确拒绝，并建议使用固定账号测试。不作为真实用户端到端 SLA 测试。
- 非文本模型及映射到已知非文本模型的请求在发起前拒绝。推理强度不被模型支持时会报告上游错误，不静默改档后冒充原档位。

### 题目与判分

题目版本 `candy-shape-v1` 明确允许通过触感选择形状、不能识别口味、不放回，先决定每种形状各取几颗。六类数量为圆形苹果/桃子/西瓜 7/9/8，星形 7/6/4。最小保证成功方案是 9 圆 + 12 星 = 21。

独立枚举验证：可选择形状时最少为 21，完全盲抓时最少为 29。因此明确题意，避免两种条件混为同题。与原项目未澄清题意、宽松判分的数值不能直接混算基线。

要求最后一行是唯一的 `FINAL_ANSWER: 整数`，精确比较 21。解释提到 21 而最终写 29，判错；缺标记、多个标记、分数/小数，标记为无法判分。截断、空输出、请求异常不算数学答错。每轮新请求，不携带历史回答或标准答案，不提供外部工具。

正确率 = 答对 / 可判分轮次；同时显示请求失败与无法判分数。无可判分轮次不显示 0% 冒充有效结论。推理 token 缺失显示“—”，不当作 0。单题仅提供能力波动线索，不自动断言降智、模型造假或自动下架账号。

### 资源与生命周期

- 每轮后端超时 180 秒，前端超时 190 秒；输出预算请求为 16384 token，是否被上游支持/接受仍取决于模型及现有网关适配。捕获返回体上限 2 MiB，网关上游读取仍遵循其已有上限。
- 单实例最多同时运行 2 轮评测；账号并发复用现有槽位控制。UI 默认串行，服务端接口每次只允许固定题目的单轮请求。
- 取消或卸载弹窗会 Abort 当前请求并停止剩余轮次。通过仅评测上下文启用的标识保留上游取消与截止时间；普通用户请求继续保留原有断连用量收尾行为。
- 仅在管理员点击“开始评测”后运行；打开弹窗只加载模型候选列表，不发送评测请求，也不读写全局开关。
- 每轮可能消耗真实上游额度；管理员探测不扣普通用户余额，也不创建普通用户计费记录。报告记录能取得的 token，用量缺失保持未知。
- 对认证、限流等真实上游错误仍使用既有网关状态处理；数学判分不调用账号恢复、封禁或调权功能。
- 不保存和导出凭据。错误仅包含状态码和操作建议，避免把上游响应体或含认证信息的 URL 回传。

### 历史保存与未来公开浏览

- 新增迁移 `242_model_evaluation_reports.sql`，在现有 PostgreSQL 中创建报告表和逐轮结果表，无新增依赖。报告不自动清理，账号/分组删除或改名不级联删除已有快照。
- 每次“开始评测”先创建报告，保存目标名称、请求模型/强度、计划轮数、题目版本、题目文本、创建管理员和时间。未能建立记录时不调用模型。
- 每轮调用前原子占位，结束后保存原始回答、判分、实际账号、模型/强度映射、用量、耗时与错误。唯一约束和事务锁保证同一报告同一轮不重复调用；重试已完成轮次直接返回保存的结果。
- 请求取消后仍以独立的最多 10 秒数据库上下文收尾保存，已完成结果不依赖弹窗存活。保存失败时把本轮结果和明确的保存失败提示返回前端，停止后续轮次，提示先导出再排查数据库。异常退出时未写入的最终回答无法恢复；占位记录保留，超过 4 分钟显示“未完成 / 已中断”，不会伪装成答错或自动重试消耗额度。
- 历史列表按目标分页，每页 20 份，只返回摘要；查看详情再加载逐轮回答。记录已保存最终结果数 / 计划轮数，允许部分完成报告。
- `visibility` 预留 `private` / `public`，创建时服务端强制 `private`。当前所有读写接口均在管理员认证下，没有普通用户/匿名读取入口，也没有发布操作。未来公开需新增脱敏只读接口及发布管理，明确筛选已发布报告；账号名称/编号、创建人、请求标识、原始错误与内部映射不可直接复用管理员详情返回。
- 上线前已存在于旧弹窗内存中的结果无法自动补回服务器；本版运行后产生的报告开始持久保存。

## 接口与修改范围

评测接口位于已有管理员认证、审计和面板限流之下：

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| POST | `/api/v1/admin/model-evaluation/reports` | 创建报告：target_type、target_id、model、effort、rounds |
| GET | `/api/v1/admin/model-evaluation/reports` | 按 target_type、target_id、page 查询历史摘要 |
| GET | `/api/v1/admin/model-evaluation/reports/:id` | 报告及逐轮结果详情 |
| POST | `/api/v1/admin/model-evaluation/reports/:id/rounds` | 执行单轮：round；参数取自报告快照 |

主要新增文件：

- `backend/internal/service/model_evaluation.go`：执行、目标校验、并发、取消与结果。
- `backend/internal/service/model_evaluation_score.go`：版本化题目、严格判分和用量解析。
- `backend/internal/service/model_evaluation_history.go`、`backend/internal/repository/model_evaluation_repo.go`：报告生命周期、持久保存和分页读取。
- `backend/internal/handler/admin/model_evaluation_handler.go`：参数边界和管理员端点。
- `frontend/src/components/admin/ModelEvaluationModal.vue`：两处入口共用弹窗。
- `frontend/src/api/admin/modelEvaluation.ts`、中英文文案与相应测试。

现有文件的改动为路由/依赖注入/入口连接，以及两项仅评测请求启用的网关观察行为：取消透传与 Chat Completions 转换前用量读取。普通转发协议不变，现有传输插件配置不变。部署时由已有迁移流程创建两张评测表，不修改现有业务表或敏感配置。

## 验证步骤

在 backend 目录：

```sh
go test -tags unit ./internal/service ./internal/handler/admin ./internal/repository ./internal/server/routes -run 'TestModelEvaluation|Test.*UpstreamContext|TestForwardResponses_ForceChatCompletionsRoutesNonStreamingToChatCompletions' -count=1
go test -tags integration ./internal/repository -run '^TestModelEvaluationRepositoryDurableHistory$' -count=1 -v
go build -o /tmp/sub2api-model-evaluation ./cmd/server
```

在 frontend 目录：

```sh
pnpm typecheck
pnpm exec vitest run src/components/admin/__tests__/ModelEvaluationModal.spec.ts src/i18n/__tests__/localeKeyCompleteness.spec.ts
pnpm build
```

覆盖：点击开始直接评测、模型列表失败时手动输入、请求参数与体积边界、严格判分、截断与缺 usage、固定账号不换号、分组映射及强度策略、不可调度与账号范围、并发释放、错误脱敏、仅评测取消传递、真实网关代码下 Responses / Chat Completions / OAuth SSE 三条路径及用量、前端取消/失败提示/卸载。新增报告快照、仅管理员可见默认值、结果保存失败、取消后保存、重复执行不重复调用、历史载入和未完成记录的测试。

本地浏览器验收使用模拟 API，验证答对/答错/429 混合结果、未知推理 token、顶部失败提示，以及刷新后从历史列表恢复报告。真实 PostgreSQL 集成测试依赖 Docker；本次本机 Docker 返回 500，尚未验证真实数据库读写，不能将集成测试跳过当作通过。真实账号调用需在部署后选定目标执行，当前开发验证不代表任何真实模型的评测结果。

## 注意事项

单题、高低推理档位和不同模型别名之间不能直接互相比较。建议先用同题版本、同模型、同生效强度、接近的测试时间建立对照；再增加不同题目和样本量。上游报告的模型字段不是模型身份保证。
