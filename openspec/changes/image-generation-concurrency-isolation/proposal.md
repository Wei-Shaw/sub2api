## Why

图片生成流式请求会比普通文本流式请求占用更长的连接、goroutine、HTTP 上游连接和账号/用户槽位。当前图片能力已经具备独立计费与更长流式超时，但仍缺少默认关闭的图片专属并发隔离开关，图片高并发时仍可能挤压普通文本流式接口。

## What Changes

- 新增服务级图片独立并发开关，默认关闭，不改变现有已部署分组和普通文本请求行为。
- 新增图片全局并发上限配置；开启后仅限制已明确是图片生成意图的请求。
- 新增图片并发满载后的溢出策略配置：默认立即拒绝，也可配置等待槽位和等待超时。
- 将图片并发限制覆盖 `/v1/images/generations`、`/v1/images/edits` 和 `/v1/responses` 显式图片生成请求。
- 保留当前图片生成开关、图片计费、图片流式续读与超时语义。
- 不在本次代码实现外部独立图片网关；只把外部网关拆分方案沉淀到本地文档。

## Capabilities

### New Capabilities
- `image-generation-concurrency-isolation`: 图片生成请求的独立并发开关、并发上限、429 行为和外部网关落地建议。

### Modified Capabilities
- `image-stream-resilience`: 图片流式续读能力在独立并发开启时受到图片专属并发上限保护，但流式续读与计费契约不变。

## Impact

- 影响 `backend/internal/config/config.go` 的 gateway 配置字段、默认值和校验。
- 影响 `backend/internal/handler/openai_images.go` 与 `backend/internal/handler/openai_gateway_handler.go` 的图片请求入口限流。
- 影响 `deploy/config.example.yaml` 的示例配置与说明。
- 影响后端测试：配置默认值/校验、图片接口限流、Responses 显式 image tool 限流。
- 新增或更新 `2ue` 本地分析文档，记录外部独立图片网关只作为后续部署方案，不在本次代码落地。
