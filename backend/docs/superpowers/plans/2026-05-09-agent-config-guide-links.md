# Agent 配置链接实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 在 Use Key modal 的 OpenAI 平台下新增短 Agent 配置链接，由后端 manifest 暴露 OMP / OpenCode 独立配置文件下载项，减少 Agent 转写长配置时的错误。

**架构：** 新增公开 `/config-guides/*` 后端 endpoint。后端根据 `api_key` 和可选 `base_url` 生成 manifest 与独立文件内容，并复用 `OpenCodeMetadataService` 获取 models.dev 与 OMP 插件 npm latest metadata。前端只负责展示一句短指令链接，同时保留现有 OpenCode / OMP 完整配置块供人工审计。

**技术栈：** Go 1.26、Gin、`net/http`、Vue 3、TypeScript、Vitest、vue-i18n。

---

## 规格与边界

规格文件：`repos/sub2api/backend/docs/superpowers/specs/2026-05-09-agent-config-guide-links-design.md`

项目规则：`repos/sub2api/AGENTS.md`

硬性边界：

- 不使用 worktree，直接在当前 `repos/sub2api` 主工作树开发。
- 不移除现有完整 OpenCode / OMP 配置块。
- 不自动写入用户本机文件，不自动安装插件。
- 不新增数据库表、一次性 token 或持久化状态。
- 带 `api_key` 的链接是 secret；相关响应必须 `Cache-Control: no-store`。
- 错误响应不得回显 `api_key`。
- `base_url` 默认不进前端链接，由后端按请求实例推导为 `/v1`。
- OpenCode 必须保留 `sub2api-openai`、Fast、Image variant、`metadata.builtin_tools` 和 `agent.image` 语义。
- OMP 必须保留 `compat.openaiProviderTools`、独立 image provider、完整 provider selector。
- 插件版本不可用时不得输出半截 `omp plugin install npm:omp-openai-provider-tools@`。

## 文件结构

### 后端

- 创建：`repos/sub2api/backend/internal/handler/config_guide_handler.go`
  - 新增 `ConfigGuideHandler`。
  - 生成 OMP / OpenCode manifest。
  - 生成 OMP `plugin.txt`、`models.yml`、`config.yml`、`image-generator.md`。
  - 生成 OpenCode `opencode.json`。
  - 处理 `api_key`、`base_url`、`Cache-Control`、`Content-Type` 与错误响应。
- 创建：`repos/sub2api/backend/internal/handler/config_guide_handler_test.go`
  - 直接测试 handler / 生成器行为。
- 修改：`repos/sub2api/backend/internal/handler/handler.go`
  - `Handlers` 增加 `ConfigGuide *ConfigGuideHandler`。
- 修改：`repos/sub2api/backend/internal/handler/wire.go`
  - `ProvideHandlers` 接收并赋值 `ConfigGuideHandler`。
  - `ProviderSet` 增加 `NewConfigGuideHandler`。
- 修改：`repos/sub2api/backend/cmd/server/wire_gen.go`
  - 运行 `go generate ./cmd/server` 后更新 Wire 生成的生产 injector。
- 修改：`repos/sub2api/backend/internal/server/routes/common.go`
  - 注册 `/config-guides/omp-openai/*` 和 `/config-guides/opencode-openai/*`。
- 修改：`repos/sub2api/backend/internal/web/embed_bypass.go`
  - 加入 `/config-guides/` bypass。
- 修改：`repos/sub2api/backend/internal/web/embed_bypass_test.go`
  - 覆盖 `/config-guides/omp-openai/manifest.json` bypass。
- 修改：`repos/sub2api/backend/internal/server/api_contract_test.go`
  - 集成 contract router 注册 `ConfigGuideHandler`。
  - 增加 manifest contract 测试。
- 修改：`repos/sub2api/backend/cmd/server/wire.go`
  - 如现有 Wire injector 声明需要新增 provider，按生成器要求同步调整。

### 前端

- 修改：`repos/sub2api/frontend/src/api/keys.ts`
  - 新增 Agent 配置链接 client 类型与 path 构造 helper。
- 修改：`repos/sub2api/frontend/src/components/keys/UseKeyModal.vue`
  - 新增短 Agent 指令 UI。
  - OMP / OpenCode tab metadata 可用时展示 manifest 链接。
  - 保留现有文件配置块。
- 修改：`repos/sub2api/frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`
  - 增加 Agent 链接 UI、默认不带 `base_url`、失败隐藏等测试。
- 修改：`repos/sub2api/frontend/src/i18n/locales/zh.ts`
  - 新增 Agent 配置链接中文文案。
- 修改：`repos/sub2api/frontend/src/i18n/locales/en.ts`
  - 新增 Agent 配置链接英文文案。

## 子代理并发边界

- 后端任务 1-3 都修改 `backend/internal/handler/config_guide_handler.go` 和 `backend/internal/handler/config_guide_handler_test.go`，必须串行或由同一个后端子代理连续完成，不能并行编辑同一文件。
- 后端任务 4 修改路由、Wire、embed bypass 和 contract test，可在任务 1-3 完成并稳定 handler API 后由另一个子代理执行。
- 前端任务 5 和任务 6 都会修改 `UseKeyModal.spec.ts`；为避免并发冲突，合并为同一个前端子代理任务执行，不拆分给两个子代理。
- 后端路由/Wire 与前端 UI 可并行，因为它们触碰文件不重叠；最终由主代理统一运行验证。

---

## 任务 1：后端 ConfigGuide handler 单元测试

**文件：**
- 创建：`repos/sub2api/backend/internal/handler/config_guide_handler_test.go`
- 修改：`repos/sub2api/backend/internal/handler/config_guide_handler.go`

- [ ] **步骤 1：创建失败的 handler 测试骨架**

创建 `repos/sub2api/backend/internal/handler/config_guide_handler_test.go`，先写测试 helper 和第一个失败测试。

建议代码骨架：

