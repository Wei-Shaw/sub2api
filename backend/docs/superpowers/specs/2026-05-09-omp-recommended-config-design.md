# OMP 推荐配置生成设计

## 背景

`sub2api` 已经在 Use Key 弹窗里提供 OpenCode 推荐配置。当前 OpenAI 平台的 OpenCode 配置不是上游 OpenCode 原生能力，而是本仓库的本地产品链：

- 使用独立 provider `sub2api-openai`，不覆盖 OpenCode 内置 `openai`。
- 从后端 `models.dev` metadata 镜像接口获取 OpenAI 模型能力。
- 在前端派生 `-Sys`、Fast、Image variant 与 `agent.image`。
- 通过本地 `metadata.builtin_tools` carrier 表达 `web_search` / `image_generation` 意图。

用户现在还需要同一把 sub2api OpenAI key 能直接生成 OMP 推荐配置。目标不是替代 OpenCode 配置，而是在同一个 Use Key 弹窗中增加 OMP 客户端入口，让用户能复制 OMP 可用的：

1. 插件安装与图像子代理快捷配置命令；
2. `~/.omp/agent/models.yml` 片段；
3. `~/.omp/agent/config.yml` 片段。

已确认 OMP 版本与本机有效配置基线：

- `omp/14.7.8`
- `omp config path` 输出 `C:\Users\34404\.omp\agent`
- 当前有效 provider 包括 `sub2api-openai` 和 `sub2api-openai-image`
- 当前有效 canonical 包括：
  - `gpt-5.5` -> `sub2api-openai/gpt-5.5`
  - `gpt-5.5-sys` -> `sub2api-openai/gpt-5.5-Sys`
  - `gpt-5.5-image-sys` -> `sub2api-openai-image/gpt-5.5-Sys`

注意：本设计不得输出或固化本机已读到的真实 API key。所有示例使用 `<USER_API_KEY>` 或测试中的 `sk-test`。

## 已确认的 OMP 配置机制

### `models.yml`

OMP 默认模型配置文件是：

```text
~/.omp/agent/models.yml
```

`models.yml` 顶层结构：

```yaml
providers:
  <provider-id>:
    baseUrl: https://api.example.com/v1
    apiKey: <USER_API_KEY>
    api: openai-responses
    compat:
      openaiProviderTools:
        enabled: true
    models:
      - id: model-id
        name: Model Name
        api: openai-responses
        reasoning: true
        input:
          - text
          - image
        contextWindow: 400000
        maxTokens: 128000
        cost:
          input: 0
          output: 0
          cacheRead: 0
          cacheWrite: 0

equivalence:
  overrides:
    <provider-id>/<model-id>: <canonical-model-id>
```

关键约束：

- 自定义 provider 有 `models` 时需要 `baseUrl`、`apiKey`（除非 `auth: none`）以及 provider 级或 model 级 `api`。
- `apiKey` 先按环境变量名解析；如果环境变量不存在，则按 literal token 使用。
- `input` 只支持 `text` 和 `image`；OpenCode 的 `pdf` 能力不能原样写入 OMP `models.yml`。
- `equivalence.overrides` 左侧是完整 concrete selector，右侧是 canonical id。右侧不得改成 `provider/model`，否则会破坏 canonical 分组语义。
- 本设计使用 `compat.openaiProviderTools`。这是 `omp-openai-provider-tools` 插件消费的扩展字段；插件未安装时，provider-native `web_search` / `image_generation` 不会生效。

### `config.yml`

OMP settings 文件是：

```text
~/.omp/agent/config.yml
```

与模型推荐相关的字段：

```yaml
defaultThinkingLevel: xhigh
serviceTier: priority

modelRoles:
  default: sub2api-openai/gpt-5.5-Sys
  slow: sub2api-openai/gpt-5.5-Sys
  smol: sub2api-openai/gpt-5.4-mini-Sys
  plan: sub2api-openai/gpt-5.5-Sys
  task: sub2api-openai/gpt-5.5-Sys:xhigh
  vision: sub2api-openai/gpt-5.5-Sys
```

本设计要求所有「模型引用」都使用带 provider id 的完整名称。例外只有：

