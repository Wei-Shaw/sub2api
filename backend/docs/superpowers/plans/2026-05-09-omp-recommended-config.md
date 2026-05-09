# OMP 推荐配置实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 在 Use Key modal 的 OpenAI 平台下新增 OMP tab，生成插件安装/图像子代理说明、`~/.omp/agent/models.yml`、`~/.omp/agent/config.yml`，并从后端自动获取 `omp-openai-provider-tools` npm latest 版本。

**架构：** 复用现有 `/api/v1/keys/opencode/openai-models` 链路，后端 `OpenCodeMetadataService` 继续负责 models.dev OpenAI metadata，同时新增 npm latest 版本元数据的独立缓存与 stale fallback；前端在 OpenAI + OMP tab 复用同一 metadata loader。前端 OMP 生成器与 OpenCode JSON 生成器分离：OpenCode 保留 `sub2api-openai`、`-Sys`、Fast、image variant 与 builtin tools carrier；OMP 使用独立 `sub2api-openai` / `sub2api-openai-image` provider、`compat.openaiProviderTools`、完整 provider selector 和动态插件版本。

**技术栈：** Go 1.26、Gin、`net/http`、Vitest、Vue 3、TypeScript、vue-i18n。

---

## 规格与边界

规格文件：`repos/sub2api/backend/docs/superpowers/specs/2026-05-09-omp-recommended-config-design.md`

项目规则：`repos/sub2api/AGENTS.md`

硬性边界：

- 不使用 worktree，直接在当前 `repos/sub2api` 主工作树开发。
- 不新增独立 OMP endpoint；扩展现有 `GET /api/v1/keys/opencode/openai-models` response。
- 不破坏现有 OpenCode 推荐配置链。
- 不把 OpenCode JSON shape 复用为 OMP 配置。
- 不把 provider 改成 `provider.openai`。
- 不硬编码生产插件版本 `0.1.2`；`0.1.2` 只可作为测试 fixture。
- 插件版本不可用时不得输出半截 `omp plugin install npm:omp-openai-provider-tools@`。
- 不泄露真实 API key；代码和测试使用 `sk-test`、`<USER_API_KEY>`。

## 文件结构

### 后端

- 修改：`repos/sub2api/backend/internal/service/opencode_openai_metadata.go`
  - 新增 `OMPProviderToolsMetadata` 类型。
  - 新增 npm latest URL、包名、TTL 常量。
  - 在 `OpenCodeMetadataService` 中加入 plugin latest cache 字段。
  - 新增 `GetOMPProviderToolsMetadata(ctx)`。
  - 保持 `GetOpenAIModels(ctx)` 行为向后兼容。
- 修改：`repos/sub2api/backend/internal/service/opencode_openai_metadata_test.go`
  - 测试 latest 获取、版本变化、stale cached fallback、unavailable 失败路径。
- 修改：`repos/sub2api/backend/internal/handler/api_key_handler.go`
  - `GetOpenCodeOpenAIModels` response 从 `{models}` 扩展为 `{models, omp_openai_provider_tools}`。
  - models 获取失败仍按现有行为返回错误；plugin latest 失败不应让 models endpoint 整体失败。
- 修改：`repos/sub2api/backend/internal/server/api_contract_test.go`
  - `installModelsDevTransport` 扩展为同时 stub models.dev 与 npm latest。
  - contract 断言 response 包含 `omp_openai_provider_tools.latest_version`，不包含凭据。

### 前端

- 修改：`repos/sub2api/frontend/src/api/keys.ts`
  - 扩展 `OpenCodeOpenAIModelsResponse`：新增 optional `omp_openai_provider_tools`。
- 修改：`repos/sub2api/frontend/src/components/keys/UseKeyModal.vue`
  - OpenAI `clientTabs` 新增 OMP。
  - `showShellTabs` 与 `showPlatformNote` 排除 `omp`。
  - `platformDescription` 在 OMP tab 返回 OMP description。
  - loader 在 OpenAI + `opencode` / `omp` 时触发。
  - 保存完整 metadata response，而不只保存 models。
  - 新增 OMP YAML/命令生成 helpers。
- 修改：`repos/sub2api/frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`
  - 扩展 mock response。
  - 新增 OMP tab、动态版本、失败路径、输出顺序、YAML 语义与 UI 文案测试。
- 修改：`repos/sub2api/frontend/src/i18n/locales/zh.ts`
  - 新增 OMP 文案。
- 修改：`repos/sub2api/frontend/src/i18n/locales/en.ts`
  - 新增 OMP 文案。

---

### 任务 1：后端插件版本元数据服务

**文件：**
- 修改：`repos/sub2api/backend/internal/service/opencode_openai_metadata.go`
- 测试：`repos/sub2api/backend/internal/service/opencode_openai_metadata_test.go`

- [ ] **步骤 1：编写 latest 获取成功和版本变化测试**

