# Admin Accounts

管理账号 feature 负责账号列表、创建/编辑、批量更新、授权和用量展示。

账号列表的“同步上游模型”通过 `useAccountUpstreamModelSync.ts` 按每批 20 个账号调用批量同步 Action，`UpstreamModelSyncResultsDialog.vue` 展示进度与逐账号结果。成功后刷新列表，失败/跳过账号保持选中；保存规则和平台边界见 [同步说明](../../../../docs/ACCOUNT_UPSTREAM_MODELS_SYNC.md)。

- `data/dtos/adminAccountDtos.ts`, `openAIQuotaDtos.ts`: 账号创建/更新、混合渠道预检、Codex 导入、OpenAI quota、模型和临时不可调度状态等账号专属协议类型；`src/types` 仅保留兼容转发。
- `data/datasources/adminAccountQueries.ts`: 列表、ETag、详情、摘要、统计、用量、临时不可调度状态、Ollama Cloud 状态、模型、Antigravity 默认映射和上游计费设置/快照查询。
- `data/datasources/adminAccountActions.ts`: 账号创建/更新、OAuth 凭据应用/错误清理、Codex session/PAT 导入、上游模型同步、CPA 测试、页面批量操作、导出、账号状态维护、重复账号、上游计费探测、OpenAI OAuth 主动批量额度查询和 Ollama Cloud 配置动作。
- `data/datasources/adminAccountOAuthActions.ts`: 通用账号与 OpenAI OAuth 授权 URL、code exchange、cookie 授权和 refresh token 动作；旧 `accountsAPI` 继续兼容转发。
- `data/datasources/adminAccountsDatasource.ts`: 旧 `accountsAPI` 兼容聚合与尚未迁移的账号操作。
- `data/datasources/scheduledTestsDatasource.ts`: 账号定时测试计划、启停、删除和结果查询。
- `presentation/pages/`: 列表查询、筛选、刷新和对话框编排。
- `presentation/widgets/AccountPriorityCell.vue`: 优先级快捷调整、450ms 合并保存与卸载清理；直接调用账号 Action，成功后由列表 owner 合并返回账号。
- `presentation/widgets/create/`: 创建表单的领域字段。
- `presentation/widgets/edit/`: 编辑表单的领域字段。
- `presentation/composables/`: 有界的表单策略、OAuth 与提交编排。

账号列表页保留路由、请求和弹窗编排，表格 DOM 由 `AccountsTableView.vue` 静态承载。列表列偏好、展示映射、今日统计和上游额度分别由同域 composable 管理；表格只消费 `accountTableViewContext.ts` 的 typed context，不直接请求 API 或创建 watcher、timer。`excel_bps` 独立筛选普通 OpenAI OAuth 账号的账号级 BPS 状态，筛选在后端分页、导出与上游计费快照前生效；编辑开关保存为 `extra.excel_bps_enabled`，显式 `false` 覆盖旧字段。

`oauth_quota` 在服务端分页前筛选已持久化的 OAuth 用量快照：支持有额度、任一窗口满额、OpenAI 有可用重置次数、OpenAI 5h 满额和 OpenAI 7d 满额；缺少快照的账号保持未知，不会被误判为有额度或满额。

请求生命周期由 presentation 明确持有：账号页负责列表 AbortController、ETag、筛选和写后刷新；今日统计 composable 用请求序号忽略过期响应；上游计费 composable 负责费率 ETag、轮询暂停条件、探测后的列表刷新和额度缓存失效。重复账号 Action 在请求成功前将幂等键同时保存在内存与 `sessionStorage`，失败重试和页面重载继续复用同一个键。

当前接口 owner 盘点：列表、详情、摘要、统计、用量和只读快照属于 Query；创建、更新、删除、授权、批量操作、导入导出与主动探测属于 Action；账号专属请求/响应类型属于 `data/dtos/adminAccountDtos.ts`。迁移按调用链分片推进，兼容 `accountsAPI` 在所有旧消费者迁完前保持相同方法和响应形状，兼容 `@/types` 继续转发已迁移类型。账号页和批量编辑对话框直接依赖 Query/Action owner，不再经过统一 admin barrel。

账号额度通知的账号级阈值仍由 `useQuotaNotifyState.ts` 管理；其中全局启用状态直接读取 `admin-settings` datasource，失败时保持关闭，不再通过统一 admin barrel 跨域访问。

定时测试面板直接依赖 `scheduledTestsDatasource.ts`，由面板继续持有打开时加载、创建后刷新、局部更新、删除和结果展开状态。

OAuth composable 和创建账号的 OAuth 兑换编排直接依赖账号 OAuth Action、账号 Action owner 或对应平台 datasource。授权 URL、state/session、代理参数、code exchange、cookie 授权、refresh token 和旧服务器 capabilities fallback 的请求语义保留在 feature data；composable 继续拥有 loading、错误反馈和凭据组装。

