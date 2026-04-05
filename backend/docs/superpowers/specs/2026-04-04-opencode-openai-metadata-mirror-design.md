# OpenCode OpenAI 推荐配置元数据镜像设计

## 背景

当前 `sub2api` 前端生成 OpenCode 推荐配置的逻辑位于：

- `frontend/src/components/keys/UseKeyModal.vue`

它依赖手写的 `openCodeOpenAIBaseModels` 常量，再在其上叠加：

- `-Sys` 模型
- `low-fast / medium-fast / high-fast / xhigh-fast` 组合变体

这条链路当前存在两个根本问题：

1. 它不是从 OpenCode built-in `openai` provider 的真实元数据来源生成出来的，而是手工维护的一份简化模型表；
2. 因为缺少 built-in `openai` 的完整能力元数据，自定义 provider `sub2api-openai` 会被 OpenCode 当成 `attachment=false`、`input.image=false`、`input.pdf=false` 等保守能力模型，从而在请求发送前就把图片/PDF 裁掉。

用户已经明确要求：

- 不再走“从本机缓存结果抄一份静态镜像”的方案；
- 而是让 `sub2api` 这一侧尽量复用/模拟 OpenCode 的完整链路：
  - `models.dev -> provider generation -> 推荐配置`

## 目标

本次只解决“OpenCode 推荐配置元数据镜像”这一件事：

1. 在 `sub2api` 这一侧新增一条专门服务于推荐配置的 OpenAI 元数据拉取/缓存/转换链；
2. 元数据来源直接是 `models.dev` 的 built-in `openai` provider，而不是本机缓存结果的手工拷贝；
3. `UseKeyModal.vue` 不再维护手写基础模型表，而是消费这条后端元数据接口；
4. 在官方元数据基础上继续叠加 `-Sys` 与 `*-fast` 变体，生成最终的 `sub2api-openai` 推荐配置；
5. 不改变服务端 `/v1/models`、账号白名单、`model_mapping`、运行时路由模型判断逻辑。

## 非目标

本次明确不做：

1. 不让服务端 `/v1/models` 自动跟 `models.dev` 同步；
2. 不调整账号白名单/映射、可调度模型集合、或新模型的路由支持；
3. 不在 repo 中实现本机 `C:\Users\34404\.config\opencode\opencode.jsonc` 的自动生成器；
4. 不把这条元数据镜像链用于其他客户端配置（如 Claude Code / Gemini CLI）生成。

## 现状关键事实

### 1. OpenCode built-in `openai` 的真实数据源

OpenCode 当前 built-in `openai` provider 的模型元数据来源链路是：

1. `packages/opencode/src/provider/models.ts`
   - 优先读取本机缓存 `~/.cache/opencode/models.json`
   - 若缓存不存在，则拉远端 `https://models.dev/api.json`
2. `packages/opencode/src/provider/provider.ts`
   - `fromModelsDevProvider()` / `fromModelsDevModel()` 把 `models.dev` 的 provider/model 数据转换成内部 `Model`
   - 映射字段包括：
     - `attachment`
     - `reasoning`
     - `tool_call`
     - `modalities.input/output`
     - `interleaved`
     - `limit`
     - `release_date`
     - `cost`
     - `cost.context_over_200k`

也就是说，官方 `openai` 不是静态写死的模型表，而是运行时生成出来的一份 provider 数据库。

### 2. 为什么 `sub2api-openai` 吃不到这些能力

在 OpenCode 里，自定义 provider 只有当 `provider_id` 与 built-in provider 同名时，才会 merge 到同一份模型数据库上并继承能力元数据。

`sub2api-openai` 不是 `openai`，所以它不会自动继承 built-in `openai` 的任何模型能力，只会保留配置里手工写入的那部分字段。

### 3. 为什么不能直接复用 `sub2api` 当前 `/v1/models`

`sub2api` 当前服务端 `/v1/models` 解决的是“当前哪些模型对外可用”，它的数据源是：

1. 账号本地 `model_mapping` 的动态汇总
2. 代码里的默认模型清单

它不是官方能力元数据目录，因此不包含 OpenCode 推荐配置所需的：

- `attachment`
- `modalities.input/output`
- `tool_call`
- `cost.context_over_200k`
- `release_date`

所以推荐配置不能直接复用 `/v1/models`。

## 设计

### 1. 后端新增推荐配置专用元数据链

在 `sub2api` 后端新增一条仅供推荐配置使用的 OpenAI 元数据链，职责分为三层：

1. **fetch 层**
   - 从 `models.dev` 获取 built-in `openai` provider 数据
   - 支持失败回退与短 TTL 缓存

