# Agent 配置链接设计

## 背景

OpenAI 平台的 Use Key 弹窗已经能为 OpenCode 和 OMP 生成完整推荐配置。当前 OMP 链路在前端输出 3 个可复制块：插件安装与图像子代理命令、`~/.omp/agent/models.yml`、`~/.omp/agent/config.yml`。这满足人工复制，但对 Agent 来说仍然偏长，容易出现以下问题：

- Agent 需要从 UI 文案中转写 YAML 或 JSON，容易丢缩进、漏字段或改错 provider selector。
- 长配置里同时包含说明、命令和文件内容，Agent 不容易判断哪些内容应写入文件。
- 用户希望一句话非常短，只把一个链接交给 Agent，让 Agent 自己下载需要的独立文件。

已确认的新方向：增加「Agent 配置链接」。Use Key 弹窗继续保留现有完整配置块供用户审计，同时新增一条很短的 Agent 指令链接。Agent 访问链接后拿到 manifest，再按 manifest 下载独立配置文件。

## 现有事实基线

### 已有推荐配置链

- OpenCode 推荐配置仍由前端 `generateOpenCodeConfig(...)` 生成。
- OMP 推荐配置仍由前端 helpers 生成：
  - `generateOMPProviderToolsPluginInstructions(...)`
  - `generateOMPModelsConfig(...)`
  - `generateOMPSettingsConfig()`
- 后端 `GET /api/v1/keys/opencode/openai-models` 当前提供 OpenAI models.dev metadata 与 `omp_openai_provider_tools.latest_version`。
- 在本规格实现前，后端不生成最终 OpenCode / OMP 配置文本；本规格将为 Agent 下载链新增后端生成能力。

### 必须保留的本地语义

本设计不得破坏 `repos/sub2api/AGENTS.md` 记录的本地产品语义，尤其是：

- `sub2api-openai` OpenCode 推荐配置链，不得改成 `provider.openai`。
- OpenCode `-Sys`、Fast、Image variant 与 `metadata.builtin_tools` carrier 语义。
- OMP 独立推荐配置链与 `compat.openaiProviderTools` 语义。
- OpenAI / OpenCode `image_generation` carrier 和 `-Sys` 访问保护语义。

### 安全与日志现状

- 请求日志中间件当前记录 `c.Request.URL.Path`，不是完整 query；但不能把这一点当成新 secret 链接的唯一安全边界。
- `api_key` 允许作为 Agent 专用链接的 GET 参数出现，但实例化链接必须视为 secret。
- 带 `api_key` 的响应必须设置 `Cache-Control: no-store`，避免中间层或浏览器缓存。
- 后端不落库、不生成长期 token、不把 secret 写入日志。

## 目标

1. 在 Use Key 弹窗中为 OpenAI 平台提供短 Agent 指令链接。
2. 链接默认指向后端 manifest endpoint，而不是在前端拼出长篇配置说明。
3. manifest 返回 Agent 可直接消费的 JSON，列出独立下载项和目标路径。
4. OMP 下载项至少包含：
   - 插件安装与图像子代理说明；
   - `models.yml`；
   - `config.yml`；
   - `image-generator.md` 模板。
5. OpenCode 下载项至少包含：
   - `opencode.json`。
6. `base_url` 默认不出现在链接中，由后端从当前请求实例推导为当前服务的 `/v1`。
7. 只有自定义或非默认场景才允许显式传 `base_url`。
8. 继续保留现有完整配置块，方便用户人工审计和手动复制。
9. 对所有带 `api_key` 的 manifest / 文件响应设置 `Cache-Control: no-store`。
10. 测试覆盖 manifest、文件下载、前端短链接、失败路径和敏感信息边界。

## 非目标

1. 不自动写入用户本机文件。
2. 不自动安装 OMP 插件。
3. 不引入数据库表、一次性 token 或服务端持久化状态。
4. 不移除现有 OpenCode / OMP 完整配置块。
5. 不把 OpenCode 与 OMP 的配置 shape 合并成一个通用模板。
6. 不为浏览器用户宣传「把 API key 放 URL」；这是 Agent 专用 secret 链接。
7. 不新增需要认证才能下载的 Agent 配置文件链；链接携带的 `api_key` 就是用于生成配置的用户 key。
8. 不改变 `/v1` 网关行为、OpenAI 模型调度或账号能力判定。