```go
package handler

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
    "time"
    "io"

    "github.com/Wei-Shaw/sub2api/internal/service"

    "github.com/gin-gonic/gin"
    "github.com/stretchr/testify/require"
)

func newConfigGuideTestRouter(h *ConfigGuideHandler) *gin.Engine {
    gin.SetMode(gin.TestMode)
    r := gin.New()
    g := r.Group("/config-guides")
    omp := g.Group("/omp-openai")
    omp.GET("/manifest.json", h.GetOMPManifest)
    omp.GET("/plugin.txt", h.GetOMPPluginInstructions)
    omp.GET("/models.yml", h.GetOMPModelsYAML)
    omp.GET("/config.yml", h.GetOMPConfigYAML)
    omp.GET("/image-generator.md", h.GetOMPImageGenerator)
    opencode := g.Group("/opencode-openai")
    opencode.GET("/manifest.json", h.GetOpenCodeManifest)
    opencode.GET("/opencode.json", h.GetOpenCodeJSON)
    return r
}

func configGuideModelsDevPayload() map[string]any {
    return map[string]any{
        "openai": map[string]any{
            "models": map[string]any{
                "gpt-5.5": map[string]any{
                    "id": "gpt-5.5",
                    "name": "GPT-5.5",
                    "reasoning": true,
                    "attachment": true,
                    "tool_call": true,
                    "structured_output": true,
                    "temperature": false,
                    "modalities": map[string]any{"input": []any{"text", "image", "pdf"}, "output": []any{"text"}},
                    "cost": map[string]any{"input": 2.5, "output": 15.0, "cache_read": 0.25, "cache_write": 3.75},
                    "limit": map[string]any{"context": 400000, "input": 272000, "output": 128000},
                    "experimental": map[string]any{
                        "modes": map[string]any{
                            "fast": map[string]any{
                                "cost": map[string]any{"input": 5.0, "output": 30.0, "cache_read": 0.5},
                                "provider": map[string]any{
                                    "body": map[string]any{"service_tier": "priority"},
                                    "headers": map[string]any{"x-test-header": "fast-mode"},
                                },
                            },
                        },
                    },
                },
                "gpt-5.4-mini": map[string]any{
                    "id": "gpt-5.4-mini",
                    "name": "GPT-5.4 Mini",
                    "reasoning": true,
                    "attachment": true,
                    "tool_call": true,
                    "structured_output": true,
                    "temperature": false,
                    "modalities": map[string]any{"input": []any{"text", "image", "pdf"}, "output": []any{"text"}},
                    "cost": map[string]any{"input": 0.25, "output": 2.0, "cache_read": 0.025},
                    "limit": map[string]any{"context": 400000, "input": 272000, "output": 128000},
                },
            },
        },
    }
}
```

再写第一个测试：

```go
func TestConfigGuideOMPManifest(t *testing.T) {
    h := newConfigGuideTestHandler(t, configGuideModelsDevPayload(), "0.1.2")
    h.now = func() time.Time { return time.Date(2026, 5, 9, 0, 0, 0, 0, time.UTC) }
    r := newConfigGuideTestRouter(h)

    w := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/config-guides/omp-openai/manifest.json?api_key=sk-test", nil)
    req.Host = "example.com"
    req.Header.Set("X-Forwarded-Proto", "https")
    r.ServeHTTP(w, req)

    require.Equal(t, http.StatusOK, w.Code)
    require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
    require.Equal(t, "no-cache", w.Header().Get("Pragma"))

    var manifest configGuideManifest
    require.NoError(t, json.Unmarshal(w.Body.Bytes(), &manifest))
    require.Equal(t, 1, manifest.SchemaVersion)
    require.Equal(t, "omp", manifest.Client)
    require.Equal(t, "https://example.com/v1", manifest.BaseURL)
    require.Len(t, manifest.Items, 4)
    require.Equal(t, "models", manifest.Items[1].ID)
    require.NotNil(t, manifest.Items[1].TargetPath)
    require.Equal(t, "~/.omp/agent/models.yml", *manifest.Items[1].TargetPath)
    require.Contains(t, manifest.Items[1].URL, "/config-guides/omp-openai/models.yml?api_key=sk-test")
    require.NotContains(t, manifest.Items[1].URL, "base_url=")
}
```

- [ ] **步骤 2：运行测试确认失败**

运行：

```bash
(cd backend && C:/Users/34404/Documents/GitHub/workbench/toolchains/go/bin/go.exe test -tags unit ./internal/handler -run TestConfigGuideOMPManifest -count=1)
```

预期：编译失败，提示 `ConfigGuideHandler` / `configGuideManifest` / `newConfigGuideTestHandler` 未定义。

- [ ] **步骤 3：实现最小 handler 类型、manifest 类型和测试 helper**

在 `config_guide_handler.go` 中加入：

```go
package handler

import (
    "net/http"
    "time"
    "strings"

    "github.com/Wei-Shaw/sub2api/internal/pkg/response"
    "github.com/Wei-Shaw/sub2api/internal/service"

    "github.com/gin-gonic/gin"
)

type ConfigGuideHandler struct {
    openCodeMetadataService *service.OpenCodeMetadataService
    now                     func() time.Time
}

func NewConfigGuideHandler(openCodeMetadataService *service.OpenCodeMetadataService) *ConfigGuideHandler {
    return &ConfigGuideHandler{
        openCodeMetadataService: openCodeMetadataService,
        now:                     time.Now,
    }
}

type configGuideManifest struct {
    SchemaVersion int               `json:"schema_version"`
    Client        string            `json:"client"`
    Title         string            `json:"title"`
    GeneratedAt   string            `json:"generated_at"`
    BaseURL       string            `json:"base_url"`
    Items         []configGuideItem `json:"items"`
    Notes         []string          `json:"notes,omitempty"`
}

type configGuideItem struct {
    ID          string  `json:"id"`
    Kind        string  `json:"kind"`
    Method      string  `json:"method"`
    URL         string  `json:"url"`
    TargetPath  *string `json:"target_path"`
    ContentType string  `json:"content_type"`
}

func setConfigGuideNoStore(c *gin.Context) {
    c.Header("Cache-Control", "no-store")
    c.Header("Pragma", "no-cache")
}

func (h *ConfigGuideHandler) currentTime() time.Time {
    if h != nil && h.now != nil {
        return h.now()
    }
    return time.Now()
}

func (h *ConfigGuideHandler) GetOMPManifest(c *gin.Context) {
    setConfigGuideNoStore(c)
    if strings.TrimSpace(c.Query("api_key")) == "" {
        response.BadRequest(c, "api_key is required")
        return
    }
    // 先返回空 manifest，用于确认红绿流程中的断言失败。
    c.JSON(http.StatusOK, configGuideManifest{})
}
```

同时在测试文件补 `newConfigGuideTestHandler`。不要访问 `service.OpenCodeMetadataService` 的未导出字段；`handler` 包无法访问 `service` 包私有字段。测试 helper 应使用 `http.DefaultTransport` 拦截 `https://models.dev/api.json` 与 `https://registry.npmjs.org/omp-openai-provider-tools/latest`，并在 `t.Cleanup` 中恢复原 transport；`service.NewOpenCodeMetadataService()` 内部默认 client 的 `Transport` 为 nil，请求时会读取 `http.DefaultTransport`。这些测试不得调用 `t.Parallel()`，避免全局 transport 互相污染。helper 构造 `NewConfigGuideHandler(service.NewOpenCodeMetadataService())`，当 npm version 为空时让 registry 响应 `502` 以模拟 unavailable。

- [ ] **步骤 4：运行测试观察断言失败**

运行同上命令。

预期：编译通过但断言失败，因为 manifest 为空。

- [ ] **步骤 5：实现 base URL 推导和 OMP manifest**

在 `config_guide_handler.go` 中实现：

```go
func configGuideQuery(c *gin.Context) (apiKey string, baseURL string, err error)
func deriveConfigGuideBaseURL(c *gin.Context) (string, error)
func firstForwardedValue(raw string) string
func absoluteConfigGuideURL(c *gin.Context, path string, query url.Values) string
func strPtr(v string) *string
```

`GetOMPManifest` 逻辑：