2. **transform 层**
   - 做与 OpenCode `fromModelsDevModel()` 同构的转换
   - 输出推荐配置可直接消费的模型结构

3. **API 层**
   - 提供给前端 `UseKeyModal.vue` 调用
   - 只暴露推荐配置需要的模型元数据，不卷入现有 `/v1/models`

### 2. 转换字段范围

后端转换链至少要对齐以下 built-in `openai` 模型字段：

- `attachment`
- `reasoning`
- `tool_call`
- `modalities.input`
- `modalities.output`
- `interleaved`
- `limit`
- `release_date`
- `cost`
- `cost.context_over_200k`

这条转换链要以 built-in `openai` 的全部模型为基础，而不是仅覆盖当前 `sub2api-openai` 已手写的那批模型。

### 3. 前端推荐配置生成改成“后端元数据 + 自定义变体叠加”

`UseKeyModal.vue` 不再维护 `openCodeOpenAIBaseModels`。

新的生成流程应为：

1. 前端调用后端推荐配置元数据接口，拿到 built-in `openai` 全量基础模型元数据；
2. 基于这份基础模型元数据，生成 `sub2api-openai` 推荐配置；
3. 在前端统一叠加：
   - `-Sys` 模型
   - `low-fast / medium-fast / high-fast / xhigh-fast` 组合变体

换句话说，官方元数据与我们自己的自定义扩展要分两层：

- 基础能力 = 后端从 `models.dev` 拉取并转换
- 自定义扩展 = 前端派生 `-Sys` / `*-fast`

### 4. 本机 `opencode.jsonc` 处理方式

本机配置仍然不进入 repo 自动生成链。

这次改动完成后，前端推荐配置会先变成能力完整的自动化来源；
随后再由人工把这份结果复制到：

- `C:\Users\34404\.config\opencode\opencode.jsonc`

## 风险与控制

### 风险 1：后端转换语义和 OpenCode 不一致

如果我们只“看起来字段名相同”，却没有按 OpenCode `fromModelsDevModel()` 的语义来映射，推荐配置仍可能缺少关键能力判断，例如：

- `attachment`
- `modalities.input.image`
- `modalities.input.pdf`

控制方式：

1. 转换逻辑显式对照 OpenCode `fromModelsDevModel()`；
2. 后端单测锁定至少 `gpt-5.4` 的关键能力字段映射。

### 风险 2：外站不可用导致推荐配置空掉

如果推荐配置每次打开都裸拉 `models.dev`，一旦外站失败，用户就拿不到推荐配置。

控制方式：

1. 后端做 TTL 缓存；
2. 保留最近一次成功结果；
3. 前端对失败态有可见提示，而不是静默空白。

### 风险 3：自定义变体把官方元数据覆盖坏

如果 `-Sys` / `*-fast` 直接写进基础元数据源，会让官方能力和自定义行为耦合，后续难维护。

控制方式：

1. 基础模型元数据只来自后端镜像接口；
2. 所有 `-Sys` / Fast 变体都在前端最终拼装阶段派生。

### 风险 4：误接进 `/v1/models`

如果把这条推荐配置元数据链接进服务端 `/v1/models`，会再次混淆：

- “推荐配置能力元数据”
- “当前服务端可路由模型列表”

控制方式：

1. 新接口命名和职责明确为“推荐配置元数据”；
2. 不复用现有 `/v1/models` handler/service；
3. 不读取账号白名单/`model_mapping` 来生成推荐配置元数据。

## 验证方案

### A. 后端单测

锁定 `models.dev openai -> 推荐配置模型结构` 的关键映射，至少覆盖 `gpt-5.4`：

- `attachment: true`
- `modalities.input` 包含 `image` 与 `pdf`
- `modalities.output` 包含 `text`
- `cost.context_over_200k` 正确保留

### B. 前端单测

锁定最终推荐配置中：

- `gpt-5.4 / gpt-5.4-Sys` 仍具备正确能力字段
- 现有 `low-fast / medium-fast / high-fast / xhigh-fast` 变体仍存在

### C. 构建与最小在线能力验证

1. `pnpm build` 必须通过
2. 用新的推荐配置做一次最小能力验证，确认新的 `sub2api-openai` 不再因为 `input.image=false` 在 OpenCode 请求前裁掉图片

## 结论

这次真正要做的不是“再维护一份更大的手写模型表”，而是把 `sub2api` 推荐配置的数据源改成与 OpenCode built-in `openai` 同构的上游来源链：

- `models.dev`
- 后端 fetch/cache/transform
- 前端消费基础元数据
- 前端叠加 `-Sys` / `*-fast`

这样推荐配置能力元数据才会真正跟官方 OpenAI provider 同步，而不是继续停留在手工维护快照阶段。