两个重新授权对话框直接依赖账号 Action 与 OAuth Action owner：普通对话框保留 `update + clear-error` 的原有写入顺序，管理员对话框保留 `apply-oauth-credentials` 的增量 extra 合并和服务端 token 缓存失效语义；成功后仍分别发出原有 `reauthorized` 事件并关闭对话框。

创建和编辑对话框拥有各自的 reactive form，并在 setup 中同步装配 watcher、授权和提交 composable。字段组件只通过明确的 typed context 读写表单与触发动作，不持有 API 或 Store。

创建/编辑链路直接依赖账号 Query/Action owner：创建、更新、混合渠道预检、Codex 导入、上游模型同步、CPA 测试和创建后主动计费探测均不再经过统一 admin barrel。Web Search 全局开关与 TLS 指纹 profile 列表继续由 `admin-settings` datasource 所有。旧 `accountsAPI` 保持同名函数身份，供旧导入和其他兼容消费者使用；数据导入与 CRS 预览/同步的运行时调用已直接进入明确 owner。

- `accountEditorContext.ts`: 字段组件的最小类型契约。
- `accountFormPolicy.ts`: 创建和编辑共用的纯表单转换。
- `useCreateAccountEditorPolicy.ts`: 创建表单 watcher 与字段动作。
- `useCreateAccountOAuthActions.ts`: 创建账号的 OAuth exchange/import/batch 流程。
- `accountEditUpdatePayload.ts`: 按账号类型和平台构造编辑 payload。
- `useEditAccountSubmission.ts`: 编辑校验、风险确认与更新请求编排。
- `useCPATestConnection.ts`: CPA 未保存表单的连接测试、策略参数和反馈状态。
- `useAccountsUpstreamBilling.ts`: 上游计费探测、额度缓存、批量查询和定时刷新。
- `useAccountColumnPreferences.ts`: 列可见性迁移与服务端派生查询参数。
- `useAccountTodayStats.ts`: 当前页今日统计的请求并发保护。
- `useAccountTablePresentation.ts`: 纯列定义、徽标和单元格格式化。
- `BulkEditOpenAICustomRelayOption.vue`: OpenAI OAuth 自定义转发端点开关与地址输入。
- `BulkEditExcelBPSOption.vue`: BPS 账号批量开关的三态选择；后端验证资格并用数据库条件限制写入。
- `BulkEditCodexThinkingTagOption.vue`: 批量更新 Codex thinking tag 规范化开关，保持批量编辑主对话框在维护上限内。

新增平台字段时同步检查创建、编辑和批量更新 payload。不要复制无边界表单、把完整 Pinia Store 传入字段组件，或把控制器重新堆回单一 SFC。运行时代码保持在 1500 行以内，新增职责应进入现有有界组件或 composable。

验证入口：

```sh
pnpm exec vitest run src/features/admin-accounts
pnpm run typecheck
```

临时不可调度规则的 scope 归一化、旧值解析和可选字段序列化由 `presentation/accountFormPolicy.ts` 维护，创建/编辑 advanced widget 共用该协议；自动范围省略 scope。显式范围须在所有节点升级后启用，详见 [同步与回退边界](../../../../docs/UPSTREAM_SYNC_20261013.md)。

Claude 5.5 的原生白名单、Antigravity 分档模型和映射预设由 `presentation/composables/useModelWhitelist.ts` 维护。同步结果在原 `models` 列表之外可附带 `metadata`；已有列表调用保持兼容。

OpenAI 与 Anthropic API-key 的 WebSearch 模拟共用已有三态字段和全局可见条件。创建 extra 的纯转换由 `accountFormPolicy.ts` 所有，编辑 payload 由 `accountEditUpdatePayload.ts` 所有；OAuth 不显示或启用该模拟。

`presentation/accountFilterState.ts` 校验并持久化账号平台/状态筛选；`presentation/accountQuota.ts` 统一 API Key、Bedrock 与 service account 的本地配额展示和编辑资格，池模式权限保持独立。

`GrokModelDiagnosticsPanel.vue` 查询已保存账号详情并显式刷新目录；诊断和 API 类型由同 feature 的 `data/dtos/grokModelDiagnostics.ts` 所有。Grok 白名单同步只补下拉可选项，不自动改变选择。账号切换后忽略迟到响应，旧节点缺少诊断字段时显示兼容提示。

`presentation/accountCreateOpenAIExtra.ts` 构造创建账号的 OpenAI extra；`openaiImageSizeTiers.ts` 与 `OpenAIImageSizeTiersField.vue` 统一创建、编辑和批量更新的图片档位。缺省/全选不限，批量 null 不修改、显式空数组清除限制；启用前须完成全部节点升级，见 [同步审查](../../../../docs/UPSTREAM_SYNC_20261104.md)。
