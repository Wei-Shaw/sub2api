# OpenCode 推荐配置模型派生对齐与 `web_search` 策略设计

## 背景

当前本地 `sub2api-openai` 推荐配置生成链，仍以“基础模型 + 本地手工变体”为主。

但最新 OpenCode upstream 已经在**运行时模型生成层**引入了更明确的模型派生逻辑：

- `packages/opencode/src/provider/provider.ts`
- `fromModelsDevProvider()`
- 真正的来源提交：`7a6ce05 2.0 exploration (#22335)`

其关键行为是：

1. 从 `models.dev` 读取基础模型
2. 读取 `model.experimental.modes`
3. 物化派生模型，例如 `gpt-5.4-fast`
4. 将 `provider.body` 转 camelCase 后落到模型级 `options`
5. 将 `provider.headers` 落到模型级 `headers`
6. 注意：`gpt-5.4-fast` 这类派生模型在 upstream 是**运行时模型 key**，其底层请求语义仍和基础模型分层处理；它不是“某段 builtin OpenAI provider 默认配置生成器”的产物。

这意味着：如果我们继续只在旧结构上补一条 `web_search`，会和 upstream 当前实际的 OpenAI provider 模型生成结果持续偏离。

## 目标

更新本地 `sub2api-openai` 推荐配置生成链，使其：

1. 跟进 upstream 当前的模型派生结构
2. 保留本地独有的 `-Sys` 变体策略
3. 明确给出 `web_search` 的推荐配置策略，但**不再假设它能直接通过 model config 字段被 OpenCode runtime 消费**

## 关键映射规则

这次必须把最终推荐配置里的“模型 key / id”规则写死，避免实现再次跑偏：

1. 我们镜像 upstream 的，是 **runtime 最终可见的模型集合与附带参数语义**，不是把 upstream 内部 `api.id` 分层原样照抄进本地推荐配置。
2. 对本地 `sub2api-openai` 推荐配置，本次继续保留现有产品语义：
   - **最终请求出去的模型名 = 推荐配置里模型对象的 key / id**
   - 也就是：
     - `gpt-5.4`
     - `gpt-5.4-fast`
     - `gpt-5.4-Sys`
     - `gpt-5.4-fast-Sys`
     都继续作为本地最终请求模型名存在
3. upstream `fromModelsDevProvider()` 中 `api.id` 与 runtime alias 的分层，这次只作为“为什么要新增 fast 模型及其 model.options/headers”的来源参考，不要求本地推荐配置也做同样的导出分层。
4. 一句话：
   - **镜像 upstream 的“模型集合与参数语义”**
   - **保留本地推荐配置“模型 key = 最终请求 id”这一产品语义**

## 非目标

- 不处理 `image_generation`
- 不处理 `code_interpreter`
- 不修改请求转发层 builtin-tools augmentation 功能本身
- 不重构整个 OpenCode metadata mirror 服务
- 不把 upstream runtime request transform 误写成“推荐配置生成契约”

## 设计边界

### 一、严格镜像 upstream 的部分

镜像对象不是 builtin OpenAI provider 的不存在“专用生成器”，而是 upstream 当前**实际生效**的运行时模型派生逻辑：

- 来源文件：`packages/opencode/src/provider/provider.ts`
- 来源函数：`fromModelsDevProvider()`
- 参考 commit：`7a6ce05 2.0 exploration (#22335)`

本地需要镜像的语义：

1. 保留基础模型
2. 读取 `experimental.modes`
3. 物化派生模型，例如：
   - `gpt-5.4-fast`
4. 将 mode 自带 `provider.body` 转 camelCase 后落到模型级 `options`
5. 将 mode 自带 `provider.headers` 落到模型级 `headers`

这里要明确一条边界：

- **我们镜像的是 upstream runtime 可见的最终模型集合**，不是去复制 upstream UI 的 custom-provider 表单输出。
- 也就是说，这次本地实现可以主动做“扁平化导出”，目的是让推荐配置结果更接近 upstream runtime 已暴露出来的模型集合，而不是声称 upstream UI 本身就会直接写出这些配置。

