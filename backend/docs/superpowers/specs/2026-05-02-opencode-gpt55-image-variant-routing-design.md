# OpenCode GPT-5.5 Image 变体与生图账号路由设计

## 背景

当前 `sub2api-openai` 的 OpenCode 推荐配置会给所有 `gpt-5.5*` 模型写入 `metadata.builtin_tools.image_generation`，因此普通 GPT-5.5 会话在请求生效时也会启用 OpenAI Responses `image_generation` built-in tool。

这在 GPT-5.5 开放给免费层级账号后会产生资源挤占问题：免费层级账号可以跑主模型 LLM，但只有付费层级账号可以使用 `image_generation` / `gpt-image-2`。如果默认配置无条件启用生图能力，大量不需要生图的普通 GPT-5.5 请求会被迫竞争少量付费生图账号。

前一轮已经完成 OpenCode `image_generation_call` 输出重写、短期下载链接、服务端 continuation 和 marker rehydrate。本设计只处理“什么时候启用生图工具”和“启用后如何路由到正确账号”。

## 目标

1. 普通 `gpt-5.5*` 默认不启用 `image_generation`，避免默认挤占付费生图账号。
2. 新增 `Image` 语义入口，并与现有 `Fast`、`Sys` 叠加，例如 `GPT-5.5 Image Fast (Sys)`。
3. 只有新格式显式声明的 `image_generation.enabled === true` 才启用生图工具。
4. 旧配置文件中的旧 `image_generation` 写法自然失效，不需要逐个提醒用户更新配置。
5. 当请求确实启用生图工具时，后端只路由到同时支持主模型和 `gpt-image-2` 的账号。
6. 默认 OpenCode 推荐配置增加一个专门的生图子代理，使用 GPT-5.5 Image Fast (Sys) 语义。
7. `web_search` 保持可用，且仍与 `image_generation` 使用同一个 `metadata.builtin_tools` 配置族。

## 非目标

1. 不修改 OpenCode 源码。
2. 不恢复或长期保存已过期的旧 `sub2api-image://...` marker 图片字节。
3. 不改变 `web_search` 的旧配置兼容性。
4. 不把所有 GPT-5.5 请求强行导向付费账号。
5. 不新增数据库表记录生图能力；优先复用账号 `Extra`、能力 projection 和现有调度约束。
6. 不在本设计中重做 OpenCode `image_generation_call` 输出重写逻辑。

## 已确认的 OpenCode 配置语义

本设计参考了本地 OpenCode 源码与官方文档：

1. OpenCode Agents 文档（`https://opencode.ai/docs/agents/`）说明 agent 可在 `opencode.json` 的 `agent` 下配置，并支持 `mode`、`description`、`model` 等字段；模型配置使用 `provider/model-id` 格式。
2. OpenCode Agents 文档说明未显式设置模型的 subagent 会使用调用它的 primary agent 模型。
3. OpenCode Config 文档（`https://opencode.ai/docs/config`）说明 `default_agent` 控制未显式指定时使用的 agent；本设计新增的是 subagent，不会作为默认 primary agent。
4. 本地源码 `agent.ts` 中 `Agent.Info` 支持 `model` 和 `variant` 字段，`config.ts` 中 `Agent` 配置 schema 也支持 `variant`。
5. 本地源码 `task.ts` 中 Task 工具会优先使用子代理自己的 `agent.model`；如果子代理没有配置 model，才继承当前 assistant 消息的模型。
6. 本地源码 `session/prompt.ts` 中 `agent.variant` 只有在 agent 配置的 model 与当前请求模型相同时才会自动落到 user message 的 `variant`。
7. 本地源码 `session/llm.ts` 中最终请求 options 的合并顺序是 provider/model/agent/variant，variant 可以覆盖或补充 `metadata.builtin_tools`。
8. OpenCode variant 当前没有独立 display name；TUI 里通常显示 variant key，例如 `image`，ACP/模型展示最多表达为基础模型名加 variant。若产品必须显示完整 `GPT-5.5 Image Fast (Sys)` 模型名，需要额外物化独立模型 key。

