## Context

当前代码已经具备图片价格字段和部分图片转发能力，但边界不完整：

- `backend/ent/schema/group.go` 只有 `rate_multiplier` 和 `image_price_1k/2k/4k`，没有分组级生图能力开关，也没有“图片是否共享分组倍率”的开关。
- `backend/internal/handler/openai_images.go` 在解析 `/v1/images/*` 后只做通用余额/订阅资格检查，没有检查分组是否允许生图。
- `backend/internal/service/openai_gateway_service.go` 对 Codex CLI 会自动注入 `image_generation` tool；通用 `/v1/responses` 只记录日志，没有把图片工具产物数量写入 `OpenAIForwardResult.ImageCount`。
- `backend/internal/service/billing_service.go` 的 `CalculateImageCost` 当前使用 `image_price_* * image_count * rate_multiplier`。这个行为本身可以作为默认兼容模式，但普通编码分组 `rate_multiplier=0.15` 且希望图片最终价为 `0.2/张` 时，管理员必须填写 `image_price=0.2/0.15`，不可读且不适合长期运营。
- `backend/internal/service/openai_gateway_service.go` 和 `backend/internal/service/gateway_service.go` 的渠道图片计费路径当前传 `RequestCount: 1`，多图请求会按 1 次收费。
- `backend/internal/service/openai_images.go` 的 OpenAI 图片尺寸分层此前只覆盖少量固定尺寸；`gpt-image-2` 官方文档已经支持满足约束的自定义 `size`，因此本地计费必须能够对未知尺寸做稳定分档，同时不能因为本地映射不认识就提前拦截请求。

用户澄清后的业务要求是：普通编码分组可以关闭生图，也可以开启生图；开启后默认继续共享现有分组倍率以保持兼容，但管理员可以打开“生图倍率独立”开关，改用单独的图片倍率输入框。图片分组是推荐的运营隔离方式，但不是唯一承载方式。

## Goals / Non-Goals

**Goals:**
- 分组具备明确的 `allow_image_generation` 开关，所有已知生图入口在调度上游前执行同一个权限判断。
- 分组具备“生图倍率是否独立”的开关；默认 `false`，即共享当前代码里的有效分组倍率。
- 生图倍率独立开关打开后，图片费用使用单独的 `image_rate_multiplier`，不再使用普通编码分组的倍率。
- 保留现有 `image_price_1k/2k/4k` 字段作为图片单价配置，不强制把它们迁移成新的语义。
- 普通编码分组在 `allow_image_generation=false` 时仍可正常使用 `gpt-5.4` / `gpt-5.5` 文本能力，但不能使用图片工具。
- 普通编码分组在 `allow_image_generation=true` 时可使用 `gpt-5.4` / `gpt-5.5 + image_generation`，且按实际图片数量收费。
- 通用 `/v1/responses`、OpenAI Images API、流式、非流式、透传路径全部把成功产出的图片数量写入 `ImageCount`。
- 渠道 `billing_mode=image` 使用真实 `ImageCount`，不再固定按 1 次收费。

**Non-Goals:**
- 不引入新的第三方依赖。
- 不改变 OpenAI 上游协议；只在现有请求转发、响应解析和计费归因层补齐控制。
- 不把“图片分组”做成唯一安全边界；分组开关和图片计费逻辑必须适用于任意开启生图的分组。
- 不在本变更中实现预扣费/资金冻结；失败请求仍不收费，成功请求按实际产物后扣费。
- 不改变默认历史图片价格行为；默认共享现有有效倍率，历史 `图片价格 * 分组/用户有效倍率` 的扣费行为保持。
- 不在本变更中新增用户级图片独立倍率覆盖；用户专属普通倍率只在共享倍率模式下继续影响图片。

## Decisions

### 0. 兼容性优先原则

本变更的默认行为必须以“不改变现有已配置分组的最终扣费”为优先级：

- 迁移不修改现有 `image_price_1k/2k/4k`。
- 迁移把所有现有分组设置为 `image_rate_independent=false`，因此现有图片路径继续使用当前有效分组倍率。
- 管理员不传新字段更新分组时，不得覆盖已保存的 `allow_image_generation`、`image_rate_independent`、`image_rate_multiplier`。
- 前端编辑旧分组时必须回显服务端值；不能因为表单默认值把旧分组从共享倍率误改成独立倍率，或把允许生图误改成禁止生图。
- 只有管理员显式打开 `image_rate_independent` 后，图片扣费才从共享倍率切换到图片独立倍率。