## 产品与 UI 设计

### 展示位置

在 `UseKeyModal.vue` 的 OpenAI 平台下新增一个置顶提示块，建议只在 `opencode` 和 `omp` tab 下展示。该块位于平台描述之后、代码块之前。

文案保持短：

```text
请按此链接完成配置：<url>
```

不同 tab 对应不同链接：

- OMP tab：`/config-guides/omp-openai/manifest.json?api_key=<KEY>`
- OpenCode tab：`/config-guides/opencode-openai/manifest.json?api_key=<KEY>`

如果未来需要传自定义 `base_url`，追加：

```text
&base_url=<encoded-url>
```

默认情况下不带 `base_url`。

### 复制行为

Agent 指令块提供独立「复制」按钮，复制内容就是完整一句话，不复制周边说明。现有文件配置块的复制行为不变。

### 与现有手动配置块的关系

- Agent 指令链接是更短的入口。
- 现有完整配置块继续保留。
- 如果 metadata 加载失败，完整配置块仍按现有失败提示展示；Agent 指令链接也应避免生成会下载到不完整配置的链接。

## HTTP API 设计

### Endpoint 列表

新增公开 endpoint，不挂在 `/api/v1` 下：

```text
GET /config-guides/omp-openai/manifest.json
GET /config-guides/omp-openai/plugin.txt
GET /config-guides/omp-openai/models.yml
GET /config-guides/omp-openai/config.yml
GET /config-guides/omp-openai/image-generator.md
GET /config-guides/opencode-openai/manifest.json
GET /config-guides/opencode-openai/opencode.json
```

这些路径必须加入嵌入式前端 bypass，否则 embed 模式下会被 SPA fallback 吃掉。

### Query 参数

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `api_key` | 是 | 用于写入生成配置的 sub2api API key。 |
| `base_url` | 否 | 自定义 OpenAI-compatible base URL。默认由当前请求推导为当前实例 `/v1`。 |

`api_key` 缺失或为空时返回 `400`，不返回任何配置正文。

### base URL 与链接推导规则

后端需要区分两个概念：

- **配置 `base_url`**：写入 `models.yml` / `opencode.json`，默认指向当前 sub2api 实例的 `/v1`。
- **manifest item `url`**：Agent 下载独立文件用的链接。

当 query 未传 `base_url` 时，后端按以下规则推导配置 `base_url`：

1. host 只使用 `c.Request.Host`，不得从未受信任的 `X-Forwarded-Host` 生成 secret-bearing URL。
2. scheme 默认根据 TLS 判断 `https` 或 `http`；如果存在 `X-Forwarded-Proto`，只读取第一个值，且只接受 `http` / `https`。
3. host 去掉首尾空白后必须非空，且不得包含 CR、LF 或其他控制字符。
4. 结果去掉尾部 `/` 后追加 `/v1`。

首版 manifest item `url` 使用同源相对路径（例如 `/config-guides/omp-openai/models.yml?api_key=...`），由 Agent 按 manifest URL 解析；这样不会因为伪造 `X-Forwarded-Host` 而把带 `api_key` 的下载链接指向攻击者域名。若未来需要绝对下载 URL，必须先引入受信任 public origin 配置或可信代理边界。

当 query 显式传 `base_url` 时：

- 必须使用 `url.Parse` 解析；
- 只接受 `http://` 或 `https://`；
- host 必须非空；
- 不允许 userinfo、query、fragment；
- 不允许 CR、LF 或其他控制字符；
- 去掉尾部 `/`；
- 不额外强制追加 `/v1`，因为传入值代表调用方明确选择。

### manifest 响应

OMP manifest 示例：