因此推荐配置可以安全生成一个 `image` subagent：

```json
{
  "agent": {
    "image": {
      "mode": "subagent",
      "description": "Generate images with GPT-5.5 Image Fast (Sys) and immediately download the result before the temporary URL expires.",
      "model": "sub2api-openai/gpt-5.5-fast-Sys",
      "variant": "image"
    }
  }
}
```

## 新 builtin_tools 配置契约

### 一、统一路径

继续使用现有统一路径：

```json
{
  "metadata": {
    "builtin_tools": {
      "web_search": true,
      "image_generation": {
        "enabled": true,
        "model": "gpt-image-2",
        "output_format": "png"
      }
    }
  }
}
```

这样 `web_search` 和 `image_generation` 仍属于同一个 carrier，不引入 `metadata.sub2api...` 这类分裂路径。

### 二、image_generation 门槛

只有以下条件同时满足时，后端才启用生图：

1. `metadata.builtin_tools.image_generation` 或顶层 `builtin_tools.image_generation` 是对象。
2. 该对象中 `enabled` 严格为 boolean `true`。
3. `model` 经过字符串 trim 与小写规范化后严格等于 `gpt-image-2`。

推荐只在 `metadata.builtin_tools` 中生成新配置；顶层 `builtin_tools` 仅作为后端一致解析路径处理，不作为 OpenCode 推荐输出。

`enabled` 是 sub2api 本地 carrier 字段，只用于启用判断。生成上游 `tools[]` 中的 `image_generation` tool 时必须删除 `enabled`，并只白名单复制 OpenAI 支持的字段，例如 `model`、`size`、`quality`、`background`、`output_format`、`output_compression`、`moderation`、`style`、`partial_images`、`input_fidelity`。未知字段必须丢弃。

### 三、双 carrier 优先级

当顶层 `builtin_tools` 与 `metadata.builtin_tools` 同时存在时，沿用当前“顶层优先”的解析规则：

1. 顶层 `builtin_tools` 存在时，作为唯一 augmentation 来源。
2. `metadata.builtin_tools` 不与顶层 carrier 合并。
3. 转发上游前仍同时剥离顶层 `builtin_tools` 和整段 `metadata`。
4. 该规则同时适用于 `web_search` 和 `image_generation`。

需要测试锁定以下组合：顶层旧 image + metadata 新 image、顶层新 image + metadata 旧 image、顶层 false + metadata web_search。

### 四、旧格式自然失效

以下旧写法必须被剥离，但不启用生图：

```json
{"metadata":{"builtin_tools":{"image_generation":true}}}
```

```json
{"metadata":{"builtin_tools":["web_search","image_generation"]}}
```

```json
{"metadata":{"builtin_tools":{"image_generation":{"model":"gpt-image-2","output_format":"png"}}}}
```

第三种对象形态缺少 `enabled: true`，也必须自然失效。这个门槛用于让已分发的旧推荐配置停止默认启用生图能力。

无效的 `image_generation` 只让该字段失效，不能影响同一 carrier 中的 `web_search`。例如：

```json
{"metadata":{"builtin_tools":{"web_search":true,"image_generation":{"model":"gpt-image-2"}}}}
```

必须只生成 web search tool，不生成 image generation tool，并在上游请求中剥离 `metadata`。

### 五、web_search 兼容性

`web_search` 保持当前语义：

1. `builtin_tools: true` 如果当前等价于启用默认内建工具，则继续只启用 `web_search`，不得隐式启用 `image_generation`。
2. `metadata.builtin_tools.web_search: true` 继续启用。
3. 顶层 `builtin_tools.web_search: true` 继续启用。
4. 数组形式中的 `"web_search"` 如果当前已支持，继续支持。
5. 消费 carrier 后仍按当前规则从上游请求中剥离私有字段，避免把 `metadata` 原样透传给 OpenAI Responses。

