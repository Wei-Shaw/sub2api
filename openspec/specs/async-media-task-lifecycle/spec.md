# async-media-task-lifecycle Specification

## Purpose
TBD - created by archiving change add-fal-async-image-platform. Update Purpose after archive.
## Requirements
### Requirement: 异步任务落库

系统 SHALL 新建可变任务表 `async_media_tasks` 记录每个异步媒体任务，至少包含：内部请求 ID、上游 `request_id`、账号/api_key/用户/分组/渠道关联、模型、image_size、quality、num_images、状态、预扣金额、最终金额、成功图片 url、COS url、失败截止时间与时间戳。任务提交时 SHALL 写入 `upstream_request_id` 以确保可追溯。

#### Scenario: 提交时落库
- **WHEN** 任务提交到 fal 队列并拿到 `request_id`
- **THEN** 系统 SHALL 创建一条 `status=pending` 的任务记录并保存 `upstream_request_id`

#### Scenario: 成功落库图片 url
- **WHEN** 任务完成并取到 fal 图片结果
- **THEN** 任务记录 SHALL 保存成功图片 url（以及转存后的 COS url）

### Requirement: 任务状态流转

任务状态 SHALL 在 `pending → running → succeeded | failed | refunded | expired` 之间流转。终态一旦写入 SHALL 不可逆。

#### Scenario: 进入运行态
- **WHEN** fal 状态变为 IN_PROGRESS
- **THEN** 任务 SHALL 更新为 `running`

#### Scenario: 完成成功
- **WHEN** fal 状态为 COMPLETED 且取到结果
- **THEN** 任务 SHALL 更新为 `succeeded` 并记录 `final_cost` 与图片 url

#### Scenario: 明确失败
- **WHEN** fal 状态接口明确返回失败
- **THEN** 任务 SHALL 进入失败并触发退费流程

### Requirement: 后台对账 reconciler

系统 SHALL 提供后台对账任务，周期扫描处于 `pending/running` 的任务，依据 `upstream_request_id` 查询 fal 状态以补完成或补退费。扫描间隔与任务失败时间 SHALL 可配置。对账操作 SHALL 幂等，避免重复退费或重复写 usage_log。

#### Scenario: 补完成
- **WHEN** reconciler 发现某任务在 fal 侧已 COMPLETED
- **THEN** 系统 SHALL 取结果、转存 COS、写终态成功并追加 usage_log

#### Scenario: 补退费（明确失败）
- **WHEN** reconciler 发现某任务在 fal 侧明确失败
- **THEN** 系统 SHALL 退费并将任务置为 `refunded`

#### Scenario: 真超时退费
- **WHEN** 任务到达可配置的失败截止时间 `fail_deadline_at` 仍未完成
- **THEN** 系统 SHALL 退费并将任务置为 `expired`

#### Scenario: 幂等保护
- **WHEN** reconciler 对同一任务重复处理
- **THEN** 系统 SHALL 依据任务当前状态去重，不重复退费或重复写 usage_log

### Requirement: 可配置扫描与失败时间

系统 SHALL 提供全局配置项以设定 reconciler 扫描间隔与任务失败截止时长。

#### Scenario: 调整扫描间隔
- **WHEN** 管理员修改扫描间隔配置
- **THEN** reconciler SHALL 按新间隔运行

#### Scenario: 调整失败时间
- **WHEN** 管理员修改任务失败时间配置
- **THEN** 新建任务的 `fail_deadline_at` SHALL 按新值计算
