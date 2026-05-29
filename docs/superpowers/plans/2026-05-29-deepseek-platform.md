# DeepSeek 平台集成实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Sub2API 中新增 DeepSeek 作为独立平台，支持 API Key 类型账号，复用 OpenAI 兼容转发逻辑，带模型白名单管理。

**Architecture:** DeepSeek 拥有独立的 `platform = "deepseek"` 常量。路由层将 DeepSeek 平台请求分发到 `OpenAIGatewayHandler`（与 OpenAI 平台共享同一 handler），因为 DeepSeek API 完全兼容 OpenAI 格式（`/v1/chat/completions`）。账号的 `credentials.base_url` 设为 `https://api.deepseek.com`，`credentials.api_key` 为 DeepSeek API Key。前端新增平台选择按钮和 API Key 表单。

**Tech Stack:** Go (Gin, Ent ORM), Vue 3, TypeScript, i18n

**Key architectural insight:** 当前路由根据 group platform 分发请求：
- `PlatformOpenAI` → `h.OpenAIGateway.Messages` / `h.OpenAIGateway.ChatCompletions`
- 其他平台 → `h.Gateway.Messages` / `h.Gateway.ChatCompletions`

DeepSeek 需要与 OpenAI 共享同一个 handler（因为格式兼容），但保持独立的 platform 标识（用于调度、模型白名单、配额等）。

---

### Task 1: 后端 — 新增 DeepSeek 平台常量

**Files:**
- Modify: `backend/internal/domain/constants.go`
- Modify: `backend/internal/service/domain_constants.go`
- Modify: `backend/internal/model/error_passthrough_rule.go`

- [ ] **Step 1: domain/constants.go 新增 DeepSeek 常量**

在 `PlatformAntigravity` 后面添加：
```go
PlatformDeepSeek    = "deepseek"
```

- [ ] **Step 2: service/domain_constants.go 新增常量 + AllowedQuotaPlatforms**

在 `PlatformAntigravity` 重新导出后面添加：
```go
PlatformDeepSeek    = domain.PlatformDeepSeek
```

在 `AllowedQuotaPlatforms` 切片中加入 `PlatformDeepSeek`。

- [ ] **Step 3: model/error_passthrough_rule.go 新增常量 + AllPlatforms**

在平台常量区添加 `PlatformDeepSeek = "deepseek"`，在 `AllPlatforms()` 返回值中加入。

- [ ] **Step 4: 编译验证**

```bash
cd backend && go build ./...
```
Expected: 编译成功

- [ ] **Step 5: Commit**

```bash
git add backend/internal/domain/constants.go backend/internal/service/domain_constants.go backend/internal/model/error_passthrough_rule.go
git commit -m "feat(deepseek): 新增 DeepSeek 平台常量定义"
```

---

### Task 2: 后端 — 新增 DeepSeek Token Provider + Account 方法

**Files:**
- Create: `backend/internal/service/deepseek_token_provider.go`
- Create: `backend/internal/service/deepseek_token_provider_test.go`
- Modify: `backend/internal/service/account.go`（IsAnthropic 方法后面）

- [ ] **Step 1: 创建 deepseek_token_provider.go**

```go
package service

import (
	"context"
	"errors"
	"strings"
)

var ErrDeepSeekOAuthNotImplemented = errors.New("deepseek OAuth not implemented")

// DeepSeekTokenProvider 管理 DeepSeek 账号访问凭证。
// 当前仅支持 API Key 模式，OAuth 模式预留接口。
type DeepSeekTokenProvider struct{}

func NewDeepSeekTokenProvider() *DeepSeekTokenProvider {
	return &DeepSeekTokenProvider{}
}

// GetAccessToken 返回 DeepSeek 账号的访问令牌。
// API Key 模式：直接返回 credentials.api_key。
// OAuth 模式：预留，返回 ErrDeepSeekOAuthNotImplemented。
func (p *DeepSeekTokenProvider) GetAccessToken(ctx context.Context, account *Account) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
	}
	if account.Platform != PlatformDeepSeek {
		return "", errors.New("not a deepseek account")
	}

	if account.Type == AccountTypeAPIKey {
		token := account.GetCredential("api_key")
		if strings.TrimSpace(token) == "" {
			return "", errors.New("api_key not found in credentials")
		}
		return token, nil
	}

	return "", ErrDeepSeekOAuthNotImplemented
}
```