## OpenCode 模型与变体设计

### 一、普通 GPT-5.5 不带生图

`openCodeBuiltinToolsForModel(id)` 不再因为 `id` 以 `gpt-5.5` 开头就自动加入 `image_generation`。普通模型只保留：

```json
{
  "web_search": true
}
```

覆盖范围包括：

1. `gpt-5.5`
2. `gpt-5.5-fast`
3. `gpt-5.5-Sys`
4. `gpt-5.5-fast-Sys`

### 二、新增 Image 变体

对 GPT-5.5 系列模型新增 `image` 变体。变体要与现有 Fast 和 Sys 模型叠加，而不是替换它们。

推荐展示语义：

1. `gpt-5.5` + `image` -> `GPT-5.5 Image`
2. `gpt-5.5-fast` + `image` -> `GPT-5.5 Image Fast`
3. `gpt-5.5-Sys` + `image` -> `GPT-5.5 Image (Sys)`
4. `gpt-5.5-fast-Sys` + `image` -> `GPT-5.5 Image Fast (Sys)`

实现上优先使用 OpenCode 的 `variants`，在目标模型的 `variants.image` 写入：

```json
{
  "metadata": {
    "builtin_tools": {
      "web_search": true,
      "image_generation": {
        "enabled": true,
        "model": "gpt-image-2",
        "output_format": "png"
      }
    }
  }
}
```

OpenCode variant 没有独立 display name，因此本轮接受 UI 显示为 `image` 或 `GPT-5.5 Fast (Sys) (image)` 这类组合表达。实现计划必须在手动 QA 中确认用户能从推荐配置中选择或通过 `agent.image` 调用该入口。如果 OpenCode UI/配置无法让用户明确触发生图入口，必须物化独立模型 key 作为 fallback，例如 `gpt-5.5-image-fast-Sys`，但该 key 的底层 `id/api.id` 仍必须映射到当前网关已识别的非 fast 上游模型。

实现上必须保留已有 reasoning variants，并避免 base 模型与 `-Sys` 派生模型共享同一个 `variants` 对象。推荐顺序是：先完成 `withSysVariants`，再遍历所有 `gpt-5.5*` 和 `gpt-5.5*-Sys` 模型，用 `{ ...(model.variants ?? {}), image: openCodeImageVariant() }` 注入；或者在 `withSysVariants` 中显式 clone `options`、`headers`、`variants`。

`gpt-5.5-fast` 和 `gpt-5.5-fast-Sys` 是 OpenCode 推荐配置里的模型 key / Fast 入口，不是上游真实模型名。现有前端会把 fast key 的底层 `id` 覆写为非 fast 模型：`gpt-5.5-fast -> id: "gpt-5.5"`，`gpt-5.5-fast-Sys -> id: "gpt-5.5-Sys"`。后端 requested model、projection key、账号能力判断不得期待上游存在 `gpt-5.5-fast-Sys`。

### 三、Reasoning 与 Image 的组合

当前 GPT-5.5 仍有 reasoning variants，例如 `none`、`low`、`medium`、`high`、`xhigh`。本轮不要求一次性生成 `image-low`、`image-medium` 等组合。

原因是本轮目标是把生图能力从默认路径中移出，并提供一个明确的生图入口；如果后续确认 OpenCode variant UI 需要同时表达 reasoning 和 image，再单独扩展组合 variant。

默认 `image` 变体不改变 reasoning effort，让模型使用当前默认推理策略。

## 默认生图子代理设计

推荐配置新增 `agent.image`：

```json
{
  "agent": {
    "image": {
      "mode": "subagent",
      "description": "Generate images with GPT-5.5 Image Fast (Sys). Use this when the user asks to create, draw, render, or edit an image. Download the generated image immediately because the sub2api image URL is short-lived.",
      "model": "sub2api-openai/gpt-5.5-fast-Sys",
      "variant": "image",
      "options": {
        "store": false
      }
    }
  }
}
```