1. 校验 `api_key`。
2. 推导 `base_url`。
3. 获取 OMP plugin metadata；不可用时 `503`。
4. 获取 OpenAI metadata；失败时 `503`。
5. 校验 `gpt-5.5` 和 `gpt-5.4-mini` 存在。
6. 返回 4 个 item；每个 item 的 `url` 使用同源相对路径，不使用 `X-Forwarded-Host`。
7. 对缺失 `api_key`、非法 `base_url`、伪造 `X-Forwarded-Host`、metadata failure、required model 缺失都在返回错误前保留 `Cache-Control: no-store` 与 `Pragma: no-cache`。

注意：manifest 里的下载 URL 要包含 `api_key`；默认情况下不包含 `base_url`。如果请求显式传了 `base_url`，下载 URL 才带上 `base_url`。

- [ ] **步骤 6：运行测试验证通过**

运行：

```bash
(cd backend && C:/Users/34404/Documents/GitHub/workbench/toolchains/go/bin/go.exe test -tags unit ./internal/handler -run TestConfigGuideOMPManifest -count=1)
```

预期：PASS。

---

## 任务 2：后端 OMP 文件下载实现

**文件：**
- 修改：`repos/sub2api/backend/internal/handler/config_guide_handler.go`
- 修改：`repos/sub2api/backend/internal/handler/config_guide_handler_test.go`

- [ ] **步骤 1：编写 OMP 文件下载测试**

在 `config_guide_handler_test.go` 追加：

```go
func TestConfigGuideOMPModelsYAML(t *testing.T) {
    h := newConfigGuideTestHandler(t, configGuideModelsDevPayload(), "9.9.9")
    r := newConfigGuideTestRouter(h)

    w := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/config-guides/omp-openai/models.yml?api_key=sk-test", nil)
    req.Host = "example.com"
    req.Header.Set("X-Forwarded-Proto", "https")
    r.ServeHTTP(w, req)

    require.Equal(t, http.StatusOK, w.Code)
    require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
    body := w.Body.String()
    require.Contains(t, body, "omp plugin install npm:omp-openai-provider-tools@9.9.9")
    require.Contains(t, body, "sub2api-openai:")
    require.Contains(t, body, "baseUrl: https://example.com/v1")
    require.Contains(t, body, "apiKey: sk-test")
    require.Contains(t, body, "sub2api-openai-image:")
    require.Contains(t, body, "imageGeneration: true")
    require.Contains(t, body, "sub2api-openai/gpt-5.4-mini-Sys: gpt-5.4-mini-sys")
    require.NotContains(t, body, "pdf")
}

func TestConfigGuideOMPConfigYAML(t *testing.T) {
    h := newConfigGuideTestHandler(t, configGuideModelsDevPayload(), "0.1.2")
    r := newConfigGuideTestRouter(h)

    w := httptest.NewRecorder()
    r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/config-guides/omp-openai/config.yml?api_key=sk-test", nil))

    require.Equal(t, http.StatusOK, w.Code)
    body := w.Body.String()
    require.Contains(t, body, "defaultThinkingLevel: xhigh")
    require.Contains(t, body, "serviceTier: priority")
    require.Contains(t, body, "default: sub2api-openai/gpt-5.5-Sys")
    require.NotContains(t, body, "sk-test")
}

func TestConfigGuideOMPPluginUnavailableDoesNotRenderPartialCommand(t *testing.T) {
    h := newConfigGuideTestHandler(t, configGuideModelsDevPayload(), "")
    r := newConfigGuideTestRouter(h)

    w := httptest.NewRecorder()
    r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/config-guides/omp-openai/plugin.txt?api_key=sk-test", nil))

    require.Equal(t, http.StatusServiceUnavailable, w.Code)
    require.NotContains(t, w.Body.String(), "omp plugin install npm:omp-openai-provider-tools@")
    require.NotContains(t, w.Body.String(), "sk-test")
}

func TestConfigGuideOMPImageGeneratorDoesNotContainAPIKey(t *testing.T) {
    h := newConfigGuideTestHandler(t, configGuideModelsDevPayload(), "0.1.2")
    r := newConfigGuideTestRouter(h)

    w := httptest.NewRecorder()
    r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/config-guides/omp-openai/image-generator.md?api_key=sk-test", nil))

    require.Equal(t, http.StatusOK, w.Code)
    require.Contains(t, w.Body.String(), "image_generator")
    require.Contains(t, w.Body.String(), "sub2api-openai-image/gpt-5.5-Sys")
    require.NotContains(t, w.Body.String(), "sk-test")
}

func TestConfigGuideOMPErrorsAreNoStoreAndDoNotEchoAPIKey(t *testing.T) {
    h := newConfigGuideTestHandler(t, configGuideModelsDevPayload(), "0.1.2")
    r := newConfigGuideTestRouter(h)

    cases := []struct {
        target string
        wantStatus int
    }{
        {"/config-guides/omp-openai/manifest.json", http.StatusBadRequest},
        {"/config-guides/omp-openai/manifest.json?api_key=sk-test&base_url=javascript:alert(1)", http.StatusBadRequest},
        {"/config-guides/omp-openai/manifest.json?api_key=sk-test&base_url=https://", http.StatusBadRequest},
        {"/config-guides/omp-openai/manifest.json?api_key=sk-test&base_url=https://user:pass@example.com/v1", http.StatusBadRequest},
        {"/config-guides/omp-openai/manifest.json?api_key=sk-test&base_url=https://example.com/v1?x=1", http.StatusBadRequest},
        {"/config-guides/omp-openai/manifest.json?api_key=sk-test&base_url=https://example.com/v1%0d%0aapiKey:%20sk-test", http.StatusBadRequest},
    }
    for _, tc := range cases {
        w := httptest.NewRecorder()
        r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.target, nil))
        require.Equal(t, tc.wantStatus, w.Code, tc.target)
        require.Equal(t, "no-store", w.Header().Get("Cache-Control"), tc.target)
        require.Equal(t, "no-cache", w.Header().Get("Pragma"), tc.target)
        require.NotContains(t, w.Body.String(), "sk-test", tc.target)
    }
}

func TestConfigGuideOMPManifestIgnoresSpoofedForwardedHost(t *testing.T) {
    h := newConfigGuideTestHandler(t, configGuideModelsDevPayload(), "0.1.2")
    r := newConfigGuideTestRouter(h)

    w := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/config-guides/omp-openai/manifest.json?api_key=sk-test", nil)
    req.Host = "example.com"
    req.Header.Set("X-Forwarded-Proto", "https")
    req.Header.Set("X-Forwarded-Host", "attacker.example")
    r.ServeHTTP(w, req)

    require.Equal(t, http.StatusOK, w.Code)
    require.Contains(t, w.Body.String(), `"base_url":"https://example.com/v1"`)
    require.NotContains(t, w.Body.String(), "attacker.example")
}

func TestConfigGuideOMPMissingRequiredModelFailsClosed(t *testing.T) {
    payload := configGuideModelsDevPayload()
    delete(payload["openai"].(map[string]any)["models"].(map[string]any), "gpt-5.4-mini")
    h := newConfigGuideTestHandler(t, payload, "0.1.2")
    r := newConfigGuideTestRouter(h)

    w := httptest.NewRecorder()
    r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/config-guides/omp-openai/models.yml?api_key=sk-test", nil))

    require.Equal(t, http.StatusServiceUnavailable, w.Code)
    require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
    require.NotContains(t, w.Body.String(), "apiKey: sk-test")
}
```