在 `repos/sub2api/backend/internal/service/opencode_openai_metadata_test.go` 追加测试。测试不要依赖真实 npm registry，使用 `httptest.NewServer`。

建议测试代码：

```go
func TestOpenCodeMetadataServiceGetOMPProviderToolsMetadata_FetchesLatestVersion(t *testing.T) {
	requests := 0
	npm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		require.Equal(t, "/omp-openai-provider-tools/latest", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{"version": "0.1.2"})
	}))
	defer npm.Close()

	svc := &OpenCodeMetadataService{
		client:       npm.Client(),
		npmLatestURL: npm.URL + "/omp-openai-provider-tools/latest",
		npmTTL:       time.Minute,
	}

	metadata := svc.GetOMPProviderToolsMetadata(context.Background())

	require.Equal(t, "omp-openai-provider-tools", metadata.Package)
	require.Equal(t, "0.1.2", metadata.LatestVersion)
	require.Equal(t, "ok", metadata.Status)
	require.Empty(t, metadata.Error)
	require.Equal(t, 1, requests)
}

func TestOpenCodeMetadataServiceGetOMPProviderToolsMetadata_FollowsVersionChangesAfterTTL(t *testing.T) {
	versions := []string{"0.1.2", "9.9.9"}
	requests := 0
	npm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		version := versions[requests]
		requests++
		_ = json.NewEncoder(w).Encode(map[string]any{"version": version})
	}))
	defer npm.Close()

	svc := &OpenCodeMetadataService{
		client:       npm.Client(),
		npmLatestURL: npm.URL,
		npmTTL:       time.Nanosecond,
	}

	first := svc.GetOMPProviderToolsMetadata(context.Background())
	time.Sleep(time.Millisecond)
	second := svc.GetOMPProviderToolsMetadata(context.Background())

	require.Equal(t, "0.1.2", first.LatestVersion)
	require.Equal(t, "9.9.9", second.LatestVersion)
	require.Equal(t, 2, requests)
}
```

- [ ] **步骤 2：编写 stale fallback 和 unavailable 测试**

继续追加：

```go
func TestOpenCodeMetadataServiceGetOMPProviderToolsMetadata_UsesStaleCacheOnFailure(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"version": "0.1.2"})
	}))
	defer ok.Close()

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer bad.Close()

	svc := &OpenCodeMetadataService{
		client:       ok.Client(),
		npmLatestURL: ok.URL,
		npmTTL:       time.Nanosecond,
	}

	first := svc.GetOMPProviderToolsMetadata(context.Background())
	require.Equal(t, "ok", first.Status)
	require.Equal(t, "0.1.2", first.LatestVersion)

	time.Sleep(time.Millisecond)
	svc.client = bad.Client()
	svc.npmLatestURL = bad.URL
	second := svc.GetOMPProviderToolsMetadata(context.Background())

	require.Equal(t, "cached", second.Status)
	require.Equal(t, "0.1.2", second.LatestVersion)
	require.NotEmpty(t, second.Error)
}

func TestOpenCodeMetadataServiceGetOMPProviderToolsMetadata_UnavailableWithoutCache(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer bad.Close()

	svc := &OpenCodeMetadataService{
		client:       bad.Client(),
		npmLatestURL: bad.URL,
		npmTTL:       time.Minute,
	}

	metadata := svc.GetOMPProviderToolsMetadata(context.Background())

	require.Equal(t, "omp-openai-provider-tools", metadata.Package)
	require.Empty(t, metadata.LatestVersion)
	require.Equal(t, "unavailable", metadata.Status)
	require.NotEmpty(t, metadata.Error)
}
```

- [ ] **步骤 3：运行后端 service 测试验证失败**

运行：

```bash
go test -tags unit ./internal/service -run 'TestOpenCodeMetadataServiceGetOMPProviderToolsMetadata' -count=1
```

工作目录：`repos/sub2api/backend`

预期：FAIL，错误包含 `GetOMPProviderToolsMetadata undefined` 或结构字段不存在。

- [ ] **步骤 4：实现类型、字段和 constructor**

在 `repos/sub2api/backend/internal/service/opencode_openai_metadata.go` 中：

```go
const (
	openCodeModelsDevURL = "https://models.dev/api.json"
	openCodeModelsTTL    = 15 * time.Minute

	ompProviderToolsPackage      = "omp-openai-provider-tools"
	ompProviderToolsNpmLatestURL = "https://registry.npmjs.org/omp-openai-provider-tools/latest"
	ompProviderToolsTTL          = 15 * time.Minute
)

type OMPProviderToolsMetadata struct {
	Package       string `json:"package"`
	LatestVersion string `json:"latest_version"`
	Status        string `json:"status"`
	Error         string `json:"error,omitempty"`
}

type OpenCodeMetadataService struct {
	client *http.Client
	url    string
	ttl    time.Duration

	npmLatestURL string
	npmTTL       time.Duration

	mu    sync.RWMutex
	cache map[string]OpenCodeOpenAIModel
	exp   time.Time

	npmCache OMPProviderToolsMetadata
	npmExp   time.Time
}
```