设计要点：

1. `mode: "subagent"`，不改变默认 primary agent。
2. `model` 固定为 `sub2api-openai/gpt-5.5-fast-Sys`，确保 Task 调用时使用 Image Fast (Sys) 基础模型。
3. `variant: "image"`，由 OpenCode runtime 合并到请求 options，开启新格式 `image_generation.enabled === true`。
4. `description` 明确触发场景，便于 primary agent 在用户要求画图时调用。
5. 提醒短期 URL 必须立即下载，与前一轮 continuation 设计保持一致。

## 后端路由设计

### 一、请求侧检测

新增 helper 解析本地 carrier：

1. `extractOpenAIBuiltinToolsCarrier` 继续读取顶层 `builtin_tools` 和 `metadata.builtin_tools`。
2. 新增 `extractOpenAIImageGenerationBuiltinToolConfig(raw any) (map[string]any, bool)`。
3. 只有 `enabled === true` 且 `model` trim / lower 后等于 `gpt-image-2` 时返回有效配置。
4. `normalizeOpenAIBuiltinTools` 只为有效配置生成上游 `{"type":"image_generation", ...}`。
5. 旧 image_generation 形态被剥离但不返回 image tool。
6. 如果最终上游 `tools[]` 中不存在 `image_generation`，但请求里残留 `tool_choice: {"type":"image_generation"}` 或等价字符串形式，必须删除该 `tool_choice` 并让请求继续；不得返回 4xx。这样旧配置自然失效后，普通文本或 `web_search` 请求仍能成功。

路由 requirement 不能只看本地 carrier，必须从最终有效 Responses tool set 判断：

1. 有效新 carrier 会向最终 `tools[]` 注入 `image_generation`，因此触发生图路由 requirement。
2. 请求原本已经带有 Responses 原生 `tools[]`，且其中存在 `type: "image_generation"` 时，也必须触发生图路由 requirement。
3. 原生 `image_generation` tool 的 `model` 缺失时按上游默认 `gpt-image-2` 处理；存在时经过 trim / lower 后必须等于 `gpt-image-2` 才进入本轮支持范围。
4. 旧/无效本地 image carrier 自身不触发生图路由 requirement；如果请求里同时没有原生有效 `image_generation` tool，则按普通请求调度。

### 二、Chat Completions 兼容路径

`openai_gateway_chat_completions.go` 中的 compat carrier 解析必须与 `/v1/responses` 主链使用同一个 image_generation 门槛：

1. `extractOpenAICompatBuiltinToolsCarrier` 仍读取顶层 `builtin_tools` 和 `metadata.builtin_tools`。
2. 只有对象且 `enabled === true` 且 `model == "gpt-image-2"` 时，才允许 Chat Completions 兼容转换产生 Responses `image_generation` tool。
3. 旧 `image_generation: true`、数组中的 `"image_generation"`、缺 `enabled:true` 的对象都只能被剥离，不得生成 image tool。
4. `web_search` 旧兼容保持不变。
5. metadata carrier 被消费后仍按项目基线整段剥离 `metadata`。
6. 如果 Chat Completions 请求包含有效 image carrier，默认模型 fallback 不得丢失生图约束：要么禁用默认模型 fallback 并直接返回 image-aware no-available 错误，要么 fallback 后的每一次调度仍携带 `RequiredResponsesImageGeneration`，且 `MainModel` 必须保持为用户原始请求的主模型规范化结果，而不是 fallback 目标模型。
7. 如果 Chat Completions 请求没有有效 image carrier，则默认模型 fallback 保持现有普通文本语义。

### 三、compact 与 passthrough 路径边界

本轮只在正常 `/v1/responses` 主链和实际转换为 Responses 的 Chat Completions 兼容链启用新 image carrier。

`/v1/responses/compact` 与 OpenAI passthrough 分支保持 strip-only 非目标路径：