- [ ] **Step 2: 创建 deepseek_token_provider_test.go**

测试：API Key 正常返回、Nil 账号报错、错误平台报错、缺失 API Key 报错、OAuth 类型返回 ErrDeepSeekOAuthNotImplemented。

- [ ] **Step 3: account.go 新增方法**

在 `IsAnthropic()` 方法后面添加：
```go
func (a *Account) IsDeepSeek() bool {
	return a.Platform == PlatformDeepSeek
}

func (a *Account) IsDeepSeekAPIKey() bool {
	return a.IsDeepSeek() && a.Type == AccountTypeAPIKey
}

func (a *Account) GetDeepSeekAPIKey() string {
	if !a.IsDeepSeekAPIKey() {
		return ""
	}
	return a.GetCredential("api_key")
}

func (a *Account) GetDeepSeekBaseURL() string {
	if !a.IsDeepSeek() {
		return ""
	}
	if a.Type == AccountTypeAPIKey {
		baseURL := a.GetCredential("base_url")
		if baseURL != "" {
			return baseURL
		}
	}
	return "https://api.deepseek.com"
}
```

- [ ] **Step 4: 运行测试**

```bash
cd backend && go test -tags=unit ./... -run TestDeepSeek -v
```

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/deepseek_token_provider.go backend/internal/service/deepseek_token_provider_test.go backend/internal/service/account.go
git commit -m "feat(deepseek): Token Provider + Account 判断方法"
```

---

### Task 3: 后端 — 路由层 DeepSeek 分发

**Files:**
- Modify: `backend/internal/server/routes/gateway.go`

**架构说明：** 当前 `gateway.go` 中，OpenAI 平台请求走 `OpenAIGatewayHandler`（支持 OpenAI 格式转发），其他平台走 `GatewayHandler`（Anthropic 格式）。DeepSeek API 兼容 OpenAI 格式，所以应该与 OpenAI 共享 handler。

- [ ] **Step 1: 修改 /v1/messages 路由**

将 `gateway.go` 中的条件从 `== service.PlatformOpenAI` 改为同时支持 OpenAI 和 DeepSeek：

```go
// /v1/messages: auto-route based on group platform
gateway.POST("/messages", func(c *gin.Context) {
    plat := getGroupPlatform(c)
    if plat == service.PlatformOpenAI || plat == service.PlatformDeepSeek {
        h.OpenAIGateway.Messages(c)
        return
    }
    h.Gateway.Messages(c)
})
```

- [ ] **Step 2: 修改 /v1/messages/count_tokens 路由**

DeepSeek 不支持 count_tokens（与 OpenAI 一致）：

```go
gateway.POST("/messages/count_tokens", func(c *gin.Context) {
    plat := getGroupPlatform(c)
    if plat == service.PlatformOpenAI || plat == service.PlatformDeepSeek {
        service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalFeatureGate)
        c.JSON(http.StatusNotFound, gin.H{...})
        return
    }
    h.Gateway.CountTokens(c)
})
```

- [ ] **Step 3: 修改 /v1/responses 路由**

```go
gateway.POST("/responses", func(c *gin.Context) {
    plat := getGroupPlatform(c)
    if plat == service.PlatformOpenAI || plat == service.PlatformDeepSeek {
        h.OpenAIGateway.Responses(c)
        return
    }
    h.Gateway.Responses(c)
})
```

- [ ] **Step 4: 修改 /v1/chat/completions 路由**

```go
gateway.POST("/chat/completions", func(c *gin.Context) {
    plat := getGroupPlatform(c)
    if plat == service.PlatformOpenAI || plat == service.PlatformDeepSeek {
        h.OpenAIGateway.ChatCompletions(c)
        return
    }
    h.Gateway.ChatCompletions(c)
})
```

- [ ] **Step 5: 修改不带 v1 前缀的别名路由（/responses、/chat/completions 等）**

在 `gateway.go` 中所有 `getGroupPlatform(c) == service.PlatformOpenAI` 判断处，添加 `|| getGroupPlatform(c) == service.PlatformDeepSeek`。

- [ ] **Step 6: 编译验证**

```bash
cd backend && go build ./...
```

- [ ] **Step 7: Commit**

```bash
git add backend/internal/server/routes/gateway.go
git commit -m "feat(deepseek): 路由层 DeepSeek 分发到 OpenAIGateway"
```

---

### Task 4: 后端 — OpenAI Gateway 增加 DeepSeek 平台支持

**Files:**
- Modify: `backend/internal/service/openai_gateway_service.go`（或相关调度/转发文件）
- Modify: `backend/internal/service/openai_account_scheduler.go`（DeepSeek 账号调度）

**关键检查点：** `OpenAIGatewayService` 的账号调度逻辑需要能调度 DeepSeek 平台账号。需要确认调度器是否按 platform 字符串过滤。

- [ ] **Step 1: 检查 OpenAI 账号调度器是否按 platform 硬编码**

检查 `openai_account_scheduler.go` 中是否有硬编码 `PlatformOpenAI` 的过滤。如果有，需要加入 `PlatformDeepSeek`。

- [ ] **Step 2: 在 OpenAI Gateway 的消息处理中加入 DeepSeek 平台识别**

检查 `openai_gateway_messages.go` 或相关方法中是否有 platform 检查。确保 DeepSeek 平台能进入账号调度流程。

- [ ] **Step 3: 确认 base_url 和 API Key 的获取**

`OpenAIGatewayService` 转发请求时使用 `account.GetBaseURL()` 或类似方法获取上游 URL，使用 `account.GetCredential("api_key")` 获取凭证。由于 DeepSeek API Key 账号的 credentials 中包含 `base_url` 和 `api_key`，这些已有的 accessor 应该能直接工作。

需要确认：`OpenAIGatewayService` 对 API Key 账号的转发 URL 构建是否使用 `GetBaseURL()` 返回的值（可被 custom base_url 覆盖）。如果是，DeepSeek 账号默认会使用 `https://api.deepseek.com`。