- `models.yml` 中 `providers.<provider-id>.models[].id`，它是 provider 内部模型 ID；
- `equivalence.overrides` 的右侧 canonical id。

## 已确认的插件依赖

生图能力依赖插件：

```text
https://github.com/jiwangyihao/omp-openai-provider-tools
```

插件安装命令不得锁定具体版本。前端必须从后端推荐配置元数据接口读取当前 npm latest 版本，并生成带明确版本号的安装命令。当前已核对的 npm latest 是 `0.1.2`，只作为本文事实快照和测试 fixture 示例，不作为生产硬编码常量。

插件当前提供：

- provider-native `web_search` 注入；
- provider-native `image_generation` 注入；
- provider 级 `compat.openaiProviderTools.enabled: true`；
- model 级 `compat.openaiProviderTools.imageGeneration: true`；
- `configure-image-agent` CLI，用于生成推荐图像子代理模板。

安装命令格式：

```bash
omp plugin install npm:omp-openai-provider-tools@<latest-version-from-api>
omp plugin doctor
```

实现时不得输出裸包名、`@latest` 或固定在本文中的 `0.1.2`。后端接口应返回当前 npm latest 版本；前端只消费该字段并拼接命令。

图像子代理预览与写入命令：

```bash
npx omp-openai-provider-tools configure-image-agent --model sub2api-openai-image/gpt-5.5-Sys --dry-run
npx omp-openai-provider-tools configure-image-agent --model sub2api-openai-image/gpt-5.5-Sys
```

`configure-image-agent` 的行为：

- 默认 agent 名：`image_generator`
- 默认 thinking level：`xhigh`
- 默认写入目录：`~/.omp/agent/agents`
- 默认文件名：`image-generator.md`
- 已有同名 agent 时默认拒绝覆盖
- `--print` 可输出模板供人工合并
- `--force` 只有用户明确确认替换时才应使用

## 目标

1. 在 OpenAI 平台的 Use Key 弹窗中新增 OMP 客户端入口。
2. OMP 入口输出 3 个可复制块，并且第 1 个块必须是插件安装和图像子代理配置命令。
3. 复用现有后端 OpenAI metadata 镜像接口，并扩展该接口返回 `omp_openai_provider_tools.latest_version`；不新增独立推荐配置 endpoint。
4. 生成 OMP `models.yml`，包含：
   - `sub2api-openai` provider；
   - `sub2api-openai-image` provider；
   - `compat.openaiProviderTools.enabled: true`；
   - image model 级 `compat.openaiProviderTools.imageGeneration: true`；
   - `equivalence.overrides`。
5. 生成 OMP `config.yml`，所有模型引用使用完整 `provider-id/model-id`。
6. 避免泄露真实 API key。
7. 不破坏现有 OpenCode 推荐配置链。

## 非目标

1. 不自动写入用户本机 OMP 配置文件。
2. 不自动安装 OMP 插件。
3. 不修改 OMP 源码或插件源码。
4. 不新增 `sub2api` 后端 `/v1/models` 行为。
5. 不新增独立 OMP 推荐配置 endpoint；首版只扩展现有 `/keys/opencode/openai-models` response。
6. 不把 OpenCode JSON 配置 shape 直接复用为 OMP 配置。
7. 不把 `sub2api-openai` 改成 `provider.openai`。
8. 不让普通 `sub2api-openai/gpt-5.5-Sys` 默认启用 `imageGeneration: true`。

## UI 设计

### 入口位置

在 `UseKeyModal.vue` 的 OpenAI 平台 `clientTabs` 中新增：

```ts
{ id: 'omp', label: 'OMP', icon: TerminalIcon }
```

OMP 是独立客户端，不是 OpenCode 的子入口。因此它应与 Codex / OpenCode 并列，而不是放到 OpenCode tab 下面。

### 输出顺序

OMP tab 的 `currentFiles` 必须按以下顺序返回：

```ts
return [
  generateOMPProviderToolsPluginInstructions(),
  generateOMPModelsConfig(apiBase, apiKey, openCodeModels.value),
  generateOMPSettingsConfig()
]
```

顺序是硬性要求：