1. 只剥离顶层 `builtin_tools` 和 `metadata.builtin_tools` / 整段 `metadata`。
2. 不执行 builtin tool augmentation。
3. 不触发 image generation continuation。
4. 不因为 image carrier 增加生图路由能力要求。
5. 不把本地私有 carrier 原样透传给上游。

如果未来要让 compact 或 passthrough 支持 Responses built-in `image_generation`，必须另写设计，因为它们的上游透传/最小 body 语义不同。

### 四、调度侧能力要求

当最终有效 Responses tool set 包含 `image_generation` 时，调度请求必须增加独立的 Responses built-in image generation 约束，不能复用 `/v1/images/*` 的 native-to-basic fallback 语义作为唯一约束。

新增请求结构建议如下：

```go
type OpenAIResponsesImageGenerationRequirement struct {
    Enabled    bool
    MainModel  string
    ImageModel string
}

type OpenAIAccountScheduleRequest struct {
    // existing fields...
    RequiredResponsesImageGeneration *OpenAIResponsesImageGenerationRequirement
}
```

约束语义：

1. `/v1/images/*` 的 `OpenAIImagesCapabilityNative` 和 `SelectAccountWithSchedulerForImages` 原有 fallback 保持不变。
2. `/v1/responses` built-in `image_generation` 不允许 native -> basic 静默降级。
3. `RequiredResponsesImageGeneration != nil` 时，账号必须同时满足主模型 `MainModel` 和生图模型 `ImageModel`。
4. sticky、`previous_response_id`、load-balance 候选过滤、fresh DB recheck、advanced scheduler、failover retry 都必须执行同一约束。
5. 该约束在 streaming / non-streaming、OpenCode continuation 第一轮和第二轮都不可降级。

建议新增更明确的 helper，而不是把所有逻辑塞进 `SupportsOpenAIImageCapability`：

```go
func (a *Account) SupportsOpenAIResponsesImageGeneration(mainModel string, imageModel string) bool
```

该 helper 至少检查：

1. `a.IsOpenAI()`。
2. 主模型按现有 `IsModelSupported(mainModel)` / projection 规则判断。
3. `imageModel == "gpt-image-2"` 必须来自明确正向证据：账号 `model_mapping` 精确命中 `gpt-image-2`、账号 `model_mapping` 中 image-specific 通配命中例如 `gpt-image-*`、`openai_capability_explicit_models`、`openai_capability_catalog_models`、或后续专门探测写入的 image-generation capability 标记。
4. 账号类型和层级允许 native image generation。

当前已知业务事实是免费层级可访问 GPT-5.5，但不能使用生图工具。因此 OAuth 账号不能再因为是 OAuth 就默认满足 native image capability。需要按 `plan_type` 或能力快照区分：

1. `plan_type == "free"` 默认不支持 built-in image generation。
2. `plan_type` 缺失的 OAuth 账号如果没有明确 `gpt-image-2` 能力，也默认不支持。
3. 付费层级账号只有在模型映射、显式能力或 catalog 支持 `gpt-image-2` 时才参与。
4. API Key 账号按显式模型映射 / capability 规则判断，不强行按 `plan_type` 过滤。
5. 宽泛通配或默认允许不能证明 `gpt-image-2` 支持：`*`、`gpt-*`、group default、空 `model_mapping` 默认允许、`IsModelSupported(imageModel)` 的 default allow、projection `DefaultAllow` 都不能作为正向证据。

### 五、主模型规范化

Responses image generation requirement 中的 `MainModel` 必须使用当前 OpenAI scheduler/projection 一致的 routing model：

1. 从请求 body 的 `model` 读取 OpenCode 请求模型。
2. 经过现有渠道映射、`-Sys` 处理和 projection key 规范化后用于调度判断。
3. fast 入口只代表 OpenCode model key / `serviceTier: "priority"` 语义，不代表上游模型名。
4. 测试必须覆盖 `gpt-5.5-fast-Sys + image`，断言后端使用 `gpt-5.5-Sys` / canonical `gpt-5.5` 路由能力，而不是期待账号声明 `gpt-5.5-fast-Sys`。