- [ ] **步骤 2：运行测试确认失败**

运行：

```bash
(cd backend && C:/Users/34404/Documents/GitHub/workbench/toolchains/go/bin/go.exe test -tags unit ./internal/handler -run "TestConfigGuideOMP(ModelsYAML|ConfigYAML|PluginUnavailable|ImageGenerator)" -count=1)
```

预期：失败，handler 方法或内容未实现。

- [ ] **步骤 3：实现 OMP 生成器与下载 handler**

在 `config_guide_handler.go` 中实现：

- `GetOMPPluginInstructions`
- `GetOMPModelsYAML`
- `GetOMPConfigYAML`
- `GetOMPImageGenerator`
- `renderOMPPluginInstructions(version string) string`
- `renderOMPModelsYAML(baseURL, apiKey, pluginVersion string, models map[string]service.OpenCodeOpenAIModel) (string, error)`
- `renderOMPSettingsYAML() string`
- `renderOMPImageGeneratorMarkdown() string`

生成 YAML 时保持前端现有输出语义：

```yaml
providers:
  sub2api-openai:
    api: openai-responses
    baseUrl: <baseURL>
    apiKey: <apiKey>
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
```

注意：

- input 只允许 `text` / `image`。
- `gpt-5.5-Sys` 由 `gpt-5.5` 派生。
- `gpt-5.4-mini-Sys` 由 `gpt-5.4-mini` 派生。
- `sub2api-openai-image/gpt-5.5-Sys` 使用 image provider。
- 插件版本不可用时，`plugin.txt`、`models.yml` 和 OMP manifest 都返回 `503`，避免输出注释里的半截命令。
- 所有 handler 开头先调用 `setConfigGuideNoStore(c)`，再做任何参数校验或外部 metadata 获取；错误响应也必须保留 `Cache-Control: no-store` 和 `Pragma: no-cache`。
- `base_url` 使用 `url.Parse` 严格校验：只允许 HTTP(S)、非空 host、无 userinfo、无 query/fragment、无 CR/LF/control chars。

- [ ] **步骤 4：运行 OMP 文件测试验证通过**

运行：

```bash
(cd backend && C:/Users/34404/Documents/GitHub/workbench/toolchains/go/bin/go.exe test -tags unit ./internal/handler -run "TestConfigGuideOMP" -count=1)
```

预期：PASS。

---

## 任务 3：后端 OpenCode manifest 与 opencode.json

**文件：**
- 修改：`repos/sub2api/backend/internal/handler/config_guide_handler.go`
- 修改：`repos/sub2api/backend/internal/handler/config_guide_handler_test.go`

- [ ] **步骤 1：编写 OpenCode 测试**

追加：

```go
func TestConfigGuideOpenCodeManifest(t *testing.T) {
    h := newConfigGuideTestHandler(t, configGuideModelsDevPayload(), "0.1.2")
    h.now = func() time.Time { return time.Date(2026, 5, 9, 0, 0, 0, 0, time.UTC) }
    r := newConfigGuideTestRouter(h)

    w := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/config-guides/opencode-openai/manifest.json?api_key=sk-test", nil)
    req.Host = "example.com"
    req.Header.Set("X-Forwarded-Proto", "https")
    r.ServeHTTP(w, req)

    require.Equal(t, http.StatusOK, w.Code)
    var manifest configGuideManifest
    require.NoError(t, json.Unmarshal(w.Body.Bytes(), &manifest))
    require.Equal(t, "opencode", manifest.Client)
    require.Len(t, manifest.Items, 1)
    require.Equal(t, "opencode", manifest.Items[0].ID)
    require.NotNil(t, manifest.Items[0].TargetPath)
    require.Equal(t, "~/.config/opencode/opencode.json", *manifest.Items[0].TargetPath)
    require.Contains(t, manifest.Items[0].URL, "/config-guides/opencode-openai/opencode.json?api_key=sk-test")
}

func TestConfigGuideOpenCodeJSONPreservesLocalSemantics(t *testing.T) {
    h := newConfigGuideTestHandler(t, configGuideModelsDevPayload(), "0.1.2")
    r := newConfigGuideTestRouter(h)

    w := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/config-guides/opencode-openai/opencode.json?api_key=sk-test", nil)
    req.Host = "example.com"
    req.Header.Set("X-Forwarded-Proto", "https")
    r.ServeHTTP(w, req)

    require.Equal(t, http.StatusOK, w.Code)
    var cfg map[string]any
    require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cfg))
    provider := cfg["provider"].(map[string]any)["sub2api-openai"].(map[string]any)
    require.Equal(t, "sk-test", provider["options"].(map[string]any)["apiKey"])
    require.Equal(t, "https://example.com/v1", provider["options"].(map[string]any)["baseURL"])
    models := provider["models"].(map[string]any)
    fast := models["gpt-5.5-fast"].(map[string]any)
    require.Equal(t, "gpt-5.5", fast["id"])
    require.Equal(t, "priority", fast["options"].(map[string]any)["serviceTier"])
    require.Equal(t, "fast-mode", fast["headers"].(map[string]any)["x-test-header"])
    fastSys := models["gpt-5.5-fast-Sys"].(map[string]any)
    require.Equal(t, "gpt-5.5-Sys", fastSys["id"])
    require.Equal(t, "priority", fastSys["options"].(map[string]any)["serviceTier"])
    for _, modelID := range []string{"gpt-5.5", "gpt-5.5-fast", "gpt-5.5-Sys", "gpt-5.5-fast-Sys"} {
        model := models[modelID].(map[string]any)
        options := model["options"].(map[string]any)
        metadata := options["metadata"].(map[string]any)
        builtinTools := metadata["builtin_tools"].(map[string]any)
        require.Equal(t, true, builtinTools["web_search"], modelID)
        require.Equal(t, false, options["store"], modelID)
    }
    sys := models["gpt-5.5-Sys"].(map[string]any)
    sysImageVariant := sys["variants"].(map[string]any)["image"].(map[string]any)
    sysImageMetadata := sysImageVariant["metadata"].(map[string]any)
    require.Contains(t, sysImageMetadata["builtin_tools"], "image_generation")
    fastSysImageVariant := fastSys["variants"].(map[string]any)["image"].(map[string]any)
    fastSysImageMetadata := fastSysImageVariant["metadata"].(map[string]any)
    require.Contains(t, fastSysImageMetadata["builtin_tools"], "image_generation")
    agent := cfg["agent"].(map[string]any)["image"].(map[string]any)
    require.Equal(t, "sub2api-openai/gpt-5.5-fast-Sys", agent["model"])
    require.Equal(t, "image", agent["variant"])
    body := w.Body.String()
    require.NotContains(t, body, `"experimental"`)
    require.NotContains(t, body, `"service_tier"`)
}

func TestConfigGuideOpenCodeMissingRequiredModelFailsClosed(t *testing.T) {
    cases := []struct {
        name   string
        mutate func(map[string]any)
    }{
        {
            name: "missing base model",
            mutate: func(payload map[string]any) {
                delete(payload["openai"].(map[string]any)["models"].(map[string]any), "gpt-5.5")
            },
        },
        {
            name: "missing fast mode",
            mutate: func(payload map[string]any) {
                gpt55 := payload["openai"].(map[string]any)["models"].(map[string]any)["gpt-5.5"].(map[string]any)
                modes := gpt55["experimental"].(map[string]any)["modes"].(map[string]any)
                delete(modes, "fast")
            },
        },
    }

    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            payload := configGuideModelsDevPayload()
            tc.mutate(payload)
            h := newConfigGuideTestHandler(t, payload, "0.1.2")
            r := newConfigGuideTestRouter(h)

            w := httptest.NewRecorder()
            r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/config-guides/opencode-openai/opencode.json?api_key=sk-test", nil))

            require.Equal(t, http.StatusServiceUnavailable, w.Code)
            require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
            require.NotContains(t, w.Body.String(), "apiKey")
            require.NotContains(t, w.Body.String(), "sk-test")
            require.NotContains(t, w.Body.String(), "gpt-5.5-fast-Sys")
        })
    }
}
```

