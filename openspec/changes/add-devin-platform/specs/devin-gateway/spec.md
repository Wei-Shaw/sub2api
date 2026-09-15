## Purpose

把 `devin` 分组的 API Key 请求（Anthropic Messages、OpenAI Chat Completions、OpenAI Responses 含 WebSocket 传输、模型列表）翻译为 Devin Connect RPC 并流式回写，复用统一调度、并发、限流与计费管线。

## ADDED Requirements

### Requirement: 入站协议分派
分组平台为 `devin` 时，网关 MUST 把 `POST /v1/messages`（Anthropic）、`POST /v1/chat/completions`、`POST /v1/responses` 分派到 Devin 转发管线；`GET /v1/responses` MUST 升级为 Responses WebSocket 传输；`GET /v1/models` MUST 返回该分组 devin 账号目录的分组模型列表。

#### Scenario: 三类协议请求
- **WHEN** devin 分组的 API Key 请求任一受支持端点
- **THEN** 请求体 MUST 被对应 codec 解码为统一 IR，再编码为 Connect RPC 上游请求

#### Scenario: 模型列表
- **WHEN** GET /v1/models
- **THEN** 返回按族聚合的 GroupedModel 列表（对外 id + thinkingLevelMap），而非上游原始 uid 列表

### Requirement: 模型路由与 thinkingLevelMap
对外模型 id（族名，如 `swe-2`）+ effort/thinking 参数 MUST 经 thinkingLevelMap 解析为上游 uid；`is_model_router` 标记的 uid MUST 先调 `AssignModel` 拿 assignment JWT 再发聊天请求；overflow 响应 MUST 触发一次备选 uid 重试。

#### Scenario: effort 选级
- **WHEN** 请求 `swe-2` 且 effort=max
- **THEN** 上游收到 `swe-2-max` uid

#### Scenario: 未知模型
- **WHEN** 模型无法解析为目录 uid
- **THEN** 返回 400 invalid_request，不换号 failover

### Requirement: 会话与 step_index
会话 id MUST 由首条消息内容确定性派生（sha256→UUID）；`step_index` 首请求为 0、后续自 2 递增；助手历史回合 MUST 合并为单条 prompt；f13/f18/f22 字段 MUST 省略。

#### Scenario: 首请求
- **WHEN** 新会话首请求
- **THEN** step_index=0 且携带派生会话 id

### Requirement: Failover 与错误分类
本地请求形状错误（`InvalidRequestError` 包裹）MUST 返回 400 并停止 failover；传输层/上游 5xx/限流 MUST 按统一 failover 状态机换号；客户端可见状态码：invalid→400、transient→502。

#### Scenario: 本地校验失败
- **WHEN** adapter 在请求构建阶段失败（validate/空 model/图片校验）
- **THEN** 返回 400，MUST NOT 选择下一个账号

#### Scenario: 传输失败
- **WHEN** 上游连接错误/超时/5xx
- **THEN** 按 failover 策略尝试下一个账号；耗尽后返回 502/503

### Requirement: Responses WebSocket 传输
`GET /v1/responses`（Upgrade: websocket）MUST 支持子协议 `responses_websockets=2026-02-06`；每回合 MUST 走与 HTTP 相同的计费/并发/failover 管线（内部伪造 gin.Context 复用 forward）；会话状态机 MUST 支持 previous_response_id 增量 transcript、待决 tool call 门控与 replacement replay；入站并发/首包超时/读限 MUST 复用 OpenAIWS 配置。

#### Scenario: 增量回合
- **WHEN** WS 会话中携带 previous_response_id 的后续请求
- **THEN** 状态机归并 transcript（call_id keep-first / id keep-last）并以流式转发

#### Scenario: 待决 tool call 门控
- **WHEN** 上一回合遗留 pending tool call 且新请求未提供对应 output
- **THEN** 返回错误事件，不向上游发起

### Requirement: 计费接入
成功回合 MUST 异步提交 usage（token 计数、rate_multiplier）；WS 与 HTTP 路径 MUST 走同一 submit 逻辑；`result.Stream` 恒为 true（入站强制流式）。

#### Scenario: 成功请求计费
- **WHEN** 流正常完成
- **THEN** 提交含 input/output tokens 的 usage 记录

### Requirement: 复合分组路由
复合平台检测 MUST 识别 `devin`/`cognition` 平台别名与 `swe-*`/`devin-*` 模型前缀为 `devin` 目标；`claude-*`/`gpt-*` 形态的 devin 目录模型 MUST 依赖显式路由配置（不与现有前缀规则冲突）。

#### Scenario: 前缀命中
- **WHEN** 复合分组的 public model 为 `swe-2`
- **THEN** 目标平台解析为 devin
