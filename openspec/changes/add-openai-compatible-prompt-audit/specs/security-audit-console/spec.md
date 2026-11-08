## ADDED Requirements

### Requirement: 管理台必须提供安全审计分组和独立提示词审计页面
控制台 SHALL 把安全相关的内容审核页面组织到“安全审计”导航分组中，并新增独立“提示词审计”页面。原 `/admin/risk-control` 路由、页面状态和功能 MUST 保持兼容；新页面路由 MUST 为 `/admin/prompt-audit` 或经实现评审确认的等价稳定路由。

#### Scenario: 管理员查看侧栏
- **WHEN** 管理员已登录且 risk_control_enabled=true
- **THEN** 侧栏 MUST 展示“安全审计”可展开分组
- **THEN** 分组 MUST 至少包含“内容审核”和“提示词审计”两个子入口

#### Scenario: 管理员打开原内容审核页面
- **WHEN** 管理员访问 `/admin/risk-control`
- **THEN** 页面 MUST 继续展示原有 Moderations、关键词、Hash、封号、邮件和记录功能
- **THEN** 页面 MUST NOT 被提示词审计配置或事件替换

#### Scenario: 功能总开关关闭
- **WHEN** risk_control_enabled=false
- **THEN** 安全审计导航和提示词审计网关执行 MUST 按现有功能开关策略停用
- **THEN** 已存储的配置和历史事件 MUST 不被删除

### Requirement: 提示词审计页面必须提供清晰的独立工作区
页面 SHALL 在同一工作区展示运行概览、审计池、审计策略、事件列表和固定保存操作区。页面 MUST 清楚区分“异步只审计”和“同步阻止”，并 MUST 展示未保存状态和最终生效状态。

#### Scenario: 初次打开页面
- **WHEN** 管理员打开提示词审计页面
- **THEN** 页面 MUST 并行或有界加载配置、运行态、分组列表和事件列表
- **THEN** 页面 MUST 展示有效模式、Worker 状态、队列状态、节点连通性和最近错误

#### Scenario: 修改但未保存配置
- **WHEN** 管理员修改审计池、分类、范围或模式开关
- **THEN** 页面 MUST 显示“有未保存的更改”
- **THEN** 运行态 MUST 继续标识服务端当前生效版本，不能把草稿显示为已生效

### Requirement: 页面必须支持完整审计池管理和真实探测
页面 SHALL 支持新增、编辑、启用、禁用和删除审计池，并允许配置 Base URL、API Key、Model、超时和 input_limit。API Key 已保存后 MUST 只显示配置状态，不能回显明文。

#### Scenario: 编辑已保存节点
- **WHEN** 管理员打开已配置 API Key 的节点
- **THEN** API Key 输入框 MUST 为空或显示不可逆占位状态
- **THEN** 未填写新 Key 保存时 MUST 保留原密文
- **THEN** 页面 MUST 提供显式清除凭据操作

#### Scenario: 执行连接测试
- **WHEN** 管理员点击节点“连接测试”
- **THEN** 页面 MUST 展示配置校验、发送请求、服务响应和测试结论状态
- **THEN** 结果 MUST 展示耗时、HTTP 状态、稳定错误码和脱敏消息

### Requirement: 页面必须支持审计范围和九类风险配置
页面 SHALL 支持全部分组或指定 group ID 范围，并展示九类 Qwen3Guard 风险分类。页面 MUST 使用目标项目真实分组数据，已删除但仍存在于配置中的分组 MUST 显示为失效项而不是被静默丢弃。

#### Scenario: 选择指定分组
- **WHEN** 管理员把范围切换为 selected 并选择一个或多个分组
- **THEN** 保存载荷 MUST 使用稳定 group ID
- **THEN** 页面 MUST 展示已选数量并支持搜索

#### Scenario: 查看风险分类
- **WHEN** 管理员查看扫描器配置
- **THEN** 页面 MUST 展示 Violent、Non-violent Illegal Acts、Sexual Content or Sexual Acts、PII、Suicide & Self-Harm、Unethical Acts、Politically Sensitive Topics、Copyright Violation、Jailbreak

### Requirement: 开启同步阻止必须有明确的风险确认
页面 SHALL 把 enabled、blocking_enabled 和 store_pass_events 作为独立开关。关闭 enabled 时 MUST 自动关闭并禁用 blocking_enabled；开启 blocking_enabled 时 MUST 展示二次确认，说明请求延迟、Block 和 Guard 不可用的 fail-closed 行为。

#### Scenario: 开启同步阻止
- **WHEN** 管理员把 blocking_enabled 从 false 切换为 true
- **THEN** 页面 MUST 在保存前展示风险确认
- **THEN** 确认文案 MUST 说明请求会等待 Guard，Block 或 Guard 不可用时不会访问上游

#### Scenario: 关闭审计总开关
- **WHEN** 管理员关闭 enabled
- **THEN** 页面草稿 MUST 同时把 blocking_enabled 设为 false

### Requirement: 配置保存必须可验证且不得泄露凭据
页面 SHALL 通过一个统一保存动作提交完整规范化配置。保存成功后 MUST 用后端返回值刷新页面快照、清除已提交 API Key 明文并显示 config_version；保存失败 MUST 保留草稿并展示稳定错误信息。

#### Scenario: 保存成功
- **WHEN** 后端成功保存配置
- **THEN** 页面 MUST 显示配置已同步和新的 config_version
- **THEN** 浏览器状态、调试日志和缓存 MUST 不再保留刚提交的 API Key 明文

