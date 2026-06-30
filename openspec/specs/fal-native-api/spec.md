# fal-native-api Specification

## Purpose
TBD - created by archiving change add-fal-async-image-platform. Update Purpose after archive.
## Requirements
### Requirement: OpenAI 伪同步图片门面

系统 SHALL 在 `POST /v1/images/generations` 与 `POST /v1/images/edits` 上对外表现为同步：内部走 fal 异步队列并阻塞等待结果，最终在同一连接内返回 OpenAI 格式图片结果。

#### Scenario: 伪同步生成成功
- **WHEN** 客户端调用 `/v1/images/generations` 且上游为 fal
- **THEN** 系统 SHALL 提交 fal 队列、轮询直至完成、并在同一请求内返回 OpenAI 格式图片结果

#### Scenario: 伪同步编辑成功
- **WHEN** 客户端调用 `/v1/images/edits` 并提供输入图片
- **THEN** 系统 SHALL 将图片输入转为 fal `image_urls[]`（及可选 `mask_url`）提交 `gpt-image-2/edit`，完成后返回 OpenAI 格式结果

#### Scenario: 伪同步阻塞超时
- **WHEN** 阻塞等待超过配置上限仍未完成
- **THEN** 系统 SHALL 返回错误，但不退费、不终结任务，交由后台对账兜底

### Requirement: fal 原生异步门面

系统 SHALL 暴露 fal 原生异步接口供客户端使用，包括 submit、status、result、queue 与 streaming。submit SHALL 立即返回 `request_id`（真异步）。

#### Scenario: 提交任务
- **WHEN** 客户端 `POST /fal/{model}` 提交任务
- **THEN** 系统 SHALL 转发 fal 队列提交并立即返回 `request_id`

#### Scenario: 查询状态
- **WHEN** 客户端查询某 `request_id` 的状态
- **THEN** 系统 SHALL 返回 fal 状态（如 IN_QUEUE/IN_PROGRESS/COMPLETED）

#### Scenario: 获取结果
- **WHEN** 客户端获取某已完成 `request_id` 的结果
- **THEN** 系统 SHALL 返回 fal 结果（图片列表）

#### Scenario: 取消任务
- **WHEN** 客户端请求取消某 `request_id`
- **THEN** 系统 SHALL 转发取消请求

#### Scenario: 流式调用
- **WHEN** 客户端发起 streaming 请求
- **THEN** 系统 SHALL 以流式方式推送 fal 事件直至完成

### Requirement: fal 原生 generations 与 edits 均支持

系统 SHALL 在原生门面同时支持文生图（`gpt-image-2`）与图生图/编辑（`gpt-image-2/edit`）两类模型。

#### Scenario: 原生编辑请求
- **WHEN** 客户端通过原生门面提交带 `image_urls[]` 的编辑请求
- **THEN** 系统 SHALL 路由到 `gpt-image-2/edit` 并按异步流程处理