OpenCode fail-closed 测试必须同时覆盖 manifest 和文件下载 endpoint；manifest 不应在 required models 不完整时返回可下载链接。可以把上述测试的请求目标扩展为 `[]string{"/config-guides/opencode-openai/manifest.json?api_key=sk-test", "/config-guides/opencode-openai/opencode.json?api_key=sk-test"}`，对每个 target 都断言 `503`、`Cache-Control: no-store`、不包含 `sk-test`，并且不包含 `gpt-5.5-fast-Sys`。

- [ ] **步骤 2：运行测试确认失败**

运行：

```bash
(cd backend && C:/Users/34404/Documents/GitHub/workbench/toolchains/go/bin/go.exe test -tags unit ./internal/handler -run "TestConfigGuideOpenCode" -count=1)
```

预期：失败，OpenCode handler 或 JSON 内容未实现。

- [ ] **步骤 3：实现 OpenCode manifest 和 JSON 生成器**

在 `config_guide_handler.go` 实现：

- `GetOpenCodeManifest`
- `GetOpenCodeJSON`
- `renderOpenCodeOpenAIConfig(baseURL, apiKey string, models map[string]service.OpenCodeOpenAIModel) ([]byte, error)`

JSON shape 与前端 `generateOpenCodeConfig('sub2api-openai', ...)` 对齐。必须包含：

```json
{
  "provider": {
    "sub2api-openai": {
      "npm": "@ai-sdk/openai",
      "name": "sub2api OpenAI",
      "options": {
        "baseURL": "https://example.com/v1",
        "apiKey": "sk-test"
      },
      "models": {}
    }
  },
  "agent": {
    "build": { "options": { "store": false } },
    "plan": { "options": { "store": false } },
    "image": {
      "mode": "subagent",
      "description": "Generate images with GPT-5.5 Image Fast (Sys)",
      "model": "sub2api-openai/gpt-5.5-fast-Sys",
      "variant": "image",
      "options": { "store": false }
    }
  },
  "$schema": "https://opencode.ai/config.json"
}
```

模型生成规则应复用后端 `OpenCodeMetadataService` 已返回的 models，包括 fast 派生模型。为 `gpt-5.5-Sys` / fast Sys / image variant 补本地语义时，必须参考当前前端 helper，不能退化为 models.dev 原样输出。
- `gpt-5.5-fast` 的 map key 是 fast selector，但内部 `id` 必须回写为 `gpt-5.5`；`gpt-5.5-fast-Sys` 内部 `id` 必须回写为 `gpt-5.5-Sys`。
- Fast 派生模型必须保留 camelCase `options.serviceTier: "priority"` 与 headers，不得把 upstream `service_tier` 原样写入 OpenCode JSON。
- Sys 派生模型必须继承 Fast options / headers。
- Root model（包括 `gpt-5.5`、`gpt-5.5-fast` 及其 `-Sys` 派生）必须保留 `options.metadata.builtin_tools.web_search: true` 和 `options.store: false`。这属于本地 OpenCode carrier 语义，不得只保留 image variant 的 `image_generation`。
- Image variant 必须挂在 `models["gpt-5.5-Sys"].variants.image` 和 `models["gpt-5.5-fast-Sys"].variants.image` 上，并带 `metadata.builtin_tools.image_generation`；不要生成 `gpt-5.5-image-Sys` 这类前端基线不存在的独立 selector。继续保持 `agent.image.model: "sub2api-openai/gpt-5.5-fast-Sys"`。

OpenCode manifest 和 `opencode.json` 成功响应前必须 fail-closed 校验 required models：`gpt-5.5` 和 materialized `gpt-5.5-fast` 都存在。不要只校验 `gpt-5.5`；`gpt-5.5-fast` 来自 `experimental.modes.fast` materialization，缺失时固定的 `agent.image.model: "sub2api-openai/gpt-5.5-fast-Sys"` 会引用不存在模型。失败时返回 `503`、保留 no-store，并且不要输出 `apiKey` 或任何部分 `opencode.json`。

- [ ] **步骤 4：运行 OpenCode 测试验证通过**

运行：

```bash
(cd backend && C:/Users/34404/Documents/GitHub/workbench/toolchains/go/bin/go.exe test -tags unit ./internal/handler -run "TestConfigGuideOpenCode" -count=1)
```

预期：PASS。

---

## 任务 4：路由、Wire、embed bypass 与契约测试

**文件：**
- 修改：`repos/sub2api/backend/internal/handler/handler.go`
- 修改：`repos/sub2api/backend/internal/handler/wire.go`
- 修改：`repos/sub2api/backend/internal/server/routes/common.go`
- 修改：`repos/sub2api/backend/internal/web/embed_bypass.go`
- 修改：`repos/sub2api/backend/internal/web/embed_bypass_test.go`
- 修改：`repos/sub2api/backend/internal/server/api_contract_test.go`

- [ ] **步骤 1：编写 embed bypass 测试**

查看 `embed_bypass_test.go` 现有表格，添加：

```go
{path: "/config-guides/omp-openai/manifest.json", want: true},
{path: "/config-guides/opencode-openai/opencode.json", want: true},
```

- [ ] **步骤 2：运行 embed bypass 测试确认失败**

运行：

```bash
(cd backend && C:/Users/34404/Documents/GitHub/workbench/toolchains/go/bin/go.exe test ./internal/web -run TestShouldBypassEmbeddedFrontend -count=1)
```

预期：失败。

- [ ] **步骤 3：实现 embed bypass**

在 `embed_bypass.go` 中加入：

```go
strings.HasPrefix(trimmed, "/config-guides/") ||
```

- [ ] **步骤 4：接入 handler 组合和路由**

在 `handler.go` 的现有 `Handlers` 结构体字段列表中新增：

```go
ConfigGuide *ConfigGuideHandler
```

修改 `wire.go`：