### 六、sticky、previous_response 与 failover

生图请求必须避免复用不支持生图的 sticky 账号：

1. sticky 命中后先检查生图能力。
2. 不满足时清除 sticky binding，并继续选择其他账号。
3. `previous_response_id` 命中账号后也必须执行同一 `SupportsOpenAIResponsesImageGeneration` 检查；不满足时释放 selection、清理不兼容 binding，并继续 load-balance 或返回 no-available。
4. failover 重试时保留生图能力要求，不能降级到普通 GPT-5.5 账号。
5. 对 OpenCode streaming / non-streaming continuation 请求也保留同样路由要求；服务端第二轮 continuation 必须继承第一轮 requirement，或显式复用第一轮已选账号。
6. fresh DB recheck 后如果账号失去 `gpt-image-2` 能力，必须继续 failover 或返回 no-available。

### 七、错误行为

当没有账号同时支持主模型和 `gpt-image-2` 时，返回可诊断错误，而不是静默退回普通账号：

```json
{
  "error": {
    "type": "no_available_accounts",
    "message": "No available OpenAI accounts support both gpt-5.5 and gpt-image-2 image_generation"
  }
}
```

具体错误类型可沿用现有 `ErrNoAvailableAccounts` 映射，但当 `RequiredResponsesImageGeneration` 非空时，所有 no-available 分支必须返回 image-aware 错误，例如 `noAvailableOpenAIResponsesImageGenerationAccountError(mainModel, imageModel)`，错误信息必须同时包含主模型与生图模型。

## 前端推荐配置变更

### 一、UseKeyModal

`frontend/src/components/keys/UseKeyModal.vue` 需要调整：

1. `openCodeBuiltinToolsForModel(id)` 对所有模型默认只返回 `web_search: true`。
2. 新增 `openCodeImageGenerationBuiltinTools()` 返回含 `enabled: true` 的新对象。
3. 构建 GPT-5.5 系列模型时，给 `variants.image` 合并 image carrier。
4. `-Sys` 派生后也必须保留 `variants.image`，且不得与基础模型共享同一个 `variants` 对象。
5. 新增 `agent.image` 推荐配置。
6. 本轮不新增 UseKeyModal 额外 UI 文案；如果实现中新增用户可见标题、说明或提示，必须同步更新 `zh.ts` / `en.ts` 并补测试。

### 二、测试期望

`UseKeyModal.spec.ts` 需要更新：

1. 普通 `gpt-5.5`、`gpt-5.5-fast`、`gpt-5.5-Sys`、`gpt-5.5-fast-Sys` 的 `options.metadata.builtin_tools` 只等于 `{ web_search: true }`。
2. 上述模型都存在 `variants.image`。
3. `variants.image.metadata.builtin_tools.image_generation.enabled` 为 `true`。
4. `variants.image.metadata.builtin_tools.image_generation.model` 为 `gpt-image-2`。
5. 生成配置中包含 `agent.image`，其 `mode`、`model`、`variant` 均符合设计。
6. 不再断言普通 GPT-5.5 默认包含 `image_generation`。
7. `models["gpt-5.5-fast-Sys"].variants.image` 存在，且 `agent.image.model` 指向的模型 key 存在并包含 `variants.image`。
8. GPT-5.5 系列原有 reasoning variants 仍保留。
9. fast key 与底层 `id` 的差异仍被锁定：`gpt-5.5-fast -> id: "gpt-5.5"`，`gpt-5.5-fast-Sys -> id: "gpt-5.5-Sys"`。

## 后端测试计划

### 一、builtin_tools 解析

新增或更新测试覆盖：