并在 `NewOpenCodeMetadataService()` 中初始化：

```go
return &OpenCodeMetadataService{
	client:       &http.Client{Timeout: 15 * time.Second},
	url:          openCodeModelsDevURL,
	ttl:          openCodeModelsTTL,
	npmLatestURL: ompProviderToolsNpmLatestURL,
	npmTTL:       ompProviderToolsTTL,
}
```

- [ ] **步骤 5：实现 `GetOMPProviderToolsMetadata`**

在 `GetOpenAIModels` 后新增：

```go
func (s *OpenCodeMetadataService) GetOMPProviderToolsMetadata(ctx context.Context) OMPProviderToolsMetadata {
	metadata := OMPProviderToolsMetadata{Package: ompProviderToolsPackage}
	if s == nil {
		metadata.Status = "unavailable"
		metadata.Error = "opencode metadata service unavailable"
		return metadata
	}

	now := time.Now()
	s.mu.RLock()
	if s.npmCache.LatestVersion != "" && now.Before(s.npmExp) {
		cached := s.npmCache
		s.mu.RUnlock()
		return cached
	}
	stale := s.npmCache
	s.mu.RUnlock()

	latest, err := s.fetchOMPProviderToolsLatest(ctx)
	if err != nil {
		if stale.LatestVersion != "" {
			stale.Status = "cached"
			stale.Error = err.Error()
			return stale
		}
		metadata.Status = "unavailable"
		metadata.Error = err.Error()
		return metadata
	}

	metadata.LatestVersion = latest
	metadata.Status = "ok"

	s.mu.Lock()
	s.npmCache = metadata
	s.npmExp = now.Add(s.npmTTL)
	s.mu.Unlock()

	return metadata
}
```

新增 helper：

```go
func (s *OpenCodeMetadataService) fetchOMPProviderToolsLatest(ctx context.Context) (string, error) {
	url := s.npmLatestURL
	if url == "" {
		url = ompProviderToolsNpmLatestURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("npm latest status: %d", resp.StatusCode)
	}

	var payload struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}

	version := strings.TrimSpace(payload.Version)
	if version == "" {
		return "", fmt.Errorf("npm latest version missing")
	}
	return version, nil
}
```

- [ ] **步骤 6：运行后端 service 测试验证通过**

运行：

```bash
go test -tags unit ./internal/service -run 'TestOpenCodeMetadataServiceGetOMPProviderToolsMetadata' -count=1
```

预期：PASS。

---

### 任务 2：后端 handler 与契约 response

**文件：**
- 修改：`repos/sub2api/backend/internal/handler/api_key_handler.go`
- 修改：`repos/sub2api/backend/internal/server/api_contract_test.go`
- 测试：`repos/sub2api/backend/internal/server/api_contract_test.go`

- [ ] **步骤 1：扩展 contract 测试的 transport stub**

修改 `installModelsDevTransport`，让它同时拦截 npm latest URL。建议签名改为：

```go
func installModelsDevTransport(t *testing.T, payload map[string]any) {
	installModelsDevTransportWithNpmLatest(t, payload, "0.1.2")
}

func installModelsDevTransportWithNpmLatest(t *testing.T, payload map[string]any, npmLatestVersion string) {
	// body := json.Marshal(payload)
	// npmBody := json.Marshal(map[string]any{"version": npmLatestVersion})
	// http.DefaultTransport = roundTripFunc(func(req *http.Request) ...)
}
```

在 transport 中：

```go
switch req.URL.String() {
case "https://models.dev/api.json":
	return modelsDevResponse, nil
case "https://registry.npmjs.org/omp-openai-provider-tools/latest":
	return npmLatestResponse, nil
}
return old.RoundTrip(req)
```

- [ ] **步骤 2：扩展 `TestAPIContract_OpenCodeOpenAIModels` 断言**

在现有 `var resp struct` 的 `Data` 中加入：

```go
OMPProviderTools struct {
	Package       string `json:"package"`
	LatestVersion string `json:"latest_version"`
	Status        string `json:"status"`
	Error         string `json:"error,omitempty"`
} `json:"omp_openai_provider_tools"`
```

在断言中加入：

```go
require.Equal(t, "omp-openai-provider-tools", resp.Data.OMPProviderTools.Package)
require.Equal(t, "0.1.2", resp.Data.OMPProviderTools.LatestVersion)
require.Equal(t, "ok", resp.Data.OMPProviderTools.Status)
require.Empty(t, resp.Data.OMPProviderTools.Error)
require.NotContains(t, body, "npm_token")
require.NotContains(t, body, "NPM_TOKEN")
```

- [ ] **步骤 3：运行契约测试验证失败**

运行：

```bash
go test -tags unit ./internal/server -run TestAPIContract_OpenCodeOpenAIModels -count=1
```

