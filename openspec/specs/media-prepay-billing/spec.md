# media-prepay-billing Specification

## Purpose
TBD - created by archiving change add-fal-async-image-platform. Update Purpose after archive.
## Requirements
### Requirement: 先扣费后退款

系统 SHALL 在任务提交时按预估金额预扣费（先扣费模式），并在 `async_media_tasks.held_cost` 记录预扣金额。任务成功 SHALL 结清费用；任务失败或真超时 SHALL 退还已扣金额。

#### Scenario: 提交时预扣费
- **WHEN** 任务提交成功并落库
- **THEN** 系统 SHALL 预扣对应金额并记录 `held_cost`

#### Scenario: 成功结算
- **WHEN** 任务成功完成
- **THEN** 系统 SHALL 确认扣费并记录 `final_cost`

#### Scenario: 失败退费
- **WHEN** 任务被判定失败或真超时
- **THEN** 系统 SHALL 退还已扣金额，退费与终态更新 SHALL 在同一事务内完成

### Requirement: usage_logs 记录扣费/退费状态

`usage_logs` SHALL 新增字段 `task_id`、`image_urls`、`cos_url`、`billing_status`(charged|refunded)，并在任务终态时追加一条记录以保持只追加语义。

#### Scenario: 成功写 usage_log
- **WHEN** 任务成功
- **THEN** 系统 SHALL 追加一条 `billing_status=charged` 的 usage_log，含 `task_id`、成功图片 url 与 `cos_url`

#### Scenario: 退费写 usage_log
- **WHEN** 任务退费
- **THEN** 系统 SHALL 追加一条 `billing_status=refunded` 的 usage_log，含 `task_id`

#### Scenario: usage_log 关联任务
- **WHEN** 查询某条 usage_log
- **THEN** 该记录 SHALL 通过 `task_id` 关联到对应 `async_media_tasks`

### Requirement: 按分辨率×质量二维计费

系统 SHALL 在 image 计费模式下按 (image_size 档位 × quality) 二维定价。定价表 SHALL 在现有按尺寸分档基础上新增 `quality` 维度。

#### Scenario: 二维定价命中
- **WHEN** 请求指定了 image_size 与 quality
- **THEN** 系统 SHALL 按对应 (size_tier, quality) 档位计算金额

#### Scenario: 兼容存量单维定价
- **WHEN** 定价记录未配置 quality 维度
- **THEN** 系统 SHALL 使用默认 quality 档位以兼容存量配置