1. `metadata.builtin_tools.image_generation.enabled=true` 会生成上游 `image_generation` tool。
2. 顶层 `builtin_tools.image_generation.enabled=true` 同样可生成 tool，但推荐配置不使用顶层路径。
3. `image_generation: true` 不生成 tool。
4. `metadata.builtin_tools: ["web_search", "image_generation"]` 只生成 `web_search`，不生成 `image_generation`。
5. 缺 `enabled:true` 的对象不生成 tool。
6. `web_search` 的旧格式保持可用。
7. carrier 消费后仍剥离顶层 `builtin_tools` 或整段 `metadata`。
8. 同一 carrier 中无效 `image_generation` 不影响 `web_search`。
9. 顶层 carrier 与 metadata carrier 同时存在时遵循“顶层优先”。
10. 上游 `image_generation` tool 不包含本地 `enabled` 字段或未知字段。
11. 旧/无效 image carrier 失效后，指向 `image_generation` 的 `tool_choice` 会被删除，普通请求继续成功。
12. Chat Completions 兼容路径同样执行 `enabled:true` 门槛。
13. Chat Completions 有效 image carrier 覆盖默认模型 fallback：fallback 被禁用，或 fallback 后仍以原始请求主模型作为 `RequiredResponsesImageGeneration.MainModel` 并保留 image-aware no-available 错误。
14. 原生 Responses `tools[]` 中已有 `image_generation` 时，即使没有本地 carrier，也会触发 `RequiredResponsesImageGeneration`。
15. compact 与 passthrough 路径保持 strip-only，不透传本地 carrier。

### 二、账号能力与调度

新增调度测试覆盖：

1. 普通 GPT-5.5 请求可以选择免费账号。
2. 含有效 image carrier 的 GPT-5.5 请求不会选择只支持 GPT-5.5 的免费账号。
3. 含有效 image carrier 的 GPT-5.5 请求会选择同时支持 `gpt-5.5` 和 `gpt-image-2` 的付费账号。
4. sticky 账号如果不支持 `gpt-image-2` 会被清理并重新选择。
5. 没有任何账号同时支持两者时返回错误。
6. failover 重试仍保留生图能力要求。
7. `previous_response_id` 命中普通 GPT-5.5 账号时必须跳过并重新选择生图账号。
8. advanced scheduler / projection 路径中，普通 GPT-5.5 请求仍可命中 free 账号，含有效 image carrier 的请求只能命中同时支持主模型和 `gpt-image-2` 的账号。
9. `TargetGroupAny`、`TargetGroupActive`、`TargetGroupExhausted`、reserve overlay、projection stale/miss 都不能绕过 image 能力要求。
10. `/v1/images/*` 现有 native/basic fallback 行为不被 Responses built-in image generation 修改污染。
11. 原生 Responses `tools[]` 中已有 `image_generation`，且 `model` 缺失或等于 `gpt-image-2` 时，必须触发与新 carrier 相同的调度 requirement。
12. Chat Completions 兼容转换后形成 Responses-shape `image_generation` tool 时，必须触发与新 carrier 相同的调度 requirement。
13. OAuth `plan_type` 缺失、OAuth `plan_type: "free"`、付费 OAuth、API Key 账号必须分别覆盖。
14. API Key 或 OAuth 账号只有宽泛 `*`、`gpt-*`、空 mapping default allow、group default、projection `DefaultAllow` 时，必须被判定为没有 `gpt-image-2` 正向证据。
15. API Key 或 OAuth 账号具备精确 `gpt-image-2`、image-specific `gpt-image-*`、explicit models 或 catalog models 证据时，才可通过 image requirement。

### 三、OpenCode image continuation 回归

保留前一轮 OpenCode image continuation 测试，并更新请求体为新 carrier：

```json
"builtin_tools": {
  "image_generation": {
    "enabled": true,
    "model": "gpt-image-2",
    "output_format": "png"
  }
}
```

旧 carrier 的测试要改为“不会注入 image tool”。