### 1. 分组字段与迁移策略

新增三个分组字段，对应“2 个开关 + 1 个输入框”：

- `allow_image_generation BOOLEAN NOT NULL DEFAULT false`
- `image_rate_independent BOOLEAN NOT NULL DEFAULT false`
- `image_rate_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0`

字段语义：

- `allow_image_generation`：是否支持当前分组生图。
- `image_rate_independent=false`：图片计费共享当前普通计费链路里的有效倍率，即当前 `userGroupRateResolver.Resolve(ctx, user.ID, groupID, group.RateMultiplier)` 得到的倍率；这保持现有行为。
- `image_rate_independent=true`：图片计费使用 `group.image_rate_multiplier`；普通编码的 `rate_multiplier` 和用户专属普通倍率不参与图片扣费。
- `image_price_1k/2k/4k`：继续表示图片基础单价，由选中的图片倍率模式继续相乘。

新建分组默认 `allow_image_generation=false`，避免新普通编码分组意外获得生图能力。为避免升级后立即打断已有图片业务，迁移对现有 `openai`、`gemini`、`antigravity` 分组回填 `allow_image_generation=true`，`anthropic` 分组保持 `false`。该回填只是兼容现状；上线后管理员必须按业务策略关闭不允许生图的普通编码分组。

迁移不改写已有 `image_price_1k/2k/4k`，并将所有现有分组设为 `image_rate_independent=false`、`image_rate_multiplier=1`。这样现有最终扣费公式保持不变：

```text
历史/默认模式图片最终扣费 = image_price_* * image_count * 当前有效分组倍率
```

普通编码分组 `rate_multiplier=0.15` 且希望图片 1K 最终扣费 `0.2/张` 时，管理员不再需要填写 `0.2/0.15`，而是设置：

```text
image_rate_independent = true
image_rate_multiplier = 1
image_price_1k = 0.2
```

如果希望图片也打折，例如图片标价 `0.2/张`、图片折扣 `0.8`，则设置：

```text
image_rate_independent = true
image_rate_multiplier = 0.8
image_price_1k = 0.2
```

### 2. 生图意图统一识别

新增一个服务层 helper，输入至少包含 endpoint、请求模型、请求体，输出是否为生图意图：

```text
isImageGenerationIntent =
  endpoint 是 /v1/images/generations 或 /v1/images/edits
  OR requested model 以 gpt-image- 开头
  OR tools[] 存在 type == image_generation
  OR tool_choice 显式指向 image_generation
```

生图意图判断必须在请求体被 Codex 注入、模型改写、渠道映射改写之前执行一次，并在这些改写之后再对最终请求体执行一次。原因是当前代码会在 `backend/internal/service/openai_gateway_service.go` 中注入 `image_generation` tool，也会在 `normalizeOpenAIResponsesImageOnlyModel` 中把 `gpt-image-*` 改写为文本模型 + 图片工具；只检查改写前或只检查改写后都可能漏掉场景。

`tool_choice` 判断只把明确指向 `image_generation` 的值视为生图意图；`auto`、`none`、`required` 本身不构成生图意图，但如果 `tools[]` 中存在 `image_generation`，仍由 `tools[]` 规则命中。

该判断必须在以下位置使用：

- `/v1/images/*` handler 解析请求后、账号调度前。
- `/v1/responses` 解析 body 后、Codex 自动注入 `image_generation` tool 前。
- `normalizeOpenAIResponsesImageOnlyModel` 把 `gpt-image-*` 改写为 Responses 文本模型前。
- OpenAI 高级 scheduler 入口保留现有账号能力检查，同时补齐渠道 restriction 检查，避免启用高级调度时绕过渠道模型限制。

当 `allow_image_generation=false` 时：

- 显式生图意图返回 HTTP 403，错误类型使用现有 `permission_error` 风格。
- Codex CLI 请求不自动注入 `image_generation` tool，也不追加图片桥接指令；如果请求没有显式生图意图，则继续按普通文本请求处理。