1. `1. Install OMP provider tools plugin`
2. `~/.omp/agent/models.yml`
3. `~/.omp/agent/config.yml`

第 1 个块必须排在所有配置文件前面。原因是生图功能依赖插件；如果说明放在配置文件后面，用户很可能复制配置后遗漏插件安装与图像子代理配置。

### 第 1 块：插件和图像子代理说明

内容：

```bash
# 1. 安装或升级 provider-native tools 插件
omp plugin install npm:omp-openai-provider-tools@<latest-version-from-api>

# 2. 检查插件健康状态
omp plugin doctor

# 3. 预览推荐图像子代理模板
npx omp-openai-provider-tools configure-image-agent --model sub2api-openai-image/gpt-5.5-Sys --dry-run

# 4. 确认预览无误后写入 ~/.omp/agent/agents/image-generator.md
npx omp-openai-provider-tools configure-image-agent --model sub2api-openai-image/gpt-5.5-Sys

# 如果已有 image_generator，命令会拒绝覆盖。
# 可用 --print 输出模板后手动合并；只有确认替换时才使用 --force。
npx omp-openai-provider-tools configure-image-agent --model sub2api-openai-image/gpt-5.5-Sys --print
```

hint 文案必须说明：

- provider-native `web_search` / `image_generation` 依赖 `omp-openai-provider-tools`。
- 后续 `models.yml` 里的 `compat.openaiProviderTools` 只是能力声明；插件未安装时不会生效。
- 安装或升级插件、写入 agent 后需要重启 OMP 会话。
- 插件不读取、不保存 API key；凭据仍由 OMP `models.yml` 管理。
- 图像子代理模型使用 `sub2api-openai-image/gpt-5.5-Sys`。

### 第 2 块：`models.yml`

输出 path：

```text
~/.omp/agent/models.yml
```

顶部建议加注释作为二次提醒：

```yaml
# 生图和 provider-native web_search 依赖插件：
#   omp plugin install npm:omp-openai-provider-tools@<latest-version-from-api>
#   omp plugin doctor
# 图像子代理推荐命令：
#   npx omp-openai-provider-tools configure-image-agent --model sub2api-openai-image/gpt-5.5-Sys --dry-run
# 安装或升级后请重启 OMP 会话。
```

注意：这个注释只是补充，不能替代第 1 个说明块。

### 第 3 块：`config.yml`

输出 path：

```text
~/.omp/agent/config.yml
```

生成推荐 `modelRoles`，所有模型引用都用完整 provider selector。

## `models.yml` 生成规则

### Provider

生成两个 provider：

```yaml
providers:
  sub2api-openai:
    api: openai-responses
    baseUrl: https://example.com/v1
    apiKey: <USER_API_KEY>
    compat:
      openaiProviderTools:
        enabled: true
    models:
      - id: gpt-5.5
        name: GPT-5.5
        api: openai-responses
        reasoning: true
        input:
          - text
          - image
        contextWindow: 400000
        maxTokens: 128000

  sub2api-openai-image:
    api: openai-responses
    baseUrl: https://example.com/v1
    apiKey: <USER_API_KEY>
    compat:
      openaiProviderTools:
        enabled: true
    models:
      - id: gpt-5.5-Sys
        name: GPT-5.5 Image (Sys)
        api: openai-responses
        reasoning: true
        input:
          - text
          - image
        contextWindow: 400000
        maxTokens: 128000
        compat:
          openaiProviderTools:
            imageGeneration: true
```

### 模型字段映射

从后端返回的 `OpenCodeOpenAIModel` 映射为 OMP model：

| OpenCode metadata | OMP `models.yml` |
| --- | --- |
| `id` | `id` |
| `name` | `name` |
| `reasoning` | `reasoning` |
| `modalities.input` | `input`，只保留 `text` / `image` |
| `limit.context` | `contextWindow` |
| `limit.output` | `maxTokens` |
| `cost.input` | `cost.input` |
| `cost.output` | `cost.output` |
| `cost.cache_read` | `cost.cacheRead` |
| `cost.cache_write` | `cost.cacheWrite` |

不得输出以下 OpenCode-only 字段：

