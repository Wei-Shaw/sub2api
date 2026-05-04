## Overview

本次只实现“图片独立并发开关”，不实现外部图片网关的运行时代码。目标是在最大程度不改变现有行为的前提下，为图片流式长连接提供服务级资源保护。

## Current Constraints

- 当前 Redis 并发槽位只有用户和账号维度，键语义是 `concurrency:user:*` 与 `concurrency:account:*`。
- 图片接口和普通 Responses 在同一个 Go 服务内运行，共享进程、HTTP 上游连接池和账号调度。
- Codex OAuth 路径会自动注入 `image_generation` tool；这个注入表示“模型具备工具能力”，不等价于当前请求一定会生图。
- `/v1/responses` 在 handler 入口只能可靠识别显式图片意图：image 模型、请求体已有 image tool、或 tool_choice 明确选择 image_generation。
- 图片实际产物计数与计费仍以 service 层的最终输出解析为准。

## Decisions

### 1. 默认关闭，保持兼容

新增配置：

- `gateway.image_concurrency.enabled`，默认 `false`。
- `gateway.image_concurrency.max_concurrent_requests`，默认 `0`，表示不限制。
- `gateway.image_concurrency.overflow_mode`，默认 `reject`，可选 `reject` / `wait`。
- `gateway.image_concurrency.wait_timeout_seconds`，默认 `30`，仅 `overflow_mode=wait` 生效。
- `gateway.image_concurrency.max_waiting_requests`，默认 `100`，仅 `overflow_mode=wait` 生效，限制当前进程内图片等待队列。

只有当 `enabled=true` 且 `max_concurrent_requests>0` 时才启用图片独立并发限制。默认配置不改变任何现有流量行为。

### 2. 进程级信号量作为第一阶段隔离

本次使用进程内有界信号量做服务级图片并发限制。原因：

- 不扩展现有 Redis `ConcurrencyCache` 接口，避免影响用户/账号并发的既有语义。
- 不新增迁移，不改变分组已有字段。
- 单实例部署可立即保护进程资源。
- 多实例部署时该限制按实例生效；文档必须明确总图片并发约等于 `实例数 × max_concurrent_requests`。

### 3. 限制对象只包含明确图片意图

纳入限制：

- `/v1/images/generations`
- `/v1/images/edits`
- `/v1/responses` 中入口请求已明确包含图片意图：image 模型、`tools[].type=image_generation`、`tool_choice` 明确选择 image_generation。

暂不纳入限制：

- 普通 Codex 请求因为服务端自动注入 image tool 而具备生图能力，但入口请求本身未明确要求生图。

这样避免把普通编码请求错误算作图片并发。后续若要对“模型运行中动态调用 image tool”做更细粒度隔离，需要在工具调用实际发生时获得可阻塞的事件，目前当前代码没有这种入口级阻塞点。

### 4. 限流行为

- `overflow_mode=reject` 时，未开始流式响应直接返回 HTTP `429`，错误类型 `rate_limit_error`。
- `overflow_mode=wait` 时，请求在当前进程内等待图片并发槽位，超过 `wait_timeout_seconds` 或超过 `max_waiting_requests` 后返回 HTTP `429`。
- 已开始流式响应时，使用现有 `handleStreamingAwareError` 写 SSE 错误事件。
- 图片并发限制命中或等待超时不触发账号 failover，不记录为上游账号失败。
- `gateway.image_stream_data_interval_timeout` 是上游图片流数据空闲超时，不用于图片排队等待。

### 5. 与外部图片网关的关系

本次不实现外部图片网关代码。外部网关方案沉淀到 `2ue` 文档：

- 推荐由 Caddy/Nginx/API Gateway 按 `/v1/images/*` 分流。
- `/v1/responses` 的图片 tool 请求不能仅靠 path 分流，必须在前置层读取 body 或保留主服务兜底。
- 即使未来拆出图片网关，主网关仍保留图片 intent 检测、开关和计费兜底，避免直连或漏判绕过。

## Risks And Mitigations

- 风险：进程级限制在多实例部署下不是全局严格限制。缓解：文档明确容量计算，后续可基于 Redis 扩展为集群级图片并发。
- 风险：Codex 自动注入 image tool 后，普通编码请求未被图片限流。缓解：这是有意选择，避免误伤普通请求；实际输出图片仍按图片计费。
- 风险：图片请求在账号槽位前被拒绝可能改变排队体验。缓解：仅当独立开关启用时生效，默认关闭；429 明确提示图片并发达到上限。