预期：FAIL，因为 handler 还没有返回 `omp_openai_provider_tools`。

- [ ] **步骤 4：更新 handler response**

修改 `repos/sub2api/backend/internal/handler/api_key_handler.go`：

```go
models, err := h.openCodeMetadataService.GetOpenAIModels(c.Request.Context())
if err != nil {
	response.ErrorFrom(c, err)
	return
}

providerTools := h.openCodeMetadataService.GetOMPProviderToolsMetadata(c.Request.Context())
response.Success(c, gin.H{
	"models":                    models,
	"omp_openai_provider_tools": providerTools,
})
```

- [ ] **步骤 5：运行后端 contract 测试验证通过**

运行：

```bash
go test -tags unit ./internal/server -run TestAPIContract_OpenCodeOpenAIModels -count=1
```

预期：PASS。

- [ ] **步骤 6：运行后端聚焦测试全集**

运行：

```bash
go test -tags unit ./internal/service ./internal/server -run 'Test.*OpenCode.*|TestAPIContract_OpenCodeOpenAIModels' -count=1
```

预期：PASS。

---

### 任务 3：前端 API 类型与 OMP UI 状态测试

**文件：**
- 修改：`repos/sub2api/frontend/src/api/keys.ts`
- 测试：`repos/sub2api/frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`

- [ ] **步骤 1：扩展 API 类型**

在 `repos/sub2api/frontend/src/api/keys.ts` 修改 response 类型：

```ts
export interface OMPProviderToolsMetadata {
  package: string
  latest_version: string
  status: 'ok' | 'cached' | 'unavailable'
  error?: string
}

export type OpenCodeOpenAIModelsResponse = {
  models: Record<string, OpenCodeOpenAIModel>
  omp_openai_provider_tools?: OMPProviderToolsMetadata
}
```

- [ ] **步骤 2：扩展测试 mock response**

在 `UseKeyModal.spec.ts` 的 `vi.mock('@/api/keys', ...)` 中把 mock response 改为：

```ts
getOpenCodeOpenAIModels: vi.fn().mockResolvedValue({
  models: openaiModelsMock,
  omp_openai_provider_tools: {
    package: 'omp-openai-provider-tools',
    latest_version: '0.1.2',
    status: 'ok'
  }
})
```

- [ ] **步骤 3：增加 OMP UI 说明与加载触发测试**

在测试文件中导入 mocked API：

```ts
import { keysAPI } from '@/api/keys'
```

新增 helper：

```ts
const mountOpenAIUseKeyModal = () => mount(UseKeyModal, {
  props: {
    show: true,
    apiKey: 'sk-test',
    baseUrl: 'https://example.com/v1',
    platform: 'openai'
  },
  global: {
    stubs: {
      BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
      Icon: { template: '<span />' }
    }
  }
})

const clickClientTab = async (wrapper: ReturnType<typeof mount>, label: string) => {
  const tab = wrapper.findAll('button').find((button) => button.text().includes(label))
  expect(tab).toBeDefined()
  await tab!.trigger('click')
  await nextTick()
  await Promise.resolve()
  await nextTick()
}
```

如果 TypeScript 对 `ReturnType<typeof mount>` 不满意，可使用已有内联写法，不引入 helper。

新增测试：

```ts
it('shows OMP tab with OMP-specific description and loads OpenAI metadata', async () => {
  const getModels = vi.mocked(keysAPI.getOpenCodeOpenAIModels)
  getModels.mockClear()
  const wrapper = mountOpenAIUseKeyModal()

  await clickClientTab(wrapper, 'keys.useKeyModal.cliTabs.omp')

  expect(getModels).toHaveBeenCalledTimes(1)
  expect(wrapper.text()).toContain('keys.useKeyModal.omp.description')
  expect(wrapper.text()).not.toContain('keys.useKeyModal.openai.note')
  expect(wrapper.text()).not.toContain('.codex')
})
```

- [ ] **步骤 4：运行前端测试验证失败**

运行：

```bash
pnpm exec vitest run src/components/keys/__tests__/UseKeyModal.spec.ts --pool=forks --poolOptions.forks.singleFork --reporter=verbose
```

工作目录：`repos/sub2api/frontend`

预期：FAIL，因为 OMP tab 和 OMP description 尚未实现。

---

### 任务 4：前端 OMP tab 基础行为与 i18n

**文件：**
- 修改：`repos/sub2api/frontend/src/components/keys/UseKeyModal.vue`
- 修改：`repos/sub2api/frontend/src/i18n/locales/zh.ts`
- 修改：`repos/sub2api/frontend/src/i18n/locales/en.ts`
- 测试：`repos/sub2api/frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`

- [ ] **步骤 1：新增 i18n 文案**

在 `zh.ts` 的 `keys.useKeyModal.cliTabs` 中加入：

```ts
omp: 'OMP'
```

在 `keys.useKeyModal` 下加入：