```json
{
  "schema_version": 1,
  "client": "omp",
  "title": "sub2api OpenAI for OMP",
  "generated_at": "2026-05-09T00:00:00Z",
  "base_url": "https://example.com/v1",
  "items": [
    {
      "id": "plugin",
      "kind": "instructions",
      "method": "GET",
      "url": "/config-guides/omp-openai/plugin.txt?api_key=sk-test",
      "target_path": null,
      "content_type": "text/plain; charset=utf-8"
    },
    {
      "id": "models",
      "kind": "file",
      "method": "GET",
      "url": "/config-guides/omp-openai/models.yml?api_key=sk-test",
      "target_path": "~/.omp/agent/models.yml",
      "content_type": "application/yaml; charset=utf-8"
    },
    {
      "id": "config",
      "kind": "file",
      "method": "GET",
      "url": "/config-guides/omp-openai/config.yml?api_key=sk-test",
      "target_path": "~/.omp/agent/config.yml",
      "content_type": "application/yaml; charset=utf-8"
    },
    {
      "id": "image-generator",
      "kind": "file",
      "method": "GET",
      "url": "/config-guides/omp-openai/image-generator.md?api_key=sk-test",
      "target_path": "~/.omp/agent/agents/image-generator.md",
      "content_type": "text/markdown; charset=utf-8"
    }
  ],
  "notes": [
    "Run plugin.txt commands before using provider-native web_search or image_generation.",
    "Restart OMP after installing or upgrading plugins and writing agent files."
  ]
}
```

OpenCode manifest 示例：

```json
{
  "schema_version": 1,
  "client": "opencode",
  "title": "sub2api OpenAI for OpenCode",
  "generated_at": "2026-05-09T00:00:00Z",
  "base_url": "https://example.com/v1",
  "items": [
    {
      "id": "opencode",
      "kind": "file",
      "method": "GET",
      "url": "/config-guides/opencode-openai/opencode.json?api_key=sk-test",
      "target_path": "~/.config/opencode/opencode.json",
      "content_type": "application/json; charset=utf-8"
    }
  ],
  "notes": [
    "This config adds provider sub2api-openai and does not replace OpenCode built-in openai provider."
  ]
}
```

### 文件响应内容

#### OMP `plugin.txt`

内容与前端第一块保持语义一致，但后端生成，插件版本来自 `OpenCodeMetadataService.GetOMPProviderToolsMetadata(ctx)`。

如果插件版本不可用，返回 `503`，不得返回半截安装命令。

#### OMP `models.yml`

后端生成内容必须与当前前端 OMP 输出保持一致：

- `sub2api-openai` provider；
- `sub2api-openai-image` provider；
- provider 级 `compat.openaiProviderTools.enabled: true`；
- image model 级 `compat.openaiProviderTools.imageGeneration: true`；
- `equivalence.overrides` 包含：
  - `sub2api-openai/gpt-5.5: gpt-5.5`
  - `sub2api-openai/gpt-5.5-Sys: gpt-5.5-sys`
  - `sub2api-openai/gpt-5.4-mini: gpt-5.4-mini`
  - `sub2api-openai/gpt-5.4-mini-Sys: gpt-5.4-mini-sys`
  - `sub2api-openai-image/gpt-5.5-Sys: gpt-5.5-image-sys`

后端必须先确认 metadata 包含 `gpt-5.5` 和 `gpt-5.4-mini`。缺失时返回 `503`，不得输出引用缺失模型的 YAML。

#### OMP `config.yml`

内容与当前前端 `generateOMPSettingsConfig()` 保持一致。首版继续保留：

```yaml
defaultThinkingLevel: xhigh
serviceTier: priority
```

关于是否默认启用 fast mode 是另一个产品决策，不在本规格中调整。

#### OMP `image-generator.md`

首版可直接由后端生成静态模板，内容表达以下约束：

- agent 名称：`image_generator`；
- 模型：`sub2api-openai-image/gpt-5.5-Sys:xhigh` 或等效 frontmatter 字段；
- 目的：生成或迭代图像；
- 非目标：不要处理普通代码修改任务。

该文件必须不包含 `api_key`。