### 二、明确保留的本地扩展

`-Sys` 仍然是本地独有扩展，不属于 upstream provider 生成逻辑。

生成顺序应改为：

1. 先镜像 upstream，得到：
   - 基础模型
   - 由 `experimental.modes` 物化出的派生模型（例如 `gpt-5.4-fast`）
2. 再对这组“最终模型集合”统一派生 `-Sys` 版本：
   - `gpt-5.4` -> `gpt-5.4-Sys`
   - `gpt-5.4-fast` -> `gpt-5.4-fast-Sys`

结论：

- Fast 不再靠本地手工 variant 方案表达
- 但每个 upstream 派生出的 fast 模型，仍然要继续拥有对应的 `-Sys` 版本

同时补一条硬约束：

- 最终推荐配置必须继续输出到 `provider["sub2api-openai"]` 下
- 不得改写成 `provider.openai`
- 不得覆盖 OpenCode 官方 builtin OpenAI provider

### 二点五、`variants` 的边界

这次 spec **不**要求镜像 upstream `ProviderTransform.variants()` 的整套运行时变体生成规则。

本次仅处理：

1. upstream `experimental.modes` 物化出的独立模型（例如 `gpt-5.4-fast`）
2. 这些独立模型再派生出的本地 `-Sys` 版本

其余现有本地 `variants` 处理保持当前策略，不在本次范围内整体重写成 upstream runtime variants 语义。

明确来说：

- 本次不要求全面镜像 upstream `ProviderTransform.variants()`
- 但对于已经被 upstream 物化成独立模型的 fast 形态，不再继续依赖本地旧的 `*-fast` variant 表达

### 三、`web_search` 的挂法

`web_search` 不是 upstream 现成配置字段，而是本地推荐配置增强。

但这里必须明确一条纠偏：

- OpenCode upstream 当前 runtime 请求链里的工具来自 `packages/opencode/src/session/llm.ts::resolveTools(input)`
- config schema 的 `Model` 支持的是 `options` / `headers` / `variants`
- **没有证据表明 model-level `tools` 是当前 upstream 正式消费的配置入口**
- 但我们本地 API 转发服务已经明确支持私有请求参数 `builtin_tools`，并会在转发层把它转换成真正的 OpenAI built-in tools 请求体

所以这次设计不能再写成“直接把 `tools:[{type:\"web_search\"}]` 挂进模型配置字段就一定生效”。

这次阶段性目标应改成两步：

1. 本次先完成 upstream 模型派生结构对齐（基础模型 + fast 派生模型 + `-Sys` 派生）
2. 同时把 `web_search` 明确记成**本地扩展策略位**，并通过我们自己的 `builtin_tools` 请求参数落地

也就是说，本次 spec 里对 `web_search` 的要求改成：

- 它必须继续是“所有推荐模型都默认带上”的目标语义
- 但实现层不能再偷懒写成 `model.tools` 这种会被误解成 upstream 原生语义的字段
- 它应当通过我们自己的 `builtin_tools` 本地参数落地
- 该参数作为模型级共享默认参数存在，并最终透传到我们的 API 转发服务，由转发层负责转换成真正的 OpenAI built-in tool 请求体

因此，这一节现在只保留一条设计约束：

- 基础模型、fast 派生模型，以及它们各自的 `-Sys` 版本，最终都必须共享同一种 `web_search` 默认开启语义
- 这层默认开启语义统一通过 `builtin_tools` 落地，而不是通过 upstream 原生 `tools` 配置字段落地

## 文件落点

本次实现限定在当前 OpenCode 推荐配置生成链与本地 `builtin_tools` 默认参数挂载点：

- `frontend/src/components/keys/UseKeyModal.vue`
- `backend/internal/service/opencode_openai_metadata.go`
- `frontend/src/api/keys.ts`
- 必要测试文件

说明：