```ts
omp: {
  description: '为 OMP 手动复制推荐配置。请先执行第一块插件命令；系统不会自动安装插件，也不会自动写入 ~/.omp/agent/models.yml 或 ~/.omp/agent/config.yml。',
  pluginHint: '先安装或升级 omp-openai-provider-tools，并按需生成 image_generator 子代理；插件不会读取或保存 API Key。',
  modelsHint: '复制到 ~/.omp/agent/models.yml。生图和 provider-native web_search 依赖第一块插件命令。',
  configHint: '复制到 ~/.omp/agent/config.yml。所有模型角色使用完整 provider/model 名称。',
  loadingHint: '正在加载 OpenAI 模型与 OMP 插件版本元数据...',
  metadataErrorHint: '无法加载 OpenAI 模型元数据，暂不能生成可用的 OMP 配置。',
  pluginVersionErrorHint: '无法获取 omp-openai-provider-tools 最新版本，暂不能生成插件安装命令。'
}
```

在 `en.ts` 对应加入：

```ts
omp: 'OMP'
```

以及：

```ts
omp: {
  description: 'Manually copy the recommended OMP configuration. Run the first plugin command block before using the config; this page does not install plugins or write ~/.omp/agent/models.yml / ~/.omp/agent/config.yml automatically.',
  pluginHint: 'Install or upgrade omp-openai-provider-tools first, then generate the image_generator subagent if needed. The plugin does not read or store API keys.',
  modelsHint: 'Copy to ~/.omp/agent/models.yml. Image generation and provider-native web_search depend on the first plugin command block.',
  configHint: 'Copy to ~/.omp/agent/config.yml. All model roles use full provider/model selectors.',
  loadingHint: 'Loading OpenAI model metadata and OMP plugin version metadata...',
  metadataErrorHint: 'Unable to load OpenAI model metadata, so usable OMP config cannot be generated yet.',
  pluginVersionErrorHint: 'Unable to fetch the latest omp-openai-provider-tools version, so the plugin install command cannot be generated yet.'
}
```

- [ ] **步骤 2：在组件中保存完整 metadata response**

修改 import：

```ts
import { keysAPI, type OpenCodeOpenAIModel, type OpenCodeOpenAIModelsResponse } from '@/api/keys'
```

新增 ref：

```ts
const openCodeMetadata = ref<OpenCodeOpenAIModelsResponse | null>(null)
```

在 `loadOpenCodeModels` 中：

```ts
const resp = await keysAPI.getOpenCodeOpenAIModels()
openCodeMetadata.value = resp
openCodeModels.value = resp.models
```

在 catch 中保持现有 error。

- [ ] **步骤 3：加入 OMP tab 与说明行为**

在 OpenAI tabs 中追加：

```ts
tabs.push({ id: 'omp', label: t('keys.useKeyModal.cliTabs.omp'), icon: TerminalIcon })
```

修改：

```ts
const showShellTabs = computed(() => !['opencode', 'omp'].includes(activeClientTab.value))
```

在 `platformDescription` 的 OpenAI case 中：

```ts
if (activeClientTab.value === 'omp') {
  return t('keys.useKeyModal.omp.description')
}
```

修改：

```ts
const showPlatformNote = computed(() => !['opencode', 'omp'].includes(activeClientTab.value))
```

- [ ] **步骤 4：让 OMP tab 触发 metadata loader**

修改 watcher 条件：

```ts
if (!['opencode', 'omp'].includes(client)) return
void loadOpenCodeModels()
```

- [ ] **步骤 5：运行前端 UI 状态测试验证通过**

运行：

```bash
pnpm exec vitest run src/components/keys/__tests__/UseKeyModal.spec.ts --pool=forks --poolOptions.forks.singleFork --reporter=verbose
```

预期：任务 3 新增的 UI 状态测试 PASS；后续 OMP 输出测试尚未添加。

---

### 任务 5：前端 OMP 输出测试

**文件：**
- 修改：`repos/sub2api/frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`

- [ ] **步骤 1：增加 OMP 成功输出测试**

新增测试：