- [ ] **Step 4: 编译验证**

```bash
cd backend && go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/openai_*.go
git commit -m "feat(deepseek): OpenAI Gateway 支持 DeepSeek 平台调度"
```

---

### Task 5: 后端 — isModelSupportedByAccount 增加 DeepSeek 分支

**Files:**
- Modify: `backend/internal/service/gateway_service.go`（isModelSupportedByAccount 方法）

- [ ] **Step 1: 在 isModelSupportedByAccount 中处理 DeepSeek**

在 `isModelSupportedByAccount` 方法中（约 line 3736），DeepSeek 使用与 OpenAI 相同的模型支持策略（API Key 账号使用账号的 model_mapping）：

```go
// DeepSeek 平台：API Key 模式，使用账号 model_mapping
if account.Platform == PlatformDeepSeek && account.Type == AccountTypeAPIKey {
    return true // 允许，由 GetMappedModel 处理
}
```

- [ ] **Step 2: GetAccessToken 增加 DeepSeek 分支**

在 `GetAccessToken` 方法中（约 line 3764），DeepSeek API Key 走已有的 `AccountTypeAPIKey` 分支（直接取 `api_key`），无需额外修改。但如果后续要接入 DeepSeek Token Provider：

```go
case AccountTypeAPIKey:
    if account.Platform == PlatformDeepSeek {
        // 未来可接入 DeepSeekTokenProvider 做 OAuth
        apiKey := account.GetCredential("api_key")
        if apiKey == "" {
            return "", "", errors.New("api_key not found in credentials")
        }
        return apiKey, "apikey", nil
    }
    apiKey := account.GetCredential("api_key")
    ...
```

实际上当前 APIKey 分支已经是通用的，**无需额外修改**。

- [ ] **Step 3: 编译验证**

```bash
cd backend && go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add backend/internal/service/gateway_service.go
git commit -m "feat(deepseek): isModelSupportedByAccount 支持 DeepSeek"
```

---

### Task 6: 前端 — 类型定义 + CreateAccountModal