- `attachment`
- `tool_call`
- `structured_output`
- `temperature`
- `knowledge`
- `interleaved`
- `modalities`
- `release_date`
- `options`
- `headers`
- `variants`
- `tools`
- `experimental`
- `provider`
- `pdf`

### `-Sys` 派生

对推荐模型派生 `-Sys`：

```yaml
- id: gpt-5.5
  name: GPT-5.5

- id: gpt-5.5-Sys
  name: GPT-5.5 (Sys)
```

`-Sys` 派生模型复制基础模型的 OMP 字段，但不得共享可变对象引用。

### Image provider

`sub2api-openai-image` 只承载显式 image-capable 入口，首版生成：

```yaml
providers:
  sub2api-openai-image:
    api: openai-responses
    baseUrl: https://example.com/v1
    apiKey: <USER_API_KEY>
    compat:
      openaiProviderTools:
        enabled: true
    models:
      - id: gpt-5.5-Sys
        name: GPT-5.5 Image (Sys)
        api: openai-responses
        reasoning: true
        input:
          - text
          - image
        contextWindow: 400000
        maxTokens: 128000
        compat:
          openaiProviderTools:
            imageGeneration: true
```

`sub2api-openai/gpt-5.5-Sys` 不得设置 `imageGeneration: true`。只有 `sub2api-openai-image/gpt-5.5-Sys` 设置 image generation。

### Equivalence

生成 `equivalence.overrides`：

```yaml
equivalence:
  overrides:
    sub2api-openai/gpt-5.5: gpt-5.5
    sub2api-openai/gpt-5.5-Sys: gpt-5.5-sys
    sub2api-openai/gpt-5.4-mini: gpt-5.4-mini
    sub2api-openai/gpt-5.4-mini-Sys: gpt-5.4-mini-sys
    sub2api-openai-image/gpt-5.5-Sys: gpt-5.5-image-sys
```

左侧必须完整，右侧必须 canonical。

## `config.yml` 生成规则

推荐输出：

```yaml
defaultThinkingLevel: xhigh
serviceTier: priority

modelRoles:
  default: sub2api-openai/gpt-5.5-Sys
  slow: sub2api-openai/gpt-5.5-Sys
  smol: sub2api-openai/gpt-5.4-mini-Sys
  plan: sub2api-openai/gpt-5.5-Sys
  task: sub2api-openai/gpt-5.5-Sys:xhigh
  vision: sub2api-openai/gpt-5.5-Sys

task:
  agentModelOverrides:
    explore: sub2api-openai/gpt-5.4-mini-Sys:xhigh
    librarian: sub2api-openai/gpt-5.4-mini-Sys:xhigh
    reviewer: sub2api-openai/gpt-5.5-Sys:xhigh
    plan: sub2api-openai/gpt-5.5-Sys:xhigh
```

说明：

- `vision` 使用普通 `sub2api-openai/gpt-5.5-Sys`，用于图像理解，不默认走生图 provider。
- 生图工作流由 `configure-image-agent --model sub2api-openai-image/gpt-5.5-Sys` 生成的子代理承载。
- `serviceTier: priority` 是本机有效配置观察到的推荐值；实现文案不要承诺它一定对所有 OMP OpenAI-compatible provider 发出 `service_tier`，除非后续实测确认。

## 后端实现位置

### `OpenCodeMetadataService`

修改：

- `backend/internal/service/opencode_openai_metadata.go`
- `backend/internal/service/opencode_openai_metadata_test.go`

新增职责：

1. 在现有 OpenCode OpenAI metadata response 中附带 OMP provider tools 插件版本元数据。
2. 通过 npm registry latest endpoint 获取 `omp-openai-provider-tools` 当前版本，例如 `https://registry.npmjs.org/omp-openai-provider-tools/latest`。
3. 插件版本必须单独 TTL 缓存，避免每次打开弹窗都打 npm registry。建议 TTL 不短于现有 models.dev metadata TTL。
4. 如果 npm registry 请求失败且存在 stale 版本缓存，返回 stale 版本并把状态标记为 `cached`。
5. 如果 npm registry 请求失败且没有 stale 版本，后端仍应返回 OpenAI models，但插件版本字段应携带明确错误状态；前端不得在这种状态下生成看似完整的安装命令。
6. 插件版本字段只包含版本号和状态，不包含 npm token、registry auth 或其他凭据。