```ts
it('renders OMP plugin, models.yml, and config.yml blocks with dynamic plugin version', async () => {
  const getModels = vi.mocked(keysAPI.getOpenCodeOpenAIModels)
  getModels.mockResolvedValueOnce({
    models: openaiModelsMock,
    omp_openai_provider_tools: {
      package: 'omp-openai-provider-tools',
      latest_version: '9.9.9',
      status: 'ok'
    }
  })
  const wrapper = mountOpenAIUseKeyModal()

  await clickClientTab(wrapper, 'keys.useKeyModal.cliTabs.omp')

  const codeBlocks = wrapper.findAll('pre code').map((code) => code.text())
  expect(codeBlocks).toHaveLength(3)

  expect(codeBlocks[0]).toContain('omp plugin install npm:omp-openai-provider-tools@9.9.9')
  expect(codeBlocks[0]).toContain('omp plugin doctor')
  expect(codeBlocks[0]).toContain('npx omp-openai-provider-tools configure-image-agent --model sub2api-openai-image/gpt-5.5-Sys --dry-run')
  expect(codeBlocks[0]).toContain('npx omp-openai-provider-tools configure-image-agent --model sub2api-openai-image/gpt-5.5-Sys')
  expect(codeBlocks[0]).toContain('--print')
  expect(codeBlocks[0]).not.toContain('omp-openai-provider-tools@latest')
  expect(codeBlocks[0]).not.toContain('omp plugin install npm:omp-openai-provider-tools\n')

  expect(codeBlocks[1]).toContain('providers:')
  expect(codeBlocks[1]).toContain('sub2api-openai:')
  expect(codeBlocks[1]).toContain('sub2api-openai-image:')
  expect(codeBlocks[1]).toContain('api: openai-responses')
  expect(codeBlocks[1]).toContain('baseUrl: https://example.com/v1')
  expect(codeBlocks[1]).toContain('apiKey: sk-test')
  expect(codeBlocks[1]).toContain('openaiProviderTools:')
  expect(codeBlocks[1]).toContain('enabled: true')
  expect(codeBlocks[1]).toContain('imageGeneration: true')
  expect(codeBlocks[1]).toContain('sub2api-openai-image/gpt-5.5-Sys: gpt-5.5-image-sys')
  expect(codeBlocks[1]).not.toContain('attachment')
  expect(codeBlocks[1]).not.toContain('tool_call')
  expect(codeBlocks[1]).not.toContain('structured_output')
  expect(codeBlocks[1]).not.toContain('temperature')
  expect(codeBlocks[1]).not.toContain('release_date')
  expect(codeBlocks[1]).not.toContain('variants')
  expect(codeBlocks[1]).not.toContain('modalities')
  expect(codeBlocks[1]).not.toContain('pdf')

  const ordinaryProvider = codeBlocks[1].slice(
    codeBlocks[1].indexOf('  sub2api-openai:'),
    codeBlocks[1].indexOf('  sub2api-openai-image:')
  )
  expect(ordinaryProvider).not.toContain('imageGeneration: true')

  expect(codeBlocks[2]).toContain('default: sub2api-openai/gpt-5.5-Sys')
  expect(codeBlocks[2]).toContain('smol: sub2api-openai/gpt-5.4-mini-Sys')
  expect(codeBlocks[2]).toContain('plan: sub2api-openai/gpt-5.5-Sys')
  expect(codeBlocks[2]).toContain('task: sub2api-openai/gpt-5.5-Sys:xhigh')
  expect(codeBlocks[2]).not.toContain('task: gpt-5.5-sys:xhigh')
  expect(codeBlocks[2]).not.toContain('smol: gpt-5.4-mini-sys')
})
```

- [ ] **步骤 2：增加 OMP unavailable 失败路径测试**

```ts
it('does not render a half plugin install command when OMP plugin version is unavailable', async () => {
  vi.mocked(keysAPI.getOpenCodeOpenAIModels).mockResolvedValueOnce({
    models: openaiModelsMock,
    omp_openai_provider_tools: {
      package: 'omp-openai-provider-tools',
      latest_version: '',
      status: 'unavailable',
      error: 'npm registry unavailable'
    }
  })
  const wrapper = mountOpenAIUseKeyModal()

  await clickClientTab(wrapper, 'keys.useKeyModal.cliTabs.omp')

  const text = wrapper.text()
  const codeBlocks = wrapper.findAll('pre code').map((code) => code.text())
  expect(text).toContain('keys.useKeyModal.omp.pluginVersionErrorHint')
  expect(codeBlocks.join('\n')).not.toContain('omp plugin install npm:omp-openai-provider-tools@')
  expect(codeBlocks.join('\n')).not.toContain('providers:')
})
```

- [ ] **步骤 3：运行前端测试验证失败**

运行：

```bash
pnpm exec vitest run src/components/keys/__tests__/UseKeyModal.spec.ts --pool=forks --poolOptions.forks.singleFork --reporter=verbose
```

预期：FAIL，因为 OMP generators 尚未实现。

---

### 任务 6：前端 OMP 输出生成器

**文件：**
- 修改：`repos/sub2api/frontend/src/components/keys/UseKeyModal.vue`
- 测试：`repos/sub2api/frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`

- [ ] **步骤 1：在 `currentFiles` 中加入 OMP 分支**

在 `currentFiles` 的 OpenCode branch 之后、平台 switch 之前加入：

