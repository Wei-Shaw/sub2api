## Why

当前代码把“能否生图”和“如何按图片收费”混在模型、分组倍率、渠道定价与 Responses 工具调用里，导致 OpenAI 普通编码分组在允许 `gpt-5.4` / `gpt-5.5` 时也能通过 `image_generation` tool 产图，并且通用 `/v1/responses` 产图不会稳定写入 `ImageCount`。需要把生图能力、图片倍率模式、图片产出数量归因拆成独立能力，保证普通编码分组可按业务开关生图，开启后既能沿用现有倍率行为，也能按需切换到图片独立倍率。

## What Changes

- 新增分组级生图能力开关，明确控制 `/v1/images/*`、`gpt-image-*`、显式 `image_generation` tool、Codex 自动注入图片工具等所有生图入口。
- 新增分组级图片倍率模式开关，默认继续共享现有分组有效倍率；打开独立模式后使用图片独立倍率输入框。
- 保留现有 `image_price_1k/2k/4k` 图片价格配置；图片最终扣费由“图片价格 × 当前倍率模式选出的倍率 × 图片数量”决定。
- 统一统计 OpenAI Responses 图片工具产物数量，使 `gpt-5.4` / `gpt-5.5` 通过 `image_generation` tool 产图时进入图片计费，而不是退化成普通 token 计费或无 usage 时不计费。
- 修正专用 Images API 与渠道图片计费场景，按实际图片数量和明确尺寸档位计费，避免固定 `RequestCount=1` 或未知尺寸静默落到 `2K`。
- 更新后台分组配置、前端类型、使用说明和测试，覆盖普通编码分组关闭生图、普通编码分组开启生图、独立图片分组承载、生图流式/非流式等场景。

## Capabilities

### New Capabilities
- `image-generation-access-control`: 定义分组级生图能力开关、所有生图意图识别规则、拒绝行为与 Codex 自动注入规则。
- `image-generation-billing-accounting`: 定义图片倍率模式、图片数量归因、尺寸档位、渠道图片价格和用量日志要求。

### Modified Capabilities
- 无。

## Impact

- Backend schema/API: `backend/ent/schema/group.go`、Ent 生成代码、数据库迁移、管理员分组 create/update/list DTO、分组缓存/序列化。
- Backend request gates: `backend/internal/handler/openai_images.go`、`backend/internal/service/openai_gateway_service.go`、`backend/internal/service/openai_codex_transform.go`、OpenAI account scheduler 相关模型/图片能力调度入口。
- Backend billing: `backend/internal/service/billing_service.go`、`backend/internal/service/openai_gateway_service.go`、`backend/internal/service/gateway_service.go`、usage log 与 account stats 成本计算路径。
- Frontend admin: `frontend/src/types/index.ts`、`frontend/src/views/admin/GroupsView.vue`、相关 i18n 文案与图片计费展示。
- Tests: OpenAI Images API、OpenAI Responses stream/non-stream/passthrough、分组开关、图片倍率模式、渠道图片计数、尺寸档位与 usage log 断言。