#### OpenCode `opencode.json`

后端生成内容必须与当前前端 `generateOpenCodeConfig('sub2api-openai', ...)` 的 OpenAI 分支保持语义一致：

- provider id 是 `sub2api-openai`；
- npm 是 `@ai-sdk/openai`；
- provider options 包含 `baseURL` 和 `apiKey`；
- 保留 `gpt-5.5`、`gpt-5.5-Sys`、Fast、Image variant；
- 保留 `agent.image`：
  - `mode: "subagent"`
  - `model: "sub2api-openai/gpt-5.5-fast-Sys"`
  - `variant: "image"`
- 保留本地 `metadata.builtin_tools` carrier。
  - root model `options.metadata.builtin_tools.web_search: true`；

后端必须先确认 metadata 包含 `gpt-5.5` 和 materialized `gpt-5.5-fast`。缺失时返回 `503`，不得输出引用缺失模型的 `opencode.json`。原因是本地推荐配置固定引用 `agent.image.model: "sub2api-openai/gpt-5.5-fast-Sys"`，如果缺少基础或 fast 派生模型，Agent 会写入不可用配置。

## 后端实现设计

### 新 handler

新增 `ConfigGuideHandler`，建议文件：

```text
backend/internal/handler/config_guide_handler.go
```

构造参数：

```go
type ConfigGuideHandler struct {
    openCodeMetadataService *service.OpenCodeMetadataService
    now func() time.Time
}
```

`now` 默认 `time.Now`，测试中可替换以稳定 `generated_at`。

### 生成器边界

推荐在同一 handler 文件中先实现私有生成器函数，避免过早抽象：

- `buildConfigGuideBaseURL(c *gin.Context) (string, error)`
- `buildConfigGuideAbsoluteURL(c *gin.Context, path string, q url.Values) string`
- `renderOpenCodeOpenAIConfig(baseURL, apiKey string, models map[string]service.OpenCodeOpenAIModel) ([]byte, error)`
- `renderOMPPluginInstructions(version string) string`
- `renderOMPModelsYAML(baseURL, apiKey, pluginVersion string, models map[string]service.OpenCodeOpenAIModel) (string, error)`
- `renderOMPSettingsYAML() string`
- `renderOMPImageGeneratorMarkdown() string`

如果前端与后端配置逻辑需要共享，可再拆到 service；首版不要为了可能复用而引入复杂模板系统。

### 路由注册

`handler.Handlers` 新增：

```go
ConfigGuide *ConfigGuideHandler
```

`ProvideHandlers` 和 Wire provider set 添加 `NewConfigGuideHandler`。

`RegisterCommonRoutes` 注册公开路由：

```go
configGuides := r.Group("/config-guides")
{
    omp := configGuides.Group("/omp-openai")
    omp.GET("/manifest.json", h.ConfigGuide.GetOMPManifest)
    omp.GET("/plugin.txt", h.ConfigGuide.GetOMPPluginInstructions)
    omp.GET("/models.yml", h.ConfigGuide.GetOMPModelsYAML)
    omp.GET("/config.yml", h.ConfigGuide.GetOMPConfigYAML)
    omp.GET("/image-generator.md", h.ConfigGuide.GetOMPImageGenerator)

    opencode := configGuides.Group("/opencode-openai")
    opencode.GET("/manifest.json", h.ConfigGuide.GetOpenCodeManifest)
    opencode.GET("/opencode.json", h.ConfigGuide.GetOpenCodeJSON)
}
```

如果 `h == nil` 或 `h.ConfigGuide == nil`，只跳过注册，不影响现有测试中只构造部分 handler 的用例。

### 响应头

所有 manifest 和文件响应，包括所有 `400` / `503` 错误响应，都必须在任何校验、外部 metadata 获取和错误返回之前设置：

```http
Cache-Control: no-store
Pragma: no-cache
```

文件成功响应还应设置准确 `Content-Type`。

### 错误处理