建议 response 形态：

```json
{
  "models": {},
  "omp_openai_provider_tools": {
    "package": "omp-openai-provider-tools",
    "latest_version": "0.1.2",
    "status": "ok"
  }
}
```

失败但无可用版本时：

```json
{
  "models": {},
  "omp_openai_provider_tools": {
    "package": "omp-openai-provider-tools",
    "latest_version": "",
    "status": "unavailable",
    "error": "npm registry unavailable"
  }
}
```

### `APIKeyHandler` 与 route

修改：

- `backend/internal/handler/api_key_handler.go`
- `backend/internal/server/api_contract_test.go`

保持 route 不变：

```text
GET /api/v1/keys/opencode/openai-models
```

该接口名字仍偏 OpenCode，但首版不新增 endpoint，避免为了 OMP 配置生成引入额外 API 面。

## 前端实现位置

### `UseKeyModal.vue`

修改点：

1. `clientTabs`：OpenAI 平台新增 `omp`。
2. `showShellTabs`：`opencode` 和 `omp` 都不显示 OS/Shell tabs。
3. `platformDescription`：OpenAI 平台下当 `activeClientTab.value === 'omp'` 时返回 `t('keys.useKeyModal.omp.description')`。
4. `showPlatformNote`：OMP tab 不显示 Codex / OpenAI 通用 OS/Shell note。
5. `currentFiles`：新增 `activeClientTab.value === 'omp'` 分支。
6. `loadOpenCodeModels()` watcher：触发条件必须是 `props.platform === 'openai' && (activeClientTab.value === 'opencode' || activeClientTab.value === 'omp')`。
7. OMP 分支必须在生成真实 `models.yml` 前处理：
   - `openCodeLoading.value`
   - `openCodeError.value`
   - `!openCodeModels.value`
   - 插件版本 `status !== 'ok' && status !== 'cached'` 或 `latest_version` 为空
8. 失败或未加载时不得生成看似可用的 `models.yml`。可以保持三块顺序，但 `models.yml` 块必须是 `{}` 或提示性占位，并通过 hint 暴露 loading/error/unavailable。
9. 成功状态输出三块：插件说明、真实 `models.yml`、真实 `config.yml`。
10. 新增 helper：
   - `generateOMPProviderToolsPluginInstructions(latestVersion)`
   - `generateOMPModelsConfig(baseUrl, apiKey, openaiSource)`
   - `generateOMPSettingsConfig()`
   - `normalizeOMPModelConfig(model)`
   - `withOMPSysVariants(models)`
   - `buildOMPEquivalenceOverrides(models)`

### `keys.ts`

首版不需要新增 API route，但需要扩展现有 response 类型：

```ts
export type OpenCodeOpenAIModelsResponse = {
  models: Record<string, OpenCodeOpenAIModel>
  omp_openai_provider_tools?: {
    package: string
    latest_version: string
    status: 'ok' | 'cached' | 'unavailable'
    error?: string
  }
}
```

### i18n

新增文案：

- `keys.useKeyModal.cliTabs.omp`
- `keys.useKeyModal.omp.pluginHint`
- `keys.useKeyModal.omp.modelsHint`
- `keys.useKeyModal.omp.configHint`
- `keys.useKeyModal.omp.description`

`keys.useKeyModal.omp.description` 必须说明：OMP tab 提供手动复制配置；第一块插件命令必须先执行；系统不会自动安装插件，也不会自动写入 `~/.omp/agent/models.yml` / `~/.omp/agent/config.yml`。

## 测试计划

### 前端单测

修改：

```text
frontend/src/components/keys/__tests__/UseKeyModal.spec.ts
```

新增或更新测试覆盖：