```ts
if (activeClientTab.value === 'omp') {
  const pluginMetadata = openCodeMetadata.value?.omp_openai_provider_tools
  const pluginVersion = pluginMetadata?.latest_version?.trim() ?? ''
  if (openCodeLoading.value) {
    return [{ path: '1. Install OMP provider tools plugin', content: '# Loading OMP metadata...', hint: t('keys.useKeyModal.omp.loadingHint') }]
  }
  if (openCodeError.value || !openCodeModels.value) {
    return [{ path: '1. Install OMP provider tools plugin', content: '# OMP metadata unavailable', hint: openCodeError.value || t('keys.useKeyModal.omp.metadataErrorHint') }]
  }
  if (!pluginMetadata || !['ok', 'cached'].includes(pluginMetadata.status) || !pluginVersion) {
    return [{ path: '1. Install OMP provider tools plugin', content: '# OMP provider tools version unavailable', hint: t('keys.useKeyModal.omp.pluginVersionErrorHint') }]
  }
  return [
    generateOMPProviderToolsPluginInstructions(pluginVersion),
    generateOMPModelsConfig(apiBase, apiKey, openCodeModels.value, pluginVersion),
    generateOMPSettingsConfig()
  ]
}
```

- [ ] **步骤 2：实现插件说明 helper**

新增：

```ts
function generateOMPProviderToolsPluginInstructions(latestVersion: string): FileConfig {
  const content = `# 1. Install or upgrade provider-native tools plugin
omp plugin install npm:omp-openai-provider-tools@${latestVersion}

# 2. Check plugin health
omp plugin doctor

# 3. Preview the recommended image subagent template
npx omp-openai-provider-tools configure-image-agent --model sub2api-openai-image/gpt-5.5-Sys --dry-run

# 4. After reviewing the preview, write ~/.omp/agent/agents/image-generator.md
npx omp-openai-provider-tools configure-image-agent --model sub2api-openai-image/gpt-5.5-Sys

# If image_generator already exists, the command refuses to overwrite it.
# Use --print to inspect and merge manually; use --force only when you intentionally replace it.
npx omp-openai-provider-tools configure-image-agent --model sub2api-openai-image/gpt-5.5-Sys --print`

  return {
    path: '1. Install OMP provider tools plugin',
    content,
    hint: t('keys.useKeyModal.omp.pluginHint')
  }
}
```

- [ ] **步骤 3：实现 OMP model normalize helpers**

新增：

```ts
const yamlString = (value: string) => value

function normalizeOMPModelConfig(model: OpenCodeOpenAIModel) {
  const input = model.modalities?.input?.filter((item) => item === 'text' || item === 'image') ?? ['text']
  return {
    id: model.id,
    name: model.name,
    api: 'openai-responses',
    reasoning: model.reasoning,
    input,
    contextWindow: model.limit?.context,
    maxTokens: model.limit?.output,
    cost: model.cost
  }
}

function withOMPSysVariants(models: Record<string, ReturnType<typeof normalizeOMPModelConfig>>) {
  const expanded: Record<string, ReturnType<typeof normalizeOMPModelConfig>> = {}
  for (const [id, model] of Object.entries(models)) {
    expanded[id] = model
    expanded[`${id}-Sys`] = {
      ...model,
      id: `${model.id}-Sys`,
      name: `${model.name} (Sys)`,
      input: [...model.input],
      cost: model.cost ? { ...model.cost } : undefined
    }
  }
  return expanded
}
```

若 TypeScript 对 `ReturnType` 与 optional `cost` 推断不稳定，可定义显式接口 `OMPModelConfig`。

- [ ] **步骤 4：实现 YAML 渲染 helper**

为避免引入新依赖，使用确定性字符串生成。新增：

```ts
function renderOMPModelYaml(model: OMPModelConfig, indent = '      ', extraLines: string[] = []) {
  const lines = [
    `${indent}- id: ${yamlString(model.id)}`,
    `${indent}  name: ${yamlString(model.name)}`,
    `${indent}  api: openai-responses`,
    `${indent}  reasoning: ${model.reasoning ? 'true' : 'false'}`,
    `${indent}  input:`,
    ...model.input.map((item) => `${indent}    - ${item}`)
  ]
  if (model.contextWindow) lines.push(`${indent}  contextWindow: ${model.contextWindow}`)
  if (model.maxTokens) lines.push(`${indent}  maxTokens: ${model.maxTokens}`)
  if (model.cost) {
    const costEntries = [
      ['input', model.cost.input],
      ['output', model.cost.output],
      ['cacheRead', model.cost.cache_read],
      ['cacheWrite', model.cost.cache_write]
    ].filter(([, value]) => typeof value === 'number')
    if (costEntries.length > 0) {
      lines.push(`${indent}  cost:`)
      for (const [key, value] of costEntries) {
        lines.push(`${indent}    ${key}: ${value}`)
      }
    }
  }
  lines.push(...extraLines)
  return lines.join('\n')
}
```

- [ ] **步骤 5：实现 `buildOMPEquivalenceOverrides` 与 models.yml**

新增：