## 兼容与迁移策略

1. 旧推荐配置中的普通 GPT-5.5 不再自动启用生图；这正是目标行为。
2. 用户如果仍使用旧 `image_generation: true`，请求会继续成功走普通 GPT-5.5，但不会生成图片。
3. 需要生图的用户通过新的 Image 变体或 `image` subagent 触发。
4. `web_search` 不受影响。
5. 后端错误信息应帮助管理员识别“没有同时支持 gpt-5.5 和 gpt-image-2 的账号”。

## 实施顺序建议

0. 按用户要求，进入实现前先在项目内 `.worktrees/` 创建隔离 worktree；Go 命令使用 `C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe`。
1. 后端先加 failing tests，锁定旧 image carrier 失效、新 image carrier 生效、web_search 不回归。
2. 扩展 builtin tools normalize helper，只引入 `enabled:true` 门槛。
3. 基于最终有效 Responses tool set 增加请求侧 image generation capability 标记，并传入 `OpenAIAccountScheduleRequest`。
4. 扩展账号能力判断，要求主模型与 `gpt-image-2` 同时支持。
5. 更新 OpenCode image continuation 测试使用新 carrier。
6. 更新 `UseKeyModal.vue` 生成 Image variant 和 `agent.image`。
7. 更新前端测试。
8. 跑后端与前端相关验证。

最低验证命令范围：

1. 后端：`C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -count=1`
2. 后端必要补充：`C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/pkg/apicompat -count=1`
3. 后端构建：`C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe build ./cmd/server`
4. 前端聚焦：`pnpm exec vitest run src/components/keys/__tests__/UseKeyModal.spec.ts --pool=forks --poolOptions.forks.singleFork --reporter=verbose`
5. 前端类型：`pnpm typecheck`
6. 前端构建：`pnpm build`
7. 空白检查：`rtk git diff --check`

## 验收标准

1. 普通 OpenCode `gpt-5.5*` 推荐配置不再默认包含 `image_generation`。
2. OpenCode 推荐配置中存在可选的 `image` 变体或等价 Image 模型入口。
3. 推荐配置中存在 `agent.image`，使用 `sub2api-openai/gpt-5.5-fast-Sys` 和 `variant: "image"`。
4. 新 image carrier 能生成上游 `image_generation` tool。
5. 旧 image carrier 不再生成上游 `image_generation` tool。
6. `web_search` 旧格式继续生成上游 web search tool。
7. 生图请求不会路由到只支持 GPT-5.5 的免费账号。
8. 生图请求能路由到同时支持 GPT-5.5 与 `gpt-image-2` 的账号。
9. OpenCode image continuation 仍不向客户端暴露 raw `image_generation_call`。
10. 上游 `image_generation` tool 不包含本地 `enabled` 字段。
11. 旧 image carrier 失效后不会留下不可用的 `tool_choice.image_generation` 导致上游 400。
12. Chat Completions 兼容路径、compact 路径和 passthrough 路径的边界行为都有测试覆盖。
13. 原生 Responses `tools[]` 中已有 `image_generation` 时，即使没有本地 carrier，也必须走同一生图账号路由约束。
14. 宽泛 wildcard / default allow 不能证明账号支持 `gpt-image-2`；只有精确或 image-specific 正向证据才能通过生图 requirement。

## 风险与后续

1. OpenCode 当前 variant UI 是否能充分表达 `Image Fast (Sys)` 需要实测；如展示不清晰，可后续物化独立模型 key。
2. `plan_type` 在现有数据中不保证对所有账号稳定存在；因此账号是否支持 `gpt-image-2` 不能只看层级，也要结合模型映射和 capability catalog。
3. 如果未来免费层级也开放 `gpt-image-2`，只需更新能力探测/账号 capability，不需要改推荐配置格式。
4. 如果后续要支持 reasoning 与 image 的组合 variant，可以新增 `image-low`、`image-medium` 等，但本轮不做。