- `api_key` 缺失：`400`。
- `base_url` 非 HTTP(S)：`400`。
- models.dev metadata 不可用：`503`。
- OMP plugin latest 不可用：`503`（仅 OMP plugin / manifest / models 相关响应）。
- required OMP model 缺失：`503`。

错误响应中不得回显 `api_key`。

## 前端实现设计

### API 类型

在 `frontend/src/api/keys.ts` 中新增纯字符串构造 helper，不通过 Axios 调用 manifest，因为 Agent 链接需要完整 URL 文本：

```ts
export function buildAgentConfigGuideURL(client: 'omp' | 'opencode', apiKey: string, baseUrl?: string): string
```

helper 返回 path + query；组件再从 `props.baseUrl` 推导 manifest origin。`props.baseUrl` 通常来自当前实例公开 API base URL，例如 `https://example.com/v1`，组件应去掉末尾 `/v1` 得到 `https://example.com`。如果 `props.baseUrl` 不可用，再回退到 `window.location.origin`。

```ts
/config-guides/omp-openai/manifest.json?api_key=...
```

原因：该 endpoint 不在 `/api/v1` 下，不能使用 `apiClient` 的 `baseURL`；同时不能只依赖 `window.location.origin`，否则反向代理或管理端域名与公开 API 域名不一致时，Agent 可能下载到错误实例。

### 组件状态

`UseKeyModal.vue` 新增一个 `FileConfig` 风格之外的 computed：

```ts
const agentConfigInstruction = computed(() => {
  if (props.platform !== 'openai') return null
  if (!['opencode', 'omp'].includes(activeClientTab.value)) return null
  if (openCodeLoading.value || openCodeError.value || !openCodeModels.value) return null
  if (activeClientTab.value === 'opencode') {
    if (!openCodeModels.value['gpt-5.5'] || !openCodeModels.value['gpt-5.5-fast']) return null
  }
  if (activeClientTab.value === 'omp') {
    if (!openCodeModels.value['gpt-5.5'] || !openCodeModels.value['gpt-5.4-mini']) return null
    const plugin = openCodeMetadata.value?.omp_openai_provider_tools
    if (!plugin || !['ok', 'cached'].includes(plugin.status) || !plugin.latest_version?.trim()) return null
  }
  return t('keys.useKeyModal.agentConfig.instruction', { url })
})
```

OMP tab 在 metadata、required models 或 plugin latest 不可用时不展示 Agent 链接；OpenCode tab 在 metadata 或 required models 不可用时不展示 Agent 链接。这样可以避免 Agent 下载失败后误判配置完成。

### i18n

新增文案：

- `keys.useKeyModal.agentConfig.copy`
- `keys.useKeyModal.agentConfig.copied`
- `keys.useKeyModal.agentConfig.hint`
- `keys.useKeyModal.agentConfig.instruction`

中文 `instruction`：

```text
请按此链接完成配置：{url}
```

英文可为：

```text
Use this link to complete the configuration: {url}
```

## 测试设计

### 后端 service / handler 测试

新增测试文件建议：

```text
backend/internal/handler/config_guide_handler_test.go
```

覆盖：

1. OMP manifest 成功返回 `items`，包含 4 个下载项。
2. OMP manifest 成功与失败响应均包含 `Cache-Control: no-store`。
3. manifest 中下载链接保留 `api_key`，但默认不包含 `base_url`。
4. manifest item `url` 是同源相对路径，不受伪造 `X-Forwarded-Host` 影响。
5. 显式传 `base_url` 时，manifest 下载链接保留 encoded `base_url`，配置文件内容使用该 base URL。
6. 非法 `base_url`（非 HTTP(S)、空 host、userinfo、query、fragment、CR/LF）返回 `400`，且不回显 `api_key`。
7. OMP `models.yml` 包含 `sub2api-openai-image/gpt-5.5-Sys` 与 `imageGeneration: true`。
8. OMP 缺 `gpt-5.4-mini` 时返回 `503`，且响应体不包含 `apiKey: sk-test`。
9. plugin latest 不可用时 OMP `plugin.txt` 返回 `503`，且不包含半截 `omp plugin install npm:omp-openai-provider-tools@`。
10. OMP `image-generator.md` 不包含 `sk-test`。
11. OpenCode `opencode.json` 包含 `sub2api-openai`、`gpt-5.5-fast-Sys`、`variant: "image"`、`metadata.builtin_tools`、Fast `options.serviceTier` 与 headers。
12. OpenCode 缺 `gpt-5.5` 或 `gpt-5.5-fast` 时返回 `503`，且响应体不包含 `apiKey: sk-test`。
13. `api_key` 缺失返回 `400`，并带 `Cache-Control: no-store`。