**Files:**
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/components/account/CreateAccountModal.vue`

- [ ] **Step 1: types/index.ts 增加 'deepseek'**

在第 690 行的 `AccountPlatform` 类型中加入 `'deepseek'`：
```ts
export type AccountPlatform = 'anthropic' | 'openai' | 'gemini' | 'antigravity' | 'deepseek'
```

- [ ] **Step 2: CreateAccountModal.vue — 平台选择按钮**

在 Antigravity 按钮后面添加 DeepSeek 按钮。找到平台选择区域的模板代码，添加：

```vue
<button
  type="button"
  @click="form.platform = 'deepseek'"
  :class="[
    'flex items-center gap-2 rounded-lg border-2 px-4 py-2.5 text-sm font-medium transition-all',
    form.platform === 'deepseek'
      ? 'border-cyan-500 bg-cyan-50 text-cyan-700 dark:bg-cyan-900/20 dark:text-cyan-400'
      : 'border-gray-200 hover:border-cyan-300 dark:border-dark-600 dark:hover:border-cyan-700'
  ]"
>
  <span class="flex h-6 w-6 items-center justify-center rounded-full bg-cyan-500 text-white text-xs font-bold">D</span>
  DeepSeek
</button>
```

- [ ] **Step 3: CreateAccountModal.vue — baseUrlHint 和 apiKeyHint**

在 `baseUrlHint` computed 中添加 DeepSeek 分支：
```ts
const baseUrlHint = computed(() => {
  if (form.platform === 'openai') return t('admin.accounts.openai.baseUrlHint')
  if (form.platform === 'gemini') return t('admin.accounts.gemini.baseUrlHint')
  if (form.platform === 'deepseek') return t('admin.accounts.deepseek.baseUrlHint')
  return t('admin.accounts.baseUrlHint')
})
```

在 `apiKeyHint` computed 中添加：
```ts
const apiKeyHint = computed(() => {
  if (form.platform === 'openai') return t('admin.accounts.openai.apiKeyHint')
  if (form.platform === 'gemini') return t('admin.accounts.gemini.apiKeyHint')
  if (form.platform === 'deepseek') return t('admin.accounts.deepseek.apiKeyHint')
  return t('admin.accounts.apiKeyHint')
})
```

- [ ] **Step 4: CreateAccountModal.vue — resetForm 默认 URL**

在 `resetForm` 函数中，`apiKeyBaseUrl.value` 的默认值需要支持 DeepSeek。检查当前逻辑：

```ts
const defaultBaseUrl =
  form.platform === 'openai'
    ? 'https://api.openai.com'
    : form.platform === 'gemini'
      ? 'https://generativelanguage.googleapis.com'
      : form.platform === 'deepseek'
        ? 'https://api.deepseek.com'
        : 'https://api.anthropic.com'
```

找到 handleSubmit 中的 `defaultBaseUrl` 变量（约 line 4477），添加 DeepSeek 分支。

- [ ] **Step 5: CreateAccountModal.vue — isOAuthFlow**

DeepSeek 当前只支持 API Key，不支持 OAuth。确保 `isOAuthFlow` 对 DeepSeek 返回 false。当前逻辑是 `accountCategory.value === 'oauth-based'` 才返回 true。需要在 DeepSeek 平台时强制 `accountCategory = 'apikey'` 或者在 `isOAuthFlow` 中添加：

```ts
const isOAuthFlow = computed(() => {
  if (form.platform === 'deepseek') return false
  // ... 原有逻辑
})
```

- [ ] **Step 6: Commit**

```bash
git add frontend/src/types/index.ts frontend/src/components/account/CreateAccountModal.vue
git commit -m "feat(deepseek): 前端类型 + 账号创建 UI"
```

---

### Task 7: 前端 — EditAccountModal + i18n + 模型白名单

**Files:**
- Modify: `frontend/src/components/account/EditAccountModal.vue`
- Modify: `frontend/src/locales/en.ts`（或对应的 i18n 文件）
- Modify: `frontend/src/locales/zh.ts`
- Modify: `frontend/src/composables/useModelWhitelist.ts`

- [ ] **Step 1: EditAccountModal.vue — baseUrl 和 apiKey placeholder**

在模板的 placeholder 条件链中加入 DeepSeek（约 line 38-44 和 60-66）：
```vue
:placeholder="
  account.platform === 'openai'
    ? 'https://api.openai.com'
    : account.platform === 'gemini'
      ? 'https://generativelanguage.googleapis.com'
      : account.platform === 'deepseek'
        ? 'https://api.deepseek.com'
        : account.platform === 'antigravity'
          ? 'https://cloudcode-pa.googleapis.com'
          : 'https://api.anthropic.com'