```ts
function buildOMPEquivalenceOverrides(models: Record<string, OMPModelConfig>) {
  const preferred = ['gpt-5.5', 'gpt-5.5-Sys', 'gpt-5.4-mini', 'gpt-5.4-mini-Sys']
  const lines = preferred
    .filter((id) => models[id])
    .map((id) => `    sub2api-openai/${id}: ${id.toLowerCase()}`)
  lines.push('    sub2api-openai-image/gpt-5.5-Sys: gpt-5.5-image-sys')
  return lines.join('\n')
}

function generateOMPModelsConfig(baseUrl: string, apiKey: string, openaiSource: Record<string, OpenCodeOpenAIModel>, pluginVersion: string): FileConfig {
  const baseModels = Object.fromEntries(
    Object.entries(openaiSource).map(([id, model]) => [id, normalizeOMPModelConfig(model)])
  )
  const models = withOMPSysVariants(baseModels)
  const selectedIds = ['gpt-5.5', 'gpt-5.5-Sys', 'gpt-5.4-mini', 'gpt-5.4-mini-Sys'].filter((id) => models[id])
  const selectedModelYaml = selectedIds.map((id) => renderOMPModelYaml(models[id])).join('\n')
  const imageModel = models['gpt-5.5-Sys'] ?? normalizeOMPModelConfig(openaiSource['gpt-5.5'])

  const imageYaml = renderOMPModelYaml(
    { ...imageModel, id: 'gpt-5.5-Sys', name: 'GPT-5.5 Image (Sys)' },
    '      ',
    [
      '        compat:',
      '          openaiProviderTools:',
      '            imageGeneration: true'
    ]
  )

  const content = `# Image generation and provider-native web_search require this plugin:
#   omp plugin install npm:omp-openai-provider-tools@${pluginVersion}
#   omp plugin doctor
# Recommended image subagent command:
#   npx omp-openai-provider-tools configure-image-agent --model sub2api-openai-image/gpt-5.5-Sys --dry-run
# Restart OMP after installing or upgrading the plugin.
providers:
  sub2api-openai:
    api: openai-responses
    baseUrl: ${baseUrl}
    apiKey: ${apiKey}
    compat:
      openaiProviderTools:
        enabled: true
    models:
${selectedModelYaml}

  sub2api-openai-image:
    api: openai-responses
    baseUrl: ${baseUrl}
    apiKey: ${apiKey}
    compat:
      openaiProviderTools:
        enabled: true
    models:
${imageYaml}

equivalence:
  overrides:
${buildOMPEquivalenceOverrides(models)}`

  return { path: '~/.omp/agent/models.yml', content, hint: t('keys.useKeyModal.omp.modelsHint') }
}
```

- [ ] **步骤 6：实现 `config.yml` helper**

新增：

```ts
function generateOMPSettingsConfig(): FileConfig {
  const content = `defaultThinkingLevel: xhigh
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
    plan: sub2api-openai/gpt-5.5-Sys:xhigh`

  return { path: '~/.omp/agent/config.yml', content, hint: t('keys.useKeyModal.omp.configHint') }
}
```

- [ ] **步骤 7：运行前端输出测试验证通过**

运行：

```bash
pnpm exec vitest run src/components/keys/__tests__/UseKeyModal.spec.ts --pool=forks --poolOptions.forks.singleFork --reporter=verbose
```

预期：PASS。

---

### 任务 7：最终验证与回归检查

**文件：**
- 所有已修改文件

- [ ] **步骤 1：运行后端聚焦测试**

运行：

```bash
go test -tags unit ./internal/service ./internal/server -run 'Test.*OpenCode.*|TestAPIContract_OpenCodeOpenAIModels' -count=1
```

工作目录：`repos/sub2api/backend`

预期：PASS。

- [ ] **步骤 2：运行前端聚焦测试**

运行：

```bash
pnpm exec vitest run src/components/keys/__tests__/UseKeyModal.spec.ts --pool=forks --poolOptions.forks.singleFork --reporter=verbose
```

工作目录：`repos/sub2api/frontend`

预期：PASS。

- [ ] **步骤 3：运行前端类型检查**

运行：

```bash
pnpm typecheck
```

工作目录：`repos/sub2api/frontend`

预期：PASS。

- [ ] **步骤 4：搜索禁止项**

使用搜索工具确认：

- `UseKeyModal.vue` 中 OMP install 命令不包含生产硬编码 `@0.1.2`。
- OMP `models.yml` 生成器不输出 `attachment`、`tool_call`、`structured_output`、`variants`、`modalities`、`pdf`。
- 代码或测试不包含真实 API key。

建议搜索模式：

```text
omp-openai-provider-tools@0\.1\.2|OPENAI_API_KEY.*sk-|sk-[A-Za-z0-9]{20,}|attachment|tool_call|structured_output|variants|modalities|pdf
```

对命中结果逐项判断：测试 fixture 的 `@0.1.2` 允许；OpenCode 生成器现有字段允许；OMP 生成器不允许。

- [ ] **步骤 5：检查 git 状态**

运行：

```bash
git status --short
```

工作目录：`repos/sub2api`

预期：只包含本计划内文件和规格/计划文档变更。
