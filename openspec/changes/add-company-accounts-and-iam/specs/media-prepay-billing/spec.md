## MODIFIED Requirements

### Requirement: 先扣费后退款

系统 SHALL 在任务提交时解析统一计费上下文，并按预估金额对实际付款用户预扣费（先扣费模式）。`async_media_tasks` SHALL 记录 `held_cost`、消费用户、`organization_id`、`payer_user_id`、`balance_source` 与权限版本快照。任务成功 SHALL 针对该快照付款方结清费用；任务失败或真超时 SHALL 向同一快照付款方退还已扣金额。提交后的权限、成员状态或组织状态变化 SHALL NOT 改变既有任务的付款方。

#### Scenario: 提交时预扣费
- **WHEN** 任务提交成功并落库
- **THEN** 系统 SHALL 从解析出的实际付款用户预扣对应金额
- **AND** SHALL 记录 `held_cost` 与完整付款方快照

#### Scenario: 共享余额任务成功结算
- **WHEN** IAM 用户以共享余额提交的任务成功完成
- **THEN** 系统 SHALL 针对任务快照中的主账号付款方确认扣费并记录 `final_cost`
- **AND** IAM 用户划拨余额 SHALL 保持不变

#### Scenario: 权限撤销后失败退费
- **WHEN** 任务预扣主账号余额后共享余额权限被撤销，随后任务失败或真超时
- **THEN** 系统 SHALL 向任务快照中的主账号退还已扣金额
- **AND** 退费与终态更新 SHALL 在同一事务内完成

#### Scenario: 选定付款方余额不足
- **WHEN** 任务提交时解析出的付款用户余额不足
- **THEN** 系统 SHALL 拒绝任务且不创建未持资的任务记录
- **AND** SHALL NOT 回退到另一余额来源

### Requirement: usage_logs 记录扣费/退费状态

`usage_logs` SHALL 包含 `task_id`、`image_urls`、`cos_url`、`billing_status` (`charged|refunded`)、`organization_id` 与 `payer_user_id`，并在任务终态时追加一条记录以保持只追加语义。记录 SHALL 同时保留消费用户，并从任务计费快照复制组织和付款用户，不得按终态时权限重新解析。

#### Scenario: 成功写 usage_log
- **WHEN** 任务成功
- **THEN** 系统 SHALL 追加一条 `billing_status=charged` 的 usage_log，含 `task_id`、成功图片 URL、`cos_url`、消费用户、组织与付款用户快照

#### Scenario: 退费写 usage_log
- **WHEN** 任务退费
- **THEN** 系统 SHALL 追加一条 `billing_status=refunded` 的 usage_log，含 `task_id` 与任务原始组织和付款用户快照

#### Scenario: usage_log 关联任务
- **WHEN** 查询某条 usage_log
- **THEN** 该记录 SHALL 通过 `task_id` 关联到对应 `async_media_tasks`