1. OpenAI 平台存在 OMP tab。
2. 点击 OMP tab 会触发 `keysAPI.getOpenCodeOpenAIModels()`。
3. 点击 OMP tab 后成功状态输出 3 个 code block。
4. 第 1 个 code block 包含：
   - `omp plugin install npm:omp-openai-provider-tools@0.1.2`（测试 fixture 版本来自 mock response，不是生产硬编码）
   - `omp plugin doctor`
   - `npx omp-openai-provider-tools configure-image-agent --model sub2api-openai-image/gpt-5.5-Sys --dry-run`
   - `npx omp-openai-provider-tools configure-image-agent --model sub2api-openai-image/gpt-5.5-Sys`
   - `--print`
   - 重启 OMP 会话提示
5. 第 1 个 code block 的插件版本来自 mock response。把 mock 版本改成 `9.9.9` 时，输出必须变成 `omp-openai-provider-tools@9.9.9`，证明前端没有硬编码 `0.1.2`。
6. 第 1 个 code block 不包含裸包名安装命令或 `@latest`：
   - `omp plugin install npm:omp-openai-provider-tools\n`
   - `omp-openai-provider-tools@latest`
7. 第 2 个 code block 是 `models.yml`，包含：
   - `providers:`
   - `sub2api-openai:`
   - `sub2api-openai-image:`
   - `api: openai-responses`
   - `baseUrl: https://example.com/v1`
   - `apiKey: sk-test`
   - `openaiProviderTools:`
   - `enabled: true`
   - `imageGeneration: true`
   - `sub2api-openai-image/gpt-5.5-Sys: gpt-5.5-image-sys`
8. 第 2 个 code block 不包含：
   - `attachment`
   - `tool_call`
   - `structured_output`
   - `temperature`
   - `release_date`
   - `variants`
   - `modalities`
   - `pdf`
9. 第 3 个 code block 是 `config.yml`，包含完整 provider selector：
   - `default: sub2api-openai/gpt-5.5-Sys`
   - `smol: sub2api-openai/gpt-5.4-mini-Sys`
   - `plan: sub2api-openai/gpt-5.5-Sys`
   - `task: sub2api-openai/gpt-5.5-Sys:xhigh`
10. 第 3 个 code block 不包含裸 canonical role：
   - `task: gpt-5.5-sys:xhigh`
   - `smol: gpt-5.4-mini-sys`
11. 顺序断言：
   - `codeBlocks[0]` 是插件说明；
   - `codeBlocks[1]` 是 `models.yml`；
   - `codeBlocks[2]` 是 `config.yml`。
12. 普通 provider 的 `gpt-5.5-Sys` 不应出现 `imageGeneration: true`；该字段只出现在 `sub2api-openai-image` model 下。测试应解析 YAML 或使用 provider/model 作用域断言，不得只做全局字符串包含/不包含。
13. OMP tab pending 状态显示 loading hint，且不生成真实 provider YAML。
14. OMP tab reject 状态显示 error hint，且不输出缺模型的可用配置。
15. 插件版本 `status: unavailable` 或 `latest_version: ""` 时，第一块必须显示无法生成安装命令的错误提示，且不得输出 `omp plugin install npm:omp-openai-provider-tools@` 这种半截命令。
16. OMP tab 顶部说明不包含 Codex CLI config directory / `.codex` 文案，并包含 OMP、plugin、`~/.omp/agent` 或等价提示。

### 后端单测

修改：

```text
backend/internal/service/opencode_openai_metadata_test.go
backend/internal/server/api_contract_test.go
```

新增或更新测试覆盖：

1. npm latest response `{"version":"0.1.2"}` 会出现在 `omp_openai_provider_tools.latest_version`。
2. npm latest 版本变化时 response 跟随变化，证明没有硬编码版本。
3. npm registry 失败且已有 stale cache 时返回 cached/stale 版本。
4. npm registry 失败且无 stale cache 时，models 仍返回，但 `omp_openai_provider_tools.status` 为 `unavailable` 且 `latest_version` 为空。
5. `api_contract_test.go` 锁定 response 包含 `omp_openai_provider_tools`，但不包含任何凭据。

### 验证命令

前端聚焦测试：

```bash
pnpm exec vitest run src/components/keys/__tests__/UseKeyModal.spec.ts --pool=forks --poolOptions.forks.singleFork --reporter=verbose
```

前端类型检查：

```bash
pnpm typecheck
```

后端服务与契约测试：