### 3. gpt-5.4 / gpt-5.5 生图承载方式

`gpt-5.4` / `gpt-5.5` 生图通过现有 OpenAI Responses API 的 `image_generation` tool 承载，不新增专用 endpoint：

```json
{
  "model": "gpt-5.4",
  "input": "生成一张图片",
  "tools": [
    {
      "type": "image_generation",
      "model": "gpt-image-2",
      "size": "1024x1024",
      "output_format": "png"
    }
  ],
  "tool_choice": { "type": "image_generation" }
}
```

`model=gpt-image-*` 发到 `/v1/responses` 时保留现有改写方向：主模型改为 Responses 文本模型，图片模型放入 `image_generation` tool。计费时如果能从工具配置得到 `gpt-image-*`，图片默认价格按该图片模型解析；如果工具未指定图片模型，则使用当前转发结果的 billing model，并优先使用分组/渠道配置价格。

### 4. 图片数量归因

新增统一图片输出解析 helper，返回去重后的图片数量和可用图片元信息。必须覆盖以下已有或可借鉴的事件形态：

- 非流式 Responses JSON：`output[]` 中 `type == image_generation_call` 且 `result` 非空。
- Responses SSE：`response.output_item.done` 中 `item.type == image_generation_call` 且 `item.result` 非空。
- Responses SSE 完成事件：`response.completed.response.output[]` 中图片工具结果。
- Images API 非流式：顶层 `data[]`。
- Images API 流式：顶层 `data[]`、`image_generation.completed`、`response.output_item.done`、`response.completed`。

去重键按优先级使用 `item.id`、`call_id`、`result` 内容 hash。只统计最终图片，不统计 `partial_image`。

`openaiStreamingResult` 增加 `imageCount`、`imageSize`、`imageBillingModel`。`handleStreamingResponse`、`handleStreamingResponsePassthrough`、`handleNonStreamingResponse`、`handleNonStreamingResponsePassthrough` 都必须把解析结果带回 `OpenAIForwardResult`。当 `ImageCount > 0` 时，即使上游 usage 为 0，也必须写 usage log 并进入图片计费。

### 5. 图片价格公式

图片计费先确定单价，再确定倍率：

```text
unit_price = 渠道 image 模式价格 或 分组 image_price_* 或 默认图片价格
image_multiplier =
  如果 group.image_rate_independent == true: group.image_rate_multiplier
  否则: 当前有效分组倍率
total_cost = unit_price * image_count
actual_cost = total_cost * image_multiplier
```

“当前有效分组倍率”必须沿用当前代码的倍率解析方式：默认配置倍率 → 分组 `rate_multiplier` → 用户专属分组倍率覆盖。这样 `image_rate_independent=false` 时完全保留当前行为。

`billing_mode=image` 的渠道价格是图片单价来源之一，仍优先于分组图片价格。图片渠道价格也必须按 `ImageCount` 计数，并使用同一套 `image_multiplier` 选择逻辑。

`billing_mode=per_request` 的非图片请求保持当前普通按次语义，继续使用普通 token 倍率；只有已经识别为图片请求且 `ImageCount > 0` 的路径使用图片计费逻辑。

`usage_logs.rate_multiplier` 继续表示“本次扣费实际使用的倍率”。因此：

- token 日志记录普通 token 有效倍率。
- image 日志在共享模式记录普通有效倍率。
- image 日志在独立模式记录 `image_rate_multiplier`。

专用 `/v1/images/*` 仍按图片请求语义计费：当 `ImageCount > 0` 时，图片价格决定费用，伴随的上游 token usage 只记录不额外计 token 费用。这保持当前 Images API 的行为。

通用 `/v1/responses + image_generation` 的混合文本+图片输出存在一个明确取舍：如果继续沿用“`ImageCount > 0` 时只按图片计费”的当前计费分支，用户可以在一次图片请求中夹带大量文本输出而只付图片费用；如果改成“图片费用 + 非图片 token 费用”，会改变当前 `billing_mode=image` 的单一计费语义，并可能让渠道图片单价不再是全包价格。本变更为最大兼容性不引入混合计费模式，但必须在 usage log 中完整记录 token 与 image_count，便于后续按数据决定是否新增 `image_plus_token` 计费模式。

