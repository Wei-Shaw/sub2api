## MODIFIED Requirements

### Requirement: 先扣费后退款

系统 SHALL 在任务提交时按完整预估金额解析统一计费上下文，并对实际付款钱包预扣费（先扣费模式）。成员划拨余额足以覆盖预估金额时 SHALL 优先选择成员；余额不足且共享权限有效时 SHALL 选择企业独立余额，不得扣减 owner/管理员个人余额，也不得拆分一次预扣。`async_media_tasks` SHALL 记录 `held_cost`、消费用户、`organization_id`、`payer_user_id`、`balance_source` 与权限版本快照。任务成功 SHALL 针对该快照钱包结清费用；任务失败或真超时 SHALL 向同一快照钱包退还已扣金额。提交后的权限、成员状态或组织状态变化 SHALL NOT 改变既有任务的钱包。

#### Scenario: 提交时预扣费
- **WHEN** 任务提交成功并落库
- **THEN** 系统 SHALL 从解析出的实际付款用户预扣对应金额
- **AND** SHALL 记录 `held_cost` 与完整付款方快照

#### Scenario: 共享余额任务成功结算
- **WHEN** IAM 用户的划拨余额不足、以共享余额提交的任务成功完成
- **THEN** 系统 SHALL 针对任务快照中的企业钱包确认扣费并记录 `final_cost`
- **AND** IAM 用户划拨余额 SHALL 保持不变
- **AND** owner/管理员个人余额 SHALL 保持不变

#### Scenario: 权限撤销后失败退费
- **WHEN** 任务预扣主账号余额后共享余额权限被撤销，随后任务失败或真超时
- **THEN** 系统 SHALL 向任务快照中的原企业钱包退还已扣金额
- **AND** 退费与终态更新 SHALL 在同一事务内完成

#### Scenario: 划拨余额覆盖预扣
- **WHEN** IAM 用户的划拨余额足以覆盖完整预估金额，即使其拥有共享余额权限
- **THEN** 系统 SHALL 从成员划拨余额预扣并记录成员付款方快照
- **AND** SHALL NOT 扣减主账号余额

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
