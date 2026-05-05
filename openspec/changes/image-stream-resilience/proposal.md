## Why

图片流式路径目前没有和普通 Responses 流式一致的断连续写策略，也没有独立于普通流式的超时控制。由于图片生成耗时更长，如果继续沿用普通流式处理方式，客户端断开时容易中断上游读取，影响图片产物收集与按图计费的准确性。

## What Changes

- 为 OpenAI Images API 和 `Responses + image_generation` 流式路径补充独立的上游续读策略，客户端断开后继续 drain 上游，尽量保留最终图片结果和计费结果。
- 为图片流式路径使用独立的流数据间隔超时与 keepalive 策略，默认比普通流式更长，不新增页面配置项。
- 保持现有普通流式配置与行为不变，避免影响已经配置好的普通文本分组。
- 让图片流式路径在超时、断连、写入失败等场景下保持图片计费语义一致。

## Capabilities

### New Capabilities
- `image-stream-resilience`: 图片流式路径的断连续读、独立超时和计费保留能力。

### Modified Capabilities
- `image-generation-billing-accounting`: 图片流式结果计数和计费结果的稳定性行为发生改变，但计费契约不变。

## Impact

- 影响 `backend/internal/service/openai_images.go` 和 `backend/internal/service/openai_images_responses.go` 的流式实现。
- 影响 `backend/internal/config/config.go` 与 `deploy/config.example.yaml` 中图片流式默认值和校验逻辑。
- 影响 `backend/internal/service/openai_images_test.go`、`backend/internal/config/config_test.go` 以及新增的图片流式稳定性测试。
- 不新增前端页面设置，不改变普通流式配置项名称和语义。