### 6. 尺寸档位与参数透传

OpenAI 图片请求的 `size` 参数必须透传给上游；本地只做计费分档，不做 OpenAI 尺寸合法性校验。无论尺寸是否满足官方约束，本地都不能因为未知尺寸或 provider-invalid 尺寸返回 400；如果上游不接受该尺寸，由上游响应错误。

官方 `gpt-image-2` 文档给出的常用尺寸与约束是本地计费分档的依据：

- 常用尺寸：`1024x1024`、`1536x1024`、`1024x1536`、`2048x2048`、`2048x1152`、`3840x2160`、`2160x3840`、`auto`。
- 自定义尺寸：官方支持满足约束的任意 `size`，包括边长、16 像素倍数、长短边比例、总像素范围等约束。
- `2560x1440` 是 2K/QHD 参考边界；超过 `2560x1440` 总像素的输出进入更高档位风险区。

OpenAI 图片尺寸分层必须按以下规则：

```text
empty, auto                       => 2K
1024x1024                         => 1K
1536x1024, 1024x1536              => 2K
1792x1024, 1024x1792              => 2K
2048x2048, 2048x1152, 1152x2048   => 2K
3840x2160, 2160x3840              => 4K
未知且无法解析为正整数 WIDTHxHEIGHT => 2K
未知且 WIDTH * HEIGHT <= 2560*1440 => 2K
未知且 WIDTH * HEIGHT > 2560*1440  => 4K
```

这个规则只决定 `ImageSize` 和扣费档位，不修改请求体，不删除未知参数，不把未知尺寸改写成预设尺寸。

## Risks / Trade-offs

- 历史普通编码分组迁移后仍默认允许生图 → 通过管理员可见开关、上线核对清单和新建分组默认关闭来控制；代码无法可靠判断“普通编码分组”和“图片分组”的业务意图。
- 默认共享现有有效倍率仍保留“图片最终价不直观”的问题 → 这是兼容性选择；需要直观设置图片最终价的分组必须打开 `image_rate_independent`。
- 独立图片倍率不会读取用户专属普通倍率 → 这是目标行为；如需要用户级图片独立倍率，应作为后续独立需求实现。
- 通用 Responses 图片工具可能同时输出文本和图片 → 本变更默认仍按图片请求语义计费并完整记录 token；若业务要求文本也收费，应新增独立的混合计费模式，不能混入本次兼容性变更。
- 本地不再拦截未知或 provider-invalid OpenAI 尺寸 → 非法尺寸会消耗一次上游请求失败成本和用户体验往返，但这是为了保证参数透传、兼容官方新增尺寸和第三方兼容提供商；计费只在成功产出最终图片后发生。
- Responses 流式解析需要在客户端断开后继续 drain 上游以完成计费 → 沿用当前流式处理“客户端断开后继续读取上游用于计费”的模式，并只新增轻量 JSON 路径提取。
- 预扣费不在本变更中实现 → 继续使用现有成功后扣费模型，避免失败请求退款、流式中断退款和图片数量未确定时预估错误。

## Migration Plan

1. 新增数据库迁移，添加 `groups.allow_image_generation`、`groups.image_rate_independent` 和 `groups.image_rate_multiplier`。
2. 回填现有分组：`openai`、`gemini`、`antigravity` 的 `allow_image_generation=true`，`anthropic=false`；所有现有分组 `image_rate_independent=false`、`image_rate_multiplier=1`。
3. 不改写现有 `image_price_1k/2k/4k`，保持默认共享倍率模式下的历史扣费结果。
4. 更新 Ent schema 与生成代码，更新后端 service/handler DTO 和前端类型。
5. 先接入权限判断，确保未开启生图的分组不会到达上游。
6. 再接入图片数量解析和图片计费倍率选择，确保开启生图的分组按图片数量收费。
7. 最后更新前端管理界面、i18n、文档和测试。
8. 回滚时只能通过新迁移回滚字段行为；不能修改已应用迁移文件。

## Open Questions

无。当前方案不依赖未确认的上游新尺寸、新模型或新 endpoint。