- `ProvideHandlers` 参数增加 `configGuideHandler *ConfigGuideHandler`。
- 返回结构体赋值 `ConfigGuide: configGuideHandler`。
- `ProviderSet` 增加 `NewConfigGuideHandler`。
- 运行 `(cd backend && C:/Users/34404/Documents/GitHub/workbench/toolchains/go/bin/go.exe generate ./cmd/server)`，并确认 `backend/cmd/server/wire_gen.go` 中的 `handler.ProvideHandlers(...)` 调用传入了 `configGuideHandler`。

修改 `routes/common.go`：

```go
if h != nil && h.ConfigGuide != nil {
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
}
```

- [ ] **步骤 5：编写 contract 测试**

在 `api_contract_test.go` 中：
新增 contract 测试前，先在同一测试文件补足 fixture helper，避免引用未定义函数。建议不要依赖不存在的 `modelsDevContractPayload()`；直接内联返回最小 payload：

```go
func configGuideContractModelsPayload() map[string]any {
    return map[string]any{
        "openai": map[string]any{
            "models": map[string]any{
                "gpt-5.5": map[string]any{
                    "id": "gpt-5.5", "name": "GPT-5.5",
                    "reasoning": true, "attachment": true, "tool_call": true, "structured_output": true,
                    "modalities": map[string]any{"input": []any{"text", "image", "pdf"}, "output": []any{"text"}},
                    "cost": map[string]any{"input": 2.5, "output": 15.0, "cache_read": 0.25},
                    "limit": map[string]any{"context": 400000, "input": 272000, "output": 128000},
                    "experimental": map[string]any{"modes": map[string]any{"fast": map[string]any{"provider": map[string]any{"body": map[string]any{"service_tier": "priority"}, "headers": map[string]any{"x-test-header": "fast-mode"}}}}},
                },
                "gpt-5.4-mini": map[string]any{
                    "id": "gpt-5.4-mini", "name": "GPT-5.4 Mini",
                    "reasoning": true, "attachment": true, "tool_call": true, "structured_output": true,
                    "modalities": map[string]any{"input": []any{"text", "image", "pdf"}, "output": []any{"text"}},
                    "cost": map[string]any{"input": 0.25, "output": 2.0, "cache_read": 0.025},
                    "limit": map[string]any{"context": 400000, "input": 272000, "output": 128000},
                },
            },
        },
    }
}
```

然后新增测试：

1. `newContractDeps` 创建 `configGuideHandler := handler.NewConfigGuideHandler(openCodeMetadataService)`。
2. 构造 `handler.Handlers` 或直接注册 route 时包含 config guide routes。
3. 新增测试：

```go
func TestAPIContract_ConfigGuideOMPManifest(t *testing.T) {
    deps := newContractDeps(t)
    installModelsDevTransport(t, configGuideContractModelsPayload())

    status, body := doRequest(t, deps.router, http.MethodGet, "/config-guides/omp-openai/manifest.json?api_key=sk-test", "", nil)

    require.Equal(t, http.StatusOK, status)
    require.Contains(t, body, `"client":"omp"`)
    require.Contains(t, body, `"models.yml"`)
    require.Contains(t, body, `"api_key=sk-test"`)
    require.NotContains(t, body, "npm_token")
    require.NotContains(t, body, "NPM_TOKEN")
}
```

如果需要检查 header，新增 helper 返回 status/body/header，或在测试里直接构造 `httptest.ResponseRecorder`。

- [ ] **步骤 5.5：验证 Wire 生产 injector 编译**

运行：

```bash
(cd backend && C:/Users/34404/Documents/GitHub/workbench/toolchains/go/bin/go.exe generate ./cmd/server)
(cd backend && C:/Users/34404/Documents/GitHub/workbench/toolchains/go/bin/go.exe build ./cmd/server)
```

预期：`wire_gen.go` 已更新且 server 构建通过。

- [ ] **步骤 6：运行后端路由相关测试**

运行：

```bash
(cd backend && C:/Users/34404/Documents/GitHub/workbench/toolchains/go/bin/go.exe test -tags unit ./internal/handler ./internal/server ./internal/web -run "TestConfigGuide|TestAPIContract_ConfigGuide|TestShouldBypassEmbeddedFrontend" -count=1)
```

预期：PASS。

---

## 任务 5：前端 Agent 链接 helper、UI 与 Vitest

**文件：**
- 修改：`repos/sub2api/frontend/src/api/keys.ts`
- 修改：`repos/sub2api/frontend/src/components/keys/UseKeyModal.vue`
- 修改：`repos/sub2api/frontend/src/i18n/locales/zh.ts`
- 修改：`repos/sub2api/frontend/src/i18n/locales/en.ts`
- 修改：`repos/sub2api/frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`
  - 同一个前端子代理同时完成红灯测试、mock 更新与 UI 实现，避免并发编辑测试文件。

- [ ] **步骤 1：编写前端失败测试并更新 mock 形状**

先在 `UseKeyModal.spec.ts` 追加任务 6 中的测试示例。由于该测试文件已 mock `@/api/keys` 且当前只导出 `keysAPI`，必须同时更新 mock：要么用 `vi.importActual('@/api/keys')` 保留真实 `buildAgentConfigGuidePath`，要么在 mock 中提供等价 helper。预期红灯应是 UI 不存在，而不是 named export 缺失。

如果选择在 mock 中提供等价 helper，必须保留默认不传 `base_url` 的行为，例如：

```ts
buildAgentConfigGuidePath: (client: 'omp' | 'opencode', apiKey: string, baseUrl?: string) => {
  const clientPath = client === 'omp' ? 'omp-openai' : 'opencode-openai'
  const params = new URLSearchParams()
  params.set('api_key', apiKey)
  if (baseUrl?.trim()) params.set('base_url', baseUrl.trim())
  return `/config-guides/${clientPath}/manifest.json?${params.toString()}`
}
```

同时更新 `vue-i18n` mock，避免 `t('keys.useKeyModal.agentConfig.instruction', { url })` 的 `{ url }` 插值在测试中被丢弃。建议改为：

```ts
vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, string>) => params?.url ? `${key} ${params.url}` : key
  })
}))
```

预期红灯应是 UI 不存在，而不是 named export 缺失或 i18n mock 丢失 URL。

- [ ] **步骤 2：实现链接 path helper**

在 `keys.ts` 添加：

```ts
export type AgentConfigGuideClient = 'omp' | 'opencode'

export function buildAgentConfigGuidePath(client: AgentConfigGuideClient, apiKey: string, baseUrl?: string): string {
  const clientPath = client === 'omp' ? 'omp-openai' : 'opencode-openai'
  const params = new URLSearchParams()
  params.set('api_key', apiKey)
  const trimmedBaseURL = baseUrl?.trim()
  if (trimmedBaseURL) params.set('base_url', trimmedBaseURL)
  return `/config-guides/${clientPath}/manifest.json?${params.toString()}`
}
```

暂时不要默认传 `base_url`。

- [ ] **步骤 3：实现 UseKeyModal 指令 UI**

导入 helper：

```ts
import { keysAPI, buildAgentConfigGuidePath, type OpenCodeOpenAIModel, type OpenCodeOpenAIModelsResponse } from '@/api/keys'
```