### 后端契约测试

在 `backend/internal/server/api_contract_test.go` 中扩展测试路由构造，确保：

- `/config-guides/omp-openai/manifest.json?api_key=sk-test` 可从集成 router 返回。
- response body 不包含 `npm_token` / `NPM_TOKEN`。
- `Cache-Control` 为 `no-store`。

### 前端测试

扩展 `UseKeyModal.spec.ts`：

1. OMP tab metadata 成功时显示短 Agent 指令。
2. 指令内容只是一句话加 URL，不包含长 YAML。
3. OMP metadata 加载失败时不显示 Agent 指令链接。
4. OpenCode required model metadata 缺失时不显示 Agent 指令链接。
5. OpenCode tab metadata 成功时显示 opencode manifest 链接。
6. 默认链接不带 `base_url=`。
7. 链接 query 包含 encoded `api_key`。
8. 现有 3 个 OMP 配置块仍保留。

## 验收标准

1. Use Key modal 的 OpenAI + OMP tab 显示短 Agent 指令链接，并保留现有 3 个配置块。
2. Use Key modal 的 OpenAI + OpenCode tab 显示短 Agent 指令链接，并保留现有 `opencode.json` 配置块。
3. 访问 OMP manifest 可拿到 4 个可下载项；每个下载项能返回独立文件内容。
4. 访问 OpenCode manifest 可拿到 `opencode.json` 下载项。
5. 默认链接不包含 `base_url`，后端能按请求推导当前实例 `/v1`。
6. 带 `api_key` 的响应均为 `Cache-Control: no-store`。
7. 任何失败响应不得回显 API key，也不得输出半截插件安装命令。
8. OpenCode 输出继续保留 `sub2api-openai`、Fast、Image variant 与 `metadata.builtin_tools`。
9. OMP 输出继续保留 `compat.openaiProviderTools`、独立 image provider 和完整 provider selector。
10. 相关后端测试、前端 Vitest、前端 typecheck 和 `git diff --check` 通过。

## 实现顺序建议

1. 后端先落 handler 单元测试和生成器，确保 manifest / 文件下载可独立工作。
2. 接入 route、handler wire 和 embed bypass，再补 contract test。
3. 前端加入短链接 UI 和 i18n。
4. 前端补 Vitest。
5. 跑完整聚焦验证。

## 风险与缓解

### 风险：前后端配置生成逻辑重复

首版接受少量重复，因为目标是让 Agent 下载独立文件。缓解方式：后端测试锁定关键语义；未来如配置模板继续扩张，再抽共享生成器或后端统一输出。

### 风险：URL 中携带 API key

这是用户明确接受的 Agent 专用约束。缓解方式：所有相关响应 `no-store`，后端不落库，错误不回显 key，访问日志只记录 path，不记录 query，并增加脱敏测试。

### 风险：embed 模式路由被 SPA fallback 吃掉

必须更新 `shouldBypassEmbeddedFrontend`，把 `/config-guides/` 加入 bypass，并补测试。

### 风险：Agent 误把 manifest 当成最终文件

manifest 使用明确 `items[].target_path` 和 `items[].content_type`，并在 notes 中说明需要下载各 item。

### 风险：默认 base URL 推导在反向代理后错误

实现时只信任 `X-Forwarded-Proto` 的第一个 `http` / `https` 值，不信任 `X-Forwarded-Host`；host 使用 `Request.Host` 或未来显式配置的 public origin。测试覆盖伪造 forwarded host 不进入 manifest item URL。