#### Scenario: 保存校验失败
- **WHEN** 后端返回节点地址、模式组合或策略校验错误
- **THEN** 页面 MUST 保留用户草稿
- **THEN** 页面 MUST 展示稳定错误码及可行动的中文说明

#### Scenario: 配置被其他管理员更新
- **WHEN** 保存返回 409 `prompt_audit_config_conflict`
- **THEN** 页面 MUST 保留本地草稿并提示服务端配置已变化
- **THEN** 页面 MUST 提供重新加载/对比入口，不得自动用旧草稿覆盖新配置

### Requirement: 页面必须展示真实运行态和同步 Guard 指标
页面 SHALL 展示 process_status、Worker 总数/活动数、队列容量/长度、queued/processing/done/failed 数、处理/失败总数、最近时间、节点连通性、配置版本一致性、Redis Payload Store 状态和同步 Guard Allow/Flag/Block/Unavailable/timeout/failover/bulkhead 指标。

#### Scenario: 配置版本未同步
- **WHEN** expected_config_version 与 active_config_version 不一致
- **THEN** 页面 MUST 显示明确的配置未同步或加载中状态
- **THEN** 页面 MUST 展示最近加载错误和时间（如存在）

#### Scenario: Worker 心跳过期
- **WHEN** heartbeat_at 超过后端定义的健康窗口
- **THEN** 页面 MUST 显示 stale 而不是 running

### Requirement: 页面必须提供可复核的事件列表和详情
页面 SHALL 提供事件分页、总数、decision/risk/endpoint/group/user/API key/request ID/prompt Hash/关键字/时间范围筛选、行选择和详情抽屉或弹窗。详情 MUST 只展示脱敏数据。

#### Scenario: 查看事件列表
- **WHEN** 管理员应用筛选
- **THEN** 表格 MUST 展示时间、用户/API key、分组、入口/模型、判定、风险、分类、预览和操作

#### Scenario: 查看事件详情
- **WHEN** 管理员打开一条事件
- **THEN** 页面 MUST 展示脱敏预览、审计摘要、结构化返回、具体风险摘要和技术信息
- **THEN** 页面 MUST 提供 request ID、prompt Hash、scanner、策略、节点、配置版本、分片数和耗时
- **THEN** 页面 MUST 不展示完整提示词或节点 API Key

#### Scenario: 复核用户身份和具体风险
- **WHEN** 事件拥有用户名、用户邮箱、API Key 名称和一个或多个风险分类
- **THEN** 页面 MUST 将用户名、邮箱和 API Key 名称分列展示并提供独立复制操作
- **THEN** 页面 MUST 为每个风险展示 category、标题、说明、严重度、动作、scanner、score 和脱敏证据摘要
- **THEN** 用户不存在或字段为空时 MUST 显示稳定 fallback，而不是把其他身份字段冒充为该字段

### Requirement: 页面必须提供防误操作的事件删除流程
页面 SHALL 支持单条删除、选中项批量删除和按筛选删除。按筛选删除 MUST 先调用预览接口，并要求明确时间范围、matched_count、snapshot_max_id、filter_hash、服务端认证 confirmation_token 和二次确认。

#### Scenario: 单条删除
- **WHEN** 管理员确认删除一条事件
- **THEN** 页面 MUST 调用单条删除接口并在成功后刷新列表与运行统计

#### Scenario: 按筛选删除
- **WHEN** 管理员已设置明确时间范围并请求按筛选删除
- **THEN** 页面 MUST 先展示匹配数量和规范化筛选摘要
- **THEN** 只有管理员再次确认后才能提交 filter_hash、confirmation_token 和 confirm=true

#### Scenario: 筛选在预览后发生变化
- **WHEN** 管理员预览后修改任意筛选条件
- **THEN** 旧 filter_hash MUST 失效
- **THEN** 旧 confirmation_token MUST 同时失效
- **THEN** 页面 MUST 要求重新预览

### Requirement: 管理 API 操作必须纳入现有管理员审计
所有配置写入、节点探测和事件删除 SHALL 复用现有管理员鉴权与管理操作审计。审计详情 MUST 使用脱敏摘要，禁止记录 API Key、完整提示词或完整请求载荷。

#### Scenario: 配置更新成功
- **WHEN** 管理员成功保存提示词审计配置
- **THEN** 管理操作审计 MUST 记录操作者、request ID、enabled、blocking_enabled、config_version、节点数量、分类数量和分组范围摘要

#### Scenario: 节点探测失败
- **WHEN** 管理员探测节点失败
- **THEN** 管理操作审计 MUST 记录节点 ID、稳定错误码、HTTP 状态和耗时
- **THEN** 审计详情 MUST 不包含 API Key 或完整 Base URL query

### Requirement: 页面必须满足响应式、可访问和国际化要求
页面 SHALL 使用现有 Vue 3、i18n 和通用组件体系，支持桌面与窄屏，所有开关、输入、按钮、状态和对话框 MUST 具有可访问名称；新增中英文文案键 MUST 成对提供且通过现有 lint、typecheck 和 Vitest。

#### Scenario: 窄屏使用
- **WHEN** 页面宽度小于桌面断点
- **THEN** 配置区、筛选区、表格和固定保存栏 MUST 可滚动或重排而不遮挡关键操作

#### Scenario: 键盘和读屏操作
- **WHEN** 用户只使用键盘或读屏访问页面
- **THEN** 审计池操作、模式开关、筛选、详情和确认对话框 MUST 可识别且可操作