```bash
go test -tags unit ./internal/service ./internal/server -run 'Test.*OpenCode.*|TestAPIContract_OpenCodeOpenAIModels' -count=1
```

实现涉及 `OpenCodeMetadataService`、`APIKeyHandler`、Use Key modal 与 i18n，因此至少运行以上三条。

## 风险与取舍

### 风险 1：插件版本获取失败或版本变化

插件版本不能硬编码。如果 npm registry 不可用或返回异常，前端生成半截安装命令会误导用户。

控制方式：

- 后端从 npm latest endpoint 获取版本并缓存。
- 后端允许返回 stale cached 版本，并通过 `status: "cached"` 暴露状态。
- 无可用版本时返回 `status: "unavailable"` 和空 `latest_version`；前端显示错误提示，不生成 `omp plugin install npm:omp-openai-provider-tools@` 半截命令。
- 测试使用 `0.1.2` / `9.9.9` fixture 验证动态版本来源，不能把任何一个版本写成生产常量。

### 风险 2：插件未安装导致生图不可用

`compat.openaiProviderTools` 本身只是模型 metadata。插件未安装时不会注入 provider-native tools。

控制方式：

- 插件说明块必须排在第 1 位。
- `models.yml` 顶部也加 YAML 注释作为二次提醒。
- hint 明确要求安装插件并重启 OMP 会话。

### 风险 3：用户已有 `image_generator`

`configure-image-agent` 默认拒绝覆盖已有 agent。

控制方式：

- 推荐先运行 `--dry-run`。
- 说明 `--print` 可用于人工合并。
- 不在推荐命令中默认使用 `--force`。

### 风险 4：OMP schema 与插件扩展字段

`compat.openaiProviderTools` 是插件消费字段。当前 OMP `model-registry.ts` 中的内置 `OpenAICompatSchema` 未必长期稳定暴露该字段。

控制方式：

- 设计中明确该字段依赖插件。
- 如果 OMP schema 后续拒绝 unknown compat 字段，需要插件或 OMP 侧先扩展 schema；本功能不能绕过 schema 写入无效配置。

### 风险 5：OpenCode 与 OMP 配置语义混用

OpenCode 使用 `variant: image` 和 `metadata.builtin_tools`；OMP 使用插件 `compat.openaiProviderTools` 和独立 image provider。

控制方式：

- 两套生成器分开实现。
- 不复用 OpenCode JSON shape。
- 测试锁定 OMP 输出不包含 OpenCode-only 字段。

## 验收标准

1. OpenAI 平台 Use Key modal 中出现 OMP tab。
2. OMP tab 成功状态输出 3 个可复制块。
3. 第 1 个块必须是插件安装、插件检查和图像子代理配置命令。
4. 第 1 个块必须使用后端返回的 `omp_openai_provider_tools.latest_version`，不得硬编码 `0.1.2`，不得输出 `@latest` 或裸包名安装。
5. 插件版本不可用时不得生成半截安装命令，也不得输出看似完整的可用配置。
6. 第 1 个块必须包含 `configure-image-agent --model sub2api-openai-image/gpt-5.5-Sys --dry-run` 和正式写入命令。
7. 第 2 个块必须是 `~/.omp/agent/models.yml`。
8. 第 3 个块必须是 `~/.omp/agent/config.yml`。
9. 所有模型引用都使用完整 `provider-id/model-id`；例外仅限 `models[].id` 和 `equivalence.overrides` 右侧 canonical id。
10. `sub2api-openai` provider 包含 `compat.openaiProviderTools.enabled: true`。
11. `sub2api-openai-image/gpt-5.5-Sys` 包含 `compat.openaiProviderTools.imageGeneration: true`。
12. 普通 `sub2api-openai/gpt-5.5-Sys` 不包含 `imageGeneration: true`。
13. `equivalence.overrides` 左侧使用完整 provider selector，右侧保留 canonical id。
14. 配置输出不得泄露本机真实 API key。
15. OMP tab 顶部说明不显示 Codex / `.codex` 文案。
16. 现有 OpenCode 推荐配置测试继续通过。
17. 后端插件版本 contract 测试通过。
18. 前端 OMP 输出测试和 `pnpm typecheck` 通过。