新增状态：

```ts
const copiedAgentConfig = ref(false)
```

新增 computed：

```ts
const agentConfigGuideClient = computed<'omp' | 'opencode' | null>(() => {
  if (props.platform !== 'openai') return null
  if (activeClientTab.value === 'omp') return 'omp'
  if (activeClientTab.value === 'opencode') return 'opencode'
  return null
})

const requiredOMPAgentConfigModelIds = ['gpt-5.5', 'gpt-5.4-mini']
const requiredOpenCodeAgentConfigModelIds = ['gpt-5.5', 'gpt-5.5-fast']

const showAgentConfigGuide = computed(() => {
  if (!agentConfigGuideClient.value) return false
  if (openCodeLoading.value || openCodeError.value || !openCodeModels.value) return false
  if (agentConfigGuideClient.value === 'omp') {
    const plugin = openCodeMetadata.value?.omp_openai_provider_tools
    const hasRequiredModels = requiredOMPAgentConfigModelIds.every((id) => Boolean(openCodeModels.value?.[id]))
    return Boolean(hasRequiredModels && plugin && ['ok', 'cached'].includes(plugin.status) && plugin.latest_version?.trim())
  }
  return requiredOpenCodeAgentConfigModelIds.every((id) => Boolean(openCodeModels.value?.[id]))
})

function getAgentConfigGuideOrigin(): string {
  const trimmed = props.baseUrl.trim().replace(/\/+$/, '')
  if (/^https?:\/\//i.test(trimmed)) {
    return trimmed.endsWith('/v1') ? trimmed.slice(0, -3) : trimmed
  }
  if (typeof window !== 'undefined' && window.location?.origin) return window.location.origin
  return ''
}

const agentConfigGuideURL = computed(() => {
  if (!showAgentConfigGuide.value || !agentConfigGuideClient.value) return ''
  const path = buildAgentConfigGuidePath(agentConfigGuideClient.value, props.apiKey)
  const origin = getAgentConfigGuideOrigin()
  return origin ? `${origin}${path}` : path
})

const agentConfigGuideInstruction = computed(() => {
  if (!agentConfigGuideURL.value) return ''
  return t('keys.useKeyModal.agentConfig.instruction', { url: agentConfigGuideURL.value })
})
```

模板中在 code blocks 前增加：

```vue
<div v-if="showAgentConfigGuide" data-testid="agent-config-guide" class="p-3 rounded-lg border border-emerald-200 dark:border-emerald-800 bg-emerald-50 dark:bg-emerald-900/20">
  <p class="text-xs text-emerald-700 dark:text-emerald-300 mb-2">
    {{ t('keys.useKeyModal.agentConfig.hint') }}
  </p>
  <div class="flex items-start gap-2">
    <code class="flex-1 text-xs break-all text-emerald-900 dark:text-emerald-100">{{ agentConfigGuideInstruction }}</code>
    <button @click="copyAgentConfigGuide" class="px-2.5 py-1 text-xs font-medium rounded-lg bg-emerald-600 text-white hover:bg-emerald-700">
      {{ copiedAgentConfig ? t('keys.useKeyModal.agentConfig.copied') : t('keys.useKeyModal.agentConfig.copy') }}
    </button>
  </div>
</div>
```

复用项目现有按钮样式，避免引入新组件。

新增复制方法：

```ts
const copyAgentConfigGuide = async () => {
  if (!agentConfigGuideInstruction.value) return
  const success = await clipboardCopy(agentConfigGuideInstruction.value, t('keys.copied'))
  if (success) {
    copiedAgentConfig.value = true
    setTimeout(() => { copiedAgentConfig.value = false }, 2000)
  }
}
```

- [ ] **步骤 4：新增 i18n 文案**

中文：

```ts
agentConfig: {
  hint: '给 Agent 使用的短配置入口。链接包含当前 API Key，请只发给可信 Agent。',
  instruction: '请按此链接完成配置：{url}',
  copy: '复制 Agent 链接',
  copied: '已复制'
}
```

英文：

```ts
agentConfig: {
  hint: 'Short configuration entry for agents. The link contains this API key; only share it with a trusted agent.',
  instruction: 'Use this link to complete the configuration: {url}',
  copy: 'Copy agent link',
  copied: 'Copied'
}
```

- [ ] **步骤 5：运行前端类型检查**

运行：

```bash
(cd frontend && pnpm typecheck)
```

预期：如果只有本任务改动，类型检查通过。若失败，按错误修复。

---

## 任务 6：前端 Vitest 覆盖 Agent 链接（由任务 5 同一前端子代理执行）

**文件：**
- 修改：`repos/sub2api/frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`

- [ ] **步骤 1：追加 OMP Agent 链接测试**

新增测试：

```ts
it('shows short OMP agent config guide link without base_url by default', async () => {
  const wrapper = mountOpenAIUseKeyModal()

  await clickClientTab(wrapper, 'keys.useKeyModal.cliTabs.omp')

  const text = wrapper.text()
  expect(text).toContain('keys.useKeyModal.agentConfig.hint')
  expect(text).toContain('keys.useKeyModal.agentConfig.instruction')
  expect(text).toContain('/config-guides/omp-openai/manifest.json?api_key=sk-test')
  expect(text).not.toContain('base_url=')

  const agentBlock = wrapper.find('[data-testid="agent-config-guide"]')
  expect(agentBlock.exists()).toBe(true)
  expect(agentBlock.text()).not.toContain('providers:')
  expect(getCodeBlocks(wrapper)).toHaveLength(3)
})
```

测试实现时给 Agent 指令块添加可定位 wrapper（例如 `data-testid="agent-config-guide"`），并只对该 wrapper 的文本断言不包含 `providers:`。不要从 manifest URL 截取到整个 modal 末尾，因为后续保留的 OMP code blocks 必然包含 `providers:`。

```ts

it('uses api base origin instead of window origin for the agent link', async () => {
  const wrapper = mountOpenAIUseKeyModal({ baseUrl: 'https://example.com/v1' })
  await clickClientTab(wrapper, 'keys.useKeyModal.cliTabs.omp')
  expect(wrapper.text()).toContain('https://example.com/config-guides/omp-openai/manifest.json?api_key=sk-test')
})

it('falls back to the page origin when api base url is relative', async () => {
  const wrapper = mountOpenAIUseKeyModal({ baseUrl: '/api/v1' })
  await clickClientTab(wrapper, 'keys.useKeyModal.cliTabs.omp')
  expect(wrapper.text()).toContain('/config-guides/omp-openai/manifest.json?api_key=sk-test')
  expect(wrapper.text()).not.toContain('/api/config-guides/')
})

it('hides OMP agent config guide link when required model metadata is missing', async () => {
  getOpenCodeModelsMock().mockResolvedValueOnce({
    models: { 'gpt-5.5': openaiModelsMock['gpt-5.5'] },
    omp_openai_provider_tools: { package: 'omp-openai-provider-tools', latest_version: '0.1.2', status: 'ok' }
  })
  const wrapper = mountOpenAIUseKeyModal()
  await clickClientTab(wrapper, 'keys.useKeyModal.cliTabs.omp')
  expect(wrapper.text()).not.toContain('/config-guides/omp-openai/manifest.json')
})

it('hides OpenCode agent config guide link when required model metadata is incomplete', async () => {
  getOpenCodeModelsMock().mockResolvedValueOnce({
    models: { 'gpt-5.5': openaiModelsMock['gpt-5.5'] },
    omp_openai_provider_tools: { package: 'omp-openai-provider-tools', latest_version: '0.1.2', status: 'ok' }
  })
  const wrapper = mountOpenAIUseKeyModal()
  await clickClientTab(wrapper, 'keys.useKeyModal.cliTabs.opencode')
  expect(wrapper.text()).not.toContain('/config-guides/opencode-openai/manifest.json')
})

```
上述 `mountOpenAIUseKeyModal({ baseUrl: ... })` 示例要求先把现有测试 helper 从无参数函数改成接收 props override，例如 `const mountOpenAIUseKeyModal = (props: Partial<Props> = {}) => mount(UseKeyModal, { props: { ...defaults, ...props } })`。