"
```

API Key placeholder 同样加入：
```vue
:placeholder="
  account.platform === 'openai'
    ? 'sk-proj-...'
    : account.platform === 'gemini'
      ? 'AIza...'
      : account.platform === 'deepseek'
        ? 'sk-...'
        : account.platform === 'antigravity'
          ? 'sk-...'
          : 'sk-ant-...'
"
```

- [ ] **Step 2: i18n — 添加 DeepSeek 翻译**

在 `frontend/src/locales/en.ts` 和 `frontend/src/locales/zh.ts` 的 `admin.accounts` 下添加：
```ts
deepseek: {
  baseUrlHint: 'Default: https://api.deepseek.com',
  apiKeyHint: 'Get your API key from https://platform.deepseek.com',
}
```

- [ ] **Step 3: useModelWhitelist.ts — DeepSeek 模型白名单 + 预设映射**

在 `deepseekModels` 数组（已存在，约 line 102）中加入 V4 模型：
```ts
const deepseekModels = [
  'deepseek-chat', 'deepseek-coder', 'deepseek-reasoner',
  'deepseek-v3', 'deepseek-v3-0324',
  'deepseek-v4-flash', 'deepseek-v4-pro',
  'deepseek-r1', 'deepseek-r1-0528',
  'deepseek-r1-distill-qwen-32b', 'deepseek-r1-distill-qwen-14b', 'deepseek-r1-distill-qwen-7b',
  'deepseek-r1-distill-llama-70b', 'deepseek-r1-distill-llama-8b'
]
```

在 `getPresetMappingsByPlatform` 函数中加入 DeepSeek 预设映射：
```ts
if (platform === 'deepseek') return deepseekPresetMappings
```

新增 `deepseekPresetMappings`：
```ts
const deepseekPresetMappings = [
  { label: 'V4 Flash', from: 'deepseek-v4-flash', to: 'deepseek-v4-flash', color: 'bg-cyan-100 text-cyan-700 hover:bg-cyan-200 dark:bg-cyan-900/30 dark:text-cyan-400' },
  { label: 'V4 Pro', from: 'deepseek-v4-pro', to: 'deepseek-v4-pro', color: 'bg-cyan-100 text-cyan-700 hover:bg-cyan-200 dark:bg-cyan-900/30 dark:text-cyan-400' },
  { label: 'V3', from: 'deepseek-v3', to: 'deepseek-v3', color: 'bg-teal-100 text-teal-700 hover:bg-teal-200 dark:bg-teal-900/30 dark:text-teal-400' },
  { label: 'R1', from: 'deepseek-r1', to: 'deepseek-r1', color: 'bg-purple-100 text-purple-700 hover:bg-purple-200 dark:bg-purple-900/30 dark:text-purple-400' },
  { label: 'Chat→V3', from: 'deepseek-chat', to: 'deepseek-v3', color: 'bg-amber-100 text-amber-700 hover:bg-amber-200 dark:bg-amber-900/30 dark:text-amber-400' },
]
```

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/account/EditAccountModal.vue frontend/src/locales/en.ts frontend/src/locales/zh.ts frontend/src/composables/useModelWhitelist.ts
git commit -m "feat(deepseek): 编辑 UI + i18n + 模型白名单"
```

---

### Task 8: 集成测试 & 验证

**Files:**
- Test: 端到端手动测试

- [ ] **Step 1: 启动后端**

```bash
cd backend && go run ./cmd/server/
```

- [ ] **Step 2: 启动前端**

```bash
cd frontend && npm run dev
```

- [ ] **Step 3: 验证 DeepSeek 账号创建**

1. 打开前端管理页面
2. 添加账号 → 选择 DeepSeek 平台
3. 确认 API Key 表单显示
4. 填入测试 API Key 和 base_url
5. 确认创建成功

- [ ] **Step 4: 验证模型白名单**

在账号配置页面确认 DeepSeek 模型列表包含 V4-Flash、V4-Pro 等。

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "chore: DeepSeek 平台集成完成"
```