- `UseKeyModal.vue` 负责最终推荐配置生成
- `opencode_openai_metadata.go` 负责 metadata mirror 与模型列表组织
- `frontend/src/api/keys.ts` 负责前端拿到 mirror 后的数据契约
- `opencode_openai_metadata.go` 只负责 mirror / 中间数据，不负责产出 `-Sys`、`web_search` 或 `provider["sub2api-openai"]` 本地扩展
- backend 现有 `filterOpenCodeOpenAIModelsForCodexOAuth()` 这层本地模型筛选策略继续保留，不在本次移除或放宽
- `builtin_tools` 的消费语义不在 OpenCode upstream，而在我们自己的 API 转发服务：
  - `backend/internal/pkg/apicompat/types.go`
  - `backend/internal/service/openai_gateway_service.go`
  - `backend/internal/service/openai_gateway_chat_completions.go`

## 注释要求

在本地实现“镜像 upstream 的模型派生逻辑”代码旁，必须补上来源注释，至少包含：

- upstream 仓库：`anomalyco/opencode`
- 文件：`packages/opencode/src/provider/provider.ts`
- 函数：`fromModelsDevProvider()`
- 参考 commit：`7a6ce05`
- permalink / 链接
- 相关行号范围
- 一句说明：本地镜像的是 upstream runtime 的“最终模型集合派生语义”，不是 UI custom-provider 表单原样输出

在本地实现“不要给 openai-compatible 盲目塞不安全默认参数”的相关判断旁，可以引用：

- 文件：`packages/opencode/src/provider/transform.ts`
- 参考 commit：`c2403d0f155b55ad067e742ed6f091312445d24e`
- 说明其含义是 upstream 对 `reasoningSummary` 注入变得更保守。

但要明确：

- 这条引用只用于解释“为什么不要继续盲目添加不安全默认参数”
- 它不是本次 `fast` 模型派生逻辑的直接来源

## 验证要求

至少覆盖以下行为：

1. 基础模型仍被保留
2. `experimental.modes.fast` 被物化成 `*-fast` 模型
3. mode `provider.body` 正确落到模型级 `options`（例如 `serviceTier: "priority"`）
4. mode `provider.headers` 正确落到模型级 `headers`
5. 每个最终模型（基础 + fast）都额外派生出 `-Sys` 版本
6. `provider["sub2api-openai"]` 继续保留，不会误写成 `provider.openai`
7. 最终推荐配置中不会泄漏 raw `experimental` 或 mode provider 原始结构
8. `web_search` 的默认开启语义在所有最终模型上保持一致
9. 不会把 `image_generation` / `code_interpreter` 一并带入
10. `gpt-5.4-fast` / `gpt-5.4-fast-Sys` 这类模型会作为独立推荐模型出现，但继续保持本地“模型 key = 最终请求 id”语义
11. 基础模型和 `-Sys` 模型不再保留旧的 `low-fast` / `medium-fast` / `high-fast` / `xhigh-fast` 这类本地 fast variants
12. 最终推荐配置中，`web_search` 的默认开启统一通过 `builtin_tools` 表达，而不是通过 `model.tools`
13. 最终推荐配置里 `builtin_tools` 的推荐输出形态统一为对象形态：`{"web_search": true}`，不在不同模型间混用 `true` / 数组 / 对象三种写法

## 预期结果

完成后，本地 `sub2api-openai` 推荐配置应同时满足：

1. 结构上跟上 upstream 当前的 OpenAI provider 模型派生方式
2. 保留本地独有的 `-Sys` 变体体系
3. 通过统一的 `builtin_tools` 默认参数，让所有推荐模型默认开启 `web_search`

一句话总结：

**先镜像 upstream 当前的 `experimental.modes -> 派生模型 + model.options/headers`，保留本地“模型 key = 最终请求 id”和 `-Sys` 扩展语义，再在这组最终模型上统一挂载本地 `builtin_tools` 默认参数，以默认开启 `web_search`。**