如果测试 i18n mock 会直接显示 key，不要强行期待中文文案；沿用现有测试风格。

- [ ] **步骤 2：追加 OpenCode Agent 链接测试**

```ts
it('shows short OpenCode agent config guide link when OpenCode metadata is loaded', async () => {
  const wrapper = mountOpenAIUseKeyModal()

  await clickClientTab(wrapper, 'keys.useKeyModal.cliTabs.opencode')

  expect(wrapper.text()).toContain('/config-guides/opencode-openai/manifest.json?api_key=sk-test')
  expect(wrapper.text()).not.toContain('base_url=')
  expect(getCodeBlocks(wrapper).join('\n')).toContain('"sub2api-openai"')
})
```

- [ ] **步骤 3：追加失败隐藏测试**

```ts
it('hides OMP agent config guide link when OMP metadata loading fails', async () => {
  getOpenCodeModelsMock().mockRejectedValueOnce(new Error('network down'))
  const wrapper = mountOpenAIUseKeyModal()

  await clickClientTab(wrapper, 'keys.useKeyModal.cliTabs.omp')

  expect(wrapper.text()).not.toContain('/config-guides/omp-openai/manifest.json')
  expect(getCodeBlocks(wrapper).join('\n')).not.toContain('providers:')
})
```

- [ ] **步骤 4：运行聚焦 Vitest**

运行：

```bash
(cd frontend && pnpm exec vitest run src/components/keys/__tests__/UseKeyModal.spec.ts --pool=forks --poolOptions.forks.singleFork --reporter=verbose)
```

预期：全部通过。stderr 中若出现测试故意 mock 的 `network down`，只要 Vitest pass 即可。

---

## 任务 7：最终验证与提交

**文件：**
- 所有本计划涉及文件。

- [ ] **步骤 1：重新生成 Wire 并运行后端聚焦测试**

运行：

```bash
(cd backend && C:/Users/34404/Documents/GitHub/workbench/toolchains/go/bin/go.exe generate ./cmd/server)
(cd backend && C:/Users/34404/Documents/GitHub/workbench/toolchains/go/bin/go.exe test -tags unit ./internal/handler ./internal/server ./internal/web -run "TestConfigGuide|TestAPIContract_(ConfigGuide|OpenCodeOpenAIModels)|TestShouldBypassEmbeddedFrontend" -count=1)
(cd backend && C:/Users/34404/Documents/GitHub/workbench/toolchains/go/bin/go.exe build ./cmd/server)
```

预期：Wire 生成成功、后端聚焦测试 PASS、server 构建 PASS。

- [ ] **步骤 2：运行既有 OpenCode metadata 测试**

运行：

```bash
(cd backend && C:/Users/34404/Documents/GitHub/workbench/toolchains/go/bin/go.exe test -tags unit ./internal/service ./internal/server -run "Test.*OpenCode.*|TestAPIContract_OpenCodeOpenAIModels" -count=1)
```

预期：PASS。

- [ ] **步骤 3：运行前端聚焦测试**

运行：

```bash
(cd frontend && pnpm exec vitest run src/components/keys/__tests__/UseKeyModal.spec.ts --pool=forks --poolOptions.forks.singleFork --reporter=verbose)
```

预期：PASS。

- [ ] **步骤 4：运行前端类型检查**

运行：

```bash
(cd frontend && pnpm typecheck)
```

预期：PASS。

- [ ] **步骤 5：运行差异空白检查**

运行：

```bash
git diff --check
```

预期：无输出，exit 0。

- [ ] **步骤 6：检查工作树和敏感信息**

运行：

```bash
git status --short --branch
```

再用专用 search 工具检查本次文件中不应出现真实 key。不得用 shell grep。

需要确认：

- 只出现测试 key `sk-test` 或 placeholder。
- 没有真实本机 API key。
- 没有 `NPM_TOKEN` / `npm_token` 泄漏。

- [ ] **步骤 7：提交**

使用 Conventional Commits 中文 subject：

```bash
git add backend/internal/handler/config_guide_handler.go backend/internal/handler/config_guide_handler_test.go backend/internal/handler/handler.go backend/internal/handler/wire.go backend/cmd/server/wire_gen.go backend/internal/server/routes/common.go backend/internal/web/embed_bypass.go backend/internal/web/embed_bypass_test.go backend/internal/server/api_contract_test.go frontend/src/api/keys.ts frontend/src/components/keys/UseKeyModal.vue frontend/src/components/keys/__tests__/UseKeyModal.spec.ts frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts backend/docs/superpowers/specs/2026-05-09-agent-config-guide-links-design.md backend/docs/superpowers/plans/2026-05-09-agent-config-guide-links.md
git commit -m "feat(config-guides): 新增 Agent 配置链接"
```

预期：commit 成功。

## 最终验收清单

- [ ] OMP tab 显示短 Agent 配置链接，且仍显示 3 个原配置块。
- [ ] OpenCode tab 显示短 Agent 配置链接，且仍显示 `opencode.json` 原配置块。
- [ ] OMP manifest 返回 4 个下载项。
- [ ] OpenCode manifest 返回 1 个下载项。
- [ ] 默认 Agent 链接不带 `base_url`。
- [ ] 后端默认按 `Request.Host` 推导当前实例 `/v1`，不信任伪造 `X-Forwarded-Host`。
- [ ] 所有带 `api_key` 的 config guide 响应都有 `Cache-Control: no-store`。
- [ ] 错误响应不回显 API key。
- [ ] 插件版本不可用时不输出半截安装命令。
- [ ] OpenCode JSON 保留 `sub2api-openai`、Fast、Image variant、`metadata.builtin_tools`。
- [ ] Wire 生成文件已更新，`go build ./cmd/server` 通过。
- [ ] OMP YAML 保留 `compat.openaiProviderTools`、image provider 与完整 provider selector。
- [ ] 后端聚焦测试通过。
- [ ] 前端 Vitest 通过。
- [ ] `pnpm typecheck` 通过。
- [ ] `git diff --check` 通过。
